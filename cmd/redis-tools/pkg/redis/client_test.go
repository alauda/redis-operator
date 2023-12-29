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

import "testing"

func TestIPv6ToURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "IPv6 with port",
			input:    "2001:0db8:85a3:0000:0000:8a2e:0370:7334:8080",
			expected: "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:8080",
		},
		{
			name:     "IPv4 address",
			input:    "192.168.1.1:8080",
			expected: "192.168.1.1:8080",
		},
		{
			name:     "IPv6 address with bracket",
			input:    "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:8080",
			expected: "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:8080",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := getAddress(test.input)
			if output != test.expected {
				t.Errorf("Expected %s, but got %s", test.expected, output)
			}
		})
	}
}
