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

// RedisFailoverStatus defines the observed state of RedisFailover

const (
	DefaultSentinelNumber = 3
)

// RedisFailoverSpec represents a Redis failover spec
type RedisFailoverSpec struct {
	Redis    RedisSettings     `json:"redis,omitempty"`
	Sentinel *SentinelSettings `json:"sentinel,omitempty"`
	// Auth
	Auth AuthSettings `json:"auth,omitempty"`

	// TODO: remove this field in 3.20, which not used
	// +kubebuilder:deprecatedversion:warning=not supported anymore
	LabelWhitelist []string `json:"labelWhitelist,omitempty"`

	// EnableActiveRedis enable active-active model for Redis
	EnableActiveRedis bool `json:"enableActiveRedis,omitempty"`
	// ServiceID the service id for activeredis
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=15
	ServiceID *int32 `json:"serviceID,omitempty"`
}

// RedisSettings defines the specification of the redis cluster
type RedisSettings struct {
	Image            string                        `json:"image,omitempty"`
	ImagePullPolicy  corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Replicas         int32                         `json:"replicas,omitempty"`
	Resources        corev1.ResourceRequirements   `json:"resources,omitempty"`
	// CustomConfig custom redis configuration
	CustomConfig map[string]string `json:"customConfig,omitempty"`
	Storage      RedisStorage      `json:"storage,omitempty"`

	// Exporter
	Exporter RedisExporter `json:"exporter,omitempty"`
	// Expose
	Expose core.InstanceAccess `json:"expose,omitempty"`
	// EnableTLS enable TLS for Redis
	EnableTLS bool `json:"enableTLS,omitempty"`

	Affinity           *corev1.Affinity           `json:"affinity,omitempty"`
	SecurityContext    *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	Tolerations        []corev1.Toleration        `json:"tolerations,omitempty"`
	NodeSelector       map[string]string          `json:"nodeSelector,omitempty"`
	PodAnnotations     map[string]string          `json:"podAnnotations,omitempty"`
	ServiceAnnotations map[string]string          `json:"serviceAnnotations,omitempty"`

	Backup  core.RedisBackup  `json:"backup,omitempty"`
	Restore core.RedisRestore `json:"restore,omitempty"`
}

// Authorization defines the authorization settings for redis
type Authorization struct {
	// Username the username for redis
	Username string `json:"username,omitempty"`
	// PasswordSecret the password secret for redis
	PasswordSecret string `json:"passwordSecret,omitempty"`
	// TLSSecret the tls secret
	TLSSecret string `json:"tlsSecret,omitempty"`
}

// SentinelReference defines the sentinel reference
type SentinelReference struct {
	// Addresses the sentinel addresses
	// +kubebuilder:validation:MinItems=3
	Nodes []SentinelMonitorNode `json:"nodes,omitempty"`
	// Auth the sentinel auth
	Auth Authorization `json:"auth,omitempty"`
}

// SentinelSettings defines the specification of the sentinel cluster
type SentinelSettings struct {
	RedisSentinelSpec `json:",inline"`
	// SentinelReference the sentinel reference
	SentinelReference *SentinelReference `json:"sentinelReference,omitempty"`
	// MonitorConfig configs for sentinel to monitor this replication, including:
	// - down-after-milliseconds
	// - failover-timeout
	// - parallel-syncs
	MonitorConfig map[string]string `json:"monitorConfig,omitempty"`
	// Quorum the number of Sentinels that need to agree about the fact the master is not reachable,
	// in order to really mark the master as failing, and eventually start a failover procedure if possible.
	// If not specified, the default value is the majority of the Sentinels.
	Quorum *int32 `json:"quorum,omitempty"`
}

// AuthSettings contains settings about auth
type AuthSettings struct {
	SecretPath string `json:"secretPath,omitempty"`
}

// RedisExporter defines the specification for the redis exporter
type RedisExporter struct {
	Enabled         bool                        `json:"enabled,omitempty"`
	Image           string                      `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SentinelExporter defines the specification for the sentinel exporter
type SentinelExporter struct {
	Enabled         bool              `json:"enabled,omitempty"`
	Image           string            `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// RedisStorage defines the structure used to store the Redis Data
type RedisStorage struct {
	KeepAfterDeletion     bool                          `json:"keepAfterDeletion,omitempty"`
	EmptyDir              *corev1.EmptyDirVolumeSource  `json:"emptyDir,omitempty"`
	PersistentVolumeClaim *corev1.PersistentVolumeClaim `json:"persistentVolumeClaim,omitempty"`
}

type Phase string

const (
	Fail            Phase = "Fail"
	Creating        Phase = "Creating"
	Pending         Phase = "Pending"
	Ready           Phase = "Ready"
	WaitingPodReady Phase = "WaitingPodReady"
	Paused          Phase = "Paused"
)

type FailoverPolicy string

const (
	SentinelFailoverPolicy FailoverPolicy = "sentinel"
	ManualFailoverPolicy   FailoverPolicy = "manual"
)

type SentinelMonitorNode struct {
	// IP the sentinel node ip
	IP string `json:"ip,omitempty"`
	// Port the sentinel node port
	Port int32 `json:"port,omitempty"`
	// Flags
	Flags string `json:"flags,omitempty"`
}

type MonitorStatus struct {
	// Policy the failover policy
	Policy FailoverPolicy `json:"policy,omitempty"`
	// Name monitor name
	Name string `json:"name,omitempty"`
	// Username sentinel username
	Username string `json:"username,omitempty"`
	// PasswordSecret
	PasswordSecret string `json:"passwordSecret,omitempty"`
	// OldPasswordSecret
	OldPasswordSecret string `json:"oldPasswordSecret,omitempty"`
	// TLSSecret the tls secret
	TLSSecret string `json:"tlsSecret,omitempty"`
	// Nodes the sentinel monitor nodes
	Nodes []SentinelMonitorNode `json:"nodes,omitempty"`
}

// RedisFailoverStatus
type RedisFailoverStatus struct {
	// Phase
	Phase Phase `json:"phase,omitempty"`
	// Message the status message
	Message string `json:"message,omitempty"`
	// Instance the redis instance replica info
	Instance RedisStatusInstance `json:"instance,omitempty"`
	// Master the redis master access info
	Master RedisStatusMaster `json:"master,omitempty"`
	// Nodes the redis cluster nodes
	Nodes []core.RedisNode `json:"nodes,omitempty"`
	// Version
	// TODO: remove this field in 3.20, which not used
	Version string `json:"version,omitempty"`
	// TLSSecret the tls secret
	TLSSecret string `json:"tlsSecret,omitempty"`
	// Monitor the monitor status
	Monitor MonitorStatus `json:"monitor,omitempty"`

	// DetailedStatusRef detailed status resource ref
	DetailedStatusRef *corev1.ObjectReference `json:"detailedStatusRef,omitempty"`
}

// RedisFailoverDetailedStatus
type RedisFailoverDetailedStatus struct {
	// Phase
	Phase Phase `json:"phase,omitempty"`
	// Message the status message
	Message string `json:"message,omitempty"`
	// Nodes the redis cluster nodes
	Nodes []core.RedisDetailedNode `json:"nodes,omitempty"`
}

type RedisStatusInstance struct {
	Redis RedisStatusInstanceRedis `json:"redis,omitempty"`
	// Sentinel the sentinel instance info
	// +kubebuilder:deprecatedversion:warning=will deprecated in 3.22
	Sentinel *RedisStatusInstanceSentinel `json:"sentinel,omitempty"`
}

type RedisStatusInstanceRedis struct {
	Size  int32 `json:"size,omitempty"`
	Ready int32 `json:"ready,omitempty"`
}

type RedisStatusInstanceSentinel struct {
	Size      int32  `json:"size,omitempty"`
	Ready     int32  `json:"ready,omitempty"`
	Service   string `json:"service,omitempty"`
	ClusterIP string `json:"clusterIp,omitempty"`
	Port      string `json:"port,omitempty"`
}

type RedisStatusMaster struct {
	Name    string                  `json:"name"`
	Status  RedisStatusMasterStatus `json:"status"`
	Address string                  `json:"address"`
}

type RedisStatusMasterStatus string

const (
	RedisStatusMasterOK   RedisStatusMasterStatus = "ok"
	RedisStatusMasterDown RedisStatusMasterStatus = "down"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.redis.replicas",description="Redis replicas"
// +kubebuilder:printcolumn:name="Sentinels",type="integer",JSONPath=".spec.sentinel.replicas",description="Redis sentinel replicas"
// +kubebuilder:printcolumn:name="Access",type="string",JSONPath=".spec.redis.expose.type",description="Instance access type"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="Instance phase"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message",description="Instance status message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"

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
