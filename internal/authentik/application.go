// Package authentik provides a client for interacting with the Authentik API.
package authentik

import (
	"context"
	"fmt"
	"net/http"

	api "goauthentik.io/api/v3"
)

// ApplicationInfo contains information about an Authentik application
type ApplicationInfo struct {
	UID  string
	Slug string
	Name string
}

// GetApplicationBySlug retrieves an application by its slug
func (c *Client) GetApplicationBySlug(ctx context.Context, slug string) (*ApplicationInfo, error) {
	app, resp, err := c.api.CoreApi.CoreApplicationsRetrieve(ctx, slug).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil // Not found is not an error
		}
		return nil, fmt.Errorf("failed to get application: %w", err)
	}

	return &ApplicationInfo{
		UID:  app.Pk,
		Slug: app.Slug,
		Name: app.Name,
	}, nil
}

// CreateApplication creates a new application in Authentik
func (c *Client) CreateApplication(ctx context.Context, slug, name string, providerID int32, opts *ApplicationOptions) (*ApplicationInfo, error) {
	req := api.NewApplicationRequest(name, slug)
	req.SetProvider(providerID)

	if opts != nil {
		if opts.Group != "" {
			req.SetGroup(opts.Group)
		}
		if opts.PolicyEngineMode != "" {
			mode, err := api.NewPolicyEngineModeFromValue(opts.PolicyEngineMode)
			if err != nil {
				return nil, fmt.Errorf("invalid policyEngineMode %q: %w", opts.PolicyEngineMode, err)
			}
			if mode != nil {
				req.SetPolicyEngineMode(*mode)
			}
		}
		if opts.MetaLaunchURL != "" {
			req.SetMetaLaunchUrl(opts.MetaLaunchURL)
		}
		if opts.MetaDescription != "" {
			req.SetMetaDescription(opts.MetaDescription)
		}
		if opts.MetaPublisher != "" {
			req.SetMetaPublisher(opts.MetaPublisher)
		}
		if opts.OpenInNewTab != nil {
			req.SetOpenInNewTab(*opts.OpenInNewTab)
		}
	}

	app, _, err := c.api.CoreApi.CoreApplicationsCreate(ctx).ApplicationRequest(*req).Execute()
	if err != nil {
		return nil, extractAPIError(err, "failed to create application")
	}

	return &ApplicationInfo{
		UID:  app.Pk,
		Slug: app.Slug,
		Name: app.Name,
	}, nil
}

// ApplicationOptions contains optional settings for application creation/update
type ApplicationOptions struct {
	Group            string
	PolicyEngineMode string
	MetaLaunchURL    string
	MetaDescription  string
	MetaPublisher    string
	OpenInNewTab     *bool
}

// UpdateApplication updates an existing application
func (c *Client) UpdateApplication(ctx context.Context, slug, name string, providerID int32, opts *ApplicationOptions) (*ApplicationInfo, error) {
	req := api.NewApplicationRequest(name, slug)
	req.SetProvider(providerID)

	if opts != nil {
		if opts.Group != "" {
			req.SetGroup(opts.Group)
		}
		if opts.PolicyEngineMode != "" {
			mode, err := api.NewPolicyEngineModeFromValue(opts.PolicyEngineMode)
			if err != nil {
				return nil, fmt.Errorf("invalid policyEngineMode %q: %w", opts.PolicyEngineMode, err)
			}
			if mode != nil {
				req.SetPolicyEngineMode(*mode)
			}
		}
		if opts.MetaLaunchURL != "" {
			req.SetMetaLaunchUrl(opts.MetaLaunchURL)
		}
		if opts.MetaDescription != "" {
			req.SetMetaDescription(opts.MetaDescription)
		}
		if opts.MetaPublisher != "" {
			req.SetMetaPublisher(opts.MetaPublisher)
		}
		if opts.OpenInNewTab != nil {
			req.SetOpenInNewTab(*opts.OpenInNewTab)
		}
	}

	app, _, err := c.api.CoreApi.CoreApplicationsUpdate(ctx, slug).ApplicationRequest(*req).Execute()
	if err != nil {
		return nil, extractAPIError(err, "failed to update application")
	}

	return &ApplicationInfo{
		UID:  app.Pk,
		Slug: app.Slug,
		Name: app.Name,
	}, nil
}

// DeleteApplication deletes an application by slug
func (c *Client) DeleteApplication(ctx context.Context, slug string) error {
	_, err := c.api.CoreApi.CoreApplicationsDestroy(ctx, slug).Execute()
	if err != nil {
		return extractAPIError(err, "failed to delete application")
	}
	return nil
}
