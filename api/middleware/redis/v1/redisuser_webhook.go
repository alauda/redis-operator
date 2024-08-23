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
	"slices"
	"strings"

	clusterv1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/util"
	security "github.com/alauda/redis-operator/pkg/security/password"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/user"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	RedisUserFinalizer = "redisusers.redis.middleware.alauda.io/finalizer"

	ACLSupportedVersionAnnotationKey = "middleware.alauda.io/acl-supported-version"
)

// log is for logging in this package.
var logger = logf.Log.WithName("RedisUser")
var dyClient client.Client

func (r *RedisUser) SetupWebhookWithManager(mgr ctrl.Manager) error {
	dyClient = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/mutate-redis-middleware-alauda-io-v1-redisuser,mutating=true,failurePolicy=fail,groups=redis.middleware.alauda.io,resources=redisusers,versions=v1,name=mredisuser.kb.io,sideEffects=none,admissionReviewVersions=v1
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-redis-middleware-alauda-io-v1-redisuser,mutating=false,failurePolicy=fail,groups=redis.middleware.alauda.io,resources=redisusers,versions=v1,name=vredisuser.kb.io,sideEffects=none,admissionReviewVersions=v1

var _ webhook.Validator = &RedisUser{}
var _ webhook.Defaulter = &RedisUser{}

func (r *RedisUser) Default() {
	logger.V(3).Info("default", "redisUser.name", r.Name)
	if r.Labels == nil {
		r.Labels = make(map[string]string)
	}
	r.Labels[builder.ManagedByLabel] = "redis-operator"
	r.Labels[builder.InstanceNameLabel] = r.Spec.RedisName

	if r.GetDeletionTimestamp() != nil {
		return
	}
	rule, err := user.NewRule(r.Spec.AclRules)
	if err != nil {
		return
	}

	version := redis.RedisVersion(config.GetRedisVersion(config.GetDefaultRedisImage()))
	switch r.Spec.Arch {
	case core.RedisSentinel, core.RedisStandalone:
		rf := &databasesv1.RedisFailover{}
		if err := dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      r.Spec.RedisName}, rf); err != nil {
			logger.Error(err, "get redis failover failed", "name", r.Name)
		} else {
			r.OwnerReferences = util.BuildOwnerReferencesWithParents(rf)
			version = redis.RedisVersion(config.GetRedisVersion(rf.Spec.Redis.Image))
		}
	case core.RedisCluster:
		cluster := &clusterv1.DistributedRedisCluster{}
		if err := dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      r.Spec.RedisName}, cluster); err != nil {
			logger.Error(err, "get redis cluster failed", "name", r.Name)
		} else {
			r.OwnerReferences = util.BuildOwnerReferencesWithParents(cluster)
			version = redis.RedisVersion(config.GetRedisVersion(cluster.Spec.Image))
		}
	}

	if r.Spec.AccountType != System && rule.IsCommandEnabled("acl", []string{"all", "admin", "slow", "dangerous"}) {
		if slices.Contains(rule.AllowedCommands, "acl") {
			cmds := rule.AllowedCommands[:0]
			for _, cmd := range rule.AllowedCommands {
				if cmd != "acl" {
					cmds = append(cmds, cmd)
				}
			}
			rule.AllowedCommands = cmds
		}
		rule.DisallowedCommands = append(rule.DisallowedCommands, "acl")
	}
	if version.IsACL2Supported() {
		rule = user.PatchRedisPubsubRules(rule)
	}
	r.Spec.AclRules = rule.Encode()
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RedisUser) ValidateCreate() (admission.Warnings, error) {
	if r.Spec.AccountType == System {
		if r.Spec.Username != "operator" {
			return nil, fmt.Errorf("system account username must be operator")
		}
		return nil, nil
	}
	if r.Spec.AccountType == Default && r.Spec.Username != "default" {
		return nil, fmt.Errorf("default account username must be default")
	}
	if len(r.Spec.PasswordSecrets) == 0 && r.Spec.AccountType != Default {
		return nil, fmt.Errorf("password secret can not be empty")
	}

	rule, err := user.NewRule(strings.ToLower(r.Spec.AclRules))
	if err != nil {
		return nil, err
	}
	if err := rule.Validate(true); err != nil {
		return nil, err
	}
	for _, v := range r.Spec.PasswordSecrets {
		if v == "" {
			return nil, fmt.Errorf("password secret can not be empty")
		}
		secret := &v1.Secret{}
		if err := dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      v,
		}, secret); err != nil {
			return nil, err
		} else if err := security.PasswordValidate(string(secret.Data["password"]), 8, 32); err != nil {
			return nil, err
		}
	}

	if r.Spec.AccountType == Custom {
		switch r.Spec.Arch {
		case core.RedisSentinel, core.RedisStandalone:
			rf := &databasesv1.RedisFailover{}
			if err := dyClient.Get(context.Background(), types.NamespacedName{
				Namespace: r.Namespace,
				Name:      r.Spec.RedisName}, rf); err != nil {
				return nil, err
			}
			if rf.Status.Phase != databasesv1.Ready {
				return nil, fmt.Errorf("redis failover %s is not ready", r.Spec.RedisName)
			}
		case core.RedisCluster:
			cluster := clusterv1.DistributedRedisCluster{}
			if err := dyClient.Get(context.Background(), types.NamespacedName{
				Namespace: r.Namespace,
				Name:      r.Spec.RedisName}, &cluster); err != nil {
				return nil, err
			}
			if cluster.Status.Status != clusterv1.ClusterStatusOK {
				return nil, fmt.Errorf("redis cluster %s is not ready", r.Spec.RedisName)
			}
		}
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *RedisUser) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	if r.Spec.AccountType == System {
		if r.Spec.Username != "operator" {
			return nil, fmt.Errorf("system account username must be operator")
		}
		return nil, nil
	}
	if r.Spec.AccountType == Default && r.Spec.Username != "default" {
		return nil, fmt.Errorf("default account username must be default")
	}
	if r.GetDeletionTimestamp() != nil {
		return nil, nil
	}
	if len(r.Spec.PasswordSecrets) == 0 && r.Spec.AccountType != Default {
		return nil, fmt.Errorf("password secret can not be empty")
	}

	//???
	// if !controllerutil.ContainsFinalizer(r, RedisUserFinalizer) {
	// 	return nil
	// }

	rule, err := user.NewRule(strings.ToLower(r.Spec.AclRules))
	if err != nil {
		return nil, err
	}
	if err := rule.Validate(true); err != nil {
		return nil, err
	}
	for _, v := range r.Spec.PasswordSecrets {
		if v == "" {
			continue
		}
		secret := &v1.Secret{}
		if err := dyClient.Get(context.Background(), types.NamespacedName{
			Namespace: r.Namespace,
			Name:      v,
		}, secret); err != nil {
			return nil, err
		} else if err := security.PasswordValidate(string(secret.Data["password"]), 8, 32); err != nil {
			return nil, err
		}
	}

	if r.Spec.AccountType == Custom {
		switch r.Spec.Arch {
		case core.RedisSentinel, core.RedisStandalone:
			rf := &databasesv1.RedisFailover{}
			if err := dyClient.Get(context.Background(), types.NamespacedName{
				Namespace: r.Namespace,
				Name:      r.Spec.RedisName}, rf); err != nil {
				return nil, err
			}
			if rf.Status.Phase != databasesv1.Ready {
				return nil, fmt.Errorf("redis failover %s is not ready", r.Spec.RedisName)
			}
		case core.RedisCluster:
			cluster := clusterv1.DistributedRedisCluster{}
			if err := dyClient.Get(context.Background(), types.NamespacedName{
				Namespace: r.Namespace,
				Name:      r.Spec.RedisName}, &cluster); err != nil {
				return nil, err
			}
			if cluster.Status.Status != clusterv1.ClusterStatusOK {
				return nil, fmt.Errorf("redis cluster %s is not ready", r.Spec.RedisName)
			}
		}
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RedisUser) ValidateDelete() (admission.Warnings, error) {
	if r.Spec.Username == user.DefaultUserName || r.Spec.Username == user.DefaultOperatorUserName {
		switch r.Spec.Arch {
		case core.RedisSentinel, core.RedisStandalone:
			rf := databasesv1.RedisFailover{}
			if err := dyClient.Get(context.Background(), types.NamespacedName{
				Namespace: r.Namespace,
				Name:      r.Spec.RedisName,
			}, &rf); errors.IsNotFound(err) {
				return nil, nil
			} else if err != nil {
				return nil, err
			} else if rf.GetDeletionTimestamp() != nil {
				return nil, nil
			}
		case core.RedisCluster:
			cluster := clusterv1.DistributedRedisCluster{}
			if err := dyClient.Get(context.Background(), types.NamespacedName{
				Namespace: r.Namespace,
				Name:      r.Spec.RedisName,
			}, &cluster); errors.IsNotFound(err) {
				return nil, nil
			} else if err != nil {
				return nil, err
			} else if cluster.GetDeletionTimestamp() != nil {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("user %s can not be deleted", r.GetName())
	}
	return nil, nil
}
