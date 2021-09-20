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

package util

import (
	"fmt"
	"strings"

	v1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
)

const (
	BaseName                      = "rf"
	SentinelName                  = "s"
	SentinelRoleName              = "sentinel"
	SentinelConfigFileName        = "sentinel.conf"
	RedisConfigFileName           = "redis.conf"
	RedisName                     = "r"
	RedisShutdownName             = "r-s"
	RedisReadinessName            = "r-readiness"
	RedisRoleName                 = "redis"
	RedisMasterName               = "mymaster"
	AppLabel                      = "redis-failover"
	HostnameTopologyKey           = "kubernetes.io/hostname"
	RedisBackupServiceAccountName = "redis-backup"
	RedisBackupRoleName           = "redis-backup"
	RedisBackupRoleBindingName    = "redis-backup"
)

const (
	RedisConfigFileNameBackup = "redis.conf.bk"
	RedisInitScript           = "init.sh"
	SentinelEntrypoint        = "entrypoint.sh"
	RedisBackupVolumeName     = "backup-data"
	S3SecretVolumeName        = "s3-secret"
)

// variables refering to the redis exporter port
const (
	ExporterPort                  = 9121
	SentinelExporterPort          = 9355
	SentinelPort                  = "26379"
	ExporterPortName              = "http-metrics"
	RedisPort                     = 6379
	RedisPortString               = "6379"
	RedisPortName                 = "redis"
	ExporterContainerName         = "redis-exporter"
	SentinelExporterContainerName = "sentinel-exporter"
	RestoreContainerName          = "restore"
	ExporterDefaultRequestCPU     = "25m"
	ExporterDefaultLimitCPU       = "50m"
	ExporterDefaultRequestMemory  = "50Mi"
	ExporterDefaultLimitMemory    = "100Mi"
)

// label
const (
	LabelInstanceName     = "app.kubernetes.io/name"
	LabelPartOf           = "app.kubernetes.io/part-of"
	LabelRedisConfig      = "redis.middleware.alauda.io/config"
	LabelRedisConfigValue = "true"
	LabelRedisRole        = "redis.middleware.alauda.io/role"
)

// Redis arch
const (
	LabelRedisArch = "redisarch"
)

// Redis role
const (
	Master = "master"
	Slave  = "slave"
)

func GenerateName(typeName, metaName string) string {
	return fmt.Sprintf("%s%s-%s", BaseName, typeName, metaName)
}

func GetRedisName(rf *v1.RedisFailover) string {
	return GenerateName(RedisName, rf.Name)
}

func GetSentinelName(rf *v1.RedisFailover) string {
	return GenerateName(SentinelName, rf.Name)
}

func GetRedisShutdownName(rf *v1.RedisFailover) string {
	return GenerateName(RedisShutdownName, rf.Name)
}

func GetRedisNameExporter(rf *v1.RedisFailover) string {
	return GenerateName(fmt.Sprintf("%s%s", RedisName, "e"), rf.Name)
}

func GetSentinelHeadlessSvc(rf *v1.RedisFailover) string {
	return GenerateName(SentinelName, fmt.Sprintf("%s-%s", rf.Name, "hl"))
}

func GetRedisNodePortSvc(rf *v1.RedisFailover) string {
	return GenerateName(fmt.Sprintf("%s-%s", RedisName, "n"), rf.Name)
}

func GetRedisSecretName(rf *v1.RedisFailover) string {
	return GenerateName(fmt.Sprintf("%s-%s", RedisName, "p"), rf.Name)
}

func GetSentinelReadinessConfigmap(rf *v1.RedisFailover) string {
	return GenerateName(fmt.Sprintf("%s-%s", SentinelName, "r"), rf.Name)
}

func GetRedisShutdownConfigMapName(rf *v1.RedisFailover) string {
	return GenerateName(fmt.Sprintf("%s-%s", RedisName, "s"), rf.Name)
}

// GetRedisSSLSecretName return the name of redis ssl secret
func GetRedisSSLSecretName(rfName string) string {
	return rfName + "-tls"
}

func GetRedisRWServiceName(rfName string) string {
	return GenerateName(RedisName, rfName) + "-read-write"
}

func GetRedisROServiceName(rfName string) string {
	return GenerateName(RedisName, rfName) + "-read-only"
}

// split storage name, example: pvc/redisfailover-persistent-keep-data-rfr-redis-sentinel-demo-0
func GetClaimName(backupDestination string) string {
	names := strings.Split(backupDestination, "/")
	if len(names) != 2 {
		return ""
	}
	return names[1]
}

func GetCronJobName(redisName, scheduleName string) string {
	return fmt.Sprintf("%s-%s", redisName, scheduleName)
}
