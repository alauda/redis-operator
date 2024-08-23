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

package v1alpha1

import (
	"github.com/alauda/redis-operator/api/core"

	smv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageType
type StorageType string

const (
	PersistentClaim StorageType = "persistent-claim"
	Ephemeral       StorageType = "ephemeral"
)

// RedisServiceMonitorSpec
type RedisServiceMonitorSpec struct {
	CustomMetricRelabelings bool                  `json:"customMetricRelabelings,omitempty"`
	MetricRelabelConfigs    []*smv1.RelabelConfig `json:"metricRelabelings,omitempty"`
	Interval                string                `json:"interval,omitempty"`
	ScrapeTimeout           string                `json:"scrapeTimeout,omitempty"`
}

// PrometheusSpec
//
//	this struct must be Deprecated, only port is used.
type PrometheusSpec struct {
	// Port number for the exporter side car.
	Port int32 `json:"port,omitempty"`

	// Namespace of Prometheus. Service monitors will be created in this namespace.
	Namespace string `json:"namespace,omitempty"`
	// Labels are key value pairs that is used to select Prometheus instance via ServiceMonitor labels.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Interval at which metrics should be scraped
	Interval string `json:"interval,omitempty"`
	//Annotations map[string]string `json:"annotations,omitempty"`
}

// Monitor
type Monitor struct {
	Image           string            `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	Prometheus *PrometheusSpec `json:"prometheus,omitempty"`
	// Arguments to the entrypoint.
	// The docker image's CMD is used if this is not provided.
	// Variable references $(VAR_NAME) are expanded using the container's environment. If a variable
	// cannot be resolved, the reference in the input string will be unchanged. The $(VAR_NAME) syntax
	// can be escaped with a double $$, ie: $$(VAR_NAME). Escaped references will never be expanded,
	// regardless of whether the variable exists or not.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell
	// +optional
	Args []string `json:"args,omitempty"`
	// List of environment variables to set in the container.
	// Cannot be updated.
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	Env []corev1.EnvVar `json:"env,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	// Compute Resources required by exporter container.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Security options the pod should run with.
	// More info: https://kubernetes.io/docs/concepts/policy/security-context/
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

type ClusterShardConfig struct {
	// Slots is the slot range for the shard, eg: 0-1000,1002,1005-1100
	//+kubebuilder:validation:Pattern:=`^(\d{1,5}|(\d{1,5}-\d{1,5}))(,(\d{1,5}|(\d{1,5}-\d{1,5})))*$`
	Slots string `json:"slots,omitempty"`
}

// RedisStorage defines the structure used to store the Redis Data
type RedisStorage struct {
	Size        resource.Quantity `json:"size"`
	Type        StorageType       `json:"type,omitempty"`
	Class       string            `json:"class"`
	DeleteClaim bool              `json:"deleteClaim,omitempty"`
}

// DistributedRedisClusterSpec defines the desired state of DistributedRedisCluster
type DistributedRedisClusterSpec struct {
	// Image is the Redis image
	Image string `json:"image,omitempty"`
	// ImagePullPolicy is the Redis image pull policy
	// TODO: reset the default value to IfNotPresent in 3.20
	ImagePullPolicy  corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Command          []string                      `json:"command,omitempty"`
	// Env is the environment variables
	// TODO: remove in 3.20
	Env []corev1.EnvVar `json:"env,omitempty"`
	// MasterSize is the number of master nodes
	// +kubebuilder:validation:Minimum=3
	MasterSize int32 `json:"masterSize"`
	// ClusterReplicas is the number of replicas for each master node
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	ClusterReplicas int32 `json:"clusterReplicas"`
	// This field specifies the assignment of cluster shard slots.
	// this config is only works for new create instance, update will not take effect after instance is startup
	Shards []ClusterShardConfig `json:"shards,omitempty"`

	// ServiceName is the service name
	// TODO: remove in 3.20, this should not changed or specified
	ServiceName string `json:"serviceName,omitempty"`

	// Use this map to setup redis service. Most of the settings is key-value format.
	//
	// For client-output-buffer-limit and rename, the values is split by group.
	Config map[string]string `json:"config,omitempty"`

	// AffinityPolicy
	// +kubebuilder:validation:Enum=SoftAntiAffinity;AntiAffinityInSharding;AntiAffinity
	AffinityPolicy core.AffinityPolicy `json:"affinityPolicy,omitempty"`

	// Set RequiredAntiAffinity to force the master-slave node anti-affinity.
	//+kubebuilder:deprecatedversion:warning="redis.kun/v1alpha2 DistributedRedisCluster is deprecated, use AffinityPolicy instead"
	// TODO: remove in 3.20
	RequiredAntiAffinity bool `json:"requiredAntiAffinity,omitempty"`
	// Affinity
	// TODO: remove in 3.20
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// NodeSelector
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	SecurityContext          *corev1.PodSecurityContext   `json:"securityContext,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext      `json:"containerSecurityContext,omitempty"`
	PodAnnotations           map[string]string            `json:"annotations,omitempty"`
	Storage                  *RedisStorage                `json:"storage,omitempty"`
	Resources                *corev1.ResourceRequirements `json:"resources,omitempty"`
	PasswordSecret           *corev1.LocalObjectReference `json:"passwordSecret,omitempty"`

	// Monitor
	// TODO: added an global button to controller wether to enable monitor
	Monitor *Monitor `json:"monitor,omitempty"`
	// ServiceMonitor
	//+kubebuilder:deprecatedversion
	// not support setup service monitor for each instance
	ServiceMonitor *RedisServiceMonitorSpec `json:"serviceMonitor,omitempty"`

	// Set backup schedule
	Backup core.RedisBackup `json:"backup,omitempty"`
	// Restore restore redis data from backup
	Restore core.RedisRestore `json:"restore,omitempty"`

	// EnableTLS
	EnableTLS bool `json:"enableTLS,omitempty"`

	// Expose config for service access
	// TODO: should rename Expose to Access
	Expose core.InstanceAccess `json:"expose,omitempty"`

	// IPFamilyPrefer the prefered IP family, enum: IPv4, IPv6
	IPFamilyPrefer corev1.IPFamily `json:"ipFamilyPrefer,omitempty"`

	// EnableActiveRedis enable active-active model for Redis
	EnableActiveRedis bool `json:"enableActiveRedis,omitempty"`
	// ServiceID the service id for activeredis
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=15
	ServiceID *int32 `json:"serviceID,omitempty"`
}

// ClusterStatus Redis Cluster status
type ClusterStatus string

const (
	// ClusterStatusOK ClusterStatus OK
	ClusterStatusOK ClusterStatus = "Healthy"
	// ClusterStatusKO ClusterStatus KO
	ClusterStatusKO ClusterStatus = "Failed"
	// ClusterStatusCreating ClusterStatus Creating
	ClusterStatusCreating = "Creating"
	// ClusterStatusRollingUpdate ClusterStatus RollingUpdate
	ClusterStatusRollingUpdate ClusterStatus = "RollingUpdate"
	// ClusterStatusRebalancing ClusterStatus rebalancing
	ClusterStatusRebalancing ClusterStatus = "Rebalancing"
	// clusterStatusPaused cluster status paused
	ClusterStatusPaused ClusterStatus = "Paused"
)

// ClusterServiceStatus
type ClusterServiceStatus string

const (
	ClusterInService    ClusterServiceStatus = "InService"
	ClusterOutOfService ClusterServiceStatus = "OutOfService"
)

// ClusterShardsSlotStatus
type ClusterShardsSlotStatus struct {
	// Slots slots this shard holds or will holds
	Slots string `json:"slots,omitempty"`
	// Status the status of this status
	Status string `json:"status,omitempty"`
	// ShardIndex indicates the slots importing from or migrate to
	ShardIndex *int32 `json:"shardId"`
}

// ClusterShards
type ClusterShards struct {
	// ID match the shard-id in redis 7.0
	Id string `json:"id,omitempty"`
	// Index the shard index
	Index int32 `json:"index"`
	// Slots records the slots status of this shard
	Slots []*ClusterShardsSlotStatus `json:"slots"`
}

// NodesPlacementInfo Redis Nodes placement mode information
type NodesPlacementInfo string

const (
	// NodesPlacementInfoBestEffort the cluster nodes placement is in best effort,
	// it means you can have 2 masters (or more) on the same VM.
	NodesPlacementInfoBestEffort NodesPlacementInfo = "BestEffort"
	// NodesPlacementInfoOptimal the cluster nodes placement is optimal,
	// it means on master by VM
	NodesPlacementInfoOptimal NodesPlacementInfo = "Optimal"
)

// DistributedRedisClusterStatus defines the observed state of DistributedRedisCluster
type DistributedRedisClusterStatus struct {
	// Status the status of the cluster
	Status ClusterStatus `json:"status"`
	// Reason the reason of the status
	Reason string `json:"reason,omitempty"`
	// NumberOfMaster the number of master nodes
	NumberOfMaster int32 `json:"numberOfMaster,omitempty"`
	// MinReplicationFactor the min replication factor
	MinReplicationFactor int32 `json:"minReplicationFactor,omitempty"`
	// MaxReplicationFactor the max replication factor
	MaxReplicationFactor int32 `json:"maxReplicationFactor,omitempty"`
	// NodesPlacement the nodes placement mode
	NodesPlacement NodesPlacementInfo `json:"nodesPlacementInfo,omitempty"`
	// Nodes the redis cluster nodes
	Nodes []core.RedisNode `json:"nodes,omitempty"`
	// ClusterStatus the cluster status
	ClusterStatus ClusterServiceStatus `json:"clusterStatus,omitempty"`
	// Shards the cluster shards
	Shards []*ClusterShards `json:"shards,omitempty"`

	// DetailedStatusRef detailed status resource ref
	DetailedStatusRef *corev1.ObjectReference `json:"detailedStatusRef,omitempty"`
}

// DistributedRedisClusterDetailedStatus defines the detailed status of DistributedRedisCluster
type DistributedRedisClusterDetailedStatus struct {
	// Status indicates current status of the cluster
	Status ClusterStatus `json:"status"`
	// Reason explains the status
	Reason string `json:"reason,omitempty"`
	// NumberOfMaster the number of master nodes
	NumberOfMaster int32 `json:"numberOfMaster,omitempty"`
	// MinReplicationFactor the min replication factor
	MinReplicationFactor int32 `json:"minReplicationFactor,omitempty"`
	// MaxReplicationFactor the max replication factor
	MaxReplicationFactor int32 `json:"maxReplicationFactor,omitempty"`
	// NodesPlacement the nodes placement mode
	NodesPlacement NodesPlacementInfo `json:"nodesPlacementInfo,omitempty"`
	// Nodes the redis cluster nodes
	Nodes []core.RedisDetailedNode `json:"nodes,omitempty"`
	// ClusterStatus the cluster status
	ClusterStatus ClusterServiceStatus `json:"clusterStatus,omitempty"`
	// Shards the cluster shards
	Shards []*ClusterShards `json:"shards,omitempty"`
}

// NewDistributedRedisClusterDetailedStatus create a new DistributedRedisClusterDetailedStatus
func NewDistributedRedisClusterDetailedStatus(status *DistributedRedisClusterStatus, nodes []core.RedisDetailedNode) *DistributedRedisClusterDetailedStatus {
	ret := &DistributedRedisClusterDetailedStatus{
		Status:               status.Status,
		Reason:               status.Reason,
		NumberOfMaster:       status.NumberOfMaster,
		MinReplicationFactor: status.MinReplicationFactor,
		MaxReplicationFactor: status.MaxReplicationFactor,
		NodesPlacement:       status.NodesPlacement,
		ClusterStatus:        status.ClusterStatus,
		Shards:               status.Shards,
		Nodes:                nodes,
	}
	for _, node := range status.Nodes {
		ret.Nodes = append(ret.Nodes, core.RedisDetailedNode{RedisNode: node})
	}
	return ret
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Shards",type="integer",JSONPath=".status.numberOfMaster",description="Current Shards"
// +kubebuilder:printcolumn:name="Service Status",type="string",JSONPath=".status.clusterStatus",description="Service status"
// +kubebuilder:printcolumn:name="Access",type="string",JSONPath=".spec.expose.type",description="Instance access type"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status",description="Instance status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.reason",description="Status message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time since creation"

// DistributedRedisCluster is the Schema for the distributedredisclusters API
type DistributedRedisCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DistributedRedisClusterSpec   `json:"spec,omitempty"`
	Status DistributedRedisClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DistributedRedisClusterList contains a list of DistributedRedisCluster
type DistributedRedisClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DistributedRedisCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DistributedRedisCluster{}, &DistributedRedisClusterList{})
}
