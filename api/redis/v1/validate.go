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
	"os"
	"path"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Validate set the values by default if not defined and checks if the values given are valid
func (r *RedisClusterBackup) Validate() error {
	if r.Spec.Source.RedisClusterName == "" {
		return fmt.Errorf("RedisFailoverName is not valid")
	}
	if r.Spec.Storage.IsZero() {
		return fmt.Errorf("backup storage can't be empty")
	}
	if r.Spec.Target.S3Option.S3Secret != "" {
		r.Spec.Source.Endpoint = []IpPort{{
			Address: fmt.Sprintf("%s-0", r.Spec.Source.RedisClusterName),
			Port:    6379,
		}, {
			Address: fmt.Sprintf("%s-1", r.Spec.Source.RedisClusterName),
			Port:    6379,
		}, {
			Address: fmt.Sprintf("%s-2", r.Spec.Source.RedisClusterName),
			Port:    6379,
		}}
		if r.Spec.Target.S3Option.Dir == "" {
			currentTime := time.Now().UTC()
			timeString := currentTime.Format("2006-01-02T15:04:05Z")
			r.Spec.Target.S3Option.Dir = path.Join("data", "backup", "redis-cluster", "manual", timeString)
		}
		if r.Spec.Image == "" {
			r.Spec.Image = os.Getenv("REDIS_TOOLS_IMAGE")
		}
		if r.Spec.BackoffLimit == nil {
			r.Spec.BackoffLimit = new(int32)
			*r.Spec.BackoffLimit = 1
		}
	}
	if r.Spec.Resources == nil {
		r.Spec.Resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("500Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		}
	}
	return nil
}
