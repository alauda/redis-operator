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

package v1

import (
	"fmt"
	"net/netip"
	"os"

	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/types/redis"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	maxNameLength         = 48
	defaultRedisNumber    = 2
	DefaultSentinelNumber = 3
)

var (
	defaultExporterImage = config.GetDefaultExporterImage()
	defaultImage         = config.GetDefaultRedisImage()
)

// Validate set the values by default if not defined and checks if the values given are valid
func (r *RedisFailover) Validate() error {
	if len(r.Name) > maxNameLength {
		return fmt.Errorf("name length can't be higher than %d", maxNameLength)
	}

	if r.Spec.Redis.Image == "" {
		r.Spec.Redis.Image = defaultImage
	}

	if r.Spec.Sentinel.Image == "" {
		r.Spec.Sentinel.Image = defaultImage
	}

	if r.Spec.Redis.Replicas <= 0 {
		r.Spec.Redis.Replicas = defaultRedisNumber
	}

	if r.Spec.Sentinel.Replicas <= 0 {
		r.Spec.Sentinel.Replicas = DefaultSentinelNumber
	}

	if r.Spec.Redis.Exporter.Image == "" {
		r.Spec.Redis.Exporter.Image = defaultExporterImage
	}

	// if r.Spec.Sentinel.Exporter.Image == "" {
	// 	r.Spec.Sentinel.Exporter.Image = defaultSentinelExporterImage
	// }

	v, err := redis.ParseRedisVersionFromImage(r.Spec.Redis.Image)
	if err != nil {
		return err
	}
	// tls only support redis >= 6.0
	if r.Spec.EnableTLS && !v.IsTLSSupported() {
		r.Spec.EnableTLS = false
	}

	if r.Spec.Redis.Resources.Size() == 0 {
		r.Spec.Redis.Resources = defaultRedisResource()
	}

	if r.Spec.Sentinel.Resources.Size() == 0 {
		r.Spec.Sentinel.Resources = defaultSentinelResource()
	}
	if os.Getenv("POD_IP") != "" {
		if operator_address, err := netip.ParseAddr(os.Getenv("POD_IP")); err == nil {
			if operator_address.Is6() && r.Spec.Redis.IPFamilyPrefer == "" {
				r.Spec.Redis.IPFamilyPrefer = v1.IPv6Protocol
			}
		}
	}
	return nil
}

func defaultRedisResource() v1.ResourceRequirements {
	return v1.ResourceRequirements{
		Requests: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("200m"),
			v1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("1000m"),
			v1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func defaultSentinelResource() v1.ResourceRequirements {
	return v1.ResourceRequirements{
		Requests: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("100m"),
			v1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("200m"),
			v1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
}
