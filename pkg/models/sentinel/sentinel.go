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

package sentinel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"reflect"
	"strconv"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	redismiddlewarealaudaiov1 "github.com/alauda/redis-operator/api/redis/v1"
	clientset "github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/pkg/security/acl"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.RedisInstance = (*RedisFailover)(nil)

type RedisFailover struct {
	databasesv1.RedisFailover
	redisUsers []*redismiddlewarealaudaiov1.RedisUser
	ctx        context.Context
	client     clientset.ClientSet
	// version    redis.RedisVersion
	users     acl.Users
	tlsConfig *tls.Config
	configmap map[string]string
	replicas  []types.RedisSentinelReplica
	sentinel  types.RedisSentinelNodes
	logger    logr.Logger
	// selector   map[string]string
}

func NewRedisFailover(ctx context.Context, k8sClient clientset.ClientSet, def *databasesv1.RedisFailover, logger logr.Logger) (*RedisFailover, error) {
	sentinel := &RedisFailover{
		RedisFailover: *def,
		ctx:           ctx,
		client:        k8sClient,
		configmap:     make(map[string]string),
		logger:        logger,
	}
	var err error
	if sentinel.users, err = sentinel.loadUsers(ctx); err != nil {
		sentinel.logger.Error(err, "load user failed")
		return nil, err
	}
	if sentinel.tlsConfig, err = sentinel.loadTLS(ctx); err != nil {
		sentinel.logger.Error(err, "loads tls failed")
		return nil, err
	}
	if sentinel.replicas, err = LoadRedisSentinelReplicas(ctx, k8sClient, sentinel, logger); err != nil {
		sentinel.logger.Error(err, "load replicas failed")
		return nil, err
	}

	if sentinel.sentinel, err = LoadRedisSentinelNodes(ctx, k8sClient, sentinel, logger); err != nil {
		sentinel.logger.Error(err, "load sentinels failed")
		return nil, err
	}

	if sentinel.Version().IsACLSupported() {
		sentinel.LoadRedisUsers(ctx)
	}

	return sentinel, nil
}

func (c *RedisFailover) UpdateStatus(ctx context.Context, status databasesv1.RedisFailoverStatus) error {
	if c == nil {
		return nil
	}
	count := 0

	for _, r := range c.replicas {
		for _, n := range r.Nodes() {
			if n.Role() == redis.RedisRoleMaster {
				status.Master.Name = n.GetName()
			}
			count += 1
		}
		if r.Status().ReadyReplicas != c.Definition().Spec.Redis.Replicas {
			if !(status.Phase == databasesv1.PhaseFail || status.Phase == databasesv1.PhasePaused) {
				status.Phase = databasesv1.PhaseWaitingPodReady
			}
		}
	}
	status.Instance.Redis.Size = int32(count)

	if count != int(c.Definition().Spec.Redis.Replicas) {
		if !(status.Phase == databasesv1.PhaseFail || status.Phase == databasesv1.PhasePaused) {
			status.Phase = databasesv1.PhaseWaitingPodReady
		}
	}
	count = 0
	status.Master.Status = databasesv1.RedisStatusMasterDown
	if c.sentinel != nil {
		for _, n := range c.sentinel.Nodes() {
			if n.Info().SentinelMaster0.Status == "ok" {
				status.Master.Address = n.Info().SentinelMaster0.Address.ToString()
				status.Master.Status = databasesv1.RedisStatusMasterOK
				status.Instance.Redis.Ready = int32(n.Info().SentinelMaster0.Replicas) + 1
				status.Instance.Sentinel.Ready = int32(n.Info().SentinelMaster0.Sentinels)
			}
			count += 1
		}
		if c.sentinel.Status().ReadyReplicas != c.Definition().Spec.Sentinel.Replicas {
			if !(status.Phase == databasesv1.PhaseFail || status.Phase == databasesv1.PhasePaused) {
				status.Phase = databasesv1.PhaseWaitingPodReady
			}
		}
	}
	if count != int(c.Definition().Spec.Sentinel.Replicas) {
		if !(status.Phase == databasesv1.PhaseFail || status.Phase == databasesv1.PhasePaused) {
			status.Phase = databasesv1.PhasePending
		}
	}
	if status.Instance.Redis.Ready != c.Definition().Spec.Redis.Replicas ||
		status.Instance.Sentinel.Ready != c.Definition().Spec.Sentinel.Replicas {
		if !(status.Phase == databasesv1.PhaseFail || status.Phase == databasesv1.PhasePaused) {
			status.Phase = databasesv1.PhaseWaitingPodReady
		}
	}

	status.Instance.Sentinel.Size = int32(count)
	svcName := sentinelbuilder.GetSentinelServiceName(c.GetName())
	status.Instance.Sentinel.Service = svcName
	if svc, err := c.client.GetService(ctx, c.GetNamespace(), svcName); err != nil {
		if errors.IsNotFound(err) {
			c.logger.Info("sen service not found")
		} else {
			return err
		}
	} else {
		status.Instance.Sentinel.Port = strconv.Itoa(int(svc.Spec.Ports[0].Port))
		status.Instance.Sentinel.ClusterIP = svc.Spec.ClusterIP
	}

	for _, repl := range c.replicas {
		if repl.Status().CurrentRevision != repl.Status().UpdateRevision {
			if !(status.Phase == databasesv1.PhaseFail || status.Phase == databasesv1.PhasePaused) {
				status.Phase = databasesv1.PhaseWaitingPodReady
			}
		}
	}

	if c.sentinel != nil && c.sentinel.Status().Replicas != c.sentinel.Status().UpdatedReplicas {
		if !(status.Phase == databasesv1.PhaseFail || status.Phase == databasesv1.PhasePaused) {
			status.Phase = databasesv1.PhaseWaitingPodReady
		}
	}

	if status.Phase == "" {
		status.Phase = databasesv1.PhaseReady
	}

	if !reflect.DeepEqual(c.RedisFailover.Status, status) {
		c.RedisFailover.Status = status
		return c.client.Client().Status().Update(ctx, &c.RedisFailover)
	}
	return nil

}

// common method

func (c *RedisFailover) Definition() *databasesv1.RedisFailover {
	if c == nil {
		return nil
	}
	return &c.RedisFailover
}

func (c *RedisFailover) Version() redis.RedisVersion {
	if c == nil {
		return redis.RedisVersionUnknown
	}

	if version, err := redis.ParseRedisVersionFromImage(c.Spec.Redis.Image); err != nil {
		c.logger.Error(err, "parse redis version failed")
		return redis.RedisVersionUnknown
	} else {
		return version
	}
}

func (c *RedisFailover) Masters() []redis.RedisNode {
	var ret []redis.RedisNode
	for _, nodes := range c.replicas {
		for _, v := range nodes.Nodes() {
			if v.Role() == redis.RedisRoleMaster {
				ret = append(ret, v)
			}
		}
	}
	return ret
}

func (c *RedisFailover) Nodes() []redis.RedisNode {
	var ret []redis.RedisNode
	for _, v := range c.replicas {
		ret = append(ret, v.Nodes()...)
	}
	return ret
}

func (c *RedisFailover) SentinelNodes() []redis.RedisNode {
	var ret []redis.RedisNode
	if c.sentinel != nil {
		ret = append(ret, c.sentinel.Nodes()...)
	}
	return ret
}

func (c *RedisFailover) Sentinel() types.RedisSentinelNodes {
	if c == nil {
		return nil
	}
	return c.sentinel
}

func (c *RedisFailover) IsInService() bool {
	return c != nil
}

func (c *RedisFailover) IsReady() bool {
	if c == nil {
		return false
	}
	if c.RedisFailover.Status.Phase == databasesv1.PhaseReady {
		return true
	}
	return false
}

func (c *RedisFailover) Users() (us acl.Users) {
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

func (c *RedisFailover) Restart(ctx context.Context) error {
	if c == nil {
		return nil
	}
	for _, v := range c.replicas {
		if err := v.Restart(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *RedisFailover) Refresh(ctx context.Context) error {
	if c == nil {
		return nil
	}
	logger := c.logger.WithName("Refresh")
	logger.V(3).Info("refreshing sentinel", "target", fmt.Sprintf("%s/%s", c.GetNamespace(), c.GetName()))

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
		return c.client.Client().Get(ctx, client.ObjectKeyFromObject(&c.RedisFailover), &cr)
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
	c.RedisFailover = cr
	err := c.RedisFailover.Validate()
	if err != nil {
		return err
	}

	if c.users, err = c.loadUsers(ctx); err != nil {
		logger.Error(err, "load users failed")
		return err
	}

	if c.replicas, err = LoadRedisSentinelReplicas(ctx, c.client, c, logger); err != nil {
		logger.Error(err, "load replicas failed")
		return err
	}
	if c.sentinel, err = LoadRedisSentinelNodes(ctx, c.client, c, logger); err != nil {
		logger.Error(err, "load sentinels failed")
		return err
	}
	return nil
}

func (c *RedisFailover) LoadRedisUsers(ctx context.Context) {
	oldOpUser, _ := c.client.GetRedisUser(ctx, c.GetNamespace(), sentinelbuilder.GenerateSentinelOperatorsRedisUserName(c.GetName()))
	oldDefultUser, _ := c.client.GetRedisUser(ctx, c.GetNamespace(), sentinelbuilder.GenerateSentinelDefaultRedisUserName(c.GetName()))
	c.redisUsers = []*redismiddlewarealaudaiov1.RedisUser{oldOpUser, oldDefultUser}
}

func (c *RedisFailover) loadUsers(ctx context.Context) (acl.Users, error) {
	var (
		name  = sentinelbuilder.GenerateSentinelACLConfigMapName(c.GetName())
		users acl.Users
	)
	if cm, err := c.client.GetConfigMap(ctx, c.GetNamespace(), name); errors.IsNotFound(err) {
		var (
			username       string
			passwordSecret string
			secret         *corev1.Secret
		)
		statefulSetName := sentinelbuilder.GetSentinelStatefulSetName(c.GetName())
		sts, err := c.client.GetStatefulSet(ctx, c.GetNamespace(), statefulSetName)
		if err != nil {
			if !errors.IsNotFound(err) {
				c.logger.Error(err, "load statefulset failed", "target", fmt.Sprintf("%s/%s", c.GetNamespace(), c.GetName()))
			}
			if c.Version().IsACLSupported() {
				passwordSecret = sentinelbuilder.GenerateSentinelACLOperatorSecretName(c.GetName())
				username = user.DefaultOperatorUserName
			}
		} else {
			spec := sts.Spec.Template.Spec
			if container := util.GetContainerByName(&spec, sentinelbuilder.ServerContainerName); container != nil {
				for _, env := range container.Env {
					if env.Name == sentinelbuilder.PasswordENV && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
						passwordSecret = env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
					} else if env.Name == sentinelbuilder.OperatorSecretName && env.Value != "" {
						passwordSecret = env.Value
					} else if env.Name == sentinelbuilder.OperatorUsername {
						username = env.Value
					}
				}
			}
		}

		if passwordSecret != "" {
			objKey := client.ObjectKey{Namespace: c.GetNamespace(), Name: passwordSecret}
			if secret, err = c.loadUserSecret(ctx, objKey); err != nil {
				c.logger.Error(err, "load user secret failed", "target", objKey)
				return nil, err
			}
		} else if c.Spec.Auth.SecretPath != "" {
			secret, err = c.client.GetSecret(ctx, c.GetNamespace(), c.Spec.Auth.SecretPath)
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
			if u, err := user.NewOperatorUser(secret, c.Version().IsACL2Supported()); err != nil {
				c.logger.Error(err, "init users failed")
				return nil, err
			} else {
				users = append(users, u)
			}

			if c.Spec.Auth.SecretPath != "" {
				secret, err = c.client.GetSecret(ctx, c.GetNamespace(), c.Spec.Auth.SecretPath)
				if err != nil {
					return nil, err
				}
				u, _ := user.NewUser(user.DefaultUserName, user.RoleDeveloper, secret)
				users = append(users, u)
			} else {
				u, _ := user.NewUser(user.DefaultUserName, user.RoleDeveloper, nil)
				users = append(users, u)
			}
		} else {
			if u, err := user.NewUser(username, role, secret); err != nil {
				c.logger.Error(err, "init users failed")
				return nil, err
			} else {
				users = append(users, u)
			}
		}
	} else if err != nil {
		c.logger.Error(err, "load default users's password secret failed", "target", fmt.Sprintf("%s/%s", c.GetNamespace(), name))
		return nil, err
	} else if users, err = acl.LoadACLUsers(ctx, c.client, cm); err != nil {
		c.logger.Error(err, "load acl failed")
		return nil, err
	}
	return users, nil
}

func (c *RedisFailover) loadUserSecret(ctx context.Context, objKey client.ObjectKey) (*corev1.Secret, error) {
	secret, err := c.client.GetSecret(ctx, objKey.Namespace, objKey.Name)
	if err != nil && !errors.IsNotFound(err) {
		c.logger.Error(err, "load default users's password secret failed", "target", objKey.String())
		return nil, err
	} else if errors.IsNotFound(err) {
		secret = sentinelbuilder.NewSentinelOpSecret(c.Definition())
		err := c.client.CreateSecret(ctx, objKey.Namespace, secret)
		if err != nil {
			return nil, err
		}
	} else if _, ok := secret.Data[user.PasswordSecretKey]; !ok {
		return nil, fmt.Errorf("no password found")
	}
	return secret, nil
}

func (c *RedisFailover) loadTLS(ctx context.Context) (*tls.Config, error) {
	if c == nil {
		return nil, nil
	}
	logger := c.logger.WithName("loadTLS")

	var secretName string

	// load current tls secret.
	// because previous cr not recorded the secret name, we should load it from statefulset
	stsName := sentinelbuilder.GetSentinelStatefulSetName(c.GetName())
	if sts, err := c.client.GetStatefulSet(ctx, c.GetNamespace(), stsName); err != nil {
		c.logger.Error(err, "load statefulset failed", "target", fmt.Sprintf("%s/%s", c.GetNamespace(), c.GetName()))
	} else {
		for _, vol := range sts.Spec.Template.Spec.Volumes {
			if vol.Name == sentinelbuilder.RedisTLSVolumeName {
				secretName = vol.VolumeSource.Secret.SecretName
			}
		}
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

func (c *RedisFailover) Selector() map[string]string {
	if c == nil {
		return nil
	}
	if len(c.replicas) > 0 {
		return c.replicas[0].Definition().Spec.Selector.MatchLabels
	}
	if c.sentinel != nil {
		return c.sentinel.Definition().Spec.Selector.MatchLabels
	}
	return nil
}

func (c *RedisFailover) IsACLUserExists() bool {
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
	}
	return true
}
