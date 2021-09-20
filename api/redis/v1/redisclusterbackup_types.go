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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RedisClusterBackupSpec defines the desired state of RedisClusterBackup
type RedisClusterBackupSpec struct {
	// Source backup source
	Source ClusterRedisBackupSource `json:"source,omitempty"`
	// Storage backup storage
	Storage resource.Quantity `json:"storage,omitempty"`
	// Image backup image
	Image string `json:"image,omitempty"`
	// Target backup target
	Target RedisBackupTarget `json:"target,omitempty"`
	// BackoffLimit backoff limit
	BackoffLimit *int32 `json:"backoffLimit,omitempty" protobuf:"varint,7,opt,name=backoffLimit"`
	// ActiveDeadlineSeconds active deadline seconds
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty" protobuf:"varint,3,opt,name=activeDeadlineSeconds"`
	// SecurityContext security context
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty" protobuf:"bytes,14,opt,name=securityContext"`
	// ImagePullSecrets image pull secrets
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,15,rep,name=imagePullSecrets"`
	// Tolerations tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty" protobuf:"bytes,22,opt,name=tolerations"`
	// Affinity affinity
	Affinity *corev1.Affinity `json:"affinity,omitempty" protobuf:"bytes,18,opt,name=affinity"`
	// PriorityClassName priority class name
	PriorityClassName string `json:"priorityClassName,omitempty" protobuf:"bytes,24,opt,name=priorityClassName"`
	// NodeSelector node selector
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`
	// Resources backup pod resource config
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// RedisClusterBackupStatus defines the observed state of RedisClusterBackup
type RedisClusterBackupStatus struct {
	// JobName job name run this backup
	JobName string `json:"jobName,omitempty"`
	// StartTime start time
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// CompletionTime completion time
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// optional
	// where store backup data in
	Destination string `json:"destination,omitempty"`
	// Condition backup condition
	Condition RedisBackupCondition `json:"condition,omitempty"`
	// LastCheckTime last check time
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`
	// show message when backup fail
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// RedisClusterBackup is the Schema for the redisclusterbackups API
type RedisClusterBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisClusterBackupSpec   `json:"spec,omitempty"`
	Status RedisClusterBackupStatus `json:"status,omitempty"`
}

// ClusterRedisBackupSource
type ClusterRedisBackupSource struct {
	// RedisClusterName redis cluster name
	RedisClusterName string `json:"redisClusterName,omitempty"`
	// StorageClassName storage class name
	StorageClassName string `json:"storageClassName,omitempty"`
	// Endpoint redis cluster endpoint
	Endpoint []IpPort `json:"endPoint,omitempty"`
	// PasswordSecret password secret
	PasswordSecret string `json:"passwordSecret,omitempty"`
	// SSLSecretName ssl secret name
	SSLSecretName string `json:"SSLSecretName,omitempty"`
}

// +kubebuilder:object:root=true

// RedisClusterBackupList contains a list of RedisBackup
type RedisClusterBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisClusterBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisClusterBackup{}, &RedisClusterBackupList{})
}
