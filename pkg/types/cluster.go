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

package types

import (
	"context"

	clusterv1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"

	"github.com/alauda/redis-operator/pkg/security/acl"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/slot"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RedisClusterShard
type RedisClusterShard interface {
	v1.Object
	GetObjectKind() schema.ObjectKind
	Definition() *appv1.StatefulSet
	Status() *appv1.StatefulSetStatus

	// Version return the current version of redis image
	Version() redis.RedisVersion

	Index() int
	Nodes() []redis.RedisNode
	// Master returns the master node of this shard which has joined the cluster
	// Keep in mind that, this not means the master has been assigned slots
	Master() redis.RedisNode
	// Replicas returns nodes whoses role is slave
	Replicas() []redis.RedisNode

	Slots() *slot.Slots
	IsImporting() bool
	IsMigrating() bool

	Restart(ctx context.Context) error
	Refresh(ctx context.Context) error
}

// RedisInstance
type RedisClusterInstance interface {
	RedisInstance
	Status() *clusterv1.DistributedRedisClusterStatus
	Definition() *clusterv1.DistributedRedisCluster
	Shards() []RedisClusterShard
	UpdateStatus(ctx context.Context, status clusterv1.ClusterStatus, message string, shards []*clusterv1.ClusterShards) error
}

type RedisInstance interface {
	v1.Object
	Users() acl.Users
	GetObjectKind() schema.ObjectKind
	Version() redis.RedisVersion
	Masters() []redis.RedisNode
	Nodes() []redis.RedisNode
	IsInService() bool
	IsReady() bool
	Restart(ctx context.Context) error
	Refresh(ctx context.Context) error
	IsACLUserExists() bool
}
