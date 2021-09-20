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

package service

import (
	"testing"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	redisbackupv1 "github.com/alauda/redis-operator/api/redis/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddBackupAnnotationsAndLabels(t *testing.T) {
	instance := &databasesv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{
			Name: "redis-failover",
		},
		Status: databasesv1.RedisFailoverStatus{
			Phase: "Ready",
		},
		Spec: databasesv1.RedisFailoverSpec{
			Redis: databasesv1.RedisSettings{
				Image: "redis:6.2.1",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
			Sentinel: databasesv1.SentinelSettings{
				Replicas: 1,
			},
		},
	}
	backup := &redisbackupv1.RedisBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "redis-backup",
		},
		Spec: redisbackupv1.RedisBackupSpec{
			Source: redisbackupv1.RedisBackupSource{
				RedisFailoverName: "redis-failover",
			},
		},
	}

	err := AddBackupAnnotationsAndLabels(instance, backup)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Check annotations
	if backup.Annotations == nil {
		t.Error("Annotations should not be nil")
	}

	expectedResources := "{\"requests\":{\"cpu\":\"500m\",\"memory\":\"512Mi\"}}"
	if backup.Annotations["sourceResources"] != expectedResources {
		t.Errorf("Expected sourceResources annotation: %s, but got: %s", expectedResources, backup.Annotations["sourceResources"])
	}
	expectedVersion := "6.2.1"
	if backup.Annotations["sourceClusterVersion"] != expectedVersion {
		t.Errorf("Expected sourceClusterVersion annotation: 6.2.1, but got: %s", backup.Annotations["sourceClusterVersion"])
	}
	if backup.Annotations["sourceSentinelReplicasMaster"] != "1" {
		t.Errorf("Expected sourceSentinelReplicasMaster annotation: 1, but got: %s", backup.Annotations["sourceSentinelReplicasMaster"])
	}
	if backup.Annotations["sourceSentinelReplicasMaster"] != "1" {
		t.Errorf("Expected sourceSentinelReplicasMaster annotation: 1, but got: %s", backup.Annotations["sourceSentinelReplicasMaster"])
	}
	if backup.Annotations["sourceSentinelReplicasSlave"] != "0" {
		t.Errorf("Expected sourceSentinelReplicasSlave annotation: 0, but got: %s", backup.Annotations["sourceSentinelReplicasSlave"])
	}

	// Check labels
	if backup.Labels == nil {
		t.Error("Labels should not be nil")
	}
	if backup.Labels["redisfailovers.databases.spotahome.com/name"] != "redis-failover" {
		t.Errorf("Expected redisfailovers.databases.spotahome.com/name label: redis-failover, but got: %s", backup.Labels["redisfailovers.databases.spotahome.com/name"])
	}
}

func TestAddBackupAnnotationsAndLabels2(t *testing.T) {
	instance := &databasesv1.RedisFailover{
		ObjectMeta: metav1.ObjectMeta{
			Name: "redis-failover",
		},
		Status: databasesv1.RedisFailoverStatus{
			Phase: "Ready",
		},
		Spec: databasesv1.RedisFailoverSpec{

			Redis: databasesv1.RedisSettings{},
			Sentinel: databasesv1.SentinelSettings{
				Replicas: 1,
			},
		},
	}
	backup := &redisbackupv1.RedisBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "redis-backup",
		},
		Spec: redisbackupv1.RedisBackupSpec{
			Source: redisbackupv1.RedisBackupSource{
				RedisFailoverName: "redis-failover",
			},
		},
	}

	err := AddBackupAnnotationsAndLabels(instance, backup)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Check annotations
	if backup.Annotations == nil {
		t.Error("Annotations should not be nil")
	}
	expectedResources := "{}"
	if backup.Annotations["sourceResources"] != expectedResources {
		t.Errorf("Expected sourceResources annotation: %s, but got: %s", expectedResources, backup.Annotations["sourceResources"])
	}
	if backup.Annotations["sourceResources"] != expectedResources {
		t.Errorf("Expected sourceResources annotation: %s, but got: %s", expectedResources, backup.Annotations["sourceResources"])
	}
	if backup.Annotations["sourceSentinelReplicasMaster"] != "1" {
		t.Errorf("Expected sourceSentinelReplicasMaster annotation: 1, but got: %s", backup.Annotations["sourceSentinelReplicasMaster"])
	}
	if backup.Annotations["sourceSentinelReplicasSlave"] != "0" {
		t.Errorf("Expected sourceSentinelReplicasSlave annotation: 0, but got: %s", backup.Annotations["sourceSentinelReplicasSlave"])
	}

	// Check labels
	if backup.Labels == nil {
		t.Error("Labels should not be nil")
	}
	if backup.Labels["redisfailovers.databases.spotahome.com/name"] != "redis-failover" {
		t.Errorf("Expected redisfailovers.databases.spotahome.com/name label: redis-failover, but got: %s", backup.Labels["redisfailovers.databases.spotahome.com/name"])
	}
}
