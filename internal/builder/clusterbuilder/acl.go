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
	"strings"

	redisv1alpha1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	security "github.com/alauda/redis-operator/pkg/security/password"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/user"
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

func GenerateClusterRedisUserName(instName, name string) string {
	return fmt.Sprintf("drc-acl-%s-%s", instName, name)
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
			Arch:            core.RedisCluster,
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

func GenerateClusterRedisUser(obj metav1.Object, u *user.User) *redismiddlewarealaudaiov1.RedisUser {
	var (
		name            = GenerateClusterRedisUserName(obj.GetName(), u.Name)
		accountType     redismiddlewarealaudaiov1.AccountType
		passwordSecrets []string
	)
	switch u.Role {
	case user.RoleOperator:
		accountType = redismiddlewarealaudaiov1.System
	default:
		if u.Name == "default" {
			accountType = redismiddlewarealaudaiov1.Default
		} else {
			accountType = redismiddlewarealaudaiov1.Custom
		}
	}
	if u.GetPassword().GetSecretName() != "" {
		passwordSecrets = append(passwordSecrets, u.GetPassword().GetSecretName())
	}
	var rules []string
	for _, rule := range u.Rules {
		rules = append(rules, rule.Encode())
	}

	return &redismiddlewarealaudaiov1.RedisUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   obj.GetNamespace(),
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: redismiddlewarealaudaiov1.RedisUserSpec{
			AccountType:     accountType,
			Arch:            core.RedisCluster,
			RedisName:       obj.GetName(),
			Username:        u.Name,
			PasswordSecrets: passwordSecrets,
			AclRules:        strings.Join(rules, " "),
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
