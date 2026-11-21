package sso

import "testing"

func TestOIDCRejectsDisallowedIssuerURLs(t *testing.T) {
	for _, issuer := range []string{
		"http://idp.example",
		"https://127.0.0.1",
		"https://10.0.0.1",
		"https://[::1]",
		"https://user@idp.example",
	} {
		if err := validateOIDCServiceURL(issuer); err == nil {
			t.Fatalf("disallowed OIDC issuer accepted: %s", issuer)
		}
	}
}
