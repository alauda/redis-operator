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

package v1

import (
	"github.com/alauda/redis-operator/api/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RedisUserSpec defines the desired state of RedisUser
type RedisUserSpec struct {
	// Redis Username (required)
	Username string `json:"username"` //用户名
	// Redis Password secret name, key is password
	PasswordSecrets []string `json:"passwordSecrets,omitempty"` //密码secret
	// redis  acl rules  string
	AclRules string `json:"aclRules,omitempty"` //acl规则
	// Redis instance  Name (required)
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	RedisName string `json:"redisName"` //redisname
	// redis user account type
	// +kubebuilder:validation:Enum=system;custom;default
	AccountType AccountType `json:"accountType,omitempty"` //账户类型 system,custom,default
	// redis user account type
	// +kubebuilder:validation:Enum=sentinel;cluster;standalone
	Arch core.Arch `json:"arch,omitempty"` //架构类型
}

type AccountType string

const (
	System  AccountType = "system"
	Custom  AccountType = "custom"
	Default AccountType = "default"
)

type Phase string

const (
	Fail    Phase = "Fail"
	Success Phase = "Success"
	Pending Phase = "Pending" //实例没就绪
)

// RedisUserStatus defines the observed state of RedisUser
type RedisUserStatus struct {
	LastUpdatedSuccess string `json:"lastUpdateSuccess,omitempty"` //最后更新时间
	// +kubebuilder:validation:Enum=Fail;Success;Pending
	Phase    Phase  `json:"Phase,omitempty"`    //Fail or Success
	Message  string `json:"message,omitempty"`  //失败信息
	AclRules string `json:"aclRules,omitempty"` //redis中的规则，会去除密码
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="username",type=string,JSONPath=`.spec.username`
//+kubebuilder:printcolumn:name="redisName",type=string,JSONPath=`.spec.redisName`

// RedisUser is the Schema for the redisusers API
type RedisUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisUserSpec   `json:"spec,omitempty"`
	Status RedisUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedisUserList contains a list of RedisUser
type RedisUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisUser{}, &RedisUserList{})
}
