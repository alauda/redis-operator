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

package validation

import (
	"fmt"
	"slices"

	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/api/core/helper"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ValidateInstanceAccess validates the instance access.
func ValidateInstanceAccess(acc *core.InstanceAccessBase, nodeCount int, warns *admission.Warnings) error {
	if acc == nil || acc.ServiceType == "" {
		return nil
	}

	if !slices.Contains([]corev1.ServiceType{
		corev1.ServiceTypeClusterIP,
		corev1.ServiceTypeNodePort,
		corev1.ServiceTypeLoadBalancer,
	}, acc.ServiceType) {
		return fmt.Errorf("unsupported service type: %s", acc.ServiceType)
	}

	if acc.ServiceType == corev1.ServiceTypeNodePort {
		if acc.NodePortSequence != "" {
			ports, err := helper.ParseSequencePorts(acc.NodePortSequence)
			if err != nil {
				return fmt.Errorf("failed to parse data storage node port sequence: %v", err)
			}
			if nodeCount != len(ports) {
				return fmt.Errorf("expected %d nodes, but got %d ports in node port sequence", nodeCount, len(ports))
			}
		}
	}
	return nil
}
