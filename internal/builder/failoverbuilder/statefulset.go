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

package failoverbuilder

import (
	"fmt"
	"net"
	"path"
	"strconv"
	"strings"

	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/builder/clusterbuilder"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/samber/lo"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

const (
	redisStorageVolumeName       = "redis-data"
	exporterContainerName        = "redis-exporter"
	graceTime                    = 30
	PasswordENV                  = "REDIS_PASSWORD"
	redisConfigurationVolumeName = "redis-config"
	RedisTmpVolumeName           = "redis-tmp"
	RedisExporterTempVolumeName  = "exporter-temp"
	RedisTLSVolumeName           = "redis-tls"
	redisAuthName                = "redis-auth"
	redisStandaloneVolumeName    = "redis-standalone"
	redisOptName                 = "redis-opt"
	OperatorUsername             = "OPERATOR_USERNAME"
	OperatorSecretName           = "OPERATOR_SECRET_NAME"
	MonitorOperatorSecretName    = "MONITOR_OPERATOR_SECRET_NAME"
	ServerContainerName          = "redis"
)

func GetRedisRWServiceName(failoverName string) string {
	return fmt.Sprintf("rfr-%s-read-write", failoverName)
}

func GetRedisROServiceName(failoverName string) string {
	return fmt.Sprintf("rfr-%s-read-only", failoverName)
}

func GenerateRedisStatefulSet(inst types.RedisFailoverInstance, selectors map[string]string, isAllACLSupported bool) *appv1.StatefulSet {

	var (
		rf = inst.Definition()

		users            = inst.Users()
		opUser           = users.GetOpUser()
		aclConfigMapName = GenerateFailoverACLConfigMapName(rf.Name)
	)
	if opUser.Role == user.RoleOperator && !isAllACLSupported {
		opUser = users.GetDefaultUser()
	}
	if !inst.Version().IsACLSupported() {
		aclConfigMapName = ""
	}

	if len(selectors) == 0 {
		selectors = lo.Assign(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleRedis, rf.Name))
	} else {
		selectors = lo.Assign(selectors, GenerateSelectorLabels(RedisArchRoleRedis, rf.Name))
	}
	labels := lo.Assign(GetCommonLabels(rf.Name), GenerateSelectorLabels(RedisArchRoleRedis, rf.Name), selectors)

	secretName := rf.Spec.Auth.SecretPath
	if opUser.GetPassword() != nil {
		secretName = opUser.GetPassword().SecretName
	}
	redisCommand := getRedisCommand(rf)
	volumeMounts := getRedisVolumeMounts(rf, secretName)
	volumes := getRedisVolumes(inst, rf, secretName)

	localhost := "127.0.0.1"
	if rf.Spec.Redis.Expose.IPFamilyPrefer == corev1.IPv6Protocol {
		localhost = "::1"
	}
	ss := &appv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GetFailoverStatefulSetName(rf.Name),
			Namespace:       rf.Namespace,
			Labels:          labels,
			OwnerReferences: util.BuildOwnerReferences(rf),
		},
		Spec: appv1.StatefulSetSpec{
			ServiceName:         GetFailoverStatefulSetName(rf.Name),
			Replicas:            &rf.Spec.Redis.Replicas,
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
					Annotations: rf.Spec.Redis.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					HostAliases: []corev1.HostAlias{
						{IP: localhost, Hostnames: []string{config.LocalInjectName}},
					},
					ServiceAccountName:            clusterbuilder.RedisInstanceServiceAccountName,
					Affinity:                      getAffinity(rf.Spec.Redis.Affinity, selectors),
					Tolerations:                   rf.Spec.Redis.Tolerations,
					NodeSelector:                  rf.Spec.Redis.NodeSelector,
					SecurityContext:               builder.GetPodSecurityContext(rf.Spec.Redis.SecurityContext),
					ImagePullSecrets:              rf.Spec.Redis.ImagePullSecrets,
					TerminationGracePeriodSeconds: pointer.Int64(clusterbuilder.DefaultTerminationGracePeriodSeconds),
					Containers: []corev1.Container{
						{
							Name:            "redis",
							Image:           rf.Spec.Redis.Image,
							ImagePullPolicy: builder.GetPullPolicy(rf.Spec.Redis.ImagePullPolicy),
							Env:             createRedisContainerEnvs(inst, opUser, aclConfigMapName),
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
								InitialDelaySeconds: graceTime,
								TimeoutSeconds:      5,
								FailureThreshold:    10,
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6379),
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 1,
								TimeoutSeconds:      5,
								FailureThreshold:    5,
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6379),
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								FailureThreshold:    5,
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6379),
									},
								},
							},
							Resources: rf.Spec.Redis.Resources,
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/opt/redis-tools failover shutdown &> /proc/1/fd/1"},
									},
								},
							},
							SecurityContext: builder.GetSecurityContext(rf.Spec.Redis.SecurityContext),
						},
					},
					InitContainers: []corev1.Container{
						createExposeContainer(rf),
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
			pvc.OwnerReferences = util.BuildOwnerReferences(rf)
		}
		if pvc.Name == "" {
			pvc.Name = getRedisDataVolumeName(rf)
		}
		ss.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{*pvc}
	}

	if inst.IsStandalone() && NeedStandaloneInit(rf) {
		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, createStandaloneInitContainer(rf))
	}

	if rf.Spec.Redis.Exporter.Enabled {
		defaultAnnotations := map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   "http",
			"prometheus.io/path":   "/metrics",
		}
		ss.Spec.Template.Annotations = lo.Assign(ss.Spec.Template.Annotations, defaultAnnotations)

		exporter := createRedisExporterContainer(rf, opUser)
		ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, exporter)
	}
	return ss
}

func createExposeContainer(rf *v1.RedisFailover) corev1.Container {
	image := config.GetRedisToolsImage(rf)
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
		Name:            "init",
		Image:           image,
		ImagePullPolicy: builder.GetPullPolicy(rf.Spec.Redis.ImagePullPolicy),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      redisOptName,
				MountPath: "/mnt/opt/",
			},
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
				Name: "NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name:  "SENTINEL_ANNOUNCE_PATH",
				Value: "/data/announce.conf",
			},
			{
				Name:  "IP_FAMILY_PREFER",
				Value: string(rf.Spec.Redis.Expose.IPFamilyPrefer),
			},
			{
				Name:  "SERVICE_TYPE",
				Value: string(rf.Spec.Redis.Expose.ServiceType),
			},
		},
		Command:         []string{"sh", "/opt/init_failover.sh"},
		SecurityContext: builder.GetSecurityContext(rf.Spec.Redis.SecurityContext),
	}
	return container
}

func createRedisContainerEnvs(inst types.RedisFailoverInstance, opUser *user.User, aclConfigMapName string) []corev1.EnvVar {
	rf := inst.Definition()

	var monitorUri string
	if inst.Monitor().Policy() == v1.SentinelFailoverPolicy {
		var sentinelNodes []string
		for _, node := range rf.Status.Monitor.Nodes {
			sentinelNodes = append(sentinelNodes, net.JoinHostPort(node.IP, strconv.Itoa(int(node.Port))))
		}
		monitorUri = fmt.Sprintf("sentinel://%s", strings.Join(sentinelNodes, ","))
	}

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
			Name:  "SERVICE_NAME",
			Value: GetFailoverStatefulSetName(rf.Name),
		},
		{
			Name: "ACL_ENABLED",
			// isAllACLSupported used to make sure the account is consistent with eachother
			Value: fmt.Sprintf("%t", opUser.Role == user.RoleOperator),
		},
		{
			Name:  "ACL_CONFIGMAP_NAME",
			Value: aclConfigMapName,
		},
		{
			Name:  OperatorUsername,
			Value: opUser.Name,
		},
		{
			Name:  OperatorSecretName,
			Value: opUser.GetPassword().GetSecretName(),
		},
		{
			Name:  "TLS_ENABLED",
			Value: fmt.Sprintf("%t", rf.Spec.Redis.EnableTLS),
		},
		{
			Name:  "PERSISTENT_ENABLED",
			Value: fmt.Sprintf("%t", rf.Spec.Redis.Storage.PersistentVolumeClaim != nil),
		},
		{
			Name:  "SERVICE_TYPE",
			Value: string(rf.Spec.Redis.Expose.ServiceType),
		},
		{
			Name:  "IP_FAMILY_PREFER",
			Value: string(rf.Spec.Redis.Expose.IPFamilyPrefer),
		},
		{
			Name:  "MONITOR_POLICY",
			Value: string(inst.Monitor().Policy()),
		},
		{
			Name:  "MONITOR_URI",
			Value: monitorUri,
		},
		{
			Name:  MonitorOperatorSecretName,
			Value: rf.Status.Monitor.PasswordSecret,
		},
	}
	return redisEnvs
}

func createStandaloneInitContainer(rf *v1.RedisFailover) corev1.Container {
	tmpPath := "/tmp-data"
	filepath := rf.Annotations[AnnotationStandaloneLoadFilePath]
	targetFile := ""
	if strings.HasSuffix(filepath, ".rdb") {
		targetFile = "/data/dump.rdb"
	} else {
		targetFile = "/data/appendonly.aof"
	}

	filepath = path.Join(tmpPath, filepath)
	command := fmt.Sprintf("if [ -e '%s' ]; then", targetFile) + "\n" +
		"echo 'redis storage file exist,skip' \n" +
		"else \n" +
		fmt.Sprintf("echo 'copy redis storage file' && cp %s %s ", filepath, targetFile) +
		fmt.Sprintf("&& chown 999:1000 %s ", targetFile) +
		fmt.Sprintf("&& chmod 644 %s \n", targetFile) +
		"fi"

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
		Name:            "standalone-pod",
		Image:           config.GetRedisToolsImage(rf),
		ImagePullPolicy: builder.GetPullPolicy(rf.Spec.Redis.ImagePullPolicy),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      getRedisDataVolumeName(rf),
				MountPath: "/data",
			},
			{
				Name:      redisStandaloneVolumeName,
				MountPath: tmpPath,
			},
		},
		Command: []string{"sh", "-c", command},
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.Bool(false),
		},
	}

	if rf.Annotations[AnnotationStandaloneInitStorage] == "hostpath" {
		container.SecurityContext = &corev1.SecurityContext{
			Privileged:   pointer.Bool(false),
			RunAsGroup:   pointer.Int64(0),
			RunAsUser:    pointer.Int64(0),
			RunAsNonRoot: pointer.Bool(false),
		}
	}
	return container
}

func createRedisExporterContainer(rf *v1.RedisFailover, opUser *user.User) corev1.Container {
	var (
		username string
		secret   string
	)
	if opUser != nil {
		username = opUser.Name
		secret = opUser.GetPassword().GetSecretName()
		if username == "default" {
			username = ""
		}
	}

	const DefaultPasswordFile = "/tmp/passwords.json"
	entrypoint := fmt.Sprintf(`
if [ -f /account/password ]; then
    echo "{\"${REDIS_ADDR}\": \"$(cat /account/password)\"}" > %s
fi
/redis_exporter`, DefaultPasswordFile)

	container := corev1.Container{
		Name:            exporterContainerName,
		Command:         []string{"/bin/sh", "-c", entrypoint},
		Image:           rf.Spec.Redis.Exporter.Image,
		ImagePullPolicy: builder.GetPullPolicy(rf.Spec.Redis.Exporter.ImagePullPolicy),
		Env: []corev1.EnvVar{
			{
				Name: "REDIS_ALIAS",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name:  "REDIS_USER",
				Value: username,
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

	if secret != "" {
		container.VolumeMounts = append(container.VolumeMounts,
			corev1.VolumeMount{Name: redisAuthName, MountPath: "/account"},
			corev1.VolumeMount{Name: RedisExporterTempVolumeName, MountPath: "/tmp"},
		)
		container.Env = append(container.Env, corev1.EnvVar{Name: "REDIS_PASSWORD_FILE", Value: DefaultPasswordFile})
	}

	if rf.Spec.Redis.EnableTLS {
		container.VolumeMounts = append(container.VolumeMounts,
			corev1.VolumeMount{Name: RedisTLSVolumeName, MountPath: "/tls"},
		)
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
				Value: fmt.Sprintf("rediss://%s:6379", config.LocalInjectName),
			},
		}...)
	} else {
		container.Env = append(container.Env, []corev1.EnvVar{
			{Name: "REDIS_ADDR",
				Value: fmt.Sprintf("redis://%s:6379", config.LocalInjectName)},
		}...)
	}
	return container
}

func getRedisCommand(rf *v1.RedisFailover) []string {
	cmds := []string{"sh", "/opt/run_failover.sh"}
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

func getRedisVolumeMounts(rf *v1.RedisFailover, secret string) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      redisConfigurationVolumeName,
			MountPath: "/redis",
		},
		{
			Name:      getRedisDataVolumeName(rf),
			MountPath: "/data",
		},
		{
			Name:      RedisTmpVolumeName,
			MountPath: "/tmp",
		},
		{
			Name:      redisOptName,
			MountPath: "/opt",
		},
	}

	if rf.Spec.Redis.EnableTLS {
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
	return volumeMounts
}

func getRedisDataVolumeName(rf *v1.RedisFailover) string {
	switch {
	case rf.Spec.Redis.Storage.PersistentVolumeClaim != nil:
		if rf.Spec.Redis.Storage.PersistentVolumeClaim.ObjectMeta.Name == "" {
			return redisStorageVolumeName
		}
		return rf.Spec.Redis.Storage.PersistentVolumeClaim.ObjectMeta.Name
	case rf.Spec.Redis.Storage.EmptyDir != nil:
		return redisStorageVolumeName
	default:
		return redisStorageVolumeName
	}
}

func getRedisVolumes(inst types.RedisFailoverInstance, rf *v1.RedisFailover, secretName string) []corev1.Volume {
	executeMode := int32(0400)
	configname := GetRedisConfigMapName(rf)
	volumes := []corev1.Volume{
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
		{
			Name: redisOptName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if dataVolume := getRedisDataVolume(rf); dataVolume != nil {
		volumes = append(volumes, *dataVolume)
	}
	if rf.Spec.Redis.EnableTLS {
		volumes = append(volumes, corev1.Volume{
			Name: RedisTLSVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: builder.GetRedisSSLSecretName(rf.Name),
				},
			},
		})
	}
	if secretName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: redisAuthName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
	}
	if rf.Spec.Redis.Exporter.Enabled {
		volumes = append(volumes, corev1.Volume{
			Name: RedisExporterTempVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: resource.NewQuantity(1<<20, resource.BinarySI), //1Mi
				},
			},
		})
	}

	if inst.IsStandalone() && NeedStandaloneInit(rf) {
		if rf.Annotations[AnnotationStandaloneInitStorage] == "hostpath" {
			// hostpath
			if rf.Annotations[AnnotationStandaloneInitHostPath] != "" {
				hostpathType := corev1.HostPathDirectory
				volumes = append(volumes, corev1.Volume{
					Name: redisStandaloneVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: rf.Annotations[AnnotationStandaloneInitHostPath],
							Type: &hostpathType,
						},
					},
				})
			}
		} else {
			// pvc
			if rf.Annotations[AnnotationStandaloneInitPvcName] != "" {
				volumes = append(volumes, corev1.Volume{
					Name: redisStandaloneVolumeName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: rf.Annotations[AnnotationStandaloneInitPvcName],
						},
					},
				})
			}
		}
	}
	return volumes
}

func getRedisDataVolume(rf *v1.RedisFailover) *corev1.Volume {
	// This will find the volumed desired by the user. If no volume defined
	// an EmptyDir will be used by default
	switch {
	case rf.Spec.Redis.Storage.PersistentVolumeClaim != nil:
		return nil
	case rf.Spec.Redis.Storage.EmptyDir != nil:
		return &corev1.Volume{
			Name: redisStorageVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: rf.Spec.Redis.Storage.EmptyDir,
			},
		}
	default:
		return &corev1.Volume{
			Name: redisStorageVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
	}
}
