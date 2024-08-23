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
	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/types"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewFailoverSentinel(inst types.RedisFailoverInstance) *v1.RedisSentinel {
	def := inst.Definition()

	sen := &v1.RedisSentinel{
		ObjectMeta: metav1.ObjectMeta{
			Name:            def.Name,
			Namespace:       def.Namespace,
			Labels:          def.Labels,
			Annotations:     def.Annotations,
			OwnerReferences: util.BuildOwnerReferences(def),
		},
		Spec: def.Spec.Sentinel.RedisSentinelSpec,
	}

	sen.Spec.Image = lo.FirstOrEmpty([]string{def.Spec.Sentinel.Image, def.Spec.Redis.Image})
	sen.Spec.ImagePullPolicy = builder.GetPullPolicy(def.Spec.Sentinel.ImagePullPolicy, def.Spec.Redis.ImagePullPolicy)
	sen.Spec.ImagePullSecrets = lo.FirstOrEmpty([][]corev1.LocalObjectReference{
		def.Spec.Sentinel.ImagePullSecrets,
		def.Spec.Redis.ImagePullSecrets,
	})
	sen.Spec.Replicas = def.Spec.Sentinel.Replicas
	sen.Spec.Resources = def.Spec.Sentinel.Resources
	sen.Spec.CustomConfig = def.Spec.Sentinel.CustomConfig
	sen.Spec.Exporter = def.Spec.Sentinel.Exporter
	if sen.Spec.EnableTLS {
		sen.Spec.ExternalTLSSecret = builder.GetRedisSSLSecretName(inst.GetName())
	}
	return sen
}
