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
	"strings"
	"time"

	"github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	cops "github.com/alauda/redis-operator/pkg/ops/cluster"
	"github.com/alauda/redis-operator/pkg/security/acl"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ actor.Actor = (*actorUpdateAccount)(nil)

func NewUpdateAccountActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorUpdateAccount{
		client: client,
		logger: logger,
	}
}

type actorUpdateAccount struct {
	client kubernetes.ClientSet

	logger logr.Logger
}

// SupportedCommands
func (a *actorUpdateAccount) SupportedCommands() []actor.Command {
	return []actor.Command{cops.CommandUpdateAccount}
}

// Do
//
// 对于账户更新，需要尽量保持在一个 reconcile 里完成，否则会出现一个实例多种密码的情况
// operator 账户为内置账户，不支持修改密码, 如果出现账户不一致的情况，可通过重启来解决
//
// 由于 redis 6.0 支持了 ACL，从 5.0 升级到 6.0，会创建ACL账户。
// 更新逻辑：
// 1. 同步 acl configmap
// 2. 同步实例账户，如果同步实例账户失败，会清理 Pod
// 前置条件
// 1. 不支持 Redis 版本回退
// 2. 更新密码不能更新 secret，需要新建 secret
func (a *actorUpdateAccount) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	cluster := val.(types.RedisClusterInstance)
	logger := a.logger.WithName(cops.CommandUpdateAccount.String()).WithValues("namespace", cluster.GetNamespace(), "name", cluster.GetName())
	logger.Info("start update account", "cluster", cluster.GetName())
	var (
		err error

		users       = cluster.Users()
		defaultUser = users.GetDefaultUser()
		opUser      = users.GetOpUser()
	)

	if defaultUser == nil {
		defaultUser, _ = user.NewUser("", user.RoleDeveloper, nil)
	}

	var (
		currentSecretName string = defaultUser.Password.GetSecretName()
		newSecretName     string
	)
	if ps := cluster.Definition().Spec.PasswordSecret; ps != nil {
		newSecretName = ps.Name
	}

	isAclEnabled := (opUser.Role == user.RoleOperator)

	name := clusterbuilder.GenerateClusterACLConfigMapName(cluster.GetName())
	oldCm, err := a.client.GetConfigMap(ctx, cluster.GetNamespace(), name)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "load configmap failed", "target", name)
		return actor.NewResultWithError(cops.CommandRequeue, err)
	} else if oldCm == nil {
		// sync acl configmap
		oldCm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       cluster.GetNamespace(),
				Labels:          cluster.GetLabels(),
				OwnerReferences: util.BuildOwnerReferences(cluster.Definition()),
			},
			Data: users.Encode(),
		}

		// create acl with old password
		// create redis acl file, after restart, the password is updated
		if err := a.client.CreateConfigMap(ctx, cluster.GetNamespace(), oldCm); err != nil {
			logger.Error(err, "create acl configmap failed", "target", oldCm.Name)
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}

		// wait for resource sync
		time.Sleep(time.Second * 3)
		if oldCm, err = a.client.GetConfigMap(ctx, cluster.GetNamespace(), name); err != nil {
			if !errors.IsNotFound(err) {
				logger.Error(err, "get configmap failed")
			}
			logger.Error(err, "get configmap failed", "target", name)
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}

	var newSecret *corev1.Secret
	if newSecretName != "" {
		if newSecret, err = a.client.GetSecret(ctx, cluster.GetNamespace(), newSecretName); errors.IsNotFound(err) {
			logger.Error(err, "get cluster secret failed", "target", newSecretName)
			return actor.NewResultWithError(cops.CommandRequeue, fmt.Errorf("secret %s not found", newSecretName))

		} else if err != nil {
			logger.Error(err, "get cluster secret failed", "target", newSecretName)
			return actor.NewResultWithError(cops.CommandRequeue, err)
		}
	}

	isUpdated := false
	if newSecretName != currentSecretName {
		defaultUser.Password, _ = user.NewPassword(newSecret)
		isUpdated = true
	}
	users = append(users[0:0], defaultUser)
	if cluster.Version().IsACLSupported() {
		if !isAclEnabled {
			secretName := clusterbuilder.GenerateClusterACLOperatorSecretName(cluster.GetName())
			ownRefs := util.BuildOwnerReferences(cluster.Definition())
			if opUser, err := acl.NewOperatorUser(ctx, a.client,
				secretName, cluster.GetNamespace(), ownRefs, cluster.Version().IsACL2Supported()); err != nil {

				logger.Error(err, "create operator user failed")
				return actor.NewResult(cops.CommandRequeue)
			} else {
				users = append(users, opUser)
				isUpdated = true
			}
			opRedisUser := clusterbuilder.GenerateClusterOperatorsRedisUser(cluster, secretName)
			if err := a.client.CreateIfNotExistsRedisUser(ctx, &opRedisUser); err != nil {
				logger.Error(err, "create operator redis user failed")
				return actor.NewResult(cops.CommandRequeue)
			}
		}

		// append renames to default user
		renameVal := cluster.Definition().Spec.Config[clusterbuilder.RedisConfig_RenameCommand]
		renames, _ := clusterbuilder.ParseRenameConfigs(renameVal)
		rule := user.Rule{
			Categories:  []string{"all"},
			KeyPatterns: []string{"*"},
		}
		for key, val := range renames {
			if key == val {
				continue
			}
			if !func() bool {
				for _, k := range rule.DisallowedCommands {
					if k == key {
						return true
					}
				}
				return false
			}() {
				rule.DisallowedCommands = append(rule.DisallowedCommands, key)
			}
		}
		defaultUser.Rules = append(defaultUser.Rules[0:0], &rule)

		if !cluster.IsACLUserExists() {
			defaultRedisUser := clusterbuilder.GenerateClusterDefaultRedisUser(cluster.Definition(), "")
			if defaultUser.Password != nil {
				defaultRedisUser = clusterbuilder.GenerateClusterDefaultRedisUser(cluster.Definition(), defaultUser.Password.GetSecretName())
			}
			if err := a.client.CreateIfNotExistsRedisUser(ctx, &defaultRedisUser); err != nil {
				logger.Error(err, "create default redis user failed")
				return actor.NewResult(cops.CommandRequeue)
			}
			opRedisUser := clusterbuilder.GenerateClusterOperatorsRedisUser(cluster, "")
			if opUser.Password != nil {
				opRedisUser = clusterbuilder.GenerateClusterOperatorsRedisUser(cluster, opUser.Password.GetSecretName())
			}
			if err := a.client.CreateIfNotExistsRedisUser(ctx, &opRedisUser); err != nil {
				logger.Error(err, "create operator redis user failed")
				return actor.NewResult(cops.CommandRequeue)
			}

		}

	}

	for k, v := range users.Encode() {
		oldCm.Data[k] = v
	}
	if isUpdated {
		// update configmap
		if cluster.Version().IsACLSupported() {
			if err := a.client.CreateIfNotExistsConfigMap(ctx, cluster.GetNamespace(), oldCm); err != nil {
				logger.Error(err, "update acl configmap failed", "target", oldCm.Name)
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}
		} else {
			if err := a.client.CreateOrUpdateConfigMap(ctx, cluster.GetNamespace(), oldCm); err != nil {
				logger.Error(err, "update acl configmap failed", "target", oldCm.Name)
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}
		}
	}

	var (
		isACLAppliedInPods = true
		isAllACLSupported  = true
	)

	// if not all nodes ready, refuse to update the password
	// this is used to ensure succeed upgrade from no acl => acl
	if !cluster.IsReady() {
		return actor.NewResult(cops.CommandHealPod)
	}
	// only update password when all pod is ready
	// here only check the startup pods, not check the termination and pending pods
	for _, shard := range cluster.Shards() {
		for _, node := range shard.Nodes() {
			if !node.CurrentVersion().IsACLSupported() {
				isAllACLSupported = false
				break
			}
			// check if acl have been applied to container
			if !node.IsACLApplied() {
				isACLAppliedInPods = false
			}
		}
		if !isAllACLSupported {
			break
		}
	}

	defaultUser = users.GetDefaultUser()
	opUser = users.GetOpUser()
	logger.V(3).Info("update account ready",
		"isACLAppliedInPods", isACLAppliedInPods,
		"isAllACLSupported", isAllACLSupported,
		"isAclEnabled", isAclEnabled,
		"acl", cluster.Version().IsACLSupported(),
		"exists", cluster.IsACLUserExists(),
	)
	if cluster.Version().IsACLSupported() && isAllACLSupported {
		if newSecretName != currentSecretName {
			if isAclEnabled {
				// hotconfig with redis acl/password
				// if some node update failed, then the pod should be deleted to restarted (TODO)
				for _, node := range cluster.Nodes() {
					if node.ContainerStatus() == nil || !node.ContainerStatus().Ready ||
						node.IsTerminating() {
						continue
					}

					if err := node.Setup(ctx, formatACLSetCommand(defaultUser)); err != nil {
						logger.Error(err, "update acl config failed")
					}
				}

				// update default user password
				ru := clusterbuilder.GenerateClusterDefaultRedisUser(cluster.Definition(), newSecretName)
				oldRu, err := a.client.GetRedisUser(ctx, cluster.GetNamespace(), ru.Name)
				if err == nil && !reflect.DeepEqual(oldRu.Spec.PasswordSecrets, ru.Spec.PasswordSecrets) {
					oldRu.Spec.PasswordSecrets = ru.Spec.PasswordSecrets
					if err := a.client.UpdateRedisUser(ctx, oldRu); err != nil {
						logger.Error(err, "update default user redisUser failed")
					}
				} else if errors.IsNotFound(err) {
					if err := a.client.CreateIfNotExistsRedisUser(ctx, &ru); err != nil {
						logger.Error(err, "create default user redisUser failed")
					}
				}

			} else {
				// this is low probability event

				// added acl account and restart
				// after the instance upgrade from old version to 6.0 supported version,
				margs := [][]interface{}{}
				for _, user := range users {
					margs = append(margs, formatACLSetCommand(user))
				}
				margs = append(
					margs,
					[]interface{}{"config", "set", "masteruser", opUser.Name},
					[]interface{}{"config", "set", "masterauth", opUser.Password.String()},
				)
				for _, node := range cluster.Nodes() {
					if node.ContainerStatus() == nil || !node.ContainerStatus().Ready ||
						node.IsTerminating() {
						continue
					}

					if err := node.Setup(ctx, margs...); err != nil {
						logger.Error(err, "update acl config failed")
					}
				}
				// then requeue to refresh cluster info
				a.logger.Info("=== requeue to refresh cluster info ===(acl)")
				return actor.NewResultWithValue(cops.CommandRequeue, time.Second)
			}
		} else if !isAclEnabled && isACLAppliedInPods {
			// added acl account and restart
			// after the instance upgrade from old version to 6.0 supported version,
			margs := [][]interface{}{}
			for _, user := range users {
				margs = append(margs, formatACLSetCommand(user))
			}
			margs = append(
				margs,
				[]interface{}{"config", "set", "masteruser", opUser.Name},
				[]interface{}{"config", "set", "masterauth", opUser.Password.String()},
			)
			for _, node := range cluster.Nodes() {
				if node.ContainerStatus() == nil || !node.ContainerStatus().Ready ||
					node.IsTerminating() {
					continue
				}

				if err := node.Setup(ctx, margs...); err != nil {
					logger.Error(err, "update acl config failed")
				}
			}

			if err := a.client.CreateOrUpdateConfigMap(ctx, cluster.GetNamespace(), oldCm); err != nil {
				logger.Error(err, "update acl configmap failed", "target", oldCm.Name)
				return actor.NewResultWithError(cops.CommandRequeue, err)
			}

			a.logger.Info("=== requeue to refresh cluster info ===(no acl)")
			// then requeue to refresh cluster info
			return actor.NewResultWithValue(cops.CommandRequeue, time.Second)
		}
	} else {
		if newSecretName != currentSecretName {
			// NOTE: failover first before update password
			logger.Info("failover before update password to force slave to reconnect to master after restart")
			for _, shard := range cluster.Shards() {
				for _, node := range shard.Nodes() {
					if !node.IsReady() || node.IsTerminating() || node.Role() == redis.RedisRoleMaster {
						break
					}

					node.Setup(ctx, []interface{}{"cluster", "failover"})
					time.Sleep(time.Second * 5)
					break
				}
			}

			// update masterauth and requirepass, and restart (ensure_resource do this)
			// hotconfig with redis acl/password
			updateMasterAuth := []interface{}{"config", "set", "masterauth", defaultUser.Password.String()}
			updateRequirePass := []interface{}{"config", "set", "requirepass", defaultUser.Password.String()}
			cmd := []string{"sh", "-c", fmt.Sprintf(`echo -n '%s' > /tmp/newpass`, defaultUser.Password.String())}
			for _, shard := range cluster.Shards() {
				for _, node := range shard.Nodes() {
					if !node.IsReady() || node.IsTerminating() {
						continue
					}

					// Retry hard
					if err := util.RetryOnTimeout(func() error {
						_, _, err := a.client.Exec(ctx, node.GetNamespace(), node.GetName(), clusterbuilder.ServerContainerName, cmd)
						return err
					}, 5); err != nil {
						logger.Error(err, "patch new secret to pod failed", "pod", node.GetName())
					}

					// for old version, use default user for auth
					if err := node.Setup(ctx, updateMasterAuth, updateRequirePass); err != nil {
						logger.Error(err, "update acl config failed")
					}
				}
			}

			// NOTE:
			// NOTE:
			// NOTE: after updated masterauth, the slave will not use this new value to connect to master
			// we must wait about 60s (repl-timeout) for them to take effect
			// logger.Info(fmt.Sprintf("wait %ds repl-timeout for slave to reconnect to master", timeout))

			cluster.UpdateStatus(ctx, v1alpha1.ClusterStatusRollingUpdate, "updating password", nil)
			return nil
		}
	}
	if cluster.Version().IsACLSupported() && !isAllACLSupported {
		return actor.NewResult(cops.CommandEnsureResource)
	}
	return nil
}

// formatACLSetCommand
//
// only acl 1 supported
func formatACLSetCommand(u *user.User) (args []interface{}) {
	if u == nil {
		return nil
	}
	if len(u.Rules) == 0 {
		u.AppendRule(&user.Rule{
			Categories:  []string{"all"},
			KeyPatterns: []string{"*"},
		})
	}
	// keep in mind that the user.Name is "default" for default user
	// when update command,password,keypattern, must reset them all
	args = append(args, "acl", "setuser", u.Name, "reset")
	for _, rule := range u.Rules {
		for _, cate := range rule.Categories {
			cate = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cate), "+"), "@")
			args = append(args, fmt.Sprintf("+@%s", cate))
		}
		for _, cmd := range rule.AllowedCommands {
			cmd = strings.TrimPrefix(cmd, "+")
			args = append(args, fmt.Sprintf("+%s", cmd))
		}

		isDisableAllCmd := false
		for _, cmd := range rule.DisallowedCommands {
			cmd = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cmd), "-"), "@")
			if cmd == "nocommands" || cmd == "-@all" {
				isDisableAllCmd = true
			}
			args = append(args, fmt.Sprintf("-%s", cmd))
		}
		if len(rule.Categories) == 0 && len(rule.AllowedCommands) == 0 && !isDisableAllCmd {
			args = append(args, "+@all")
		}

		if len(rule.KeyPatterns) == 0 {
			rule.KeyPatterns = append(rule.KeyPatterns, "*")
		}
		for _, pattern := range rule.KeyPatterns {
			pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "~")
			// Reference: https://raw.githubusercontent.com/antirez/redis/7.0/redis.conf
			if !strings.HasPrefix(pattern, "%") {
				pattern = fmt.Sprintf("~%s", pattern)
			}
			args = append(args, pattern)
		}
		for _, pattern := range rule.Channels {
			pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "&")
			// Reference: https://raw.githubusercontent.com/antirez/redis/7.0/redis.conf
			if !strings.HasPrefix(pattern, "&") {
				pattern = fmt.Sprintf("&%s", pattern)
			}
			args = append(args, pattern)
		}
		passwd := u.Password.String()
		if passwd == "" {
			args = append(args, "nopass")
		} else {
			args = append(args, fmt.Sprintf(">%s", passwd))
		}

		// NOTE: on must after reset
		args = append(args, "on")
	}
	return
}
