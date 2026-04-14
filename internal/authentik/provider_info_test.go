package authentik

import (
	"testing"

	api "goauthentik.io/api/v3"
)

func TestProviderInfoFromResponse(t *testing.T) {
	provider := &api.OAuth2Provider{
		Pk:   42,
		Name: "test-provider",
	}
	provider.SetClientId("cid-123")
	provider.SetClientSecret("csec-456")

	info := providerInfoFromResponse(provider)

	if info.ID != 42 {
		t.Errorf("expected ID 42, got %d", info.ID)
	}
	if info.Name != "test-provider" {
		t.Errorf("expected Name %q, got %q", "test-provider", info.Name)
	}
	if info.ClientID != "cid-123" {
		t.Errorf("expected ClientID %q, got %q", "cid-123", info.ClientID)
	}
	if info.ClientSecret != "csec-456" {
		t.Errorf("expected ClientSecret %q, got %q", "csec-456", info.ClientSecret)
	}
}

func TestProviderInfoFromResponse_EmptyOptionalFields(t *testing.T) {
	provider := &api.OAuth2Provider{
		Pk:   1,
		Name: "minimal",
	}

	info := providerInfoFromResponse(provider)

	if info.ID != 1 {
		t.Errorf("expected ID 1, got %d", info.ID)
	}
	if info.ClientID != "" {
		t.Errorf("expected empty ClientID, got %q", info.ClientID)
	}
	if info.ClientSecret != "" {
		t.Errorf("expected empty ClientSecret, got %q", info.ClientSecret)
	}
}

func TestCoreAPI_ReturnsNonNil(t *testing.T) {
	c, err := NewClient("https://auth.example.com", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.CoreAPI() == nil {
		t.Error("expected non-nil CoreAPI")
	}
}

func TestProvidersAPI_ReturnsNonNil(t *testing.T) {
	c, err := NewClient("https://auth.example.com", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ProvidersAPI() == nil {
		t.Error("expected non-nil ProvidersAPI")
	}
}

func TestFlowsAPI_ReturnsNonNil(t *testing.T) {
	c, err := NewClient("https://auth.example.com", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.FlowsAPI() == nil {
		t.Error("expected non-nil FlowsAPI")
	}
}
