// Package main is the entry point for the authentik-k8s-operator.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	_ "go.uber.org/automaxprocs"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	manager "sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	webhookserver "sigs.k8s.io/controller-runtime/pkg/webhook"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
	"github.com/JeffResc/authentik-k8s-operator/internal/controller"
	"github.com/JeffResc/authentik-k8s-operator/internal/webhook"
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
	var enableWebhook bool
	var webhookPort int
	var webhookCertDir string
	var requeueInterval time.Duration
	var enableEventWebhook bool
	var eventWebhookPort int
	var eventWebhookExternalURL string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&developmentMode, "development", false,
		"Enable development mode logging (human-readable output instead of JSON).")
	flag.BoolVar(&enableWebhook, "enable-webhook", false,
		"Enable the validating admission webhook for AuthentikOAuth2Application resources.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "The port the webhook server binds to.")
	flag.StringVar(&webhookCertDir, "webhook-cert-dir", "", "The directory containing TLS certificates for the webhook server.")
	flag.DurationVar(&requeueInterval, "requeue-interval", controller.DefaultRequeueDelay, "Interval between periodic drift detection reconciliations.")
	flag.BoolVar(&enableEventWebhook, "enable-event-webhook", false,
		"Enable the Authentik event webhook receiver for near-instant drift detection.")
	flag.IntVar(&eventWebhookPort, "event-webhook-port", 9444, "The port the event webhook receiver binds to.")
	flag.StringVar(&eventWebhookExternalURL, "event-webhook-external-url", "",
		"The external URL that Authentik will use to send event webhooks (e.g. http://operator.namespace.svc:9444/webhook).")

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
	if u, err := url.Parse(authentikURL); err != nil || u.Scheme == "" || u.Host == "" {
		setupLog.Error(err, "AUTHENTIK_URL is not a valid URL", "url", authentikURL)
		os.Exit(1)
	}
	if authentikToken == "" {
		setupLog.Error(nil, "AUTHENTIK_TOKEN environment variable is required")
		os.Exit(1)
	}

	mgrOpts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "authentik-operator.k8s.io",
	}
	if enableWebhook {
		mgrOpts.WebhookServer = webhookserver.NewServer(webhookserver.Options{
			Port:    webhookPort,
			CertDir: webhookCertDir,
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	reconciler := &controller.AuthentikOAuth2ApplicationReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorder("authentik-operator"),
		AuthentikURL:   authentikURL,
		AuthentikToken: authentikToken,
		RequeueDelay:   requeueInterval,
		NewAuthentikClient: func(baseURL, token string) (authentik.Client, error) {
			return authentik.NewClient(baseURL, token)
		},
	}

	// Set up the event webhook channel if enabled
	var eventChan chan event.GenericEvent
	if enableEventWebhook {
		if eventWebhookExternalURL == "" {
			setupLog.Error(nil, "--event-webhook-external-url is required when --enable-event-webhook is set")
			os.Exit(1)
		}
		eventChan = make(chan event.GenericEvent, 100)
		reconciler.EventChannel = eventChan
	}

	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AuthentikOAuth2Application")
		os.Exit(1)
	}

	if err = (&controller.AuthentikSAMLApplicationReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Recorder:       mgr.GetEventRecorder("authentik-operator"),
		AuthentikURL:   authentikURL,
		AuthentikToken: authentikToken,
		RequeueDelay:   requeueInterval,
		NewAuthentikClient: func(baseURL, token string) (authentik.Client, error) {
			return authentik.NewClient(baseURL, token)
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AuthentikSAMLApplication")
		os.Exit(1)
	}

	if enableWebhook {
		if err := authentikv1alpha1.SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AuthentikOAuth2Application")
			os.Exit(1)
		}
		if err := authentikv1alpha1.SetupSAMLWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AuthentikSAMLApplication")
			os.Exit(1)
		}
	}

	// Add health check for the manager
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	// Create a reusable client for readiness probes to avoid allocating
	// a new HTTP client and TCP connection on every probe request.
	readinessClient, err := authentik.NewClient(authentikURL, authentikToken)
	if err != nil {
		setupLog.Error(err, "unable to create Authentik client for readiness probe")
		os.Exit(1)
	}
	authentikReadyCheck := func(req *http.Request) error {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		return readinessClient.HealthCheck(ctx)
	}
	if err := mgr.AddReadyzCheck("readyz", authentikReadyCheck); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Start the event webhook receiver and register with Authentik
	if enableEventWebhook {
		receiver := webhook.NewReceiver(mgr.GetClient(), eventChan)
		mux := http.NewServeMux()
		mux.Handle("/webhook", receiver)
		eventServer := &http.Server{
			Addr:              fmt.Sprintf(":%d", eventWebhookPort),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}

		// Register the HTTP server as a manager Runnable so it is started
		// and stopped together with the controller manager.
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := eventServer.Shutdown(shutdownCtx); err != nil {
					setupLog.Error(err, "event webhook receiver shutdown error")
				}
			}()
			setupLog.Info("starting event webhook receiver", "port", eventWebhookPort)
			if err := eventServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("event webhook receiver failed: %w", err)
			}
			return nil
		})); err != nil {
			setupLog.Error(err, "unable to add event webhook receiver runnable")
			os.Exit(1)
		}

		// Register the webhook transport and notification rule in Authentik
		akClient, err := authentik.NewClient(authentikURL, authentikToken)
		if err != nil {
			setupLog.Error(err, "failed to create Authentik client for event webhook registration")
			os.Exit(1)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := akClient.EnsureEventWebhookConfig(ctx, eventWebhookExternalURL+"/webhook"); err != nil {
			setupLog.Error(err, "failed to register event webhook in Authentik")
			// Non-fatal: the operator can still work with polling-based drift detection
			setupLog.Info("event webhook registration failed, falling back to polling-only drift detection")
		} else {
			setupLog.Info("event webhook registered in Authentik", "url", eventWebhookExternalURL+"/webhook")
		}
		cancel()
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
