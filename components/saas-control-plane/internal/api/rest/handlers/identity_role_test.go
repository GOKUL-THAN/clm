package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/m2p/clm/internal/saas/identity"
)

func TestIdentityRoleHandlersRejectAdminSuperAdminAssignment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		method func(*IdentityHandler, *gin.Context)
		path   string
		body   string
		params gin.Params
	}{
		{
			name:   "create user",
			method: (*IdentityHandler).CreateUser,
			path:   "/identity/users",
			body:   `{"username":"target","email":"target@example.com","first_name":"Target","last_name":"User","password":"password123","role":"super_admin"}`,
		},
		{
			name:   "assign user role",
			method: (*IdentityHandler).SetUserRole,
			path:   "/identity/users/target/role",
			body:   `{"role":"super_admin"}`,
			params: gin.Params{{Key: "id", Value: "target"}},
		},
		{
			name:   "create group with role",
			method: (*IdentityHandler).CreateGroup,
			path:   "/identity/groups",
			body:   `{"name":"target-group","role":"super_admin"}`,
		},
		{
			name:   "assign group role",
			method: (*IdentityHandler).SetGroupRole,
			path:   "/identity/groups/target/role",
			body:   `{"role":"super_admin"}`,
			params: gin.Params{{Key: "gid", Value: "target"}},
		},
	}

	handler := &IdentityHandler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			ctx.Params = tt.params
			ctx.Set("role", "admin")

			tt.method(handler, ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
		})
	}
}

func TestIdentityRoleAssignmentAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		actorRole    string
		requested    string
		wantAllowed  bool
		wantHTTPCode int
	}{
		{name: "admin may assign viewer", actorRole: "admin", requested: "viewer", wantAllowed: true, wantHTTPCode: http.StatusOK},
		{name: "admin cannot assign super admin", actorRole: "admin", requested: "super_admin", wantHTTPCode: http.StatusForbidden},
		{name: "super admin may assign super admin", actorRole: "super_admin", requested: "super_admin", wantAllowed: true, wantHTTPCode: http.StatusOK},
		{name: "unknown role is rejected", actorRole: "super_admin", requested: "root", wantHTTPCode: http.StatusBadRequest},
		{name: "missing role context is rejected", requested: "viewer", wantHTTPCode: http.StatusForbidden},
	}

	handler := &IdentityHandler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.actorRole != "" {
				ctx.Set("role", tt.actorRole)
			}

			allowed := handler.authorizeRoleAssignment(ctx, tt.requested)
			if allowed != tt.wantAllowed {
				t.Fatalf("allowed = %t, want %t", allowed, tt.wantAllowed)
			}
			if recorder.Code != tt.wantHTTPCode {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantHTTPCode, recorder.Body.String())
			}
		})
	}
}

func TestIdentityGroupMembershipEnforcesInheritedRoleCeiling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groups := []identity.Group{{
		ID:         "parent",
		RealmRoles: []string{"super_admin"},
		SubGroups:  []identity.Group{{ID: "child", Name: "nested"}},
	}}

	tests := []struct {
		name         string
		actorRole    string
		groupID      string
		wantAllowed  bool
		wantHTTPCode int
	}{
		{name: "admin cannot join inherited super admin group", actorRole: "admin", groupID: "child", wantHTTPCode: http.StatusForbidden},
		{name: "super admin may join inherited super admin group", actorRole: "super_admin", groupID: "child", wantAllowed: true, wantHTTPCode: http.StatusOK},
		{name: "unknown group is rejected", actorRole: "admin", groupID: "missing", wantHTTPCode: http.StatusNotFound},
	}

	handler := &IdentityHandler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/", nil)
			ctx.Set("role", tt.actorRole)

			allowed := handler.authorizeGroupMembership(ctx, groups, tt.groupID)
			if allowed != tt.wantAllowed {
				t.Fatalf("allowed = %t, want %t", allowed, tt.wantAllowed)
			}
			if recorder.Code != tt.wantHTTPCode {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantHTTPCode, recorder.Body.String())
			}
		})
	}
}
