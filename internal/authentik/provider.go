package authentik

import (
	"context"
	"fmt"
	"net/http"

	api "goauthentik.io/api/v3"
)

// ProviderInfo contains information about an OAuth2 provider
type ProviderInfo struct {
	ID           int32
	Name         string
	ClientID     string
	ClientSecret string
}

// OAuth2ProviderOptions contains settings for OAuth2 provider creation/update
type OAuth2ProviderOptions struct {
	AuthorizationFlow    string
	RedirectURIs         []string
	Scopes               []string
	ClientType           string
	AccessCodeValidity   string
	AccessTokenValidity  string
	RefreshTokenValidity string
	SubMode              string
	IncludeClaimsInToken *bool
	IssuerMode           string
	PropertyMappings     []string
}

// GetOAuth2ProviderByName retrieves an OAuth2 provider by name
func (c *Client) GetOAuth2ProviderByName(ctx context.Context, name string) (*ProviderInfo, error) {
	// List providers and filter by name
	providers, resp, err := c.api.ProvidersApi.ProvidersOauth2List(ctx).Name(name).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	if len(providers.Results) == 0 {
		return nil, nil
	}

	provider := providers.Results[0]
	return &ProviderInfo{
		ID:           provider.Pk,
		Name:         provider.Name,
		ClientID:     provider.GetClientId(),
		ClientSecret: provider.GetClientSecret(),
	}, nil
}

// GetOAuth2ProviderByID retrieves an OAuth2 provider by ID
func (c *Client) GetOAuth2ProviderByID(ctx context.Context, id int32) (*ProviderInfo, error) {
	provider, resp, err := c.api.ProvidersApi.ProvidersOauth2Retrieve(ctx, id).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	return &ProviderInfo{
		ID:           provider.Pk,
		Name:         provider.Name,
		ClientID:     provider.GetClientId(),
		ClientSecret: provider.GetClientSecret(),
	}, nil
}

// buildRedirectURIs converts string slice to RedirectURIRequest slice
func buildRedirectURIs(uris []string) []api.RedirectURIRequest {
	result := make([]api.RedirectURIRequest, len(uris))
	for i, uri := range uris {
		result[i] = api.RedirectURIRequest{
			Url: uri,
		}
	}
	return result
}

// CreateOAuth2Provider creates a new OAuth2 provider
func (c *Client) CreateOAuth2Provider(ctx context.Context, name string, opts *OAuth2ProviderOptions) (*ProviderInfo, error) {
	if opts == nil {
		return nil, fmt.Errorf("provider options are required")
	}

	// Get the authorization flow UUID
	flowSlug := opts.AuthorizationFlow
	flow, resp, err := c.api.FlowsApi.FlowsInstancesRetrieve(ctx, flowSlug).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("authorization flow %q not found", flowSlug)
		}
		return nil, fmt.Errorf("failed to get authorization flow: %w", err)
	}

	// Build redirect URIs
	redirectURIs := buildRedirectURIs(opts.RedirectURIs)

	// Create the request - new API requires (name, authorizationFlow, clientType, redirectUris)
	clientType := "confidential"
	if opts.ClientType != "" {
		clientType = opts.ClientType
	}

	req := api.NewOAuth2ProviderRequest(name, flow.Pk, clientType, redirectURIs)

	// Set token validity
	if opts.AccessCodeValidity != "" {
		req.SetAccessCodeValidity(opts.AccessCodeValidity)
	}
	if opts.AccessTokenValidity != "" {
		req.SetAccessTokenValidity(opts.AccessTokenValidity)
	}
	if opts.RefreshTokenValidity != "" {
		req.SetRefreshTokenValidity(opts.RefreshTokenValidity)
	}

	// Set sub mode
	if opts.SubMode != "" {
		subMode, err := api.NewSubModeEnumFromValue(opts.SubMode)
		if err == nil && subMode != nil {
			req.SetSubMode(*subMode)
		}
	}

	// Set claims in token
	if opts.IncludeClaimsInToken != nil {
		req.SetIncludeClaimsInIdToken(*opts.IncludeClaimsInToken)
	}

	// Set issuer mode
	if opts.IssuerMode != "" {
		issuerMode, err := api.NewIssuerModeEnumFromValue(opts.IssuerMode)
		if err == nil && issuerMode != nil {
			req.SetIssuerMode(*issuerMode)
		}
	}

	// Set property mappings if specified
	if len(opts.PropertyMappings) > 0 {
		req.SetPropertyMappings(opts.PropertyMappings)
	}

	provider, _, err := c.api.ProvidersApi.ProvidersOauth2Create(ctx).OAuth2ProviderRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	return &ProviderInfo{
		ID:           provider.Pk,
		Name:         provider.Name,
		ClientID:     provider.GetClientId(),
		ClientSecret: provider.GetClientSecret(),
	}, nil
}

// UpdateOAuth2Provider updates an existing OAuth2 provider
func (c *Client) UpdateOAuth2Provider(ctx context.Context, id int32, name string, opts *OAuth2ProviderOptions) (*ProviderInfo, error) {
	if opts == nil {
		return nil, fmt.Errorf("provider options are required")
	}

	// Get the authorization flow UUID
	flowSlug := opts.AuthorizationFlow
	flow, resp, err := c.api.FlowsApi.FlowsInstancesRetrieve(ctx, flowSlug).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("authorization flow %q not found", flowSlug)
		}
		return nil, fmt.Errorf("failed to get authorization flow: %w", err)
	}

	// Build redirect URIs
	redirectURIs := buildRedirectURIs(opts.RedirectURIs)

	// Create the request
	clientType := "confidential"
	if opts.ClientType != "" {
		clientType = opts.ClientType
	}

	req := api.NewOAuth2ProviderRequest(name, flow.Pk, clientType, redirectURIs)

	// Set token validity
	if opts.AccessCodeValidity != "" {
		req.SetAccessCodeValidity(opts.AccessCodeValidity)
	}
	if opts.AccessTokenValidity != "" {
		req.SetAccessTokenValidity(opts.AccessTokenValidity)
	}
	if opts.RefreshTokenValidity != "" {
		req.SetRefreshTokenValidity(opts.RefreshTokenValidity)
	}

	// Set sub mode
	if opts.SubMode != "" {
		subMode, err := api.NewSubModeEnumFromValue(opts.SubMode)
		if err == nil && subMode != nil {
			req.SetSubMode(*subMode)
		}
	}

	// Set claims in token
	if opts.IncludeClaimsInToken != nil {
		req.SetIncludeClaimsInIdToken(*opts.IncludeClaimsInToken)
	}

	// Set issuer mode
	if opts.IssuerMode != "" {
		issuerMode, err := api.NewIssuerModeEnumFromValue(opts.IssuerMode)
		if err == nil && issuerMode != nil {
			req.SetIssuerMode(*issuerMode)
		}
	}

	// Set property mappings if specified
	if len(opts.PropertyMappings) > 0 {
		req.SetPropertyMappings(opts.PropertyMappings)
	}

	provider, _, err := c.api.ProvidersApi.ProvidersOauth2Update(ctx, id).OAuth2ProviderRequest(*req).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	return &ProviderInfo{
		ID:           provider.Pk,
		Name:         provider.Name,
		ClientID:     provider.GetClientId(),
		ClientSecret: provider.GetClientSecret(),
	}, nil
}

// DeleteOAuth2Provider deletes an OAuth2 provider by ID
func (c *Client) DeleteOAuth2Provider(ctx context.Context, id int32) error {
	_, err := c.api.ProvidersApi.ProvidersOauth2Destroy(ctx, id).Execute()
	if err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	return nil
}
