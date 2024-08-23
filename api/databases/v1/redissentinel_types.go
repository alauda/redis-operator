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
	"github.com/alauda/redis-operator/api/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RedisSentinelSpec defines the desired state of RedisSentinel
type RedisSentinelSpec struct {
	// Image the redis sentinel image
	Image string `json:"image,omitempty"`
	// ImagePullPolicy the image pull policy
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecrets the image pull secrets
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Replicas the number of sentinel replicas
	// +kubebuilder:validation:Minimum=3
	Replicas int32 `json:"replicas,omitempty"`
	// Resources the resources for sentinel
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Config the config for sentinel
	CustomConfig map[string]string `json:"customConfig,omitempty"`
	// Exporter defines the specification for the sentinel exporter
	Exporter *SentinelExporter `json:"exporter,omitempty"`
	// Expose
	Expose core.InstanceAccess `json:"expose,omitempty"`
	// PasswordSecret
	PasswordSecret string `json:"passwordSecret,omitempty"`
	// EnableTLS enable TLS for redis
	EnableTLS bool `json:"enableTLS,omitempty"`
	// ExternalTLSSecret the external TLS secret to use, if not provided, the operator will create one
	ExternalTLSSecret string `json:"externalTLSSecret,omitempty"`

	Affinity           *corev1.Affinity           `json:"affinity,omitempty"`
	SecurityContext    *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	Tolerations        []corev1.Toleration        `json:"tolerations,omitempty"`
	NodeSelector       map[string]string          `json:"nodeSelector,omitempty"`
	PodAnnotations     map[string]string          `json:"podAnnotations,omitempty"`
	ServiceAnnotations map[string]string          `json:"serviceAnnotations,omitempty"`
}

// SentinelPhase
type SentinelPhase string

const (
	// SentinelCreating the sentinel creating phase
	SentinelCreating SentinelPhase = "Creating"
	// SentinelPaused the sentinel paused phase
	SentinelPaused SentinelPhase = "Paused"
	// SentinelReady the sentinel ready phase
	SentinelReady SentinelPhase = "Ready"
	// SentinelFail the sentinel fail phase
	SentinelFail SentinelPhase = "Fail"
)

// RedisSentinelStatus defines the observed state of RedisSentinel
type RedisSentinelStatus struct {
	// Phase the status phase
	Phase SentinelPhase `json:"phase,omitempty"`
	// Message the status message
	Message string `json:"message,omitempty"`
	// Nodes the redis cluster nodes
	Nodes []core.RedisNode `json:"nodes,omitempty"`
	// TLSSecret the tls secret
	TLSSecret string `json:"tlsSecret,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas",description="Sentinel replicas"
// +kubebuilder:printcolumn:name="Access",type="string",JSONPath=".spec.expose.type",description="Instance access type"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="Instance phase"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message",description="Instance status message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"

// RedisSentinel is the Schema for the redissentinels API
type RedisSentinel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisSentinelSpec   `json:"spec,omitempty"`
	Status RedisSentinelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RedisSentinelList contains a list of RedisSentinel
type RedisSentinelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisSentinel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisSentinel{}, &RedisSentinelList{})
}
