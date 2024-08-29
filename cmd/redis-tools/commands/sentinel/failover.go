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

package sentinel

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/commands"
	"github.com/alauda/redis-operator/pkg/redis"
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
)

func Failover(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	var (
		sentinelUri       = c.String("monitor-uri")
		name              = c.String("name")
		failoverAddresses = c.StringSlice("escape")
	)
	if sentinelUri == "" {
		return nil
	}

	var sentinelNodes []string
	if val, err := url.Parse(sentinelUri); err != nil {
		return cli.Exit(fmt.Sprintf("parse sentinel uri failed, error=%s", err), 1)
	} else {
		sentinelNodes = strings.Split(val.Host, ",")
	}

	senAuthInfo, err := commands.LoadMonitorAuthInfo(c, ctx, client)
	if err != nil {
		logger.Error(err, "load sentinel auth info failed")
		return cli.Exit(fmt.Sprintf("load sentinel auth info failed, error=%s", err), 1)
	}

	var redisCli redis.RedisClient
	for _, node := range sentinelNodes {
		redisCli = redis.NewRedisClient(node, *senAuthInfo)
		if _, err = redisCli.Do(ctx, "PING"); err != nil {
			logger.Error(err, "ping sentinel node failed", "node", node)
			redisCli.Close()
			redisCli = nil
			continue
		}
		break
	}
	if redisCli == nil {
		return cli.Exit("no sentinel node available", 1)
	}
	defer redisCli.Close()

	if info, err := redisCli.Info(ctx); err != nil {
		logger.Error(err, "get sentinel info failed")
		return cli.Exit("get sentinel info failed", 1)
	} else {
		currentMaster := info.SentinelMaster0.Address.String()
		logger.Info("check current master", "addr", currentMaster)
		if currentMaster != "" {
			if info.SentinelMaster0.Status == "ok" {
				if !slices.Contains(failoverAddresses, currentMaster) {
					logger.Info("current master is ok, no need to failover")
					return nil
				}
			} else {
				logger.Info("current master is not healthy, try failover", "addr", currentMaster)
				failoverAddresses = append(failoverAddresses, currentMaster)
			}
		} else {
			logger.Info("master not registered", "name", name)
			return nil
		}
	}

	needFailover := func() (bool, error) {
		logger.Info("assess if failover is required")
		info, err := redisCli.Info(ctx)
		if err != nil {
			logger.Error(err, "get sentinel info failed")
			return false, err
		}
		logger.Info("current master", "master", info.SentinelMaster0)
		if info.SentinelMaster0.Name != name {
			logger.Info("master not registered yet, abort failover")
			return false, nil
		}
		masterAddr := info.SentinelMaster0.Address.String()
		if slices.Contains(failoverAddresses, masterAddr) || info.SentinelMaster0.Status != "ok" {
			// check slaves
			if info.SentinelMaster0.Replicas == 0 {
				logger.Info("no suitable replica to promote, abort failover")
				return false, nil
			}
			// TODO: check slaves status
			return true, nil
		}
		return false, nil
	}

	// get current master
	failoverSucceed := false
__FAILOVER_END__:
	for i := 0; i < 6; i++ {
		if nf, err := needFailover(); err != nil {
			logger.Error(err, "check failover failed, retry later")
			time.Sleep(time.Second * 5)
			continue
		} else if nf {
			if _, err := redisCli.Do(ctx, "SENTINEL", "FAILOVER", name); err != nil {
				logger.Error(err, "do sentinel failover failed, retry later")
				time.Sleep(time.Second * 5)
				continue
			}
			for j := 0; j < 6; j++ {
				if nf, err := needFailover(); err != nil {
					time.Sleep(time.Second * 5)
					continue
				} else if nf {
					time.Sleep(time.Second * 5)
					continue
				} else {
					failoverSucceed = true
					break __FAILOVER_END__
				}
			}
		} else {
			failoverSucceed = true
			break
		}
	}

	if !failoverSucceed {
		logger.Info("failover failed, start redis node directly")
		return cli.Exit("failover failed", 1)
	}
	return nil
}
