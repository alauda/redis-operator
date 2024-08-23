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

	"github.com/alauda/redis-operator/api/core"
	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder/sentinelbuilder"
	ops "github.com/alauda/redis-operator/internal/ops/failover"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
)

var _ actor.Actor = (*actorCleanResource)(nil)

func init() {
	actor.Register(core.RedisSentinel, NewCleanResourceActor)
}

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
	return []actor.Command{ops.CommandCleanResource}
}

func (a *actorCleanResource) Version() *semver.Version {
	return semver.MustParse("3.18.0")
}

// Do
func (a *actorCleanResource) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	logger := val.Logger().WithValues("actor", ops.CommandCleanResource.String())

	inst := val.(types.RedisFailoverInstance)
	cr := inst.Definition()

	if inst.IsReady() {
		// TODO: deprecated in 3.22
		name := sentinelbuilder.GetSentinelStatefulSetName(inst.GetName())
		sts, err := a.client.GetStatefulSet(ctx, cr.Namespace, name)
		if err != nil {
			if !errors.IsNotFound(err) {
				logger.Error(err, "failed to get sentinel statefulset")
				return actor.RequeueWithError(err)
			}
		} else if sts != nil && sts.Status.ReadyReplicas == *sts.Spec.Replicas {
			if _, err := a.client.GetDeployment(ctx, cr.Namespace, name); err != nil {
				if !errors.IsNotFound(err) {
					return actor.RequeueWithError(err)
				}
			} else if err := a.client.DeleteDeployment(ctx, cr.Namespace, name); err != nil {
				logger.Error(err, "failed to delete old sentinel deployment")
				return actor.RequeueWithError(err)
			}
		}

		// delete sentinel after standalone is ready for old pod to gracefully shutdown
		if !inst.IsBindedSentinel() {
			var sen v1.RedisSentinel
			if err := a.client.Client().Get(ctx, client.ObjectKey{
				Namespace: inst.GetNamespace(),
				Name:      inst.GetName(),
			}, &sen); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "failed to get sentinel statefulset", "sentinel", inst.GetName())
				return actor.RequeueWithError(err)
			} else if err == nil {
				if err = a.client.Client().Delete(ctx, &sen); err != nil {
					logger.Error(err, "failed to delete binded sentinel", "sentinel", inst.GetName())
					return actor.RequeueWithError(err)
				}
			}
		}
	}
	return nil
}
