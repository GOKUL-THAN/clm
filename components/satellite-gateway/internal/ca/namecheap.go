package ca

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Namecheap SSL API.
//
// This is a deliberately different shape from GoDaddy and it is the reason the
// Provider interface earns its keep:
//
//   - Every call is a GET to one endpoint with a Command parameter, not a REST
//     resource path.
//   - Credentials travel as query parameters, not headers.
//   - Responses are XML, and a failure arrives as HTTP 200 with
//     Status="ERROR" in the body — so checking the status code alone would treat
//     every error as a success.
//   - Purchase and issuance are two separate steps: ssl.create buys a credit,
//     ssl.activate then submits the CSR against it.
//   - The account's calling IP must be allow-listed in the Namecheap dashboard,
//     which is a common first-time failure and worth reporting clearly.
type NamecheapProvider struct {
	apiUser  string
	apiKey   string
	userName string
	clientIP string
	baseURL  string
	env      string
	client   *http.Client
	logger   *zap.Logger
}

// NamecheapConfig configures the provider.
type NamecheapConfig struct {
	APIUser string
	APIKey  string
	// UserName is usually the same as APIUser; it differs when acting for a
	// sub-account.
	UserName string
	// ClientIP must be an address allow-listed on the Namecheap account. The API
	// rejects the call outright when it does not match.
	ClientIP string
	Sandbox  bool
	BaseURL  string // overrides the endpoint, for testing
}

const (
	namecheapProduction = "https://api.namecheap.com/xml.response"
	namecheapSandbox    = "https://api.sandbox.namecheap.com/xml.response"
)

// NewNamecheapProvider builds a Namecheap provider.
func NewNamecheapProvider(cfg NamecheapConfig, logger *zap.Logger) (*NamecheapProvider, error) {
	if strings.TrimSpace(cfg.APIUser) == "" || strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("a Namecheap ApiUser and ApiKey are required")
	}
	if strings.TrimSpace(cfg.ClientIP) == "" {
		return nil, fmt.Errorf("a Namecheap ClientIp is required and must be allow-listed " +
			"on the account, otherwise every call is rejected")
	}

	user := cfg.UserName
	if strings.TrimSpace(user) == "" {
		user = cfg.APIUser
	}

	base := strings.TrimSpace(cfg.BaseURL)
	env := "production"
	if base == "" {
		if cfg.Sandbox {
			base = namecheapSandbox
			env = "sandbox"
		} else {
			base = namecheapProduction
		}
	} else {
		env = "custom"
	}

	return &NamecheapProvider{
		apiUser: cfg.APIUser, apiKey: cfg.APIKey,
		userName: user, clientIP: cfg.ClientIP,
		baseURL: base, env: env,
		client:  &http.Client{Timeout: 45 * time.Second},
		logger:  logger,
	}, nil
}

func (n *NamecheapProvider) Name() string { return "namecheap" }

func (n *NamecheapProvider) Capabilities() Capabilities {
	return Capabilities{
		Synchronous: false,
		CertTypes:   []string{"dv", "ov", "ev"},
		Wildcard:    true,
		MultiSAN:    true,
		DCVMethods:  []string{DCVHTTP, DCVDNS, DCVEmail},
		RequiresCSR: true,
		Revocation:  true,
		Quoting:     true,
		MaxValidityYears: 1,
		Notes: "Purchase and issuance are separate calls (ssl.create then " +
			"ssl.activate). The calling IP must be allow-listed on the account.",
	}
}

// namecheapResponse is the envelope every command returns.
type namecheapResponse struct {
	XMLName xml.Name `xml:"ApiResponse"`
	Status  string   `xml:"Status,attr"`
	Errors  []struct {
		Number string `xml:"Number,attr"`
		Text   string `xml:",chardata"`
	} `xml:"Errors>Error"`
	Warnings []struct {
		Number string `xml:"Number,attr"`
		Text   string `xml:",chardata"`
	} `xml:"Warnings>Warning"`
	CommandResponse struct {
		Type string `xml:"Type,attr"`
		Raw  []byte `xml:",innerxml"`
	} `xml:"CommandResponse"`
}

// call performs a command and returns the CommandResponse inner XML.
func (n *NamecheapProvider) call(ctx context.Context, command string, params map[string]string) ([]byte, error) {
	q := url.Values{}
	q.Set("ApiUser", n.apiUser)
	q.Set("ApiKey", n.apiKey)
	q.Set("UserName", n.userName)
	q.Set("ClientIp", n.clientIP)
	q.Set("Command", command)
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}

	// POST rather than GET: a CSR is far too long for a URL, and the credentials
	// stay out of any intermediary's access log this way.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL,
		strings.NewReader(q.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Namecheap: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("reading Namecheap response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("Namecheap returned HTTP %d: %s",
			resp.StatusCode, truncate(string(raw), 240))
	}

	var env namecheapResponse
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decoding Namecheap XML: %w (body: %s)", err, truncate(string(raw), 200))
	}

	// A Namecheap failure is HTTP 200 with Status="ERROR". Trusting the status
	// code would silently treat every rejection as success.
	if !strings.EqualFold(env.Status, "OK") {
		msgs := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			text := strings.TrimSpace(e.Text)
			// 1011150 is the IP-not-allow-listed error, by far the most common
			// first-time failure, so name the fix.
			if e.Number == "1011150" {
				text += " — add " + n.clientIP + " to the API access allow-list on the Namecheap account"
			}
			msgs = append(msgs, e.Number+": "+text)
		}
		if len(msgs) == 0 {
			msgs = append(msgs, "no error detail returned")
		}
		return nil, fmt.Errorf("Namecheap %s failed: %s", command, strings.Join(msgs, "; "))
	}

	for _, w := range env.Warnings {
		n.logger.Warn("namecheap warning",
			zap.String("command", command), zap.String("number", w.Number),
			zap.String("text", strings.TrimSpace(w.Text)))
	}

	return env.CommandResponse.Raw, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// Quote lists the SSL products available on the account.
func (n *NamecheapProvider) Quote(ctx context.Context, req QuoteRequest) ([]Quote, error) {
	raw, err := n.call(ctx, "namecheap.users.getPricing", map[string]string{
		"ProductType": "SSLCERTIFICATE",
	})
	if err != nil {
		return nil, err
	}

	// Each level is its own struct. encoding/xml cannot combine a nested path with
	// an attribute in one tag (`a>b,attr` is invalid and makes Unmarshal fail for
	// the whole document), so the nesting has to be spelled out.
	type ncPrice struct {
		Duration     string `xml:"Duration,attr"`
		DurationType string `xml:"DurationType,attr"`
		Price        string `xml:"Price,attr"`
		Currency     string `xml:"Currency,attr"`
	}
	type ncProduct struct {
		Name   string    `xml:"Name,attr"`
		Prices []ncPrice `xml:"Price"`
	}
	type ncCategory struct {
		Name     string      `xml:"Name,attr"`
		Products []ncProduct `xml:"Product"`
	}
	type ncProductType struct {
		Name       string       `xml:"Name,attr"`
		Categories []ncCategory `xml:"ProductCategory"`
	}
	var out struct {
		Result struct {
			ProductTypes []ncProductType `xml:"ProductType"`
		} `xml:"UserGetPricingResult"`
	}
	if err := xml.Unmarshal(wrap(raw), &out); err != nil {
		return nil, fmt.Errorf("decoding Namecheap pricing: %w", err)
	}

	wantType := NormaliseCertType(req.CertType)
	wantWildcard := HasWildcard(req.Domains)

	quotes := []Quote{}
	for _, pt := range out.Result.ProductTypes {
		for _, cat := range pt.Categories {
			for _, prod := range cat.Products {
				lower := strings.ToLower(prod.Name)
				isWildcard := strings.Contains(lower, "wildcard")
				// Only filter when the caller expressed a preference; a mismatch
				// otherwise hides products that would in fact do the job.
				if wantWildcard && !isWildcard {
					continue
				}
				productType := namecheapProductType(prod.Name)
				if wantType != "" && productType != "" && wantType != productType {
					continue
				}

				for _, price := range prod.Prices {
					years, _ := strconv.Atoi(price.Duration)
					if req.ValidityYears > 0 && years > 0 && years != req.ValidityYears {
						continue
					}
					amount, _ := strconv.ParseFloat(price.Price, 64)
					currency := price.Currency
					if currency == "" {
						currency = "USD"
					}
					quotes = append(quotes, Quote{
						Provider: n.Name(), ProductName: prod.Name, ProductID: prod.Name,
						Price: amount, Currency: currency,
						CertType: nonEmptyStr(productType, "dv"),
						Validity: price.Duration + " " + strings.ToLower(price.DurationType),
						Wildcard: isWildcard,
						IssuanceTime: namecheapIssuanceTime(productType),
					})
				}
			}
		}
	}
	return quotes, nil
}

// wrap makes an inner-XML fragment parseable as a document.
func wrap(inner []byte) []byte {
	return append(append([]byte("<r>"), inner...), []byte("</r>")...)
}

func nonEmptyStr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// namecheapProductType infers dv/ov/ev from a product name, because the pricing
// response does not label validation level directly.
func namecheapProductType(name string) string {
	l := strings.ToLower(name)
	switch {
	case strings.Contains(l, "ev "), strings.Contains(l, "extended"):
		return "ev"
	case strings.Contains(l, "instantssl"), strings.Contains(l, "premiumssl"),
		strings.Contains(l, "organization"), strings.Contains(l, "ov "):
		return "ov"
	case strings.Contains(l, "positivessl"), strings.Contains(l, "essentialssl"),
		strings.Contains(l, "domain"), strings.Contains(l, "dv "):
		return "dv"
	}
	return ""
}

func namecheapIssuanceTime(certType string) string {
	switch certType {
	case "ev":
		return "1-5 business days (organisation vetting)"
	case "ov":
		return "1-3 business days (organisation vetting)"
	default:
		return "minutes to hours once validation completes"
	}
}

// Issue buys a certificate credit and then activates it with the CSR.
//
// These are two distinct Namecheap commands, and the split matters: ssl.create
// spends money, so if ssl.activate then fails the credit still exists on the
// account. The returned order ID is the CertificateID from ssl.create, which is
// what makes a retry possible without buying a second credit.
func (n *NamecheapProvider) Issue(ctx context.Context, req IssueRequest) (*IssueResult, error) {
	if strings.TrimSpace(req.CSRPEM) == "" {
		return nil, fmt.Errorf("Namecheap signs a CSR the customer supplies, so a CSR is required")
	}
	if len(req.Domains) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}

	certType := NormaliseCertType(req.CertType)
	if certType != "dv" && req.Organization == nil {
		return nil, fmt.Errorf("%s certificates require organisation details, which the CA validates", certType)
	}

	years := req.ValidityYears
	if years <= 0 {
		years = 1
	}

	// Step 1: buy the credit.
	product := namecheapProductName(certType, HasWildcard(req.Domains), len(req.Domains))
	createRaw, err := n.call(ctx, "namecheap.ssl.create", map[string]string{
		"Type":  product,
		"Years": strconv.Itoa(years),
	})
	if err != nil {
		return nil, fmt.Errorf("purchasing the certificate credit: %w", err)
	}

	var created struct {
		Result struct {
			IsSuccess   string `xml:"IsSuccess,attr"`
			Certificate struct {
				CertificateID string `xml:"CertificateID,attr"`
				SSLType       string `xml:"SSLType,attr"`
				Years         string `xml:"Years,attr"`
				Status        string `xml:"Status,attr"`
			} `xml:"SSLCertificate"`
		} `xml:"SSLCreateResult"`
	}
	if err := xml.Unmarshal(wrap(createRaw), &created); err != nil {
		return nil, fmt.Errorf("decoding the Namecheap purchase response: %w", err)
	}
	certID := created.Result.Certificate.CertificateID
	if certID == "" {
		return nil, fmt.Errorf("Namecheap accepted the purchase but returned no CertificateID")
	}

	n.logger.Info("namecheap credit purchased",
		zap.String("certificate_id", certID), zap.String("product", product),
		zap.String("env", n.env))

	res := &IssueResult{
		Provider: n.Name(), OrderID: certID, Status: StatusPending,
	}

	// Step 2: activate it with the CSR.
	dcv := req.DCVMethod
	if dcv == "" {
		dcv = DCVDNS
	}
	activateParams := map[string]string{
		"CertificateID":  certID,
		"CSR":            strings.TrimSpace(req.CSRPEM),
		"AdminEmailAddress": req.ContactEmail,
		"WebServerType":  "nginx",
		"DNSDCVMethod":   namecheapDCV(dcv),
	}
	if o := req.Organization; o != nil {
		activateParams["AdminOrganizationName"] = o.Name
		activateParams["AdminCity"] = o.City
		activateParams["AdminStateProvince"] = o.State
		activateParams["AdminCountry"] = o.Country
		activateParams["AdminAddress1"] = o.Address
		activateParams["AdminPhone"] = o.Phone
		activateParams["AdminJobTitle"] = o.JobTitle
		activateParams["AdminFirstName"] = o.ContactFirstName
		activateParams["AdminLastName"] = o.ContactLastName
	}

	activateRaw, err := n.call(ctx, "namecheap.ssl.activate", activateParams)
	if err != nil {
		// The credit is bought and still usable, so say so — otherwise an operator
		// retries and pays twice.
		res.Message = fmt.Sprintf(
			"the certificate credit was purchased (id %s) but activation failed: %v. "+
				"Retry activation against this id rather than ordering again.", certID, err)
		return res, nil
	}

	var activated struct {
		Result struct {
			ID        string `xml:"ID,attr"`
			IsSuccess string `xml:"IsSuccess,attr"`
			HTTPDCV   struct {
				FileName    string `xml:"FileName"`
				FileContent string `xml:"FileContent"`
			} `xml:"HttpDCValidation"`
			DNSDCV struct {
				HostName string `xml:"HostName"`
				Target   string `xml:"Target"`
			} `xml:"DNSDCValidation"`
		} `xml:"SSLActivateResult"`
	}
	// The decode error is reported rather than ignored. Swallowing it hid an
	// invalid struct tag once already: Unmarshal failed for the whole document and
	// every field came back empty, which looked exactly like a CA that had
	// returned no validation instructions.
	if err := xml.Unmarshal(wrap(activateRaw), &activated); err != nil {
		res.Message = "activated at Namecheap, but the activation response could not " +
			"be decoded: " + err.Error()
		return res, nil
	}

	r := activated.Result
	res.Validation = namecheapValidation(req.Domains[0],
		r.HTTPDCV.FileName, r.HTTPDCV.FileContent, r.DNSDCV.HostName, r.DNSDCV.Target)
	res.Message = "activated at Namecheap; waiting on domain control validation"

	if len(res.Validation) == 0 {
		res.Message += ". No validation instructions were returned — check the " +
			"Namecheap dashboard, which may be using email validation"
	}
	return res, nil
}

// namecheapValidation builds the instruction list from an activate response.
func namecheapValidation(domain, httpFile, httpBody, dnsHost, dnsTarget string) []ValidationInstruction {
	out := []ValidationInstruction{}
	if strings.TrimSpace(dnsHost) != "" && strings.TrimSpace(dnsTarget) != "" {
		out = append(out, ValidationInstruction{
			Type: DCVDNS, Domain: domain,
			Name: dnsHost, Value: dnsTarget,
			Detail: "create this CNAME record; Namecheap validates automatically once it resolves",
		})
	}
	if strings.TrimSpace(httpFile) != "" {
		out = append(out, ValidationInstruction{
			Type: DCVHTTP, Domain: domain,
			Name:   "/.well-known/pki-validation/" + httpFile,
			Value:  httpBody,
			Detail: "serve this file over plain HTTP at the path shown",
		})
	}
	return out
}

// namecheapProductName maps a cert type onto a Namecheap product.
func namecheapProductName(certType string, wildcard bool, sans int) string {
	switch certType {
	case "ev":
		if sans > 1 {
			return "EV Multi Domain SSL"
		}
		return "EV SSL"
	case "ov":
		if wildcard {
			return "PremiumSSL Wildcard"
		}
		if sans > 1 {
			return "Multi Domain SSL"
		}
		return "InstantSSL"
	default:
		if wildcard {
			return "PositiveSSL Wildcard"
		}
		if sans > 1 {
			return "PositiveSSL Multi Domain"
		}
		return "PositiveSSL"
	}
}

// namecheapDCV maps our method names onto Namecheap's.
func namecheapDCV(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case DCVHTTP:
		return "HTTPDCV"
	case DCVEmail:
		return "EMAIL"
	default:
		return "CNAME_CSR_HASH"
	}
}

func (n *NamecheapProvider) OrderStatus(ctx context.Context, orderID string) (*IssueResult, error) {
	if strings.TrimSpace(orderID) == "" {
		return nil, fmt.Errorf("an order id is required")
	}

	raw, err := n.call(ctx, "namecheap.ssl.getInfo", map[string]string{
		"CertificateID": orderID,
		// Ask for the certificate itself so an issued order can be collected in
		// the same call rather than needing a second round trip.
		"Returncertificate": "true",
		"Returntype":        "individual",
	})
	if err != nil {
		return nil, err
	}

	var out struct {
		Result struct {
			Status     string `xml:"Status,attr"`
			Type       string `xml:"Type,attr"`
			CommonName string `xml:"CommonName,attr"`
			Expires    string `xml:"Expires,attr"`
			Details    struct {
				Certificates struct {
					Certificate string `xml:"Certificate"`
					CACerts     struct {
						Certificates []struct {
							Type string `xml:"Type,attr"`
							Body string `xml:",chardata"`
						} `xml:"Certificate"`
					} `xml:"CaCertificates"`
				} `xml:"Certificates"`
			} `xml:"CertificateDetails"`
		} `xml:"SSLGetInfoResult"`
	}
	if err := xml.Unmarshal(wrap(raw), &out); err != nil {
		return nil, fmt.Errorf("decoding the Namecheap order status: %w", err)
	}
	info := out.Result

	res := &IssueResult{Provider: n.Name(), OrderID: orderID}

	switch strings.ToLower(strings.TrimSpace(info.Status)) {
	case "active":
		res.Status = StatusIssued
	case "revoked":
		res.Status = StatusRevoked
		res.Message = "the certificate has been revoked at Namecheap"
	case "cancelled", "canceled", "expired", "rejected":
		res.Status = StatusFailed
		res.Message = "Namecheap reports the order as " + info.Status
	default:
		res.Status = StatusPending
		res.Message = "Namecheap order state: " + nonEmptyStr(info.Status, "unknown")
	}

	leafPEM := info.Details.Certificates.Certificate
	if res.Status == StatusIssued && strings.TrimSpace(leafPEM) != "" {
		var b strings.Builder
		writePEM(&b, ensurePEMArmour(leafPEM, "CERTIFICATE"))
		for _, inter := range info.Details.Certificates.CACerts.Certificates {
			writePEM(&b, ensurePEMArmour(inter.Body, "CERTIFICATE"))
		}
		res.ChainPEM = b.String()

		if leaf, err := ParseChain(res.ChainPEM); err == nil {
			res.NotBefore = leaf.NotBefore
			res.NotAfter = leaf.NotAfter
			res.Issuer = leaf.Issuer.CommonName
			res.Serial = leaf.SerialNumber.Text(16)
		}
	}
	return res, nil
}

// ensurePEMArmour adds BEGIN/END lines when the API returned a bare base64 body.
// Namecheap is inconsistent about this between products, and a chain without
// armour will not parse.
func ensurePEMArmour(body, kind string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if strings.Contains(body, "-----BEGIN") {
		return body
	}
	return "-----BEGIN " + kind + "-----\n" + body + "\n-----END " + kind + "-----"
}

func (n *NamecheapProvider) Revoke(ctx context.Context, req RevokeRequest) error {
	if strings.TrimSpace(req.OrderID) == "" {
		return fmt.Errorf("Namecheap revokes by CertificateID, which is not recorded for this certificate")
	}
	// The API requires the certificate type alongside the id.
	if _, err := n.call(ctx, "namecheap.ssl.revokecertificate", map[string]string{
		"CertificateID": req.OrderID,
		"CertificateType": "PositiveSSL",
	}); err != nil {
		return err
	}
	n.logger.Info("namecheap certificate revoked", zap.String("certificate_id", req.OrderID))
	return nil
}

// Environment reports which Namecheap environment this account talks to.
func (n *NamecheapProvider) Environment() string { return n.env }
