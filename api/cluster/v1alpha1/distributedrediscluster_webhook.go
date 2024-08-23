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

package v1alpha1

import (
	"os"

	"github.com/alauda/redis-operator/api/core/helper"
	"github.com/alauda/redis-operator/pkg/types/redis"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Init
func (ins *DistributedRedisCluster) Default() error {
	// added default forbid command
	if ins.Spec.Config == nil {
		ins.Spec.Config = map[string]string{}
	}
	ver, _ := redis.ParseRedisVersionFromImage(ins.Spec.Image)
	if ins.Spec.EnableTLS && !ver.IsTLSSupported() {
		ins.Spec.EnableTLS = false
	}

	if ins.Spec.MasterSize < 3 {
		ins.Spec.MasterSize = 3
	}

	if ins.Spec.ServiceName == "" {
		ins.Spec.ServiceName = ins.Name
	}

	if ins.Spec.Resources == nil || ins.Spec.Resources.Size() == 0 {
		ins.Spec.Resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		}
	}

	if ins.Spec.IPFamilyPrefer == "" {
		ins.Spec.IPFamilyPrefer = helper.GetDefaultIPFamily(os.Getenv("POD_IP"))
	}

	if ins.Spec.Storage != nil && ins.Spec.Storage.Type == "" {
		ins.Spec.Storage.Type = PersistentClaim
	}

	// check if it's need to set default save
	// check if aof enabled
	if ins.Spec.Config["appendonly"] != "yes" &&
		ins.Spec.Config["repl-diskless-sync"] != "yes" &&
		(ins.Spec.Config["save"] == "" || ins.Spec.Config["save"] == `""`) {

		ins.Spec.Config["save"] = "60 10000 300 100 600 1"
	}

	if ins.Spec.Config["appendonly"] == "yes" {
		if ins.Spec.Config["appendfsync"] == "" {
			ins.Spec.Config["appendfsync"] = "everysec"
		}
	}

	mon := ins.Spec.Monitor
	if mon != nil {
		if mon.Prometheus == nil {
			mon.Prometheus = &PrometheusSpec{}
		}
		if ins.Spec.PodAnnotations == nil {
			ins.Spec.PodAnnotations = make(map[string]string)
		}

		if ins.Spec.Monitor.Resources == nil {
			ins.Spec.Monitor.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("300Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("300Mi"),
				},
			}
		}
	}
	return nil
}
