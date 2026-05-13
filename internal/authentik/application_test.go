package authentik

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "goauthentik.io/api/v3"
)

func TestGetApplicationBySlug_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		app := api.Application{
			Pk:   "app-uid-123",
			Slug: "my-app",
			Name: "My App",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetApplicationBySlug(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected application info, got nil")
		return
	}
	if info.UID != "app-uid-123" {
		t.Errorf("expected UID %q, got %q", "app-uid-123", info.UID)
	}
	if info.Slug != "my-app" {
		t.Errorf("expected Slug %q, got %q", "my-app", info.Slug)
	}
	if info.Name != "My App" {
		t.Errorf("expected Name %q, got %q", "My App", info.Name)
	}
}

func TestGetApplicationBySlug_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail": "Not found."}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetApplicationBySlug(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for not found, got %+v", info)
	}
}

func TestCreateApplication_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app := api.Application{
			Pk:   "new-app-uid",
			Slug: "new-app",
			Name: "New App",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(app)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.CreateApplication(context.Background(), "new-app", "New App", 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.UID != "new-app-uid" {
		t.Errorf("expected UID %q, got %q", "new-app-uid", info.UID)
	}
}

func TestCreateApplication_WithOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		app := api.Application{
			Pk:   "opts-app-uid",
			Slug: "opts-app",
			Name: "Opts App",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(app)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	openInNewTab := true
	opts := &ApplicationOptions{
		Group:           "test-group",
		MetaLaunchURL:   "https://example.com",
		MetaDescription: "A test app",
		MetaPublisher:   "Test Publisher",
		OpenInNewTab:    &openInNewTab,
	}
	info, err := c.CreateApplication(context.Background(), "opts-app", "Opts App", 1, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Slug != "opts-app" {
		t.Errorf("expected Slug %q, got %q", "opts-app", info.Slug)
	}
}

func TestCreateApplication_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"slug": ["application with this slug already exists."]}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.CreateApplication(context.Background(), "dup-app", "Dup App", 1, nil)
	if err == nil {
		t.Fatal("expected error for API error, got nil")
	}
}

func TestUpdateApplication_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		app := api.Application{
			Pk:   "updated-uid",
			Slug: "my-app",
			Name: "Updated App",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.UpdateApplication(context.Background(), "my-app", "Updated App", 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "Updated App" {
		t.Errorf("expected Name %q, got %q", "Updated App", info.Name)
	}
}

func TestDeleteApplication_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	err := c.DeleteApplication(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteApplication_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail": "internal error"}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	err := c.DeleteApplication(context.Background(), "my-app")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
