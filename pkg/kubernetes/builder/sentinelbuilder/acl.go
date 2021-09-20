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

package sentinelbuilder

import (
	"fmt"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/redis/v1"
	security "github.com/alauda/redis-operator/pkg/security/password"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GenerateSentinelACLConfigMapName(name string) string {
	return fmt.Sprintf("rfr-acl-%s", name)
}

// acl operator secret
func GenerateSentinelACLOperatorSecretName(name string) string {
	return fmt.Sprintf("rfr-acl-%s-operator-secret", name)
}

func NewSentinelOpSecret(rf *databasesv1.RedisFailover) *corev1.Secret {
	randPassword, _ := security.GeneratePassword(security.MaxPasswordLen)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateSentinelACLOperatorSecretName(rf.Name),
			Namespace:       rf.Namespace,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": []byte(randPassword),
			"username": []byte("operator"),
		},
	}
}

func NewSentinelAclConfigMap(rf *databasesv1.RedisFailover, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateSentinelACLConfigMapName(rf.Name),
			Namespace:       rf.Namespace,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Data: data,
	}
}

func GenerateSentinelOperatorsRedisUserName(name string) string {
	return fmt.Sprintf("rfr-acl-%s-operator", name)
}

func GenerateSentinelOperatorsRedisUser(st types.RedisFailoverInstance, passwordsecret string) redismiddlewarealaudaiov1.RedisUser {
	passwordsecrets := []string{}
	if passwordsecret != "" {
		passwordsecrets = append(passwordsecrets, passwordsecret)
	}
	rule := "~* +@all"
	if st.Version().IsACL2Supported() {
		rule = "~* &* +@all"
	}
	return redismiddlewarealaudaiov1.RedisUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateSentinelOperatorsRedisUserName(st.GetName()),
			Namespace: st.GetNamespace(),
		},
		Spec: redismiddlewarealaudaiov1.RedisUserSpec{
			AccountType:     redismiddlewarealaudaiov1.System,
			Arch:            redis.SentinelArch,
			RedisName:       st.GetName(),
			Username:        "operator",
			PasswordSecrets: passwordsecrets,
			AclRules:        rule,
		},
	}
}

func GenerateSentinelDefaultRedisUserName(name string) string {
	return fmt.Sprintf("rfr-acl-%s-default", name)
}

func GenerateSentinelDefaultRedisUser(rf *databasesv1.RedisFailover, passwordsecret string) redismiddlewarealaudaiov1.RedisUser {
	passwordsecrets := []string{}
	if passwordsecret != "" {
		passwordsecrets = append(passwordsecrets, passwordsecret)
	}
	return redismiddlewarealaudaiov1.RedisUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateSentinelDefaultRedisUserName(rf.Name),
			Namespace: rf.Namespace,
		},
		Spec: redismiddlewarealaudaiov1.RedisUserSpec{
			AccountType:     redismiddlewarealaudaiov1.Default,
			Arch:            redis.SentinelArch,
			RedisName:       rf.Name,
			Username:        "default",
			PasswordSecrets: passwordsecrets,
			AclRules:        "allkeys +@all -acl -flushall -flushdb -keys",
		},
	}
}
