package authentik

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "goauthentik.io/api/v3"
)

func TestGetSAMLProviderByName_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		provider := api.SAMLProvider{
			Pk:   10,
			Name: "saml-provider",
		}
		resp := api.PaginatedSAMLProviderList{
			Pagination: api.Pagination{Count: 1, Current: 1, TotalPages: 1, StartIndex: 1, EndIndex: 1},
			Results:    []api.SAMLProvider{provider},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetSAMLProviderByName(context.Background(), "saml-provider")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected provider info, got nil")
	}
	if info.ID != 10 {
		t.Errorf("expected ID 10, got %d", info.ID)
	}
	if info.Name != "saml-provider" {
		t.Errorf("expected name %q, got %q", "saml-provider", info.Name)
	}
}

func TestGetSAMLProviderByName_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := api.PaginatedSAMLProviderList{
			Pagination: api.Pagination{Count: 0, Current: 1, TotalPages: 0},
			Results:    []api.SAMLProvider{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetSAMLProviderByName(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for not found, got %+v", info)
	}
}

func TestGetSAMLProviderByName_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail": "server error"}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.GetSAMLProviderByName(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetSAMLProviderByID_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		provider := api.SAMLProvider{
			Pk:   20,
			Name: "provider-20",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(provider)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetSAMLProviderByID(context.Background(), 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != 20 {
		t.Errorf("expected ID 20, got %d", info.ID)
	}
}

func TestGetSAMLProviderByID_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail": "Not found."}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetSAMLProviderByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for not found, got %+v", info)
	}
}

func TestDeleteSAMLProvider_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	err := c.DeleteSAMLProvider(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSAMLProvider_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail": "internal error"}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	err := c.DeleteSAMLProvider(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetSAMLProviderMetadata_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := api.SAMLMetadata{}
		resp.SetMetadata("<md:EntityDescriptor>test</md:EntityDescriptor>")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	metadata, err := c.GetSAMLProviderMetadata(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metadata != "<md:EntityDescriptor>test</md:EntityDescriptor>" {
		t.Errorf("unexpected metadata: %q", metadata)
	}
}

func TestGetSAMLProviderMetadata_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail": "Not found."}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.GetSAMLProviderMetadata(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for not found provider, got nil")
	}
}

func TestCreateSAMLProvider_NilOpts(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	_, err := c.CreateSAMLProvider(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error for nil opts, got nil")
	}
}

func TestCreateSAMLProvider_MissingAuthorizationFlow(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	_, err := c.CreateSAMLProvider(context.Background(), "test", &SAMLProviderOptions{
		InvalidationFlow: "inval-flow",
		ACSUrl:           "https://example.com/acs",
	})
	if err == nil {
		t.Fatal("expected error for missing authorization flow, got nil")
	}
}

func TestCreateSAMLProvider_MissingInvalidationFlow(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	_, err := c.CreateSAMLProvider(context.Background(), "test", &SAMLProviderOptions{
		AuthorizationFlow: "auth-flow",
		ACSUrl:            "https://example.com/acs",
	})
	if err == nil {
		t.Fatal("expected error for missing invalidation flow, got nil")
	}
}

func TestCreateSAMLProvider_MissingACSUrl(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	_, err := c.CreateSAMLProvider(context.Background(), "test", &SAMLProviderOptions{
		AuthorizationFlow: "auth-flow",
		InvalidationFlow:  "inval-flow",
	})
	if err == nil {
		t.Fatal("expected error for missing ACS URL, got nil")
	}
}

func TestUpdateSAMLProvider_NilOpts(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	_, err := c.UpdateSAMLProvider(context.Background(), 1, "test", nil)
	if err == nil {
		t.Fatal("expected error for nil opts, got nil")
	}
}

func TestIsUUID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"ABCDEF00-1234-5678-9ABC-DEF012345678", true},
		{"00000000-0000-0000-0000-000000000000", true},
		{"not-a-uuid", false},
		{"", false},
		{"550e8400-e29b-41d4-a716-44665544000", false},   // too short
		{"550e8400-e29b-41d4-a716-4466554400000", false}, // too long
		{"550e8400e29b41d4a716446655440000", false},      // no dashes
		{"authentik-saml-email", false},                  // property mapping name
		{"urn:oasis:names:tc:SAML:2.0", false},           // SAML URN
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isUUID(tt.input); got != tt.want {
				t.Errorf("isUUID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
