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
	"github.com/alauda/redis-operator/pkg/ops/cluster"
	cops "github.com/alauda/redis-operator/pkg/ops/cluster"
	"github.com/alauda/redis-operator/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
)

var _ actor.Actor = (*actorHealPod)(nil)

func NewHealPodActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorHealPod{
		client: client,
		logger: logger,
	}
}

type actorHealPod struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorHealPod) SupportedCommands() []actor.Command {
	return []actor.Command{cluster.CommandHealPod}
}

// Do
func (a *actorHealPod) Do(ctx context.Context, cluster types.RedisInstance) *actor.ActorResult {
	logger := a.logger.WithName(cops.CommandHealPod.String()).WithValues("namespace", cluster.GetNamespace(), "name", cluster.GetName())

	// clean terminating pods
	var (
		now       = time.Now()
		isUpdated = false
	)

	for _, node := range cluster.Nodes() {
		timestamp := node.GetDeletionTimestamp()
		if timestamp == nil {
			continue
		}
		grace := time.Second * 30
		if val := node.GetDeletionGracePeriodSeconds(); val != nil {
			grace = time.Duration(*val) * time.Second
		}
		grace += time.Minute * 5

		if now.Sub(timestamp.Time) <= grace {
			continue
		}

		objKey := client.ObjectKey{Namespace: node.GetNamespace(), Name: node.GetName()}
		logger.V(2).Info("for delete pod", "name", node.GetName())
		// force delete the terminating pods
		if err := a.client.DeletePod(ctx, cluster.GetNamespace(), node.GetName(), true); err != nil {
			logger.Error(err, "force delete pod failed", "target", objKey)
		} else {
			isUpdated = true
			logger.Info("force delete blocked terminating pod", "target", objKey)
		}
	}
	if isUpdated {
		return actor.NewResult(cops.CommandRequeue)
	}
	if len(cluster.Nodes()) == 0 {
		return actor.NewResult(cops.CommandEnsureResource)
	}
	return nil
}
