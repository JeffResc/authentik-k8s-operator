package authentik

import (
	"context"
	"fmt"
	"net/http"

	api "goauthentik.io/api/v3"
)

func isUUID(s string) bool {
	// Reuse the package-level uuidPattern from metrics.go, but require a full match.
	// The metrics pattern is unanchored; checking length + match gives an exact match.
	return len(s) == 36 && uuidPattern.MatchString(s)
}

// SAMLProviderInfo contains information about a SAML provider
type SAMLProviderInfo struct {
	ID   int32
	Name string
}

// SAMLProviderOptions contains settings for SAML provider creation/update
type SAMLProviderOptions struct {
	AuthorizationFlow  string
	InvalidationFlow   string
	ACSUrl             string
	Issuer             string
	Audience           string
	SPBinding          string
	SigningKeypair     string
	DigestAlgorithm    string
	SignatureAlgorithm string
	PropertyMappings   []string
}

// GetSAMLProviderByName retrieves a SAML provider by name
func (c *APIClient) GetSAMLProviderByName(ctx context.Context, name string) (*SAMLProviderInfo, error) {
	providers, resp, err := c.api.ProvidersApi.ProvidersSamlList(ctx).Name(name).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list SAML providers: %w", err)
	}

	if len(providers.Results) == 0 {
		return nil, nil
	}

	p := providers.Results[0]
	return &SAMLProviderInfo{ID: p.Pk, Name: p.Name}, nil
}

// GetSAMLProviderByID retrieves a SAML provider by ID
func (c *APIClient) GetSAMLProviderByID(ctx context.Context, id int32) (*SAMLProviderInfo, error) {
	provider, resp, err := c.api.ProvidersApi.ProvidersSamlRetrieve(ctx, id).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get SAML provider: %w", err)
	}

	return &SAMLProviderInfo{ID: provider.Pk, Name: provider.Name}, nil
}

// CreateSAMLProvider creates a new SAML provider
func (c *APIClient) CreateSAMLProvider(ctx context.Context, name string, opts *SAMLProviderOptions) (*SAMLProviderInfo, error) {
	req, err := c.buildSAMLProviderRequest(ctx, name, opts)
	if err != nil {
		return nil, err
	}

	provider, _, err := c.api.ProvidersApi.ProvidersSamlCreate(ctx).SAMLProviderRequest(*req).Execute()
	if err != nil {
		return nil, extractAPIError(err, "failed to create SAML provider")
	}

	return &SAMLProviderInfo{ID: provider.Pk, Name: provider.Name}, nil
}

// UpdateSAMLProvider updates an existing SAML provider
func (c *APIClient) UpdateSAMLProvider(ctx context.Context, id int32, name string, opts *SAMLProviderOptions) (*SAMLProviderInfo, error) {
	req, err := c.buildSAMLProviderRequest(ctx, name, opts)
	if err != nil {
		return nil, err
	}

	provider, _, err := c.api.ProvidersApi.ProvidersSamlUpdate(ctx, id).SAMLProviderRequest(*req).Execute()
	if err != nil {
		return nil, extractAPIError(err, "failed to update SAML provider")
	}

	return &SAMLProviderInfo{ID: provider.Pk, Name: provider.Name}, nil
}

// DeleteSAMLProvider deletes a SAML provider by ID
func (c *APIClient) DeleteSAMLProvider(ctx context.Context, id int32) error {
	_, err := c.api.ProvidersApi.ProvidersSamlDestroy(ctx, id).Execute()
	if err != nil {
		return extractAPIError(err, "failed to delete SAML provider")
	}
	return nil
}

// GetSAMLProviderMetadata retrieves the IdP metadata XML for a SAML provider
func (c *APIClient) GetSAMLProviderMetadata(ctx context.Context, providerID int32) (string, error) {
	metadata, resp, err := c.api.ProvidersApi.ProvidersSamlMetadataRetrieve(ctx, providerID).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("SAML provider %d not found", providerID)
		}
		return "", extractAPIError(err, "failed to get SAML metadata")
	}
	return metadata.GetMetadata(), nil
}

func (c *APIClient) buildSAMLProviderRequest(ctx context.Context, name string, opts *SAMLProviderOptions) (*api.SAMLProviderRequest, error) {
	if opts == nil {
		return nil, fmt.Errorf("SAML provider options are required")
	}
	if opts.AuthorizationFlow == "" {
		return nil, fmt.Errorf("authorizationFlow is required")
	}
	if opts.InvalidationFlow == "" {
		return nil, fmt.Errorf("invalidationFlow is required")
	}
	if opts.ACSUrl == "" {
		return nil, fmt.Errorf("acsUrl is required")
	}

	// Resolve flow UUIDs
	authFlow, resp, err := c.api.FlowsApi.FlowsInstancesRetrieve(ctx, opts.AuthorizationFlow).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("authorization flow %q not found", opts.AuthorizationFlow)
		}
		return nil, extractAPIError(err, "failed to get authorization flow")
	}

	invalidationFlow, resp, err := c.api.FlowsApi.FlowsInstancesRetrieve(ctx, opts.InvalidationFlow).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("invalidation flow %q not found", opts.InvalidationFlow)
		}
		return nil, extractAPIError(err, "failed to get invalidation flow")
	}

	req := api.NewSAMLProviderRequest(name, authFlow.Pk, invalidationFlow.Pk, opts.ACSUrl)

	if opts.Issuer != "" {
		req.SetIssuer(opts.Issuer)
	}
	if opts.Audience != "" {
		req.SetAudience(opts.Audience)
	}
	if opts.SPBinding != "" {
		spBinding, err := api.NewSAMLBindingsEnumFromValue(opts.SPBinding)
		if err != nil {
			return nil, fmt.Errorf("invalid spBinding %q: %w", opts.SPBinding, err)
		}
		req.SetSpBinding(*spBinding)
	}
	if opts.DigestAlgorithm != "" {
		da, err := api.NewDigestAlgorithmEnumFromValue(opts.DigestAlgorithm)
		if err != nil {
			return nil, fmt.Errorf("invalid digestAlgorithm %q: %w", opts.DigestAlgorithm, err)
		}
		req.SetDigestAlgorithm(*da)
	}
	if opts.SignatureAlgorithm != "" {
		sa, err := api.NewSignatureAlgorithmEnumFromValue(opts.SignatureAlgorithm)
		if err != nil {
			return nil, fmt.Errorf("invalid signatureAlgorithm %q: %w", opts.SignatureAlgorithm, err)
		}
		req.SetSignatureAlgorithm(*sa)
	}
	if opts.SigningKeypair != "" {
		kpUUID, err := c.GetCertificateByName(ctx, opts.SigningKeypair)
		if err != nil {
			return nil, fmt.Errorf("failed to look up signing keypair %q: %w", opts.SigningKeypair, err)
		}
		req.SetSigningKp(kpUUID)
	}
	if len(opts.PropertyMappings) > 0 {
		uuids := make([]string, 0, len(opts.PropertyMappings))
		for _, pm := range opts.PropertyMappings {
			if isUUID(pm) {
				uuids = append(uuids, pm)
			} else {
				uuid, err := c.GetSAMLPropertyMappingByName(ctx, pm)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve SAML property mapping %q: %w", pm, err)
				}
				uuids = append(uuids, uuid)
			}
		}
		req.SetPropertyMappings(uuids)
	}

	return req, nil
}
