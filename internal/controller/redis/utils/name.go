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

package utils

import (
	"fmt"
	"time"
)

const (
	RedisBaseName = "redis"
)

func GenerateRedisSecretName(instanceName string) string {
	return fmt.Sprintf("%s-%s-password-%d", RedisBaseName, instanceName, time.Now().Unix())
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
