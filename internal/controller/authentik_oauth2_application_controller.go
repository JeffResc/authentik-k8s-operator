// Package controller implements the Kubernetes controller for AuthentikOAuth2Application resources.
package controller

import (
	"context"
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/prometheus/client_golang/prometheus"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
	"github.com/JeffResc/authentik-k8s-operator/internal/template"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// FinalizerName is the finalizer name for AuthentikOAuth2Application resources
	FinalizerName = "goauthentik.io/finalizer"

	// DefaultRequeueDelay is the default delay for periodic drift detection requeue.
	DefaultRequeueDelay = 5 * time.Minute
)

// NewAuthentikClientFunc is a factory function type for creating Authentik API clients.
type NewAuthentikClientFunc func(baseURL, token string) (authentik.Client, error)

// AuthentikOAuth2ApplicationReconciler reconciles a AuthentikOAuth2Application object
type AuthentikOAuth2ApplicationReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	Recorder           events.EventRecorder
	AuthentikURL       string
	AuthentikToken     string
	NewAuthentikClient NewAuthentikClientFunc
	RequeueDelay       time.Duration

	// EventChannel receives external events (e.g. from the Authentik webhook
	// receiver) that should trigger reconciliation. Optional — leave nil to
	// disable external event-driven reconciliation.
	EventChannel <-chan event.GenericEvent
}

// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikoauth2applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikoauth2applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikoauth2applications/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *AuthentikOAuth2ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)
	timer := prometheus.NewTimer(reconcileDuration.WithLabelValues("AuthentikOAuth2Application"))
	defer func() {
		timer.ObserveDuration()
		res := "success"
		if retErr != nil {
			res = "error"
		}
		reconcileTotal.WithLabelValues("AuthentikOAuth2Application", res).Inc()
	}()

	// Fetch the AuthentikOAuth2Application instance
	app := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, probably deleted
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch AuthentikOAuth2Application")
		return ctrl.Result{}, err
	}

	// Create Authentik client
	akClient, err := r.NewAuthentikClient(r.AuthentikURL, r.AuthentikToken)
	if err != nil {
		logger.Error(err, "failed to create Authentik client")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "AuthentikError", "Reconcile", "Failed to create Authentik client: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to create Authentik client: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to create Authentik client: %w", err)
	}

	// Handle deletion
	if !app.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, app, akClient)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(app, FinalizerName) {
		controllerutil.AddFinalizer(app, FinalizerName)
		if err := r.Update(ctx, app); err != nil {
			logger.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Validate the template if provided
	if err := template.ValidateTemplate(app.Spec.Secret.Template); err != nil {
		logger.Error(err, "invalid secret template")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "TemplateError", "Validate", "Invalid secret template: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonTemplateError, fmt.Sprintf("Invalid secret template: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		// User error — don't requeue until CR is updated
		return ctrl.Result{}, nil
	}

	// Reconcile the OAuth2 provider
	providerInfo, err := r.reconcileProvider(ctx, app, akClient)
	if err != nil {
		logger.Error(err, "failed to reconcile provider")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "ProviderError", "Reconcile", "Failed to reconcile provider: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile provider: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to reconcile provider: %w", err)
	}

	// Reconcile the application
	appInfo, err := r.reconcileOAuth2Application(ctx, app, akClient, providerInfo.ID)
	if err != nil {
		logger.Error(err, "failed to reconcile application")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "ApplicationError", "Reconcile", "Failed to reconcile application: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile application: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to reconcile application: %w", err)
	}

	// Reconcile the secret
	if err := r.reconcileSecret(ctx, app, akClient, providerInfo); err != nil {
		logger.Error(err, "failed to reconcile secret")
		r.Recorder.Eventf(app, nil, corev1.EventTypeWarning, "SecretError", "Reconcile", "Failed to reconcile secret: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonSecretError, fmt.Sprintf("Failed to reconcile secret: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to reconcile secret: %w", err)
	}

	// Emit event on first sync or when spec changes (not on periodic drift checks)
	generationChanged := app.Status.ObservedGeneration != app.Generation

	// Update status
	app.Status.ApplicationUID = appInfo.UID
	app.Status.ProviderID = providerInfo.ID
	app.Status.SecretName = app.GetSecretName()
	app.Status.ClientID = providerInfo.ClientID
	app.Status.ObservedGeneration = app.Generation

	if err := r.setCondition(ctx, app, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "Application synced to Authentik"); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	if generationChanged {
		r.Recorder.Eventf(app, nil, corev1.EventTypeNormal, "Synced", "Reconcile",
			"Application synced to Authentik (provider=%d, secret=%s)", providerInfo.ID, app.GetSecretName())
	}

	logger.Info("successfully reconciled AuthentikOAuth2Application",
		"applicationUID", appInfo.UID,
		"providerID", providerInfo.ID,
		"secretName", app.GetSecretName())

	// Requeue for drift detection
	return ctrl.Result{RequeueAfter: r.RequeueDelay}, nil
}

// handleDeletion handles the deletion of an AuthentikOAuth2Application
func (r *AuthentikOAuth2ApplicationReconciler) handleDeletion(ctx context.Context, app *authentikv1alpha1.AuthentikOAuth2Application, akClient authentik.Client) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(app, FinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("handling deletion of AuthentikOAuth2Application")

	// Delete the application from Authentik
	if err := deleteAuthentikApplication(ctx, akClient, app.GetSlug()); err != nil {
		logger.Error(err, "failed to delete application from Authentik")
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonDeletionFailed, fmt.Sprintf("Failed to delete application: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, err
	}

	// Delete the provider from Authentik
	if app.Status.ProviderID != 0 {
		existingProvider, err := akClient.GetOAuth2ProviderByID(ctx, app.Status.ProviderID)
		if err != nil {
			logger.Error(err, "failed to check if provider exists")
			if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
				authentikv1alpha1.ReasonDeletionFailed, fmt.Sprintf("Failed to check if provider exists: %v", err)); condErr != nil {
				logger.Error(condErr, "failed to update status condition")
			}
			return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to check if provider exists: %w", err)
		}

		if existingProvider != nil {
			if err := akClient.DeleteOAuth2Provider(ctx, app.Status.ProviderID); err != nil {
				logger.Error(err, "failed to delete provider from Authentik")
				if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
					authentikv1alpha1.ReasonDeletionFailed, fmt.Sprintf("Failed to delete provider: %v", err)); condErr != nil {
					logger.Error(condErr, "failed to update status condition")
				}
				return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to delete provider from Authentik: %w", err)
			}
			logger.Info("deleted provider from Authentik", "providerID", app.Status.ProviderID)
		}
	}

	r.Recorder.Eventf(app, nil, corev1.EventTypeNormal, "Deleted", "Delete", "Authentik resources cleaned up")

	// Remove finalizer
	controllerutil.RemoveFinalizer(app, FinalizerName)
	if err := r.Update(ctx, app); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("successfully deleted AuthentikOAuth2Application")
	return ctrl.Result{}, nil
}

// reconcileProvider ensures the OAuth2 provider exists and is configured correctly
func (r *AuthentikOAuth2ApplicationReconciler) reconcileProvider(ctx context.Context, app *authentikv1alpha1.AuthentikOAuth2Application, akClient authentik.Client) (*authentik.ProviderInfo, error) {
	logger := log.FromContext(ctx)
	providerName := app.GetProviderName()

	opts := &authentik.OAuth2ProviderOptions{
		AuthorizationFlow:    app.Spec.Provider.AuthorizationFlow,
		InvalidationFlow:     app.Spec.Provider.InvalidationFlow,
		RedirectURIs:         app.Spec.Provider.RedirectURIs,
		Scopes:               app.Spec.Provider.Scopes,
		ClientType:           app.Spec.Provider.ClientType,
		AccessCodeValidity:   app.Spec.Provider.AccessCodeValidity,
		AccessTokenValidity:  app.Spec.Provider.AccessTokenValidity,
		RefreshTokenValidity: app.Spec.Provider.RefreshTokenValidity,
		SubMode:              app.Spec.Provider.SubMode,
		IncludeClaimsInToken: app.Spec.Provider.IncludeClaimsInIDToken,
		IssuerMode:           app.Spec.Provider.IssuerMode,
		PropertyMappings:     app.Spec.Provider.PropertyMappings,
		SigningKey:           app.Spec.Provider.SigningKey,
	}

	// Check if provider exists by name
	existingProvider, err := akClient.GetOAuth2ProviderByName(ctx, providerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing provider: %w", err)
	}

	if existingProvider != nil {
		// Update existing provider
		logger.Info("updating existing OAuth2 provider", "name", providerName, "id", existingProvider.ID)
		return akClient.UpdateOAuth2Provider(ctx, existingProvider.ID, providerName, opts)
	}

	// Create new provider
	logger.Info("creating new OAuth2 provider", "name", providerName)
	return akClient.CreateOAuth2Provider(ctx, providerName, opts)
}

// reconcileOAuth2Application ensures the application exists and is configured correctly
func (r *AuthentikOAuth2ApplicationReconciler) reconcileOAuth2Application(ctx context.Context, app *authentikv1alpha1.AuthentikOAuth2Application, akClient authentik.Client, providerID int32) (*authentik.ApplicationInfo, error) {
	opts := &authentik.ApplicationOptions{
		Group:            app.Spec.Group,
		PolicyEngineMode: app.Spec.PolicyEngineMode,
		MetaLaunchURL:    app.Spec.MetaLaunchURL,
		MetaDescription:  app.Spec.MetaDescription,
		MetaPublisher:    app.Spec.MetaPublisher,
		OpenInNewTab:     app.Spec.OpenInNewTab,
	}

	return reconcileApplication(ctx, akClient, app.GetSlug(), app.Spec.Name, providerID, opts)
}

// reconcileSecret ensures the Kubernetes secret exists with the correct data
func (r *AuthentikOAuth2ApplicationReconciler) reconcileSecret(ctx context.Context, app *authentikv1alpha1.AuthentikOAuth2Application, akClient authentik.Client, providerInfo *authentik.ProviderInfo) error {
	secretName := app.GetSecretName()

	deleteStaleSecret(ctx, r.Client, app.Namespace, app.Status.SecretName, secretName)

	// Get OIDC URLs from the Authentik API
	providerURLs, err := akClient.GetOAuth2ProviderURLs(ctx, providerInfo.ID)
	if err != nil {
		return fmt.Errorf("failed to get provider URLs: %w", err)
	}

	// Prepare template data
	data := template.SecretData{
		ClientID:        providerInfo.ClientID,
		ClientSecret:    providerInfo.ClientSecret,
		IssuerURL:       providerURLs.Issuer,
		AuthURL:         providerURLs.Authorize,
		TokenURL:        providerURLs.Token,
		UserInfoURL:     providerURLs.UserInfo,
		LogoutURL:       providerURLs.Logout,
		JWKSURL:         providerURLs.JWKS,
		ProviderInfoURL: providerURLs.ProviderInfo,
		Slug:            app.GetSlug(),
		Name:            app.Spec.Name,
	}

	// Render the secret data
	secretData, err := template.RenderSecretData(app.Spec.Secret.Template, data)
	if err != nil {
		return fmt.Errorf("failed to render secret template: %w", err)
	}

	return reconcileSecretObject(ctx, r.Client, r.Scheme, app, secretName, app.Namespace, secretData, app.Spec.Secret.Labels, app.Spec.Secret.Annotations)
}

// setCondition sets a condition on the AuthentikOAuth2Application and updates the status.
func (r *AuthentikOAuth2ApplicationReconciler) setCondition(ctx context.Context, app *authentikv1alpha1.AuthentikOAuth2Application, status metav1.ConditionStatus, reason, message string) error {
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
func (r *AuthentikOAuth2ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Owns(&corev1.Secret{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 2})

	if r.EventChannel != nil {
		b = b.WatchesRawSource(source.Channel(r.EventChannel, &handler.EnqueueRequestForObject{}))
	}

	return b.Complete(r)
}
