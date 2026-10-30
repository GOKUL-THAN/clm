package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ClientInfo is a realm client as shown in the portal.
type ClientInfo struct {
	ID       string `json:"id"`
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Public   bool   `json:"publicClient"`
}

// ListClients returns the realm's OIDC clients.
func (c *Client) ListClients(ctx context.Context) ([]ClientInfo, error) {
	raw, err := c.do(ctx, http.MethodGet, "/clients", nil)
	if err != nil {
		return nil, err
	}
	var clients []ClientInfo
	if err := json.Unmarshal(raw, &clients); err != nil {
		return nil, fmt.Errorf("decode clients: %w", err)
	}
	return clients, nil
}

// ListAllSessions walks every client in the realm and collects active user
// sessions. Keycloak has no realm-wide session endpoint, so this fans out.
func (c *Client) ListAllSessions(ctx context.Context) ([]Session, error) {
	clients, err := c.ListClients(ctx)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	all := make([]Session, 0)
	for _, cl := range clients {
		if !cl.Enabled {
			continue
		}
		sessions, err := c.clientSessions(ctx, cl.ID)
		if err != nil {
			// A client without the right scope just yields nothing useful.
			continue
		}
		for _, s := range sessions {
			if seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			all = append(all, s)
		}
	}
	return all, nil
}

// clientSessions lists active sessions for one client's internal id.
func (c *Client) clientSessions(ctx context.Context, clientUUID string) ([]Session, error) {
	raw, err := c.do(ctx, http.MethodGet,
		"/clients/"+url.PathEscape(clientUUID)+"/user-sessions?max=500", nil)
	if err != nil {
		return nil, err
	}
	var sessions []Session
	if err := json.Unmarshal(raw, &sessions); err != nil {
		return nil, fmt.Errorf("decode client sessions: %w", err)
	}
	return sessions, nil
}

// DeleteSession removes a single session by id.
func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := c.do(ctx, http.MethodDelete, "/sessions/"+url.PathEscape(sessionID), nil)
	return err
}

// ListGroups returns the realm's group tree.
func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	raw, err := c.do(ctx, http.MethodGet, "/groups?briefRepresentation=false", nil)
	if err != nil {
		return nil, err
	}
	var groups []Group
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, fmt.Errorf("decode groups: %w", err)
	}
	return groups, nil
}

// CreateGroup adds a top-level group.
func (c *Client) CreateGroup(ctx context.Context, name string) error {
	_, err := c.do(ctx, http.MethodPost, "/groups", Group{Name: name})
	return err
}

// DeleteGroup removes a group by id.
func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/groups/"+url.PathEscape(id), nil)
	return err
}

// AddUserToGroup puts a user into a group.
func (c *Client) AddUserToGroup(ctx context.Context, userID, groupID string) error {
	_, err := c.do(ctx, http.MethodPut,
		"/users/"+url.PathEscape(userID)+"/groups/"+url.PathEscape(groupID), nil)
	return err
}

// RemoveUserFromGroup takes a user out of a group.
func (c *Client) RemoveUserFromGroup(ctx context.Context, userID, groupID string) error {
	_, err := c.do(ctx, http.MethodDelete,
		"/users/"+url.PathEscape(userID)+"/groups/"+url.PathEscape(groupID), nil)
	return err
}

// GetUserGroups lists the groups a user belongs to.
func (c *Client) GetUserGroups(ctx context.Context, userID string) ([]Group, error) {
	raw, err := c.do(ctx, http.MethodGet, "/users/"+url.PathEscape(userID)+"/groups", nil)
	if err != nil {
		return nil, err
	}
	var groups []Group
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, fmt.Errorf("decode user groups: %w", err)
	}
	return groups, nil
}

// GetGroupMembers lists the users in a group.
func (c *Client) GetGroupMembers(ctx context.Context, groupID string) ([]User, error) {
	raw, err := c.do(ctx, http.MethodGet,
		"/groups/"+url.PathEscape(groupID)+"/members?briefRepresentation=false", nil)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(raw, &users); err != nil {
		return nil, fmt.Errorf("decode group members: %w", err)
	}
	return users, nil
}

// GetGroupRoles returns the realm roles mapped to a group. Members inherit these.
func (c *Client) GetGroupRoles(ctx context.Context, groupID string) ([]Role, error) {
	raw, err := c.do(ctx, http.MethodGet,
		"/groups/"+url.PathEscape(groupID)+"/role-mappings/realm", nil)
	if err != nil {
		return nil, err
	}
	var roles []Role
	if err := json.Unmarshal(raw, &roles); err != nil {
		return nil, fmt.Errorf("decode group roles: %w", err)
	}
	return roles, nil
}

// SetGroupRole makes roleName the group's only CLM realm role, mirroring the
// single-effective-role model used for users. Members inherit it.
func (c *Client) SetGroupRole(ctx context.Context, groupID, roleName string) error {
	current, err := c.GetGroupRoles(ctx, groupID)
	if err != nil {
		return err
	}

	stale := make([]Role, 0, len(current))
	for _, r := range current {
		if isInternalRole(r.Name) {
			continue
		}
		if r.Name == roleName {
			return nil // already mapped
		}
		stale = append(stale, Role{ID: r.ID, Name: r.Name})
	}

	path := "/groups/" + url.PathEscape(groupID) + "/role-mappings/realm"
	if len(stale) > 0 {
		if _, err := c.do(ctx, http.MethodDelete, path, stale); err != nil {
			return fmt.Errorf("clear existing group roles: %w", err)
		}
	}
	if roleName == "" {
		return nil // caller only wanted the mapping cleared
	}

	role, err := c.getRoleByName(ctx, roleName)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodPost, path, []Role{{ID: role.ID, Name: role.Name}})
	return err
}

// CreateGroupWithRole creates a group and optionally maps a realm role to it in
// one call, which is how the portal's Groups tab creates them.
func (c *Client) CreateGroupWithRole(ctx context.Context, name, roleName string) (string, error) {
	if err := c.CreateGroup(ctx, name); err != nil {
		return "", err
	}

	// Keycloak returns the new id in a Location header; resolving by name keeps
	// this working through the nginx proxy.
	groups, err := c.ListGroups(ctx)
	if err != nil {
		return "", err
	}
	var id string
	for _, g := range groups {
		if g.Name == name {
			id = g.ID
			break
		}
	}
	if id == "" {
		return "", fmt.Errorf("group %q not found after creation", name)
	}

	if roleName != "" {
		if err := c.SetGroupRole(ctx, id, roleName); err != nil {
			return id, fmt.Errorf("group created but role mapping failed: %w", err)
		}
	}
	return id, nil
}
