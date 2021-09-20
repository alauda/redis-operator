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

package v1alpha1

import "github.com/alauda/redis-operator/pkg/types/redis"

// DC settings
type DcOption struct {
	InitRole      redis.RedisRole `json:"initRole,omitempty"`
	InitADDRESS   []RedisAddress  `json:"initAddress,omitempty"`
	HealPolicy    HealPolicy      `json:"healPolicy,omitempty"`
	CheckerPolicy CheckerPolicy   `json:"checkerPolicy,omitempty"`
}

type CheckerPolicy struct {
	PfailNodeCnt int  `json:"pfailNodeCnt,omitempty"`
	Retry        int  `json:"retry,omitempty"`
	Enable       bool `json:"enable,omitempty"`
}

type RedisAddress struct {
	IP      string `json:"Ip,omitempty"`
	Port    string `json:"Port,omitempty"`
	BusPort string `json:"busPort,omitempty"`
}

type HealPolicy string

const (
	None              HealPolicy = "none"
	Normal            HealPolicy = "normal"
	Force             HealPolicy = "force"
	TakeOver          HealPolicy = "takeover"
	TakeOverAndForget HealPolicy = "takeoverAndForget"
)
