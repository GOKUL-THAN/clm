package sso

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap/zaptest"
)

func TestGroupMapper(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mapper := NewGroupMapper(logger)
	tenantID := uuid.New()
	ctx := context.Background()

	// 1. Add priority mappings
	adminRule, err := mapper.AddMapping(ctx, tenantID, &GroupRoleMapping{
		GroupName:   "AWSReservedSSO_AdministratorAccess",
		TargetRole:  models.RoleSuperAdmin,
		Priority:    90,
		Description: "Cloud platform super admins",
	})
	if err != nil {
		t.Fatalf("failed to add admin rule: %v", err)
	}

	secRule, err := mapper.AddMapping(ctx, tenantID, &GroupRoleMapping{
		GroupName:   "M2P-SECURITY-OPERATIONS",
		TargetRole:  models.RoleOperator,
		Priority:    80,
		Description: "SecOps team",
	})
	if err != nil {
		t.Fatalf("failed to add sec rule: %v", err)
	}

	auditRule, err := mapper.AddMapping(ctx, tenantID, &GroupRoleMapping{
		GroupName:   "M2P-EXTERNAL-AUDITORS",
		TargetRole:  models.RoleAuditor,
		Priority:    50,
		Description: "Compliance auditors",
	})
	if err != nil {
		t.Fatalf("failed to add audit rule: %v", err)
	}

	// 2. Verify List
	list := mapper.ListMappings(tenantID)
	if len(list) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(list))
	}
	// Verify sorted by priority descending
	if list[0].Priority != 90 || list[1].Priority != 80 || list[2].Priority != 50 {
		t.Fatalf("mappings not sorted by priority: %v, %v, %v", list[0].Priority, list[1].Priority, list[2].Priority)
	}

	// 3. Resolve role: user has both SecOps and Admin groups; higher priority Admin wins
	userGroups := []string{"M2P-DEVELOPERS", "M2P-SECURITY-OPERATIONS", "AWSReservedSSO_AdministratorAccess"}
	resolvedRole, matchedRule := mapper.ResolveRole(tenantID, userGroups, models.RoleViewer)
	if resolvedRole != models.RoleSuperAdmin {
		t.Fatalf("expected role super_admin, got %s", resolvedRole)
	}
	if matchedRule == nil || matchedRule.ID != adminRule.ID {
		t.Fatalf("matched rule incorrect: %+v", matchedRule)
	}

	// 4. Resolve role: user has only Auditor group
	resolvedRole2, _ := mapper.ResolveRole(tenantID, []string{"M2P-EXTERNAL-AUDITORS"}, models.RoleViewer)
	if resolvedRole2 != models.RoleAuditor {
		t.Fatalf("expected role auditor, got %s", resolvedRole2)
	}

	// 5. Resolve role: user has no matching groups, falls back to default
	resolvedRole3, matchedRule3 := mapper.ResolveRole(tenantID, []string{"UNKNOWN-GROUP"}, models.RoleViewer)
	if resolvedRole3 != models.RoleViewer {
		t.Fatalf("expected fallback role viewer, got %s", resolvedRole3)
	}
	if matchedRule3 != nil {
		t.Fatalf("expected nil matched rule, got %+v", matchedRule3)
	}

	// 6. Delete mapping
	err = mapper.DeleteMapping(tenantID, secRule.ID)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if len(mapper.ListMappings(tenantID)) != 2 {
		t.Fatalf("expected 2 rules after delete")
	}
	_ = auditRule
}

