package authentik

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "goauthentik.io/api/v3"
)

// newTestClient creates an APIClient pointed at the given httptest server.
func newTestClient(t *testing.T, server *httptest.Server) *APIClient {
	t.Helper()
	cfg := api.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = server.Listener.Addr().String()
	cfg.AddDefaultHeader("Authorization", "Bearer test-token")
	return &APIClient{
		api:     api.NewAPIClient(cfg),
		baseURL: server.URL,
	}
}

func TestNewClient_Valid(t *testing.T) {
	c, err := NewClient("https://auth.example.com", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.GetBaseURL() != "https://auth.example.com" {
		t.Errorf("expected base URL %q, got %q", "https://auth.example.com", c.GetBaseURL())
	}
}

func TestNewClient_TrailingSlash(t *testing.T) {
	c, err := NewClient("https://auth.example.com/", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.GetBaseURL() != "https://auth.example.com" {
		t.Errorf("expected trailing slash stripped, got %q", c.GetBaseURL())
	}
}

func TestNewClient_HTTPScheme(t *testing.T) {
	c, err := NewClient("http://localhost:9000", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.GetBaseURL() != "http://localhost:9000" {
		t.Errorf("expected base URL %q, got %q", "http://localhost:9000", c.GetBaseURL())
	}
}

func TestHealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/core/brands/current/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		brand := api.CurrentBrand{
			MatchedDomain: "example.com",
			BrandingTitle: "Test",
			UiTheme:       api.UITHEMEENUM_AUTOMATIC,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(brand)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	if err := c.HealthCheck(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHealthCheck_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	err := c.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestGetCertificateByName_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cert := api.CertificateKeyPair{}
		cert.Pk = "cert-uuid-123"
		cert.Name = "my-cert"
		cert.PrivateKeyAvailable = true
		cert.CertificateDownloadUrl = ""
		cert.PrivateKeyDownloadUrl = ""

		resp := api.PaginatedCertificateKeyPairList{
			Pagination: api.Pagination{Count: 1, Current: 1, TotalPages: 1, StartIndex: 1, EndIndex: 1},
			Results:    []api.CertificateKeyPair{cert},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	uuid, err := c.GetCertificateByName(context.Background(), "my-cert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uuid != "cert-uuid-123" {
		t.Errorf("expected uuid %q, got %q", "cert-uuid-123", uuid)
	}
}

func TestGetCertificateByName_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := api.PaginatedCertificateKeyPairList{
			Pagination: api.Pagination{Count: 0, Current: 1, TotalPages: 0},
			Results:    []api.CertificateKeyPair{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.GetCertificateByName(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing certificate, got nil")
	}
}

func TestGetScopeMappingByName_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mapping := api.ScopeMapping{}
		mapping.Pk = "scope-uuid-456"
		mapping.Name = "openid"
		mapping.Expression = "return True"
		mapping.ScopeName = "openid"
		mapping.Component = "ak-provider-scope-mapping-form"
		mapping.VerboseName = "Scope Mapping"
		mapping.VerboseNamePlural = "Scope Mappings"
		mapping.MetaModelName = "authentik_providers_oauth2.scopemapping"

		resp := api.PaginatedScopeMappingList{
			Pagination: api.Pagination{Count: 1, Current: 1, TotalPages: 1, StartIndex: 1, EndIndex: 1},
			Results:    []api.ScopeMapping{mapping},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	uuid, err := c.GetScopeMappingByName(context.Background(), "openid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uuid != "scope-uuid-456" {
		t.Errorf("expected uuid %q, got %q", "scope-uuid-456", uuid)
	}
}

func TestGetScopeMappingByName_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := api.PaginatedScopeMappingList{
			Pagination: api.Pagination{Count: 0, Current: 1, TotalPages: 0},
			Results:    []api.ScopeMapping{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.GetScopeMappingByName(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing scope mapping, got nil")
	}
}

func TestExtractAPIError_StandardError(t *testing.T) {
	err := extractAPIError(http.ErrServerClosed, "test operation")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	expected := "test operation: http: Server closed"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
