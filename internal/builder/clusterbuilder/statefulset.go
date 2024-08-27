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
	"crypto/sha1" // #nosec
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	redisv1alpha1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/go-logr/logr"
)

const (
	DefaultRedisServerPort    = 6379
	DefaultRedisServerBusPort = 16379

	GenericKey       = "redis.kun"
	LabelClusterName = GenericKey + "/name"
	StatefulSetLabel = "statefulSet"

	hostnameTopologyKey = "kubernetes.io/hostname"

	// Container
	CheckContainerName         = "init"
	ServerContainerName        = "redis"
	ExporterContainerName      = "exporter"
	ConfigSyncContainerName    = "sidecar"
	RedisDataContainerPortName = "client"

	// Volume
	RedisStorageVolumeName          = "redis-data"
	RedisTempVolumeName             = "temp"
	RedisExporterTempVolumeName     = "exporter-temp"
	RedisOperatorPasswordVolumeName = "operator-password"
	ConfigVolumeName                = "conf"
	RedisTLSVolumeName              = "redis-tls"
	RedisOptVolumeName              = "redis-opt"
	// Mount path
	StorageVolumeMountPath          = "/data"
	OperatorPasswordVolumeMountPath = "/account"
	ConfigVolumeMountPath           = "/conf"
	TLSVolumeMountPath              = "/tls"
	RedisOptVolumeMountPath         = "/opt"
	RedisTmpVolumeMountPath         = "/tmp"

	// Env
	OperatorUsername   = "OPERATOR_USERNAME"
	OperatorSecretName = "OPERATOR_SECRET_NAME"

	PrometheusExporterPortNumber    = 9100
	PrometheusExporterTelemetryPath = "/metrics"
)

const (
	ProbeDelaySeconds                          = 30
	DefaultTerminationGracePeriodSeconds int64 = 300
)

// NewStatefulSetForCR creates a new StatefulSet for the given Cluster.
func NewStatefulSetForCR(c types.RedisClusterInstance, isAllACLSupported bool, index int) (*appsv1.StatefulSet, error) {
	cluster := c.Definition()

	var (
		spec             = cluster.Spec
		users            = c.Users()
		opUser           = users.GetOpUser()
		aclConfigMapName = GenerateClusterACLConfigMapName(cluster.GetName())
	)
	if opUser.Role == user.RoleOperator && !isAllACLSupported {
		opUser = users.GetDefaultUser()
	}
	if !c.Version().IsACLSupported() {
		aclConfigMapName = ""
	}

	var (
		volumes         = redisVolumes(cluster, opUser)
		stsName         = ClusterStatefulSetName(cluster.Name, index)
		labels          = GetClusterStatefulsetSelectorLabels(cluster.Name, index)
		headlessSvcName = ClusterHeadlessSvcName(cluster.Name, index)

		size = spec.ClusterReplicas + 1
	)
	envs := []corev1.EnvVar{
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
			Value: headlessSvcName,
		},
		{
			Name:  "TERMINATION_GRACE_PERIOD",
			Value: fmt.Sprintf("%d", DefaultTerminationGracePeriodSeconds),
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
			Value: fmt.Sprintf("%t", cluster.Spec.EnableTLS),
		},
		{
			Name: "PERSISTENT_ENABLED",
			Value: fmt.Sprintf("%t", cluster.Spec.Storage != nil &&
				cluster.Spec.Storage.Type == redisv1alpha1.PersistentClaim),
		},
		{
			Name:  "NODEPORT_ENABLED",
			Value: fmt.Sprintf("%t", cluster.Spec.Expose.ServiceType == corev1.ServiceTypeNodePort),
		},
		{
			Name:  "IP_FAMILY_PREFER",
			Value: string(cluster.Spec.IPFamilyPrefer),
		},
		{
			Name:  "REDIS_ADDRESS",
			Value: net.JoinHostPort(config.LocalInjectName, strconv.FormatInt(DefaultRedisServerPort, 10)),
		},
		{
			Name:  "SERVICE_TYPE",
			Value: string(cluster.Spec.Expose.ServiceType),
		},
	}
	if cluster.Spec.Expose.ServiceType == corev1.ServiceTypeNodePort && len(cluster.Spec.Expose.NodePortSequence) > 0 {
		envs = append(envs, corev1.EnvVar{
			Name:  "CUSTOM_PORT_ENABLED",
			Value: "true",
		})
	}
	if c.Version().IsClusterShardSupported() {
		// TODO: use real shard id in redis7
		data := sha1.Sum([]byte(fmt.Sprintf("%s/%s", cluster.Namespace, stsName))) // #nosec
		shardId := fmt.Sprintf("%x", data)
		envs = append(envs, corev1.EnvVar{
			Name:  "SHARD_ID",
			Value: shardId,
		})
	}

	if cluster.Spec.EnableTLS {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "TLS_CA_CERT_FILE",
				Value: "/tls/ca.crt",
			},
			corev1.EnvVar{
				Name:  "TLS_CLIENT_KEY_FILE",
				Value: "/tls/tls.key",
			},
			corev1.EnvVar{
				Name:  "TLS_CLIENT_CERT_FILE",
				Value: "/tls/tls.crt",
			})
	}

	localhost := "127.0.0.1"
	if cluster.Spec.IPFamilyPrefer == corev1.IPv6Protocol {
		localhost = "::1"
	}

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            stsName,
			Namespace:       cluster.Namespace,
			Labels:          labels,
			OwnerReferences: util.BuildOwnerReferences(cluster),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: headlessSvcName,
			Replicas:    &size,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: cluster.Spec.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					HostAliases: []corev1.HostAlias{
						{
							IP:        localhost,
							Hostnames: []string{config.LocalInjectName},
						},
					},
					TerminationGracePeriodSeconds: pointer.Int64(DefaultTerminationGracePeriodSeconds),
					ServiceAccountName:            RedisInstanceServiceAccountName,
					ImagePullSecrets:              spec.ImagePullSecrets,
					Tolerations:                   spec.Tolerations,
					NodeSelector:                  spec.NodeSelector,
					Affinity:                      getAffinity(cluster, labels, stsName),
					Containers: []corev1.Container{
						redisServerContainer(cluster, opUser, envs, index),
					},
					SecurityContext: getSecurityContext(spec.SecurityContext),
					Volumes:         volumes,
				},
			},
		},
	}

	if spec.Storage != nil && spec.Storage.Type == redisv1alpha1.PersistentClaim {
		ss.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			persistentClaim(cluster, labels),
		}
		if spec.Storage.DeleteClaim {
			// set an owner reference so the persistent volumes are deleted when the cluster be deleted.
			ss.Spec.VolumeClaimTemplates[0].OwnerReferences = util.BuildOwnerReferences(cluster)
		}
	}

	initContainer, container := buildContainers(cluster, opUser, envs)
	if initContainer != nil && container != nil {
		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, *initContainer)
		ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, *container)
	}

	if spec.Monitor != nil {
		ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, redisExporterContainer(cluster, opUser))
	}
	return ss, nil
}

func persistentClaim(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) corev1.PersistentVolumeClaim {
	var sc *string
	if cluster.Spec.Storage.Class != "" {
		sc = &cluster.Spec.Storage.Class
	}
	mode := corev1.PersistentVolumeFilesystem
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   RedisStorageVolumeName,
			Labels: labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: cluster.Spec.Storage.Size,
				},
			},
			StorageClassName: sc,
			VolumeMode:       &mode,
		},
	}
}

func redisServerContainer(cluster *redisv1alpha1.DistributedRedisCluster, u *user.User, envs []corev1.EnvVar, index int) corev1.Container {
	shutdownArgs := []string{"sh", "-c", "/opt/redis-tools cluster shutdown  &> /proc/1/fd/1"}
	startArgs := []string{"sh", "/opt/run.sh"}
	if cluster.Spec.EnableActiveRedis {
		startArgs = append(startArgs,
			"--loadmodule", "/modules/activeredis.so",
			"service_id", fmt.Sprintf("%d", *cluster.Spec.ServiceID),
			"service_uid", string(cluster.UID),
			"shard_id", fmt.Sprintf("%d", index),
		)
	}

	container := corev1.Container{
		Env:             envs,
		Name:            ServerContainerName,
		Image:           cluster.Spec.Image,
		ImagePullPolicy: builder.GetPullPolicy(cluster.Spec.ImagePullPolicy),
		Command:         startArgs,
		SecurityContext: getContainerSecurityContext(cluster.Spec.ContainerSecurityContext),
		Ports: []corev1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: DefaultRedisServerPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "gossip",
				ContainerPort: DefaultRedisServerBusPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		VolumeMounts: volumeMounts(cluster, u),
		StartupProbe: &corev1.Probe{
			InitialDelaySeconds: ProbeDelaySeconds,
			TimeoutSeconds:      5,
			FailureThreshold:    5,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(DefaultRedisServerPort),
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
			TimeoutSeconds:      10,
			SuccessThreshold:    1,
			FailureThreshold:    3,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/opt/redis-tools", "helper", "healthcheck", "ping"},
				},
			},
		},
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(DefaultRedisServerPort),
				},
			},
		},
		Resources: *cluster.Spec.Resources,
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: shutdownArgs,
				},
			},
		},
	}
	container.Env = append(container.Env, cluster.Spec.Env...)

	return container
}

func buildContainers(cluster *redisv1alpha1.DistributedRedisCluster, user *user.User, envs []corev1.EnvVar) (*corev1.Container, *corev1.Container) {
	image := config.GetRedisToolsImage(cluster)
	if image == "" {
		return nil, nil
	}

	initContainer := corev1.Container{
		Name:            CheckContainerName,
		Image:           image,
		ImagePullPolicy: builder.GetPullPolicy(cluster.Spec.ImagePullPolicy),
		Env:             envs,
		Command:         []string{"sh", "-c", "/opt/init.sh"},
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
			{Name: RedisStorageVolumeName, MountPath: StorageVolumeMountPath},
			{Name: RedisOptVolumeName, MountPath: "/mnt/opt"},
			{Name: RedisTempVolumeName, MountPath: RedisTmpVolumeMountPath},
		},
	}

	container := corev1.Container{
		Name:            ConfigSyncContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env:             envs,
		Command:         []string{"/opt/redis-tools", "runner", "cluster", "--sync-l2c"},
		SecurityContext: getContainerSecurityContext(cluster.Spec.ContainerSecurityContext),
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
			{Name: RedisStorageVolumeName, MountPath: StorageVolumeMountPath, ReadOnly: true},
		},
	}

	if user.Password.GetSecretName() != "" {
		mount := corev1.VolumeMount{
			Name:      RedisOperatorPasswordVolumeName,
			MountPath: OperatorPasswordVolumeMountPath,
		}
		initContainer.VolumeMounts = append(initContainer.VolumeMounts, mount)
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}

	if cluster.Spec.EnableTLS {
		mount := corev1.VolumeMount{
			Name:      RedisTLSVolumeName,
			MountPath: TLSVolumeMountPath,
		}
		initContainer.VolumeMounts = append(initContainer.VolumeMounts, mount)
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}
	return &initContainer, &container
}

func redisExporterContainer(cluster *redisv1alpha1.DistributedRedisCluster, user *user.User) corev1.Container {
	const DefaultPasswordFile = "/tmp/passwords.json"
	entrypoint := fmt.Sprintf(`
if [ -f /account/password ]; then
    echo "{\"${REDIS_ADDR}\": \"$(cat /account/password)\"}" > %s
fi
/redis_exporter --web.listen-address=:%d --web.telemetry-path=%s %s`,
		DefaultPasswordFile,
		PrometheusExporterPortNumber,
		PrometheusExporterTelemetryPath,
		strings.Join(cluster.Spec.Monitor.Args, " "),
	)
	container := corev1.Container{
		Name:            ExporterContainerName,
		Command:         []string{"/bin/sh", "-c", entrypoint},
		Image:           cluster.Spec.Monitor.Image,
		ImagePullPolicy: builder.GetPullPolicy(cluster.Spec.Monitor.ImagePullPolicy, cluster.Spec.ImagePullPolicy),
		Ports: []corev1.ContainerPort{
			{
				Name:          "prom-http",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: PrometheusExporterPortNumber,
			},
		},
		Env:             cluster.Spec.Monitor.Env,
		Resources:       *cluster.Spec.Monitor.Resources,
		SecurityContext: getContainerSecurityContext(cluster.Spec.Monitor.SecurityContext),
	}

	name := user.Name
	if user.Name == "default" {
		name = ""
	}
	if user.Password.GetSecretName() != "" {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: OperatorUsername, Value: user.Name},
			corev1.EnvVar{Name: OperatorSecretName, Value: user.GetPassword().GetSecretName()},
			corev1.EnvVar{Name: "REDIS_USER", Value: name},
			corev1.EnvVar{Name: "REDIS_PASSWORD_FILE", Value: DefaultPasswordFile},
		)

		container.VolumeMounts = append(container.VolumeMounts,
			corev1.VolumeMount{
				Name:      RedisOperatorPasswordVolumeName,
				MountPath: OperatorPasswordVolumeMountPath,
			},
			corev1.VolumeMount{
				Name:      RedisExporterTempVolumeName,
				MountPath: RedisTmpVolumeMountPath,
			},
		)
	}

	if cluster.Spec.EnableTLS {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      RedisTLSVolumeName,
			MountPath: TLSVolumeMountPath,
		})

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
				Name: "REDIS_ADDR",
				// NOTE: use dns to escape ipv4/ipv6 check
				Value: fmt.Sprintf("rediss://%s:%d", config.LocalInjectName, DefaultRedisServerPort),
			},
		}...)
	} else {
		container.Env = append(container.Env, []corev1.EnvVar{
			{Name: "REDIS_ADDR",
				Value: fmt.Sprintf("redis://%s:%d", config.LocalInjectName, DefaultRedisServerPort)},
		}...)
	}
	return container
}

func volumeMounts(cluster *redisv1alpha1.DistributedRedisCluster, user *user.User) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{Name: ConfigVolumeName, MountPath: ConfigVolumeMountPath},
		{Name: RedisStorageVolumeName, MountPath: StorageVolumeMountPath},
		{Name: RedisOptVolumeName, MountPath: RedisOptVolumeMountPath},
		{Name: RedisTempVolumeName, MountPath: RedisTmpVolumeMountPath},
	}
	if user.Password.GetSecretName() != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      RedisOperatorPasswordVolumeName,
			MountPath: OperatorPasswordVolumeMountPath,
		})
	}
	if cluster.Spec.EnableTLS {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      RedisTLSVolumeName,
			MountPath: TLSVolumeMountPath,
		})
	}
	return volumeMounts
}

func getContainerSecurityContext(secctx *corev1.SecurityContext) *corev1.SecurityContext {
	// 999 is the default userid for redis offical docker image
	// 1000 is the default groupid for redis offical docker image
	userId, groupId := int64(999), int64(1000)
	if secctx == nil {
		secctx = &corev1.SecurityContext{
			RunAsUser:              &userId,
			RunAsGroup:             &groupId,
			RunAsNonRoot:           pointer.Bool(true),
			ReadOnlyRootFilesystem: pointer.Bool(true),
		}
	} else {
		if secctx.RunAsUser == nil {
			secctx.RunAsUser = &userId
		}
		if secctx.RunAsGroup == nil {
			secctx.RunAsGroup = &groupId
		}
		if *secctx.RunAsUser != 0 {
			if secctx.RunAsNonRoot == nil {
				secctx.RunAsNonRoot = pointer.Bool(true)
			}
		} else {
			secctx.RunAsNonRoot = nil
		}
		if secctx.ReadOnlyRootFilesystem == nil {
			secctx.ReadOnlyRootFilesystem = pointer.Bool(true)
		}
	}
	return secctx
}

func getSecurityContext(secctx *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	// 999 is the default userid for redis official docker image
	// 1000 is the default groupid for redis official docker image
	groupID := int64(1000)
	if secctx == nil {
		secctx = &corev1.PodSecurityContext{
			FSGroup: &groupID,
			// RunAsNonRoot: pointer.Bool(true),
		}
	} else {
		if secctx.FSGroup == nil {
			secctx.FSGroup = &groupID
		}
	}
	return secctx
}

func redisVolumes(cluster *redisv1alpha1.DistributedRedisCluster, user *user.User) []corev1.Volume {
	// NOTE: when upgrade from 3.8.1 to 3.8.2,3.10.1
	// the SecurityContext not updated, which specified the runAsUser, runAsGroup, fsGroup
	// if use 0400, without fsGroup specified, the mounted file will not readable to non root uses.
	//
	// DefaultMode >= 0444
	volumes := []corev1.Volume{
		{
			Name: ConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: RedisConfigMapName(cluster.Name),
					},
				},
			},
		},
		{
			Name: RedisOptVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: RedisTempVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: resource.NewQuantity(1<<20, resource.BinarySI), //1Mi
				},
			},
		},
	}

	if user.Password.GetSecretName() != "" {
		volumes = append(volumes, corev1.Volume{
			Name: RedisOperatorPasswordVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: user.Password.GetSecretName(),
				},
			},
		})
	}

	if cluster.Spec.Monitor != nil {
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

	if cluster.Spec.EnableTLS {
		volumes = append(volumes, corev1.Volume{
			Name: RedisTLSVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: builder.GetRedisSSLSecretName(cluster.Name),
				},
			},
		})
	}

	dataVolume := redisDataVolume(cluster)
	if dataVolume != nil {
		volumes = append(volumes, *dataVolume)
	}
	return volumes
}
func redisDataVolume(cluster *redisv1alpha1.DistributedRedisCluster) *corev1.Volume {
	// This will find the volumed desired by the user. If no volume defined
	// an EmptyDir will be used by default
	if cluster.Spec.Storage == nil {
		return emptyVolume()
	}

	switch cluster.Spec.Storage.Type {
	case redisv1alpha1.PersistentClaim:
		return nil
	case redisv1alpha1.Ephemeral:
		return emptyVolume()
	default:
		return emptyVolume()
	}
}

func getAffinity(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string, ssName string) *corev1.Affinity {
	affinity := cluster.Spec.Affinity
	if affinity != nil {
		return affinity
	}

	policy := cluster.Spec.AffinityPolicy
	if policy == "" {
		if cluster.Spec.RequiredAntiAffinity {
			policy = core.AntiAffinity
		}
	}

	switch policy {
	case core.AntiAffinityInSharding:
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						TopologyKey: hostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								LabelClusterName: cluster.Name,
								StatefulSetLabel: ssName,
							},
						},
					},
				},
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					{
						Weight: 100,
						PodAffinityTerm: corev1.PodAffinityTerm{
							TopologyKey: hostnameTopologyKey,
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{LabelClusterName: cluster.Name},
							},
						},
					},
				},
			},
		}
	case core.AntiAffinity:
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						TopologyKey: hostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{LabelClusterName: cluster.Name},
						},
					},
				},
			},
		}
	}

	// return a SOFT anti-affinity by default
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 80,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: hostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								LabelClusterName: cluster.Name,
								StatefulSetLabel: ssName,
							},
						},
					},
				},
				{
					Weight: 20,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: hostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{LabelClusterName: cluster.Name},
						},
					},
				},
			},
		},
	}
}

// IsStatefulsetChanged
func IsStatefulsetChanged(newSts, sts *appv1.StatefulSet, logger logr.Logger) bool {
	// statefulset check
	if !reflect.DeepEqual(newSts.GetLabels(), sts.GetLabels()) ||
		!reflect.DeepEqual(newSts.GetAnnotations(), sts.GetAnnotations()) {
		logger.V(2).Info("labels or annotations diff")
		return true
	}

	if *newSts.Spec.Replicas != *sts.Spec.Replicas ||
		newSts.Spec.ServiceName != sts.Spec.ServiceName {
		logger.V(2).Info("replicas diff")
		return true
	}

	for _, name := range []string{
		RedisStorageVolumeName,
	} {
		oldPvc := util.GetVolumeClaimTemplatesByName(sts.Spec.VolumeClaimTemplates, name)
		newPvc := util.GetVolumeClaimTemplatesByName(newSts.Spec.VolumeClaimTemplates, name)
		if oldPvc == nil && newPvc == nil {
			continue
		}
		if (oldPvc == nil && newPvc != nil) || (oldPvc != nil && newPvc == nil) {
			logger.V(2).Info("pvc diff", "name", name, "old", oldPvc, "new", newPvc)
			return true
		}

		if !reflect.DeepEqual(oldPvc.Spec, newPvc.Spec) {
			logger.V(2).Info("pvc diff", "name", name, "old", oldPvc.Spec, "new", newPvc.Spec)
			return true
		}
	}

	return IsPodTemplasteChanged(&newSts.Spec.Template, &sts.Spec.Template, logger)
}

func emptyVolume() *corev1.Volume {
	return &corev1.Volume{
		Name: RedisStorageVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func ClusterStatefulSetName(clusterName string, i int) string {
	return fmt.Sprintf("drc-%s-%d", clusterName, i)
}

func ClusterHeadlessSvcName(name string, i int) string {
	return fmt.Sprintf("%s-%d", name, i)
}

func ParsePodShardAndIndex(name string) (shard int, index int, err error) {
	fields := strings.Split(name, "-")
	if len(fields) < 3 {
		return -1, -1, fmt.Errorf("invalid pod name %s", name)
	}
	if index, err = strconv.Atoi(fields[len(fields)-1]); err != nil {
		return -1, -1, fmt.Errorf("invalid pod name %s", name)
	}
	if shard, err = strconv.Atoi(fields[len(fields)-2]); err != nil {
		return -1, -1, fmt.Errorf("invalid pod name %s", name)
	}
	return shard, index, nil
}
