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

package clusterbuilder

import (
	"fmt"

	redisv1alpha1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewHeadlessSvcForCR creates a new headless service for the given Cluster.
func NewHeadlessSvcForCR(cluster *redisv1alpha1.DistributedRedisCluster, index int) *corev1.Service {
	name := ClusterHeadlessSvcName(cluster.Spec.ServiceName, index)
	labels := GetClusterStatefulsetSelectorLabels(cluster.Name, index)
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if cluster.Spec.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	clientPort := corev1.ServicePort{Name: "client", Port: 6379}
	gossipPort := corev1.ServicePort{Name: "gossip", Port: 16379}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       cluster.Namespace,
			Labels:          labels,
			OwnerReferences: util.BuildOwnerReferences(cluster),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Ports:          []corev1.ServicePort{clientPort, gossipPort},
			Selector:       labels,
			ClusterIP:      corev1.ClusterIPNone,
		},
	}
	return svc
}

func NewServiceForCR(cluster *redisv1alpha1.DistributedRedisCluster) *corev1.Service {
	selectors := GetClusterStatefulsetSelectorLabels(cluster.Name, -1)
	labels := GetClusterStatefulsetSelectorLabels(cluster.Name, -1)
	// Set redis arch label, for identifying redis arch in prometheus, so wo can find redis metrics data for redis cluster only.
	labels[builder.LabelRedisArch] = string(core.RedisCluster)
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if cluster.Spec.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	var ports []corev1.ServicePort
	clientPort := corev1.ServicePort{Name: "client", Port: 6379}
	gossipPort := corev1.ServicePort{Name: "gossip", Port: 16379}
	ports = append(ports, clientPort, gossipPort)

	if cluster.Spec.Monitor != nil {
		ports = append(ports, corev1.ServicePort{Name: "prom-http", Port: PrometheusExporterPortNumber})
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            cluster.Name,
			Namespace:       cluster.Namespace,
			OwnerReferences: util.BuildOwnerReferences(cluster),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Ports:          ports,
			Selector:       selectors,
		},
	}
	return svc
}

func NewNodeportSvc(cluster *redisv1alpha1.DistributedRedisCluster, name string, labels map[string]string, port int32) *corev1.Service {
	clientPort := corev1.ServicePort{Name: "client", Port: 6379, NodePort: port}
	selectorLabels := map[string]string{
		builder.PodNameLabelKey: name,
	}
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if cluster.Spec.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            name,
			Namespace:       cluster.Namespace,
			OwnerReferences: util.BuildOwnerReferences(cluster),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Ports:          []corev1.ServicePort{clientPort},
			Selector:       selectorLabels,
			Type:           corev1.ServiceTypeNodePort,
		},
	}
	return svc
}

func NewPodService(cluster *redisv1alpha1.DistributedRedisCluster, name string, typ corev1.ServiceType,
	labels map[string]string, annotations map[string]string) *corev1.Service {

	clientPort := corev1.ServicePort{Name: "client", Port: 6379}
	gossipPort := corev1.ServicePort{Name: "gossip", Port: 16379}
	selectorLabels := map[string]string{
		builder.PodNameLabelKey: name,
	}
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if cluster.Spec.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Annotations:     annotations,
			Name:            name,
			Namespace:       cluster.Namespace,
			OwnerReferences: util.BuildOwnerReferences(cluster),
		},
		Spec: corev1.ServiceSpec{
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Ports:          []corev1.ServicePort{clientPort, gossipPort},
			Selector:       selectorLabels,
			Type:           typ,
		},
	}
	return svc
}

func NewServiceWithType(cluster *redisv1alpha1.DistributedRedisCluster, typ corev1.ServiceType, port int32) *corev1.Service {
	name := RedisNodePortSvcName(cluster.Name)
	selectors := GetClusterStatefulsetSelectorLabels(cluster.Name, -1)
	labels := GetClusterStatefulsetSelectorLabels(cluster.Name, -1)
	// TODO: remove this
	// Set redis arch label, for identifying redis arch in prometheus, so wo can find redis metrics data for redis cluster only.
	labels[builder.LabelRedisArch] = string(core.RedisCluster)
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if cluster.Spec.IPFamilyPrefer == corev1.IPv6Protocol {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}
	var ports []corev1.ServicePort
	clientPort := corev1.ServicePort{Name: "client", Port: 6379, Protocol: "TCP", NodePort: port}
	ports = append(ports, clientPort)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            name,
			Namespace:       cluster.Namespace,
			Annotations:     cluster.Spec.Expose.Annotations,
			OwnerReferences: util.BuildOwnerReferences(cluster),
		},
		Spec: corev1.ServiceSpec{
			Type:           typ,
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Ports:          ports,
			Selector:       selectors,
		},
	}
	return svc
}

func RedisNodePortSvcName(clusterName string) string {
	return fmt.Sprintf("drc-%s-nodeport", clusterName)
}

func ClusterStatefulSetSvcName(clusterName string, index string) string {
	return fmt.Sprintf("drc-%s-%s", clusterName, index)
}

func ClusterNodeSvcName(clusterName string, shard, repl int) string {
	return fmt.Sprintf("drc-%s-%d-%d", clusterName, shard, repl)
}
