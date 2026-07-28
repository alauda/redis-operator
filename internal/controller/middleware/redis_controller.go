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

package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	redisfailover "github.com/alauda/redis-operator/api/databases/v1"
	rdsv1 "github.com/alauda/redis-operator/api/middleware/v1"
	"github.com/alauda/redis-operator/internal/builder/clusterbuilder"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/controller/middleware/redis"
	redissvc "github.com/alauda/redis-operator/internal/controller/middleware/redis"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/internal/vc"
	"github.com/alauda/redis-operator/pkg/actor"

	"github.com/go-logr/logr"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Logger       logr.Logger
	ActorManager *actor.ActorManager
}

const (
	createRedisSentinel               = "~create-redis-sentinel"
	createRedisCluster                = "~create-redis-cluster"
	pvcFinalizer                      = "delete-pvc"
	requeueSecond                     = 10 * time.Second
	DisableRdsManagementAnnotationKey = "cpaas.io/disable-rds-management"
)

//+kubebuilder:rbac:groups=middleware.alauda.io,resources=redis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=middleware.alauda.io,resources=redis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=middleware.alauda.io,resources=redis/finalizers,verbs=update
//+kubebuilder:rbac:groups=middleware.alauda.io,resources=imageversions,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(context.TODO()).WithValues("target", req.NamespacedName).WithName("RDS")

	if strings.Contains(req.NamespacedName.Name, createRedisSentinel) {
		//引发同步的是管理视图的哨兵模式的redis创建
		return ctrl.Result{}, r.createRedisByManagerUICreatedWithSentinel(ctx, req.NamespacedName)
	} else if strings.Contains(req.NamespacedName.Name, createRedisCluster) {
		//引发同步的是管理视图的集群模式的redis创建
		return ctrl.Result{}, r.createRedisByManagerUICreatedWithCluster(ctx, req.NamespacedName)
	}

	inst := &rdsv1.Redis{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Fail to get redis")
		return reconcile.Result{}, err
	}
	if inst.GetDeletionTimestamp() != nil {
		if err := r.processFinalizer(inst); err != nil {
			logger.Error(err, "fail to process finalizer")
			return r.updateInstanceStatus(ctx, inst, err, logger)
		}
		return ctrl.Result{}, nil
	}

	oldInst := inst.DeepCopy()
	inst.Default()
	if !reflect.DeepEqual(oldInst, inst) {
		if err := r.Update(ctx, inst); err != nil {
			logger.Error(err, "fail to update redis")
			return ctrl.Result{}, err
		}
	}

	if inst.Annotations[config.CRAutoUpgradeKey] == "false" &&
		(inst.Spec.UpgradeOption.AutoUpgrade == nil || *inst.Spec.UpgradeOption.AutoUpgrade) {
		// patch autoUpgrade
		operatorVersion := inst.Annotations["operatorVersion"]
		if strings.HasPrefix(operatorVersion, "v3.14.") ||
			strings.HasPrefix(operatorVersion, "v3.15.") ||
			strings.HasPrefix(operatorVersion, "v3.16.") {
			ver, err := semver.NewVersion(operatorVersion)
			if err != nil {
				logger.Error(err, "invalid operator version", "operatorVersion", operatorVersion)
				if _, err := r.updateInstanceStatus(ctx, inst, err, logger); err != nil {
					logger.Error(err, "fail to update redis instance status")
				}
				return ctrl.Result{}, nil
			}

			inst.Spec.UpgradeOption.AutoUpgrade = pointer.Bool(false)
			inst.Spec.UpgradeOption.CRVersion = ver.String()
			if _, err := r.updateInstance(ctx, inst, logger); err != nil {
				logger.Error(err, "fail to update redis instance")
			}
			inst.Status.UpgradeStatus.CRVersion = ver.String()
			if _, err := r.updateInstanceStatus(ctx, inst, nil, logger); err != nil {
				logger.Error(err, "fail to update redis instance status")
			}
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
	}

	var (
		err             error
		bv              *vc.BundleVersion
		operatorVersion = config.GetOperatorVersion()
	)

	if inst.Spec.UpgradeOption == nil || inst.Spec.UpgradeOption.AutoUpgrade == nil || *inst.Spec.UpgradeOption.AutoUpgrade {
		// NOTE: upgrade to current version no matter what current version is
		// which means if current redis version is 6.0.20, if the BundleVersion contains a redis with 6.0.19
		// it will do upgrade anyway
		if bv, err = vc.GetLatestBundleVersion(ctx, r.Client); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "fail to get latest bundle version", "operatorVersion", operatorVersion)
			return r.updateInstanceStatus(ctx, inst, err, logger)
		}
		if bv == nil {
			if bv, err = vc.GetBundleVersion(ctx, r.Client, inst.Status.UpgradeStatus.CRVersion); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "fail to get bundle version", "operatorVersion", operatorVersion)
				return r.updateInstanceStatus(ctx, inst, err, logger)
			}
		}
	} else {
		var latestBV *vc.BundleVersion
		if inst.Spec.UpgradeOption.CRVersion != "" {
			if bv, err = vc.GetBundleVersion(ctx, r.Client, inst.Spec.UpgradeOption.CRVersion); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "fail to get bundle version", "operatorVersion", operatorVersion)
				return r.updateInstanceStatus(ctx, inst, err, logger)
			}
		} else if inst.Status.UpgradeStatus.CRVersion != "" {
			if bv, err = vc.GetBundleVersion(ctx, r.Client, inst.Status.UpgradeStatus.CRVersion); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "fail to get bundle version", "operatorVersion", operatorVersion)
				return r.updateInstanceStatus(ctx, inst, err, logger)
			}
		} else {
			if latestBV, err = vc.GetLatestBundleVersion(ctx, r.Client); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "fail to get latest bundle version", "operatorVersion", operatorVersion)
				return r.updateInstanceStatus(ctx, inst, err, logger)
			}
			bv = latestBV
		}

		if inst.Status.UpgradeStatus.CRVersion != "" {
			if latestBV == nil {
				if latestBV, err = vc.GetLatestBundleVersion(ctx, r.Client); err != nil && !errors.IsNotFound(err) {
					logger.Error(err, "fail to get latest bundle version", "operatorVersion", operatorVersion)
					return r.updateInstanceStatus(ctx, inst, err, logger)
				}
			}
			if latestBV != nil &&
				latestBV.Spec.CrVersion != inst.Status.UpgradeStatus.CRVersion &&
				latestBV.Spec.CrVersion != inst.Annotations[config.CRUpgradeableVersion] {
				if vc.IsBundleVersionUpgradeable(bv, latestBV, inst.Spec.Version) {
					comp := latestBV.GetComponent(config.CoreComponentName, inst.Spec.Version)
					inst.Annotations[config.CRUpgradeableVersion] = latestBV.Spec.CrVersion
					inst.Annotations[config.CRUpgradeableComponentVersion] = comp.Version

					if _, err := r.updateInstance(ctx, inst, logger); err != nil {
						logger.Error(err, "fail to update redis instance")
					}
				}
			}
		}
	}
	if bv == nil {
		err := fmt.Errorf("bundle version not found")
		logger.Error(err, "fail to get any usable ImageVersion")
		return r.updateInstanceStatus(ctx, inst, err, logger)
	}

	if operatorVersion != "" && operatorVersion != inst.Annotations["operatorVersion"] {
		logger.V(3).Info("redis inst operatorVersion is not match")
		if inst.Annotations == nil {
			inst.Annotations = make(map[string]string)
		}
		inst.Annotations["operatorVersion"] = operatorVersion
		return r.updateInstance(ctx, inst, logger)
	}

	// ensure redis password secret
	if err := r.reconcileSecret(inst, logger); err != nil {
		logger.Error(err, "fail to reconcile secret")
		return r.updateInstanceStatus(ctx, inst, err, logger)
	}

	if inst.Status.UpgradeStatus.CRVersion != bv.Spec.CrVersion {
		inst.Status.UpgradeStatus.CRVersion = bv.Spec.CrVersion
		_, _ = r.updateInstanceStatus(ctx, inst, err, logger)
	}
	if ver := inst.Status.UpgradeStatus.CRVersion; ver != "" && ver == inst.Annotations[config.CRUpgradeableVersion] {
		delete(inst.Annotations, config.CRUpgradeableVersion)
		delete(inst.Annotations, config.CRUpgradeableComponentVersion)
		if _, err := r.updateInstance(ctx, inst, logger); err != nil {
			logger.Error(err, "fail to update redis instance")
		}
	}

	// ensure redis inst
	switch inst.Spec.Arch {
	case core.RedisCluster:
		if ptr := inst.Spec.Replicas.Cluster.Slave; ptr == nil || *ptr < 0 {
			inst.Spec.Replicas.Cluster.Slave = pointer.Int32(0)
		}

		if err := r.reconcileCluster(ctx, inst, bv, logger); err != nil {
			logger.Error(err, "fail to reconcile redis cluster")
			return r.updateInstanceStatus(ctx, inst, err, logger)
		}
	case core.RedisSentinel, core.RedisStandalone:
		if ptr := inst.Spec.Replicas.Sentinel.Master; ptr == nil || *ptr < 1 {
			inst.Spec.Replicas.Sentinel.Master = pointer.Int32(1)
		}
		if ptr := inst.Spec.Replicas.Sentinel.Slave; ptr == nil || *ptr < 0 {
			inst.Spec.Replicas.Sentinel.Slave = pointer.Int32(0)
		}
		if err := r.reconcileFailover(ctx, inst, bv, logger); err != nil {
			logger.Error(err, fmt.Sprintf("fail to reconcile redis %s", inst.Spec.Arch))
			return r.updateInstanceStatus(ctx, inst, err, logger)
		}
	default:
		err = fmt.Errorf("this arch isn't valid, must be cluster, sentinel or standalone")
		return ctrl.Result{}, err
	}
	if err := r.ensurePvcSize(ctx, inst, logger); err != nil {
		logger.Error(err, "fail to reconcile pvcs size")
	}
	if err := r.patchResources(ctx, inst, logger); err != nil {
		logger.Error(err, "fail to patch resources")
	}

	if !inst.Status.Restored && inst.Status.Phase == rdsv1.RedisPhaseReady {
		inst.Status.Restored = true
	}
	return r.updateInstanceStatus(ctx, inst, err, logger)
}

func (r *RedisReconciler) patchResources(ctx context.Context, inst *rdsv1.Redis, logger logr.Logger) error {
	if inst.Spec.Patches == nil || len(inst.Spec.Patches.Services) == 0 {
		return nil
	}
	for _, psvc := range inst.Spec.Patches.Services {
		if psvc.Name == "" {
			logger.Error(fmt.Errorf("service name is empty"), "invalid patch service", "service", psvc)
			continue
		}

		var oldSvc v1.Service
		// TODO: only support create now service now
		if err := r.Get(ctx, types.NamespacedName{Name: psvc.Name, Namespace: inst.Namespace}, &oldSvc); errors.IsNotFound(err) {
			if len(psvc.Spec.Selector) == 0 || len(psvc.Spec.Ports) == 0 {
				logger.Error(fmt.Errorf("invalid service"), "please check service ports and selector", "service", psvc)
				continue
			}

			svc := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:            psvc.Name,
					Namespace:       inst.Namespace,
					Labels:          psvc.Labels,
					Annotations:     psvc.Annotations,
					OwnerReferences: util.BuildOwnerReferences(inst),
				},
				Spec: psvc.Spec,
			}
			if psvc.Labels == nil {
				svc.Labels = inst.Status.MatchLabels
			}
			if err := r.Create(ctx, svc); err != nil {
				logger.Error(err, "fail to create patch service", "service", psvc)
				continue
			}
		} else if err != nil {
			return err
		} else {
			// do service merge
			isUpdated := false
			for _, field := range []struct {
				old, new any
			}{
				{&oldSvc.Labels, psvc.Labels},
				{&oldSvc.Annotations, psvc.Annotations},
				{&oldSvc.Spec.Selector, psvc.Spec.Selector},
				{&oldSvc.Spec.Type, psvc.Spec.Type},
				{&oldSvc.Spec.IPFamilies, psvc.Spec.IPFamilies},
				{&oldSvc.Spec.IPFamilyPolicy, psvc.Spec.IPFamilyPolicy},
				{&oldSvc.Spec.ExternalIPs, psvc.Spec.ExternalIPs},
				{&oldSvc.Spec.SessionAffinity, psvc.Spec.SessionAffinity},
				{&oldSvc.Spec.SessionAffinityConfig, psvc.Spec.SessionAffinityConfig},
				{&oldSvc.Spec.LoadBalancerSourceRanges, psvc.Spec.LoadBalancerSourceRanges},
				{&oldSvc.Spec.AllocateLoadBalancerNodePorts, psvc.Spec.AllocateLoadBalancerNodePorts},
				{&oldSvc.Spec.ExternalName, psvc.Spec.ExternalName},
				{&oldSvc.Spec.ExternalTrafficPolicy, psvc.Spec.ExternalTrafficPolicy},
				{&oldSvc.Spec.InternalTrafficPolicy, psvc.Spec.InternalTrafficPolicy},
				{&oldSvc.Spec.PublishNotReadyAddresses, psvc.Spec.PublishNotReadyAddresses},
			} {
				oldVal := reflect.ValueOf(field.old).Elem()
				newVal := reflect.ValueOf(field.new)

				if !newVal.IsZero() && !reflect.DeepEqual(oldVal.Interface(), newVal.Interface()) {
					reflect.ValueOf(field.old).Elem().Set(reflect.ValueOf(field.new))
					isUpdated = true
				}
			}
			for _, port := range psvc.Spec.Ports {
				oldPort := func() *v1.ServicePort {
					for i := range oldSvc.Spec.Ports {
						p := &oldSvc.Spec.Ports[i]
						if p.Name == port.Name {
							return p
						}
					}
					return nil
				}()
				if oldPort == nil {
					if port.Port > 0 {
						oldSvc.Spec.Ports = append(oldSvc.Spec.Ports, port)
						isUpdated = true
					}
				} else if !reflect.DeepEqual(*oldPort, port) {
					*oldPort = port
					isUpdated = true
				}
			}
			if isUpdated {
				if err := r.Update(ctx, &oldSvc); err != nil {
					logger.Error(err, "fail to update patch service", "service", psvc)
					continue
				}
			}
		}
	}
	return nil
}

func (r *RedisReconciler) ensurePvcSize(ctx context.Context, inst *rdsv1.Redis, logger logr.Logger) error {
	if inst.Spec.Persistent != nil && len(inst.Status.MatchLabels) > 0 {
		size := resource.NewQuantity(inst.Spec.Resources.Limits.Memory().Value()*2, resource.BinarySI)
		if inst.Spec.PersistentSize != nil {
			size = inst.Spec.PersistentSize
		}

		switch inst.Spec.Arch {
		case core.RedisCluster:
			shards := int32(0)
			if inst.Spec.Replicas != nil && inst.Spec.Replicas.Cluster != nil && inst.Spec.Replicas.Cluster.Shard != nil {
				shards = *inst.Spec.Replicas.Cluster.Shard
			} else {
				return nil
			}

			labels := lo.Assign(inst.Status.MatchLabels)
			storageConfigVal := inst.Annotations[rdsv1.RedisClusterPVCSizeAnnotation]
			storageConfig := map[int32]string{}
			if storageConfigVal != "" {
				if err := json.Unmarshal([]byte(storageConfigVal), &storageConfig); err != nil {
					logger.Error(err, "fail to unmarshal shard storage config")
					return err
				}
			}

			isUpdated := false
			for i := int32(0); i < shards; i++ {
				labels["statefulSet"] = clusterbuilder.ClusterStatefulSetName(inst.Name, int(i))
				if maxQuantity, err := redissvc.GetShardMaxPVCQuantity(ctx, r.Client, inst.Namespace, labels); err != nil {
					logger.Error(err, "fail to get shard max pvc quantity")
					continue
				} else {
					if storageConfig[i] == "" {
						storageConfig[i] = maxQuantity.String()
						isUpdated = true
					} else if s, err := resource.ParseQuantity(storageConfig[i]); err != nil {
						storageConfig[i] = maxQuantity.String()
						isUpdated = true
					} else if maxQuantity.Cmp(s) > 0 {
						storageConfig[i] = maxQuantity.String()
						isUpdated = true
					}
				}
			}
			if isUpdated {
				data, _ := json.Marshal(storageConfig)
				inst.Annotations[rdsv1.RedisClusterPVCSizeAnnotation] = string(data)
				if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					var oldInst rdsv1.Redis
					if err := r.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
						return err
					}
					oldInst.Annotations = inst.Annotations
					oldInst.Labels = inst.Labels
					oldInst.Spec = inst.Spec
					return r.Update(ctx, &oldInst)
				}); errors.IsNotFound(err) {
					return nil
				} else if err != nil {
					logger.Error(err, "get redis failed")
				}
			}

			for i := int32(0); i < shards; i++ {
				shardSize := size.DeepCopy()
				if s, err := resource.ParseQuantity(storageConfig[i]); err == nil {
					shardSize = s
				}
				labels["statefulSet"] = clusterbuilder.ClusterStatefulSetName(inst.Name, int(i))

				if err := redissvc.ResizePVCs(ctx, r.Client, inst.Namespace, labels, shardSize); err != nil {
					return err
				}
			}
		default:
			labels := inst.Status.MatchLabels
			if maxQuantity, err := redissvc.GetShardMaxPVCQuantity(ctx, r.Client, inst.Namespace, labels); err != nil {
				logger.Error(err, "fail to get shard max pvc quantity")
				return err
			} else {
				if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					var oldInst rdsv1.Redis
					if err := r.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
						return err
					}
					if maxQuantity.Cmp(*size) > 0 {
						oldInst.Spec.PersistentSize = maxQuantity
						size = maxQuantity
						if err := r.Update(ctx, &oldInst); err != nil {
							logger.Error(err, "fail to update redis inst")
						}
					}
					return nil
				}); errors.IsNotFound(err) {
					return nil
				} else if err != nil {
					logger.Error(err, "get redis failed")
				}
			}
			if err := redissvc.ResizePVCs(ctx, r.Client, inst.Namespace, labels, *size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *RedisReconciler) reconcileFailover(ctx context.Context, inst *rdsv1.Redis, bv *vc.BundleVersion, logger logr.Logger) error {
	if inst.Spec.CustomConfig == nil {
		inst.Spec.CustomConfig = map[string]string{}
	}

	failover := &redisfailover.RedisFailover{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      redis.GetRedisFailoverName(inst.Name),
		Namespace: inst.Namespace,
	}, failover); errors.IsNotFound(err) {
		failover, err = redissvc.GenerateRedisFailover(inst, bv)
		if err != nil {
			return err
		}
		// record actor versions too keep actions consistent
		failover.Annotations[config.CRVersionKey] = bv.Spec.CrVersion

		if err := r.Create(ctx, failover); err != nil {
			inst.SetStatusError(err.Error())
			logger.Error(err, "fail to create redis failover inst")
			return err
		}
		inst.Status.MatchLabels = redis.GetRedisSentinelLabels(inst.Name, failover.Name)
		return nil
	} else if err != nil {
		return err
	}

	if len(inst.Status.MatchLabels) == 0 {
		inst.Status.MatchLabels = redis.GetRedisSentinelLabels(inst.Name, failover.Name)
	}
	for key := range redissvc.GetRedisConfigsApplyPolicyByVersion(inst.Spec.Version) {
		if inst.Spec.CustomConfig[key] != failover.Spec.Redis.CustomConfig[key] {
			if inst.Spec.PodAnnotations == nil {
				inst.Spec.PodAnnotations = map[string]string{}
			}
			inst.Spec.PodAnnotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339Nano)
			break
		}
	}

	newFailover, err := redissvc.GenerateRedisFailover(inst, bv)
	if err != nil {
		logger.Error(err, "fail to generate redis failover inst")
		return err
	}
	newFailover.Annotations[config.CRVersionKey] = bv.Spec.CrVersion

	// TRICK: keep old persistent volume claim, expecially for persistentVolumeClaim name
	// we should keep the old pvc name, because the pvc name may not be "redis-data" which created manually
	newFailover.Spec.Redis.Storage = failover.Spec.Redis.Storage

	// ensure inst should update
	if redissvc.ShouldUpdateFailover(failover, newFailover, logger) {
		newFailover.ResourceVersion = failover.ResourceVersion
		newFailover.Status = failover.Status
		if err := r.updateRedisFailoverInstance(ctx, newFailover); err != nil {
			inst.SetStatusError(err.Error())
			logger.Error(err, "fail to update redis failover inst")
			return err
		}
		failover = newFailover
	}

	inst.Status.LastShardCount = 1
	inst.Status.LastVersion = inst.Spec.Version
	inst.Status.ClusterNodes = failover.Status.Nodes
	inst.Status.DetailedStatusRef = failover.Status.DetailedStatusRef
	if failover.Status.Phase == redisfailover.Fail {
		logger.V(3).Info("redis inst is fail")
		inst.SetStatusError(failover.Status.Message)
	} else if failover.Status.Phase == redisfailover.Ready {
		logger.V(3).Info("redis inst is ready")
		inst.SetStatusReady()
	} else if failover.Status.Phase == redisfailover.Paused {
		logger.V(3).Info("redis inst is paused")
		inst.SetStatusPaused()
	} else {
		logger.V(3).Info("redis inst is unhealthy, waiting redis failover to up", "phase", failover.Status.Phase)
		inst.SetStatusUnReady(failover.Status.Message)
	}
	return nil
}

func (r *RedisReconciler) reconcileCluster(ctx context.Context, inst *rdsv1.Redis, bv *vc.BundleVersion, logger logr.Logger) error {
	cluster := &v1alpha1.DistributedRedisCluster{}
	if inst.Spec.PodAnnotations == nil {
		inst.Spec.PodAnnotations = make(map[string]string)
	}
	if inst.Spec.Pause {
		inst.Spec.PodAnnotations[redissvc.PauseAnnotationKey] = metav1.NewTime(time.Now()).Format(time.RFC3339)
	}

	if err := r.Get(ctx, types.NamespacedName{
		Name:      redis.GetRedisClusterName(inst.Name),
		Namespace: inst.Namespace,
	}, cluster); errors.IsNotFound(err) {
		cluster, err = redissvc.GenerateRedisCluster(inst, bv)
		if err != nil {
			return err
		}
		// Record actor versions too keep actions consistent
		cluster.Annotations[config.CRVersionKey] = bv.Spec.CrVersion

		if err := r.Create(ctx, cluster); err != nil {
			inst.SetStatusError(err.Error())
			logger.Error(err, "fail to create redis cluster inst")
			return err
		}
		inst.Status.MatchLabels = redis.GetRedisClusterLabels(inst.Name, cluster.Name)
		return nil
	} else if err != nil {
		return err
	}

	if len(inst.Status.MatchLabels) == 0 {
		inst.Status.MatchLabels = redis.GetRedisClusterLabels(inst.Name, cluster.Name)
	}
	for key := range redissvc.GetRedisConfigsApplyPolicyByVersion(inst.Spec.Version) {
		if inst.Spec.CustomConfig[key] != cluster.Spec.Config[key] {
			if inst.Spec.PodAnnotations == nil {
				inst.Spec.PodAnnotations = map[string]string{}
			}
			inst.Spec.PodAnnotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339Nano)
			break
		}
	}
	newCluster, err := redissvc.GenerateRedisCluster(inst, bv)
	if err != nil {
		return err
	}
	newCluster.Annotations[config.CRVersionKey] = cluster.Annotations[config.CRVersionKey]
	if bv.Spec.CrVersion != cluster.Annotations[config.CRVersionKey] {
		// record actor versions too keep actions consistent
		newCluster.Annotations[config.CRVersionKey] = bv.Spec.CrVersion
	}
	// TRICK: keep old persistent volume claim, expecially for persistentVolumeClaim name
	// we should keep the old pvc name, because the pvc name may not be "redis-data" which created manually
	newCluster.Spec.Storage = cluster.Spec.Storage

	// ensure inst should update
	if redissvc.ShouldUpdateCluster(cluster, newCluster, logger) {
		newCluster.ResourceVersion = cluster.ResourceVersion
		newCluster.Status = cluster.Status
		if err := r.updateRedisClusterInstance(ctx, newCluster); err != nil {
			inst.SetStatusError(err.Error())
			logger.Error(err, "fail to update redis cluster inst")
			return err
		}
		cluster = newCluster
	}

	inst.Status.LastShardCount = cluster.Spec.MasterSize
	inst.Status.LastVersion = inst.Spec.Version
	inst.Status.ClusterNodes = cluster.Status.Nodes
	inst.Status.DetailedStatusRef = cluster.Status.DetailedStatusRef
	if redissvc.ClusterIsUp(cluster) {
		logger.V(3).Info("redis inst is ready")
		inst.SetStatusReady()
	} else if cluster.Status.Status == v1alpha1.ClusterStatusPaused {
		inst.SetStatusPaused()
	} else if cluster.Status.Status == v1alpha1.ClusterStatusRebalancing {
		inst.SetStatusRebalancing(cluster.Status.Reason)
	} else if cluster.Status.Status == v1alpha1.ClusterStatusKO {
		inst.SetStatusError(cluster.Status.Reason)
	} else {
		logger.V(3).Info("redis inst is unhealthy, waiting redis cluster to up")
		inst.SetStatusUnReady(cluster.Status.Reason)
	}
	return nil
}

func (r *RedisReconciler) reconcileSecret(inst *rdsv1.Redis, logger logr.Logger) error {
	if inst.PasswordIsEmpty() {
		inst.Status.PasswordSecretName = ""
		return nil
	}

	if len(inst.Spec.PasswordSecret) != 0 { //use spec passwd secret
		secret := &v1.Secret{}
		if err := r.Get(context.TODO(), types.NamespacedName{
			Name:      inst.Spec.PasswordSecret,
			Namespace: inst.Namespace,
		}, secret); err != nil {
			logger.Error(err, "fail to get password secret")
			if errors.IsNotFound(err) {
				inst.Status.PasswordSecretName = ""
			}
			return err
		}
		inst.Status.PasswordSecretName = inst.Spec.PasswordSecret
		return nil
	}
	return nil
}

func (r *RedisReconciler) updateRedisClusterInstance(ctx context.Context, inst *v1alpha1.DistributedRedisCluster) error {
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst v1alpha1.DistributedRedisCluster
		if err := r.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			return err
		}
		inst.ResourceVersion = oldInst.ResourceVersion
		return r.Update(ctx, inst)
	}); err != nil {
		return err
	}
	return nil
}

func (r *RedisReconciler) updateRedisFailoverInstance(ctx context.Context, inst *redisfailover.RedisFailover) error {
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst redisfailover.RedisFailover
		if err := r.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			return err
		}
		inst.ResourceVersion = oldInst.ResourceVersion
		return r.Update(ctx, inst)
	}); err != nil {
		return err
	}
	return nil
}

func (r *RedisReconciler) updateInstanceStatus(ctx context.Context, inst *rdsv1.Redis, err error, logger logr.Logger) (ctrl.Result, error) {
	logger.V(3).Info("updating inst state")
	if err != nil {
		inst.SetStatusError(err.Error())
	} else {
		inst.RecoverStatusError()
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst rdsv1.Redis
		if err := r.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			return err
		}
		if reflect.DeepEqual(oldInst.Status, inst.Status) {
			return nil
		}
		inst.ResourceVersion = oldInst.ResourceVersion
		return r.Status().Update(ctx, inst)
	}); errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else {
		return ctrl.Result{RequeueAfter: requeueSecond}, err
	}
}

func (r *RedisReconciler) updateInstance(ctx context.Context, inst *rdsv1.Redis, logger logr.Logger) (ctrl.Result, error) {
	logger.V(3).Info("updating instance")
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var oldInst rdsv1.Redis
		if err := r.Get(ctx, client.ObjectKeyFromObject(inst), &oldInst); err != nil {
			return err
		}
		inst.ResourceVersion = oldInst.ResourceVersion
		return r.Update(ctx, inst)
	}); errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else {
		return ctrl.Result{RequeueAfter: requeueSecond}, err
	}
}

func (r *RedisReconciler) processFinalizer(inst *rdsv1.Redis) error {
	for _, v := range inst.GetFinalizers() {
		if v == pvcFinalizer {
			// delete pvcs
			if inst.Status.MatchLabels == nil {
				return fmt.Errorf("can't delete inst pvcs, status.matchLabels is empty")
			}
			err := r.DeleteAllOf(context.TODO(), &v1.PersistentVolumeClaim{}, client.InNamespace(inst.Namespace),
				client.MatchingLabels(inst.Status.MatchLabels))
			if err != nil {
				return err
			}
			controllerutil.RemoveFinalizer(inst, v)
			err = r.Update(context.TODO(), inst)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *RedisReconciler) createRedisByManagerUICreatedWithCluster(ctx context.Context, nsn types.NamespacedName) error {
	nsn.Name = strings.ReplaceAll(nsn.Name, createRedisCluster, "")
	cluster := &v1alpha1.DistributedRedisCluster{}
	err := r.Get(ctx, nsn, cluster)
	if err != nil {
		return err
	}

	// check if cluster disabled rds management for special cases
	// eg. devops doesn't need rds
	if val := cluster.Annotations[DisableRdsManagementAnnotationKey]; strings.ToLower(val) == "true" {
		return nil
	}

	var secret *v1.Secret
	if cluster.Spec.PasswordSecret != nil {
		secret = &v1.Secret{}
		err = r.Get(ctx, types.NamespacedName{
			Name:      cluster.Spec.PasswordSecret.Name,
			Namespace: cluster.Namespace,
		}, secret)
		if err != nil {
			return err
		}

	}
	inst := redissvc.GenerateClusterRedisByManagerUI(cluster, r.Scheme, secret)
	if inst.Spec.Version == "" {
		return fmt.Errorf("version is not valid, available version: 4.0, 5.0, 6.0")
	}
	//创建资源
	if err = r.Create(ctx, inst); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
	}
	if err = controllerutil.SetControllerReference(inst, cluster, r.Scheme); err != nil {
		return err
	}
	return r.Update(ctx, cluster)
}

func (r *RedisReconciler) createRedisByManagerUICreatedWithSentinel(ctx context.Context, nsn types.NamespacedName) error {
	nsn.Name = strings.ReplaceAll(nsn.Name, createRedisSentinel, "")
	failover := &redisfailover.RedisFailover{}
	err := r.Get(ctx, nsn, failover)
	if err != nil {
		return err
	}

	// check if failover disabled rds management for special cases
	// eg. devops doesn't need rds
	if val := failover.Annotations[DisableRdsManagementAnnotationKey]; strings.ToLower(val) == "true" {
		return nil
	}

	sts := &appsv1.StatefulSet{}
	err = r.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("rfr-%s", nsn.Name), Namespace: nsn.Namespace}, sts)
	if err != nil {
		return err
	}
	var secret *v1.Secret
	if failover.Spec.Auth.SecretPath != "" {
		secret = &v1.Secret{}
		err = r.Get(ctx, types.NamespacedName{
			Name:      failover.Spec.Auth.SecretPath,
			Namespace: failover.Namespace,
		}, secret)
		if err != nil {
			return err
		}
	}
	inst := redissvc.GenerateFailoverRedisByManagerUI(failover, sts, secret)
	if inst.Spec.Version == "" {
		return fmt.Errorf("version is not valid, available version: 4.0, 5.0, 6.0")
	}

	if err = r.Create(ctx, inst); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
	}
	if err = controllerutil.SetControllerReference(inst, failover, r.Scheme); err != nil {
		return err
	}
	return r.Update(ctx, failover)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&rdsv1.Redis{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 4}).
		Owns(&v1alpha1.DistributedRedisCluster{}).
		Owns(&redisfailover.RedisFailover{}).
		Watches(&redisfailover.RedisFailover{}, handler.Funcs{
			CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				for _, v := range e.Object.GetOwnerReferences() {
					if v.Kind == "Redis" {
						q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
							Name:      e.Object.GetName(),
							Namespace: e.Object.GetNamespace(),
						}})
						return
					}
				}

				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      e.Object.GetName() + createRedisSentinel,
					Namespace: e.Object.GetNamespace(),
				}})
			}}).
		Watches(&v1alpha1.DistributedRedisCluster{}, handler.Funcs{
			CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				for _, v := range e.Object.GetOwnerReferences() {
					if v.Kind == "Redis" {
						q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
							Name:      e.Object.GetName(),
							Namespace: e.Object.GetNamespace(),
						}})
						return
					}
				}
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      e.Object.GetName() + createRedisCluster,
					Namespace: e.Object.GetNamespace(),
				}})
			}}).
		Complete(r)
}
