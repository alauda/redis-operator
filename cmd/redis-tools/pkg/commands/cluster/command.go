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
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/commands"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/kubernetes/client"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/logger"
	"github.com/urfave/cli/v2"
)

func NewCommand(ctx context.Context) *cli.Command {
	return &cli.Command{
		Name:  "cluster",
		Usage: "cluster set commands",
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
				Name:    "pod-uid",
				Usage:   "The id of current pod",
				EnvVars: []string{"POD_UID"},
			},
			&cli.StringFlag{
				Name:    "service-name",
				Usage:   "Service name of the statefulset",
				EnvVars: []string{"SERVICE_NAME"},
			},
			&cli.StringFlag{
				Name:    "operator-username",
				Usage:   "Operator username",
				EnvVars: []string{"OPERATOR_USERNAME"},
			},
			&cli.StringFlag{
				Name:    "operator-secret-name",
				Usage:   "Operator user password secret name",
				EnvVars: []string{"OPERATOR_SECRET_NAME"},
			},
			&cli.BoolFlag{
				Name:    "acl",
				Usage:   "Enable acl",
				EnvVars: []string{"ACL_ENABLED"},
				Hidden:  true,
			},
			&cli.StringFlag{
				Name:    "acl-config",
				Usage:   "Acl config map name",
				EnvVars: []string{"ACL_CONFIGMAP_NAME"},
			},
			&cli.BoolFlag{
				Name:    "tls",
				Usage:   "Enable tls",
				EnvVars: []string{"TLS_ENABLED"},
			},
			&cli.StringFlag{
				Name:    "tls-key-file",
				Usage:   "Name of the client key file (including full path)",
				EnvVars: []string{"TLS_CLIENT_KEY_FILE"},
				Value:   "/tls/tls.key",
			},
			&cli.StringFlag{
				Name:    "tls-cert-file",
				Usage:   "Name of the client certificate file (including full path)",
				EnvVars: []string{"TLS_CLIENT_CERT_FILE"},
				Value:   "/tls/tls.crt",
			},
			&cli.StringFlag{
				Name:    "tls-ca-file",
				Usage:   "Name of the ca file (including full path)",
				EnvVars: []string{"TLS_CA_CERT_FILE"},
				Value:   "/tls/ca.crt",
			},
			&cli.StringFlag{
				Name:    "nodeport-enabled",
				Usage:   "nodeport switch",
				EnvVars: []string{"NODEPORT_ENABLED"},
			},
			&cli.StringFlag{
				Name:    "ip-family",
				Usage:   "IP_FAMILY for expose",
				EnvVars: []string{"IP_FAMILY_PREFER"},
			},
			&cli.StringFlag{
				Name:    "custom-port-enabled",
				Usage:   "CUSTOM_PORT_ENABLED for expose",
				EnvVars: []string{"CUSTOM_PORT_ENABLED"},
				Value:   "false",
			},
		},
		Subcommands: []*cli.Command{
			{
				Name:  "expose",
				Usage: "Create nodeport service for current pod to announce",
				Flags: []cli.Flag{},
				Action: func(c *cli.Context) error {
					var (
						namespace       = c.String("namespace")
						podName         = c.String("pod-name")
						ipFamily        = c.String("ip-family")
						customPort      = c.String("custom-port-enabled") == "true"
						nodeportEnabled = c.String("nodeport-enabled") == "true"
					)
					if namespace == "" {
						return cli.Exit("require namespace", 1)
					}
					if podName == "" {
						return cli.Exit("require podname", 1)
					}

					logger := logger.NewLogger(c).WithName("Expose")

					client, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}

					if err := ExposeNodePort(ctx, client, namespace, podName, ipFamily, nodeportEnabled, customPort, logger); err != nil {
						logger.Error(err, "expose node port failed")
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
			{
				Name:        "heal",
				Usage:       "heal [options]",
				Description: "heal is used to healing current node",
				Flags: []cli.Flag{
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
				Action: func(c *cli.Context) error {
					logger := logger.NewLogger(c).WithName("Heal")

					client, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}
					if err := Heal(ctx, c, client, logger); err != nil {
						logger.Error(err, "expose node port failed")
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
			{
				Name:  "healthcheck",
				Usage: "Redis cluster health check",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "addr",
						Usage: "Redis instance service address",
						Value: "local.inject:6379",
					},
					&cli.IntFlag{
						Name:    "timeout",
						Aliases: []string{"t"},
						Usage:   "Timeout time of ping",
						Value:   3,
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:  "readiness",
						Usage: "Redis node readiness check",
						Action: func(c *cli.Context) error {
							var (
								serviceAddr = c.String("addr")
								timeout     = c.Int64("timeout")
							)
							logger := logger.NewLogger(c).WithName("readiness")

							if timeout <= 0 {
								timeout = 4
							}
							if serviceAddr == "" {
								serviceAddr = "local.inject:6379"
							}

							ctx, cancel := context.WithTimeout(ctx, time.Second*time.Duration(timeout))
							defer cancel()

							info, err := commands.LoadAuthInfo(c, ctx)
							if err != nil {
								logger.Error(err, "load auth info failed")
								return cli.Exit(err, 1)
							}

							if err := Readiness(ctx, serviceAddr, *info); err != nil {
								logger.Error(err, "check readiness failed")
								return cli.Exit(err, 1)
							}
							return nil
						},
					},
					{
						Name:  "proxy_check",
						Usage: "proxy check ",
						Action: func(c *cli.Context) error {
							var (
								serviceAddr = "127.0.0.1:6379"
								timeout     = c.Int64("timeout")
							)
							logger := logger.NewLogger(c).WithName("proxy_check")

							if timeout <= 0 {
								timeout = 4
							}
							ctx, cancel := context.WithTimeout(ctx, time.Second*time.Duration(timeout))
							defer cancel()

							info, err := commands.LoadProxyAuthInfo(c, ctx)
							if err != nil {
								logger.Error(err, "load auth info failed")
							}
							if err := Check(ctx, serviceAddr, *info); err != nil {
								logger.Error(err, "check proxy failed")
								return cli.Exit(err, 1)
							}
							return nil
						},
					},
					{
						Name:  "liveness",
						Usage: "Redis node liveness check, which just checked tcp socket",
						Action: func(c *cli.Context) error {
							var (
								serviceAddr = c.String("addr")
								timeout     = c.Int64("timeout")
							)
							logger := logger.NewLogger(c).WithName("liveness")

							if timeout <= 0 {
								timeout = 5
							}
							if serviceAddr == "" {
								serviceAddr = "local.inject:6379"
							}

							if err := TcpSocket(ctx, serviceAddr, time.Second*time.Duration(timeout)); err != nil {
								logger.Error(err, "ping failed")
								return cli.Exit(err, 1)
							}
							return nil
						},
					},
				},
			},
			{
				Name:  "shutdown",
				Usage: "Shutdown redis nodes",
				Flags: []cli.Flag{
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
					&cli.IntFlag{
						Name:    "timeout",
						Aliases: []string{"t"},
						Usage:   "Timeout time of shutdown",
						Value:   500,
					},
				},
				Action: func(c *cli.Context) error {
					logger := logger.NewLogger(c).WithName("Shutdown")

					client, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}
					if err := Shutdown(ctx, c, client, logger); err != nil {
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
		},
	}
}
