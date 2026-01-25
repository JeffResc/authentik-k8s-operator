package authentik

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	api "goauthentik.io/api/v3"
)

// extractAPIError extracts detailed error information from Authentik API errors
func extractAPIError(err error, operation string) error {
	var apiErr api.GenericOpenAPIError
	if errors.As(err, &apiErr) {
		body := string(apiErr.Body())
		if body != "" {
			return fmt.Errorf("%s: %s - %s", operation, apiErr.Error(), body)
		}
		return fmt.Errorf("%s: %s", operation, apiErr.Error())
	}
	return fmt.Errorf("%s: %w", operation, err)
}

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

// GetAuthorizationURL returns the OIDC authorization endpoint for a specific application
func (c *Client) GetAuthorizationURL(slug string) string {
	return fmt.Sprintf("%s/application/o/%s/authorize/", c.baseURL, slug)
}

// GetTokenURL returns the OIDC token endpoint for a specific application
func (c *Client) GetTokenURL(slug string) string {
	return fmt.Sprintf("%s/application/o/%s/token/", c.baseURL, slug)
}

// GetUserInfoURL returns the OIDC userinfo endpoint for a specific application
func (c *Client) GetUserInfoURL(slug string) string {
	return fmt.Sprintf("%s/application/o/%s/userinfo/", c.baseURL, slug)
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
		return extractAPIError(err, "health check failed")
	}
	if resp == nil {
		return fmt.Errorf("health check failed: nil response")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}
