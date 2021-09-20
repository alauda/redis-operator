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
	"encoding/json"
	"fmt"
	"time"

	clientset "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/pkg/models"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.RedisSentinelReplica = (*RedisSentinelReplica)(nil)

type RedisSentinelReplica struct {
	appv1.StatefulSet
	client   clientset.ClientSet
	sentinel types.RedisInstance
	nodes    []redis.RedisNode

	logger logr.Logger
}

func LoadRedisSentinelReplicas(ctx context.Context, client clientset.ClientSet, sentinel types.RedisInstance, logger logr.Logger) ([]types.RedisSentinelReplica, error) {

	var shards []types.RedisSentinelReplica
	name := sentinelbuilder.GetSentinelStatefulSetName(sentinel.GetName())
	sts, err := client.GetStatefulSet(ctx, sentinel.GetNamespace(), name)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("load statefulset not found", "name", name)
			return shards, nil
		}
		logger.Info("load statefulset failed", "name", name)
		return nil, err
	}
	if node, err := NewRedisSentinelReplica(ctx, client, sentinel, sts, logger); err != nil {
		logger.Error(err, "parse shard failed")
	} else {
		shards = append(shards, node)
	}
	return shards, nil
}

func (s *RedisSentinelReplica) Version() redis.RedisVersion {
	if s == nil {
		return redis.RedisVersionUnknown
	}

	container := util.GetContainerByName(&s.Spec.Template.Spec, sentinelbuilder.ServerContainerName)
	ver, _ := redis.ParseRedisVersionFromImage(container.Image)
	return ver
}

func NewRedisSentinelReplica(ctx context.Context, client clientset.ClientSet, sentinel types.RedisInstance, sts *appv1.StatefulSet, logger logr.Logger) (types.RedisSentinelReplica, error) {
	if client == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if sentinel == nil {
		return nil, fmt.Errorf("require cluster instance")
	}
	if sts == nil {
		return nil, fmt.Errorf("require statefulset")
	}

	sentinelNode := &RedisSentinelReplica{
		StatefulSet: *sts,
		client:      client,
		sentinel:    sentinel,
		logger:      logger,
	}

	users := sentinel.Users()
	var err error
	if sentinelNode.nodes, err = models.LoadRedisNodes(ctx, client, sts, users.GetOpUser(), logger); err != nil {
		logger.Error(err, "load shard nodes failed", "shard", sts.GetName())
		return nil, err
	}
	return sentinelNode, nil
}

func (s *RedisSentinelReplica) Nodes() []redis.RedisNode {
	if s == nil {
		return nil
	}
	return s.nodes
}

func (s *RedisSentinelReplica) Replicas() []redis.RedisNode {
	if s == nil || len(s.nodes) == 0 {
		return nil
	}
	var replicas []redis.RedisNode
	for _, node := range s.nodes {
		if node.Role() == redis.RedisRoleSlave {
			replicas = append(replicas, node)
		}
	}
	return replicas
}

func (s *RedisSentinelReplica) Master() redis.RedisNode {
	if s == nil || len(s.nodes) == 0 {
		return nil
	}

	var master redis.RedisNode
	for _, node := range s.nodes {
		// if the node joined, and is master, then it's the master
		if node.Role() == redis.RedisRoleMaster {
			master = node
		}
	}
	// the master node may failed, or is a new cluster without slots assigned
	return master
}

func (s *RedisSentinelReplica) Restart(ctx context.Context) error {
	// update all shards
	logger := s.logger.WithName("Restart")

	data, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{
						"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339Nano),
					},
				},
			},
		},
	})

	if err := s.client.Client().Patch(ctx, &s.StatefulSet,
		client.RawPatch(k8stypes.StrategicMergePatchType, data)); err != nil {
		logger.Error(err, "restart statefulset failed", "target", client.ObjectKeyFromObject(&s.StatefulSet))
		return err
	}
	return nil
}

func (s *RedisSentinelReplica) Refresh(ctx context.Context) error {
	logger := s.logger.WithName("Refresh")

	var err error
	if s.nodes, err = models.LoadRedisNodes(ctx, s.client, &s.StatefulSet, s.sentinel.Users().GetOpUser(), logger); err != nil {
		logger.Error(err, "load shard nodes failed", "shard", s.GetName())
		return err
	}
	return nil
}

func (s *RedisSentinelReplica) Status() *appv1.StatefulSetStatus {
	if s == nil {
		return nil
	}
	return &s.StatefulSet.Status
}

func (s *RedisSentinelReplica) Definition() *appv1.StatefulSet {
	if s == nil {
		return nil
	}
	return &s.StatefulSet
}
