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

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/pkg/types/redis"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type RedisSentinelReplica interface {
	v1.Object
	GetObjectKind() schema.ObjectKind
	Definition() *appv1.StatefulSet
	Status() *appv1.StatefulSetStatus

	// Version return the current version of redis image
	Version() redis.RedisVersion

	Nodes() []redis.RedisNode
	// Master returns the master node of this shard which has joined the cluster
	// Keep in mind that, this not means the master has been assigned slots
	Master() redis.RedisNode
	// Replicas returns nodes whoses role is slave
	Replicas() []redis.RedisNode

	Restart(ctx context.Context) error
	Refresh(ctx context.Context) error
}

type RedisSentinelNodes interface {
	v1.Object
	GetObjectKind() schema.ObjectKind
	Definition() *appv1.Deployment
	Status() *appv1.DeploymentStatus
	// Version return the current version of redis image
	Version() redis.RedisVersion

	Nodes() []redis.RedisNode

	Restart(ctx context.Context) error
	Refresh(ctx context.Context) error
}

type RedisFailoverInstance interface {
	RedisInstance
	Definition() *databasesv1.RedisFailover
	SentinelNodes() []redis.RedisNode
	Sentinel() RedisSentinelNodes
	UpdateStatus(ctx context.Context, status databasesv1.RedisFailoverStatus) error
	Selector() map[string]string
}
