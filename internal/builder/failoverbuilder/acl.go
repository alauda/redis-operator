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

package failoverbuilder

import (
	"fmt"
	"strings"

	"github.com/alauda/redis-operator/api/core"
	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
	midv1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	"github.com/alauda/redis-operator/internal/util"
	security "github.com/alauda/redis-operator/pkg/security/password"
	"github.com/alauda/redis-operator/pkg/types/user"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GenerateFailoverACLConfigMapName(name string) string {
	return fmt.Sprintf("rfr-acl-%s", name)
}

// acl operator secret
func GenerateFailoverACLOperatorSecretName(name string) string {
	return fmt.Sprintf("rfr-acl-%s-operator-secret", name)
}

func NewFailoverOpSecret(rf *databasesv1.RedisFailover) *corev1.Secret {
	randPassword, _ := security.GeneratePassword(security.MaxPasswordLen)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateFailoverACLOperatorSecretName(rf.Name),
			Namespace:       rf.Namespace,
			OwnerReferences: util.BuildOwnerReferences(rf),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": []byte(randPassword),
			"username": []byte("operator"),
		},
	}
}

func NewFailoverAclConfigMap(rf *databasesv1.RedisFailover, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateFailoverACLConfigMapName(rf.Name),
			Namespace:       rf.Namespace,
			OwnerReferences: util.BuildOwnerReferences(rf),
		},
		Data: data,
	}
}

func GenerateFailoverOperatorsRedisUserName(name string) string {
	return fmt.Sprintf("rfr-acl-%s-operator", name)
}

func GenerateFailoverDefaultRedisUserName(name string) string {
	return fmt.Sprintf("rfr-acl-%s-default", name)
}

func GenerateFailoverRedisUserName(instName, name string) string {
	return fmt.Sprintf("rfr-acl-%s-%s", instName, name)
}

func GenerateFailoverRedisUser(obj metav1.Object, u *user.User) *midv1.RedisUser {
	var (
		name            = GenerateFailoverRedisUserName(obj.GetName(), u.Name)
		accountType     midv1.AccountType
		passwordSecrets []string
	)
	switch u.Role {
	case user.RoleOperator:
		accountType = midv1.System
	default:
		if u.Name == "default" {
			accountType = midv1.Default
		} else {
			accountType = midv1.Custom
		}
	}
	if u.GetPassword().GetSecretName() != "" {
		passwordSecrets = append(passwordSecrets, u.GetPassword().GetSecretName())
	}
	var rules []string
	for _, rule := range u.Rules {
		rules = append(rules, rule.Encode())
	}

	return &midv1.RedisUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   obj.GetNamespace(),
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: midv1.RedisUserSpec{
			AccountType:     accountType,
			Arch:            core.RedisSentinel,
			RedisName:       obj.GetName(),
			Username:        u.Name,
			PasswordSecrets: passwordSecrets,
			AclRules:        strings.Join(rules, " "),
		},
	}
}
