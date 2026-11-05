package cloud

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

// GCP certificate deployment.
//
// Deploys to Compute Engine SSL certificates, which is what an HTTPS load
// balancer serves from. A new certificate resource is inserted and the target
// HTTPS proxy is repointed at it, because a Compute SSL certificate is immutable:
// there is no update, so rotation is always create-then-swap. The old resource is
// deliberately left in place — deleting it while the proxy still references it
// breaks the listener, and a stale unused certificate costs nothing.
//
// Authentication is a service account JWT exchanged for an access token, built
// with the standard library. Pulling in the Google SDK for two REST calls would
// add a large dependency tree to a binary that is otherwise stdlib plus AWS.
type GCPDeployer struct {
	client *http.Client
	logger *zap.Logger
}

// NewGCPDeployer creates the deployer. Credentials arrive per request, so one
// instance serves every project.
func NewGCPDeployer(logger *zap.Logger) *GCPDeployer {
	return &GCPDeployer{
		client: &http.Client{Timeout: 60 * time.Second},
		logger: logger,
	}
}

func (d *GCPDeployer) Name() string     { return "gcp-lb" }
func (d *GCPDeployer) Category() string { return "cloud" }

// serviceAccount is the part of a GCP service account JSON key that matters here.
type serviceAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

// gcpToken exchanges a service account key for an OAuth2 access token using the
// JWT bearer flow.
func (d *GCPDeployer) gcpToken(ctx context.Context, sa *serviceAccount, scope string) (string, error) {
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("the service account private_key is not valid PEM")
	}

	var signer crypto.Signer
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		signer = k
	} else if any, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := any.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("the service account key is not RSA, which the JWT flow requires")
		}
		signer = rsaKey
	} else {
		return "", fmt.Errorf("parsing the service account private key: %w", err)
	}

	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token" // #nosec G101 -- fixed public OAuth token endpoint.
	}

	now := time.Now()
	header := base64URL(`{"alg":"RS256","typ":"JWT"}`)
	claims := base64URL(fmt.Sprintf(
		`{"iss":%q,"scope":%q,"aud":%q,"iat":%d,"exp":%d}`,
		sa.ClientEmail, scope, tokenURI, now.Unix(), now.Add(30*time.Minute).Unix()))

	signingInput := header + "." + claims
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return "", fmt.Errorf("signing the assertion: %w", err)
	}
	assertion := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting a GCP access token: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("GCP token endpoint returned HTTP %d: %s",
			resp.StatusCode, clip(string(raw), 240))
	}

	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decoding the GCP token response: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("GCP returned no access token: %s %s", out.Error, out.ErrorDesc)
	}
	return out.AccessToken, nil
}

func base64URL(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// Deploy inserts a new SSL certificate and repoints the target proxy at it.
//
// Required credentials: service_account_json. Optional: project_id (defaults to
// the one in the key). target.ResourceID names the target HTTPS proxy to update;
// without it the certificate is created but nothing serves it, which is reported
// rather than passed off as success.
func (d *GCPDeployer) Deploy(ctx context.Context, req *deployers.DeployRequest) (*deployers.DeployResult, error) {
	started := time.Now()
	target := req.Target
	res := &deployers.DeployResult{Target: target}

	saJSON := target.Credentials["service_account_json"]
	if strings.TrimSpace(saJSON) == "" {
		res.Error = "service_account_json is required for a GCP target"
		return res, fmt.Errorf("%s", res.Error)
	}
	var sa serviceAccount
	if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
		res.Error = "service_account_json is not valid JSON: " + err.Error()
		return res, fmt.Errorf("%s", res.Error)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		res.Error = "service_account_json is missing client_email or private_key"
		return res, fmt.Errorf("%s", res.Error)
	}

	project := strings.TrimSpace(target.Credentials["project_id"])
	if project == "" {
		project = sa.ProjectID
	}
	if project == "" {
		res.Error = "no project_id: supply it or use a service account key that carries one"
		return res, fmt.Errorf("%s", res.Error)
	}

	token, err := d.gcpToken(ctx, &sa, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		res.Error = err.Error()
		return res, err
	}

	base := strings.TrimSuffix(strings.TrimSpace(target.Credentials["base_url"]), "/")
	if base == "" {
		base = "https://compute.googleapis.com"
	}

	// A Compute SSL certificate is immutable, so the name has to be unique per
	// rotation. The timestamp suffix is what makes create-then-swap possible.
	certName := strings.TrimSpace(target.Credentials["cert_name"])
	if certName == "" {
		certName = "clm"
	}
	resourceName := fmt.Sprintf("%s-%s", sanitiseGCPName(certName), time.Now().UTC().Format("20060102-150405"))

	chain := req.FullChainPEM
	if len(chain) == 0 {
		chain = append(append([]byte{}, req.CertPEM...), req.ChainPEM...)
	}

	body := map[string]any{
		"name":        resourceName,
		"certificate": string(chain),
		"privateKey":  string(req.KeyPEM),
		"description": "uploaded by CLM",
	}
	insertURL := fmt.Sprintf("%s/compute/v1/projects/%s/global/sslCertificates", base, project)

	var inserted struct {
		Name       string `json:"name"`
		SelfLink   string `json:"selfLink"`
		TargetLink string `json:"targetLink"`
	}
	if err := d.callGCP(ctx, token, http.MethodPost, insertURL, body, &inserted); err != nil {
		res.Error = err.Error()
		return res, err
	}

	certLink := inserted.TargetLink
	if certLink == "" {
		certLink = fmt.Sprintf("%s/compute/v1/projects/%s/global/sslCertificates/%s",
			base, project, resourceName)
	}
	res.AdapterResource = certLink

	d.logger.Info("gcp ssl certificate created",
		zap.String("project", project), zap.String("name", resourceName))

	proxy := strings.TrimSpace(target.ResourceID)
	if proxy == "" {
		res.Success = true
		res.Message = fmt.Sprintf(
			"SSL certificate %s created in project %s, but no target HTTPS proxy was "+
				"named (target.resource_id), so nothing is serving it yet",
			resourceName, project)
		res.Duration = time.Since(started)
		return res, nil
	}

	swapURL := fmt.Sprintf("%s/compute/v1/projects/%s/targetHttpsProxies/%s/setSslCertificates",
		base, project, proxy)
	if err := d.callGCP(ctx, token, http.MethodPost, swapURL,
		map[string]any{"sslCertificates": []string{certLink}}, nil); err != nil {
		// The certificate exists but is not in service. Saying so matters: a retry
		// should not create a second one before the cause is understood.
		res.Error = fmt.Sprintf(
			"SSL certificate %s was created but attaching it to proxy %s failed: %v",
			resourceName, proxy, err)
		res.Duration = time.Since(started)
		return res, fmt.Errorf("%s", res.Error)
	}

	res.Success = true
	res.Verified = true
	res.Message = fmt.Sprintf("SSL certificate %s created and attached to target HTTPS proxy %s. "+
		"The previous certificate resource was left in place, since deleting one still "+
		"referenced by a proxy breaks the listener.", resourceName, proxy)
	res.Duration = time.Since(started)

	d.logger.Info("gcp deployment complete",
		zap.String("project", project), zap.String("proxy", proxy),
		zap.String("certificate", resourceName))
	return res, nil
}

// callGCP performs an authenticated Compute API call.
func (d *GCPDeployer) callGCP(ctx context.Context, token, method, url string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("calling the GCP Compute API: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("GCP returned HTTP %d: %s", resp.StatusCode, gcpError(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding the GCP response: %w", err)
		}
	}
	return nil
}

// gcpError extracts the message from a Google API error body.
func gcpError(raw []byte) string {
	var e struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Errors  []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &e); err != nil || e.Error.Message == "" {
		return clip(string(raw), 260)
	}
	parts := []string{e.Error.Message}
	for _, sub := range e.Error.Errors {
		if sub.Reason != "" {
			parts = append(parts, sub.Reason)
		}
	}
	return strings.Join(parts, " | ")
}

// sanitiseGCPName forces a value into the shape Compute resource names require:
// lowercase letters, digits and hyphens, starting with a letter.
func sanitiseGCPName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '.' || r == '_' || r == ' ' || r == '*':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" || (out[0] >= '0' && out[0] <= '9') {
		out = "clm-" + out
	}
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	return out
}

// Verify returns explicit Compute API evidence. For certificate-only targets it
// reads back the exact resource created by this attempt; for proxy targets it
// also confirms that exact resource is attached. Endpoint serving is checked
// independently by the executor when a proxy is named.
func (d *GCPDeployer) Verify(ctx context.Context, req *deployers.DeployRequest, applied *deployers.DeployResult) (*deployers.VerificationEvidence, error) {
	target := req.Target
	evidence := &deployers.VerificationEvidence{
		Kind: "adapter", Method: "gcp-compute-ssl-certificate-read", Adapter: d.Name(),
		CheckedAt: time.Now().UTC(),
	}
	fail := func(err error) (*deployers.VerificationEvidence, error) {
		evidence.Message = err.Error()
		return evidence, err
	}
	saJSON := target.Credentials["service_account_json"]
	if strings.TrimSpace(saJSON) == "" {
		return fail(fmt.Errorf("service_account_json is required to verify a GCP target"))
	}
	var sa serviceAccount
	if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
		return fail(fmt.Errorf("service_account_json is not valid JSON: %w", err))
	}
	project := strings.TrimSpace(target.Credentials["project_id"])
	if project == "" {
		project = sa.ProjectID
	}
	if applied == nil || strings.TrimSpace(applied.AdapterResource) == "" {
		return fail(fmt.Errorf("the GCP deployment returned no sslCertificate resource to verify"))
	}
	certLink := applied.AdapterResource
	evidence.Resource = certLink

	token, err := d.gcpToken(ctx, &sa, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return fail(err)
	}

	base := strings.TrimSuffix(strings.TrimSpace(target.Credentials["base_url"]), "/")
	if base == "" {
		base = "https://compute.googleapis.com"
	}
	certURL := certLink
	if strings.HasPrefix(certURL, "/") {
		certURL = base + certURL
	}
	var certResource struct {
		Name string `json:"name"`
	}
	if err := d.callGCP(ctx, token, http.MethodGet, certURL, nil, &certResource); err != nil {
		return fail(err)
	}
	if certResource.Name == "" {
		return fail(fmt.Errorf("GCP returned no sslCertificate name"))
	}

	if strings.TrimSpace(target.ResourceID) == "" {
		evidence.Success = true
		evidence.Message = "GCP returned the exact Compute SSL certificate resource; no target proxy was requested"
		return evidence, nil
	}

	evidence.Method = "gcp-compute-target-proxy-read"
	var proxy struct {
		SSLCertificates []string `json:"sslCertificates"`
	}
	if err := d.callGCP(ctx, token, http.MethodGet,
		fmt.Sprintf("%s/compute/v1/projects/%s/global/targetHttpsProxies/%s", base, project, target.ResourceID),
		nil, &proxy); err != nil {
		return fail(err)
	}
	attached := false
	for _, resource := range proxy.SSLCertificates {
		if resource == certLink || strings.HasSuffix(resource, "/"+certResource.Name) {
			attached = true
			break
		}
	}
	if !attached {
		return fail(fmt.Errorf("GCP target HTTPS proxy does not reference the certificate created by this deployment"))
	}
	evidence.Success = true
	evidence.Message = "GCP target HTTPS proxy references the certificate created by this attempt; serving state checked separately"
	return evidence, nil
}

// Rollback is not implemented: reverting means repointing the proxy at the
// previous certificate resource, and the gateway does not record which one that
// was. Returning an error is honest; silently doing nothing would not be.
func (d *GCPDeployer) Rollback(ctx context.Context, target *deployers.DeployTarget) error {
	return fmt.Errorf("rollback is not implemented for GCP: repoint the target HTTPS " +
		"proxy at the previous sslCertificate resource, which is still present")
}
