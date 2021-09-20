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
	v1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	RedisArchRoleRedis = "redis"
	RedisArchRoleSEN   = "sentinel"
	RedisRoleMaster    = "master"
	RedisRoleSlave     = "slave"
	RedisRoleLabel     = "redis.middleware.alauda.io/role"
	RedisSVCPort       = 6379
	RedisSVCPortName   = "redis"
)

func NewRWSvcForCR(rf *v1.RedisFailover) *corev1.Service {
	selectorLabels := GenerateSelectorLabels(RedisArchRoleRedis, rf.Name)
	selectorLabels[RedisRoleLabel] = RedisRoleMaster
	labels := GetCommonLabels(rf.Name, selectorLabels)
	svcName := GetRedisRWServiceName(rf.Name)
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            svcName,
			Namespace:       rf.Namespace,
			Labels:          labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
			Annotations:     rf.Spec.Redis.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Type:           corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       RedisSVCPort,
					TargetPort: intstr.FromInt(RedisSVCPort),
					Protocol:   corev1.ProtocolTCP,
					Name:       RedisSVCPortName,
				},
			},
			Selector: selectorLabels,
		},
	}
}

func NewReadOnlyForCR(rf *v1.RedisFailover) *corev1.Service {
	selectorLabels := GenerateSelectorLabels(RedisArchRoleRedis, rf.Name)
	selectorLabels[RedisRoleLabel] = RedisRoleSlave
	labels := GetCommonLabels(rf.Name, selectorLabels)
	svcName := GetRedisROServiceName(rf.Name)
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            svcName,
			Namespace:       rf.Namespace,
			Labels:          labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
			Annotations:     rf.Spec.Redis.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Type:           corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       RedisSVCPort,
					TargetPort: intstr.FromInt(RedisSVCPort),
					Protocol:   corev1.ProtocolTCP,
					Name:       RedisSVCPortName,
				},
			},
			Selector: selectorLabels,
		},
	}
}

func NewSentinelServiceForCR(rf *v1.RedisFailover, selectors map[string]string) *corev1.Service {
	name := GetSentinelServiceName(rf.Name)
	namespace := rf.Namespace
	sentinelTargetPort := intstr.FromInt(26379)

	selectorLabels := MergeMap(GenerateSelectorLabels(RedisArchRoleSEN, rf.Name), GetCommonLabels(rf.Name))
	if len(selectors) > 0 {
		selectorLabels = MergeMap(selectors, GenerateSelectorLabels(RedisArchRoleSEN, rf.Name))
	}
	labels := GetCommonLabels(rf.Name, GenerateSelectorLabels(RedisArchRoleSEN, rf.Name), selectorLabels)

	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Selector:       selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       "sentinel",
					Port:       26379,
					TargetPort: sentinelTargetPort,
					Protocol:   "TCP",
				},
			},
		},
	}

	if rf.Spec.Expose.EnableNodePort {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       namespace,
				Labels:          labels,
				OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
			},
			Spec: corev1.ServiceSpec{
				IPFamilyPolicy: &ptype,
				IPFamilies:     protocol,
				Selector:       selectorLabels,
				Type:           corev1.ServiceTypeNodePort,
				Ports: []corev1.ServicePort{
					{
						Name:       "sentinel",
						Port:       26379,
						TargetPort: sentinelTargetPort,
						Protocol:   "TCP",
						NodePort:   rf.Spec.Expose.AccessPort,
					},
				},
			},
		}
	}
	return svc
}

func NewExporterServiceForCR(rf *v1.RedisFailover, selectors map[string]string) *corev1.Service {
	name := GetSentinelStatefulSetName(rf.Name)
	namespace := rf.Namespace
	selectorLabels := GenerateSelectorLabels(RedisArchRoleRedis, rf.Name)
	labels := GetCommonLabels(rf.Name, selectors, selectorLabels)
	labels[LabelRedisArch] = RedisArchRoleSEN
	defaultAnnotations := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   "http",
		"prometheus.io/path":   "/metrics",
	}
	annotations := MergeMap(defaultAnnotations, rf.Spec.Redis.ServiceAnnotations)
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
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
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Type:           corev1.ServiceTypeClusterIP,
			ClusterIP:      corev1.ClusterIPNone,
			Selector:       selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http-metrics",
					Port:       9121,
					TargetPort: intstr.FromInt(9121),
					Protocol:   "TCP",
				},
			},
		},
	}
}

func NewRedisNodePortService(rf *v1.RedisFailover, index string, nodePort int32, selectors map[string]string) *corev1.Service {
	namespace := rf.Namespace
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	_labels := util.MergeMap(GetCommonLabels(rf.Name), selectors, GenerateSelectorLabels(RedisArchRoleRedis, rf.Name))
	selectorLabels := map[string]string{
		"statefulset.kubernetes.io/pod-name": GetSentinelStatefulSetName(rf.Name) + "-" + index,
	}

	redisTargetPort := intstr.FromInt(6379)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GetSentinelStatefulSetName(rf.Name) + "-" + index,
			Namespace:       namespace,
			Labels:          _labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Type:           corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Port:       6379,
					Protocol:   corev1.ProtocolTCP,
					Name:       "client",
					TargetPort: redisTargetPort,
					NodePort:   nodePort,
				},
			},
			Selector: selectorLabels,
		},
	}
}
