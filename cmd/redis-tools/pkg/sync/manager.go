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
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ConfigManager struct {
	client *kubernetes.Clientset
	logger logr.Logger
}

func NewConfigManager(client *kubernetes.Clientset, logger logr.Logger) (*ConfigManager, error) {
	if client == nil {
		return nil, fmt.Errorf("Clientset required")
	}

	cm := ConfigManager{
		client: client,
		logger: logger.WithName("ConfigManager"),
	}
	return &cm, nil
}

// Get
func (m *ConfigManager) Get(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
	cm, err := m.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// Save
func (m *ConfigManager) Save(ctx context.Context, cm *corev1.ConfigMap) error {
	if cm == nil {
		return nil
	}

	oldCm, err := m.client.CoreV1().ConfigMaps(cm.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		if _, err := m.client.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			m.logger.Error(err, "create configmap failed", "namespace", cm.Namespace, "name", cm.Name)
			return err
		}
		return nil
	} else if err != nil {
		m.logger.Error(err, "check configmap status failed", "namespace", cm.Namespace, "name", cm.Name)
		return err
	}
	oldCm.Data = cm.Data
	if _, err := m.client.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, oldCm, metav1.UpdateOptions{}); err != nil {
		m.logger.Error(err, "update configmap failed", "namespace", cm.Namespace, cm.Name)
		return err
	}
	return nil
}

// Delete
func (m *ConfigManager) Delete(ctx context.Context, namespace, name string) error {
	err := m.client.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		m.logger.Error(err, "delete configmap failed", "namespace", namespace, name)
		return err
	}
	return nil
}
