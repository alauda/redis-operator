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

package v1

import (
	"context"
	"fmt"
	"strings"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	clusterv1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const RedisUserFinalizer = "redisusers.redis.middleware.alauda.io/finalizer"

var redislog = log.Log.WithName("redisuser-webhook")
var dyClient client.Client

func (r *RedisUser) SetupWebhookWithManager(mgr ctrl.Manager) error {
	dyClient = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/mutate-redis-middleware-alauda-io-v1-redisuser,mutating=true,failurePolicy=fail,groups=redis.middleware.alauda.io,resources=redisusers,versions=v1,name=mredisuser.kb.io,sideEffects=none,admissionReviewVersions=v1beta1
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-redis-middleware-alauda-io-v1-redisuser,mutating=false,failurePolicy=fail,groups=redis.middleware.alauda.io,resources=redisusers,versions=v1,name=vredisuser.kb.io,sideEffects=none,admissionReviewVersions=v1beta1

var _ webhook.Validator = &RedisUser{}
var _ webhook.Defaulter = &RedisUser{}

func (r *RedisUser) Default() {
	redislog.V(3).Info("default", "redisUser.name", r.Name)
	if r.Labels == nil {
		r.Labels = make(map[string]string)
	}
	r.Labels["managed-by"] = "redis-operator"
	r.Labels["middleware.instance/name"] = r.Spec.RedisName

	if r.GetDeletionTimestamp() != nil {
		return
	}
	if r.Spec.AclRules != "" {
		// 转为小写
		r.Spec.AclRules = strings.ToLower(r.Spec.AclRules)
	}
	if r.Spec.AccountType == Custom {
		existsAcl := false
		for _, v := range strings.Split(r.Spec.AclRules, " ") {
			if v == "-acl" || v == "-@admin" || v == "-@all" || v == "-@dangerous" {
				existsAcl = true
				break
			}
		}
		if !existsAcl {
			r.Spec.AclRules = fmt.Sprintf("%s -acl", r.Spec.AclRules)
		}
	}

	switch r.Spec.Arch {
	case redis.SentinelArch:
		rf := &databasesv1.RedisFailover{}
		_err := dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      r.Spec.RedisName}, rf)
		if _err != nil {
			redislog.Error(_err, "get redis failover failed", "name", r.Name)
		} else {
			r.OwnerReferences = util.BuildOwnerReferencesWithParents(rf)
		}
	case redis.ClusterArch:
		cluster := &clusterv1.DistributedRedisCluster{}

		_err := dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      r.Spec.RedisName}, cluster)
		if _err != nil {
			redislog.Error(_err, "get redis cluster failed", "name", r.Name)
		} else {
			r.OwnerReferences = util.BuildOwnerReferencesWithParents(cluster)
		}
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RedisUser) ValidateCreate() (admission.Warnings, error) {
	redislog.V(3).Info("validate create", "name", r.Name)
	if r.GetDeletionTimestamp() != nil {
		return nil, nil
	}
	if r.Spec.AccountType == Custom {
		err := util.CheckUserRuleUpdate(r.Spec.AclRules)
		if err != nil {
			return nil, fmt.Errorf("acl rules is invalid: %v", err)
		}
	}
	if r.Spec.AccountType == System {
		if r.Spec.Username != "operator" {
			return nil, fmt.Errorf("system account username must be operator")
		}
		return nil, nil
	}
	if r.Spec.AccountType == Default {
		if r.Spec.Username != "default" {
			return nil, fmt.Errorf("default account username must be default")
		}
		err := util.CheckRule(r.Spec.AclRules)
		if err != nil {
			return nil, fmt.Errorf("acl rules is invalid: %v", err)
		}
		for _, v := range r.Spec.PasswordSecrets {
			if v == "" {
				continue
			}
			_secret := &v1.Secret{}
			err := dyClient.Get(context.Background(), types.NamespacedName{
				Namespace: r.Namespace,
				Name:      v,
			}, _secret)
			if err != nil {
				return nil, err
			} else {
				if v, ok := _secret.Data["password"]; len(v) == 0 || !ok {
					return nil, fmt.Errorf("password secret key password is empty or no exists")
				}
			}
		}
		return nil, nil
	}

	switch r.Spec.Arch {
	case redis.SentinelArch:
		rf := &databasesv1.RedisFailover{}
		_err := dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      r.Spec.RedisName}, rf)
		if _err != nil {
			return nil, _err
		}
		if rf.Status.Phase != databasesv1.PhaseReady {
			return nil, fmt.Errorf("redis failover %s is not ready", r.Spec.RedisName)
		}
	case redis.ClusterArch:
		cluster := clusterv1.DistributedRedisCluster{}
		_err := dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      r.Spec.RedisName}, &cluster)
		if _err != nil {
			return nil, _err
		}
		if cluster.Status.Status != clusterv1.ClusterStatusOK {
			return nil, fmt.Errorf("redis cluster %s is not ready", r.Spec.RedisName)
		}
	}

	err := util.CheckRule(r.Spec.AclRules)
	if err != nil {
		return nil, fmt.Errorf("acl rules is invalid: %v", err)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *RedisUser) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	if r.Spec.AccountType == System {
		return nil, nil
	}
	if r.GetDeletionTimestamp() != nil {
		return nil, nil
	}
	if !controllerutil.ContainsFinalizer(r, RedisUserFinalizer) {
		return nil, nil
	}
	redislog.V(3).Info("validate update", "name", r.Name)
	err := util.CheckRule(r.Spec.AclRules)
	if err != nil {
		return nil, fmt.Errorf("acl rules is invalid: %v", err)
	}
	if err := util.CheckUserRuleUpdate(r.Spec.AclRules); err != nil {
		return nil, fmt.Errorf("acl rules is invalid: %v", err)
	}

	switch r.Spec.Arch {
	case redis.SentinelArch:
		rf := &databasesv1.RedisFailover{}
		err = dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      r.Spec.RedisName}, rf)
		if err != nil {
			return nil, err
		}
		if rf.Status.Phase != databasesv1.PhaseReady {
			return nil, fmt.Errorf("redis failover %s is not ready", r.Spec.RedisName)
		}
	case redis.ClusterArch:
		cluster := clusterv1.DistributedRedisCluster{}
		err = dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      r.Spec.RedisName}, &cluster)
		if err != nil {
			return nil, err
		}
		if cluster.Status.Status != clusterv1.ClusterStatusOK {
			return nil, fmt.Errorf("redis cluster %s is not ready", r.Spec.RedisName)
		}
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RedisUser) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}
