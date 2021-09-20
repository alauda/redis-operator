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
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type ClusterNode struct {
	Id        string
	Addr      string
	RawFlag   string
	MasterId  string
	PingSend  int64
	PongRecv  int64
	Epoch     int64
	LinkState string
	Slots     []string

	Role string
}

func (n *ClusterNode) IsSelf() bool {
	if n == nil {
		return false
	}
	return strings.HasPrefix(n.RawFlag, "myself")
}

var (
	invalidAddrReg = regexp.MustCompile(`^:\d+$`)
)

func ParseNodeFromClusterNode(line string) (*ClusterNode, error) {
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return nil, fmt.Errorf("invalid node info %v", fields)
	}

	n := ClusterNode{Id: fields[0]}
	// if the node has join with other node, the addres is empty
	if index := strings.Index(fields[1], "@"); index > 0 {
		n.Addr = fields[1][0:index]
		if invalidAddrReg.MatchString(n.Addr) {
			n.Addr = ""
		}
	}

	n.RawFlag = fields[2]
	if fields[3] != "-" {
		n.MasterId = fields[3]
	}

	n.PingSend, _ = strconv.ParseInt(fields[4], 10, 64)
	n.PongRecv, _ = strconv.ParseInt(fields[5], 10, 64)
	n.Epoch, _ = strconv.ParseInt(fields[6], 10, 64)
	n.LinkState = fields[7]
	if len(fields) > 8 {
		// TODO: parse slots
		n.Slots = append(n.Slots, fields[8:]...)
	}
	return &n, nil
}

type ClusterNodes []*ClusterNode

func (ns ClusterNodes) Self() *ClusterNode {
	for _, n := range ns {
		if n.IsSelf() {
			return n
		}
	}
	return nil
}
