package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := authentikv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	return s
}

func TestReceiver_PostEnqueuesEvents(t *testing.T) {
	app := &authentikv1alpha1.AuthentikOAuth2Application{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(app).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "")

	payload := map[string]string{"body": "test event", "severity": "notice"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	select {
	case evt := <-eventChan:
		if evt.Object.GetName() != "my-app" {
			t.Errorf("expected event for 'my-app', got %q", evt.Object.GetName())
		}
	case <-time.After(time.Second):
		t.Fatal("expected event on channel, got none")
	}
}

func TestReceiver_EnqueuesMultipleApps(t *testing.T) {
	app1 := &authentikv1alpha1.AuthentikOAuth2Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
	}
	app2 := &authentikv1alpha1.AuthentikOAuth2Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-2", Namespace: "ns2"},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(app1, app2).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "")

	payload := map[string]string{"body": "model updated"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Should have 2 events enqueued
	count := 0
	timeout := time.After(time.Second)
	for {
		select {
		case <-eventChan:
			count++
			if count == 2 {
				return
			}
		case <-timeout:
			t.Fatalf("expected 2 events, got %d", count)
		}
	}
}

func TestReceiver_RejectsGet(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "")

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestReceiver_RejectsInvalidJSON(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestReceiver_NoAppsNoEvents(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "")

	payload := map[string]string{"body": "test"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Channel should be empty
	select {
	case evt := <-eventChan:
		t.Fatalf("expected no events, got %v", evt)
	default:
		// Expected
	}
}

func TestReceiver_FullChannelDoesNotBlock(t *testing.T) {
	app := &authentikv1alpha1.AuthentikOAuth2Application{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(app).Build()
	// Buffer of 0 — immediately full
	eventChan := make(chan event.GenericEvent)

	receiver := NewReceiver(k8sClient, eventChan, "")

	payload := map[string]string{"body": "test"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	// This should not block even though the channel is full
	done := make(chan struct{})
	go func() {
		receiver.ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-done:
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeHTTP blocked on full channel")
	}
}

func TestReceiver_ManyAppsChannelSaturation(t *testing.T) {
	const numApps = 200
	const channelBuffer = 50

	// Create 200 apps
	builder := fake.NewClientBuilder().WithScheme(newScheme(t))
	for i := 0; i < numApps; i++ {
		builder = builder.WithObjects(&authentikv1alpha1.AuthentikOAuth2Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("app-%03d", i),
				Namespace: "default",
			},
		})
	}
	k8sClient := builder.Build()
	eventChan := make(chan event.GenericEvent, channelBuffer)

	receiver := NewReceiver(k8sClient, eventChan, "")

	payload := map[string]string{"body": "bulk event", "severity": "notice"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	// Should complete without blocking despite channel being too small for all apps
	done := make(chan struct{})
	go func() {
		receiver.ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-done:
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeHTTP blocked with many apps and small channel buffer")
	}

	// Channel should be at capacity
	if len(eventChan) != channelBuffer {
		t.Errorf("expected channel to be full at %d, got %d", channelBuffer, len(eventChan))
	}
}

func TestReceiver_AuthRejectsMissingToken(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "my-secret")

	payload := map[string]string{"body": "test"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestReceiver_AuthRejectsWrongToken(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "my-secret")

	payload := map[string]string{"body": "test"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestReceiver_AuthAcceptsCorrectToken(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "my-secret")

	payload := map[string]string{"body": "test"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer my-secret")
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReceiver_AuthRejectsNonBearerScheme(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	eventChan := make(chan event.GenericEvent, 10)

	receiver := NewReceiver(k8sClient, eventChan, "my-secret")

	payload := map[string]string{"body": "test"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Basic my-secret")
	rr := httptest.NewRecorder()

	receiver.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
