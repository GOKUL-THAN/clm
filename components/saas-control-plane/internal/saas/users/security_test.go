package users

import (
	"errors"
	"testing"

	"github.com/m2p/clm/pkg/models"
)

func TestCanAssignRoleEnforcesPrivilegeCeiling(t *testing.T) {
	tests := []struct {
		name      string
		actor     string
		requested models.UserRole
		wantErr   error
	}{
		{name: "tenant admin can assign tenant admin", actor: "tenant_admin", requested: models.RoleTenantAdmin},
		{name: "tenant admin cannot assign platform admin", actor: "tenant_admin", requested: models.RoleAdmin, wantErr: ErrRoleAssignmentDenied},
		{name: "tenant admin cannot assign super admin", actor: "tenant_admin", requested: models.RoleSuperAdmin, wantErr: ErrRoleAssignmentDenied},
		{name: "platform admin can assign tenant role", actor: "admin", requested: models.RoleOperator},
		{name: "platform admin cannot assign super admin", actor: "admin", requested: models.RoleSuperAdmin, wantErr: ErrRoleAssignmentDenied},
		{name: "super admin can assign platform admin", actor: "super_admin", requested: models.RoleAdmin},
		{name: "unknown role is rejected", actor: "super_admin", requested: models.UserRole("root"), wantErr: ErrInvalidRole},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanAssignRole(tt.actor, tt.requested)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("CanAssignRole() error = %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("CanAssignRole() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
