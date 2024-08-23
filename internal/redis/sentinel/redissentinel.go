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
	"strconv"
	"strings"

	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/api/core/helper"
	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/builder"
	"github.com/alauda/redis-operator/internal/builder/sentinelbuilder"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ types.RedisSentinelInstance = (*RedisSentinel)(nil)

type RedisSentinel struct {
	databasesv1.RedisSentinel

	client        clientset.ClientSet
	eventRecorder record.EventRecorder

	replication *RedisSentinelReplication
	tlsConfig   *tls.Config
	users       acl.Users
	logger      logr.Logger
}

func NewRedisSentinel(ctx context.Context, cliset clientset.ClientSet, eventRecorder record.EventRecorder, def *databasesv1.RedisSentinel, logger logr.Logger) (*RedisSentinel, error) {
	if cliset == nil {
		return nil, fmt.Errorf("require clientset")
	}
	if def == nil {
		return nil, fmt.Errorf("require sentinel instance")
	}

	inst := RedisSentinel{
		RedisSentinel: *def,

		client:        cliset,
		eventRecorder: eventRecorder,
		logger:        logger.WithName("S").WithValues("instance", client.ObjectKeyFromObject(def).String()),
	}

	var err error
	if inst.tlsConfig, err = inst.loadTLS(ctx, inst.logger); err != nil {
		inst.logger.Error(err, "load tls config failed")
		return nil, err
	}
	if inst.users, err = inst.loadUsers(ctx, inst.logger); err != nil {
		inst.logger.Error(err, "load users failed")
		return nil, err
	}
	if inst.replication, err = NewRedisSentinelReplication(ctx, cliset, &inst, inst.logger); err != nil {
		inst.logger.Error(err, "create replication failed")
		return nil, err
	}
	return &inst, nil
}

func (s *RedisSentinel) loadUsers(ctx context.Context, logger logr.Logger) (acl.Users, error) {
	if s == nil {
		return nil, fmt.Errorf("nil sentinel instance")
	}
	passwordSecret := s.Definition().Spec.PasswordSecret
	if passwordSecret != "" {
		if secret, err := s.client.GetSecret(ctx, s.GetNamespace(), passwordSecret); err != nil {
			logger.Error(err, "load password secret failed", "name", passwordSecret)
			return nil, err
		} else if u, err := user.NewSentinelUser("", user.RoleDeveloper, secret); err != nil {
			logger.Error(err, "create user failed", "name", passwordSecret)
			return nil, err
		} else {
			return acl.Users{u}, nil
		}
	}
	u, _ := user.NewSentinelUser("", user.RoleDeveloper, nil)
	// return default user with out password
	return acl.Users{u}, nil
}

func (s *RedisSentinel) loadTLS(ctx context.Context, logger logr.Logger) (*tls.Config, error) {
	if s == nil {
		return nil, fmt.Errorf("nil sentinel instance")
	}

	if !s.Spec.EnableTLS || s.Status.TLSSecret == "" {
		return nil, nil
	}
	secretName := s.Status.TLSSecret

	if secret, err := s.client.GetSecret(ctx, s.GetNamespace(), secretName); err != nil {
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

func (s *RedisSentinel) NamespacedName() client.ObjectKey {
	if s == nil {
		return client.ObjectKey{}
	}
	return client.ObjectKey{Namespace: s.GetNamespace(), Name: s.GetName()}
}

func (s *RedisSentinel) Arch() redis.RedisArch {
	return core.RedisStdSentinel
}

func (s *RedisSentinel) Version() redis.RedisVersion {
	if s == nil || s.Spec.Image == "" {
		return redis.RedisVersionUnknown
	}
	ver, _ := redis.ParseRedisVersionFromImage(s.Spec.Image)
	return ver
}

func (s *RedisSentinel) Definition() *databasesv1.RedisSentinel {
	if s == nil {
		return nil
	}
	return &s.RedisSentinel
}

// TODO: implement acl when redis 5.0 deprecated and import redis 8 or valkey 8
func (s *RedisSentinel) Users() acl.Users {
	return nil
}

func (s *RedisSentinel) Replication() types.RedisSentinelReplication {
	return s.replication
}

func (s *RedisSentinel) TLSConfig() *tls.Config {
	if s == nil {
		return nil
	}
	return s.tlsConfig
}

func (s *RedisSentinel) Nodes() []redis.RedisSentinelNode {
	if s == nil || s.replication == nil {
		return nil
	}
	return s.replication.Nodes()
}

func (s *RedisSentinel) RawNodes(ctx context.Context) ([]corev1.Pod, error) {
	if s == nil {
		return nil, nil
	}
	// get pods according to statefulset
	name := sentinelbuilder.GetSentinelStatefulSetName(s.GetName())
	sts, err := s.client.GetStatefulSet(ctx, s.GetNamespace(), name)
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		s.logger.Info("load statefulset failed", "name", s.GetNamespace())
		return nil, err
	}

	// load pods by statefulset selector
	ret, err := s.client.GetStatefulSetPodsByLabels(ctx, sts.GetNamespace(), sts.Spec.Selector.MatchLabels)
	if err != nil {
		s.logger.Error(err, "loads pods of sentinel statefulset failed")
		return nil, err
	}
	return ret.Items, nil
}

func (s *RedisSentinel) Selector() map[string]string {
	// TODO: delete this method
	return sentinelbuilder.GenerateSelectorLabels("sentinel", s.GetName())
}

func (s *RedisSentinel) Restart(ctx context.Context, annotationKeyVal ...string) error {
	// update all shards
	logger := s.logger.WithName("Restart")

	if s.replication != nil {
		logger.V(3).Info("restart replication", "target", s.replication.NamespacedName)
		return s.replication.Restart(ctx, annotationKeyVal...)
	}
	return nil
}

func (s *RedisSentinel) Refresh(ctx context.Context) error {
	logger := s.logger.WithName("Refresh")

	var err error
	if s.replication != nil {
		logger.V(3).Info("refresh replication", "target", s.replication.NamespacedName)
		if err = s.replication.Refresh(ctx); err != nil {
			logger.Error(err, "refresh replication failed")
			return err
		}
	}
	return nil
}

func (s *RedisSentinel) IsReady() bool {
	if s == nil || s.replication == nil {
		return false
	}
	return s.replication.IsReady()
}

func (s *RedisSentinel) IsInService() bool {
	if s == nil || s.replication == nil {
		return false
	}
	readyReplicas := s.replication.Status().ReadyReplicas
	return readyReplicas >= (s.Spec.Replicas/2)+1
}

func (s *RedisSentinel) IsResourceFullfilled(ctx context.Context) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("nil sentinel instance")
	}
	var (
		serviceKey = corev1.SchemeGroupVersion.WithKind("Service")
		stsKey     = appsv1.SchemeGroupVersion.WithKind("StatefulSet")
	)
	resources := map[schema.GroupVersionKind][]string{
		serviceKey: {
			sentinelbuilder.GetSentinelServiceName(s.GetName()),         // rfs-<name>
			sentinelbuilder.GetSentinelHeadlessServiceName(s.GetName()), // rfs-<name>-hl
		},
		stsKey: {
			sentinelbuilder.GetSentinelStatefulSetName(s.GetName()),
		},
	}
	if s.Spec.Expose.ServiceType == corev1.ServiceTypeLoadBalancer ||
		s.Spec.Expose.ServiceType == corev1.ServiceTypeNodePort {
		for i := 0; i < int(s.Spec.Replicas); i++ {
			svcName := sentinelbuilder.GetSentinelNodeServiceName(s.GetName(), i)
			resources[serviceKey] = append(resources[serviceKey], svcName)
		}
	}

	for gvk, names := range resources {
		for _, name := range names {
			var obj unstructured.Unstructured
			obj.SetGroupVersionKind(gvk)

			err := s.client.Client().Get(ctx, client.ObjectKey{Namespace: s.GetNamespace(), Name: name}, &obj)
			if errors.IsNotFound(err) {
				s.logger.V(3).Info("resource not found", "target", util.ObjectKey(s.GetNamespace(), name))
				return false, nil
			} else if err != nil {
				s.logger.Error(err, "get resource failed", "target", util.ObjectKey(s.GetNamespace(), name))
				return false, err
			}
			// if gvk == stsKey {
			// 	if replicas, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas"); err != nil {
			// 		s.logger.Error(err, "get service replicas failed", "target", util.ObjectKey(s.GetNamespace(), name))
			// 		return false, err
			// 	} else if found && replicas != int64(s.Spec.Replicas) {
			// 		s.logger.Info("@@@@@@@ found", "replicas", replicas, "s.Spec.Replicas", s.Spec.Replicas)
			// 		return false, nil
			// 	}
			// }
		}
	}
	return true, nil
}

func (s *RedisSentinel) GetPassword() (string, error) {
	if s == nil {
		return "", nil
	}
	if s.Spec.PasswordSecret == "" {
		return "", nil
	}
	secret, err := s.client.GetSecret(context.Background(), s.GetNamespace(), s.Spec.PasswordSecret)
	if err != nil {
		return "", err
	}
	return string(secret.Data["password"]), nil
}

func (s *RedisSentinel) UpdateStatus(ctx context.Context, st types.InstanceStatus, msg string) error {
	if s == nil {
		return fmt.Errorf("nil sentinel instance")
	}

	var (
		status = s.RedisSentinel.Status.DeepCopy()
		sen    = lo.IfF(s.replication != nil, func() *appsv1.StatefulSetStatus {
			return s.replication.Status()
		}).Else(nil)
		nodeports = map[int32]struct{}{}
	)
	switch st {
	case types.OK:
		status.Phase = databasesv1.SentinelReady
	case types.Fail:
		status.Phase = databasesv1.SentinelFail
	case types.Paused:
		status.Phase = databasesv1.SentinelPaused
	default:
		status.Phase = databasesv1.SentinelCreating
	}
	status.Message = msg

	status.Nodes = status.Nodes[:0]
	for _, node := range s.Nodes() {
		n := core.RedisNode{
			Role:        "Sentinel",
			IP:          node.DefaultIP().String(),
			Port:        fmt.Sprintf("%d", node.Port()),
			PodName:     node.GetName(),
			StatefulSet: s.replication.GetName(),
			NodeName:    node.NodeIP().String(),
		}
		status.Nodes = append(status.Nodes, n)
		if port := node.Definition().Labels[builder.PodAnnouncePortLabelKey]; port != "" {
			val, _ := strconv.ParseInt(port, 10, 32)
			nodeports[int32(val)] = struct{}{}
		}
	}

	phase, msg := func() (databasesv1.SentinelPhase, string) {
		// use passed status if provided
		if status.Phase == databasesv1.SentinelFail || status.Phase == databasesv1.SentinelPaused {
			return status.Phase, status.Message
		}

		// check creating
		if sen == nil || sen.CurrentReplicas != s.Definition().Spec.Replicas ||
			sen.Replicas != s.Definition().Spec.Replicas {
			return databasesv1.SentinelCreating, ""
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
			return databasesv1.SentinelCreating, fmt.Sprintf("pods %s pending", strings.Join(pendingPods, ","))
		}

		// check nodeport applied
		if seq := s.Spec.Expose.NodePortSequence; s.Spec.Expose.ServiceType == corev1.ServiceTypeNodePort {
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
				return databasesv1.SentinelCreating, fmt.Sprintf("nodeport %s not applied", strings.Join(notAppliedPorts, ","))
			}
		}

		// make sure all is ready
		if sen != nil &&
			sen.ReadyReplicas == s.Definition().Spec.Replicas &&
			sen.CurrentReplicas == sen.ReadyReplicas &&
			sen.CurrentRevision == sen.UpdateRevision {

			return databasesv1.SentinelReady, ""
		}

		var notReadyPods []string
		for _, node := range s.Nodes() {
			if !node.IsReady() {
				notReadyPods = append(notReadyPods, node.GetName())
			}
		}
		if len(notReadyPods) > 0 {
			return databasesv1.SentinelCreating, fmt.Sprintf("pods %s not ready", strings.Join(notReadyPods, ","))
		}
		return databasesv1.SentinelCreating, ""
	}()
	status.Phase, status.Message = phase, lo.If(msg == "", status.Message).Else(msg)

	// update status
	s.RedisSentinel.Status = *status
	if err := s.client.UpdateRedisSentinelStatus(ctx, &s.RedisSentinel); err != nil {
		s.logger.Error(err, "update RedisFailover status failed")
		return err
	}
	return nil
}

func (s *RedisSentinel) IsACLAppliedToAll() bool {
	// TODO: implement acl when redis 5.0 deprecated and import redis 8 or valkey 8
	return false
}

func (s *RedisSentinel) IsACLUserExists() bool {
	// TODO: implement acl when redis 5.0 deprecated and import redis 8 or valkey 8
	return false
}

func (s *RedisSentinel) IsACLApplied() bool {
	// TODO: implement acl when redis 5.0 deprecated and import redis 8 or valkey 8
	return false
}

func (c *RedisSentinel) Logger() logr.Logger {
	if c == nil {
		return logr.Discard()
	}
	return c.logger
}

func (c *RedisSentinel) SendEventf(eventtype, reason, messageFmt string, args ...interface{}) {
	if c == nil {
		return
	}
	c.eventRecorder.Eventf(c.Definition(), eventtype, reason, messageFmt, args...)
}
