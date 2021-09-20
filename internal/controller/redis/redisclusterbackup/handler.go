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

package redisclusterbackup

import (
	"context"
	"errors"
	"net/url"
	"reflect"

	redisbackupv1 "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/internal/controller/redis/redisclusterbackup/service"
	"github.com/alauda/redis-operator/pkg/config"
	k8s "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rfLabelNameKey   = "redisclusterbackups.redis.middleware.alauda.io/name"
	labelInstanceKey = "redisclusterbackups.redis.middleware.alauda.io/instanceName"
)

var (
	defaultLabels = map[string]string{
		"managed-by": "redis-cluster-operator",
	}
)

type RedisClusterBackupHandler struct {
	backupService service.RedisClusterBackupClient
	k8sservices   k8s.ClientSet
	logger        logr.Logger
}

func NewRedisClusterBackupHandler(k8sservice k8s.ClientSet, backupService service.RedisClusterBackupClient, logger logr.Logger) *RedisClusterBackupHandler {
	return &RedisClusterBackupHandler{
		k8sservices:   k8sservice,
		backupService: backupService,
		logger:        logger,
	}
}

func (r *RedisClusterBackupHandler) Ensure(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error {
	or := createOwnerReferences(backup)
	// Create the labels every object derived from this need to have.
	labels := getLabels(backup)

	if err := r.backupService.EnsureInfoAnnotationsAndLabels(ctx, backup); err != nil {
		return err
	}

	if err := r.backupService.EnsureRoleReady(ctx, backup); err != nil {
		return err
	}
	if err := r.backupService.EnsureStorageReady(ctx, backup, labels, or); err != nil {
		return err
	}
	if err := r.backupService.EnsureBackupJobCreated(ctx, backup, labels, or); err != nil {
		r.logger.Info("EnsureBackupJobCreated", "err", err.Error())
		return err
	}
	if err := r.backupService.EnsureBackupCompleted(ctx, backup); err != nil {
		r.logger.Info("EnsureBackupCompleted", "err", err.Error())
		return err
	}
	if err := r.backupService.UpdateBackupStatus(ctx, backup); err != nil {
		r.logger.Info("UpdateBackupStatus", "err", err.Error())
		return err
	}
	return nil
}

func getLabels(cluster *redisbackupv1.RedisClusterBackup) map[string]string {
	dynLabels := map[string]string{
		rfLabelNameKey:   cluster.Name,
		labelInstanceKey: cluster.Spec.Source.RedisClusterName,
	}
	return util.MergeMap(defaultLabels, dynLabels)
}

func createOwnerReferences(rf *redisbackupv1.RedisClusterBackup) []metav1.OwnerReference {
	t := reflect.TypeOf(&redisbackupv1.RedisClusterBackup{}).Elem()
	ownerRef := metav1.NewControllerRef(rf, redisbackupv1.GroupVersion.WithKind(t.Name()))
	ownerRef.BlockOwnerDeletion = nil
	ownerRef.Controller = nil
	return []metav1.OwnerReference{*ownerRef}
}

func (r *RedisClusterBackupHandler) DeleteS3(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error {
	if backup.Spec.Target.S3Option.S3Secret == "" {
		return nil
	}
	secret, err := r.k8sservices.GetSecret(ctx, backup.Namespace, backup.Spec.Target.S3Option.S3Secret)
	if err != nil {
		return err
	}
	token := ""
	if v, ok := secret.Data[config.S3_TOKEN]; ok {
		token = string(v)
	}
	secure := false
	s3_url := string(secret.Data[config.S3_ENDPOINTURL])
	endpoint, err := url.Parse(s3_url)
	if err != nil {
		return err
	}
	if endpoint.Scheme == "https" {
		secure = true
	}

	s3Client, err := minio.New(endpoint.Host,
		&minio.Options{
			Creds:  credentials.NewStaticV4(string(secret.Data[config.S3_ACCESS_KEY_ID]), string(secret.Data[config.S3_SECRET_ACCESS_KEY]), token),
			Region: string(secret.Data[config.S3_REGION]),
			Secure: secure,
		})
	if err != nil {
		return err
	}
	if exists, err := s3Client.BucketExists(ctx, backup.Spec.Target.S3Option.Bucket); err != nil {
		return err
	} else if !exists {
		return errors.New("S3 Bucket no exists")
	}
	var err_list []error
	objectsCh := make(chan minio.ObjectInfo)

	go func() {
		defer close(objectsCh)
		opts := minio.ListObjectsOptions{Prefix: backup.Spec.Target.S3Option.Dir, Recursive: true}
		for object := range s3Client.ListObjects(ctx, backup.Spec.Target.S3Option.Bucket, opts) {
			if object.Err != nil {
				r.logger.Info("ListObjects", "Err", object.Err.Error())
				err_list = append(err_list, object.Err)
			}
			objectsCh <- object
		}
	}()

	errorCh := s3Client.RemoveObjects(ctx, backup.Spec.Target.S3Option.Bucket, objectsCh, minio.RemoveObjectsOptions{})
	for e := range errorCh {
		if e.Err != nil {
			return err
		}
	}
	if len(err_list) != 0 {
		err_text := "ListObjects Err:"
		for _, v := range err_list {
			err_text += v.Error()
		}
		return errors.New(err_text)
	}
	if len(err_list) != 0 {
		err_text := "ListObjects Err:"
		for _, v := range err_list {
			err_text += v.Error()
		}
		return errors.New(err_text)
	}
	return nil
}
