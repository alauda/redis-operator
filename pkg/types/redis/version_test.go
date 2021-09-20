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

func TestParseRedisVersion(t *testing.T) {
	type args struct {
		v string
	}
	tests := []struct {
		name    string
		args    args
		want    RedisVersion
		wantErr bool
	}{
		{
			name:    "patch version",
			args:    args{v: "6.0-alpine.000b26a0c3b6"},
			want:    RedisVersion6,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRedisVersion(tt.args.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRedisVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseRedisVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRedisVersionFromImage(t *testing.T) {
	type args struct {
		u string
	}
	tests := []struct {
		name    string
		args    args
		want    RedisVersion
		wantErr bool
	}{
		{
			name:    "4.0-alpine",
			args:    args{u: "build-harbor.alauda.cn/middleware/redis:4.0-alpine"},
			want:    RedisVersion4,
			wantErr: false,
		},
		{
			name:    "4.0-alpine.xxx",
			args:    args{u: "build-harbor.alauda.cn/middleware/redis:4.0-alpine.000b26a0c3b6"},
			want:    RedisVersion4,
			wantErr: false,
		},
		{
			name:    "5.0-alpine",
			args:    args{u: "build-harbor.alauda.cn/middleware/redis:5.0-alpine"},
			want:    RedisVersion5,
			wantErr: false,
		},
		{
			name:    "5.0-alpine.xxx",
			args:    args{u: "build-harbor.alauda.cn/middleware/redis:5.0-alpine.000b26a0c3b6"},
			want:    RedisVersion5,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRedisVersionFromImage(tt.args.u)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRedisVersionFromImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseRedisVersionFromImage() = %v, want %v", got, tt.want)
			}
		})
	}
}
