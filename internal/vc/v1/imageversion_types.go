// +kubebuilder:skip

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

// +kubebuilder:object:generate:=true

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ComponentVersion struct {
	Image          string                `json:"image"`
	Tag            string                `json:"tag"`
	Version        string                `json:"version,omitempty"`
	DisplayVersion string                `json:"displayVersion,omitempty"`
	Extensions     map[string]Extensions `json:"extensions,omitempty"`
}

type Extensions struct {
	Version string `json:"version"`
}

// ImageVersionSpec defines the desired state of ImageVersion
type ImageVersionSpec struct {
	CrVersion  string               `json:"crVersion,omitempty"`
	Components map[string]Component `json:"components,omitempty"`
}

// ImageVersionStatus defines the observed state of ImageVersion
type ImageVersionStatus struct{}

type Component struct {
	CoreComponent     bool               `json:"coreComponent,omitempty"`
	ComponentVersions []ComponentVersion `json:"versions"`
}

// +kubebuilder:object:root=true

// ImageVersion is the Schema for the imageversions API
type ImageVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageVersionSpec   `json:"spec,omitempty"`
	Status ImageVersionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageVersionList contains a list of ImageVersion
type ImageVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageVersion{}, &ImageVersionList{})
}
