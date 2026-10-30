package identity

import (
	"context"
	"net/http"
	"sort"
	"strings"
)

// Service adds portal-oriented convenience on top of the raw Admin API client.
type Service struct {
	*Client
}

// NewService wraps a Keycloak client for use by the REST handlers.
func NewService(c *Client) *Service { return &Service{Client: c} }

// Ping reports whether the Keycloak Admin API is reachable and the configured
// admin credentials work.
func (s *Service) Ping(ctx context.Context) error {
	_, err := s.do(ctx, http.MethodGet, "", nil) // GET /admin/realms/{realm}
	return err
}

// ListUsersWithRoles returns users along with their realm roles resolved, which
// is what the portal's user table needs in a single call.
func (s *Service) ListUsersWithRoles(ctx context.Context, search string, first, max int) ([]User, error) {
	users, err := s.ListUsers(ctx, search, first, max)
	if err != nil {
		return nil, err
	}
	for i := range users {
		roles, err := s.GetUserRoles(ctx, users[i].ID)
		if err != nil {
			continue // never fail the whole listing over one user
		}
		names := make([]string, 0, len(roles))
		for _, r := range roles {
			if isInternalRole(r.Name) {
				continue
			}
			names = append(names, r.Name)
		}
		sort.Strings(names)
		users[i].Roles = names
	}
	return users, nil
}

// isInternalRole reports whether a role is Keycloak plumbing rather than a CLM
// RBAC role.
func isInternalRole(name string) bool {
	if strings.HasPrefix(name, "default-roles-") {
		return true
	}
	return name == "offline_access" || name == "uma_authorization"
}

// Stats builds the summary shown on the Identity Management header cards.
func (s *Service) Stats(ctx context.Context) *RealmStats {
	stats := &RealmStats{Realm: s.Realm()}

	users, err := s.ListUsers(ctx, "", 0, 500)
	if err != nil {
		return stats // Reachable stays false
	}
	stats.Reachable = true
	stats.Users = len(users)
	for _, u := range users {
		if u.Enabled {
			stats.EnabledUsers++
		}
	}
	if roles, err := s.GetRealmRoles(ctx); err == nil {
		stats.Roles = len(roles)
	}
	if idps, err := s.ListIdentityProviders(ctx); err == nil {
		stats.IdentityProviders = len(idps)
	}
	if sessions, err := s.ListAllSessions(ctx); err == nil {
		stats.ActiveSessions = len(sessions)
	}
	return stats
}

// CLMRoles returns a user's realm roles with Keycloak's internal roles
// (default-roles-*, offline_access, uma_authorization) filtered out, so callers
// see only meaningful CLM RBAC roles.
func (s *Service) CLMRoles(ctx context.Context, userID string) ([]string, error) {
	roles, err := s.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		if isInternalRole(r.Name) {
			continue
		}
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return names, nil
}
