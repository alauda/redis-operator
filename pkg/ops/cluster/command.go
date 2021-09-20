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

import "github.com/alauda/redis-operator/pkg/actor"

type clusterCommand struct {
	typ string
}

func (c *clusterCommand) String() string {
	if c == nil {
		return ""
	}
	return c.typ
}

var (
	CommandUpdateAccount  actor.Command = &clusterCommand{typ: "CommandUpdateAccount"}
	CommandUpdateConfig                 = &clusterCommand{typ: "CommandUpdateConfig"}
	CommandEnsureResource               = &clusterCommand{typ: "CommandEnsureResource"}
	CommandHealPod                      = &clusterCommand{typ: "CommandHealPod"}
	CommandCleanResource                = &clusterCommand{typ: "CommandCleanResource"}

	CommandJoinNode    = &clusterCommand{typ: "CommandJoinNode"}
	CommandEnsureSlots = &clusterCommand{typ: "CommandEnsureSlots"}
	CommandRebalance   = &clusterCommand{typ: "CommandRebalance"}

	CommandRequeue = &clusterCommand{typ: "CommandRequeue"}
	CommandAbort   = &clusterCommand{typ: "CommandAbort"}
	CommandPaused  = &clusterCommand{typ: "CommandPaused"}
)
