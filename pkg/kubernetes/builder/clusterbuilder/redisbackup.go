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

package clusterbuilder

import (
	"fmt"

	"github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/util"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func GenerateCronJobName(redisName, scheduleName string) string {
	return fmt.Sprintf("%s-%s", redisName, scheduleName)
}

func NewRedisClusterBackupCronJobFromCR(schedule v1alpha1.Schedule, cluster *v1alpha1.DistributedRedisCluster) *batchv1.CronJob {
	image := config.GetDefaultBackupImage()
	if cluster.Spec.Backup.Image != "" {
		image = cluster.Spec.Backup.Image
	}
	_labels := GetClusterLabels(cluster.Name, map[string]string{"redisclusterbackups.redis.middleware.alauda.io/instanceName": cluster.Name})
	delete(_labels, "redis.kun/name")

	job := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateCronJobName(cluster.Name, schedule.Name),
			Namespace:       cluster.Namespace,
			Labels:          _labels,
			OwnerReferences: util.BuildOwnerReferences(cluster),
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   schedule.Schedule,
			SuccessfulJobsHistoryLimit: &schedule.Keep,
			FailedJobsHistoryLimit:     &schedule.Keep,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: _labels,
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: pointer.Int32(0),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: _labels,
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: RedisInstanceServiceAccountName,
							RestartPolicy:      corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:  "backup-schedule",
									Image: image,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("200Mi"),
											corev1.ResourceCPU:    resource.MustParse("200m"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("500Mi"),
											corev1.ResourceCPU:    resource.MustParse("500m"),
										},
									},
									ImagePullPolicy: "Always",
									Command:         []string{"/bin/sh"},
									Args:            []string{"-c", "/opt/redis-tools backup schedule"},
									Env: []corev1.EnvVar{
										{
											Name: "BACKUP_JOB_NAME",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.name",
												},
											},
										},
										{
											Name: "BACKUP_JOB_UID",
											ValueFrom: &corev1.EnvVarSource{
												FieldRef: &corev1.ObjectFieldSelector{
													FieldPath: "metadata.uid",
												},
											},
										},
										{
											Name:  "BACKUP_IMAGE",
											Value: image,
										},
										{
											Name:  "REDIS_CLUSTER_NAME",
											Value: cluster.Name,
										},
										{
											Name:  "STORAGE_CLASS_NAME",
											Value: schedule.Storage.StorageClassName,
										},
										{
											Name:  "STORAGE_SIZE",
											Value: schedule.Storage.Size.String(),
										},
										{
											Name:  "SCHEDULE_NAME",
											Value: schedule.Name,
										},
									},
									SecurityContext: &corev1.SecurityContext{},
								},
							},
							SecurityContext: &corev1.PodSecurityContext{},
						},
					},
				},
			},
		},
	}
	if schedule.KeepAfterDeletion {
		job.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env = append(job.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "KEEP_AFTER_DELETION",
				Value: "true",
			})
	}
	if schedule.Target.S3Option.S3Secret != "" {
		job.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env = append(job.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "BACKOFF_LIMIT",
				Value: "6",
			},
			corev1.EnvVar{
				Name:  "S3_BUCKET_NAME",
				Value: schedule.Target.S3Option.Bucket,
			},
			corev1.EnvVar{
				Name:  "S3_SECRET",
				Value: schedule.Target.S3Option.S3Secret,
			},
		)
	}
	return job
}
