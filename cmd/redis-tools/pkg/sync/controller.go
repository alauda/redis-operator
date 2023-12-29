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
	rerrors "errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type UpdateEvent struct {
	FilePath  string
	Data      string
	Timestamp int64
}

type Filter interface {
	Truncate(filename string, data string) (string, error)
}

type ControllerOptions struct {
	Namespace       string
	ConfigMapName   string
	OwnerReferences []metav1.OwnerReference
	SyncInterval    time.Duration
	Filters         []Filter
}

// Controller
type Controller struct {
	cmManager *ConfigManager

	dataUpdateChan chan *UpdateEvent
	filters        []Filter

	options ControllerOptions
	logger  logr.Logger
}

func NewController(client *kubernetes.Clientset, options ControllerOptions, logger logr.Logger) (*Controller, error) {
	logger = logger.WithName("Controller")

	if options.SyncInterval <= time.Second*3 {
		options.SyncInterval = time.Second * 10
	}
	if options.Namespace == "" {
		return nil, fmt.Errorf("namespace required")
	}
	if options.ConfigMapName == "" {
		return nil, fmt.Errorf("configmap name required")
	}
	if len(options.OwnerReferences) == 0 {
		logger.Info("WARNING: resource owner reference not specified")
	}

	cmManager, err := NewConfigManager(client, logger)
	if err != nil {
		logger.Error(err, "create ConfigManager failed")
		return nil, err
	}

	ctrl := Controller{
		cmManager:      cmManager,
		dataUpdateChan: make(chan *UpdateEvent, 100),
		options:        options,
		filters:        options.Filters,
		logger:         logger,
	}
	return &ctrl, nil
}

func (c *Controller) Handler(ctx context.Context, fs *FileStat) error {
	logger := c.logger.WithName("Handler")

	data, err := os.ReadFile(fs.FilePath())
	if err != nil {
		logger.Error(err, "load file content failed", "filepath", fs.FilePath())
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.dataUpdateChan <- &UpdateEvent{FilePath: fs.FilePath(), Data: string(data)}:
	}
	return nil
}

func (c *Controller) Run(ctx context.Context) error {
	logger := c.logger

	getConfigMap := func() (*v1.ConfigMap, error) {
		configMap, err := c.cmManager.Get(ctx, c.options.Namespace, c.options.ConfigMapName)
		if errors.IsNotFound(err) {
			configMap = &v1.ConfigMap{}
			configMap.Namespace = c.options.Namespace
			configMap.Name = c.options.ConfigMapName
			configMap.OwnerReferences = c.options.OwnerReferences

			if err := c.cmManager.Save(ctx, configMap); err != nil {
				logger.Error(err, "init configmap failed", "namespace", configMap.Namespace, "name", configMap.Name)
			}
		} else if rerrors.Is(err, context.Canceled) {
			return nil, nil
		} else if err != nil {
			logger.Error(err, "get configmap failed", "namespace", configMap.Namespace, "name", configMap.Name)
			return nil, err
		}

		if configMap.Data == nil {
			configMap.Data = map[string]string{}
		}
		return configMap, nil
	}

	configMap, err := getConfigMap()
	if err != nil {
		return err
	}

	ticker := time.NewTicker(c.options.SyncInterval)
	defer ticker.Stop()

	isChanged := int64(0)
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-c.dataUpdateChan:
			if !ok {
				return nil
			}

			oldSize := configMap.Size()
			oldData := configMap.Data[event.FilePath]

			// etcd limiteds configmap/secret size limit to 1Mi
			if oldSize-len(oldData)+len(event.Data) >= 1024*1024*1024-4096 {
				logger.Error(fmt.Errorf("data size has exceed 1Mi"), "filepath", event.FilePath)
				continue
			}

			if event.Data == oldData {
				continue
			}

			filename := path.Base(event.FilePath)
			if func() bool {
				for _, filter := range c.filters {
					if data, err := filter.Truncate(filename, event.Data); err != nil {
						return false
					} else {
						event.Data = data
					}
				}
				return true
			}() {
				configMap.Data[filename] = event.Data
				isChanged += 1
			}
		case <-ticker.C:
			if isChanged == 0 {
				continue
			}

			func() {
				ctx, cancel := context.WithTimeout(ctx, c.options.SyncInterval-time.Second)
				defer cancel()

				if err := c.cmManager.Save(ctx, configMap); err != nil {
					logger.Error(err, "update configmap failed")
				}
			}()
			isChanged = 0
		}
	}
}
