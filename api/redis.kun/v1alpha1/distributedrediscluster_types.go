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
	redisfailoverv1alpha1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/pkg/types/redis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AffinityPolicy
type AffinityPolicy string

const (
	// SoftAntiAffinity the master and slave will be scheduled on different node if possible
	SoftAntiAffinity AffinityPolicy = "SoftAntiAffinity"
	// AntiAffinityInSharding the master and slave must be scheduled on different node
	AntiAffinityInSharding AffinityPolicy = "AntiAffinityInSharding"
	// AntiAffinity all redis pods must be scheduled on different node
	AntiAffinity AffinityPolicy = "AntiAffinity"
)

// StorageType
type StorageType string

const (
	PersistentClaim StorageType = "persistent-claim"
	Ephemeral       StorageType = "ephemeral"
)

// Monitor
type Monitor struct {
	// Image monitor image
	Image string `json:"image,omitempty"`

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

// DistributedRedisClusterSpec defines the desired state of DistributedRedisCluster
type DistributedRedisClusterSpec struct {
	// Image is the Redis image to run.
	Image string `json:"image,omitempty"`
	// ImagePullPolicy is the pull policy for the Redis image.
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecrets is the list of pull secrets for the Redis image.
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// Command is the Redis image command.
	Command []string `json:"command,omitempty"`
	// Env inject envs to redis pods.
	Env []corev1.EnvVar `json:"env,omitempty"`
	// MasterSize is the number of shards
	MasterSize int32 `json:"masterSize,omitempty"`
	// ClusterReplicas is the number of replicas for each shard
	ClusterReplicas int32 `json:"clusterReplicas,omitempty"`
	// ServiceName is the name of the statefulset
	ServiceName string `json:"serviceName,omitempty"`

	// Use this map to setup redis service. Most of the settings is key-value format.
	//
	// For client-output-buffer-limit and rename, the values is split by group.
	Config map[string]string `json:"config,omitempty"`
	// This field specifies the assignment of cluster shard slots.
	// this config is only works for new create instance, update will not take effect after instance is startup
	Shards []ClusterShardConfig `json:"shards,omitempty"`

	// AffinityPolicy
	// +kubebuilder:validation:Enum=SoftAntiAffinity;AntiAffinityInSharding;AntiAffinity
	AffinityPolicy AffinityPolicy `json:"affinityPolicy,omitempty"`

	// Affinity
	// +kubebuilder:deprecated:warning="This version is deprecated in favor of AffinityPolicy"
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// NodeSelector
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// SecurityContext for redis pods
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// ContainerSecurityContext for redis container
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`
	// Annotations annotations inject to redis pods
	Annotations map[string]string `json:"annotations,omitempty"`
	// Storage storage config for redis pods
	Storage *RedisStorage `json:"storage,omitempty"`
	// StorageType storage type for redis pods
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// PasswordSecret password secret for redis pods
	PasswordSecret *corev1.LocalObjectReference `json:"passwordSecret,omitempty"`

	// Monitor
	Monitor *Monitor `json:"monitor,omitempty"`
	// ServiceMonitor
	ServiceMonitor redisfailoverv1alpha1.RedisServiceMonitorSpec `json:"serviceMonitor,omitempty"`

	// Backup and proxy
	//
	// TODO: refactor backup
	// Backup and proxy should decoupling from DistributedRedisCluster.

	// Set backup schedule
	Backup redisfailoverv1alpha1.RedisBackup `json:"backup,omitempty"`
	// Restore restore redis data from backup
	Restore redisfailoverv1alpha1.RedisRestore `json:"restore,omitempty"`
	// EnableTLS enable TLS for redis
	EnableTLS bool `json:"enableTLS,omitempty"`
	// Expose config for service access
	Expose redisfailoverv1alpha1.RedisExpose `json:"expose,omitempty"`
	// IPFamilyPrefer the prefered IP family, enum: IPv4, IPv6
	IPFamilyPrefer corev1.IPFamily `json:"ipFamilyPrefer,omitempty"`
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

// RedisClusterNode represent a RedisCluster Node
type RedisClusterNode struct {
	// ID id of redis-server
	ID string `json:"id"`
	// Role redis-server role
	Role redis.RedisRole `json:"role"`
	// IP current pod ip
	IP string `json:"ip"`
	// Port current pod port
	Port string `json:"port"`
	// Slots this master node holds
	Slots []string `json:"slots,omitempty"`
	// MasterRef referred to the master node
	MasterRef string `json:"masterRef,omitempty"`
	// PodName pod name
	PodName string `json:"podName"`
	// NodeName node name the pod hosted
	NodeName string `json:"nodeName"`
	// StatefulSet the statefulset current pod belongs
	StatefulSet string `json:"statefulSet"`
}

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
	Nodes []RedisClusterNode `json:"nodes,omitempty"`
	// ClusterStatus the cluster status
	ClusterStatus ClusterServiceStatus `json:"clusterStatus,omitempty"`
	// Shards the shards status
	Shards []*ClusterShards `json:"shards,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Shards",type="integer",JSONPath=".status.numberOfMaster",description="Current Shards"
//+kubebuilder:printcolumn:name="Service Status",type="string",JSONPath=".status.clusterStatus",description="Service status"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status",description="Instance status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.reason",description="Status message"

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
