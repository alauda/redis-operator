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
	"strconv"

	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/ops/sentinel"
	"github.com/alauda/redis-operator/pkg/redis"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/go-logr/logr"
)

var _ actor.Actor = (*actorHealMaster)(nil)

func NewActorHealMasterActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorHealMaster{
		client: client,
		logger: logger,
	}
}

type actorHealMaster struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorHealMaster) SupportedCommands() []actor.Command {
	return []actor.Command{sentinel.CommandHealMaster}
}

func (a *actorHealMaster) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	a.logger.Info("heal master", "sentinel", val.GetName())
	st := val.(types.RedisFailoverInstance)
	if len(st.Nodes()) != int(st.Definition().Spec.Redis.Replicas) {
		return actor.NewResult(sentinel.CommandEnsureResource)
	}
	rf := st.Definition()
	masterCount := map[redis.Address]int{}
	for _, node := range st.SentinelNodes() {
		if node.Info().SentinelMaster0.Status == "ok" {
			masterCount[node.Info().SentinelMaster0.Address]++
		}

	}

	//获取masterCount 最大的Address
	var maxAddress redis.Address
	var maxCount int
	for address, count := range masterCount {
		if count > maxCount {
			maxCount = count
			maxAddress = address
		}
	}
	if maxCount == 0 {
		a.logger.Info("no master found")
	}
	exists := false
	for _, n := range st.Nodes() {
		if n.DefaultIP().String() == maxAddress.Host() && n.Port() == maxAddress.Port() {
			exists = true
		}
	}
	// 如果 maxCount >= (rf.Spec.Redis.Replicas/2),就将所有redis节点的master设置为masterCount最多的master
	if maxCount >= int(rf.Spec.Redis.Replicas/2) {
		for _, node := range st.Nodes() {
			a.logger.Info("set sentinel's master", "masterIp", maxAddress.Host(), "masterPort", maxAddress.Port(), "exists", exists)
			if exists {
				if err := node.ReplicaOf(ctx, maxAddress.Host(), strconv.Itoa(maxAddress.Port())); err != nil {
					a.logger.Error(err, "set master failed")
					return actor.NewResultWithError(sentinel.CommandRequeue, err)
				}
			}

		}
	} else {
		for _, node := range st.Nodes() {
			if len(st.Masters()) == 1 {
				master := st.Masters()[0]
				a.logger.Info("set node master", "masterIp", master.DefaultIP(), "masterPort", master.Port())
				if err := node.ReplicaOf(ctx, master.DefaultIP().String(), strconv.Itoa(master.Port())); err != nil {
					a.logger.Error(err, "set master failed")
					return actor.NewResultWithError(sentinel.CommandRequeue, err)
				}
			}
		}
	}

	return actor.NewResult(sentinel.CommandPatchLabels)
}
