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

package redisuser

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	"github.com/alauda/redis-operator/api/core"
	ruv1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/builder/clusterbuilder"
	"github.com/alauda/redis-operator/internal/builder/failoverbuilder"
	"github.com/alauda/redis-operator/internal/redis/cluster"
	"github.com/alauda/redis-operator/internal/redis/failover"
	"github.com/alauda/redis-operator/pkg/kubernetes"
)

type RedisUserHandler struct {
	k8sClient     kubernetes.ClientSet
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

func NewRedisUserHandler(k8sservice kubernetes.ClientSet, eventRecorder record.EventRecorder, logger logr.Logger) *RedisUserHandler {
	return &RedisUserHandler{
		k8sClient:     k8sservice,
		eventRecorder: eventRecorder,
		logger:        logger.WithName("RedisUserHandler"),
	}
}

func (r *RedisUserHandler) Delete(ctx context.Context, inst ruv1.RedisUser, logger logr.Logger) error {
	logger.V(3).Info("redis user delete", "redis user name", inst.Name, "type", inst.Spec.Arch)
	if inst.Spec.Username == user.DefaultUserName || inst.Spec.Username == user.DefaultOperatorUserName {
		return nil
	}

	name := clusterbuilder.GenerateClusterACLConfigMapName(inst.Spec.RedisName)
	if inst.Spec.Arch == core.RedisSentinel {
		name = failoverbuilder.GenerateFailoverACLConfigMapName(inst.Spec.RedisName)
	}
	if configMap, err := r.k8sClient.GetConfigMap(ctx, inst.Namespace, name); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "delete user from configmap failed")
			return err
		}
	} else if _, ok := configMap.Data[inst.Spec.Username]; ok {
		delete(configMap.Data, inst.Spec.Username)
		if err := r.k8sClient.UpdateConfigMap(ctx, inst.Namespace, configMap); err != nil {
			logger.Error(err, "delete user from configmap failed", "configmap", configMap.Name)
			return err
		}
	}

	switch inst.Spec.Arch {
	case core.RedisCluster:
		logger.V(3).Info("cluster", "redis name", inst.Spec.RedisName)
		rc, err := r.k8sClient.GetDistributedRedisCluster(ctx, inst.Namespace, inst.Spec.RedisName)
		if errors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}

		rcm, err := cluster.NewRedisCluster(ctx, r.k8sClient, r.eventRecorder, rc, logger)
		if err != nil {
			return err
		}
		if !rcm.IsReady() {
			logger.V(3).Info("redis instance is not ready", "redis name", inst.Spec.RedisName)
			return fmt.Errorf("redis instance is not ready")
		}

		for _, node := range rcm.Nodes() {
			err := node.Setup(ctx, []interface{}{"ACL", "DELUSER", inst.Spec.Username})
			if err != nil {
				logger.Error(err, "acl del user failed", "node", node.GetName())
				return err
			}
			logger.V(3).Info("acl del user success", "node", node.GetName())
		}
	case core.RedisSentinel, core.RedisStandalone:
		logger.V(3).Info("sentinel", "redis name", inst.Spec.RedisName)
		rf, err := r.k8sClient.GetRedisFailover(ctx, inst.Namespace, inst.Spec.RedisName)
		if errors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}

		rfm, err := failover.NewRedisFailover(ctx, r.k8sClient, r.eventRecorder, rf, logger)
		if err != nil {
			return err
		}
		if !rfm.IsReady() {
			logger.V(3).Info("redis instance is not ready", "redis name", inst.Spec.RedisName)
			return fmt.Errorf("redis instance is not ready")
		}
		for _, node := range rfm.Nodes() {
			err := node.Setup(ctx, []interface{}{"ACL", "DELUSER", inst.Spec.Username})
			if err != nil {
				logger.Error(err, "acl del user failed", "node", node.GetName())
				return err
			}
			logger.V(3).Info("acl del user success", "node", node.GetName())
		}
	}
	return nil
}

func (r *RedisUserHandler) Do(ctx context.Context, inst ruv1.RedisUser, logger logr.Logger) error {
	if inst.Annotations == nil {
		inst.Annotations = map[string]string{}
	}

	passwords := []string{}
	// operators account skip
	if inst.Spec.AccountType == ruv1.System {
		return nil
	}

	userPassword := &user.Password{}
	for _, secretName := range inst.Spec.PasswordSecrets {
		secret, err := r.k8sClient.GetSecret(ctx, inst.Namespace, secretName)
		if err != nil {
			return err
		}
		if secret.GetLabels() == nil {
			secret.SetLabels(map[string]string{})
		}

		if secret.Labels[builder.InstanceNameLabel] != inst.Spec.RedisName ||
			len(secret.GetOwnerReferences()) == 0 || secret.OwnerReferences[0].UID != inst.GetUID() {

			secret.Labels[builder.ManagedByLabel] = "redis-operator"
			secret.Labels[builder.InstanceNameLabel] = inst.Spec.RedisName
			secret.OwnerReferences = util.BuildOwnerReferences(&inst)
			if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return r.k8sClient.UpdateSecret(ctx, inst.Namespace, secret)
			}); err != nil {
				logger.Error(err, "update secret owner failed", "secret", secret.Name)
				return err
			}
		}
		passwords = append(passwords, string(secret.Data["password"]))
		userPassword = &user.Password{
			SecretName: secretName,
		}
	}

	logger.V(3).Info("redis user do", "redis user name", inst.Name, "type", inst.Spec.Arch)
	switch inst.Spec.Arch {
	case core.RedisCluster:
		logger.V(3).Info("cluster", "redis name", inst.Spec.RedisName)
		rc, err := r.k8sClient.GetDistributedRedisCluster(ctx, inst.Namespace, inst.Spec.RedisName)
		if err != nil {
			return err
		}

		rcm, err := cluster.NewRedisCluster(ctx, r.k8sClient, r.eventRecorder, rc, logger)
		if err != nil {
			return err
		}
		if !rcm.IsReady() {
			logger.V(3).Info("redis instance is not ready", "redis name", inst.Spec.RedisName)
			return fmt.Errorf("redis instance is not ready")
		}

		aclRules := inst.Spec.AclRules
		if rcm.Version().IsACLSupported() {
			rule, err := user.NewRule(inst.Spec.AclRules)
			if err != nil {
				logger.V(3).Info("rule parse failed", "rule", inst.Spec.AclRules)
				return err
			}
			rule = user.PatchRedisClusterClientRequiredRules(rule)
			aclRules = rule.Encode()
		}
		userObj, err := user.NewUserFromRedisUser(inst.Spec.Username, aclRules, userPassword)
		if err != nil {
			return err
		}
		info, err := json.Marshal(userObj)
		if err != nil {
			return err
		}

		configmap, err := r.k8sClient.GetConfigMap(ctx, inst.Namespace, clusterbuilder.GenerateClusterACLConfigMapName(inst.Spec.RedisName))
		if err != nil {
			return err
		}
		configmap.Data[inst.Spec.Username] = string(info)

		if inst.Spec.AccountType != ruv1.System {
			for _, node := range rcm.Nodes() {
				_, err := node.SetACLUser(ctx, inst.Spec.Username, passwords, aclRules)
				if err != nil {
					logger.Error(err, "acl set user failed", "node", node.GetName())
					return err
				}
				logger.V(3).Info("acl set user success", "node", node.GetName())
			}
		} else {
			logger.V(3).Info("skip system account online update", "username", inst.Spec.Username)
		}

		if err := r.k8sClient.UpdateConfigMap(ctx, inst.Namespace, configmap); err != nil {
			logger.Error(err, "update configmap failed", "configmap", configmap.Name)
			return err
		}
	case core.RedisSentinel, core.RedisStandalone:
		logger.V(3).Info("sentinel", "redis name", inst.Spec.RedisName)
		rf, err := r.k8sClient.GetRedisFailover(ctx, inst.Namespace, inst.Spec.RedisName)
		if err != nil {
			return err
		}
		rfm, err := failover.NewRedisFailover(ctx, r.k8sClient, r.eventRecorder, rf, logger)
		if err != nil {
			return err
		}
		if !rfm.IsReady() {
			logger.V(3).Info("redis instance is not ready", "redis name", inst.Spec.RedisName)
			return fmt.Errorf("redis instance is not ready")
		}
		configmap, err := r.k8sClient.GetConfigMap(ctx, inst.Namespace, failoverbuilder.GenerateFailoverACLConfigMapName(inst.Spec.RedisName))
		if err != nil {
			return err
		}
		userObj, err := user.NewUserFromRedisUser(inst.Spec.Username, inst.Spec.AclRules, userPassword)
		if err != nil {
			return err
		}
		info, err := json.Marshal(userObj)
		if err != nil {
			return err
		}
		configmap.Data[inst.Spec.Username] = string(info)

		if inst.Spec.AccountType != ruv1.System {
			for _, node := range rfm.Nodes() {
				_, err := node.SetACLUser(ctx, inst.Spec.Username, passwords, inst.Spec.AclRules)
				if err != nil {
					logger.Error(err, "acl set user failed", "node", node.GetName())
					return err
				}
				logger.V(3).Info("acl set user success", "node", node.GetName())
			}
		} else {
			logger.V(3).Info("skip system account online update", "username", inst.Spec.Username)
		}

		if err := r.k8sClient.UpdateConfigMap(ctx, inst.Namespace, configmap); err != nil {
			logger.Error(err, "update configmap failed", "configmap", configmap.Name)
			return err
		}
	}
	return nil
}
