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
	app.Status.ProviderID = 5

	mock := successSAMLMock()
	mock.getSAMLProviderByIDRes = &authentik.SAMLProviderInfo{ID: 5, Name: "test-saml-app-provider"}
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
	app.Status.ApplicationUID = "uid"

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

// --- SAML ownership tests ---

// samlRecordingMock records the mutating SAML and application calls.
type samlRecordingMock struct {
	*mockClient
	createdSAMLProviders   int
	updatedSAMLProviderIDs []int32
	deletedSAMLProviderIDs []int32
	createdAppSlugs        []string
	updatedAppSlugs        []string
	deletedAppSlugs        []string
}

func (m *samlRecordingMock) CreateSAMLProvider(ctx context.Context, name string, opts *authentik.SAMLProviderOptions) (*authentik.SAMLProviderInfo, error) {
	m.createdSAMLProviders++
	return m.mockClient.CreateSAMLProvider(ctx, name, opts)
}

func (m *samlRecordingMock) UpdateSAMLProvider(ctx context.Context, id int32, name string, opts *authentik.SAMLProviderOptions) (*authentik.SAMLProviderInfo, error) {
	m.updatedSAMLProviderIDs = append(m.updatedSAMLProviderIDs, id)
	return m.mockClient.UpdateSAMLProvider(ctx, id, name, opts)
}

func (m *samlRecordingMock) DeleteSAMLProvider(ctx context.Context, id int32) error {
	m.deletedSAMLProviderIDs = append(m.deletedSAMLProviderIDs, id)
	return m.mockClient.DeleteSAMLProvider(ctx, id)
}

func (m *samlRecordingMock) CreateApplication(ctx context.Context, slug, name string, providerID int32, opts *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	m.createdAppSlugs = append(m.createdAppSlugs, slug)
	return m.mockClient.CreateApplication(ctx, slug, name, providerID, opts)
}

func (m *samlRecordingMock) UpdateApplication(ctx context.Context, slug, name string, providerID int32, opts *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	m.updatedAppSlugs = append(m.updatedAppSlugs, slug)
	return m.mockClient.UpdateApplication(ctx, slug, name, providerID, opts)
}

func (m *samlRecordingMock) DeleteApplication(ctx context.Context, slug string) error {
	m.deletedAppSlugs = append(m.deletedAppSlugs, slug)
	return m.mockClient.DeleteApplication(ctx, slug)
}

func newRecordingSAMLReconciler(s *runtime.Scheme, app *authentikv1alpha1.AuthentikSAMLApplication, mock *mockClient) (*AuthentikSAMLApplicationReconciler, *samlRecordingMock) {
	rec := &samlRecordingMock{mockClient: mock}
	r := newSAMLReconciler(s, app, mock)
	r.NewAuthentikClient = func(string, string) (authentik.Client, error) {
		return rec, nil
	}
	return r, rec
}

func newAttackerSAMLApp() *authentikv1alpha1.AuthentikSAMLApplication {
	app := newSAMLApp("attacker", "team-b")
	app.Finalizers = []string{SAMLFinalizerName}
	app.Spec.Slug = "victim-app"
	app.Spec.Provider.ACSUrl = "https://attacker.example.test/acs"
	return app
}

func TestSAMLCrossNamespaceSlugAdoption_RefusesForeignProvider(t *testing.T) {
	s := newScheme(t)
	mock := successSAMLMock()
	mock.getSAMLProviderByNameRes = &authentik.SAMLProviderInfo{ID: 9, Name: "victim-app-provider"}
	mock.updateSAMLProviderRes = &authentik.SAMLProviderInfo{ID: 9, Name: "victim-app-provider"}
	mock.getAppBySlugRes = &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"}
	mock.updateAppRes = &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"}
	r, rec := newRecordingSAMLReconciler(s, newAttackerSAMLApp(), mock)

	if _, err := r.Reconcile(context.Background(), reqFor("attacker", "team-b")); err == nil {
		t.Fatal("expected an error when the provider name belongs to a foreign provider")
	}
	if len(rec.updatedSAMLProviderIDs) != 0 || rec.createdSAMLProviders != 0 {
		t.Errorf("expected no provider writes, got updates=%v creates=%d", rec.updatedSAMLProviderIDs, rec.createdSAMLProviders)
	}
	if len(rec.updatedAppSlugs) != 0 || len(rec.createdAppSlugs) != 0 {
		t.Errorf("expected no application writes, got updates=%v creates=%v", rec.updatedAppSlugs, rec.createdAppSlugs)
	}

	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("attacker", "team-b").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if updated.Status.ProviderID != 0 {
		t.Errorf("expected ProviderID=0, got %d", updated.Status.ProviderID)
	}
}

func TestSAMLCrossNamespaceSlugAdoption_RefusesForeignApplication(t *testing.T) {
	s := newScheme(t)
	mock := successSAMLMock()
	mock.createSAMLProviderRes = &authentik.SAMLProviderInfo{ID: 10, Name: "victim-app-provider"}
	mock.getAppBySlugRes = &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"}
	mock.updateAppRes = &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"}
	r, rec := newRecordingSAMLReconciler(s, newAttackerSAMLApp(), mock)

	if _, err := r.Reconcile(context.Background(), reqFor("attacker", "team-b")); err == nil {
		t.Fatal("expected an error when the slug belongs to a foreign application")
	}
	if len(rec.updatedAppSlugs) != 0 || len(rec.createdAppSlugs) != 0 {
		t.Errorf("expected no application writes, got updates=%v creates=%v", rec.updatedAppSlugs, rec.createdAppSlugs)
	}

	updated := &authentikv1alpha1.AuthentikSAMLApplication{}
	if err := r.Get(context.Background(), reqFor("attacker", "team-b").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	// The provider this CR created is recorded so that deletion cleans it up.
	if updated.Status.ProviderID != 10 {
		t.Errorf("expected ProviderID=10, got %d", updated.Status.ProviderID)
	}
	if updated.Status.ApplicationUID != "" {
		t.Errorf("expected empty ApplicationUID, got %q", updated.Status.ApplicationUID)
	}
}

func TestSAMLCrossNamespaceSlugDeletion_SkipsForeignApplication(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newAttackerSAMLApp()
	app.DeletionTimestamp = &now
	app.Spec.Secret.Template = "{{ .Invalid"

	mock := &mockClient{
		getAppBySlugRes:        &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"},
		getSAMLProviderByIDRes: &authentik.SAMLProviderInfo{ID: 9},
	}
	r, rec := newRecordingSAMLReconciler(s, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("attacker", "team-b")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.deletedAppSlugs) != 0 {
		t.Errorf("expected no application deletion, got %v", rec.deletedAppSlugs)
	}
	if len(rec.deletedSAMLProviderIDs) != 0 {
		t.Errorf("expected no provider deletion, got %v", rec.deletedSAMLProviderIDs)
	}
}

func TestSAMLCrossNamespaceSlugDeletion_SkipsApplicationWithOtherUID(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newAttackerSAMLApp()
	app.DeletionTimestamp = &now
	app.Status.ApplicationUID = "attacker-uid"

	mock := &mockClient{
		getAppBySlugRes: &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"},
	}
	r, rec := newRecordingSAMLReconciler(s, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("attacker", "team-b")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.deletedAppSlugs) != 0 {
		t.Errorf("expected no application deletion, got %v", rec.deletedAppSlugs)
	}
}

func TestSAMLReconcile_UpdatesOwnedObjects(t *testing.T) {
	s := newScheme(t)
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	app.Status.ProviderID = 5
	app.Status.ApplicationUID = "saml-app-uid"

	mock := successSAMLMock()
	mock.getSAMLProviderByIDRes = &authentik.SAMLProviderInfo{ID: 5, Name: "test-saml-app-provider"}
	mock.updateSAMLProviderRes = &authentik.SAMLProviderInfo{ID: 5, Name: "test-saml-app-provider"}
	mock.getAppBySlugRes = &authentik.ApplicationInfo{UID: "saml-app-uid", Slug: "test-saml-app"}
	mock.updateAppRes = &authentik.ApplicationInfo{UID: "saml-app-uid", Slug: "test-saml-app"}
	r, rec := newRecordingSAMLReconciler(s, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.updatedSAMLProviderIDs) != 1 || rec.updatedSAMLProviderIDs[0] != 5 {
		t.Errorf("expected UpdateSAMLProvider(5), got %v", rec.updatedSAMLProviderIDs)
	}
	if rec.createdSAMLProviders != 0 {
		t.Errorf("expected no provider creation, got %d", rec.createdSAMLProviders)
	}
	if len(rec.updatedAppSlugs) != 1 || rec.updatedAppSlugs[0] != "test-saml-app" {
		t.Errorf("expected UpdateApplication(test-saml-app), got %v", rec.updatedAppSlugs)
	}
}

func TestSAMLHandleDeletion_DeletesOwnedApplication(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newSAMLApp("test-saml-app", "default")
	app.Finalizers = []string{SAMLFinalizerName}
	app.DeletionTimestamp = &now
	app.Status.ProviderID = 42
	app.Status.ApplicationUID = "uid"

	mock := &mockClient{
		getAppBySlugRes:        &authentik.ApplicationInfo{UID: "uid", Slug: "test-saml-app"},
		getSAMLProviderByIDRes: &authentik.SAMLProviderInfo{ID: 42},
	}
	r, rec := newRecordingSAMLReconciler(s, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("test-saml-app", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.deletedAppSlugs) != 1 || rec.deletedAppSlugs[0] != "test-saml-app" {
		t.Errorf("expected DeleteApplication(test-saml-app), got %v", rec.deletedAppSlugs)
	}
	if len(rec.deletedSAMLProviderIDs) != 1 || rec.deletedSAMLProviderIDs[0] != 42 {
		t.Errorf("expected DeleteSAMLProvider(42), got %v", rec.deletedSAMLProviderIDs)
	}
}
