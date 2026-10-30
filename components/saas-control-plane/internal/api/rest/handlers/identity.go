package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/saas/identity"
	"go.uber.org/zap"
)

// IdentityHandler exposes Keycloak realm administration to the Admin Portal so
// the Keycloak Admin Console never has to be reached directly.
type IdentityHandler struct {
	svc    *identity.Service
	logger *zap.Logger
}

// NewIdentityHandler creates a new identity handler.
func NewIdentityHandler(svc *identity.Service, logger *zap.Logger) *IdentityHandler {
	return &IdentityHandler{svc: svc, logger: logger}
}

// fail maps a Keycloak error onto an HTTP response, preserving the upstream
// status when Keycloak supplied one.
func (h *IdentityHandler) fail(c *gin.Context, err error, fallback string) {
	if apiErr, ok := err.(*identity.APIError); ok {
		status := apiErr.StatusCode()
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		c.JSON(status, gin.H{"error": apiErr.Error()})
		return
	}
	h.logger.Error(fallback, zap.Error(err))
	c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
}

// Status reports realm health plus summary counters for the page header.
// GET /api/v1/identity/status
func (h *IdentityHandler) Status(c *gin.Context) {
	stats := h.svc.Stats(c.Request.Context())
	c.JSON(http.StatusOK, stats)
}

// ListUsers returns realm users with their roles resolved.
// GET /api/v1/identity/users?search=
func (h *IdentityHandler) ListUsers(c *gin.Context) {
	users, err := h.svc.ListUsersWithRoles(c.Request.Context(), c.Query("search"), 0, 200)
	if err != nil {
		h.fail(c, err, "list keycloak users")
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users, "total": len(users)})
}

// GetUser returns a single user including roles and sessions.
// GET /api/v1/identity/users/:id
func (h *IdentityHandler) GetUser(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	user, err := h.svc.GetUser(ctx, id)
	if err != nil {
		h.fail(c, err, "get keycloak user")
		return
	}
	if roles, err := h.svc.CLMRoles(ctx, id); err == nil {
		user.Roles = roles
	}
	sessions, _ := h.svc.GetUserSessions(ctx, id)

	c.JSON(http.StatusOK, gin.H{"user": user, "sessions": sessions})
}

// createIdentityUserRequest is the payload for creating a realm user.
type createIdentityUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email" binding:"required,email"`
	// First and last name are mandatory, not cosmetic: the clm realm's declarative
	// user profile marks firstName and lastName as required attributes, and the
	// VERIFY_PROFILE required action is enabled. A user missing either one is
	// refused at login with "Account is not fully set up", so accepting a blank
	// name would create an account that can never sign in. Validated in the
	// handler rather than by a binding tag so the reason can be explained.
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Password      string `json:"password" binding:"required,min=8"`
	TempPassword  bool   `json:"temp_password"`
	Role          string `json:"role"`
	Enabled       *bool  `json:"enabled"`
	EmailVerified bool   `json:"email_verified"`
}

// CreateUser provisions a realm user and optionally assigns a role.
// POST /api/v1/identity/users
func (h *IdentityHandler) CreateUser(c *gin.Context) {
	var req createIdentityUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// A temporary password adds an UPDATE_PASSWORD required action, and Keycloak
	// refuses a Direct Grant login while any required action is outstanding
	// ("Account is not fully set up"). Both portals authenticate exclusively with
	// Direct Grant, so honouring this would silently create an account that can
	// never sign in. Refuse it instead of producing a broken user.
	if req.TempPassword {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "temporary passwords are not supported: the portals use the " +
				"Direct Grant flow, which cannot complete a forced password change, " +
				"so the account would be unable to log in",
			"hint": "create the user with a permanent password and ask them to " +
				"change it from the portal, or clear required actions afterwards " +
				"via POST /api/v1/identity/users/:id/unlock",
		})
		return
	}

	// The realm requires firstName and lastName, and Keycloak turns an incomplete
	// profile into a VERIFY_PROFILE action at login time, refusing the Direct Grant
	// with "Account is not fully set up". Reject up front with the actual reason.
	if strings.TrimSpace(req.FirstName) == "" || strings.TrimSpace(req.LastName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "first_name and last_name are required: the identity realm marks " +
				"them as mandatory profile attributes, and a user missing either one is " +
				"refused at login with \"Account is not fully set up\"",
		})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	username := req.Username
	if username == "" {
		username = req.Email
	}

	// Stamp the explicitly authorized tenant onto every new identity. Platform
	// tokens may omit tenant_id, in which case MutationAudit has validated and
	// exposed X-CLM-Tenant-ID as audit_tenant_id before this handler runs.
	attrs := map[string][]string{}
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		if value, exists := c.Get("audit_tenant_id"); exists {
			tenantID, _ = value.(uuid.UUID)
		}
	}
	if tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a target tenant is required"})
		return
	}
	attrs["tenant_id"] = []string{tenantID.String()}

	ctx := c.Request.Context()
	id, err := h.svc.CreateUser(ctx, identity.User{
		Username:      username,
		Email:         req.Email,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Enabled:       enabled,
		EmailVerified: req.EmailVerified,
		Attributes:    attrs,
	}, req.Password, false)
	if err != nil {
		h.fail(c, err, "create keycloak user")
		return
	}

	if req.Role != "" {
		if err := h.svc.SetUserRole(ctx, id, req.Role); err != nil {
			// The user exists; surface the role problem without losing the id.
			c.JSON(http.StatusCreated, gin.H{
				"id":      id,
				"warning": "user created but role assignment failed: " + err.Error(),
			})
			return
		}
	}

	h.logger.Info("keycloak user created",
		zap.String("id", id), zap.String("email", req.Email), zap.String("role", req.Role))
	c.JSON(http.StatusCreated, gin.H{"id": id, "message": "user created"})
}

// updateIdentityUserRequest carries editable profile fields.
type updateIdentityUserRequest struct {
	Email         string `json:"email"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Enabled       *bool  `json:"enabled"`
	EmailVerified *bool  `json:"email_verified"`
}

// UpdateUser edits a user's profile.
// PUT /api/v1/identity/users/:id
func (h *IdentityHandler) UpdateUser(c *gin.Context) {
	var req updateIdentityUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	current, err := h.svc.GetUser(ctx, c.Param("id"))
	if err != nil {
		h.fail(c, err, "get keycloak user for update")
		return
	}

	// Keycloak's user PUT replaces the representation, so start from current.
	payload := identity.User{
		Username:      current.Username,
		Email:         current.Email,
		FirstName:     current.FirstName,
		LastName:      current.LastName,
		Enabled:       current.Enabled,
		EmailVerified: current.EmailVerified,
		Attributes:    current.Attributes,
	}
	if req.Email != "" {
		payload.Email = req.Email
	}
	if req.FirstName != "" {
		payload.FirstName = req.FirstName
	}
	if req.LastName != "" {
		payload.LastName = req.LastName
	}
	if req.Enabled != nil {
		payload.Enabled = *req.Enabled
	}
	if req.EmailVerified != nil {
		payload.EmailVerified = *req.EmailVerified
	}

	if err := h.svc.UpdateUser(ctx, c.Param("id"), payload); err != nil {
		h.fail(c, err, "update keycloak user")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

// DeleteUser removes a user from the realm.
// DELETE /api/v1/identity/users/:id
func (h *IdentityHandler) DeleteUser(c *gin.Context) {
	if err := h.svc.DeleteUser(c.Request.Context(), c.Param("id")); err != nil {
		h.fail(c, err, "delete keycloak user")
		return
	}
	h.logger.Info("keycloak user deleted", zap.String("id", c.Param("id")))
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

// SetEnabled enables or disables a user.
// POST /api/v1/identity/users/:id/enabled
func (h *IdentityHandler) SetEnabled(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetEnabled(c.Request.Context(), c.Param("id"), req.Enabled); err != nil {
		h.fail(c, err, "toggle keycloak user")
		return
	}
	state := "disabled"
	if req.Enabled {
		state = "enabled"
	}
	c.JSON(http.StatusOK, gin.H{"message": "user " + state})
}

// ResetPassword sets a new password for a user.
// POST /api/v1/identity/users/:id/password
func (h *IdentityHandler) ResetPassword(c *gin.Context) {
	var req struct {
		Password  string `json:"password" binding:"required,min=8"`
		Temporary bool   `json:"temporary"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Same constraint as user creation: a temporary password adds an
	// UPDATE_PASSWORD required action, which blocks Direct Grant logins entirely.
	if req.Temporary {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "temporary passwords are not supported: the forced password " +
				"change cannot be completed through the Direct Grant flow used by " +
				"the portals, and the account would be locked out",
			"hint": "set a permanent password instead",
		})
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), c.Param("id"), req.Password, false); err != nil {
		h.fail(c, err, "reset keycloak password")
		return
	}
	h.logger.Info("keycloak password reset", zap.String("id", c.Param("id")))
	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

// UnlockUser makes a locked-out account able to log in again.
// POST /api/v1/identity/users/:id/unlock
//
// Keycloak answers "Account is not fully set up" to a Direct Grant login both
// when a required action is stored on the user (UPDATE_PASSWORD, VERIFY_EMAIL)
// and when a required user-profile attribute is missing — the clm realm requires
// firstName and lastName, so an incomplete profile yields a VERIFY_PROFILE action
// that never shows up in requiredActions. This repairs both.
//
// Optional first_name/last_name override the names derived from the email.
func (h *IdentityHandler) UnlockUser(c *gin.Context) {
	var req struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	_ = c.ShouldBindJSON(&req) // body is optional

	id := c.Param("id")
	changes, err := h.svc.RepairLogin(c.Request.Context(), id, req.FirstName, req.LastName)
	if err != nil {
		h.fail(c, err, "repair keycloak login")
		return
	}
	h.logger.Info("keycloak account repaired",
		zap.String("id", id), zap.Strings("changes", changes))
	c.JSON(http.StatusOK, gin.H{
		"message": "the user can log in now",
		"changes": changes,
	})
}

// SetUserRole makes the supplied role the user's only CLM realm role.
// POST /api/v1/identity/users/:id/role
func (h *IdentityHandler) SetUserRole(c *gin.Context) {
	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetUserRole(c.Request.Context(), c.Param("id"), req.Role); err != nil {
		h.fail(c, err, "assign keycloak role")
		return
	}
	h.logger.Info("keycloak role assigned",
		zap.String("id", c.Param("id")), zap.String("role", req.Role))
	c.JSON(http.StatusOK, gin.H{"message": "role updated to " + req.Role})
}

// LogoutUser kills every session for a user.
// POST /api/v1/identity/users/:id/logout
func (h *IdentityHandler) LogoutUser(c *gin.Context) {
	if err := h.svc.LogoutUser(c.Request.Context(), c.Param("id")); err != nil {
		h.fail(c, err, "logout keycloak user")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "all sessions revoked"})
}

// ListRoles returns the realm's CLM roles.
// GET /api/v1/identity/roles
func (h *IdentityHandler) ListRoles(c *gin.Context) {
	roles, err := h.svc.GetRealmRoles(c.Request.Context())
	if err != nil {
		h.fail(c, err, "list keycloak roles")
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

// CreateRole adds a realm role.
// POST /api/v1/identity/roles
func (h *IdentityHandler) CreateRole(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.CreateRealmRole(c.Request.Context(), req.Name, req.Description); err != nil {
		h.fail(c, err, "create keycloak role")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "role created"})
}

// DeleteRole removes a realm role.
// DELETE /api/v1/identity/roles/:name
func (h *IdentityHandler) DeleteRole(c *gin.Context) {
	if err := h.svc.DeleteRealmRole(c.Request.Context(), c.Param("name")); err != nil {
		h.fail(c, err, "delete keycloak role")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role deleted"})
}

// ListProviders returns configured external identity providers.
// GET /api/v1/identity/providers
func (h *IdentityHandler) ListProviders(c *gin.Context) {
	idps, err := h.svc.ListIdentityProviders(c.Request.Context())
	if err != nil {
		h.fail(c, err, "list identity providers")
		return
	}
	// Advertise which social providers can be added with just id + secret.
	c.JSON(http.StatusOK, gin.H{
		"providers":         idps,
		"available":         identity.SocialProviders,
		"redirect_uri_hint": "/auth/realms/" + h.svc.Realm() + "/broker/{alias}/endpoint",
	})
}

// createIdentityProviderRequest supports both social and generic OIDC providers.
type createIdentityProviderRequest struct {
	Type         string `json:"type" binding:"required"` // google|microsoft|github|oidc|saml
	Alias        string `json:"alias"`
	DisplayName  string `json:"display_name"`
	ClientID     string `json:"client_id" binding:"required"`
	ClientSecret string `json:"client_secret" binding:"required"`
	DefaultScope string `json:"default_scope"`

	// Generic OIDC only
	AuthorizationURL string `json:"authorization_url"`
	TokenURL         string `json:"token_url"`
	UserInfoURL      string `json:"user_info_url"`
	Issuer           string `json:"issuer"`
}

// CreateProvider registers an external IdP so users can sign in with it.
// POST /api/v1/identity/providers
func (h *IdentityHandler) CreateProvider(c *gin.Context) {
	var req createIdentityProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var idp identity.IdentityProvider
	if _, isSocial := identity.SocialProviders[req.Type]; isSocial {
		idp = identity.NewSocialProvider(req.Type, req.ClientID, req.ClientSecret, req.DefaultScope)
		if req.Alias != "" {
			idp.Alias = req.Alias
		}
		if req.DisplayName != "" {
			idp.DisplayName = req.DisplayName
		}
	} else if req.Type == "oidc" {
		if req.Alias == "" || req.AuthorizationURL == "" || req.TokenURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "alias, authorization_url and token_url are required for a generic OIDC provider",
			})
			return
		}
		name := req.DisplayName
		if name == "" {
			name = req.Alias
		}
		idp = identity.NewOIDCProvider(req.Alias, name, req.ClientID, req.ClientSecret,
			req.AuthorizationURL, req.TokenURL, req.UserInfoURL, req.Issuer)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider type: " + req.Type})
		return
	}

	if err := h.svc.CreateIdentityProvider(c.Request.Context(), idp); err != nil {
		h.fail(c, err, "create identity provider")
		return
	}
	h.logger.Info("identity provider created", zap.String("alias", idp.Alias))
	c.JSON(http.StatusCreated, gin.H{
		"message":      "identity provider created",
		"alias":        idp.Alias,
		"redirect_uri": "/auth/realms/" + h.svc.Realm() + "/broker/" + idp.Alias + "/endpoint",
	})
}

// ToggleProvider enables or disables an existing IdP.
// PUT /api/v1/identity/providers/:alias
func (h *IdentityHandler) ToggleProvider(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled is required"})
		return
	}

	ctx := c.Request.Context()
	alias := c.Param("alias")
	idps, err := h.svc.ListIdentityProviders(ctx)
	if err != nil {
		h.fail(c, err, "list identity providers")
		return
	}
	for _, idp := range idps {
		if idp.Alias != alias {
			continue
		}
		idp.Enabled = *req.Enabled
		if err := h.svc.UpdateIdentityProvider(ctx, alias, idp); err != nil {
			h.fail(c, err, "update identity provider")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "identity provider updated"})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "identity provider not found"})
}

// DeleteProvider removes an external IdP.
// DELETE /api/v1/identity/providers/:alias
func (h *IdentityHandler) DeleteProvider(c *gin.Context) {
	if err := h.svc.DeleteIdentityProvider(c.Request.Context(), c.Param("alias")); err != nil {
		h.fail(c, err, "delete identity provider")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "identity provider deleted"})
}

// ListSessions returns every active SSO session in the realm.
// GET /api/v1/identity/sessions
func (h *IdentityHandler) ListSessions(c *gin.Context) {
	sessions, err := h.svc.ListAllSessions(c.Request.Context())
	if err != nil {
		h.fail(c, err, "list keycloak sessions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions, "total": len(sessions)})
}

// DeleteSession revokes a single session.
// DELETE /api/v1/identity/sessions/:sid
func (h *IdentityHandler) DeleteSession(c *gin.Context) {
	if err := h.svc.DeleteSession(c.Request.Context(), c.Param("sid")); err != nil {
		h.fail(c, err, "delete keycloak session")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "session revoked"})
}

// ListClients returns the realm's OIDC clients (read-only view).
// GET /api/v1/identity/clients
func (h *IdentityHandler) ListClients(c *gin.Context) {
	clients, err := h.svc.ListClients(c.Request.Context())
	if err != nil {
		h.fail(c, err, "list keycloak clients")
		return
	}
	c.JSON(http.StatusOK, gin.H{"clients": clients})
}

// ListGroups returns the realm group tree.
// GET /api/v1/identity/groups
func (h *IdentityHandler) ListGroups(c *gin.Context) {
	groups, err := h.svc.ListGroups(c.Request.Context())
	if err != nil {
		h.fail(c, err, "list keycloak groups")
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

// ListGroupsDetailed returns the realm group tree with each group's mapped realm
// role and member count, which is what the portal's Groups tab renders.
// GET /api/v1/identity/groups (replaces the plain listing)
func (h *IdentityHandler) ListGroupsDetailed(c *gin.Context) {
	ctx := c.Request.Context()

	groups, err := h.svc.ListGroups(ctx)
	if err != nil {
		h.fail(c, err, "list keycloak groups")
		return
	}

	type groupView struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		Path    string   `json:"path"`
		Roles   []string `json:"roles"`
		Members int      `json:"members"`
	}

	out := make([]groupView, 0, len(groups))
	for _, g := range groups {
		view := groupView{ID: g.ID, Name: g.Name, Path: g.Path, Roles: []string{}}
		// A failure on one group should not blank the whole page.
		if roles, err := h.svc.GetGroupRoles(ctx, g.ID); err == nil {
			for _, r := range roles {
				if r.Name == "offline_access" || r.Name == "uma_authorization" ||
					strings.HasPrefix(r.Name, "default-roles-") {
					continue
				}
				view.Roles = append(view.Roles, r.Name)
			}
		}
		if members, err := h.svc.GetGroupMembers(ctx, g.ID); err == nil {
			view.Members = len(members)
		}
		out = append(out, view)
	}

	c.JSON(http.StatusOK, gin.H{"groups": out, "total": len(out)})
}

// CreateGroup adds a realm group, optionally mapping a role to it.
// POST /api/v1/identity/groups
func (h *IdentityHandler) CreateGroup(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := h.svc.CreateGroupWithRole(c.Request.Context(), req.Name, req.Role)
	if err != nil {
		h.fail(c, err, "create keycloak group")
		return
	}
	h.logger.Info("keycloak group created",
		zap.String("name", req.Name), zap.String("role", req.Role))
	c.JSON(http.StatusCreated, gin.H{"id": id, "message": "group created"})
}

// DeleteGroup removes a realm group. Members are not deleted, they simply lose
// whatever role the group granted them.
// DELETE /api/v1/identity/groups/:gid
func (h *IdentityHandler) DeleteGroup(c *gin.Context) {
	if err := h.svc.DeleteGroup(c.Request.Context(), c.Param("gid")); err != nil {
		h.fail(c, err, "delete keycloak group")
		return
	}
	h.logger.Info("keycloak group deleted", zap.String("id", c.Param("gid")))
	c.JSON(http.StatusOK, gin.H{"message": "group deleted"})
}

// SetGroupRole maps a realm role onto a group; members inherit it.
// POST /api/v1/identity/groups/:gid/role
func (h *IdentityHandler) SetGroupRole(c *gin.Context) {
	var req struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetGroupRole(c.Request.Context(), c.Param("gid"), req.Role); err != nil {
		h.fail(c, err, "set keycloak group role")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "group role updated"})
}

// ListGroupMembers returns the users in a group.
// GET /api/v1/identity/groups/:gid/members
func (h *IdentityHandler) ListGroupMembers(c *gin.Context) {
	members, err := h.svc.GetGroupMembers(c.Request.Context(), c.Param("gid"))
	if err != nil {
		h.fail(c, err, "list keycloak group members")
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members, "total": len(members)})
}

// AddUserToGroup puts a user into a group.
// PUT /api/v1/identity/users/:id/groups/:gid
func (h *IdentityHandler) AddUserToGroup(c *gin.Context) {
	if err := h.svc.AddUserToGroup(c.Request.Context(), c.Param("id"), c.Param("gid")); err != nil {
		h.fail(c, err, "add user to keycloak group")
		return
	}
	h.logger.Info("user added to group",
		zap.String("user", c.Param("id")), zap.String("group", c.Param("gid")))
	c.JSON(http.StatusOK, gin.H{"message": "user added to group"})
}

// RemoveUserFromGroup takes a user out of a group.
// DELETE /api/v1/identity/users/:id/groups/:gid
func (h *IdentityHandler) RemoveUserFromGroup(c *gin.Context) {
	if err := h.svc.RemoveUserFromGroup(c.Request.Context(), c.Param("id"), c.Param("gid")); err != nil {
		h.fail(c, err, "remove user from keycloak group")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user removed from group"})
}

// ListUserGroups returns the groups a user belongs to.
// GET /api/v1/identity/users/:id/groups
func (h *IdentityHandler) ListUserGroups(c *gin.Context) {
	groups, err := h.svc.GetUserGroups(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.fail(c, err, "list user groups")
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups, "total": len(groups)})
}
