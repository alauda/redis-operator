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

package cluster

import (
	"context"
	"os"
	"path"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Sync
func SyncToLocal(ctx context.Context, client *kubernetes.Clientset, namespace, name,
	workspace, target string, logger logr.Logger) ([]byte, error) {

	targetFile := path.Join(workspace, target)
	// check if node.conf exists
	if data, err := os.ReadFile(targetFile); len(data) > 0 {
		// ignore if this file exists and not empty
		return data, nil
	} else if os.IsPermission(err) {
		logger.Error(err, "no permission")
		return nil, err
	}

	var cm *v1.ConfigMap
	if err := RetryGet(func() (err error) {
		cm, err = client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		return
	}, 20); errors.IsNotFound(err) {
		logger.Info("no synced nodes.conf found")
		return nil, nil
	} else if err != nil {
		logger.Error(err, "get configmap failed", "name", name)
		return nil, nil
	}

	val := cm.Data[target]
	logger.Info("sync backup nodes.conf to local", "file", targetFile, "nodes.conf", val)
	return []byte(val), os.WriteFile(targetFile, []byte(val), 0644)
}
