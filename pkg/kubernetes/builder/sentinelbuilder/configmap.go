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
	"bytes"
	"fmt"
	"path"
	"sort"
	"strings"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RedisConfig_MaxMemory               = "maxmemory"
	RedisConfig_MaxMemoryPolicy         = "maxmemory-policy"
	RedisConfig_ClientOutputBufferLimit = "client-output-buffer-limit"
	RedisConfig_Save                    = "save"
	RedisConfig_RenameCommand           = "rename-command"
	RedisConfig_Appendonly              = "appendonly"
	RedisConfig_ReplDisklessSync        = "repl-diskless-sync"
)

var MustQuoteRedisConfig = map[string]struct{}{
	"tls-protocols": {},
}

var MustUpperRedisConfig = map[string]struct{}{
	"tls-ciphers":      {},
	"tls-ciphersuites": {},
	"tls-protocols":    {},
}

func NewSentinelConfigMap(rf *databasesv1.RedisFailover, selectors map[string]string) *corev1.ConfigMap {
	name := GetSentinelDeploymentName(rf.Name)
	namespace := rf.Namespace
	labels := MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleSEN, rf.Name), selectors)
	sentinelConfigContent := `sentinel monitor mymaster 127.0.0.1 6379 2
sentinel down-after-milliseconds mymaster 1000
sentinel failover-timeout mymaster 3000
sentinel parallel-syncs mymaster 2`

	version, _ := redis.ParseRedisVersionFromImage(rf.Spec.Sentinel.Image)
	innerConfigs := version.CustomConfigs(redis.SentinelArch)
	for k, v := range innerConfigs {
		sentinelConfigContent += fmt.Sprintf("\n%s %s", k, v)
	}

	sentinelEntrypointTemplate := `#!/bin/sh

redis-server %s --sentinel $* | sed 's/auth-pass .*/auth-pass ******/'`

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Data: map[string]string{
			util.SentinelConfigFileName: sentinelConfigContent,
			util.SentinelEntrypoint: fmt.Sprintf(
				sentinelEntrypointTemplate,
				path.Join("/redis", util.SentinelConfigFileName),
			),
		},
	}
}

func NewSentinelProbeConfigMap(rf *databasesv1.RedisFailover, selectors map[string]string) *corev1.ConfigMap {
	name := GetSentinelReadinessConfigMapName(rf.Name)
	namespace := rf.Namespace
	tlsOptions := ""
	if rf.Spec.EnableTLS {
		tlsOptions = GenerateRedisTLSOptions()
	}
	labels := MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleSEN, rf.Name), selectors)
	checkContent := `#!/usr/bin/env sh
set -eou pipefail
redis-cli  -p 26379 %s -h $(hostname)  ping
slaves=$(redis-cli  -p 26379 %s  info sentinel|grep master0| grep -Eo 'slaves=[0-9]+' | awk -F= '{print $2}')
status=$(redis-cli  -p 26379 %s  info sentinel|grep master0| grep -Eo 'status=\w+' | awk -F= '{print $2}')
if [ "$status" != "ok" ]; then 
    exit 1
fi
if [ $slaves -lt 0 ]; then
	exit 1
fi`

	checkContent = fmt.Sprintf(checkContent, tlsOptions, tlsOptions, tlsOptions)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Data: map[string]string{
			"readiness.sh": checkContent,
		},
	}
}

func NewRedisScriptConfigMap(rf *databasesv1.RedisFailover, selectors map[string]string) *corev1.ConfigMap {
	name := GetRedisScriptConfigMapName(rf.Name)
	namespace := rf.Namespace
	labels := MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleRedis, rf.Name), selectors)
	envSentinelHost := GetEnvSentinelHost(rf.Name)
	envSentinelPort := GetEnvSentinelPort(rf.Name)
	tlsOptions := ""
	if rf.Spec.EnableTLS {
		tlsOptions = GenerateRedisTLSOptions()
	}
	ipOptions := ""
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		ipOptions = "-h ::1"
	}
	shutdownContent := fmt.Sprintf(`#!/bin/sh
	ANNOUNCE_CONFIG="/data/announce.conf"
	master=""
	response_code=""
	echo "Asking sentinel who is master..."
	master=$(redis-cli -h ${%s} -p ${%s}  %s  --csv SENTINEL get-master-addr-by-name mymaster | tr ',' ' ' | tr -d '\"' |cut -d' ' -f1)
	echo "Master is $master,Saving rdb"
	REDIS_PASSWORD=$(cat /account/password)
	REDIS_USERNAME=$(cat /account/username)
	if [[ $REDIS_PASSWORD  ]]; then 
		if [[ $REDIS_USERNAME ]]; then
			redis-cli  -a  "$REDIS_PASSWORD" --user $REDIS_USERNAME %s %s SAVE
		else
		    redis-cli  -a  "$REDIS_PASSWORD"  %s %s SAVE
		fi
	else
		redis-cli %s %s SAVE
	fi
	announce_ip="NULL"
	if [ -f ${ANNOUNCE_CONFIG} ]; then
		announce_ip=$(cat ${ANNOUNCE_CONFIG} |grep announce-ip|awk '{print $2}')
	fi
	if [ $master = $(hostname -i) ] || [ $master = $announce_ip ]; then
		while [ ! "$response_code" = "OK" ]; do
			  response_code=$(redis-cli -h ${%s} -p ${%s} %s SENTINEL failover mymaster)
			echo "after failover with code $response_code"
			sleep 1
		done
	fi`, envSentinelHost, envSentinelPort, tlsOptions, ipOptions, tlsOptions, ipOptions, tlsOptions, ipOptions, tlsOptions, envSentinelHost, envSentinelPort, tlsOptions)
	startContent := fmt.Sprintf(`#!/bin/sh
	REDIS_PASSWORD=$(cat /account/password)
	REDIS_USERNAME=$(cat /account/username)
	if [[ $REDIS_PASSWORD  ]]; then 
		if [[ $REDIS_USERNAME ]]; then
			output=$(redis-cli  -a  "$REDIS_PASSWORD" --user $REDIS_USERNAME  %s %s INFO|grep '^loading\|master_sync_in_progress\|^role:' || echo "")
		else 
			output=$(redis-cli  -a  "$REDIS_PASSWORD" %s %s INFO|grep '^loading\|master_sync_in_progress\|^role:' || echo "")
		fi
	else
		output=$(redis-cli %s %s INFO |grep '^loading\|master_sync_in_progress\|^role:' || echo "")
	fi
	
	if [[ "$output" == *"loading:0"*  ]]; then
		if [[ ! "$output" == *"role:master"* ]]; then
			echo "Redis is not master,check master_sync_in_progress"
			if [[ "$output" == *"master_sync_in_progress:0"* ]]; then 
				echo "Redis loading is 0 and master_sync_in_progress is 0. Script exiting normally."
				exit 0
			else
				echo "master_sync_in_progress is not 0. Script exiting with an error."
				exit 1
			fi
		fi
	
		echo "master exit 0"
		exit 0 
	else
		echo "Redis loading is not 0 . Script exiting with an error."
		exit 1
	fi`, ipOptions, tlsOptions, ipOptions, tlsOptions, ipOptions, tlsOptions)

	autoSlaveContent := fmt.Sprintf(`#!/bin/sh
echo "check sentinel status"
status=$(redis-cli -h ${%s} -p ${%s} %s  info sentinel|grep master0| grep -Eo 'status=\w+' | awk -F= '{print $2}' || echo "")
if [ "$status" != "ok" ]; then 
	exit 0
fi

echo "get master"
master=$(redis-cli -h ${%s} -p ${%s}  %s  --csv SENTINEL get-master-addr-by-name mymaster | tr ',' ' ' | tr -d '\"' |cut -d' ' -f1 || echo "")
masterPort=$(redis-cli -h ${%s} -p ${%s}  %s  --csv SENTINEL get-master-addr-by-name mymaster | tr ',' ' ' | tr -d '\"' |cut -d' ' -f2 || echo "")

echo "master: $master"
echo "masterPort: $masterPort"

if [ "$master" = "" ] || [ "$masterPort" = "" ]; then
    exit 0
fi

if [ "$master" != "127.0.0.1" ] || [ "$master" = "$(hostname -i)" ] || [ "$master" = "$announce_ip" ]; then
	echo "slaveof $master $masterPort" > /data/slaveof.conf
fi`, envSentinelHost, envSentinelPort, tlsOptions, envSentinelHost, envSentinelPort, tlsOptions, envSentinelHost, envSentinelPort, tlsOptions)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Data: map[string]string{
			"shutdown.sh":     shutdownContent,
			"start.sh":        startContent,
			"auto_replica.sh": autoSlaveContent,
		},
	}
}

func GetEnvSentinelHost(name string) string {
	return "RFS_REDIS_SERVICE_HOST"
}

func GetEnvSentinelPort(name string) string {
	return "RFS_REDIS_SERVICE_PORT_SENTINEL"
}

func NewRedisConfigMap(st types.RedisFailoverInstance, selectors map[string]string) *corev1.ConfigMap {
	rf := st.Definition()
	customConfig := rf.Spec.Redis.CustomConfig

	default_config := make(map[string]string)
	default_config["loglevel"] = "notice"
	default_config["stop-writes-on-bgsave-error"] = "yes"
	default_config["rdbcompression"] = "yes"
	default_config["rdbchecksum"] = "yes"
	default_config["slave-read-only"] = "yes"
	default_config["repl-diskless-sync"] = "no"
	default_config["slowlog-max-len"] = "128"
	default_config["slowlog-log-slower-than"] = "10000"
	default_config["maxclients"] = "10000"
	default_config["hz"] = "50"
	default_config["timeout"] = "60"
	default_config["tcp-keepalive"] = "300"
	default_config["tcp-backlog"] = "511"
	default_config["protected-mode"] = "no"

	version, _ := redis.ParseRedisVersionFromImage(rf.Spec.Redis.Image)
	innerRedisConfig := version.CustomConfigs(redis.SentinelArch)
	default_config = util.MergeMap(default_config, innerRedisConfig)

	for k, v := range customConfig {
		k = strings.ToLower(k)
		v = strings.TrimSpace(v)
		if k == "save" && v == "60 100" {
			continue
		}
		default_config[k] = v
	}

	// check if it's need to set default save
	// check if aof enabled
	if customConfig[RedisConfig_Appendonly] != "yes" &&
		customConfig[RedisConfig_ReplDisklessSync] != "yes" &&
		(customConfig[RedisConfig_Save] == "" || customConfig[RedisConfig_Save] == `""`) {

		default_config["save"] = "60 10000 300 100 600 1"
	}

	if limits := rf.Spec.Redis.Resources.Limits; limits != nil {
		if configedMem := customConfig[RedisConfig_MaxMemory]; configedMem == "" {
			memLimit, _ := limits.Memory().AsInt64()
			if policy := customConfig[RedisConfig_MaxMemoryPolicy]; policy == "noeviction" {
				memLimit = int64(float64(memLimit) * 0.8)
			} else {
				memLimit = int64(float64(memLimit) * 0.7)
			}
			if memLimit > 0 {
				default_config[RedisConfig_MaxMemory] = fmt.Sprintf("%d", memLimit)
			}
		}
	}

	renameConfig := map[string]string{}
	if renameVal := customConfig[RedisConfig_RenameCommand]; renameVal != "" {
		fields := strings.Fields(strings.ToLower(renameVal))
		if len(fields)%2 == 0 {
			for i := 0; i < len(fields); i += 2 {
				renameConfig[fields[i]] = fields[i+1]
			}
		}
	}
	var renameVal []string
	for k, v := range renameConfig {
		if k == v || k == "config" {
			continue
		}
		if v == "" {
			v = `""`
		}
		renameVal = append(renameVal, k, v)
	}
	if len(renameVal) > 0 {
		default_config[RedisConfig_RenameCommand] = strings.Join(renameVal, " ")
	}

	if st.Version().IsACLSupported() {
		delete(default_config, RedisConfig_RenameCommand)
	}

	keys := make([]string, 0, len(default_config))
	for k := range default_config {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buffer bytes.Buffer
	for _, k := range keys {
		v := default_config[k]
		if v == "" || v == `""` {
			buffer.WriteString(fmt.Sprintf("%s \"\"\n", k))
			continue
		}
		switch k {
		case RedisConfig_ClientOutputBufferLimit:
			fields := strings.Fields(v)
			if len(fields)%4 != 0 {
				continue
			}
			for i := 0; i < len(fields); i += 4 {
				buffer.WriteString(fmt.Sprintf("%s %s %s %s %s\n", k, fields[i], fields[i+1], fields[i+2], fields[i+3]))
			}
		case RedisConfig_Save, RedisConfig_RenameCommand:
			fields := strings.Fields(v)
			if len(fields)%2 != 0 {
				continue
			}
			for i := 0; i < len(fields); i += 2 {
				buffer.WriteString(fmt.Sprintf("%s %s %s\n", k, fields[i], fields[i+1]))
			}
		default:
			if _, ok := MustQuoteRedisConfig[k]; ok && !strings.HasPrefix(v, `"`) {
				v = fmt.Sprintf(`"%s"`, v)
			}
			if _, ok := MustUpperRedisConfig[k]; ok {
				v = fmt.Sprintf(`%s`, strings.ToUpper(v))
			}
			buffer.WriteString(fmt.Sprintf("%s %s\n", k, v))
		}
	}

	entrypoint := fmt.Sprintf(`#!/bin/sh
CONFIG_FILE="/tmp/redis.conf"
cat /redis/redis.conf > $CONFIG_FILE
REDIS_PASSWORD=$(cat /account/password)
REDIS_USERNAME=$(cat /account/username)
ACL_ARGS=""
ACL_CONFIG="/tmp/acl.conf"

if [ ! -z "${REDIS_PASSWORD}" ]; then
    echo "requirepass \"${REDIS_PASSWORD}\"" >> $CONFIG_FILE
    echo "masterauth \"${REDIS_PASSWORD}\"" >> $CONFIG_FILE
fi

if [ -e /tmp/newpass ]; then
	echo "new passwd found"
	REDIS_PASSWORD=$(cat /tmp/newpass)
fi

if [ ! -z "${REDIS_USERNAME}" ]; then
	echo "masteruser \"${REDIS_USERNAME}\"" >> $CONFIG_FILE
fi

if [[ ! -z "${ACL_CONFIGMAP_NAME}" ]]; then
    echo "# Run: generate acl "
	echo "${ACL_CONFIGMAP_NAME} ${POD_NAMESPACE}"
    /opt/redis-tools helper generate acl --name ${ACL_CONFIGMAP_NAME} --namespace ${POD_NAMESPACE} >> ${ACL_CONFIG} || exit 1
    ACL_ARGS="--aclfile ${ACL_CONFIG}"
fi

ANNOUNCE_CONFIG="/data/announce.conf"	
if [ -f ${ANNOUNCE_CONFIG} ]; then
	echo "append announce conf to redis config"
	echo "" >> $CONFIG_FILE
	cat ${ANNOUNCE_CONFIG} >> $CONFIG_FILE
fi


SLAVEOF_CONFIG="/data/slaveof.conf"
if [ -f ${SLAVEOF_CONFIG} ]; then
	echo "append slaveof conf to redis config"
	echo "" >> $CONFIG_FILE
	cat ${SLAVEOF_CONFIG} >> $CONFIG_FILE
else 
	echo "slaveof 127.0.0.1 6379" >> $CONFIG_FILE
fi

POD_IPS_LIST=$(echo "${POD_IPS}"|sed 's/,/ /g')
ARGS=""
if [ ! -z "${POD_IPS}" ]; then
	if [[ ${IP_FAMILY_PREFER} == "IPv6" ]]; then
		ARGS="${ARGS} --bind ${POD_IPS_LIST}  ::1"
	else
		ARGS="${ARGS} --bind ${POD_IPS_LIST} 127.0.0.1"
	fi
fi
chmod 0640 $CONFIG_FILE
chmod 0640 $ACL_CONFIG

redis-server $CONFIG_FILE ${ACL_ARGS} ${ARGS} $@`)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GetRedisConfigMapName(rf),
			Namespace:       rf.Namespace,
			Labels:          GetCommonLabels(rf.Name, selectors),
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Data: map[string]string{
			util.RedisConfigFileName: buffer.String(),
			util.SentinelEntrypoint:  entrypoint,
		},
	}
}

func GetRedisConfigMapName(rf *databasesv1.RedisFailover) string {
	version := config.GetRedisVersion(rf.Spec.Redis.Image)
	name := GetSentinelStatefulSetName(rf.Name)
	if version != "" {
		name = name + "-" + version
	}
	return name
}

func GetRedisScriptConfigMapName(name string) string {
	return fmt.Sprintf("rfr-s-%s", name)
}
