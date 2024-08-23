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

import "github.com/alauda/redis-operator/internal/builder"

const (
	RedisFailoverType = "redis-failover"
	RedisClusterType  = "distributed-redis-cluster"
)

func GetRedisSentinelLabels(instanceName, failoverName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":    failoverName,
		"app.kubernetes.io/part-of": RedisFailoverType,
		builder.InstanceTypeLabel:   RedisFailoverType,
		builder.InstanceNameLabel:   instanceName,
	}
}

func GetRedisClusterLabels(instanceName, clusterName string) map[string]string {
	return map[string]string{
		"managed-by":              "redis-cluster-operator",
		"redis.kun/name":          clusterName,
		builder.InstanceTypeLabel: RedisClusterType,
		builder.InstanceNameLabel: instanceName,
	}
}

func GetRedisFailoverName(instanceName string) string {
	return instanceName
}
func GetRedisClusterName(instanceName string) string {
	return instanceName
}

func GetRedisStorageVolumeName(instanceName string) string {
	return "redis-data"
}
