package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
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

func newApp(name, namespace string) *authentikv1alpha1.AuthentikApplication {
	return &authentikv1alpha1.AuthentikApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: authentikv1alpha1.AuthentikApplicationSpec{
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

func newReconciler(s *runtime.Scheme, app *authentikv1alpha1.AuthentikApplication, mock *mockClient) *AuthentikApplicationReconciler {
	objs := []runtime.Object{}
	if app != nil {
		objs = append(objs, app)
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&authentikv1alpha1.AuthentikApplication{}).
		Build()

	return &AuthentikApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       record.NewFakeRecorder(10),
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
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
	if result.Requeue || result.RequeueAfter != 0 {
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
	if !result.Requeue {
		t.Error("expected Requeue=true after adding finalizer")
	}

	// Verify finalizer was added
	updated := &authentikv1alpha1.AuthentikApplication{}
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
	if result.RequeueAfter != RequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", RequeueDelay, result.RequeueAfter)
	}

	// Verify status was updated
	updated := &authentikv1alpha1.AuthentikApplication{}
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
		WithStatusSubresource(&authentikv1alpha1.AuthentikApplication{}).
		Build()

	r := &AuthentikApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       record.NewFakeRecorder(10),
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	result, err := r.Reconcile(context.Background(), reqFor("test-app", "default"))
	if err == nil {
		t.Fatal("expected error for client creation failure")
	}
	if result.RequeueAfter != RequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", RequeueDelay, result.RequeueAfter)
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
	if result.RequeueAfter != RequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", RequeueDelay, result.RequeueAfter)
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
	if result.RequeueAfter != RequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", RequeueDelay, result.RequeueAfter)
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
	if result.RequeueAfter != RequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", RequeueDelay, result.RequeueAfter)
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
	if result.Requeue || result.RequeueAfter != 0 {
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
	if result.RequeueAfter != RequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", RequeueDelay, result.RequeueAfter)
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
	if result.RequeueAfter != RequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", RequeueDelay, result.RequeueAfter)
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
	if result.Requeue || result.RequeueAfter != 0 {
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
	if result.Requeue || result.RequeueAfter != time.Duration(0) {
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
	if result.RequeueAfter != RequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", RequeueDelay, result.RequeueAfter)
	}
}
