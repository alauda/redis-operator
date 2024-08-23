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
	"slices"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/internal/config"
	ops "github.com/alauda/redis-operator/internal/ops/failover"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ actor.Actor = (*actorHealPod)(nil)

func init() {
	actor.Register(core.RedisSentinel, NewHealPodActor)
}

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
	return []actor.Command{ops.CommandHealPod}
}

func (a *actorHealPod) Version() *semver.Version {
	return semver.MustParse("3.18.0")
}

// Do
func (a *actorHealPod) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	logger := val.Logger().WithValues("actor", ops.CommandHealPod.String())

	// clean terminating pods
	var (
		inst = val.(types.RedisFailoverInstance)
		now  = time.Now()
	)

	pods, err := inst.RawNodes(ctx)
	if err != nil {
		logger.Error(err, "get pods failed")
		return actor.RequeueWithError(err)
	}

	for _, pod := range pods {
		timestamp := pod.GetDeletionTimestamp()
		if timestamp == nil {
			continue
		}
		grace := time.Second * 30
		if val := pod.GetDeletionGracePeriodSeconds(); val != nil {
			grace = time.Duration(*val) * time.Second
		}
		if now.Sub(timestamp.Time) <= grace {
			continue
		}

		objKey := client.ObjectKey{Namespace: pod.GetNamespace(), Name: pod.GetName()}
		logger.V(2).Info("for delete pod", "name", pod.GetName())
		// force delete the terminating pods
		if err := a.client.DeletePod(ctx, inst.GetNamespace(), pod.GetName(), client.GracePeriodSeconds(0)); err != nil {
			logger.Error(err, "force delete pod failed", "target", objKey)
		} else {
			inst.SendEventf(corev1.EventTypeWarning, config.EventCleanResource, "force delete blocked terminating pod %s", objKey.Name)
			logger.Info("force delete blocked terminating pod", "target", objKey)
			return actor.Requeue()
		}
	}

	if typ := inst.Definition().Spec.Redis.Expose.ServiceType; typ == corev1.ServiceTypeNodePort ||
		typ == corev1.ServiceTypeLoadBalancer {
		for _, node := range inst.Nodes() {
			if !node.IsReady() {
				continue
			}
			announceIP := node.DefaultIP().String()
			announcePort := node.Port()

			svc, err := a.client.GetService(ctx, inst.GetNamespace(), node.GetName())
			if errors.IsNotFound(err) {
				logger.Info("service not found", "name", node.GetName())
				return actor.NewResult(ops.CommandEnsureResource)
			} else if err != nil {
				logger.Error(err, "get service failed", "name", node.GetName())
				return actor.RequeueWithError(err)
			}
			if typ == corev1.ServiceTypeNodePort {
				port := util.GetServicePortByName(svc, "client")
				if port != nil {
					if int(port.NodePort) != announcePort {
						if err := a.client.DeletePod(ctx, inst.GetNamespace(), node.GetName()); err != nil {
							logger.Error(err, "delete pod failed", "name", node.GetName())
							return actor.RequeueWithError(err)
						} else {
							inst.SendEventf(corev1.EventTypeWarning, config.EventCleanResource,
								"force delete pod with inconsist annotation %s", node.GetName())
							return actor.Requeue()
						}
					}
				} else {
					logger.Error(fmt.Errorf("service port not found"), "service port not found", "name", node.GetName(), "port", "client")
				}
			} else if typ == corev1.ServiceTypeLoadBalancer {
				if index := slices.IndexFunc(svc.Status.LoadBalancer.Ingress, func(ing corev1.LoadBalancerIngress) bool {
					return ing.IP == announceIP || ing.Hostname == announceIP
				}); index < 0 {
					if err := a.client.DeletePod(ctx, inst.GetNamespace(), node.GetName()); err != nil {
						logger.Error(err, "delete pod failed", "name", node.GetName())
						return actor.RequeueWithError(err)
					} else {
						inst.SendEventf(corev1.EventTypeWarning, config.EventCleanResource,
							"force delete pod with inconsist annotation %s", node.GetName())
						return actor.Requeue()
					}
				}
			}
		}
	}

	if fullfilled, _ := inst.IsResourceFullfilled(ctx); !fullfilled {
		return actor.NewResult(ops.CommandEnsureResource)
	}
	return nil
}
