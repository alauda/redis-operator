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

package actor

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	redisbackup "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/ops/cluster"
	cops "github.com/alauda/redis-operator/pkg/ops/cluster"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ actor.Actor = (*actorEnsureResource)(nil)

func NewEnsureResourceActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorEnsureResource{
		client: client,
		logger: logger,
	}
}

type actorEnsureResource struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorEnsureResource) SupportedCommands() []actor.Command {
	return []actor.Command{cluster.CommandEnsureResource}
}

// Do
func (a *actorEnsureResource) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	cluster := val.(types.RedisClusterInstance)
	logger := a.logger.WithName(cops.CommandEnsureResource.String()).WithValues("namespace", cluster.GetNamespace(), "name", cluster.GetName())

	if cluster.Definition().Spec.ServiceName == "" {
		cluster.Definition().Spec.ServiceName = cluster.GetName()
	}
	if (cluster.Definition().Spec.Annotations != nil) && cluster.Definition().Spec.Annotations[config.PAUSE_ANNOTATION_KEY] != "" {
		if ret := a.pauseStatefulSet(ctx, cluster, logger); ret != nil {
			return ret
		}
		if ret := a.pauseBackupCronJob(ctx, cluster, logger); ret != nil {
			return ret
		}
		return actor.NewResult(cops.CommandPaused)
	}

	if ret := a.ensureServiceAccount(ctx, cluster, logger); ret != nil {
		return ret
	}

	if ret := a.ensureService(ctx, cluster, logger); ret != nil {
		return ret
	}

	if ret := a.ensureTLS(ctx, cluster, logger); ret != nil {
		return ret
	}

	if ret := a.ensureBackupSchedule(ctx, cluster, logger); ret != nil {
		return ret
	}

	if ret := a.ensureConfigMap(ctx, cluster, logger); ret != nil {
		return ret
	}

	if ret := a.ensureStatefulset(ctx, cluster, logger); ret != nil {
		return ret
	}
	// if ret := a.ensureRedisUser(ctx, cluster, logger); ret != nil {
	// 	return ret
	// }
	return nil
}

/*
func (a *actorEnsureResource) ensureRedisUser(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cr := cluster.Definition()
	if cluster.Users().GetOpUser() != nil {
		redisUser := clusterbuilder.GenerateClusterOperatorsRedisUser(cluster, cluster.Users().GetOpUser().GetPassword().SecretName)
		if err := a.client.CreateIfNotExistsRedisUser(ctx, &redisUser); err != nil {
			logger.Error(err, "create redis user failed", "target", client.ObjectKeyFromObject(&redisUser))
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}
	if cluster.Users().GetOpUser() != nil {
		redisUser := clusterbuilder.GenerateClusterDefaultRedisUser(cr, cluster.Users().GetDefaultUser().GetPassword().SecretName)
		if err := a.client.CreateIfNotExistsRedisUser(ctx, &redisUser); err != nil {
			logger.Error(err, "create redis user failed", "target", client.ObjectKeyFromObject(&redisUser))
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}

	return nil
}
*/

func (a *actorEnsureResource) pauseStatefulSet(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cr := cluster.Definition()
	labels := clusterbuilder.GetClusterLabels(cr.Name, nil)
	stss, err := a.client.ListStatefulSetByLabels(ctx, cr.Namespace, labels)
	if err != nil {
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	if len(stss.Items) == 0 {
		return nil
	}
	for _, sts := range stss.Items {
		if *sts.Spec.Replicas == 0 {
			continue
		}
		*sts.Spec.Replicas = 0
		if err = a.client.UpdateStatefulSet(ctx, cr.Namespace, &sts); err != nil {
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) pauseBackupCronJob(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cr := cluster.Definition()
	selectorLabels := map[string]string{
		"redis.kun/name": cr.Name,
		"managed-by":     "redis-cluster-operator",
	}
	jobsRes, err := a.client.ListCronJobs(ctx, cr.GetNamespace(), client.ListOptions{
		LabelSelector: labels.SelectorFromSet(selectorLabels),
	})
	if err != nil {
		logger.Error(err, "load cronjobs failed")
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	for _, val := range jobsRes.Items {
		if err := a.client.DeleteCronJob(ctx, cr.GetNamespace(), val.Name); err != nil {
			logger.Error(err, "delete cronjob failed", "target", client.ObjectKeyFromObject(&val))
		}
	}
	return nil
}

// ensureServiceAccount
func (a *actorEnsureResource) ensureServiceAccount(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cr := cluster.Definition()

	sa := clusterbuilder.NewServiceAccount(cr)
	role := clusterbuilder.NewRole(cr)
	binding := clusterbuilder.NewRoleBinding(cr)
	clusterRole := clusterbuilder.NewClusterRole(cr)
	clusterBinding := clusterbuilder.NewClusterRoleBinding(cr)

	if err := a.client.CreateOrUpdateServiceAccount(ctx, cluster.GetNamespace(), sa); err != nil {
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	if err := a.client.CreateOrUpdateRole(ctx, cluster.GetNamespace(), role); err != nil {
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	if err := a.client.CreateOrUpdateRoleBinding(ctx, cluster.GetNamespace(), binding); err != nil {
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	if err := a.client.CreateOrUpdateClusterRole(ctx, clusterRole); err != nil {
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	if oldClusterRb, err := a.client.GetClusterRoleBinding(ctx, clusterBinding.Name); err != nil {
		if errors.IsNotFound(err) {
			if err := a.client.CreateClusterRoleBinding(ctx, clusterBinding); err != nil {
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}
		} else {
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	} else {
		exists := false
		for _, sub := range oldClusterRb.Subjects {
			if sub.Namespace == cluster.GetNamespace() {
				exists = true
			}
		}
		if !exists && len(oldClusterRb.Subjects) > 0 {
			oldClusterRb.Subjects = append(oldClusterRb.Subjects,
				rbacv1.Subject{Kind: "ServiceAccount",
					Name:      util.RedisBackupServiceAccountName,
					Namespace: cluster.GetNamespace()},
			)
			err := a.client.CreateOrUpdateClusterRoleBinding(ctx, oldClusterRb)
			if err != nil {
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}
		}
	}
	return nil
}

// ensureConfigMap
func (a *actorEnsureResource) ensureConfigMap(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cm, err := clusterbuilder.NewConfigMapForCR(cluster)
	if err != nil {
		logger.Error(err, "new configmap failed")
		// this should not return errors, else there must be a breaking error
		return actor.NewResultWithError(cops.CommandAbort, err)
	}
	if oldCm, err := a.client.GetConfigMap(ctx, cluster.GetNamespace(), cm.GetName()); errors.IsNotFound(err) {
		if err := a.client.CreateConfigMap(ctx, cluster.GetNamespace(), cm); err != nil {
			logger.Error(err, "update configmap failed")
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	} else if !reflect.DeepEqual(oldCm.Data, cm.Data) {
		if err := a.client.UpdateConfigMap(ctx, cluster.GetNamespace(), cm); err != nil {
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}
	return nil
}

// ensureTLS
func (a *actorEnsureResource) ensureTLS(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	if !cluster.Definition().Spec.EnableTLS || !cluster.Version().IsTLSSupported() {
		return nil
	}
	cert := clusterbuilder.NewCertificate(cluster.Definition())

	oldCert, err := a.client.GetCertificate(ctx, cluster.GetNamespace(), cert.GetName())
	if err != nil && !errors.IsNotFound(err) {
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}

	if err := a.client.CreateIfNotExistsCertificate(ctx, cluster.GetNamespace(), cert); err != nil {
		logger.Error(err, "request for certificate failed")
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}

	var (
		found      = false
		secretName = clusterbuilder.GetRedisSSLSecretName(cluster.GetName())
	)
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second * time.Duration(i))

		if secret, _ := a.client.GetSecret(ctx, cluster.GetNamespace(), secretName); secret != nil {
			found = true
			break
		}

		// check when the certificate created
		if time.Since(oldCert.GetCreationTimestamp().Time) > time.Minute*5 {
			return actor.NewResultWithError(cops.CommandAbort, fmt.Errorf("issue for tls certificate failed, please check the cert-manager"))
		}
	}
	if !found {
		return actor.NewResult(cops.CommandRequeue)
	}
	return nil
}

// ensureStatefulset
func (a *actorEnsureResource) ensureStatefulset(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cr := cluster.Definition()
	isRestoring := func(sts *appv1.StatefulSet) bool {
		if cr.Spec.Restore.BackupName != "" {
			if sts == nil {
				return true
			}
			if util.GetContainerByName(&sts.Spec.Template.Spec, clusterbuilder.RestoreContainerName) == nil {
				return true
			}
		}
		return false
	}

	var (
		err    error
		backup *redisbackup.RedisClusterBackup
		// isAllACLSupported is used to make sure 5=>6 upgrade can to failover succeed
		isAllACLSupported = true
		updated           = false
	)
	if cr.Spec.Restore.BackupName != "" {
		if backup, err = a.client.GetRedisClusterBackup(ctx, cr.GetNamespace(), cr.Spec.Restore.BackupName); err != nil {
			logger.Error(err, "load redis cluster backup cr failed",
				"target", client.ObjectKey{Namespace: cr.GetNamespace(), Name: cr.Spec.Restore.BackupName})
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}

__end__:
	for _, shard := range cluster.Shards() {
		for _, node := range shard.Nodes() {
			if !node.CurrentVersion().IsACLSupported() || !node.IsACLApplied() {
				isAllACLSupported = false
				break __end__
			}
		}
	}

	for i := 0; i < int(cr.Spec.MasterSize); i++ {
		// statefulset
		name := clusterbuilder.ClusterStatefulSetName(cr.GetName(), i)
		if oldSts, err := a.client.GetStatefulSet(ctx, cr.GetNamespace(), name); errors.IsNotFound(err) {
			newSts, err := clusterbuilder.NewStatefulSetForCR(cluster, false, isAllACLSupported, backup, i)
			if err != nil {
				logger.Error(err, "generate statefulset failed")
				return actor.NewResultWithError(cops.CommandAbort, err)
			}
			if err := a.client.CreateStatefulSet(ctx, cr.GetNamespace(), newSts); err != nil {
				logger.Error(err, "create statefulset failed")
				return actor.NewResultWithError(cops.CommandAbort, err)
			}
			updated = true
		} else if err != nil {
			logger.Error(err, "get statefulset failed")
			return actor.NewResultWithError(cops.CommandRequeue, err)
		} else if newSts, err := clusterbuilder.NewStatefulSetForCR(cluster, isRestoring(oldSts), isAllACLSupported, backup, i); err != nil {
			logger.Error(err, "build init statefulset failed")
			return actor.NewResultWithError(cops.CommandAbort, err)
		} else {
			// overwrite persist fields
			// keep old affinity for topolvm cases
			newSts.Spec.Template.Spec.Affinity = oldSts.Spec.Template.Spec.Affinity
			// keep old selector for upgrade
			if !reflect.DeepEqual(oldSts.Spec.Selector.MatchLabels, newSts.Spec.Selector.MatchLabels) {
				newSts.Spec.Selector.MatchLabels = oldSts.Spec.Selector.MatchLabels
			}
			// keep labels for pvc
			for i, npvc := range newSts.Spec.VolumeClaimTemplates {
				if oldPvc := util.GetVolumeClaimTemplatesByName(oldSts.Spec.VolumeClaimTemplates, npvc.Name); oldPvc != nil {
					newSts.Spec.VolumeClaimTemplates[i].Labels = oldPvc.Labels
				}
			}

			// merge restart annotations, if statefulset is more new, not restart statefulset
			newSts.Spec.Template.Annotations = MergeAnnotations(newSts.Spec.Template.Annotations,
				oldSts.Spec.Template.Annotations)

			if clusterbuilder.IsRedisClusterStatefulsetChanged(newSts, oldSts, logger) {
				if err := a.client.UpdateStatefulSet(ctx, cr.GetNamespace(), newSts); err != nil {
					logger.Error(err, "update statefulset failed", "target", client.ObjectKeyFromObject(newSts))
					return actor.NewResultWithError(cops.CommandRequeue, err)
				}
				updated = true
			}
		}
	}

	if updated {
		return actor.NewResult(cops.CommandRequeue)
	}
	return nil
}

// ensureService ensure other services
func (a *actorEnsureResource) ensureService(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cr := cluster.Definition()

	for i := 0; i < int(cr.Spec.MasterSize); i++ {
		// init headless service
		// use serviceName and selectors from statefulset
		svc := clusterbuilder.NewHeadlessSvcForCR(cr, i)
		// TODO: check if service changed
		if err := a.client.CreateIfNotExistsService(ctx, cr.GetNamespace(), svc); err != nil {
			logger.Error(err, "create headless service failed", "target", client.ObjectKeyFromObject(svc))
		}
	}

	// common service
	svc := clusterbuilder.NewServiceForCR(cr)
	if err := a.client.CreateIfNotExistsService(ctx, cr.GetNamespace(), svc); err != nil {
		logger.Error(err, "create service failed", "target", client.ObjectKeyFromObject(svc))
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}

	if cr.Spec.Expose.EnableNodePort {
		// pod svc
		if cr.Spec.Expose.DataStorageNodePortSequence != "" {
			logger.Info("EnsureRedisNodePortService DataStorageNodePortSequence", "Namespace", cr.Namespace, "Name", cr.Name)
			if err := a.ensureRedisNodePortService(ctx, cluster, logger); err != nil {
				return err
			}
		}
		nodePortSvc := clusterbuilder.NewNodePortServiceForCR(cr)
		if err := a.client.CreateIfNotExistsService(ctx, cr.GetNamespace(), nodePortSvc); err != nil {
			logger.Error(err, "create nodeport service failed", "target", client.ObjectKeyFromObject(nodePortSvc))
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisNodePortService(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cr := cluster.Definition()

	configedPorts, err := util.ParsePortSequence(cr.Spec.Expose.DataStorageNodePortSequence)
	if err != nil {
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	getClientPort := func(svc *corev1.Service) int32 {
		if port := getServicePortByName(svc, "client"); port != nil {
			return port.NodePort
		}
		return 0
	}
	getGossipPort := func(svc *corev1.Service) int32 {
		if port := getServicePortByName(svc, "gossip"); port != nil {
			return port.NodePort
		}
		return 0
	}

	fetchAllPodBindedServices := func() ([]corev1.Service, *actor.ActorResult) {
		var (
			services []corev1.Service
		)

		labels := clusterbuilder.GetClusterLabels(cr.Name, nil)
		if svcRes, err := a.client.GetServiceByLabels(ctx, cr.Namespace, labels); err != nil {
			return nil, actor.NewResultWithError(cops.CommandRequeue, err)
		} else {
			// ignore services without pod selector
			for _, svc := range svcRes.Items {
				if svc.Spec.Selector["statefulset.kubernetes.io/pod-name"] != "" {
					services = append(services, svc)
				}
			}
		}
		return services, nil
	}

	serviceNameRange := map[string]struct{}{}
	for shard := 0; shard < int(cr.Spec.MasterSize); shard++ {
		for replica := 0; replica < int(cr.Spec.ClusterReplicas)+1; replica++ {
			serviceName := clusterbuilder.ClusterNodeSvcName(cr.Name, shard, replica)
			serviceNameRange[serviceName] = struct{}{}
		}
	}

	labels := clusterbuilder.GetClusterLabels(cr.Name, nil)
	clusterNodePortSvc, err := a.client.GetService(ctx, cr.GetNamespace(), clusterbuilder.RedisNodePortSvcName(cr.Name))
	if err != nil && !errors.IsNotFound(err) {
		a.logger.Error(err, "get cluster nodeport service failed", "target", clusterbuilder.RedisNodePortSvcName(cr.Name))
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}

	// the whole process is divided into three steps:
	// 1. delete service not in nodeport range
	// 2. create new service without gossip port
	// 3. update existing service and restart pod (only one pod is restarted at a same time for each shard)
	// 4. check again if all gossip port is added

	services, ret := fetchAllPodBindedServices()
	if ret != nil {
		return ret
	}

	// 1. delete service not in nodeport range
	//
	// when pod not exists and service not in nodeport range, delete service
	// NOTE: only delete service whose pod is not found
	//       let statefulset auto scale up/down for pods
	for _, svc := range services {
		if _, exists := serviceNameRange[svc.Name]; !exists || !isPortInRange(configedPorts, getClientPort(&svc)) {
			pod, err := a.client.GetPod(ctx, svc.Namespace, svc.Name)
			if errors.IsNotFound(err) {
				logger.Info("release nodeport service", "service", svc.Name, "port", getClientPort(&svc))
				if err = a.client.DeleteService(ctx, svc.Namespace, svc.Name); err != nil {
					return actor.NewResultWithError(cops.CommandRequeue, err)
				}
			} else if err != nil {
				logger.Error(err, "get pods failed", "target", client.ObjectKeyFromObject(pod))
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}
		}
	}
	if clusterNodePortSvc != nil && isPortInRange(configedPorts, clusterNodePortSvc.Spec.Ports[0].NodePort) {
		// delete cluster nodeport service
		if err := a.client.DeleteService(ctx, cr.GetNamespace(), clusterNodePortSvc.Name); err != nil {
			a.logger.Error(err, "delete service failed", "target", client.ObjectKeyFromObject(clusterNodePortSvc))
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}

	if services, ret = fetchAllPodBindedServices(); ret != nil {
		return ret
	}

	// 2. create new service
	var (
		newPorts                 []int32
		bindedNodeports          []int32
		needUpdateServices       []*corev1.Service
		needUpdateGossipServices []*corev1.Service
	)
	for _, svc := range services {
		bindedNodeports = append(bindedNodeports, getClientPort(&svc), getGossipPort(&svc))
	}
	if clusterNodePortSvc != nil && len(clusterNodePortSvc.Spec.Ports) > 0 {
		bindedNodeports = append(bindedNodeports, clusterNodePortSvc.Spec.Ports[0].NodePort)
	}
	// filter used ports
	for _, port := range configedPorts {
		if !isPortInRange(bindedNodeports, port) {
			newPorts = append(newPorts, port)
		}
	}
	for shard := 0; shard < int(cr.Spec.MasterSize); shard++ {
		for replica := 0; replica < int(cr.Spec.ClusterReplicas)+1; replica++ {
			serviceName := clusterbuilder.ClusterNodeSvcName(cr.Name, shard, replica)
			oldService, err := a.client.GetService(ctx, cr.Namespace, serviceName)
			if errors.IsNotFound(err) {
				if len(newPorts) == 0 {
					continue
				}
				port := newPorts[0]
				svc := clusterbuilder.NewNodeportSvc(cr, serviceName, labels, port)
				if err = a.client.CreateService(ctx, svc.Namespace, svc); err != nil {
					a.logger.Error(err, "create nodeport service failed", "target", client.ObjectKeyFromObject(svc))
					return actor.NewResultWithValue(cops.CommandRequeue, err)
				}
				newPorts = newPorts[1:]
			} else if err != nil {
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}

			// check old service for compability
			if len(oldService.OwnerReferences) == 0 || oldService.OwnerReferences[0].Kind == "Pod" {
				oldService.OwnerReferences = util.BuildOwnerReferences(cr)
				if err := a.client.UpdateService(ctx, oldService.Namespace, oldService); err != nil {
					a.logger.Error(err, "update nodeport service failed", "target", client.ObjectKeyFromObject(oldService))
					return actor.NewResultWithValue(cops.CommandRequeue, err)
				}
			}
			if isPortInRange(configedPorts, getGossipPort(oldService)) {
				needUpdateGossipServices = append(needUpdateGossipServices, oldService)
			} else if port := getClientPort(oldService); port != 0 && !isPortInRange(configedPorts, port) {
				needUpdateServices = append(needUpdateServices, oldService)
			}
		}
	}

	// 3. update existing service and restart pod (only one pod is restarted at a same time for each shard)
	if len(needUpdateServices) > 0 && len(newPorts) > 0 {
		port, svc := newPorts[0], needUpdateServices[0]
		if sp := getServicePortByName(svc, "client"); sp != nil {
			sp.NodePort = port
		}

		// NOTE: here not make sure the failover success, because the nodeport updated, the communication will be failed
		//       in k8s, the nodeport can still access for sometime after the nodeport updated
		//
		// update service
		if err = a.client.UpdateService(ctx, svc.Namespace, svc); err != nil {
			a.logger.Error(err, "update nodeport service failed", "target", client.ObjectKeyFromObject(svc), "port", port)
			return actor.NewResultWithValue(cops.CommandRequeue, err)
		}
		if pod, _ := a.client.GetPod(ctx, cr.Namespace, svc.Spec.Selector["statefulset.kubernetes.io/pod-name"]); pod != nil {
			if err := a.client.DeletePod(ctx, cr.Namespace, pod.Name); err != nil {
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}
			return actor.NewResult(cops.CommandRequeue)
		}
	}
	for _, svc := range needUpdateGossipServices {
		ports := svc.Spec.Ports[0:0]
		for _, port := range svc.Spec.Ports {
			if port.Name == "client" {
				ports = append(ports, port)
				break
			}
		}
		svc.Spec.Ports = ports
		if err = a.client.UpdateService(ctx, svc.Namespace, svc); err != nil {
			a.logger.Error(err, "update nodeport service failed", "target", client.ObjectKeyFromObject(svc))
			return actor.NewResultWithValue(cops.CommandRequeue, err)
		}
		// update gossip service
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "gossip", Port: 16379})
		if err = a.client.UpdateService(ctx, svc.Namespace, svc); err != nil {
			a.logger.Error(err, "update nodeport service failed", "target", client.ObjectKeyFromObject(svc))
			return actor.NewResultWithValue(cops.CommandRequeue, err)
		}
		if pod, _ := a.client.GetPod(ctx, cr.Namespace, svc.Spec.Selector["statefulset.kubernetes.io/pod-name"]); pod != nil {
			if err := a.client.DeletePod(ctx, cr.Namespace, pod.Name); err != nil {
				a.logger.Error(err, "delete pod failed", "target", client.ObjectKeyFromObject(pod))
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}
			return actor.NewResult(cops.CommandRequeue)
		}
	}

	// 4. check again if all gossip port is added
	for shard := 0; shard < int(cr.Spec.MasterSize); shard++ {
		for replica := 0; replica < int(cr.Spec.ClusterReplicas)+1; replica++ {
			serviceName := clusterbuilder.ClusterNodeSvcName(cr.Name, shard, replica)
			if svc, err := a.client.GetService(ctx, cr.Namespace, serviceName); errors.IsNotFound(err) {
				continue
			} else if err != nil {
				a.logger.Error(err, "get service failed", "target", client.ObjectKeyFromObject(svc))
			} else if port := getGossipPort(svc); port == 0 && len(svc.Spec.Ports) == 1 {
				// update gossip service
				svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "gossip", Port: 16379})
				if err = a.client.UpdateService(ctx, svc.Namespace, svc); err != nil {
					a.logger.Error(err, "update nodeport service failed", "target", client.ObjectKeyFromObject(svc))
					return actor.NewResultWithValue(cops.CommandRequeue, err)
				}
				if pod, _ := a.client.GetPod(ctx, cr.Namespace, svc.Spec.Selector["statefulset.kubernetes.io/pod-name"]); pod != nil {
					if err := a.client.DeletePod(ctx, cr.Namespace, pod.Name); err != nil {
						a.logger.Error(err, "delete pod failed", "target", client.ObjectKeyFromObject(pod))
						return actor.NewResultWithError(cops.CommandRequeue, err)
					}
					return actor.NewResult(cops.CommandRequeue)
				}
			}
		}
	}
	return nil
}

// ensureBackupSchedule
func (a *actorEnsureResource) ensureBackupSchedule(ctx context.Context, cluster types.RedisClusterInstance, logger logr.Logger) *actor.ActorResult {
	cr := cluster.Definition()

	for _, schedule := range cr.Spec.Backup.Schedule {
		ls := map[string]string{
			"redis.kun/name":         cr.Name,
			"redis.kun/scheduleName": schedule.Name,
		}
		res, err := a.client.ListRedisClusterBackups(ctx, cr.GetNamespace(), client.ListOptions{
			LabelSelector: labels.SelectorFromSet(ls),
		})
		if err != nil {
			logger.Error(err, "load backups failed", "target", client.ObjectKeyFromObject(cr))
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
		sort.SliceStable(res.Items, func(i, j int) bool {
			return res.Items[i].GetCreationTimestamp().After(res.Items[j].GetCreationTimestamp().Time)
		})
		for i := len(res.Items) - 1; i >= int(schedule.Keep); i-- {
			item := res.Items[i]
			if err := a.client.DeleteRedisClusterBackup(ctx, item.GetNamespace(), item.GetName()); err != nil {
				logger.V(2).Error(err, "clean old backup failed", "target", client.ObjectKeyFromObject(&item))
			}
		}
	}

	// check backup schedule
	// ref: https://jira.alauda.cn/browse/MIDDLEWARE-21009
	deprecatedSelectorLabels := map[string]string{
		"redis.kun/name": cr.Name,
		"managed-by":     "redis-cluster-operator",
	}
	deprecatedJobsRes, err := a.client.ListCronJobs(ctx, cr.GetNamespace(), client.ListOptions{
		LabelSelector: labels.SelectorFromSet(deprecatedSelectorLabels),
	})
	if err != nil {
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}

	for _, v := range deprecatedJobsRes.Items {
		logger.Info("delete deprecated cronjob", "target", client.ObjectKeyFromObject(&v))
		err := a.client.DeleteCronJob(ctx, cr.GetNamespace(), v.Name)
		if err != nil {
			logger.Error(err, "delete cronjob failed", "target", client.ObjectKeyFromObject(&v))
		}
	}
	selectorLabels := map[string]string{
		"redisclusterbackups.redis.middleware.alauda.io/instanceName": cr.Name,
		"managed-by": "redis-cluster-operator",
	}
	jobsRes, err := a.client.ListCronJobs(ctx, cr.GetNamespace(), client.ListOptions{
		LabelSelector: labels.SelectorFromSet(selectorLabels),
	})
	if err != nil {
		logger.Error(err, "load cronjobs failed")
		return actor.NewResultWithError(cops.CommandRequeue, err)
	}
	jobsMap := map[string]*batchv1.CronJob{}
	for _, item := range jobsRes.Items {
		jobsMap[item.Name] = &item
	}
	for _, sched := range cr.Spec.Backup.Schedule {
		sc := v1alpha1.Schedule{
			Name:              sched.Name,
			Schedule:          sched.Schedule,
			Keep:              sched.Keep,
			KeepAfterDeletion: sched.KeepAfterDeletion,
			Storage:           v1alpha1.RedisBackupStorage(sched.Storage),
			Target:            sched.Target,
		}
		job := clusterbuilder.NewRedisClusterBackupCronJobFromCR(sc, cr)
		if oldJob := jobsMap[job.GetName()]; oldJob != nil {
			if clusterbuilder.IsCronJobChanged(job, oldJob, logger) {
				if err := a.client.UpdateCronJob(ctx, oldJob.GetNamespace(), job); err != nil {
					logger.Error(err, "update cronjob failed", "target", client.ObjectKeyFromObject(oldJob))
					return actor.NewResultWithError(cops.CommandRequeue, err)
				}
			}
			jobsMap[job.GetName()] = nil
		} else if err := a.client.CreateCronJob(ctx, cr.GetNamespace(), job); err != nil {
			logger.Error(err, "create redisbackup cronjob failed", "target", client.ObjectKeyFromObject(job))
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}

	for name, val := range jobsMap {
		if val == nil {
			continue
		}
		if err := a.client.DeleteCronJob(ctx, cr.GetNamespace(), name); err != nil {
			logger.Error(err, "delete cronjob failed", "target", client.ObjectKeyFromObject(val))
		}
	}
	return nil
}

// NOTE: removed proxy ensure, remove proxy ensure to rds

const (
	redisRestartAnnotation = "kubectl.kubernetes.io/restartedAt"
)

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

func getServicePortByName(svc *corev1.Service, name string) *corev1.ServicePort {
	for i := range svc.Spec.Ports {
		if svc.Spec.Ports[i].Name == name {
			return &svc.Spec.Ports[i]
		}
	}
	return nil
}

func isPortInRange(ports []int32, p int32) bool {
	if p == 0 {
		return false
	}
	for _, port := range ports {
		if p == port {
			return true
		}
	}
	return false
}
