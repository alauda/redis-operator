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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	redisbackupv1 "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/pkg/config"
	k8s "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RedisClusterBackupClient interface {
	EnsureInfoAnnotationsAndLabels(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error
	EnsureStorageReady(ctx context.Context, backup *redisbackupv1.RedisClusterBackup, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureBackupJobCreated(ctx context.Context, backup *redisbackupv1.RedisClusterBackup, labels map[string]string, ownerRefs []metav1.OwnerReference) error
	EnsureBackupCompleted(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error
	UpdateBackup(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error
	UpdateBackupStatus(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error
	EnsureRoleReady(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error
}

type RedisClusterBackupKubeClient struct {
	K8SService k8s.ClientSet
	logger     logr.Logger
}

func NewRedisClusterBackupKubeClient(k8sService k8s.ClientSet, logger logr.Logger) *RedisClusterBackupKubeClient {
	return &RedisClusterBackupKubeClient{
		K8SService: k8sService,
		logger:     logger,
	}
}

func (r *RedisClusterBackupKubeClient) EnsureInfoAnnotationsAndLabels(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error {
	instance, err := r.K8SService.GetDistributedRedisCluster(ctx, backup.Namespace, backup.Spec.Source.RedisClusterName)
	if err != nil {
		return err
	}
	if instance.Status.Reason != "OK" && instance.Status.Status != v1alpha1.ClusterStatusOK {
		return fmt.Errorf("sentinel cluster :%s, is not Ready", instance.Name)
	}
	if backup.Annotations == nil {
		backup.Annotations = map[string]string{}
	}
	err = backup.Validate()
	if err != nil {
		return err
	}
	backup.Annotations["sourceClusterReplicasShard"] = strconv.Itoa(int(instance.Spec.MasterSize))
	backup.Annotations["sourceClusterReplicasSlave"] = strconv.Itoa(int(instance.Spec.ClusterReplicas))

	backup.Annotations["sourceClusterVersion"] = config.GetRedisVersion(instance.Spec.Image)
	if !instance.Spec.Resources.Limits.Memory().IsZero() {
		size := resource.NewQuantity(instance.Spec.Resources.Limits.Memory().Value()*int64(instance.Spec.MasterSize), resource.BinarySI)
		if size.Cmp(backup.Spec.Storage) > 0 {
			backup.Spec.Storage = *size
		}
	}
	res, _ := json.Marshal(instance.Spec.Resources)
	backup.Annotations["sourceResources"] = string(res)
	if backup.Labels == nil {
		backup.Labels = map[string]string{}
	}
	backup.Labels["redis.kun/name"] = backup.Spec.Source.RedisClusterName
	return r.UpdateBackup(ctx, backup)
}

func (r *RedisClusterBackupKubeClient) EnsureStorageReady(ctx context.Context, backup *redisbackupv1.RedisClusterBackup, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	if backup.Status.Destination != "" {
		return nil
	}
	if backup.Spec.Source.StorageClassName == "" {
		return nil
	}
	pvc := generatorPVC(backup, labels, ownerRefs)
	old_pvcs, err := r.K8SService.ListPvcByLabel(ctx, pvc.Namespace, labels)
	if err != nil {
		return err
	}
	if old_pvcs == nil || len(old_pvcs.Items) == 0 {
		err := r.K8SService.CreatePVC(ctx, backup.Namespace, pvc)
		if err != nil {
			return err
		}
		backup.Status.Destination = fmt.Sprintf("%s/%s", "pvc", pvc.Name)
	} else if len(old_pvcs.Items) > 0 {
		backup.Status.Destination = fmt.Sprintf("%s/%s", "pvc", old_pvcs.Items[0].Name)
	}
	return nil
}

func (r *RedisClusterBackupKubeClient) EnsureBackupJobCreated(ctx context.Context, backup *redisbackupv1.RedisClusterBackup, labels map[string]string, ownerRefs []metav1.OwnerReference) error {
	if backup.Status.JobName != "" {
		return nil
	}
	cluster, err := r.K8SService.GetDistributedRedisCluster(ctx, backup.Namespace, backup.Spec.Source.RedisClusterName)

	if err != nil {
		return nil
	}
	err = cluster.Init()
	if err != nil {
		r.logger.Error(err, "init cluster error")
	}
	job := &batchv1.Job{}
	if backup.Spec.Target.S3Option.S3Secret != "" {
		err = backup.Validate()
		if err != nil {
			return err
		}
		config_map := generateBackupConfigMap(backup, labels, ownerRefs, cluster)
		err := r.K8SService.CreateIfNotExistsConfigMap(ctx, backup.Namespace, config_map)
		if err != nil {
			return err
		}
		job = generateBackupJobForS3(backup, labels, ownerRefs, cluster)
	} else if backup.Spec.Source.RedisClusterName != "" {
		err = backup.Validate()
		if err != nil {
			return err
		}
		if cluster.Spec.MasterSize != cluster.Status.NumberOfMaster {
			return fmt.Errorf("cluster %s is not ready", cluster.Name)
		}
		jobs, err := r.K8SService.ListJobsByLabel(ctx, backup.Namespace, labels)
		if err == nil && len(jobs.Items) > 0 {
			backup.Status.JobName = jobs.Items[0].Name
			backup.Status.Condition = redisbackupv1.RedisBackupRunning
			r.logger.Info("back up job is exists", "instance:", backup.Name)
			return nil
		} else if err != nil {
			r.logger.Info(err.Error())
		}
		job = generateBackupJob(backup, cluster, labels, ownerRefs)

	} else {
		return fmt.Errorf("backup source is not valid")
	}
	jobs, err := r.K8SService.ListJobsByLabel(ctx, backup.Namespace, labels)
	if err == nil && len(jobs.Items) > 0 {
		backup.Status.JobName = jobs.Items[0].Name
		backup.Status.Condition = redisbackupv1.RedisBackupRunning
		r.logger.Info("back up job is exists", "instance:", backup.Name)
		return nil
	} else if err != nil {
		r.logger.Info(err.Error())
	}

	err = r.K8SService.CreateIfNotExistsJob(ctx, backup.Namespace, job)
	if err != nil {
		return err
	}

	backup.Status.JobName = job.Name
	backup.Status.Condition = redisbackupv1.RedisBackupRunning

	return nil
}

func (r *RedisClusterBackupKubeClient) EnsureBackupCompleted(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error {
	if backup.Status.Condition != redisbackupv1.RedisBackupRunning {
		return nil
	}
	job, err := r.K8SService.GetJob(ctx, backup.Namespace, backup.Status.JobName)
	if err != nil {
		return err
	}
	if job.Status.Succeeded > 0 {
		backup.Status.Condition = redisbackupv1.RedisBackupComplete
		backup.Status.StartTime = job.Status.StartTime
		backup.Status.CompletionTime = job.Status.CompletionTime
		return nil
	}
	if job.Status.Failed > *job.Spec.BackoffLimit {
		backup.Status.Condition = redisbackupv1.RedisBackupFailed
		if len(job.Status.Conditions) > 0 {
			backup.Status.Message = job.Status.Conditions[0].Message
		} else {
			backup.Status.Message = "Unknown"
		}
		return nil
	}
	// running
	backup.Status.LastCheckTime = &metav1.Time{Time: metav1.Now().Time}
	return nil
}

func (r *RedisClusterBackupKubeClient) UpdateBackup(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error {
	return r.K8SService.UpdateRedisClusterBackup(ctx, backup)
}

func (r *RedisClusterBackupKubeClient) UpdateBackupStatus(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error {
	return r.K8SService.UpdateRedisClusterBackupStatus(ctx, backup)
}

func (r *RedisClusterBackupKubeClient) EnsureRoleReady(ctx context.Context, backup *redisbackupv1.RedisClusterBackup) error {
	// check sa
	_, err := r.K8SService.GetServiceAccount(context.TODO(), backup.Namespace, util.RedisBackupServiceAccountName)
	if err != nil {
		if errors.IsNotFound(err) {
			err := r.K8SService.CreateServiceAccount(context.TODO(), backup.Namespace, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RedisBackupServiceAccountName,
					Namespace: backup.Namespace,
				},
			})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	// check role
	_, err = r.K8SService.GetRole(context.TODO(), backup.Namespace, util.RedisBackupRoleName)
	if err != nil {
		if errors.IsNotFound(err) {
			err := r.K8SService.CreateRole(context.TODO(), backup.Namespace, &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RedisBackupRoleName,
					Namespace: backup.Namespace,
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{corev1.GroupName},
						Resources: []string{"pods", "pods/exec"},
						Verbs:     []string{"*"},
					},
					{
						APIGroups: []string{redisbackupv1.GroupVersion.Group},
						Resources: []string{"*"},
						Verbs:     []string{"*"},
					},
				},
			})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// check role binding
	_, err = r.K8SService.GetRoleBinding(context.TODO(), backup.Namespace, util.RedisBackupRoleBindingName)
	if err != nil {
		if errors.IsNotFound(err) {
			err := r.K8SService.CreateRoleBinding(context.TODO(), backup.Namespace, &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RedisBackupRoleBindingName,
					Namespace: backup.Namespace,
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "Role",
					Name:     util.RedisBackupRoleName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      util.RedisBackupServiceAccountName,
						Namespace: backup.Namespace,
					},
				},
			})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}
