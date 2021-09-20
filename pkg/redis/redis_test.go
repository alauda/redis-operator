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
	"testing"
)

func TestParseIPAndPort(t *testing.T) {
	testCases := []struct {
		input        string
		expectedIP   string
		expectedPort int
		expectError  bool
	}{
		{"192.168.1.1:8080", "192.168.1.1", 8080, false},
		{"1335::172:168:200:5d1:32428", "1335::172:168:200:5d1", 32428, false},
		{"invalid-ip:port", "", 0, true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			ip, port, err := parseIPAndPort(tc.input)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if ip.String() != tc.expectedIP {
					t.Errorf("Expected IP: %s, got: %s", tc.expectedIP, ip.String())
				}

				if port != tc.expectedPort {
					t.Errorf("Expected port: %d, got: %d", tc.expectedPort, port)
				}
			}
		})
	}
}
