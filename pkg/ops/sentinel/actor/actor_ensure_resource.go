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

package actor

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"time"

	v1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	"github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	redisbackup "github.com/alauda/redis-operator/api/redis/v1"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/config"
	"github.com/alauda/redis-operator/pkg/kubernetes"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/clusterbuilder"
	"github.com/alauda/redis-operator/pkg/kubernetes/builder/sentinelbuilder"
	"github.com/alauda/redis-operator/pkg/ops/sentinel"
	sops "github.com/alauda/redis-operator/pkg/ops/sentinel"
	"github.com/alauda/redis-operator/pkg/types"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/alauda/redis-operator/pkg/util"
	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ actor.Actor = (*actorEnsureResource)(nil)

func NewEnsureResourceActor(client kubernetes.ClientSet, logger logr.Logger) actor.Actor {
	return &actorEnsureResource{
		client: client,
		logger: logger,
	}
}

type actorEnsureResource struct {
	client kubernetes.ClientSet
	logger logr.Logger
}

func (a *actorEnsureResource) SupportedCommands() []actor.Command {
	return []actor.Command{sentinel.CommandEnsureResource}
}

// Do
func (a *actorEnsureResource) Do(ctx context.Context, val types.RedisInstance) *actor.ActorResult {
	sentinel := val.(types.RedisFailoverInstance)
	logger := a.logger.WithName(sops.CommandEnsureResource.String()).WithValues("namespace", sentinel.GetNamespace(), "name", sentinel.GetName())

	if (sentinel.Definition().Spec.Redis.PodAnnotations != nil) && sentinel.Definition().Spec.Redis.PodAnnotations[config.PAUSE_ANNOTATION_KEY] != "" {

		if ret := a.pauseStatefulSet(ctx, sentinel, logger); ret != nil {
			return ret
		}
		if ret := a.pauseBackupCronJob(ctx, sentinel, logger); ret != nil {
			return ret
		}
		if ret := a.pauseDeployment(ctx, sentinel, logger); ret != nil {
			return ret
		}
		return actor.NewResult(sops.CommandPaused)
	}
	if ret := a.ensureServiceAccount(ctx, sentinel, logger); ret != nil {
		return ret
	}
	if ret := a.ensureService(ctx, sentinel, logger); ret != nil {
		return ret
	}

	// ensure extra 资源,
	if ret := a.ensureExtraResource(ctx, sentinel, logger); ret != nil {
		return ret
	}
	// ensure secret
	// if ret := a.ensureRedisUser(ctx, sentinel, logger); ret != nil {
	// 	return ret
	// }

	// ensure configMap
	if ret := a.ensureConfigMap(ctx, sentinel, logger); ret != nil {
		return ret
	}

	// ensure statefulSet
	if ret := a.ensureRedisStatefulSet(ctx, sentinel, logger); ret != nil {
		return ret
	}

	// ensure deployment

	if ret := a.ensureRedisDeployment(ctx, sentinel, logger); ret != nil {
		return ret
	}

	if ret := a.ensureBackupSchedule(ctx, sentinel, logger); ret != nil {
		return ret
	}

	return nil
}

func (a *actorEnsureResource) ensureRedisDeployment(ctx context.Context, sentinel types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := sentinel.Definition()
	selector := sentinel.Selector()
	if ret := a.ensureDeployPodDisruptionBudget(ctx, cr, logger, selector); ret != nil {
		return ret
	}

	deploy := sentinelbuilder.GenerateSentinelDeployment(cr, selector)
	// get old deployment
	oldDeploy, err := a.client.GetDeployment(ctx, cr.Namespace, deploy.Name)
	if errors.IsNotFound(err) {
		if err := a.client.CreateDeployment(ctx, cr.Namespace, deploy); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
		return nil
	} else if err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if sentinelbuilder.DiffDeployment(deploy, oldDeploy, logger) {
		err = a.client.UpdateDeployment(ctx, cr.Namespace, deploy)
		if err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisStatefulSet(ctx context.Context, sentinel types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := sentinel.Definition()
	selector := sentinel.Selector()
	// ensure Sentinel statefulSet
	if ret := a.ensurePodDisruptionBudget(ctx, cr, logger, selector); ret != nil {
		return ret
	}
	isRestoring := func(sts *appv1.StatefulSet) bool {
		if sts == nil {
			return true
		}
		if util.GetContainerByName(&sts.Spec.Template.Spec, sentinelbuilder.RestoreContainerName) == nil {
			return true
		}
		return false
	}
	backupResourceExists := false
	var backup *redisbackup.RedisBackup
	if cr.Spec.Redis.Restore.BackupName != "" {
		var err error
		backup, err = a.client.GetRedisBackup(ctx, cr.Namespace, cr.Spec.Redis.Restore.BackupName)
		if err == nil {
			backupResourceExists = true
		} else {
			logger.Info("backup resource not found", "backupName", cr.Spec.Redis.Restore.BackupName)
		}
	}
	sts := sentinelbuilder.GenerateRedisStatefulSet(cr, backup, selector, "")
	if sentinel.Version().IsACLSupported() {
		sts = sentinelbuilder.GenerateRedisStatefulSet(cr, backup, selector, sentinel.Users().GetOpUser().Password.SecretName)
	}
	err := a.client.CreateIfNotExistsStatefulSet(ctx, cr.Namespace, sts)
	if err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}

	oldSts, err := a.client.GetStatefulSet(ctx, cr.Namespace, sts.Name)
	if errors.IsNotFound(err) {
		err := a.client.CreateStatefulSet(ctx, cr.Namespace, sts)
		if err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
		return nil
	} else if err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if !backupResourceExists && isRestoring(sts) {
		sts = sentinelbuilder.GenerateRedisStatefulSet(cr, backup, selector, "")
		if sentinel.Version().IsACLSupported() {
			sts = sentinelbuilder.GenerateRedisStatefulSet(cr, nil, selector, sentinel.Users().GetOpUser().Password.SecretName)
		}
	}
	if !reflect.DeepEqual(oldSts.Spec.Selector.MatchLabels, sts.Spec.Selector.MatchLabels) {
		sts.Spec.Selector.MatchLabels = oldSts.Spec.Selector.MatchLabels
	}
	if clusterbuilder.IsRedisClusterStatefulsetChanged(sts, oldSts, logger) {
		if err = a.client.UpdateStatefulSet(ctx, cr.Namespace, sts); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureDeployPodDisruptionBudget(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	pdb := sentinelbuilder.NewDeployPodDisruptionBudgetForCR(rf, selectors)

	if oldPdb, err := a.client.GetPodDisruptionBudget(context.TODO(), rf.Namespace, pdb.Name); errors.IsNotFound(err) {
		if err := a.client.CreatePodDisruptionBudget(ctx, rf.Namespace, pdb); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	} else if err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	} else if !reflect.DeepEqual(oldPdb.Spec.Selector, pdb.Spec.Selector) {
		oldPdb.Labels = pdb.Labels
		if err := a.client.UpdatePodDisruptionBudget(ctx, rf.Namespace, oldPdb); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensurePodDisruptionBudget(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selectors map[string]string) *actor.ActorResult {

	pdb := sentinelbuilder.NewPodDisruptionBudgetForCR(rf, selectors)

	if oldPdb, err := a.client.GetPodDisruptionBudget(context.TODO(), rf.Namespace, pdb.Name); errors.IsNotFound(err) {
		if err := a.client.CreatePodDisruptionBudget(ctx, rf.Namespace, pdb); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	} else if err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	} else if !reflect.DeepEqual(oldPdb.Spec.Selector, pdb.Spec.Selector) {
		oldPdb.Labels = pdb.Labels
		oldPdb.Spec.Selector = pdb.Spec.Selector
		if err := a.client.UpdatePodDisruptionBudget(ctx, rf.Namespace, oldPdb); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisUser(ctx context.Context, sentinel types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	if !sentinel.Version().IsACLSupported() {
		return nil
	}
	cr := sentinel.Definition()
	secret := sentinelbuilder.NewSentinelOpSecret(cr)
	if err := a.client.CreateIfNotExistsSecret(ctx, cr.Namespace, secret); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	for _, _user := range sentinel.Users() {
		if _user.Name == user.DefaultUserName {
			ru := sentinelbuilder.GenerateSentinelDefaultRedisUser(sentinel.Definition(), sentinel.Definition().Spec.Auth.SecretPath)
			oldRu, err := a.client.GetRedisUser(ctx, sentinel.GetNamespace(), ru.Name)
			if err == nil {
				oldRu.Spec.PasswordSecrets = ru.Spec.PasswordSecrets
				ru = *oldRu
			}
			if err := a.client.CreateOrUpdateRedisUser(ctx, &ru); err != nil {
				a.logger.Error(err, "update default user redisUser failed")
			}
		}
		if _user.Name == user.DefaultOperatorUserName {
			ru := sentinelbuilder.GenerateSentinelOperatorsRedisUser(sentinel, _user.Password.SecretName)

			oldRu, err := a.client.GetRedisUser(ctx, sentinel.GetNamespace(), ru.Name)
			if err == nil {
				oldRu.Spec.PasswordSecrets = ru.Spec.PasswordSecrets
				ru = *oldRu
			}
			if err := a.client.CreateOrUpdateRedisUser(ctx, &ru); err != nil {
				a.logger.Error(err, "update operator user redisUser failed")
			}
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureConfigMap(ctx context.Context, sentinel types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := sentinel.Definition()
	selector := sentinel.Selector()
	// ensure Sentinel configMap
	if ret := a.ensureSentinelConfigMap(ctx, cr, logger, selector); ret != nil {
		return ret
	}
	// ensure Redis configMap
	if ret := a.ensureRedisConfigMap(ctx, sentinel, logger, selector); ret != nil {
		return ret
	}

	// ensure Redis script configMap
	if ret := a.ensureRedisScriptConfigMap(ctx, cr, logger, selector); ret != nil {
		return ret
	}

	if ret := sentinelbuilder.NewSentinelAclConfigMap(cr, sentinel.Users().Encode()); ret != nil {
		if err := a.client.CreateIfNotExistsConfigMap(ctx, cr.Namespace, ret); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureSentinelConfigMap(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selectors map[string]string) *actor.ActorResult {

	//ensure sentinel config
	senitnelConfigMap := sentinelbuilder.NewSentinelConfigMap(rf, selectors)
	err := a.client.CreateIfNotExistsConfigMap(ctx, rf.Namespace, senitnelConfigMap)
	if err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.UpdateIfConfigMapChanged(ctx, senitnelConfigMap); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}

	//ensure sentinel probe config
	senitnelProbeConfigMap := sentinelbuilder.NewSentinelProbeConfigMap(rf, selectors)
	if err = a.client.CreateIfNotExistsConfigMap(ctx, rf.Namespace, senitnelProbeConfigMap); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.UpdateIfConfigMapChanged(ctx, senitnelProbeConfigMap); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}

	return nil
}

func (a *actorEnsureResource) ensureRedisConfigMap(ctx context.Context, st types.RedisFailoverInstance, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	rf := st.Definition()
	if ret := a.ensureRedisConfig(ctx, st, logger, selectors); ret != nil {
		return ret
	}
	if ret := a.ensureRedisScriptConfigMap(ctx, rf, logger, selectors); ret != nil {
		return ret
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisConfig(ctx context.Context, st types.RedisFailoverInstance, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	rf := st.Definition()
	configMap := sentinelbuilder.NewRedisConfigMap(st, selectors)
	if err := a.client.CreateIfNotExistsConfigMap(ctx, rf.Namespace, configMap); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.UpdateIfConfigMapChanged(ctx, configMap); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisScriptConfigMap(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selector map[string]string) *actor.ActorResult {
	configMap := sentinelbuilder.NewRedisScriptConfigMap(rf, selector)
	if err := a.client.CreateIfNotExistsConfigMap(ctx, rf.Namespace, configMap); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.UpdateIfConfigMapChanged(ctx, configMap); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	return nil
}

func (a *actorEnsureResource) ensureExtraResource(ctx context.Context, sentinel types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := sentinel.Definition()
	selectors := sentinel.Selector()
	// ensure Redis ssl
	if cr.Spec.EnableTLS {
		if ret := a.ensureRedisSSL(ctx, cr, logger, selectors); ret != nil {
			return ret
		}
	}
	if ret := a.ensureServiceMonitor(ctx, cr, logger); ret != nil {
		return ret
	}

	return nil
}

func (a *actorEnsureResource) ensureServiceMonitor(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger) *actor.ActorResult {
	if !rf.Spec.Redis.Exporter.Enabled {
		return nil
	}
	watchNamespace := os.Getenv("WATCH_NAMESPACE")
	if watchNamespace == "" {
		watchNamespace = "operators"
	}
	sentinelLabels := map[string]string{
		"app.kubernetes.io/part-of": "redis-failover",
	}
	sentinelSM := sentinelbuilder.NewServiceMonitorForCR(rf, sentinelLabels)
	sentinelSM.Namespace = watchNamespace
	if oldsm, err := a.client.GetServiceMonitor(ctx, watchNamespace, sentinelSM.Name); err != nil {
		if errors.IsNotFound(err) {
			if err := a.client.CreateServiceMonitor(context.TODO(), watchNamespace, sentinelSM); err != nil {
				logger.Info(err.Error())
			}
		}
		logger.Info(err.Error())
	} else {
		if len(oldsm.OwnerReferences) == 0 {
			if err := a.client.CreateOrUpdateServiceMonitor(context.TODO(), watchNamespace, sentinelSM); err != nil {
				logger.Info(err.Error())
			}
		}
		if len(oldsm.Spec.Endpoints) == 0 {
			if err := a.client.CreateOrUpdateServiceMonitor(context.TODO(), watchNamespace, sentinelSM); err != nil {
				logger.Info(err.Error())
			}
		} else {
			if len(oldsm.Spec.Endpoints[0].MetricRelabelConfigs) == 0 {
				if err := a.client.CreateOrUpdateServiceMonitor(context.TODO(), watchNamespace, sentinelSM); err != nil {
					logger.Info(err.Error())
				}
			}
			if rf.Spec.ServiceMonitor.CustomMetricRelabelings {
				if err := a.client.CreateOrUpdateServiceMonitor(context.TODO(), watchNamespace, sentinelSM); err != nil {
					logger.Info(err.Error())
				}
			}
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisSSL(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	cert := sentinelbuilder.NewCertificate(rf, selectors)
	if err := a.client.CreateIfNotExistsCertificate(ctx, rf.Namespace, cert); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	oldCert, err := a.client.GetCertificate(ctx, rf.Namespace, cert.GetName())
	if err != nil && !errors.IsNotFound(err) {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	var (
		found      = false
		secretName = sentinelbuilder.GetRedisSSLSecretName(rf.Name)
	)
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second * time.Duration(i))

		if secret, _ := a.client.GetSecret(ctx, rf.Namespace, secretName); secret != nil {
			found = true
			break
		}

		// check when the certificate created
		if time.Now().Sub(oldCert.GetCreationTimestamp().Time) > time.Minute*5 {
			return actor.NewResultWithError(sops.CommandAbort, fmt.Errorf("issue for tls certificate failed, please check the cert-manager"))
		}
	}
	if !found {
		return actor.NewResult(sops.CommandRequeue)
	}

	return nil
}

func (a *actorEnsureResource) pauseDeployment(ctx context.Context, st types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := st.Definition()
	name := sentinelbuilder.GetSentinelDeploymentName(cr.Name)
	if deploy, err := a.client.GetDeployment(ctx, cr.Namespace, name); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return actor.NewResultWithError(sops.CommandRequeue, err)
	} else {
		if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas == 0 {
			return nil
		}
		*deploy.Spec.Replicas = 0
		if err = a.client.UpdateDeployment(ctx, cr.Namespace, deploy); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) pauseStatefulSet(ctx context.Context, st types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := st.Definition()
	name := sentinelbuilder.GetSentinelStatefulSetName(cr.Name)
	if sts, err := a.client.GetStatefulSet(ctx, cr.Namespace, name); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return actor.NewResultWithError(sops.CommandRequeue, err)
	} else {
		if sts.Spec.Replicas == nil || *sts.Spec.Replicas == 0 {
			return nil
		}
		*sts.Spec.Replicas = 0
		if err = a.client.UpdateStatefulSet(ctx, cr.Namespace, sts); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}
	return nil
}

func (a *actorEnsureResource) pauseBackupCronJob(ctx context.Context, st types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := st.Definition()
	selectorLabels := map[string]string{
		"redisfailovers.databases.spotahome.com/name": cr.Name,
	}
	jobsRes, err := a.client.ListCronJobs(ctx, cr.GetNamespace(), client.ListOptions{
		LabelSelector: labels.SelectorFromSet(selectorLabels),
	})
	if err != nil {
		logger.Error(err, "load cronjobs failed")
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	for _, val := range jobsRes.Items {
		if err := a.client.DeleteCronJob(ctx, cr.GetNamespace(), val.Name); err != nil {
			logger.Error(err, "delete cronjob failed", "target", client.ObjectKeyFromObject(&val))
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureServiceAccount(ctx context.Context, sentinel types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := sentinel.Definition()
	sa := clusterbuilder.NewServiceAccount(cr)
	role := clusterbuilder.NewRole(cr)
	binding := clusterbuilder.NewRoleBinding(cr)
	clusterRole := clusterbuilder.NewClusterRole(cr)
	clusterRoleBinding := clusterbuilder.NewClusterRoleBinding(cr)

	if err := a.client.CreateOrUpdateServiceAccount(ctx, sentinel.GetNamespace(), sa); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.CreateOrUpdateRole(ctx, sentinel.GetNamespace(), role); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.CreateOrUpdateRoleBinding(ctx, sentinel.GetNamespace(), binding); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.CreateOrUpdateClusterRole(ctx, clusterRole); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if oldClusterRb, err := a.client.GetClusterRoleBinding(ctx, clusterRoleBinding.Name); err != nil {
		if errors.IsNotFound(err) {
			if err := a.client.CreateClusterRoleBinding(ctx, clusterRoleBinding); err != nil {
				return actor.NewResultWithError(sops.CommandRequeue, err)
			}
		} else {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	} else {
		exists := false
		for _, sub := range oldClusterRb.Subjects {
			if sub.Namespace == sentinel.GetNamespace() {
				exists = true
			}
		}
		if !exists && len(oldClusterRb.Subjects) > 0 {
			oldClusterRb.Subjects = append(oldClusterRb.Subjects,
				rbacv1.Subject{Kind: "ServiceAccount",
					Name:      util.RedisBackupServiceAccountName,
					Namespace: sentinel.GetNamespace()},
			)
			err := a.client.CreateOrUpdateClusterRoleBinding(ctx, oldClusterRb)
			if err != nil {
				return actor.NewResultWithError(sops.CommandRequeue, err)
			}
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureService(ctx context.Context, sentinel types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := sentinel.Definition()
	//read write svc
	rwSvc := sentinelbuilder.NewRWSvcForCR(cr)
	roSvc := sentinelbuilder.NewReadOnlyForCR(cr)
	if err := a.client.CreateIfNotExistsService(ctx, sentinel.GetNamespace(), rwSvc); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.UpdateIfSelectorChangedService(ctx, sentinel.GetNamespace(), rwSvc); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.CreateIfNotExistsService(ctx, sentinel.GetNamespace(), roSvc); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	if err := a.client.UpdateIfSelectorChangedService(ctx, sentinel.GetNamespace(), roSvc); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}

	selector := sentinel.Selector()
	senService := sentinelbuilder.NewSentinelServiceForCR(cr, selector)
	oldService, err := a.client.GetService(ctx, sentinel.GetNamespace(), senService.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := a.client.CreateIfNotExistsService(ctx, sentinel.GetNamespace(), senService); err != nil {
				return actor.NewResultWithError(sops.CommandRequeue, err)
			}
		} else {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	} else {
		// Update the old service or custom port service
		if senService.Spec.Ports[0].NodePort != oldService.Spec.Ports[0].NodePort || !reflect.DeepEqual(senService.Spec.Selector, oldService.Spec.Selector) {
			if err := a.client.UpdateService(ctx, sentinel.GetNamespace(), senService); err != nil {
				return actor.NewResultWithError(sops.CommandRequeue, err)
			}
		}
		if a.client.UpdateIfSelectorChangedService(ctx, sentinel.GetNamespace(), senService); err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}

	exporterService := sentinelbuilder.NewExporterServiceForCR(cr, selector)
	if err := a.client.CreateIfNotExistsService(ctx, sentinel.GetNamespace(), exporterService); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}

	if err := a.client.UpdateIfSelectorChangedService(ctx, sentinel.GetNamespace(), exporterService); err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}

	if ret := a.ensureRedisNodePortService(ctx, cr, logger, selector); ret != nil {
		return ret
	}
	return nil
}

func (a *actorEnsureResource) ensureRedisNodePortService(ctx context.Context, rf *v1.RedisFailover, logger logr.Logger, selectors map[string]string) *actor.ActorResult {
	if len(rf.Spec.Expose.DataStorageNodePortMap) == 0 && rf.Spec.Expose.DataStorageNodePortSequence == "" {
		return nil
	}
	if !rf.Spec.Expose.EnableNodePort {
		return nil
	}

	if len(rf.Spec.Expose.DataStorageNodePortMap) > 0 {
		logger.Info("EnsureRedisNodePortService DataStorageNodePortMap", "Namepspace", rf.Namespace, "Name", rf.Name)
		need_svc_list := []string{}
		for index, port := range rf.Spec.Expose.DataStorageNodePortMap {
			svc := sentinelbuilder.NewRedisNodePortService(rf, index, port, selectors)

			need_svc_list = append(need_svc_list, svc.Name)
			old_svc, err := a.client.GetService(ctx, rf.Namespace, svc.Name)
			if err != nil {
				if errors.IsNotFound(err) {
					_err := a.client.CreateIfNotExistsService(ctx, rf.Namespace, svc)
					if _err != nil {
						return actor.NewResultWithError(sops.CommandRequeue, _err)
					} else {
						continue
					}
				} else {
					return actor.NewResultWithError(sops.CommandRequeue, err)
				}
			}
			if old_svc.Spec.Ports[0].NodePort != port {
				old_svc.Spec.Ports[0].NodePort = port
				if len(old_svc.OwnerReferences) > 0 && old_svc.OwnerReferences[0].Kind == "Pod" {
					if err := a.client.DeletePod(ctx, rf.Namespace, old_svc.Name); err != nil {
						logger.Error(err, "DeletePod")
						return actor.NewResultWithError(sops.CommandRequeue, err)
					}
				} else {
					if err := a.client.CreateOrUpdateService(ctx, rf.Namespace, old_svc); err != nil {
						if !errors.IsNotFound(err) {
							logger.Error(err, "CreateOrUpdateService")
							return actor.NewResultWithError(sops.CommandRequeue, err)
						}
					}
				}
			}
			if !reflect.DeepEqual(old_svc.Labels, svc.Labels) {
				old_svc.Labels = svc.Labels
				old_svc.Spec.Selector = svc.Spec.Selector
				if err := a.client.UpdateService(ctx, rf.Namespace, old_svc); err != nil {
					logger.Error(err, "Update servcie labels failed")
				}
			}
		}
		//回收多余端口
		svcs, err := a.client.GetServiceByLabels(ctx, rf.Namespace, selectors)
		if err != nil {
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
		for _, svc := range svcs.Items {
			if svc.Spec.Selector["statefulset.kubernetes.io/pod-name"] != "" {
				if !slices.Contains(need_svc_list, svc.Name) {
					if _, err := a.client.GetPod(ctx, rf.Namespace, svc.Spec.Selector["statefulset.kubernetes.io/pod-name"]); errors.IsNotFound(err) {
						if err := a.client.DeleteService(ctx, rf.Namespace, svc.Name); err != nil {
							return actor.NewResultWithError(sops.CommandRequeue, err)
						}
					}
				}
			}
		}
	}

	svcs, err := a.client.GetServiceByLabels(ctx, rf.Namespace, selectors)
	if err != nil {
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}

	for _, svc := range svcs.Items {
		if svc.Spec.Selector["statefulset.kubernetes.io/pod-name"] != "" {
			pod, err := a.client.GetPod(ctx, rf.Namespace, svc.Spec.Selector["statefulset.kubernetes.io/pod-name"])
			if err == nil {
				if pod.Labels["middleware.alauda.io/announce_port"] != "" &&
					pod.Labels["middleware.alauda.io/announce_port"] != strconv.Itoa(int(svc.Spec.Ports[0].NodePort)) {
					_err := a.client.DeletePod(ctx, rf.Namespace, pod.Name)
					if _err != nil {
						return actor.NewResultWithError(sops.CommandRequeue, _err)
					}
				}

			}
		}
	}
	return nil
}

func (a *actorEnsureResource) ensureBackupSchedule(ctx context.Context, st types.RedisFailoverInstance, logger logr.Logger) *actor.ActorResult {
	cr := st.Definition()
	selectors := st.Selector()

	for _, schedule := range cr.Spec.Redis.Backup.Schedule {
		ls := map[string]string{
			"redisfailovers.databases.spotahome.com/name":         cr.Name,
			"redisfailovers.databases.spotahome.com/scheduleName": schedule.Name,
		}

		res, err := a.client.ListRedisBackups(ctx, cr.GetNamespace(), client.ListOptions{
			LabelSelector: labels.SelectorFromSet(ls),
		})
		if err != nil {
			logger.Error(err, "load backups failed", "target", client.ObjectKeyFromObject(cr))
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
		sort.SliceStable(res.Items, func(i, j int) bool {
			return res.Items[i].GetCreationTimestamp().After(res.Items[j].GetCreationTimestamp().Time)
		})
		for i := len(res.Items) - 1; i >= int(schedule.Keep); i-- {
			item := res.Items[i]
			if err := a.client.DeleteRedisBackup(ctx, item.GetNamespace(), item.GetName()); err != nil {
				logger.V(2).Error(err, "clean old backup failed", "target", client.ObjectKeyFromObject(&item))
			}
		}
	}

	// check backup schedule
	var desiredCronJob []string
	for _, b := range cr.Spec.Redis.Backup.Schedule {
		desiredCronJob = append(desiredCronJob, clusterbuilder.GenerateCronJobName(cr.Name, b.Name))
	}

	selectorLabels := map[string]string{
		"redisfailovers.databases.spotahome.com/name": cr.Name,
	}
	jobsRes, err := a.client.ListCronJobs(ctx, cr.GetNamespace(), client.ListOptions{
		LabelSelector: labels.SelectorFromSet(selectorLabels),
	})
	if err != nil {
		logger.Error(err, "load cronjobs failed")
		return actor.NewResultWithError(sops.CommandRequeue, err)
	}
	jobsMap := map[string]*batchv1.CronJob{}
	for _, item := range jobsRes.Items {
		jobsMap[item.Name] = &item
	}
	for _, sched := range cr.Spec.Redis.Backup.Schedule {
		sc := v1alpha1.Schedule{
			Name:              sched.Name,
			Schedule:          sched.Schedule,
			Keep:              sched.Keep,
			KeepAfterDeletion: sched.KeepAfterDeletion,
			Storage:           v1alpha1.RedisBackupStorage(sched.Storage),
			Target:            sched.Target,
		}
		job := sentinelbuilder.NewRedisFailoverBackupCronJobFromCR(sc, cr, selectors)
		if oldJob := jobsMap[job.GetName()]; oldJob != nil {
			if clusterbuilder.IsCronJobChanged(job, oldJob, logger) {
				if err := a.client.UpdateCronJob(ctx, oldJob.GetNamespace(), job); err != nil {
					logger.Error(err, "update cronjob failed", "target", client.ObjectKeyFromObject(oldJob))
					return actor.NewResultWithError(sops.CommandRequeue, err)
				}
			}
			jobsMap[job.GetName()] = nil
		} else if err := a.client.CreateCronJob(ctx, cr.GetNamespace(), job); err != nil {
			logger.Error(err, "create redisbackup cronjob failed", "target", client.ObjectKeyFromObject(job))
			return actor.NewResultWithError(sops.CommandRequeue, err)
		}
	}

	for name, val := range jobsMap {
		if val == nil {
			continue
		}
		if err := a.client.DeleteCronJob(ctx, cr.GetNamespace(), name); err != nil {
			logger.Error(err, "delete cronjob failed", "target", client.ObjectKeyFromObject(val))
		}
	}
	return nil
}
