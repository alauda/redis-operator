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
	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/samber/lo"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NewPodDisruptionBudget(sen *databasesv1.RedisSentinel, selectors map[string]string) *policyv1.PodDisruptionBudget {
	maxUnavailable := intstr.FromInt(int(sen.Spec.Replicas) / 2)
	namespace := sen.GetNamespace()
	selectors = lo.Assign(selectors, GenerateSelectorLabels(RedisArchRoleSEN, sen.Name))
	labels := lo.Assign(GetCommonLabels(sen.Name), GenerateSelectorLabels(RedisArchRoleSEN, sen.Name), selectors)

	name := GetSentinelStatefulSetName(sen.Name)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: util.BuildOwnerReferences(sen),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectors,
			},
		},
	}
}
