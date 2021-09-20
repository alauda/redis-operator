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

package security

import (
	"fmt"
	"testing"
)

func TestNewPassword(t *testing.T) {
	for i := 8; i < 33; i++ {
		t.Run(fmt.Sprintf("size: %d", i), func(t *testing.T) {
			got, err := GeneratePassword(i)
			if err != nil {
				t.Errorf("NewPassword() error = %v", err)
				return
			}
			t.Logf("password %s", got)
			if err := PasswordValidate(got, 8, 32); err != nil {
				t.Errorf("PasswordValidate() error = %v", err)
				return
			}
		})
	}
}

func TestPasswordValidate(t *testing.T) {
	minLen := 8
	maxLen := 32
	type args struct {
		pwd string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "less chars",
			args: args{
				pwd: "a$1",
			},
			wantErr: true,
		},
		{
			name: "only letters",
			args: args{
				pwd: "abcdefgh",
			},
			wantErr: true,
		},
		{
			name: "only numbers",
			args: args{
				pwd: "123456789",
			},
			wantErr: true,
		},
		{
			name: "only special chars",
			args: args{
				pwd: "~!@#$^*()-=+?",
			},
			wantErr: true,
		},
		{
			name: "unsupport special char",
			args: args{
				pwd: "China123$.",
			},
			wantErr: true,
		},
		{
			name: "only letter and number",
			args: args{
				pwd: "Abcd1234",
			},
			wantErr: true,
		},
		{
			name: "only letter and special",
			args: args{
				pwd: "Abcd+=-?",
			},
			wantErr: true,
		},
		{
			name: "only letter and special",
			args: args{
				pwd: "#$^@1234",
			},
			wantErr: true,
		},
		{
			name: "letters numbers and special",
			args: args{
				pwd: "$China123",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PasswordValidate(tt.args.pwd, minLen, maxLen); (err != nil) != tt.wantErr {
				t.Errorf("PasswordValidate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
