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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
	Recorder           record.EventRecorder
	AuthentikURL       string
	AuthentikToken     string
	NewAuthentikClient NewAuthentikClientFunc
	RequeueDelay       time.Duration
}

// +kubebuilder:rbac:groups=goauthentik.io,resources=authentiksamlapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentiksamlapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentiksamlapplications/finalizers,verbs=update

// Reconcile reconciles an AuthentikSAMLApplication resource.
func (r *AuthentikSAMLApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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
		r.Recorder.Eventf(app, corev1.EventTypeWarning, "AuthentikError", "Failed to create Authentik client: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to create Authentik client: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to create Authentik client: %w", err)
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
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile the SAML provider
	providerInfo, err := r.reconcileProvider(ctx, app, akClient)
	if err != nil {
		logger.Error(err, "failed to reconcile SAML provider")
		r.Recorder.Eventf(app, corev1.EventTypeWarning, "ProviderError", "Failed to reconcile SAML provider: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile SAML provider: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to reconcile SAML provider: %w", err)
	}

	// Reconcile the application
	appOpts := &authentik.ApplicationOptions{
		Group:            app.Spec.Group,
		PolicyEngineMode: app.Spec.PolicyEngineMode,
		MetaLaunchURL:    app.Spec.MetaLaunchURL,
		MetaDescription:  app.Spec.MetaDescription,
		MetaPublisher:    app.Spec.MetaPublisher,
		OpenInNewTab:     app.Spec.OpenInNewTab,
	}

	var appInfo *authentik.ApplicationInfo
	existingApp, err := akClient.GetApplicationBySlug(ctx, app.GetSlug())
	if err != nil {
		logger.Error(err, "failed to check for existing application")
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile application: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to reconcile application: %w", err)
	}

	if existingApp != nil {
		appInfo, err = akClient.UpdateApplication(ctx, app.GetSlug(), app.Spec.Name, providerInfo.ID, appOpts)
	} else {
		appInfo, err = akClient.CreateApplication(ctx, app.GetSlug(), app.Spec.Name, providerInfo.ID, appOpts)
	}
	if err != nil {
		logger.Error(err, "failed to reconcile application")
		r.Recorder.Eventf(app, corev1.EventTypeWarning, "ApplicationError", "Failed to reconcile application: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile application: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to reconcile application: %w", err)
	}

	// Reconcile the secret with SAML metadata
	if err := r.reconcileSecret(ctx, app, akClient, providerInfo); err != nil {
		logger.Error(err, "failed to reconcile secret")
		r.Recorder.Eventf(app, corev1.EventTypeWarning, "SecretError", "Failed to reconcile secret: %v", err)
		if condErr := r.setCondition(ctx, app, metav1.ConditionFalse,
			authentikv1alpha1.ReasonSecretError, fmt.Sprintf("Failed to reconcile secret: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to reconcile secret: %w", err)
	}

	generationChanged := app.Status.ObservedGeneration != app.Generation

	app.Status.ApplicationUID = appInfo.UID
	app.Status.ProviderID = providerInfo.ID
	app.Status.SecretName = app.GetSecretName()
	app.Status.ObservedGeneration = app.Generation

	if err := r.setCondition(ctx, app, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "SAML application synced to Authentik"); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	if generationChanged {
		r.Recorder.Eventf(app, corev1.EventTypeNormal, "Synced",
			"SAML application synced to Authentik (provider=%d, secret=%s)", providerInfo.ID, app.GetSecretName())
	}

	logger.Info("successfully reconciled AuthentikSAMLApplication",
		"applicationUID", appInfo.UID,
		"providerID", providerInfo.ID,
		"secretName", app.GetSecretName())

	return ctrl.Result{RequeueAfter: r.RequeueDelay}, nil
}

func (r *AuthentikSAMLApplicationReconciler) handleDeletion(ctx context.Context, app *authentikv1alpha1.AuthentikSAMLApplication, akClient authentik.Client) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(app, SAMLFinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("handling deletion of AuthentikSAMLApplication")

	existingApp, err := akClient.GetApplicationBySlug(ctx, app.GetSlug())
	if err != nil {
		return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to check if application exists: %w", err)
	}
	if existingApp != nil {
		if err := akClient.DeleteApplication(ctx, app.GetSlug()); err != nil {
			return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to delete application from Authentik: %w", err)
		}
		logger.Info("deleted application from Authentik", "slug", app.GetSlug())
	}

	if app.Status.ProviderID != 0 {
		existingProvider, err := akClient.GetSAMLProviderByID(ctx, app.Status.ProviderID)
		if err != nil {
			return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to check if SAML provider exists: %w", err)
		}
		if existingProvider != nil {
			if err := akClient.DeleteSAMLProvider(ctx, app.Status.ProviderID); err != nil {
				return ctrl.Result{RequeueAfter: r.RequeueDelay}, fmt.Errorf("failed to delete SAML provider from Authentik: %w", err)
			}
			logger.Info("deleted SAML provider from Authentik", "providerID", app.Status.ProviderID)
		}
	}

	r.Recorder.Event(app, corev1.EventTypeNormal, "Deleted", "Authentik SAML resources cleaned up")

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

	existing, err := akClient.GetSAMLProviderByName(ctx, providerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing SAML provider: %w", err)
	}

	if existing != nil {
		logger.Info("updating existing SAML provider", "name", providerName, "id", existing.ID)
		return akClient.UpdateSAMLProvider(ctx, existing.ID, providerName, opts)
	}

	logger.Info("creating new SAML provider", "name", providerName)
	return akClient.CreateSAMLProvider(ctx, providerName, opts)
}

func (r *AuthentikSAMLApplicationReconciler) reconcileSecret(ctx context.Context, app *authentikv1alpha1.AuthentikSAMLApplication, akClient authentik.Client, providerInfo *authentik.SAMLProviderInfo) error {
	logger := log.FromContext(ctx)
	secretName := app.GetSecretName()

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

	existing := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: app.Namespace}, existing); err == nil {
		if secretDataEqual(existing.Data, secretData) {
			logger.V(1).Info("secret data unchanged, skipping update", "name", secretName)
			return nil
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: app.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels["app.kubernetes.io/managed-by"] = "authentik-operator"
		secret.Labels["goauthentik.io/application"] = app.Name
		for k, v := range app.Spec.Secret.Labels {
			secret.Labels[k] = v
		}

		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		for k, v := range app.Spec.Secret.Annotations {
			secret.Annotations[k] = v
		}

		secret.Data = secretData
		secret.Type = corev1.SecretTypeOpaque

		return controllerutil.SetControllerReference(app, secret, r.Scheme)
	})

	if err != nil {
		return fmt.Errorf("failed to create or update secret: %w", err)
	}

	logger.Info("reconciled secret", "name", secretName, "operation", op)
	return nil
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.AuthentikSAMLApplication{}).
		Owns(&corev1.Secret{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}
