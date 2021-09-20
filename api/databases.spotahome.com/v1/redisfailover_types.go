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
	smv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RedisFailoverSpec represents a Redis failover spec
type RedisFailoverSpec struct {
	// Redis redis data node settings
	Redis RedisSettings `json:"redis,omitempty"`
	// Sentinel sentinel node settings
	Sentinel SentinelSettings `json:"sentinel,omitempty"`
	// Auth default user auth settings
	Auth AuthSettings `json:"auth,omitempty"`
	// LabelWhitelist is a list of label names that are allowed to be present on a pod
	LabelWhitelist []string `json:"labelWhitelist,omitempty"`
	// EnableTLS enable TLS for Redis
	EnableTLS bool `json:"enableTLS,omitempty"`
	// ServiceMonitor service monitor for prometheus
	ServiceMonitor RedisServiceMonitorSpec `json:"serviceMonitor,omitempty"`
	// Expose service access configuration
	Expose RedisExpose `json:"expose,omitempty"`
}

// RedisServiceMonitorSpec
type RedisServiceMonitorSpec struct {
	// CustomMetricRelabelings custom metric relabelings
	CustomMetricRelabelings bool `json:"customMetricRelabelings,omitempty"`
	// MetricRelabelConfigs metric relabel configs
	MetricRelabelConfigs []*smv1.RelabelConfig `json:"metricRelabelings,omitempty"`
	// Interval
	Interval string `json:"interval,omitempty"`
	// ScrapeTimeout
	ScrapeTimeout string `json:"scrapeTimeout,omitempty"`
}

// RedisExpose for nodeport
type RedisExpose struct {
	// EnableNodePort enable nodeport
	EnableNodePort bool `json:"enableNodePort,omitempty"`
	// ExposeImage expose image
	ExposeImage string `json:"exposeImage,omitempty"`
	// AccessPort lb service access port
	AccessPort int32 `json:"accessPort,omitempty"`
	// DataStorageNodePortSequence redis port list separated by commas
	DataStorageNodePortSequence string `json:"dataStorageNodePortSequence,omitempty"`
	// DataStorageNodePortMap redis port map referred by pod name
	DataStorageNodePortMap map[string]int32 `json:"dataStorageNodePortMap,omitempty"`
}

// RedisSettings defines the specification of the redis cluster
type RedisSettings struct {
	// Image is the Redis image to run.
	Image string `json:"image,omitempty"`
	// ImagePullPolicy is the Image pull policy.
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Replicas is the number of Redis replicas to run.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	Replicas int32 `json:"replicas,omitempty"`
	// Resources is the resource requirements for the Redis container.
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// CustomConfig is a map of custom Redis configuration options.
	CustomConfig map[string]string `json:"customConfig,omitempty"`
	// Storage redis data persistence settings.
	Storage RedisStorage `json:"storage,omitempty"`
	// Exporter prometheus exporter settings.
	Exporter RedisExporter `json:"exporter,omitempty"`
	// Affinity is the affinity settings for the Redis pods.
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// SecurityContext is the security context for the Redis pods.
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// ImagePullSecrets is the list of secrets used to pull the Redis image from a private registry.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Tolerations is the list of tolerations for the Redis pods.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// NodeSelector is the node selector for the Redis pods.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// PodAnnotations is the annotations for the Redis pods.
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
	// ServiceAnnotations is the annotations for the Redis service.
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`
	// HostNetwork is the host network settings for the Redis pods.
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// DNSPolicy is the DNS policy for the Redis pods.
	DNSPolicy corev1.DNSPolicy `json:"dnsPolicy,omitempty"`
	// Backup schedule backup configuration.
	Backup RedisBackup `json:"backup,omitempty"`
	// Restore restore redis instance from backup.
	Restore RedisRestore `json:"restore,omitempty"`
	// IPFamily represents the IP Family (IPv4 or IPv6). This type is used to express the family of an IP expressed by a type (e.g. service.spec.ipFamilies).
	// +kubebuilder:validation:Enum=IPv4;IPv6
	IPFamilyPrefer corev1.IPFamily `json:"ipFamilyPrefer,omitempty"`
}

// SentinelSettings defines the specification of the sentinel cluster
type SentinelSettings struct {
	// Image is the Redis image to run.
	Image string `json:"image,omitempty"`
	// ImagePullPolicy is the Image pull policy.
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Replicas is the number of Redis replicas to run.
	// +kubebuilder:validation:Minimum=3
	Replicas int32 `json:"replicas,omitempty"`
	// Resources is the resource requirements for the Redis container.
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// CustomConfig sentinel configuration options.
	CustomConfig map[string]string `json:"customConfig,omitempty"`
	// Command startup commands
	Command []string `json:"command,omitempty"`
	// Affinity is the affinity settings for the Redis pods.
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// SecurityContext is the security context for the Redis pods.
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// ImagePullSecrets is the list of secrets used to pull the Redis image from a private registry.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Tolerations is the list of tolerations for the Redis pods.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// NodeSelector is the node selector for the Redis pods.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// PodAnnotations is the annotations for the Redis pods.
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
	// ServiceAnnotations is the annotations for the Redis service.
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`
	// Exporter prometheus exporter settings.
	Exporter SentinelExporter `json:"exporter,omitempty"`
	// HostNetwork is the host network settings for the Redis pods.
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// DNSPolicy is the DNS policy for the Redis pods.
	DNSPolicy corev1.DNSPolicy `json:"dnsPolicy,omitempty"`
}

// AuthSettings contains settings about auth
type AuthSettings struct {
	// SecretName is the name of the secret containing the auth credentials.
	SecretPath string `json:"secretPath,omitempty"`
}

// RedisExporter defines the specification for the redis exporter
type RedisExporter struct {
	// Enabled is the flag to enable redis exporter
	Enabled bool `json:"enabled,omitempty"`
	// Image exporter image
	Image string `json:"image,omitempty"`
	// ImagePullPolicy image pull policy.
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// SentinelExporter defines the specification for the sentinel exporter
type SentinelExporter struct {
	// Enabled is the flag to enable sentinel exporter
	Enabled bool `json:"enabled,omitempty"`
	// Image exporter image
	Image string `json:"image,omitempty"`
	// ImagePullPolicy image pull policy.
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// RedisStorage defines the structure used to store the Redis Data
type RedisStorage struct {
	// KeepAfterDeletion is the flag to keep the data after the RedisFailover is deleted.
	KeepAfterDeletion bool `json:"keepAfterDeletion,omitempty"`
	// PersistentVolumeClaim is the PVC volume source.
	PersistentVolumeClaim *corev1.PersistentVolumeClaim `json:"persistentVolumeClaim,omitempty"`
}

// RedisBackup defines the structure used to backup the Redis Data
type RedisBackup struct {
	// Image is the Redis backup image to run.
	Image string `json:"image,omitempty"`
	// Schedule is the backup schedule.
	Schedule []Schedule `json:"schedule,omitempty"`
}

// Schedule
type Schedule struct {
	// Name is the scheduled backup name.
	Name string `json:"name,omitempty"`
	// Schedule crontab like schedule.
	Schedule string `json:"schedule"`
	// Keep is the number of backups to keep.
	// +kubebuilder:validation:Minimum=1
	Keep int32 `json:"keep"`
	// KeepAfterDeletion is the flag to keep the data after the RedisFailover is deleted.
	KeepAfterDeletion bool `json:"keepAfterDeletion,omitempty"`
	// Storage is the backup storage configuration.
	Storage RedisBackupStorage `json:"storage"`
	// Target is the backup target configuration.
	Target RedisBackupTarget `json:"target,omitempty"`
}

// RedisBackupTarget
type RedisBackupTarget struct {
	// S3Option is the S3 backup target configuration.
	S3Option S3Option `json:"s3Option,omitempty"`
}

// S3Option
type S3Option struct {
	// S3Secret s3 storage access secret
	S3Secret string `json:"s3Secret,omitempty"`
	// Bucket s3 storage bucket
	Bucket string `json:"bucket,omitempty"`
	// Dir s3 storage dir
	Dir string `json:"dir,omitempty"`
}

// RedisBackupStorage
type RedisBackupStorage struct {
	// StorageClassName is the name of the StorageClass to use for the PersistentVolumeClaim.
	StorageClassName string `json:"storageClassName,omitempty"`
	// Size is the size of the PersistentVolumeClaim.
	Size resource.Quantity `json:"size,omitempty"`
}

// RedisBackup defines the structure used to restore the Redis Data
type RedisRestore struct {
	// Image is the Redis restore image to run.
	Image string `json:"image,omitempty"`
	// ImagePullPolicy is the Image pull policy.
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// BackupName is the backup cr name to restore.
	BackupName string `json:"backupName,omitempty"`
}

// Phase
type Phase string

// RedisFailoverStatus defines the observed state of RedisFailover
type RedisFailoverStatus struct {
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Message
	Message string `json:"message,omitempty"`
	// Creating, Pending, Fail, Ready
	Phase Phase `json:"phase,omitempty"`

	// Instance
	Instance RedisStatusInstance `json:"instance,omitempty"`
	// Master master status
	Master RedisStatusMaster `json:"master,omitempty"`
	// Version
	Version string `json:"version,omitempty"`
}

// RedisStatusInstance
type RedisStatusInstance struct {
	Redis    RedisStatusInstanceRedis    `json:"redis,omitempty"`
	Sentinel RedisStatusInstanceSentinel `json:"sentinel,omitempty"`
}

// RedisStatusInstanceRedis
type RedisStatusInstanceRedis struct {
	Size  int32 `json:"size,omitempty"`
	Ready int32 `json:"ready,omitempty"`
}

// RedisStatusInstanceSentinel
type RedisStatusInstanceSentinel struct {
	Size      int32  `json:"size,omitempty"`
	Ready     int32  `json:"ready,omitempty"`
	Service   string `json:"service,omitempty"`
	ClusterIP string `json:"clusterIp,omitempty"`
	Port      string `json:"port,omitempty"`
}

// RedisStatusMaster
type RedisStatusMaster struct {
	// Name master pod name
	Name string `json:"name"`
	// Status master service status
	Status RedisStatusMasterStatus `json:"status"`
	// Address master access ip:port
	Address string `json:"address"`
}

type RedisStatusMasterStatus string

const (
	// RedisStatusMasterOK master is online
	RedisStatusMasterOK RedisStatusMasterStatus = "ok"
	// RedisStatusMasterDown master is offline
	RedisStatusMasterDown RedisStatusMasterStatus = "down"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Master",type="string",JSONPath=".status.master.address",description="Master address"
//+kubebuilder:printcolumn:name="Master Status",type="string",JSONPath=".status.master.status",description="Master status"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Instance reconcile phase"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.reason",description="Status message"

// RedisFailover is the Schema for the redisfailovers API
type RedisFailover struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisFailoverSpec   `json:"spec,omitempty"`
	Status RedisFailoverStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedisFailoverList contains a list of RedisFailover
type RedisFailoverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisFailover `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisFailover{}, &RedisFailoverList{})
}
