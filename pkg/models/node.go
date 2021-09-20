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

package models

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/sentinelbuilder"
	rediscli "github.com/alauda/redis-operator/pkg/redis"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/slot"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.RedisNode = (*RedisNode)(nil)

func LoadRedisSentinelNodes(ctx context.Context, client kubernetes.ClientSet, deploy *appv1.Deployment, newUser *user.User, logger logr.Logger) ([]redis.RedisNode, error) {
	if client == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if deploy == nil {
		return nil, fmt.Errorf("require deployment")
	}
	pods, err := client.GetDeploymentPods(ctx, deploy.GetNamespace(), deploy.GetName())
	if err != nil {
		logger.Error(err, "loads pods of shard failed")
		return nil, err
	}
	nodes := []redis.RedisNode{}
	for _, pod := range pods.Items {
		if node, err := NewRedisSentinelNode(ctx, client, deploy, &pod, newUser, logger); err != nil {
			logger.Error(err, "parse redis node failed", "pod", pod.Name)
		} else {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func NewRedisSentinelNode(ctx context.Context, client kubernetes.ClientSet, deploy *appv1.Deployment, pod *corev1.Pod, newUser *user.User, logger logr.Logger) (redis.RedisNode, error) {
	if client == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if deploy == nil {
		return nil, fmt.Errorf("require deployment")
	}
	if pod == nil {
		return nil, fmt.Errorf("require pod")
	}

	node := RedisNode{
		Pod:        *pod,
		client:     client,
		deployment: deploy,
		newUser:    newUser,
		logger:     logger.WithName("RedisNode"),
	}

	var err error
	if node.opUser, err = node.loadOperatorUser(ctx); err != nil {
		return nil, err
	}
	if node.tlsConfig, err = node.loadTLS(ctx); err != nil {
		return nil, err
	}

	if node.IsContainerReady() {
		redisCli, err := node.getRedisConnect(ctx, &node)
		if err != nil {
			return nil, err
		}
		defer redisCli.Close()

		if node.info, node.config, node.nodes, err = node.loadRedisInfo(ctx, &node, redisCli); err != nil {
			return nil, err
		}
	}
	return &node, nil
}

// LoadRedisNodes
func LoadRedisNodes(ctx context.Context, client kubernetes.ClientSet, sts *appv1.StatefulSet, newUser *user.User, logger logr.Logger) ([]redis.RedisNode, error) {
	if client == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if sts == nil {
		return nil, fmt.Errorf("require statefulset")
	}

	// load pods by statefulset selector
	pods, err := client.GetStatefulSetPodsByLabels(ctx, sts.GetNamespace(), sts.Spec.Selector.MatchLabels)
	if err != nil {
		logger.Error(err, "loads pods of shard failed")
		return nil, err
	}

	nodes := []redis.RedisNode{}
	for _, pod := range pods.Items {
		if !func() bool {
			for _, own := range pod.OwnerReferences {
				if own.UID == sts.GetUID() {
					return true
				}
			}
			return false
		}() {
			continue
		}

		if node, err := NewRedisNode(ctx, client, sts, &pod, newUser, logger); err != nil {
			logger.Error(err, "parse redis node failed", "pod", pod.Name)
		} else {
			nodes = append(nodes, node)
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].Index() < nodes[j].Index()
	})
	return nodes, nil
}

func LoadSentinelRedisNodes(ctx context.Context, client kubernetes.ClientSet, sts *appv1.StatefulSet, newUser *user.User, logger logr.Logger) ([]redis.RedisNode, error) {
	if client == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if sts == nil {
		return nil, fmt.Errorf("require statefulset")
	}

	// load pods by statefulset selector
	pods, err := client.GetStatefulSetPodsByLabels(ctx, sts.GetNamespace(), sts.Spec.Selector.MatchLabels)
	if err != nil {
		logger.Error(err, "loads pods of shard failed")
		return nil, err
	}

	nodes := []redis.RedisNode{}
	for _, pod := range pods.Items {
		if !func() bool {
			for _, own := range pod.OwnerReferences {
				if own.UID == sts.GetUID() {
					return true
				}
			}
			return false
		}() {
			continue
		}

		if node, err := NewRedisNode(ctx, client, sts, &pod, newUser, logger); err != nil {
			logger.Error(err, "parse redis node failed", "pod", pod.Name)
		} else {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

type RedisNode struct {
	corev1.Pod
	client      kubernetes.ClientSet
	statefulSet *appv1.StatefulSet
	deployment  *appv1.Deployment
	opUser      *user.User
	newUser     *user.User
	tlsConfig   *tls.Config
	info        *rediscli.RedisInfo
	nodes       rediscli.ClusterNodes
	config      map[string]string
	logger      logr.Logger
}

type RedisSentinelNode struct {
}

func (n *RedisNode) Definition() *corev1.Pod {
	if n == nil {
		return nil
	}
	return &n.Pod
}

// NewRedisNode
func NewRedisNode(ctx context.Context, client kubernetes.ClientSet, sts *appv1.StatefulSet, pod *corev1.Pod, newUser *user.User, logger logr.Logger) (redis.RedisNode, error) {
	if client == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if sts == nil {
		return nil, fmt.Errorf("require statefulset")
	}
	if pod == nil {
		return nil, fmt.Errorf("require pod")
	}

	node := RedisNode{
		Pod:         *pod,
		client:      client,
		statefulSet: sts,
		newUser:     newUser,
		logger:      logger.WithName("RedisNode"),
	}

	var err error
	if node.opUser, err = node.loadOperatorUser(ctx); err != nil {
		return nil, err
	}
	if node.tlsConfig, err = node.loadTLS(ctx); err != nil {
		return nil, err
	}

	if node.IsContainerReady() {
		redisCli, err := node.getRedisConnect(ctx, &node)
		if err != nil {
			return nil, err
		}
		defer redisCli.Close()

		if node.info, node.config, node.nodes, err = node.loadRedisInfo(ctx, &node, redisCli); err != nil {
			return nil, err
		}
	}
	return &node, nil
}

// loadOperatorUser
//
// every pod still mount secret to the pod. for acl supported and not supported versions, the difference is that:
// unsupported: the mount secret is the default user secret, which maybe changed
// supported: the mount secret is operator's secret. the operator secret never changes, which is used only internal
//
// this method is used to fetch pod's operator secret
// for versions without acl supported, there exists cases that the env secret not consistent with the server
func (s *RedisNode) loadOperatorUser(ctx context.Context) (*user.User, error) {
	if s == nil {
		return nil, nil
	}
	logger := s.logger.WithName("loadOperatorUser")

	var (
		secretName string
		username   string
	)
	if s.IsSentinelPod() {
		return user.NewUser("", user.RoleDeveloper, nil)
	}
	container := util.GetContainerByName(&s.Spec, clusterbuilder.ServerContainerName)
	if container == nil {
		return nil, fmt.Errorf("server container not found")
	}
	for _, env := range container.Env {
		if env.Name == clusterbuilder.PasswordENV && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
			secretName = env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
		} else if env.Name == clusterbuilder.OperatorSecretName && env.Value != "" {
			secretName = env.Value
		} else if env.Name == clusterbuilder.OperatorUsername {
			username = env.Value
		}
	}

	if secretName != "" {
		if secret, err := s.client.GetSecret(ctx, s.GetNamespace(), secretName); err != nil {
			logger.Error(err, "get user secret failed", "target", fmt.Sprintf("%s/%s", s.GetNamespace(), secretName))
			return nil, err
		} else if user, err := user.NewUser(username, user.RoleDeveloper, secret); err != nil {
			return nil, err
		} else {
			return user, nil
		}
	}
	// return default user with out password
	return user.NewUser("", user.RoleDeveloper, nil)
}

func (n *RedisNode) loadTLS(ctx context.Context) (*tls.Config, error) {
	if n == nil {
		return nil, nil
	}
	logger := n.logger

	var name string
	for _, vol := range n.Spec.Volumes {
		if vol.Name == clusterbuilder.RedisTLSVolumeName && vol.Secret != nil && vol.Secret.SecretName != "" {
			name = vol.Secret.SecretName
			break
		}
	}
	if name == "" {
		return nil, nil
	}

	secret, err := n.client.GetSecret(ctx, n.GetNamespace(), name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		logger.Error(err, "get secret failed", "target", name)
		return nil, err
	}

	if secret.Data[corev1.TLSCertKey] == nil || secret.Data[corev1.TLSPrivateKeyKey] == nil ||
		secret.Data["ca.crt"] == nil {
		logger.Error(fmt.Errorf("invalid tls secret"), "tls secret is invaid")
		return nil, fmt.Errorf("tls secret is invalid")
	}
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

func (n *RedisNode) getRedisConnect(ctx context.Context, node *RedisNode) (rediscli.RedisClient, error) {
	if n == nil {
		return nil, fmt.Errorf("nil node")
	}
	logger := n.logger.WithName("getRedisConnect")

	if n.Status() != corev1.PodRunning || !n.IsContainerReady() {
		logger.Error(fmt.Errorf("get redis info failed"), "pod not ready", "target",
			client.ObjectKey{Namespace: node.Namespace, Name: node.Name})
		return nil, fmt.Errorf("node not ready")
	}

	var (
		err  error
		host string
		port = n.InternalPort()
	)
	// this should not happen for running pods
	if host = node.DefaultInternalIP().String(); host == "" {
		return nil, fmt.Errorf("no ip found for pod %s", node.Name)
	}
	if strings.Contains(host, ":") {
		host = fmt.Sprintf("[%s]", host)
	}

	for _, user := range []*user.User{node.opUser, node.newUser} {
		if user == nil {
			continue
		}
		name := user.Name
		if !node.CurrentVersion().IsACLSupported() {
			name = ""
		}
		password := user.Password.String()
		if node.IsSentinelPod() {
			password = ""
		}
		rediscli := rediscli.NewRedisClient(fmt.Sprintf("%s:%d", host, port), rediscli.AuthConfig{
			Username:  name,
			Password:  password,
			TLSConfig: node.tlsConfig,
		})

		nctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()

		if err = rediscli.Ping(nctx); err != nil {
			if strings.Contains(err.Error(), "NOAUTH Authentication required") ||
				strings.Contains(err.Error(), "invalid password") ||
				strings.Contains(err.Error(), "Client sent AUTH, but no password is set") ||
				strings.Contains(err.Error(), "invalid username-password pair") {
				continue
			}
			logger.Error(err, "check connection to redis failed", "address", node.DefaultInternalIP().String())
			return nil, err
		}
		return rediscli, nil
	}
	if err == nil {
		err = fmt.Errorf("no usable account to connect to redis instance")
	}
	return nil, err
}

// loadRedisInfo
func (n *RedisNode) loadRedisInfo(ctx context.Context, node *RedisNode, redisCli rediscli.RedisClient) (info *rediscli.RedisInfo,
	config map[string]string, nodes rediscli.ClusterNodes, err error) {
	// fetch redis info
	if info, err = redisCli.Info(ctx); err != nil {
		n.logger.Error(err, "load redis info failed")
		return nil, nil, nil, err
	}
	if !n.IsSentinelPod() {
		// fetch current config
		if config, err = redisCli.ConfigGet(ctx, "*"); err != nil {
			n.logger.Error(err, "get redis config failed, ignore")
		}
	}

	if info.ClusterEnabled == "1" {
		// load cluster nodes
		if items, err := redisCli.Nodes(ctx); err != nil {
			n.logger.Error(err, "load redis cluster nodes info failed, ignore")
		} else {
			for _, n := range items {
				// clean disconnected nodes
				if n.LinkState == "disconnected" && strings.Contains(n.RawFlag, "noaddr") {
					if _, err := redisCli.Do(ctx, "CLUSTER", "FORGET", n.Id); err != nil {
						node.logger.Error(err, "forget disconnected node failed", "id", n.Id)
					}
				} else {
					nodes = append(nodes, n)
				}
			}
		}

	}
	return info, config, nodes, nil
}

// Refresh not concurrency safe
func (n *RedisNode) Refresh(ctx context.Context) (err error) {
	if n == nil {
		return nil
	}

	// refresh pod first
	if pod, err := n.client.GetPod(ctx, n.GetNamespace(), n.GetName()); err != nil {
		n.logger.Error(err, "refresh pod failed")
		return err
	} else {
		n.Pod = *pod
	}

	opUser, err := n.loadOperatorUser(ctx)
	if err != nil {
		return err
	} else {
		n.opUser = opUser
	}

	if n.IsContainerReady() {
		redisCli, err := n.getRedisConnect(ctx, n)
		if err != nil {
			return err
		}
		defer redisCli.Close()

		if n.info, n.config, n.nodes, err = n.loadRedisInfo(ctx, n, redisCli); err != nil {
			n.logger.Error(err, "refresh info failed")
			return err
		}
	}
	return nil
}

// ID get cluster id
//
// TODO: if it's possible generate a const id, use this id as cluster id
func (n *RedisNode) ID() string {
	if n == nil || n.nodes == nil {
		return ""
	}
	if n.nodes.Self() != nil {
		return n.nodes.Self().Id
	}
	return ""
}

func (n *RedisNode) MasterID() string {
	if n == nil {
		return ""
	}
	if n.nodes.Self() != nil {
		return n.nodes.Self().MasterId
	}
	return ""
}

// IsMasterFailed
func (n *RedisNode) IsMasterFailed() bool {
	if n == nil {
		return false
	}
	self := n.nodes.Self()
	if self == nil {
		return false
	}
	if n.Role() == redis.RedisRoleMaster {
		return false
	}
	if self.MasterId != "" {
		for _, info := range n.nodes {
			if info.Id == self.MasterId {
				if strings.Contains(info.RawFlag, "fail") {
					return true
				}
				return false
			}
		}
	}
	return false
}

// IsConnected
func (n *RedisNode) IsConnected() bool {
	if n == nil {
		return false
	}
	if n.nodes.Self() != nil {
		return n.nodes.Self().LinkState == "connected"
	}
	return false
}

// IsContainerReady
func (n *RedisNode) IsContainerReady() bool {
	if n == nil {
		return false
	}

	for _, cond := range n.Pod.Status.ContainerStatuses {
		if cond.Name == clusterbuilder.ServerContainerName || cond.Name == sentinelbuilder.SentinelContainerName {
			// assume the main process is ready in 10s
			if cond.Started != nil && *cond.Started && cond.State.Running != nil &&
				time.Now().Sub(cond.State.Running.StartedAt.Time) > time.Second*10 {
				return true
			}
		}
	}
	return false
}

func (n *RedisNode) IsSentinelPod() bool {
	if n == nil {
		return false
	}
	for _, cond := range n.Pod.Status.ContainerStatuses {
		if cond.Name == sentinelbuilder.SentinelContainerName {
			return true
		}
	}
	return false
}

// IsReady
func (n *RedisNode) IsReady() bool {
	if n == nil {
		return false
	}

	for _, cond := range n.Pod.Status.ContainerStatuses {
		if cond.Name == clusterbuilder.ServerContainerName {
			return cond.Ready
		}
	}
	return false
}

// IsTerminating
func (n *RedisNode) IsTerminating() bool {
	if n == nil {
		return false
	}

	return n.DeletionTimestamp != nil
}

// master_link_status is up
func (n *RedisNode) IsMasterLinkUp() bool {
	if n == nil || n.info == nil {
		return false
	}
	return n.info.MasterLinkStatus == "up"
}

// IsJoined
func (n *RedisNode) IsJoined() bool {
	if n == nil {
		return false
	}
	brotherCount := 0
	for _, nn := range n.nodes {
		if nn.LinkState == "connected" && !nn.IsSelf() {
			brotherCount += 1
		}
	}
	return (n.nodes.Self() != nil) && (n.nodes.Self().Addr != "") && brotherCount > 0
}

// Slots
func (n *RedisNode) Slots() *slot.Slots {
	if n == nil {
		return nil
	}

	role := n.Role()
	if self := n.nodes.Self(); self != nil && role == redis.RedisRoleMaster {
		slots := slot.NewSlots()
		if err := slots.Load(self.Slots); err != nil {
			// this should not happen
			n.logger.Error(err, "load slots failed", "raw", self.Slots)
			return nil
		}
		return slots
	}
	return nil
}

// Index returns the index of the related pod
func (n *RedisNode) Index() int {
	if n == nil {
		return -1
	}

	name := n.Pod.Name
	if i := strings.LastIndex(name, "-"); i > 0 {
		index, _ := strconv.ParseInt(name[i+1:], 10, 64)
		return int(index)
	}
	return -1
}

func (n *RedisNode) IsACLApplied() bool {
	// check if acl have been applied to container
	container := util.GetContainerByName(&n.Pod.Spec, clusterbuilder.ServerContainerName)
	if n.IsSentinelPod() {
		container = util.GetContainerByName(&n.Pod.Spec, sentinelbuilder.SentinelContainerName)
	}
	for _, env := range container.Env {
		if env.Name == "ACL_CONFIGMAP_NAME" {
			return true
		}
	}
	return false
}

func (n *RedisNode) CurrentVersion() redis.RedisVersion {
	if n == nil {
		return ""
	}

	// parse version from redis image
	container := util.GetContainerByName(&n.Pod.Spec, clusterbuilder.ServerContainerName)
	if n.IsSentinelPod() {
		container = util.GetContainerByName(&n.Pod.Spec, sentinelbuilder.SentinelContainerName)
	}
	if ver, _ := redis.ParseRedisVersionFromImage(container.Image); ver != redis.RedisVersionUnknown {
		return ver
	}

	v, _ := redis.ParseRedisVersion(n.info.RedisVersion)
	return v
}

func (n *RedisNode) Role() redis.RedisRole {
	if n == nil || n.info == nil {
		return redis.RedisRoleNone
	}
	return redis.NewRedisRole(n.info.Role)
}

func (n *RedisNode) Config() map[string]string {
	if n == nil || n.config == nil {
		return nil
	}
	return n.config
}

func (n *RedisNode) ConfigedMasterIP() string {
	if n == nil || n.info == nil {
		return ""
	}
	return n.info.MasterHost
}

func (n *RedisNode) ConfigedMasterPort() string {
	if n == nil || n.info == nil {
		return ""
	}
	return n.info.MasterPort
}

// Setup only return the last command error
func (n *RedisNode) Setup(ctx context.Context, margs ...[]any) (err error) {
	if n == nil {
		return nil
	}

	redisCli, err := n.getRedisConnect(ctx, n)
	if err != nil {
		return err
	}
	defer redisCli.Close()

	for _, args := range margs {
		if len(args) == 0 {
			continue
		}
		cmd, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("the command must be string")
		}

		func() {
			ctx, cancel := context.WithTimeout(ctx, time.Second*10)
			defer cancel()

			if _, err = redisCli.Do(ctx, cmd, args[1:]...); err != nil {
				// ignore forget nodes error
				n.logger.Error(err, "set config failed", "target", n.GetName(), "address", n.DefaultInternalIP().String(), "port", n.InternalPort(), "cmd", cmd)
			}
		}()
	}
	return
}

func (n *RedisNode) SetACLUser(ctx context.Context, username string, passwords []string, rules string) (interface{}, error) {
	if n == nil {
		return nil, nil
	}

	redisCli, err := n.getRedisConnect(ctx, n)
	if err != nil {
		return nil, err
	}
	defer redisCli.Close()
	cmds := [][]interface{}{{"ACL", "SETUSER", username, "reset"}}
	for _, password := range passwords {
		cmds = append(cmds, []interface{}{"ACL", "SETUSER", username, ">" + password})
	}
	if len(passwords) == 0 {
		cmds = append(cmds, []interface{}{"ACL", "SETUSER", username, "nopass"})
	}

	rule_slice := []string{"ACL", "SETUSER", username}
	rule_slice = append(rule_slice, strings.Split(rules, " ")...)
	interfaceSlice := make([]interface{}, len(rule_slice))
	for i, v := range rule_slice {
		interfaceSlice[i] = v
	}
	cmds = append(cmds, interfaceSlice)
	cmds = append(cmds, []interface{}{"ACL", "SETUSER", username, "on"})
	cmds = append(cmds, []interface{}{"ACL", "LIST"})
	cmd_list := []string{}
	args_list := [][]interface{}{}
	for _, cmd := range cmds {
		_cmd, _ := cmd[0].(string)
		cmd_list = append(cmd_list, _cmd)
		args_list = append(args_list, cmd[1:])
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()
	results, err := redisCli.Pipelining(ctx, cmd_list, args_list)
	if err != nil {
		return nil, err
	}

	result_list, err := rediscli.Values(results, err)
	if err != nil {
		return results, err
	}
	acl_list := result_list[len(result_list)-1]
	switch acls := acl_list.(type) {
	case []interface{}:
		interfaceSlice := []interface{}{}
		for _, v := range acls {
			r, _err := rediscli.String(v, err)
			if _err != nil {
				return acl_list, _err
			}
			interfaceSlice = append(interfaceSlice, r)
		}
		return interfaceSlice, nil
	default:
		return acl_list, fmt.Errorf("acl list failed")
	}
}

func (n *RedisNode) Query(ctx context.Context, cmd string, args ...any) (any, error) {
	if n == nil {
		return nil, nil
	}

	redisCli, err := n.getRedisConnect(ctx, n)
	if err != nil {
		return nil, err
	}
	defer redisCli.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()

	return redisCli.Do(ctx, cmd, args...)
}

func (n *RedisNode) Info() rediscli.RedisInfo {
	if n == nil || n.info == nil {
		return rediscli.RedisInfo{}
	}
	return *n.info
}

func (n *RedisNode) Port() int {
	port := 6379
	if container := util.GetContainerByName(&n.Pod.Spec, clusterbuilder.ServerContainerName); container != nil {
		for _, p := range container.Ports {
			if p.Name == clusterbuilder.RedisDataContainerPortName {
				port = int(p.ContainerPort)
				break
			}
		}
	}
	if n.IsSentinelPod() {
		if container := util.GetContainerByName(&n.Pod.Spec, sentinelbuilder.SentinelContainerName); container != nil {
			for _, p := range container.Ports {
				if p.Name == sentinelbuilder.SentinelContainerPortName {
					port = int(p.ContainerPort)
					break
				}
			}
		}
	}
	if value, ok := n.Pod.Labels["middleware.alauda.io/announce_port"]; ok {
		if value != "" {
			_port, err := strconv.Atoi(value)
			if err == nil {
				port = _port
			}
		}
	}
	return port
}

func (n *RedisNode) InternalPort() int {
	port := 6379
	if container := util.GetContainerByName(&n.Pod.Spec, clusterbuilder.ServerContainerName); container != nil {
		for _, p := range container.Ports {
			if p.Name == clusterbuilder.RedisDataContainerPortName {
				port = int(p.ContainerPort)
				break
			}
		}
	}
	if container := util.GetContainerByName(&n.Pod.Spec, sentinelbuilder.SentinelContainerName); container != nil {
		for _, p := range container.Ports {
			if p.Name == sentinelbuilder.SentinelContainerPortName {
				port = int(p.ContainerPort)
				break
			}
		}
	}

	return port
}

func (n *RedisNode) DefaultIP() net.IP {
	if value := n.Pod.Labels["middleware.alauda.io/announce_ip"]; value != "" {
		address := strings.Replace(value, "-", ":", -1)
		return net.ParseIP(address)
	}
	ips := n.IPs()
	if len(ips) > 0 {
		return ips[0]
	}
	return nil
}

func (n *RedisNode) DefaultInternalIP() net.IP {
	ips := n.IPs()
	if len(ips) == 0 {
		return nil
	}

	var ipFamilyPrefer string
	if container := util.GetContainerByName(&n.Pod.Spec, clusterbuilder.ServerContainerName); container != nil {
		for _, env := range container.Env {
			if env.Name == "IP_FAMILY_PREFER" {
				ipFamilyPrefer = env.Value
				break
			}
		}
	}
	if container := util.GetContainerByName(&n.Pod.Spec, sentinelbuilder.SentinelContainerName); container != nil {
		for _, env := range container.Env {
			if env.Name == "IP_FAMILY_PREFER" {
				ipFamilyPrefer = env.Value
				break
			}
		}
	}

	if ipFamilyPrefer != "" {
		for _, ip := range n.IPs() {
			addr, err := netip.ParseAddr(ip.String())
			if err != nil {
				continue
			}
			if addr.Is4() && ipFamilyPrefer == string(corev1.IPv4Protocol) ||
				addr.Is6() && ipFamilyPrefer == string(corev1.IPv6Protocol) {
				return ip
			}
		}
	}
	return ips[0]
}

func (n *RedisNode) IPort() int {
	if value := n.Pod.Labels["middleware.alauda.io/announce_iport"]; value != "" {
		port, err := strconv.Atoi(value)
		if err == nil {
			return port
		}
	}
	return n.InternalIPort()
}

func (n *RedisNode) InternalIPort() int {
	return n.InternalPort() + 10000
}

func (n *RedisNode) IPs() []net.IP {
	if n == nil {
		return nil
	}
	ips := []net.IP{}
	for _, podIp := range n.Pod.Status.PodIPs {
		ips = append(ips, net.ParseIP(podIp.IP))
	}
	return ips
}

func (n *RedisNode) GetPod() *corev1.Pod {
	return &n.Pod
}

func (n *RedisNode) NodeIP() net.IP {
	if n == nil {
		return nil
	}
	return net.ParseIP(n.Pod.Status.HostIP)
}

// ContainerStatus
func (n *RedisNode) ContainerStatus() *corev1.ContainerStatus {
	if n == nil {
		return nil
	}
	for _, status := range n.Pod.Status.ContainerStatuses {
		if status.Name == clusterbuilder.ServerContainerName {
			return &status
		}
	}
	return nil
}

// Status
func (n *RedisNode) Status() corev1.PodPhase {
	if n == nil {
		return corev1.PodUnknown
	}
	return n.Pod.Status.Phase
}

func (n *RedisNode) SetMonitor(ctx context.Context, ip, port, user, password, quorum string) error {

	if err := n.Setup(ctx, []interface{}{"sentinel", "remove", "mymaster"}); err != nil {
		n.logger.Info("try remove mymaster failed", "err", err.Error())
	}

	n.logger.Info("set monitor", "ip", ip, "port", port, "quorum", quorum)
	if err := n.Setup(ctx, []interface{}{"sentinel", "monitor", "mymaster", ip, port, quorum}); err != nil {
		return err
	}
	if password != "" {
		if n.CurrentVersion().IsACLSupported() {
			if user != "" {
				if err := n.Setup(ctx, []interface{}{"sentinel", "set", "mymaster", "auth-user", user}); err != nil {
					return err
				}
			}
		}
		if err := n.Setup(ctx, []interface{}{"sentinel", "set", "mymaster", "auth-pass", password}); err != nil {
			return err
		}
		//reset
		if err := n.Setup(ctx, []interface{}{"sentinel", "reset", "mymaster"}); err != nil {
			return err
		}
	}

	return nil
}

func (n *RedisNode) ReplicaOf(ctx context.Context, ip, port string) error {
	if n.DefaultIP().String() == ip && strconv.Itoa(n.Port()) == port {
		return nil
	}
	if n.Info().MasterHost == ip && n.Info().MasterPort == port && n.Info().MasterLinkStatus == "up" {
		return nil
	}
	if err := n.Setup(ctx, []interface{}{"slaveof", ip, port}); err != nil {
		return err
	}
	return nil
}
