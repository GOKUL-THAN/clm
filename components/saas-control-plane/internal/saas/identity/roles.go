package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GetRealmRoles lists all realm roles, excluding Keycloak's internal defaults.
func (c *Client) GetRealmRoles(ctx context.Context) ([]Role, error) {
	raw, err := c.do(ctx, http.MethodGet, "/roles", nil)
	if err != nil {
		return nil, err
	}
	var all []Role
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("decode roles: %w", err)
	}
	roles := make([]Role, 0, len(all))
	for _, r := range all {
		// default-roles-clm etc. are Keycloak plumbing, not CLM RBAC roles.
		if strings.HasPrefix(r.Name, "default-roles-") {
			continue
		}
		if r.Name == "offline_access" || r.Name == "uma_authorization" {
			continue
		}
		roles = append(roles, r)
	}
	return roles, nil
}

// GetUserRoles returns the realm roles directly assigned to a user.
func (c *Client) GetUserRoles(ctx context.Context, userID string) ([]Role, error) {
	raw, err := c.do(ctx, http.MethodGet,
		"/users/"+url.PathEscape(userID)+"/role-mappings/realm", nil)
	if err != nil {
		return nil, err
	}
	var roles []Role
	if err := json.Unmarshal(raw, &roles); err != nil {
		return nil, fmt.Errorf("decode user roles: %w", err)
	}
	return roles, nil
}

// getRoleByName resolves a realm role, which is needed because assignment
// requires both the role id and name.
func (c *Client) getRoleByName(ctx context.Context, name string) (*Role, error) {
	raw, err := c.do(ctx, http.MethodGet, "/roles/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}
	var r Role
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("decode role %q: %w", name, err)
	}
	return &r, nil
}

// AssignRealmRole grants a realm role to a user.
func (c *Client) AssignRealmRole(ctx context.Context, userID, roleName string) error {
	role, err := c.getRoleByName(ctx, roleName)
	if err != nil {
		return err
	}
	body := []Role{{ID: role.ID, Name: role.Name}}
	_, err = c.do(ctx, http.MethodPost,
		"/users/"+url.PathEscape(userID)+"/role-mappings/realm", body)
	return err
}

// RemoveRealmRole revokes a realm role from a user.
func (c *Client) RemoveRealmRole(ctx context.Context, userID, roleName string) error {
	role, err := c.getRoleByName(ctx, roleName)
	if err != nil {
		return err
	}
	body := []Role{{ID: role.ID, Name: role.Name}}
	_, err = c.do(ctx, http.MethodDelete,
		"/users/"+url.PathEscape(userID)+"/role-mappings/realm", body)
	return err
}

// CreateRealmRole adds a new realm role.
func (c *Client) CreateRealmRole(ctx context.Context, name, description string) error {
	_, err := c.do(ctx, http.MethodPost, "/roles",
		Role{Name: name, Description: description})
	return err
}

// DeleteRealmRole removes a realm role.
func (c *Client) DeleteRealmRole(ctx context.Context, name string) error {
	_, err := c.do(ctx, http.MethodDelete, "/roles/"+url.PathEscape(name), nil)
	return err
}

// SetUserRole makes roleName the user's only CLM realm role. CLM RBAC treats a
// user as having one effective role, so existing role mappings are cleared
// first to avoid a user silently keeping a higher privilege.
func (c *Client) SetUserRole(ctx context.Context, userID, roleName string) error {
	current, err := c.GetUserRoles(ctx, userID)
	if err != nil {
		return err
	}

	stale := make([]Role, 0, len(current))
	for _, r := range current {
		if strings.HasPrefix(r.Name, "default-roles-") ||
			r.Name == "offline_access" || r.Name == "uma_authorization" {
			continue // leave Keycloak plumbing intact
		}
		if r.Name == roleName {
			return nil // already the only role we care about
		}
		stale = append(stale, Role{ID: r.ID, Name: r.Name})
	}

	if len(stale) > 0 {
		if _, err := c.do(ctx, http.MethodDelete,
			"/users/"+url.PathEscape(userID)+"/role-mappings/realm", stale); err != nil {
			return fmt.Errorf("clear existing roles: %w", err)
		}
	}
	return c.AssignRealmRole(ctx, userID, roleName)
}
