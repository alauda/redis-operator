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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
)

func ScheduleCreateRedisBackup(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	keepAfterDeletion := c.String("redis-after-deletion")
	backupJobName := c.String("redis-backup-job-name")
	redisClusterName := c.String("redis-cluster-name")
	scheduleName := c.String("redis-schedule-name")
	s3BucketName := c.String("s3-bucket-name")
	backoffLimit := c.String("backoff-limit")
	image := c.String("redis-backup-image")

	if image == "" {
		image = "''"
	}
	redisFailoverName := c.String("redis-failover-name")
	storageClassName := c.String("redis-storage-class-name")
	storage := c.String("redis-storage-size")
	s3Secret := c.String("s3-secret")
	content := ""

	if redisClusterName != "" {
		content = genRedisClusterYAML(keepAfterDeletion, backupJobName, redisClusterName, scheduleName, image, storageClassName, storage, s3BucketName, backoffLimit, s3Secret)
	}
	if redisFailoverName != "" {
		content = genFailoverYAML(keepAfterDeletion, backupJobName, redisFailoverName, scheduleName, image, storageClassName, storage, s3BucketName, backoffLimit, s3Secret)
	}
	logger.Info("Gen success", "content", content)
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(content)
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logger.Error(err, string(exitError.Stderr))
			return err
		}
	}
	return nil
}

func genRedisClusterYAML(keepAfterDeletion, backupJobName, redisClusterName, scheduleName, image, storageClassName, storage, s3BucketName, backoffLimit, s3Secret string) string {
	var yamlContent string
	if s3BucketName != "" {
		currentTime := time.Now().UTC()
		s3Dir := fmt.Sprintf("data/backup/redis-cluster/schedule/%s", currentTime.Format("2006-01-02T15:04:05Z"))

		yamlContent = fmt.Sprintf(`apiVersion: redis.middleware.alauda.io/v1
kind: RedisClusterBackup
metadata:
  name: %s
  annotations:
    createType: auto
  labels:
    redis.kun/name: %s
    redis.kun/scheduleName: %s
spec:
  backoffLimit: %s
  image: %s
  source:
    redisClusterName: %s
  storage: %s
  target:
    s3Option:
      bucket: %s
      dir: %s
      s3Secret: %s`, backupJobName, redisClusterName, scheduleName, backoffLimit, image, redisClusterName, storage, s3BucketName, s3Dir, s3Secret)
	} else if keepAfterDeletion == "true" {
		yamlContent = fmt.Sprintf(`apiVersion: redis.middleware.alauda.io/v1
kind: RedisClusterBackup
metadata:
  annotations:
    createType: auto
  name: %s
  labels:
    redis.kun/name: %s
    redis.kun/scheduleName: %s
spec:
  image: %s
  source:
    redisClusterName: %s
    storageClassName: %s
  storage: %s`, backupJobName, redisClusterName, scheduleName, image, redisClusterName, storageClassName, storage)
	} else {
		backupJobUID := "example-backup-job-uid" // Set the value based on your requirement

		yamlContent = fmt.Sprintf(`apiVersion: redis.middleware.alauda.io/v1
kind: RedisClusterBackup
metadata:
  annotations:
    createType: auto
  name: %s
  ownerReferences:
  - apiVersion: v1
    kind: Pod
    name: %s
    uid: %s
  labels:
    redis.kun/name: %s
    redis.kun/scheduleName: %s
spec:
  image: %s
  source:
    redisClusterName: %s
    storageClassName: %s
  storage: %s`, backupJobName, backupJobName, backupJobUID, redisClusterName, scheduleName, image, redisClusterName, storageClassName, storage)
	}
	return yamlContent
}

func genFailoverYAML(keepAfterDeletion, backupJobName, redisFailoverName, scheduleName, image, storageClassName, storage, s3BucketName, backoffLimit, s3Secret string) string {
	var yamlContent string
	if s3BucketName != "" {
		currentTime := time.Now().UTC()
		s3Dir := fmt.Sprintf("data/backup/redis-sentinel/schedule/%s", currentTime.Format("2006-01-02T15:04:05Z"))
		yamlContent = fmt.Sprintf(`apiVersion: redis.middleware.alauda.io/v1
kind: RedisBackup
metadata:
  annotations:
    createType: auto
  name: %s
  labels:
    redisfailovers.databases.spotahome.com/name: %s
    redisfailovers.databases.spotahome.com/scheduleName: %s
spec:
  backoffLimit: %s
  image: %s
  source:
    redisName: %s
  storage: %s
  target:
    s3Option:
      bucket: %s
      dir: %s
      s3Secret: %s`, backupJobName, redisFailoverName, scheduleName, backoffLimit, image, redisFailoverName, storage, s3BucketName, s3Dir, s3Secret)
	} else if keepAfterDeletion == "true" {
		yamlContent = fmt.Sprintf(`apiVersion: redis.middleware.alauda.io/v1
kind: RedisBackup
metadata:
  annotations:
    createType: auto
  name: %s
  labels:
    redisfailovers.databases.spotahome.com/name: %s
    redisfailovers.databases.spotahome.com/scheduleName: %s
spec:
  image: %s
  source:
    redisFailoverName: %s
    storageClassName: %s
  storage: %s`, backupJobName, redisFailoverName, scheduleName, image, redisFailoverName, storageClassName, storage)
	} else {
		backupJobUID := "example-backup-job-uid" // Set the value based on your requirement

		yamlContent = fmt.Sprintf(`apiVersion: redis.middleware.alauda.io/v1
kind: RedisBackup
metadata:
  annotations:
    createType: auto
  name: %s
  ownerReferences:
  - apiVersion: v1
    kind: Pod
    name: %s
    uid: %s
  labels:
    redisfailovers.databases.spotahome.com/name: %s
    redisfailovers.databases.spotahome.com/scheduleName: %s
spec:
  image: %s
  source:
    redisFailoverName: %s
    storageClassName: %s
  storage: %s`, backupJobName, backupJobName, backupJobUID, redisFailoverName, scheduleName, image, redisFailoverName, storageClassName, storage)
	}
	return yamlContent
}
