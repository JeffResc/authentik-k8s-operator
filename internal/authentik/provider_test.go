package authentik

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "goauthentik.io/api/v3"
)

func TestGetOAuth2ProviderByName_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		provider := api.OAuth2Provider{
			Pk:   42,
			Name: "test-provider",
		}
		provider.SetClientId("client-id-abc")
		provider.SetClientSecret("client-secret-xyz")

		resp := api.PaginatedOAuth2ProviderList{
			Pagination: api.Pagination{Count: 1, Current: 1, TotalPages: 1, StartIndex: 1, EndIndex: 1},
			Results:    []api.OAuth2Provider{provider},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetOAuth2ProviderByName(context.Background(), "test-provider")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected provider info, got nil")
		return
	}
	if info.ID != 42 {
		t.Errorf("expected ID 42, got %d", info.ID)
	}
	if info.ClientID != "client-id-abc" {
		t.Errorf("expected ClientID %q, got %q", "client-id-abc", info.ClientID)
	}
	if info.ClientSecret != "client-secret-xyz" {
		t.Errorf("expected ClientSecret %q, got %q", "client-secret-xyz", info.ClientSecret)
	}
}

func TestGetOAuth2ProviderByName_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := api.PaginatedOAuth2ProviderList{
			Pagination: api.Pagination{Count: 0, Current: 1, TotalPages: 0},
			Results:    []api.OAuth2Provider{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetOAuth2ProviderByName(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for not found, got %+v", info)
	}
}

func TestGetOAuth2ProviderByID_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		provider := api.OAuth2Provider{
			Pk:   99,
			Name: "provider-99",
		}
		provider.SetClientId("cid")
		provider.SetClientSecret("csec")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(provider)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetOAuth2ProviderByID(context.Background(), 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != 99 {
		t.Errorf("expected ID 99, got %d", info.ID)
	}
}

func TestGetOAuth2ProviderByID_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail": "Not found."}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	info, err := c.GetOAuth2ProviderByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for not found, got %+v", info)
	}
}

func TestDeleteOAuth2Provider_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	err := c.DeleteOAuth2Provider(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteOAuth2Provider_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail": "internal error"}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	err := c.DeleteOAuth2Provider(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetOAuth2ProviderURLs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		urls := api.NewOAuth2ProviderSetupURLs(
			"https://auth.example.com/application/o/test/",
			"https://auth.example.com/application/o/authorize/",
			"https://auth.example.com/application/o/token/",
			"https://auth.example.com/application/o/userinfo/",
			"https://auth.example.com/application/o/test/.well-known/openid-configuration",
			"https://auth.example.com/application/o/test/end-session/",
			"https://auth.example.com/application/o/test/jwks/",
		)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(urls)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	urls, err := c.GetOAuth2ProviderURLs(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.Issuer != "https://auth.example.com/application/o/test/" {
		t.Errorf("unexpected Issuer: %q", urls.Issuer)
	}
	if urls.JWKS != "https://auth.example.com/application/o/test/jwks/" {
		t.Errorf("unexpected JWKS: %q", urls.JWKS)
	}
}

func TestGetOAuth2ProviderURLs_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail": "Not found."}`))
	}))
	defer server.Close()

	c := newTestClient(t, server)
	_, err := c.GetOAuth2ProviderURLs(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for not found provider, got nil")
	}
}

func TestCreateOAuth2Provider_NilOpts(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	_, err := c.CreateOAuth2Provider(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error for nil opts, got nil")
	}
}

func TestCreateOAuth2Provider_ValidationFailure(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	// Missing required fields
	_, err := c.CreateOAuth2Provider(context.Background(), "test", &OAuth2ProviderOptions{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestUpdateOAuth2Provider_NilOpts(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	_, err := c.UpdateOAuth2Provider(context.Background(), 1, "test", nil)
	if err == nil {
		t.Fatal("expected error for nil opts, got nil")
	}
}

func TestUpdateOAuth2Provider_ValidationFailure(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	_, err := c.UpdateOAuth2Provider(context.Background(), 1, "test", &OAuth2ProviderOptions{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestOAuth2ProviderOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    OAuth2ProviderOptions
		wantErr bool
	}{
		{
			name: "valid",
			opts: OAuth2ProviderOptions{
				AuthorizationFlow: "auth-flow",
				InvalidationFlow:  "inval-flow",
				RedirectURIs:      []string{"https://example.com/callback"},
			},
			wantErr: false,
		},
		{
			name: "missing authorization flow",
			opts: OAuth2ProviderOptions{
				InvalidationFlow: "inval-flow",
				RedirectURIs:     []string{"https://example.com/callback"},
			},
			wantErr: true,
		},
		{
			name: "missing invalidation flow",
			opts: OAuth2ProviderOptions{
				AuthorizationFlow: "auth-flow",
				RedirectURIs:      []string{"https://example.com/callback"},
			},
			wantErr: true,
		},
		{
			name: "missing redirect URIs",
			opts: OAuth2ProviderOptions{
				AuthorizationFlow: "auth-flow",
				InvalidationFlow:  "inval-flow",
			},
			wantErr: true,
		},
		{
			name: "empty redirect URI",
			opts: OAuth2ProviderOptions{
				AuthorizationFlow: "auth-flow",
				InvalidationFlow:  "inval-flow",
				RedirectURIs:      []string{""},
			},
			wantErr: true,
		},
		{
			name: "custom URI schemes for native apps",
			opts: OAuth2ProviderOptions{
				AuthorizationFlow: "auth-flow",
				InvalidationFlow:  "inval-flow",
				RedirectURIs: []string{
					"app.immich:///oauth-callback",
					"app.immich:/",
					"argocd://auth/callback",
					"com.example.app:/oauth2redirect",
				},
			},
			wantErr: false,
		},
		{
			name: "redirect URI missing scheme",
			opts: OAuth2ProviderOptions{
				AuthorizationFlow: "auth-flow",
				InvalidationFlow:  "inval-flow",
				RedirectURIs:      []string{"example.com/callback"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildRedirectURIs(t *testing.T) {
	uris := buildRedirectURIs([]string{"https://a.com", "https://b.com"})
	if len(uris) != 2 {
		t.Fatalf("expected 2 URIs, got %d", len(uris))
	}
	if uris[0].Url != "https://a.com" {
		t.Errorf("expected first URI %q, got %q", "https://a.com", uris[0].Url)
	}
	if uris[0].MatchingMode != api.MATCHINGMODEENUM_STRICT {
		t.Errorf("expected STRICT matching mode")
	}
}

func TestBuildRedirectURIs_Empty(t *testing.T) {
	uris := buildRedirectURIs([]string{})
	if len(uris) != 0 {
		t.Errorf("expected 0 URIs, got %d", len(uris))
	}
}
