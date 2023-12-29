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

package sync

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NewOwnerReference
func NewOwnerReference(ctx context.Context, client *kubernetes.Clientset, namespace, podName string) ([]metav1.OwnerReference, error) {
	if client == nil {
		return nil, nil
	}

	pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var name string
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "StatefulSet" {
			name = ownerRef.Name
			break
		}
	}
	if sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{}); err != nil {
		return nil, err
	} else {
		return sts.OwnerReferences, nil
	}
}
