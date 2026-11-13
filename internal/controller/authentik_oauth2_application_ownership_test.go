package controller

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
)

// recordingClient wraps mockClient and records the mutating Authentik calls.
type recordingClient struct {
	*mockClient
	calls []string
}

func (m *recordingClient) UpdateOAuth2Provider(ctx context.Context, id int32, name string, opts *authentik.OAuth2ProviderOptions) (*authentik.ProviderInfo, error) {
	m.calls = append(m.calls, "UpdateOAuth2Provider")
	return m.mockClient.UpdateOAuth2Provider(ctx, id, name, opts)
}

func (m *recordingClient) DeleteOAuth2Provider(ctx context.Context, id int32) error {
	m.calls = append(m.calls, "DeleteOAuth2Provider")
	return m.mockClient.DeleteOAuth2Provider(ctx, id)
}

func (m *recordingClient) CreateApplication(ctx context.Context, slug, name string, providerID int32, opts *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	m.calls = append(m.calls, "CreateApplication")
	return m.mockClient.CreateApplication(ctx, slug, name, providerID, opts)
}

func (m *recordingClient) UpdateApplication(ctx context.Context, slug, name string, providerID int32, opts *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	m.calls = append(m.calls, "UpdateApplication")
	return m.mockClient.UpdateApplication(ctx, slug, name, providerID, opts)
}

func (m *recordingClient) DeleteApplication(ctx context.Context, slug string) error {
	m.calls = append(m.calls, "DeleteApplication")
	return m.mockClient.DeleteApplication(ctx, slug)
}

func (m *recordingClient) SyncGroupBindings(ctx context.Context, appUID string, groupNames []string) error {
	m.calls = append(m.calls, "SyncGroupBindings")
	return m.mockClient.SyncGroupBindings(ctx, appUID, groupNames)
}

func newRecordingReconciler(t *testing.T, app *authentikv1alpha1.AuthentikOAuth2Application, mock *recordingClient) *AuthentikOAuth2ApplicationReconciler {
	t.Helper()
	r := newReconciler(newScheme(t), app, mock.mockClient)
	r.NewAuthentikClient = func(string, string) (authentik.Client, error) { return mock, nil }
	return r
}

func assertNoCall(t *testing.T, calls []string, names ...string) {
	t.Helper()
	for _, n := range names {
		if slices.Contains(calls, n) {
			t.Errorf("unexpected call %s (calls: %v)", n, calls)
		}
	}
}

func TestCrossNamespaceSlugAdoption(t *testing.T) {
	app := newApp("attacker", "team-b")
	app.Spec.Slug = "victim-app"
	app.Spec.GroupBindings = []string{"attackers"}
	app.Finalizers = []string{FinalizerName}

	mock := &recordingClient{mockClient: successMock()}
	mock.getProviderByNameRes = &authentik.ProviderInfo{ID: 7, Name: "victim-app-provider"}
	mock.updateProviderRes = &authentik.ProviderInfo{ID: 7, ClientID: "victim-cid", ClientSecret: "victim-secret"}
	mock.getAppBySlugRes = &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"}
	mock.updateAppRes = &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"}
	r := newRecordingReconciler(t, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("attacker", "team-b")); err == nil {
		t.Fatal("expected error when the provider name belongs to another owner")
	}
	assertNoCall(t, mock.calls, "UpdateOAuth2Provider", "UpdateApplication", "CreateApplication", "SyncGroupBindings")

	secret := &corev1.Secret{}
	err := r.Get(context.Background(), types.NamespacedName{Name: "attacker-oauth", Namespace: "team-b"}, secret)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected no secret in tenant namespace, got err=%v data=%v", err, secret.Data)
	}

	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("attacker", "team-b").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if updated.Status.ProviderID != 0 || updated.Status.ApplicationUID != "" {
		t.Errorf("expected empty status IDs, got ProviderID=%d ApplicationUID=%q", updated.Status.ProviderID, updated.Status.ApplicationUID)
	}

	// Delete the CR. The operator must not delete the foreign application or provider.
	now := metav1.Now()
	updated.DeletionTimestamp = &now
	mock.calls = nil
	mock.getProviderByIDRes = &authentik.ProviderInfo{ID: 7}
	r = newRecordingReconciler(t, updated, mock)
	if _, err := r.Reconcile(context.Background(), reqFor("attacker", "team-b")); err != nil {
		t.Fatalf("unexpected deletion error: %v", err)
	}
	assertNoCall(t, mock.calls, "DeleteApplication", "DeleteOAuth2Provider")
}

func TestReconcile_ForeignApplicationSlugRejected(t *testing.T) {
	app := newApp("attacker", "team-b")
	app.Spec.Slug = "victim-app"
	app.Spec.GroupBindings = []string{"attackers"}
	app.Finalizers = []string{FinalizerName}

	// The provider name is free, but the application slug belongs to another owner.
	mock := &recordingClient{mockClient: successMock()}
	mock.getAppBySlugRes = &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"}
	mock.updateAppRes = &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"}
	r := newRecordingReconciler(t, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("attacker", "team-b")); err == nil {
		t.Fatal("expected error when the application slug belongs to another owner")
	}
	assertNoCall(t, mock.calls, "UpdateApplication", "CreateApplication", "SyncGroupBindings")

	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("attacker", "team-b").NamespacedName, updated); err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if updated.Status.ApplicationUID != "" {
		t.Errorf("expected empty ApplicationUID, got %q", updated.Status.ApplicationUID)
	}
	// The provider this CR created is recorded so that deletion can clean it up.
	if updated.Status.ProviderID != 1 {
		t.Errorf("expected ProviderID=1 for the created provider, got %d", updated.Status.ProviderID)
	}
}

func TestHandleDeletion_DeletesUnownedSlug(t *testing.T) {
	now := metav1.Now()
	app := newApp("attacker", "team-b")
	app.Spec.Slug = "victim-app"
	app.Spec.Secret.Template = "{{ .Invalid"
	app.Finalizers = []string{FinalizerName}
	app.DeletionTimestamp = &now

	mock := &recordingClient{mockClient: &mockClient{
		getAppBySlugRes: &authentik.ApplicationInfo{UID: "victim-uid", Slug: "victim-app"},
	}}
	r := newRecordingReconciler(t, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("attacker", "team-b")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoCall(t, mock.calls, "DeleteApplication", "DeleteOAuth2Provider")

	// The finalizer is removed so that the CR does not get stuck.
	updated := &authentikv1alpha1.AuthentikOAuth2Application{}
	err := r.Get(context.Background(), reqFor("attacker", "team-b").NamespacedName, updated)
	if err == nil && slices.Contains(updated.Finalizers, FinalizerName) {
		t.Error("expected finalizer to be removed")
	}
}

func TestHandleDeletion_DeletesOwnedApplication(t *testing.T) {
	now := metav1.Now()
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.DeletionTimestamp = &now
	app.Status.ApplicationUID = "app-uid"
	app.Status.ProviderID = 42

	mock := &recordingClient{mockClient: &mockClient{
		getAppBySlugRes:    &authentik.ApplicationInfo{UID: "app-uid", Slug: "test-app"},
		getProviderByIDRes: &authentik.ProviderInfo{ID: 42},
	}}
	r := newRecordingReconciler(t, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("test-app", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Contains(mock.calls, "DeleteApplication") || !slices.Contains(mock.calls, "DeleteOAuth2Provider") {
		t.Errorf("expected owned application and provider to be deleted, calls: %v", mock.calls)
	}
}

func TestReconcile_OwnedProviderAndApplicationUpdated(t *testing.T) {
	app := newApp("test-app", "default")
	app.Finalizers = []string{FinalizerName}
	app.Status.ApplicationUID = "app-uid"
	app.Status.ProviderID = 5

	mock := &recordingClient{mockClient: successMock()}
	mock.getProviderByIDRes = &authentik.ProviderInfo{ID: 5, Name: "test-app-provider"}
	mock.updateProviderRes = &authentik.ProviderInfo{ID: 5, Name: "test-app-provider", ClientID: "cid", ClientSecret: "csec"}
	mock.getAppBySlugRes = &authentik.ApplicationInfo{UID: "app-uid", Slug: "test-app"}
	mock.updateAppRes = &authentik.ApplicationInfo{UID: "app-uid", Slug: "test-app"}
	r := newRecordingReconciler(t, app, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("test-app", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Contains(mock.calls, "UpdateOAuth2Provider") || !slices.Contains(mock.calls, "UpdateApplication") {
		t.Errorf("expected owned provider and application to be updated, calls: %v", mock.calls)
	}
}
