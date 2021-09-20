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

package clientset

import (
	"context"

	clusterv1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DistributedRedisCluster interface {
	GetDistributedRedisCluster(ctx context.Context, namespace, name string) (*clusterv1.DistributedRedisCluster, error)
}

type DistributedRedisClusterOption struct {
	client client.Client
	logger logr.Logger
}

func NewDistributedRedisCluster(kubeClient client.Client, logger logr.Logger) DistributedRedisCluster {
	logger = logger.WithValues("service", "k8s.deployment")
	return &DistributedRedisClusterOption{
		client: kubeClient,
		logger: logger,
	}
}

func (r *DistributedRedisClusterOption) GetDistributedRedisCluster(ctx context.Context, namespace, name string) (*clusterv1.DistributedRedisCluster, error) {
	ret := clusterv1.DistributedRedisCluster{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &ret)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}
