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
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
)

func Backup(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	appendCommands := c.String("append-commands")
	redisPassword := c.String("redis-password")

	instanceName := c.String("redis-name")
	clusterIndex := c.String("redis-cluster-index")
	podName := instanceName
	if clusterIndex == "" {
		podName = fmt.Sprintf("rfr-%s-0", instanceName)
	}
	cliCmd := exec.Command("redis-cli")
	logger.Info("backup starting")
	if redisPassword != "" {
		cliCmd.Args = append(cliCmd.Args, "-a", redisPassword)
	}

	if appendCommands != "" {
		appendCommandsList := SplitAndTrimSpace(appendCommands)
		cliCmd.Args = append(cliCmd.Args, appendCommandsList...)
	}
	//最后一次bgsave时间
	startLastSaveTimestamp, err := ExecLastSave(cliCmd, podName, logger)
	if err != nil {
		return err
	}
	for range make([]struct{}, 300) {
		err := ExecBgSave(cliCmd, podName, logger)
		if err != nil {
			return err
		}
		time.Sleep(time.Duration(time.Duration.Seconds(5)))
		endLastSaveTimestamp, err := ExecLastSave(cliCmd, podName, logger)
		if err != nil {
			return err
		}
		// 查看bgsave 有再一次支持
		if endLastSaveTimestamp > startLastSaveTimestamp {
			err = KubectlRedisDump(podName, clusterIndex, logger)
			if err != nil {
				return err
			}
			break
		}
	}

	return nil
}

func ExecLastSave(cliCmd *exec.Cmd, podName string, logger logr.Logger) (string, error) {
	lastSaveCmd := &exec.Cmd{}
	lastSaveCmd.Args = append(cliCmd.Args, "LASTSAVE")
	output, err := KubectlExec(podName, "redis", logger, lastSaveCmd)
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logger.Error(err, string(exitError.Stderr))
			return string(output), err
		}
	}
	logger.Info(string(output))
	return string(output), nil
}

func ExecBgSave(cliCmd *exec.Cmd, podName string, logger logr.Logger) error {
	bgSaveCmd := &exec.Cmd{}
	bgSaveCmd.Args = append(cliCmd.Args, "BGSAVE")
	output, err := KubectlExec(podName, "redis", logger, bgSaveCmd)
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logger.Error(err, string(exitError.Stderr))
			return err
		}
	}
	logger.Info(string(output))
	return nil
}

func KubectlExec(podName, containerName string, logger logr.Logger, command *exec.Cmd) (string, error) {
	cmd := exec.Command("kubectl", "exec", "-it", podName, "-c", "redis", "--")
	cmd.Args = append(cmd.Args, command.Args...)
	output, _err := cmd.Output()
	if _err != nil {
		if exitError, ok := _err.(*exec.ExitError); ok {
			logger.Error(_err, string(exitError.Stderr))
			return string(exitError.Stderr), _err
		}
	}
	logger.Info("exc kubectl rdb dump success", "output", string(output))
	return string(output), _err

}

func SplitAndTrimSpace(s string) []string {
	parts := strings.Split(s, " ")
	var nonEmptyParts []string
	for _, part := range parts {
		trimmedPart := strings.TrimSpace(part)
		if trimmedPart != "" {
			nonEmptyParts = append(nonEmptyParts, trimmedPart)
		}
	}
	return nonEmptyParts
}

func KubectlRedisDump(podName, clusterIndex string, logger logr.Logger) error {
	logger.Info("redis  dump to local", "podName", podName)
	backupDir := path.Join("/backup", clusterIndex)
	if clusterIndex != "" {
		cmd := exec.Command("kubectl", "cp", fmt.Sprintf("%s:%s", podName, "nodes.conf"), path.Join(backupDir, "nodes.conf"), "-c", "redis")
		output, _err := cmd.Output()
		if _err != nil {
			if exitError, ok := _err.(*exec.ExitError); ok {
				logger.Error(_err, string(exitError.Stderr))
			}
		} else {
			logger.Info("exc kubectl nodes.conf dump success", "output", string(output))
		}
	}

	cmd := exec.Command("kubectl", "cp", fmt.Sprintf("%s:%s", podName, "dump.rdb"), path.Join(backupDir, "dump.rdb"), "-c", "redis")
	output, _err := cmd.Output()
	if _err != nil {
		if exitError, ok := _err.(*exec.ExitError); ok {
			logger.Error(_err, string(exitError.Stderr))
		}
	} else {
		logger.Info("exc kubectl rdb dump success", "output:", string(output))
		return _err
	}

	cmd = exec.Command("kubectl", "cp", fmt.Sprintf("%s:%s", podName, "appendonly.aof"), path.Join(backupDir, "appendonly.aof"), "-c", "redis")
	output, _err = cmd.Output()
	if _err != nil {
		if exitError, ok := _err.(*exec.ExitError); ok {
			logger.Error(_err, string(exitError.Stderr))
			return _err
		}
	}
	logger.Info("exc kubectl aof dump success", "output:", string(output))
	return nil
}

func BgSave(redisPassword, appendCommands, svcName string, logger logr.Logger) {
	cmd := exec.Command("redis-cli", "-h", svcName)
	if redisPassword != "" {
		cmd.Args = append(cmd.Args, "-a", redisPassword)
	}
	if appendCommands != "" {
		appendCommandsList := SplitAndTrimSpace(appendCommands)
		cmd.Args = append(cmd.Args, appendCommandsList...)
	}
	cmd.Args = append(cmd.Args, "LASTSAVE")
	output, _err := cmd.Output()
	if _err != nil {
		if exitError, ok := _err.(*exec.ExitError); ok {
			logger.Error(_err, string(exitError.Stderr))
		}
	}
	logger.Info(string(output))

}
