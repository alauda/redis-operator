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
	"crypto/rand"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func GenerateRedisTLSOptions() string {
	return "--tls --cert /tls/tls.crt --key /tls/tls.key --cacert /tls/ca.crt"
}

func GenerateRedisPasswordOptions() string {
	return "--requirepass ${REDIS_PASSWORD} --masterauth ${REDIS_PASSWORD} --protected-mode yes "
}

func GenerateRedisRandPassword() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", buf)
}

func GenerateRedisRebuildAnnotation() map[string]string {
	m := make(map[string]string)
	m["middle.alauda.cn/rebuild"] = "true"
	return m
}

func GetEnvSentinelHost(name string) string {
	return "RFS_REDIS_SERVICE_HOST"
}

func GetEnvSentinelPort(name string) string {
	return "RFS_REDIS_SERVICE_PORT_SENTINEL"
}

// ExtractLastNumber 提取字符串中最后一个"-"之后的数字
func ExtractLastNumber(s string) int {
	parts := strings.Split(s, "-")
	lastPart := parts[len(parts)-1]

	num, err := strconv.Atoi(lastPart)
	if err != nil {
		return -1
	}

	return num
}

// CompareStrings 比较两个字符串，返回 -1, 0, 1
func CompareStrings(a, b string) int {
	aNum, bNum := ExtractLastNumber(a), ExtractLastNumber(b)

	if aNum < bNum {
		return -1
	} else if aNum > bNum {
		return 1
	}
	return 0
}

func ParsePortSequence(portSequence string) ([]int32, error) {
	portRanges := strings.Split(portSequence, ",")
	portMap := make(map[int32]struct{})

	for _, portRange := range portRanges {
		portRangeParts := strings.Split(portRange, "-")

		if len(portRangeParts) == 1 {
			port, err := strconv.Atoi(portRangeParts[0])
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
