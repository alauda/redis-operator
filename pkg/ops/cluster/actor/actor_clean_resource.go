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
	"time"

	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	cops "github.com/alauda/redis-operator/pkg/ops/cluster"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/slot"

	"github.com/go-logr/logr"
)

var _ actor.Actor = (*actorCleanResource)(nil)

func NewCleanResourceActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorCleanResource{
		client: client,
		logger: logger,
	}
}

type actorCleanResource struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorCleanResource) SupportedCommands() []actor.Command {
	return []actor.Command{cops.CommandCleanResource}
}

// Do
func (a *actorCleanResource) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	cluster := val.(types.RedisClusterInstance)
	cr := cluster.Definition()
	logger := a.logger.WithName(cops.CommandCleanResource.String()).WithValues("namespace", cr.Namespace, "name", cr.Name)

	var (
		failedNodeId = []string{}
		shards       = cluster.Shards()
	)

	for _, shard := range shards {
		if shard.Index() >= int(cr.Spec.MasterSize) {
			// if not empty, break clean. should clean by order
			if !(shard.Slots().Count(slot.SlotAssigned) == 0 &&
				shard.Slots().Count(slot.SlotMigrating) == 0 &&
				shard.Slots().Count(slot.SlotImporting) == 0) {

				break
			}

			for _, node := range shard.Nodes() {
				failedNodeId = append(failedNodeId, node.ID())
			}
		}
	}

	if len(failedNodeId) > 0 {
		var margs [][]interface{}
		for _, id := range failedNodeId {
			margs = append(margs, []interface{}{"cluster", "forget", id})
		}
		for i := 0; i < int(cr.Spec.MasterSize) && len(cluster.Shards()) >= int(cr.Spec.MasterSize); i++ {
			shard := cluster.Shards()[i]
			for _, node := range shard.Nodes() {
				node.Setup(ctx, margs...)
			}
		}

		time.Sleep(time.Second * 5)

		for _, shard := range shards {
			if shard.Index() >= int(cr.Spec.MasterSize) {
				// if not empty, break clean. should clean by order
				if !(shard.Slots().Count(slot.SlotAssigned) == 0 &&
					shard.Slots().Count(slot.SlotMigrating) == 0 &&
					shard.Slots().Count(slot.SlotImporting) == 0) {

					break
				}

				// delete the shard
				name := clusterbuilder.ClusterStatefulSetName(cr.GetName(), shard.Index())
				logger.Info("clean resource", "statefulset", name)
				// NOTE: DeleteStatefulSet with GracePeriodSeconds=0
				if err := a.client.DeleteStatefulSet(ctx, cr.GetNamespace(), name, true); err != nil {
					a.logger.Error(err, "delete statefulset failed", "target", fmt.Sprintf("%s/%s", cr.GetNamespace(), name))
				}
				for _, node := range shard.Nodes() {
					// delete pods
					if err := a.client.DeletePod(ctx, node.GetNamespace(), node.GetName(), true); err != nil {
						a.logger.Error(err, "force delete pod failed", "target", fmt.Sprintf("%s/%s", cr.GetNamespace(), node.GetName()))
					}

					// delete cm
					name := "sync-" + node.GetName()
					if err := a.client.DeleteConfigMap(ctx, node.GetNamespace(), name); err != nil {
						a.logger.Error(err, "delete pod related nodes.conf configmap failed", "target", fmt.Sprintf("%s/%s", cr.GetNamespace(), name))
					}
				}
			}
		}
		return actor.NewResult(cops.CommandRequeue)
	}
	return nil
}
