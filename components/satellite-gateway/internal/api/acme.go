package api

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/m2p/clm/internal/satellite/audit"
	"github.com/m2p/clm/internal/satellite/ca"
	"github.com/m2p/clm/internal/satellite/dns01"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme"
)

// Automated certificate issuance, renewal and revocation over ACME.
//
// Before this, POST /api/v1/certificates recorded a row with status "pending" and
// nothing ever issued anything — the gateway was an inventory, not a lifecycle
// manager. This closes that gap using golang.org/x/crypto/acme, which the module
// already carries, so no new dependency was needed.
//
// Design notes:
//
//   - HTTP-01 remains the automatic default for ordinary names. DNS-01 is used
//     for wildcards and can be selected for every SAN through a provider-neutral
//     contract. DNS credentials are external references; they never enter the
//     certificate inventory or an API response.
//   - Private keys never enter the inventory, the JSON snapshots or any API
//     response. They are written to <state.dir>/keys/<id>.key with mode 0600 and
//     referenced by path. A key that appears in a snapshot or a log is a key that
//     has leaked.
//   - The default directory is Let's Encrypt **staging**. Production has strict
//     rate limits and issues publicly trusted certificates, so pointing at it has
//     to be a deliberate choice rather than something that happens by default.

// LetsEncryptStaging is the default ACME directory.
const LetsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"

// ACMEConfig is the runtime ACME configuration.
type ACMEConfig struct {
	Enabled            bool
	Environment        string
	DirectoryURL       string
	Email              string
	AcceptTOS          bool
	KeyType            string // ecdsa | rsa
	RenewBeforeDays    int
	InsecureSkipVerify bool
	// ChallengeType is auto, http-01, or dns-01. Auto preserves HTTP-01 for
	// ordinary names and uses DNS-01 when a wildcard requires it.
	ChallengeType string
	DNS01         *dns01.Solver
	// KeyDir is where issued private keys are written.
	KeyDir string
}

// ACMEManager owns the ACME account and performs orders.
type ACMEManager struct {
	cfg    ACMEConfig
	client *acme.Client
	logger *zap.Logger
	dns01  *dns01.Solver

	mu sync.RWMutex
	// challenges maps an HTTP-01 token to its key authorization. The responder
	// reads it; the order flow writes it.
	challenges map[string]string
	account    *acme.Account
	accountErr string
	registered bool
}

// NewACMEManager prepares a manager. It does not contact the CA: registration is
// deferred to first use so a CA outage cannot stop the gateway from starting.
func NewACMEManager(cfg ACMEConfig, logger *zap.Logger) (*ACMEManager, error) {
	if cfg.DirectoryURL == "" {
		cfg.DirectoryURL = LetsEncryptStaging
	}
	if cfg.KeyType == "" {
		cfg.KeyType = "ecdsa"
	}
	if cfg.RenewBeforeDays <= 0 {
		cfg.RenewBeforeDays = 30
	}
	cfg.ChallengeType = strings.ToLower(strings.TrimSpace(cfg.ChallengeType))
	if cfg.ChallengeType == "" {
		cfg.ChallengeType = "auto"
	}
	switch cfg.ChallengeType {
	case "auto", "http-01", "dns-01":
	default:
		return nil, fmt.Errorf("acme.challenge_type must be auto, http-01, or dns-01")
	}
	if cfg.ChallengeType == "dns-01" && cfg.DNS01 == nil {
		return nil, fmt.Errorf("acme.challenge_type is dns-01 but no live DNS-01 provider is configured")
	}
	production := strings.EqualFold(strings.TrimSpace(cfg.Environment), "production") ||
		strings.EqualFold(strings.TrimSpace(cfg.Environment), "prod")
	if production && cfg.InsecureSkipVerify {
		return nil, fmt.Errorf("production ACME TLS verification cannot be disabled")
	}
	if cfg.KeyDir == "" {
		return nil, fmt.Errorf("a key directory is required; set state.dir so issued keys can be stored")
	}
	if err := os.MkdirAll(cfg.KeyDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating key dir: %w", err)
	}

	m := &ACMEManager{
		cfg:        cfg,
		logger:     logger,
		dns01:      cfg.DNS01,
		challenges: map[string]string{},
	}

	accountKey, err := m.loadOrCreateAccountKey()
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}
	if cfg.InsecureSkipVerify {
		// Only for a test CA such as Pebble, which uses its own untrusted root.
		logger.Warn("ACME TLS verification is disabled — acceptable only against a test CA")
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	m.client = &acme.Client{
		Key:          accountKey,
		DirectoryURL: cfg.DirectoryURL,
		HTTPClient:   httpClient,
		UserAgent:    "clm-satellite-gateway",
	}
	return m, nil
}

// accountKeyPath is kept separate from issued keys so rotating one does not
// disturb the other.
func (m *ACMEManager) accountKeyPath() string {
	return filepath.Join(m.cfg.KeyDir, "acme-account.key")
}

// loadOrCreateAccountKey reuses the existing account key when present. Creating a
// fresh key on every start would register a new ACME account each time and burn
// through the CA's account rate limits.
func (m *ACMEManager) loadOrCreateAccountKey() (crypto.Signer, error) {
	path := m.accountKeyPath()
	if data, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("account key at %s is not valid PEM", path)
		}
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing account key: %w", err)
		}
		m.logger.Info("reusing existing ACME account key", zap.String("path", path))
		return key, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating account key: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encoding account key: %w", err)
	}
	if err := os.WriteFile(path,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		return nil, fmt.Errorf("writing account key: %w", err)
	}
	m.logger.Info("generated a new ACME account key", zap.String("path", path))
	return key, nil
}

// ensureAccount registers with the CA once.
func (m *ACMEManager) ensureAccount(ctx context.Context) error {
	m.mu.RLock()
	done := m.registered
	m.mu.RUnlock()
	if done {
		return nil
	}

	if !m.cfg.AcceptTOS {
		return fmt.Errorf("acme.accept_tos must be set to true: an ACME account " +
			"cannot be created without agreeing to the CA's terms of service")
	}

	acct := &acme.Account{}
	if m.cfg.Email != "" {
		acct.Contact = []string{"mailto:" + m.cfg.Email}
	}

	registered, err := m.client.Register(ctx, acct, acme.AcceptTOS)
	if err != nil {
		// An already-registered key is success, not failure.
		if strings.Contains(strings.ToLower(err.Error()), "already") {
			m.mu.Lock()
			m.registered = true
			m.mu.Unlock()
			return nil
		}
		m.mu.Lock()
		m.accountErr = err.Error()
		m.mu.Unlock()
		return fmt.Errorf("registering ACME account: %w", err)
	}

	m.mu.Lock()
	m.account = registered
	m.registered = true
	m.accountErr = ""
	m.mu.Unlock()

	m.logger.Info("ACME account registered",
		zap.String("directory", m.cfg.DirectoryURL), zap.String("uri", registered.URI))
	return nil
}

// IssuedCert is the result of a successful order.
type IssuedCert struct {
	ChainPEM  string
	KeyPath   string
	NotBefore time.Time
	NotAfter  time.Time
	Serial    string
	Issuer    string
	Leaf      *x509.Certificate
}

// Issue runs a full ACME order for the supplied names and returns the chain.
//
// The first name becomes the certificate's common name; the rest are SANs.
func (m *ACMEManager) Issue(ctx context.Context, certID string, domains []string) (*IssuedCert, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}
	if err := m.ensureAccount(ctx); err != nil {
		return nil, err
	}

	authzIDs := make([]acme.AuthzID, 0, len(domains))
	for _, d := range domains {
		authzIDs = append(authzIDs, acme.AuthzID{Type: "dns", Value: d})
	}

	order, err := m.client.AuthorizeOrder(ctx, authzIDs)
	if err != nil {
		return nil, fmt.Errorf("creating order: %w", err)
	}

	// Keep the order URL separately. Only AuthorizeOrder populates Order.URI —
	// it comes from the response's Location header, which later polls do not
	// carry — so re-reading it from a refreshed order yields an empty string and
	// any subsequent request silently posts to "".
	orderURI := order.URI

	// Satisfy every authorization the CA asks for.
	for _, url := range order.AuthzURLs {
		authz, err := m.client.GetAuthorization(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("fetching authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue // already valid, e.g. a cached authorization
		}

		chal, challengeType := m.pickChallenge(authz)
		if chal == nil {
			if authz.Wildcard || strings.HasPrefix(authz.Identifier.Value, "*.") {
				if m.cfg.ChallengeType == "http-01" {
					return nil, fmt.Errorf("wildcard %s cannot use explicit http-01 mode; configure DNS-01 or use auto",
						authz.Identifier.Value)
				}
				return nil, fmt.Errorf("no usable dns-01 challenge offered for wildcard %s; configure a live DNS-01 provider",
					authz.Identifier.Value)
			}
			return nil, fmt.Errorf("no usable ACME challenge offered for %s (mode %s)",
				authz.Identifier.Value, m.cfg.ChallengeType)
		}

		switch challengeType {
		case "dns-01":
			recordValue, err := m.client.DNS01ChallengeRecord(chal.Token)
			if err != nil {
				return nil, fmt.Errorf("building DNS-01 challenge record: %w", err)
			}
			err = m.dns01.Solve(ctx, authz.Identifier.Value, recordValue, func(validateCtx context.Context) error {
				if _, err := m.client.Accept(validateCtx, chal); err != nil {
					return fmt.Errorf("accepting challenge: %w", err)
				}
				if _, err := m.client.WaitAuthorization(validateCtx, url); err != nil {
					return fmt.Errorf("authorization for %s failed: %w", authz.Identifier.Value, err)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}

		case "http-01":
			keyAuth, err := m.client.HTTP01ChallengeResponse(chal.Token)
			if err != nil {
				return nil, fmt.Errorf("building challenge response: %w", err)
			}

			m.mu.Lock()
			m.challenges[chal.Token] = keyAuth
			m.mu.Unlock()
			// The token is only needed until the CA has validated it.
			defer func(tok string) {
				m.mu.Lock()
				delete(m.challenges, tok)
				m.mu.Unlock()
			}(chal.Token)

			if _, err := m.client.Accept(ctx, chal); err != nil {
				return nil, fmt.Errorf("accepting challenge: %w", err)
			}
			if _, err := m.client.WaitAuthorization(ctx, url); err != nil {
				return nil, fmt.Errorf("authorization for %s failed: %w", authz.Identifier.Value, err)
			}
		}
	}

	order, err = m.client.WaitOrder(ctx, orderURI)
	if err != nil {
		return nil, fmt.Errorf("waiting for order: %w", err)
	}
	if order.FinalizeURL == "" {
		return nil, fmt.Errorf("the CA returned no finalize URL for the order (status %q)", order.Status)
	}

	certKey, keyPEM, err := m.generateKey()
	if err != nil {
		return nil, err
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}, certKey)
	if err != nil {
		return nil, fmt.Errorf("creating CSR: %w", err)
	}

	derChain, _, err := m.client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		// Finalization is allowed to be asynchronous: the CA may accept the CSR,
		// move the order to "processing" and only publish the certificate URL on a
		// later poll. In that case the immediate fetch has no URL to use and fails,
		// even though issuance is succeeding. Poll the order and fetch explicitly
		// rather than treating it as an error.
		m.logger.Info("finalize did not return a certificate immediately; polling the order",
			zap.String("reason", err.Error()))

		finished, werr := m.client.WaitOrder(ctx, orderURI)
		if werr != nil {
			return nil, fmt.Errorf("finalizing order: %w (and polling it then failed: %v)", err, werr)
		}
		if finished.CertURL == "" {
			return nil, fmt.Errorf("the CA finalized the order but published no certificate URL "+
				"(order status %q): %w", finished.Status, err)
		}
		derChain, err = m.client.FetchCert(ctx, finished.CertURL, true)
		if err != nil {
			return nil, fmt.Errorf("fetching the issued certificate: %w", err)
		}
	}
	if len(derChain) == 0 {
		return nil, fmt.Errorf("the CA returned an empty certificate chain")
	}

	leaf, err := x509.ParseCertificate(derChain[0])
	if err != nil {
		return nil, fmt.Errorf("parsing issued certificate: %w", err)
	}

	var chainPEM strings.Builder
	for _, der := range derChain {
		if err := pem.Encode(&chainPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
			return nil, fmt.Errorf("encoding chain: %w", err)
		}
	}

	// Key files are immutable per issued serial. Reusing certID.key would erase
	// the active version before renewal deployment had been verified, making
	// rollback impossible after a partial rollout.
	keyPath := filepath.Join(m.cfg.KeyDir, certID+"-"+leaf.SerialNumber.Text(16)+".key")
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("writing private key: %w", err)
	}

	m.logger.Info("certificate issued",
		zap.Strings("domains", domains),
		zap.String("issuer", leaf.Issuer.CommonName),
		zap.Time("not_after", leaf.NotAfter))

	return &IssuedCert{
		ChainPEM:  chainPEM.String(),
		KeyPath:   keyPath,
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
		Serial:    leaf.SerialNumber.Text(16),
		Issuer:    leaf.Issuer.CommonName,
		Leaf:      leaf,
	}, nil
}

// Revoke revokes a certificate at the CA using its own private key.
func (m *ACMEManager) Revoke(ctx context.Context, chainPEM, keyPath string, reason acme.CRLReasonCode) error {
	if err := m.ensureAccount(ctx); err != nil {
		return err
	}

	block, _ := pem.Decode([]byte(chainPEM))
	if block == nil {
		return fmt.Errorf("stored chain is not valid PEM")
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("reading private key: %w", err)
	}
	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil {
		return fmt.Errorf("stored key is not valid PEM")
	}
	signer, err := parsePrivateKey(keyBlock)
	if err != nil {
		return err
	}

	if err := m.client.RevokeCert(ctx, signer, block.Bytes, reason); err != nil {
		return fmt.Errorf("revoking at the CA: %w", err)
	}
	m.logger.Info("certificate revoked at the CA", zap.String("key_path", keyPath))
	return nil
}

// parsePrivateKey handles both key types the manager can generate.
func parsePrivateKey(block *pem.Block) (crypto.Signer, error) {
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if signer, ok := k.(crypto.Signer); ok {
			return signer, nil
		}
	}
	return nil, fmt.Errorf("unsupported private key format")
}

// generateKey produces a certificate key of the configured type.
func (m *ACMEManager) generateKey() (crypto.Signer, []byte, error) {
	if strings.EqualFold(m.cfg.KeyType, "rsa") {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, fmt.Errorf("generating RSA key: %w", err)
		}
		return key, pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		}), nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating ECDSA key: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encoding ECDSA key: %w", err)
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}

// pickChallenge chooses DNS-01 for every authorization in dns-01 mode and for
// wildcard authorizations in auto mode. HTTP-01 remains the preferred automatic
// path for ordinary names so existing deployments need no DNS credentials.
func (m *ACMEManager) pickChallenge(authz *acme.Authorization) (*acme.Challenge, string) {
	wildcard := authz.Wildcard || strings.HasPrefix(strings.TrimSpace(authz.Identifier.Value), "*.")
	if m.cfg.ChallengeType == "http-01" {
		if wildcard {
			return nil, "http-01"
		}
		return pickChallengeType(authz, "http-01"), "http-01"
	}
	if wildcard || m.cfg.ChallengeType == "dns-01" {
		if m.dns01 == nil {
			return nil, "dns-01"
		}
		return pickChallengeType(authz, "dns-01"), "dns-01"
	}
	if challenge := pickChallengeType(authz, "http-01"); challenge != nil {
		return challenge, "http-01"
	}
	if m.dns01 != nil {
		return pickChallengeType(authz, "dns-01"), "dns-01"
	}
	return nil, ""
}

func pickChallengeType(authz *acme.Authorization, challengeType string) *acme.Challenge {
	for _, challenge := range authz.Challenges {
		if challenge.Type == challengeType {
			return challenge
		}
	}
	return nil
}

// SupportsHTTP01 reports whether ordinary authorizations may use HTTP-01.
func (m *ACMEManager) SupportsHTTP01() bool {
	return m != nil && m.cfg.ChallengeType != "dns-01"
}

// SupportsDNS01 reports whether wildcard and DNS-only issuance can run under
// the selected challenge policy.
func (m *ACMEManager) SupportsDNS01() bool {
	return m != nil && m.dns01 != nil && m.cfg.ChallengeType != "http-01"
}

// ChallengeResponse returns the key authorization for a token, for the responder.
func (m *ACMEManager) ChallengeResponse(token string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.challenges[token]
	return v, ok
}

// Status describes the manager for the portal.
func (m *ACMEManager) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	staging := strings.Contains(m.cfg.DirectoryURL, "staging") ||
		!strings.Contains(m.cfg.DirectoryURL, "letsencrypt.org")
	http01Enabled := m.SupportsHTTP01()
	dns01Enabled := m.SupportsDNS01()
	status := map[string]any{
		"enabled":            m.cfg.Enabled,
		"directory_url":      m.cfg.DirectoryURL,
		"email":              m.cfg.Email,
		"key_type":           m.cfg.KeyType,
		"renew_before_days":  m.cfg.RenewBeforeDays,
		"account_registered": m.registered,
		"account_error":      m.accountErr,
		"non_production_ca":  staging,
		"challenge_type":     m.cfg.ChallengeType,
		"pending_challenges": len(m.challenges),
		"http01_enabled":     http01Enabled,
		"dns01_enabled":      dns01Enabled,
	}
	if dns01Enabled {
		status["dns01_provider"] = m.dns01.ProviderName()
	}
	return status
}

// RenewBefore exposes the renewal window for the scheduler.
func (m *ACMEManager) RenewBefore() time.Duration {
	return time.Duration(m.cfg.RenewBeforeDays) * 24 * time.Hour
}

// --- HTTP handlers ---

// handleACMEChallenge answers an HTTP-01 validation request.
//
// Unauthenticated by necessity: the caller is the CA's validation server, which
// has no credentials for this gateway. It only ever returns a key authorization
// for a token the gateway is currently proving, so there is nothing to leak — an
// unknown token gets a 404.
// GET /.well-known/acme-challenge/{token}
func (s *Server) handleACMEChallenge(w http.ResponseWriter, r *http.Request) {
	if s.acme == nil {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	resp, ok := s.acme.ChallengeResponse(token)
	if !ok {
		s.logger.Warn("acme challenge requested for an unknown token",
			zap.String("token", token), zap.String("remote", r.RemoteAddr))
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(resp))
}

// handleACMEStatus reports the ACME configuration and account state.
// GET /api/v1/acme/status
func (s *Server) handleACMEStatus(w http.ResponseWriter, r *http.Request) {
	if s.acme == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"reason": "ACME is not configured; set acme.enabled and acme.accept_tos, " +
				"and state.dir so issued keys can be stored",
		})
		return
	}
	writeJSON(w, http.StatusOK, s.acme.Status())
}

// handleRenewCertificate renews a managed certificate now.
// POST /api/v1/certificates/{id}/renew
func (s *Server) handleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	if s.acme == nil {
		writeErr(w, http.StatusServiceUnavailable, "ACME is not configured on this gateway")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "renewing a certificate requires an admin or cert_manager role")
		return
	}

	cert, ok := s.inventory.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "certificate not found")
		return
	}
	if cert.ManagedBy == "" {
		writeErr(w, http.StatusBadRequest,
			"only certificates this gateway ordered can be renewed; a discovered "+
				"certificate has no private key here, so there is nothing to renew with")
		return
	}
	if cert.ManagedBy != "acme" {
		// Renewing at a commercial CA means buying another certificate. Doing that
		// from a button labelled "renew" would spend money without the operator
		// intending to, so it is refused and pointed at the explicit order path.
		writeErr(w, http.StatusBadRequest, fmt.Sprintf(
			"this certificate was issued by %q, where renewal means placing a new "+
				"paid order. Use POST /api/v1/certificates/issue with ca_account=%s "+
				"so the cost is explicit",
			cert.ManagedBy, nonEmpty(cert.CAAccount, cert.ManagedBy)))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()

	if err := s.renewCertificate(ctx, cert); err != nil {
		s.recordAudit(r, "certificate.renew", "certificate", cert.ID, cert.Domain, audit.ResultFailure, err.Error())
		writeErr(w, http.StatusBadGateway, "renewal failed: "+err.Error())
		return
	}
	s.recordAudit(r, "certificate.renew", "certificate", cert.ID, cert.Domain, audit.ResultSuccess,
		fmt.Sprintf("renewal_count=%d not_after=%s", cert.RenewalCount, cert.NotAfter.Format(time.RFC3339)))
	writeJSON(w, http.StatusOK, map[string]any{
		"id": cert.ID, "domain": cert.Domain,
		"not_after": cert.NotAfter, "renewal_count": cert.RenewalCount,
		"message": "renewed",
	})
}

// handleRecoverRenewal reconciles an indeterminate renewal by redeploying the
// last durably committed version to every active binding and independently
// verifying each endpoint. It never promotes the candidate implicitly.
// POST /api/v1/certificates/{id}/renew/recover
func (s *Server) handleRecoverRenewal(w http.ResponseWriter, r *http.Request) {
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "recovering a renewal requires an admin or cert_manager role")
		return
	}

	certID := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	if err := s.recoverRenewal(ctx, certID, callerName(r)); err != nil {
		s.recordAudit(r, "certificate.renew.recover", "certificate", certID, "", audit.ResultFailure, err.Error())
		writeErr(w, http.StatusBadGateway, "renewal recovery failed: "+err.Error())
		return
	}

	cert, ok := s.inventory.Get(certID)
	domain, activeVersionID := "", ""
	if ok {
		domain = cert.Domain
		activeVersionID = cert.ActiveVersionID
	}
	s.recordAudit(r, "certificate.renew.recover", "certificate", certID, domain,
		audit.ResultSuccess, "last committed version redeployed and verified")
	writeJSON(w, http.StatusOK, map[string]any{
		"id": certID, "domain": domain, "status": "recovered",
		"active_version_id": activeVersionID,
		"message":           "last committed certificate version redeployed and verified",
	})
}

// renewCertificate issues a candidate version, replays every currently active
// deployment binding, and activates the new version only after all endpoint
// checks pass. The previous version remains available for automatic recovery.
func (s *Server) renewCertificate(ctx context.Context, cert *Certificate) error {
	return s.issueAndRolloutRenewal(ctx, cert)
}

// handleRevokeCertificate revokes a managed certificate at the CA.
// POST /api/v1/certificates/{id}/revoke
func (s *Server) handleRevokeCertificate(w http.ResponseWriter, r *http.Request) {
	if s.acme == nil {
		writeErr(w, http.StatusServiceUnavailable, "ACME is not configured on this gateway")
		return
	}
	if !canManageFleet(callerClaims(r).EffectiveRole()) {
		writeErr(w, http.StatusForbidden, "revoking a certificate requires an admin or cert_manager role")
		return
	}

	certID := r.PathValue("id")
	unlockCertificate := s.lockCertificateOperation(certID)
	defer unlockCertificate()
	cert, ok := s.inventory.Get(certID)
	if !ok {
		writeErr(w, http.StatusNotFound, "certificate not found")
		return
	}
	if cert.ManagedBy == "" {
		writeErr(w, http.StatusBadRequest,
			"only certificates this gateway ordered can be revoked; a discovered "+
				"certificate belongs to someone else's CA account")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &req)

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	// Route to whichever CA issued it. ACME signs the revocation with the
	// certificate's own key; a commercial CA revokes by order id on the account.
	if err := s.revokeVia(ctx, cert, req.Reason); err != nil {
		s.recordAudit(r, "certificate.revoke", "certificate", cert.ID, cert.Domain, audit.ResultFailure, err.Error())
		writeErr(w, http.StatusBadGateway, "revocation failed: "+err.Error())
		return
	}

	cert.RevokedAt = time.Now()
	cert.Status = "revoked"
	s.inventory.Upsert(cert)

	s.logger.Info("certificate revoked",
		zap.String("domain", cert.Domain), zap.String("by", callerName(r)))
	s.recordAudit(r, "certificate.revoke", "certificate", cert.ID, cert.Domain, audit.ResultSuccess,
		nonEmpty(req.Reason, "no reason given"))
	writeJSON(w, http.StatusOK, map[string]any{
		"id": cert.ID, "domain": cert.Domain, "revoked_at": cert.RevokedAt,
		"message": "revoked at the CA; replace any deployment still serving it",
	})
}

// revokeVia sends the revocation to the CA that issued the certificate.
func (s *Server) revokeVia(ctx context.Context, cert *Certificate, reason string) error {
	if s.caRegistry == nil || s.caRegistry.Count() == 0 {
		return fmt.Errorf("no CA accounts are configured")
	}

	account, err := s.caRegistry.Get(nonEmpty(cert.CAAccount, cert.ManagedBy))
	if err != nil {
		return fmt.Errorf("the CA account that issued this certificate is no longer configured: %w", err)
	}

	req := ca.RevokeRequest{
		OrderID: cert.CAOrderID, ChainPEM: cert.ChainPEM, Reason: reason,
	}
	// ACME requires the certificate's own key to sign the revocation; read it only
	// for the provider that needs it, so a key is never loaded unnecessarily.
	if account.Provider_().Name() == "acme" {
		if cert.SecretRef == "" {
			return fmt.Errorf("the private key for this certificate is not held here, " +
				"so an ACME revocation cannot be signed")
		}
		req.KeyPEM = cert.SecretRef
	}
	return account.Provider_().Revoke(ctx, req)
}

// revocationReason maps a friendly name to an RFC 5280 reason code.
func revocationReason(name string) acme.CRLReasonCode {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "key_compromise", "keycompromise":
		return acme.CRLReasonKeyCompromise
	case "superseded":
		return acme.CRLReasonSuperseded
	case "cessation", "cessation_of_operation":
		return acme.CRLReasonCessationOfOperation
	case "affiliation_changed":
		return acme.CRLReasonAffiliationChanged
	default:
		return acme.CRLReasonUnspecified
	}
}
