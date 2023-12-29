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
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/go-logr/logr"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
)

func downloadRdbFromS3(targetFile string) {
	secureFlag := false
	s3_url := os.Getenv("S3_ENDPOINT")
	endpoint, err := url.Parse(s3_url)
	if err != nil {
		log.Fatalln(err)
	}
	if endpoint.Scheme == "https" {
		secureFlag = true
	}
	s3Client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(ReadFileToString("AWS_ACCESS_KEY_ID"), ReadFileToString("AWS_SECRET_ACCESS_KEY"), ReadFileToString("S3_TOKEN")),
		Region: os.Getenv("S3_REGION"),
		Secure: secureFlag,
	})
	if err != nil {
		log.Fatalln(err)
	}
	reader, err := s3Client.GetObject(context.Background(), os.Getenv("S3_BUCKET_NAME"), os.Getenv("S3_OBJECT_NAME"), minio.GetObjectOptions{})
	if err != nil {
		log.Fatalln(err)
	}
	defer reader.Close()
	localFile, err := os.Create(targetFile)
	if err != nil {
		log.Fatalln(err)
	}
	defer localFile.Close()

	stat, err := reader.Stat()
	if err != nil {
		log.Fatalln(err)
	}

	if _, err := io.CopyN(localFile, reader, stat.Size); err != nil {
		log.Fatalln(err)
	}
	log.Printf("File %s download success!", targetFile)

}

func checkRdb(targetFile string) (string, error) {
	if os.Getenv("RDB_CHECK") == "true" {
		out, err := exec.Command("redis-check-rdb", targetFile).Output()
		if err == nil && strings.Contains(string(out), "RDB ERROR DETECTED") {
			return string(out), fmt.Errorf("rdb check err")
		}
		return string(out), err
	}
	return "", nil
}

func Pull(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	s3ObjectName := c.String("s3-object-name")
	// Prevent auto-generation of slots by the RDB during restoration to avoid slot confusion.
	if getPodIndex() != "0" && strings.Contains(s3ObjectName, "redis-cluster") {
		logger.Info("skip down cluster nodes when restore")
		return nil
	}
	TARGET_FILE := os.Getenv("TARGET_FILE")
	logger.Info(fmt.Sprintf("TARGET_FILE: %s", TARGET_FILE))
	if _, err := os.Stat(TARGET_FILE); err == nil {
		logger.Info(fmt.Sprintf("TARGET_FILE: %s exists", TARGET_FILE))
		out, _err := checkRdb(TARGET_FILE)
		logger.Info(fmt.Sprintf("check Rdb out : %s", out))
		if _err != nil {
			logger.Info("check rdb fail!")
			return _err
		}
		if os.Getenv("REWRITE") != "false" {
			logger.Info("skip re download rdb file")
			return nil
		}
	}
	//if no exists or rdb check err,re download
	if os.Getenv("S3_ENDPOINT") != "" {
		downloadRdbFromS3(TARGET_FILE)
	}
	out, _err := checkRdb(TARGET_FILE)
	logger.Info(fmt.Sprintf("check Rdb out : %s", out))
	if _err != nil {
		logger.Info("check rdb fail!")
		return _err
	}
	return nil
}
