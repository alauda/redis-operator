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

type RedisSentinelNodes struct {
	appv1.Deployment
	client   clientset.ClientSet
	sentinel types.RedisInstance
	nodes    []redis.RedisNode
	logger   logr.Logger
}

func LoadRedisSentinelNodes(ctx context.Context, client clientset.ClientSet, sentinel types.RedisInstance, logger logr.Logger) (types.RedisSentinelNodes, error) {
	name := sentinelbuilder.GetSentinelDeploymentName(sentinel.GetName())
	var shard types.RedisSentinelNodes
	deploy, err := client.GetDeployment(ctx, sentinel.GetNamespace(), name)
	if err != nil {
		if errors.IsNotFound(err) {
			return shard, nil
		}
		logger.Info("load deployment failed", "name", name)
		return nil, err
	}
	if shard, err = NewRedisSentinelNode(ctx, client, sentinel, deploy, logger); err != nil {
		logger.Error(err, "parse shard failed")
	}
	return shard, nil
}

func (s *RedisSentinelNodes) Version() redis.RedisVersion {
	if s == nil {
		return redis.RedisVersionUnknown
	}
	container := util.GetContainerByName(&s.Spec.Template.Spec, sentinelbuilder.ServerContainerName)
	ver, _ := redis.ParseRedisVersionFromImage(container.Image)
	return ver
}

func NewRedisSentinelNode(ctx context.Context, client clientset.ClientSet, sentinel types.RedisInstance, deploy *appv1.Deployment, logger logr.Logger) (types.RedisSentinelNodes, error) {
	if client == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if sentinel == nil {
		return nil, fmt.Errorf("require sentinel instance")
	}
	if deploy == nil {
		return nil, fmt.Errorf("require deployment")
	}

	node := RedisSentinelNodes{
		Deployment: *deploy,
		client:     client,
		sentinel:   sentinel,
		logger:     logger.WithName("SentinelNode"),
	}
	user := sentinel.Users()
	var err error
	if node.nodes, err = models.LoadRedisSentinelNodes(ctx, client, deploy, user.GetOpUser(), logger); err != nil {
		logger.Error(err, "load shard nodes failed", "shard", deploy.GetName())
		return nil, err
	}
	return &node, nil
}

func (s *RedisSentinelNodes) Definition() *appv1.Deployment {
	if s == nil {
		return nil
	}
	return &s.Deployment
}

func (s *RedisSentinelNodes) Nodes() []redis.RedisNode {
	if s == nil {
		return nil
	}
	return s.nodes
}

func (s *RedisSentinelNodes) Restart(ctx context.Context) error {
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

	if err := s.client.Client().Patch(ctx, &s.Deployment,
		client.RawPatch(k8stypes.StrategicMergePatchType, data)); err != nil {
		logger.Error(err, "restart deployment failed", "target", client.ObjectKeyFromObject(&s.Deployment))
		return err
	}
	return nil
}

func (s *RedisSentinelNodes) Refresh(ctx context.Context) error {
	logger := s.logger.WithName("Refresh")

	var err error
	if s.nodes, err = models.LoadRedisSentinelNodes(ctx, s.client, &s.Deployment, s.sentinel.Users().GetOpUser(), logger); err != nil {
		logger.Error(err, "load shard nodes failed", "shard", s.GetName())
		return err
	}
	return nil
}

func (s *RedisSentinelNodes) Status() *appv1.DeploymentStatus {
	if s == nil {
		return nil
	}
	return &s.Deployment.Status
}
