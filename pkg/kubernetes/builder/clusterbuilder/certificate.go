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
	"time"

	redisv1alpha1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/util"
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	v12 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenerateCertName
func GenerateCertName(name string) string {
	return name + "-cert"
}

func GetRedisSSLSecretName(name string) string {
	return fmt.Sprintf("%s-tls", name)
}

// GetServiceDNSName
func GetServiceDNSName(serviceName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc", serviceName, namespace)
}

// NewCertificate
func NewCertificate(drc *redisv1alpha1.DistributedRedisCluster) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateCertName(drc.Name),
			Namespace:       drc.Namespace,
			Labels:          GetClusterLabels(drc.GetName(), nil),
			OwnerReferences: util.BuildOwnerReferences(drc),
		},
		Spec: certv1.CertificateSpec{
			// 10 year
			Duration: &metav1.Duration{Duration: 87600 * time.Hour},
			DNSNames: []string{
				GetServiceDNSName(drc.Spec.ServiceName, drc.Namespace),
			},
			IssuerRef:  v12.ObjectReference{Kind: certv1.ClusterIssuerKind, Name: "cpaas-ca"},
			SecretName: GetRedisSSLSecretName(drc.Name),
		},
	}
}
