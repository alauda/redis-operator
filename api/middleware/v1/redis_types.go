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
	clusterv1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	redisfailoverv1 "github.com/alauda/redis-operator/api/databases/v1"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradeOption defines the upgrade strategy for the Redis instance.
type UpgradeOption struct {
	// CRVersion indicates the version to upgrade to.
	CRVersion string `json:"crVersion,omitempty"`
	// AutoUpgrade whether upgrade automatically
	AutoUpgrade *bool `json:"autoUpgrade,omitempty"`
}

// UpgradeStatus indicates the status of the bundle upgrade.
type UpgradeStatus struct {
	// CRVersion indicates the version to upgrade to.
	CRVersion string `json:"crVersion,omitempty"`
	// Message indicates the message of the upgrade.
	Message string `json:"message,omitempty"`
}

// SentinelSettings defines the specification of the sentinel cluster
type SentinelSettings struct {
	redisfailoverv1.RedisSentinelSpec `json:",inline"`
	// ExternalSentinel defines the sentinel reference
	ExternalSentinel *redisfailoverv1.SentinelReference `json:"external,omitempty"`
}

// InstanceAccess
type InstanceAccess struct {
	core.InstanceAccessBase `json:",inline"`

	// EnableNodePort defines if the nodeport is enabled
	// TODO: remove this field in 3.22
	// +kubebuilder:deprecated:warning="use serviceType instead"
	EnableNodePort bool `json:"enableNodePort,omitempty"`
}

// RedisSpec defines the desired state of Redis
type RedisSpec struct {
	// Version supports 5.0, 6.0, 6.2, 7.0, 7.2, 7.4
	// +kubebuilder:validation:Enum="5.0";"6.0";"6.2";"7.0";"7.2";"7.4"
	Version string `json:"version"`
	// Arch supports cluster, sentinel
	// +kubebuilder:validation:Enum="cluster";"sentinel";"standalone"
	Arch core.Arch `json:"arch"`
	// Resources for setting resource requirements for the Pod Resources *v1.ResourceRequirements
	Resources *corev1.ResourceRequirements `json:"resources"`

	// Persistent for Redis
	Persistent *RedisPersistent `json:"persistent,omitempty"`
	// PersistentSize set the size of the persistent volume for the Redis
	PersistentSize *resource.Quantity `json:"persistentSize,omitempty"`
	// PasswordSecret set the Kubernetes Secret containing the Redis password PasswordSecret string,key `password`
	PasswordSecret string `json:"passwordSecret,omitempty"`
	// Replicas defines desired number of replicas for Redis
	Replicas *RedisReplicas `json:"replicas,omitempty"`
	// Affinity specifies the affinity for the Pod
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// AffinityPolicy support SoftAntiAffinity, AntiAffinityInSharding, AntiAffinity, Default SoftAntiAffinity
	// +optional
	// +kubebuilder:validation:Enum="SoftAntiAffinity";"AntiAffinityInSharding";"AntiAffinity"
	AffinityPolicy core.AffinityPolicy `json:"affinityPolicy,omitempty"`
	// NodeSelector specifies the node selector for the Pod
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// tolerations defines tolerations for the Pod
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// SecurityContext sets security attributes for the Pod SecurityContex
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// CustomConfig defines custom Redis configuration settings. Some of these settings can be modified using the config set command at runtime.
	// +optional
	CustomConfig map[string]string `json:"customConfig,omitempty"`
	// PodAnnotations holds Kubernetes Pod annotations PodAnnotations
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
	//  Expose defines information for Redis nodePorts settings
	// +optional
	Expose InstanceAccess `json:"expose,omitempty"`
	// Exporter defines Redis exporter settings
	// +optional
	Exporter *redisfailoverv1.RedisExporter `json:"exporter,omitempty"`
	// EnableTLS enables TLS for Redis
	// +optional
	EnableTLS bool `json:"enableTLS,omitempty"`
	// IPFamilyPrefer sets the preferable IP family for the Pod and Redis
	// IPFamily represents the IP Family (IPv4 or IPv6). This type is used to express the family of an IP expressed by a type (e.g. service.spec.ipFamilies).
	// +kubebuilder:validation:Enum="IPv4";"IPv6";""
	IPFamilyPrefer corev1.IPFamily `json:"ipFamilyPrefer,omitempty"`
	// Pause field indicates whether Redis is paused.
	// +optional
	Pause bool `json:"pause,omitempty"`

	// Sentinel defines Sentinel configuration settings Sentinel
	// +optional
	Sentinel *redisfailoverv1.SentinelSettings `json:"sentinel,omitempty"`

	// EnableActiveRedis enable active-active model for Redis
	EnableActiveRedis bool `json:"enableActiveRedis,omitempty"`
	// ServiceID the service id for activeredis
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=15
	ServiceID *int32 `json:"serviceID,omitempty"`

	// UpgradeOption defines the upgrade strategy for the Redis instance.
	UpgradeOption *UpgradeOption `json:"upgradeOption,omitempty"`
	// Provides the ability to patch the generated manifest of several child resources.
	Patches *RedisPatchSpec `json:"patches,omitempty"`
}

// RedisPersistent defines the storage of Redis
type RedisPersistent struct {
	// This field specifies the name of the storage class that should be used for the persistent storage of Redis
	// +kubebuilder:validation:Required
	StorageClassName string `json:"storageClassName"`
}

// RedisReplicas defines the replicas of Redis
type RedisReplicas struct {
	// This field specifies the number of replicas for Redis Cluster
	Cluster *ClusterReplicas `json:"cluster,omitempty"`
	// This field specifies the number of replicas for Redis sentinel
	Sentinel *SentinelReplicas `json:"sentinel,omitempty"`
}

type ClusterReplicas struct {
	// This field specifies the number of master in Redis Cluster.
	// +kubebuilder:validation:Minimum=3
	Shard *int32 `json:"shard"`
	// This field specifies the number of replica nodes per Redis Cluster master.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	Slave *int32 `json:"slave,omitempty"`
	// This field specifies the assignment of cluster shard slots.
	// this config is only works for new create instance, update will not take effect after instance is startup
	Shards []clusterv1.ClusterShardConfig `json:"shards,omitempty"`
}

type SentinelReplicas struct {
	// sentinel master nodes, only 1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	Master *int32 `json:"master"`
	// This field specifies the number of replica nodes.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	Slave *int32 `json:"slave,omitempty"`
}

// RedisStatus defines the observed state of Redis
type RedisStatus struct {
	// Phase indicates whether all the resource for the instance is ok.
	// Values are as below:
	//   Initializing - Resource is in Initializing or Reconcile
	//   Ready        - All resources is ok. In most cases, Ready means the cluster is ok to use
	//   Error        - Error found when do resource init
	Phase RedisPhase `json:"phase,omitempty"`
	// This field contains an additional message for the instance's status
	Message string `json:"message,omitempty"`
	// The name of the kubernetes Secret that contains Redis password.
	PasswordSecretName string `json:"passwordSecretName,omitempty"`
	// The name of the kubernetes Service for Redis
	ServiceName string `json:"serviceName,omitempty"`
	// Matching labels selector for Redis
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	// ClusterNodes redis nodes info
	ClusterNodes []core.RedisNode `json:"clusterNodes,omitempty"`
	// Restored indicates whether the instance has been restored from a backup.
	// if the instance is set to restore from a backup, when the restore is completed, the restored field will be set to true.
	Restored bool `json:"restored,omitempty"`
	// LastShardCount indicates the last number of shards in the Redis Cluster.
	LastShardCount int32 `json:"lastShardCount,omitempty"`
	// LastVersion indicates the last version of the Redis instance.
	LastVersion string `json:"lastVersion,omitempty"`

	// UpgradeStatus indicates the status of the bundle upgrade.
	UpgradeStatus UpgradeStatus `json:"upgradeStatus,omitempty"`
	// DetailedStatusRef detailed status resource ref
	DetailedStatusRef *corev1.ObjectReference `json:"detailedStatusRef,omitempty"`
}

// RedisPhase
type RedisPhase string

const (
	// Initializing
	RedisPhaseInit RedisPhase = "Initializing"
	// Rebalancing
	RedisPhaseRebalancing RedisPhase = "Rebalancing"
	// Ready
	RedisPhaseReady RedisPhase = "Ready"
	// Error
	RedisPhaseError RedisPhase = "Error"
	// Paused
	RedisPhasePaused RedisPhase = "Paused"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Arch",type="string",JSONPath=".spec.arch",description="Instance arch"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="Redis version"
// +kubebuilder:printcolumn:name="Access",type="string",JSONPath=".spec.expose.type",description="Instance access type"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="Instance phase"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message",description="Instance status message"
// +kubebuilder:printcolumn:name="Bundle Version",type="string",JSONPath=".status.upgradeStatus.crVersion",description="Bundle Version"
// +kubebuilder:printcolumn:name="AutoUpgrade",type="boolean",JSONPath=".spec.upgradeOption.autoUpgrade",description="Enable instance auto upgrade"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"

// Redis is the Schema for the redis API
type Redis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisSpec   `json:"spec,omitempty"`
	Status RedisStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RedisList contains a list of Redis
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Redis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Redis{}, &RedisList{})
}

func (r *Redis) PasswordIsEmpty() bool {
	return len(r.Spec.PasswordSecret) == 0
}

func (r *Redis) SetStatusInit() {
	r.Status.Phase = RedisPhaseInit
}

func (r *Redis) SetStatusReady() {
	r.Status.Phase = RedisPhaseReady
	r.Status.Message = ""
}

func (r *Redis) SetStatusPaused() {
	r.Status.Phase = RedisPhasePaused
	r.Status.Message = ""
}

func (r *Redis) SetStatusRebalancing(msg string) {
	r.Status.Phase = RedisPhaseRebalancing
	r.Status.Message = msg
}

func (r *Redis) SetStatusUnReady(msg string) {
	r.Status.Phase = RedisPhaseInit
	r.Status.Message = msg
}

func (r *Redis) SetStatusError(msg string) {
	r.Status.Phase = RedisPhaseError
	r.Status.Message = msg
}

func (r *Redis) RecoverStatusError() {
	if r.Status.Phase == RedisPhaseError {
		r.Status.Phase = RedisPhaseInit
	}
	// r.Status.Message = ""
}

func (r *Redis) SetPasswordSecret(secretName string) {
	r.Status.PasswordSecretName = secretName
}

func (r *Redis) SetServiceName(serviceName string) {
	r.Status.ServiceName = serviceName
}

func (r *Redis) SetMatchLabels(labels map[string]string) {
	r.Status.MatchLabels = labels
}

// Provides the ability to patch the generated manifest of several child resources.
type RedisPatchSpec struct {
	// Patch configuration for the Service created to serve traffic to the cluster.
	Services []*Service `json:"services,omitempty"`
}

// EmbeddedObjectMeta is an embedded subset of the fields included in k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta.
// Only fields which are relevant to embedded resources are included.
type EmbeddedObjectMeta struct {
	// Name must be unique within a namespace. Is required when creating resources, although
	// some resources may allow a client to request the generation of an appropriate name
	// automatically. Name is primarily intended for creation idempotence and configuration
	// definition.
	// Cannot be updated.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`

	// Namespace defines the space within each name must be unique. An empty namespace is
	// equivalent to the "default" namespace, but "default" is the canonical representation.
	// Not all objects are required to be scoped to a namespace - the value of this field for
	// those objects will be empty.
	//
	// Must be a DNS_LABEL.
	// Cannot be updated.
	// More info: http://kubernetes.io/docs/user-guide/namespaces
	// +optional
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,3,opt,name=namespace"`

	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty" protobuf:"bytes,11,rep,name=labels"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,12,rep,name=annotations"`
}

// Patch configuration for the Service created to serve traffic to the cluster.
// Allows for the manifest of the created Service to be overwritten with custom configuration.
type Service struct {
	// +optional
	*EmbeddedObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the behavior of a Service.
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec v1.ServiceSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}
