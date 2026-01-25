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
