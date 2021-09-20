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
	"github.com/alauda/redis-operator/pkg/types/redis"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RedisUserSpec defines the desired state of RedisUser
type RedisUserSpec struct {
	// Username
	// +kubebuilder:validation:MaxLength=64
	Username string `json:"username"`
	// Redis Password secret name, key is password
	PasswordSecrets []string `json:"passwordSecrets,omitempty"`
	// AclRules acl rules
	// +kubebuilder:validation:MaxLength=4096
	AclRules string `json:"aclRules,omitempty"`
	// Redis instance  Name (required)
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	RedisName string `json:"redisName"`
	// redis user account type
	// +kubebuilder:validation:Enum=system;custom;default
	AccountType AccountType `json:"accountType,omitempty"`
	// redis user account type
	// +kubebuilder:validation:Enum=sentinel;cluster
	// +kubebuilder:default=sentinel
	Arch redis.RedisArch `json:"arch,omitempty"`
}

// AccountType
type AccountType string

const (
	// System only operator is the system account
	System  AccountType = "system"
	Custom  AccountType = "custom"
	Default AccountType = "default"
)

// Phase
type Phase string

const (
	Fail    Phase = "Fail"
	Success Phase = "Success"
	Pending Phase = "Pending" //实例没就绪
)

// RedisUserStatus defines the observed state of RedisUser
type RedisUserStatus struct {
	// LastUpdatedSuccess is the last time the user was successfully updated.
	LastUpdatedSuccess string `json:"lastUpdateSuccess,omitempty"`
	// +kubebuilder:validation:Enum=Fail;Success;Pending
	Phase Phase `json:"Phase,omitempty"`
	// Message
	Message string `json:"message,omitempty"`
	// AclRules last configed acl rule
	AclRules string `json:"aclRules,omitempty"`
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
