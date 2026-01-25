package authentik

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	api "goauthentik.io/api/v3"
)

// Client wraps the Authentik API client
type Client struct {
	api     *api.APIClient
	baseURL string
}

// NewClient creates a new Authentik API client
func NewClient(baseURL, token string) (*Client, error) {
	// Ensure URL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	cfg := api.NewConfiguration()
	cfg.Host = strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	cfg.Scheme = "https"
	if strings.HasPrefix(baseURL, "http://") {
		cfg.Scheme = "http"
	}

	// Add bearer token authentication
	cfg.AddDefaultHeader("Authorization", fmt.Sprintf("Bearer %s", token))

	client := api.NewAPIClient(cfg)

	return &Client{
		api:     client,
		baseURL: baseURL,
	}, nil
}

// GetBaseURL returns the base URL of the Authentik instance
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// GetIssuerURL returns the OIDC issuer URL
func (c *Client) GetIssuerURL() string {
	return fmt.Sprintf("%s/application/o/", c.baseURL)
}

// GetAuthorizationURL returns the OIDC authorization endpoint
func (c *Client) GetAuthorizationURL(slug string) string {
	return fmt.Sprintf("%s/application/o/authorize/", c.baseURL)
}

// GetTokenURL returns the OIDC token endpoint
func (c *Client) GetTokenURL(slug string) string {
	return fmt.Sprintf("%s/application/o/token/", c.baseURL)
}

// GetUserInfoURL returns the OIDC userinfo endpoint
func (c *Client) GetUserInfoURL(slug string) string {
	return fmt.Sprintf("%s/application/o/userinfo/", c.baseURL)
}

// GetProviderIssuerURL returns the provider-specific OIDC issuer URL
func (c *Client) GetProviderIssuerURL(slug string) string {
	return fmt.Sprintf("%s/application/o/%s/", c.baseURL, slug)
}

// CoreAPI returns the Core API client
func (c *Client) CoreAPI() *api.CoreApiService {
	return c.api.CoreApi
}

// ProvidersAPI returns the Providers API client
func (c *Client) ProvidersAPI() *api.ProvidersApiService {
	return c.api.ProvidersApi
}

// FlowsAPI returns the Flows API client
func (c *Client) FlowsAPI() *api.FlowsApiService {
	return c.api.FlowsApi
}

// HealthCheck performs a basic health check against the Authentik API
func (c *Client) HealthCheck(ctx context.Context) error {
	_, resp, err := c.api.CoreApi.CoreBrandsCurrentRetrieve(ctx).Execute()
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}
