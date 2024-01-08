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
	"strings"

	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"

	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/sentinelbuilder"
	clustermodel "github.com/alauda/redis-operator/pkg/models/cluster"
	sentinelmodle "github.com/alauda/redis-operator/pkg/models/sentinel"
	"github.com/go-logr/logr"
	"k8s.io/utils/strings/slices"
)

type RedisUserHandler struct {
	k8sClient kubernetes.ClientSet
	logger    logr.Logger
}

func NewRedisUserHandler(k8sservice kubernetes.ClientSet, logger logr.Logger) *RedisUserHandler {
	return &RedisUserHandler{
		k8sClient: k8sservice,
		logger:    logger,
	}
}

func (r *RedisUserHandler) Delete(ctx context.Context, instance redismiddlewarealaudaiov1.RedisUser) error {
	r.logger.V(3).Info("redis user delete", "redis user name", instance.Name, "type", instance.Spec.Arch)
	switch instance.Spec.Arch {
	case redis.ClusterArch:
		r.logger.V(3).Info("cluster", "redis name", instance.Spec.RedisName)
		rc, err := r.k8sClient.GetDistributedRedisCluster(ctx, instance.Namespace, instance.Spec.RedisName)
		if err != nil {
			if errors.IsNotFound(err) {
				r.logger.Info("redis instance is not found", "redis name", instance.Spec.RedisName)
				return nil
			}
			return err
		}
		rcm, err := clustermodel.NewRedisCluster(ctx, r.k8sClient, rc, r.logger)
		if !rcm.IsReady() {
			r.logger.Info("redis instance is not ready", "redis name", instance.Spec.RedisName)
			return fmt.Errorf("redis instance is not ready")
		}
		if err != nil {
			return err
		}

		for _, node := range rcm.Nodes() {
			err := node.Setup(ctx, []interface{}{"ACL", "DELUSER", instance.Spec.Username})
			if err != nil {
				r.logger.Error(err, "acl del user failed", "node", node.GetName())
				return err
			}
			r.logger.Info("acl del user success", "node", node.GetName())
		}
		configMap, err := r.k8sClient.GetConfigMap(ctx, instance.Namespace, clusterbuilder.GenerateClusterACLConfigMapName(instance.Spec.RedisName))
		if err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		delete(configMap.Data, instance.Spec.Username)
		if err := r.k8sClient.UpdateConfigMap(ctx, instance.Namespace, configMap); err != nil {
			return err
		}
	case redis.SentinelArch:
		r.logger.V(3).Info("sentinel", "redis name", instance.Spec.RedisName)
		rf, err := r.k8sClient.GetRedisFailover(ctx, instance.Namespace, instance.Spec.RedisName)
		if err != nil {
			if errors.IsNotFound(err) {
				r.logger.Info("redis instance is not found", "redis name", instance.Spec.RedisName)
				return nil
			}
			return err
		}

		rfm, err := sentinelmodle.NewRedisFailover(ctx, r.k8sClient, rf, r.logger)
		if !rfm.IsReady() {
			r.logger.Info("redis instance is not ready", "redis name", instance.Spec.RedisName)
			return fmt.Errorf("redis instance is not ready")
		}
		if err != nil {
			return err
		}
		for _, node := range rfm.Nodes() {
			err := node.Setup(ctx, []interface{}{"ACL", "DELUSER", instance.Spec.Username})
			if err != nil {
				r.logger.Error(err, "acl del user failed", "node", node.GetName())
				return err
			}
			r.logger.Info("acl del user success", "node", node.GetName())
		}

	}
	return nil
}

func (r *RedisUserHandler) Do(ctx context.Context, instance redismiddlewarealaudaiov1.RedisUser) error {
	passwords := []string{}
	// operators account skip
	if instance.Spec.AccountType == redismiddlewarealaudaiov1.System {
		return nil
	}
	password_obj := &user.Password{}
	for _, secretName := range instance.Spec.PasswordSecrets {
		secret, err := r.k8sClient.GetSecret(ctx, instance.Namespace, secretName)
		if err != nil {
			return err
		}
		if secret.GetLabels() == nil {
			secret.SetLabels(map[string]string{})
		}

		if secret.Labels["middleware.instance/name"] != instance.Spec.RedisName {
			secret.Labels["managed-by"] = "redis-operator"
			secret.Labels["middleware.instance/name"] = instance.Spec.RedisName
			secret.OwnerReferences = util.BuildOwnerReferences(&instance)
			err = r.k8sClient.UpdateSecret(ctx, instance.Namespace, secret)
			if err != nil {
				return err
			}
		}
		passwords = append(passwords, string(secret.Data["password"]))
		password_obj = &user.Password{
			SecretName: secretName,
		}
	}

	r.logger.Info("redis user do", "redis user name", instance.Name, "type", instance.Spec.Arch)
	switch instance.Spec.Arch {
	case redis.ClusterArch:
		r.logger.Info("cluster", "redis name", instance.Spec.RedisName)
		rc, err := r.k8sClient.GetDistributedRedisCluster(ctx, instance.Namespace, instance.Spec.RedisName)
		if err != nil {
			return err
		}
		rcm, err := clustermodel.NewRedisCluster(ctx, r.k8sClient, rc, r.logger)
		if err != nil {
			return err
		}
		if !rcm.IsReady() {
			r.logger.Info("redis instance is not ready", "redis name", instance.Spec.RedisName)
			return fmt.Errorf("redis instance is not ready")
		}

		configmap, err := r.k8sClient.GetConfigMap(ctx, instance.Namespace, clusterbuilder.GenerateClusterACLConfigMapName(instance.Spec.RedisName))
		if err != nil {
			return err
		}
		rule := instance.Spec.AclRules
		if rcm.Version().IsACLSupported() {
			rule = AddClusterRule(instance.Spec.AclRules)
		}
		userObj, err := user.NewUserFromRedisUser(instance.Spec.Username, rule, password_obj)
		if err != nil {
			return err
		}
		info, err := json.Marshal(userObj)
		if err != nil {
			return err
		}
		configmap.Data[instance.Spec.Username] = string(info)

		for _, node := range rcm.Nodes() {

			_, err := node.SetACLUser(ctx, instance.Spec.Username, passwords, rule)
			if err != nil {
				r.logger.Error(err, "acl set user failed", "node", node.GetName())
				return err
			}
			r.logger.Info("acl set user success", "node", node.GetName())
		}

		if err := r.k8sClient.UpdateConfigMap(ctx, instance.Namespace, configmap); err != nil {
			return err
		}
	case redis.SentinelArch:
		r.logger.Info("sentinel", "redis name", instance.Spec.RedisName)
		rf, err := r.k8sClient.GetRedisFailover(ctx, instance.Namespace, instance.Spec.RedisName)
		if err != nil {
			return err
		}
		rfm, err := sentinelmodle.NewRedisFailover(ctx, r.k8sClient, rf, r.logger)
		if err != nil {
			return err
		}
		if !rfm.IsReady() {
			r.logger.Info("redis instance is not ready", "redis name", instance.Spec.RedisName)
			return fmt.Errorf("redis instance is not ready")
		}
		configmap, err := r.k8sClient.GetConfigMap(ctx, instance.Namespace, sentinelbuilder.GenerateSentinelACLConfigMapName(instance.Spec.RedisName))
		if err != nil {
			return err
		}
		userObj, err := user.NewUserFromRedisUser(instance.Spec.Username, instance.Spec.AclRules, password_obj)
		if err != nil {
			return err
		}
		info, err := json.Marshal(userObj)
		if err != nil {
			return err
		}
		configmap.Data[instance.Spec.Username] = string(info)
		for _, node := range rfm.Nodes() {
			_, err := node.SetACLUser(ctx, instance.Spec.Username, passwords, instance.Spec.AclRules)
			if err != nil {
				r.logger.Error(err, "acl set user failed", "node", node.GetName())
				return err
			}
			r.logger.Info("acl set user success", "node", node.GetName())
		}
		if err := r.k8sClient.UpdateConfigMap(ctx, instance.Namespace, configmap); err != nil {
			return err
		}

	}

	return nil
}

// 补充cluster 规则
func AddClusterRule(rules string) string {
	//slice rule
	for _, rule := range strings.Split(rules, " ") {
		if slices.Contains([]string{"-@all", "-@dangerous", "-@admin", "-@slow"}, rule) {
			rules += " " + "+cluster|slots +cluster|nodes +cluster|info +cluster|keyslot +cluster|GETKEYSINSLOT +cluster|COUNTKEYSINSLOT"
			return rules
		}
	}
	return rules
}
