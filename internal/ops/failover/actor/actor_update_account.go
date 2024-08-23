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
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/alauda/redis-operator/api/core"
	midv1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	"github.com/alauda/redis-operator/internal/builder/failoverbuilder"
	"github.com/alauda/redis-operator/internal/config"
	ops "github.com/alauda/redis-operator/internal/ops/failover"
	"github.com/alauda/redis-operator/internal/ops/sentinel"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/security/acl"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ actor.Actor = (*actorUpdateAccount)(nil)

func init() {
	actor.Register(core.RedisSentinel, NewUpdateAccountActor)
}

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

func (a *actorUpdateAccount) Version() *semver.Version {
	return semver.MustParse("3.18.0")
}

// SupportedCommands
func (a *actorUpdateAccount) SupportedCommands() []actor.Command {
	return []actor.Command{ops.CommandUpdateAccount}
}

func (a *actorUpdateAccount) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	logger := val.Logger().WithValues("actor", ops.CommandUpdateAccount.String())

	var (
		inst        = val.(types.RedisFailoverInstance)
		users       = inst.Users()
		defaultUser = users.GetDefaultUser()
		opUser      = users.GetOpUser()
	)

	if defaultUser == nil {
		defaultUser, _ = user.NewUser("", user.RoleDeveloper, nil, inst.Version().IsACL2Supported())
	}

	var (
		currentSecretName string = defaultUser.Password.GetSecretName()
		newSecretName     string
	)
	if ps := inst.Definition().Spec.Auth.SecretPath; ps != "" {
		newSecretName = ps
	}

	isAclEnabled := (opUser.Role == user.RoleOperator)

	name := failoverbuilder.GenerateFailoverACLConfigMapName(inst.GetName())
	oldCm, err := a.client.GetConfigMap(ctx, inst.GetNamespace(), name)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "load configmap failed", "target", name)
		return actor.NewResultWithError(ops.CommandRequeue, err)
	} else if oldCm == nil {
		// sync acl configmap
		oldCm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       inst.GetNamespace(),
				Labels:          inst.GetLabels(),
				OwnerReferences: util.BuildOwnerReferences(inst.Definition()),
			},
			Data: users.Encode(true),
		}

		// create acl with old password
		// create redis acl file, after restart, the password is updated
		if err := a.client.CreateConfigMap(ctx, inst.GetNamespace(), oldCm); err != nil {
			logger.Error(err, "create acl configmap failed", "target", oldCm.Name)
			return actor.NewResultWithError(ops.CommandRequeue, err)
		}

		// wait for resource sync
		time.Sleep(time.Second * 1)
		if oldCm, err = a.client.GetConfigMap(ctx, inst.GetNamespace(), name); err != nil {
			logger.Error(err, "get configmap failed", "target", name)
			return actor.NewResultWithError(ops.CommandRequeue, err)
		}
	}

	var newSecret *corev1.Secret
	if newSecretName != "" {
		if newSecret, err = a.client.GetSecret(ctx, inst.GetNamespace(), newSecretName); errors.IsNotFound(err) {
			logger.Error(err, "get sentinel secret failed", "target", newSecretName)
			return actor.NewResultWithError(ops.CommandRequeue, fmt.Errorf("secret %s not found", newSecretName))
		} else if err != nil {
			logger.Error(err, "get sentinel secret failed", "target", newSecretName)
			return actor.NewResultWithError(ops.CommandRequeue, err)
		}
	}

	isUpdated := false
	if newSecretName != currentSecretName {
		defaultUser.Password, _ = user.NewPassword(newSecret)
		isUpdated = true
	}
	users = append(users[0:0], defaultUser)
	if inst.Version().IsACLSupported() {
		if !isAclEnabled {
			secretName := failoverbuilder.GenerateFailoverACLOperatorSecretName(inst.GetName())
			ownRefs := util.BuildOwnerReferences(inst.Definition())
			opUser, err := acl.NewOperatorUser(ctx, a.client, secretName, inst.GetNamespace(), ownRefs, inst.Version().IsACL2Supported())
			if err != nil {
				logger.Error(err, "create operator user failed")
				return actor.NewResult(ops.CommandRequeue)
			} else {
				users = append(users, opUser)
				isUpdated = true
			}

			opRedisUser := failoverbuilder.GenerateFailoverRedisUser(inst, opUser)
			if err := a.client.CreateOrUpdateRedisUser(ctx, opRedisUser); err != nil {
				logger.Error(err, "create operator redis user failed")
				return actor.NewResult(ops.CommandRequeue)
			}
			inst.SendEventf(corev1.EventTypeNormal, config.EventCreateUser, "created operator user to enable acl")
		} else {
			if newOpUser, err := acl.NewOperatorUser(ctx, a.client,
				opUser.Password.SecretName, inst.GetNamespace(), nil, inst.Version().IsACL2Supported()); err != nil {
				logger.Error(err, "create operator user failed")
				return actor.NewResult(ops.CommandRequeue)
			} else {
				opRedisUser := failoverbuilder.GenerateFailoverRedisUser(inst, newOpUser)
				if err := a.client.CreateOrUpdateRedisUser(ctx, opRedisUser); err != nil {
					logger.Error(err, "update operator redis user failed")
					return actor.NewResult(ops.CommandRequeue)
				}
				inst.SendEventf(corev1.EventTypeNormal, config.EventCreateUser, "created/updated operator user")

				opUser.Rules = newOpUser.Rules
				users = append(users, opUser)

				isUpdated = true
			}
		}

		defaultRedisUser := failoverbuilder.GenerateFailoverRedisUser(inst, defaultUser)
		defaultRedisUser.Annotations[midv1.ACLSupportedVersionAnnotationKey] = inst.Version().String()
		if oldDefaultRU, err := a.client.GetRedisUser(ctx, inst.GetNamespace(), defaultRedisUser.GetName()); errors.IsNotFound(err) {
			if err := a.client.CreateIfNotExistsRedisUser(ctx, defaultRedisUser); err != nil {
				logger.Error(err, "update default redis user failed")
				return actor.NewResult(ops.CommandRequeue)
			}
			inst.SendEventf(corev1.EventTypeNormal, config.EventCreateUser, "created default user")
		} else if err != nil {
			logger.Error(err, "get default redisuser failed")
			return actor.NewResultWithError(ops.CommandRequeue, err)
		} else if inst.Version().IsACL2Supported() {
			oldVersion := redis.RedisVersion(oldDefaultRU.Annotations[midv1.ACLSupportedVersionAnnotationKey])
			// COMP: if old version not support acl2, and new version is supported, update acl rules for compatibility
			if !oldVersion.IsACL2Supported() {
				fields := strings.Fields(oldDefaultRU.Spec.AclRules)
				if !slices.Contains(fields, "&*") && !slices.Contains(fields, "allchannels") {
					oldDefaultRU.Spec.AclRules = fmt.Sprintf("%s &*", oldDefaultRU.Spec.AclRules)
				}
				if oldDefaultRU.Annotations == nil {
					oldDefaultRU.Annotations = make(map[string]string)
				}
				oldDefaultRU.Annotations[midv1.ACLSupportedVersionAnnotationKey] = inst.Version().String()
				if err := a.client.UpdateRedisUser(ctx, oldDefaultRU); err != nil {
					logger.Error(err, "update default redis user failed")
					return actor.NewResult(ops.CommandRequeue)
				}
				inst.SendEventf(corev1.EventTypeNormal, config.EventUpdateUser, "migrate default user acl rules to support channels")
			}
		}
	}

	if !reflect.DeepEqual(users.Encode(true), oldCm.Data) {
		isUpdated = true
	}
	for k, v := range users.Encode(true) {
		oldCm.Data[k] = v
	}
	if isUpdated {
		if err := a.client.CreateOrUpdateConfigMap(ctx, inst.GetNamespace(), oldCm); err != nil {
			logger.Error(err, "update acl configmap failed", "target", oldCm.Name)
			return actor.NewResultWithError(ops.CommandRequeue, err)
		}
		if err := inst.Refresh(ctx); err != nil {
			logger.Error(err, "refresh resource failed")
			return actor.NewResultWithError(ops.CommandRequeue, err)
		}
	}

	var (
		isACLAppliedInPods = true
		isAllACLSupported  = true
		isAllPodReady      = true
	)
	for _, node := range inst.Nodes() {
		if !node.CurrentVersion().IsACLSupported() {
			isAllACLSupported = false
			break
		}
		// check if acl have been applied to container
		if !node.IsACLApplied() {
			isACLAppliedInPods = false
		}
		if node.ContainerStatus() == nil || !node.ContainerStatus().Ready ||
			node.IsTerminating() {
			isAllPodReady = false
		}
	}

	logger.V(3).Info("update account",
		"isAllACLSupported", isAllACLSupported,
		"isACLAppliedInPods", isACLAppliedInPods,
		"version", inst.Users().Encode(true),
	)

	if inst.Version().IsACLSupported() {
		if isAllACLSupported {
			if !isACLAppliedInPods && isAllPodReady {
				margs := [][]interface{}{}
				margs = append(
					margs,
					[]interface{}{"config", "set", "masteruser", inst.Users().GetOpUser().Name},
					[]interface{}{"config", "set", "masterauth", inst.Users().GetOpUser().Password},
				)
				for _, node := range inst.Nodes() {
					if node.ContainerStatus() == nil || !node.IsReady() || node.IsTerminating() {
						continue
					}
					if err := node.Setup(ctx, margs...); err != nil {
						logger.Error(err, "update acl config failed")
					}
				}
				inst.SendEventf(corev1.EventTypeNormal, config.EventUpdatePassword, "updated instance password and injected acl users")
			}
		}
	} else {
		var (
			password = ""
			secret   *corev1.Secret
		)
		if passwordSecret := inst.Definition().Spec.Auth.SecretPath; passwordSecret != "" {
			if secret, err = a.client.GetSecret(ctx, inst.GetNamespace(), passwordSecret); err == nil {
				password = string(secret.Data["password"])
			} else if !errors.IsNotFound(err) {
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
		}

		updateMasterAuth := []interface{}{"config", "set", "masterauth", password}
		updateRequirePass := []interface{}{"config", "set", "requirepass", password}
		// 如果全部节点更新密码,设置sentinel的密码
		allRedisNodeApplied := true
		for _, node := range inst.Nodes() {
			if node.ContainerStatus() == nil || !node.IsReady() || node.IsTerminating() {
				allRedisNodeApplied = false
				continue
			}
			if err := node.Setup(ctx, updateMasterAuth, updateRequirePass); err != nil {
				allRedisNodeApplied = false
				logger.Error(err, "update nodes auth info failed")
			}

			// Retry hard
			cmd := []string{"sh", "-c", fmt.Sprintf(`echo -n '%s' > /tmp/newpass`, password)}
			if err := util.RetryOnTimeout(func() error {
				_, _, err := a.client.Exec(ctx, node.GetNamespace(), node.GetName(), failoverbuilder.ServerContainerName, cmd)
				return err
			}, 5); err != nil {
				logger.Error(err, "patch new secret to pod failed", "pod", node.GetName())
			}
		}
		if allRedisNodeApplied {
			if err := inst.Monitor().UpdateConfig(ctx, map[string]string{"auth-pass": password}); err != nil {
				logger.Error(err, "update sentinel auth info failed")
			}
		}
		users := inst.Users()
		if secret != nil {
			passwd, err := user.NewPassword(secret)
			if err != nil {
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
			users.GetDefaultUser().Password = passwd
		} else {
			users.GetDefaultUser().Password = nil
		}
		data := users.Encode(true)
		err := a.client.CreateOrUpdateConfigMap(ctx, inst.GetNamespace(), failoverbuilder.NewFailoverAclConfigMap(inst.Definition(), data))
		if err != nil {
			return actor.NewResultWithError(sentinel.CommandRequeue, err)
		}
		inst.SendEventf(corev1.EventTypeNormal, config.EventUpdatePassword, "updated instance password")

		return actor.NewResult(sentinel.CommandRequeue)
	}
	return nil
}
