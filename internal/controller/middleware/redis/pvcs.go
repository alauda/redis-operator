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

package redis

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// storage
	StorageProvisionerKey = "volume.beta.kubernetes.io/storage-provisioner"
	TopoLVMProvisionerKey = "topolvm.cybozu.com"
)

func GetShardMaxPVCQuantity(ctx context.Context, client ctrlClient.Client, namespace string, labels map[string]string) (*resource.Quantity, error) {
	var maxQuantity = resource.NewQuantity(0, resource.BinarySI)
	pvcs := &v1.PersistentVolumeClaimList{}
	if err := client.List(ctx, pvcs, ctrlClient.InNamespace(namespace), ctrlClient.MatchingLabels(labels)); err != nil {
		return nil, err
	}
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != v1.ClaimBound {
			continue
		}
		if pvc.Spec.Resources.Requests.Storage().Cmp(*maxQuantity) > 0 {
			maxQuantity = pvc.Spec.Resources.Requests.Storage()
		}
	}
	return maxQuantity, nil
}

func ResizePVCs(ctx context.Context, client ctrlClient.Client, namespace string, labels map[string]string, storageQuantity resource.Quantity) error {
	if labels == nil {
		return nil
	}
	pvcs := &v1.PersistentVolumeClaimList{}
	if err := client.List(ctx, pvcs, ctrlClient.InNamespace(namespace), ctrlClient.MatchingLabels(labels)); err != nil {
		return err
	}

	for _, pvc := range pvcs.Items {
		newPVC := pvc.DeepCopy()
		// determine whether the storage class support expand
		if newPVC.Annotations[StorageProvisionerKey] != TopoLVMProvisionerKey {
			continue
		}
		// wait pvc to bound
		if pvc.Status.Phase != v1.ClaimBound {
			continue
		}
		// compare storage size
		if pvc.Spec.Resources.Requests.Storage().Equal(storageQuantity) {
			continue
		}
		newPVC.Spec.Resources.Requests[v1.ResourceStorage] = storageQuantity
		if err := client.Update(ctx, newPVC); err != nil {
			return err
		}
	}
	return nil
}
