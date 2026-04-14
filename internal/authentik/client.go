package authentik

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	api "goauthentik.io/api/v3"
)

// extractAPIError extracts detailed error information from Authentik API errors
func extractAPIError(err error, operation string) error {
	// Try to extract GenericOpenAPIError which contains the response body
	var apiErrPtr *api.GenericOpenAPIError
	if errors.As(err, &apiErrPtr) && apiErrPtr != nil {
		// Try body first
		body := string(apiErrPtr.Body())
		if body != "" {
			return fmt.Errorf("%s: %s - %s", operation, apiErrPtr.Error(), body)
		}
		// Try model (SDK decodes ValidationError for 400s)
		if model := apiErrPtr.Model(); model != nil {
			return fmt.Errorf("%s: %s - %+v", operation, apiErrPtr.Error(), model)
		}
		return fmt.Errorf("%s: %s", operation, apiErrPtr.Error())
	}

	// Check if error implements the Body() method directly (interface check)
	type bodyError interface {
		Body() []byte
		Error() string
	}
	if be, ok := err.(bodyError); ok {
		body := string(be.Body())
		if body != "" {
			return fmt.Errorf("%s: %s - %s", operation, be.Error(), body)
		}
	}

	return fmt.Errorf("%s: %w", operation, err)
}

// Client defines the interface for interacting with the Authentik API.
// This enables mock implementations for testing.
type Client interface {
	HealthCheck(ctx context.Context) error
	GetOAuth2ProviderByName(ctx context.Context, name string) (*ProviderInfo, error)
	GetOAuth2ProviderByID(ctx context.Context, id int32) (*ProviderInfo, error)
	CreateOAuth2Provider(ctx context.Context, name string, opts *OAuth2ProviderOptions) (*ProviderInfo, error)
	UpdateOAuth2Provider(ctx context.Context, id int32, name string, opts *OAuth2ProviderOptions) (*ProviderInfo, error)
	DeleteOAuth2Provider(ctx context.Context, id int32) error
	GetOAuth2ProviderURLs(ctx context.Context, providerID int32) (*ProviderURLs, error)
	GetApplicationBySlug(ctx context.Context, slug string) (*ApplicationInfo, error)
	CreateApplication(ctx context.Context, slug, name string, providerID int32, opts *ApplicationOptions) (*ApplicationInfo, error)
	UpdateApplication(ctx context.Context, slug, name string, providerID int32, opts *ApplicationOptions) (*ApplicationInfo, error)
	DeleteApplication(ctx context.Context, slug string) error
}

// Verify APIClient implements Client at compile time.
var _ Client = (*APIClient)(nil)

// APIClient wraps the Authentik API client
type APIClient struct {
	api     *api.APIClient
	baseURL string
}

// NewClient creates a new Authentik API client with automatic retry on transient errors.
func NewClient(baseURL, token string) (*APIClient, error) {
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

	// Configure retryable HTTP client for transient error resilience,
	// wrapping the instrumented transport for Prometheus metrics.
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Logger = slog.Default()
	retryClient.HTTPClient.Transport = &instrumentedTransport{
		next: newRateLimitedTransport(http.DefaultTransport, 10, 20),
	}
	cfg.HTTPClient = retryClient.StandardClient()

	client := api.NewAPIClient(cfg)

	return &APIClient{
		api:     client,
		baseURL: baseURL,
	}, nil
}

// GetBaseURL returns the base URL of the Authentik instance
func (c *APIClient) GetBaseURL() string {
	return c.baseURL
}

// CoreAPI returns the Core API client
func (c *APIClient) CoreAPI() *api.CoreApiService {
	return c.api.CoreApi
}

// ProvidersAPI returns the Providers API client
func (c *APIClient) ProvidersAPI() *api.ProvidersApiService {
	return c.api.ProvidersApi
}

// FlowsAPI returns the Flows API client
func (c *APIClient) FlowsAPI() *api.FlowsApiService {
	return c.api.FlowsApi
}

// HealthCheck performs a basic health check against the Authentik API
func (c *APIClient) HealthCheck(ctx context.Context) error {
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

// GetCertificateByName looks up a certificate/keypair by name and returns its UUID
func (c *APIClient) GetCertificateByName(ctx context.Context, name string) (string, error) {
	certs, resp, err := c.api.CryptoApi.CryptoCertificatekeypairsList(ctx).Name(name).PageSize(1).Execute()
	if err != nil {
		return "", extractAPIError(err, "failed to list certificates")
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to list certificates: status %d", resp.StatusCode)
	}

	if len(certs.Results) == 0 {
		return "", fmt.Errorf("certificate %q not found", name)
	}

	// Return the first matching certificate's UUID
	return certs.Results[0].Pk, nil
}

// GetScopeMappingByName looks up a scope mapping by its scope name (e.g., "openid", "email", "profile")
func (c *APIClient) GetScopeMappingByName(ctx context.Context, scopeName string) (string, error) {
	mappings, resp, err := c.api.PropertymappingsApi.PropertymappingsProviderScopeList(ctx).ScopeName(scopeName).PageSize(1).Execute()
	if err != nil {
		return "", extractAPIError(err, "failed to list scope mappings")
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to list scope mappings: status %d", resp.StatusCode)
	}

	if len(mappings.Results) == 0 {
		return "", fmt.Errorf("scope mapping for %q not found", scopeName)
	}

	// Return the first matching scope mapping's UUID
	return mappings.Results[0].Pk, nil
}
