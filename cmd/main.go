// Package main is the entry point for the authentik-k8s-operator.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
	"github.com/JeffResc/authentik-k8s-operator/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(authentikv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var developmentMode bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&developmentMode, "development", false,
		"Enable development mode logging (human-readable output instead of JSON).")

	opts := zap.Options{
		Development: developmentMode,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Allow environment variable override for development mode
	if os.Getenv("DEVELOPMENT_MODE") == "true" {
		opts.Development = true
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Get Authentik configuration from environment
	authentikURL := os.Getenv("AUTHENTIK_URL")
	authentikToken := os.Getenv("AUTHENTIK_TOKEN")

	if authentikURL == "" {
		setupLog.Error(nil, "AUTHENTIK_URL environment variable is required")
		os.Exit(1)
	}
	if authentikToken == "" {
		setupLog.Error(nil, "AUTHENTIK_TOKEN environment variable is required")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "authentik-operator.k8s.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.AuthentikApplicationReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		AuthentikURL:   authentikURL,
		AuthentikToken: authentikToken,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AuthentikApplication")
		os.Exit(1)
	}

	// Add health check for the manager
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	// Add readiness check that verifies Authentik connectivity
	authentikReadyCheck := func(req *http.Request) error {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		client, err := authentik.NewClient(authentikURL, authentikToken)
		if err != nil {
			return err
		}
		return client.HealthCheck(ctx)
	}
	if err := mgr.AddReadyzCheck("readyz", authentikReadyCheck); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
