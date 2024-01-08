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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clusterv1alpha1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/ops"
)

// DistributedRedisClusterReconciler reconciles a DistributedRedisCluster object
type DistributedRedisClusterReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
	Engine        *ops.OpEngine
}

const (
	ControllerVersion      = "v1alpha1"
	ControllerVersionLabel = "controllerVersion"
)

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

	// switch for turn /on off this operator
	if instance.Labels[ControllerVersionLabel] == ControllerVersion {
		return ctrl.Result{}, nil
	}

	// NOTE: make sure not overwrite the labels from cr
	delete(instance.Labels, ControllerVersionLabel)

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

	// ================ setup default ===================

	// TODO: to this in webhook
	// here do some default value check and convert
	_ = instance.Init()

	// ================ setup default end ===================

	return r.Engine.Run(ctx, &instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DistributedRedisClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.DistributedRedisCluster{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
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
