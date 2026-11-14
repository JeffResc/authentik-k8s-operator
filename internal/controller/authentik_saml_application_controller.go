package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/prometheus/client_golang/prometheus"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
	"github.com/JeffResc/authentik-k8s-operator/internal/template"
)

const (
	// SAMLFinalizerName is the finalizer name for AuthentikSAMLApplication resources
	SAMLFinalizerName = "goauthentik.io/saml-finalizer"
)

// AuthentikSAMLApplicationReconciler reconciles AuthentikSAMLApplication objects
type AuthentikSAMLApplicationReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	Recorder           events.EventRecorder
	AuthentikURL       string
	AuthentikToken     string
	NewAuthentikClient NewAuthentikClientFunc
	RequeueDelay       time.Duration

	// AllowedSigningKeypairs restricts spec.provider.signingKeypair to these
	// Authentik keypair names. Empty allows any keypair.
	AllowedSigningKeypairs []string

	// EventChannel receives external events (e.g. from the Authentik webhook
	// receiver) that should trigger reconciliation.
	EventChannel <-chan event.GenericEvent

	errors     *errorTracker
	errorsOnce sync.Once
}

// ensureErrorTracker lazily initializes the error tracker.
func (r *AuthentikSAMLApplicationReconciler) ensureErrorTracker() {
	r.errorsOnce.Do(func() {
		r.errors = newErrorTracker()
	})
}

// +kubebuilder:rbac:groups=goauthentik.io,resources=authentiksamlapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentiksamlapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentiksamlapplications/finalizers,verbs=update

// Reconcile reconciles an AuthentikSAMLApplication resource.
func (r *AuthentikSAMLApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)
	timer := prometheus.NewTimer(reconcileDuration.WithLabelValues("AuthentikSAMLApplication"))
	defer func() {
		timer.ObserveDuration()
		res := "success"
		if retErr != nil {
			res = "error"
		}
		reconcileTotal.WithLabelValues("AuthentikSAMLApplication", res).Inc()
	}()
	r.ensureErrorTracker()
	key := req.NamespacedName.String() //nolint:staticcheck // QF1008 suggestion doesn't apply to ctrl.Request

	app := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch AuthentikSAMLApplication")
		return ctrl.Result{}, err
	}

	akClient, err := r.NewAuthentikClient(r.AuthentikURL, r.AuthentikToken)
	if err != nil {
		logger.Error(err, "failed to create Authentik client")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "AuthentikError", "Reconcile", "Failed to create Authentik client: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to create Authentik client: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to create Authentik client: %w", err)
	}

	if !app.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, app, akClient)
	}

	if !controllerutil.ContainsFinalizer(app, SAMLFinalizerName) {
		controllerutil.AddFinalizer(app, SAMLFinalizerName)
		if err := r.Update(ctx, app); err != nil {
			logger.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Validate the template if provided
	if err := template.ValidateTemplate(app.Spec.Secret.Template); err != nil {
		logger.Error(err, "invalid secret template")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "TemplateError", "Reconcile", "Invalid secret template: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonTemplateError, fmt.Sprintf("Invalid secret template: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		// User error — don't requeue until CR is updated
		return ctrl.Result{}, nil
	}

	if err := checkSigningKeypairAllowed(r.AllowedSigningKeypairs, app.Spec.Provider.SigningKeypair); err != nil {
		logger.Error(err, "signing keypair not allowed")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "ValidationFailed", "Validate", "%v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonValidationFailed, err.Error()); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		// User error — don't requeue until CR is updated
		return ctrl.Result{}, nil
	}

	// Reconcile the SAML provider
	providerInfo, err := r.reconcileProvider(ctx, app, akClient)
	if err != nil {
		logger.Error(err, "failed to reconcile SAML provider")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "ProviderError", "Reconcile", "Failed to reconcile SAML provider: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile SAML provider: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to reconcile SAML provider: %w", err)
	}

	// Record the provider ID now. The ID is the ownership record for later
	// reconciles and for deletion.
	if app.Status.ProviderID != providerInfo.ID {
		app.Status.ProviderID = providerInfo.ID
		if err := r.Status().Update(ctx, app); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to record SAML provider ID: %w", err)
		}
	}

	// Refuse to change an application that this CR did not create.
	if err := verifySAMLApplicationOwnership(ctx, akClient, app); err != nil {
		logger.Error(err, "application ownership check failed")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "ApplicationError", "Reconcile", "Failed to reconcile application: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile application: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to reconcile application: %w", err)
	}

	// Reconcile the application
	appOpts := &authentik.ApplicationOptions{
		Group:            app.Spec.Group,
		PolicyEngineMode: app.Spec.PolicyEngineMode,
		MetaLaunchURL:    app.Spec.MetaLaunchURL,
		MetaDescription:  app.Spec.MetaDescription,
		MetaPublisher:    app.Spec.MetaPublisher,
		OpenInNewTab:     app.Spec.OpenInNewTab,
		GroupBindings:    app.Spec.GroupBindings,
	}

	appInfo, err := reconcileApplication(ctx, akClient, app.GetSlug(), app.Spec.Name, providerInfo.ID, appOpts)
	if err != nil {
		logger.Error(err, "failed to reconcile application")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "ApplicationError", "Reconcile", "Failed to reconcile application: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile application: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to reconcile application: %w", err)
	}

	// Record the application UID now. The UID is the ownership record for
	// later reconciles and for deletion.
	if app.Status.ApplicationUID != appInfo.UID {
		app.Status.ApplicationUID = appInfo.UID
		if err := r.Status().Update(ctx, app); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to record application UID: %w", err)
		}
	}

	// Reconcile the secret with SAML metadata
	if err := r.reconcileSecret(ctx, app, akClient, providerInfo); err != nil {
		logger.Error(err, "failed to reconcile secret")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "SecretError", "Reconcile", "Failed to reconcile secret: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonSecretError, fmt.Sprintf("Failed to reconcile secret: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to reconcile secret: %w", err)
	}

	generationChanged := app.Status.ObservedGeneration != app.Generation

	app.Status.SecretName = app.GetSecretName()
	app.Status.ObservedGeneration = app.Generation

	if err := r.setCondition(ctx, app, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "SAML application synced to Authentik"); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	if generationChanged {
		r.Recorder.Eventf(app, nil, corev1.EventTypeNormal, "Synced", "Reconcile",
			"SAML application synced to Authentik (provider=%d, secret=%s)", providerInfo.ID, app.GetSecretName())
	}

	r.errors.recordSuccess(key)

	logger.Info("successfully reconciled AuthentikSAMLApplication",
		"applicationUID", appInfo.UID,
		"providerID", providerInfo.ID,
		"secretName", app.GetSecretName())

	return ctrl.Result{RequeueAfter: r.RequeueDelay}, nil
}

func (r *AuthentikSAMLApplicationReconciler) handleDeletion(ctx context.Context, app *authentikv1alpha1.AuthentikSAMLApplication, akClient authentik.Client) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	key := client.ObjectKeyFromObject(app).String()

	if !controllerutil.ContainsFinalizer(app, SAMLFinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("handling deletion of AuthentikSAMLApplication")

	if err := deleteOwnedSAMLApplication(ctx, akClient, app); err != nil {
		return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, err
	}

	if app.Status.ProviderID != 0 {
		existingProvider, err := akClient.GetSAMLProviderByID(ctx, app.Status.ProviderID)
		if err != nil {
			return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to check if SAML provider exists: %w", err)
		}
		if existingProvider != nil {
			if err := akClient.DeleteSAMLProvider(ctx, app.Status.ProviderID); err != nil {
				return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to delete SAML provider from Authentik: %w", err)
			}
			logger.Info("deleted SAML provider from Authentik", "providerID", app.Status.ProviderID)
		}
	}

	r.Recorder.Eventf(app, nil, corev1.EventTypeNormal, "Deleted", "Delete", "Authentik SAML resources cleaned up")

	controllerutil.RemoveFinalizer(app, SAMLFinalizerName)
	if err := r.Update(ctx, app); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AuthentikSAMLApplicationReconciler) reconcileProvider(ctx context.Context, app *authentikv1alpha1.AuthentikSAMLApplication, akClient authentik.Client) (*authentik.SAMLProviderInfo, error) {
	logger := log.FromContext(ctx)
	providerName := app.GetProviderName()

	opts := &authentik.SAMLProviderOptions{
		AuthorizationFlow:  app.Spec.Provider.AuthorizationFlow,
		InvalidationFlow:   app.Spec.Provider.InvalidationFlow,
		ACSUrl:             app.Spec.Provider.ACSUrl,
		Issuer:             app.Spec.Provider.Issuer,
		Audience:           app.Spec.Provider.Audience,
		SPBinding:          app.Spec.Provider.SPBinding,
		SigningKeypair:     app.Spec.Provider.SigningKeypair,
		DigestAlgorithm:    app.Spec.Provider.DigestAlgorithm,
		SignatureAlgorithm: app.Spec.Provider.SignatureAlgorithm,
		PropertyMappings:   app.Spec.Provider.PropertyMappings,
	}

	// Only the provider recorded in status.ProviderID belongs to this CR.
	if app.Status.ProviderID != 0 {
		owned, err := akClient.GetSAMLProviderByID(ctx, app.Status.ProviderID)
		if err != nil {
			return nil, fmt.Errorf("failed to check for owned SAML provider: %w", err)
		}
		if owned != nil {
			logger.Info("updating owned SAML provider", "name", providerName, "id", owned.ID)
			return akClient.UpdateSAMLProvider(ctx, owned.ID, providerName, opts)
		}
	}

	existing, err := akClient.GetSAMLProviderByName(ctx, providerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing SAML provider: %w", err)
	}

	if existing != nil {
		return nil, fmt.Errorf("SAML provider %q (id %d) already exists and is not owned by this resource", providerName, existing.ID)
	}

	logger.Info("creating new SAML provider", "name", providerName)
	return akClient.CreateSAMLProvider(ctx, providerName, opts)
}

// verifySAMLApplicationOwnership returns an error if an application with the
// CR's slug exists and its UID is not the one recorded in status.ApplicationUID.
func verifySAMLApplicationOwnership(ctx context.Context, akClient authentik.Client, app *authentikv1alpha1.AuthentikSAMLApplication) error {
	slug := app.GetSlug()
	existing, err := akClient.GetApplicationBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("failed to check for existing application: %w", err)
	}
	if existing == nil {
		return nil
	}
	if app.Status.ApplicationUID == "" || existing.UID != app.Status.ApplicationUID {
		return fmt.Errorf("application %q already exists and is not owned by this resource", slug)
	}
	return nil
}

// deleteOwnedSAMLApplication deletes the application with the CR's slug only
// if its UID is the one recorded in status.ApplicationUID.
func deleteOwnedSAMLApplication(ctx context.Context, akClient authentik.Client, app *authentikv1alpha1.AuthentikSAMLApplication) error {
	logger := log.FromContext(ctx)
	slug := app.GetSlug()

	if app.Status.ApplicationUID == "" {
		logger.Info("no owned application recorded, skipping application deletion", "slug", slug)
		return nil
	}

	existing, err := akClient.GetApplicationBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("failed to check if application exists: %w", err)
	}
	if existing == nil {
		return nil
	}
	if existing.UID != app.Status.ApplicationUID {
		logger.Info("application is not owned by this resource, skipping deletion", "slug", slug)
		return nil
	}

	if err := akClient.DeleteApplication(ctx, slug); err != nil {
		return fmt.Errorf("failed to delete application from Authentik: %w", err)
	}
	logger.Info("deleted application from Authentik", "slug", slug)
	return nil
}

func (r *AuthentikSAMLApplicationReconciler) reconcileSecret(ctx context.Context, app *authentikv1alpha1.AuthentikSAMLApplication, akClient authentik.Client, providerInfo *authentik.SAMLProviderInfo) error {
	secretName := app.GetSecretName()

	deleteStaleSecret(ctx, r.Client, app, app.Namespace, app.Status.SecretName, secretName)

	metadata, err := akClient.GetSAMLProviderMetadata(ctx, providerInfo.ID)
	if err != nil {
		return fmt.Errorf("failed to get SAML metadata: %w", err)
	}

	data := template.SAMLSecretData{
		Metadata: metadata,
		Slug:     app.GetSlug(),
		Name:     app.Spec.Name,
	}

	secretData, err := template.RenderSAMLSecretData(app.Spec.Secret.Template, data)
	if err != nil {
		return fmt.Errorf("failed to render SAML secret template: %w", err)
	}

	return reconcileSecretObject(ctx, r.Client, r.Scheme, app, secretName, app.Namespace, secretData, app.Spec.Secret.Labels, app.Spec.Secret.Annotations)
}

func (r *AuthentikSAMLApplicationReconciler) setCondition(ctx context.Context, app *authentikv1alpha1.AuthentikSAMLApplication, status metav1.ConditionStatus, reason, message string) error {
	condition := metav1.Condition{
		Type:               authentikv1alpha1.ConditionTypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: app.Generation,
		LastTransitionTime: metav1.Now(),
	}
	meta.SetStatusCondition(&app.Status.Conditions, condition)

	if err := r.Status().Update(ctx, app); err != nil {
		return fmt.Errorf("failed to update status condition: %w", err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuthentikSAMLApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.AuthentikSAMLApplication{}).
		Owns(&corev1.Secret{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 2})

	if r.EventChannel != nil {
		b = b.WatchesRawSource(source.Channel(r.EventChannel, &handler.EnqueueRequestForObject{}))
	}

	return b.Complete(r)
}
