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

package ops

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	v1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/clientset"
	clustermodel "github.com/alauda/redis-operator/pkg/models/cluster"
	sentinelmodle "github.com/alauda/redis-operator/pkg/models/sentinel"
	"github.com/alauda/redis-operator/pkg/ops/cluster"
	"github.com/alauda/redis-operator/pkg/ops/sentinel"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RuleEngine interface {
	Inspect(ctx context.Context, instance types.RedisInstance) *actor.ActorResult
}

// OpEngine
type OpEngine struct {
	client        kubernetes.ClientSet
	eventRecorder record.EventRecorder

	clusterRuleEngine RuleEngine
	sentinelRulEngine RuleEngine
	actorManager      *actor.ActorManager

	logger logr.Logger
}

// NewOpEngine
func NewOpEngine(client client.Client, eventRecorder record.EventRecorder, manager *actor.ActorManager, logger logr.Logger) (*OpEngine, error) {
	if client == nil {
		return nil, fmt.Errorf("require k8s clientset")
	}
	if eventRecorder == nil {
		return nil, fmt.Errorf("require k8s event recorder")
	}
	if manager == nil {
		return nil, fmt.Errorf("require actor manager")
	}

	e := OpEngine{
		client:        clientset.New(client, logger),
		eventRecorder: eventRecorder,
		actorManager:  manager,
		logger:        logger.WithName("OpEngine"),
	}

	if engine, err := cluster.NewRuleEngine(e.client, eventRecorder, logger); err != nil {
		return nil, err
	} else {
		e.clusterRuleEngine = engine
	}

	if engine, err := sentinel.NewRuleEngine(e.client, eventRecorder, logger); err != nil {
		return nil, err
	} else {
		e.sentinelRulEngine = engine
	}
	return &e, nil
}

// Run
func (e *OpEngine) Run(ctx context.Context, val interface{}) (ctrl.Result, error) {
	// one reconcile only keeps 5minutes
	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	switch instance := val.(type) {
	case *v1.RedisFailover:
		failover, err := e.loadFailoverInstance(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		return e.reconcile(ctx, failover)
	case *v1alpha1.DistributedRedisCluster:
		cluster, err := e.loadClusterInstance(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		return e.reconcile(ctx, cluster)
	}

	return ctrl.Result{RequeueAfter: DefaultRequeueDuration}, nil
}

const (
	MaxCallDepth                = 15
	DefaultRequeueDuration      = time.Second * 10
	DefaultAbortRequeueDuration = time.Minute
	PauseRequeueDuration        = time.Minute * 10
	DefaultReconcileTimeout     = time.Minute * 5
)

func (e *OpEngine) reconcile(ctx context.Context, val types.RedisInstance) (requeue ctrl.Result, err error) {
	logger := e.logger.WithValues("namespace", val.GetNamespace(), "name", val.GetName())

	defer func() {
		if e := recover(); e != nil {
			logger.Error(fmt.Errorf("%s", e), "reconcile panic")
			err = fmt.Errorf("%s", e)
			debug.PrintStack()
		}
	}()

	inspect := func(ctx context.Context, ins types.RedisInstance) *actor.ActorResult {
		switch instance := val.(type) {
		case types.RedisFailoverInstance:
			if instance.Definition() != nil {
				logger.Info("inspect sentinel", "ns", instance.Definition().Namespace, "target", instance.Definition().GetName())
				return e.sentinelRulEngine.Inspect(ctx, ins)
			}
		case types.RedisClusterInstance:
			if instance.Definition() != nil {
				logger.Info("inspect cluster", "ns", instance.Definition().Namespace, "target", instance.Definition().GetName())
				return e.clusterRuleEngine.Inspect(ctx, ins)
			}

		}
		return actor.NewResult(cluster.CommandAbort)
	}

	actorDo := func(ctx context.Context, a actor.Actor, ins types.RedisInstance) *actor.ActorResult {
		return a.Do(ctx, ins)
	}

	sendEvent := func(ins types.RedisInstance, typ, reason string, msg string, args ...interface{}) {
		switch instance := val.(type) {
		case types.RedisFailoverInstance:
			if instance.Definition() != nil {
				e.eventRecorder.Eventf(instance.Definition(), typ, reason, msg, args...)
			}
		case types.RedisClusterInstance:
			if instance.Definition() != nil {
				e.eventRecorder.Eventf(instance.Definition(), typ, reason, msg, args...)
			}
		}
	}

	updateStatus := func(ctx context.Context, ins types.RedisInstance, status v1alpha1.ClusterStatus, stStatus v1.RedisFailoverStatus, msg string) error {
		switch instance := val.(type) {
		case types.RedisFailoverInstance:
			if instance.Definition() != nil {
				return instance.UpdateStatus(ctx, stStatus)
			}
		case types.RedisClusterInstance:
			if instance.Definition() != nil {
				return instance.UpdateStatus(ctx, status, msg, nil)
			}
		}

		return nil
	}

	var (
		failMsg         string
		crStatus        v1alpha1.ClusterStatus
		stStatus        v1.RedisFailoverStatus
		objKey          = client.ObjectKey{Namespace: val.GetNamespace(), Name: val.GetName()}
		ret             = inspect(ctx, val)
		lastCommand     actor.Command
		requeueDuration time.Duration
	)
	if ret == nil {
		switch instance := val.(type) {
		case types.RedisClusterInstance:
			if instance.Definition() != nil {
				ret = actor.NewResult(cluster.CommandEnsureResource)
			}
		case types.RedisFailoverInstance:
			if instance.Definition() != nil {
				ret = actor.NewResult(sentinel.CommandEnsureResource)
			}
		}
	}

__end__:
	for depth := 0; ret != nil && depth <= MaxCallDepth; depth += 1 {
		if depth == MaxCallDepth {
			msg := fmt.Sprintf("event loop reached %d threshold, abort current loop", MaxCallDepth)
			if lastCommand != nil {
				msg = fmt.Sprintf("event loop reached %d threshold, lastCommand %s, abort current loop", MaxCallDepth, lastCommand)
			}
			sendEvent(val, corev1.EventTypeWarning, "ThresholdLimit", msg)
			// use depth to limit the call black hole
			logger.Info(fmt.Sprintf("reconcile call depth exceeds %d", MaxCallDepth), "force requeue", "target", objKey)

			requeueDuration = time.Second * 30
			break __end__
		}
		msg := fmt.Sprintf("run command %s, depth %d", ret.NextCommand().String(), depth)
		if lastCommand != nil {
			msg = fmt.Sprintf("run command %s, last command %s, depth %d", ret.NextCommand().String(), lastCommand.String(), depth)
		}
		logger.Info(msg)

		switch ret.NextCommand() {
		case cluster.CommandAbort, sentinel.CommandAbort:
			requeueDuration, _ = ret.Result().(time.Duration)
			if err := ret.Err(); err != nil {
				logger.Error(err, "reconcile aborted", "target", objKey)
				failMsg = err.Error()
				stStatus.Message = failMsg
				crStatus = v1alpha1.ClusterStatusKO
				stStatus.Phase = v1.PhaseFail
			} else {
				logger.Info("reconcile aborted", "target", objKey)
			}
			if requeueDuration == 0 {
				requeueDuration = DefaultAbortRequeueDuration
			}
			break __end__
		case cluster.CommandRequeue, sentinel.CommandRequeue:
			requeueDuration, _ = ret.Result().(time.Duration)
			if err := ret.Err(); err != nil {
				logger.Error(err, "reconcile requeue", "target", objKey.Name)
				failMsg = err.Error()
				stStatus.Message = failMsg
			} else {
				logger.Info("reconcile requeue", "target", objKey.Name)
			}
			break __end__
		case cluster.CommandPaused, sentinel.CommandPaused:
			requeueDuration, _ = ret.Result().(time.Duration)
			crStatus = v1alpha1.ClusterStatusPaused
			stStatus.Phase = v1.PhasePaused
			if err := ret.Err(); err != nil {
				logger.Error(err, "reconcile requeue", "target", objKey.Name)
				failMsg = err.Error()
				stStatus.Message = failMsg
			} else {
				logger.Info("reconcile requeue", "target", objKey.Name)
			}
			if requeueDuration == 0 {
				requeueDuration = PauseRequeueDuration
			}
			break __end__
		}

		ctx = context.WithValue(ctx, "depth", depth)
		if lastCommand != nil {
			ctx = context.WithValue(ctx, "last_command", lastCommand.String())
		}

		actors := e.actorManager.Get(ret.NextCommand())
		if len(actors) == 0 {
			err := fmt.Errorf("unknown command %s", ret.NextCommand())
			logger.Error(err, "actor for command not register")
			return ctrl.Result{}, err
		}
		lastCommand = ret.NextCommand()

		if ret = actorDo(ctx, actors[0], val); ret == nil {
			logger.V(3).Info("actor return nil, inspect again")
			ret = inspect(ctx, val)
		}

		if ret != nil && lastCommand != nil && ret.NextCommand().String() == lastCommand.String() {
			ret = actor.NewResult(cluster.CommandRequeue)
		}
		depth += 1
	}

	if err := updateStatus(ctx, val, crStatus, stStatus, failMsg); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "update status before requeue failed")
	}
	if requeueDuration == 0 {
		requeueDuration = DefaultRequeueDuration
	}
	return ctrl.Result{RequeueAfter: requeueDuration}, nil
}

// loadClusterInstance
func (e *OpEngine) loadClusterInstance(ctx context.Context, ins *v1alpha1.DistributedRedisCluster) (types.RedisClusterInstance, error) {
	if c, err := clustermodel.NewRedisCluster(ctx, e.client, ins, e.logger); err != nil {
		e.logger.Error(err, "load cluster failed", "target", client.ObjectKeyFromObject(ins))
		return nil, err
	} else {
		return c, nil
	}
}

// loadFailoverInstance
func (e *OpEngine) loadFailoverInstance(ctx context.Context, ins *v1.RedisFailover) (types.RedisFailoverInstance, error) {
	if c, err := sentinelmodle.NewRedisFailover(ctx, e.client, ins, e.logger); err != nil {
		e.logger.Error(err, "load failover failed", "target", client.ObjectKeyFromObject(ins))
		return nil, err
	} else {
		return c, nil
	}
}
