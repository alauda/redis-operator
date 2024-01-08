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

//This is used to rename the rdb node downloaded from redis-cluster using redis-cli

package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
)

type NodeInfo struct {
	Name         string    `json:"name,omitempty"`
	Host         string    `json:"host,omitempty"`
	Port         int64     `json:"port,omitempty"`
	Replicate    string    `json:"replicate,omitempty"`
	SLotsCount   int64     `json:"slots_count,omitempty"`
	Slots        [][]int64 `json:"slots,omitempty"`
	Flags        string    `json:"flags,omitempty"`
	CurrentEpoch int64     `json:"current_epoch,omitempty"`
}

func RenameCluster(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	dataDir := c.String("data-dir")
	filePath := path.Join(dataDir, "nodes.json")

	buff, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error(err, "read nodes.json err")
		return err
	}
	var dat []NodeInfo
	if err := json.Unmarshal(buff, &dat); err != nil {
		logger.Error(err, "json.Unmarshal nodes.json err")
		return err
	}
	filtered := []NodeInfo{}
	for _, v := range dat {
		if v.SLotsCount != 0 {
			filtered = append(filtered, v)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		iLen := len(filtered[i].Slots[len(filtered[i].Slots)-1])
		jLen := len(filtered[j].Slots[len(filtered[j].Slots)-1])
		iLast := filtered[i].Slots[len(filtered[i].Slots)-1][iLen-1]
		jLast := filtered[j].Slots[len(filtered[j].Slots)-1][jLen-1]
		return iLast < jLast
	})
	log.Println("Filtered file len: ", len(filtered))
	for i, node := range filtered {
		rdbFilename := fmt.Sprintf("redis-node-%s-%d-%s.rdb", node.Host, node.Port, node.Name)
		err := os.Rename(path.Join(dataDir, rdbFilename), path.Join(dataDir, fmt.Sprintf("%d.rdb", i)))
		if err != nil {
			logger.Error(err, "rename file err", "file", rdbFilename)
			return err
		}
		nodeFile := path.Join(dataDir, fmt.Sprintf("%d.node.conf", i))
		slotStr := ""
		for _, slot_range := range node.Slots {
			start := strconv.Itoa(int(slot_range[0]))
			if len(slot_range) > 1 {
				end := strconv.Itoa(int(slot_range[1]))
				slotStr += start + "-" + end + " "
			} else {
				slotStr += start + " "
			}

		}
		line := fmt.Sprintf(`%s %s:%d@%d myself,%s - 0 0 %d connected %s
vars currentEpoch %d lastVoteEpoch 0`, node.Name, "127.0.0.0", 6379, 16379, node.Flags, node.CurrentEpoch, slotStr, node.CurrentEpoch)
		err = os.WriteFile(nodeFile, []byte(line), 0666)
		if err != nil {
			logger.Error(err, "write node file err", "file", nodeFile)
			return err
		}
	}
	return nil
}
