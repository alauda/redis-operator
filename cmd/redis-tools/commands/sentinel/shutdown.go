package sentinel

import (
	"context"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/commands/runner"
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
)

// Shutdown
func Shutdown(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	timeout := time.Duration(c.Int("timeout")) * time.Second
	if timeout == 0 {
		timeout = time.Second * 30
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// sync current nodes.conf to configmap
	logger.Info("persistent sentinel.conf to secret")
	if err := runner.SyncFromLocalToEtcd(c, ctx, "secret", false, logger); err != nil {
		logger.Error(err, "persistent sentinel.conf to configmap failed")
	}
	return nil
}
