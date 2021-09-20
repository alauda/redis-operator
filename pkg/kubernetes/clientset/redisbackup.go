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

type RedisBackup interface {
	GetRedisBackup(ctx context.Context, namespace string, name string) (*redisbackup.RedisBackup, error)
	ListRedisBackups(ctx context.Context, namespace string, listOps client.ListOptions) (*redisbackup.RedisBackupList, error)
	UpdateRedisBackup(ctx context.Context, backup *redisbackup.RedisBackup) error
	UpdateRedisBackupStatus(ctx context.Context, backup *redisbackup.RedisBackup) error
	DeleteRedisBackup(ctx context.Context, namespace string, name string) error
}

type RedisBackupOption struct {
	client client.Client
	logger logr.Logger
}

func NewRedisBackup(kubeClient client.Client, logger logr.Logger) RedisBackup {
	logger = logger.WithValues("service", "k8s.RedisBackup")
	return &RedisBackupOption{
		client: kubeClient,
		logger: logger,
	}
}

func (r *RedisBackupOption) GetRedisBackup(ctx context.Context, namespace string, name string) (*redisbackup.RedisBackup, error) {
	redis_backup := &redisbackup.RedisBackup{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, redis_backup)

	if err != nil {
		return nil, err
	}
	return redis_backup, err
}

func (r *RedisBackupOption) ListRedisBackups(ctx context.Context, namespace string, listOps client.ListOptions) (*redisbackup.RedisBackupList, error) {
	rl := &redisbackup.RedisBackupList{}
	err := r.client.List(ctx, rl, &listOps)
	if err != nil {
		return nil, err
	}
	return rl, err
}

func (r *RedisBackupOption) UpdateRedisBackup(ctx context.Context, backup *redisbackup.RedisBackup) error {
	if err := r.client.Update(ctx, backup); err != nil {
		return err
	}
	r.logger.Info("redisbackup updated", "name", backup.Name)

	return nil
}

// UpdateRedisBackup update redisbackup.Service interface.
func (r *RedisBackupOption) UpdateRedisBackupStatus(ctx context.Context, backup *redisbackup.RedisBackup) error {
	if err := r.client.Status().Update(ctx, backup); err != nil {
		return err
	}
	r.logger.Info("redisbackup status updated", "name", backup.Name)

	return nil
}

func (r *RedisBackupOption) DeleteRedisBackup(ctx context.Context, namespace string, name string) error {
	redis_backup := &redisbackup.RedisBackup{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, redis_backup); err != nil {
		return err
	}
	return r.client.Delete(ctx, redis_backup)
}
