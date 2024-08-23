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
	"time"

	redisv1alpha1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/util"
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	v12 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewCertificate
func NewCertificate(drc *redisv1alpha1.DistributedRedisCluster) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            builder.GenerateCertName(drc.Name),
			Namespace:       drc.Namespace,
			Labels:          GetClusterLabels(drc.GetName(), nil),
			OwnerReferences: util.BuildOwnerReferences(drc),
		},
		Spec: certv1.CertificateSpec{
			// 10 year
			Duration: &metav1.Duration{Duration: 87600 * time.Hour},
			DNSNames: []string{
				builder.GetServiceDNSName(drc.Spec.ServiceName, drc.Namespace),
				builder.GetServiceDNSName(RedisProxySvcName(drc.Name), drc.Namespace),
			},
			IssuerRef:  v12.ObjectReference{Kind: certv1.ClusterIssuerKind, Name: "cpaas-ca"},
			SecretName: builder.GetRedisSSLSecretName(drc.Name),
		},
	}
}
