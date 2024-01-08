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

package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/netip"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/redis"
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// Shutdown 在退出时做 failover
//
// NOTE: 在4.0, 5.0中，在更新密码时，会重启实例。但是由于密码在重启前已经热更新，导致其他脚本无法连接到实例，包括shutdown脚本
// 为了解决这个问题，针对4,5 版本，会在重启前，先做failover，将master failover 到-0 节点。
// 由于重启是逆序的，最后一个pod启动成功之后，会使用新密码连接到 master，从而确保服务一直可用,切数据不会丢失
func Shutdown(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	var (
		podName = c.String("pod-name")
		timeout = time.Duration(c.Int("timeout")) * time.Second
	)
	if timeout == 0 {
		timeout = time.Second * 300
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	logger.Info("check local nodes.conf")
	nodes, err := getClusterNodes(c, ctx, client, false, logger)
	if err != nil {
		logger.Error(err, "parse nodes.conf with unknown error")
		return err
	}
	if nodes == nil {
		logger.Info("no nodes found")
		return nil
	}

	self := nodes.Self()
	if !self.IsJoined() {
		logger.Info("node not joined")
		return nil
	}

	authInfo, err := getRedisAuthInfo(c)
	if err != nil {
		logger.Error(err, "load redis operator user info failed")
		return err
	}

	if self.Role == redis.MasterRole {
		randInt := rand.Intn(50) + 1
		duration := time.Duration(randInt) * time.Second
		logger.Info(fmt.Sprintf("Wait for %s to escape failover conflict", duration))
		time.Sleep(duration)

		pods, _ := getPodsOfShard(ctx, c, client, logger)
		for _, pod := range pods {
			if pod.GetName() == podName {
				continue
			}
			if pod.GetDeletionTimestamp() != nil {
				continue
			}
			// ignore not ready pod
			if !func() bool {
				for _, cont := range pod.Status.ContainerStatuses {
					if cont.Name == "redis" && cont.Ready {
						return true
					}
				}
				return false
			}() {
				continue
			}

			logger.Info(fmt.Sprintf("check pod %s", pod.GetName()))
			if err := func() error {
				container := getContainerByName(pod.Spec.Containers, "redis")
				ipFamilyPrefer := func() string {
					for _, env := range container.Env {
						if env.Name == "IP_FAMILY_PREFER" {
							return env.Value
						}
					}
					return ""
				}()

				addr := net.JoinHostPort(pod.Status.PodIP, "6379")
				for _, podIp := range pod.Status.PodIPs {
					ip, _ := netip.ParseAddr(podIp.IP)
					if ip.Is6() && ipFamilyPrefer == string(v1.IPv6Protocol) {
						addr = net.JoinHostPort(podIp.IP, "6379")
						break
					} else if ip.Is4() && ipFamilyPrefer == string(v1.IPv4Protocol) {
						addr = net.JoinHostPort(podIp.IP, "6379")
						break
					}
				}

				logger.Info("check node", "pod", pod.GetName(), "addr", addr)
				redisClient := redis.NewClient(addr, *authInfo)
				defer redisClient.Close()

				nodes, err := redisClient.Nodes(ctx)
				if err != nil {
					logger.Error(err, "load cluster nodes failed")
					return err
				}
				currentNode := nodes.Self()
				if currentNode.MasterID == "" {
					return fmt.Errorf("unjoined node")
				} else if currentNode.MasterID == self.ID {
					mastrNode := nodes.Get(self.ID)
					action := NoFailoverAction
					if mastrNode.IsFailed() {
						action = ForceFailoverAction
					}
					if err := doRedisFailover(ctx, redisClient, action, logger); err != nil {
						return err
					}
				} else {
					err := fmt.Errorf("as if the shard got multi master")
					logger.Error(err, "failover aborted, let operator to fix this")
					return err
				}
				return nil
			}(); err != nil {
				continue
			}
			break
		}
	}

	// wait for some time for nodes to sync info
	time.Sleep(time.Second * 10)

	addr := net.JoinHostPort("local.inject", "6379")
	redisClient := redis.NewClient(addr, *authInfo)
	defer redisClient.Close()

	logger.Info("shutdown node")
	if _, err = redisClient.Do(ctx, "SHUTDOWN"); err != nil && !errors.Is(err, io.EOF) {
		logger.Error(err, "graceful shutdown failed")
	}
	return nil
}

func getContainerByName(containers []v1.Container, name string) *v1.Container {
	for _, container := range containers {
		if container.Name == name {
			return &container
		}
	}
	return nil
}
