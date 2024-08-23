package runner

import (
	"context"
	"os"
	"path"
	"strings"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/commands"
	"github.com/alauda/redis-operator/cmd/redis-tools/sync"
	"github.com/alauda/redis-operator/cmd/redis-tools/util"
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func SyncFromLocalToEtcd(c *cli.Context, ctx context.Context, resourceKind string, watch bool, logger logr.Logger) error {
	var (
		namespace      = c.String("namespace")
		podName        = c.String("pod-name")
		workspace      = c.String("workspace")
		filename       = c.String("config-name")
		resourcePrefix = c.String("prefix")
		syncInterval   = c.Int64("interval")
	)

	client, err := util.NewClient()
	if err != nil {
		logger.Error(err, "create k8s client failed, error=%s", err)
		return cli.Exit(err, 1)
	}

	// sync to local
	name := strings.Join([]string{strings.TrimSuffix(resourcePrefix, "-"), podName}, "-")
	ownRefs, err := commands.NewOwnerReference(ctx, client, namespace, podName)
	if err != nil {
		return cli.Exit(err, 1)
	}
	if watch {
		// start sync process
		return WatchAndSync(ctx, client, resourceKind, namespace, name, workspace, filename, syncInterval, ownRefs, logger)
	}

	// write once
	filePath := path.Join(workspace, filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error(err, "read file failed", "file", filePath)
		return err
	}
	obj := sync.PersistentObject{}
	obj.Set(filename, data)
	return obj.Save(ctx, client, resourceKind, namespace, name, ownRefs, logger)
}

func WatchAndSync(ctx context.Context, client *kubernetes.Clientset, resourceKind, namespace, name, workspace, target string,
	syncInterval int64, ownerRefs []metav1.OwnerReference, logger logr.Logger) error {

	ctrl, err := sync.NewController(client, sync.ControllerOptions{
		ResourceKind:    resourceKind,
		Namespace:       namespace,
		Name:            name,
		OwnerReferences: ownerRefs,
		SyncInterval:    time.Duration(syncInterval) * time.Second,
		Filters:         []sync.Filter{&sync.RedisClusterFilter{}},
	}, logger)
	if err != nil {
		return err
	}
	fileWathcer, _ := sync.NewFileWatcher(ctrl.Handler, logger)

	logger.Info("watch file", "file", path.Join(workspace, target))
	if err := fileWathcer.Add(path.Join(workspace, target)); err != nil {
		logger.Error(err, "watch file failed, error=%s")
		return cli.Exit(err, 1)
	}

	go func() {
		_ = fileWathcer.Run(ctx)
	}()
	return ctrl.Run(ctx)
}
