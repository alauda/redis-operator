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

	"github.com/alauda/redis-operator/internal/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/internal/util"
	clientset "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.RedisSentinelReplication = (*RedisSentinelReplication)(nil)

type RedisSentinelReplication struct {
	appv1.StatefulSet
	client   clientset.ClientSet
	instance types.RedisSentinelInstance
	nodes    []redis.RedisSentinelNode
	logger   logr.Logger
}

func NewRedisSentinelReplication(ctx context.Context, client clientset.ClientSet, inst types.RedisSentinelInstance, logger logr.Logger) (*RedisSentinelReplication, error) {
	if client == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if inst == nil {
		return nil, fmt.Errorf("require sentinel instance")
	}

	name := sentinelbuilder.GetSentinelStatefulSetName(inst.GetName())
	sts, err := client.GetStatefulSet(ctx, inst.GetNamespace(), name)
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		logger.Info("load deployment failed", "name", name)
		return nil, err
	}

	node := RedisSentinelReplication{
		StatefulSet: *sts,
		client:      client,
		instance:    inst,
		logger:      logger.WithName("RedisSentinelReplication"),
	}
	if node.nodes, err = LoadRedisSentinelNodes(ctx, client, sts, inst.Users().GetOpUser(), logger); err != nil {

		logger.Error(err, "load shard nodes failed", "shard", sts.GetName())
		return nil, err
	}
	return &node, nil
}

func (s *RedisSentinelReplication) NamespacedName() client.ObjectKey {
	if s == nil {
		return client.ObjectKey{}
	}
	return client.ObjectKey{
		Namespace: s.GetNamespace(),
		Name:      s.GetName(),
	}
}

func (s *RedisSentinelReplication) Version() redis.RedisVersion {
	if s == nil {
		return redis.RedisVersionUnknown
	}
	container := util.GetContainerByName(&s.Spec.Template.Spec, sentinelbuilder.SentinelContainerName)
	ver, _ := redis.ParseRedisVersionFromImage(container.Image)
	return ver
}

func (s *RedisSentinelReplication) Definition() *appv1.StatefulSet {
	if s == nil {
		return nil
	}
	return &s.StatefulSet
}

func (s *RedisSentinelReplication) Nodes() []redis.RedisSentinelNode {
	if s == nil {
		return nil
	}
	return s.nodes
}

func (s *RedisSentinelReplication) Restart(ctx context.Context, annotationKeyVal ...string) error {
	// update all shards
	logger := s.logger.WithName("Restart")

	kv := map[string]string{
		"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339Nano),
	}
	for i := 0; i < len(annotationKeyVal)-1; i += 2 {
		kv[annotationKeyVal[i]] = annotationKeyVal[i+1]
	}

	data, _ := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": kv,
				},
			},
		},
	})

	if err := s.client.Client().Patch(ctx, &s.StatefulSet,
		client.RawPatch(k8stypes.StrategicMergePatchType, data)); err != nil {
		logger.Error(err, "restart deployment failed", "target", client.ObjectKeyFromObject(&s.StatefulSet))
		return err
	}
	return nil
}

func (s *RedisSentinelReplication) IsReady() bool {
	if s == nil {
		return false
	}
	return s.Status().ReadyReplicas == *s.Spec.Replicas && s.Status().UpdateRevision == s.Status().CurrentRevision
}

func (s *RedisSentinelReplication) Refresh(ctx context.Context) error {
	logger := s.logger.WithName("Refresh")

	var err error
	if s.nodes, err = LoadRedisSentinelNodes(ctx, s.client, &s.StatefulSet, s.instance.Users().GetOpUser(), logger); err != nil {
		logger.Error(err, "load shard nodes failed", "shard", s.GetName())
		return err
	}
	return nil
}

func (s *RedisSentinelReplication) Status() *appv1.StatefulSetStatus {
	if s == nil {
		return nil
	}
	return &s.StatefulSet.Status
}
