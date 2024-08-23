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
	"sort"
	"strings"

	"github.com/alauda/redis-operator/api/core"
	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	constConfig = map[string]string{
		"dir": "/data",
	}
)

func NewSentinelConfigMap(sen *v1.RedisSentinel, selectors map[string]string) (*corev1.ConfigMap, error) {
	defaultConfig := make(map[string]string)
	defaultConfig["loglevel"] = "notice"
	defaultConfig["maxclients"] = "10000"
	defaultConfig["tcp-keepalive"] = "300"
	defaultConfig["tcp-backlog"] = "511"

	version, _ := redis.ParseRedisVersionFromImage(sen.Spec.Image)
	innerRedisConfig := version.CustomConfigs(core.RedisStdSentinel)
	defaultConfig = lo.Assign(defaultConfig, innerRedisConfig)

	for k, v := range sen.Spec.CustomConfig {
		defaultConfig[strings.ToLower(k)] = strings.TrimSpace(v)
	}
	for k, v := range constConfig {
		defaultConfig[strings.ToLower(k)] = strings.TrimSpace(v)
	}

	keys := make([]string, 0, len(defaultConfig))
	for k := range defaultConfig {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buffer bytes.Buffer
	for _, k := range keys {
		v := defaultConfig[k]
		if v == "" || v == `""` {
			buffer.WriteString(fmt.Sprintf("%s \"\"\n", k))
			continue
		}

		if _, ok := builder.MustQuoteRedisConfig[k]; ok && !strings.HasPrefix(v, `"`) {
			v = fmt.Sprintf(`"%s"`, v)
		}
		if _, ok := builder.MustUpperRedisConfig[k]; ok {
			v = strings.ToUpper(v)
		}
		buffer.WriteString(fmt.Sprintf("%s %s\n", k, v))
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GetSentinelConfigMapName(sen.GetName()),
			Namespace:       sen.Namespace,
			Labels:          GetCommonLabels(sen.Name, selectors),
			OwnerReferences: util.BuildOwnerReferences(sen),
		},
		Data: map[string]string{
			SentinelConfigFileName: buffer.String(),
		},
	}, nil
}
