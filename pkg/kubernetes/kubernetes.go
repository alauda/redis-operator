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

package kubernetes

import (
	"github.com/alauda/redis-operator/pkg/kubernetes/clientset"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Service is the kubernetes service entrypoint.
type ClientSet interface {
	clientset.Certificate
	clientset.ConfigMap
	clientset.CronJob
	clientset.Deployment
	clientset.Job
	clientset.NameSpaces
	clientset.Pod
	clientset.PodDisruptionBudget
	clientset.PVC
	clientset.RBAC
	clientset.Secret
	clientset.Service
	clientset.ServiceAccount
	clientset.StatefulSet
	clientset.ServiceMonitor

	clientset.RedisFailover
	clientset.RedisSentinel
	clientset.DistributedRedisCluster
	clientset.Node
	clientset.RedisUser
	Client() client.Client
}
