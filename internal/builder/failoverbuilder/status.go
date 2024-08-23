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

package failoverbuilder

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/internal/util"
	"github.com/alauda/redis-operator/pkg/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ShouldUpdateDetailedStatusConfigMap(cm *corev1.ConfigMap, status *v1.RedisFailoverDetailedStatus) bool {
	if cm.Data == nil || cm.Data["status"] == "" {
		return true
	}

	oldStatus := &v1.RedisFailoverDetailedStatus{}
	if err := json.Unmarshal([]byte(cm.Data["status"]), oldStatus); err != nil {
		return true
	}
	if oldStatus.Phase != status.Phase || oldStatus.Message != status.Message || len(oldStatus.Nodes) != len(status.Nodes) {
		return true
	}

	// if the last update is more than 5 minutes ago, we should update the status
	tsVal := cm.Annotations["updateTimestamp"]
	timestamp, _ := strconv.ParseInt(tsVal, 10, 64)
	if timestamp+60*5 < time.Now().Unix() {
		return true
	}

	for i := 0; i < len(oldStatus.Nodes); i++ {
		onode, nnode := oldStatus.Nodes[i], status.Nodes[i]
		if onode.Role != nnode.Role ||
			onode.IP != nnode.IP ||
			onode.Port != nnode.Port ||
			onode.Version != nnode.Version ||
			onode.NodeName != nnode.NodeName ||

			// if the memory usage is different than 1MB, we should update the status
			math.Abs(float64(onode.UsedMemory-nnode.UsedMemory)) >= 1024*1024 ||
			math.Abs(float64(onode.UsedMemoryDataset-nnode.UsedMemoryDataset)) > 1024*1024 {
			return true
		}
	}
	return false
}

// NewRedisFailoverDetailedStatusConfigMap creates a new ConfigMap for the given Cluster
func NewRedisFailoverDetailedStatusConfigMap(inst types.RedisFailoverInstance, status *v1.RedisFailoverDetailedStatus) (*corev1.ConfigMap, error) {
	data, _ := json.MarshalIndent(status, "", "  ")
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("rfr-%s-status", inst.GetName()),
			Namespace: inst.GetNamespace(),
			Labels:    GetCommonLabels(inst.GetName(), nil),
			Annotations: map[string]string{
				"updateTimestamp": fmt.Sprintf("%d", metav1.Now().Unix()),
			},
			OwnerReferences: util.BuildOwnerReferencesWithParents(inst.Definition()),
		},
		Data: map[string]string{"status": string(data)},
	}, nil
}
