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

// RedisSentinel the sen service that knows how to interact with k8s to get them
type RedisSentinel interface {
	// ListRedisSentinels lists the redisfailovers on a cluster.
	ListRedisSentinels(ctx context.Context, namespace string, opts client.ListOptions) (*databasesv1.RedisSentinelList, error)
	// GetRedisSentinel get the redisfailover on a cluster.
	GetRedisSentinel(ctx context.Context, namespace, name string) (*databasesv1.RedisSentinel, error)
	// UpdateRedisSentinel update the redisfailover on a cluster.
	UpdateRedisSentinel(ctx context.Context, sen *databasesv1.RedisSentinel) error
	// UpdateRedisSentinelStatus
	UpdateRedisSentinelStatus(ctx context.Context, inst *databasesv1.RedisSentinel) error
}

// RedisSentinelService is the RedisSentinel service implementation using API calls to kubernetes.
type RedisSentinelService struct {
	client client.Client
	logger logr.Logger
}

// NewRedisSentinelService returns a new Workspace KubeService.
func NewRedisSentinelService(client client.Client, logger logr.Logger) *RedisSentinelService {
	logger = logger.WithName("k8s.redisfailover")

	return &RedisSentinelService{
		client: client,
		logger: logger,
	}
}

// ListRedisSentinels satisfies redisfailover.Service interface.
func (r *RedisSentinelService) ListRedisSentinels(ctx context.Context, namespace string, opts client.ListOptions) (*databasesv1.RedisSentinelList, error) {
	ret := databasesv1.RedisSentinelList{}
	if err := r.client.List(ctx, &ret, &opts); err != nil {
		return nil, err
	}
	return &ret, nil
}

// GetRedisSentinel satisfies redisfailover.Service interface.
func (r *RedisSentinelService) GetRedisSentinel(ctx context.Context, namespace, name string) (*databasesv1.RedisSentinel, error) {
	ret := databasesv1.RedisSentinel{}
	err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &ret)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}

// UpdateRedisSentinel
func (r *RedisSentinelService) UpdateRedisSentinel(ctx context.Context, inst *databasesv1.RedisSentinel) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst databasesv1.RedisSentinel
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			r.logger.Error(err, "get RedisSentinel failed")
			return err
		}
		inst.ResourceVersion = oldInst.ResourceVersion
		return r.client.Update(ctx, inst)
	})
}

func (r *RedisSentinelService) UpdateRedisSentinelStatus(ctx context.Context, inst *databasesv1.RedisSentinel) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst databasesv1.RedisSentinel
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			r.logger.Error(err, "get RedisSentinel failed")
			return err
		}
		if !reflect.DeepEqual(oldInst.Status, inst.Status) {
			inst.ResourceVersion = oldInst.ResourceVersion
			return r.client.Status().Update(ctx, inst)
		}
		return nil
	})
}

func (r *RedisSentinelService) DeleteRedisSentinel(ctx context.Context, namespace string, name string) error {
	ret := databasesv1.RedisSentinel{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &ret); err != nil {
		return err
	}
	return r.client.Delete(ctx, &ret)
}
