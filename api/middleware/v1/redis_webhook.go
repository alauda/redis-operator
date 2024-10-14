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
package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	clusterv1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	corehelper "github.com/alauda/redis-operator/api/core/helper"
	coreVal "github.com/alauda/redis-operator/api/core/validation"
	redisfailoverv1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/api/middleware/v1/helper"
	"github.com/alauda/redis-operator/api/middleware/v1/validation"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/pkg/slot"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	redisRestartAnnotation        = "kubectl.kubernetes.io/restartedAt"
	PauseAnnotationKey            = "app.cpaas.io/pause-timestamp"
	RedisClusterPVCSizeAnnotation = "middleware.alauda.io/storage_size"

	ConfigReplBacklogSizeKey = "repl-backlog-size"
)

// log is for logging in this package.
var logger = logf.Log.WithName("redis-webhook")
var mgrClient client.Client

func (r *Redis) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgrClient = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/mutate-middleware-alauda-io-v1-redis,mutating=true,failurePolicy=fail,groups=middleware.alauda.io,resources=redis,versions=v1,name=mredis.kb.io,sideEffects=none,admissionReviewVersions=v1
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-middleware-alauda-io-v1-redis,mutating=false,failurePolicy=fail,groups=middleware.alauda.io,resources=redis,versions=v1,name=vredis.kb.io,sideEffects=none,admissionReviewVersions=v1

var _ webhook.Validator = (*Redis)(nil)
var _ webhook.Defaulter = (*Redis)(nil)

func (r *Redis) Default() {
	if r.Annotations == nil {
		r.Annotations = make(map[string]string)
	}
	if r.Spec.CustomConfig == nil {
		r.Spec.CustomConfig = make(map[string]string)
	}
	if r.Spec.PodAnnotations == nil {
		r.Spec.PodAnnotations = make(map[string]string)
	}
	if r.Spec.UpgradeOption == nil {
		r.Spec.UpgradeOption = &UpgradeOption{}
	}

	// TODO: apply this config to the innner redis instance
	if r.Spec.CustomConfig[ConfigReplBacklogSizeKey] == "" {
		// https://raw.githubusercontent.com/antirez/redis/7.0/redis.conf
		// https://docs.redis.com/latest/rs/databases/active-active/manage/#replication-backlog
		if r.Spec.Resources != nil {
			for _, resource := range []*resource.Quantity{r.Spec.Resources.Limits.Memory(), r.Spec.Resources.Requests.Memory()} {
				if resource == nil {
					continue
				}
				if val, ok := resource.AsInt64(); ok {
					val = int64(0.01 * float64(val))
					if val > 256*1024*1024 {
						val = 256 * 1024 * 1024
					} else if val < 1024*1024 {
						val = 1024 * 1024
					}
					r.Spec.CustomConfig[ConfigReplBacklogSizeKey] = fmt.Sprintf("%d", val)
				}
			}
		}
	}

	ver := redis.RedisVersion(r.Spec.Version)
	if rename := r.Spec.CustomConfig["rename-command"]; !ver.IsACLSupported() || rename != "" {
		r.Spec.CustomConfig["rename-command"] = helper.BuildRenameCommand(rename)
	}

	if r.Spec.Pause {
		if _, ok := r.Spec.PodAnnotations[PauseAnnotationKey]; !ok {
			r.Spec.PodAnnotations[PauseAnnotationKey] = metav1.NewTime(time.Now()).Format(time.RFC3339)
		}
	} else {
		delete(r.Spec.PodAnnotations, PauseAnnotationKey)
	}

	// reset expose image
	if r.Spec.Expose.EnableNodePort && r.Spec.Expose.ServiceType == "" {
		r.Spec.Expose.ServiceType = v1.ServiceTypeNodePort
	}
	if r.Spec.Expose.ServiceType != v1.ServiceTypeNodePort {
		r.Spec.Expose.EnableNodePort = false
	}
	if r.Spec.IPFamilyPrefer == "" {
		r.Spec.IPFamilyPrefer = corehelper.GetDefaultIPFamily(os.Getenv("POD_IP"))
	}

	// init exporter settings
	if r.Spec.Exporter == nil {
		r.Spec.Exporter = &redisfailoverv1.RedisExporter{Enabled: true}
	}
	if r.Spec.Exporter.Resources.Limits.Cpu().IsZero() ||
		r.Spec.Exporter.Resources.Limits.Memory().IsZero() {
		r.Spec.Exporter.Resources = v1.ResourceRequirements{
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse("50m"),
				v1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse("100m"),
				v1.ResourceMemory: resource.MustParse("384Mi"),
			},
		}
	}

	if r.Spec.Persistent != nil && r.Spec.Persistent.StorageClassName != "" &&
		(r.Spec.PersistentSize == nil || r.Spec.PersistentSize.IsZero()) {
		size := resource.NewQuantity(r.Spec.Resources.Limits.Memory().Value()*2, resource.BinarySI)
		r.Spec.PersistentSize = size
	}

	// init pvc annotations
	if r.Spec.PersistentSize != nil {
		if val, _ := r.Spec.PersistentSize.AsInt64(); val > 0 {
			if r.Spec.Arch == core.RedisCluster {
				storageConfig := map[int32]string{}
				storageConfigVal := r.Annotations[RedisClusterPVCSizeAnnotation]
				if storageConfigVal != "" {
					if err := json.Unmarshal([]byte(storageConfigVal), &storageConfig); err != nil {
						logger.Error(err, "failed to unmarshal storage size")
					}
				}
				if r.Spec.Replicas != nil && r.Spec.Replicas.Cluster != nil && r.Spec.Replicas.Cluster.Shard != nil {
					for i := int32(0); i < *r.Spec.Replicas.Cluster.Shard; i++ {
						if val := storageConfig[i]; val == "" {
							storageConfig[i] = r.Spec.PersistentSize.String()
						}
					}
				}
				data, _ := json.Marshal(storageConfig)
				r.Annotations[RedisClusterPVCSizeAnnotation] = string(data)
			}
		}
	}

	switch r.Spec.Arch {
	case core.RedisSentinel:
		if r.Spec.Replicas == nil {
			r.Spec.Replicas = &RedisReplicas{}
		}
		if r.Spec.Replicas.Sentinel == nil {
			r.Spec.Replicas.Sentinel = &SentinelReplicas{}
		}
		if r.Spec.Sentinel == nil {
			r.Spec.Sentinel = &redisfailoverv1.SentinelSettings{}
		}
		if r.Spec.Sentinel.SentinelReference == nil {
			if r.Spec.Replicas.Sentinel.Master == nil || *r.Spec.Replicas.Sentinel.Master != 1 {
				r.Spec.Replicas.Sentinel.Master = pointer.Int32(1)
			}
			if r.Spec.Sentinel.Replicas <= 0 {
				r.Spec.Sentinel.Replicas = 3
			}

			sentinel := r.Spec.Sentinel
			if r.Annotations["createType"] != "managerView" &&
				sentinel.Resources.Limits.Cpu().IsZero() && sentinel.Resources.Limits.Memory().IsZero() {
				sentinel.Resources = v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU:    resource.MustParse("100m"),
						v1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU:    resource.MustParse("100m"),
						v1.ResourceMemory: resource.MustParse("128Mi"),
					},
				}
			}

			r.Spec.Sentinel.Expose.ServiceType = r.Spec.Expose.ServiceType
			// reset pod assignment
			r.Spec.Sentinel.Expose.AccessPort = r.Spec.Expose.AccessPort
			if r.Spec.Expose.ServiceType == v1.ServiceTypeNodePort {
				if len(r.Spec.Expose.NodePortMap) != 0 {
					items := lo.MapToSlice(r.Spec.Expose.NodePortMap, func(k string, v int32) [2]int32 {
						ik, _ := strconv.ParseInt(k, 10, 32)
						return [2]int32{int32(ik), v}
					})
					slices.SortStableFunc(items, func(i, j [2]int32) int {
						if i[0] < j[0] {
							return -1
						}
						return 1
					})
					ports := lo.Map(items, func(item [2]int32, i int) string {
						return fmt.Sprintf("%d", item[1])
					})
					r.Spec.Expose.NodePortSequence = strings.Join(ports, ",")
					// TODO: deprecate NodePortMap in 3.22, for 3.14 use this field
					// r.Spec.Expose.NodePortMap = nil
				}
			}
		}
	case core.RedisStandalone:
		if r.Spec.Replicas == nil {
			r.Spec.Replicas = &RedisReplicas{}
		}
		if r.Spec.Replicas.Sentinel == nil {
			r.Spec.Replicas.Sentinel = &SentinelReplicas{}
		}
		r.Spec.Replicas.Sentinel.Master = pointer.Int32(1)
		r.Spec.Replicas.Sentinel.Slave = nil
		r.Spec.Sentinel = nil

		if r.Spec.Expose.ServiceType == v1.ServiceTypeNodePort {
			if len(r.Spec.Expose.NodePortMap) != 0 {
				items := lo.MapToSlice(r.Spec.Expose.NodePortMap, func(k string, v int32) [2]int32 {
					ik, _ := strconv.ParseInt(k, 10, 32)
					return [2]int32{int32(ik), v}
				})
				slices.SortStableFunc(items, func(i, j [2]int32) int {
					if i[0] < j[0] {
						return -1
					}
					return 1
				})
				ports := lo.Map(items, func(item [2]int32, i int) string {
					return fmt.Sprintf("%d", item[1])
				})
				r.Spec.Expose.NodePortSequence = strings.Join(ports, ",")
				// TODO: deprecate NodePortMap in 3.22, for 3.14 use this field
				// r.Spec.Expose.NodePortMap = nil
			}
		}
	}

	if r.Spec.UpgradeOption == nil || r.Spec.UpgradeOption.AutoUpgrade == nil || *r.Spec.UpgradeOption.AutoUpgrade {
		delete(r.Annotations, config.CRUpgradeableVersion)
		delete(r.Annotations, config.CRUpgradeableComponentVersion)
		r.Spec.UpgradeOption.CRVersion = ""
	}

	// Patch
	if r.Spec.Arch == core.RedisSentinel {
		needCheckAff := r.Spec.UpgradeOption == nil || r.Spec.UpgradeOption.AutoUpgrade == nil || *r.Spec.UpgradeOption.AutoUpgrade
		if r.Spec.UpgradeOption != nil &&
			!strings.HasPrefix(r.Spec.UpgradeOption.CRVersion, "3.14") &&
			!strings.HasPrefix(r.Spec.UpgradeOption.CRVersion, "3.15") &&
			!strings.HasPrefix(r.Spec.UpgradeOption.CRVersion, "3.16") &&
			!strings.HasPrefix(r.Spec.UpgradeOption.CRVersion, "3.17") {
			needCheckAff = true
		}

		// fix sentinel affinity error
		if needCheckAff {
			labels := []string{
				"redissentinels.databases.spotahome.com/name", // NOTE: keep this key as the first
				"middleware.instance/name",
				"app.kubernetes.io/name",
			}
			isLabelSpecified := func(term v1.PodAffinityTerm) bool {
				if term.LabelSelector == nil {
					return false
				}
				for _, exp := range term.LabelSelector.MatchExpressions {
					if slices.Contains(labels, exp.Key) &&
						exp.Operator == metav1.LabelSelectorOpIn &&
						slices.Contains(exp.Values, r.Name) {
						return true
					}
				}
				for k, v := range term.LabelSelector.MatchLabels {
					if slices.Contains(labels, k) && v == r.Name {
						return true
					}
				}
				return false
			}

			if r.Spec.Sentinel.Affinity == nil {
				r.Spec.Sentinel.Affinity = &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{},
				}
			}
			if r.Spec.Sentinel.Affinity.PodAntiAffinity == nil {
				r.Spec.Sentinel.Affinity.PodAntiAffinity = &v1.PodAntiAffinity{}
			}

			antiAff := r.Spec.Sentinel.Affinity.PodAntiAffinity
			if len(antiAff.RequiredDuringSchedulingIgnoredDuringExecution) == 0 {
				foundLabelMatch := false
				for _, item := range antiAff.PreferredDuringSchedulingIgnoredDuringExecution {
					if flag := isLabelSpecified(item.PodAffinityTerm); flag {
						foundLabelMatch = true
						break
					}
				}
				if !foundLabelMatch {
					term := v1.PodAffinityTerm{
						TopologyKey: "kubernetes.io/hostname",
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      labels[0],
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{r.Name},
								},
								{
									Key:      "app.kubernetes.io/component",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"sentinel"},
								},
							},
						},
					}
					antiAff.PreferredDuringSchedulingIgnoredDuringExecution = []v1.WeightedPodAffinityTerm{
						{Weight: 100, PodAffinityTerm: term},
					}
				}
			} else {
				foundLabelMatch := false
				for _, term := range antiAff.RequiredDuringSchedulingIgnoredDuringExecution {
					if flag := isLabelSpecified(term); flag {
						foundLabelMatch = true
						break
					}
				}
				if !foundLabelMatch {
					term := v1.PodAffinityTerm{
						TopologyKey: "kubernetes.io/hostname",
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      labels[0],
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{r.Name},
								},
								{
									Key:      "app.kubernetes.io/component",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"sentinel"},
								},
							},
						},
					}
					antiAff.RequiredDuringSchedulingIgnoredDuringExecution = []v1.PodAffinityTerm{term}
				}
			}
		}
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Redis) ValidateCreate() (warns admission.Warnings, err error) {
	if err := validation.ValidatePasswordSecret(r.Namespace, r.Spec.PasswordSecret, mgrClient, &warns); err != nil {
		return warns, err
	}

	if err := coreVal.ValidateInstanceAccess(&r.Spec.Expose.InstanceAccessBase, getNodeCountByArch(r.Spec.Arch, r.Spec.Replicas), &warns); err != nil {
		return warns, err
	}

	switch r.Spec.Arch {
	case core.RedisCluster:
		if r.Spec.Replicas == nil || r.Spec.Replicas.Cluster == nil || r.Spec.Replicas.Cluster.Shard == nil {
			return nil, fmt.Errorf("instance replicas not specified")
		}
		shards := int32(len(r.Spec.Replicas.Cluster.Shards))
		if shards > 0 {
			if shards != *r.Spec.Replicas.Cluster.Shard {
				return nil, fmt.Errorf("specified shard slots list length not not match shards count")
			}
			var (
				fullSlots *slot.Slots
				total     int
			)
			for _, shard := range r.Spec.Replicas.Cluster.Shards {
				if shardSlots, err := slot.LoadSlots(shard.Slots); err != nil {
					return nil, fmt.Errorf("failed to load shard slots: %v", err)
				} else {
					fullSlots = fullSlots.Union(shardSlots)
					total += shardSlots.Count(slot.SlotAssigned)
				}
			}
			if !fullSlots.IsFullfilled() {
				return nil, fmt.Errorf("specified shard slots not fullfilled")
			}
			if total > slot.RedisMaxSlots {
				return nil, fmt.Errorf("specified shard slots duplicated")
			}
		}
	case core.RedisSentinel:
		if r.Spec.Replicas == nil || r.Spec.Replicas.Sentinel == nil {
			return nil, fmt.Errorf("instance replicas not specified")
		}
		if r.Spec.Replicas.Sentinel == nil || r.Spec.Replicas.Sentinel.Master == nil || *r.Spec.Replicas.Sentinel.Master != 1 {
			return nil, fmt.Errorf("spec.replicas.sentinel.master must be 1")
		}
		if r.Spec.Sentinel == nil {
			return nil, fmt.Errorf("spec.sentinel not specified")
		}
		if r.Spec.Sentinel.Replicas != 0 && (r.Spec.Sentinel.Replicas%2 == 0 || r.Spec.Sentinel.Replicas < 3) {
			return nil, fmt.Errorf("sentinel replicas must be odd and greater >= 3")
		}
		if r.Spec.Sentinel.PasswordSecret != "" {
			if err := validation.ValidatePasswordSecret(r.Namespace, r.Spec.Sentinel.PasswordSecret, mgrClient, &warns); err != nil {
				return warns, fmt.Errorf("sentinel password secret: %v", err)
			}
		}

		if err := coreVal.ValidateInstanceAccess(&r.Spec.Sentinel.Expose.InstanceAccessBase, int(r.Spec.Sentinel.Replicas), &warns); err != nil {
			return warns, err
		}

		if r.Spec.Expose.ServiceType == v1.ServiceTypeNodePort {
			portMap := map[int32]struct{}{}
			if port := r.Spec.Sentinel.Expose.AccessPort; port != 0 {
				portMap[port] = struct{}{}
			}
			ports, _ := corehelper.ParseSequencePorts(r.Spec.Expose.NodePortSequence)
			for _, port := range ports {
				if _, ok := portMap[port]; ok {
					return nil, fmt.Errorf("port %d has assigned to spec.sentinel.expose.accessPort", port)
				}
				portMap[port] = struct{}{}
			}
			ports, _ = corehelper.ParseSequencePorts(r.Spec.Sentinel.Expose.NodePortSequence)
			for _, port := range ports {
				if _, ok := portMap[port]; ok {
					if port == r.Spec.Sentinel.Expose.AccessPort {
						return nil, fmt.Errorf("port %d has assigned to spec.sentinel.expose.accessPort", port)
					}
					return nil, fmt.Errorf("port %d has assigned to spec.expose.dataStorageNodePortSequence", port)
				}
			}
		}
	case core.RedisStandalone:
		if r.Spec.Replicas == nil || r.Spec.Replicas.Sentinel == nil || r.Spec.Replicas.Sentinel.Master == nil || *r.Spec.Replicas.Sentinel.Master != 1 {
			return nil, fmt.Errorf("spec.replicas.sentinel.master must be 1")
		}
		r.Spec.Replicas.Sentinel.Slave = nil
	}

	if r.Spec.EnableTLS {
		ver := redis.RedisVersion(r.Spec.Version)
		if !ver.IsTLSSupported() {
			return warns, fmt.Errorf("tls not supported in version %s", r.Spec.Version)
		}
	}

	if err := validation.ValidateActiveRedisService(r.Spec.EnableActiveRedis, r.Spec.ServiceID, &warns); err != nil {
		return warns, err
	}
	return
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Redis) ValidateUpdate(_ runtime.Object) (warns admission.Warnings, err error) {
	if err := validation.ValidatePasswordSecret(r.Namespace, r.Spec.PasswordSecret, mgrClient, &warns); err != nil {
		return warns, err
	}

	switch r.Spec.Arch {
	case core.RedisCluster:
		if r.Spec.Replicas == nil || r.Spec.Replicas.Cluster == nil || r.Spec.Replicas.Cluster.Shard == nil {
			return nil, fmt.Errorf("instance replicas not specified")
		}

		if err := coreVal.ValidateInstanceAccess(&r.Spec.Expose.InstanceAccessBase,
			helper.CalculateNodeCount(r.Spec.Arch, r.Spec.Replicas.Cluster.Shard, r.Spec.Replicas.Cluster.Slave),
			&warns); err != nil {
			return warns, err
		}

		shards := int32(len(r.Spec.Replicas.Cluster.Shards))
		if shards > 0 && shards == *r.Spec.Replicas.Cluster.Shard {
			// for update validator, only check slots fullfilled
			var (
				fullSlots *slot.Slots
				total     int
			)
			for _, shard := range r.Spec.Replicas.Cluster.Shards {
				if shardSlots, err := slot.LoadSlots(shard.Slots); err != nil {
					return nil, fmt.Errorf("failed to load shard slots: %v", err)
				} else {
					fullSlots = fullSlots.Union(shardSlots)
					total += shardSlots.Count(slot.SlotAssigned)
				}
			}
			if !fullSlots.IsFullfilled() {
				return nil, fmt.Errorf("specified shard slots not fullfilled")
			}
			if total > slot.RedisMaxSlots {
				return nil, fmt.Errorf("specified shard slots duplicated")
			}
		}

		if r.Status.DetailedStatusRef != nil && r.Status.DetailedStatusRef.Name != "" {
			var (
				detailedStatus   clusterv1.DistributedRedisClusterDetailedStatus
				detailedStatusCM v1.ConfigMap
			)
			if err := mgrClient.Get(context.TODO(), client.ObjectKey{
				Namespace: r.Namespace,
				Name:      r.Status.DetailedStatusRef.Name,
			}, &detailedStatusCM); errors.IsNotFound(err) {
			} else if err != nil {
				return nil, err
			} else {
				if err := json.Unmarshal([]byte(detailedStatusCM.Data["status"]), &detailedStatus); err != nil {
					return nil, err
				}
				// validate resource scaling
				dataset := helper.ExtractShardDatasetUsedMemory(r.Name, int(r.Status.LastShardCount), detailedStatus.Nodes)
				if err := validation.ValidateClusterScalingResource(*r.Spec.Replicas.Cluster.Shard, r.Spec.Resources, dataset, &warns); err != nil {
					return warns, err
				}
			}
		}
	case core.RedisSentinel:
		if r.Spec.Replicas == nil || r.Spec.Replicas.Sentinel == nil {
			return nil, fmt.Errorf("instance replicas not specified")
		}
		if r.Spec.Replicas.Sentinel == nil || r.Spec.Replicas.Sentinel.Master == nil || *r.Spec.Replicas.Sentinel.Master != 1 {
			return nil, fmt.Errorf("spec.replicas.sentinel.master must be 1")
		}
		if r.Spec.Sentinel.Replicas != 0 && (r.Spec.Sentinel.Replicas%2 == 0 || r.Spec.Sentinel.Replicas < 3) {
			return nil, fmt.Errorf("sentinel replicas must be odd and greater >= 3")
		}

		if err := coreVal.ValidateInstanceAccess(&r.Spec.Expose.InstanceAccessBase,
			helper.CalculateNodeCount(r.Spec.Arch, r.Spec.Replicas.Sentinel.Master, r.Spec.Replicas.Sentinel.Slave),
			&warns); err != nil {
			return warns, err
		}
		if err := coreVal.ValidateInstanceAccess(&r.Spec.Sentinel.Expose.InstanceAccessBase, int(r.Spec.Sentinel.Replicas), &warns); err != nil {
			return warns, err
		}

		if r.Spec.Expose.ServiceType == v1.ServiceTypeNodePort {
			portMap := map[int32]struct{}{}
			if port := r.Spec.Sentinel.Expose.AccessPort; port != 0 {
				portMap[port] = struct{}{}
			}
			ports, _ := corehelper.ParseSequencePorts(r.Spec.Expose.NodePortSequence)
			for _, port := range ports {
				if _, ok := portMap[port]; ok {
					return nil, fmt.Errorf("port %d has assigned to spec.sentinel.expose.accessPort", port)
				}
				portMap[port] = struct{}{}
			}
			ports, _ = corehelper.ParseSequencePorts(r.Spec.Sentinel.Expose.NodePortSequence)
			for _, port := range ports {
				if _, ok := portMap[port]; ok {
					if port == r.Spec.Sentinel.Expose.AccessPort {
						return nil, fmt.Errorf("port %d has assigned to spec.sentinel.expose.accessPort", port)
					}
					return nil, fmt.Errorf("port %d has assigned to spec.expose.dataStorageNodePortSequence", port)
				}
			}
		}

		if r.Status.DetailedStatusRef != nil && r.Status.DetailedStatusRef.Name != "" {
			var (
				detailedStatus   redisfailoverv1.RedisFailoverDetailedStatus
				detailedStatusCM v1.ConfigMap
			)
			if err := mgrClient.Get(context.TODO(), client.ObjectKey{
				Namespace: r.Namespace,
				Name:      r.Status.DetailedStatusRef.Name,
			}, &detailedStatusCM); errors.IsNotFound(err) {
			} else if err != nil {
				return nil, err
			} else {
				if err := json.Unmarshal([]byte(detailedStatusCM.Data["status"]), &detailedStatus); err != nil {
					return nil, err
				}
				// validate resource scaling
				dataset := helper.ExtractShardDatasetUsedMemory(r.Name, 1, detailedStatus.Nodes)
				if err := validation.ValidateReplicationScalingResource(r.Spec.Resources, dataset[0], &warns); err != nil {
					return warns, err
				}
			}
		}
	case core.RedisStandalone:
		if r.Spec.Replicas == nil || r.Spec.Replicas.Sentinel == nil ||
			r.Spec.Replicas.Sentinel.Master == nil || *r.Spec.Replicas.Sentinel.Master != 1 {
			return nil, fmt.Errorf("spec.replicas.sentinel.master must be 1")
		}

		if err := coreVal.ValidateInstanceAccess(&r.Spec.Expose.InstanceAccessBase,
			helper.CalculateNodeCount(r.Spec.Arch, r.Spec.Replicas.Sentinel.Master, r.Spec.Replicas.Sentinel.Slave),
			&warns); err != nil {
			return warns, err
		}

		if r.Status.DetailedStatusRef != nil && r.Status.DetailedStatusRef.Name != "" {
			var (
				detailedStatus   redisfailoverv1.RedisFailoverDetailedStatus
				detailedStatusCM v1.ConfigMap
			)
			if err := mgrClient.Get(context.TODO(), client.ObjectKey{
				Namespace: r.Namespace,
				Name:      r.Status.DetailedStatusRef.Name,
			}, &detailedStatusCM); errors.IsNotFound(err) {
			} else if err != nil {
				return nil, err
			} else {
				if err := json.Unmarshal([]byte(detailedStatusCM.Data["status"]), &detailedStatus); err != nil {
					return nil, err
				}
				// validate resource scaling
				dataset := helper.ExtractShardDatasetUsedMemory(r.Name, 1, detailedStatus.Nodes)
				if err := validation.ValidateReplicationScalingResource(r.Spec.Resources, dataset[0], &warns); err != nil {
					return warns, err
				}
			}
		}
	}

	if r.Spec.EnableTLS {
		ver := redis.RedisVersion(r.Spec.Version)
		if !ver.IsTLSSupported() {
			return warns, fmt.Errorf("tls not supported in version %s", r.Spec.Version)
		}
	}

	if err := validation.ValidateActiveRedisService(r.Spec.EnableActiveRedis, r.Spec.ServiceID, &warns); err != nil {
		return warns, err
	}
	return
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Redis) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func getNodeCountByArch(arch core.Arch, replicas *RedisReplicas) int {
	if replicas == nil {
		return 0
	}

	switch arch {
	case core.RedisCluster:
		if replicas.Cluster == nil || replicas.Cluster.Shard == nil {
			return 0
		}
		if replicas.Cluster != nil && replicas.Cluster.Slave != nil {
			return int(*replicas.Cluster.Shard) * int((*replicas.Cluster.Slave)+1)
		}
		return int(*replicas.Cluster.Shard)
	case core.RedisSentinel:
		if replicas.Sentinel == nil {
			return 0
		}
		if replicas.Sentinel.Slave != nil {
			return int(*replicas.Sentinel.Slave) + 1
		}
		return 1
	case core.RedisStandalone:
		return 1
	default:
		return 0
	}
}
