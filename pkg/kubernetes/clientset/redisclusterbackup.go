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

	redisbackup "github.com/alauda/redis-operator/api/redis/v1"
)

type RedisClusterBackup interface {
	GetRedisClusterBackup(ctx context.Context, namespace string, name string) (*redisbackup.RedisClusterBackup, error)
	ListRedisClusterBackups(ctx context.Context, namespace string, listOps client.ListOptions) (*redisbackup.RedisClusterBackupList, error)
	UpdateRedisClusterBackup(ctx context.Context, backup *redisbackup.RedisClusterBackup) error
	UpdateRedisClusterBackupStatus(ctx context.Context, backup *redisbackup.RedisClusterBackup) error
	DeleteRedisClusterBackup(ctx context.Context, namespace string, name string) error
}

type RedisClusterBackupOption struct {
	client client.Client
	logger logr.Logger
}

func NewRedisClusterBackup(kubeClient client.Client, logger logr.Logger) RedisClusterBackup {
	logger = logger.WithValues("service", "k8s.RedisClusterBackup")
	return &RedisClusterBackupOption{
		client: kubeClient,
		logger: logger,
	}
}

func (r *RedisClusterBackupOption) GetRedisClusterBackup(ctx context.Context, namespace string, name string) (*redisbackup.RedisClusterBackup, error) {
	redis_backup := &redisbackup.RedisClusterBackup{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, redis_backup); err != nil {
		return nil, err
	}
	return redis_backup, nil
}

func (r *RedisClusterBackupOption) ListRedisClusterBackups(ctx context.Context, namespace string, listOps client.ListOptions) (*redisbackup.RedisClusterBackupList, error) {
	rl := &redisbackup.RedisClusterBackupList{}
	err := r.client.List(ctx, rl, &listOps)
	if err != nil {
		return nil, err
	}
	return rl, err
}

func (r *RedisClusterBackupOption) UpdateRedisClusterBackup(ctx context.Context, backup *redisbackup.RedisClusterBackup) error {
	if err := r.client.Update(ctx, backup); err != nil {
		return err
	}
	r.logger.Info("redisclusterbackup updated", "name", backup.Name)

	return nil
}

// UpdateRedisClusterBackup update redisbackup.Service interface.
func (r *RedisClusterBackupOption) UpdateRedisClusterBackupStatus(ctx context.Context, backup *redisbackup.RedisClusterBackup) error {
	if err := r.client.Status().Update(ctx, backup); err != nil {
		return err
	}
	r.logger.Info("redisclusterbackup status updated", "name", backup.Name)

	return nil
}

func (r *RedisClusterBackupOption) DeleteRedisClusterBackup(ctx context.Context, namespace string, name string) error {
	redis_backup := &redisbackup.RedisClusterBackup{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, redis_backup); err != nil {
		return err
	}
	return r.client.Delete(ctx, redis_backup)
}
