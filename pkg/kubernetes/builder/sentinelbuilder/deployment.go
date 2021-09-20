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

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

func GenerateSentinelDeployment(rf *databasesv1.RedisFailover, selectors map[string]string) *appv1.Deployment {
	name := util.GetSentinelName(rf)
	namespace := rf.Namespace
	if len(selectors) == 0 {
		selectors = MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(util.SentinelRoleName, rf.Name))
	} else {
		selectors = MergeMap(selectors, GenerateSelectorLabels(util.SentinelRoleName, rf.Name))
	}
	labels := MergeMap(GetCommonLabels(rf.Name), GenerateSelectorLabels(util.SentinelRoleName, rf.Name), selectors)

	const configMountPathPrefix = "/conf"
	sentinelCommand := getSentinelCommand(configMountPathPrefix, rf)
	localhost := "127.0.0.1"
	if rf.Spec.Redis.IPFamilyPrefer == corev1.IPv6Protocol {
		localhost = "::1"
	}
	deploy := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: GetOwnerReferenceForRedisFailover(rf),
			Annotations:     util.GenerateRedisRebuildAnnotation(),
		},
		Spec: appv1.DeploymentSpec{
			Replicas: &rf.Spec.Sentinel.Replicas,
			Strategy: appv1.DeploymentStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &appv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "35%"},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selectors,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      selectors,
					Annotations: rf.Spec.Sentinel.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					HostAliases: []corev1.HostAlias{
						{
							IP:        localhost,
							Hostnames: []string{LocalInjectName},
						},
					},
					Affinity:         getAffinity(rf.Spec.Sentinel.Affinity, selectors),
					Tolerations:      rf.Spec.Sentinel.Tolerations,
					NodeSelector:     rf.Spec.Sentinel.NodeSelector,
					SecurityContext:  getSecurityContext(rf.Spec.Sentinel.SecurityContext),
					ImagePullSecrets: rf.Spec.Sentinel.ImagePullSecrets,
					InitContainers: []corev1.Container{
						{
							Name:            "sentinel-config-copy",
							Image:           rf.Spec.Sentinel.Image,
							ImagePullPolicy: pullPolicy(rf.Spec.Sentinel.ImagePullPolicy),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sentinel-config",
									MountPath: "/redis",
								},
								{
									Name:      "sentinel-config-writable",
									MountPath: "/redis-writable",
								},
							},
							Command: []string{
								"cp",
								"-n",
								fmt.Sprintf("/redis/%s", util.SentinelConfigFileName),
								fmt.Sprintf("/redis-writable/%s", util.SentinelConfigFileName),
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "sentinel",
							Image:           rf.Spec.Sentinel.Image,
							ImagePullPolicy: pullPolicy(rf.Spec.Sentinel.ImagePullPolicy),
							Ports: []corev1.ContainerPort{
								{
									Name:          "sentinel",
									ContainerPort: 26379,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "readiness-probe",
									MountPath: "/redis-probe",
								},
								{
									Name:      "sentinel-config-writable",
									MountPath: "/redis",
								},
								{
									Name:      "sentinel-config",
									MountPath: configMountPathPrefix,
								},
							},
							Command: sentinelCommand,
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"sh",
											"-c",
											"redis-cli -h local.inject -p 26379 sentinel flushconfig",
										},
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								PeriodSeconds:    15,
								FailureThreshold: 5,
								TimeoutSeconds:   5,
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"sh",
											"/redis-probe/readiness.sh",
										},
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 30,
								PeriodSeconds:       60,
								FailureThreshold:    5,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "-h", "local.inject", "-p", "26379", "ping"},
									},
								},
							},
							Resources: rf.Spec.Sentinel.Resources,
							SecurityContext: &corev1.SecurityContext{
								ReadOnlyRootFilesystem: pointer.Bool(true),
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sentinel-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: util.GetSentinelName(rf),
									},
								},
							},
						},
						{
							Name: "readiness-probe",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: util.GetSentinelReadinessConfigmap(rf),
									},
								},
							},
						},
						{
							Name: "sentinel-config-writable",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
	if rf.Spec.EnableTLS {
		deploy.Spec.Template.Spec.Volumes = append(deploy.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: RedisTLSVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: util.GetRedisSSLSecretName(rf.Name),
				},
			},
		})
		deploy.Spec.Template.Spec.Containers[0].VolumeMounts = append(deploy.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      RedisTLSVolumeName,
				MountPath: "/tls",
			})
	}
	return deploy
}

func getSentinelCommand(pathPrefix string, rf *databasesv1.RedisFailover) []string {
	if len(rf.Spec.Sentinel.Command) > 0 {
		return rf.Spec.Sentinel.Command
	}

	command := []string{"sh", path.Join(pathPrefix, util.SentinelEntrypoint)}
	if rf.Spec.EnableTLS {
		command = append(command, getSentinelTLSCommand()...)
	}
	return command

}

func getSentinelTLSCommand() []string {
	return []string{
		"--tls-port",
		"26379",
		"--port",
		"0",
		"--tls-cert-file",
		"/tls/tls.crt",
		"--tls-key-file",
		"/tls/tls.key",
		"--tls-ca-cert-file",
		"/tls/ca.crt",
		"--tls-replication",
		"yes",
	}
}

func DiffDeployment(new *appv1.Deployment, old *appv1.Deployment, logger logr.Logger) bool {
	if new.Spec.Replicas != nil && old.Spec.Replicas != nil && *new.Spec.Replicas != *old.Spec.Replicas {
		return true
	}
	if new.Spec.Strategy.Type != old.Spec.Strategy.Type {
		return true
	}
	if new.Spec.Strategy.RollingUpdate != nil && old.Spec.Strategy.RollingUpdate != nil {
		if new.Spec.Strategy.RollingUpdate.MaxUnavailable != nil && old.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			if new.Spec.Strategy.RollingUpdate.MaxUnavailable.Type != old.Spec.Strategy.RollingUpdate.MaxUnavailable.Type {
				return true
			}
			if new.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal != old.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal {
				return true
			}
			if new.Spec.Strategy.RollingUpdate.MaxUnavailable.IntVal != old.Spec.Strategy.RollingUpdate.MaxUnavailable.IntVal {
				return true
			}
		}
	}
	return clusterbuilder.IsPodTemplasteChanged(&new.Spec.Template, &old.Spec.Template, logger)
}
