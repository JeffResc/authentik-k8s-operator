package controller

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
)

// mockClient implements authentik.Client for testing.
type mockClient struct {
	healthCheckErr       error
	getProviderByNameRes *authentik.ProviderInfo
	getProviderByNameErr error
	getProviderByIDRes   *authentik.ProviderInfo
	getProviderByIDErr   error
	createProviderRes    *authentik.ProviderInfo
	createProviderErr    error
	updateProviderRes    *authentik.ProviderInfo
	updateProviderErr    error
	deleteProviderErr    error
	getProviderURLsRes   *authentik.ProviderURLs
	getProviderURLsErr   error
	getAppBySlugRes      *authentik.ApplicationInfo
	getAppBySlugErr      error
	createAppRes         *authentik.ApplicationInfo
	createAppErr         error
	updateAppRes         *authentik.ApplicationInfo
	updateAppErr         error
	deleteAppErr         error

	// SAML provider fields
	getSAMLProviderByNameRes *authentik.SAMLProviderInfo
	getSAMLProviderByNameErr error
	getSAMLProviderByIDRes   *authentik.SAMLProviderInfo
	getSAMLProviderByIDErr   error
	createSAMLProviderRes    *authentik.SAMLProviderInfo
	createSAMLProviderErr    error
	updateSAMLProviderRes    *authentik.SAMLProviderInfo
	updateSAMLProviderErr    error
	deleteSAMLProviderErr    error
	getSAMLMetadataRes       string
	getSAMLMetadataErr       error
}

func (m *mockClient) HealthCheck(context.Context) error {
	return m.healthCheckErr
}

func (m *mockClient) GetOAuth2ProviderByName(context.Context, string) (*authentik.ProviderInfo, error) {
	return m.getProviderByNameRes, m.getProviderByNameErr
}

func (m *mockClient) GetOAuth2ProviderByID(context.Context, int32) (*authentik.ProviderInfo, error) {
	return m.getProviderByIDRes, m.getProviderByIDErr
}

func (m *mockClient) CreateOAuth2Provider(_ context.Context, _ string, _ *authentik.OAuth2ProviderOptions) (*authentik.ProviderInfo, error) {
	return m.createProviderRes, m.createProviderErr
}

func (m *mockClient) UpdateOAuth2Provider(_ context.Context, _ int32, _ string, _ *authentik.OAuth2ProviderOptions) (*authentik.ProviderInfo, error) {
	return m.updateProviderRes, m.updateProviderErr
}

func (m *mockClient) DeleteOAuth2Provider(context.Context, int32) error {
	return m.deleteProviderErr
}

func (m *mockClient) GetOAuth2ProviderURLs(context.Context, int32) (*authentik.ProviderURLs, error) {
	return m.getProviderURLsRes, m.getProviderURLsErr
}

func (m *mockClient) GetSAMLProviderByName(context.Context, string) (*authentik.SAMLProviderInfo, error) {
	return m.getSAMLProviderByNameRes, m.getSAMLProviderByNameErr
}

func (m *mockClient) GetSAMLProviderByID(context.Context, int32) (*authentik.SAMLProviderInfo, error) {
	return m.getSAMLProviderByIDRes, m.getSAMLProviderByIDErr
}

func (m *mockClient) CreateSAMLProvider(_ context.Context, _ string, _ *authentik.SAMLProviderOptions) (*authentik.SAMLProviderInfo, error) {
	return m.createSAMLProviderRes, m.createSAMLProviderErr
}

func (m *mockClient) UpdateSAMLProvider(_ context.Context, _ int32, _ string, _ *authentik.SAMLProviderOptions) (*authentik.SAMLProviderInfo, error) {
	return m.updateSAMLProviderRes, m.updateSAMLProviderErr
}

func (m *mockClient) DeleteSAMLProvider(context.Context, int32) error {
	return m.deleteSAMLProviderErr
}

func (m *mockClient) GetSAMLProviderMetadata(context.Context, int32) (string, error) {
	return m.getSAMLMetadataRes, m.getSAMLMetadataErr
}

func (m *mockClient) GetApplicationBySlug(context.Context, string) (*authentik.ApplicationInfo, error) {
	return m.getAppBySlugRes, m.getAppBySlugErr
}

func (m *mockClient) CreateApplication(_ context.Context, _ string, _ string, _ int32, _ *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	return m.createAppRes, m.createAppErr
}

func (m *mockClient) UpdateApplication(_ context.Context, _ string, _ string, _ int32, _ *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	return m.updateAppRes, m.updateAppErr
}

func (m *mockClient) DeleteApplication(context.Context, string) error {
	return m.deleteAppErr
}

// --- Test helpers ---

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("failed to add client-go scheme: %v", err)
	}
	if err := authentikv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add authentik scheme: %v", err)
	}
	return s
}

func newApp(name, namespace string) *authentikv1alpha1.AuthentikOAuth2Application {
	return &authentikv1alpha1.AuthentikOAuth2Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: authentikv1alpha1.AuthentikOAuth2ApplicationSpec{
			Name: "Test App",
			Provider: authentikv1alpha1.OAuth2ProviderSpec{
				AuthorizationFlow: "default-auth-flow",
				InvalidationFlow:  "default-inval-flow",
				RedirectURIs:      []string{"https://example.com/callback"},
			},
		},
	}
}

func successMock() *mockClient {
	return &mockClient{
		createProviderRes: &authentik.ProviderInfo{
			ID: 1, Name: "test-app-provider", ClientID: "cid", ClientSecret: "csec",
		},
		createAppRes: &authentik.ApplicationInfo{
			UID: "app-uid", Slug: "test-app", Name: "Test App",
		},
		getProviderURLsRes: &authentik.ProviderURLs{
			Issuer: "https://auth/issuer", Authorize: "https://auth/authorize",
			Token: "https://auth/token", UserInfo: "https://auth/userinfo",
			Logout: "https://auth/logout", JWKS: "https://auth/jwks",
			ProviderInfo: "https://auth/.well-known",
		},
	}
}

func newReconciler(s *runtime.Scheme, app *authentikv1alpha1.AuthentikOAuth2Application, mock *mockClient) *AuthentikOAuth2ApplicationReconciler {
	objs := []runtime.Object{}
	if app != nil {
		objs = append(objs, app)
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Build()

	return &AuthentikOAuth2ApplicationReconciler{
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

func reqFor(name, namespace string) ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	}
}

// --- Tests ---

func TestReconcile_NotFound(t *testing.T) {
	s := newScheme(t)
	r := newReconciler(s, nil, successMock())

	result, err := r.Reconcile(context.Background(), reqFor("nonexistent", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for not found, got %+v", result)
	}
}

func TestReconcile_AddsFinalizer(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	r := newReconciler(s, app, successMock())

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == (ctrl.Result{}) {
		t.Error("expected requeue after adding finalizer")
	}

	// Verify finalizer was added
	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get updated app: %v", err)
	}
	found := false
	for _, f := range updated.Finalizers {
		if f == FinalizerName {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finalizer to be added")
	}
}

func TestReconcile_HappyPath(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	r := newReconciler(s, app, successMock())

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	// Verify status was updated
	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get updated app: %v", err)
	}
	if updated.Status.ProviderID != 1 {
		t.Errorf("expected ProviderID=1, got %d", updated.Status.ProviderID)
	}
	if updated.Status.ApplicationUID != "app-uid" {
		t.Errorf("expected ApplicationUID=%q, got %q", "app-uid", updated.Status.ApplicationUID)
	}
	if updated.Status.ClientID != "cid" {
		t.Errorf("expected ClientID=%q, got %q", "cid", updated.Status.ClientID)
	}
	if updated.Status.SecretName != "test-app-oauth" {
		t.Errorf("expected SecretName=%q, got %q", "test-app-oauth", updated.Status.SecretName)
	}

	// Verify secret was created
	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-app-oauth", Namespace: "default"}, secret); err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}
	if string(secret.Data["client-id"]) != "cid" {
		t.Errorf("expected secret client-id=%q, got %q", "cid", string(secret.Data["client-id"]))
	}
}

func TestReconcile_ClientCreationFailure(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(app).
		WithStatusSubresource(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Build()

	r := &AuthentikOAuth2ApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       events.NewFakeRecorder(10),
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		RequeueDelay:   DefaultRequeueDelay,
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for client creation failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestReconcile_ProviderFailure(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getProviderByNameErr = fmt.Errorf("authentik unavailable")
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for provider failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestReconcile_ApplicationFailure(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getAppBySlugErr = fmt.Errorf("authentik unavailable")
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for application failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestReconcile_SecretFailure(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getProviderURLsErr = fmt.Errorf("failed to get URLs")
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for secret failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestReconcile_InvalidTemplate(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.Spec.Secret.Template = "{{ .Invalid"

	r := newReconciler(s, app, successMock())

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("expected nil error for template validation failure, got %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue for invalid template, got %+v", result)
	}
}

func TestReconcile_UpdateExistingProvider(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getProviderByNameRes = &authentik.ProviderInfo{ID: 5, Name: "test-app-provider", ClientID: "old-cid"}
	mock.updateProviderRes = &authentik.ProviderInfo{
		ID: 5, Name: "test-app-provider", ClientID: "cid", ClientSecret: "csec",
	}
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestReconcile_UpdateExistingApplication(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getAppBySlugRes = &authentik.ApplicationInfo{UID: "old-uid", Slug: "test-app", Name: "Old Name"}
	mock.updateAppRes = &authentik.ApplicationInfo{UID: "app-uid", Slug: "test-app", Name: "Test App"}
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestHandleDeletion_Success(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.DeletionTimestamp = &now
	app.Status.ProviderID = 42

	mock := &mockClient{
		getAppBySlugRes:    &authentik.ApplicationInfo{UID: "uid", Slug: "test-app"},
		getProviderByIDRes: &authentik.ProviderInfo{ID: 42},
	}
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue after successful deletion, got %+v", result)
	}
}

func TestHandleDeletion_AppNotFound(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.DeletionTimestamp = &now

	// App and provider not found in Authentik — should still succeed
	mock := &mockClient{}
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %+v", result)
	}
}

func TestHandleDeletion_DeleteAppError(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.DeletionTimestamp = &now

	mock := &mockClient{
		getAppBySlugRes: &authentik.ApplicationInfo{UID: "uid", Slug: "test-app"},
		deleteAppErr:    fmt.Errorf("api error"),
	}
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for delete app failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

// --- Edge case tests ---

func TestReconcile_StaleSecretCleanup(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.Spec.Secret.Name = "new-secret"
	app.Status.SecretName = "old-secret"

	mock := successMock()
	// Build reconciler manually so we can add the old secret to the fake client
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{"key": []byte("old-value")},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(app, oldSecret).
		WithStatusSubresource(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Build()

	r := &AuthentikOAuth2ApplicationReconciler{
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

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	// Old secret should be deleted
	old := &corev1.Secret{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "old-secret", Namespace: "default"}, old)
	if err == nil {
		t.Error("expected old secret to be deleted")
	}

	// New secret should exist
	newSec := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "new-secret", Namespace: "default"}, newSec); err != nil {
		t.Fatalf("expected new secret to exist: %v", err)
	}

	// Status should reflect new secret name
	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get updated app: %v", err)
	}
	if updated.Status.SecretName != "new-secret" {
		t.Errorf("expected SecretName=%q, got %q", "new-secret", updated.Status.SecretName)
	}
}

func TestReconcile_SecretExternallyDeleted(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	// Status says secret exists, but we don't add it to fake client
	app.Status.SecretName = "test-app-oauth"

	r := newReconciler(s, app, successMock())

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	// Secret should be recreated
	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-app-oauth", Namespace: "default"}, secret); err != nil {
		t.Fatalf("expected secret to be recreated: %v", err)
	}
	if string(secret.Data["client-id"]) != "cid" {
		t.Errorf("expected client-id=%q, got %q", "cid", string(secret.Data["client-id"]))
	}
}

func TestReconcile_GenerationSkip_NoEventOnDriftCheck(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.Generation = 2
	app.Status.ObservedGeneration = 2

	recorder := events.NewFakeRecorder(10)
	mock := successMock()
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(app).
		WithStatusSubresource(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Build()

	r := &AuthentikOAuth2ApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       recorder,
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		RequeueDelay:   DefaultRequeueDelay,
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return mock, nil
		},
	}

	_, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No "Synced" event should be emitted since generation didn't change
	select {
	case evt := <-recorder.Events:
		t.Errorf("expected no Synced event on drift check, got: %s", evt)
	default:
		// Expected: no events
	}
}

func TestReconcile_GenerationChanged_EmitsSyncedEvent(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.Generation = 2
	app.Status.ObservedGeneration = 1

	recorder := events.NewFakeRecorder(10)
	mock := successMock()
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(app).
		WithStatusSubresource(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Build()

	r := &AuthentikOAuth2ApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       recorder,
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		RequeueDelay:   DefaultRequeueDelay,
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return mock, nil
		},
	}

	_, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have a "Synced" event
	select {
	case evt := <-recorder.Events:
		if evt == "" {
			t.Error("expected non-empty Synced event")
		}
	default:
		t.Error("expected Synced event when generation changed, got none")
	}
}

func TestReconcile_ProviderExistsButAppDoesNot(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	// Provider already exists, app does not
	mock.getProviderByNameRes = &authentik.ProviderInfo{ID: 5, Name: "test-app-provider", ClientID: "cid", ClientSecret: "csec"}
	mock.updateProviderRes = &authentik.ProviderInfo{ID: 5, Name: "test-app-provider", ClientID: "cid", ClientSecret: "csec"}
	// getAppBySlugRes is nil (not found) — createAppRes is already set by successMock

	r := newReconciler(s, app, mock)
	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if updated.Status.ProviderID != 5 {
		t.Errorf("expected ProviderID=5, got %d", updated.Status.ProviderID)
	}
	if updated.Status.ApplicationUID != "app-uid" {
		t.Errorf("expected ApplicationUID=%q, got %q", "app-uid", updated.Status.ApplicationUID)
	}
}

func TestReconcile_AppExistsButProviderDoesNot(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	// Provider does not exist — createProviderRes is already set by successMock
	mock.getProviderByNameRes = nil
	// App already exists
	mock.getAppBySlugRes = &authentik.ApplicationInfo{UID: "existing-uid", Slug: "test-app", Name: "Test App"}
	mock.updateAppRes = &authentik.ApplicationInfo{UID: "updated-uid", Slug: "test-app", Name: "Test App"}

	r := newReconciler(s, app, mock)
	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	// Provider was newly created
	if updated.Status.ProviderID != 1 {
		t.Errorf("expected ProviderID=1, got %d", updated.Status.ProviderID)
	}
	// App was updated (not created)
	if updated.Status.ApplicationUID != "updated-uid" {
		t.Errorf("expected ApplicationUID=%q, got %q", "updated-uid", updated.Status.ApplicationUID)
	}
}

func TestReconcile_ProviderURLsPartialData(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getProviderURLsRes = &authentik.ProviderURLs{
		Issuer:    "https://auth/issuer",
		Authorize: "https://auth/authorize",
		Token:     "https://auth/token",
		// UserInfo, Logout, JWKS, ProviderInfo left empty
	}

	r := newReconciler(s, app, mock)
	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-app-oauth", Namespace: "default"}, secret); err != nil {
		t.Fatalf("expected secret to exist: %v", err)
	}
	if string(secret.Data["issuer-url"]) != "https://auth/issuer" {
		t.Errorf("expected issuer-url=%q, got %q", "https://auth/issuer", string(secret.Data["issuer-url"]))
	}
}

func TestHandleDeletion_AppDeleteSucceedsProviderDeleteFails(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.DeletionTimestamp = &now
	app.Status.ProviderID = 42

	mock := &mockClient{
		getAppBySlugRes:    &authentik.ApplicationInfo{UID: "uid", Slug: "test-app"},
		getProviderByIDRes: &authentik.ProviderInfo{ID: 42},
		deleteProviderErr:  fmt.Errorf("provider API timeout"),
	}
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for provider delete failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	// Finalizer should still be present
	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	found := false
	for _, f := range updated.Finalizers {
		if f == FinalizerName {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finalizer to still be present after partial deletion failure")
	}
}

func TestHandleDeletion_ProviderCheckFails(t *testing.T) {
	s := newScheme(t)
	now := metav1.Now()
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.DeletionTimestamp = &now
	app.Status.ProviderID = 42

	mock := &mockClient{
		// App deletion succeeds (app not found)
		getProviderByIDErr: fmt.Errorf("network error"),
	}
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for provider check failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
}

func TestReconcile_CustomSecretLabelsAndAnnotations(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.Spec.Secret.Labels = map[string]string{"team": "platform"}
	app.Spec.Secret.Annotations = map[string]string{"note": "managed"}

	r := newReconciler(s, app, successMock())
	_, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-app-oauth", Namespace: "default"}, secret); err != nil {
		t.Fatalf("expected secret to exist: %v", err)
	}

	// Check custom label
	if secret.Labels["team"] != "platform" {
		t.Errorf("expected label team=platform, got %q", secret.Labels["team"])
	}
	// Check managed-by label
	if secret.Labels["app.kubernetes.io/managed-by"] != "authentik-operator" {
		t.Errorf("expected managed-by label, got %q", secret.Labels["app.kubernetes.io/managed-by"])
	}
	// Check application label
	if secret.Labels["goauthentik.io/application"] != "test-app" {
		t.Errorf("expected application label, got %q", secret.Labels["goauthentik.io/application"])
	}
	// Check custom annotation
	if secret.Annotations["note"] != "managed" {
		t.Errorf("expected annotation note=managed, got %q", secret.Annotations["note"])
	}
}

func TestReconcile_CustomTemplate(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.Spec.Secret.Template = "my-id: {{ .ClientID }}\nmy-secret: {{ .ClientSecret }}"

	r := newReconciler(s, app, successMock())
	_, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-app-oauth", Namespace: "default"}, secret); err != nil {
		t.Fatalf("expected secret to exist: %v", err)
	}

	if string(secret.Data["my-id"]) != "cid" {
		t.Errorf("expected my-id=%q, got %q", "cid", string(secret.Data["my-id"]))
	}
	if string(secret.Data["my-secret"]) != "csec" {
		t.Errorf("expected my-secret=%q, got %q", "csec", string(secret.Data["my-secret"]))
	}
	// Default keys should not be present
	if _, ok := secret.Data["client-id"]; ok {
		t.Error("expected default key 'client-id' to not be present with custom template")
	}
}

func TestReconcile_SecretDataUnchanged_SkipsUpdate(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	mock := successMock()

	r := newReconciler(s, app, mock)

	// First reconcile — creates the secret
	_, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("first reconcile unexpected error: %v", err)
	}

	// Second reconcile — secret data should be unchanged, skips update
	_, err = r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err != nil {
		t.Fatalf("second reconcile unexpected error: %v", err)
	}

	// Secret should still exist with correct data
	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "test-app-oauth", Namespace: "default"}, secret); err != nil {
		t.Fatalf("expected secret to still exist: %v", err)
	}
	if string(secret.Data["client-id"]) != "cid" {
		t.Errorf("expected client-id=%q, got %q", "cid", string(secret.Data["client-id"]))
	}
}

func TestReconcile_StatusCondition_OnProviderError(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getProviderByNameErr = fmt.Errorf("api timeout")
	r := newReconciler(s, app, mock)

	_, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error")
	}

	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}

	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}
	cond := updated.Status.Conditions[0]
	if cond.Type != authentikv1alpha1.ConditionTypeReady {
		t.Errorf("expected condition type=%q, got %q", authentikv1alpha1.ConditionTypeReady, cond.Type)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected condition status=False, got %s", cond.Status)
	}
	if cond.Reason != authentikv1alpha1.ReasonAuthentikError {
		t.Errorf("expected reason=%q, got %q", authentikv1alpha1.ReasonAuthentikError, cond.Reason)
	}
}

func TestReconcile_StatusCondition_OnSecretError(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getProviderURLsErr = fmt.Errorf("urls unavailable")
	r := newReconciler(s, app, mock)

	_, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error")
	}

	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}

	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}
	cond := updated.Status.Conditions[0]
	if cond.Reason != authentikv1alpha1.ReasonSecretError {
		t.Errorf("expected reason=%q, got %q", authentikv1alpha1.ReasonSecretError, cond.Reason)
	}
}

func TestReconcile_UpdateProviderFailure(t *testing.T) {
	s := newScheme(t)
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}

	mock := successMock()
	mock.getProviderByNameRes = &authentik.ProviderInfo{ID: 5, Name: "test-app-provider"}
	mock.updateProviderErr = fmt.Errorf("update denied")
	r := newReconciler(s, app, mock)

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for update provider failure")
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}

	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("test-app", "default").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}
	if updated.Status.Conditions[0].Reason != authentikv1alpha1.ReasonAuthentikError {
		t.Errorf("expected reason=%q, got %q", authentikv1alpha1.ReasonAuthentikError, updated.Status.Conditions[0].Reason)
	}
}
