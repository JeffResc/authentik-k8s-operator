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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
	"github.com/JeffResc/authentik-k8s-operator/internal/template"
)

const (
	// FinalizerName is the finalizer name for AuthentikApplication resources
	FinalizerName = "goauthentik.io/finalizer"

	// RequeueDelay is the default delay for requeue
	RequeueDelay = 5 * time.Minute
)

// AuthentikApplicationReconciler reconciles a AuthentikApplication object
type AuthentikApplicationReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	AuthentikURL   string
	AuthentikToken string
}

// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *AuthentikApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AuthentikApplication instance
	app := &authentikv1alpha1.AuthentikApplication{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, probably deleted
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch AuthentikApplication")
		return ctrl.Result{}, err
	}

	// Create Authentik client
	akClient, err := authentik.NewClient(r.AuthentikURL, r.AuthentikToken)
	if err != nil {
		logger.Error(err, "failed to create Authentik client")
		r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to create Authentik client: %v", err))
		return ctrl.Result{RequeueAfter: RequeueDelay}, nil
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
		r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonTemplateError, fmt.Sprintf("Invalid secret template: %v", err))
		return ctrl.Result{}, nil // Don't requeue until CR is updated
	}

	// Reconcile the OAuth2 provider
	providerInfo, err := r.reconcileProvider(ctx, app, akClient)
	if err != nil {
		logger.Error(err, "failed to reconcile provider")
		r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile provider: %v", err))
		return ctrl.Result{RequeueAfter: RequeueDelay}, nil
	}

	// Reconcile the application
	appInfo, err := r.reconcileApplication(ctx, app, akClient, providerInfo.ID)
	if err != nil {
		logger.Error(err, "failed to reconcile application")
		r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile application: %v", err))
		return ctrl.Result{RequeueAfter: RequeueDelay}, nil
	}

	// Reconcile the secret
	if err := r.reconcileSecret(ctx, app, akClient, providerInfo); err != nil {
		logger.Error(err, "failed to reconcile secret")
		r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonSecretError, fmt.Sprintf("Failed to reconcile secret: %v", err))
		return ctrl.Result{RequeueAfter: RequeueDelay}, nil
	}

	// Update status
	app.Status.ApplicationUID = appInfo.UID
	app.Status.ProviderID = providerInfo.ID
	app.Status.SecretName = app.GetSecretName()
	app.Status.ClientID = providerInfo.ClientID
	app.Status.ObservedGeneration = app.Generation

	r.setCondition(ctx, app, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "Application synced to Authentik")

	if err := r.Status().Update(ctx, app); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("successfully reconciled AuthentikApplication",
		"applicationUID", appInfo.UID,
		"providerID", providerInfo.ID,
		"secretName", app.GetSecretName())

	// Requeue for drift detection
	return ctrl.Result{RequeueAfter: RequeueDelay}, nil
}

// handleDeletion handles the deletion of an AuthentikApplication
func (r *AuthentikApplicationReconciler) handleDeletion(ctx context.Context, app *authentikv1alpha1.AuthentikApplication, akClient *authentik.Client) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(app, FinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("handling deletion of AuthentikApplication")

	// Delete the application from Authentik
	existingApp, err := akClient.GetApplicationBySlug(ctx, app.GetSlug())
	if err != nil {
		logger.Error(err, "failed to check if application exists")
		return ctrl.Result{RequeueAfter: RequeueDelay}, nil
	}

	if existingApp != nil {
		if err := akClient.DeleteApplication(ctx, app.GetSlug()); err != nil {
			logger.Error(err, "failed to delete application from Authentik")
			return ctrl.Result{RequeueAfter: RequeueDelay}, nil
		}
		logger.Info("deleted application from Authentik", "slug", app.GetSlug())
	}

	// Delete the provider from Authentik
	if app.Status.ProviderID != 0 {
		existingProvider, err := akClient.GetOAuth2ProviderByID(ctx, app.Status.ProviderID)
		if err != nil {
			logger.Error(err, "failed to check if provider exists")
			return ctrl.Result{RequeueAfter: RequeueDelay}, nil
		}

		if existingProvider != nil {
			if err := akClient.DeleteOAuth2Provider(ctx, app.Status.ProviderID); err != nil {
				logger.Error(err, "failed to delete provider from Authentik")
				return ctrl.Result{RequeueAfter: RequeueDelay}, nil
			}
			logger.Info("deleted provider from Authentik", "providerID", app.Status.ProviderID)
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(app, FinalizerName)
	if err := r.Update(ctx, app); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("successfully deleted AuthentikApplication")
	return ctrl.Result{}, nil
}

// reconcileProvider ensures the OAuth2 provider exists and is configured correctly
func (r *AuthentikApplicationReconciler) reconcileProvider(ctx context.Context, app *authentikv1alpha1.AuthentikApplication, akClient *authentik.Client) (*authentik.ProviderInfo, error) {
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
func (r *AuthentikApplicationReconciler) reconcileApplication(ctx context.Context, app *authentikv1alpha1.AuthentikApplication, akClient *authentik.Client, providerID int32) (*authentik.ApplicationInfo, error) {
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
func (r *AuthentikApplicationReconciler) reconcileSecret(ctx context.Context, app *authentikv1alpha1.AuthentikApplication, akClient *authentik.Client, providerInfo *authentik.ProviderInfo) error {
	logger := log.FromContext(ctx)
	secretName := app.GetSecretName()
	slug := app.GetSlug()

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

// setCondition sets a condition on the AuthentikApplication and updates the status.
// Errors are logged but not returned, as status updates are best-effort and the next
// reconciliation will retry the update.
func (r *AuthentikApplicationReconciler) setCondition(ctx context.Context, app *authentikv1alpha1.AuthentikApplication, status metav1.ConditionStatus, reason, message string) {
	logger := log.FromContext(ctx)
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
		logger.Error(err, "failed to update status condition", "reason", reason, "message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuthentikApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.AuthentikApplication{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
