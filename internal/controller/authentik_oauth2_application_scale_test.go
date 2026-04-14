package controller

import (
	"context"
	"fmt"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
)

// trackingMockClient wraps mockClient and tracks call counts in a thread-safe way.
type trackingMockClient struct {
	mu                   sync.Mutex
	createProviderCalls  int
	updateProviderCalls  int
	createAppCalls       int
	updateAppCalls       int
	getProviderURLsCalls int

	// Per-name results to support unique IDs across many resources
	providerIDByName map[string]int32
	appUIDByName     map[string]string
	nextProviderID   int32
}

func newTrackingMock() *trackingMockClient {
	return &trackingMockClient{
		providerIDByName: make(map[string]int32),
		appUIDByName:     make(map[string]string),
		nextProviderID:   1,
	}
}

func (m *trackingMockClient) HealthCheck(context.Context) error { return nil }
func (m *trackingMockClient) GetOAuth2ProviderByName(_ context.Context, name string) (*authentik.ProviderInfo, error) {
	return nil, nil
}
func (m *trackingMockClient) GetOAuth2ProviderByID(context.Context, int32) (*authentik.ProviderInfo, error) {
	return nil, nil
}
func (m *trackingMockClient) CreateOAuth2Provider(_ context.Context, name string, _ *authentik.OAuth2ProviderOptions) (*authentik.ProviderInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createProviderCalls++
	id := m.nextProviderID
	m.nextProviderID++
	m.providerIDByName[name] = id
	return &authentik.ProviderInfo{ID: id, Name: name, ClientID: fmt.Sprintf("cid-%d", id), ClientSecret: fmt.Sprintf("csec-%d", id)}, nil
}
func (m *trackingMockClient) UpdateOAuth2Provider(_ context.Context, id int32, name string, _ *authentik.OAuth2ProviderOptions) (*authentik.ProviderInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateProviderCalls++
	return &authentik.ProviderInfo{ID: id, Name: name, ClientID: fmt.Sprintf("cid-%d", id), ClientSecret: fmt.Sprintf("csec-%d", id)}, nil
}
func (m *trackingMockClient) DeleteOAuth2Provider(context.Context, int32) error { return nil }
func (m *trackingMockClient) GetOAuth2ProviderURLs(_ context.Context, id int32) (*authentik.ProviderURLs, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getProviderURLsCalls++
	return &authentik.ProviderURLs{
		Issuer: fmt.Sprintf("https://auth/issuer/%d", id), Authorize: "https://auth/authorize",
		Token: "https://auth/token", UserInfo: "https://auth/userinfo",
		Logout: "https://auth/logout", JWKS: "https://auth/jwks",
		ProviderInfo: "https://auth/.well-known",
	}, nil
}
func (m *trackingMockClient) GetSAMLProviderByName(context.Context, string) (*authentik.SAMLProviderInfo, error) {
	return nil, nil
}
func (m *trackingMockClient) GetSAMLProviderByID(context.Context, int32) (*authentik.SAMLProviderInfo, error) {
	return nil, nil
}
func (m *trackingMockClient) CreateSAMLProvider(context.Context, string, *authentik.SAMLProviderOptions) (*authentik.SAMLProviderInfo, error) {
	return nil, nil
}
func (m *trackingMockClient) UpdateSAMLProvider(context.Context, int32, string, *authentik.SAMLProviderOptions) (*authentik.SAMLProviderInfo, error) {
	return nil, nil
}
func (m *trackingMockClient) DeleteSAMLProvider(context.Context, int32) error { return nil }
func (m *trackingMockClient) GetSAMLProviderMetadata(context.Context, int32) (string, error) {
	return "", nil
}
func (m *trackingMockClient) GetApplicationBySlug(_ context.Context, slug string) (*authentik.ApplicationInfo, error) {
	return nil, nil
}
func (m *trackingMockClient) CreateApplication(_ context.Context, slug, name string, _ int32, _ *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createAppCalls++
	uid := fmt.Sprintf("uid-%s", slug)
	m.appUIDByName[slug] = uid
	return &authentik.ApplicationInfo{UID: uid, Slug: slug, Name: name}, nil
}
func (m *trackingMockClient) UpdateApplication(_ context.Context, slug, name string, _ int32, _ *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateAppCalls++
	return &authentik.ApplicationInfo{UID: fmt.Sprintf("uid-%s", slug), Slug: slug, Name: name}, nil
}
func (m *trackingMockClient) DeleteApplication(context.Context, string) error { return nil }

func TestScale_ManyResourcesReconcileIndependently(t *testing.T) {
	const numApps = 50
	s := newScheme(t)
	mock := newTrackingMock()

	// Create all apps as runtime objects
	objs := make([]runtime.Object, numApps)
	for i := 0; i < numApps; i++ {
		app := newApp(fmt.Sprintf("app-%03d", i), "default")
		app.Finalizers = []string{FinalizerName}
		objs[i] = app
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Build()

	r := &AuthentikOAuth2ApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       events.NewFakeRecorder(numApps * 2),
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		RequeueDelay:   DefaultRequeueDelay,
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return mock, nil
		},
	}

	for i := 0; i < numApps; i++ {
		name := fmt.Sprintf("app-%03d", i)
		result, err := r.Reconcile(context.Background(), reqFor(name, "default"))
		if err != nil {
			t.Fatalf("app %s: unexpected error: %v", name, err)
		}
		if result.RequeueAfter != DefaultRequeueDelay {
			t.Errorf("app %s: expected RequeueAfter=%v, got %v", name, DefaultRequeueDelay, result.RequeueAfter)
		}
	}

	// Verify each app got its own secret
	for i := 0; i < numApps; i++ {
		name := fmt.Sprintf("app-%03d", i)
		secretName := name + "-oauth"
		secret := &corev1.Secret{}
		if err := r.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: "default"}, secret); err != nil {
			t.Errorf("app %s: expected secret %s to exist: %v", name, secretName, err)
		}
	}

	// Verify mock call counts
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.createProviderCalls != numApps {
		t.Errorf("expected %d provider creates, got %d", numApps, mock.createProviderCalls)
	}
	if mock.createAppCalls != numApps {
		t.Errorf("expected %d app creates, got %d", numApps, mock.createAppCalls)
	}
	if mock.getProviderURLsCalls != numApps {
		t.Errorf("expected %d provider URL fetches, got %d", numApps, mock.getProviderURLsCalls)
	}
}

func TestScale_RapidSpecUpdates(t *testing.T) {
	const numUpdates = 10
	s := newScheme(t)
	mock := newTrackingMock()

	app := newApp("rapid-app", "default")
	app.Finalizers = []string{FinalizerName}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(app).
		WithStatusSubresource(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Build()

	recorder := events.NewFakeRecorder(numUpdates * 2)
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

	for gen := int64(1); gen <= numUpdates; gen++ {
		// Simulate spec change by updating the app's generation
		current := &authentikv1alpha1.AuthentikOAuth2Application{}
		if err := r.Get(context.Background(), reqFor("rapid-app", "default").NamespacedName, current); err != nil {
			t.Fatalf("gen %d: failed to get app: %v", gen, err)
		}
		current.Generation = gen
		current.Spec.Name = fmt.Sprintf("Rapid App Gen %d", gen)
		if err := r.Update(context.Background(), current); err != nil {
			t.Fatalf("gen %d: failed to update app: %v", gen, err)
		}

		_, err := r.Reconcile(context.Background(), reqFor("rapid-app", "default"))
		if err != nil {
			t.Fatalf("gen %d: unexpected error: %v", gen, err)
		}
	}

	// Verify final status
	final := &authentikv1alpha1.AuthentikOAuth2Application{}
	if err := r.Get(context.Background(), reqFor("rapid-app", "default").NamespacedName, final); err != nil {
		t.Fatalf("failed to get final app: %v", err)
	}
	if final.Status.ObservedGeneration != numUpdates {
		t.Errorf("expected ObservedGeneration=%d, got %d", numUpdates, final.Status.ObservedGeneration)
	}

	// Count Synced events -- each generation change should produce one
	syncedCount := 0
	for {
		select {
		case <-recorder.Events:
			syncedCount++
		default:
			goto done
		}
	}
done:
	if syncedCount != numUpdates {
		t.Errorf("expected %d Synced events, got %d", numUpdates, syncedCount)
	}
}

func TestScale_ConcurrentReconciles(t *testing.T) {
	const numApps = 10
	s := newScheme(t)
	mock := newTrackingMock()

	objs := make([]runtime.Object, numApps)
	for i := 0; i < numApps; i++ {
		app := newApp(fmt.Sprintf("concurrent-%03d", i), "default")
		app.Finalizers = []string{FinalizerName}
		objs[i] = app
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&authentikv1alpha1.AuthentikOAuth2Application{}).
		Build()

	r := &AuthentikOAuth2ApplicationReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       events.NewFakeRecorder(numApps * 2),
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		RequeueDelay:   DefaultRequeueDelay,
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return mock, nil
		},
	}

	var wg sync.WaitGroup
	errs := make([]error, numApps)

	for i := 0; i < numApps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("concurrent-%03d", idx)
			_, err := r.Reconcile(context.Background(), reqFor(name, "default"))
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent-%03d: unexpected error: %v", i, err)
		}
	}

	// Verify each got its own secret
	for i := 0; i < numApps; i++ {
		name := fmt.Sprintf("concurrent-%03d", i)
		secret := &corev1.Secret{}
		if err := r.Get(context.Background(), types.NamespacedName{Name: name + "-oauth", Namespace: "default"}, secret); err != nil {
			t.Errorf("%s: expected secret to exist: %v", name, err)
		}
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.createProviderCalls != numApps {
		t.Errorf("expected %d provider creates, got %d", numApps, mock.createProviderCalls)
	}
}
