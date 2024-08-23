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
	"context"
	"crypto/tls"
	"flag"
	"os"
	"strings"
	"time"

	clusterv1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
	middlewareredisv1 "github.com/alauda/redis-operator/api/middleware/redis/v1"
	middlewarev1 "github.com/alauda/redis-operator/api/middleware/v1"
	"github.com/alauda/redis-operator/internal/config"
	rediskuncontrollers "github.com/alauda/redis-operator/internal/controller/cluster"
	databasescontroller "github.com/alauda/redis-operator/internal/controller/databases"
	middlewarecontrollers "github.com/alauda/redis-operator/internal/controller/middleware"
	redismiddlewarealaudaiocontrollers "github.com/alauda/redis-operator/internal/controller/middleware"
	"github.com/alauda/redis-operator/internal/ops"
	_ "github.com/alauda/redis-operator/internal/ops/cluster/actor"
	_ "github.com/alauda/redis-operator/internal/ops/failover/actor"
	_ "github.com/alauda/redis-operator/internal/ops/sentinel/actor"
	"github.com/alauda/redis-operator/internal/vc"
	"github.com/alauda/redis-operator/pkg/actor"
	"github.com/alauda/redis-operator/pkg/kubernetes/clientset"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/samber/lo"
	"go.uber.org/zap/zapcore"
	coordination "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(coordination.AddToScheme(scheme))
	utilruntime.Must(certv1.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(databasesv1.AddToScheme(scheme))
	utilruntime.Must(middlewarev1.AddToScheme(scheme))
	utilruntime.Must(middlewareredisv1.AddToScheme(scheme))
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
	var secureMetrics bool
	var enableHTTP2 bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false, "If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development:     true,
		StacktraceLevel: zapcore.FatalLevel,
		TimeEncoder:     zapcore.RFC3339TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}
	webhookServer := webhook.NewServer(webhook.Options{TLSOpts: tlsOpts, Port: 9443})
	cacheOpts := cache.Options{Scheme: scheme}

	var namespaces []string
	{
		watchedNamespaces := lo.FirstOr([]string{os.Getenv("WATCH_ALL_NAMESPACES")}, os.Getenv("WATCH_NAMESPACE"))
		if watchedNamespaces != "" {
			namespaces = strings.Split(watchedNamespaces, ",")
			cacheOpts.DefaultNamespaces = map[string]cache.Config{}
			for _, ns := range namespaces {
				cacheOpts.DefaultNamespaces[ns] = cache.Config{}
			}
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		Cache:                  cacheOpts,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "926185dc.alauda.cn",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	noCacheCli, err := client.New(mgr.GetConfig(), client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create client")
		os.Exit(1)
	}

	eventRecorder := mgr.GetEventRecorderFor("redis-operator")
	// init actor
	clientset := clientset.NewWithConfig(noCacheCli, mgr.GetConfig(), mgr.GetLogger())
	actorManager := actor.NewActorManager(clientset, setupLog.WithName("ActorManager"))
	engine, err := ops.NewOpEngine(noCacheCli, eventRecorder, actorManager, mgr.GetLogger())
	if err != nil {
		setupLog.Error(err, "init op engine failed")
		os.Exit(1)
	}

	if err = (&databasescontroller.RedisFailoverReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: eventRecorder,
		Engine:        engine,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RedisFailover")
		os.Exit(1)
	}
	if err = (&databasescontroller.RedisSentinelReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: eventRecorder,
		Engine:        engine,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RedisSentinel")
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

	if err = (&middlewarecontrollers.RedisReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Logger:       mgr.GetLogger(),
		ActorManager: actorManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Redis")
		os.Exit(1)
	}
	if err = (&middlewarev1.Redis{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Redis")
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
	if err = (&middlewareredisv1.RedisUser{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "RedisUser")
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

	if err := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		logger := setupLog.WithName("BundleVersion")
		setupLog.Info("check old bundle version")
		if len(namespaces) > 0 {
			for _, namespace := range namespaces {
				if err := vc.InstallOldImageVersion(ctx, noCacheCli, namespace, logger); err != nil {
					setupLog.Error(err, "install old image version failed")
					return err
				}
			}
		} else {
			if err := vc.InstallOldImageVersion(ctx, noCacheCli, "", logger); err != nil {
				setupLog.Error(err, "install old image version failed")
				return err
			}
		}
		setupLog.Info("install current bundle version", "version", config.GetOperatorVersion())
		if err := vc.InstallCurrentImageVersion(ctx, noCacheCli, logger); err != nil {
			setupLog.Error(err, "install current image version failed")
			return err
		}
		setupLog.Info("reset latest bundle version")
		if err := vc.CleanImageVersions(ctx, noCacheCli, logger); err != nil {
			setupLog.Error(err, "clean bundle versions failed")
			return err
		}
		return nil
	}(); err != nil {
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	actorManager.Print()
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
