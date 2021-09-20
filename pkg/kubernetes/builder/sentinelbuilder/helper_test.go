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

package sentinelbuilder

import (
	"reflect"
	"testing"
)

func TestMergeMap(t *testing.T) {
	t.Run("MergeMap with empty maps", func(t *testing.T) {
		// Test merging empty maps
		merged := MergeMap()
		expected := map[string]string{}
		if !reflect.DeepEqual(merged, expected) {
			t.Errorf("MergeMap() = %v, want %v", merged, expected)
		}
	})

	t.Run("MergeMap with non-empty maps", func(t *testing.T) {
		// Test merging non-empty maps
		map1 := map[string]string{"a": "1", "b": "2"}
		map2 := map[string]string{"b": "3", "c": "4"}
		map3 := map[string]string{"d": "5"}
		map3 = nil

		merged := MergeMap(map1, map2, map3)
		expected := map[string]string{"a": "1", "b": "3", "c": "4"}

		if !reflect.DeepEqual(merged, expected) {
			t.Errorf("MergeMap() = %v, want %v", merged, expected)
		}
	})
}
