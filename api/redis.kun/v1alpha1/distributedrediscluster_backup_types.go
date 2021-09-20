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

package v1alpha1

import (
	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type BackupSourceSpec struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Arguments to the restore job
	Args []string `json:"args,omitempty"`
}

// RedisStorage defines the structure used to store the Redis Data
type RedisStorage struct {
	Size        resource.Quantity `json:"size"`
	Type        StorageType       `json:"type,omitempty"`
	Class       string            `json:"class"`
	DeleteClaim bool              `json:"deleteClaim,omitempty"`
}

// RedisRestore defines the structure used to restore the Redis Data
type RedisRestore struct {
	Image           string            `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	BackupName      string            `json:"backupName,omitempty"`
}

// RedisBackup defines the structure used to backup the Redis Data
type RedisBackup struct {
	Image    string     `json:"image,omitempty"`
	Schedule []Schedule `json:"schedule,omitempty"`
}

type Schedule struct {
	Name              string                        `json:"name,omitempty"`
	Schedule          string                        `json:"schedule"`
	Keep              int32                         `json:"keep"`
	KeepAfterDeletion bool                          `json:"keepAfterDeletion,omitempty"`
	Storage           RedisBackupStorage            `json:"storage"`
	Target            databasesv1.RedisBackupTarget `json:"target,omitempty"`
}

type RedisBackupStorage struct {
	StorageClassName string            `json:"storageClassName,omitempty"`
	Size             resource.Quantity `json:"size,omitempty"`
}
