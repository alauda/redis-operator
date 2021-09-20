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
	"fmt"
	"path"
	"time"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const RedisBackupKind = "RedisKind"

// RedisBackupSource
type RedisBackupSource struct {
	// RedisFailoverName redisfailover name
	RedisFailoverName string `json:"redisFailoverName,omitempty"`
	// RedisName redis instance name
	RedisName string `json:"redisName,omitempty"`
	// StorageClassName
	StorageClassName string `json:"storageClassName,omitempty"`
	// SourceType redis cluster type
	SourceType ClusterType `json:"sourceType,omitempty"`
	// Endpoint redis endpoint
	Endpoint []IpPort `json:"endPoint,omitempty"`
	// PasswordSecret
	PasswordSecret string `json:"passwordSecret,omitempty"`
	// SSLSecretName redis ssl secret name
	SSLSecretName string `json:"SSLSecretName,omitempty"`
}

// RedisBackupSpec defines the desired state of RedisBackup
type RedisBackupSpec struct {
	// Source
	Source RedisBackupSource `json:"source,omitempty"`
	// Storage
	Storage resource.Quantity `json:"storage,omitempty"`
	// Image
	Image string `json:"image,omitempty"`
	// Target backup target
	Target databasesv1.RedisBackupTarget `json:"target,omitempty"`
	// Resources resource requirements for the job
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// BackoffLimit backoff limit for the job
	BackoffLimit *int32 `json:"backoffLimit,omitempty" protobuf:"varint,7,opt,name=backoffLimit"`
	// ActiveDeadlineSeconds active deadline seconds for the job
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty" protobuf:"varint,3,opt,name=activeDeadlineSeconds"`
	// NodeSelector node selector for the job
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`
	// SecurityContext security context for the job
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty" protobuf:"bytes,14,opt,name=securityContext"`
	// ImagePullSecrets image pull secrets for the job
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,15,rep,name=imagePullSecrets"`
	// Tolerations tolerations for the job
	Tolerations []corev1.Toleration `json:"tolerations,omitempty" protobuf:"bytes,22,opt,name=tolerations"`
	// Affinity affinity for the job
	Affinity *corev1.Affinity `json:"affinity,omitempty" protobuf:"bytes,18,opt,name=affinity"`
	// PriorityClassName priority class name for the job
	PriorityClassName string `json:"priorityClassName,omitempty" protobuf:"bytes,24,opt,name=priorityClassName"`
	// RestoreCreateAt restore create at
	RestoreCreateAt *metav1.Time `json:"restoreCreateAt,omitempty"`
}

type RedisBackupTarget struct {
	// S3Option
	S3Option S3Option `json:"s3Option,omitempty"`
	// GRPCOption grpc option
	GRPCOption GRPCOption `json:"grpcOption,omitempty"`
}

// GRPCOption TODO
type GRPCOption struct {
	GRPC_PASSWORD_SECRET string `json:"secretName,omitempty" `
}

// S3Option
type S3Option struct {
	// S3Secret
	S3Secret string `json:"s3Secret,omitempty"`
	// Bucket
	Bucket string `json:"bucket,omitempty"`
	// Dir
	Dir string `json:"dir,omitempty"`
}

// IpPort
type IpPort struct {
	Address    string `json:"address,omitempty"`
	Port       int64  `json:"port,omitempty"`
	MasterName string `json:"masterName,omitempty"`
}

type ClusterType string

const (
	Cluster    ClusterType = "cluster"
	Sentinel   ClusterType = "sentinel"
	Standalone ClusterType = "standalone"
)

// RedisBackupStatus defines the observed state of RedisBackup
type RedisBackupStatus struct {
	// JobName
	JobName string `json:"jobName,omitempty"`
	// StartTime
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// CompletionTime
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// Destination where store backup data in
	// +optional
	Destination string `json:"destination,omitempty"`
	// Condition
	Condition RedisBackupCondition `json:"condition,omitempty"`
	// LastCheckTime
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`
	// show message when backup fail
	// +optional
	Message string `json:"message,omitempty"`
}

// RedisBackupCondition
type RedisBackupCondition string

// These are valid conditions of a redis backup.
const (
	// RedisBackupRunning means the job running its execution.
	RedisBackupRunning RedisBackupCondition = "Running"
	// RedisBackupComplete means the job has completed its execution.
	RedisBackupComplete RedisBackupCondition = "Complete"
	// RedisBackupFailed means the job has failed its execution.
	RedisBackupFailed RedisBackupCondition = "Failed"
	// "RedisDeleteFailed" means that the deletion of backup-related resources has failed.
	RedisDeleteFailed RedisBackupCondition = "DeleteFailed"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RedisBackup is the Schema for the redisbackups API
type RedisBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisBackupSpec   `json:"spec,omitempty"`
	Status RedisBackupStatus `json:"status,omitempty"`
}

// Validate set the values by default if not defined and checks if the values given are valid
func (r *RedisBackup) Validate() error {
	if r.Spec.Source.RedisFailoverName == "" && r.Spec.Source.RedisName == "" {
		return fmt.Errorf("RedisFailoverName is not valid")
	}
	if r.Spec.Storage.IsZero() {
		return fmt.Errorf("backup storage can't be empty")
	}

	if r.Spec.Target.S3Option.S3Secret != "" {
		r.Spec.Source.Endpoint = []IpPort{{Address: fmt.Sprintf("rfs-%s", r.Spec.Source.RedisName),
			Port:       26379,
			MasterName: "mymaster",
		}}
		if r.Spec.Target.S3Option.Dir == "" {
			currentTime := time.Now().UTC()
			timeString := currentTime.Format("2006-01-02T15:04:05Z")
			r.Spec.Target.S3Option.Dir = path.Join("data", "backup", "redis-sentinel", "manual", timeString)
		}
		if r.Spec.Image == "" {
			r.Spec.Image = config.GetDefaultBackupImage()
		}
		if r.Spec.BackoffLimit == nil {
			r.Spec.BackoffLimit = new(int32)
			*r.Spec.BackoffLimit = 1
		}
	}
	if r.Spec.Resources.Limits.Cpu().IsZero() && r.Spec.Resources.Limits.Memory().IsZero() {
		r.Spec.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("500Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		}
	}
	return nil
}

//+kubebuilder:object:root=true

// RedisBackupList contains a list of RedisBackup
type RedisBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisBackup{}, &RedisBackupList{})
}
