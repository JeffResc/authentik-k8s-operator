package authentik

import (
	"context"
	"fmt"

	api "goauthentik.io/api/v3"
)

// bindingsPageSize is large enough to return every binding of one application in one page.
const bindingsPageSize = 1000

// SyncGroupBindings makes the group bindings of an application match groupNames.
// It creates missing bindings and deletes group bindings for groups not in the list.
// Policy and user bindings are not changed.
func (c *APIClient) SyncGroupBindings(ctx context.Context, appUID string, groupNames []string) error {
	wanted := make(map[string]int32, len(groupNames))
	for i, name := range groupNames {
		pk, err := c.getGroupPKByName(ctx, name)
		if err != nil {
			return err
		}
		wanted[pk] = int32(i)
	}

	bindings, _, err := c.api.PoliciesAPI.PoliciesBindingsList(ctx).Target(appUID).PageSize(bindingsPageSize).Execute()
	if err != nil {
		return extractAPIError(err, "failed to list policy bindings")
	}

	for _, b := range bindings.GetResults() {
		group := b.GetGroup()
		if group == "" {
			continue
		}
		if _, ok := wanted[group]; ok {
			delete(wanted, group)
			continue
		}
		if _, err := c.api.PoliciesAPI.PoliciesBindingsDestroy(ctx, b.GetPk()).Execute(); err != nil {
			return extractAPIError(err, "failed to delete policy binding")
		}
	}

	for groupPK, order := range wanted {
		req := api.NewPolicyBindingRequest(appUID, order)
		req.SetGroup(groupPK)
		if _, _, err := c.api.PoliciesAPI.PoliciesBindingsCreate(ctx).PolicyBindingRequest(*req).Execute(); err != nil {
			return extractAPIError(err, "failed to create policy binding")
		}
	}

	return nil
}

// getGroupPKByName returns the primary key of the group with exactly this name.
func (c *APIClient) getGroupPKByName(ctx context.Context, name string) (string, error) {
	groups, _, err := c.api.CoreAPI.CoreGroupsList(ctx).Name(name).IncludeUsers(false).Execute()
	if err != nil {
		return "", extractAPIError(err, "failed to list groups")
	}
	for _, g := range groups.GetResults() {
		if g.GetName() == name {
			return g.GetPk(), nil
		}
	}
	return "", fmt.Errorf("group %q not found in authentik", name)
}
