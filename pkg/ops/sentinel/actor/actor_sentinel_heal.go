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
	"strconv"

	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/ops/sentinel"
	p_redis "github.com/alauda/redis-operator/pkg/redis"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/redis"
	"github.com/go-logr/logr"
)

var _ actor.Actor = (*actorSentinelHeal)(nil)

func NewSentinelHealActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorSentinelHeal{
		client: client,
		logger: logger,
	}
}

type actorSentinelHeal struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorSentinelHeal) SupportedCommands() []actor.Command {
	return []actor.Command{sentinel.CommandSentinelHeal}
}

func (a *actorSentinelHeal) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	st := val.(types.RedisFailoverInstance)
	a.logger.Info("sentinel heal", "sentinel", st.GetName())
	user := ""
	password := st.Users().GetDefaultUser().Password.String()
	if st.Version().IsACLSupported() && st.Users().GetOpUser() != nil {
		user = st.Users().GetOpUser().Name
		password = st.Users().GetOpUser().Password.String()
	}
	var replicateMaster redis.RedisNode
	if st.Sentinel() != nil && *st.Sentinel().Definition().Spec.Replicas != st.Definition().Spec.Sentinel.Replicas {
		return actor.NewResult(sentinel.CommandEnsureResource)
	}
	// 查看sentinel info中status正常的sentinel
	if len(st.Masters()) == 0 {
		a.logger.V(3).Info("no master found")
		return actor.NewResult(sentinel.CommandEnsureResource)
	} else {
		replicateMaster = st.Masters()[0]
	}
	sentinelMasterMap := map[p_redis.Address]int{}
	odown := 0
	sdown := 0
	for _, sen := range st.SentinelNodes() {
		if sen.Info().SentinelMaster0.Status == "ok" {
			sentinelMasterMap[sen.Info().SentinelMaster0.Address] += 1
		}
		if sen.Info().SentinelMaster0.Status == "odown" {
			odown += 1
		}
		if sen.Info().SentinelMaster0.Status == "sdown" {
			sdown += 1
		}
		//有多的节点是未知节点
		if (sen.Info().SentinelMaster0.Sentinels > int(st.Definition().Spec.Sentinel.Replicas)) ||
			(sen.Info().SentinelMaster0.Replicas > int(st.Definition().Spec.Redis.Replicas-1)) {
			a.logger.Info("sentinel has unknown node")
			if sen.CurrentVersion().IsACLSupported() {
				if err := sen.Setup(ctx, []interface{}{"sentinel", "set", "mymaster", "auth-user", user}); err != nil {
					a.logger.Info("set auth-user failed", "err", err.Error())
				}
				if err := sen.Setup(ctx, []interface{}{"sentinel", "set", "mymaster", "auth-password", password}); err != nil {
					a.logger.Info("set auth-user failed", "err", err.Error())
				}
			}
			if err := sen.Setup(ctx, []interface{}{"sentinel", "reset", "*"}); err != nil {
				a.logger.Info("reset sen failed", "err", err.Error())
			}

			if err := sen.Refresh(ctx); err != nil {
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
		}
	}

	//如果所有的节点认为客观下线，刷新所有节点
	if len(st.SentinelNodes()) == odown {
		for _, sen := range st.SentinelNodes() {
			if err := sen.Setup(ctx, []interface{}{"sentinel", "reset", "*"}); err != nil {
				a.logger.Info("odown reset sen failed", "err", err.Error())
			}
		}
	}
	//刷新 主观下线的节点
	if odown == 0 && sdown > 0 && len(st.Masters()) > 0 {
		for _, sen := range st.SentinelNodes() {
			if sen.Info().SentinelMaster0.Status == "sdown" {
				if err := sen.Setup(ctx, []interface{}{"sentinel", "reset", "*"}); err != nil {
					a.logger.Info("odown reset sen failed", "err", err.Error())
				}
			}
		}
	}

	a.logger.V(3).Info("sentinelMasterMap", "sentinelMasterMap", sentinelMasterMap)
	// 获取sentinelMasterMap 最多count的master address
	var maxAddress p_redis.Address
	var maxCount int
	for address, count := range sentinelMasterMap {
		if count > maxCount {
			maxCount = count
			maxAddress = address
		}
	}
	a.logger.V(3).Info("maxAddress", "maxAddress", maxAddress, "maxCount", maxCount)
	quorum := strconv.Itoa(int(st.Definition().Spec.Sentinel.Replicas/2 + 1))
	if replicateMaster.IsReady() { //master ready
		if maxCount < 1 {
			//redis 有master ,哨兵没有 master
			a.logger.Info("redis has master, sentinel has no master")
			for _, sen := range st.SentinelNodes() {
				//reset mymaster
				err := sen.SetMonitor(ctx, replicateMaster.DefaultIP().String(), strconv.Itoa(replicateMaster.Port()), user, password, quorum)
				if err != nil {
					a.logger.Error(err, "sentinel set monitor failed")
					return actor.NewResultWithError(sentinel.CommandRequeue, err)
				}

			}
		} else {
			// redis就绪，所有哨兵指向同一master
			for _, sen := range st.SentinelNodes() {
				if sen.Info().SentinelMaster0.Address != maxAddress {
					a.logger.Info("redis ready, all sentinel point to same master")
					err := sen.SetMonitor(ctx, maxAddress.Host(), strconv.Itoa(maxAddress.Port()), user, password, quorum)
					if err != nil {
						a.logger.Error(err, "sentinel set monitor failed")
						return actor.NewResultWithError(sentinel.CommandRequeue, err)
					}
				}
			}
		}
	}
	err := st.Refresh(ctx)
	if err != nil {
		a.logger.Error(err, "refresh sentinel failed")
		return actor.NewResultWithError(sentinel.CommandRequeue, err)
	}
	// 节点在statefulset 中，sentinel 指向最多的master
	masterInSts := false
	for _, node := range st.Nodes() {
		// maxAddress 在 node 中
		if node.DefaultIP().String() == maxAddress.Host() && node.Port() == maxAddress.Port() {
			masterInSts = true
			for _, sn := range st.SentinelNodes() {
				if sn.Info().SentinelMaster0.Address != maxAddress {
					a.logger.Info("sts has master, all sentinel point to same master")
					err := sn.SetMonitor(ctx, maxAddress.Host(), strconv.Itoa(maxAddress.Port()), user, password, quorum)
					if err != nil {
						a.logger.Error(err, "sentinel set monitor failed")
						return actor.NewResultWithError(sentinel.CommandRequeue, err)
					}
				}
			}
		}
	}
	if !masterInSts {
		a.logger.Info("master not in sts, sentinel set monitor to master")
		for _, sn := range st.SentinelNodes() {
			err := sn.SetMonitor(ctx, replicateMaster.DefaultIP().String(), strconv.Itoa(replicateMaster.Port()), user, password, quorum)
			if err != nil {
				a.logger.Error(err, "sentinel set monitor failed")
				return actor.NewResultWithError(sentinel.CommandRequeue, err)
			}
		}
	}

	return nil
}
