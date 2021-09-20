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
	"fmt"

	redisfailoverv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	redisbackupv1 "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/util"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

func generateBackupConfigMap(backup *redisbackupv1.RedisBackup, labels map[string]string, ownerRefs []metav1.OwnerReference,
	rf *redisfailoverv1.RedisFailover) *corev1.ConfigMap {
	name := generatorJobConfigMapName(backup)
	namespace := backup.Namespace
	appendCommands := ""
	if rf.Spec.EnableTLS {
		appendCommands += util.GenerateRedisTLSOptions()
	}
	script_content := `
set -e
master_ip=$(redis-cli -h %s -p %d  %s  --csv SENTINEL get-master-addr-by-name %s | tr ',' ' ' | tr -d '\"' |cut -d' ' -f1)
master_port=$(redis-cli -h %s -p %d  %s  --csv SENTINEL get-master-addr-by-name %s | tr ',' ' ' | tr -d '\"' |cut -d' ' -f2)

if [ -f /backup/dump.rdb ]
then
	echo 'dump.rdb exist, skip backup data'
	exit 0
fi

if [ -f /redis-password/password ]
then
	export REDIS_PASSWORD=$(cat /redis-password/password)
fi

if [ ! -z "${REDIS_PASSWORD}" ]; then
	redis-cli -a "${REDIS_PASSWORD}" -h "$master_ip" -p "$master_port" %s --rdb /backup/dump.rdb
	echo "down finish"
else 
	redis-cli -h "$master_ip" -p "$master_port" %s --rdb /backup/dump.rdb
	echo "down finish"
fi 

if [ ! -z "${S3_ENDPOINT}" ]; then 
	/opt/redis-tools backup push
	echo "push s3 success"
fi
`
	n := rand.Int() % len(backup.Spec.Source.Endpoint)
	ipPort := backup.Spec.Source.Endpoint[n]
	masterName := "mymaster"
	if ipPort.MasterName != "" {
		masterName = ipPort.MasterName
	}

	Content := fmt.Sprintf(script_content, ipPort.Address, ipPort.Port, appendCommands, masterName, ipPort.Address, ipPort.Port, appendCommands, masterName, appendCommands, appendCommands)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Data: map[string]string{
			"backup.sh": Content,
		},
	}
}

func generateBackupJobForS3(backup *redisbackupv1.RedisBackup, labels map[string]string, ownerRefs []metav1.OwnerReference,
	rf *redisfailoverv1.RedisFailover) *batchv1.Job {
	name := generatorJobNameForS3(backup)
	namespace := backup.Namespace

	image := backup.Spec.Image

	executeMode := int32(0744)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: backup.Spec.BackoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ActiveDeadlineSeconds: backup.Spec.ActiveDeadlineSeconds,
					PriorityClassName:     backup.Spec.PriorityClassName,
					SecurityContext:       backup.Spec.SecurityContext,
					NodeSelector:          backup.Spec.NodeSelector,
					Tolerations:           backup.Spec.Tolerations,
					Affinity:              backup.Spec.Affinity,
					ServiceAccountName:    util.RedisBackupServiceAccountName,
					RestartPolicy:         corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "backup",
							Image:           image,
							ImagePullPolicy: "Always",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "backup-data",
									MountPath: "/backup",
								},
								{
									Name:      util.S3SecretVolumeName,
									MountPath: "/s3_secret",
									ReadOnly:  true,
								},
								{
									Name:      "script-data",
									MountPath: "/script",
								},
							},
							Command:   []string{"/bin/sh"},
							Args:      []string{"-c", "/script/backup.sh"},
							Resources: backup.Spec.Resources,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "script-data",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: generatorJobConfigMapName(backup),
									},
									DefaultMode: &executeMode,
								},
							},
						},
					},
				},
			},
		},
	}
	data_volume := corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
	if backup.Spec.Source.StorageClassName != "" {
		data_volume = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: util.GetClaimName(backup.Status.Destination),
			},
		}
	}
	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{Name: "backup-data", VolumeSource: data_volume})

	if rf.Spec.Auth.SecretPath != "" {
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "redis-password",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: rf.Spec.Auth.SecretPath},
			},
		})
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "redis-password",
			MountPath: "/redis-password",
			ReadOnly:  true,
		})
	}

	if backup.Spec.Target.S3Option.S3Secret != "" {
		s3secretVolumes := corev1.Volume{
			Name: util.S3SecretVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: backup.Spec.Target.S3Option.S3Secret},
			},
		}
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, s3secretVolumes)

		job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, []corev1.EnvVar{
			{Name: "S3_ENDPOINT", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: backup.Spec.Target.S3Option.S3Secret,
					},
					Key: config.S3_ENDPOINTURL,
				},
			}},
			{Name: "S3_REGION", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: backup.Spec.Target.S3Option.S3Secret,
					},
					Key: config.S3_REGION,
				},
			}},
			{Name: "DATA_DIR", Value: "/backup"},
			{Name: "S3_OBJECT_DIR", Value: backup.Spec.Target.S3Option.Dir},
			{Name: "S3_BUCKET_NAME", Value: backup.Spec.Target.S3Option.Bucket},
		}...)
	}

	SSLSecretName := util.GetRedisSSLSecretName(rf.Name)
	if backup.Spec.Source.SSLSecretName != "" {
		SSLSecretName = backup.Spec.Source.SSLSecretName
	}
	if rf.Spec.EnableTLS {
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "redis-tls",
			MountPath: "/tls",
		})
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "redis-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: SSLSecretName,
				},
			},
		})
	}

	return job
}

func generateBackupJob(backup *redisbackupv1.RedisBackup, labels map[string]string, ownerRefs []metav1.OwnerReference,
	rf *redisfailoverv1.RedisFailover) *batchv1.Job {
	name := generatorJobName(backup)
	namespace := backup.Namespace
	image := config.GetDefaultBackupImage()
	if backup.Spec.Image != "" {
		image = backup.Spec.Image
	}

	backoffLimit := int32(0)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: util.RedisBackupServiceAccountName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "backup",
							Image:           image,
							Resources:       backup.Spec.Resources,
							ImagePullPolicy: "Always",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "backup-data",
									MountPath: "/backup",
								},
							},
							Env: []corev1.EnvVar{
								{Name: "REDIS_NAME",
									Value: backup.Spec.Source.RedisFailoverName,
								},
							},

							Command: []string{"/bin/sh"},
							Args:    []string{"-c", fmt.Sprintf("/opt/redis-tools backup backup")},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "backup-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: util.GetClaimName(backup.Status.Destination),
								},
							},
						},
					},
				},
			},
		},
	}

	if rf.Spec.Auth.SecretPath != "" {
		job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: rf.Spec.Auth.SecretPath,
					},
					Key: config.RedisSecretPasswordKey,
				},
			},
		})
	}

	// append redis cli commands
	appendCommands := ""
	if rf.Spec.EnableTLS {
		appendCommands += util.GenerateRedisTLSOptions()
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "redis-tls",
			MountPath: "/tls",
		})
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "redis-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: util.GetRedisSSLSecretName(rf.Name),
				},
			},
		})
	}
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		appendCommands += " -h ::1 "
	}
	job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "APPEND_COMMANDS",
		Value: appendCommands,
	})
	return job
}

func generatorJobName(rb *redisbackupv1.RedisBackup) string {
	hashString := rand.String(5)
	return fmt.Sprintf("%s-%s", rb.Name, hashString)
}

func generatorPVC(backup *redisbackupv1.RedisBackup, labels map[string]string, ownerRefs []metav1.OwnerReference) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            generatorJobName(backup),
			Namespace:       backup.Namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: backup.Spec.Storage,
				},
			},
		},
	}
	if backup.Spec.Source.StorageClassName != "" {
		pvc.Spec.StorageClassName = &backup.Spec.Source.StorageClassName
	}
	return pvc
}

func generatorJobNameForS3(rb *redisbackupv1.RedisBackup) string {

	return fmt.Sprintf("%s-%s", "rb-job", rb.Name)
}

func generatorJobConfigMapName(rb *redisbackupv1.RedisBackup) string {
	return fmt.Sprintf("%s-%s", "rb", rb.Name)
}
