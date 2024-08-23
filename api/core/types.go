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

package core

// +kubebuilder:object:generate=true

import (
	"net"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type Arch string

const (
	// RedisCluster is the Redis Cluster arch
	RedisCluster Arch = "cluster"
	// RedisSentinel is the Redis Sentinel arch, which should be renamed to Failover
	RedisSentinel Arch = "sentinel"
	// RedisStandalone is the Redis Standalone arch
	RedisStandalone Arch = "standalone"
	// RedisStdSentinel is the Redis Standard Sentinel arch
	RedisStdSentinel Arch = "stdsentinel"
)

// RedisRole redis node role type
type RedisRole string

const (
	// RedisRoleMaster Master node role
	RedisRoleMaster RedisRole = "Master"
	// RedisRoleReplica Master node role
	RedisRoleReplica RedisRole = "Slave"
	// RedisRoleSentinel Master node role
	RedisRoleSentinel RedisRole = "Sentinel"
	// RedisRoleNone None node role
	RedisRoleNone RedisRole = "None"
)

// AffinityPolicy
type AffinityPolicy string

const (
	SoftAntiAffinity       AffinityPolicy = "SoftAntiAffinity"
	AntiAffinityInSharding AffinityPolicy = "AntiAffinityInSharding"
	AntiAffinity           AffinityPolicy = "AntiAffinity"
)

// InstanceAccessBase
type InstanceAccessBase struct {
	// ServiceType defines the type of the all related service
	// +kubebuilder:validation:Enum=NodePort;LoadBalancer;ClusterIP
	ServiceType corev1.ServiceType `json:"type,omitempty"`

	// The annnotations of the service which attached to services
	Annotations map[string]string `json:"annotations,omitempty"`
	// AccessPort defines the lb access nodeport
	AccessPort int32 `json:"accessPort,omitempty"`
	// NodePortMap defines the map of the nodeport for redis nodes
	// NodePortSequence defines the sequence of the nodeport for redis cluster only
	NodePortSequence string `json:"dataStorageNodePortSequence,omitempty"`
	// NodePortMap defines the map of the nodeport for redis sentinel only
	// TODO: deprecated this field with NodePortSequence in 3.22
	// Reversed for 3.14 backward compatibility
	NodePortMap map[string]int32 `json:"dataStorageNodePortMap,omitempty"`
}

// InstanceAccess
type InstanceAccess struct {
	InstanceAccessBase `json:",inline"`

	// IPFamily represents the IP Family (IPv4 or IPv6). This type is used to express the family of an IP expressed by a type (e.g. service.spec.ipFamilies).
	// +kubebuilder:validation:Enum=IPv4;IPv6
	IPFamilyPrefer corev1.IPFamily `json:"ipFamilyPrefer,omitempty"`

	// Image defines the image used to expose redis from annotations
	Image string `json:"image,omitempty"`
	// ImagePullPolicy defines the image pull policy
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// RedisNode represent a RedisCluster Node
type RedisNode struct {
	// ID is the redis cluster node id, not runid
	ID string `json:"id,omitempty"`
	// Role is the role of the node, master or slave
	Role RedisRole `json:"role"`
	// IP is the ip of the node. if access announce is enabled, it will be the access ip
	IP string `json:"ip"`
	// Port is the port of the node. if access announce is enabled, it will be the access port
	Port string `json:"port"`
	// Slots is the slot range for the shard, eg: 0-1000,1002,1005-1100
	Slots []string `json:"slots,omitempty"`
	// MasterRef is the master node id of this node
	MasterRef string `json:"masterRef,omitempty"`
	// PodName current pod name
	PodName string `json:"podName"`
	// NodeName is the node name of the node where holds the pod
	NodeName string `json:"nodeName"`
	// StatefulSet is the statefulset name of this pod
	StatefulSet string `json:"statefulSet"`
}

// RedisDetailedNode represent a redis Node with more details
type RedisDetailedNode struct {
	RedisNode
	// Version version of redis
	Version string `json:"version,omitempty"`
	// UsedMemory
	UsedMemory int64 `json:"usedMemory,omitempty"`
	// UsedMemoryDataset
	UsedMemoryDataset int64 `json:"usedMemoryDataset,omitempty"`
}

// RedisBackup defines the structure used to backup the Redis Data
type RedisBackup struct {
	Image    string     `json:"image,omitempty"`
	Schedule []Schedule `json:"schedule,omitempty"`
}

type Schedule struct {
	Name              string             `json:"name,omitempty"`
	Schedule          string             `json:"schedule"`
	Keep              int32              `json:"keep"`
	KeepAfterDeletion bool               `json:"keepAfterDeletion,omitempty"`
	Storage           RedisBackupStorage `json:"storage"`
	Target            RedisBackupTarget  `json:"target,omitempty"`
}

type RedisBackupTarget struct {
	// S3Option
	S3Option S3Option `json:"s3Option,omitempty"`
}

type S3Option struct {
	S3Secret string `json:"s3Secret,omitempty"`
	Bucket   string `json:"bucket,omitempty"`
	Dir      string `json:"dir,omitempty"`
}

type RedisBackupStorage struct {
	StorageClassName string            `json:"storageClassName,omitempty"`
	Size             resource.Quantity `json:"size,omitempty"`
}

// RedisBackup defines the structure used to restore the Redis Data
type RedisRestore struct {
	Image           string            `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	BackupName      string            `json:"backupName,omitempty"`
}

// HostPort
type HostPort struct {
	// Host the sentinel host
	Host string `json:"host,omitempty"`
	// Port the sentinel port
	Port int32 `json:"port,omitempty"`
}

func (hp *HostPort) String() string {
	return net.JoinHostPort(hp.Host, strconv.Itoa(int(hp.Port)))
}
