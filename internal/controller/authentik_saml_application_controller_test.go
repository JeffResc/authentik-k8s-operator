package controller

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
)

// --- SAML test helpers ---

func newSAMLApp(name, namespace string) *authentikv1alpha1.AuthentikSAMLApplication {
	return &authentikv1alpha1.AuthentikSAMLApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: authentikv1alpha1.AuthentikSAMLApplicationSpec{
			ApplicationBaseSpec: authentikv1alpha1.ApplicationBaseSpec{Name: "Test SAML App"},
			Provider: authentikv1alpha1.SAMLProviderSpec{
				AuthorizationFlow: "default-auth-flow",
				InvalidationFlow:  "default-inval-flow",
				ACSUrl:            "https://example.com/acs",
			},
		},
	}
}

func successSAMLMock() *mockClient {
	return &mockClient{
		createSAMLProviderRes: &authentik.SAMLProviderInfo{
			ID: 1, Name: "test-saml-app-provider",
		},
		createAppRes: &authentik.ApplicationInfo{
			UID: "saml-app-uid", Slug: "test-saml-app", Name: "Test SAML App",
		},
		getSAMLMetadataRes: `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://auth/test"></EntityDescriptor>`,
	}
}

func newSAMLReconciler(s *runtime.Scheme, app *authentikv1alpha1.AuthentikSAMLApplication, mock *mockClient) *AuthentikSAMLApplicationReconciler {
	objs := []runtime.Object{}
	if app != nil {
		objs = append(objs, app)
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&authentikv1alpha1.AuthentikSAMLApplication{}).
		Build()

	return &AuthentikSAMLApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       events.NewFakeRecorder(10),
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		RequeueDelay:   DefaultRequeueDelay,
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return mock, nil
		},
	}
}

// --- SAML Tests ---

func TestSAMLReconcile_HappyPath(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	r := newSAMLReconciler(s, app, successSAMLMock())

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("test-saml-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get updated app: %v", err)
	}
	if updated.Status.ProviderID != 1 {
		t.Errorf("expected ProviderID=1, got %d", updated.Status.ProviderID)
	}
	if updated.Status.ApplicationUID != "saml-app-uid" {
		t.Errorf("expected ApplicationUID=%q, got %q", "saml-app-uid", updated.Status.ApplicationUID)
	}
	if updated.Status.SecretName != "test-saml-app-saml" {
		t.Errorf("expected SecretName=%q, got %q", "test-saml-app-saml", updated.Status.SecretName)
	}

	// Verify secret exists with metadata
	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-saml-app-saml", Namespace: "default"}, secret); err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}
	if _, ok := secret.Data["metadata"]; !ok {
		t.Error("expected secret to contain 'metadata' key")
	}
}

func TestSAMLReconcile_AddsFinalizer(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	r := newSAMLReconciler(s, app, successSAMLMock())

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == (ctrl.Result{}) {
		t.Error("expected requeue after adding finalizer")
	}

	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("test-saml-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get updated app: %v", err)
	}
	found := false
	for _, f := range updated.Finalizers {
		if f == SAMLFinalizerName {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected SAMLFinalizerName to be added")
	}
	// Should NOT have OAuth2 finalizer
	for _, f := range updated.Finalizers {
		if f == FinalizerName {
			t.Error("unexpected OAuth2 finalizer on SAML resource")
		}
	}
}

func TestSAMLReconcile_MetadataInSecret(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}

	mock := successSAMLMock()
	expectedMetadata := `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://auth/test"></EntityDescriptor>`
	mock.getSAMLMetadataRes = expectedMetadata

	r := newSAMLReconciler(s, app, mock)
	_, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-saml-app-saml", Namespace: "default"}, secret); err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}

	if string(secret.Data["metadata"]) != expectedMetadata {
		t.Errorf("expected metadata=%q, got %q", expectedMetadata, string(secret.Data["metadata"]))
	}
}

func TestSAMLReconcile_MetadataRetrievalFailure(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}

	mock := successSAMLMock()
	mock.getSAMLMetadataErr = fmt.Errorf("metadata unavailable")
	r := newSAMLReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err == nil {
		t.Fatal("expected error for metadata retrieval failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("test-saml-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}
	if updated.Status.Conditions[0].Reason != authentikv1alpha1.ReasonSecretError {
		t.Errorf("expected reason=%q, got %q", authentikv1alpha1.ReasonSecretError, updated.Status.Conditions[0].Reason)
	}
}

func TestSAMLReconcile_UpdateExistingProvider(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}

	mock := successSAMLMock()
	mock.getSAMLProviderByNameRes = &authentik.SAMLProviderInfo{ID: 5, Name: "test-saml-app-provider"}
	mock.updateSAMLProviderRes = &authentik.SAMLProviderInfo{ID: 5, Name: "test-saml-app-provider"}
	r := newSAMLReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("test-saml-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if updated.Status.ProviderID != 5 {
		t.Errorf("expected ProviderID=5 (updated), got %d", updated.Status.ProviderID)
	}
}

func TestSAMLReconcile_ProviderCreationFailure(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}

	mock := successSAMLMock()
	mock.createSAMLProviderErr = fmt.Errorf("API error")
	mock.createSAMLProviderRes = nil
	r := newSAMLReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err == nil {
		t.Fatal("expected error for provider creation failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestSAMLReconcile_ApplicationFailure(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}

	mock := successSAMLMock()
	mock.getAppBySlugErr = fmt.Errorf("authentik unavailable")
	r := newSAMLReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err == nil {
		t.Fatal("expected error for application failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestSAMLHandleDeletion_Success(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	app.DeletionTimestamp = &now
	app.Status.ProviderID = 42

	mock := &mockClient{
		getAppBySlugRes:        &authentik.ApplicationInfo{UID: "uid", Slug: "test-saml-app"},
		getSAMLProviderByIDRes: &authentik.SAMLProviderInfo{ID: 42},
	}
	r := newSAMLReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue after successful deletion, got %+v", result)
	}
}

func TestSAMLHandleDeletion_AppDeleteFails(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	app.DeletionTimestamp = &now

	mock := &mockClient{
		getAppBySlugRes: &authentik.ApplicationInfo{UID: "uid", Slug: "test-saml-app"},
		deleteAppErr:    fmt.Errorf("forbidden"),
	}
	r := newSAMLReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err == nil {
		t.Fatal("expected error for app delete failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestSAMLHandleDeletion_ProviderDeleteFails(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	app.DeletionTimestamp = &now
	app.Status.ProviderID = 42

	mock := &mockClient{
		// App not found in Authentik (already deleted)
		getSAMLProviderByIDRes: &authentik.SAMLProviderInfo{ID: 42},
		deleteSAMLProviderErr:  fmt.Errorf("timeout"),
	}
	r := newSAMLReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err == nil {
		t.Fatal("expected error for provider delete failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	// Finalizer should still be present
	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("test-saml-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	found := false
	for _, f := range updated.Finalizers {
		if f == SAMLFinalizerName {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finalizer to still be present after provider delete failure")
	}
}

func TestSAMLReconcile_InvalidTemplate(t *testing.T) {
	// Invalid templates should be caught early via ValidateTemplate (matching
	// OAuth2 controller behavior): no error returned, no requeue, and the
	// status condition reason should be TemplateError.
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	app.Spec.Secret.Template = "{{ .Invalid"

	r := newSAMLReconciler(s, app, successSAMLMock())

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err != nil {
		t.Fatalf("expected nil error for template validation failure, got %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for invalid template, got %+v", result)
	}

	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("test-saml-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}
	if updated.Status.Conditions[0].Reason != authentikv1alpha1.ReasonTemplateError {
		t.Errorf("expected reason=%q, got %q", authentikv1alpha1.ReasonTemplateError, updated.Status.Conditions[0].Reason)
	}
}

func TestSAMLReconcile_StaleSecretCleanup(t *testing.T) {
	// When spec.secret.name changes, the old secret should be deleted and
	// the new one created (matching OAuth2 controller behavior).
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	app.Spec.Secret.Name = "new-saml-secret"
	app.Status.SecretName = "old-saml-secret"

	mock := successSAMLMock()
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-saml-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{"key": []byte("old-value")},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(app, oldSecret).
		WithStatusSubresource(&authentikv1alpha1.AuthentikSAMLApplication{}).
		Build()

	r := &AuthentikSAMLApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       events.NewFakeRecorder(10),
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		RequeueDelay:   DefaultRequeueDelay,
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return mock, nil
		},
	}

	result, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	// Old secret should be deleted
	old := &corev1.Secret{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "old-saml-secret", Namespace: "default"}, old)
	if err == nil {
		t.Error("expected old secret to be deleted")
	}

	// New secret should exist
	newSec := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "new-saml-secret", Namespace: "default"}, newSec); err != nil {
		t.Fatalf("expected new secret to exist: %v", err)
	}

	// Status should reflect new secret name
	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("test-saml-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get updated app: %v", err)
	}
	if updated.Status.SecretName != "new-saml-secret" {
		t.Errorf("expected SecretName=%q, got %q", "new-saml-secret", updated.Status.SecretName)
	}
}

func TestSAMLReconcile_CustomLabelsAndAnnotations(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	app.Spec.Secret.Labels = map[string]string{"team": "security"}
	app.Spec.Secret.Annotations = map[string]string{"description": "saml-metadata"}

	r := newSAMLReconciler(s, app, successSAMLMock())
	_, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-saml-app-saml", Namespace: "default"}, secret); err != nil {
		t.Fatalf("expected secret to exist: %v", err)
	}

	if secret.Labels["team"] != "security" {
		t.Errorf("expected label team=security, got %q", secret.Labels["team"])
	}
	if secret.Labels["app.kubernetes.io/managed-by"] != "authentik-operator" {
		t.Errorf("expected managed-by label, got %q", secret.Labels["app.kubernetes.io/managed-by"])
	}
	if secret.Labels["goauthentik.io/application"] != "test-saml-app" {
		t.Errorf("expected application label, got %q", secret.Labels["goauthentik.io/application"])
	}
	if secret.Annotations["description"] != "saml-metadata" {
		t.Errorf("expected annotation description=saml-metadata, got %q", secret.Annotations["description"])
	}
}
