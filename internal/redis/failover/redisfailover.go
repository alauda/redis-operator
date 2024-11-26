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

package failover

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/api/core/helper"
	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
	redisv1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/builder/clusterbuilder"
	"github.com/alauda/redis-operator/internal/builder/failoverbuilder"
	"github.com/alauda/redis-operator/internal/redis/failover/monitor"
	"github.com/alauda/redis-operator/internal/util"
	clientset "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/security/acl"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/user"

	"github.com/go-logr/logr"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.RedisFailoverInstance = (*RedisFailover)(nil)

type RedisFailover struct {
	databasesv1.RedisFailover

	client        clientset.ClientSet
	eventRecorder record.EventRecorder

	users       acl.Users
	tlsConfig   *tls.Config
	configmap   map[string]string
	replication types.RedisReplication
	monitor     types.FailoverMonitor

	redisUsers []*redisv1.RedisUser

	logger logr.Logger
}

func NewRedisFailover(ctx context.Context, k8sClient clientset.ClientSet, eventRecorder record.EventRecorder, def *databasesv1.RedisFailover, logger logr.Logger) (*RedisFailover, error) {
	inst := &RedisFailover{
		RedisFailover: *def,
		client:        k8sClient,
		eventRecorder: eventRecorder,
		configmap:     make(map[string]string),
		logger:        logger.WithName("F").WithValues("instance", client.ObjectKeyFromObject(def).String()),
	}
	var err error
	if inst.users, err = inst.loadUsers(ctx); err != nil {
		inst.logger.Error(err, "load user failed")
		return nil, err
	}
	if inst.tlsConfig, err = inst.loadTLS(ctx); err != nil {
		inst.logger.Error(err, "loads tls failed")
		return nil, err
	}

	if inst.replication, err = LoadRedisReplication(ctx, k8sClient, inst, inst.logger); err != nil {
		inst.logger.Error(err, "load replicas failed")
		return nil, err
	}
	if inst.monitor, err = monitor.LoadFailoverMonitor(ctx, k8sClient, inst, inst.logger); err != nil {
		inst.logger.Error(err, "load monitor failed")
		return nil, err
	}

	if inst.Version().IsACLSupported() {
		inst.LoadRedisUsers(ctx)
	}
	return inst, nil
}

func (s *RedisFailover) NamespacedName() client.ObjectKey {
	if s == nil {
		return client.ObjectKey{}
	}
	return client.ObjectKey{Namespace: s.GetNamespace(), Name: s.GetName()}
}

func (s *RedisFailover) Arch() redis.RedisArch {
	return core.RedisSentinel
}

func (s *RedisFailover) UpdateStatus(ctx context.Context, st types.InstanceStatus, msg string) error {
	if s == nil {
		return nil
	}

	var (
		err       error
		replicas  = int32(len(s.Nodes()))
		nodeports = map[int32]struct{}{}
		rs        = lo.IfF(s != nil && s.replication != nil, func() *appsv1.StatefulSetStatus {
			return s.replication.Status()
		}).Else(nil)
		sentinel *databasesv1.RedisSentinel
		cr       = s.Definition()
		status   = cr.Status.DeepCopy()
	)

	switch st {
	case types.OK:
		status.Phase = databasesv1.Ready
	case types.Fail:
		status.Phase = databasesv1.Fail
	case types.Paused:
		status.Phase = databasesv1.Paused
	default:
		status.Phase = ""
	}
	status.Message = msg

	if s.IsBindedSentinel() {
		if sentinel, err = s.client.GetRedisSentinel(ctx, s.GetNamespace(), s.GetName()); err != nil && !errors.IsNotFound(err) {
			s.logger.Error(err, "get RedisSentinel failed")
			return err
		}
	}

	// collect instance statistics
	status.Nodes = status.Nodes[:0]
	detailedNodes := []core.RedisDetailedNode{}
	for _, node := range s.Nodes() {
		detailedNode := core.RedisDetailedNode{
			RedisNode: core.RedisNode{
				Role:        node.Role(),
				MasterRef:   node.MasterID(),
				IP:          node.DefaultIP().String(),
				Port:        fmt.Sprintf("%d", node.Port()),
				PodName:     node.GetName(),
				StatefulSet: s.replication.GetName(),
				NodeName:    node.NodeIP().String(),
			},
			Version:           node.Info().RedisVersion,
			UsedMemory:        node.Info().UsedMemory,
			UsedMemoryDataset: node.Info().UsedMemoryDataset,
		}
		status.Nodes = append(status.Nodes, detailedNode.RedisNode)
		detailedNodes = append(detailedNodes, detailedNode)
		if node.Role() == core.RedisRoleMaster {
			status.Master.Name = node.GetName()
			status.Master.Address = net.JoinHostPort(node.DefaultIP().String(), fmt.Sprintf("%d", node.Port()))
			status.Master.Status = databasesv1.RedisStatusMasterOK
		}
		if port := node.Definition().Labels[builder.PodAnnouncePortLabelKey]; port != "" {
			val, _ := strconv.ParseInt(port, 10, 32)
			nodeports[int32(val)] = struct{}{}
		}
	}

	status.Instance.Redis.Size = replicas
	status.Instance.Redis.Ready = 0
	for _, node := range s.Nodes() {
		if node.IsReady() {
			status.Instance.Redis.Ready++
		}
	}

	if s.Monitor() != nil {
		masterNode, err := s.Monitor().Master(ctx)
		if err != nil {
			if err != monitor.ErrNoMaster {
				s.logger.Error(err, "get master failed")
			}
			status.Master.Address = ""
			status.Master.Status = databasesv1.RedisStatusMasterDown
		} else if masterNode != nil {
			status.Master.Address = masterNode.Address()
			status.Master.Address = masterNode.Address()
			if masterNode.Flags == "master" {
				status.Master.Status = databasesv1.RedisStatusMasterOK
			} else {
				status.Master.Status = databasesv1.RedisStatusMasterDown
			}
		}
		status.Master.Name = s.RedisFailover.Status.Monitor.Name
	}

	phase, msg := func() (databasesv1.Phase, string) {
		// use passed status if provided
		if status.Phase == databasesv1.Fail || status.Phase == databasesv1.Paused {
			return status.Phase, status.Message
		}

		if sentinel != nil {
			switch sentinel.Status.Phase {
			case databasesv1.SentinelCreating:
				return databasesv1.Creating, sentinel.Status.Message
			case databasesv1.SentinelFail:
				return databasesv1.Fail, sentinel.Status.Message
			}
		}

		// check creating
		if rs == nil || rs.CurrentReplicas != s.Definition().Spec.Redis.Replicas ||
			rs.Replicas != s.Definition().Spec.Redis.Replicas {
			return databasesv1.Creating, ""
		}

		var pendingPods []string
		// check pending
		for _, node := range s.Nodes() {
			for _, cond := range node.Definition().Status.Conditions {
				if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
					pendingPods = append(pendingPods, node.GetName())
				}
			}
		}
		if len(pendingPods) > 0 {
			return databasesv1.Pending, fmt.Sprintf("pods %s pending", strings.Join(pendingPods, ","))
		}

		// check nodeport applied
		if seq := s.Spec.Redis.Expose.NodePortSequence; s.Spec.Redis.Expose.ServiceType == corev1.ServiceTypeNodePort {
			var (
				notAppliedPorts = []string{}
				customPorts, _  = helper.ParseSequencePorts(seq)
			)
			for _, port := range customPorts {
				if _, ok := nodeports[port]; !ok {
					notAppliedPorts = append(notAppliedPorts, strconv.Itoa(int(port)))
				}
			}
			if len(notAppliedPorts) > 0 {
				return databasesv1.WaitingPodReady, fmt.Sprintf("nodeport %s not applied", strings.Join(notAppliedPorts, ","))
			}
		}

		var notReadyPods []string
		for _, node := range s.Nodes() {
			if !node.IsReady() {
				notReadyPods = append(notReadyPods, node.GetName())
			}
		}
		if len(notReadyPods) > 0 {
			return databasesv1.WaitingPodReady, fmt.Sprintf("pods %s not ready", strings.Join(notReadyPods, ","))
		}

		if isAllMonitored, _ := s.Monitor().AllNodeMonitored(ctx); !isAllMonitored {
			return databasesv1.Creating, "not all nodes monitored"
		}

		// make sure all is ready
		if (rs != nil &&
			rs.ReadyReplicas == s.Definition().Spec.Redis.Replicas &&
			rs.CurrentReplicas == rs.ReadyReplicas &&
			rs.CurrentRevision == rs.UpdateRevision) &&
			// sentinel
			(sentinel == nil || sentinel.Status.Phase == databasesv1.SentinelReady) &&
			// instance status
			(status.Master.Status == databasesv1.RedisStatusMasterOK) {

			return databasesv1.Ready, ""
		}
		return databasesv1.WaitingPodReady, ""
	}()
	status.Phase, status.Message = phase, lo.If(msg == "", status.Message).Else(msg)

	// update detailed configmap
	detailStatus := databasesv1.RedisFailoverDetailedStatus{
		Phase:   status.Phase,
		Message: status.Message,
		Nodes:   detailedNodes,
	}
	detailStatusCM, _ := failoverbuilder.NewRedisFailoverDetailedStatusConfigMap(s, &detailStatus)
	if oldCm, err := s.client.GetConfigMap(ctx, s.GetNamespace(), detailStatusCM.GetName()); errors.IsNotFound(err) {
		if err := s.client.CreateConfigMap(ctx, s.GetNamespace(), detailStatusCM); err != nil {
			s.logger.Error(err, "create detailed status configmap failed")
		}
	} else if err != nil {
		s.logger.Error(err, "get detailed status configmap failed")
	} else if failoverbuilder.ShouldUpdateDetailedStatusConfigMap(oldCm, &detailStatus) {
		if err := s.client.UpdateConfigMap(ctx, s.GetNamespace(), detailStatusCM); err != nil {
			s.logger.Error(err, "update detailed status configmap failed")
		}
	}
	if status.DetailedStatusRef == nil {
		status.DetailedStatusRef = &corev1.ObjectReference{
			Kind: "ConfigMap",
			Name: detailStatusCM.GetName(),
		}
	}
	// update status
	s.RedisFailover.Status = *status
	if err := s.client.UpdateRedisFailoverStatus(ctx, &s.RedisFailover); err != nil {
		s.logger.Error(err, "update RedisFailover status failed")
		return err
	}
	return nil
}

func (s *RedisFailover) Definition() *databasesv1.RedisFailover {
	if s == nil {
		return nil
	}
	return &s.RedisFailover
}

func (s *RedisFailover) Version() redis.RedisVersion {
	if s == nil {
		return redis.RedisVersionUnknown
	}

	if version, err := redis.ParseRedisVersionFromImage(s.Spec.Redis.Image); err != nil {
		s.logger.Error(err, "parse redis version failed")
		return redis.RedisVersionUnknown
	} else {
		return version
	}
}

func (s *RedisFailover) Masters() []redis.RedisNode {
	if s == nil || s.replication == nil {
		return nil
	}
	var ret []redis.RedisNode
	for _, v := range s.replication.Nodes() {
		if v.Role() == core.RedisRoleMaster {
			ret = append(ret, v)
		}
	}
	return ret
}

func (s *RedisFailover) Nodes() []redis.RedisNode {
	if s == nil || s.replication == nil {
		return nil
	}
	return append([]redis.RedisNode{}, s.replication.Nodes()...)
}

func (s *RedisFailover) RawNodes(ctx context.Context) ([]corev1.Pod, error) {
	if s == nil {
		return nil, nil
	}
	name := failoverbuilder.GetFailoverStatefulSetName(s.GetName())
	sts, err := s.client.GetStatefulSet(ctx, s.GetNamespace(), name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		s.logger.Error(err, "load statefulset failed", "name", name)
		return nil, err
	}
	// load pods by statefulset selector
	ret, err := s.client.GetStatefulSetPodsByLabels(ctx, sts.GetNamespace(), sts.Spec.Selector.MatchLabels)
	if err != nil {
		s.logger.Error(err, "loads pods of shard failed")
		return nil, err
	}
	return ret.Items, nil
}

func (s *RedisFailover) IsInService() bool {
	if s == nil || s.Monitor() == nil {
		return false
	}

	master, err := s.Monitor().Master(context.TODO())
	if err == monitor.ErrNoMaster || err == monitor.ErrMultipleMaster {
		return false
	} else if err != nil {
		s.logger.Error(err, "get master failed")
		return false
	} else if master != nil && master.Flags == "master" {
		return true
	}
	return false
}

func (s *RedisFailover) IsReady() bool {
	if s == nil {
		return false
	}
	if s.RedisFailover.Status.Phase == databasesv1.Ready {
		return true
	}
	return false
}

func (s *RedisFailover) Users() (us acl.Users) {
	if s == nil {
		return nil
	}

	// clone before return
	for _, user := range s.users {
		u := *user
		if u.Password != nil {
			p := *u.Password
			u.Password = &p
		}
		us = append(us, &u)
	}
	return
}

func (s *RedisFailover) TLSConfig() *tls.Config {
	if s == nil {
		return nil
	}
	return s.tlsConfig
}

func (s *RedisFailover) Restart(ctx context.Context, annotationKeyVal ...string) error {
	if s == nil || s.replication == nil {
		return nil
	}
	return s.replication.Restart(ctx, annotationKeyVal...)
}

func (s *RedisFailover) Refresh(ctx context.Context) error {
	if s == nil {
		return nil
	}
	logger := s.logger.WithName("Refresh")
	logger.V(3).Info("refreshing sentinel", "target", util.ObjectKey(s.GetNamespace(), s.GetName()))

	// load cr
	var cr databasesv1.RedisFailover
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
		return s.client.Client().Get(ctx, client.ObjectKeyFromObject(&s.RedisFailover), &cr)
	}); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		logger.Error(err, "get RedisFailover failed")
		return err
	}
	if cr.Name == "" {
		return fmt.Errorf("RedisFailover is nil")
	}
	s.RedisFailover = cr
	err := s.RedisFailover.Validate()
	if err != nil {
		return err
	}

	if s.users, err = s.loadUsers(ctx); err != nil {
		logger.Error(err, "load users failed")
		return err
	}

	if s.replication, err = LoadRedisReplication(ctx, s.client, s, logger); err != nil {
		logger.Error(err, "load replicas failed")
		return err
	}
	return nil
}

func (s *RedisFailover) LoadRedisUsers(ctx context.Context) {
	oldOpUser, _ := s.client.GetRedisUser(ctx, s.GetNamespace(), failoverbuilder.GenerateFailoverOperatorsRedisUserName(s.GetName()))
	oldDefultUser, _ := s.client.GetRedisUser(ctx, s.GetNamespace(), failoverbuilder.GenerateFailoverDefaultRedisUserName(s.GetName()))
	s.redisUsers = []*redisv1.RedisUser{oldOpUser, oldDefultUser}
}

func (s *RedisFailover) loadUsers(ctx context.Context) (acl.Users, error) {
	var (
		name  = failoverbuilder.GenerateFailoverACLConfigMapName(s.GetName())
		users acl.Users
	)

	if s.Version().IsACLSupported() {
		getPassword := func(secretName string) (*user.Password, error) {
			if secret, err := s.loadUserSecret(ctx, client.ObjectKey{
				Namespace: s.GetNamespace(),
				Name:      secretName,
			}); err != nil {
				return nil, err
			} else {
				if password, err := user.NewPassword(secret); err != nil {
					return nil, err
				} else {
					return password, nil
				}
			}
		}
		for _, name := range []string{
			failoverbuilder.GenerateFailoverOperatorsRedisUserName(s.GetName()),
			failoverbuilder.GenerateFailoverDefaultRedisUserName(s.GetName()),
		} {
			if ru, err := s.client.GetRedisUser(ctx, s.GetNamespace(), name); err != nil {
				s.logger.Error(err, "load operator user failed")
				users = nil
				break
			} else {
				var password *user.Password
				if len(ru.Spec.PasswordSecrets) > 0 {
					if password, err = getPassword(ru.Spec.PasswordSecrets[0]); err != nil {
						s.logger.Error(err, "load operator user password failed")
						return nil, err
					}
				}
				if u, err := user.NewUserFromRedisUser(ru.Spec.Username, ru.Spec.AclRules, password); err != nil {
					s.logger.Error(err, "load operator user failed")
					return nil, err
				} else {
					users = append(users, u)
				}
			}
		}
	}
	if len(users) == 0 {
		if cm, err := s.client.GetConfigMap(ctx, s.GetNamespace(), name); errors.IsNotFound(err) {
			var (
				username       string
				passwordSecret string
				secret         *corev1.Secret
			)
			statefulSetName := failoverbuilder.GetFailoverStatefulSetName(s.GetName())
			sts, err := s.client.GetStatefulSet(ctx, s.GetNamespace(), statefulSetName)
			if err != nil {
				if !errors.IsNotFound(err) {
					s.logger.Error(err, "load statefulset failed", "target", util.ObjectKey(s.GetNamespace(), s.GetName()))
				}
				if s.Version().IsACLSupported() {
					passwordSecret = failoverbuilder.GenerateFailoverACLOperatorSecretName(s.GetName())
					username = user.DefaultOperatorUserName
				}
			} else {
				spec := sts.Spec.Template.Spec
				if container := util.GetContainerByName(&spec, failoverbuilder.ServerContainerName); container != nil {
					for _, env := range container.Env {
						if env.Name == failoverbuilder.PasswordENV && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
							passwordSecret = env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
						} else if env.Name == failoverbuilder.OperatorSecretName && env.Value != "" {
							passwordSecret = env.Value
						} else if env.Name == failoverbuilder.OperatorUsername {
							username = env.Value
						}
					}
				}
				if passwordSecret == "" {
					// COMPAT: for old sentinel version, the secret is mounted to the pod
					for _, vol := range spec.Volumes {
						if vol.Name == "redis-auth" && vol.Secret != nil {
							passwordSecret = vol.Secret.SecretName
							break
						}
					}
				}
			}

			if passwordSecret != "" {
				objKey := client.ObjectKey{Namespace: s.GetNamespace(), Name: passwordSecret}
				if secret, err = s.loadUserSecret(ctx, objKey); err != nil {
					s.logger.Error(err, "load user secret failed", "target", objKey)
					return nil, err
				}
			} else if passwordSecret := s.Spec.Auth.SecretPath; passwordSecret != "" {
				secret, err = s.client.GetSecret(ctx, s.GetNamespace(), passwordSecret)
				if err != nil {
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
				if u, err := user.NewOperatorUser(secret, s.Version().IsACL2Supported()); err != nil {
					s.logger.Error(err, "init users failed")
					return nil, err
				} else {
					users = append(users, u)
				}

				if passwordSecret := s.Spec.Auth.SecretPath; passwordSecret != "" {
					secret, err = s.client.GetSecret(ctx, s.GetNamespace(), passwordSecret)
					if err != nil {
						return nil, err
					}
					u, _ := user.NewUser(user.DefaultUserName, user.RoleDeveloper, secret, s.Version().IsACL2Supported())
					users = append(users, u)
				} else {
					u, _ := user.NewUser(user.DefaultUserName, user.RoleDeveloper, nil, s.Version().IsACL2Supported())
					users = append(users, u)
				}
			} else {
				if u, err := user.NewUser(username, role, secret, s.Version().IsACL2Supported()); err != nil {
					s.logger.Error(err, "init users failed")
					return nil, err
				} else {
					users = append(users, u)
				}
			}
		} else if err != nil {
			s.logger.Error(err, "load default users's password secret failed", "target", util.ObjectKey(s.GetNamespace(), name))
			return nil, err
		} else if users, err = acl.LoadACLUsers(ctx, s.client, cm); err != nil {
			s.logger.Error(err, "load acl failed")
			return nil, err
		}
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
	if s.Version().IsACL2Supported() {
		rule.Channels = []string{"*"}
	}

	renameVal := s.Definition().Spec.Redis.CustomConfig[failoverbuilder.RedisConfig_RenameCommand]
	renames, _ := clusterbuilder.ParseRenameConfigs(renameVal)
	if len(renameVal) > 0 {
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

func (s *RedisFailover) loadUserSecret(ctx context.Context, objKey client.ObjectKey) (*corev1.Secret, error) {
	secret, err := s.client.GetSecret(ctx, objKey.Namespace, objKey.Name)
	if err != nil && !errors.IsNotFound(err) {
		s.logger.Error(err, "load default users's password secret failed", "target", objKey.String())
		return nil, err
	} else if errors.IsNotFound(err) {
		secret = failoverbuilder.NewFailoverOpSecret(s.Definition())
		err := s.client.CreateSecret(ctx, objKey.Namespace, secret)
		if err != nil {
			return nil, err
		}
	} else if _, ok := secret.Data[user.PasswordSecretKey]; !ok {
		return nil, fmt.Errorf("no password found")
	}
	return secret, nil
}

func (s *RedisFailover) loadTLS(ctx context.Context) (*tls.Config, error) {
	if s == nil {
		return nil, nil
	}
	logger := s.logger.WithName("loadTLS")

	secretName := s.Status.TLSSecret
	if secretName == "" {
		// load current tls secret.
		// because previous cr not recorded the secret name, we should load it from statefulset
		stsName := failoverbuilder.GetFailoverStatefulSetName(s.GetName())
		if sts, err := s.client.GetStatefulSet(ctx, s.GetNamespace(), stsName); err != nil {
			s.logger.Error(err, "load statefulset failed", "target", util.ObjectKey(s.GetNamespace(), s.GetName()))
		} else {
			for _, vol := range sts.Spec.Template.Spec.Volumes {
				if vol.Name == failoverbuilder.RedisTLSVolumeName {
					secretName = vol.VolumeSource.Secret.SecretName
				}
			}
		}
	}
	if secretName == "" {
		return nil, nil
	}
	if secret, err := s.client.GetSecret(ctx, s.GetNamespace(), secretName); err != nil {
		logger.Error(err, "secret not found", "name", secretName)
		return nil, err
	} else if cert, err := util.LoadCertConfigFromSecret(secret); err != nil {
		logger.Error(err, "load cert config failed")
		return nil, err
	} else {
		return cert, nil
	}
}

func (s *RedisFailover) IsBindedSentinel() bool {
	if s == nil || s.RedisFailover.Spec.Sentinel == nil {
		return false
	}
	return s.RedisFailover.Spec.Sentinel.SentinelReference == nil
}

func (s *RedisFailover) Selector() map[string]string {
	if s == nil {
		return nil
	}
	if s.replication != nil && s.replication.Definition() != nil {
		return s.replication.Definition().Spec.Selector.MatchLabels
	}
	return nil
}

func (s *RedisFailover) IsACLUserExists() bool {
	if !s.Version().IsACLSupported() {
		return false
	}
	if len(s.redisUsers) == 0 {
		return false
	}
	for _, v := range s.redisUsers {
		if v == nil {
			return false
		}
	}
	return true
}

func (s *RedisFailover) IsResourceFullfilled(ctx context.Context) (bool, error) {
	var (
		serviceKey  = corev1.SchemeGroupVersion.WithKind("Service")
		stsKey      = appsv1.SchemeGroupVersion.WithKind("StatefulSet")
		sentinelKey = databasesv1.GroupVersion.WithKind("RedisSentinel")
	)
	resources := map[schema.GroupVersionKind][]string{
		serviceKey: {
			failoverbuilder.GetFailoverStatefulSetName(s.GetName()), // rfr-<name>
			failoverbuilder.GetRedisROServiceName(s.GetName()),      // rfr-<name>-read-only
			failoverbuilder.GetRedisRWServiceName(s.GetName()),      // rfr-<name>-read-write
		},
		stsKey: {
			failoverbuilder.GetFailoverStatefulSetName(s.GetName()),
		},
	}
	if s.IsBindedSentinel() {
		resources[sentinelKey] = []string{s.GetName()}
	}

	if s.Spec.Redis.Expose.ServiceType == corev1.ServiceTypeLoadBalancer ||
		s.Spec.Redis.Expose.ServiceType == corev1.ServiceTypeNodePort {
		for i := 0; i < int(s.Spec.Redis.Replicas); i++ {
			stsName := failoverbuilder.GetFailoverStatefulSetName(s.GetName())
			resources[serviceKey] = append(resources[serviceKey], fmt.Sprintf("%s-%d", stsName, i))
		}
	}

	for gvk, names := range resources {
		for _, name := range names {
			var obj unstructured.Unstructured
			obj.SetGroupVersionKind(gvk)

			err := s.client.Client().Get(ctx, client.ObjectKey{Namespace: s.GetNamespace(), Name: name}, &obj)
			if errors.IsNotFound(err) {
				s.logger.V(3).Info("resource not found", "kind", gvk.Kind, "target", util.ObjectKey(s.GetNamespace(), name))
				return false, nil
			} else if err != nil {
				s.logger.Error(err, "get resource failed", "kind", gvk.Kind, "target", util.ObjectKey(s.GetNamespace(), name))
				return false, err
			}
		}
	}

	if !s.IsStandalone() {
		// check sentinel
		newSen := failoverbuilder.NewFailoverSentinel(s)
		oldSen, err := s.client.GetRedisSentinel(ctx, s.GetNamespace(), s.GetName())
		if errors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			s.logger.Error(err, "get sentinel failed", "target", client.ObjectKeyFromObject(newSen))
			return false, err
		}
		if !reflect.DeepEqual(newSen.Spec, oldSen.Spec) ||
			!reflect.DeepEqual(newSen.Labels, oldSen.Labels) ||
			!reflect.DeepEqual(newSen.Annotations, oldSen.Annotations) {
			oldSen.Spec = newSen.Spec
			oldSen.Labels = newSen.Labels
			oldSen.Annotations = newSen.Annotations
			return false, nil
		}
	}
	return true, nil
}

func (s *RedisFailover) IsACLAppliedToAll() bool {
	if s == nil || !s.Version().IsACLSupported() {
		return false
	}
	for _, node := range s.Nodes() {
		if !node.CurrentVersion().IsACLSupported() || !node.IsACLApplied() {
			return false
		}
	}
	return true
}

func (c *RedisFailover) Logger() logr.Logger {
	if c == nil {
		return logr.Discard()
	}
	return c.logger
}

func (c *RedisFailover) SendEventf(eventtype, reason, messageFmt string, args ...interface{}) {
	if c == nil {
		return
	}
	c.eventRecorder.Eventf(c.Definition(), eventtype, reason, messageFmt, args...)
}

func (s *RedisFailover) Monitor() types.FailoverMonitor {
	if s == nil {
		return nil
	}
	return s.monitor
}

func (c *RedisFailover) IsStandalone() bool {
	if c == nil {
		return false
	}
	return c.RedisFailover.Spec.Sentinel == nil
}
