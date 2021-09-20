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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	middlewarev1 "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/internal/controller/redis/redisclusterbackup"
	"github.com/alauda/redis-operator/internal/controller/redis/redisclusterbackup/service"
	k8s "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/clientset"
	"github.com/go-logr/logr"
)

// RedisBackupReconciler reconciles a RedisBackup object
type RedisClusterBackupReconciler struct {
	client.Client
	k8sClient k8s.ClientSet
	handler   *redisclusterbackup.RedisClusterBackupHandler
	Logger    logr.Logger
	Scheme    *runtime.Scheme
}

const RedisClusterBackupFinalizer = "redisclusterbackups.redis.middleware.alauda.io/finalizer"

//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisclusterbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisclusterbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=redisclusterbackups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *RedisClusterBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := middlewarev1.RedisClusterBackup{}
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "get instance failed", "instance", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "get instance failed", "instance", req.NamespacedName)
		return ctrl.Result{}, err
	}
	isMarkedToBeDeleted := obj.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		logger.Info("finalizeRedisBackup", "Namespace", obj.Namespace, "Instance", obj.Name)
		if err := r.finalizeRedisBackup(ctx, logger, &obj); err != nil {
			if obj.Status.Message != err.Error() {
				obj.Status.Condition = middlewarev1.RedisDeleteFailed
				obj.Status.Message = err.Error()
				if _err := r.k8sClient.UpdateRedisClusterBackupStatus(ctx, &obj); _err != nil {
					logger.Error(_err, "update backup status failed", "instance", req.NamespacedName)
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, err
		} else {
			logger.Info("RemoveFinalizer", "Namespace", obj.Namespace, "Instance", obj.Name)
			controllerutil.RemoveFinalizer(&obj, RedisBackupFinalizer)
			err := r.Update(ctx, &obj)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if err := obj.Validate(); err != nil {
		logger.Error(err, "validate failed", "instance", req.NamespacedName)
		obj.Status.Condition = middlewarev1.RedisBackupFailed
		obj.Status.Message = err.Error()
		if err := r.k8sClient.UpdateRedisClusterBackupStatus(ctx, &obj); err != nil {
			logger.Error(err, "update backup status failed", "instance", req.NamespacedName)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if err := r.handler.Ensure(ctx, &obj); err != nil {
		return ctrl.Result{}, err
	}

	obj = middlewarev1.RedisClusterBackup{}
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "get instance failed", "instance", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(&obj, RedisBackupFinalizer) {
		controllerutil.AddFinalizer(&obj, RedisBackupFinalizer)
		err := r.Update(ctx, &obj)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if obj.Status.Condition == middlewarev1.RedisBackupRunning {
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisClusterBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.SetupHandler(mgr)

	return ctrl.NewControllerManagedBy(mgr).
		For(&middlewarev1.RedisClusterBackup{}).
		Complete(r)
}

// SetupHandler
func (r *RedisClusterBackupReconciler) SetupHandler(mgr ctrl.Manager) {
	r.k8sClient = clientset.New(mgr.GetClient(), r.Logger)
	client := service.NewRedisClusterBackupKubeClient(r.k8sClient, r.Logger)
	r.handler = redisclusterbackup.NewRedisClusterBackupHandler(r.k8sClient, client, r.Logger)
}

func (r *RedisClusterBackupReconciler) finalizeRedisBackup(ctx context.Context, reqLogger logr.Logger, backup *middlewarev1.RedisClusterBackup) error {
	if backup.Spec.Target.S3Option.S3Secret != "" {
		if err := r.handler.DeleteS3(ctx, backup); err != nil {
			reqLogger.Info("DeleteS3", "Err", err.Error())
			return err
		}
	}

	return nil
}
