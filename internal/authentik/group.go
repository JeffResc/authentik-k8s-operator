package authentik

import (
	"context"
	"fmt"
	"net/http"

	api "goauthentik.io/api/v3"
)

// GroupOwnerAttribute is the group attribute key that marks the AuthentikGroup
// CR that owns the group. Its value is "<namespace>/<name>".
const GroupOwnerAttribute = "goauthentik.io/owner"

// GroupInfo contains information about an Authentik group
type GroupInfo struct {
	UID  string
	Name string
	// Owner is the value of the GroupOwnerAttribute attribute, or "" if unset.
	Owner string
}

// GroupOptions contains settings for group creation/update.
// Parents and Roles accept names or UUIDs.
type GroupOptions struct {
	Name string
	// Owner is written to the GroupOwnerAttribute attribute when set.
	Owner       string
	IsSuperuser bool
	Parents     []string
	Roles       []string
	Attributes  map[string]interface{}
}

// resolvedGroupFields holds group fields with all references resolved to UUIDs.
// Slices and maps are always non-nil so the SDK sends them and empty values clear
// existing settings in Authentik.
type resolvedGroupFields struct {
	parents    []string
	roles      []string
	attributes map[string]interface{}
}

// GetGroupByUID retrieves a group by its UUID. Returns nil if not found.
func (c *APIClient) GetGroupByUID(ctx context.Context, uid string) (*GroupInfo, error) {
	group, resp, err := c.api.CoreAPI.CoreGroupsRetrieve(ctx, uid).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, extractAPIError(err, "failed to get group")
	}
	return newGroupInfo(group), nil
}

func newGroupInfo(g *api.Group) *GroupInfo {
	owner, _ := g.Attributes[GroupOwnerAttribute].(string)
	return &GroupInfo{UID: g.Pk, Name: g.Name, Owner: owner}
}

// GetGroupByName retrieves a group by its exact name. Returns nil if not found.
func (c *APIClient) GetGroupByName(ctx context.Context, name string) (*GroupInfo, error) {
	groups, _, err := c.api.CoreAPI.CoreGroupsList(ctx).Name(name).PageSize(1).Execute()
	if err != nil {
		return nil, extractAPIError(err, "failed to list groups")
	}
	if len(groups.Results) == 0 {
		return nil, nil
	}
	return newGroupInfo(&groups.Results[0]), nil
}

// CreateGroup creates a new group
func (c *APIClient) CreateGroup(ctx context.Context, opts *GroupOptions) (*GroupInfo, error) {
	fields, err := c.resolveGroupFields(ctx, opts)
	if err != nil {
		return nil, err
	}

	req := api.NewGroupRequest(opts.Name)
	req.SetIsSuperuser(opts.IsSuperuser)
	req.SetParents(fields.parents)
	req.SetRoles(fields.roles)
	req.SetAttributes(fields.attributes)

	group, _, err := c.api.CoreAPI.CoreGroupsCreate(ctx).GroupRequest(*req).Execute()
	if err != nil {
		return nil, extractAPIError(err, "failed to create group")
	}
	return &GroupInfo{UID: group.Pk, Name: group.Name}, nil
}

// UpdateGroup updates an existing group. It uses PATCH so group membership is left unchanged.
func (c *APIClient) UpdateGroup(ctx context.Context, uid string, opts *GroupOptions) (*GroupInfo, error) {
	fields, err := c.resolveGroupFields(ctx, opts)
	if err != nil {
		return nil, err
	}

	req := api.NewPatchedGroupRequest()
	req.SetName(opts.Name)
	req.SetIsSuperuser(opts.IsSuperuser)
	req.SetParents(fields.parents)
	req.SetRoles(fields.roles)
	req.SetAttributes(fields.attributes)

	group, _, err := c.api.CoreAPI.CoreGroupsPartialUpdate(ctx, uid).PatchedGroupRequest(*req).Execute()
	if err != nil {
		return nil, extractAPIError(err, "failed to update group")
	}
	return &GroupInfo{UID: group.Pk, Name: group.Name}, nil
}

// DeleteGroup deletes a group by UUID
func (c *APIClient) DeleteGroup(ctx context.Context, uid string) error {
	_, err := c.api.CoreAPI.CoreGroupsDestroy(ctx, uid).Execute()
	if err != nil {
		return extractAPIError(err, "failed to delete group")
	}
	return nil
}

func (c *APIClient) resolveGroupFields(ctx context.Context, opts *GroupOptions) (*resolvedGroupFields, error) {
	if opts == nil {
		return nil, fmt.Errorf("group options are required")
	}
	if opts.Name == "" {
		return nil, fmt.Errorf("group name is required")
	}

	fields := &resolvedGroupFields{
		parents:    make([]string, 0, len(opts.Parents)),
		roles:      make([]string, 0, len(opts.Roles)),
		attributes: make(map[string]interface{}, len(opts.Attributes)+1),
	}
	for k, v := range opts.Attributes {
		fields.attributes[k] = v
	}
	if opts.Owner != "" {
		fields.attributes[GroupOwnerAttribute] = opts.Owner
	}

	for _, p := range opts.Parents {
		if isUUID(p) {
			fields.parents = append(fields.parents, p)
			continue
		}
		parent, err := c.GetGroupByName(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve parent group %q: %w", p, err)
		}
		if parent == nil {
			return nil, fmt.Errorf("parent group %q not found", p)
		}
		fields.parents = append(fields.parents, parent.UID)
	}

	for _, r := range opts.Roles {
		if isUUID(r) {
			fields.roles = append(fields.roles, r)
			continue
		}
		uid, err := c.getRoleUUIDByName(ctx, r)
		if err != nil {
			return nil, err
		}
		fields.roles = append(fields.roles, uid)
	}

	return fields, nil
}

func (c *APIClient) getRoleUUIDByName(ctx context.Context, name string) (string, error) {
	roles, _, err := c.api.RbacAPI.RbacRolesList(ctx).Name(name).PageSize(1).Execute()
	if err != nil {
		return "", extractAPIError(err, fmt.Sprintf("failed to resolve role %q", name))
	}
	if len(roles.Results) == 0 {
		return "", fmt.Errorf("role %q not found", name)
	}
	return roles.Results[0].Pk, nil
}
