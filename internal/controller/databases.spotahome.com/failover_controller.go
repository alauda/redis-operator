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

package failover

import (
	"context"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/pkg/ops"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
		instance.Status.Phase = databasesv1.PhaseFail
		if err := r.Status().Update(ctx, &instance); err != nil {
			logger.Error(err, "update status failed")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	return r.Engine.Run(ctx, &instance)

}

func (r *RedisFailoverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasesv1.RedisFailover{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
