package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/m2p/clm/internal/api/rest/handlers"
	"github.com/m2p/clm/internal/api/rest/middleware"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/internal/saas/aicontract"
	"github.com/m2p/clm/internal/saas/contract"
	"github.com/m2p/clm/internal/saas/approval"
	"github.com/m2p/clm/internal/saas/cloudaccounts"
	"github.com/m2p/clm/internal/saas/deploy"
	"github.com/m2p/clm/internal/saas/identity"
	"github.com/m2p/clm/internal/saas/onboarding"
	"github.com/m2p/clm/internal/saas/itsm"
	"github.com/m2p/clm/internal/saas/risk"
	"github.com/m2p/clm/internal/saas/integrations"
	"github.com/m2p/clm/internal/saas/organization"
	"github.com/m2p/clm/internal/saas/sso"
	"github.com/m2p/clm/internal/saas/tenant"
	"github.com/m2p/clm/internal/saas/users"
	auditservice "github.com/m2p/clm/internal/services/audit"
	"github.com/m2p/clm/pkg/config"
	"go.uber.org/zap"
)

// Dependencies groups all service dependencies for the router.
type Dependencies struct {
	UserService         *users.Service
	TenantService       *tenant.Service
	SSOService          *sso.Service
	DeployService       *deploy.Service
	CloudAccountService *cloudaccounts.Service
	AuditService        *auditservice.Service
	IdentityService     *identity.Service
	ApprovalEngine      *approval.Engine
	OnboardingService   *onboarding.Service
	ITSMController        *itsm.ITSMController
	RiskController        *risk.RiskController
	IntegrationController *integrations.IntegrationController
	DepartmentController  *organization.DepartmentController
	ContractService       contract.Service
	AIContractService     aicontract.Service
	SatelliteRepo       core.SatelliteRepository
	Config              *config.ServerConfig
	Logger              *zap.Logger
}

// NewRouter creates and configures the gin router with all routes.
func NewRouter(deps *Dependencies) *gin.Engine {
	if deps.Config.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger(deps.Logger))

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "clm-saas-control-plane"})
	})

	// Public routes — auth
	// Corporate Enterprise Controllers
	itsmCtrl := deps.ITSMController
	if itsmCtrl == nil {
		itsmCtrl = itsm.NewITSMController(deps.Logger)
	}
	riskCtrl := deps.RiskController
	if riskCtrl == nil {
		riskCtrl = risk.NewRiskController(deps.Logger)
	}
	integCtrl := deps.IntegrationController
	if integCtrl == nil {
		integCtrl = integrations.NewIntegrationController(deps.Logger)
	}
	deptCtrl := deps.DepartmentController
	if deptCtrl == nil {
		deptCtrl = organization.NewDepartmentController(deps.Logger)
	}
	entHandler := handlers.NewEnterpriseOperationsHandler(itsmCtrl, riskCtrl, integCtrl, deptCtrl, deps.Logger)

	contractSvc := deps.ContractService
	if contractSvc == nil {
		contractSvc = contract.NewService(deps.Logger)
	}
	aiSvc := deps.AIContractService
	if aiSvc == nil {
		aiSvc = aicontract.NewEngine(deps.Logger)
	}
	contractHandler := handlers.NewContractsHandler(contractSvc, aiSvc, deps.Logger)

	authHandler := handlers.NewAuthHandler(deps.UserService, deps.TenantService)
	r.POST("/api/v1/auth/login", authHandler.Login)
	r.POST("/api/v1/auth/register", authHandler.Register)

	// SSO routes (public — redirects and callbacks)
	var ssoHandler *handlers.SSOHandler
	if deps.SSOService != nil {
		ssoHandler = handlers.NewSSOHandler(deps.SSOService)
		r.GET("/api/v1/auth/sso/providers", ssoHandler.ListProviders)
		r.GET("/api/v1/auth/saml/login", ssoHandler.SAMLLogin)
		r.POST("/api/v1/auth/saml/callback", ssoHandler.SAMLCallback)
		r.GET("/api/v1/auth/oidc/login", ssoHandler.OIDCLogin)
		r.GET("/api/v1/auth/oidc/callback", ssoHandler.OIDCCallback)
		r.GET("/api/v1/auth/google/login", ssoHandler.GoogleLogin)
		r.GET("/api/v1/auth/google/callback", ssoHandler.GoogleCallback)
		r.GET("/api/v1/auth/microsoft/login", ssoHandler.MicrosoftLogin)
		r.GET("/api/v1/auth/microsoft/callback", ssoHandler.MicrosoftCallback)
	}

	// Social signup routes (platform-level, no tenant_id required)
	socialSignupMsg := func(provider string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":    provider + " sign-up is not yet configured",
				"message":  "OAuth credentials for " + provider + " need to be configured by the platform admin. Please use email registration for now.",
				"provider": provider,
			})
		}
	}
	if deps.SSOService == nil {
		r.GET("/api/v1/auth/google/login", socialSignupMsg("Google"))
		r.GET("/api/v1/auth/microsoft/login", socialSignupMsg("Microsoft"))
	}
	r.GET("/api/v1/auth/github/login", socialSignupMsg("GitHub"))
	r.GET("/api/v1/auth/github/callback", socialSignupMsg("GitHub"))

	// Gateway transport is separately authenticated with a per-satellite bearer
	// credential and, when configured, a tenant/satellite-bound client certificate.
	satHandler := handlers.NewSatelliteHandler(deps.SatelliteRepo, deps.Config)
	gateway := r.Group("/api/v1/satellites/:id")
	gateway.Use(middleware.SatelliteAuth(deps.SatelliteRepo, *deps.Config))
	gateway.POST("/heartbeat", satHandler.Heartbeat)
	gateway.GET("/commands", satHandler.PullCommands)
	gateway.POST("/commands/:cmd_id/ack", satHandler.AcknowledgeCommand)
	gateway.POST("/commands/:cmd_id/lease", satHandler.RenewCommandLease)
	gateway.POST("/commands/:cmd_id/result", satHandler.ReportCommandResult)

	// Protected routes (JWT required)
	api := r.Group("/api/v1")
	api.Use(middleware.AuthMiddleware(deps.Config.JWT))
	if deps.AuditService != nil {
		api.Use(middleware.MutationAudit(deps.AuditService, deps.Logger))
	}
	{
		// Tenant management (super_admin only)
		tenantHandler := handlers.NewTenantHandler(deps.TenantService)
		tenants := api.Group("/tenants")
		{
			// admin is included alongside super_admin so the portal's Tenants
			// page works for both roles; only super_admin may delete.
			tenants.POST("", middleware.RoleRequired("super_admin", "admin"), tenantHandler.CreateTenant)
			tenants.GET("", middleware.RoleRequired("super_admin", "admin"), tenantHandler.ListTenants)
			tenants.GET("/:id", tenantHandler.GetTenant)
			tenants.PATCH("/:id", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), tenantHandler.UpdateTenant)
			tenants.DELETE("/:id", middleware.RoleRequired("super_admin"), tenantHandler.DeleteTenant)

			if ssoHandler != nil {
				tenants.POST("/:id/sso", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), ssoHandler.ConfigureSSO)
				tenants.GET("/:id/sso", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), ssoHandler.GetSSO)
				tenants.DELETE("/:id/sso", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), ssoHandler.DeleteSSO)
				tenants.GET("/:id/sso/mappings", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), ssoHandler.ListGroupMappings)
				tenants.POST("/:id/sso/mappings", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), ssoHandler.CreateGroupMapping)
				tenants.DELETE("/:id/sso/mappings/:mapping_id", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), ssoHandler.DeleteGroupMapping)
			}
		}

		// Enterprise customer onboarding (hybrid-managed AWS account bootstrap)
		if deps.OnboardingService != nil {
			onboardingHandler := handlers.NewOnboardingHandler(deps.OnboardingService)
			onboardingGroup := api.Group("/onboarding")
			{
				onboardingGroup.POST("/preflight", middleware.RoleRequired("super_admin", "admin"), onboardingHandler.Preflight)
				onboardingGroup.POST("/bootstrap", middleware.RoleRequired("super_admin", "admin"), onboardingHandler.Bootstrap)
			}
		}

		// User management. Cross-tenant targeting is explicit via tenant_id and
		// is permitted only to platform roles by tenantForRequest.
		userHandler := handlers.NewUserHandler(deps.UserService)
		usersGroup := api.Group("/users")
		{
			usersGroup.POST("", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), userHandler.CreateUser)
			usersGroup.GET("", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "auditor"), userHandler.ListUsers)
			usersGroup.GET("/:id", userHandler.GetUser)
			usersGroup.PATCH("/:id/role", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), userHandler.UpdateRole)
			usersGroup.DELETE("/:id", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), userHandler.DisableUser)
		}

		// Gateway fleet management + deployment
		satellites := api.Group("/satellites")
		{
			satellites.POST("", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "operator"), satHandler.RegisterSatellite)
			satellites.GET("", satHandler.ListSatellites)
			satellites.GET("/:id", satHandler.GetSatellite)
			satellites.PATCH("/:id", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), satHandler.UpdateSatellite)
			satellites.DELETE("/:id", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), satHandler.DeleteSatellite)
			satellites.POST("/:id/credentials/rotate", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), satHandler.RotateCredential)
			satellites.POST("/:id/credentials/revoke", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), satHandler.RevokeCredential)
			satellites.POST("/:id/commands/:cmd_id/cancel", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "operator"), satHandler.CancelCommand)
			satellites.POST("/:id/commands/:cmd_id/retry", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "operator"), satHandler.RetryCommand)

			if deps.DeployService != nil && deps.CloudAccountService != nil {
				deployHandler := handlers.NewDeployHandler(deps.DeployService, deps.CloudAccountService)
				satellites.POST("/deploy", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), gatewayDeploymentCreateHandler(deps.Config.Env, deployHandler.Deploy))
				satellites.GET("/deploy", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), deployHandler.ListDeployJobs)
				satellites.GET("/deploy/:job_id", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), deployHandler.GetJobStatus)
				satellites.GET("/deploy/:job_id/logs", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), deployHandler.GetJobLogs)
				satellites.POST("/deploy/:job_id/cancel", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), deployHandler.CancelJob)
				satellites.POST("/deploy/validate-credentials", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), deployHandler.ValidateCredentials)
				satellites.POST("/deploy/list-infra", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), deployHandler.ListInfra)
				satellites.DELETE("/:id/teardown", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), deployHandler.Teardown)
				satellites.POST("/:id/upgrade", middleware.RoleRequired("super_admin", "admin", "tenant_admin"), deployHandler.Upgrade)
			}
		}

		// Tenant-scoped encrypted cloud accounts. Credentials are accepted only on
		// create/rotate and are never returned by these routes.
		if deps.CloudAccountService != nil {
			cloudHandler := handlers.NewCloudAccountHandler(deps.CloudAccountService)
			cloud := api.Group("/cloud-accounts", middleware.RoleRequired("super_admin", "admin", "tenant_admin"))
			cloud.POST("", cloudHandler.Create)
			cloud.GET("", cloudHandler.List)
			cloud.GET("/:id", cloudHandler.Get)
			cloud.PATCH("/:id", cloudHandler.Rename)
			cloud.POST("/:id/rotate", cloudHandler.Rotate)
			cloud.POST("/:id/validate", cloudHandler.Validate)
			cloud.DELETE("/:id", cloudHandler.Delete)
		}

		if deps.AuditService != nil {
			auditHandler := handlers.NewAuditHandler(deps.AuditService)
			api.GET("/audits", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "auditor"), auditHandler.List)
		}

		// Corporate multi-level approval workflow with segregation of duties (ARC-004)
		if deps.ApprovalEngine != nil {
			approvalHandler := handlers.NewApprovalHandler(deps.ApprovalEngine)
			approvals := api.Group("/approvals")
			{
				approvals.POST("", approvalHandler.Submit)
				approvals.GET("/pending", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "approver"), approvalHandler.ListPending)
				approvals.GET("/:id", approvalHandler.GetRequest)
				approvals.POST("/:id/approve", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "approver"), approvalHandler.Approve)
				approvals.POST("/:id/reject", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "approver"), approvalHandler.Reject)
				approvals.POST("/:id/escalate", middleware.RoleRequired("super_admin", "admin", "tenant_admin", "approver"), approvalHandler.Escalate)
			}
		}

		// Keycloak realm administration, surfaced natively in the Admin Portal
		// so the Keycloak Admin Console is never exposed to operators.
		if deps.IdentityService != nil {
			idHandler := handlers.NewIdentityHandler(deps.IdentityService, deps.Logger)
			ident := api.Group("/identity", middleware.RoleRequired("super_admin", "admin"))
			{
				ident.GET("/status", idHandler.Status)
				ident.GET("/clients", idHandler.ListClients)

				// Local realm groups. Members inherit the group's realm role,
				// which is how RBAC is managed by team rather than per user.
				ident.GET("/groups", idHandler.ListGroupsDetailed)
				ident.POST("/groups", idHandler.CreateGroup)
				ident.DELETE("/groups/:gid", idHandler.DeleteGroup)
				ident.POST("/groups/:gid/role", idHandler.SetGroupRole)
				ident.GET("/groups/:gid/members", idHandler.ListGroupMembers)

				ident.GET("/users", idHandler.ListUsers)
				ident.POST("/users", idHandler.CreateUser)
				ident.GET("/users/:id", idHandler.GetUser)
				ident.PUT("/users/:id", idHandler.UpdateUser)
				ident.DELETE("/users/:id", idHandler.DeleteUser)
				ident.POST("/users/:id/enabled", idHandler.SetEnabled)
				ident.POST("/users/:id/password", idHandler.ResetPassword)
				ident.POST("/users/:id/unlock", idHandler.UnlockUser)
				ident.POST("/users/:id/role", idHandler.SetUserRole)
				ident.POST("/users/:id/logout", idHandler.LogoutUser)
				ident.GET("/users/:id/groups", idHandler.ListUserGroups)
				ident.PUT("/users/:id/groups/:gid", idHandler.AddUserToGroup)
				ident.DELETE("/users/:id/groups/:gid", idHandler.RemoveUserFromGroup)

				ident.GET("/roles", idHandler.ListRoles)
				ident.POST("/roles", idHandler.CreateRole)
				ident.DELETE("/roles/:name", idHandler.DeleteRole)

				ident.GET("/providers", idHandler.ListProviders)
				ident.POST("/providers", idHandler.CreateProvider)
				ident.PUT("/providers/:alias", idHandler.ToggleProvider)
				ident.DELETE("/providers/:alias", idHandler.DeleteProvider)

				ident.GET("/sessions", idHandler.ListSessions)
				ident.DELETE("/sessions/:sid", idHandler.DeleteSession)
			}
		}
		// Enterprise Operations (ITSM, Risk, Integrations, Departments)
		enterprise := api.Group("/enterprise")
		{
			enterprise.GET("/incidents", entHandler.ListIncidents)
			enterprise.POST("/incidents", entHandler.RaiseIncident)
			enterprise.PATCH("/incidents/:id/resolve", entHandler.ResolveIncident)
			enterprise.GET("/risks", entHandler.ListRisks)
			enterprise.POST("/risks", entHandler.LogRisk)
			enterprise.GET("/integrations", entHandler.ListIntegrations)
			enterprise.POST("/integrations", entHandler.CreateIntegration)
			enterprise.GET("/departments", entHandler.ListDepartments)
			enterprise.POST("/departments", entHandler.CreateDepartment)
		}

		// Enterprise Contract Lifecycle Management (CLM)
		contracts := api.Group("/contracts")
		{
			contracts.GET("", contractHandler.ListContracts)
			contracts.POST("", contractHandler.CreateContract)
			contracts.GET("/analytics", contractHandler.GetAnalytics)
			contracts.GET("/:id", contractHandler.GetContract)
			contracts.PUT("/:id", contractHandler.UpdateContract)
			contracts.POST("/:id/transition", contractHandler.TransitionContract)
			contracts.GET("/:id/versions", contractHandler.ListVersions)
			contracts.POST("/:id/versions", contractHandler.CreateVersion)
			contracts.GET("/:id/negotiations", contractHandler.ListNegotiations)
			contracts.POST("/:id/negotiations", contractHandler.SubmitNegotiation)
			contracts.GET("/:id/signatures", contractHandler.ListSignatures)
			contracts.POST("/:id/signatures", contractHandler.AddSigner)
			contracts.POST("/signatures/:sig_id/execute", contractHandler.ExecuteSignature)
			contracts.GET("/obligations", contractHandler.ListObligations)
			contracts.POST("/:id/obligations", contractHandler.AddObligation)
			contracts.POST("/obligations/:obl_id/fulfill", contractHandler.FulfillObligation)
			contracts.POST("/:id/ai-analyze", contractHandler.AnalyzeContractAI)
			contracts.GET("/:id/ai-analysis", contractHandler.GetAIAnalysis)
			contracts.POST("/:id/ask", contractHandler.AskAI)

			// Requests & Intake
			contracts.GET("/requests", contractHandler.ListRequests)
			contracts.POST("/requests", contractHandler.SubmitRequest)
			contracts.PATCH("/requests/:id/triage", contractHandler.TriageRequest)
			contracts.POST("/requests/:id/convert", contractHandler.ConvertRequest)

			// Templates & Clauses
			contracts.GET("/templates", contractHandler.ListTemplates)
			contracts.POST("/templates", contractHandler.CreateTemplate)
			contracts.GET("/clauses", contractHandler.ListClauses)
			contracts.POST("/clauses", contractHandler.CreateClause)
			contracts.POST("/templates/assemble", contractHandler.AssembleContract)
		}

	}

	return r
}

// gatewayDeploymentCreateHandler disables the legacy cloud bootstrap in every
// environment. The current deployment adapters cannot inject a one-time
// satellite credential and tenant/satellite-bound client identity, so allowing
// them to create a gateway would produce an instance that cannot authenticate.
// Keep the endpoint fail-closed until the secure bootstrap contract is complete.
func gatewayDeploymentCreateHandler(_ string, _ gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "secure gateway deployment bootstrap is not configured",
			"message": "gateway deployment requires a pre-provisioned satellite credential and tenant-bound mTLS identity",
		})
	}
}
