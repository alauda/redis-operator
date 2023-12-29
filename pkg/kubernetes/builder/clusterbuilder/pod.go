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
	"reflect"

	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
)

func IsPodTemplasteChanged(newTplSpec, oldTplSpec *v1.PodTemplateSpec, logger logr.Logger) bool {
	if (newTplSpec == nil && oldTplSpec != nil) || (newTplSpec != nil && oldTplSpec == nil) ||
		!reflect.DeepEqual(newTplSpec.Labels, oldTplSpec.Labels) ||
		!reflect.DeepEqual(newTplSpec.Annotations, oldTplSpec.Annotations) {
		logger.V(2).Info("pod labels diff")
		return true
	}

	newSpec, oldSpec := newTplSpec.Spec, oldTplSpec.Spec
	if len(newSpec.InitContainers) != len(oldSpec.InitContainers) ||
		len(newSpec.Containers) != len(oldSpec.Containers) {
		logger.V(2).Info("pod containers length diff")
		return true
	}

	// nodeselector
	if !reflect.DeepEqual(newSpec.NodeSelector, oldSpec.NodeSelector) ||
		!reflect.DeepEqual(newSpec.Affinity, oldSpec.Affinity) ||
		!reflect.DeepEqual(newSpec.Tolerations, oldSpec.Tolerations) {
		logger.V(2).Info("pod nodeselector|affinity|tolerations diff")
		return true
	}

	if !reflect.DeepEqual(newSpec.SecurityContext, oldSpec.SecurityContext) ||
		newSpec.HostNetwork != oldSpec.HostNetwork ||
		newSpec.ServiceAccountName != oldSpec.ServiceAccountName {
		logger.V(2).Info("pod securityContext or hostnetwork or serviceaccount diff",
			"securityContext", !reflect.DeepEqual(newSpec.SecurityContext, oldSpec.SecurityContext),
			"new", newSpec.SecurityContext, "old", oldSpec.SecurityContext)
		return true
	}

	if len(newSpec.Volumes) != len(oldSpec.Volumes) {
		return true
	}
	for _, newVol := range newSpec.Volumes {
		oldVol := util.GetVolumeByName(oldSpec.Volumes, newVol.Name)
		if oldVol == nil {
			return true
		}
		if newVol.Secret != nil && (oldVol.Secret == nil || oldVol.Secret.SecretName != newVol.Secret.SecretName) {
			return true
		}
		if newVol.ConfigMap != nil && (oldVol.ConfigMap == nil || oldVol.ConfigMap.Name != newVol.ConfigMap.Name) {
			return true
		}
		if newVol.PersistentVolumeClaim != nil &&
			(oldVol.PersistentVolumeClaim == nil || oldVol.PersistentVolumeClaim.ClaimName != newVol.PersistentVolumeClaim.ClaimName) {
			return true
		}
		if newVol.HostPath != nil && (oldVol.HostPath == nil || oldVol.HostPath.Path != newVol.HostPath.Path) {
			return true
		}
		if newVol.EmptyDir != nil && (oldVol.EmptyDir == nil || oldVol.EmptyDir.Medium != newVol.EmptyDir.Medium) {
			return true
		}
	}

	containerNames := map[string]struct{}{}
	for _, c := range newSpec.InitContainers {
		containerNames[c.Name] = struct{}{}
	}
	for _, c := range newSpec.Containers {
		containerNames[c.Name] = struct{}{}
	}

	for name := range containerNames {
		oldCon, newCon := util.GetContainerByName(&oldSpec, name), util.GetContainerByName(&newSpec, name)
		if (oldCon == nil && newCon != nil) || (oldCon != nil && newCon == nil) {
			logger.V(2).Info("pod containers not match diff")
			return true
		}

		// check almost all fields of container
		// should make sure that apiserver not return noset default value
		if oldCon.Image != newCon.Image || oldCon.ImagePullPolicy != newCon.ImagePullPolicy ||
			!reflect.DeepEqual(oldCon.Resources, newCon.Resources) ||
			!reflect.DeepEqual(loadEnvs(oldCon.Env), loadEnvs(newCon.Env)) ||
			!reflect.DeepEqual(oldCon.Command, newCon.Command) ||
			!reflect.DeepEqual(oldCon.Args, newCon.Args) ||
			!reflect.DeepEqual(oldCon.Ports, newCon.Ports) ||
			!reflect.DeepEqual(oldCon.Lifecycle, newCon.Lifecycle) ||
			!reflect.DeepEqual(oldCon.VolumeMounts, newCon.VolumeMounts) {

			logger.V(2).Info("pod containers config diff",
				"image", oldCon.Image != newCon.Image,
				"imagepullpolicy", oldCon.ImagePullPolicy != newCon.ImagePullPolicy,
				"resources", !reflect.DeepEqual(oldCon.Resources, newCon.Resources),
				"env", !reflect.DeepEqual(loadEnvs(oldCon.Env), loadEnvs(newCon.Env)),
				"command", !reflect.DeepEqual(oldCon.Command, newCon.Command),
				"args", !reflect.DeepEqual(oldCon.Args, newCon.Args),
				"ports", !reflect.DeepEqual(oldCon.Ports, newCon.Ports),
				"lifecycle", !reflect.DeepEqual(oldCon.Lifecycle, newCon.Lifecycle),
				"volumemounts", !reflect.DeepEqual(oldCon.VolumeMounts, newCon.VolumeMounts),
			)
			return true
		}
	}

	return false
}

func loadEnvs(envs []v1.EnvVar) map[string]string {
	kvs := map[string]string{}
	for _, item := range envs {
		if item.ValueFrom != nil {
			switch {
			case item.ValueFrom.FieldRef != nil:
				kvs[item.Name] = item.ValueFrom.FieldRef.FieldPath
			case item.ValueFrom.SecretKeyRef != nil:
				kvs[item.Name] = item.ValueFrom.SecretKeyRef.Name
			case item.ValueFrom.ConfigMapKeyRef != nil:
				kvs[item.Name] = item.ValueFrom.ConfigMapKeyRef.Name
			case item.ValueFrom.ResourceFieldRef != nil:
				kvs[item.Name] = item.ValueFrom.ResourceFieldRef.Resource
			}
		} else if item.Value != "" {
			kvs[item.Name] = item.Value
		}
	}
	return kvs
}
