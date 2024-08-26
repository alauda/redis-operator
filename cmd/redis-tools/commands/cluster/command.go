package cluster

import (
	"context"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/commands"
	"github.com/alauda/redis-operator/cmd/redis-tools/util"
	"github.com/urfave/cli/v2"
	corev1 "k8s.io/api/core/v1"
)

func NewCommand(ctx context.Context) *cli.Command {
	return &cli.Command{
		Name:  "cluster",
		Usage: "Cluster set commands",
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
			&cli.StringFlag{
				Name:    "service-type",
				Usage:   "Service type for sentinel service",
				EnvVars: []string{"SERVICE_TYPE"},
				Value:   "ClusterIP",
			},
		},
		Subcommands: []*cli.Command{
			{
				Name:  "expose",
				Usage: "Create nodeport service for current pod to announce",
				Flags: []cli.Flag{},
				Action: func(c *cli.Context) error {
					var (
						namespace   = c.String("namespace")
						podName     = c.String("pod-name")
						ipFamily    = c.String("ip-family")
						serviceType = corev1.ServiceType(c.String("service-type"))
					)
					if namespace == "" {
						return cli.Exit("require namespace", 1)
					}
					if podName == "" {
						return cli.Exit("require podname", 1)
					}

					logger := util.NewLogger(c).WithName("Expose")

					client, err := util.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}

					if err := ExposeNodePort(ctx, client, namespace, podName, ipFamily, serviceType, logger); err != nil {
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
						Name:  "config-name",
						Usage: "Node config file name",
						Value: "nodes.conf",
					},
					&cli.StringFlag{
						Name:  "prefix",
						Usage: "Configmap name prefix",
						Value: "sync-",
					},
					&cli.StringFlag{
						Name:    "shard-id",
						Usage:   "Shard ID for redis cluster shard",
						EnvVars: []string{"SHARD_ID"},
					},
				},
				Action: func(c *cli.Context) error {
					logger := util.NewLogger(c).WithName("Heal")

					client, err := util.NewClient()
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
							logger := util.NewLogger(c).WithName("readiness")

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
						Name:  "liveness",
						Usage: "Redis node liveness check, which just checked tcp socket",
						Action: func(c *cli.Context) error {
							var (
								serviceAddr = c.String("addr")
								timeout     = c.Int64("timeout")
							)
							logger := util.NewLogger(c).WithName("liveness")

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
						Name:  "config-name",
						Usage: "Node config file name",
						Value: "nodes.conf",
					},
					&cli.StringFlag{
						Name:  "prefix",
						Usage: "Configmap name prefix",
						Value: "sync-",
					},
					&cli.IntFlag{
						Name:    "timeout",
						Aliases: []string{"t"},
						Usage:   "Timeout time of shutdown",
						Value:   500,
					},
				},
				Action: func(c *cli.Context) error {
					logger := util.NewLogger(c).WithName("Shutdown")

					client, err := util.NewClient()
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