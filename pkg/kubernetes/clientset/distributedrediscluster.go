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
	"reflect"

	clusterv1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DistributedRedisCluster interface {
	GetDistributedRedisCluster(ctx context.Context, namespace, name string) (*clusterv1.DistributedRedisCluster, error)
	// UpdateRedisFailover update the redisfailover on a cluster.
	UpdateDistributedRedisCluster(ctx context.Context, inst *clusterv1.DistributedRedisCluster) error
	// UpdateRedisFailoverStatus
	UpdateDistributedRedisClusterStatus(ctx context.Context, inst *clusterv1.DistributedRedisCluster) error
}

type DistributedRedisClusterOption struct {
	client client.Client
	logger logr.Logger
}

func NewDistributedRedisCluster(kubeClient client.Client, logger logr.Logger) DistributedRedisCluster {
	logger = logger.WithName("DistributedRedisCluster")
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

// UpdateRedisFailover
func (r *DistributedRedisClusterOption) UpdateDistributedRedisCluster(ctx context.Context, inst *clusterv1.DistributedRedisCluster) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst clusterv1.DistributedRedisCluster
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			r.logger.Error(err, "get DistributedRedisCluster failed")
			return err
		}
		inst.ResourceVersion = oldInst.ResourceVersion
		return r.client.Update(ctx, inst)
	})
}

func (r *DistributedRedisClusterOption) UpdateDistributedRedisClusterStatus(ctx context.Context, inst *clusterv1.DistributedRedisCluster) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst clusterv1.DistributedRedisCluster
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			r.logger.Error(err, "get DistributedRedisCluster failed")
			return err
		}

		if !reflect.DeepEqual(oldInst.Status, inst.Status) {
			inst.ResourceVersion = oldInst.ResourceVersion
			return r.client.Status().Update(ctx, inst)
		}
		return nil
	})
}
