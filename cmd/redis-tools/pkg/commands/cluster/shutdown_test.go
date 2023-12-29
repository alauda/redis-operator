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

import "testing"

func Test_parseStatefulsetIndex(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "drc-test-0-0",
			want: 0,
		},
		{
			name: "drc-test-abd-efef-efwe-1-19",
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseStatefulsetIndex(tt.name); got != tt.want {
				t.Errorf("parseStatefulsetIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}
