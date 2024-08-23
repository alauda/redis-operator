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

	"github.com/go-logr/logr"
	"github.com/samber/lo"

	"github.com/alauda/redis-operator/api/core"
	redisfailover "github.com/alauda/redis-operator/api/databases/v1"
	v1 "github.com/alauda/redis-operator/api/middleware/v1"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/internal/vc"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GenerateRedisFailover(instance *v1.Redis, bv *vc.BundleVersion) (*redisfailover.RedisFailover, error) {
	image, err := bv.GetRedisImage(instance.Spec.Version)
	if instance.Spec.EnableActiveRedis {
		image, err = bv.GetActiveRedisImage(instance.Spec.Version)
	}
	if err != nil {
		return nil, err
	}

	if instance.Spec.Arch == core.RedisSentinel {
		if instance.Spec.Sentinel == nil {
			instance.Spec.Sentinel = &redisfailover.SentinelSettings{}
		}
	} else {
		instance.Spec.Sentinel = nil
	}

	var (
		access = core.InstanceAccess{
			InstanceAccessBase: *instance.Spec.Expose.InstanceAccessBase.DeepCopy(),
			IPFamilyPrefer:     instance.Spec.IPFamilyPrefer,
		}
		exporter = instance.Spec.Exporter.DeepCopy()
		backup   = instance.Spec.Backup
		restore  = instance.Spec.Restore
		sentinel = instance.Spec.Sentinel

		annotations = map[string]string{}
	)

	for key, comp := range bv.Spec.Components {
		if key == "redis" {
			annotations[config.BuildImageVersionKey("redis")] = image
		} else if len(comp.ComponentVersions) > 0 {
			annotations[config.BuildImageVersionKey(key)], _ = bv.GetImage(key, "")
		}
	}
	if exporter == nil {
		exporter = &redisfailover.RedisExporter{Enabled: true}
	}
	exporter.Image, _ = bv.GetRedisExporterImage()
	if exporter.Resources.Limits.Cpu().IsZero() || exporter.Resources.Limits.Memory().IsZero() {
		exporter.Resources = corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("384Mi"),
			},
		}
	}

	access.Image, _ = bv.GetRedisToolsImage()
	// HARDCODE: compatible with 3.14.x
	if strings.HasPrefix(bv.Spec.CrVersion, "3.14.") {
		access.Image, _ = bv.GetExposePodImage()
	}

	if len(backup.Schedule) > 0 {
		backup.Image, _ = bv.GetRedisToolsImage()
	}
	if restore.BackupName != "" {
		restore.Image, _ = bv.GetRedisToolsImage()
	}
	if instance.Status.Restored {
		restore = core.RedisRestore{}
	}

	if sentinel != nil {
		if sentinel.SentinelReference == nil {
			sentinel.Image = image
			sentinel.Expose.AccessPort = instance.Spec.Expose.AccessPort
			sentinel.Expose.Image, _ = bv.GetRedisToolsImage()
			sentinel.Expose.ServiceType = access.ServiceType
			sentinel.Expose.IPFamilyPrefer = instance.Spec.IPFamilyPrefer
			sentinel.EnableTLS = instance.Spec.EnableTLS

			if len(sentinel.NodeSelector) == 0 {
				sentinel.NodeSelector = instance.Spec.NodeSelector
			}
			if sentinel.Tolerations == nil {
				sentinel.Tolerations = instance.Spec.Tolerations
			}
			if sentinel.SecurityContext == nil {
				sentinel.SecurityContext = instance.Spec.SecurityContext
			}
			sentinel.PodAnnotations = lo.Assign(sentinel.PodAnnotations, instance.Spec.PodAnnotations)
		}
	} else {
		annotations["standalone"] = "true"
		if !instance.Status.Restored {
			// TRICK: used to migrate from devops old redis versions, included keys:
			// * redis-standalone/storage-type - storage type, pvc or hostpath
			// * redis-standalone/filepath
			// * redis-standalone/pvc-name
			// * redis-standalone/hostpath
			// TODO: remove in acp 3.22
			for k, v := range instance.Annotations {
				if strings.HasPrefix(k, "redis-standalone") {
					annotations[k] = v
				}
			}
		}
	}

	failover := &redisfailover.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{
			Name:            GetRedisFailoverName(instance.Name),
			Namespace:       instance.Namespace,
			Labels:          GetRedisSentinelLabels(instance.Name, GetRedisFailoverName(instance.Name)),
			Annotations:     annotations,
			OwnerReferences: util.BuildOwnerReferences(instance),
		},
		Spec: redisfailover.RedisFailoverSpec{
			Redis: redisfailover.RedisSettings{
				Image:          image,
				Replicas:       int32(*instance.Spec.Replicas.Sentinel.Slave) + 1,
				Resources:      *instance.Spec.Resources,
				CustomConfig:   instance.Spec.CustomConfig,
				PodAnnotations: lo.Assign(instance.Spec.PodAnnotations),
				Exporter:       *exporter,
				Backup:         backup,
				Restore:        restore,
				Expose:         access,
				EnableTLS:      instance.Spec.EnableTLS,

				Affinity:        instance.Spec.Affinity,
				NodeSelector:    instance.Spec.NodeSelector,
				Tolerations:     instance.Spec.Tolerations,
				SecurityContext: instance.Spec.SecurityContext,
			},
			Auth: redisfailover.AuthSettings{
				SecretPath: instance.Spec.PasswordSecret,
			},
			Sentinel:          sentinel,
			EnableActiveRedis: instance.Spec.EnableActiveRedis,
			ServiceID:         instance.Spec.ServiceID,
		},
	}

	if instance.Spec.Persistent != nil && instance.Spec.Persistent.StorageClassName != "" &&
		(instance.Spec.PersistentSize == nil || instance.Spec.PersistentSize.IsZero()) {
		size := resource.NewQuantity(instance.Spec.Resources.Limits.Memory().Value()*2, resource.BinarySI)
		instance.Spec.PersistentSize = size
	}
	var sc *string
	if instance.Spec.Persistent != nil && instance.Spec.Persistent.StorageClassName != "" {
		sc = &instance.Spec.Persistent.StorageClassName
	}
	if size := instance.Spec.PersistentSize; size != nil {
		failover.Spec.Redis.Storage.KeepAfterDeletion = true
		failover.Spec.Redis.Storage.PersistentVolumeClaim = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   GetRedisStorageVolumeName(instance.Name),
				Labels: GetRedisSentinelLabels(instance.Name, failover.Name),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: *size},
				},
				StorageClassName: sc,
			},
		}
	}
	return failover, nil
}

func ShouldUpdateFailover(failover, newFailover *redisfailover.RedisFailover, logger logr.Logger) bool {
	if newFailover.Spec.EnableActiveRedis != failover.Spec.EnableActiveRedis ||
		(newFailover.Spec.ServiceID == nil && failover.Spec.ServiceID != nil) ||
		(newFailover.Spec.ServiceID != nil && failover.Spec.ServiceID == nil) ||
		((newFailover.Spec.ServiceID != nil && failover.Spec.ServiceID != nil) &&
			(*newFailover.Spec.ServiceID != *failover.Spec.ServiceID)) {
		return true
	}

	if diffAnnonation(newFailover.Annotations, failover.Annotations) ||
		!reflect.DeepEqual(newFailover.Labels, failover.Labels) {
		return true
	}
	if !reflect.DeepEqual(newFailover.Spec.Redis, failover.Spec.Redis) ||
		!reflect.DeepEqual(failover.Spec.Sentinel, newFailover.Spec.Sentinel) {
		return true
	}
	if !reflect.DeepEqual(failover.Spec.Redis.Expose, newFailover.Spec.Redis.Expose) {
		return true
	}
	if failover.Spec.Redis.EnableTLS != newFailover.Spec.Redis.EnableTLS ||
		failover.Spec.Auth.SecretPath != newFailover.Spec.Auth.SecretPath {
		logger.V(3).Info("failover secrepath diff")
		return true
	}

	newPvc := newFailover.Spec.Redis.Storage.PersistentVolumeClaim
	pvc := failover.Spec.Redis.Storage.PersistentVolumeClaim
	if newPvc != nil && pvc != nil {
		val1, _ := pvc.Spec.Resources.Requests.Storage().AsInt64()
		val2, _ := newPvc.Spec.Resources.Requests.Storage().AsInt64()
		if val1 != val2 {
			return true
		}
	} else if newPvc != nil || pvc != nil {
		return true
	}
	return false
}

func GenerateFailoverRedisByManagerUI(failover *redisfailover.RedisFailover, sts *appsv1.StatefulSet, secret *corev1.Secret) *v1.Redis {
	var redisContainer corev1.Container
	for _, v := range sts.Spec.Template.Spec.Containers {
		if v.Name == "redis" {
			redisContainer = v
		}
	}
	var master int32 = 1
	var slave int32
	if *sts.Spec.Replicas == 0 {
		slave = 0
	} else {
		slave = *sts.Spec.Replicas - 1
	}

	instance := &v1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:        failover.Name,
			Namespace:   failover.Namespace,
			Labels:      failover.Labels,
			Annotations: map[string]string{"createType": "managerView"},
		},
		Spec: v1.RedisSpec{
			Version:    GetRedisVersion(redisContainer.Image),
			Arch:       core.RedisSentinel,
			Resources:  &redisContainer.Resources,
			Persistent: &v1.RedisPersistent{},
			//PersistentSize: nil,
			//Password:       nil,
			//PasswordSecret: "",
			Replicas: &v1.RedisReplicas{
				Sentinel: &v1.SentinelReplicas{
					Master: &master,
					Slave:  &slave,
				},
			},
			Backup:               failover.Spec.Redis.Backup,
			Restore:              failover.Spec.Redis.Restore,
			Affinity:             failover.Spec.Redis.Affinity,
			NodeSelector:         failover.Spec.Redis.NodeSelector,
			Tolerations:          failover.Spec.Redis.Tolerations,
			SecurityContext:      failover.Spec.Redis.SecurityContext,
			CustomConfig:         failover.Spec.Redis.CustomConfig,
			PodAnnotations:       failover.Spec.Redis.PodAnnotations,
			Sentinel:             failover.Spec.Sentinel,
			SentinelCustomConfig: failover.Spec.Sentinel.CustomConfig,
			Exporter:             &failover.Spec.Redis.Exporter,
			EnableTLS:            failover.Spec.Redis.EnableTLS,
			PasswordSecret:       failover.Spec.Auth.SecretPath,
		},
	}

	//Persistent、PersistentSize
	if failover.Spec.Redis.Storage.PersistentVolumeClaim != nil {
		instance.Spec.Persistent.StorageClassName = *failover.Spec.Redis.Storage.PersistentVolumeClaim.Spec.StorageClassName
		instance.Spec.PersistentSize = failover.Spec.Redis.Storage.PersistentVolumeClaim.Spec.Resources.Requests.Storage()
	}

	//Password PasswordSecret
	if secret != nil {
		instance.Spec.PasswordSecret = secret.Name
	}

	// label
	if len(instance.Labels) == 0 {
		instance.Labels = make(map[string]string)
	}
	for k, v := range GetRedisSentinelLabels(instance.Name, GetRedisFailoverName(instance.Name)) {
		instance.Labels[k] = v
	}
	return instance
}

func diffBackup(b1, b2 *core.RedisBackup) bool {
	if b1 == nil && b2 == nil {
		return false
	}
	if (b1 == nil && b2 != nil) || (b1 != nil && b2 == nil) {
		return true
	}

	if b1.Image != b2.Image {
		return true
	}
	if len(b1.Schedule) != len(b2.Schedule) {
		return true
	} else if len(b1.Schedule) > 0 {
		for i, s := range b1.Schedule {
			ss := b2.Schedule[i]
			if s.Schedule != ss.Schedule {
				return true
			}
			if s.Name != ss.Name {
				return true
			}
			if s.Keep != ss.Keep {
				return true
			}
			if s.KeepAfterDeletion != ss.KeepAfterDeletion {
				return true
			}
			if s.Storage.StorageClassName != ss.Storage.StorageClassName {
				return true
			}
			if s.Storage.Size.Size() != ss.Storage.Size.Size() {
				return true
			}
			if !reflect.DeepEqual(s.Target.S3Option, ss.Target.S3Option) {
				return true
			}
		}
	}
	return false
}

func resourceDiff(r1 corev1.ResourceRequirements, r2 corev1.ResourceRequirements) bool {
	if result := r1.Requests.Cpu().Cmp(*r2.Requests.Cpu()); result != 0 {
		return true
	}
	if result := r1.Requests.Memory().Cmp(*r2.Requests.Memory()); result != 0 {
		return true
	}
	if result := r1.Limits.Memory().Cmp(*r2.Limits.Memory()); result != 0 {
		return true
	}
	if result := r1.Limits.Memory().Cmp(*r2.Limits.Memory()); result != 0 {
		return true
	}
	return false
}
