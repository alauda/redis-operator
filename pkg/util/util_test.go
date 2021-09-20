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

package util

import (
	"fmt"
	"testing"

	"github.com/alauda/redis-operator/pkg/types/slot"
)

func TestCompareStrings(t *testing.T) {
	tests := []struct {
		a      string
		b      string
		result int
	}{
		{"rfr-s-v6-0", "rfr-s-v6-1", -1},
		{"rfr-s-v6-2", "rfr-s-v6-1", 1},
		{"rfr-s-v6-10", "rfr-s-v6-10", 0},
		{"rfr-s-v6-11", "rfr-s-v6-0", 1},
		{"rfr-s-v6-15", "rfr-s-v6-5", 1},
		{"", "rfr-s-v6-0", -1}, // 测试空字符串
		{"rfr-s-v6-0", "", 1},  // 测试空字符串
		{"", "", 0},            // 测试两个空字符串
	}

	for _, test := range tests {
		res := CompareStrings(test.a, test.b)
		if res != test.result {
			t.Errorf("CompareStrings(%s, %s) expected %d, got %d", test.a, test.b, test.result, res)
		}
	}
}

func TestParsePortSequence(t *testing.T) {
	testCases := []struct {
		name          string
		portSequence  string
		expectedPorts []int32
		expectedError error
	}{
		{
			name:          "Basic range",
			portSequence:  "1-3",
			expectedPorts: []int32{1, 2, 3},
			expectedError: nil,
		},
		{
			name:          "Single value",
			portSequence:  "5",
			expectedPorts: []int32{5},
			expectedError: nil,
		},
		{
			name:          "Mixed ranges and single values",
			portSequence:  "3,4-6,7,9-10",
			expectedPorts: []int32{3, 4, 5, 6, 7, 9, 10},
			expectedError: nil,
		},
		{
			name:          "Invalid format",
			portSequence:  "4-6,7-",
			expectedPorts: nil,
			expectedError: fmt.Errorf("strconv.Atoi: parsing \"\": invalid syntax"),
		},
		{
			name:          "Unsorted and overlapping",
			portSequence:  "9-10,7,4-6,3,5-8",
			expectedPorts: []int32{3, 4, 5, 6, 7, 8, 9, 10},
			expectedError: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ports, err := ParsePortSequence(tc.portSequence)

			if tc.expectedError != nil && err == nil {
				t.Errorf("Expected error, got nil")
			}

			if tc.expectedError == nil && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tc.expectedError != nil && err != nil && tc.expectedError.Error() != err.Error() {
				t.Errorf("Expected error '%v', got '%v'", tc.expectedError, err)
			}

			if len(tc.expectedPorts) != len(ports) {
				t.Errorf("Expected ports length %v, got %v", len(tc.expectedPorts), len(ports))
			}

			for i, port := range tc.expectedPorts {
				if port != ports[i] {
					t.Errorf("Expected port %v at position %v, got %v", port, i, ports[i])
				}
			}
		})
	}
}

func TestSlot(t *testing.T) {
	// for update validator, only check slots fullfilled
	var (
		fullSlots *slot.Slots
		total     int
	)
	for _, shard := range []struct{ Slots string }{
		{"0-6000"},
		{"5001-10000"},
		{"10001-16383"},
	} {
		if shardSlots, err := slot.LoadSlots(shard.Slots); err != nil {
			t.Errorf("failed to load shard slots: %v", err)
		} else {
			fullSlots = fullSlots.Union(shardSlots)
			total += shardSlots.Count(slot.SlotAssigned)
		}
	}
	if !fullSlots.IsFullfilled() {
		t.Errorf("specified shard slots not fullfilled all slots")
	}
	if total > 16384 {
		t.Errorf("specified shard slots duplicated")
	}
}

func TestCheckRule(t *testing.T) {
	tests := []struct {
		input    string
		expected error
	}{
		{"allkeys +@example +@keyspace +@read", fmt.Errorf("acl rule group example is not allowed")},
		{"allkeys -@write +@write +@geo +@pubsub", fmt.Errorf("acl rule group write is duplicated")},
		{"allkeys +@list +@hash -@invalid", fmt.Errorf("acl rule group invalid is not allowed")},
		{"allkeys +@keyspace +@read +@write +cluster|info", nil},
		{"allkeys +@keyspace +@read +@write +cluster|info on", fmt.Errorf("acl rule on is not allowed")},
		{"~* +@all -keys", nil},
		{"~* dsada", fmt.Errorf("acl rule dsada is not allowed")},
		{"~* >dsada", fmt.Errorf("acl password rule >dsada is not allowed")},
		{"~* <dsada", fmt.Errorf("acl password rule <dsada is not allowed")},
		{"allkeys ~test +@all -acl -flushall -flushdb -keys", nil},
		{"allkeys ~test +@all $sd -flushall -flushdb -keys", fmt.Errorf("acl rule $sd is not allowed")},
	}

	for _, test := range tests {
		err := CheckRule(test.input)
		if (err == nil && test.expected != nil) || (err != nil && test.expected == nil) || (err != nil && err.Error() != test.expected.Error()) {
			t.Errorf("For input '%s', expected error: %v, got: %v", test.input, test.expected, err)
		}
	}
}

func TestCheckUserRuleUpdate(t *testing.T) {
	tests := []struct {
		name    string
		rule    string
		wantErr bool
	}{
		{
			name:    "Test with +acl rule",
			rule:    "+acl",
			wantErr: true,
		},
		{
			name:    "Test with +@slow rule and no -acl",
			rule:    "+@slow",
			wantErr: true,
		},
		{
			name:    "Test with valid rule",
			rule:    "-acl +@read",
			wantErr: false,
		},
		{
			name:    "Test with dup acl rule",
			rule:    "+acl -acl +acl",
			wantErr: true,
		},
		{name: "exampele 1",
			rule:    "allkeys +@all -@dangerous",
			wantErr: false,
		},
		{name: "example 2",
			rule:    "allkeys -@all +@write +@read -@dangerous",
			wantErr: false,
		},
		{
			name:    "example 3",
			rule:    "allkeys -@all +@read -keys",
			wantErr: false,
		},
		{
			name:    "example 4",
			rule:    "allkeys +@all -acl",
			wantErr: false,
		},
		{
			name:    "default",
			rule:    "allkeys +@all -acl -flushall -flushdb -keys",
			wantErr: false,
		},
		{name: "acl",
			rule:    "+acl",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckUserRuleUpdate(tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckDefaultUserRuleUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
