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

package redis

import (
	"context"
	"net"
	"strings"

	rediscli "github.com/alauda/redis-operator/pkg/redis"
	"github.com/alauda/redis-operator/pkg/types/slot"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type RedisArch string

const (
	StandaloneArch RedisArch = "standalone"
	SentinelArch             = "sentinel"
	ClusterArch              = "cluster"
)

type SentinelRole string

const (
	SentinelRoleMaster   SentinelRole = "master"
	SentinelRoleSlave    SentinelRole = "slave"
	SentinelRoleSentinel SentinelRole = "sentinel"
)

// RedisRole redis node role type
type RedisRole string

const (
	// RedisRoleMaster RedisCluster Master node role
	RedisRoleMaster RedisRole = "Master"
	// RedisRoleSlave RedisCluster Master node role
	RedisRoleSlave RedisRole = "Slave"
	// RedisNodeRoleNone None node role
	RedisRoleNone RedisRole = "None"
)

func NewRedisRole(v string) RedisRole {
	switch strings.ToLower(v) {
	case "master":
		return RedisRoleMaster
	case "slave", "replica":
		return RedisRoleSlave
	}
	return RedisRoleNone
}

// RedisNode
type RedisNode interface {
	GetObjectKind() schema.ObjectKind
	metav1.Object

	Definition() *corev1.Pod

	// ID returns cluster node id
	ID() string
	// Index the index of statefulset
	Index() int
	// IsConnected indicate where this node is accessable
	IsConnected() bool
	// IsTerminating indicate whether is pod is deleted
	IsTerminating() bool
	// IsMasterLinkUp indicate whether the master link is up
	IsMasterLinkUp() bool
	// IsReady indicate whether is main container is ready
	IsReady() bool
	// IsJoined will indicate whether this node has joined with other nodes.
	// be sure that, this can't indicate that all pods has joined
	IsJoined() bool
	// MasterID if this node is a slave, return master id it replica to
	MasterID() string
	// IsMasterFailed returns where the master is failed. if itself is master, this func will always return false
	IsMasterFailed() bool
	// CurrentVersion return current redis server version
	// this value maybe differ with cr def when do version upgrade
	CurrentVersion() RedisVersion

	// IsACLApplied returns true when the main container got ACL_CONFIGMAP_NAME env
	IsACLApplied() bool

	// Role returns the role of current node
	// be sure that for the new start redis server, the role is master when in cluster mode
	Role() RedisRole
	// Slots if this node is master, returns the slots this nodes assigned
	// else returns nil
	Slots() *slot.Slots

	GetPod() *corev1.Pod
	Config() map[string]string
	ConfigedMasterIP() string
	ConfigedMasterPort() string
	// Setup
	Setup(ctx context.Context, margs ...[]any) error
	SetMonitor(ctx context.Context, ip, port, user, password, quorum string) error
	ReplicaOf(ctx context.Context, ip, port string) error
	SetACLUser(ctx context.Context, username string, passwords []string, rules string) (interface{}, error)
	Query(ctx context.Context, cmd string, args ...any) (any, error)
	Info() rediscli.RedisInfo

	IPort() int
	InternalIPort() int
	Port() int
	InternalPort() int
	DefaultIP() net.IP
	DefaultInternalIP() net.IP
	IPs() []net.IP
	NodeIP() net.IP

	Status() corev1.PodPhase
	ContainerStatus() *corev1.ContainerStatus

	Refresh(ctx context.Context) error
}
