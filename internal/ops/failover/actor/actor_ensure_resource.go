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
	"slices"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/api/core/helper"
	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/builder/clusterbuilder"
	"github.com/alauda/redis-operator/internal/builder/failoverbuilder"
	"github.com/alauda/redis-operator/internal/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/internal/config"
	ops "github.com/alauda/redis-operator/internal/ops/failover"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/samber/lo"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ actor.Actor = (*actorEnsureResource)(nil)

func init() {
	actor.Register(core.RedisSentinel, NewEnsureResourceActor)
}

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
	return []actor.Command{ops.CommandEnsureResource}
}

func (a *actorEnsureResource) Version() *semver.Version {
	return semver.MustParse("3.18.0")
}

// Do
func (a *actorEnsureResource) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	logger := val.Logger().WithValues("actor", ops.CommandEnsureResource.String())

	inst := val.(types.RedisFailoverInstance)
	if (inst.Definition().Spec.Redis.PodAnnotations != nil) && inst.Definition().Spec.Redis.PodAnnotations[config.PAUSE_ANNOTATION_KEY] != "" {
		if ret := a.pauseStatefulSet(ctx, inst, logger); ret != nil {
			return ret
		}
		if ret := a.pauseSentinel(ctx, inst, logger); ret != nil {
			return ret
		}
		return actor.Pause()
	}

	if ret := a.ensureRedisSSL(ctx, inst, logger); ret != nil {
		return ret
	}
	if ret := a.ensureServiceAccount(ctx, inst, logger); ret != nil {
		return ret
	}
	if ret := a.ensureSentinel(ctx, inst, logger); ret != nil {
		return ret
	}
	if ret := a.ensureService(ctx, inst, logger); ret != nil {
		return ret
	}
	if ret := a.ensureConfigMap(ctx, inst, logger); ret != nil {
		return ret
	}
	if ret := a.ensureRedisStatefulSet(ctx, inst, logger); ret != nil {
		return ret
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisStatefulSet(ctx context.Context, inst types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	var (
		err      error
		cr       = inst.Definition()
		selector = inst.Selector()
		// isAllACLSupported is used to make sure 5=>6 upgrade can to failover succeed
		isAllACLSupported = inst.IsACLAppliedToAll()
	)

	// ensure inst statefulSet
	if ret := a.ensurePodDisruptionBudget(ctx, cr, logger, selector); ret != nil {
		return ret
	}

	sts := failoverbuilder.GenerateRedisStatefulSet(inst, selector, isAllACLSupported)
	oldSts, err := a.client.GetStatefulSet(ctx, cr.Namespace, sts.Name)
	if errors.IsNotFound(err) {
		if err := a.client.CreateStatefulSet(ctx, cr.Namespace, sts); err != nil {
			return actor.RequeueWithError(err)
		}
		return nil
	} else if err != nil {
		logger.Error(err, "get statefulset failed", "target", client.ObjectKeyFromObject(sts))
		return actor.RequeueWithError(err)
	}

	// TODO: remove this and reset rds pvc resize logic
	// keep old volumeClaimTemplates
	sts.Spec.VolumeClaimTemplates = oldSts.Spec.VolumeClaimTemplates

	if clusterbuilder.IsStatefulsetChanged(sts, oldSts, logger) {
		if *oldSts.Spec.Replicas > *sts.Spec.Replicas {
			// scale down
			oldSts.Spec.Replicas = sts.Spec.Replicas
			if err := a.client.UpdateStatefulSet(ctx, cr.Namespace, oldSts); err != nil {
				logger.Error(err, "scale down statefulset failed", "target", client.ObjectKeyFromObject(oldSts))
				return actor.RequeueWithError(err)
			}
			time.Sleep(time.Second * 3)
		}

		// patch pods with new labels in selector
		pods, err := inst.RawNodes(ctx)
		if err != nil {
			logger.Error(err, "get pods failed")
			return actor.RequeueWithError(err)
		}
		for _, item := range pods {
			pod := item.DeepCopy()
			pod.Labels = lo.Assign(pod.Labels, inst.Selector())
			if !reflect.DeepEqual(pod.Labels, item.Labels) {
				if err := a.client.UpdatePod(ctx, pod.GetNamespace(), pod); err != nil {
					logger.Error(err, "patch pod label failed", "target", client.ObjectKeyFromObject(pod))
					return actor.RequeueWithError(err)
				}
			}
		}
		time.Sleep(time.Second * 3)
		if err := a.client.DeleteStatefulSet(ctx, cr.Namespace, sts.Name,
			client.PropagationPolicy(metav1.DeletePropagationOrphan)); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "delete old statefulset failed", "target", client.ObjectKeyFromObject(sts))
			return actor.RequeueWithError(err)
		}
		if err = a.client.CreateStatefulSet(ctx, cr.Namespace, sts); err != nil {
			logger.Error(err, "update statefulset failed", "target", client.ObjectKeyFromObject(sts))
			return actor.RequeueWithError(err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensurePodDisruptionBudget(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	pdb := failoverbuilder.NewPodDisruptionBudgetForCR(rf, selectors)

	if oldPdb, err := a.client.GetPodDisruptionBudget(context.TODO(), rf.Namespace, pdb.Name); errors.IsNotFound(err) {
		if err := a.client.CreatePodDisruptionBudget(ctx, rf.Namespace, pdb); err != nil {
			return actor.RequeueWithError(err)
		}
	} else if err != nil {
		return actor.RequeueWithError(err)
	} else if !reflect.DeepEqual(oldPdb.Spec.Selector, pdb.Spec.Selector) {
		oldPdb.Labels = pdb.Labels
		oldPdb.Spec.Selector = pdb.Spec.Selector
		if err := a.client.UpdatePodDisruptionBudget(ctx, rf.Namespace, oldPdb); err != nil {
			return actor.RequeueWithError(err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureConfigMap(ctx context.Context, inst types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := inst.Definition()
	selector := inst.Selector()
	// ensure Redis configMap
	if ret := a.ensureRedisConfigMap(ctx, inst, logger, selector); ret != nil {
		return ret
	}

	if inst.Version().IsACLSupported() {
		if ret := failoverbuilder.NewFailoverAclConfigMap(cr, inst.Users().Encode(true)); ret != nil {
			if err := a.client.CreateIfNotExistsConfigMap(ctx, cr.Namespace, ret); err != nil {
				return actor.RequeueWithError(err)
			}
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisConfigMap(ctx context.Context, st types.RedisFailoverInstance, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	rf := st.Definition()
	configMap, err := failoverbuilder.NewRedisConfigMap(st, selectors)
	if err != nil {
		return actor.RequeueWithError(err)
	}
	if err := a.client.CreateIfNotExistsConfigMap(ctx, rf.Namespace, configMap); err != nil {
		return actor.RequeueWithError(err)
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisSSL(ctx context.Context, inst types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	rf := inst.Definition()
	if !rf.Spec.Redis.EnableTLS {
		return nil
	}

	cert := failoverbuilder.NewCertificate(rf, inst.Selector())
	if err := a.client.CreateIfNotExistsCertificate(ctx, rf.Namespace, cert); err != nil {
		return actor.RequeueWithError(err)
	}
	oldCert, err := a.client.GetCertificate(ctx, rf.Namespace, cert.GetName())
	if err != nil && !errors.IsNotFound(err) {
		return actor.RequeueWithError(err)
	}

	var (
		secretName = builder.GetRedisSSLSecretName(rf.Name)
		secret     *corev1.Secret
	)
	for i := 0; i < 5; i++ {
		if secret, _ = a.client.GetSecret(ctx, rf.Namespace, secretName); secret != nil {
			break
		}
		// check when the certificate created
		if time.Since(oldCert.GetCreationTimestamp().Time) > time.Minute*5 {
			return actor.NewResultWithError(ops.CommandAbort, fmt.Errorf("issue for tls certificate failed, please check the cert-manager"))
		}
		time.Sleep(time.Second * time.Duration(i+1))
	}
	if secret == nil {
		return actor.NewResult(ops.CommandRequeue)
	}
	return nil
}

func (a *actorEnsureResource) ensureServiceAccount(ctx context.Context, inst types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := inst.Definition()
	sa := clusterbuilder.NewServiceAccount(cr)
	role := clusterbuilder.NewRole(cr)
	binding := clusterbuilder.NewRoleBinding(cr)
	clusterRole := clusterbuilder.NewClusterRole(cr)
	clusterRoleBinding := clusterbuilder.NewClusterRoleBinding(cr)

	if err := a.client.CreateOrUpdateServiceAccount(ctx, inst.GetNamespace(), sa); err != nil {
		logger.Error(err, "create service account failed", "target", client.ObjectKeyFromObject(sa))
		return actor.RequeueWithError(err)
	}
	if err := a.client.CreateOrUpdateRole(ctx, inst.GetNamespace(), role); err != nil {
		return actor.RequeueWithError(err)
	}
	if err := a.client.CreateOrUpdateRoleBinding(ctx, inst.GetNamespace(), binding); err != nil {
		return actor.RequeueWithError(err)
	}
	if err := a.client.CreateOrUpdateClusterRole(ctx, clusterRole); err != nil {
		return actor.RequeueWithError(err)
	}
	if oldClusterRb, err := a.client.GetClusterRoleBinding(ctx, clusterRoleBinding.Name); err != nil {
		if errors.IsNotFound(err) {
			if err := a.client.CreateClusterRoleBinding(ctx, clusterRoleBinding); err != nil {
				return actor.RequeueWithError(err)
			}
		} else {
			return actor.RequeueWithError(err)
		}
	} else {
		exists := false
		for _, sub := range oldClusterRb.Subjects {
			if sub.Namespace == inst.GetNamespace() {
				exists = true
			}
		}
		if !exists && len(oldClusterRb.Subjects) > 0 {
			oldClusterRb.Subjects = append(oldClusterRb.Subjects,
				rbacv1.Subject{Kind: "ServiceAccount",
					Name:      clusterbuilder.RedisInstanceServiceAccountName,
					Namespace: inst.GetNamespace()},
			)
			err := a.client.CreateOrUpdateClusterRoleBinding(ctx, oldClusterRb)
			if err != nil {
				return actor.RequeueWithError(err)
			}
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureSentinel(ctx context.Context, inst types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	if !inst.IsBindedSentinel() {
		return nil
	}

	{
		// COMP: patch labels for old deployment pods
		// TODO: remove in 3.22
		selectors := sentinelbuilder.GenerateSelectorLabels(sentinelbuilder.RedisArchRoleSEN, inst.GetName())
		name := sentinelbuilder.GetSentinelStatefulSetName(inst.GetName())
		if ret, err := a.client.GetDeploymentPods(ctx, inst.GetNamespace(), name); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "get sentinel deployment pods failed", "target", util.ObjectKey(inst.GetNamespace(), name))
			return actor.RequeueWithError(err)
		} else if ret != nil {
			for _, item := range ret.Items {
				pod := item.DeepCopy()
				pod.Labels = lo.Assign(pod.Labels, selectors)
				// not patch sentinel labels
				delete(pod.Labels, "redissentinels.databases.spotahome.com/name")
				if !reflect.DeepEqual(pod.Labels, item.Labels) {
					if err := a.client.UpdatePod(ctx, pod.GetNamespace(), pod); err != nil {
						logger.Error(err, "patch sentinel pod label failed", "target", client.ObjectKeyFromObject(pod))
						return actor.RequeueWithError(err)
					}
				}
			}
		}
	}

	newSen := failoverbuilder.NewFailoverSentinel(inst)
	oldSen, err := a.client.GetRedisSentinel(ctx, inst.GetNamespace(), inst.GetName())
	if errors.IsNotFound(err) {
		if err := a.client.Client().Create(ctx, newSen); err != nil {
			logger.Error(err, "create sentinel failed", "target", client.ObjectKeyFromObject(newSen))
			return actor.RequeueWithError(err)
		}
		return nil
	} else if err != nil {
		logger.Error(err, "get sentinel failed", "target", client.ObjectKeyFromObject(newSen))
		return actor.RequeueWithError(err)
	}
	if !reflect.DeepEqual(newSen.Spec, oldSen.Spec) ||
		!reflect.DeepEqual(newSen.Labels, oldSen.Labels) ||
		!reflect.DeepEqual(newSen.Annotations, oldSen.Annotations) {
		oldSen.Spec = newSen.Spec
		oldSen.Labels = newSen.Labels
		oldSen.Annotations = newSen.Annotations
		if err := a.client.UpdateRedisSentinel(ctx, oldSen); err != nil {
			logger.Error(err, "update sentinel failed", "target", client.ObjectKeyFromObject(oldSen))
			return actor.RequeueWithError(err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureService(ctx context.Context, inst types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := inst.Definition()
	// read write svc
	rwSvc := failoverbuilder.NewRWSvcForCR(cr)
	roSvc := failoverbuilder.NewReadOnlyForCR(cr)
	if err := a.client.CreateOrUpdateIfServiceChanged(ctx, inst.GetNamespace(), rwSvc); err != nil {
		return actor.RequeueWithError(err)
	}
	if err := a.client.CreateOrUpdateIfServiceChanged(ctx, inst.GetNamespace(), roSvc); err != nil {
		return actor.RequeueWithError(err)
	}

	selector := inst.Selector()
	exporterService := failoverbuilder.NewExporterServiceForCR(cr, selector)
	if err := a.client.CreateOrUpdateIfServiceChanged(ctx, inst.GetNamespace(), exporterService); err != nil {
		return actor.RequeueWithError(err)
	}

	if ret := a.cleanUselessService(ctx, cr, logger, selector); ret != nil {
		return ret
	}
	switch cr.Spec.Redis.Expose.ServiceType {
	case corev1.ServiceTypeNodePort:
		if ret := a.ensureRedisSpecifiedNodePortService(ctx, inst, logger, selector); ret != nil {
			return ret
		}
	case corev1.ServiceTypeLoadBalancer:
		if ret := a.ensureRedisPodService(ctx, cr, logger, selector); ret != nil {
			return ret
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisSpecifiedNodePortService(ctx context.Context,
	inst types.RedisFailoverInstance, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	cr := inst.Definition()

	if len(cr.Spec.Redis.Expose.NodePortSequence) == 0 {
		return a.ensureRedisPodService(ctx, cr, logger, selectors)
	}

	logger.V(3).Info("ensure redis cluster nodeports", "namepspace", cr.Namespace, "name", cr.Name)
	configedPorts, err := helper.ParseSequencePorts(cr.Spec.Redis.Expose.NodePortSequence)
	if err != nil {
		return actor.RequeueWithError(err)
	}
	getClientPort := func(svc *corev1.Service, args ...string) int32 {
		name := "client"
		if len(args) > 0 {
			name = args[0]
		}
		if port := util.GetServicePortByName(svc, name); port != nil {
			return port.NodePort
		}
		return 0
	}

	serviceNameRange := map[string]struct{}{}
	for i := 0; i < int(cr.Spec.Redis.Replicas); i++ {
		serviceName := failoverbuilder.GetFailoverNodePortServiceName(cr, i)
		serviceNameRange[serviceName] = struct{}{}
	}

	// the whole process is divided into 3 steps:
	// 1. delete service not in nodeport range
	// 2. create new service
	// 3. update existing service and restart pod (only one pod is restarted at a same time for each shard)

	// 1. delete service not in nodeport range
	//
	// when pod not exists and service not in nodeport range, delete service
	// NOTE: only delete service whose pod is not found
	//       let statefulset auto scale up/down for pods
	labels := failoverbuilder.GenerateSelectorLabels("redis", cr.Name)
	services, ret := a.fetchAllPodBindedServices(ctx, cr.Namespace, labels)
	if ret != nil {
		return ret
	}
	for _, svc := range services {
		svc := svc.DeepCopy()
		occupiedPort := getClientPort(svc)
		if _, exists := serviceNameRange[svc.Name]; !exists || !slices.Contains(configedPorts, occupiedPort) {
			_, err := a.client.GetPod(ctx, svc.Namespace, svc.Name)
			if errors.IsNotFound(err) {
				logger.Info("release nodeport service", "service", svc.Name, "port", occupiedPort)
				if err = a.client.DeleteService(ctx, svc.Namespace, svc.Name); err != nil {
					return actor.RequeueWithError(err)
				}
			} else if err != nil {
				logger.Error(err, "get pods failed", "target", client.ObjectKeyFromObject(svc))
				return actor.RequeueWithError(err)
			}
		}
	}
	if services, ret = a.fetchAllPodBindedServices(ctx, cr.Namespace, labels); ret != nil {
		return ret
	}

	// 2. create new service
	var (
		newPorts           []int32
		bindedNodeports    []int32
		needUpdateServices []*corev1.Service
	)
	for _, svc := range services {
		svc := svc.DeepCopy()
		bindedNodeports = append(bindedNodeports, getClientPort(svc))
	}
	// filter used ports
	for _, port := range configedPorts {
		if !slices.Contains(bindedNodeports, port) {
			newPorts = append(newPorts, port)
		}
	}
	for i := 0; i < int(cr.Spec.Redis.Replicas); i++ {
		serviceName := failoverbuilder.GetFailoverNodePortServiceName(cr, i)
		oldService, err := a.client.GetService(ctx, cr.Namespace, serviceName)
		if errors.IsNotFound(err) {
			if len(newPorts) == 0 {
				continue
			}
			port := newPorts[0]
			svc := failoverbuilder.NewPodNodePortService(cr, i, labels, port)
			if err = a.client.CreateService(ctx, svc.Namespace, svc); err != nil {
				a.logger.Error(err, "create nodeport service failed", "target", client.ObjectKeyFromObject(svc))
				return actor.NewResultWithValue(ops.CommandRequeue, err)
			}
			newPorts = newPorts[1:]
			continue
		} else if err != nil {
			return actor.RequeueWithError(err)
		}

		svc := failoverbuilder.NewPodNodePortService(cr, i, labels, getClientPort(oldService))
		// check old service for compability
		if len(oldService.OwnerReferences) == 0 ||
			oldService.OwnerReferences[0].Kind == "Pod" ||
			!reflect.DeepEqual(oldService.Spec, svc.Spec) ||
			!reflect.DeepEqual(oldService.Labels, svc.Labels) ||
			!reflect.DeepEqual(oldService.Annotations, svc.Annotations) {

			oldService.OwnerReferences = util.BuildOwnerReferences(cr)
			oldService.Spec = svc.Spec
			oldService.Labels = svc.Labels
			oldService.Annotations = svc.Annotations
			if err := a.client.UpdateService(ctx, oldService.Namespace, oldService); err != nil {
				a.logger.Error(err, "update nodeport service failed", "target", client.ObjectKeyFromObject(oldService))
				return actor.NewResultWithValue(ops.CommandRequeue, err)
			}
		}
		if port := getClientPort(oldService); port != 0 && !slices.Contains(configedPorts, port) {
			needUpdateServices = append(needUpdateServices, oldService)
		}
	}

	// 3. update existing service and restart pod (only one pod is restarted at a same time for each shard)
	if len(needUpdateServices) > 0 && len(newPorts) > 0 {
		port, svc := newPorts[0], needUpdateServices[0]
		if sp := util.GetServicePortByName(svc, "client"); sp != nil {
			sp.NodePort = port
		}

		// NOTE: here not make sure the failover success, because the nodeport updated, the communication will be failed
		//       in k8s, the nodeport can still access for sometime after the nodeport updated
		//
		// update service
		if err = a.client.UpdateService(ctx, svc.Namespace, svc); err != nil {
			a.logger.Error(err, "update nodeport service failed", "target", client.ObjectKeyFromObject(svc), "port", port)
			return actor.NewResultWithValue(ops.CommandRequeue, err)
		}
		if pod, _ := a.client.GetPod(ctx, cr.Namespace, svc.Spec.Selector[builder.PodNameLabelKey]); pod != nil {
			if err := a.client.DeletePod(ctx, cr.Namespace, pod.Name); err != nil {
				return actor.RequeueWithError(err)
			}
			return actor.NewResult(ops.CommandRequeue)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisPodService(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	for i := 0; i < int(rf.Spec.Redis.Replicas); i++ {
		newSvc := failoverbuilder.NewPodService(rf, i, selectors)
		if svc, err := a.client.GetService(ctx, rf.Namespace, newSvc.Name); errors.IsNotFound(err) {
			if err = a.client.CreateService(ctx, rf.Namespace, newSvc); err != nil {
				logger.Error(err, "create service failed", "target", client.ObjectKeyFromObject(newSvc))
				return actor.RequeueWithError(err)
			}
		} else if err != nil {
			logger.Error(err, "get service failed", "target", client.ObjectKeyFromObject(newSvc))
			return actor.NewResult(ops.CommandRequeue)
		} else if newSvc.Spec.Type != svc.Spec.Type ||
			!reflect.DeepEqual(newSvc.Spec.Selector, svc.Spec.Selector) ||
			!reflect.DeepEqual(newSvc.Labels, svc.Labels) ||
			!reflect.DeepEqual(newSvc.Annotations, svc.Annotations) {
			svc.Spec = newSvc.Spec
			svc.Labels = newSvc.Labels
			svc.Annotations = newSvc.Annotations
			if err = a.client.UpdateService(ctx, rf.Namespace, svc); err != nil {
				logger.Error(err, "update service failed", "target", client.ObjectKeyFromObject(svc))
				return actor.RequeueWithError(err)
			}
		}
	}
	return nil
}

func (a *actorEnsureResource) cleanUselessService(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	services, err := a.fetchAllPodBindedServices(ctx, rf.Namespace, selectors)
	if err != nil {
		return err
	}
	for _, item := range services {
		svc := item.DeepCopy()
		index, err := util.ParsePodIndex(svc.Name)
		if err != nil {
			logger.Error(err, "parse svc name failed", "target", client.ObjectKeyFromObject(svc))
			continue
		}
		if index >= int(rf.Spec.Redis.Replicas) {
			_, err := a.client.GetPod(ctx, svc.Namespace, svc.Name)
			if errors.IsNotFound(err) {
				if err = a.client.DeleteService(ctx, svc.Namespace, svc.Name); err != nil {
					return actor.RequeueWithError(err)
				}
			} else if err != nil {
				logger.Error(err, "get pods failed", "target", client.ObjectKeyFromObject(svc))
				return actor.RequeueWithError(err)
			}
		}
	}
	return nil
}

func (a *actorEnsureResource) pauseStatefulSet(ctx context.Context, inst types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := inst.Definition()
	name := failoverbuilder.GetFailoverStatefulSetName(cr.Name)
	if sts, err := a.client.GetStatefulSet(ctx, cr.Namespace, name); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return actor.RequeueWithError(err)
	} else {
		if sts.Spec.Replicas == nil || *sts.Spec.Replicas == 0 {
			return nil
		}
		*sts.Spec.Replicas = 0
		if err = a.client.UpdateStatefulSet(ctx, cr.Namespace, sts); err != nil {
			return actor.RequeueWithError(err)
		}
		inst.SendEventf(corev1.EventTypeNormal, config.EventPause, "pause statefulset %s", name)
	}
	return nil
}

func (a *actorEnsureResource) pauseSentinel(ctx context.Context, inst types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	if def := inst.Definition(); def.Spec.Sentinel == nil ||
		(def.Spec.Sentinel.SentinelReference == nil && def.Spec.Sentinel.Image == "") {
		return nil
	}

	sen, err := a.client.GetRedisSentinel(ctx, inst.GetNamespace(), inst.GetName())
	if err != nil {
		return actor.RequeueWithError(err)
	}
	if sen.Spec.Replicas == 0 {
		return nil
	}
	sen.Spec.Replicas = 0
	if err := a.client.UpdateRedisSentinel(ctx, sen); err != nil {
		logger.Error(err, "pause sentinel failed", "target", client.ObjectKeyFromObject(sen))
		return actor.RequeueWithError(err)
	}
	inst.SendEventf(corev1.EventTypeNormal, config.EventPause, "pause instance sentinels")
	return nil
}

func (a *actorEnsureResource) fetchAllPodBindedServices(ctx context.Context, namespace string, selector map[string]string) ([]corev1.Service, *actor.ActorResult) {
	var services []corev1.Service
	if svcRes, err := a.client.GetServiceByLabels(ctx, namespace, selector); err != nil {
		return nil, actor.RequeueWithError(err)
	} else {
		// ignore services without pod selector
		for _, svc := range svcRes.Items {
			if svc.Spec.Selector[builder.PodNameLabelKey] != "" {
				services = append(services, svc)
			}
		}
	}
	return services, nil
}
