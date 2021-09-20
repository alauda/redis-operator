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

package clusterbuilder

import (
	"fmt"

	redisv1alpha1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/redis/v1"
	security "github.com/alauda/redis-operator/pkg/security/password"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GenerateClusterACLConfigMapName(name string) string {
	return fmt.Sprintf("drc-acl-%s", name)
}

func GenerateClusterACLOperatorSecretName(name string) string {
	return fmt.Sprintf("drc-acl-%s-operator-secret", name)
}
func GenerateClusterOperatorsRedisUserName(name string) string {
	return fmt.Sprintf("drc-acl-%s-operator", name)
}

func GenerateClusterOperatorsRedisUser(rc types.RedisClusterInstance, passwordsecret string) redismiddlewarealaudaiov1.RedisUser {
	passwordsecrets := []string{}
	if passwordsecret != "" {
		passwordsecrets = append(passwordsecrets, passwordsecret)
	}
	rule := "~* +@all"
	if rc.Version().IsACL2Supported() {
		rule = "~* &* +@all"
	}
	return redismiddlewarealaudaiov1.RedisUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateClusterOperatorsRedisUserName(rc.GetName()),
			Namespace: rc.GetNamespace(),
		},
		Spec: redismiddlewarealaudaiov1.RedisUserSpec{
			AccountType:     redismiddlewarealaudaiov1.System,
			Arch:            redis.ClusterArch,
			RedisName:       rc.GetName(),
			Username:        "operator",
			PasswordSecrets: passwordsecrets,
			AclRules:        rule,
		},
	}
}

func GenerateClusterDefaultRedisUserName(name string) string {
	return fmt.Sprintf("drc-acl-%s-default", name)
}

func GenerateClusterDefaultRedisUser(drc *redisv1alpha1.DistributedRedisCluster, passwordsecret string) redismiddlewarealaudaiov1.RedisUser {
	passwordsecrets := []string{}
	if passwordsecret != "" {
		passwordsecrets = append(passwordsecrets, passwordsecret)
	}
	return redismiddlewarealaudaiov1.RedisUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateClusterDefaultRedisUserName(drc.Name),
			Namespace: drc.Namespace,
		},
		Spec: redismiddlewarealaudaiov1.RedisUserSpec{
			AccountType:     redismiddlewarealaudaiov1.Default,
			Arch:            redis.ClusterArch,
			RedisName:       drc.Name,
			Username:        "default",
			PasswordSecrets: passwordsecrets,
			AclRules:        "allkeys +@all -acl -flushall -flushdb -keys",
		},
	}
}

func NewClusterOpSecret(drc *redisv1alpha1.DistributedRedisCluster) *corev1.Secret {
	randPassword, _ := security.GeneratePassword(security.MaxPasswordLen)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateClusterACLOperatorSecretName(drc.Name),
			Namespace:       drc.Namespace,
			OwnerReferences: drc.GetOwnerReferences(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": []byte(randPassword),
			"username": []byte("operator"),
		},
	}
}
