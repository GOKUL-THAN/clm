// Package sso provides enterprise single sign-on and identity provider federation.
//
// Patterned after corporate AWS IAM Identity Center and Keycloak federated directory models
// (M2P AWS-sso-access and aws-identity-center), this module maps enterprise directory groups
// (e.g. AWS Identity Center permission sets, Okta groups, Azure AD security groups)
// directly to CLM tenant roles with priority-based conflict resolution, JIT role synchronization,
// and audit logging.
package sso

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

var (
	ErrMappingNotFound    = errors.New("group mapping not found")
	ErrDuplicateGroupRule  = errors.New("a rule for this group already exists in the tenant")
	ErrInvalidRole         = errors.New("invalid target role specified")
	ErrInvalidGroupName    = errors.New("group name cannot be empty")
)

// GroupRoleMapping maps an enterprise directory / IdP group to an internal CLM role.
type GroupRoleMapping struct {
	ID          uuid.UUID       `json:"id"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	GroupName   string          `json:"group_name"`   // e.g. "M2P-PLATFORM-ADMINS", "AWSReservedSSO_AdministratorAccess"
	TargetRole  models.UserRole `json:"target_role"`  // super_admin, security_admin, operator, etc.
	Priority    int             `json:"priority"`     // Higher number takes precedence (1 - 100)
	Description string          `json:"description"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// GroupMapper manages corporate group-to-role mappings per tenant.
type GroupMapper struct {
	mu       sync.RWMutex
	mappings map[uuid.UUID][]*GroupRoleMapping // tenantID -> mappings
	logger   *zap.Logger
}

// NewGroupMapper creates a new directory group mapping controller.
func NewGroupMapper(logger *zap.Logger) *GroupMapper {
	return &GroupMapper{
		mappings: make(map[uuid.UUID][]*GroupRoleMapping),
		logger:   logger,
	}
}

// AddMapping registers or updates an IdP group-to-role rule.
func (m *GroupMapper) AddMapping(ctx context.Context, tenantID uuid.UUID, mapping *GroupRoleMapping) (*GroupRoleMapping, error) {
	if mapping.GroupName == "" {
		return nil, ErrInvalidGroupName
	}
	if !isValidRole(mapping.TargetRole) {
		return nil, ErrInvalidRole
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tenantRules := m.mappings[tenantID]
	for _, r := range tenantRules {
		if strings.EqualFold(r.GroupName, mapping.GroupName) && r.ID != mapping.ID {
			return nil, ErrDuplicateGroupRule
		}
	}

	now := time.Now()
	if mapping.ID == uuid.Nil {
		mapping.ID = uuid.New()
		mapping.CreatedAt = now
	}
	mapping.TenantID = tenantID
	mapping.UpdatedAt = now

	// Replace existing or append
	updated := false
	for i, r := range tenantRules {
		if r.ID == mapping.ID {
			tenantRules[i] = mapping
			updated = true
			break
		}
	}
	if !updated {
		tenantRules = append(tenantRules, mapping)
	}

	// Keep sorted by Priority descending
	sort.Slice(tenantRules, func(i, j int) bool {
		return tenantRules[i].Priority > tenantRules[j].Priority
	})
	m.mappings[tenantID] = tenantRules

	m.logger.Info("directory group mapping saved",
		zap.String("tenant_id", tenantID.String()),
		zap.String("group", mapping.GroupName),
		zap.String("role", string(mapping.TargetRole)),
		zap.Int("priority", mapping.Priority),
	)

	return mapping, nil
}

// ListMappings returns all configured group mappings for a tenant.
func (m *GroupMapper) ListMappings(tenantID uuid.UUID) []*GroupRoleMapping {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rules := m.mappings[tenantID]
	out := make([]*GroupRoleMapping, len(rules))
	copy(out, rules)
	return out
}

// DeleteMapping removes a group mapping rule by ID.
func (m *GroupMapper) DeleteMapping(tenantID uuid.UUID, mappingID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules := m.mappings[tenantID]
	found := false
	newRules := make([]*GroupRoleMapping, 0, len(rules))
	for _, r := range rules {
		if r.ID == mappingID {
			found = true
			continue
		}
		newRules = append(newRules, r)
	}

	if !found {
		return ErrMappingNotFound
	}
	m.mappings[tenantID] = newRules
	return nil
}

// ResolveRole evaluates an incoming user's IdP groups against tenant rules.
// Returns the resolved role (based on highest priority matching rule), or the defaultRole.
func (m *GroupMapper) ResolveRole(tenantID uuid.UUID, userGroups []string, defaultRole models.UserRole) (models.UserRole, *GroupRoleMapping) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rules := m.mappings[tenantID]
	if len(rules) == 0 || len(userGroups) == 0 {
		return defaultRole, nil
	}

	// Create set of normalized groups
	groupSet := make(map[string]struct{}, len(userGroups))
	for _, g := range userGroups {
		groupSet[strings.ToLower(strings.TrimSpace(g))] = struct{}{}
	}

	// Rules are sorted by Priority desc; first match wins
	for _, rule := range rules {
		normalizedRuleGroup := strings.ToLower(strings.TrimSpace(rule.GroupName))
		if _, ok := groupSet[normalizedRuleGroup]; ok {
			return rule.TargetRole, rule
		}
	}

	return defaultRole, nil
}

func isValidRole(r models.UserRole) bool {
	switch r {
	case models.RoleSuperAdmin, models.RoleAdmin, models.RoleOperator, models.RoleAuditor, models.RoleViewer:
		return true
	default:
		return false
	}
}

