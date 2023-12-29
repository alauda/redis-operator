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

import "strings"

type ProxyInfo struct {
	UnknownRole int
	FailCount   int
}

func ParseProxyInfo(data string) ProxyInfo {
	// data to line
	proxyInfo := ProxyInfo{}
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sl := strings.SplitN(line, ":", 2)
		if len(sl) != 2 {
			continue
		}
		if sl[0] == "Role" && sl[1] == "unknown" {
			proxyInfo.UnknownRole++
		}
		if sl[0] == "CurrentIsFail" && sl[1] == "1" {
			proxyInfo.FailCount++
		}
	}
	return proxyInfo
}
