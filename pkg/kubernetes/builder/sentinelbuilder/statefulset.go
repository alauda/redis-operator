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
	"os"
	"path"
	"strconv"
	"strings"

	v1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	redisbackup "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

const (
	redisShutdownConfigurationVolumeName = "redis-shutdown-config"
	redisStorageVolumeName               = "redis-data"
	exporterContainerName                = "redis-exporter"
	graceTime                            = 30
	PasswordENV                          = "REDIS_PASSWORD"
	redisConfigurationVolumeName         = "redis-config"
	RedisTmpVolumeName                   = "redis-tmp"
	RedisTLSVolumeName                   = "redis-tls"
	LocalInjectName                      = "local.inject"
	redisAuthName                        = "redis-auth"
	redisOptName                         = "redis-opt"
	OperatorUsername                     = "OPERATOR_USERNAME"
	OperatorSecretName                   = "OPERATOR_SECRET_NAME"
	ServerContainerName                  = "redis"
	SentinelContainerName                = "sentinel"
	SentinelContainerPortName            = "sentinel"
)

func GetRedisRWServiceName(sentinelName string) string {
	return fmt.Sprintf("rfr-%s-read-write", sentinelName)
}

func GetRedisROServiceName(sentinelName string) string {
	return fmt.Sprintf("rfr-%s-read-only", sentinelName)
}

func GenerateRedisStatefulSet(rf *v1.RedisFailover, rb *redisbackup.RedisBackup, selectors map[string]string, acl string) *appv1.StatefulSet {
	name := GetSentinelStatefulSetName(rf.Name)
	namespace := rf.Namespace

	if len(selectors) == 0 {
		selectors = MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleRedis, rf.Name))
	} else {
		selectors = MergeMap(selectors, GenerateSelectorLabels(RedisArchRoleRedis, rf.Name))
	}
	labels := MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleRedis, rf.Name), selectors)

	redisCommand := getRedisCommand(rf)
	volumeMounts := getRedisVolumeMounts(rf, acl)
	secretName := rf.Spec.Auth.SecretPath
	if acl != "" {
		secretName = acl
	}
	volumes := getRedisVolumes(rf, secretName)
	probeArg := "redis-cli  %s"
	tlsOptions := ""
	if rf.Spec.EnableTLS {
		tlsOptions = util.GenerateRedisTLSOptions()
	}
	probeArg = fmt.Sprintf(probeArg, tlsOptions)
	if acl != "" {
		probeArg = fmt.Sprintf("%s -a $(cat /account/password) --user $(cat /account/username) -h %s ping", probeArg, LocalInjectName)
	} else if rf.Spec.Auth.SecretPath != "" {
		probeArg = fmt.Sprintf("%s -a $(cat /account/password) -h %s ping", probeArg, LocalInjectName)
	} else {
		probeArg = fmt.Sprintf("%s -h %s ping", probeArg, LocalInjectName)
	}
	localhost := "127.0.0.1"
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		localhost = "::1"
	}
	ss := &appv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
			Annotations:     util.GenerateRedisRebuildAnnotation(),
		},
		Spec: appv1.StatefulSetSpec{
			ServiceName: name,
			Replicas:    &rf.Spec.Redis.Replicas,
			UpdateStrategy: appv1.StatefulSetUpdateStrategy{
				Type: "RollingUpdate",
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selectors,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      selectors,
					Annotations: rf.Spec.Redis.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					HostAliases: []corev1.HostAlias{
						{
							IP:        localhost,
							Hostnames: []string{LocalInjectName},
						},
					},
					Affinity:         getAffinity(rf.Spec.Redis.Affinity, selectors),
					Tolerations:      rf.Spec.Redis.Tolerations,
					NodeSelector:     rf.Spec.Redis.NodeSelector,
					SecurityContext:  getSecurityContext(rf.Spec.Redis.SecurityContext),
					ImagePullSecrets: rf.Spec.Redis.ImagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:            "redis",
							Image:           rf.Spec.Redis.Image,
							ImagePullPolicy: pullPolicy(rf.Spec.Redis.ImagePullPolicy),
							Env: []corev1.EnvVar{
								{
									Name: "POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name: "POD_IPS",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIPs",
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: 6379,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: volumeMounts,
							Command:      redisCommand,
							StartupProbe: &corev1.Probe{
								InitialDelaySeconds: 5,
								FailureThreshold:    10,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"sh",
											"/redis-shutdown/start.sh",
										},
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: graceTime,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"sh",
											"-c",
											probeArg,
										},
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: graceTime,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"sh", "-c", probeArg},
									},
								},
							},
							Resources: rf.Spec.Redis.Resources,
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "/redis-shutdown/shutdown.sh"},
									},
								},
							},
							SecurityContext: &corev1.SecurityContext{
								ReadOnlyRootFilesystem: pointer.Bool(true),
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	if rf.Spec.Redis.Storage.PersistentVolumeClaim != nil {
		pvc := rf.Spec.Redis.Storage.PersistentVolumeClaim.DeepCopy()
		if len(pvc.Spec.AccessModes) == 0 {
			pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		}
		if !rf.Spec.Redis.Storage.KeepAfterDeletion {
			// Set an owner reference so the persistent volumes are deleted when the rc is
			pvc.OwnerReferences = GetOwnerReferenceForRedisFailover(rf)
		}
		if pvc.Name == "" {
			pvc.Name = getRedisDataVolumeName(rf)
		}
		ss.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			*pvc,
		}
	}
	ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  util.GetEnvSentinelHost(rf.Name),
		Value: util.GetSentinelName(rf),
	})
	ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  util.GetEnvSentinelPort(rf.Name),
		Value: "26379",
	})

	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "IP_FAMILY_PREFER",
			Value: string(rf.Spec.Redis.IPFamilyPrefer),
		})
	}
	if rf.Spec.Redis.Restore.BackupName != "" {
		restore := createRestoreContainer(rf)
		if rb.Spec.Target.S3Option.S3Secret != "" {
			restore = createRestoreContainerForS3(rf, rb)
		}
		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, restore)
		if rb.Spec.Target.S3Option.S3Secret == "" {
			backupVolumes := corev1.Volume{
				Name: util.RedisBackupVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: util.GetClaimName(rb.Status.Destination),
					},
				},
			}
			ss.Spec.Template.Spec.Volumes = append(ss.Spec.Template.Spec.Volumes, backupVolumes)
		} else {
			s3secretVolumes := corev1.Volume{
				Name: util.S3SecretVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: rb.Spec.Target.S3Option.S3Secret},
				},
			}
			ss.Spec.Template.Spec.Volumes = append(ss.Spec.Template.Spec.Volumes, s3secretVolumes)
		}
	}

	if rf.Spec.Expose.EnableNodePort || rf.Spec.Redis.IPFamilyPrefer != "" {
		expose := createExposeContainer(rf)
		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, expose)
		ss.Spec.Template.Spec.ServiceAccountName = clusterbuilder.RedisInstanceServiceAccountName
	}

	if acl != "" {
		ss.Spec.Template.Spec.ServiceAccountName = clusterbuilder.RedisInstanceServiceAccountName
		ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  OperatorSecretName,
			Value: GenerateSentinelACLOperatorSecretName(rf.Name),
		})
		ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  OperatorUsername,
			Value: user.DefaultOperatorUserName,
		})
		ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "ACL_CONFIGMAP_NAME",
			Value: GenerateSentinelACLConfigMapName(rf.Name),
		})
		ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		})

		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, createRedisToolInitContainer(rf))
	}

	if rf.Spec.Redis.Storage.PersistentVolumeClaim == nil {
		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, createAutoReplicaContainer(rf, acl))
	}

	if rf.Spec.Redis.Exporter.Enabled {
		defaultAnnotations := map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   "http",
			"prometheus.io/path":   "/metrics",
		}
		ss.Spec.Template.Annotations = util.MergeMap(ss.Spec.Template.Annotations, defaultAnnotations)

		exporter := createRedisExporterContainer(rf, acl)
		ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, exporter)
	}

	return ss
}

func createRedisToolInitContainer(rf *v1.RedisFailover) corev1.Container {
	image := config.GetRedisToolsImage()
	initContainer := corev1.Container{
		Name:            "redis-tools",
		Image:           image,
		ImagePullPolicy: corev1.PullAlways,
		Command:         []string{"sh", "-c", "cp /opt/* /mnt/opt/ && chmod 555 /mnt/opt/*"},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      redisOptName,
				MountPath: "/mnt/opt/",
			},
		},
	}

	return initContainer
}

func createExposeContainer(rf *v1.RedisFailover) corev1.Container {
	image := rf.Spec.Expose.ExposeImage
	if image == "" {
		image = config.GetRedisToolsImage()
	}
	privileged := false
	container := corev1.Container{
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		},
		Name:            "expose-pod",
		Image:           image,
		ImagePullPolicy: pullPolicy(rf.Spec.Redis.ImagePullPolicy),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      getRedisDataVolumeName(rf),
				MountPath: "/data",
			},
		},
		Env: []corev1.EnvVar{
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			}, {
				Name:  "SENTINEL_ANNOUNCE_PATH",
				Value: "/data/announce.conf",
			},
			{
				Name:  "NODEPORT_ENABLED",
				Value: strconv.FormatBool(rf.Spec.Expose.EnableNodePort),
			},
			{
				Name:  "IP_FAMILY",
				Value: string(rf.Spec.Redis.IPFamilyPrefer),
			},
		},
		Command:         []string{"/expose_pod"},
		SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
	}
	return container
}

func createRedisExporterContainer(rf *v1.RedisFailover, secret string) corev1.Container {
	container := corev1.Container{
		Name:            exporterContainerName,
		Image:           rf.Spec.Redis.Exporter.Image,
		ImagePullPolicy: pullPolicy(rf.Spec.Redis.Exporter.ImagePullPolicy),
		Env: []corev1.EnvVar{
			{
				Name: "REDIS_ALIAS",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 9121,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			ReadOnlyRootFilesystem: pointer.Bool(true),
		},
	}
	if rf.Spec.Auth.SecretPath != "" || secret != "" {
		//挂载 rf.Spec.Auth.SecretPath 到account
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      redisAuthName,
			MountPath: "/account",
		})
	}
	if secret != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "REDIS_USER",
			Value: "operator",
		},
		)
	}
	local_host := "127.0.0.1"
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		local_host = "[::1]"
	}
	if rf.Spec.EnableTLS {
		container.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      RedisTLSVolumeName,
				MountPath: "/tls",
			},
		}
		container.Env = append(container.Env, []corev1.EnvVar{
			{
				Name:  "REDIS_EXPORTER_TLS_CLIENT_KEY_FILE",
				Value: "/tls/tls.key",
			},
			{
				Name:  "REDIS_EXPORTER_TLS_CLIENT_CERT_FILE",
				Value: "/tls/tls.crt",
			},
			{
				Name:  "REDIS_EXPORTER_TLS_CA_CERT_FILE",
				Value: "/tls/ca.crt",
			},
			{
				Name:  "REDIS_EXPORTER_SKIP_TLS_VERIFICATION",
				Value: "true",
			},
			{
				Name:  "REDIS_ADDR",
				Value: fmt.Sprintf("redis://%s:6379", local_host),
			},
		}...)
	} else if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		container.Env = append(container.Env, []corev1.EnvVar{
			{Name: "REDIS_ADDR",
				Value: fmt.Sprintf("redis://%s:6379", local_host)},
		}...)
	}

	return container
}

func createAutoReplicaContainer(rf *v1.RedisFailover, secret string) corev1.Container {
	image := rf.Spec.Redis.Image
	privileged := false
	container := corev1.Container{
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		},
		Name:            "auto-replica",
		Image:           image,
		ImagePullPolicy: pullPolicy(rf.Spec.Redis.ImagePullPolicy),
		VolumeMounts:    getRedisVolumeMounts(rf, secret),
		Command:         []string{"sh", "/redis-shutdown/auto_replica.sh"},
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Env: []corev1.EnvVar{
			{
				Name:  util.GetEnvSentinelHost(rf.Name),
				Value: util.GetSentinelName(rf),
			},
			{
				Name:  util.GetEnvSentinelPort(rf.Name),
				Value: "26379",
			},
		},
	}
	return container
}

func getSecurityContext(secctx *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	// 999 is the default userid for redis offical docker image
	// 1000 is the default groupid for redis offical docker image
	userId, groupId := int64(999), int64(1000)
	if secctx == nil {
		secctx = &corev1.PodSecurityContext{
			RunAsUser:    &userId,
			RunAsGroup:   &groupId,
			FSGroup:      &groupId,
			RunAsNonRoot: pointer.Bool(true),
		}
	} else {
		if secctx.RunAsUser == nil {
			secctx.RunAsUser = &userId
		}
		if secctx.RunAsGroup == nil {
			secctx.RunAsGroup = &groupId
		}
		if secctx.FSGroup == nil {
			secctx.FSGroup = &groupId
		}
		if *secctx.RunAsUser != 0 {
			if secctx.RunAsNonRoot == nil {
				secctx.RunAsNonRoot = pointer.Bool(true)
			}
		} else {
			secctx.RunAsNonRoot = nil
		}
	}
	return secctx
}

func getRedisCommand(rf *v1.RedisFailover) []string {
	cmds := []string{
		"sh",
		"/redis/entrypoint.sh",
		"--tcp-keepalive 60",
	}
	if rf.Spec.EnableTLS {
		cmds = append(cmds, getRedisTLSCommand()...)
	}
	return cmds
}

func getAffinity(affinity *corev1.Affinity, labels map[string]string) *corev1.Affinity {
	if affinity != nil {
		return affinity
	}

	// Return a SOFT anti-affinity
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: util.HostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		},
	}
}

func getRedisTLSCommand() []string {
	return []string{
		"--tls-port",
		"6379",
		"--port",
		"0",
		"--tls-replication",
		"yes",
		"--tls-cert-file",
		"/tls/tls.crt",
		"--tls-key-file",
		"/tls/tls.key",
		"--tls-ca-cert-file",
		"/tls/ca.crt",
	}
}

func getRedisVolumeMounts(rf *v1.RedisFailover, secret string) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      redisConfigurationVolumeName,
			MountPath: "/redis",
		},
		{
			Name:      redisShutdownConfigurationVolumeName,
			MountPath: "/redis-shutdown",
		},
		{
			Name:      getRedisDataVolumeName(rf),
			MountPath: "/data",
		},
		{
			Name:      RedisTmpVolumeName,
			MountPath: "/tmp",
		},
	}
	if rf.Spec.EnableTLS {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      RedisTLSVolumeName,
			MountPath: "/tls",
		})
	}
	if rf.Spec.Auth.SecretPath != "" || secret != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      redisAuthName,
			MountPath: "/account",
		})
	}
	if secret != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      redisOptName,
			MountPath: "/opt",
		})
	}

	return volumeMounts
}

func getRedisDataVolumeName(rf *v1.RedisFailover) string {
	switch {
	case rf.Spec.Redis.Storage.PersistentVolumeClaim != nil:
		if rf.Spec.Redis.Storage.PersistentVolumeClaim.ObjectMeta.Name == "" {
			return redisStorageVolumeName
		}
		return rf.Spec.Redis.Storage.PersistentVolumeClaim.ObjectMeta.Name
	default:
		return redisStorageVolumeName
	}
}

func pullPolicy(specPolicy corev1.PullPolicy) corev1.PullPolicy {
	if specPolicy == "" {
		return corev1.PullAlways
	}
	return specPolicy
}

func getRedisVolumes(rf *v1.RedisFailover, secretName string) []corev1.Volume {
	shutdownConfigMapName := util.GetRedisShutdownConfigMapName(rf)

	executeMode := int32(0744)
	configname := GetRedisConfigMapName(rf)
	volumes := []corev1.Volume{
		{
			Name: redisShutdownConfigurationVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: shutdownConfigMapName,
					},
					DefaultMode: &executeMode,
				},
			},
		},
		{
			Name: redisConfigurationVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configname,
					},
					DefaultMode: &executeMode,
				},
			},
		},
		{
			Name: RedisTmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: resource.NewQuantity(1<<20, resource.BinarySI), //1Mi
				},
			},
		},
	}

	dataVolume := getRedisDataVolume(rf)
	if dataVolume != nil {
		volumes = append(volumes, *dataVolume)
	}
	if rf.Spec.EnableTLS {
		volumes = append(volumes, corev1.Volume{
			Name: RedisTLSVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: util.GetRedisSSLSecretName(rf.Name),
				},
			},
		})
	}
	if rf.Spec.Auth.SecretPath != "" || secretName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: redisAuthName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
		volumes = append(volumes, corev1.Volume{
			Name: redisOptName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	return volumes
}

func getRedisDataVolume(rf *v1.RedisFailover) *corev1.Volume {
	// This will find the volumed desired by the user. If no volume defined
	// an EmptyDir will be used by default
	switch {
	case rf.Spec.Redis.Storage.PersistentVolumeClaim != nil:
		return nil
	default:
		return &corev1.Volume{
			Name: redisStorageVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
	}
}

func createRestoreContainer(rf *v1.RedisFailover) corev1.Container {
	image := config.GetDefaultBackupImage()
	if rf.Spec.Redis.Restore.Image != "" {
		image = rf.Spec.Redis.Restore.Image
	}
	container := corev1.Container{
		Name:            util.RestoreContainerName,
		Image:           image,
		ImagePullPolicy: pullPolicy(rf.Spec.Redis.Restore.ImagePullPolicy),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      util.RedisBackupVolumeName,
				MountPath: "/backup",
			},
			{
				Name:      getRedisDataVolumeName(rf),
				MountPath: "/data",
			},
		},
		Command: []string{"/bin/sh"},
		Args:    []string{"-c", "/opt/redis-tools backup restore"},
	}
	return container
}

func createRestoreContainerForS3(rf *v1.RedisFailover, rb *redisbackup.RedisBackup) corev1.Container {
	image := rb.Spec.Image
	// 不使用redis-backup 恢复镜像
	if strings.Contains(image, "redis-backup:") {
		image = os.Getenv(config.GetDefaultBackupImage())
	}
	container := corev1.Container{
		Name:            util.RestoreContainerName,
		Image:           image,
		ImagePullPolicy: pullPolicy(rf.Spec.Redis.Restore.ImagePullPolicy),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      getRedisDataVolumeName(rf),
				MountPath: "/data",
			},
			{
				Name:      util.S3SecretVolumeName,
				MountPath: "/s3_secret",
				ReadOnly:  true,
			},
		},
		Env: []corev1.EnvVar{
			{Name: "RDB_CHECK", Value: "true"},
			{Name: "TARGET_FILE", Value: "/data/dump.rdb"},
			{Name: "S3_ENDPOINT",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: rb.Spec.Target.S3Option.S3Secret,
						},
						Key: config.S3_ENDPOINTURL,
					},
				},
			},
			{Name: "S3_REGION", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: rb.Spec.Target.S3Option.S3Secret,
					},
					Key: config.S3_REGION,
				},
			}},
			{Name: "S3_BUCKET_NAME", Value: rb.Spec.Target.S3Option.Bucket},
			{Name: "S3_OBJECT_NAME", Value: path.Join(rb.Spec.Target.S3Option.Dir, "dump.rdb")},
		},
		Command: []string{"/opt/redis-tools", "backup", "pull"},
	}
	return container
}
