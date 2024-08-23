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

package cluster

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rediskunv1alpha1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	"github.com/alauda/redis-operator/internal/ops"
	_ "github.com/alauda/redis-operator/internal/ops/cluster/actor"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes/clientset"
)

var _ = Describe("DistributedRedisClusterReconciler", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName = "redis-cluster"
			namespace    = "default"
		)

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}
		inst := &rediskunv1alpha1.DistributedRedisCluster{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Cron")
			err := k8sClient.Get(ctx, typeNamespacedName, inst)
			if err != nil && errors.IsNotFound(err) {
				resource := &rediskunv1alpha1.DistributedRedisCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: rediskunv1alpha1.DistributedRedisClusterSpec{
						Image:           "redis:6.0.20",
						MasterSize:      3,
						ClusterReplicas: 1,
						Config: map[string]string{
							"save": "900 1",
						},
						AffinityPolicy: core.AntiAffinityInSharding,
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
						IPFamilyPrefer: corev1.IPv4Protocol,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &rediskunv1alpha1.DistributedRedisCluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Cron")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")

			var (
				eventRecorder = record.NewFakeRecorder(10)
				logger, _     = logr.FromContext(context.TODO())

				clientset    = clientset.NewWithConfig(k8sClient, cfg, logger)
				actorManager = actor.NewActorManager(clientset, logger.WithName("ActorManager"))
			)

			engine, err := ops.NewOpEngine(k8sClient, eventRecorder, actorManager, logger)
			if err != nil {
				panic(err)
			}

			controllerReconciler := &DistributedRedisClusterReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				EventRecorder: eventRecorder,
				Engine:        engine,
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.

			err = k8sClient.Get(ctx, typeNamespacedName, inst)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Reconcile successfully", "instance", inst)
		})
	})
})
