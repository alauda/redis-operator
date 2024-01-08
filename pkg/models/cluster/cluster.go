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
	"strings"

	clusterv1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/redis/v1"
	clientset "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/security/acl"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/slot"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.RedisInstance = (*RedisCluster)(nil)

type RedisCluster struct {
	clusterv1.DistributedRedisCluster

	ctx        context.Context
	client     clientset.ClientSet
	redisUsers []*redismiddlewarealaudaiov1.RedisUser
	shards     []types.RedisClusterShard
	// version    redis.RedisVersion
	users     acl.Users
	tlsConfig *tls.Config
	configmap map[string]string

	logger logr.Logger
}

// NewRedisCluster
func NewRedisCluster(ctx context.Context, k8sClient clientset.ClientSet, def *clusterv1.DistributedRedisCluster, logger logr.Logger) (*RedisCluster, error) {
	cluster := RedisCluster{
		DistributedRedisCluster: *def,
		ctx:                     ctx,
		client:                  k8sClient,
		configmap:               map[string]string{},
		logger:                  logger.WithName("RedisCluster"),
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
	if cluster.shards, err = LoadRedisClusterShards(ctx, k8sClient, &cluster, logger); err != nil {
		cluster.logger.Error(err, "loads cluster shards failed", "cluster", def.Name)
		return nil, err
	}

	if cluster.Version().IsACLSupported() {
		cluster.LoadRedisUsers(ctx)

	}
	return &cluster, nil
}

func (c *RedisCluster) LoadRedisUsers(ctx context.Context) {
	oldOpUser, _ := c.client.GetRedisUser(ctx, c.GetNamespace(), clusterbuilder.GenerateClusterOperatorsRedisUserName(c.GetName()))
	oldDefultUser, _ := c.client.GetRedisUser(ctx, c.GetNamespace(), clusterbuilder.GenerateClusterDefaultRedisUserName(c.GetName()))
	c.redisUsers = []*redismiddlewarealaudaiov1.RedisUser{oldOpUser, oldDefultUser}
}

// ctx
func (c *RedisCluster) Restart(ctx context.Context) error {
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
	_ = cr.Init()
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

// UpdateStatus
func (c *RedisCluster) UpdateStatus(ctx context.Context, status clusterv1.ClusterStatus, message string, shards []*clusterv1.ClusterShards) error {
	if c == nil {
		return nil
	}
	logger := c.logger.WithName("UpdateStatus")

	logger.Info("UpdateStatus")

	cr := &c.DistributedRedisCluster
	cr.Status.ClusterStatus = clusterv1.ClusterOutOfService
	if c.IsInService() {
		cr.Status.ClusterStatus = clusterv1.ClusterInService
	}
	// force update the message
	if status != "" {
		if err := c.client.Client().Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
			logger.Error(err, "get DistributedRedisCluster failed")
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		cr.Status.Status = status
		cr.Status.Reason = message
		if len(cr.Status.Shards) == 0 || status == clusterv1.ClusterStatusRebalancing {
			cr.Status.Shards = shards
		}
	} else {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		cr.Status.ClusterStatus = clusterv1.ClusterOutOfService
		if c.IsInService() {
			cr.Status.ClusterStatus = clusterv1.ClusterInService
		}

		var (
			isResourceReady  = (len(c.shards) == int(cr.Spec.MasterSize))
			isRollingRestart = false
			isSlotMigrating  = false
			isHealthy        = true
			allSlots         = slot.NewSlots()
			unSchedulePods   []string
		)

		for _, shards := range cr.Status.Shards {
			for _, status := range shards.Slots {
				if status.Status == slot.SlotMigrating.String() || status.Status == slot.SlotImporting.String() {
					isSlotMigrating = true
				}
			}
		}

		// check if all resources fullfilled
		for i, shard := range c.shards {
			if i != shard.Index() || shard.Status().ReadyReplicas != cr.Spec.ClusterReplicas+1 {
				isHealthy = false
				isResourceReady = false
				break
			}
			if len(shard.Replicas()) != int(cr.Spec.ClusterReplicas) {
				isHealthy = false
			}

			if shard.Status().UpdateRevision != "" &&
				(shard.Status().CurrentRevision != shard.Status().UpdateRevision ||
					*shard.Definition().Spec.Replicas != shard.Status().UpdatedReplicas) {

				isRollingRestart = true
			} else if shard.Definition().Spec.Replicas != &shard.Status().ReadyReplicas {
				isResourceReady = false
			}
			slots := shard.Slots()
			allSlots = allSlots.Union(slots)

			// output message for pending pods
			for _, node := range shard.Nodes() {
				if node.Status() == corev1.PodPending {
					for _, cond := range node.Definition().Status.Conditions {
						if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
							unSchedulePods = append(unSchedulePods, node.GetName())
						}
					}
				}
				if node.IsMasterFailed() {
					isHealthy = false
				}
			}
		}

		if cr.Status.ClusterStatus == clusterv1.ClusterOutOfService && allSlots.Count(slot.SlotAssigned) > 0 {
			isHealthy = false
			subSlots := slot.NewFullSlots().Sub(allSlots)
			message = fmt.Sprintf("slots %s missing", subSlots.String())
		}

		if isRollingRestart {
			cr.Status.Status = clusterv1.ClusterStatusRollingUpdate
		} else if isSlotMigrating {
			cr.Status.Status = clusterv1.ClusterStatusRebalancing
		} else if isResourceReady {
			cr.Status.Status = clusterv1.ClusterStatusOK
			if cr.Status.ClusterStatus != clusterv1.ClusterInService {
				cr.Status.Status = clusterv1.ClusterStatusKO
			}
		} else if isHealthy && cr.Status.ClusterStatus == clusterv1.ClusterInService {
			cr.Status.Status = clusterv1.ClusterStatusOK
			message = "OK"
		} else {
			cr.Status.Status = clusterv1.ClusterStatusCreating
		}

		// only update shards when cluster in service
		if len(shards) > 0 && (len(cr.Status.Shards) == 0 || cr.Status.ClusterStatus == clusterv1.ClusterInService) {
			cr.Status.Shards = shards
		}

		if len(unSchedulePods) > 0 {
			message = fmt.Sprintf("pods %s Unschedulable.%s", strings.Join(unSchedulePods, ", "), message)
		}
		cr.Status.Reason = message
	}

	cr.Status.Nodes = cr.Status.Nodes[0:0]
	cr.Status.NumberOfMaster = 0
	cr.Status.NodesPlacement = clusterv1.NodesPlacementInfoOptimal
	nodePlacement := map[string]struct{}{}
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
			cnode := clusterv1.RedisClusterNode{
				ID:          node.ID(),
				Role:        node.Role(),
				MasterRef:   node.MasterID(),
				IP:          node.DefaultIP().String(),
				Port:        fmt.Sprintf("%d", node.Port()),
				PodName:     node.GetName(),
				StatefulSet: shard.GetName(),
				NodeName:    node.NodeIP().String(),
				Slots:       []string{},
			}
			if v := node.Slots().String(); v != "" {
				cnode.Slots = append(cnode.Slots, v)
			}
			cr.Status.Nodes = append(cr.Status.Nodes, cnode)
		}
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		status := cr.Status
		if err := c.client.Client().Get(ctx, client.ObjectKeyFromObject(cr), cr); err != nil {
			logger.Error(err, "get DistributedRedisCluster failed")
			return err
		}
		cr.Status = status
		return c.client.Client().Status().Update(ctx, cr)
	}); errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		logger.Error(err, "get DistributedRedisCluster failed")
		return err
	}
	_ = cr.Init()

	return nil
}

// UpdateStatus
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

func (c *RedisCluster) Masters() []redis.RedisNode {
	var ret []redis.RedisNode
	for _, shard := range c.shards {
		ret = append(ret, shard.Master())
	}
	return ret
}

func (c *RedisCluster) UntrustedNodes() []redis.RedisNode {
	return nil
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
			((status.CurrentRevision == status.UpdateRevision && status.UpdatedReplicas == status.ReadyReplicas) ||
				status.UpdateRevision == "")) {

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
					c.logger.Error(err, "load statefulset failed", "target", fmt.Sprintf("%s/%s", c.GetNamespace(), c.GetName()))
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
			u, _ := user.NewUser(user.DefaultUserName, user.RoleDeveloper, nil)
			users = append(users, u)
		} else {
			if u, err := user.NewUser(username, role, secret); err != nil {
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
				c.logger.Error(err, "load statefulset failed", "target", fmt.Sprintf("%s/%s", c.GetNamespace(), c.GetName()))
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
			InsecureSkipVerify: true,
			RootCAs:            caCertPool,
			Certificates:       []tls.Certificate{cert},
		}, nil
	}
}
