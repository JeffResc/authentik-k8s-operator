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

// fullAPIHandler provides a mock Authentik server that handles flow lookups,
// scope mappings, certificates, SAML property mappings, and provider CRUD.
type fullAPIHandler struct{}

func (h *fullAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path

	switch {
	// Flow retrieval
	case strings.HasPrefix(path, "/api/v3/flows/instances/") && r.Method == http.MethodGet:
		slug := strings.TrimPrefix(path, "/api/v3/flows/instances/")
		slug = strings.TrimSuffix(slug, "/")
		flow := api.FlowSet{Pk: "flow-uuid-" + slug, Slug: slug, Name: slug, Title: slug}
		flow.SetDesignation(api.FLOWDESIGNATIONENUM_AUTHORIZATION)
		json.NewEncoder(w).Encode(flow)

	// Scope mapping list
	case path == "/api/v3/propertymappings/provider/scope/" && r.Method == http.MethodGet:
		scopeName := r.URL.Query().Get("scope_name")
		mapping := api.ScopeMapping{Pk: "scope-uuid-" + scopeName, Name: scopeName, ScopeName: scopeName}
		mapping.Expression = "return True"
		mapping.Component = "ak-provider-scope-mapping-form"
		mapping.VerboseName = "Scope Mapping"
		mapping.VerboseNamePlural = "Scope Mappings"
		mapping.MetaModelName = "authentik_providers_oauth2.scopemapping"
		resp := api.PaginatedScopeMappingList{
			Pagination: api.Pagination{Count: 1, Current: 1, TotalPages: 1, StartIndex: 1, EndIndex: 1},
			Results:    []api.ScopeMapping{mapping},
		}
		json.NewEncoder(w).Encode(resp)

	// Certificate list
	case path == "/api/v3/crypto/certificatekeypairs/" && r.Method == http.MethodGet:
		cert := api.CertificateKeyPair{Pk: "cert-uuid-123", Name: "test-cert"}
		cert.PrivateKeyAvailable = true
		resp := api.PaginatedCertificateKeyPairList{
			Pagination: api.Pagination{Count: 1, Current: 1, TotalPages: 1, StartIndex: 1, EndIndex: 1},
			Results:    []api.CertificateKeyPair{cert},
		}
		json.NewEncoder(w).Encode(resp)

	// SAML property mapping list
	case path == "/api/v3/propertymappings/provider/saml/" && r.Method == http.MethodGet:
		name := r.URL.Query().Get("name")
		mapping := api.SAMLPropertyMapping{Pk: "saml-pm-uuid-" + name, Name: name}
		mapping.Expression = "return {}"
		mapping.Component = "ak-property-mapping-saml"
		mapping.VerboseName = "SAML Property Mapping"
		mapping.VerboseNamePlural = "SAML Property Mappings"
		mapping.MetaModelName = "authentik_providers_saml.samlpropertymapping"
		resp := api.PaginatedSAMLPropertyMappingList{
			Pagination: api.Pagination{Count: 1, Current: 1, TotalPages: 1, StartIndex: 1, EndIndex: 1},
			Results:    []api.SAMLPropertyMapping{mapping},
		}
		json.NewEncoder(w).Encode(resp)

	// OAuth2 provider create
	case path == "/api/v3/providers/oauth2/" && r.Method == http.MethodPost:
		provider := api.OAuth2Provider{Pk: 100, Name: "created-provider"}
		provider.SetClientId("new-cid")
		provider.SetClientSecret("new-csec")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(provider)

	// OAuth2 provider update
	case strings.HasPrefix(path, "/api/v3/providers/oauth2/") && r.Method == http.MethodPut:
		provider := api.OAuth2Provider{Pk: 100, Name: "updated-provider"}
		provider.SetClientId("upd-cid")
		provider.SetClientSecret("upd-csec")
		json.NewEncoder(w).Encode(provider)

	// SAML provider create
	case path == "/api/v3/providers/saml/" && r.Method == http.MethodPost:
		provider := api.SAMLProvider{Pk: 200, Name: "created-saml-provider"}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(provider)

	// SAML provider update
	case strings.HasPrefix(path, "/api/v3/providers/saml/") && r.Method == http.MethodPut:
		provider := api.SAMLProvider{Pk: 200, Name: "updated-saml-provider"}
		json.NewEncoder(w).Encode(provider)

	default:
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"detail": "Not found."})
	}
}

func TestCreateOAuth2Provider_Success(t *testing.T) {
	server := httptest.NewServer(&fullAPIHandler{})
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.CreateOAuth2Provider(context.Background(), "test-provider", &OAuth2ProviderOptions{
		AuthorizationFlow: "auth-flow",
		InvalidationFlow:  "inval-flow",
		RedirectURIs:      []string{"https://example.com/callback"},
		Scopes:            []string{"openid", "email"},
		SigningKey:        "test-cert",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != 100 {
		t.Errorf("expected ID 100, got %d", info.ID)
	}
	if info.ClientID != "new-cid" {
		t.Errorf("expected ClientID %q, got %q", "new-cid", info.ClientID)
	}
}

func TestUpdateOAuth2Provider_Success(t *testing.T) {
	server := httptest.NewServer(&fullAPIHandler{})
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.UpdateOAuth2Provider(context.Background(), 100, "test-provider", &OAuth2ProviderOptions{
		AuthorizationFlow: "auth-flow",
		InvalidationFlow:  "inval-flow",
		RedirectURIs:      []string{"https://example.com/callback"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != 100 {
		t.Errorf("expected ID 100, got %d", info.ID)
	}
}

func TestCreateOAuth2Provider_WithAllOptions(t *testing.T) {
	server := httptest.NewServer(&fullAPIHandler{})
	defer server.Close()

	includeClaimsInToken := true
	c := newTestClient(t, server)
	_, err := c.CreateOAuth2Provider(context.Background(), "full-opts", &OAuth2ProviderOptions{
		AuthorizationFlow:    "auth-flow",
		InvalidationFlow:     "inval-flow",
		RedirectURIs:         []string{"https://example.com/callback"},
		ClientType:           "confidential",
		AccessCodeValidity:   "minutes=1",
		AccessTokenValidity:  "minutes=5",
		RefreshTokenValidity: "days=30",
		SubMode:              "hashed_user_id",
		IncludeClaimsInToken: &includeClaimsInToken,
		IssuerMode:           "per_provider",
		Scopes:               []string{"openid"},
		SigningKey:           "test-cert",
		PropertyMappings:     []string{"extra-uuid"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateSAMLProvider_Success(t *testing.T) {
	server := httptest.NewServer(&fullAPIHandler{})
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.CreateSAMLProvider(context.Background(), "saml-provider", &SAMLProviderOptions{
		AuthorizationFlow: "auth-flow",
		InvalidationFlow:  "inval-flow",
		ACSUrl:            "https://example.com/acs",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != 200 {
		t.Errorf("expected ID 200, got %d", info.ID)
	}
}

func TestUpdateSAMLProvider_Success(t *testing.T) {
	server := httptest.NewServer(&fullAPIHandler{})
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.UpdateSAMLProvider(context.Background(), 200, "saml-provider", &SAMLProviderOptions{
		AuthorizationFlow: "auth-flow",
		InvalidationFlow:  "inval-flow",
		ACSUrl:            "https://example.com/acs",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != 200 {
		t.Errorf("expected ID 200, got %d", info.ID)
	}
}

func TestCreateSAMLProvider_WithAllOptions(t *testing.T) {
	server := httptest.NewServer(&fullAPIHandler{})
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.CreateSAMLProvider(context.Background(), "full-saml", &SAMLProviderOptions{
		AuthorizationFlow:  "auth-flow",
		InvalidationFlow:   "inval-flow",
		ACSUrl:             "https://example.com/acs",
		Issuer:             "https://idp.example.com",
		Audience:           "https://sp.example.com",
		SPBinding:          "post",
		DigestAlgorithm:    "http://www.w3.org/2001/04/xmlenc#sha256",
		SignatureAlgorithm: "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256",
		SigningKeypair:     "test-cert",
		PropertyMappings:   []string{"saml-email-mapping"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSAMLPropertyMappingByName_Found(t *testing.T) {
	server := httptest.NewServer(&fullAPIHandler{})
	defer server.Close()

	c := newTestClient(t, server)
	uuid, err := c.GetSAMLPropertyMappingByName(context.Background(), "email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uuid != "saml-pm-uuid-email" {
		t.Errorf("expected %q, got %q", "saml-pm-uuid-email", uuid)
	}
}

func TestGetSAMLPropertyMappingByName_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := api.PaginatedSAMLPropertyMappingList{
			Pagination: api.Pagination{Count: 0, Current: 1, TotalPages: 0},
			Results:    []api.SAMLPropertyMapping{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.GetSAMLPropertyMappingByName(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing SAML property mapping, got nil")
	}
}
