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
	"fmt"

	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	RedisArchRoleSEN     = "sentinel"
	RedisSentinelSVCPort = 26379
)

func NewSentinelServiceForCR(inst *v1.RedisSentinel, selectors map[string]string) *corev1.Service {
	var (
		namespace = inst.Namespace
		name      = GetSentinelServiceName(inst.Name)
		ptype     = corev1.IPFamilyPolicySingleStack
		protocol  = []corev1.IPFamily{}
	)
	if inst.Spec.Expose.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}

	selectorLabels := lo.Assign(GenerateSelectorLabels(RedisArchRoleSEN, inst.Name), GetCommonLabels(inst.Name))
	if len(selectors) > 0 {
		selectorLabels = lo.Assign(selectors, GenerateSelectorLabels(RedisArchRoleSEN, inst.Name))
	}
	// NOTE: remove this label for compatibility for old instances
	// TODO: remove this in 3.22
	delete(selectorLabels, "redissentinels.databases.spotahome.com/name")
	labels := GetCommonLabels(inst.Name, GenerateSelectorLabels(RedisArchRoleSEN, inst.Name), selectorLabels)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			Annotations:     inst.Spec.Expose.Annotations,
			OwnerReferences: util.BuildOwnerReferences(inst),
		},
		Spec: corev1.ServiceSpec{
			Type:           inst.Spec.Expose.ServiceType,
			IPFamilyPolicy: &ptype,
			IPFamilies:     protocol,
			Selector:       selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       SentinelContainerPortName,
					Port:       RedisSentinelSVCPort,
					TargetPort: intstr.FromInt(RedisSentinelSVCPort),
					Protocol:   "TCP",
					NodePort:   inst.Spec.Expose.AccessPort,
				},
			},
		},
	}
}

func NewSentinelHeadlessServiceForCR(inst *v1.RedisSentinel, selectors map[string]string) *corev1.Service {
	name := GetSentinelHeadlessServiceName(inst.Name)
	namespace := inst.Namespace
	selectorLabels := GenerateSelectorLabels(RedisArchRoleSEN, inst.Name)
	labels := GetCommonLabels(inst.Name, selectors, selectorLabels)
	labels[builder.LabelRedisArch] = RedisArchRoleSEN
	annotations := inst.Spec.Expose.Annotations
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if inst.Spec.Expose.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			Annotations:     annotations,
			OwnerReferences: util.BuildOwnerReferences(inst),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Type:           corev1.ServiceTypeClusterIP,
			ClusterIP:      corev1.ClusterIPNone,
			Selector:       selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       SentinelContainerPortName,
					Port:       RedisSentinelSVCPort,
					TargetPort: intstr.FromInt(RedisSentinelSVCPort),
					Protocol:   "TCP",
				},
			},
		},
	}
}

func NewRedisNodePortService(inst *v1.RedisSentinel, index int, nodePort int32, selectors map[string]string) *corev1.Service {
	var (
		namespace = inst.Namespace
		name      = fmt.Sprintf("%s-%d", GetSentinelStatefulSetName(inst.Name), index)
		ptype     = corev1.IPFamilyPolicySingleStack
		protocol  = []corev1.IPFamily{}
	)
	if inst.Spec.Expose.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	labels := lo.Assign(GetCommonLabels(inst.Name), selectors, GenerateSelectorLabels(RedisArchRoleSEN, inst.Name))

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			Annotations:     inst.Spec.Expose.Annotations,
			OwnerReferences: util.BuildOwnerReferences(inst),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Type:           corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Port:       RedisSentinelSVCPort,
					TargetPort: intstr.FromInt(RedisSentinelSVCPort),
					Protocol:   corev1.ProtocolTCP,
					Name:       SentinelContainerPortName,
					NodePort:   nodePort,
				},
			},
			Selector: map[string]string{builder.PodNameLabelKey: name},
		},
	}
}

// NewPodService returns a new Service for the given RedisFailover and index, with the configed service type
func NewPodService(sen *v1.RedisSentinel, index int, selectors map[string]string) *corev1.Service {
	return NewPodNodePortService(sen, index, selectors, 0)
}

func NewPodNodePortService(sen *v1.RedisSentinel, index int, selectors map[string]string, nodePort int32) *corev1.Service {
	var (
		namespace = sen.Namespace
		name      = GetSentinelNodeServiceName(sen.Name, index)
		ptype     = corev1.IPFamilyPolicySingleStack
		protocol  = []corev1.IPFamily{}
	)
	if sen.Spec.Expose.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	labels := lo.Assign(GetCommonLabels(sen.Name), selectors, GenerateSelectorLabels(RedisArchRoleSEN, sen.Name))
	selectorLabels := map[string]string{
		builder.PodNameLabelKey: name,
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			Annotations:     sen.Spec.Expose.Annotations,
			OwnerReferences: util.BuildOwnerReferences(sen),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Type:           corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Port:       RedisSentinelSVCPort,
					TargetPort: intstr.FromInt(RedisSentinelSVCPort),
					Protocol:   corev1.ProtocolTCP,
					Name:       SentinelContainerPortName,
					NodePort:   nodePort,
				},
			},
			Selector: selectorLabels,
		},
	}
}
