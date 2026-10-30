package ca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// GoDaddy Certificates API.
//
// Ordering is asynchronous: POST /v1/certificates returns a certificateId while
// the order sits in PENDING_ISSUANCE until domain control is validated. The
// certificate is only downloadable once the state reaches ISSUED, so Issue
// returns StatusPending with the validation instructions and the caller comes
// back through OrderStatus.
//
// Auth is an sso-key header rather than a bearer token:
//
//	Authorization: sso-key <key>:<secret>
//
// The OTE environment (api.ote-godaddy.com) is a genuine sandbox with its own
// credentials, which is what makes this integration testable without spending
// money. Production is a separate host and separate keys.
type GoDaddyProvider struct {
	key        string
	secret     string
	baseURL    string
	env        string
	shopperID  string
	client     *http.Client
	logger     *zap.Logger
}

// GoDaddyConfig configures the provider.
type GoDaddyConfig struct {
	Key    string
	Secret string
	// Environment is "ote" (sandbox) or "production".
	Environment string
	// ShopperID is required when the API key belongs to a reseller acting for a
	// customer; ignored otherwise.
	ShopperID string
	BaseURL   string // overrides the host, for testing
}

const (
	goDaddyProduction = "https://api.godaddy.com"
	goDaddyOTE        = "https://api.ote-godaddy.com"
)

// NewGoDaddyProvider builds a GoDaddy provider.
func NewGoDaddyProvider(cfg GoDaddyConfig, logger *zap.Logger) (*GoDaddyProvider, error) {
	if strings.TrimSpace(cfg.Key) == "" || strings.TrimSpace(cfg.Secret) == "" {
		return nil, fmt.Errorf("a GoDaddy API key and secret are required")
	}

	base := strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	env := strings.ToLower(strings.TrimSpace(cfg.Environment))
	if base == "" {
		if env == "production" || env == "prod" {
			base = goDaddyProduction
			env = "production"
		} else {
			base = goDaddyOTE
			env = "ote"
		}
	} else if env == "" {
		env = "custom"
	}

	return &GoDaddyProvider{
		key: cfg.Key, secret: cfg.Secret,
		baseURL: base, env: env, shopperID: cfg.ShopperID,
		client:  &http.Client{Timeout: 45 * time.Second},
		logger:  logger,
	}, nil
}

func (g *GoDaddyProvider) Name() string { return "godaddy" }

func (g *GoDaddyProvider) Capabilities() Capabilities {
	return Capabilities{
		Synchronous: false,
		CertTypes:   []string{"dv", "ov", "ev"},
		Wildcard:    true,
		MultiSAN:    true,
		DCVMethods:  []string{DCVHTTP, DCVDNS, DCVEmail},
		RequiresCSR: true,
		Revocation:  true,
		// GoDaddy has no public pricing endpoint on the Certificates API, so
		// quoting returns nothing rather than fabricated numbers.
		Quoting:          false,
		MaxValidityYears: 2,
		Notes: "Orders are asynchronous: the certificate is downloadable only once " +
			"the order reaches ISSUED. Pricing is not exposed by the API.",
	}
}

// do performs an authenticated request and decodes the response.
func (g *GoDaddyProvider) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "sso-key "+g.key+":"+g.secret)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if g.shopperID != "" {
		req.Header.Set("X-Shopper-Id", g.shopperID)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("calling GoDaddy: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("reading GoDaddy response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("GoDaddy returned HTTP %d: %s", resp.StatusCode, goDaddyError(raw))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding GoDaddy response: %w", err)
	}
	return nil
}

// goDaddyError pulls the useful part out of an error body. GoDaddy returns a
// structured error with per-field detail, and surfacing only the HTTP status
// would hide the reason an order was rejected.
func goDaddyError(raw []byte) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Fields  []struct {
			Path    string `json:"path"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw, &e); err != nil || (e.Message == "" && e.Code == "") {
		s := strings.TrimSpace(string(raw))
		if len(s) > 300 {
			s = s[:300] + "..."
		}
		if s == "" {
			return "(empty response body)"
		}
		return s
	}

	parts := []string{}
	if e.Code != "" {
		parts = append(parts, e.Code)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Path, f.Message))
	}
	return strings.Join(parts, " | ")
}

// goDaddyProductFor maps a cert type and shape onto a GoDaddy productType.
func goDaddyProductFor(certType string, wildcard bool, sans int) string {
	switch NormaliseCertType(certType) {
	case "ev":
		return "EV_SSL"
	case "ov":
		if wildcard {
			return "OV_WILDCARD_SSL"
		}
		if sans > 1 {
			return "OV_UCC_SSL"
		}
		return "OV_SSL"
	default:
		if wildcard {
			return "DV_WILDCARD_SSL"
		}
		if sans > 1 {
			return "DV_UCC_SSL"
		}
		return "DV_SSL"
	}
}

func (g *GoDaddyProvider) Quote(ctx context.Context, req QuoteRequest) ([]Quote, error) {
	// Deliberately empty: the Certificates API exposes no pricing, and returning
	// plausible-looking numbers would be worse than returning none.
	return nil, nil
}

// goDaddyOrderRequest is the create-order payload.
type goDaddyOrderRequest struct {
	CSR              string `json:"csr"`
	ProductType      string `json:"productType"`
	Period           int    `json:"period"`
	CommonName       string `json:"commonName,omitempty"`
	SubjectAlternativeNames []string `json:"subjectAlternativeNames,omitempty"`
	SlotSize         string `json:"slotSize,omitempty"`
	Contact          *goDaddyContact      `json:"contact,omitempty"`
	Organization     *goDaddyOrganization `json:"organization,omitempty"`
	// RootType selects the chain. STARFIELD_SHA_2 is GoDaddy's modern default.
	RootType string `json:"rootType,omitempty"`
}

type goDaddyContact struct {
	Email     string `json:"email"`
	NameFirst string `json:"nameFirst,omitempty"`
	NameLast  string `json:"nameLast,omitempty"`
	Phone     string `json:"phone,omitempty"`
	JobTitle  string `json:"jobTitle,omitempty"`
}

type goDaddyOrganization struct {
	Name        string                `json:"name"`
	Phone       string                `json:"phone,omitempty"`
	Address     *goDaddyAddress       `json:"address,omitempty"`
	AssumedName string                `json:"assumedName,omitempty"`
	RegistrationAgent string          `json:"registrationAgent,omitempty"`
	RegistrationNumber string         `json:"registrationNumber,omitempty"`
}

type goDaddyAddress struct {
	Address1   string `json:"address1,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postalCode,omitempty"`
	Country    string `json:"country,omitempty"`
}

// goDaddyCertificate is the order/status representation.
type goDaddyCertificate struct {
	CertificateID string `json:"certificateId"`
	Status        string `json:"status"`
	CommonName    string `json:"commonName"`
	SubjectAlternativeNames []string `json:"subjectAlternativeNames"`
	Period        int    `json:"period"`
	ProductType   string `json:"productType"`
	ValidStart    string `json:"validStart"`
	ValidEnd      string `json:"validEnd"`
	SerialNumber  string `json:"serialNumber"`
	Contact       *goDaddyContact `json:"contact,omitempty"`
}

func (g *GoDaddyProvider) Issue(ctx context.Context, req IssueRequest) (*IssueResult, error) {
	if strings.TrimSpace(req.CSRPEM) == "" {
		return nil, fmt.Errorf("GoDaddy signs a CSR the customer supplies, so a CSR is required")
	}
	if len(req.Domains) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}

	certType := NormaliseCertType(req.CertType)
	if certType != "dv" && req.Organization == nil {
		return nil, fmt.Errorf("%s certificates require organisation details, which the CA validates", certType)
	}

	period := req.ValidityYears
	if period <= 0 {
		period = 1
	}
	if period > 2 {
		// GoDaddy sells multi-year as billing terms; a public certificate still
		// cannot exceed 398 days of validity.
		period = 2
	}

	order := goDaddyOrderRequest{
		// GoDaddy expects the CSR without PEM armour on some endpoints and with it
		// on others; the armoured form is accepted and is what the API documents.
		CSR:         strings.TrimSpace(req.CSRPEM),
		ProductType: goDaddyProductFor(certType, HasWildcard(req.Domains), len(req.Domains)),
		Period:      period,
		CommonName:  req.Domains[0],
		RootType:    "STARFIELD_SHA_2",
	}
	if len(req.Domains) > 1 {
		order.SubjectAlternativeNames = req.Domains[1:]
	}
	if req.ContactEmail != "" {
		order.Contact = &goDaddyContact{Email: req.ContactEmail}
	}
	if o := req.Organization; o != nil {
		if order.Contact == nil {
			order.Contact = &goDaddyContact{}
		}
		if order.Contact.Email == "" {
			order.Contact.Email = o.ContactEmail
		}
		order.Contact.NameFirst = o.ContactFirstName
		order.Contact.NameLast = o.ContactLastName
		order.Contact.Phone = o.ContactPhone
		order.Contact.JobTitle = o.JobTitle

		order.Organization = &goDaddyOrganization{
			Name:  o.Name,
			Phone: o.Phone,
			Address: &goDaddyAddress{
				Address1: o.Address, City: o.City,
				State: o.State, Country: o.Country,
			},
		}
	}

	var created goDaddyCertificate
	if err := g.do(ctx, http.MethodPost, "/v1/certificates", order, &created); err != nil {
		return nil, err
	}
	if created.CertificateID == "" {
		return nil, fmt.Errorf("GoDaddy accepted the order but returned no certificateId")
	}

	g.logger.Info("godaddy order created",
		zap.String("certificate_id", created.CertificateID),
		zap.String("product", order.ProductType),
		zap.String("status", created.Status),
		zap.String("env", g.env))

	// A freshly created order is essentially never ISSUED, but check rather than
	// assume: a re-issue of an already-validated domain can complete immediately.
	res := g.resultFor(&created)
	res.OrderID = created.CertificateID

	if res.Status == StatusIssued {
		if chain, err := g.download(ctx, created.CertificateID); err == nil {
			g.fillFromChain(res, chain)
		}
		return res, nil
	}

	if v, err := g.validationFor(ctx, created.CertificateID); err == nil {
		res.Validation = v
	} else {
		g.logger.Warn("could not read GoDaddy validation instructions",
			zap.String("certificate_id", created.CertificateID), zap.Error(err))
		res.Message += " Validation details could not be read; check the GoDaddy dashboard."
	}
	return res, nil
}

func (g *GoDaddyProvider) OrderStatus(ctx context.Context, orderID string) (*IssueResult, error) {
	if strings.TrimSpace(orderID) == "" {
		return nil, fmt.Errorf("an order id is required")
	}

	var cert goDaddyCertificate
	if err := g.do(ctx, http.MethodGet, "/v1/certificates/"+orderID, nil, &cert); err != nil {
		return nil, err
	}

	res := g.resultFor(&cert)
	res.OrderID = orderID

	switch res.Status {
	case StatusIssued:
		chain, err := g.download(ctx, orderID)
		if err != nil {
			// Issued but not yet downloadable is a real transient state; report it
			// as pending so the poller tries again instead of recording a failure.
			res.Status = StatusPending
			res.Message = "GoDaddy reports the order as issued but the chain is not " +
				"downloadable yet: " + err.Error()
			return res, nil
		}
		g.fillFromChain(res, chain)
	case StatusPending:
		if v, err := g.validationFor(ctx, orderID); err == nil {
			res.Validation = v
		}
	}
	return res, nil
}

// resultFor maps GoDaddy's order states onto the shared model.
func (g *GoDaddyProvider) resultFor(c *goDaddyCertificate) *IssueResult {
	res := &IssueResult{Provider: g.Name(), Serial: c.SerialNumber}

	switch strings.ToUpper(strings.TrimSpace(c.Status)) {
	case "ISSUED":
		res.Status = StatusIssued
	case "REVOKED":
		res.Status = StatusRevoked
		res.Message = "the certificate has been revoked at GoDaddy"
	case "CANCELED", "CANCELLED", "DENIED", "EXPIRED", "FAILED":
		res.Status = StatusFailed
		res.Message = "GoDaddy reports the order as " + c.Status
	case "PENDING_ISSUANCE", "PENDING_REKEY", "PENDING_REVOCATION", "PENDING":
		res.Status = StatusPending
		res.Message = "waiting on domain control validation at GoDaddy (" + c.Status + ")"
	default:
		res.Status = StatusPending
		res.Message = "GoDaddy order state: " + c.Status
	}

	if c.ValidStart != "" {
		if t, err := time.Parse(time.RFC3339, c.ValidStart); err == nil {
			res.NotBefore = t
		}
	}
	if c.ValidEnd != "" {
		if t, err := time.Parse(time.RFC3339, c.ValidEnd); err == nil {
			res.NotAfter = t
		}
	}
	return res
}

// fillFromChain takes the authoritative dates and issuer from the certificate
// itself rather than trusting the order metadata.
func (g *GoDaddyProvider) fillFromChain(res *IssueResult, chainPEM string) {
	res.ChainPEM = chainPEM
	leaf, err := ParseChain(chainPEM)
	if err != nil {
		return
	}
	res.NotBefore = leaf.NotBefore
	res.NotAfter = leaf.NotAfter
	res.Issuer = leaf.Issuer.CommonName
	res.Serial = leaf.SerialNumber.Text(16)
}

// download fetches the issued chain.
func (g *GoDaddyProvider) download(ctx context.Context, certificateID string) (string, error) {
	var out struct {
		Pems struct {
			Certificate  string `json:"certificate"`
			Intermediate string `json:"intermediate"`
			Root         string `json:"root"`
			Cross        string `json:"cross"`
		} `json:"pems"`
		SerialNumber string `json:"serialNumber"`
	}
	if err := g.do(ctx, http.MethodGet,
		"/v1/certificates/"+certificateID+"/download", nil, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Pems.Certificate) == "" {
		return "", fmt.Errorf("GoDaddy returned no leaf certificate")
	}

	// Leaf first, then intermediates: the order matters to TLS clients, and the
	// root is omitted because serving it is pointless and wastes handshake bytes.
	var b strings.Builder
	writePEM(&b, out.Pems.Certificate)
	writePEM(&b, out.Pems.Intermediate)
	writePEM(&b, out.Pems.Cross)
	return b.String(), nil
}

func writePEM(b *strings.Builder, pemText string) {
	pemText = strings.TrimSpace(pemText)
	if pemText == "" {
		return
	}
	b.WriteString(pemText)
	if !strings.HasSuffix(pemText, "\n") {
		b.WriteString("\n")
	}
}

// validationFor reads the outstanding domain control challenges.
func (g *GoDaddyProvider) validationFor(ctx context.Context, certificateID string) ([]ValidationInstruction, error) {
	var out struct {
		Domains []struct {
			Domain     string `json:"domain"`
			Status     string `json:"status"`
			DomainInfo struct {
				HTTPURL      string `json:"httpUrl"`
				HTTPFilename string `json:"httpFilename"`
				HTTPBody     string `json:"httpBody"`
				DNSName      string `json:"dnsName"`
				DNSValue     string `json:"dnsValue"`
				Emails       []string `json:"emails"`
			} `json:"domainInfo"`
		} `json:"domains"`
	}
	if err := g.do(ctx, http.MethodGet,
		"/v1/certificates/"+certificateID+"/domainVerifications", nil, &out); err != nil {
		return nil, err
	}

	instructions := []ValidationInstruction{}
	for _, d := range out.Domains {
		if strings.EqualFold(d.Status, "VALIDATED") || strings.EqualFold(d.Status, "COMPLETE") {
			continue
		}
		info := d.DomainInfo
		switch {
		case info.DNSName != "" && info.DNSValue != "":
			instructions = append(instructions, ValidationInstruction{
				Type: DCVDNS, Domain: d.Domain,
				Name: info.DNSName, Value: info.DNSValue,
				Detail: "create this TXT record, then GoDaddy will validate automatically",
			})
		case info.HTTPFilename != "" || info.HTTPURL != "":
			name := info.HTTPURL
			if name == "" {
				name = "/.well-known/pki-validation/" + info.HTTPFilename
			}
			instructions = append(instructions, ValidationInstruction{
				Type: DCVHTTP, Domain: d.Domain,
				Name: name, Value: info.HTTPBody,
				Detail: "serve this file over plain HTTP at the path shown",
			})
		case len(info.Emails) > 0:
			instructions = append(instructions, ValidationInstruction{
				Type: DCVEmail, Domain: d.Domain, Emails: info.Emails,
				Detail: "approve the validation email GoDaddy sends to one of these addresses",
			})
		}
	}
	return instructions, nil
}

func (g *GoDaddyProvider) Revoke(ctx context.Context, req RevokeRequest) error {
	if strings.TrimSpace(req.OrderID) == "" {
		return fmt.Errorf("GoDaddy revokes by certificate id, which is not recorded for this certificate")
	}

	reason := goDaddyReason(req.Reason)
	body := map[string]string{"reason": reason}
	if err := g.do(ctx, http.MethodPost,
		"/v1/certificates/"+req.OrderID+"/revoke", body, nil); err != nil {
		return err
	}
	g.logger.Info("godaddy certificate revoked",
		zap.String("certificate_id", req.OrderID), zap.String("reason", reason))
	return nil
}

// goDaddyReason maps our reason names onto the enum GoDaddy accepts. An
// unrecognised value would fail the whole call, so anything unknown becomes the
// neutral choice.
func goDaddyReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "key_compromise", "keycompromise":
		return "KEY_COMPROMISE"
	case "affiliation_changed":
		return "AFFILIATION_CHANGED"
	case "superseded":
		return "SUPERSEDED"
	case "cessation", "cessation_of_operation":
		return "CESSATION_OF_OPERATION"
	default:
		return "PRIVILEGE_WITHDRAWN"
	}
}

// Environment reports which GoDaddy environment this account talks to.
func (g *GoDaddyProvider) Environment() string { return g.env }
