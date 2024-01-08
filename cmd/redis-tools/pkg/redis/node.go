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

package redis

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/types/slot"
)

const (
	MasterRole = "master"
	SlaveRole  = "slave"
)

type Node struct {
	ID          string
	Addr        string
	Flags       string
	Role        string
	MasterID    string
	PingSend    int64
	PingRecv    int64
	ConfigEpoch int64
	LinkState   string
	slots       []string
	rawInfo     string
	// slaves      []*Node
}

func (n *Node) IsSelf() bool {
	if n == nil {
		return false
	}
	return strings.HasPrefix(n.Flags, "myself")
}

func (n *Node) IsFailed() bool {
	if n == nil {
		return true
	}
	return strings.Contains(n.Flags, "fail")
}

func (n *Node) IsConnected() bool {
	if n == nil {
		return false
	}
	return n.LinkState == "connected"
}

func (n *Node) IsJoined() bool {
	if n == nil {
		return false
	}
	return n.Addr != ""
}

func (n *Node) Slots() *slot.Slots {
	if n == nil {
		return nil
	}
	if n == nil || n.Role == SlaveRole || len(n.slots) == 0 {
		return nil
	}

	slots := slot.NewSlots()
	_ = slots.Load(n.slots)

	return slots
}

func (n *Node) Raw() string {
	if n == nil {
		return ""
	}
	return n.rawInfo
}

type Nodes []*Node

func (ns Nodes) Get(id string) *Node {
	for _, n := range ns {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (ns Nodes) Self() *Node {
	for _, n := range ns {
		if n.IsSelf() {
			return n
		}
	}
	return nil
}

func (ns Nodes) Replicas(id string) (ret []*Node) {
	for _, n := range ns {
		if n.MasterID == id {
			ret = append(ret, n)
		}
	}
	return
}

func (ns Nodes) Masters() (ret []*Node) {
	for _, n := range ns {
		if n.Role == MasterRole && len(n.slots) > 0 {
			ret = append(ret, n)
		}
	}
	return
}

func (ns Nodes) Marshal() ([]byte, error) {
	data := []map[string]string{}
	for _, n := range ns {
		d := map[string]string{}
		d["id"] = n.ID
		d["addr"] = n.Addr
		d["flags"] = n.Flags
		d["role"] = n.Role
		d["master_id"] = n.MasterID
		d["ping_send"] = strconv.FormatInt(n.PingSend, 10)
		d["ping_recv"] = strconv.FormatInt(n.PingRecv, 10)
		d["config_epoch"] = strconv.FormatInt(n.ConfigEpoch, 10)
		d["link_state"] = n.LinkState
		d["slots"] = strings.Join(n.slots, ",")
		data = append(data, d)
	}
	return json.Marshal(data)
}

var (
	invalidAddrReg = regexp.MustCompile(`^:\d+$`)
)

// parseNodes
//
// format:
//
//	<id> <ip:port@cport> <flags> <master> <ping-sent> <pong-recv> <config-epoch> <link-state> <slot> <slot> ... <slot>
func ParseNodes(data string) (nodes Nodes) {
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "vars") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		addrPair := strings.SplitN(fields[1], "@", 2)
		if len(addrPair) != 2 {
			continue
		}
		addr := addrPair[0]
		if invalidAddrReg.MatchString(addr) {
			addr = ""
		}
		role := fields[2]
		if strings.Contains(fields[2], SlaveRole) {
			role = SlaveRole
		} else if strings.Contains(fields[2], MasterRole) {
			role = MasterRole
		}
		pingSend, _ := strconv.ParseInt(fields[4], 10, 64)
		pongRecv, _ := strconv.ParseInt(fields[5], 10, 64)
		epoch, _ := strconv.ParseInt(fields[6], 10, 64)

		node := &Node{
			ID:          fields[0],
			Addr:        addr,
			Flags:       fields[2],
			Role:        role,
			MasterID:    strings.TrimPrefix(fields[3], "-"),
			PingSend:    pingSend,
			PingRecv:    pongRecv,
			ConfigEpoch: epoch,
			LinkState:   fields[7],
			slots:       fields[8:],
			rawInfo:     line,
		}
		nodes = append(nodes, node)
	}
	return
}
