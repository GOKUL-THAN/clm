package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

// Azure certificate deployment.
//
// Imports the certificate into Key Vault, which is where App Service, Application
// Gateway and Front Door all read from. Key Vault is the right target because it
// is the one place a certificate can be put once and consumed by several
// services; deploying straight to an Application Gateway listener would tie the
// certificate to a single resource.
//
// Authentication is the OAuth2 client credentials flow against Entra ID, built
// with the standard library — no JWT signing needed, unlike GCP, because Azure
// accepts a client secret directly.
type AzureDeployer struct {
	client *http.Client
	logger *zap.Logger
}

// NewAzureDeployer creates the deployer. Credentials arrive per request.
func NewAzureDeployer(logger *zap.Logger) *AzureDeployer {
	return &AzureDeployer{
		client: &http.Client{Timeout: 60 * time.Second},
		logger: logger,
	}
}

func (d *AzureDeployer) Name() string     { return "azure-keyvault" }
func (d *AzureDeployer) Category() string { return "cloud" }

// azureToken performs the client credentials exchange.
//
// The resource scope must match what is being called: Key Vault has its own
// audience (vault.azure.net) rather than the ARM one, and using the wrong scope
// produces a token that authenticates but is refused by the vault.
func (d *AzureDeployer) azureToken(ctx context.Context, tenant, clientID, secret, scope, authHost string) (string, error) {
	if authHost == "" {
		authHost = "https://login.microsoftonline.com"
	}
	endpoint := fmt.Sprintf("%s/%s/oauth2/v2.0/token", strings.TrimSuffix(authHost, "/"), tenant)

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", secret)
	form.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting an Azure access token: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("Azure token endpoint returned HTTP %d: %s",
			resp.StatusCode, azureError(raw))
	}

	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decoding the Azure token response: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("Azure returned no access token: %s %s", out.Error, out.ErrorDesc)
	}
	return out.AccessToken, nil
}

// Deploy imports the certificate into Key Vault.
//
// Required credentials: tenant_id, client_id, client_secret.
// target.VaultName identifies the vault and target.ResourceID the certificate name.
func (d *AzureDeployer) Deploy(ctx context.Context, req *deployers.DeployRequest) (*deployers.DeployResult, error) {
	started := time.Now()
	target := req.Target
	res := &deployers.DeployResult{Target: target}

	creds := target.Credentials
	missing := []string{}
	for _, k := range []string{"tenant_id", "client_id", "client_secret"} {
		if strings.TrimSpace(creds[k]) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		res.Error = "an Azure target requires: " + strings.Join(missing, ", ")
		return res, fmt.Errorf("%s", res.Error)
	}

	vaultName := strings.TrimSpace(target.VaultName)
	if vaultName == "" {
		res.Error = "target.vault_name must identify the Azure Key Vault"
		return res, fmt.Errorf("%s", res.Error)
	}
	certName := strings.TrimSpace(target.ResourceID)
	if certName == "" {
		res.Error = "target.resource_id must give the certificate name to use inside the vault"
		return res, fmt.Errorf("%s", res.Error)
	}
	certName = sanitiseAzureName(certName)

	vaultHost := strings.TrimSpace(creds["vault_url"])
	if vaultHost == "" {
		vaultHost = fmt.Sprintf("https://%s.vault.azure.net", strings.TrimSpace(target.VaultName))
	}
	vaultHost = strings.TrimSuffix(vaultHost, "/")

	// Key Vault's own audience, not the ARM one.
	scope := strings.TrimSpace(creds["scope"])
	if scope == "" {
		scope = "https://vault.azure.net/.default"
	}

	token, err := d.azureToken(ctx, creds["tenant_id"], creds["client_id"],
		creds["client_secret"], scope, creds["auth_host"])
	if err != nil {
		res.Error = err.Error()
		return res, err
	}

	// Key Vault accepts a PEM bundle when the policy content type says so. The key
	// must come first for the vault to associate it with the leaf.
	var bundle bytes.Buffer
	bundle.Write(req.KeyPEM)
	if !bytes.HasSuffix(req.KeyPEM, []byte("\n")) {
		bundle.WriteByte('\n')
	}
	if len(req.FullChainPEM) > 0 {
		bundle.Write(req.FullChainPEM)
	} else {
		bundle.Write(req.CertPEM)
		bundle.Write(req.ChainPEM)
	}

	body := map[string]any{
		"value": base64.StdEncoding.EncodeToString(bundle.Bytes()),
		"policy": map[string]any{
			"secret_props": map[string]any{"contentType": "application/x-pem-file"},
			// Key Vault would otherwise try to renew the certificate itself through
			// its own issuer, which would fight with this gateway's renewal.
			"issuer": map[string]any{"name": "Unknown"},
		},
	}

	apiVersion := strings.TrimSpace(creds["api_version"])
	if apiVersion == "" {
		apiVersion = "7.4"
	}
	importURL := fmt.Sprintf("%s/certificates/%s/import?api-version=%s",
		vaultHost, url.PathEscape(certName), apiVersion)

	var imported struct {
		ID         string `json:"id"`
		Kid        string `json:"kid"`
		Sid        string `json:"sid"`
		Attributes struct {
			Expires int64 `json:"exp"`
		} `json:"attributes"`
	}
	if err := d.callAzure(ctx, token, http.MethodPost, importURL, body, &imported); err != nil {
		res.Error = err.Error()
		res.Duration = time.Since(started)
		return res, err
	}

	res.Success = true
	res.Verified = imported.ID != ""
	res.Message = fmt.Sprintf(
		"certificate imported into Key Vault %s as %q. Any App Service, Application "+
			"Gateway or Front Door binding that reads this vault entry picks it up; a "+
			"binding pinned to a specific version still needs repointing.",
		strings.TrimSpace(target.VaultName), certName)
	res.Duration = time.Since(started)

	d.logger.Info("azure key vault import complete",
		zap.String("vault", target.VaultName), zap.String("certificate", certName))
	return res, nil
}

// callAzure performs an authenticated Key Vault call.
func (d *AzureDeployer) callAzure(ctx context.Context, token, method, url string, body, out any) error {
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
		return fmt.Errorf("calling Azure Key Vault: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("Azure returned HTTP %d: %s", resp.StatusCode, azureError(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding the Azure response: %w", err)
		}
	}
	return nil
}

// azureError pulls the message out of an Azure error body, which nests error
// details one level deeper than most APIs.
func azureError(raw []byte) string {
	var e struct {
		Error struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			InnerError struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"innererror"`
		} `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return clip(string(raw), 260)
	}
	parts := []string{}
	if e.Error.Code != "" {
		parts = append(parts, e.Error.Code)
	}
	if e.Error.Message != "" {
		parts = append(parts, e.Error.Message)
	}
	if e.Error.InnerError.Message != "" {
		parts = append(parts, e.Error.InnerError.Message)
	}
	if e.ErrorDescription != "" {
		parts = append(parts, clip(e.ErrorDescription, 200))
	}
	if len(parts) == 0 {
		return clip(string(raw), 260)
	}
	return strings.Join(parts, " | ")
}

// sanitiseAzureName forces a value into Key Vault's certificate name rules:
// alphanumerics and hyphens only.
func sanitiseAzureName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if out == "" {
		out = "clm-certificate"
	}
	if len(out) > 127 {
		out = strings.Trim(out[:127], "-")
	}
	return out
}

// Verify reads the certificate back out of the vault and returns explicit
// Key Vault import evidence. This is storage evidence, not serving evidence.
func (d *AzureDeployer) Verify(ctx context.Context, req *deployers.DeployRequest, applied *deployers.DeployResult) (*deployers.VerificationEvidence, error) {
	target := req.Target
	evidence := &deployers.VerificationEvidence{
		Kind: "adapter", Method: "azure-key-vault-read", Adapter: d.Name(),
		Resource: target.ResourceID, CheckedAt: time.Now().UTC(),
	}
	fail := func(err error) (*deployers.VerificationEvidence, error) {
		evidence.Message = err.Error()
		return evidence, err
	}
	creds := target.Credentials
	for _, k := range []string{"tenant_id", "client_id", "client_secret"} {
		if strings.TrimSpace(creds[k]) == "" {
			return fail(fmt.Errorf("%s is required to verify an Azure target", k))
		}
	}
	if strings.TrimSpace(target.VaultName) == "" {
		return fail(fmt.Errorf("target.vault_name must identify the Azure Key Vault"))
	}
	if target.ResourceID == "" {
		return fail(fmt.Errorf("target.resource_id must give the certificate name"))
	}

	vaultHost := strings.TrimSpace(creds["vault_url"])
	if vaultHost == "" {
		vaultHost = fmt.Sprintf("https://%s.vault.azure.net", strings.TrimSpace(target.VaultName))
	}
	scope := strings.TrimSpace(creds["scope"])
	if scope == "" {
		scope = "https://vault.azure.net/.default"
	}

	token, err := d.azureToken(ctx, creds["tenant_id"], creds["client_id"],
		creds["client_secret"], scope, creds["auth_host"])
	if err != nil {
		return fail(err)
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := d.callAzure(ctx, token, http.MethodGet,
		fmt.Sprintf("%s/certificates/%s?api-version=7.4",
			strings.TrimSuffix(vaultHost, "/"), url.PathEscape(sanitiseAzureName(target.ResourceID))),
		nil, &out); err != nil {
		return fail(err)
	}
	if out.ID == "" {
		return fail(fmt.Errorf("Azure Key Vault returned no certificate identifier"))
	}
	evidence.Resource = out.ID
	evidence.Success = true
	evidence.Message = "Azure Key Vault returned the imported certificate; no serving endpoint was requested"
	return evidence, nil
}

// Rollback is not implemented. Key Vault keeps every version, so reverting means
// repointing consumers at an earlier version — which the gateway does not track.
func (d *AzureDeployer) Rollback(ctx context.Context, target *deployers.DeployTarget) error {
	return fmt.Errorf("rollback is not implemented for Azure: Key Vault retains previous " +
		"versions, so repoint the consuming service at the earlier certificate version")
}
