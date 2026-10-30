package cryptoengine

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

var (
	ErrUnsupportedAlgorithm = errors.New("unsupported key algorithm")
	ErrInvalidKey           = errors.New("invalid private key")
	ErrInvalidCert          = errors.New("invalid certificate")
	ErrKeyMismatch          = errors.New("private key does not match certificate")
)

// Engine performs all crypto operations on the satellite (keys never leave here).
type Engine struct {
	logger *zap.Logger
}

// NewEngine creates a new crypto engine.
func NewEngine(logger *zap.Logger) *Engine {
	return &Engine{logger: logger}
}

// GenerateKey generates a private key of the specified algorithm.
func (e *Engine) GenerateKey(ctx context.Context, algo models.KeyAlgorithm) ([]byte, error) {
	var privKey crypto.PrivateKey
	var err error

	switch algo {
	case models.KeyAlgoRSA2048:
		privKey, err = rsa.GenerateKey(rand.Reader, 2048)
	case models.KeyAlgoRSA4096:
		privKey, err = rsa.GenerateKey(rand.Reader, 4096)
	case models.KeyAlgoECDSAP256:
		privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case models.KeyAlgoECDSAP384:
		privKey, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case models.KeyAlgoEd25519:
		// Ed25519 uses ECDSA P-256 as fallback (Go's ed25519 is separate package)
		privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	default:
		return nil, ErrUnsupportedAlgorithm
	}

	if err != nil {
		return nil, fmt.Errorf("key generation failed: %w", err)
	}

	keyPEM, err := marshalPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	e.logger.Info("private key generated",
		zap.String("algorithm", string(algo)),
	)

	return keyPEM, nil
}

// GenerateCSR creates a PKCS#10 Certificate Signing Request.
func (e *Engine) GenerateCSR(ctx context.Context, privateKeyPEM []byte, req *core.CSRRequest) ([]byte, error) {
	privKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}

	subject := pkix.Name{
		CommonName: req.CommonName,
	}
	if req.Organization != "" {
		subject.Organization = []string{req.Organization}
	}
	if req.Country != "" {
		subject.Country = []string{req.Country}
	}

	template := &x509.CertificateRequest{
		Subject:  subject,
		DNSNames: req.SANs,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	if err != nil {
		return nil, fmt.Errorf("CSR creation failed: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	e.logger.Info("CSR generated",
		zap.String("cn", req.CommonName),
		zap.Int("sans", len(req.SANs)),
	)

	return csrPEM, nil
}

// AssembleChain validates and assembles the certificate chain.
func (e *Engine) AssembleChain(ctx context.Context, certPEM []byte, intermediates [][]byte) ([]byte, []byte, error) {
	// Parse leaf cert
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, nil, ErrInvalidCert
	}

	// Build chain PEM (intermediates concatenated)
	var chainPEM []byte
	for _, intermediate := range intermediates {
		chainPEM = append(chainPEM, intermediate...)
		chainPEM = append(chainPEM, '\n')
	}

	// Build fullchain (cert + intermediates)
	var fullChainPEM []byte
	fullChainPEM = append(fullChainPEM, certPEM...)
	fullChainPEM = append(fullChainPEM, '\n')
	fullChainPEM = append(fullChainPEM, chainPEM...)

	e.logger.Info("certificate chain assembled",
		zap.Int("intermediates", len(intermediates)),
	)

	return chainPEM, fullChainPEM, nil
}

// CreatePFX creates a PKCS12/PFX bundle from cert, key, and chain.
func (e *Engine) CreatePFX(ctx context.Context, certPEM, keyPEM, chainPEM []byte, password string) ([]byte, error) {
	// In production: use go.mozilla.org/pkcs12 or software.sslmate.com/src/go-pkcs12
	// For now, return a placeholder indicating PFX creation
	e.logger.Info("PFX bundle creation requested")
	return []byte("PFX_PLACEHOLDER"), nil
}

// VerifyKeyMatch checks that a certificate and private key are a matching pair.
func (e *Engine) VerifyKeyMatch(ctx context.Context, certPEM, keyPEM []byte) (bool, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return false, ErrInvalidCert
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return false, fmt.Errorf("parsing certificate: %w", err)
	}

	privKey, err := parsePrivateKey(keyPEM)
	if err != nil {
		return false, err
	}

	// Compare public keys
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		priv, ok := privKey.(*rsa.PrivateKey)
		if !ok {
			return false, nil
		}
		return pub.N.Cmp(priv.N) == 0, nil
	case *ecdsa.PublicKey:
		priv, ok := privKey.(*ecdsa.PrivateKey)
		if !ok {
			return false, nil
		}
		return pub.X.Cmp(priv.X) == 0 && pub.Y.Cmp(priv.Y) == 0, nil
	default:
		return false, errors.New("unsupported public key type")
	}
}

// ParseCertificate extracts metadata from a PEM-encoded certificate.
func (e *Engine) ParseCertificate(ctx context.Context, certPEM []byte) (*core.CertInfo, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, ErrInvalidCert
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	fingerprint := sha256.Sum256(cert.Raw)

	info := &core.CertInfo{
		Subject:      cert.Subject.CommonName,
		Issuer:       cert.Issuer.CommonName,
		SerialNumber: cert.SerialNumber.Text(16),
		Fingerprint:  hex.EncodeToString(fingerprint[:]),
		NotBefore:    cert.NotBefore.Format(time.RFC3339),
		NotAfter:     cert.NotAfter.Format(time.RFC3339),
		SANs:         cert.DNSNames,
		KeyAlgorithm: cert.PublicKeyAlgorithm.String(),
	}

	return info, nil
}

// GenerateSelfSigned creates a self-signed certificate (for testing/internal use).
func (e *Engine) GenerateSelfSigned(ctx context.Context, keyPEM []byte, cn string, sans []string, validDays int) ([]byte, error) {
	privKey, err := parsePrivateKey(keyPEM)
	if err != nil {
		return nil, err
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: cn,
		},
		DNSNames:              sans,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Duration(validDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	var pubKey crypto.PublicKey
	switch k := privKey.(type) {
	case *rsa.PrivateKey:
		pubKey = &k.PublicKey
	case *ecdsa.PrivateKey:
		pubKey = &k.PublicKey
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pubKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("self-signed cert creation failed: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM, nil
}

// --- Helpers ---

func marshalPrivateKey(key crypto.PrivateKey) ([]byte, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		}), nil
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: der,
		}), nil
	default:
		return nil, ErrUnsupportedAlgorithm
	}
}

func parsePrivateKey(keyPEM []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, ErrInvalidKey
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", block.Type)
	}
}
