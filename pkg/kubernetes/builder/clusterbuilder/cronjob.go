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

package clusterbuilder

import (
	"reflect"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
)

func IsCronJobChanged(newJob, oldJob *batchv1.CronJob, logger logr.Logger) bool {
	if (newJob == nil && oldJob != nil) || (newJob != nil && oldJob == nil) {
		logger.V(2).Info("cronjob work diff")
		return true
	}
	if newJob.Name != oldJob.Name ||
		!reflect.DeepEqual(newJob.Labels, oldJob.Labels) {
		logger.V(2).Info("cronjob labels diff")
		return true
	}

	// Spec
	if !reflect.DeepEqual(newJob.Spec.FailedJobsHistoryLimit, oldJob.Spec.FailedJobsHistoryLimit) ||
		!reflect.DeepEqual(newJob.Spec.SuccessfulJobsHistoryLimit, oldJob.Spec.SuccessfulJobsHistoryLimit) ||
		newJob.Spec.Schedule != oldJob.Spec.Schedule {
		logger.V(2).Info("cronjob schedule diff",
			"FailedJobsHistoryLimit", !reflect.DeepEqual(newJob.Spec.FailedJobsHistoryLimit, oldJob.Spec.FailedJobsHistoryLimit),
			"SuccessfulJobsHistoryLimit", !reflect.DeepEqual(newJob.Spec.SuccessfulJobsHistoryLimit, oldJob.Spec.SuccessfulJobsHistoryLimit),
			"Schedule", newJob.Spec.Schedule != oldJob.Spec.Schedule,
		)
		return true
	}

	// template
	oldTpl := oldJob.Spec.JobTemplate
	newTpl := newJob.Spec.JobTemplate
	if !reflect.DeepEqual(newTpl.Labels, oldTpl.Labels) {
		logger.V(2).Info("cronjob template labels diff")
		return true
	}
	return IsPodTemplasteChanged(&newTpl.Spec.Template, &oldTpl.Spec.Template, logger)
}
