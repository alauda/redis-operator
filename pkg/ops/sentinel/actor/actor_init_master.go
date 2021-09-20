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
	sops "github.com/alauda/redis-operator/pkg/ops/sentinel"
	"github.com/alauda/redis-operator/pkg/types"
	r_types "github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/go-logr/logr"
)

var _ actor.Actor = (*actorInitMaster)(nil)

func NewActorInitMasterActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorInitMaster{
		client: client,
		logger: logger,
	}
}

type actorInitMaster struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorInitMaster) SupportedCommands() []actor.Command {
	return []actor.Command{sentinel.CommandInitMaster}
}

func (a *actorInitMaster) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {

	logger := a.logger.WithName("Do").WithValues("namespace", val.GetNamespace(), "name", val.GetName())
	st := val.(types.RedisFailoverInstance)
	logger.Info("init master", "sentinel", st.GetName())
	var offsetMap = make(map[r_types.RedisNode]int64)
	var createTimestampMap = make(map[r_types.RedisNode]int64)
	for _, node := range st.Nodes() {
		offsetMap[node] = node.Info().MasterReplOffset
		createTimestampMap[node] = node.GetPod().GetCreationTimestamp().Unix()
	}
	// 如果有master且有slave,则不需要设置master
	for _, node := range st.Nodes() {
		if node.Info().Role == "master" && node.Info().ConnectedReplicas != 0 {
			return nil
		}
	}
	// 获取offset node ,如果max offset >0,且 并设置为master

	var master r_types.RedisNode
	var maxOffset int64 = 0
	for _, node := range st.Nodes() {
		if offsetMap[node] > maxOffset {
			maxOffset = offsetMap[node]
			master = node
		}
	}

	//如果任何一个sentinel的master status为ok,则不需要设置master，等待起自愈
	for _, v := range st.SentinelNodes() {
		if v.Info().SentinelMaster0.Status == "ok" {
			return nil
		}
	}

	if maxOffset > 0 {
		for _, node := range st.Nodes() {

			if node == master {
				logger.Info("(offset) set master", "master", master.GetName())
				if err := node.ReplicaOf(ctx, "no", "one"); err != nil {
					logger.Error(err, "set master failed")
					return actor.NewResultWithError(sops.CommandRequeue, err)
				}
			} else {
				logger.Info("(offset) set slave", "slave", node.GetName(), "master", master.GetName())
				if err := node.ReplicaOf(ctx, master.DefaultIP().String(), strconv.Itoa(master.Port())); err != nil {
					logger.Error(err, "set slave failed")
					return actor.NewResultWithError(sops.CommandRequeue, err)
				}
			}
		}
		return nil
	}

	// 获取创建时间最早的node,并设置为master
	var minTimestamp int64 = 0
	for _, node := range st.Nodes() {
		if minTimestamp == 0 {
			minTimestamp = createTimestampMap[node]
			master = node
		}
		if createTimestampMap[node] < minTimestamp {
			minTimestamp = createTimestampMap[node]
			master = node
		}
	}
	for _, node := range st.Nodes() {
		if node == master {
			logger.Info("(createTime)set master", "master", master.GetName())
			if err := node.ReplicaOf(ctx, "no", "one"); err != nil {
				logger.Error(err, "set master failed")
				return actor.NewResultWithError(sops.CommandRequeue, err)
			}
		} else {
			logger.Info("(createTime)set slave", "slave", node.GetName(), "master", master.GetName())
			if err := node.ReplicaOf(ctx, master.DefaultIP().String(), strconv.Itoa(master.Port())); err != nil {
				return actor.NewResultWithError(sops.CommandRequeue, err)
			}
		}
	}
	return actor.NewResult(sops.CommandPatchLabels)
}
