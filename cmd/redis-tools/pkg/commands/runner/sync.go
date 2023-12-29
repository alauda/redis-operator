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

package runner

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/commands"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/kubernetes/client"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/sync"
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func SyncFromLocalToConfigMap(c *cli.Context, ctx context.Context, logger logr.Logger) error {
	var (
		namespace       = c.String("namespace")
		podName         = c.String("pod-name")
		workspace       = c.String("workspace")
		nodeConfigName  = c.String("node-config-name")
		configMapPrefix = c.String("prefix")
		syncInterval    = c.Int64("interval")
	)

	client, err := client.NewClient()
	if err != nil {
		logger.Error(err, "create k8s client failed, error=%s", err)
		return cli.Exit(err, 1)
	}
	// sync to local
	name := strings.Join([]string{strings.TrimSuffix(configMapPrefix, "-"), podName}, "-")
	ownRefs, err := commands.NewOwnerReference(ctx, client, namespace, podName)
	if err != nil {
		return cli.Exit(err, 1)
	}
	// start sync process
	return WatchAndSync(ctx, client, namespace, name, workspace, nodeConfigName, syncInterval, ownRefs, logger)
}

func WatchAndSync(ctx context.Context, client *kubernetes.Clientset, namespace, name, workspace, target string,
	syncInterval int64, ownerRefs []v1.OwnerReference, logger logr.Logger) error {

	ctrl, err := sync.NewController(client, sync.ControllerOptions{
		Namespace:       namespace,
		ConfigMapName:   name,
		OwnerReferences: ownerRefs,
		SyncInterval:    time.Duration(syncInterval) * time.Second,
		Filters:         []sync.Filter{&sync.RedisClusterFilter{}},
	}, logger)
	if err != nil {
		return err
	}
	fileWathcer, _ := sync.NewFileWatcher(ctrl.Handler, logger)

	if err := fileWathcer.Add(path.Join(workspace, target)); err != nil {
		logger.Error(err, "watch file failed, error=%s")
		return cli.Exit(err, 1)
	}

	go func() {
		_ = fileWathcer.Run(ctx)
	}()
	return ctrl.Run(ctx)
}
