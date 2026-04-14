// Package controller implements the Kubernetes controller for AuthentikOAuth2Application resources.
package controller

import (
	"context"
	"fmt"
	"time"

	"bytes"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
func (r *AuthentikOAuth2ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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
	appInfo, err := r.reconcileApplication(ctx, app, akClient, providerInfo.ID)
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
	existingApp, err := akClient.GetApplicationBySlug(ctx, app.GetSlug())
	if err != nil {
		logger.Error(err, "failed to check if application exists")
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonDeletionFailed, fmt.Sprintf("Failed to check if application exists: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to check if application exists: %w", err)
	}

	if existingApp != nil {
		if err := akClient.DeleteApplication(ctx, app.GetSlug()); err != nil {
			logger.Error(err, "failed to delete application from Authentik")
			if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
				authentikv1alpha1.ReasonDeletionFailed, fmt.Sprintf("Failed to delete application: %v", err)); condErr != nil {
				logger.Error(condErr, "failed to update status condition")
			}
			return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to delete application from Authentik: %w", err)
		}
		logger.Info("deleted application from Authentik", "slug", app.GetSlug())
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

// reconcileApplication ensures the application exists and is configured correctly
func (r *AuthentikOAuth2ApplicationReconciler) reconcileApplication(ctx context.Context, app *authentikv1alpha1.AuthentikOAuth2Application, akClient authentik.Client, providerID int32) (*authentik.ApplicationInfo, error) {
	logger := log.FromContext(ctx)
	slug := app.GetSlug()

	opts := &authentik.ApplicationOptions{
		Group:            app.Spec.Group,
		PolicyEngineMode: app.Spec.PolicyEngineMode,
		MetaLaunchURL:    app.Spec.MetaLaunchURL,
		MetaDescription:  app.Spec.MetaDescription,
		MetaPublisher:    app.Spec.MetaPublisher,
		OpenInNewTab:     app.Spec.OpenInNewTab,
	}

	// Check if application exists
	existingApp, err := akClient.GetApplicationBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing application: %w", err)
	}

	if existingApp != nil {
		// Update existing application
		logger.Info("updating existing application", "slug", slug)
		return akClient.UpdateApplication(ctx, slug, app.Spec.Name, providerID, opts)
	}

	// Create new application
	logger.Info("creating new application", "slug", slug)
	return akClient.CreateApplication(ctx, slug, app.Spec.Name, providerID, opts)
}

// reconcileSecret ensures the Kubernetes secret exists with the correct data
func (r *AuthentikOAuth2ApplicationReconciler) reconcileSecret(ctx context.Context, app *authentikv1alpha1.AuthentikOAuth2Application, akClient authentik.Client, providerInfo *authentik.ProviderInfo) error {
	logger := log.FromContext(ctx)
	secretName := app.GetSecretName()
	slug := app.GetSlug()

	// Delete stale secret if the name changed
	if app.Status.SecretName != "" && app.Status.SecretName != secretName {
		oldSecret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: app.Status.SecretName, Namespace: app.Namespace}, oldSecret); err == nil {
			if err := r.Delete(ctx, oldSecret); err != nil {
				logger.Error(err, "failed to delete stale secret", "name", app.Status.SecretName)
			} else {
				logger.Info("deleted stale secret after name change", "oldName", app.Status.SecretName, "newName", secretName)
				r.Recorder.Eventf(app, nil, corev1.EventTypeNormal, "SecretCleanup", "Reconcile", "Deleted stale secret %s", app.Status.SecretName)
			}
		}
	}

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
		Slug:            slug,
		Name:            app.Spec.Name,
	}

	// Render the secret data
	secretData, err := template.RenderSecretData(app.Spec.Secret.Template, data)
	if err != nil {
		return fmt.Errorf("failed to render secret template: %w", err)
	}

	// Check if the existing secret already has the correct data
	existing := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: app.Namespace}, existing); err == nil {
		if secretDataEqual(existing.Data, secretData) {
			logger.V(1).Info("secret data unchanged, skipping update", "name", secretName)
			return nil
		}
	}

	// Build the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: app.Namespace,
		},
	}

	// Create or update the secret
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set labels
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels["app.kubernetes.io/managed-by"] = "authentik-operator"
		secret.Labels["goauthentik.io/application"] = app.Name
		for k, v := range app.Spec.Secret.Labels {
			secret.Labels[k] = v
		}

		// Set annotations
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		for k, v := range app.Spec.Secret.Annotations {
			secret.Annotations[k] = v
		}

		// Set data
		secret.Data = secretData
		secret.Type = corev1.SecretTypeOpaque

		// Set owner reference
		return controllerutil.SetControllerReference(app, secret, r.Scheme)
	})

	if err != nil {
		return fmt.Errorf("failed to create or update secret: %w", err)
	}

	logger.Info("reconciled secret", "name", secretName, "operation", op)
	return nil
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

// secretDataEqual compares two secret data maps for byte-level equality.
func secretDataEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !bytes.Equal(v, b[k]) {
			return false
		}
	}
	return true
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
