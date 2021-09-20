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
	"strings"
	"time"

	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/pkg/ops/sentinel"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/strings/slices"
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
	return []actor.Command{sentinel.CommandUpdateAccount}
}

func (a *actorUpdateAccount) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	sen := val.(types.RedisFailoverInstance)
	var (
		isACLAppliedInPods = true
		isAllACLSupported  = true
		isAllPodReady      = true
	)
	for _, node := range sen.Nodes() {
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
	a.logger.V(3).Info("update account",
		"isAllACLSupported", isAllACLSupported,
		"isACLAppliedInPods", isACLAppliedInPods,
		"version", sen.Users().Encode(),
	)

	if isAllACLSupported && isACLAppliedInPods {
		opSecretName := sentinelbuilder.GenerateSentinelACLOperatorSecretName(sen.GetName())
		secret := sentinelbuilder.NewSentinelOpSecret(sen.Definition())
		if err := a.client.CreateIfNotExistsSecret(ctx, sen.GetNamespace(), secret); err != nil {
			a.logger.Info("create operator secret", "secret", secret)
		}
		users := sen.Users()
		if _, ok := users.Encode()[user.DefaultOperatorUserName]; !ok {
			opUser, err := user.NewOperatorUser(secret, sen.Version().IsACL2Supported())
			if err != nil {
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
			users = append(users, opUser)
			data := users.Encode()
			allApplied := true
			if isAllPodReady {
				for _, node := range sen.Nodes() {
					if err := node.Setup(ctx, formatACLSetCommand(users.GetOpUser())); err != nil {
						allApplied = false
						a.logger.Error(err, "update acl config failed")
					}
				}
				if allApplied {
					err = a.client.CreateOrUpdateConfigMap(ctx, sen.GetNamespace(), sentinelbuilder.NewSentinelAclConfigMap(sen.Definition(), data))
					if err != nil {
						return actor.NewResultWithError(sentinel.CommandRequeue, err)
					}
				}
			}
			return actor.NewResult(sentinel.CommandRequeue)
		} else if users.GetOpUser() != nil && sen.Version().IsACL2Supported() {
			if len(users.GetOpUser().Rules[0].Channels) == 0 {
				users.GetOpUser().Rules[0].Channels = []string{"*"}
			}
			data := users.Encode()
			allApplied := true
			if isAllPodReady {
				for _, node := range sen.Nodes() {
					if err := node.Setup(ctx, formatACLSetCommand(users.GetOpUser())); err != nil {
						allApplied = false
						a.logger.Error(err, "update acl config failed")
					}
				}
				if allApplied {
					err := a.client.CreateOrUpdateConfigMap(ctx, sen.GetNamespace(), sentinelbuilder.NewSentinelAclConfigMap(sen.Definition(), data))
					if err != nil {
						return actor.NewResultWithError(sentinel.CommandRequeue, err)
					}
				}
			}
		}

		data := users.Encode()
		err := a.client.CreateIfNotExistsConfigMap(ctx, sen.GetNamespace(), sentinelbuilder.NewSentinelAclConfigMap(sen.Definition(), data))
		if err != nil {
			return actor.NewResultWithError(sentinel.CommandRequeue, err)
		}
		if sen.Users().GetOpUser() == nil && sen.Version().IsACLSupported() {
			ru := sentinelbuilder.GenerateSentinelOperatorsRedisUser(sen, opSecretName)
			if err := a.client.CreateIfNotExistsRedisUser(ctx, &ru); err != nil {
				a.logger.Error(err, "create operator user redisUser failed")
			}
		}
		for _, _user := range sen.Users() {
			if _user.Name == user.DefaultUserName {
				ru := sentinelbuilder.GenerateSentinelDefaultRedisUser(sen.Definition(), sen.Definition().Spec.Auth.SecretPath)
				oldRu, err := a.client.GetRedisUser(ctx, sen.GetNamespace(), ru.Name)
				if err == nil && !slices.Equal(oldRu.Spec.PasswordSecrets, ru.Spec.PasswordSecrets) {
					oldRu.Spec.PasswordSecrets = ru.Spec.PasswordSecrets
					ru = *oldRu
					if err := a.client.UpdateRedisUser(ctx, &ru); err != nil {
						a.logger.Error(err, "update default user redisUser failed")
					}
				} else if errors.IsNotFound(err) {
					if err := a.client.CreateRedisUser(ctx, &ru); err != nil {
						a.logger.Error(err, "create default user redisUser failed")
					}
				}
			}
			if _user.Name == user.DefaultOperatorUserName {
				if _user.Password == nil {
					return actor.NewResultWithError(sentinel.CommandRequeue, fmt.Errorf("operator user password is nil"))
				}
				ru := sentinelbuilder.GenerateSentinelOperatorsRedisUser(sen, _user.Password.SecretName)
				oldRu, err := a.client.GetRedisUser(ctx, sen.GetNamespace(), ru.Name)
				if err == nil && !slices.Equal(oldRu.Spec.PasswordSecrets, ru.Spec.PasswordSecrets) {
					oldRu.Spec.PasswordSecrets = ru.Spec.PasswordSecrets
					ru = *oldRu
					if err := a.client.UpdateRedisUser(ctx, &ru); err != nil {
						a.logger.Error(err, "update default user redisUser failed")
					}
				} else if errors.IsNotFound(err) {
					if err := a.client.CreateRedisUser(ctx, &ru); err != nil {
						a.logger.Error(err, "create operator user redisUser failed")
					}
				}
			}

		}
	} else if sen.Version().IsACLSupported() && isAllACLSupported {
		margs := [][]interface{}{}
		margs = append(
			margs,
			[]interface{}{"config", "set", "masteruser", sen.Users().GetOpUser().Name},
			[]interface{}{"config", "set", "masterauth", sen.Users().GetOpUser().Password},
		)
		for _, node := range sen.Nodes() {
			if node.ContainerStatus() == nil || !node.ContainerStatus().Ready ||
				node.IsTerminating() {
				continue
			}

			if err := node.Setup(ctx, margs...); err != nil {
				a.logger.Error(err, "update acl config failed")
			}
		}
		// then requeue to refresh cluster info
		a.logger.Info("requeue to refresh cluster info")
		return actor.NewResultWithValue(sentinel.CommandRequeue, time.Second)
	} else if sen.Version().IsACLSupported() && !isAllACLSupported && isAllPodReady {
		secret := sentinelbuilder.NewSentinelOpSecret(sen.Definition())
		if err := a.client.CreateIfNotExistsSecret(ctx, sen.GetNamespace(), secret); err != nil {
			a.logger.Info("create operator secret", "secret", secret)
		}
		users := sen.Users()
		if _, ok := users.Encode()[user.DefaultOperatorUserName]; !ok {
			opUser, err := user.NewOperatorUser(secret, sen.Version().IsACL2Supported())
			if err != nil {
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
			users = append(users, opUser)
			data := users.Encode()
			err = a.client.CreateOrUpdateConfigMap(ctx, sen.GetNamespace(), sentinelbuilder.NewSentinelAclConfigMap(sen.Definition(), data))
			if err != nil {
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
		}
	} else {
		password := ""
		secret := &corev1.Secret{}
		if sen.Definition().Spec.Auth.SecretPath != "" {
			_secret, err := a.client.GetSecret(ctx, sen.GetNamespace(), sen.Definition().Spec.Auth.SecretPath)
			if err == nil {
				secret = _secret
				password = string(secret.Data["password"])
			} else if !errors.IsNotFound(err) {
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
		}
		updateMasterAuth := []interface{}{"config", "set", "masterauth", password}
		updateRequirePass := []interface{}{"config", "set", "requirepass", password}
		for _, node := range sen.Nodes() {
			if node.ContainerStatus() == nil || !node.ContainerStatus().Ready ||
				node.IsTerminating() {
				continue
			}

			if err := node.Setup(ctx, updateMasterAuth); err != nil {
				a.logger.Error(err, "")
			}
			if err := node.Setup(ctx, updateRequirePass); err != nil {
				a.logger.Error(err, "")
			}

			cmd := []string{"sh", "-c", fmt.Sprintf(`echo -n '%s' > /tmp/newpass`, password)}
			if !node.IsReady() || node.IsTerminating() {
				continue
			}

			// Retry hard
			if err := util.RetryOnTimeout(func() error {
				_, _, err := a.client.Exec(ctx, node.GetNamespace(), node.GetName(), clusterbuilder.ServerContainerName, cmd)
				return err
			}, 5); err != nil {
				a.logger.Error(err, "patch new secret to pod failed", "pod", node.GetName())
			}
		}
		users := sen.Users()
		if sen.Definition().Spec.Auth.SecretPath != "" {
			passwd, err := user.NewPassword(secret)
			if err != nil {
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
			users.GetDefaultUser().Password = passwd
		} else {
			users.GetDefaultUser().Password = nil
		}
		data := users.Encode()
		err := a.client.CreateOrUpdateConfigMap(ctx, sen.GetNamespace(), sentinelbuilder.NewSentinelAclConfigMap(sen.Definition(), data))
		if err != nil {
			return actor.NewResultWithError(sentinel.CommandRequeue, err)
		}
	}

	return actor.NewResult(sentinel.CommandEnsureResource)
}

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
