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

package redis

import (
	"context"
	"reflect"
	"strings"
	"time"

	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/internal/controller/redis/redisuser"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RedisUserReconciler reconciles a RedisUser object
type RedisUserReconciler struct {
	client.Client
	K8sClient kubernetes.ClientSet
	Scheme    *runtime.Scheme
	Logger    logr.Logger
	Record    record.EventRecorder
	Handler   redisuser.RedisUserHandler
}

//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisusers/finalizers,verbs=update

func (r *RedisUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger.Info("redis user reconcile", "redis user name", req.Name)
	instance := redismiddlewarealaudaiov1.RedisUser{}
	err := r.Client.Get(ctx, req.NamespacedName, &instance)
	if err != nil {
		r.Logger.Error(err, "get redis user failed")
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	isMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		r.Logger.Info("finalizeRedisUser", "Namespace", instance.Namespace, "Instance", instance.Name)
		if err := r.Handler.Delete(ctx, instance); err != nil {
			if instance.Status.Message != err.Error() {
				instance.Status.Phase = redismiddlewarealaudaiov1.Fail
				instance.Status.Message = "Delete Fail:" + err.Error()
				if _err := r.Client.Status().Update(ctx, &instance); _err != nil {
					r.Logger.Error(_err, "update user status failed", "instance", req.NamespacedName)
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		} else {
			r.Logger.Info("RemoveFinalizer", "Namespace", instance.Namespace, "Instance", instance.Name)
			controllerutil.RemoveFinalizer(&instance, redismiddlewarealaudaiov1.RedisUserFinalizer)
			err := r.Update(ctx, &instance)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if err := r.Handler.Do(ctx, instance); err != nil {
		if strings.Contains(err.Error(), "redis instance is not ready") ||
			strings.Contains(err.Error(), "node not ready") ||
			strings.Contains(err.Error(), "ERR unknown command `ACL`") {
			r.Logger.Info("redis instance is not ready", "redis name", instance.Spec.RedisName)
			instance.Status.Message = err.Error()
			instance.Status.Phase = redismiddlewarealaudaiov1.Pending
			instance.Status.LastUpdatedSuccess = time.Now().Format(time.RFC3339)
			_err := r.Client.Status().Update(ctx, &instance)
			if _err != nil {
				r.Logger.Error(_err, "update redis user status failed")
			}
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}
		instance.Status.Message = err.Error()
		instance.Status.Phase = redismiddlewarealaudaiov1.Fail
		instance.Status.LastUpdatedSuccess = time.Now().Format(time.RFC3339)
		r.Logger.Error(err, "redis user reconcile failed")
		_err := r.Client.Status().Update(ctx, &instance)
		if _err != nil {
			r.Logger.Error(_err, "update redis user status failed")
		}
		return reconcile.Result{}, err
	}
	instance.Status.Phase = redismiddlewarealaudaiov1.Success
	instance.Status.Message = ""
	instance.Status.LastUpdatedSuccess = time.Now().Format(time.RFC3339)
	r.Logger.V(3).Info("redis user reconcile success")
	err = r.Client.Status().Update(ctx, &instance)
	if err != nil {
		r.Logger.Error(err, "update redis user status failed")
	}
	if !controllerutil.ContainsFinalizer(&instance, redismiddlewarealaudaiov1.RedisUserFinalizer) {
		controllerutil.AddFinalizer(&instance, redismiddlewarealaudaiov1.RedisUserFinalizer)
		err := r.Update(ctx, &instance)
		if err != nil {
			r.Logger.Error(err, "update redis finalizer user failed")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *RedisUserReconciler) SetupEventRecord(mgr ctrl.Manager) {
	r.Record = mgr.GetEventRecorderFor("redis-user-operator")
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Handler = *redisuser.NewRedisUserHandler(r.K8sClient, r.Logger)
	return ctrl.NewControllerManagedBy(mgr).
		For(&redismiddlewarealaudaiov1.RedisUser{}).
		Owns(&corev1.Secret{}).
		WithEventFilter(CustomGenerationChangedPredicate(r.Logger)).
		Complete(r)
}

// Imitate predicate.GenerationChangedPredicate to write
// But let the Secret object pass, because the Secret object will not trigger reconcile

func CustomGenerationChangedPredicate(logger logr.Logger) predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			//logger.Info("UpdateFunc", "old", e.ObjectOld, "new", e.ObjectNew, "generation", e.ObjectNew.GetGeneration())
			if e.ObjectOld == nil {
				return false
			}
			if e.ObjectNew == nil {
				return false
			}
			if reflect.TypeOf(e.ObjectNew).String() == reflect.TypeOf(&corev1.Secret{}).String() {
				return true
			}
			// Ignore updates to CR status in which case metadata.Generation does not change
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
	}
}
