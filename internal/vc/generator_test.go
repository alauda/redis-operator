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

package vc

import (
	"reflect"
	"testing"

	"github.com/alauda/redis-operator/internal/config"
	vc "github.com/alauda/redis-operator/internal/vc/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_mergeImageVersion(t *testing.T) {
	version := "1.0.0"
	type args struct {
		oldIV *vc.ImageVersion
		newIV *vc.ImageVersion
	}
	tests := []struct {
		name string
		args args
		want *vc.ImageVersion
	}{
		{
			name: "merge image version",
			args: args{
				oldIV: &vc.ImageVersion{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							config.InstanceTypeKey: config.CoreComponentName,
							config.CRVersionKey:    version,
						},
						Name: GenerateRedisBundleImageVersion(version),
					},
					Spec: vc.ImageVersionSpec{
						CrVersion: version,
						Components: map[string]vc.Component{
							"redis": {
								CoreComponent: true,
								ComponentVersions: []vc.ComponentVersion{
									{
										Image:          "build-harbor.alauda.cn/middleware/redis",
										Tag:            "4.0-alpine.b2d19531",
										Version:        "4.0.0",
										DisplayVersion: "4.0",
									},
									{
										Image:          "build-harbor.alauda.cn/middleware/redis",
										Tag:            "5.0-alpine.b2d19531",
										Version:        "5.0.0",
										DisplayVersion: "5.0",
									},
								},
							},
							"redis-exporter": {
								ComponentVersions: []vc.ComponentVersion{
									{
										Image: "build-harbor.alauda.cn/middleware/oliver006/redis_exporter",
										Tag:   "v1.3.5-bb5bec2c",
									},
								},
							},
							"redis-tools": {
								ComponentVersions: []vc.ComponentVersion{
									{
										Image: "build-harbor.alauda.cn/middleware/redis-tools",
										Tag:   "v3.14.7",
									},
								},
							},
							"expose-pod": {
								ComponentVersions: []vc.ComponentVersion{
									{
										Image: "build-harbor.alauda.cn/middleware/expose-pod",
										Tag:   "v3.14.50",
									},
								},
							},
						},
					},
				},
				newIV: &vc.ImageVersion{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							config.InstanceTypeKey: config.CoreComponentName,
							config.CRVersionKey:    version,
						},
						Name: GenerateRedisBundleImageVersion(version),
					},
					Spec: vc.ImageVersionSpec{
						CrVersion: version,
						Components: map[string]vc.Component{
							"redis": {
								CoreComponent: true,
								ComponentVersions: []vc.ComponentVersion{
									{
										Image:          "build-harbor.alauda.cn/middleware/redis",
										Tag:            "5.0-alpine.b2d19531",
										Version:        "5.0.0",
										DisplayVersion: "5.0",
									},
									{
										Image:          "build-harbor.alauda.cn/middleware/redis",
										Tag:            "6.0-alpine.b2d19531",
										Version:        "6.0.0",
										DisplayVersion: "6.0",
									},
								},
							},
							"activeredis": {
								ComponentVersions: []vc.ComponentVersion{
									{
										Image:          "build-harbor.alauda.cn/middleware/redis",
										Tag:            "6.0-alpine.b2d19531",
										Version:        "6.0.0",
										DisplayVersion: "6.0",
									},
								},
							},
							"redis-exporter": {
								ComponentVersions: []vc.ComponentVersion{
									{
										Image: "build-harbor.alauda.cn/middleware/oliver006/redis_exporter",
										Tag:   "v1.3.5-bb5bec2c",
									},
								},
							},
							"redis-tools": {
								ComponentVersions: []vc.ComponentVersion{
									{
										Image: "build-harbor.alauda.cn/middleware/redis-tools",
										Tag:   "v3.14.7",
									},
								},
							},
						},
					},
				},
			},
			want: &vc.ImageVersion{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						config.InstanceTypeKey: config.CoreComponentName,
						config.CRVersionKey:    version,
					},
					Name: GenerateRedisBundleImageVersion(version),
				},
				Spec: vc.ImageVersionSpec{
					CrVersion: version,
					Components: map[string]vc.Component{
						"redis": {
							CoreComponent: true,
							ComponentVersions: []vc.ComponentVersion{
								{
									Image:          "build-harbor.alauda.cn/middleware/redis",
									Tag:            "5.0-alpine.b2d19531",
									Version:        "5.0.0",
									DisplayVersion: "5.0",
								},
								{
									Image:          "build-harbor.alauda.cn/middleware/redis",
									Tag:            "6.0-alpine.b2d19531",
									Version:        "6.0.0",
									DisplayVersion: "6.0",
								},
								{
									Image:          "build-harbor.alauda.cn/middleware/redis",
									Tag:            "4.0-alpine.b2d19531",
									Version:        "4.0.0",
									DisplayVersion: "4.0",
								},
							},
						},
						"activeredis": {
							ComponentVersions: []vc.ComponentVersion{
								{
									Image:          "build-harbor.alauda.cn/middleware/redis",
									Tag:            "6.0-alpine.b2d19531",
									Version:        "6.0.0",
									DisplayVersion: "6.0",
								},
							},
						},
						"redis-exporter": {
							ComponentVersions: []vc.ComponentVersion{
								{
									Image: "build-harbor.alauda.cn/middleware/oliver006/redis_exporter",
									Tag:   "v1.3.5-bb5bec2c",
								},
							},
						},
						"redis-tools": {
							ComponentVersions: []vc.ComponentVersion{
								{
									Image: "build-harbor.alauda.cn/middleware/redis-tools",
									Tag:   "v3.14.7",
								},
							},
						},
						"expose-pod": {
							ComponentVersions: []vc.ComponentVersion{
								{
									Image: "build-harbor.alauda.cn/middleware/expose-pod",
									Tag:   "v3.14.50",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeImageVersion(tt.args.oldIV, tt.args.newIV); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeImageVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
