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

package main

import (
	"flag"
	"os"

	databasesv1 "github.com/alauda/redis-operator/api/databases.spotahome.com/v1"
	clusterv1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	middlewarev1 "github.com/alauda/redis-operator/api/redis/v1"

	redisfailovercontrollers "github.com/alauda/redis-operator/internal/controller/databases.spotahome.com"
	middlewarecontrollers "github.com/alauda/redis-operator/internal/controller/redis"
	redismiddlewarealaudaiocontrollers "github.com/alauda/redis-operator/internal/controller/redis"
	rediskuncontrollers "github.com/alauda/redis-operator/internal/controller/redis.kun"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes/clientset"
	"github.com/alauda/redis-operator/pkg/ops"
	cactor "github.com/alauda/redis-operator/pkg/ops/cluster/actor"
	sactor "github.com/alauda/redis-operator/pkg/ops/sentinel/actor"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	smv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"go.uber.org/zap/zapcore"
	coordination "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(certv1.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(databasesv1.AddToScheme(scheme))
	utilruntime.Must(middlewarev1.AddToScheme(scheme))
	utilruntime.Must(smv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(middlewarev1.AddToScheme(scheme))
	utilruntime.Must(coordination.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

// custom rbac
//+kubebuilder:rbac:groups=apps,resources=statefulsets;statefulsets/finalizers;deployments;deployments/finalizers;daemonsets;replicasets,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups=batch,resources=jobs;jobs/finalizers;cronjobs;cronjobs/finalizers,verbs=get;list;watch;create;update;delete;deletecollection
//+kubebuilder:rbac:groups=*,resources=pods;pods/exec;configmaps;configmaps/finalizers;secrets;secrets/finalizers;services;services/finalizers;persistentvolumeclaims;persistentvolumeclaims/finalizers;endpoints,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=*,resources=events,verbs=get;list;watch;create;update;delete;deletecollection
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets;poddisruptionbudgets/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=*,resources=pods;pods/exec;configmaps;endpoints;services;services/finalizers,verbs=*

//+kubebuilder:rbac:groups=*,resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=*,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;clusterroles;rolebindings;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// referral
//+kubebuilder:rbac:groups=redis.kun,resources=*,verbs=*
//+kubebuilder:rbac:groups=redis.middleware.alauda.io,resources=*,verbs=*
//+kubebuilder:rbac:groups=middleware.alauda.io,resources=redis,verbs=*

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development:     true,
		StacktraceLevel: zapcore.FatalLevel,
		TimeEncoder:     zapcore.RFC3339TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Namespace:              os.Getenv("WATCH_NAMESPACE"),
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "926185dc.alauda.cn",
	})

	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	eventRecorder := mgr.GetEventRecorderFor("redis-operator")

	// init actor
	clientset := clientset.NewWithConfig(mgr.GetClient(), mgr.GetConfig(), mgr.GetLogger())

	actorManager := actor.NewActorManager()
	_ = actorManager.Add(sactor.NewEnsureResourceActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(sactor.NewSentinelHealActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(sactor.NewActorHealMasterActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(sactor.NewActorInitMasterActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(sactor.NewSentinelUpdateConfig(clientset, mgr.GetLogger()))
	_ = actorManager.Add(sactor.NewPatchLabelsActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(sactor.NewUpdateAccountActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(cactor.NewCleanResourceActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(cactor.NewEnsureResourceActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(cactor.NewEnsureSlotsActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(cactor.NewHealPodActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(cactor.NewJoinNodeActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(cactor.NewRebalanceActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(cactor.NewUpdateConfigActor(clientset, mgr.GetLogger()))
	_ = actorManager.Add(cactor.NewUpdateAccountActor(clientset, mgr.GetLogger()))

	engine, err := ops.NewOpEngine(mgr.GetClient(), eventRecorder, actorManager, mgr.GetLogger())
	if err != nil {
		setupLog.Error(err, "init op engine failed")
		os.Exit(1)
	}

	if err = (&middlewarecontrollers.RedisBackupReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Logger: mgr.GetLogger(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RedisBackup")
		os.Exit(1)
	}

	if err = (&rediskuncontrollers.DistributedRedisClusterReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: eventRecorder,
		Engine:        engine,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DistributedRedisCluster")
		os.Exit(1)
	}

	if err = (&redisfailovercontrollers.RedisFailoverReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: eventRecorder,
		Engine:        engine,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RedisFailover")
		os.Exit(1)
	}

	if err = (&middlewarecontrollers.RedisClusterBackupReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Logger: mgr.GetLogger(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RedisClusterBackup")
		os.Exit(1)
	}

	if err = (&middlewarev1.RedisUser{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "RedisUser")
		os.Exit(1)
	}

	if err = (&redismiddlewarealaudaiocontrollers.RedisUserReconciler{
		K8sClient: clientset,
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Logger:    mgr.GetLogger(),
		Record:    eventRecorder,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RedisUser")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
