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
	"log"
	"net/url"
	"os"
	"path"

	"github.com/go-logr/logr"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/kubernetes"
)

func PushFile2S3(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	secureFlag := false
	s3Url := c.String("s3-endpoint")
	s3Region := c.String("s3-region")
	dataDir := c.String("data-dir")
	s3ObjectDir := c.String("s3-object-dir")
	s3BucketName := c.String("s3-bucket-name")
	if s3Url == "" {
		logger.Info("S3_ENDPOINT is empty")
		return fmt.Errorf("S3_ENDPOINT is empty")
	}
	endpoint, err := url.Parse(s3Url)
	if err != nil {
		logger.Error(err, "S3_ENDPOINT is invalid")
		return err
	}
	if endpoint.Scheme == "https" {
		secureFlag = true
	}
	s3Client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(ReadFileToString("AWS_ACCESS_KEY_ID"), ReadFileToString("AWS_SECRET_ACCESS_KEY"), ReadFileToString("S3_TOKEN")),
		Region: s3Region,
		Secure: secureFlag,
	})
	if err != nil {
		logger.Error(err, "S3 client init failed")
		return err
	}
	files, err := os.ReadDir(dataDir)
	if err != nil {
		logger.Error(err, "Read data dir failed")
		return err
	}
	for _, file := range files {
		filepath := path.Join(dataDir, file.Name())
		object, err := os.Open(filepath)
		if err != nil {
			logger.Error(err, "S3 open file failed")
			return err
		}
		defer object.Close()
		objectStat, err := object.Stat()
		if err != nil {
			logger.Error(err, "S3 get file stat failed")
			return err
		}
		s3Path := path.Join(s3ObjectDir, file.Name())
		n, err := s3Client.PutObject(context.Background(), s3BucketName, s3Path, object, objectStat.Size(), minio.PutObjectOptions{ContentType: "application/octet-stream"})
		if err != nil {
			logger.Error(err, "S3 upload file failed")
			return err
		}
		log.Println("Uploaded", s3Path, " of size: ", n.Size, "Successfully.")
	}
	return nil
}

func ReadFileToString(filename string) string {
	filename = path.Join("/s3_secret", filename)
	content, err := os.ReadFile(filename)
	if err != nil {
		log.Println(err)
		return ""
	}
	str := string(content)
	return str
}
