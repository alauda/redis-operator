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

package sync

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/commands/cluster"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/redis"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/sync"
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func getRedisNodeInfo(redisClient redis.Client, logger logr.Logger) (*redis.Node, []*redis.Node, error) {
	nodes, err := redisClient.Nodes(context.TODO())
	if err != nil {
		return nil, nil, err
	}
	return nodes.Self(), nodes, nil
}

/*
func redisFailover(redisClient redis.Client, logger logr.Logger) error {
	self, _, err := getRedisNodeInfo(redisClient, logger)
	if err != nil {
		return err
	}

	logger.Info("try to do redis failover", "detail", self)
	if self.Role == redis.MasterRole {
		return nil
	}

	if _, err := redisClient.Do(context.TODO(), "cluster", "failover", "force"); err != nil {
		return err
	}

	for i := 0; ; i++ {
		time.Sleep(time.Second * 5)

		self, _, err := getRedisNodeInfo(redisClient, logger)
		if err != nil || self == nil {
			return err
		}
		if self.Role == redis.MasterRole {
			return nil
		}
		if i > 6 {
			logger.Error(fmt.Errorf("failover blocked"), "failover take too long time, please check cluster status manually and do manual failover. When fixed, restart the pod again", "addr", self.Addr)
		}
	}
}
*/

type FailoverAction string

const (
	NoFailoverAction       FailoverAction = ""
	ForceFailoverAction    FailoverAction = "FORCE"
	TakeoverFailoverAction FailoverAction = "TAKEOVER"
)

func doRedisFailover(ctx context.Context, cli redis.Client, action FailoverAction, logger logr.Logger) (err error) {
	args := []interface{}{"FAILOVER"}
	if action != "" {
		args = append(args, action)
	}
	if _, err := cli.Do(ctx, "CLUSTER", args...); err != nil {
		logger.Error(err, "do failover failed", "action", action)
		return err
	}

	logger.Info("wait 5s for failover")
	time.Sleep(time.Second * 5)

	nodes, err := cli.Nodes(ctx)
	if err != nil {
		logger.Error(err, "fetch cluster nodes failed")
		return err
	}
	self := nodes.Self()
	if self == nil {
		return fmt.Errorf("get nodes info failed, as if the nodes.conf is broken")
	}
	if self.Role == redis.MasterRole {
		logger.Info("failover succeed")
		return nil
	}
	if action == ForceFailoverAction {
		return doRedisFailover(ctx, cli, TakeoverFailoverAction, logger)
	}
	return fmt.Errorf("do manual failover failed")
}

// Sync
func SyncToLocal(ctx context.Context, client *kubernetes.Clientset, namespace, name, workspace, target string, redisOptions redis.AuthInfo, logger logr.Logger) error {
	// check if node.conf exists
	targetFile := path.Join(workspace, target)
	nodesConfData := ""
	if data, err := os.ReadFile(targetFile); err != nil && !os.IsNotExist(err) {
		logger.Error(err, "load local config failed")
		return err
	} else if len(data) == 0 {
		var cm *v1.ConfigMap
		if err := cluster.RetryGet(func() (err error) {
			cm, err = client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
			return
		}, 3); errors.IsNotFound(err) {
			return nil
		} else if err != nil {
			logger.Error(err, "get configmap failed", "name", name)
			return err
		}
		nodesConfData = cm.Data[target]
	} else {
		if err := os.WriteFile(fmt.Sprintf("%s.bak.1", targetFile), data, 0644); err != nil {
			logger.Error(err, "backup file failed", "file", targetFile)
		}
		nodesConfData = string(data)
	}

	if len(nodesConfData) > 0 {
		nodes := redis.ParseNodes(string(nodesConfData))
		self := nodes.Self()
		if self == nil {
			logger.Error(fmt.Errorf("get self node info failed"), "invalid node config file")
			return nil
		}
		slaves := nodes.Replicas(self.ID)

		logger.Info("checking node", "id", self.ID, "role", self.Role, "slaves", slaves)
		if self.Role == redis.MasterRole {
			var (
				newMaster  *redis.Node
				firstSlave *redis.Node
			)
			for _, slave := range slaves {
				if end := func() bool {
					redisClient := redis.NewClient(slave.Addr, redisOptions)
					defer redisClient.Close()

					slaveNodeInfo, _, err := getRedisNodeInfo(redisClient, logger)
					if err != nil {
						logger.Error(err, "get slave info failed", "addr", slave.Addr)
						return false
					}
					if slaveNodeInfo != nil && slaveNodeInfo.Role == redis.MasterRole {
						newMaster = slaveNodeInfo
						return true
					}
					if firstSlave == nil {
						firstSlave = slaveNodeInfo
					}
					return false
				}(); end {
					break
				}
			}

			if newMaster == nil && firstSlave != nil {
				redisClient := redis.NewClient(firstSlave.Addr, redisOptions)
				defer redisClient.Close()

				if err := doRedisFailover(ctx, redisClient, ForceFailoverAction, logger); err != nil {
					logger.Error(err, "redis failover failed", "addr", firstSlave.Addr)
				} else {
					logger.Info("failover succeed", "master", firstSlave.Addr)
				}
			}
		}
		_ = os.WriteFile(targetFile, []byte(nodesConfData), 0644)
	}
	return nil
}

func WatchAndSync(ctx context.Context, client *kubernetes.Clientset, namespace, name, workspace, target string,
	syncInterval int64, ownerRefs []metav1.OwnerReference, logger logr.Logger) error {

	ctrl, err := sync.NewController(client, sync.ControllerOptions{
		Namespace:       namespace,
		ConfigMapName:   name,
		OwnerReferences: ownerRefs,
		SyncInterval:    time.Duration(syncInterval) * time.Second,
		Filters:         []sync.Filter{&sync.RedisClusterFilter{}},
	}, logger)
	if err != nil {
		return err
	}
	fileWathcer, _ := sync.NewFileWatcher(ctrl.Handler, logger)

	if err := fileWathcer.Add(path.Join(workspace, target)); err != nil {
		logger.Error(err, "watch file failed, error=%s")
		return cli.Exit(err, 1)
	}

	go func() {
		_ = fileWathcer.Run(ctx)
	}()
	return ctrl.Run(ctx)
}
