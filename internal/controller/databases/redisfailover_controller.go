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

package databases

import (
	"context"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
	redisv1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/builder/failoverbuilder"
	"github.com/alauda/redis-operator/internal/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/ops"
)

type RedisFailoverReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
	Engine        *ops.OpEngine
}

// +kubebuilder:rbac:groups=databases.spotahome.com,resources=redisfailovers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=databases.spotahome.com,resources=redisfailovers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=databases.spotahome.com,resources=redisfailovers/finalizers,verbs=update

// Reconcile
func (r *RedisFailoverReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)

	var instance databasesv1.RedisFailover
	if err := r.Get(ctx, req.NamespacedName, &instance); errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "get resource failed")
		return ctrl.Result{}, err
	}
	err := instance.Validate()
	if err != nil {
		instance.Status.Message = err.Error()
		instance.Status.Phase = databasesv1.Fail
		if err := r.Status().Update(ctx, &instance); err != nil {
			logger.Error(err, "update status failed")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	if crVersion := instance.Annotations[config.CRVersionKey]; crVersion == "" {
		managedByRds := func() bool {
			for _, ref := range instance.GetOwnerReferences() {
				if ref.Kind == "Redis" {
					return true
				}
			}
			return false
		}()

		if managedByRds {
			logger.Info("no actor version specified, waiting for rds register rds version")
			return ctrl.Result{RequeueAfter: time.Second * 15}, nil
		} else if config.GetOperatorVersion() != "" {
			instance.Annotations[config.CRVersionKey] = config.GetOperatorVersion()
			if err := r.Client.Update(ctx, &instance); err != nil {
				logger.Error(err, "update instance actor version failed")
			}
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
	}

	{
		var (
			nodes       []databasesv1.SentinelMonitorNode
			serviceName = sentinelbuilder.GetSentinelHeadlessServiceName(instance.GetName())
			status      = &instance.Status
			oldStatus   = instance.Status.DeepCopy()
		)
		if instance.Spec.Sentinel == nil {
			status.Monitor = databasesv1.MonitorStatus{
				Policy: databasesv1.ManualFailoverPolicy,
			}
		} else {
			// TODO: use DNS SRV replace config all sentinel node address, which will cause data pods restart
			for i := 0; i < int(instance.Spec.Sentinel.Replicas); i++ {
				podName := sentinelbuilder.GetSentinelNodeServiceName(instance.GetName(), i)
				srv := fmt.Sprintf("%s.%s.%s", podName, serviceName, instance.GetNamespace())
				nodes = append(nodes, databasesv1.SentinelMonitorNode{IP: srv, Port: 26379})
			}

			status.Monitor.Policy = databasesv1.SentinelFailoverPolicy
			if instance.Spec.Sentinel.SentinelReference == nil {
				// HARDCODE: use mymaster as sentinel monitor name
				status.Monitor.Name = "mymaster"

				// append history password secrets
				// NOTE: here recorded empty password
				passwordSecret := instance.Spec.Sentinel.PasswordSecret
				if status.Monitor.PasswordSecret != passwordSecret {
					status.Monitor.OldPasswordSecret = status.Monitor.PasswordSecret
					status.Monitor.PasswordSecret = passwordSecret
				}

				if instance.Spec.Sentinel.EnableTLS {
					if instance.Spec.Sentinel.ExternalTLSSecret != "" {
						status.Monitor.TLSSecret = instance.Spec.Sentinel.ExternalTLSSecret
					} else {
						status.Monitor.TLSSecret = builder.GetRedisSSLSecretName(instance.GetName())
					}
				}
				status.Monitor.Nodes = nodes
			} else {
				status.Monitor.Name = fmt.Sprintf("%s.%s", instance.GetNamespace(), instance.GetName())
				status.Monitor.OldPasswordSecret = status.Monitor.PasswordSecret
				status.Monitor.PasswordSecret = instance.Spec.Sentinel.SentinelReference.Auth.PasswordSecret
				status.Monitor.TLSSecret = instance.Spec.Sentinel.SentinelReference.Auth.TLSSecret
				status.Monitor.Nodes = instance.Spec.Sentinel.SentinelReference.Nodes
			}
		}
		if !reflect.DeepEqual(status, oldStatus) {
			if err := r.Status().Update(ctx, &instance); err != nil {
				logger.Error(err, "update status failed")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
	}

	var user redisv1.RedisUser
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: instance.Namespace,
		Name:      failoverbuilder.GenerateFailoverDefaultRedisUserName(instance.GetName()),
	}, &user); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "get user failed")
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
	} else {
		isUpdated := false
		if secretName := instance.Spec.Auth.SecretPath; secretName != "" {
			if len(user.Spec.PasswordSecrets) == 0 || user.Spec.PasswordSecrets[0] != secretName {
				user.Spec.PasswordSecrets = append(user.Spec.PasswordSecrets[0:0], secretName)
				isUpdated = true
			}
		} else {
			if len(user.Spec.PasswordSecrets) != 0 {
				user.Spec.PasswordSecrets = nil
				isUpdated = true
			}
		}
		if isUpdated {
			if err := r.Update(ctx, &user); err != nil {
				return ctrl.Result{RequeueAfter: time.Second * 5}, nil
			}
		}
	}
	return r.Engine.Run(ctx, &instance)
}

func (r *RedisFailoverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasesv1.RedisFailover{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 16}).
		Owns(&databasesv1.RedisSentinel{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
