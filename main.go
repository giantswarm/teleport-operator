/*
Copyright 2023.

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
	"flag"
	"os"
	"strconv"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/giantswarm/teleport-operator/internal/controller"
	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleport"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(capi.AddToScheme(scheme))
	utilruntime.Must(appv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var enableTeleportBot bool
	var probeAddr string
	var namespace string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enableTeleportBot, "tbot", false,
		"Enable teleport bot for teleport-operator. "+
			"Enabling this will ensure teleport bot configmap is created and app.spec.extraConfig is updated.")
	flag.StringVar(&namespace, "namespace", "", "Namespace where operator is deployed")
	opts := zap.Options{
		Development: false,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	restConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "765979b4.giantswarm.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctrlClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := context.Background()
	config, err := config.GetConfigFromConfigMap(ctx, ctrlClient, namespace)
	if err != nil {
		setupLog.Error(err, "unable to get secret config")
		os.Exit(1)
	}

	useCombinedTokens, err := strconv.ParseBool(os.Getenv("USE_COMBINED_TOKENS"))
	if err != nil {
		useCombinedTokens = false
	}
	tele := teleport.New(namespace, config, token.NewGenerator())
	if err = (&controller.ClusterReconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("Cluster"),
		Scheme:            mgr.GetScheme(),
		Teleport:          tele,
		IsBotEnabled:      enableTeleportBot,
		Namespace:         namespace,
		UseCombinedTokens: useCombinedTokens,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster")
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

	setupLog.Info("is teleport bot enabled?", "enabled", enableTeleportBot)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
