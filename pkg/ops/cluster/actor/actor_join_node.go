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
	"fmt"
	"strings"
	"time"

	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/ops/cluster"
	cops "github.com/alauda/redis-operator/pkg/ops/cluster"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/slot"
	"github.com/go-logr/logr"
)

var _ actor.Actor = (*actorJoinNode)(nil)

func NewJoinNodeActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorJoinNode{
		client: client,
		logger: logger,
	}
}

type actorJoinNode struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorJoinNode) SupportedCommands() []actor.Command {
	return []actor.Command{cluster.CommandJoinNode}
}

// Do
func (a *actorJoinNode) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	cluster := val.(types.RedisClusterInstance)
	logger := a.logger.WithName(cops.CommandJoinNode.String()).WithValues("namespace", cluster.GetNamespace(), "name", cluster.GetName())

	// force refresh the cluster
	if err := cluster.Refresh(ctx); err != nil {
		logger.Error(err, "refresh cluster info failed")
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}

	var (
		joined   []redis.RedisNode
		unjoined []redis.RedisNode
	)
	// check all nodes unjoined
	for _, shard := range cluster.Shards() {
		for _, node := range shard.Nodes() {
			if node.ContainerStatus() == nil || node.ID() == "" || node.IsTerminating() {
				continue
			}
			if !node.IsJoined() {
				unjoined = append(unjoined, node)
			} else {
				joined = append(joined, node)
			}
		}
		if master := shard.Master(); master != nil {
			masterIP, masterPort := master.DefaultIP().String(), fmt.Sprintf("%d", master.Port())
			replicas := shard.Replicas()
			for _, replica := range replicas {
				if replica.ConfigedMasterIP() != masterIP || replica.ConfigedMasterPort() != masterPort {
					unjoined = append(unjoined, replica)
				}
			}
		}
	}

	if len(unjoined) > 0 && len(joined) == 0 {
		joined = append(joined, unjoined[0])
		unjoined = unjoined[1:]
	}
	needRefresh := false
	if len(unjoined) > 0 {
		var margs [][]interface{}
		for _, node := range unjoined {
			margs = append(margs, []interface{}{"cluster", "meet", node.DefaultInternalIP().String(), node.InternalPort(), node.InternalIPort()})
		}
		for _, targetNode := range joined {
			if err := targetNode.Setup(ctx, margs...); err != nil {
				return actor.NewResultWithError(cops.CommandAbort, fmt.Errorf("set up cluster meet failed"))
			}
			needRefresh = true
		}
		time.Sleep(time.Second * 5)
	}

	// set update master's replicas
	for _, shard := range cluster.Shards() {
		if err := shard.Refresh(ctx); err != nil {
			logger.Error(err, "refresh shard nodes failed", "shard", shard.GetName())
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}

		master := shard.Master()
		if master != nil {
			for _, node := range shard.Nodes() {
				if node.IsTerminating() ||
					node.ID() == "" || node.ID() == master.ID() ||
					// If the master link is not up, reconfiguration is required.
					(node.MasterID() == master.ID() && node.IsMasterLinkUp()) {
					continue
				}

				logger.Info("setup replicate", "pod", node.GetName(), "myid", node.ID(), "role", node.Role(),
					"masterid", master.ID(), "masterlinkstatus", node.IsMasterLinkUp())
				if node.Role() == redis.RedisRoleMaster && node.Slots().Count(slot.SlotAssigned) > 0 {
					// if this node is master and has slots, do rebalance
					return actor.NewResult(cops.CommandRebalance)
				}
				if err := node.Setup(ctx, []interface{}{"cluster", "replicate", master.ID()}); err != nil {
					logger.Error(err, "replicate node failed", "node", node.ID())
					if strings.Contains(err.Error(), "ERR To set a master the node must be empty and without assigned slots") {
						// rejoin this node with master nodes
						args := []interface{}{"cluster", "meet", node.DefaultInternalIP().String(), node.InternalPort(), node.InternalIPort()}
						if err := master.Setup(ctx, args); err != nil {
							return actor.NewResultWithError(cops.CommandAbort, fmt.Errorf("set up cluster meet failed"))
						}
						needRefresh = true
					}
				}
				needRefresh = true
			}
		} else if len(shard.Nodes()) > 0 {
			// no node is master, no node can be used as new master
			// all the node is slaves, do force failover to elect a new master
			var offset int64 = -1
			for _, node := range shard.Nodes() {
				logger.V(2).Info("check node", "node", node.GetName(),
					"status", node.IsTerminating(),
					"id", node.ID(),
					"reploffset", node.Info().MasterReplOffset,
				)
				if node.IsTerminating() || node.ID() == "" {
					continue
				}
				if node.Info().MasterReplOffset > offset {
					offset = node.Info().MasterReplOffset
					master = node
				}
			}
			if master == nil {
				// NOTE: below cases will cause this branch:
				// 1. pod startup is blocked by scheduler (not enough resource)
				// 2. the main container crashed
				// when this happends, nothing we can do, just requeue
				logger.Info("no node found to setup as master, as if the the container startup blocked or failed")
				return actor.NewResult(cops.CommandRequeue)
			}

			logger.Info("do replica failover", "importing", shard.IsImporting(), "migrating", shard.IsMigrating())
			if !shard.IsImporting() && !shard.IsMigrating() {
				// here use force takeover, for in some cases,
				// the node may failed to connect to other nodes, and can't get vote from them.
				args := []interface{}{"CLUSTER", "FAILOVER", "FORCE"}
				if subRet := func() *actor.ActorResult {
					ctx, cancel := context.WithTimeout(ctx, time.Second*10)
					defer cancel()

					logger.Info("do replica failover", "node", master.GetName(), "action", args[2])
					if err := master.Setup(ctx, args); err != nil {
						logger.Error(err, "do replica failover failed", "node", master.GetName())
					}
					return actor.NewResultWithValue(cops.CommandRequeue, time.Second*30)
				}(); subRet != nil {
					return subRet
				}
			}
		} else {
			// NOTE: below cases will cause this branch:
			// 1. instance is paused
			// when this happends, nothing we can do, just requeue
			logger.Info("no node found to setup as master,try to ensure resources")
			return actor.NewResult(cops.CommandRequeue)
		}
	}

	if needRefresh {
		time.Sleep(1)
		// force refresh the cluster
		if err := cluster.Refresh(ctx); err != nil {
			logger.Error(err, "refresh cluster info failed")
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}
	return nil
}
