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

package helper

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alauda/redis-operator/api/core"
	"github.com/samber/lo"
)

var (
	shardIndexReg = regexp.MustCompile(`^.*-(\d+)$`)
)

func parseShardIndex(name string) int {
	if shardIndexReg.MatchString(name) {
		matches := shardIndexReg.FindStringSubmatch(name)
		if len(matches) == 2 {
			val, _ := strconv.ParseInt(matches[1], 10, 32)
			return int(val)
		}
	}
	return 0
}

// ExtractShardDatasetUsedMemory extract the used memory dataset from the shard ordered by index
func ExtractShardDatasetUsedMemory(name string, shards int, nodes []core.RedisDetailedNode) (ret []int64) {
	if shards == 0 {
		return
	}
	data := map[int]int64{}
	for _, node := range nodes {
		name := strings.ReplaceAll(node.StatefulSet, "-"+name, "")
		index := parseShardIndex(name)
		if val, ok := data[index]; ok {
			data[index] = lo.Max([]int64{val, node.UsedMemoryDataset})
		} else {
			data[index] = node.UsedMemoryDataset
		}
	}

	ret = make([]int64, lo.Max([]int{len(data), shards}))
	for index, val := range data {
		ret[index] = val
	}
	return
}

func BuildRenameCommand(rawRename string) string {
	// check restart config
	renameConfigs := map[string]string{
		"flushall": "",
		"flushdb":  "",
	}
	if rawRename != "" {
		fields := strings.Fields(strings.ToLower(rawRename))
		if len(fields)%2 == 0 {
			for i := 0; i < len(fields); i += 2 {
				renameConfigs[fields[i]] = fields[i+1]
			}
		}
	}
	keys := []string{}
	for key := range renameConfigs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	renameVal := ""
	for _, key := range keys {
		val := renameConfigs[key]
		if val == "" {
			val = `""`
		}
		renameVal = strings.TrimSpace(fmt.Sprintf("%s %s %s", renameVal, key, val))
	}
	return renameVal
}

func CalculateNodeCount(arch core.Arch, masterCount *int32, replicaCount *int32) int {
	switch arch {
	case core.RedisCluster:
		if replicaCount != nil {
			return int(*masterCount) * int((*replicaCount)+1)
		}
		return int(*masterCount)
	case core.RedisSentinel:
		if replicaCount != nil {
			return int(*masterCount) + int(*replicaCount)
		}
		return int(*masterCount)
	case core.RedisStandalone:
		return 1
	default:
		return 0
	}
}
