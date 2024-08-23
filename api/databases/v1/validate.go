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

	"github.com/alauda/redis-operator/api/core/helper"
)

const (
	maxNameLength      = 48
	defaultRedisNumber = 2
)

// Validate set the values by default if not defined and checks if the values given are valid
// TODO: remove this validation and use the kubebuilder validation
func (r *RedisFailover) Validate() error {
	if len(r.Name) > maxNameLength {
		return fmt.Errorf("name length can't be higher than %d", maxNameLength)
	}

	if r.Spec.Redis.Replicas <= 0 {
		r.Spec.Redis.Replicas = defaultRedisNumber
	}
	if r.Spec.Redis.Expose.IPFamilyPrefer == "" {
		r.Spec.Redis.Expose.IPFamilyPrefer = helper.GetDefaultIPFamily(os.Getenv("POD_IP"))
	}
	if r.Spec.Sentinel != nil {
		if r.Spec.Sentinel.SentinelReference == nil {
			if r.Spec.Sentinel.Replicas <= 0 {
				r.Spec.Sentinel.Replicas = DefaultSentinelNumber
			}
			if r.Spec.Sentinel.Expose.IPFamilyPrefer == "" {
				r.Spec.Sentinel.Expose.IPFamilyPrefer = r.Spec.Redis.Expose.IPFamilyPrefer
			}
		}
	}
	return nil
}
