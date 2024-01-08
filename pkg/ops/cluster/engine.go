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

package cluster

import (
	"context"
	"fmt"
	"reflect"
	"time"

	clusterv1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/slot"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StatusGenerator
type RuleEngine struct {
	client        kubernetes.ClientSet
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

// NewRuleEngine
func NewRuleEngine(client kubernetes.ClientSet, eventRecorder record.EventRecorder, logger logr.Logger) (*RuleEngine, error) {
	if client == nil {
		return nil, fmt.Errorf("require client set")
	}
	if eventRecorder == nil {
		return nil, fmt.Errorf("require EventRecorder")
	}

	ctrl := RuleEngine{
		client:        client,
		eventRecorder: eventRecorder,
		logger:        logger,
	}
	return &ctrl, nil
}

// Inspect
func (g *RuleEngine) Inspect(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	if g == nil {
		return nil
	}
	cluster := val.(types.RedisClusterInstance)
	logger := g.logger.WithName("Inspect").WithValues("namespace", cluster.GetNamespace(), "name", cluster.GetName())
	if cluster == nil {
		logger.Info("cluster is nil")
		return nil
	}
	cr := cluster.Definition()

	if (cr.Spec.Annotations != nil) && cr.Spec.Annotations[config.PAUSE_ANNOTATION_KEY] != "" {
		return actor.NewResult(CommandEnsureResource)
	}

	// 0. allocate the slots and record them in cr
	check_0 := func() *actor.ActorResult {
		logger.V(3).Info("check_0")
		return g.allocateSlots(ctx, cluster)
	}
	if ret := check_0(); ret != nil {
		return ret
	}

	// 1. check hotconfig: password and config
	check_1 := func() *actor.ActorResult {
		logger.V(3).Info("check_1")

		// check password, make sure the password sync with the cr setting
		// before update password, we should first make sure all the pod supports acl.
		// or the upgrade of redis from 5 to 6 will fail with unexcpeted errors
		if changed, err := g.isPasswordChanged(ctx, cluster); err != nil {
			return actor.NewResultWithValue(CommandRequeue, err)
		} else if changed {
			return actor.NewResult(CommandUpdateAccount)
		}

		if changed, err := g.isConfigMapChanged(ctx, cluster); err != nil {
			return actor.NewResultWithValue(CommandRequeue, err)
		} else if changed {
			return actor.NewResult(CommandUpdateConfig)
		}
		return nil
	}
	if ret := check_1(); ret != nil {
		return ret
	}

	// 2. check if there is enough shard
	check_2 := func() *actor.ActorResult {
		logger.V(3).Info("check_2", "shards", len(cluster.Shards()), "desired", cr.Spec.MasterSize)

		if int(cr.Spec.MasterSize) > len(cluster.Shards()) {
			// 2.1 if missing shard
			return actor.NewResult(CommandEnsureResource)
		}
		if changed, err := g.isCustomServerChanged(ctx, cluster); err != nil {
			logger.Error(err, "isCustomServerChanged")
			return actor.NewResultWithValue(CommandRequeue, err)
		} else if changed {
			logger.Info("isCustomServerChanged actor  CommandEnsureResource start")
			return actor.NewResult(CommandEnsureResource)
		}
		return nil
	}
	if ret := check_2(); ret != nil {
		return ret
	}

	// 3. check if every shard has got one pod to join
	check_3 := func() *actor.ActorResult {
		logger.V(3).Info("check_3")
		var (
			unjoinedNodes    = 0
			readyCount       = 0
			needNodeReplicas = false
		)
		for i, shard := range cluster.Shards() {
			if shard.Index() != i {
				return actor.NewResult(CommandEnsureResource)
			}
			if shard.Status().ReadyReplicas > 0 {
				readyCount += 1
			}
			masterCount := 0
			master := shard.Master()
			for i, node := range shard.Nodes() {
				if i < int(cluster.Definition().Spec.ClusterReplicas)+1 {
					if node.ID() != "" && !node.IsJoined() {
						unjoinedNodes += 1
					}
				}

				// check if exists nodes not joined as replica of master
				if node.Role() == redis.RedisRoleMaster {
					masterCount += 1
				} else if node.Role() == redis.RedisRoleSlave {
					if master != nil {
						if node.MasterID() != master.ID() {
							needNodeReplicas = true
							break
						}

						// when update the announce ip/port, the nodes need to be meet again
						// some nodes need replicate again
						masterIP, masterPort := master.DefaultIP().String(), fmt.Sprintf("%d", master.Port())
						replicas := shard.Replicas()
						logger.V(3).Info("check replica", "masterIP", masterIP, "masterPort", masterPort)
						for _, replica := range replicas {
							logger.V(3).Info("check replica", "replicamasterip", replica.ConfigedMasterIP(), "replicamasterport", replica.ConfigedMasterPort())
							if replica.ConfigedMasterIP() != masterIP || replica.ConfigedMasterPort() != masterPort {

								unjoinedNodes += 1
								break
							}
						}
					}
				}
			}

			if needNodeReplicas = (len(shard.Nodes()) > 0 && masterCount != 1) || needNodeReplicas; needNodeReplicas {
				break
			}
		}

		logger.Info("unjoin", "unjoinedNodes", unjoinedNodes, "needNodeReplicas", needNodeReplicas)
		if unjoinedNodes > 0 || needNodeReplicas {
			// 3.1 if got node not joined, join them
			return actor.NewResult(CommandJoinNode)
		} else if readyCount < int(cluster.Definition().Spec.MasterSize) {
			// 3.2 check why shard still not usable, pod is pending?
			return actor.NewResult(CommandHealPod)
		}
		return nil
	}
	if ret := check_3(); ret != nil {
		return ret
	}

	// 4. here all startup pods is joined together, we should check if the cluster is healthy
	check_4 := func() *actor.ActorResult {
		logger.V(3).Info("check_4")
		for _, shard := range cluster.Shards() {
			if shard.Master() == nil {
				// 4.1 master node not found, pod may failed, do failover
				return actor.NewResult(CommandEnsureSlots)
			}

			for _, node := range shard.Nodes() {
				if node.Role() == redis.RedisRoleMaster && !node.IsJoined() {
					// 4.2 check if every shards has only one master
					return actor.NewResult(CommandJoinNode)
				}
			}
		}

		shardsWithoutSlot := 0
		for _, shard := range cluster.Shards() {
			if shard.Slots().Count(slot.SlotAssigned) == 0 {
				shardsWithoutSlot += 1
			}
		}
		if int(cr.Spec.MasterSize) == len(cluster.Shards()) {
			// 4.4 check if there exists new shards, do slots migrating
			if shardsWithoutSlot > 0 {
				return actor.NewResult(CommandRebalance)
			}
		} else if gap := len(cluster.Shards()) - int(cr.Spec.MasterSize); gap > shardsWithoutSlot {
			// 4.5 found extra shards with slots, do scale down
			//
			// CommandRebalance actor should yield CommandDeleteResource when slots migrate finished
			return actor.NewResult(CommandRebalance)
		} else if gap > 0 && gap == shardsWithoutSlot {
			// 4.6 found extra shards without slots, clean extra statefulset
			return actor.NewResult(CommandCleanResource)
		}
		for _, shard := range cluster.Status().Shards {
			for _, status := range shard.Slots {
				// 4.7 continue failed migrating
				if status.Status == slot.SlotMigrating.String() || status.Status == slot.SlotImporting.String() {
					return actor.NewResult(CommandRebalance)
				}
			}
		}
		return nil
	}
	if ret := check_4(); ret != nil {
		return ret
	}

	// 5. here we should check if all the pods fullfilled
	check_5 := func() *actor.ActorResult {
		logger.V(3).Info("check_5")

		replicas := int(cr.Spec.ClusterReplicas)
		for _, shard := range cluster.Shards() {
			nodeCount := len(shard.Nodes())
			if len(shard.Nodes()) < replicas+1 || len(shard.Nodes()) > replicas+1 {
				lastNode := shard.Nodes()[nodeCount-1]
				// 5.1 shard replica need to scale up
				if lastNode.Index() != replicas {
					return actor.NewResult(CommandEnsureResource)
				}
				// for statefulset, the missing of the middle pod should not happen
			}

			for i := nodeCount - 1; i >= 0; i-- {
				now := time.Now()

				node := shard.Nodes()[i]
				// 5.2 long time termination pod found
				if node.IsTerminating() && now.Sub(node.GetDeletionTimestamp().Time) >= time.Second*30 {
					return actor.NewResult(CommandHealPod)
				}
				// 5.3 long time pending pod found
				if node.Status() == corev1.PodPending &&
					now.Sub(node.GetCreationTimestamp().Time) > time.Second*30 {
					return actor.NewResult(CommandHealPod)
				}

				// 5.4 pod failed
				// TODO: pod fail is hard to check, not clear what caused the fail
			}
		}
		return nil
	}
	if ret := check_5(); ret != nil {
		return ret
	}
	return nil
}

// allocateSlots
// when cluster in these state, we can reallocate the slots
// a. new create shards without slots assigned
// b. the slots is all assigned and the cluster is not scaling up/down
func (g *RuleEngine) allocateSlots(ctx context.Context, cluster types.RedisClusterInstance) *actor.ActorResult {
	cr := cluster.Definition()

	if len(cr.Status.Shards) == 0 {
		var newSlots []*slot.Slots
		for _, shard := range cluster.Shards() {
			if s := shard.Slots(); s != nil {
				newSlots = append(newSlots, s)
			}
		}

		if len(newSlots) == 0 {
			if cr.Spec.MasterSize == int32(len(cr.Spec.Shards)) {
				for _, shard := range cr.Spec.Shards {
					if shardSlots, err := slot.LoadSlots(shard.Slots); err != nil {
						return actor.NewResultWithError(CommandAbort, err)
					} else {
						newSlots = append(newSlots, shardSlots)
					}
				}
			} else {
				newSlots = slot.Allocate(int(cr.Spec.MasterSize), nil)
			}
		} else if !slot.IsFullfilled(newSlots...) {
			// NOTE: refuse to assign slots because the old instance is not healthy
			// the new operator relies to shards assign. if a unhealthy instance comes, we cann't reconcile it.

			err := fmt.Errorf("cann't take over unhealthy instance, please fix the instance first")
			if err := cluster.UpdateStatus(ctx, clusterv1.ClusterStatusKO, err.Error(), nil); err != nil {
				return actor.NewResultWithError(CommandRequeue, err)
			}
			return actor.NewResultWithError(CommandAbort, err)
		}
		shardStatus := buildStatusOfShards(cluster, newSlots)

		g.eventRecorder.Eventf(cluster.Definition(), corev1.EventTypeNormal,
			"PreassignSlots", "preassigned slots for %d shards", cr.Spec.MasterSize)

		if err := cluster.UpdateStatus(ctx, clusterv1.ClusterStatusCreating, "", shardStatus); err != nil {
			return actor.NewResultWithError(CommandRequeue, err)
		}
	}

	// if not fullfilled, not allow rebalance
	if !cluster.IsInService() {
		return nil
	}

	for _, shard := range cluster.Definition().Status.Shards {
		for _, status := range shard.Slots {
			if status.Status == slot.SlotAssigned.String() {
				continue
			}
			shardStatus := buildStatusOfShards(cluster, nil)
			status := cr.Status
			status.Shards = shardStatus
			status.Status = clusterv1.ClusterStatusRebalancing
			status.ClusterStatus = clusterv1.ClusterInService

			g.eventRecorder.Eventf(cluster.Definition(), corev1.EventTypeNormal,
				"UpdateSlotsStatus", "update shards slots status")
			if err := cluster.UpdateStatus(ctx, clusterv1.ClusterStatusRebalancing, "", shardStatus); err != nil {
				return actor.NewResultWithError(CommandRequeue, err)
			}
			return nil
		}
	}

	if currentSize := len(cluster.Definition().Status.Shards); currentSize != int(cr.Spec.MasterSize) {
		var oldSlots []*slot.Slots
		for _, shard := range cluster.Shards() {
			if shard.Slots().Count(slot.SlotAssigned) > 0 {
				oldSlots = append(oldSlots, shard.Slots())
			}
		}

		g.eventRecorder.Eventf(cluster.Definition(), corev1.EventTypeNormal,
			"RebalanceSlots", "rebalance slots assigned status because of scaling %d=>%d",
			currentSize, cr.Spec.MasterSize)

		for i, oldSlot := range oldSlots {
			fmt.Printf("old=index: %d, sots: %s\n", i, oldSlot)
		}
		// scale up/down
		newSlots := slot.Allocate(int(cr.Spec.MasterSize), oldSlots)
		shardStatus := buildStatusOfShards(cluster, newSlots)
		if err := cluster.UpdateStatus(ctx, clusterv1.ClusterStatusRebalancing, "", shardStatus); err != nil {
			return actor.NewResultWithError(CommandRequeue, err)
		}
	}

	// TODO: do reshard to fix slot fragement

	return nil
}

// isPasswordChanged
//
// check logic
// 1. check if secretname same
// 2. check if acl config created
// 3. check if operator user enabled
func (g *RuleEngine) isPasswordChanged(ctx context.Context, cluster types.RedisClusterInstance) (bool, error) {
	logger := g.logger.WithName("isPasswordChanged")

	var (
		cr                = cluster.Definition()
		currentSecretName string
		users             = cluster.Users()
	)

	if cr.Spec.PasswordSecret != nil && cr.Spec.PasswordSecret.Name != "" {
		currentSecretName = cr.Spec.PasswordSecret.Name
	}
	defaultUser := users.GetDefaultUser()
	if defaultUser.GetPassword().GetSecretName() != currentSecretName {
		return true, nil
	}

	name := clusterbuilder.GenerateClusterACLConfigMapName(cluster.GetName())
	if _, err := g.client.GetConfigMap(ctx, cluster.GetNamespace(), name); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "load configmap failed", "target", client.ObjectKey{Namespace: cluster.GetNamespace(), Name: name})
		return false, err
	} else if errors.IsNotFound(err) {
		// init acl configmap
		return true, nil
	}

	isAclEnabled := (users.GetOpUser().Role == user.RoleOperator)
	if cluster.Version().IsACLSupported() && !isAclEnabled {
		return true, nil
	}
	if cluster.Version().IsACLSupported() && !cluster.IsACLUserExists() {
		return true, nil
	}
	return false, nil
}

func (g *RuleEngine) isCustomServerChanged(ctx context.Context, cluster types.RedisClusterInstance) (bool, error) {

	portsMap := make(map[int32]bool)
	cr := cluster.Definition()
	if cr.Spec.Expose.DataStorageNodePortSequence == "" {
		return false, nil
	}
	labels := clusterbuilder.GetClusterLabels(cr.Name, nil)
	svcs, err := g.client.GetServiceByLabels(ctx, cr.Namespace, labels)
	if err != nil {
		return true, err
	}
	ports, err := util.ParsePortSequence(cr.Spec.Expose.DataStorageNodePortSequence)
	for _, v := range ports {
		portsMap[v] = true
	}
	if err != nil {
		return false, err
	}
	nodePortMap := make(map[int32]bool)
	for _, svc := range svcs.Items {
		if svc.Spec.Selector["statefulset.kubernetes.io/pod-name"] != "" {
			if len(svc.Spec.Ports) > 1 {
				nodePortMap[svc.Spec.Ports[0].NodePort] = true
			}
		}
	}
	if !reflect.DeepEqual(portsMap, nodePortMap) {
		return true, err
	}
	return false, err

}

func (g *RuleEngine) isConfigMapChanged(ctx context.Context, cluster types.RedisClusterInstance) (bool, error) {
	logger := g.logger.WithName("isConfigMapChanged")
	newCm, _ := clusterbuilder.NewConfigMapForCR(cluster)
	oldCm, err := g.client.GetConfigMap(ctx, newCm.Namespace, newCm.Name)
	if errors.IsNotFound(err) || oldCm.Data[clusterbuilder.RedisConfKey] == "" {
		return true, nil
	} else if err != nil {
		logger.Error(err, "get old configmap failed")
		return false, err
	}

	// check if config changed
	newConf, _ := clusterbuilder.LoadRedisConfig(newCm.Data[clusterbuilder.RedisConfKey])
	oldConf, _ := clusterbuilder.LoadRedisConfig(oldCm.Data[clusterbuilder.RedisConfKey])
	added, changed, deleted := oldConf.Diff(newConf)
	if len(added)+len(changed)+len(deleted) != 0 {
		return true, nil
	}
	return false, nil
}

func buildStatusOfShards(cluster types.RedisClusterInstance, slots []*slot.Slots) (ret []*clusterv1.ClusterShards) {
	for i, slot := range slots {
		fmt.Printf("new: index: %d, slot: %s\n", i, slot)
	}
	statusShards := cluster.Definition().Status.Shards
	maxShards := len(cluster.Shards())
	if int(cluster.Definition().Spec.MasterSize) > maxShards {
		maxShards = int(cluster.Definition().Spec.MasterSize)
	}
	if len(statusShards) > maxShards {
		maxShards = len(statusShards)
	}

	var (
		isNewAssign    = (len(cluster.Definition().Status.Shards) == 0)
		importingGroup = map[int]map[int]*slot.Slots{}
		migratingGroup = map[int]map[int]*slot.Slots{}
		assignGroup    = map[int]*slot.Slots{}

		slotsFutureShard  = map[int]int{}
		slotsCurrentShard = map[int]int{}
	)

	// calucate importing/migrating slots
	if len(slots) > 0 {
		for i, slot := range slots {
			for _, j := range slot.Slots() {
				slotsFutureShard[j] = i
			}
		}

		var (
			shard     *slot.Slots
			newSlots  *slot.Slots
			oldShards []*slot.Slots
		)

		// build shards status
		for _, shard := range statusShards {
			shardSlot := slot.NewSlots()
			for _, status := range shard.Slots {
				_ = shardSlot.Set(status.Slots, slot.NewSlotAssignStatusFromString(status.Status))
			}
			oldShards = append(oldShards, shardSlot)

			for _, i := range shardSlot.Slots() {
				slotsCurrentShard[i] = int(shard.Index)
			}
		}

		for i := 0; i < maxShards; i++ {
			shard, newSlots = nil, nil
			if i < len(oldShards) {
				shard = oldShards[i]
			}
			if i < len(slots) {
				newSlots = slots[i]
			}

			if !isNewAssign {
				var (
					importingSubGroup = map[int]*slot.Slots{}
					migratingSubGroup = map[int]*slot.Slots{}

					newAddedSlots   = newSlots.Sub(shard)
					newRemovedSlots = shard.Sub(newSlots)
				)
				for _, slotIndex := range newAddedSlots.Slots() {
					shardIndex := slotsCurrentShard[slotIndex]
					if tmpSlot := importingSubGroup[shardIndex]; tmpSlot == nil {
						importingSubGroup[shardIndex] = slot.NewSlots()
					}
					_ = importingSubGroup[shardIndex].Set(slotIndex, slot.SlotAssigned)
				}
				for _, slotIndex := range newRemovedSlots.Slots() {
					shardIndex := slotsFutureShard[slotIndex]
					if tmpSlot := migratingSubGroup[shardIndex]; tmpSlot == nil {
						migratingSubGroup[shardIndex] = slot.NewSlots()
					}
					_ = migratingSubGroup[shardIndex].Set(slotIndex, slot.SlotAssigned)
				}
				importingGroup[i] = importingSubGroup
				migratingGroup[i] = migratingSubGroup
				assignGroup[i] = shard
			} else {
				assignGroup[i] = newSlots
			}
		}
	} else {
		// build shards status
		for _, shard := range cluster.Shards() {
			for _, i := range shard.Slots().Slots() {
				slotsCurrentShard[i] = shard.Index()
			}
		}

		for i, statusShard := range statusShards {
			var (
				assignedSlots     = slot.NewSlots()
				importingSubGroup = map[int]*slot.Slots{}
				migratingSubGroup = map[int]*slot.Slots{}
			)

			for _, status := range statusShard.Slots {
				if status.Status == slot.SlotAssigned.String() {
					_ = assignedSlots.Set(status.Slots, slot.SlotAssigned)
				}
			}
			for _, status := range statusShard.Slots {
				if status.Status == slot.SlotImporting.String() {
					tmpSlots := slot.NewSlots()
					_ = tmpSlots.Set(status.Slots, slot.SlotAssigned)
					for _, slotIndex := range tmpSlots.Slots() {
						// import succeed
						if shardIndex, ok := slotsCurrentShard[slotIndex]; ok && shardIndex == i {
							_ = assignedSlots.Set(slotIndex, slot.SlotAssigned)
						} else {
							if tmpSlot := importingSubGroup[int(*status.ShardIndex)]; tmpSlot == nil {
								importingSubGroup[int(*status.ShardIndex)] = slot.NewSlots()
							}
							_ = importingSubGroup[int(*status.ShardIndex)].Set(slotIndex, slot.SlotAssigned)
						}
					}
				} else if status.Status == slot.SlotMigrating.String() {
					tmpSlots := slot.NewSlots()
					_ = tmpSlots.Set(status.Slots, slot.SlotAssigned)
					for _, slotIndex := range tmpSlots.Slots() {
						if shardIndex, ok := slotsCurrentShard[slotIndex]; ok && shardIndex == int(*status.ShardIndex) {
							_ = assignedSlots.Set(slotIndex, slot.SlotUnAssigned)
						} else {
							if tmpSlot := migratingSubGroup[int(*status.ShardIndex)]; tmpSlot == nil {
								migratingSubGroup[int(*status.ShardIndex)] = slot.NewSlots()
							}
							_ = migratingSubGroup[int(*status.ShardIndex)].Set(slotIndex, slot.SlotAssigned)
						}
					}
				}
			}
			importingGroup[i] = importingSubGroup
			migratingGroup[i] = migratingSubGroup
			assignGroup[i] = assignedSlots
		}
	}

	for i := 0; i < maxShards; i++ {
		foundSlots := false
		status := clusterv1.ClusterShards{
			Index: int32(i),
		}
		if key := assignGroup[i].String(); key != "" {
			status.Slots = append(status.Slots, &clusterv1.ClusterShardsSlotStatus{
				Status:     slot.SlotAssigned.String(),
				Slots:      key,
				ShardIndex: pointer.Int32(int32(i)),
			})
			foundSlots = true
		}
		for targetId, tmpSlots := range importingGroup[i] {
			if key := tmpSlots.String(); key != "" {
				status.Slots = append(status.Slots, &clusterv1.ClusterShardsSlotStatus{
					Status:     slot.SlotImporting.String(),
					Slots:      key,
					ShardIndex: pointer.Int32(int32(targetId)),
				})
			}
			foundSlots = true
		}
		for targetId, tmpSlots := range migratingGroup[i] {
			if key := tmpSlots.String(); key != "" {
				status.Slots = append(status.Slots, &clusterv1.ClusterShardsSlotStatus{
					Status:     slot.SlotMigrating.String(),
					Slots:      key,
					ShardIndex: pointer.Int32(int32(targetId)),
				})
			}
			foundSlots = true
		}
		if i >= int(cluster.Definition().Spec.MasterSize) && !foundSlots {
			continue
		}
		ret = append(ret, &status)
	}
	return
}
