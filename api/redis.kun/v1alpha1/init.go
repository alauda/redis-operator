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
	"fmt"
	"net/netip"
	"os"

	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/types/redis"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// Dumplicate with clusterbuilder
	PrometheusExporterPort          = 9100
	PrometheusExporterTelemetryPath = "/metrics"
)

func (ins *DistributedRedisCluster) Init() error {
	if ins.Spec.Config == nil {
		ins.Spec.Config = map[string]string{}
	}

	if ins.Spec.Image == "" {
		ins.Spec.Image = config.GetDefaultRedisImage()
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

	if os.Getenv("POD_IP") != "" {
		if operator_address, err := netip.ParseAddr(os.Getenv("POD_IP")); err == nil {
			if operator_address.Is6() && ins.Spec.IPFamilyPrefer == "" {
				ins.Spec.IPFamilyPrefer = v1.IPv6Protocol
			}
		}
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
		if mon.Image == "" {
			mon.Image = config.GetDefaultExporterImage()
		}

		if ins.Spec.Annotations == nil {
			ins.Spec.Annotations = make(map[string]string)
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
		ins.Spec.Annotations["prometheus.io/scrape"] = "true"
		ins.Spec.Annotations["prometheus.io/path"] = PrometheusExporterTelemetryPath
		ins.Spec.Annotations["prometheus.io/port"] = fmt.Sprintf("%d", PrometheusExporterPort)
	}
	return nil
}
