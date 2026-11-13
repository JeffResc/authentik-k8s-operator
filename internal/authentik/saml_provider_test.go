package authentik

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		return
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

// samlIdentityConflictServer fakes Authentik. It answers flow lookups and
// lists one existing SAML provider named owner for the given issuer and audience.
// It counts provider create and update calls.
func samlIdentityConflictServer(t *testing.T, owner, issuer, audience string, writes *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, "/api/v3/flows/instances/") && r.Method == http.MethodGet:
			slug := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v3/flows/instances/"), "/")
			json.NewEncoder(w).Encode(api.Flow{Pk: "flow-uuid-" + slug, Slug: slug, Name: slug, Title: slug,
				Designation: api.FLOWDESIGNATIONENUM_AUTHORIZATION, Stages: []string{}, Policies: []string{}})
		case path == "/api/v3/providers/saml/" && r.Method == http.MethodGet:
			q := r.URL.Query()
			results := []api.SAMLProvider{}
			if (q.Has("issuer_override") && q.Get("issuer_override") == issuer) ||
				(q.Has("audience") && q.Get("audience") == audience) {
				p := api.SAMLProvider{Pk: 1, Name: owner}
				p.SetIssuerOverride(issuer)
				p.SetAudience(audience)
				results = append(results, p)
			}
			json.NewEncoder(w).Encode(api.PaginatedSAMLProviderList{
				Pagination: api.Pagination{Count: float32(len(results)), Current: 1, TotalPages: 1},
				Results:    results,
			})
		case path == "/api/v3/providers/saml/" && r.Method == http.MethodPost:
			*writes++
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(api.SAMLProvider{Pk: 2, Name: "tenant-b-provider"})
		case strings.HasPrefix(path, "/api/v3/providers/saml/") && r.Method == http.MethodPut:
			*writes++
			json.NewEncoder(w).Encode(api.SAMLProvider{Pk: 1, Name: owner})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"detail": "Not found."})
		}
	}))
}

func TestCreateSAMLProvider_RejectsIssuerOrAudienceOfOtherProvider(t *testing.T) {
	const issuer = "https://idp.example/team-a"
	const audience = "https://sp-a.example"

	tests := []struct {
		name     string
		issuer   string
		audience string
	}{
		{"issuer", issuer, "https://sp-b.example"},
		{"audience", "https://idp.example/team-b", audience},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			server := samlIdentityConflictServer(t, "team-a-provider", issuer, audience, &writes)
			defer server.Close()

			c := newTestClient(t, server)
			_, err := c.CreateSAMLProvider(context.Background(), "tenant-b-provider", &SAMLProviderOptions{
				AuthorizationFlow: "auth-flow",
				InvalidationFlow:  "inval-flow",
				ACSUrl:            "https://tenant-b.example/acs",
				Issuer:            tt.issuer,
				Audience:          tt.audience,
			})
			if err == nil {
				t.Fatal("expected error for issuer/audience used by another provider, got nil")
			}
			if !strings.Contains(err.Error(), "team-a-provider") {
				t.Errorf("expected error to name the conflicting provider, got %v", err)
			}
			if writes != 0 {
				t.Errorf("expected no provider create call, got %d", writes)
			}
		})
	}
}

func TestUpdateSAMLProvider_AllowsOwnIssuerAndAudience(t *testing.T) {
	const issuer = "https://idp.example/team-a"
	const audience = "https://sp-a.example"
	writes := 0
	server := samlIdentityConflictServer(t, "team-a-provider", issuer, audience, &writes)
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.UpdateSAMLProvider(context.Background(), 1, "team-a-provider", &SAMLProviderOptions{
		AuthorizationFlow: "auth-flow",
		InvalidationFlow:  "inval-flow",
		ACSUrl:            "https://sp-a.example/acs",
		Issuer:            issuer,
		Audience:          audience,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writes != 1 {
		t.Errorf("expected 1 provider update call, got %d", writes)
	}
}
