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

package backup

import (
	"context"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/kubernetes/client"
	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/logger"
	"github.com/urfave/cli/v2"
)

func NewCommand(ctx context.Context) *cli.Command {
	return &cli.Command{
		Name:  "backup",
		Usage: "backup commands",
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
				Name:    "append-commands",
				Usage:   "APPEND_COMMANDS for redis-cli",
				EnvVars: []string{"APPEND_COMMANDS"},
			},
			&cli.StringFlag{
				Name:    "redis-password",
				Usage:   "redis-password for redis-cli",
				EnvVars: []string{"REDIS_PASSWORD"},
			},
			&cli.StringFlag{
				Name:    "redis-name",
				Usage:   "redis-name for redis-cli",
				EnvVars: []string{"REDIS_NAME"},
			},
			&cli.StringFlag{
				Name:    "redis-cluster-index",
				Usage:   "redis shard index for backup",
				EnvVars: []string{"REDIS_ClUSTER_INDEX"},
			},
			&cli.StringFlag{
				Name:    "redis-backup-image",
				Usage:   "backup image",
				EnvVars: []string{"BACKUP_IMAGE"},
			},
			&cli.StringFlag{
				Name:    "redis-failover-name",
				Usage:   "redis sentinel name for backup",
				EnvVars: []string{"REDIS_FAILOVER_NAME"},
			},
			&cli.StringFlag{
				Name:    "redis-storage-class-name",
				Usage:   "STORAGE_CLASS_NAME",
				EnvVars: []string{"STORAGE_CLASS_NAME"},
			},
			&cli.StringFlag{
				Name:    "redis-storage-size",
				Usage:   "STORAGE_CLASS_NAME",
				EnvVars: []string{"STORAGE_SIZE"},
			},
			&cli.StringFlag{
				Name:    "redis-schedule-name",
				Usage:   "SCHEDULE_NAME",
				EnvVars: []string{"SCHEDULE_NAME"},
			},
			&cli.StringFlag{
				Name:    "redis-cluster-name",
				Usage:   "REDIS_CLUSTER_NAME",
				EnvVars: []string{"REDIS_CLUSTER_NAME"},
			},
			&cli.StringFlag{
				Name:    "redis-after-deletion",
				Usage:   "KEEP_AFTER_DELETION",
				EnvVars: []string{"KEEP_AFTER_DELETION"},
			},
			&cli.StringFlag{
				Name:    "redis-backup-job-name",
				Usage:   "",
				EnvVars: []string{"BACKUP_JOB_NAME"},
			},
			&cli.StringFlag{
				Name:    "s3-endpoint",
				Usage:   "S3_ENDPOINT",
				EnvVars: []string{"S3_ENDPOINT"},
			},
			&cli.StringFlag{
				EnvVars: []string{"S3_REGION"},
				Name:    "s3-region",
				Usage:   "S3_REGION",
			},
			&cli.StringFlag{
				EnvVars: []string{"DATA_DIR"},
				Name:    "data-dir",
				Usage:   "DATA_DIR",
			},
			&cli.StringFlag{
				EnvVars: []string{"S3_OBJECT_DIR"},
				Name:    "s3-object-dir",
				Usage:   "S3_OBJECT_DIR",
			},
			&cli.StringFlag{
				EnvVars: []string{"S3_BUCKET_NAME"},
				Name:    "s3-bucket-name",
				Usage:   "S3_BUCKET_NAME",
			},
			&cli.StringFlag{
				Name:    "s3-object-name",
				Usage:   "S3_OBJECT_NAME",
				EnvVars: []string{"S3_OBJECT_NAME"},
			},
			&cli.StringFlag{
				Name:    "s3-secret",
				Usage:   "S3_SECRET",
				EnvVars: []string{"S3_SECRET"},
			},
			&cli.StringFlag{
				Name:    "backoff-limit",
				Usage:   "BACKOFF_LIMIT",
				EnvVars: []string{"BACKOFF_LIMIT"},
			},
		},
		Subcommands: []*cli.Command{
			{
				Name:  "backup",
				Usage: "backup [option]",
				Flags: []cli.Flag{},
				Action: func(c *cli.Context) error {

					logger := logger.NewLogger(c).WithName("backup")

					client, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}
					err = Backup(ctx, c, client, logger)
					if err != nil {
						logger.Error(err, "backup, error")
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
			{
				Name:        "restore",
				Usage:       "restore [options]",
				Description: "restore",
				Flags:       []cli.Flag{},
				Action: func(c *cli.Context) error {
					logger := logger.NewLogger(c).WithName("Restore")
					err := Restore(ctx, c, logger)
					if err != nil {
						logger.Error(err, "backup, error")
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
			{
				Name:        "schedule",
				Usage:       "schedule [options]",
				Description: "schedule",
				Flags:       []cli.Flag{},
				Action: func(c *cli.Context) error {
					logger := logger.NewLogger(c).WithName("Schedule")
					kubenetesClient, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}

					err = ScheduleCreateRedisBackup(ctx, c, kubenetesClient, logger)
					if err != nil {
						logger.Error(err, "schedule, error")
						return cli.Exit(err, 1)
					}

					return nil
				},
			},
			{
				Name:        "pull",
				Usage:       "pull [options]",
				Description: "pull",
				Flags:       []cli.Flag{},
				Action: func(c *cli.Context) error {
					logger := logger.NewLogger(c).WithName("pull")
					kubenetesClient, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}

					err = Pull(ctx, c, kubenetesClient, logger)
					if err != nil {
						logger.Error(err, "pull, error")
						return cli.Exit(err, 1)
					}

					return nil
				},
			},
			{
				Name:        "push",
				Usage:       "push [options]",
				Description: "push",
				Flags:       []cli.Flag{},
				Action: func(c *cli.Context) error {
					logger := logger.NewLogger(c).WithName("push")
					kubenetesClient, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}

					err = PushFile2S3(ctx, c, kubenetesClient, logger)
					if err != nil {
						logger.Error(err, "push, error")
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
			{
				Name:        "rename",
				Usage:       "rename [options]",
				Description: "rename",
				Flags:       []cli.Flag{},
				Action: func(c *cli.Context) error {
					logger := logger.NewLogger(c).WithName("rename")
					kubenetesClient, err := client.NewClient()
					if err != nil {
						logger.Error(err, "create k8s client failed, error=%s", err)
						return cli.Exit(err, 1)
					}

					err = RenameCluster(ctx, c, kubenetesClient, logger)
					if err != nil {
						logger.Error(err, "rename, error")
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
		},
	}
}
