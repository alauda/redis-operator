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
	"reflect"
	"testing"

	"github.com/alauda/redis-operator/api/core"
	"k8s.io/utils/pointer"
)

func Test_parseShardIndex(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "drc-test-0-0",
			args: args{
				name: "drc-test-0-0",
			},
			want: 0,
		},
		{
			name: "drc-test-0-999",
			args: args{
				name: "drc-test-0-999",
			},
			want: 999,
		},
		{
			name: "rfr-test",
			args: args{
				name: "rfr-test",
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseShardIndex(tt.args.name); got != tt.want {
				t.Errorf("parseShardIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractShardDatasetUsedMemory(t *testing.T) {
	type args struct {
		name   string
		shards int
		nodes  []core.RedisDetailedNode
	}
	tests := []struct {
		name    string
		args    args
		wantRet []int64
	}{
		{
			name: "redis cluster name=test",
			args: args{
				name:   "test",
				shards: 3,
				nodes: []core.RedisDetailedNode{
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-1"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-1"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-2"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-2"}, UsedMemoryDataset: 110},
				},
			},
			wantRet: []int64{100, 100, 110},
		},
		{
			name: "redis cluster name=test-0",
			args: args{
				name:   "test-0",
				shards: 3,
				nodes: []core.RedisDetailedNode{
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-0"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-0"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-1"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-1"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-2"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-2"}, UsedMemoryDataset: 110},
				},
			},
			wantRet: []int64{100, 100, 110},
		},
		{
			name: "redis cluster with datasize different",
			args: args{
				name:   "test-0",
				shards: 3,
				nodes: []core.RedisDetailedNode{
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-0"}, UsedMemoryDataset: 10},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-0"}, UsedMemoryDataset: 110},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-1"}, UsedMemoryDataset: 0},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-1"}, UsedMemoryDataset: 0},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-2"}, UsedMemoryDataset: 0},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-2"}, UsedMemoryDataset: 130},
				},
			},
			wantRet: []int64{110, 0, 130},
		},
		{
			name: "redis cluster with not enough nodes",
			args: args{
				name:   "test-0",
				shards: 6,
				nodes: []core.RedisDetailedNode{
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-0"}, UsedMemoryDataset: 10},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-0"}, UsedMemoryDataset: 110},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-1"}, UsedMemoryDataset: 0},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-1"}, UsedMemoryDataset: 0},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-2"}, UsedMemoryDataset: 0},
					{RedisNode: core.RedisNode{StatefulSet: "drc-test-0-2"}, UsedMemoryDataset: 130},
				},
			},
			wantRet: []int64{110, 0, 130, 0, 0, 0},
		},
		{
			name: "redis sentinel name=test",
			args: args{
				name:   "test",
				shards: 1,
				nodes: []core.RedisDetailedNode{
					{RedisNode: core.RedisNode{StatefulSet: "rfr-test"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "rfr-test"}, UsedMemoryDataset: 110},
				},
			},
			wantRet: []int64{110},
		},
		{
			name: "redis sentinel name=test-0",
			args: args{
				name:   "test-0",
				shards: 1,
				nodes: []core.RedisDetailedNode{
					{RedisNode: core.RedisNode{StatefulSet: "rfr-test-0"}, UsedMemoryDataset: 100},
					{RedisNode: core.RedisNode{StatefulSet: "rfr-test-0"}, UsedMemoryDataset: 100},
				},
			},
			wantRet: []int64{100},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotRet := ExtractShardDatasetUsedMemory(tt.args.name, tt.args.shards, tt.args.nodes); !reflect.DeepEqual(gotRet, tt.wantRet) {
				t.Errorf("ExtractShardDatasetUsedMemory() = %v, want %v", gotRet, tt.wantRet)
			}
		})
	}
}

func TestBuildRenameCommand(t *testing.T) {
	type args struct {
		rawRename string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "empty",
			args: args{
				rawRename: "",
			},
			want: `flushall "" flushdb ""`,
		},
		{
			name: "with keys disabled",
			args: args{
				rawRename: `keys ""`,
			},
			want: `flushall "" flushdb "" keys ""`,
		},
		{
			name: "with keys renamed",
			args: args{
				rawRename: `keys "abc123"`,
			},
			want: `flushall "" flushdb "" keys "abc123"`,
		},
		{
			name: "with open flushall",
			args: args{
				rawRename: "flushall flushall",
			},
			want: `flushall flushall flushdb ""`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildRenameCommand(tt.args.rawRename); got != tt.want {
				t.Errorf("BuildRenameCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateNodeCount(t *testing.T) {
	type args struct {
		arch         core.Arch
		masterCount  *int32
		replicaCount *int32
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "redis cluster with replicas",
			args: args{
				arch:         core.RedisCluster,
				masterCount:  pointer.Int32(3),
				replicaCount: pointer.Int32(1),
			},
			want: 6,
		},
		{
			name: "redis cluster without replicas",
			args: args{
				arch:         core.RedisCluster,
				masterCount:  pointer.Int32(3),
				replicaCount: nil,
			},
			want: 3,
		},
		{
			name: "redis sentinel/standalone with replicas",
			args: args{
				arch:         core.RedisSentinel,
				masterCount:  pointer.Int32(1),
				replicaCount: pointer.Int32(2),
			},
			want: 3,
		},
		{
			name: "redis sentinel/standalone without replicas",
			args: args{
				arch:         core.RedisSentinel,
				masterCount:  pointer.Int32(1),
				replicaCount: nil,
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateNodeCount(tt.args.arch, tt.args.masterCount, tt.args.replicaCount); got != tt.want {
				t.Errorf("CalculateNodeCount() = %v, want %v", got, tt.want)
			}
		})
	}
}
