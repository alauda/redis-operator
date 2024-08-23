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

package redis

import (
	"reflect"
	"strings"
	"time"

	v1alpha1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	v1 "github.com/alauda/redis-operator/api/middleware/v1"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/internal/vc"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func GetRedisVersion(image string) string {
	switch {
	case strings.Contains(image, "redis:4.0"):
		return "4.0"
	case strings.Contains(image, "redis:5.0"):
		return "5.0"
	case strings.Contains(image, "redis:6.0"):
		return "6.0"
	}
	return ""
}

type ApplyPolicy string

const (
	Unsupported  ApplyPolicy = "Unsupported"
	RestartApply ApplyPolicy = "RestartApply"
)

func GetRedisConfigsApplyPolicyByVersion(ver string) map[string]ApplyPolicy {
	data := map[string]ApplyPolicy{
		"databases":           RestartApply,
		"rename-command":      RestartApply,
		"rdbchecksum":         RestartApply,
		"tcp-backlog":         RestartApply,
		"io-threads":          RestartApply,
		"io-threads-do-reads": RestartApply,
	}
	rv := redis.RedisVersion(ver)
	if rv.IsACLSupported() {
		delete(data, "rename-command")
	}
	return data
}

const (
	ControllerVersionLabel = "controllerVersion"
)

func GenerateRedisCluster(instance *v1.Redis, bv *vc.BundleVersion) (*v1alpha1.DistributedRedisCluster, error) {
	customConfig := instance.Spec.CustomConfig
	if customConfig == nil {
		customConfig = map[string]string{}
	}
	if instance.Spec.PodAnnotations == nil {
		instance.Spec.PodAnnotations = map[string]string{}
	}

	image, err := bv.GetRedisImage(instance.Spec.Version)
	if instance.Spec.EnableActiveRedis {
		image, err = bv.GetActiveRedisImage(instance.Spec.Version)
	}
	if err != nil {
		return nil, err
	}

	labels := GetRedisClusterLabels(instance.Name, GetRedisClusterName(instance.Name))
	if v := instance.Labels[ControllerVersionLabel]; v != "" {
		labels[ControllerVersionLabel] = v
	}

	var (
		podSec       = instance.Spec.SecurityContext
		containerSec *corev1.SecurityContext
		access       = core.InstanceAccess{
			InstanceAccessBase: *instance.Spec.Expose.InstanceAccessBase.DeepCopy(),
			IPFamilyPrefer:     instance.Spec.IPFamilyPrefer,
		}
		backup      = instance.Spec.Backup
		restore     = instance.Spec.Restore
		annotations = map[string]string{}
	)
	for key, comp := range bv.Spec.Components {
		if key == "redis" {
			annotations[config.BuildImageVersionKey("redis")] = image
		} else if len(comp.ComponentVersions) > 0 {
			annotations[config.BuildImageVersionKey(key)], _ = bv.GetImage(key, "")
		}
	}

	access.Image, _ = bv.GetRedisToolsImage()
	if len(backup.Schedule) > 0 {
		backup.Image, _ = bv.GetRedisToolsImage()
	}
	if restore.BackupName != "" {
		restore.Image, _ = bv.GetRedisToolsImage()
	}
	if instance.Status.Restored {
		restore = core.RedisRestore{}
	}

	exporterImage, _ := bv.GetRedisExporterImage()
	monitor := &v1alpha1.Monitor{
		Image: exporterImage,
		Resources: &corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("300Mi"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("300Mi"),
			},
		},
	}

	if podSec != nil {
		containerSec = &corev1.SecurityContext{
			SELinuxOptions: podSec.SELinuxOptions,
			WindowsOptions: podSec.WindowsOptions,
			RunAsUser:      podSec.RunAsUser,
			RunAsGroup:     podSec.RunAsGroup,
			RunAsNonRoot:   podSec.RunAsNonRoot,
			SeccompProfile: podSec.SeccompProfile,
		}
	}

	shards := instance.Spec.Replicas.Cluster.Shards
	if *instance.Spec.Replicas.Cluster.Shard != int32(len(shards)) {
		shards = nil
	}

	cluster := &v1alpha1.DistributedRedisCluster{
		ObjectMeta: v12.ObjectMeta{
			Name:            GetRedisClusterName(instance.Name),
			Namespace:       instance.Namespace,
			Labels:          labels,
			Annotations:     annotations,
			OwnerReferences: util.BuildOwnerReferences(instance),
		},
		Spec: v1alpha1.DistributedRedisClusterSpec{
			Image:                    image,
			MasterSize:               *instance.Spec.Replicas.Cluster.Shard,
			ClusterReplicas:          *instance.Spec.Replicas.Cluster.Slave,
			Resources:                instance.Spec.Resources,
			Config:                   customConfig,
			Shards:                   shards,
			Affinity:                 instance.Spec.Affinity,
			AffinityPolicy:           instance.Spec.AffinityPolicy,
			NodeSelector:             instance.Spec.NodeSelector,
			Tolerations:              instance.Spec.Tolerations,
			SecurityContext:          instance.Spec.SecurityContext,
			ContainerSecurityContext: containerSec,
			PodAnnotations:           instance.Spec.PodAnnotations,
			IPFamilyPrefer:           instance.Spec.IPFamilyPrefer,
			EnableTLS:                instance.Spec.EnableTLS,
			EnableActiveRedis:        instance.Spec.EnableActiveRedis,
			ServiceID:                instance.Spec.ServiceID,
			// with custom images
			Expose:  access,
			Backup:  backup,
			Restore: restore,
			Monitor: monitor,
		},
	}
	// add password secret
	if !instance.PasswordIsEmpty() {
		cluster.Spec.PasswordSecret = &corev1.LocalObjectReference{Name: instance.Status.PasswordSecretName}
	} else {
		cluster.Spec.PasswordSecret = nil
	}

	if instance.Spec.Persistent != nil && instance.Spec.Persistent.StorageClassName != "" &&
		(instance.Spec.PersistentSize == nil || instance.Spec.PersistentSize.IsZero()) {
		size := resource.NewQuantity(instance.Spec.Resources.Limits.Memory().Value()*2, resource.BinarySI)
		instance.Spec.PersistentSize = size
	}
	sc := ""
	if instance.Spec.Persistent != nil {
		sc = instance.Spec.Persistent.StorageClassName
	}
	if size := instance.Spec.PersistentSize; size != nil {
		cluster.Spec.Storage = &v1alpha1.RedisStorage{
			Type:        v1alpha1.PersistentClaim,
			Size:        *size,
			Class:       sc,
			DeleteClaim: false,
		}
	}
	return cluster, nil
}

func ShouldUpdateCluster(cluster, newCluster *v1alpha1.DistributedRedisCluster, logger logr.Logger) bool {
	if newCluster.Spec.EnableActiveRedis != cluster.Spec.EnableActiveRedis ||
		(newCluster.Spec.ServiceID == nil && cluster.Spec.ServiceID != nil) ||
		(newCluster.Spec.ServiceID != nil && cluster.Spec.ServiceID == nil) ||
		((newCluster.Spec.ServiceID != nil && cluster.Spec.ServiceID != nil) &&
			(*newCluster.Spec.ServiceID != *cluster.Spec.ServiceID)) {
		return true
	}

	if !reflect.DeepEqual(cluster.Labels, newCluster.Labels) ||
		!reflect.DeepEqual(cluster.Annotations, newCluster.Annotations) {
		return true
	}

	if cluster.Spec.Image != newCluster.Spec.Image {
		return true
	}
	if resourceDiff(*newCluster.Spec.Resources, *cluster.Spec.Resources) {
		logger.V(3).Info("cluster resources diff")
		return true
	}
	if !reflect.DeepEqual(cluster.Spec.PasswordSecret, newCluster.Spec.PasswordSecret) {
		logger.V(3).Info("cluster password diff")
		return true
	}
	if newCluster.Spec.MasterSize != cluster.Spec.MasterSize {
		logger.V(3).Info("cluster replicas diff")
		return true
	}
	if newCluster.Spec.ClusterReplicas != cluster.Spec.ClusterReplicas {
		logger.V(3).Info("cluster slave diff")
		return true
	}
	if !reflect.DeepEqual(newCluster.Spec.Config, cluster.Spec.Config) {
		logger.V(3).Info("cluster customconfig diff")
		return true
	}

	if diffAnnonation(newCluster.Spec.PodAnnotations, cluster.Spec.PodAnnotations) {
		logger.V(3).Info("pod annotations diff")
		return true
	}
	if diffBackup(&cluster.Spec.Backup, &newCluster.Spec.Backup) {
		logger.V(3).Info("redis backup diff")
		return true
	}
	if !reflect.DeepEqual(&cluster.Spec.Restore, &newCluster.Spec.Restore) {
		logger.V(3).Info("redis restore diff")
		return true
	}

	if !reflect.DeepEqual(cluster.Spec.Expose, newCluster.Spec.Expose) {
		logger.V(3).Info("redis expose diff")
		return true
	}

	if cluster.Spec.AffinityPolicy != newCluster.Spec.AffinityPolicy ||
		!reflect.DeepEqual(cluster.Spec.NodeSelector, newCluster.Spec.NodeSelector) ||
		!reflect.DeepEqual(cluster.Spec.Tolerations, newCluster.Spec.Tolerations) {
		return true
	}
	if !reflect.DeepEqual(cluster.Spec.SecurityContext, newCluster.Spec.SecurityContext) {
		return true
	}

	storage1 := cluster.Spec.Storage
	storage2 := newCluster.Spec.Storage
	if storage1 != nil && storage2 != nil {
		val1, _ := storage1.Size.AsInt64()
		val2, _ := storage2.Size.AsInt64()
		if val1 != val2 {
			return true
		}
	} else if storage1 != nil || storage2 != nil {
		return true
	}
	return false
}

func GenerateClusterRedisByManagerUI(cluster *v1alpha1.DistributedRedisCluster, scheme *runtime.Scheme, secret *corev1.Secret) *v1.Redis {
	instance := &v1.Redis{
		ObjectMeta: v12.ObjectMeta{
			Name:        cluster.Name,
			Namespace:   cluster.Namespace,
			Labels:      cluster.Labels,
			Annotations: map[string]string{"createType": "managerView"},
		},
		Spec: v1.RedisSpec{
			Arch:      core.RedisCluster,
			Version:   GetRedisVersion(cluster.Spec.Image),
			Resources: cluster.Spec.Resources,
			Replicas: &v1.RedisReplicas{
				Cluster: &v1.ClusterReplicas{
					Shard: &cluster.Spec.MasterSize,
					Slave: &cluster.Spec.ClusterReplicas,
				},
			},
			Backup:          cluster.Spec.Backup,
			Restore:         cluster.Spec.Restore,
			Affinity:        cluster.Spec.Affinity,
			NodeSelector:    cluster.Spec.NodeSelector,
			Tolerations:     cluster.Spec.Tolerations,
			SecurityContext: cluster.Spec.SecurityContext,
			PodAnnotations:  cluster.Spec.PodAnnotations,
			CustomConfig:    cluster.Spec.Config,
			// Monitor:         cluster.Spec.Monitor,
			EnableTLS: cluster.Spec.EnableTLS,
		},
	}

	//			Persistent:
	//			PersistentSize:
	if cluster.Spec.Storage != nil {
		instance.Spec.Persistent = &v1.RedisPersistent{
			StorageClassName: cluster.Spec.Storage.Class,
		}
		instance.Spec.PersistentSize = &cluster.Spec.Storage.Size
	}
	//			Password:
	//			PasswordSecret:
	if secret != nil {
		instance.Spec.PasswordSecret = secret.Name
	}
	// label
	instance.Labels = make(map[string]string)
	for k, v := range GetRedisClusterLabels(instance.Name, GetRedisClusterName(instance.Name)) {
		instance.Labels[k] = v
	}
	return instance
}

func ClusterIsUp(cluster *v1alpha1.DistributedRedisCluster) bool {
	return cluster.Status.Status == v1alpha1.ClusterStatusOK &&
		cluster.Status.ClusterStatus == v1alpha1.ClusterInService
}

const (
	redisRestartAnnotation = "kubectl.kubernetes.io/restartedAt"
	PauseAnnotationKey     = "app.cpaas.io/pause-timestamp"
)

func diffAnnonation(rfAnnotations map[string]string, targetAnnotations map[string]string) bool {
	if len(rfAnnotations) == 0 {
		if len(targetAnnotations) > 0 && targetAnnotations[PauseAnnotationKey] != "" {
			return true
		}
		return false
	} else {
		if len(targetAnnotations) > 0 && targetAnnotations[PauseAnnotationKey] != rfAnnotations[PauseAnnotationKey] {
			return true
		}
	}
	if len(targetAnnotations) == 0 {
		return true
	}
	for k, v := range rfAnnotations {
		if k == redisRestartAnnotation {
			if v == "" {
				continue
			}
			targetV := targetAnnotations[redisRestartAnnotation]
			if targetV == "" {
				return true
			}
			newTime, err1 := time.Parse(time.RFC3339Nano, v)
			targetTime, err2 := time.Parse(time.RFC3339Nano, targetV)
			if err1 != nil || err2 != nil {
				return true
			}
			if newTime.After(targetTime) {
				return true
			}
		} else if targetAnnotations[k] != v {
			return true
		}
	}
	return false
}

func MergeAnnotations(t, s map[string]string) map[string]string {
	if t == nil {
		return s
	}
	if s == nil {
		return t
	}

	for k, v := range s {
		if k == redisRestartAnnotation {
			tRestartAnn := t[k]
			if tRestartAnn == "" && v != "" {
				t[k] = v
			}

			tTime, err1 := time.Parse(time.RFC3339Nano, tRestartAnn)
			sTime, err2 := time.Parse(time.RFC3339Nano, v)
			if err1 != nil || err2 != nil || sTime.After(tTime) {
				t[k] = v
			} else {
				t[k] = tRestartAnn
			}
		} else {
			t[k] = v
		}
	}
	return t
}
