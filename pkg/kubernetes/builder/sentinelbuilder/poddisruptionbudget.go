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
	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NewDeployPodDisruptionBudgetForCR(rf *databasesv1.RedisFailover, selectors map[string]string) *policyv1.PodDisruptionBudget {
	maxUnavailable := intstr.FromInt(2)
	namespace := rf.Namespace
	selectors = MergeMap(selectors, GenerateSelectorLabels(RedisArchRoleSEN, rf.Name))
	labels := MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleSEN, rf.Name), selectors)

	name := GetSentinelDeploymentName(rf.Name)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectors,
			},
		},
	}
}

func NewPodDisruptionBudgetForCR(rf *databasesv1.RedisFailover, selectors map[string]string) *policyv1.PodDisruptionBudget {
	maxUnavailable := intstr.FromInt(2)
	namespace := rf.Namespace
	selectors = MergeMap(selectors, GenerateSelectorLabels(RedisArchRoleRedis, rf.Name))
	labels := MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleRedis, rf.Name), selectors)

	name := GetSentinelStatefulSetName(rf.Name)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectors,
			},
		},
	}
}
