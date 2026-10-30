package identity

// User mirrors the Keycloak UserRepresentation fields the portal needs.
type User struct {
	ID               string              `json:"id,omitempty"`
	Username         string              `json:"username"`
	Email            string              `json:"email,omitempty"`
	FirstName        string              `json:"firstName,omitempty"`
	LastName         string              `json:"lastName,omitempty"`
	Enabled          bool                `json:"enabled"`
	EmailVerified    bool                `json:"emailVerified"`
	CreatedTimestamp int64               `json:"createdTimestamp,omitempty"`
	Attributes       map[string][]string `json:"attributes,omitempty"`
	RequiredActions  []string            `json:"requiredActions,omitempty"`
	Credentials      []Credential        `json:"credentials,omitempty"`

	// Roles is populated by the service layer, not by Keycloak itself.
	Roles []string `json:"roles,omitempty"`
}

// Credential represents a password to set on a user.
type Credential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

// Role is a Keycloak realm role.
type Role struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Composite   bool   `json:"composite,omitempty"`
	ClientRole  bool   `json:"clientRole,omitempty"`
}

// IdentityProvider is a Keycloak IdentityProviderRepresentation.
type IdentityProvider struct {
	Alias                     string            `json:"alias"`
	DisplayName               string            `json:"displayName,omitempty"`
	ProviderID                string            `json:"providerId"`
	Enabled                   bool              `json:"enabled"`
	TrustEmail                bool              `json:"trustEmail,omitempty"`
	StoreToken                bool              `json:"storeToken,omitempty"`
	FirstBrokerLoginFlowAlias string            `json:"firstBrokerLoginFlowAlias,omitempty"`
	Config                    map[string]string `json:"config,omitempty"`
}

// Session is a Keycloak UserSessionRepresentation.
type Session struct {
	ID         string            `json:"id"`
	Username   string            `json:"username"`
	UserID     string            `json:"userId"`
	IPAddress  string            `json:"ipAddress"`
	Start      int64             `json:"start"`
	LastAccess int64             `json:"lastAccess"`
	Clients    map[string]string `json:"clients,omitempty"`
}

// Group is a Keycloak GroupRepresentation.
type Group struct {
	ID        string   `json:"id,omitempty"`
	Name      string   `json:"name"`
	Path      string   `json:"path,omitempty"`
	SubGroups []Group  `json:"subGroups,omitempty"`
	RealmRoles []string `json:"realmRoles,omitempty"`
}

// RealmStats is a lightweight summary for the portal header cards.
type RealmStats struct {
	Realm             string `json:"realm"`
	Users             int    `json:"users"`
	EnabledUsers      int    `json:"enabled_users"`
	Roles             int    `json:"roles"`
	IdentityProviders int    `json:"identity_providers"`
	ActiveSessions    int    `json:"active_sessions"`
	Reachable         bool   `json:"reachable"`
}
