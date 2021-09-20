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

	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/pkg/ops/sentinel"
	"github.com/alauda/redis-operator/pkg/types"
	t_redis "github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/go-logr/logr"
)

var _ actor.Actor = (*actorPatchLabels)(nil)

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

func (a *actorPatchLabels) SupportedCommands() []actor.Command {
	return []actor.Command{sentinel.CommandPatchLabels}
}

func (a *actorPatchLabels) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	// 给pod添加对应角色的labels
	st := val.(types.RedisFailoverInstance)
	for _, v := range st.Nodes() {
		if v.Definition() != nil {
			if v.Role() == t_redis.RedisRoleMaster {

				if v.Definition().GetLabels()[sentinelbuilder.RedisRoleLabel] != sentinelbuilder.RedisRoleMaster {
					err := a.client.PatchPodLabel(ctx, v.Definition(), sentinelbuilder.RedisRoleLabel, sentinelbuilder.RedisRoleMaster)
					if err != nil {
						a.logger.Error(err, "patch pod label failed")
						return actor.NewResultWithError(sentinel.CommandRequeue, err)
					}
				}

			}
			if v.Role() == t_redis.RedisRoleSlave {
				if v.Definition().GetLabels()[sentinelbuilder.RedisRoleLabel] != sentinelbuilder.RedisRoleSlave {
					err := a.client.PatchPodLabel(ctx, v.Definition(), sentinelbuilder.RedisRoleLabel, sentinelbuilder.RedisRoleSlave)
					if err != nil {
						a.logger.Error(err, "patch pod label failed")
						return actor.NewResultWithError(sentinel.CommandRequeue, err)
					}
				}
			}
		}
	}
	return actor.NewResult(sentinel.CommandSentinelHeal)
}
