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

package clientset

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// NameSpaces the client that knows how to interact with kubernetes to manage them
type NameSpaces interface {
	// GetNameSpace get namespace info form kubernetes
	GetNameSpace(ctx context.Context, namespace string) (*corev1.Namespace, error)
}

// NameSpacesOption is the NameSpaces client implementation using API calls to kubernetes.
type NameSpacesOption struct {
	client client.Client
	logger logr.Logger
}

// NewNameSpaces returns a new NameSpaces client.
func NewNameSpaces(logger logr.Logger) NameSpaces {
	logger = logger.WithValues("service", "k8s.namespaces")
	cfg, err := config.GetConfig()
	if err != nil {
		panic(err)
	}
	kubeClient, err := client.New(cfg, client.Options{})
	if err != nil {
		panic(err)
	}
	return &NameSpacesOption{
		client: kubeClient,
		logger: logger,
	}
}

// GetNameSpace implement the NameSpaces.Interface
func (n *NameSpacesOption) GetNameSpace(ctx context.Context, namespace string) (*corev1.Namespace, error) {
	nm := &corev1.Namespace{}
	err := n.client.Get(ctx, types.NamespacedName{
		Name:      namespace,
		Namespace: namespace,
	}, nm)
	if err != nil {
		return nil, err
	}
	return nm, err
}
