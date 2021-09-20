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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisfailoverv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
)

// RedisFailover the RF service that knows how to interact with k8s to get them
type RedisFailover interface {
	// ListRedisFailovers lists the redisfailovers on a cluster.
	ListRedisFailovers(ctx context.Context, namespace string, opts client.ListOptions) (*redisfailoverv1.RedisFailoverList, error)
	// GetRedisFailover get the redisfailover on a cluster.
	GetRedisFailover(ctx context.Context, namespace, name string) (*redisfailoverv1.RedisFailover, error)
	// UpdateRedisFailover update the redisfailover on a cluster.
	UpdateRedisFailover(ctx context.Context, rf *redisfailoverv1.RedisFailover) error
}

// RedisFailoverService is the RedisFailover service implementation using API calls to kubernetes.
type RedisFailoverService struct {
	client client.Client
	logger logr.Logger
}

// NewRedisFailoverService returns a new Workspace KubeService.
func NewRedisFailoverService(client client.Client, logger logr.Logger) *RedisFailoverService {
	logger = logger.WithName("k8s.redisfailover")

	return &RedisFailoverService{
		client: client,
		logger: logger,
	}
}

// ListRedisFailovers satisfies redisfailover.Service interface.
func (r *RedisFailoverService) ListRedisFailovers(ctx context.Context, namespace string, opts client.ListOptions) (*redisfailoverv1.RedisFailoverList, error) {
	ret := redisfailoverv1.RedisFailoverList{}
	err := r.client.List(ctx, &ret, &opts)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}

// GetRedisFailover satisfies redisfailover.Service interface.
func (r *RedisFailoverService) GetRedisFailover(ctx context.Context, namespace, name string) (*redisfailoverv1.RedisFailover, error) {
	ret := redisfailoverv1.RedisFailover{}
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
func (r *RedisFailoverService) UpdateRedisFailover(ctx context.Context, n *redisfailoverv1.RedisFailover) error {
	o := redisfailoverv1.RedisFailover{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      n.Name,
		Namespace: n.Namespace,
	}, &o)
	if err != nil {
		return err
	}

	o.Spec = n.Spec
	o.Status = n.Status
	if err := r.client.Update(ctx, &o); err != nil {
		r.logger.Error(err, "update redis failover failed")
		return err
	}
	if err := r.client.Status().Update(ctx, &o); err != nil {
		r.logger.Error(err, "update redis failover status failed")
		return err
	}
	return err
}

func (r *RedisFailoverService) DeleteRedisFailover(ctx context.Context, namespace string, name string) error {
	ret := redisfailoverv1.RedisFailover{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &ret); err != nil {
		return err
	}
	return r.client.Delete(ctx, &ret)
}
