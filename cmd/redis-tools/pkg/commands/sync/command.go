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
	"crypto/tls"
	"strings"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/kubernetes/client"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/logger"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/redis"
	"github.com/urfave/cli/v2"
)

func NewCommand(ctx context.Context) *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "[Deprecated] Sync configfile from/to configmap",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "namespace",
				Usage:   "Namespace of current pod",
				EnvVars: []string{"NAMESPACE"},
			},
			&cli.StringFlag{
				Name:    "pod-name",
				Usage:   "The name of current pod",
				EnvVars: []string{"POD_NAME"},
			},
			&cli.StringFlag{
				Name:  "workspace",
				Usage: "Workspace of this container",
				Value: "/data",
			},
			&cli.StringFlag{
				Name:  "node-config-name",
				Usage: "Node config file name",
				Value: "nodes.conf",
			},
			&cli.StringFlag{
				Name:  "prefix",
				Usage: "Configmap name prefix",
				Value: "sync-",
			},
		},
		Subcommands: []*cli.Command{
			{
				Name:  "c2l",
				Usage: "Sync configfile from configmap to local",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "password",
						Usage:   "Redis instance password",
						EnvVars: []string{"REDIS_PASSWORD"},
					},
					&cli.StringFlag{
						Name:    "tls-key-file",
						Usage:   "Name of the client key file (including full path) if the server requires TLS client authentication",
						EnvVars: []string{"TLS_CLIENT_KEY_FILE"},
					},
					&cli.StringFlag{
						Name:    "tls-cert-file",
						Usage:   "Name of the client certificate file (including full path) if the server requires TLS client authentication",
						EnvVars: []string{"TLS_CLIENT_CERT_FILE"},
					},
				},
				Action: func(c *cli.Context) error {
					var (
						namespace       = c.String("namespace")
						podName         = c.String("pod-name")
						workspace       = c.String("workspace")
						nodeConfigName  = c.String("node-config-name")
						configMapPrefix = c.String("prefix")
						password        = c.String("password")
						tlsKeyFile      = c.String("tls-key-file")
						tlsCertFile     = c.String("tls-cert-file")
					)

					ctx, cancel := context.WithCancel(ctx)
					defer cancel()

					logger := logger.NewLogger(c)

					client, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}

					// sync to local
					name := strings.Join([]string{strings.TrimSuffix(configMapPrefix, "-"), podName}, "-")

					opts := redis.AuthInfo{
						Password: password,
					}
					if tlsKeyFile != "" && tlsCertFile != "" {
						cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
						if err != nil {
							logger.Error(err, "load tls certificates failed")
							return cli.Exit(err, 1)
						}
						opts.TLSConf = &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						}
					}
					if err := SyncToLocal(ctx, client, namespace, name, workspace, nodeConfigName, opts, logger); err != nil {
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
			{
				Name:  "l2c",
				Usage: "Sync configfile from local to configmap",
				Flags: []cli.Flag{
					&cli.Int64Flag{
						Name:        "interval",
						Usage:       "Configmap sync interval",
						Value:       5,
						DefaultText: "5s",
					},
				},
				Action: func(c *cli.Context) error {
					var (
						namespace       = c.String("namespace")
						podName         = c.String("pod-name")
						workspace       = c.String("workspace")
						nodeConfigName  = c.String("node-config-name")
						configMapPrefix = c.String("prefix")
						syncInterval    = c.Int64("interval")
					)

					ctx, cancel := context.WithCancel(ctx)
					defer cancel()

					logger := logger.NewLogger(c)

					client, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}

					name := strings.Join([]string{strings.TrimSuffix(configMapPrefix, "-"), podName}, "-")
					ownRefs, err := NewOwnerReference(ctx, client, namespace, podName)
					if err != nil {
						return cli.Exit(err, 1)
					}
					// start sync process
					return WatchAndSync(ctx, client, namespace, name, workspace, nodeConfigName, syncInterval, ownRefs, logger)
				},
			},
		},
	}
}
