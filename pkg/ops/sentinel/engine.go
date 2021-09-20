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
	"fmt"

	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
)

type RuleEngine struct {
	client        kubernetes.ClientSet
	eventRecorder record.EventRecorder
	logger        logr.Logger
}

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

func (g *RuleEngine) Inspect(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	if g == nil {
		return nil
	}
	sentinel := val.(types.RedisFailoverInstance)

	logger := g.logger.WithName("Inspect").WithValues("namespace", sentinel.GetNamespace(), "name", sentinel.GetName())
	if sentinel == nil {
		logger.Info("failover is nil")
		return nil
	}
	cr := sentinel.Definition()

	if (cr.Spec.Redis.PodAnnotations != nil) && cr.Spec.Redis.PodAnnotations[config.PAUSE_ANNOTATION_KEY] != "" {
		return actor.NewResult(CommandEnsureResource)
	}
	logger.V(3).Info("Inspecting sentinel")

	check_password := func() *actor.ActorResult {
		logger.V(3).Info("check_password")
		return g.CheckRedisPasswordChange(ctx, sentinel)
	}

	if ret := check_password(); ret != nil {
		logger.V(3).Info("check_password", "result", ret)
		return ret
	}

	if len(sentinel.Nodes()) == 0 || len(sentinel.SentinelNodes()) == 0 {
		logger.Info("sentinel has no nodes,ensure resource")
		return actor.NewResult(CommandEnsureResource)
	}

	//first: check labels
	check_2 := func() *actor.ActorResult {
		logger.V(3).Info("check_labels")
		return g.CheckRedisLabels(ctx, sentinel)
	}
	if ret := check_2(); ret != nil {
		logger.V(3).Info("check_labels", "result", ret)
		return ret
	}

	// check only one master
	check_0 := func() *actor.ActorResult {
		logger.V(3).Info("check_only_one_master")
		return g.CheckMaster(ctx, sentinel)
	}
	if ret := check_0(); ret != nil {
		logger.V(3).Info("check_only_one_master", "result", ret)
		return ret
	}

	// check sentinel
	check_1 := func() *actor.ActorResult {
		logger.V(3).Info("check_sentinel")
		return g.CheckSentinel(ctx, sentinel)
	}
	if ret := check_1(); ret != nil {
		logger.V(3).Info("check_sentinel", "result", ret)
		return ret
	}

	check_configmap := func() *actor.ActorResult {
		logger.V(3).Info("check_configmap")
		return g.CheckRedisConfig(ctx, sentinel)
	}
	if ret := check_configmap(); ret != nil {
		logger.V(3).Info("check_configmap", "result", ret)
		return ret
	}

	check_finally := func() *actor.ActorResult {
		logger.V(3).Info("check_finnaly")
		return g.FinallyCheck(ctx, sentinel)
	}
	if ret := check_finally(); ret != nil {
		logger.V(3).Info("check_finnaly", "result", ret)
		return ret
	}

	logger.Info("sentinel inspect is healthy", "sentinel", sentinel.GetName(), "namespace", sentinel.GetNamespace())
	return nil

}

func (g *RuleEngine) CheckRedisLabels(ctx context.Context, st types.RedisInstance) *actor.ActorResult {
	if len(st.Masters()) != 1 {
		return nil
	}
	for _, v := range st.Nodes() {
		labels := v.Definition().GetLabels()
		if len(labels) > 0 {
			switch v.Role() {
			case redis.RedisRoleMaster:
				if labels[sentinelbuilder.RedisRoleLabel] != sentinelbuilder.RedisRoleMaster {
					g.logger.Info("master labels not match", "node", v.GetName(), "labels", labels)
					return actor.NewResult(CommandPatchLabels)
				}

			case redis.RedisRoleSlave:
				if labels[sentinelbuilder.RedisRoleLabel] != sentinelbuilder.RedisRoleSlave {
					g.logger.Info("slave labels not match", "node", v.GetName(), "labels", labels)
					return actor.NewResult(CommandPatchLabels)
				}
			}
		}
	}
	return nil
}

// 检测是否只有一个master
func (g *RuleEngine) CheckMaster(ctx context.Context, st types.RedisFailoverInstance) *actor.ActorResult {
	if len(st.Nodes()) == 0 {
		return actor.NewResult(CommandEnsureResource)
	}

	if len(st.Masters()) == 0 {

		return actor.NewResult(CommandInitMaster)
	}
	// 如果脑裂根据sentienl 选最优
	if len(st.Masters()) > 1 {
		g.logger.Info("master more than one", "sentinel", st.GetName())
		return actor.NewResultWithError(CommandHealMaster, fmt.Errorf("sentinel %s has more than one master", st.GetName()))
	}
	// 节点掉了，根据sentinel选最优
	for _, n := range st.Nodes() {
		if n.Info().MasterLinkStatus == "down" && n.Role() == redis.RedisRoleSlave {
			g.logger.Info("master link down", "sentinel", st.GetName(), "node", n.GetName())
			return actor.NewResult(CommandHealMaster)
		}

	}
	return nil
}

func (g *RuleEngine) CheckSentinel(ctx context.Context, st types.RedisFailoverInstance) *actor.ActorResult {
	rf := st.Definition()
	if len(st.SentinelNodes()) == 0 {
		return actor.NewResult(CommandEnsureResource)
	}
	for _, v := range st.SentinelNodes() {
		if v.Info().SentinelMaster0.Status != "ok" {
			g.logger.Info("sentinel status not ok", "sentinel", v.GetName(), "status", v.Info().SentinelMaster0.Status)
			return actor.NewResult(CommandSentinelHeal)
		}
		if v.Info().SentinelMaster0.Replicas != len(st.Nodes())-1 {
			g.logger.Info("sentinel slaves not match", "sentinel", v.GetName(), "slaves", v.Info().SentinelMaster0.Replicas, "nodes", rf.Spec.Redis.Replicas)
			return actor.NewResult(CommandHealMaster)
		}
	}
	return nil
}

func (g *RuleEngine) CheckRedisPasswordChange(ctx context.Context, st types.RedisFailoverInstance) *actor.ActorResult {
	if st.Version().IsACLSupported() && !st.IsACLUserExists() {
		return actor.NewResult(CommandUpdateAccount)
	}
	for _, v := range st.Users() {
		if v.Name == user.DefaultUserName {
			if v.Password == nil && st.Definition().Spec.Auth.SecretPath != "" {
				return actor.NewResult(CommandUpdateAccount)
			}
			if v.Password != nil && v.Password.SecretName != st.Definition().Spec.Auth.SecretPath {
				return actor.NewResult(CommandUpdateAccount)
			}
		}
		if v.Name == user.DefaultOperatorUserName {
			for _, vv := range st.Nodes() {
				container := util.GetContainerByName(&vv.Definition().Spec, sentinelbuilder.ServerContainerName)
				if container != nil && container.Env != nil {
					for _, vc := range container.Env {
						if vc.Name == sentinelbuilder.OperatorSecretName && vc.ValueFrom != nil {
							if vc.ValueFrom.SecretKeyRef.Name != st.Definition().Spec.Auth.SecretPath {
								return actor.NewResult(CommandUpdateAccount)
							}
						}
					}

				}
			}
			if len(v.Rules) != 0 && len(v.Rules[0].Channels) == 0 && st.Version().IsACL2Supported() {
				return actor.NewResult(CommandUpdateAccount)
			}
		}
	}
	return nil
}

func (g *RuleEngine) CheckRedisConfig(ctx context.Context, st types.RedisFailoverInstance) *actor.ActorResult {

	newCm := sentinelbuilder.NewRedisConfigMap(st, st.Selector())
	oldCm, err := g.client.GetConfigMap(ctx, newCm.GetNamespace(), newCm.GetName())
	if errors.IsNotFound(err) || oldCm.Data[clusterbuilder.RedisConfKey] == "" {
		return actor.NewResultWithError(CommandEnsureResource, fmt.Errorf("configmap %s not found", newCm.GetName()))
	} else if err != nil {
		return actor.NewResultWithError(CommandRequeue, err)
	}
	newConf, _ := clusterbuilder.LoadRedisConfig(newCm.Data[clusterbuilder.RedisConfKey])
	oldConf, _ := clusterbuilder.LoadRedisConfig(oldCm.Data[clusterbuilder.RedisConfKey])
	added, changed, deleted := oldConf.Diff(newConf)
	if len(added)+len(changed)+len(deleted) != 0 {
		return actor.NewResult(CommandUpdateConfig)
	}
	return nil
}

// 最后比对数量是否相同
func (g *RuleEngine) FinallyCheck(ctx context.Context, st types.RedisFailoverInstance) *actor.ActorResult {
	rf := st.Definition()
	for _, n := range st.Nodes() {
		if n.Role() == redis.RedisRoleMaster && n.Info().ConnectedReplicas != int64(st.Definition().Spec.Redis.Replicas-1) {
			g.logger.Info("master slaves not match", "sentinel", st.GetName(), "slaves", n.Info().ConnectedReplicas, "replicas", rf.Spec.Redis.Replicas)
			return actor.NewResult(CommandHealMaster)
		}
	}
	for _, v := range st.SentinelNodes() {
		if v.Info().SentinelMaster0.Sentinels != int(st.Definition().Spec.Sentinel.Replicas) {
			g.logger.Info("sentinel sentinels not match", "sentinel", v.GetName(), "sentinels", v.Info().SentinelMaster0.Sentinels, "replicas", rf.Spec.Sentinel.Replicas)
			return actor.NewResult(CommandSentinelHeal)
		}
	}

	return nil
}
