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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
)

func Restore(ctx context.Context, c *cli.Context, logger logr.Logger) error {
	clusterIndex := c.String("redis-cluster-index")
	dataDir := "/data"
	backupDir := path.Join("/backup", clusterIndex)

	if getPodIndex() != "0" && clusterIndex != "" {
		logger.Info("sts's pod index is not 0, cluster skip restore")
		return nil
	}

	if clusterIndex != "" {
		dumpFile := "nodes.conf"
		if (!FileExists(path.Join(dataDir, dumpFile))) && FileExists(path.Join(backupDir, dumpFile)) {
			content, err := FileToString(path.Join(backupDir, dumpFile))
			if err != nil {
				return err
			}
			slots := ExtractMyselfSlots(content)
			node_id, err := GenerateRandomCode(40)
			if err != nil {
				return err
			}
			line := fmt.Sprintf(`%s %s:%d@%d %s - 0 0 %d connected %s
vars currentEpoch %d lastVoteEpoch 0`, node_id, "127.0.0.1", 6379, 16379, "myself,master", 1, slots, 1)
			logger.Info("nodes info", "line:", line)
			err = os.WriteFile(path.Join(dataDir, dumpFile), []byte(line), 0666)
			if err != nil {
				logger.Error(err, "write  node file err")
				return err
			}
		} else {
			logger.Info("data  dir's node conf rdb exists or skip rdb")
		}
	}

	dumpFile := "dump.rdb"
	if (!FileExists(path.Join(dataDir, dumpFile))) && FileExists(path.Join(backupDir, dumpFile)) {
		if err := CopyFile(path.Join(backupDir, dumpFile), path.Join(dataDir, dumpFile), logger); err != nil {
			logger.Error(err, "copy rdb err")
		}
	} else {
		logger.Info("data dir's rdb exists or skip rdb")
	}

	dumpFile = "appendonly.aof"
	if (!FileExists(path.Join(dataDir, dumpFile))) && FileExists(path.Join(backupDir, dumpFile)) {
		if err := CopyFile(path.Join(backupDir, dumpFile), path.Join(dataDir, dumpFile), logger); err != nil {
			logger.Error(err, "copy aof err")
		}
	} else {
		logger.Info("data dir's aof exists or skip ")
	}
	return nil
}

func CopyFile(sourceFile, destinationFile string, logger logr.Logger) error {
	// 打开源文件
	src, err := os.Open(sourceFile)
	if err != nil {
		return err
	}
	defer src.Close()

	// 创建目标文件
	dst, err := os.Create(destinationFile)
	if err != nil {
		return err
	}
	defer dst.Close()

	// 复制文件内容
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	// 刷新缓冲区，确保文件内容已写入目标文件
	err = dst.Sync()
	if err != nil {
		return err
	}

	logger.Info("File copy success", "sourceFile", sourceFile, "destinationFile", destinationFile)
	return nil
}

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	} else {
		panic(err)
	}
}

func getPodIndex() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	parts := strings.Split(hostname, "-")
	return parts[len(parts)-1]

}

func GenerateRandomCode(length int) (string, error) {
	randomBytes := make([]byte, length/2)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	randomCode := hex.EncodeToString(randomBytes)
	return randomCode, nil
}

func ExtractMyselfSlots(data string) string {
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if strings.Contains(line, "myself") {
			columns := strings.Fields(line)
			if len(columns) >= 9 {
				return strings.Join(columns[8:], " ")
			}
		}
	}
	return ""
}

func FileToString(filename string) (string, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}

	str := string(content)
	return str, nil
}
