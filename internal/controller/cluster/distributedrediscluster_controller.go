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

package cluster

import (
	"context"
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

	clusterv1alpha1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	"github.com/alauda/redis-operator/internal/builder/clusterbuilder"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/ops"
)

// DistributedRedisClusterReconciler reconciles a DistributedRedisCluster object
type DistributedRedisClusterReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
	Engine        *ops.OpEngine
}

//+kubebuilder:rbac:groups=redis.kun,resources=distributedredisclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.kun,resources=distributedredisclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.kun,resources=distributedredisclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DistributedRedisClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)

	var instance clusterv1alpha1.DistributedRedisCluster
	if err := r.Get(ctx, req.NamespacedName, &instance); errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "get resource failed")
		return ctrl.Result{}, err
	}

	// update default status
	if instance.Status.Status == "" && len(instance.Status.Shards) == 0 {
		// update status to creating
		r.EventRecorder.Eventf(&instance, corev1.EventTypeNormal, "Creating", "new instance request")

		instance.Status.Status = clusterv1alpha1.ClusterStatusCreating
		if err := r.Status().Update(ctx, &instance); err != nil {
			logger.Error(err, "update instance status failed")
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}
	}

	if instance.Spec.EnableActiveRedis && (instance.Spec.ServiceID == nil || *instance.Spec.ServiceID < 0 || *instance.Spec.ServiceID > 15) {
		instance.Status.Status = clusterv1alpha1.ClusterStatusKO
		instance.Status.Reason = "service id must be in [0, 15]"
		_ = r.Status().Update(ctx, &instance)
		return ctrl.Result{}, nil
	}

	crVersion := instance.Annotations[config.CRVersionKey]
	if crVersion == "" {
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
			// update crVersion to instance
			instance.Annotations[config.CRVersionKey] = config.GetOperatorVersion()
			if err := r.Update(ctx, &instance); err != nil {
				logger.Error(err, "update instance actor version failed")
			}
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
	}

	// ================ setup default ===================

	_ = instance.Default()

	var user redismiddlewarealaudaiov1.RedisUser
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: instance.Namespace,
		Name:      clusterbuilder.GenerateClusterDefaultRedisUserName(instance.GetName()),
	}, &user); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
	} else {
		isUpdated := false
		if instance.Spec.PasswordSecret != nil && instance.Spec.PasswordSecret.Name != "" {
			secretName := instance.Spec.PasswordSecret.Name
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

	// ================ setup default end ===================

	return r.Engine.Run(ctx, &instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DistributedRedisClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.DistributedRedisCluster{}).
		// WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 16}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

// // ensureServiceMonitor
// TODO
// func ensureServiceMonitor(ctx context.Context, cluster types.RedisClusterInstance) *actor.ActorResult {
// 	cr := cluster.Definition()
// 	watchNamespace := config.GetWatchNamespace()
//
// 	sm := clusterbuilder.NewServiceMonitorForCR(cr)
// 	if oldSm, err := a.client.GetServiceMonitor(ctx, namespace, sm.GetName()); errors.IsNotFound(err) {
// 		if err := a.client.CreateServiceMonitor(ctx, namespace, sm); err != nil {
// 			a.logger.Error(err, "create servicemonitor failed", "target", client.ObjectKeyFromObject(sm))
// 			return actor.NewResultWithError(cops.CommandRequeue, err)
// 		}
// 	} else if err != nil {
// 		a.logger.Error(err, "load servicemonitor failed", "target", client.ObjectKeyFromObject(sm))
// 		return actor.NewResultWithError(cops.CommandRequeue, err)
// 	} else {
// 		if len(oldSm.Spec.Endpoints) == 0 ||
// 			len(oldSm.Spec.Endpoints[0].MetricRelabelConfigs) == 0 ||
// 			cr.Spec.ServiceMonitor.CustomMetricRelabelings {
// 			if err := a.client.CreateOrUpdateServiceMonitor(ctx, cr.GetNamespace(), sm); err != nil {
// 				a.logger.Error(err, "update servicemonitor failed", "target", client.ObjectKeyFromObject(sm))
// 				return actor.NewResultWithError(cops.CommandRequeue, err)
// 			}
// 		}
// 	}
// 	return nil
// }
