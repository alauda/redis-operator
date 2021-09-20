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
	"time"

	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	cops "github.com/alauda/redis-operator/pkg/ops/cluster"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/slot"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

var _ actor.Actor = (*actorEnsureSlots)(nil)

func NewEnsureSlotsActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorEnsureSlots{
		client: client,
		logger: logger,
	}
}

type actorEnsureSlots struct {
	client kubernetes.ClientSet

	logger logr.Logger
}

// SupportedCommands
func (a *actorEnsureSlots) SupportedCommands() []actor.Command {
	return []actor.Command{cops.CommandEnsureSlots}
}

// Do
// 该 Actor 处理槽分配与槽丢失的情况, 槽的迁移由 Rebanalce + sidecar 处理
// 处理逻辑如下：
// 判断是否有 master 节点存在，如果无 master 节点，则 failover
// 槽的分配和迁移只根据 cr.status.shards 中预分配的信息来，如果发现某个槽丢失了，以下逻辑会自动添加回来
// 特殊情况：
//
//	如果发现某个槽被意外移到了其他节点，该槽不会被移动回来，operator 不会处理这个情况，并保留这个状态。在下一个 Reconcile 中，operator 会在cluster inservice 时，刷新 cr.status.shards 信息
func (a *actorEnsureSlots) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	cluster := val.(types.RedisClusterInstance)
	cr := cluster.Definition()
	logger := a.logger.WithName(cops.CommandEnsureSlots.String()).WithValues("namespace", cr.Namespace, "name", cr.Name)
	logger.Info("ensure slots")

	// force refresh the cluster
	if err := cluster.Refresh(ctx); err != nil {
		logger.Error(err, "refresh cluster info failed")
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	if len(cluster.Shards()) == 0 {
		return actor.NewResult(cops.CommandEnsureResource)
	}

	// check is slots fullfilled
	var (
		allSlots    = slot.NewSlots()
		shardsSlots = map[int]types.RedisClusterShard{}
	)
	for i, shard := range cluster.Shards() {
		if i != shard.Index() {
			return actor.NewResult(cops.CommandEnsureResource)
		}
		allSlots = allSlots.Union(shard.Slots())
		shardsSlots[shard.Index()] = shard

		if shard.Master() != nil {
			continue
		}
		if len(shard.Nodes()) != int(cluster.Definition().Spec.ClusterReplicas)+1 {
			return actor.NewResult(cops.CommandRequeue)
		}
		for _, node := range shard.Nodes() {
			if node.Status() == corev1.PodPending || node.IsTerminating() {
				return actor.NewResult(cops.CommandHealPod)
			}
			if !node.IsReady() {
				return actor.NewResult(cops.CommandRequeue)
			}
		}
	}
	if allSlots.IsFullfilled() {
		return nil
	}

	var (
		failedShards     []types.RedisClusterShard
		validMasterCount int
		aliveMasterCount int
	)

	// Only masters will slots can vote for the slave to promote to be a master
	for _, shard := range cr.Status.Shards {
		for _, status := range shard.Slots {
			if status.Status == slot.SlotAssigned.String() {
				validMasterCount += 1
				break
			}
		}
	}

	// check shard master info
	// NOTE: here not check the case when the last statefulset is removed
	for i := 0; i < len(cluster.Shards()); i++ {
		shard := cluster.Shards()[i]
		if shard.Index() != i {
			// some shard missing, fix them first. this should not happen here
			return actor.NewResult(cops.CommandEnsureResource)
		}
		// check if master exists
		if shard.Master() == nil {
			if len(shard.Replicas()) == 0 {
				// pod missing
				return actor.NewResult(cops.CommandHealPod)
			}
			failedShards = append(failedShards, shard)
		} else {
			aliveMasterCount += 1
		}
	}

	if len(failedShards) > 0 {
		args := []interface{}{"CLUSTER", "FAILOVER"}
		for _, shard := range failedShards {
			args = args[0:2]
			takeover := aliveMasterCount*2 < validMasterCount
			if takeover {
				args = append(args, "TAKEOVER")
			} else {
				args = append(args, "FORCE")
			}
			for _, node := range shard.Replicas() {
				if node.IsTerminating() {
					continue
				}
				if subRet := func() *actor.ActorResult {
					ctx, cancel := context.WithTimeout(ctx, time.Second*10)
					defer cancel()

					logger.Info("do replica failover", "node", node.GetName(), "action", args[2])
					if err := node.Setup(ctx, args); err != nil {
						logger.Error(err, "do replica failover failed", "node", node.GetName())
						// slave unexpectedly not reachable, requeue
						return actor.NewResult(cops.CommandRequeue)
					}
					time.Sleep(time.Second * 5)

					if err := node.Refresh(ctx); err != nil {
						logger.Error(err, "refresh node info failed, try with other failover")
						return nil
					}
					return nil
				}(); subRet != nil {
					return subRet
				}

				// failover succees
				if node.Role() == redis.RedisRoleMaster {
					aliveMasterCount += 1
					break
				}
			}
		}
		return actor.NewResult(cops.CommandRequeue)
	}

	needRefresh := false
	for _, shardStatus := range cluster.Definition().Status.Shards {
		assignedSlots := slot.NewSlots()
		for _, status := range shardStatus.Slots {
			assignedSlots.Set(status.Slots, slot.NewSlotAssignStatusFromString(status.Status))
		}
		shard := shardsSlots[int(shardStatus.Index)]
		node := shard.Master()
		if err := node.Refresh(ctx); err != nil {
			logger.Error(err, "refresh node failed")
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
		if node.Role() != redis.RedisRoleMaster {
			continue
		}

		// NOTE: the operator will update the shards status when the cluster is ok in service

		// if one slots migrated manully, the operator will not move it back
		// assign the slot back to current master
		missingSlots := assignedSlots.Sub(shard.Slots()).Sub(allSlots).Slots()
		if len(missingSlots) > 0 {
			if len(missingSlots) < shard.Slots().Count(slot.SlotAssigned) {
				logger.Info("WARNING: as if the slots have been moved unexpectedly, this may case the cluster run in an unbalance state")
			}
			args := []interface{}{"CLUSTER", "ADDSLOTS"}
			for _, slot := range missingSlots {
				args = append(args, slot)
			}
			if err := node.Setup(ctx, args); err != nil {
				logger.Error(err, "assign slots to shard failed", "shard", shard.GetName())
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}
			needRefresh = true
		}
	}

	if needRefresh {
		// force refresh the cluster
		if err := cluster.Refresh(ctx); err != nil {
			logger.Error(err, "refresh cluster info failed")
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}
	return nil
}
