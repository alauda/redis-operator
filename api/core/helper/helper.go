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
	"net/netip"
	"sort"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
)

// ParseSequencePorts
func ParseSequencePorts(portSequence string) ([]int32, error) {
	portRanges := strings.Split(portSequence, ",")
	portMap := make(map[int32]struct{})

	for _, portRange := range portRanges {
		portRangeParts := strings.Split(portRange, "-")

		if len(portRangeParts) == 1 {
			port, err := strconv.ParseInt(portRangeParts[0], 10, 32)
			if err != nil {
				return nil, err
			}
			portMap[int32(port)] = struct{}{}
		} else if len(portRangeParts) == 2 {
			start, err := strconv.Atoi(portRangeParts[0])
			if err != nil {
				return nil, err
			}
			end, err := strconv.Atoi(portRangeParts[1])
			if err != nil {
				return nil, err
			}

			for i := start; i <= end; i++ {
				portMap[int32(i)] = struct{}{}
			}
		} else {
			return nil, fmt.Errorf("invalid port range format: %s", portRange)
		}
	}

	ports := make([]int32, 0, len(portMap))
	for port := range portMap {
		ports = append(ports, port)
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })

	return ports, nil
}

func GetDefaultIPFamily(ip string) v1.IPFamily {
	if ip == "" {
		return ""
	}
	if addr, err := netip.ParseAddr(ip); err == nil {
		if addr.Is6() {
			return v1.IPv6Protocol
		}
	}
	return ""
}
