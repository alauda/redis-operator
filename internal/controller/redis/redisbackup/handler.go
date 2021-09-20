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

package redisbackup

import (
	"context"
	"errors"
	"net/url"
	"reflect"

	redisbackupv1 "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/internal/controller/redis/redisbackup/service"
	"github.com/alauda/redis-operator/pkg/config"
	k8s "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rfLabelManagedByKey = "app.kubernetes.io/managed-by"
	rfLabelNameKey      = "redisbackups.redis.middleware.alauda.io/name"
	labelInstanceKey    = "redisbackups.redis.middleware.alauda.io/instanceName"

	operatorName = "redis-backup"
)

var (
	defaultLabels = map[string]string{
		rfLabelManagedByKey: operatorName,
	}
)

// RedisBackupHandler is the Redis Backup handler. This handler will create the required
// resources that a RF needs.
type RedisBackupHandler struct {
	backupService service.RedisBackupClient
	k8sservices   k8s.ClientSet
	logger        logr.Logger
}

// NewRedisBackupHandler returns a new RF handler
func NewRedisBackupHandler(k8sservice k8s.ClientSet, backupService service.RedisBackupClient, logger logr.Logger) *RedisBackupHandler {
	return &RedisBackupHandler{
		k8sservices:   k8sservice,
		backupService: backupService,
		logger:        logger,
	}
}

func (r *RedisBackupHandler) Ensure(ctx context.Context, backup *redisbackupv1.RedisBackup) error {
	or := createOwnerReferences(backup)
	// Create the labels every object derived from this need to have.
	labels := getLabels(backup)

	if backup.Spec.Source.RedisName != "" && backup.Spec.Source.SourceType == redisbackupv1.Cluster {
		if err := r.backupService.EnsureInfoAnnotationsAndLabelsForCluster(ctx, backup); err != nil {
			return err
		}
	} else {
		if err := r.backupService.EnsureInfoAnnotationsAndLabels(ctx, backup); err != nil {
			return err
		}
	}
	if err := r.backupService.EnsureRoleReady(ctx, backup); err != nil {
		return err
	}
	if err := r.backupService.EnsureStorageReady(ctx, backup, labels, or); err != nil {
		return err
	}
	if err := r.backupService.EnsureBackupJobCreated(ctx, backup, labels, or); err != nil {
		return err
	}
	if err := r.backupService.EnsureBackupCompleted(ctx, backup); err != nil {
		return err
	}
	if err := r.backupService.UpdateBackupStatus(ctx, backup); err != nil {
		return err
	}
	return nil
}

// getLabels merges the labels (dynamic and operator static ones).
func getLabels(rf *redisbackupv1.RedisBackup) map[string]string {
	dynLabels := map[string]string{
		rfLabelNameKey:   rf.Name,
		labelInstanceKey: rf.Spec.Source.RedisFailoverName,
	}
	return util.MergeMap(defaultLabels, dynLabels)
}

func createOwnerReferences(rf *redisbackupv1.RedisBackup) []metav1.OwnerReference {
	t := reflect.TypeOf(&redisbackupv1.RedisBackup{}).Elem()

	ownerRef := metav1.NewControllerRef(rf, redisbackupv1.GroupVersion.WithKind(t.Name()))
	ownerRef.BlockOwnerDeletion = nil
	ownerRef.Controller = nil
	return []metav1.OwnerReference{*ownerRef}
}

func (r *RedisBackupHandler) DeleteS3(ctx context.Context, backup *redisbackupv1.RedisBackup) error {
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
