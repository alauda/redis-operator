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
	"path"

	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/builder/clusterbuilder"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/samber/lo"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

const (
	SentinelConfigVolumeName      = "sentinel-config"
	SentinelConfigVolumeMountPath = "/conf"
	RedisTLSVolumeName            = "redis-tls"
	RedisTLSVolumeMountPath       = "/tls"
	RedisDataVolumeName           = "data"
	RedisDataVolumeMountPath      = "/data"
	RedisAuthName                 = "redis-auth"
	RedisAuthMountPath            = "/account"
	RedisOptName                  = "redis-opt"
	RedisOptMountPath             = "/opt"
	OperatorUsername              = "OPERATOR_USERNAME"
	OperatorSecretName            = "OPERATOR_SECRET_NAME"
	SentinelContainerName         = "sentinel"
	SentinelContainerPortName     = "sentinel"
	graceTime                     = 30
)

func NewSentinelStatefulset(sen types.RedisSentinelInstance, selectors map[string]string) *appv1.StatefulSet {
	var (
		inst           = sen.Definition()
		passwordSecret = inst.Spec.PasswordSecret
	)
	if len(selectors) == 0 {
		selectors = GenerateSelectorLabels(RedisArchRoleSEN, inst.GetName())
	} else {
		selectors = lo.Assign(selectors, GenerateSelectorLabels(RedisArchRoleSEN, inst.GetName()))
	}
	labels := lo.Assign(GetCommonLabels(inst.GetName()), selectors)

	startArgs := []string{"sh", "/opt/run_sentinel.sh"}
	shutdownArgs := []string{"sh", "-c", "/opt/redis-tools sentinel shutdown &> /proc/1/fd/1"}
	volumes := getVolumes(inst, passwordSecret)
	volumeMounts := getRedisVolumeMounts(inst, passwordSecret)
	envs := createRedisContainerEnvs(inst)

	localhost := "127.0.0.1"
	if inst.Spec.Expose.IPFamilyPrefer == corev1.IPv6Protocol {
		localhost = "::1"
	}
	ss := &appv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GetSentinelStatefulSetName(inst.GetName()),
			Namespace:       inst.GetNamespace(),
			Labels:          labels,
			OwnerReferences: util.BuildOwnerReferences(inst),
		},
		Spec: appv1.StatefulSetSpec{
			ServiceName:         GetSentinelHeadlessServiceName(inst.GetName()),
			Replicas:            &inst.Spec.Replicas,
			PodManagementPolicy: appv1.ParallelPodManagement,
			UpdateStrategy: appv1.StatefulSetUpdateStrategy{
				Type: appv1.RollingUpdateStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selectors,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: inst.Spec.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					HostAliases: []corev1.HostAlias{
						{IP: localhost, Hostnames: []string{config.LocalInjectName}},
					},
					Containers: []corev1.Container{
						{
							Name:            SentinelContainerName,
							Command:         startArgs,
							Image:           inst.Spec.Image,
							ImagePullPolicy: builder.GetPullPolicy(inst.Spec.ImagePullPolicy),
							Env:             envs,
							Ports: []corev1.ContainerPort{
								{
									Name:          SentinelContainerPortName,
									ContainerPort: 26379,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							StartupProbe: &corev1.Probe{
								InitialDelaySeconds: 3,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(26379),
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								FailureThreshold:    5,
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(26379),
									},
								},
							},
							Resources: inst.Spec.Resources,
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: shutdownArgs,
									},
								},
							},
							SecurityContext: &corev1.SecurityContext{
								ReadOnlyRootFilesystem: pointer.Bool(true),
							},
							VolumeMounts: volumeMounts,
						},
					},
					ImagePullSecrets:              inst.Spec.ImagePullSecrets,
					SecurityContext:               builder.GetPodSecurityContext(inst.Spec.SecurityContext),
					ServiceAccountName:            clusterbuilder.RedisInstanceServiceAccountName,
					Affinity:                      getAffinity(inst.Spec.Affinity, selectors),
					Tolerations:                   inst.Spec.Tolerations,
					NodeSelector:                  inst.Spec.NodeSelector,
					Volumes:                       volumes,
					TerminationGracePeriodSeconds: pointer.Int64(graceTime),
				},
			},
		},
	}

	ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, *buildInitContainer(inst, nil))
	ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, *buildAgentContainer(inst, envs))

	if inst.Spec.Expose.ServiceType == corev1.ServiceTypeNodePort ||
		inst.Spec.Expose.ServiceType == corev1.ServiceTypeLoadBalancer {

		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, buildExposeContainer(inst))
	}
	return ss
}

func buildExposeContainer(inst *v1.RedisSentinel) corev1.Container {
	container := corev1.Container{
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
		Name:            "expose",
		Image:           config.GetRedisToolsImage(inst),
		ImagePullPolicy: builder.GetPullPolicy(inst.Spec.ImagePullPolicy),
		VolumeMounts: []corev1.VolumeMount{
			{Name: RedisDataVolumeName, MountPath: RedisDataVolumeMountPath},
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
				Name: "NAMESPACE",
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
				Name:  "IP_FAMILY_PREFER",
				Value: string(inst.Spec.Expose.IPFamilyPrefer),
			},
			{
				Name:  "SERVICE_TYPE",
				Value: string(inst.Spec.Expose.ServiceType),
			},
		},
		Command:         []string{"/opt/redis-tools", "sentinel", "expose"},
		SecurityContext: builder.GetSecurityContext(inst.Spec.SecurityContext),
	}
	return container
}

func buildInitContainer(inst *v1.RedisSentinel, _ []corev1.EnvVar) *corev1.Container {
	image := config.GetRedisToolsImage(inst)
	if image == "" {
		return nil
	}

	return &corev1.Container{
		Name:            "init",
		Image:           image,
		ImagePullPolicy: util.GetPullPolicy(inst.Spec.ImagePullPolicy),
		Command:         []string{"sh", "/opt/init_sentinel.sh"},
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
			{Name: RedisOptName, MountPath: path.Join("/mnt", RedisOptMountPath)},
		},
	}
}

func buildAgentContainer(inst *v1.RedisSentinel, envs []corev1.EnvVar) *corev1.Container {
	image := config.GetRedisToolsImage(inst)
	if image == "" {
		return nil
	}
	container := corev1.Container{
		Name:            "agent",
		Image:           image,
		ImagePullPolicy: util.GetPullPolicy(inst.Spec.ImagePullPolicy),
		Env:             envs,
		Command:         []string{"/opt/redis-tools", "sentinel", "agent"},
		SecurityContext: builder.GetSecurityContext(inst.Spec.SecurityContext),
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
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      RedisDataVolumeName,
				MountPath: RedisDataVolumeMountPath,
			},
		},
	}

	if inst.Spec.PasswordSecret != "" {
		vol := corev1.VolumeMount{
			Name:      RedisAuthName,
			MountPath: RedisAuthMountPath,
		}
		container.VolumeMounts = append(container.VolumeMounts, vol)
	}
	if inst.Spec.EnableTLS {
		vol := corev1.VolumeMount{
			Name:      RedisTLSVolumeName,
			MountPath: RedisTLSVolumeMountPath,
		}
		container.VolumeMounts = append(container.VolumeMounts, vol)
	}
	return &container
}

func createRedisContainerEnvs(inst *v1.RedisSentinel) []corev1.EnvVar {
	redisEnvs := []corev1.EnvVar{
		{
			Name: "NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: "POD_UID",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.uid",
				},
			},
		},
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
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name:  OperatorSecretName,
			Value: inst.Spec.PasswordSecret,
		},
		{
			Name:  "TLS_ENABLED",
			Value: fmt.Sprintf("%t", inst.Spec.EnableTLS),
		},
		{
			Name:  "SERVICE_TYPE",
			Value: string(inst.Spec.Expose.ServiceType),
		},
		{
			Name:  "IP_FAMILY_PREFER",
			Value: string(inst.Spec.Expose.IPFamilyPrefer),
		},
	}
	return redisEnvs
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
						TopologyKey: builder.HostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		},
	}
}

func getRedisVolumeMounts(inst *v1.RedisSentinel, secretName string) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      SentinelConfigVolumeName,
			MountPath: SentinelConfigVolumeMountPath,
		},
		{
			Name:      RedisDataVolumeName,
			MountPath: RedisDataVolumeMountPath,
		},
		{
			Name:      RedisOptName,
			MountPath: RedisOptMountPath,
		},
	}

	if inst.Spec.EnableTLS {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      RedisTLSVolumeName,
			MountPath: RedisTLSVolumeMountPath,
		})
	}
	if secretName != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      RedisAuthName,
			MountPath: RedisAuthMountPath,
		})
	}
	return volumeMounts
}

func getVolumes(inst *v1.RedisSentinel, secretName string) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: SentinelConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: GetSentinelConfigMapName(inst.GetName()),
					},
					DefaultMode: pointer.Int32(0400),
				},
			},
		},
		{
			Name: RedisDataVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: RedisOptName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	if inst.Spec.EnableTLS {
		volumes = append(volumes, corev1.Volume{
			Name: RedisTLSVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  builder.GetRedisSSLSecretName(inst.Name),
					DefaultMode: pointer.Int32(0400),
				},
			},
		})
	}
	if secretName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: RedisAuthName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: pointer.Int32(0400),
				},
			},
		})
	}
	return volumes
}
