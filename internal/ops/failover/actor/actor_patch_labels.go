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

package actor

import (
	"context"
	"net"
	"slices"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/internal/builder/failoverbuilder"
	ops "github.com/alauda/redis-operator/internal/ops/failover"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/go-logr/logr"
)

var _ actor.Actor = (*actorPatchLabels)(nil)

func init() {
	actor.Register(core.RedisSentinel, NewPatchLabelsActor)
}

func NewPatchLabelsActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorPatchLabels{
		client: client,
		logger: logger,
	}
}

type actorPatchLabels struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorPatchLabels) Version() *semver.Version {
	return semver.MustParse("3.18.0")
}

func (a *actorPatchLabels) SupportedCommands() []actor.Command {
	return []actor.Command{ops.CommandPatchLabels}
}

func (a *actorPatchLabels) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	logger := val.Logger().WithValues("actor", ops.CommandPatchLabels.String())
	inst := val.(types.RedisFailoverInstance)

	masterNode, err := inst.Monitor().Master(ctx)
	if err != nil {
		logger.Error(err, "get master failed")
		actor.RequeueWithError(err)
	}

	pods, err := inst.RawNodes(ctx)
	if err != nil {
		logger.Error(err, "get pods failed")
		return actor.RequeueWithError(err)
	}
	masterAddr := net.JoinHostPort(masterNode.IP, masterNode.Port)

	for _, pod := range pods {
		var node redis.RedisNode
		_ = slices.IndexFunc(inst.Nodes(), func(i redis.RedisNode) bool {
			if i.GetName() == pod.GetName() {
				node = i
				return true
			}
			return false
		})

		roleLabelVal := pod.GetLabels()[failoverbuilder.RedisRoleLabel]
		if node == nil {
			if roleLabelVal != "" {
				if err := a.client.PatchPodLabel(ctx, pod.DeepCopy(), failoverbuilder.RedisRoleLabel, ""); err != nil {
					logger.Error(err, "patch pod label failed")
					return actor.RequeueWithError(err)
				}
			}
			continue
		}
		nodeAddr := net.JoinHostPort(node.DefaultIP().String(), strconv.Itoa(node.Port()))
		if node.Role() == core.RedisRoleMaster && nodeAddr == masterAddr {
			if roleLabelVal != failoverbuilder.RedisRoleMaster {
				err := a.client.PatchPodLabel(ctx, node.Definition(), failoverbuilder.RedisRoleLabel, failoverbuilder.RedisRoleMaster)
				if err != nil {
					logger.Error(err, "patch pod label failed")
					return actor.RequeueWithError(err)
				}
			}
		} else if node.Role() == core.RedisRoleReplica {
			if roleLabelVal != failoverbuilder.RedisRoleReplica {
				err := a.client.PatchPodLabel(ctx, node.Definition(), failoverbuilder.RedisRoleLabel, failoverbuilder.RedisRoleReplica)
				if err != nil {
					logger.Error(err, "patch pod label failed")
					return actor.RequeueWithError(err)
				}
			}
		} else {
			if roleLabelVal != "" {
				err := a.client.PatchPodLabel(ctx, node.Definition(), failoverbuilder.RedisRoleLabel, "")
				if err != nil {
					logger.Error(err, "patch pod label failed")
					return actor.RequeueWithError(err)
				}
			}
		}
	}
	return nil
}
