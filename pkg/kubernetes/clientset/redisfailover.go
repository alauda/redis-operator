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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
)

// RedisFailover the RF service that knows how to interact with k8s to get them
type RedisFailover interface {
	// ListRedisFailovers lists the redisfailovers on a cluster.
	ListRedisFailovers(ctx context.Context, namespace string, opts client.ListOptions) (*databasesv1.RedisFailoverList, error)
	// GetRedisFailover get the redisfailover on a cluster.
	GetRedisFailover(ctx context.Context, namespace, name string) (*databasesv1.RedisFailover, error)
	// UpdateRedisFailover update the redisfailover on a cluster.
	UpdateRedisFailover(ctx context.Context, inst *databasesv1.RedisFailover) error
	// UpdateRedisFailoverStatus
	UpdateRedisFailoverStatus(ctx context.Context, inst *databasesv1.RedisFailover) error
}

// RedisFailoverService is the RedisFailover service implementation using API calls to kubernetes.
type RedisFailoverService struct {
	client client.Client
	logger logr.Logger
}

// NewRedisFailoverService returns a new Workspace KubeService.
func NewRedisFailoverService(client client.Client, logger logr.Logger) *RedisFailoverService {
	logger = logger.WithName("RedisFailover")

	return &RedisFailoverService{
		client: client,
		logger: logger,
	}
}

// ListRedisFailovers satisfies redisfailover.Service interface.
func (r *RedisFailoverService) ListRedisFailovers(ctx context.Context, namespace string, opts client.ListOptions) (*databasesv1.RedisFailoverList, error) {
	ret := databasesv1.RedisFailoverList{}
	err := r.client.List(ctx, &ret, &opts)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}

// GetRedisFailover satisfies redisfailover.Service interface.
func (r *RedisFailoverService) GetRedisFailover(ctx context.Context, namespace, name string) (*databasesv1.RedisFailover, error) {
	ret := databasesv1.RedisFailover{}
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
func (r *RedisFailoverService) UpdateRedisFailover(ctx context.Context, inst *databasesv1.RedisFailover) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst databasesv1.RedisFailover
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			r.logger.Error(err, "get RedisFailover failed")
			return err
		}
		inst.ResourceVersion = oldInst.ResourceVersion
		return r.client.Update(ctx, inst)
	})
}

func (r *RedisFailoverService) UpdateRedisFailoverStatus(ctx context.Context, inst *databasesv1.RedisFailover) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst databasesv1.RedisFailover
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			r.logger.Error(err, "get RedisFailover failed")
			return err
		}
		if !reflect.DeepEqual(oldInst.Status, inst.Status) {
			inst.ResourceVersion = oldInst.ResourceVersion
			return r.client.Status().Update(ctx, inst)
		}
		return nil
	})
}

func (r *RedisFailoverService) DeleteRedisFailover(ctx context.Context, namespace string, name string) error {
	ret := databasesv1.RedisFailover{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &ret); err != nil {
		return err
	}
	return r.client.Delete(ctx, &ret)
}
