package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ListUsers returns realm users. search matches username, email, first and
// last name. Pass max <= 0 for the default page size.
func (c *Client) ListUsers(ctx context.Context, search string, first, max int) ([]User, error) {
	q := url.Values{}
	if search != "" {
		q.Set("search", search)
	}
	if first > 0 {
		q.Set("first", strconv.Itoa(first))
	}
	if max <= 0 {
		max = 200
	}
	q.Set("max", strconv.Itoa(max))
	q.Set("briefRepresentation", "false")

	raw, err := c.do(ctx, http.MethodGet, "/users?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(raw, &users); err != nil {
		return nil, fmt.Errorf("decode users: %w", err)
	}
	return users, nil
}

// GetUser fetches a single user by Keycloak ID.
func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	raw, err := c.do(ctx, http.MethodGet, "/users/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}
	return &u, nil
}

// CountUsers returns the total number of users in the realm.
func (c *Client) CountUsers(ctx context.Context) (int, error) {
	raw, err := c.do(ctx, http.MethodGet, "/users/count", nil)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(string(raw))
	if err != nil {
		return 0, fmt.Errorf("decode user count: %w", err)
	}
	return n, nil
}

// CreateUser creates a user and, when password is non-empty, sets it. The new
// user's Keycloak ID is returned.
func (c *Client) CreateUser(ctx context.Context, u User, password string, temporary bool) (string, error) {
	if password != "" {
		u.Credentials = []Credential{{Type: "password", Value: password, Temporary: temporary}}
	}
	// Keycloak requires a username; fall back to the email.
	if u.Username == "" {
		u.Username = u.Email
	}

	_, status, err := c.doRaw(ctx, http.MethodPost, "/users", u)
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		c.invalidateToken()
		_, status, err = c.doRaw(ctx, http.MethodPost, "/users", u)
		if err != nil {
			return "", err
		}
	}
	if status == http.StatusConflict {
		return "", fmt.Errorf("a user with that username or email already exists")
	}
	if status < 200 || status > 299 {
		return "", fmt.Errorf("create user failed with status %d", status)
	}

	// Keycloak returns the ID in the Location header, but resolving by
	// username is more portable across proxy setups.
	created, err := c.findByUsername(ctx, u.Username)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// findByUsername resolves a user by exact username.
func (c *Client) findByUsername(ctx context.Context, username string) (*User, error) {
	q := url.Values{}
	q.Set("username", username)
	q.Set("exact", "true")

	raw, err := c.do(ctx, http.MethodGet, "/users?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(raw, &users); err != nil {
		return nil, fmt.Errorf("decode users: %w", err)
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user %q not found after creation", username)
	}
	return &users[0], nil
}

// UpdateUser applies a partial update to an existing user.
func (c *Client) UpdateUser(ctx context.Context, id string, u User) error {
	_, err := c.do(ctx, http.MethodPut, "/users/"+url.PathEscape(id), u)
	return err
}

// DeleteUser permanently removes a user from the realm.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/users/"+url.PathEscape(id), nil)
	return err
}

// SetEnabled enables or disables a user without deleting them.
func (c *Client) SetEnabled(ctx context.Context, id string, enabled bool) error {
	body := map[string]any{"enabled": enabled}
	_, err := c.do(ctx, http.MethodPut, "/users/"+url.PathEscape(id), body)
	return err
}

// ResetPassword replaces a user's password. When temporary is true the user
// must change it at next login.
func (c *Client) ResetPassword(ctx context.Context, id, password string, temporary bool) error {
	body := Credential{Type: "password", Value: password, Temporary: temporary}
	_, err := c.do(ctx, http.MethodPut,
		"/users/"+url.PathEscape(id)+"/reset-password", body)
	return err
}

// SendActionEmail asks Keycloak to email the user a required-action link,
// e.g. "UPDATE_PASSWORD" or "VERIFY_EMAIL".
func (c *Client) SendActionEmail(ctx context.Context, id string, actions []string) error {
	_, err := c.do(ctx, http.MethodPut,
		"/users/"+url.PathEscape(id)+"/execute-actions-email", actions)
	return err
}

// LogoutUser invalidates every session belonging to the user.
func (c *Client) LogoutUser(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodPost, "/users/"+url.PathEscape(id)+"/logout", nil)
	return err
}

// GetUserSessions lists the active sessions for one user.
func (c *Client) GetUserSessions(ctx context.Context, id string) ([]Session, error) {
	raw, err := c.do(ctx, http.MethodGet, "/users/"+url.PathEscape(id)+"/sessions", nil)
	if err != nil {
		return nil, err
	}
	var sessions []Session
	if err := json.Unmarshal(raw, &sessions); err != nil {
		return nil, fmt.Errorf("decode sessions: %w", err)
	}
	return sessions, nil
}

// ClearRequiredActions removes every pending required action from a user.
//
// Keycloak refuses a Direct Grant (password) login while a user has any required
// action outstanding, answering "Account is not fully set up". Both CLM portals
// authenticate exclusively with Direct Grant, so a user carrying an action such
// as UPDATE_PASSWORD cannot sign in at all. This is the repair path for accounts
// left in that state.
//
// Keycloak's user PUT replaces the representation, so the current record is read
// and merged rather than patched — sending only requiredActions would drop
// required fields such as email.
func (c *Client) ClearRequiredActions(ctx context.Context, id string) error {
	current, err := c.GetUser(ctx, id)
	if err != nil {
		return err
	}

	// User.RequiredActions is tagged omitempty, so an empty slice would be
	// dropped from the body and Keycloak would clear nothing. Use a local type
	// that always emits the field.
	payload := struct {
		Username        string              `json:"username"`
		Email           string              `json:"email,omitempty"`
		FirstName       string              `json:"firstName,omitempty"`
		LastName        string              `json:"lastName,omitempty"`
		Enabled         bool                `json:"enabled"`
		EmailVerified   bool                `json:"emailVerified"`
		Attributes      map[string][]string `json:"attributes,omitempty"`
		RequiredActions []string            `json:"requiredActions"`
	}{
		Username:        current.Username,
		Email:           current.Email,
		FirstName:       current.FirstName,
		LastName:        current.LastName,
		Enabled:         current.Enabled,
		EmailVerified:   current.EmailVerified,
		Attributes:      current.Attributes,
		RequiredActions: []string{},
	}

	_, err = c.do(ctx, http.MethodPut, "/users/"+url.PathEscape(id), payload)
	return err
}

// RepairLogin makes an account able to complete a Direct Grant login again and
// reports what it changed.
//
// Two separate conditions produce Keycloak's "Account is not fully set up":
//
//  1. a stored required action such as UPDATE_PASSWORD, and
//  2. a missing required user-profile attribute — the clm realm marks firstName
//     and lastName required, so Keycloak derives a VERIFY_PROFILE action at
//     login time. That one never appears in requiredActions, which is why
//     clearing actions alone does not fix such an account.
//
// firstName/lastName are used when supplied, otherwise names are derived from the
// email local part so the profile becomes complete.
func (c *Client) RepairLogin(ctx context.Context, id, firstName, lastName string) ([]string, error) {
	current, err := c.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}

	var changes []string
	if len(current.RequiredActions) > 0 {
		changes = append(changes, "cleared pending actions: "+strings.Join(current.RequiredActions, ", "))
	}

	first, last := current.FirstName, current.LastName
	derivedFirst, derivedLast := namesFromEmail(current.Email, current.Username)

	if firstName != "" {
		first = firstName
	} else if first == "" {
		first = derivedFirst
	}
	if lastName != "" {
		last = lastName
	} else if last == "" {
		last = derivedLast
	}
	if first != current.FirstName || last != current.LastName {
		changes = append(changes, fmt.Sprintf("completed required profile name (%s %s)", first, last))
	}

	payload := struct {
		Username        string              `json:"username"`
		Email           string              `json:"email,omitempty"`
		FirstName       string              `json:"firstName"`
		LastName        string              `json:"lastName"`
		Enabled         bool                `json:"enabled"`
		EmailVerified   bool                `json:"emailVerified"`
		Attributes      map[string][]string `json:"attributes,omitempty"`
		RequiredActions []string            `json:"requiredActions"`
	}{
		Username:        current.Username,
		Email:           current.Email,
		FirstName:       first,
		LastName:        last,
		Enabled:         current.Enabled,
		EmailVerified:   true, // VERIFY_EMAIL would block the grant just the same
		Attributes:      current.Attributes,
		RequiredActions: []string{},
	}
	if !current.EmailVerified {
		changes = append(changes, "marked email verified")
	}
	if !current.Enabled {
		payload.Enabled = true
		changes = append(changes, "re-enabled the account")
	}

	if _, err := c.do(ctx, http.MethodPut, "/users/"+url.PathEscape(id), payload); err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		changes = append(changes, "no blocking condition found")
	}
	return changes, nil
}

// namesFromEmail derives a plausible first and last name from an email local
// part, used to satisfy the realm's required profile attributes when repairing an
// account whose names were never filled in.
func namesFromEmail(email, username string) (string, string) {
	local := email
	if local == "" {
		local = username
	}
	if i := strings.Index(local, "@"); i > 0 {
		local = local[:i]
	}
	parts := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '+'
	})
	title := func(s string) string {
		if s == "" {
			return ""
		}
		return strings.ToUpper(s[:1]) + s[1:]
	}
	switch {
	case len(parts) >= 2:
		return title(parts[0]), title(parts[len(parts)-1])
	case len(parts) == 1:
		return title(parts[0]), "User"
	default:
		return "CLM", "User"
	}
}

// SetUserAttribute stores a single attribute on a user, preserving the rest of
// the record and any attributes already present.
func (c *Client) SetUserAttribute(ctx context.Context, id, key string, value string) error {
	current, err := c.GetUser(ctx, id)
	if err != nil {
		return err
	}

	attrs := current.Attributes
	if attrs == nil {
		attrs = make(map[string][]string)
	}
	attrs[key] = []string{value}

	payload := User{
		Username:      current.Username,
		Email:         current.Email,
		FirstName:     current.FirstName,
		LastName:      current.LastName,
		Enabled:       current.Enabled,
		EmailVerified: current.EmailVerified,
		Attributes:    attrs,
	}

	_, err = c.do(ctx, http.MethodPut, "/users/"+url.PathEscape(id), payload)
	return err
}
