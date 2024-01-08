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
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	redisv1alpha1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RestoreSucceeded = "succeeded"

	RedisConfKey = "redis.conf"
)

// NewConfigMapForCR creates a new ConfigMap for the given Cluster
func NewConfigMapForCR(cluster types.RedisClusterInstance) (*corev1.ConfigMap, error) {
	redisConfContent, err := buildRedisConfigs(cluster)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            RedisConfigMapName(cluster.GetName()),
			Namespace:       cluster.GetNamespace(),
			Labels:          GetClusterLabels(cluster.GetName(), nil),
			OwnerReferences: util.BuildOwnerReferences(cluster.Definition()),
		},
		Data: map[string]string{
			RedisConfKey: redisConfContent,
			// 跳过pod异常升级重启失败的问题
			"shutdown.sh": "echo skip",
			"fix-ip.sh":   "echo skip",
		},
	}, nil
}

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

var ForbidToRenameCommands = map[string]struct{}{
	"config":  {},
	"cluster": {},
}

// buildRedisConfigs
//
// TODO: validate config and config value. check the empty value
func buildRedisConfigs(cluster types.RedisClusterInstance) (string, error) {
	cr := cluster.Definition()
	var buffer bytes.Buffer

	var (
		keys             = make([]string, 0, len(cr.Spec.Config))
		innerRedisConfig = cluster.Version().CustomConfigs(redis.ClusterArch)
		configMap        = util.MergeMap(cr.Spec.Config, innerRedisConfig)
	)

	// check memory-policy
	if cluster != nil && cr.Spec.Resources != nil && cr.Spec.Resources.Limits != nil {
		osMem, _ := cr.Spec.Resources.Limits.Memory().AsInt64()
		if configedMem := configMap[RedisConfig_MaxMemory]; configedMem == "" {
			var recommendMem int64
			if policy := cr.Spec.Config[RedisConfig_MaxMemoryPolicy]; policy == "noeviction" {
				recommendMem = int64(float64(osMem) * 0.8)
			} else {
				recommendMem = int64(float64(osMem) * 0.7)
			}
			configMap[RedisConfig_MaxMemory] = fmt.Sprintf("%d", recommendMem)
		}
		// TODO: validate user input
	}

	// check if it's needed to set default save
	// check if aof enabled
	if configMap[RedisConfig_Appendonly] != "yes" &&
		configMap[RedisConfig_ReplDisklessSync] != "yes" &&
		(configMap[RedisConfig_Save] == "" || configMap[RedisConfig_Save] == `""`) {

		configMap["save"] = "60 10000 300 100 600 1"
	}

	// for redis >=6.0, disable rename-command
	if !cluster.Version().IsACLSupported() {
		var (
			renameVal []string
		)
		renameConfig, err := ParseRenameConfigs(configMap[RedisConfig_RenameCommand])
		if err != nil {
			return "", err
		}
		for k, v := range renameConfig {
			if _, ok := ForbidToRenameCommands[k]; ok || k == v {
				continue
			}
			renameVal = append(renameVal, k, v)
		}
		if len(renameVal) > 0 {
			configMap[RedisConfig_RenameCommand] = strings.Join(renameVal, " ")
		}
	} else {
		delete(configMap, RedisConfig_RenameCommand)
	}

	for k, v := range configMap {
		if policy := RedisConfigRestartPolicy[k]; policy == Forbid {
			continue
		}

		lowerKey, trimVal := strings.ToLower(k), strings.TrimSpace(v)
		keys = append(keys, lowerKey)
		if lowerKey != k || !strings.EqualFold(trimVal, v) {
			configMap[lowerKey] = trimVal
		}
		// TODO: filter illgal config
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := configMap[k]

		if v == "" || v == `""` {
			buffer.WriteString(fmt.Sprintf("%s \"\"\n", k))
			continue
		}

		switch k {
		case RedisConfig_ClientOutputBufferLimit:
			fields := strings.Fields(v)
			if len(fields)%4 != 0 {
				return "", fmt.Errorf(`value "%s" for config %s is invalid`, v, k)
			}
			for i := 0; i < len(fields); i += 4 {
				buffer.WriteString(fmt.Sprintf("%s %s %s %s %s\n", k, fields[i], fields[i+1], fields[i+2], fields[i+3]))
			}
		case RedisConfig_Save, RedisConfig_RenameCommand:
			fields := strings.Fields(v)
			if len(fields)%2 != 0 {
				return "", fmt.Errorf(`value "%s" for config %s is invalid`, v, k)
			}
			for i := 0; i < len(fields); i += 2 {
				buffer.WriteString(fmt.Sprintf("%s %s %s\n", k, fields[i], fields[i+1]))
			}
		default:
			if _, ok := MustQuoteRedisConfig[k]; ok && !strings.HasPrefix(v, `"`) {
				v = fmt.Sprintf(`"%s"`, v)
			}
			if _, ok := MustUpperRedisConfig[k]; ok {
				v = strings.ToUpper(v)
			}
			buffer.WriteString(fmt.Sprintf("%s %s\n", k, v))
		}
	}
	return buffer.String(), nil
}

func RedisConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-%s", "redis-cluster", clusterName)
}

func NewConfigMapForRestore(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            RestoreConfigMapName(cluster.Name),
			Namespace:       cluster.Namespace,
			Labels:          labels,
			OwnerReferences: util.BuildOwnerReferences(cluster),
		},
		Data: map[string]string{
			RestoreSucceeded: strconv.Itoa(0),
		},
	}
}

func RestoreConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-%s", "rediscluster-restore", clusterName)
}

type RedisConfigSettingRule string

const (
	OK             RedisConfigSettingRule = "OK"
	RequireRestart RedisConfigSettingRule = "Restart"
	Forbid         RedisConfigSettingRule = "Forbid"
)

var RedisConfigRestartPolicy = map[string]RedisConfigSettingRule{
	// forbid
	"include":          Forbid,
	"loadmodule":       Forbid,
	"bind":             Forbid,
	"protected-mode":   Forbid,
	"port":             Forbid,
	"tls-port":         Forbid,
	"tls-cert-file":    Forbid,
	"tls-key-file":     Forbid,
	"tls-ca-cert-file": Forbid,
	"tls-ca-cert-dir":  Forbid,
	"unixsocket":       Forbid,
	"unixsocketperm":   Forbid,
	"daemonize":        Forbid,
	"supervised":       Forbid,
	"pidfile":          Forbid,
	"logfile":          Forbid,
	"syslog-enabled":   Forbid,
	"syslog-ident":     Forbid,
	"syslog-facility":  Forbid,
	"always-show-logo": Forbid,
	"dbfilename":       Forbid,
	"appendfilename":   Forbid,
	"dir":              Forbid,
	"slaveof":          Forbid,
	"replicaof":        Forbid,
	"gopher-enabled":   Forbid,
	// "ignore-warnings":       Forbid,
	"aclfile":               Forbid,
	"requirepass":           Forbid,
	"masterauth":            Forbid,
	"masteruser":            Forbid,
	"slave-announce-ip":     Forbid,
	"replica-announce-ip":   Forbid,
	"slave-announce-port":   Forbid,
	"replica-announce-port": Forbid,
	"cluster-enabled":       Forbid,
	"cluster-config-file":   Forbid,

	// RequireRestart
	"tcp-backlog":         RequireRestart,
	"databases":           RequireRestart,
	"rename-command":      RequireRestart,
	"rdbchecksum":         RequireRestart,
	"io-threads":          RequireRestart,
	"io-threads-do-reads": RequireRestart,
}

type RedisConfigValues []string

func (v *RedisConfigValues) String() string {
	if v == nil {
		return ""
	}
	sort.Strings(*v)
	return strings.Join(*v, " ")
}

// RedisConfig
type RedisConfig map[string]RedisConfigValues

// LoadRedisConfig
func LoadRedisConfig(data string) (RedisConfig, error) {
	conf := RedisConfig{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			continue
		}
		key := fields[0]
		// filter unsupported config
		if policy := RedisConfigRestartPolicy[key]; policy == Forbid {
			continue
		}
		val := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(fields[1]), `"`), `"`)
		conf[key] = append(conf[key], val)
	}
	return conf, nil
}

// Diff return diff two n RedisConfig
func (o RedisConfig) Diff(n RedisConfig) (added, changed, deleted map[string]RedisConfigValues) {
	if len(n) == 0 {
		return nil, nil, o
	}
	if len(o) == 0 {
		return n, nil, nil
	}

	if reflect.DeepEqual(o, n) {
		return nil, nil, nil
	}

	added, changed, deleted = map[string]RedisConfigValues{}, map[string]RedisConfigValues{}, map[string]RedisConfigValues{}
	for key, vals := range n {
		val := util.UnifyValueUnit(vals.String())
		if oldVals, ok := o[key]; ok {
			if util.UnifyValueUnit(oldVals.String()) != val {
				changed[key] = vals
			}
		} else {
			added[key] = vals
		}
	}
	for key, vals := range o {
		if _, ok := n[key]; !ok {
			deleted[key] = vals
		}
	}
	return
}

func ParseRenameConfigs(val string) (ret map[string]string, err error) {
	if val == "" {
		return
	}
	val = strings.ToLower(strings.TrimSpace(val))

	ret = map[string]string{}
	fields := strings.Fields(val)
	if len(fields)%2 == 0 {
		for i := 0; i < len(fields); i += 2 {
			if fields[i+1] == "" {
				fields[i+1] = `""`
			}
			ret[fields[i]] = fields[i+1]
		}
	} else {
		err = fmt.Errorf("invalid rename value %s", val)
	}
	return
}
