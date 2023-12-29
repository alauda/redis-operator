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

package helper

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/types/user"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetUserPassword
func GetUserPassword(ctx context.Context, client *kubernetes.Clientset, namespace, name, username, secretName string) (string, error) {
	cm, err := GetConfigmap(ctx, client, namespace, name)
	if err != nil {
		return "", err
	}
	users, err := LoadACLUsers(ctx, client, cm)
	if err != nil {
		return "", err
	}
	if username == "" {
		username = "default"
	}
	for _, user := range users {
		if user.Name == username {
			return user.Password.String(), nil
		}
	}
	if secretName != "" {
		if secret, err := GetSecret(ctx, client, namespace, secretName); err != nil {
			return "", err
		} else if v := secret.Data["password"]; v != nil {
			return string(v), nil
		}
	}
	return "", nil
}

func GenerateACL(ctx context.Context, client *kubernetes.Clientset, namespace, name string) (ret []string, err error) {
	cm, err := GetConfigmap(ctx, client, namespace, name)
	if err != nil {
		return nil, err
	}
	users, err := LoadACLUsers(ctx, client, cm)
	if err != nil {
		return nil, err
	}

	// format
	for _, u := range users {
		ret = append(ret, strings.Join(formatACLSetCommand(u), " "))
	}
	return
}

// LoadACLUsers load acls from configmap
func LoadACLUsers(ctx context.Context, clientset *kubernetes.Clientset, cm *v1.ConfigMap) ([]*user.User, error) {
	var users []*user.User
	if cm == nil {
		return users, nil
	}
	for name, userData := range cm.Data {
		if name == "" {
			name = "default"
		}

		var u user.User
		if err := json.Unmarshal([]byte(userData), &u); err != nil {
			return nil, fmt.Errorf("parse user %s failed, error=%s", name, err)
		}
		if u.Password != nil && u.Password.SecretName != "" {
			if secret, err := GetSecret(ctx, clientset, cm.Namespace, u.Password.SecretName); err != nil {
				return nil, err
			} else {
				u.Password, _ = user.NewPassword(secret)
			}
		}
		u.Name = name

		if err := u.Validate(); err != nil {
			return nil, fmt.Errorf(`user "%s" is invalid, %s`, u.Name, err)
		}
		users = append(users, &u)
	}
	return users, nil
}

func GetSecret(ctx context.Context, client *kubernetes.Clientset, namespace, name string) (*corev1.Secret, error) {
	var (
		err    error
		secret *corev1.Secret
	)
	for i := 0; i < 5; i++ {
		if secret, err = func() (*corev1.Secret, error) {
			ctx, cancel := context.WithTimeout(ctx, time.Second*30)
			defer cancel()

			return client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		}(); err != nil {
			time.Sleep(time.Second * 1)
			continue
		}
		break
	}
	return secret, err
}

func GetConfigmap(ctx context.Context, client *kubernetes.Clientset, namespace, name string) (*corev1.ConfigMap, error) {
	var (
		err error
		cm  *corev1.ConfigMap
	)
	for i := 0; i < 5; i++ {
		if cm, err = func() (*corev1.ConfigMap, error) {
			ctx, cancel := context.WithTimeout(ctx, time.Second*30)
			defer cancel()

			return client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		}(); err != nil {
			time.Sleep(time.Second * 1)
			continue
		}
		break
	}
	return cm, err
}

// formatACLSetCommand
//
// only acl 1 supported
func formatACLSetCommand(user *user.User) (args []string) {
	// keep in mind that the user.Name is "default" for default user
	// when update command,password,keypattern, must reset them all
	args = []string{"user", user.Name}
	for _, rule := range user.Rules {
		for _, cate := range rule.Categories {
			cate = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cate), "+"), "@")
			args = append(args, fmt.Sprintf("+@%s", cate))
		}
		for _, cmd := range rule.AllowedCommands {
			cmd = strings.TrimPrefix(cmd, "+")
			args = append(args, fmt.Sprintf("+%s", cmd))
		}

		isDisableAllCmd := false
		for _, cmd := range rule.DisallowedCommands {
			cmd = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cmd), "-"), "@")
			if cmd == "nocommands" || cmd == "all" {
				isDisableAllCmd = true
			}
			args = append(args, fmt.Sprintf("-%s", cmd))
		}
		if len(rule.Categories) == 0 && len(rule.AllowedCommands) == 0 && !isDisableAllCmd {
			args = append(args, "+@all")
		}
		for _, pattern := range rule.KeyPatterns {
			pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "~")
			if !strings.HasPrefix(pattern, "%") {
				pattern = fmt.Sprintf("~%s", pattern)
			}
			args = append(args, pattern)
		}

		passwd := user.Password.String()
		if passwd == "" {
			args = append(args, "nopass")
		} else {
			args = append(args, fmt.Sprintf(">%s", passwd))
		}

		// NOTE: on must after reset
		args = append(args, "on")
	}
	return
}
