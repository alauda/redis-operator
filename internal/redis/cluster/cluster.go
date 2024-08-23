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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"slices"
	"strconv"
	"strings"

	clusterv1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/api/core/helper"
	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/builder/clusterbuilder"
	"github.com/alauda/redis-operator/internal/config"
	"github.com/alauda/redis-operator/internal/util"
	clientset "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/security/acl"
	"github.com/alauda/redis-operator/pkg/slot"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ types.RedisInstance        = (*RedisCluster)(nil)
	_ types.RedisClusterInstance = (*RedisCluster)(nil)
)

type RedisCluster struct {
	clusterv1.DistributedRedisCluster

	client        clientset.ClientSet
	eventRecorder record.EventRecorder
	redisUsers    []*redismiddlewarealaudaiov1.RedisUser
	shards        []types.RedisClusterShard
	users         acl.Users
	tlsConfig     *tls.Config

	logger logr.Logger
}

// NewRedisCluster
func NewRedisCluster(ctx context.Context, k8sClient clientset.ClientSet, eventRecorder record.EventRecorder, def *clusterv1.DistributedRedisCluster, logger logr.Logger) (*RedisCluster, error) {
	cluster := RedisCluster{
		DistributedRedisCluster: *def,

		client:        k8sClient,
		eventRecorder: eventRecorder,
		logger:        logger.WithName("C").WithValues("instanec", client.ObjectKeyFromObject(def).String()),
	}

	var err error
	// load after shard
	if cluster.users, err = cluster.loadUsers(ctx); err != nil {
		cluster.logger.Error(err, "loads users failed")
		return nil, err
	}

	// load tls
	if cluster.tlsConfig, err = cluster.loadTLS(ctx); err != nil {
		cluster.logger.Error(err, "loads tls failed")
		return nil, err
	}

	// load shards
	if cluster.shards, err = LoadRedisClusterShards(ctx, k8sClient, &cluster, cluster.logger); err != nil {
		cluster.logger.Error(err, "loads cluster shards failed", "cluster", def.Name)
		return nil, err
	}

	if cluster.Version().IsACLSupported() {
		cluster.LoadRedisUsers(ctx)

	}
	return &cluster, nil
}

func (c *RedisCluster) Arch() redis.RedisArch {
	return core.RedisCluster
}

func (c *RedisCluster) NamespacedName() client.ObjectKey {
	return client.ObjectKey{Namespace: c.GetNamespace(), Name: c.GetName()}
}

func (c *RedisCluster) LoadRedisUsers(ctx context.Context) {
	oldOpUser, _ := c.client.GetRedisUser(ctx, c.GetNamespace(), clusterbuilder.GenerateClusterOperatorsRedisUserName(c.GetName()))
	oldDefultUser, _ := c.client.GetRedisUser(ctx, c.GetNamespace(), clusterbuilder.GenerateClusterDefaultRedisUserName(c.GetName()))
	c.redisUsers = []*redismiddlewarealaudaiov1.RedisUser{oldOpUser, oldDefultUser}
}

// ctx
func (c *RedisCluster) Restart(ctx context.Context, annotationKeyVal ...string) error {
	if c == nil {
		return nil
	}
	for _, shard := range c.shards {
		if err := shard.Restart(ctx); errors.IsNotFound(err) {
			continue
		} else {
			return err
		}
	}
	return nil
}

// Refresh refresh users, shards
func (c *RedisCluster) Refresh(ctx context.Context) error {
	if c == nil {
		return nil
	}
	logger := c.logger.WithName("Refresh")

	// load cr
	var cr clusterv1.DistributedRedisCluster
	if err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		if errors.IsInternalError(err) ||
			errors.IsServerTimeout(err) ||
			errors.IsTimeout(err) ||
			errors.IsTooManyRequests(err) ||
			errors.IsServiceUnavailable(err) {
			return true
		}
		return false
	}, func() error {
		return c.client.Client().Get(ctx, client.ObjectKeyFromObject(&c.DistributedRedisCluster), &cr)
	}); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		logger.Error(err, "get DistributedRedisCluster failed")
		return err
	}
	_ = cr.Default()
	c.DistributedRedisCluster = cr

	var err error
	if c.users, err = c.loadUsers(ctx); err != nil {
		logger.Error(err, "load users failed")
		return err
	}

	if c.shards, err = LoadRedisClusterShards(ctx, c.client, c, logger); err != nil {
		logger.Error(err, "refresh cluster shards failed", "cluster", c.GetName())
		return err
	}
	return nil
}

// RewriteShards
func (c *RedisCluster) RewriteShards(ctx context.Context, shards []*clusterv1.ClusterShards) error {
	if c == nil || len(shards) == 0 {
		return nil
	}
	logger := c.logger.WithName("RewriteShards")

	if err := c.Refresh(ctx); err != nil {
		return err
	}
	cr := &c.DistributedRedisCluster
	if len(cr.Status.Shards) == 0 || c.IsInService() {
		// only update shards when cluster in service
		cr.Status.Shards = shards
	}
	if err := c.client.UpdateDistributedRedisClusterStatus(ctx, cr); err != nil {
		logger.Error(err, "update DistributedRedisCluster status failed")
		return err
	}
	return c.UpdateStatus(ctx, types.Any, "")
}

func (c *RedisCluster) UpdateStatus(ctx context.Context, st types.InstanceStatus, message string) error {
	if c == nil {
		return nil
	}
	logger := c.logger.WithName("UpdateStatus")

	if err := c.Refresh(ctx); err != nil {
		return err
	}

	var (
		cr               = &c.DistributedRedisCluster
		status           clusterv1.ClusterStatus
		isResourceReady  = (len(c.shards) == int(cr.Spec.MasterSize))
		isRollingRestart = false
		isSlotMigrating  = false
		allSlots         = slot.NewSlots()
		unSchedulePods   []string
		messages         []string
	)
	switch st {
	case types.OK:
		status = clusterv1.ClusterStatusOK
	case types.Fail:
		status = clusterv1.ClusterStatusKO
	case types.Paused:
		status = clusterv1.ClusterStatusPaused
	}
	if message != "" {
		messages = append(messages, message)
	}
	cr.Status.ClusterStatus = clusterv1.ClusterOutOfService
	if c.IsInService() {
		cr.Status.ClusterStatus = clusterv1.ClusterInService
	}

__end_slot_migrating__:
	for _, shards := range cr.Status.Shards {
		for _, status := range shards.Slots {
			if status.Status == slot.SlotMigrating.String() || status.Status == slot.SlotImporting.String() {
				isSlotMigrating = true
				break __end_slot_migrating__
			}
		}
	}

	// check if all resources fullfilled
	for i, shard := range c.shards {
		if i != shard.Index() ||
			shard.Status().ReadyReplicas != cr.Spec.ClusterReplicas+1 ||
			len(shard.Replicas()) != int(cr.Spec.ClusterReplicas) {
			isResourceReady = false
		}

		if shard.Status().CurrentRevision != shard.Status().UpdateRevision &&
			(*shard.Definition().Spec.Replicas != shard.Status().ReadyReplicas ||
				shard.Status().UpdatedReplicas != shard.Status().Replicas ||
				shard.Status().ReadyReplicas != shard.Status().CurrentReplicas) {
			isRollingRestart = true
		}
		slots := shard.Slots()
		if i < len(cr.Status.Shards) {
			allSlots = allSlots.Union(slots)
		}

		// output message for pending pods
		for _, node := range shard.Nodes() {
			if node.Status() == corev1.PodPending {
				for _, cond := range node.Definition().Status.Conditions {
					if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
						unSchedulePods = append(unSchedulePods, node.GetName())
					}
				}
			}
		}
	}
	if len(unSchedulePods) > 0 {
		messages = append(messages, fmt.Sprintf("pods %s unschedulable", strings.Join(unSchedulePods, ",")))
	}

	if cr.Status.ClusterStatus == clusterv1.ClusterOutOfService && allSlots.Count(slot.SlotAssigned) > 0 {
		subSlots := slot.NewFullSlots().Sub(allSlots)
		messages = append(messages, fmt.Sprintf("slots %s missing", subSlots.String()))
	}

	if status != "" {
		cr.Status.Status = status
	} else {
		if isRollingRestart {
			cr.Status.Status = clusterv1.ClusterStatusRollingUpdate
		} else if isSlotMigrating {
			cr.Status.Status = clusterv1.ClusterStatusRebalancing
		} else if isResourceReady {
			if cr.Status.ClusterStatus != clusterv1.ClusterInService {
				cr.Status.Status = clusterv1.ClusterStatusKO
			} else {
				cr.Status.Status = clusterv1.ClusterStatusOK
				cr.Status.Reason = "OK"
			}
		} else {
			cr.Status.Status = clusterv1.ClusterStatusCreating
		}
	}
	if cr.Status.Status == clusterv1.ClusterStatusRebalancing {
		var migratingSlots []string
		for _, shards := range cr.Status.Shards {
			for _, status := range shards.Slots {
				if status.Status == slot.SlotMigrating.String() {
					migratingSlots = append(migratingSlots, status.Slots)
				}
			}
		}
		if len(migratingSlots) > 0 {
			message = fmt.Sprintf("slots %s migrating", strings.Join(migratingSlots, ","))
			messages = append(messages, message)
		}
	}
	cr.Status.Reason = strings.Join(messages, "; ")

	if cr.Status.Status == clusterv1.ClusterStatusOK &&
		c.Spec.Expose.ServiceType == corev1.ServiceTypeNodePort &&
		c.Spec.Expose.NodePortSequence != "" {

		nodeports := map[int32]struct{}{}
		for _, node := range c.Nodes() {
			if port := node.Definition().Labels[builder.PodAnnouncePortLabelKey]; port != "" {
				val, _ := strconv.ParseInt(port, 10, 32)
				nodeports[int32(val)] = struct{}{}
			}
		}

		assignedPorts, _ := helper.ParseSequencePorts(cr.Spec.Expose.NodePortSequence)
		// check nodeport applied
		notAppliedPorts := []string{}
		for _, port := range assignedPorts {
			if _, ok := nodeports[port]; !ok {
				notAppliedPorts = append(notAppliedPorts, fmt.Sprintf("%d", port))
			}
		}
		if len(notAppliedPorts) > 0 {
			cr.Status.Status = clusterv1.ClusterStatusRollingUpdate
			cr.Status.Reason = fmt.Sprintf("nodeport %s not applied", notAppliedPorts)
		}
	}

	cr.Status.Nodes = cr.Status.Nodes[0:0]
	cr.Status.NumberOfMaster = 0
	cr.Status.NodesPlacement = clusterv1.NodesPlacementInfoOptimal
	var (
		nodePlacement = map[string]struct{}{}
		detailedNodes []core.RedisDetailedNode
	)

	// update master count and node info
	for _, shard := range c.shards {
		master := shard.Master()
		if master != nil {
			cr.Status.NumberOfMaster += 1
		}
		for _, node := range shard.Nodes() {
			if _, ok := nodePlacement[node.NodeIP().String()]; ok {
				cr.Status.NodesPlacement = clusterv1.NodesPlacementInfoBestEffort
			} else {
				nodePlacement[node.NodeIP().String()] = struct{}{}
			}
			cnode := core.RedisDetailedNode{
				RedisNode: core.RedisNode{
					ID:          node.ID(),
					Role:        node.Role(),
					MasterRef:   node.MasterID(),
					IP:          node.DefaultIP().String(),
					Port:        fmt.Sprintf("%d", node.Port()),
					PodName:     node.GetName(),
					StatefulSet: shard.GetName(),
					NodeName:    node.NodeIP().String(),
					Slots:       []string{},
				},
				Version:           node.Info().RedisVersion,
				UsedMemory:        node.Info().UsedMemory,
				UsedMemoryDataset: node.Info().UsedMemoryDataset,
			}
			if v := node.Slots().String(); v != "" {
				cnode.Slots = append(cnode.Slots, v)
			}
			cr.Status.Nodes = append(cr.Status.Nodes, cnode.RedisNode)
			detailedNodes = append(detailedNodes, cnode)
		}
	}

	// update status configmap
	detailedStatus := clusterv1.NewDistributedRedisClusterDetailedStatus(&cr.Status, detailedNodes)
	detailedStatusCM, _ := clusterbuilder.NewRedisClusterDetailedStatusConfigMap(c, detailedStatus)
	if oldDetailedStatusCM, err := c.client.GetConfigMap(ctx, c.GetNamespace(), detailedStatusCM.Name); errors.IsNotFound(err) {
		if err := c.client.CreateConfigMap(ctx, c.GetNamespace(), detailedStatusCM); err != nil {
			logger.Error(err, "update detailed status failed")
		}
	} else if err != nil {
		logger.Error(err, "get detailed status configmap failed")
	} else if clusterbuilder.ShouldUpdateDetailedStatusConfigMap(oldDetailedStatusCM, detailedStatus) {
		if err := c.client.UpdateConfigMap(ctx, c.GetNamespace(), detailedStatusCM); err != nil {
			logger.Error(err, "update detailed status failed")
		}
	}

	if cr.Status.DetailedStatusRef == nil {
		cr.Status.DetailedStatusRef = &corev1.ObjectReference{}
		cr.Status.DetailedStatusRef.Kind = "ConfigMap"
		cr.Status.DetailedStatusRef.Name = detailedStatusCM.Name
	}
	if err := c.client.UpdateDistributedRedisClusterStatus(ctx, cr); errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		logger.Error(err, "get DistributedRedisCluster failed")
		return err
	}
	return nil
}

// Status return the status of the cluster
func (c *RedisCluster) Status() *clusterv1.DistributedRedisClusterStatus {
	if c == nil {
		return nil
	}
	return &c.DistributedRedisCluster.Status
}

// Definition
func (c *RedisCluster) Definition() *clusterv1.DistributedRedisCluster {
	if c == nil {
		return nil
	}
	return &c.DistributedRedisCluster
}

// Version
func (c *RedisCluster) Version() redis.RedisVersion {
	if c == nil {
		return redis.RedisVersionUnknown
	}

	if version, err := redis.ParseRedisVersionFromImage(c.Spec.Image); err != nil {
		c.logger.Error(err, "parse redis version failed")
		return redis.RedisVersionUnknown
	} else {
		return version
	}
}

func (c *RedisCluster) Shards() []types.RedisClusterShard {
	if c == nil {
		return nil
	}
	return c.shards
}

func (c *RedisCluster) Nodes() []redis.RedisNode {
	var ret []redis.RedisNode
	for _, shard := range c.shards {
		ret = append(ret, shard.Nodes()...)
	}
	return ret
}

func (c *RedisCluster) RawNodes(ctx context.Context) ([]corev1.Pod, error) {
	if c == nil {
		return nil, nil
	}

	selector := clusterbuilder.GetClusterStatefulsetSelectorLabels(c.GetName(), -1)
	// load pods by statefulset selector
	ret, err := c.client.GetStatefulSetPodsByLabels(ctx, c.GetNamespace(), selector)
	if err != nil {
		c.logger.Error(err, "loads pods of sentinel statefulset failed")
		return nil, err
	}
	return ret.Items, nil
}

func (c *RedisCluster) Masters() []redis.RedisNode {
	var ret []redis.RedisNode
	for _, shard := range c.shards {
		ret = append(ret, shard.Master())
	}
	return ret
}

// IsInService
func (c *RedisCluster) IsInService() bool {
	if c == nil {
		return false
	}

	slots := slot.NewSlots()
	// check is cluster slots is fullfilled
	for _, shard := range c.Shards() {
		slots = slots.Union(shard.Slots())
	}
	return slots.IsFullfilled()
}

// IsReady
func (c *RedisCluster) IsReady() bool {
	for _, shard := range c.shards {
		status := shard.Status()
		if !(status.ReadyReplicas == *shard.Definition().Spec.Replicas &&
			((status.CurrentRevision == status.UpdateRevision && status.CurrentReplicas == status.ReadyReplicas) || status.UpdateRevision == "")) {

			return false
		}
	}
	return true
}

func (c *RedisCluster) Users() (us acl.Users) {
	if c == nil {
		return nil
	}

	// clone before return
	for _, user := range c.users {
		u := *user
		if u.Password != nil {
			p := *u.Password
			u.Password = &p
		}
		us = append(us, &u)
	}
	return
}

func (c *RedisCluster) TLSConfig() *tls.Config {
	if c == nil {
		return nil
	}
	return c.tlsConfig
}

// TLS
func (c *RedisCluster) TLS() *tls.Config {
	if c == nil {
		return nil
	}
	return c.tlsConfig
}

// loadUsers
func (c *RedisCluster) loadUsers(ctx context.Context) (acl.Users, error) {
	var (
		name  = clusterbuilder.GenerateClusterACLConfigMapName(c.GetName())
		users acl.Users
	)
	// NOTE: load acl config first. if acl config not exists, then this may be
	// an old instance(upgrade from old redis or operator version).
	// migrate old password account to acl
	if cm, err := c.client.GetConfigMap(ctx, c.GetNamespace(), name); errors.IsNotFound(err) {
		var (
			username       string
			passwordSecret string
			currentSecret  string
			secret         *corev1.Secret
		)
		if c.Spec.PasswordSecret != nil {
			currentSecret = c.Spec.PasswordSecret.Name
		}

		// load current tls secret.
		// because previous cr not recorded the secret name, we should load it from statefulset
		exists := false
		for i := 0; i < int(c.Spec.MasterSize); i += 2 {
			statefulsetName := clusterbuilder.ClusterStatefulSetName(c.GetName(), i)
			sts, err := c.client.GetStatefulSet(ctx, c.GetNamespace(), statefulsetName)
			if err != nil {
				if !errors.IsNotFound(err) {
					c.logger.Error(err, "load statefulset failed", "target", util.ObjectKey(c.GetNamespace(), c.GetName()))
				}
				continue
			}
			exists = true
			spec := sts.Spec.Template.Spec
			if container := util.GetContainerByName(&spec, clusterbuilder.ServerContainerName); container != nil {
				for _, env := range container.Env {
					if env.Name == clusterbuilder.PasswordENV && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
						passwordSecret = env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
					} else if env.Name == clusterbuilder.OperatorSecretName && env.Value != "" {
						passwordSecret = env.Value
					} else if env.Name == clusterbuilder.OperatorUsername {
						username = env.Value
					}
				}
			}
			if passwordSecret != currentSecret {
				break
			}
		}
		if !exists {
			username = user.DefaultUserName
			passwordSecret = currentSecret
			if c.Version().IsACLSupported() {
				username = user.DefaultOperatorUserName
				passwordSecret = clusterbuilder.GenerateClusterACLOperatorSecretName(c.GetName())
			}
		}
		if passwordSecret != "" {
			objKey := client.ObjectKey{Namespace: c.GetNamespace(), Name: passwordSecret}
			if secret, err = c.loadUserSecret(ctx, objKey); err != nil {
				c.logger.Error(err, "load user secret failed", "target", objKey)
				return nil, err
			}
		}
		role := user.RoleDeveloper
		if username == user.DefaultOperatorUserName {
			role = user.RoleOperator
		} else if username == "" {
			username = user.DefaultUserName
		}
		if role == user.RoleOperator {
			if u, err := user.NewOperatorUser(secret, c.Version().IsACL2Supported()); err != nil {
				c.logger.Error(err, "init users failed")
				return nil, err
			} else {
				users = append(users, u)
			}
			u, _ := user.NewUser(user.DefaultUserName, user.RoleDeveloper, nil, c.Version().IsACL2Supported())
			users = append(users, u)
		} else {
			if u, err := user.NewUser(username, role, secret, c.Version().IsACL2Supported()); err != nil {
				c.logger.Error(err, "init users failed")
				return nil, err
			} else {
				users = append(users, u)
			}
		}
	} else if err != nil {
		c.logger.Error(err, "get acl configmap failed", "name", name)
		return nil, err
	} else if users, err = acl.LoadACLUsers(ctx, c.client, cm); err != nil {
		c.logger.Error(err, "load acl failed")
		return nil, err
	}

	var (
		defaultUser = users.GetDefaultUser()
		rule        *user.Rule
	)
	if len(defaultUser.Rules) > 0 {
		rule = defaultUser.Rules[0]
	} else {
		rule = &user.Rule{}
	}
	if c.Version().IsACL2Supported() {
		rule.Channels = []string{"*"}
	}

	renameVal := c.Definition().Spec.Config[clusterbuilder.RedisConfig_RenameCommand]
	renames, _ := clusterbuilder.ParseRenameConfigs(renameVal)
	if len(renames) > 0 {
		rule.DisallowedCommands = []string{}
		for key, val := range renames {
			if key != val && !slices.Contains(rule.DisallowedCommands, key) {
				rule.DisallowedCommands = append(rule.DisallowedCommands, key)
			}
		}
	}
	defaultUser.Rules = append(defaultUser.Rules[0:0], rule)

	return users, nil
}

// loadUserSecret
func (c *RedisCluster) loadUserSecret(ctx context.Context, objKey client.ObjectKey) (*corev1.Secret, error) {
	secret, err := c.client.GetSecret(ctx, objKey.Namespace, objKey.Name)
	if err != nil && !errors.IsNotFound(err) {
		c.logger.Error(err, "load default users's password secret failed", "target", objKey.String())
		return nil, err
	} else if errors.IsNotFound(err) {
		if objKey.Name == clusterbuilder.GenerateClusterACLOperatorSecretName(c.GetName()) {
			secret = clusterbuilder.NewClusterOpSecret(c.Definition())
			err := c.client.CreateSecret(ctx, objKey.Namespace, secret)
			if err != nil {
				return nil, err
			}
		}
	} else if _, ok := secret.Data[user.PasswordSecretKey]; !ok {
		return nil, fmt.Errorf("no password found")
	}
	return secret, nil
}

func (c *RedisCluster) IsACLUserExists() bool {
	if !c.Version().IsACLSupported() {
		return false
	}
	if len(c.redisUsers) == 0 {
		return false
	}
	for _, v := range c.redisUsers {
		if v == nil {
			return false
		}
		if v.Spec.Username == "" {
			return false
		}
	}
	return true
}

func (c *RedisCluster) IsResourceFullfilled(ctx context.Context) (bool, error) {
	var (
		serviceKey = corev1.SchemeGroupVersion.WithKind("Service")
		stsKey     = appsv1.SchemeGroupVersion.WithKind("StatefulSet")
	)
	resources := map[schema.GroupVersionKind][]string{
		serviceKey: {c.GetName()}, // <name>
	}
	for i := 0; i < int(c.Spec.MasterSize); i++ {
		headlessSvcName := clusterbuilder.ClusterHeadlessSvcName(c.GetName(), i)
		resources[serviceKey] = append(resources[serviceKey], headlessSvcName) // <name>-<index>

		stsName := clusterbuilder.ClusterStatefulSetName(c.GetName(), i)
		resources[stsKey] = append(resources[stsKey], stsName)
	}
	if c.Spec.Expose.ServiceType == corev1.ServiceTypeLoadBalancer || c.Spec.Expose.ServiceType == corev1.ServiceTypeNodePort {
		resources[serviceKey] = append(resources[serviceKey], clusterbuilder.RedisNodePortSvcName(c.GetName())) // drc-<name>-nodeport
		for i := 0; i < int(c.Spec.MasterSize); i++ {
			stsName := clusterbuilder.ClusterStatefulSetName(c.GetName(), i)
			for j := 0; j < int(c.Spec.ClusterReplicas+1); j++ {
				resources[serviceKey] = append(resources[serviceKey], fmt.Sprintf("%s-%d", stsName, j))
			}
		}
	}

	for gvk, names := range resources {
		for _, name := range names {
			var obj unstructured.Unstructured
			obj.SetGroupVersionKind(gvk)

			err := c.client.Client().Get(ctx, client.ObjectKey{Namespace: c.GetNamespace(), Name: name}, &obj)
			if errors.IsNotFound(err) {
				c.logger.V(3).Info("resource not found", "target", util.ObjectKey(c.GetNamespace(), name))
				return false, nil
			} else if err != nil {
				c.logger.Error(err, "get resource failed", "target", util.ObjectKey(c.GetNamespace(), name))
				return false, err
			}
		}
	}

	for i := 0; i < int(c.Spec.MasterSize); i++ {
		stsName := clusterbuilder.ClusterStatefulSetName(c.GetName(), i)
		sts, err := c.client.GetStatefulSet(ctx, c.GetNamespace(), stsName)
		if err != nil {
			if errors.IsNotFound(err) {
				c.logger.V(3).Info("statefulset not found", "target", util.ObjectKey(c.GetNamespace(), stsName))
				return false, nil
			}
			c.logger.Error(err, "get statefulset failed", "target", util.ObjectKey(c.GetNamespace(), stsName))
			return false, err
		}
		if sts.Spec.Replicas == nil || *sts.Spec.Replicas != c.Spec.ClusterReplicas+1 {
			return false, nil
		}
		redisToolsImage := config.GetRedisToolsImage(c)
		if redisToolsImage == "" {
			return false, fmt.Errorf("redis-tools image not found")
		}
		spec := sts.Spec.Template.Spec
		for _, container := range append(append([]corev1.Container{}, spec.InitContainers...), spec.Containers...) {
			if !strings.Contains(container.Image, "middleware/redis-tools") {
				continue
			}
			if container.Image != redisToolsImage {
				return false, nil
			}
		}
	}
	return true, nil
}

func (c *RedisCluster) IsACLAppliedToAll() bool {
	if c == nil || !c.Version().IsACLSupported() {
		return false
	}
	for _, shard := range c.Shards() {
		for _, node := range shard.Nodes() {
			if !node.CurrentVersion().IsACLSupported() || !node.IsACLApplied() {
				return false
			}
		}
	}
	return true
}

func (c *RedisCluster) Logger() logr.Logger {
	if c == nil {
		return logr.Discard()
	}
	return c.logger
}

func (c *RedisCluster) SendEventf(eventtype, reason, messageFmt string, args ...interface{}) {
	if c == nil {
		return
	}
	c.eventRecorder.Eventf(c.Definition(), eventtype, reason, messageFmt, args...)
}

// loadTLS
func (c *RedisCluster) loadTLS(ctx context.Context) (*tls.Config, error) {
	if c == nil {
		return nil, nil
	}
	logger := c.logger.WithName("loadTLS")

	var secretName string

	// load current tls secret.
	// because previous cr not recorded the secret name, we should load it from statefulset
	for i := 0; i < int(c.Spec.MasterSize); i += 2 {
		statefulsetName := clusterbuilder.ClusterStatefulSetName(c.GetName(), i)
		if sts, err := c.client.GetStatefulSet(ctx, c.GetNamespace(), statefulsetName); err != nil {
			if !errors.IsNotFound(err) {
				c.logger.Error(err, "load statefulset failed", "target", util.ObjectKey(c.GetNamespace(), c.GetName()))
			}
			continue
		} else {
			for _, vol := range sts.Spec.Template.Spec.Volumes {
				if vol.Name == clusterbuilder.RedisTLSVolumeName {
					secretName = vol.VolumeSource.Secret.SecretName
				}
			}
		}
		break
	}

	if secretName == "" {
		return nil, nil
	}

	if secret, err := c.client.GetSecret(ctx, c.GetNamespace(), secretName); err != nil {
		logger.Error(err, "secret not found", "name", secretName)
		return nil, err
	} else if secret.Data[corev1.TLSCertKey] == nil || secret.Data[corev1.TLSPrivateKeyKey] == nil ||
		secret.Data["ca.crt"] == nil {

		logger.Error(fmt.Errorf("invalid tls secret"), "tls secret is invaid")
		return nil, fmt.Errorf("tls secret is invalid")
	} else {
		cert, err := tls.X509KeyPair(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey])
		if err != nil {
			logger.Error(err, "generate X509KeyPair failed")
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(secret.Data["ca.crt"])

		return &tls.Config{
			InsecureSkipVerify: true, // #nosec
			RootCAs:            caCertPool,
			Certificates:       []tls.Certificate{cert},
		}, nil
	}
}
