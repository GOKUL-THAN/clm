package deployers

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const defaultEndpointVerifyTimeout = 10 * time.Second

// EndpointVerifyOptions describes an independent observation of a TLS-serving
// endpoint. Host is only used for dialing; ServerName is the SNI and SAN name.
type EndpointVerifyOptions struct {
	Host                string
	Port                int
	ServerName          string
	Protocol            string // tls or postgresql
	ExpectedFingerprint string
	ExpectedIssuer      string
	TrustPEM            []byte
	Timeout             time.Duration
}

// LeafSHA256 returns the lowercase SHA-256 fingerprint of the first certificate
// in PEM input.
func LeafSHA256(certPEM []byte) (string, error) {
	leaf, err := firstCertificate(certPEM)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(leaf.Raw)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyEndpoint performs a bounded TCP/TLS observation. PostgreSQL endpoints
// first use the protocol SSLRequest negotiation before starting TLS.
func VerifyEndpoint(ctx context.Context, opts EndpointVerifyOptions) (*VerificationEvidence, error) {
	host := strings.TrimSpace(opts.Host)
	serverName := strings.TrimSpace(opts.ServerName)
	protocol := strings.ToLower(strings.TrimSpace(opts.Protocol))
	if protocol == "" {
		protocol = "tls"
	}
	if host == "" {
		return nil, fmt.Errorf("serving host is required")
	}
	if opts.Port < 1 || opts.Port > 65535 {
		return nil, fmt.Errorf("serving port must be between 1 and 65535")
	}
	if serverName == "" {
		serverName = strings.Trim(host, "[]")
	}
	if protocol != "tls" && protocol != "postgresql" {
		return nil, fmt.Errorf("unsupported endpoint protocol %q", protocol)
	}

	timeout := opts.Timeout
	if timeout <= 0 || timeout > defaultEndpointVerifyTimeout {
		timeout = defaultEndpointVerifyTimeout
	}
	verifyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	address := net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(opts.Port))
	evidence := &VerificationEvidence{
		Kind:                "endpoint",
		Method:              "tls-handshake",
		DialAddress:         address,
		ServerName:          serverName,
		Protocol:            protocol,
		ExpectedFingerprint: normaliseFingerprint(opts.ExpectedFingerprint),
		CheckedAt:           time.Now().UTC(),
		IssuerMatch:         strings.TrimSpace(opts.ExpectedIssuer) == "",
	}
	if protocol == "postgresql" {
		evidence.Method = "postgresql-sslrequest-tls"
	}

	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(verifyCtx, "tcp", address)
	if err != nil {
		evidence.Message = "TCP connection failed: " + err.Error()
		return evidence, fmt.Errorf("dialing TLS endpoint %s: %w", address, err)
	}
	defer conn.Close()
	if deadline, ok := verifyCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if protocol == "postgresql" {
		var sslRequest [8]byte
		binary.BigEndian.PutUint32(sslRequest[0:4], 8)
		binary.BigEndian.PutUint32(sslRequest[4:8], 80877103)
		if _, err := conn.Write(sslRequest[:]); err != nil {
			evidence.Message = "PostgreSQL SSLRequest write failed: " + err.Error()
			return evidence, fmt.Errorf("sending PostgreSQL SSLRequest: %w", err)
		}
		var response [1]byte
		if _, err := conn.Read(response[:]); err != nil {
			evidence.Message = "PostgreSQL SSLRequest response failed: " + err.Error()
			return evidence, fmt.Errorf("reading PostgreSQL SSLRequest response: %w", err)
		}
		if response[0] != 'S' {
			evidence.Message = fmt.Sprintf("PostgreSQL refused TLS with response %q", response[0])
			return evidence, fmt.Errorf("PostgreSQL endpoint %s did not accept SSLRequest", address)
		}
	}

	// #nosec G402 -- accept the peer certificate to record it, then verify hostname, validity, issuer, and fingerprint below.
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err := tlsConn.HandshakeContext(verifyCtx); err != nil {
		evidence.Message = "TLS handshake failed: " + err.Error()
		return evidence, fmt.Errorf("TLS handshake with %s: %w", address, err)
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		evidence.Message = "TLS peer returned no certificate"
		return evidence, fmt.Errorf("TLS endpoint %s returned no certificate", address)
	}

	leaf := state.PeerCertificates[0]
	sum := sha256.Sum256(leaf.Raw)
	evidence.ObservedFingerprint = hex.EncodeToString(sum[:])
	evidence.Subject = leaf.Subject.String()
	evidence.Issuer = leaf.Issuer.String()
	evidence.NotBefore = leaf.NotBefore
	evidence.NotAfter = leaf.NotAfter
	now := time.Now()
	evidence.ValidityValid = !now.Before(leaf.NotBefore) && !now.After(leaf.NotAfter)
	evidence.FingerprintMatch = evidence.ExpectedFingerprint != "" &&
		evidence.ObservedFingerprint == evidence.ExpectedFingerprint
	evidence.HostnameValid = leaf.VerifyHostname(serverName) == nil
	if expectedIssuer := strings.TrimSpace(opts.ExpectedIssuer); expectedIssuer != "" {
		evidence.IssuerMatch = strings.Contains(strings.ToLower(leaf.Issuer.String()), strings.ToLower(expectedIssuer)) ||
			strings.EqualFold(leaf.Issuer.CommonName, expectedIssuer)
	}

	roots, _ := x509.SystemCertPool()
	if roots == nil {
		roots = x509.NewCertPool()
	}
	intermediates := x509.NewCertPool()
	for _, cert := range state.PeerCertificates[1:] {
		intermediates.AddCert(cert)
	}
	addTrustCertificates(roots, intermediates, opts.TrustPEM)
	_, chainErr := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates})
	evidence.ChainTrusted = chainErr == nil

	var failures []string
	if !evidence.FingerprintMatch {
		failures = append(failures, "leaf fingerprint does not match the deployed certificate")
	}
	if !evidence.ValidityValid {
		failures = append(failures, "leaf certificate is not currently valid")
	}
	if !evidence.ChainTrusted {
		failures = append(failures, "certificate chain is not trusted")
	}
	if !evidence.HostnameValid {
		failures = append(failures, "certificate SANs do not match server_name")
	}
	if !evidence.IssuerMatch {
		failures = append(failures, "certificate issuer does not match expected_issuer")
	}
	if len(failures) > 0 {
		evidence.Message = strings.Join(failures, "; ")
		if chainErr != nil && !evidence.ChainTrusted {
			evidence.Message += ": " + chainErr.Error()
		}
		return evidence, errors.New(evidence.Message)
	}

	evidence.Success = true
	evidence.Message = "observed the expected valid, trusted certificate with matching SANs"
	return evidence, nil
}

func firstCertificate(certPEM []byte) (*x509.Certificate, error) {
	rest := certPEM
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			return nil, fmt.Errorf("certificate PEM contains no CERTIFICATE block")
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing leaf certificate: %w", err)
		}
		return cert, nil
	}
}

func addTrustCertificates(roots, intermediates *x509.CertPool, trustPEM []byte) {
	rest := trustPEM
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			return
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA {
			continue
		}
		if cert.CheckSignatureFrom(cert) == nil {
			roots.AddCert(cert)
		} else {
			intermediates.AddCert(cert)
		}
	}
}

func normaliseFingerprint(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ":", "")
	return value
}
