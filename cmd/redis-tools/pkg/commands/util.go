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

package commands

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/redis"
	"github.com/urfave/cli/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	DefaultSecretMountPath = "/account/password"
)

func LoadProxyAuthInfo(c *cli.Context, ctx context.Context) (*redis.AuthInfo, error) {

	if data, err := os.ReadFile(DefaultSecretMountPath); err != nil {
		return &redis.AuthInfo{}, err
	} else {
		return &redis.AuthInfo{
			Password: string(data),
		}, nil
	}
}

func LoadAuthInfo(c *cli.Context, ctx context.Context) (*redis.AuthInfo, error) {
	var (
		// acl
		opUsername = c.String("operator-username")
		opSecret   = c.String("operator-secret-name")

		// tls
		isTLSEnabled = c.Bool("tls")
		tlsKeyFile   = c.String("tls-key-file")
		tlsCertFile  = c.String("tls-cert-file")
	)

	var (
		err      error
		tlsConf  *tls.Config
		password string
	)

	if opSecret != "" {
		if data, err := os.ReadFile(DefaultSecretMountPath); err != nil {
			return nil, err
		} else {
			password = string(data)
		}
	}

	if isTLSEnabled {
		if tlsConf, err = LoadTLSCofig(tlsKeyFile, tlsCertFile); err != nil {
			return nil, err
		}
	}
	return &redis.AuthInfo{
		Username: opUsername,
		Password: password,
		TLSConf:  tlsConf,
	}, nil
}

func LoadTLSCofig(tlsKeyFile, tlsCertFile string) (*tls.Config, error) {
	if tlsKeyFile == "" || tlsCertFile == "" {
		return nil, fmt.Errorf("tls file path not configed")
	}
	cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}, nil
}

// NewOwnerReference
func NewOwnerReference(ctx context.Context, client *kubernetes.Clientset, namespace, podName string) ([]metav1.OwnerReference, error) {
	if client == nil {
		return nil, nil
	}

	pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var name string
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "StatefulSet" {
			name = ownerRef.Name
			break
		}
	}
	if sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{}); err != nil {
		return nil, err
	} else {
		return sts.OwnerReferences, nil
	}
}
