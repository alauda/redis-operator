/*
Copyright 2023 The RedisOperator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/redis"
	types "github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RedisInstancePersistence(ctx context.Context, mgrCli client.Client, namespace string, logger logr.Logger) error {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		// cluster instance
		var (
			listResp  v1alpha1.DistributedRedisClusterList
			cursor    string
			maxDbSize int64
		)
		for {
			if err := mgrCli.List(ctx, &listResp, &client.ListOptions{Limit: 100, Namespace: namespace, Continue: cursor}); err != nil {
				logger.Error(err, "load redis cluster instance failed")
				break
			}

			for _, ins := range listResp.Items {
				if ins.Spec.Config["save"] == "" || ins.Spec.Storage == nil || ins.Spec.Storage.Size.IsZero() {
					continue
				}
				logger.Info("do persistence for DistributedRedisCluster", "instance", ins.Name)

				var password string
				if ins.Spec.PasswordSecret != nil && ins.Spec.PasswordSecret.Name != "" {
					var secret v1.Secret
					if err := mgrCli.Get(ctx, client.ObjectKey{
						Namespace: ins.Namespace,
						Name:      ins.Spec.PasswordSecret.Name,
					}, &secret); err != nil {
						logger.Error(err, "get redis password secret failed")
						continue
					}
					password = string(secret.Data["password"])
				}

				for _, node := range ins.Status.Nodes {
					if node.Role != types.RedisRoleMaster {
						continue
					}
					func() {
						redisCli := redis.NewRedisClient(net.JoinHostPort(node.IP, node.Port), redis.AuthConfig{
							Password: password,
						})
						defer redisCli.Close()

						nctx, cancel := context.WithTimeout(ctx, time.Second*10)
						defer cancel()

						info, _ := redisCli.Info(ctx)
						if info != nil && info.UsedMemoryDataset > maxDbSize {
							maxDbSize = info.UsedMemoryDataset
						}
						if _, err := redisCli.Do(nctx, "BGSAVE"); err != nil && err != redis.ErrNil && err != io.EOF {
							logger.Error(err, "redis instance bgsave failed", "instance", ins.Name, "node", net.JoinHostPort(node.IP, node.Port))
						}
					}()
				}
			}

			cursor = listResp.Continue
			if cursor != "" {
				continue
			}
			break
		}

		duration := time.Duration((float64(maxDbSize)/1024/1024/1024)*10) * time.Second
		if duration > 0 {
			// do best effort to make sure persistence is done
			logger.Info(fmt.Sprintf("wait %d for cluster persistence done", duration))
			time.Sleep(duration)
		}
	}()

	go func() {
		defer wg.Done()

		// sentinel instance
		var (
			listResp  databasesv1.RedisFailoverList
			cursor    string
			maxDbSize int64
		)
		for {
			if err := mgrCli.List(ctx, &listResp, &client.ListOptions{Limit: 100, Namespace: namespace, Continue: cursor}); err != nil {
				logger.Error(err, "load redis sentinel instance failed")
				break
			}

			for _, ins := range listResp.Items {
				if ins.Spec.Redis.CustomConfig["save"] == "" || ins.Spec.Redis.Storage.PersistentVolumeClaim == nil {
					continue
				}
				logger.Info("do persistence for RedisFailover", "instance", ins.Name)

				var password string
				if ins.Spec.Auth.SecretPath != "" {
					var secret v1.Secret
					if err := mgrCli.Get(ctx, client.ObjectKey{
						Namespace: ins.Namespace,
						Name:      ins.Spec.Auth.SecretPath,
					}, &secret); err != nil {
						logger.Error(err, "get redis password secret failed")
						continue
					}
					password = string(secret.Data["password"])
				}
				if addr := ins.Status.Master.Address; addr != "" {
					redisCli := redis.NewRedisClient(addr, redis.AuthConfig{
						Password: password,
					})
					defer redisCli.Close()

					nctx, cancel := context.WithTimeout(ctx, time.Second*1)
					defer cancel()

					info, _ := redisCli.Info(ctx)
					if info != nil && info.UsedMemoryDataset > maxDbSize {
						maxDbSize = info.UsedMemoryDataset
					}

					if _, err := redisCli.Do(nctx, "BGSAVE"); err != nil && err != redis.ErrNil && err != io.EOF {
						logger.Error(err, "redis instance bgsave failed", "instance", ins.Name, "node", addr)
					}
				}
			}

			cursor = listResp.Continue
			if cursor != "" {
				continue
			}
			break
		}

		duration := time.Duration((float64(maxDbSize)/1024/1024/1024)*10) * time.Second
		if duration > 0 {
			// do best effort to make sure persistence is done
			logger.Info(fmt.Sprintf("wait %d for sentinel persistence done", duration))
			time.Sleep(duration)
		}
	}()
	wg.Wait()

	time.Sleep(3 * time.Second)

	return nil
}
