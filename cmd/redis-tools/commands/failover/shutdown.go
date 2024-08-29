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

package failover

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/commands"
	"github.com/alauda/redis-operator/pkg/redis"
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
)

// Shutdown 在退出时做 failover
//
// NOTE: 在4.0, 5.0中，在更新密码时，会重启实例。但是由于密码在重启前已经热更新，导致其他脚本无法连接到实例，包括shutdown脚本
// 为了解决这个问题，针对4,5 版本，会在重启前，先做failover，将master failover 到-0 节点。
// 由于重启是逆序的，最后一个pod启动成功之后，会使用新密码连接到 master，从而确保服务一直可用,切数据不会丢失
func Shutdown(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	var (
		podIPs  = c.String("pod-ips")
		name    = c.String("name")
		monitor = strings.ToLower(c.String("monitor-policy"))
		timeout = time.Duration(c.Int("timeout")) * time.Second
	)
	if timeout == 0 {
		timeout = time.Second * 300
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	authInfo, err := commands.LoadAuthInfo(c, ctx)
	if err != nil {
		logger.Error(err, "load redis operator user info failed")
		return err
	}
	senAuthInfo, err := commands.LoadMonitorAuthInfo(c, ctx, client)
	if err != nil {
		logger.Error(err, "load redis operator user info failed")
		return err
	}

	addr := net.JoinHostPort("local.inject", "6379")
	redisClient := redis.NewRedisClient(addr, *authInfo)
	defer redisClient.Close()

	info, err := func() (*redis.RedisInfo, error) {
		var (
			err  error
			info *redis.RedisInfo
		)
		for i := 0; i < 5; i++ {
			if info, err = redisClient.Info(ctx); err != nil {
				logger.Error(err, "get info failed, retry...")
				time.Sleep(time.Second)
				continue
			}
			break
		}
		return info, err
	}()
	if err != nil {
		logger.Error(err, "check node role failed, abort auto failover")
		return err
	}

	if info.Role == "master" && monitor == "sentinel" {
		err = func() error {
			serveAddresses := map[string]struct{}{}
			if podIPs != "" {
				for _, ip := range strings.Split(podIPs, ",") {
					serveAddresses[net.JoinHostPort(ip, "6379")] = struct{}{}
				}
			}
			if config, err := func() (map[string]string, error) {
				var (
					err error
					cfg map[string]string
				)
				for i := 0; i < 5; i++ {
					cfg, err = redisClient.ConfigGet(ctx, "*")
					if err != nil {
						logger.Error(err, "get config failed")
						time.Sleep(time.Second)
						continue
					}
					break
				}
				return cfg, err
			}(); err != nil {
				logger.Error(err, "get config failed ")
			} else {
				ip, port := config["replica-announce-ip"], config["replica-announce-port"]
				if ip != "" && port != "" {
					serveAddresses[net.JoinHostPort(ip, port)] = struct{}{}
				}
			}

			var sentinelNodes []string
			if val := c.String("monitor-uri"); val == "" {
				logger.Error(err, "require monitor uri")
				return errors.New("require monitor uri")
			} else if u, err := url.Parse(val); err != nil {
				logger.Error(err, "parse monitor uri failed")
				return err
			} else {
				sentinelNodes = strings.Split(u.Host, ",")
			}

			var senClient redis.RedisClient
			for _, node := range sentinelNodes {
				if senClient, err = func() (redis.RedisClient, error) {
					senClient := redis.NewRedisClient(node, *senAuthInfo)
					if _, err := senClient.DoWithTimeout(ctx, time.Second, "PING"); err != nil {
						logger.Error(err, "ping node failed")
						senClient.Close()
						return nil, err
					}
					return senClient, nil
				}(); senClient != nil {
					break
				}
			}
			if senClient == nil {
				logger.Error(err, "get sentinel client failed")
				return err
			}
			defer senClient.Close()

		__FAILOVER_END__:
			for i := 0; ; i++ {
				info, err := senClient.Info(ctx)
				if err != nil {
					if strings.Contains(err.Error(), "no such host") {
						logger.Error(err, "sentinel node is down, give up manual failover")
						break __FAILOVER_END__
					}
					logger.Error(err, "get info failed")
					time.Sleep(time.Second)
					continue
				}
				addr := info.SentinelMaster0.Address.String()
				if _, ok := serveAddresses[addr]; !ok {
					logger.Info("current node is not master, skip failover", "master", addr)
					break __FAILOVER_END__
				}
				if info.SentinelMaster0.Replicas == 0 {
					logger.Info("current master has no replicas, skip failover")
					break __FAILOVER_END__
				}

				logger.Info("current node is master, try failover", "master", addr)
				if _, err = senClient.DoWithTimeout(ctx, time.Second, "SENTINEL", "FAILOVER", name); err != nil {
					if strings.Contains(err.Error(), "no such host") {
						logger.Error(err, "sentinel node is down, give up manual failover")
						break __FAILOVER_END__
					} else if strings.Contains(err.Error(), "INPROG Failover already in progress") {
						logger.Info("failover in progress")
					} else {
						logger.Error(err, "do failover failed, retry in 10s")
						time.Sleep(time.Second * 10)
						continue
					}
				}
				for j := 0; j < 6; j++ {
					if info, err = senClient.Info(ctx); err != nil {
						if strings.Contains(err.Error(), "no such host") {
							logger.Error(err, "sentinel node is down, give up manual failover")
							break __FAILOVER_END__
						}
						logger.Error(err, "get info failed")
					} else {
						addr := info.SentinelMaster0.Address.String()
						if _, ok := serveAddresses[addr]; !ok {
							logger.Info("failover success", "master", addr)
							break __FAILOVER_END__
						}
					}
					time.Sleep(time.Second * 5)
				}
			}
			return nil
		}()
	}

	time.Sleep(time.Second * 10)

	logger.Info("do shutdown node")
	// NOTE: here set timeout to 300s, which will try best to do a shutdown snapshot
	// if the data is too large, this snapshot may not be completed
	if _, err := redisClient.DoWithTimeout(ctx, time.Second*300, "SHUTDOWN"); err != nil && !errors.Is(err, io.EOF) {
		logger.Error(err, "graceful shutdown failed")
	}
	return err
}
