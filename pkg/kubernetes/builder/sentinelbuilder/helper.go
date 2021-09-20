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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelRedisArch       = "redisarch"
	RestoreContainerName = "restore"
)

func MergeMap(extraMap ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, item := range extraMap {
		for k, v := range item {
			result[k] = v
		}
	}
	return result
}

func GetCommonLabels(name string, extra ...map[string]string) map[string]string {
	labels := getPublicLabels(name)
	for _, item := range extra {
		for k, v := range item {
			labels[k] = v
		}
	}
	return labels
}

func getPublicLabels(name string) map[string]string {
	return map[string]string{
		"redisfailovers.databases.spotahome.com/name": name,
		"app.kubernetes.io/managed-by":                "redis-operator",
		"middleware.instance/name":                    name,
		"middleware.instance/type":                    "redis-failover",
	}
}

func GenerateSelectorLabels(component, name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of":   "redis-failover",
		"app.kubernetes.io/component": component,
		"app.kubernetes.io/name":      name,
	}
}

func GetSentinelStatefulSetName(sentinelName string) string {
	return fmt.Sprintf("rfr-%s", sentinelName)
}

func GetSentinelDeploymentName(sentinelName string) string {
	return fmt.Sprintf("rfs-%s", sentinelName)
}

func GetSentinelServiceName(sentinelName string) string {
	return fmt.Sprintf("rfs-%s", sentinelName)
}

func GetSentinelReadinessConfigMapName(sentinelName string) string {
	return fmt.Sprintf("rfs-r-%s", sentinelName)
}

func GenerateRedisTLSOptions() string {
	return "--tls --cert /tls/tls.crt --key /tls/tls.key --cacert /tls/ca.crt"
}

func GetOwnerReferenceForRedisFailover(rf *databasesv1.RedisFailover) []metav1.OwnerReference {
	rcvk := databasesv1.GroupVersion.WithKind("RedisFailover")
	owref := []metav1.OwnerReference{
		*metav1.NewControllerRef(rf, rcvk),
	}
	*owref[0].BlockOwnerDeletion = false
	return owref
}
