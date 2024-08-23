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

package middleware

import (
	"context"
	"reflect"
	"strings"
	"time"

	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	"github.com/alauda/redis-operator/internal/controller/middleware/redisuser"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	security "github.com/alauda/redis-operator/pkg/security/password"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RedisUserReconciler reconciles a RedisUser object
type RedisUserReconciler struct {
	client.Client
	K8sClient kubernetes.ClientSet
	Scheme    *runtime.Scheme
	Record    record.EventRecorder
	Handler   redisuser.RedisUserHandler
	Logger    logr.Logger
}

//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisusers/finalizers,verbs=update

func (r *RedisUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(context.TODO()).WithName("RedisUser").WithValues("namespace", req.Namespace, "name", req.Name)

	instance := redismiddlewarealaudaiov1.RedisUser{}
	err := r.Client.Get(ctx, req.NamespacedName, &instance)
	if err != nil {
		logger.Error(err, "get redis user failed")
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	isMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		logger.Info("finalizeRedisUser", "Namespace", instance.Namespace, "Instance", instance.Name)
		if err := r.Handler.Delete(ctx, instance, logger); err != nil {
			if instance.Status.Message != err.Error() {
				instance.Status.Phase = redismiddlewarealaudaiov1.Fail
				instance.Status.Message = "clean user failed with error " + err.Error()
				if _err := r.Client.Status().Update(ctx, &instance); _err != nil {
					logger.Error(_err, "update user status failed", "instance", req.NamespacedName)
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{RequeueAfter: time.Second * 30}, err
		} else {
			logger.Info("RemoveFinalizer", "Namespace", instance.Namespace, "Instance", instance.Name)
			controllerutil.RemoveFinalizer(&instance, redismiddlewarealaudaiov1.RedisUserFinalizer)
			if err := r.Update(ctx, &instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// verify redis user password
	for _, name := range instance.Spec.PasswordSecrets {
		if name == "" {
			continue
		}
		secret := &v1.Secret{}
		if err := r.Get(context.Background(), types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      name,
		}, secret); err != nil {
			logger.Info("get secret failed", "secret name", name)
			instance.Status.Message = err.Error()
			instance.Status.Phase = redismiddlewarealaudaiov1.Fail
			return ctrl.Result{}, r.Client.Status().Update(ctx, &instance)
		} else if err := security.PasswordValidate(string(secret.Data["password"]), 8, 32); err != nil {
			if instance.Spec.AccountType != redismiddlewarealaudaiov1.System {
				instance.Status.Message = err.Error()
				instance.Status.Phase = redismiddlewarealaudaiov1.Fail
				return ctrl.Result{RequeueAfter: time.Minute}, r.Client.Status().Update(ctx, &instance)
			}
		}
	}

	if err := r.Handler.Do(ctx, instance, logger); err != nil {
		if strings.Contains(err.Error(), "redis instance is not ready") ||
			strings.Contains(err.Error(), "node not ready") ||
			strings.Contains(err.Error(), "user not operator") ||
			strings.Contains(err.Error(), "ERR unknown command `ACL`") {
			logger.V(3).Info("redis instance is not ready", "redis name", instance.Spec.RedisName)
			instance.Status.Message = err.Error()
			instance.Status.Phase = redismiddlewarealaudaiov1.Pending
			instance.Status.LastUpdatedSuccess = time.Now().Format(time.RFC3339)
			if err := r.updateRedisUserStatus(ctx, &instance); err != nil {
				logger.Error(err, "update redis user status to Pending failed")
			}
			return ctrl.Result{RequeueAfter: time.Second * 15}, nil
		}

		instance.Status.Message = err.Error()
		instance.Status.Phase = redismiddlewarealaudaiov1.Fail
		instance.Status.LastUpdatedSuccess = time.Now().Format(time.RFC3339)
		logger.Error(err, "redis user reconcile failed")
		if err := r.updateRedisUserStatus(ctx, &instance); err != nil {
			logger.Error(err, "update redis user status to Fail failed")
		}
		return reconcile.Result{RequeueAfter: time.Second * 10}, nil
	}
	instance.Status.Phase = redismiddlewarealaudaiov1.Success
	instance.Status.Message = ""
	instance.Status.LastUpdatedSuccess = time.Now().Format(time.RFC3339)
	logger.V(3).Info("redis user reconcile success")
	if err := r.updateRedisUserStatus(ctx, &instance); err != nil {
		logger.Error(err, "update redis user status to Success failed")
		return reconcile.Result{RequeueAfter: time.Second * 10}, err
	}
	if !controllerutil.ContainsFinalizer(&instance, redismiddlewarealaudaiov1.RedisUserFinalizer) {
		controllerutil.AddFinalizer(&instance, redismiddlewarealaudaiov1.RedisUserFinalizer)
		if err := r.updateRedisUser(ctx, &instance); err != nil {
			logger.Error(err, "update redis finalizer user failed")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *RedisUserReconciler) updateRedisUserStatus(ctx context.Context, inst *redismiddlewarealaudaiov1.RedisUser) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldUser redismiddlewarealaudaiov1.RedisUser
		if err := r.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: inst.Name}, &oldUser); err != nil {
			return err
		}
		inst.ResourceVersion = oldUser.ResourceVersion
		return r.Status().Update(ctx, inst)
	})
}

func (r *RedisUserReconciler) updateRedisUser(ctx context.Context, inst *redismiddlewarealaudaiov1.RedisUser) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldUser redismiddlewarealaudaiov1.RedisUser
		if err := r.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: inst.Name}, &oldUser); err != nil {
			return err
		}
		inst.ResourceVersion = oldUser.ResourceVersion
		return r.Update(ctx, inst)
	})
}

func (r *RedisUserReconciler) SetupEventRecord(mgr ctrl.Manager) {
	r.Record = mgr.GetEventRecorderFor("redis-user-operator")
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Handler = *redisuser.NewRedisUserHandler(r.K8sClient, r.Record, r.Logger)
	return ctrl.NewControllerManagedBy(mgr).
		For(&redismiddlewarealaudaiov1.RedisUser{}).
		Owns(&corev1.Secret{}).
		WithEventFilter(CustomGenerationChangedPredicate(r.Logger)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 4}).
		Complete(r)
}

// Imitate predicate.GenerationChangedPredicate to write
// But let the Secret object pass, because the Secret object will not trigger reconcile

func CustomGenerationChangedPredicate(logger logr.Logger) predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
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
