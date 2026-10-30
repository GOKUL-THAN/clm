package sso

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"

	"go.uber.org/zap"
)

// SAMLProvider implements SAML 2.0 Service Provider login flow.
type SAMLProvider struct {
	config *SAMLConfig
	logger *zap.Logger
	idpCert *x509.Certificate
}

// NewSAMLProvider creates a new SAML provider instance.
func NewSAMLProvider(config *SAMLConfig, logger *zap.Logger) (*SAMLProvider, error) {
	p := &SAMLProvider{
		config: config,
		logger: logger,
	}

	// Parse IdP certificate if provided
	if config.IDPCert != "" {
		cert, err := parseCertificatePEM(config.IDPCert)
		if err != nil {
			return nil, fmt.Errorf("parsing IdP certificate: %w", err)
		}
		p.idpCert = cert
	}

	return p, nil
}

func (p *SAMLProvider) Type() ProviderType {
	return ProviderSAML
}

// GetAuthURL generates a SAML AuthnRequest and returns the IdP redirect URL.
func (p *SAMLProvider) GetAuthURL(ctx context.Context, state string) (*AuthURL, error) {
	if p.config.IDPSSOURL == "" {
		return nil, errors.New("SAML IdP SSO URL not configured")
	}

	// Build SAML AuthnRequest
	authnRequest := buildSAMLAuthnRequest(p.config.SPEntityID, p.config.SPACSURL)

	// Base64 + deflate encode the request
	encoded := base64.StdEncoding.EncodeToString([]byte(authnRequest))

	// Build redirect URL with SAMLRequest parameter
	redirectURL, err := url.Parse(p.config.IDPSSOURL)
	if err != nil {
		return nil, fmt.Errorf("parsing IdP SSO URL: %w", err)
	}

	q := redirectURL.Query()
	q.Set("SAMLRequest", encoded)
	q.Set("RelayState", state)
	redirectURL.RawQuery = q.Encode()

	p.logger.Info("SAML AuthnRequest generated",
		zap.String("idp_url", p.config.IDPSSOURL),
		zap.String("sp_entity_id", p.config.SPEntityID),
	)

	return &AuthURL{
		URL:      redirectURL.String(),
		State:    state,
		Provider: ProviderSAML,
	}, nil
}

// HandleCallback processes the SAML Response posted back from the IdP.
func (p *SAMLProvider) HandleCallback(ctx context.Context, params map[string]string) (*SSOUser, error) {
	samlResponse := params["SAMLResponse"]
	if samlResponse == "" {
		return nil, errors.New("missing SAMLResponse parameter")
	}

	// Decode base64 SAML response
	responseXML, err := base64.StdEncoding.DecodeString(samlResponse)
	if err != nil {
		return nil, fmt.Errorf("decoding SAMLResponse: %w", err)
	}

	// Parse the SAML response
	var response samlResponseEnvelope
	if err := xml.Unmarshal(responseXML, &response); err != nil {
		return nil, fmt.Errorf("parsing SAML response XML: %w", err)
	}

	// Validate status
	if response.Status.StatusCode.Value != "urn:oasis:names:tc:SAML:2.0:status:Success" {
		return nil, fmt.Errorf("SAML authentication failed: %s", response.Status.StatusCode.Value)
	}

	// Extract user attributes from assertion
	user := &SSOUser{
		Provider: ProviderSAML,
	}

	for _, attr := range response.Assertion.AttributeStatement.Attributes {
		switch {
		case matchesAttribute(attr.Name, p.config.AttributeMap, "email"):
			if len(attr.Values) > 0 {
				user.Email = attr.Values[0].Value
			}
		case matchesAttribute(attr.Name, p.config.AttributeMap, "name"):
			if len(attr.Values) > 0 {
				user.Name = attr.Values[0].Value
			}
		case matchesAttribute(attr.Name, p.config.AttributeMap, "role"):
			for _, v := range attr.Values {
				user.Groups = append(user.Groups, v.Value)
			}
		}
	}

	// Fallback: use NameID as email if not found in attributes
	if user.Email == "" && response.Assertion.Subject.NameID.Value != "" {
		user.Email = response.Assertion.Subject.NameID.Value
	}

	if user.Email == "" {
		return nil, errors.New("could not extract email from SAML assertion")
	}

	p.logger.Info("SAML login successful",
		zap.String("email", user.Email),
		zap.String("name", user.Name),
	)

	return user, nil
}

// ValidateConfig checks if the SAML configuration is complete.
func (p *SAMLProvider) ValidateConfig() error {
	if p.config.IDPSSOURL == "" {
		return errors.New("SAML: idp_sso_url is required")
	}
	if p.config.SPEntityID == "" {
		return errors.New("SAML: sp_entity_id is required")
	}
	if p.config.SPACSURL == "" {
		return errors.New("SAML: sp_acs_url is required")
	}
	return nil
}

// --- SAML XML structures ---

type samlResponseEnvelope struct {
	XMLName   xml.Name         `xml:"Response"`
	Status    samlStatus       `xml:"Status"`
	Assertion samlAssertion    `xml:"Assertion"`
}

type samlStatus struct {
	StatusCode struct {
		Value string `xml:"Value,attr"`
	} `xml:"StatusCode"`
}

type samlAssertion struct {
	Subject            samlSubject            `xml:"Subject"`
	AttributeStatement samlAttributeStatement `xml:"AttributeStatement"`
}

type samlSubject struct {
	NameID struct {
		Value string `xml:",chardata"`
	} `xml:"NameID"`
}

type samlAttributeStatement struct {
	Attributes []samlAttribute `xml:"Attribute"`
}

type samlAttribute struct {
	Name   string           `xml:"Name,attr"`
	Values []samlAttrValue  `xml:"AttributeValue"`
}

type samlAttrValue struct {
	Value string `xml:",chardata"`
}

// --- Helpers ---

func buildSAMLAuthnRequest(entityID, acsURL string) string {
	return fmt.Sprintf(`<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"
  xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"
  AssertionConsumerServiceURL="%s"
  Destination=""
  IssueInstant="2024-01-01T00:00:00Z"
  Version="2.0">
  <saml:Issuer>%s</saml:Issuer>
</samlp:AuthnRequest>`, acsURL, entityID)
}

func matchesAttribute(attrName string, mapping map[string]string, target string) bool {
	if mapping == nil {
		// Default SAML attribute names
		defaults := map[string]string{
			"email": "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
			"name":  "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name",
			"role":  "http://schemas.microsoft.com/ws/2008/06/identity/claims/role",
		}
		return attrName == defaults[target]
	}
	return attrName == mapping[target]
}

func parseCertificatePEM(certStr string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certStr))
	if block == nil {
		return nil, errors.New("failed to decode PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}
