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

package sentinelbuilder

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/utils/pointer"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GenerateCronJobName(redisName, scheduleName string) string {
	return fmt.Sprintf("%s-%s", redisName, scheduleName)
}

func NewRedisFailoverBackupCronJobFromCR(schedule v1alpha1.Schedule, rf *databasesv1.RedisFailover, selectors map[string]string) *batchv1.CronJob {
	image := config.GetDefaultBackupImage()

	if rf.Spec.Redis.Backup.Image != "" {
		image = rf.Spec.Redis.Backup.Image
	}
	labels := MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(util.RedisBackupRoleName, rf.Name))
	job := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateCronJobName(rf.Name, schedule.Name),
			Namespace:       rf.Namespace,
			Labels:          labels,
			OwnerReferences: util.BuildOwnerReferences(rf),
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   schedule.Schedule,
			SuccessfulJobsHistoryLimit: &schedule.Keep,
			FailedJobsHistoryLimit:     &schedule.Keep,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: pointer.Int32(0),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: clusterbuilder.RedisInstanceServiceAccountName,
							RestartPolicy:      corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
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
									Name:            "backup-schedule",
									Image:           image,
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
											Name:  "REDIS_FAILOVER_NAME",
											Value: rf.Name,
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
				Name:  "REDIS_NAME",
				Value: rf.Name,
			},
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
