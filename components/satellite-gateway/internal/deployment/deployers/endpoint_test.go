package deployers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

type endpointTestCert struct {
	tlsCert tls.Certificate
	leafPEM []byte
	rootPEM []byte
}

func makeEndpointTestCert(t *testing.T, dnsName string, notBefore, notAfter time.Time) endpointTestCert {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Endpoint Test Root"},
		NotBefore: time.Now().Add(-24 * time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: dnsName},
		DNSNames: []string{dnsName}, NotBefore: notBefore, NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(append(append([]byte{}, leafPEM...), rootPEM...), keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return endpointTestCert{tlsCert: tlsCert, leafPEM: leafPEM, rootPEM: rootPEM}
}

func startTLSEndpoint(t *testing.T, cert tls.Certificate) (string, int) {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := net.LookupPort("tcp", portText)
	return host, port
}

func TestVerifyEndpointDirectTLS(t *testing.T) {
	cert := makeEndpointTestCert(t, "service.example.test", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	host, port := startTLSEndpoint(t, cert.tlsCert)
	fingerprint, err := LeafSHA256(cert.leafPEM)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := VerifyEndpoint(context.Background(), EndpointVerifyOptions{
		Host: host, Port: port, ServerName: "service.example.test",
		ExpectedFingerprint: fingerprint, ExpectedIssuer: "Endpoint Test Root", TrustPEM: cert.rootPEM,
	})
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if !evidence.Success || !evidence.FingerprintMatch || !evidence.ValidityValid ||
		!evidence.ChainTrusted || !evidence.HostnameValid || !evidence.IssuerMatch {
		t.Fatalf("incomplete evidence: %+v", evidence)
	}
}

func TestVerifyEndpointRejectsFingerprintHostnameAndIssuer(t *testing.T) {
	cert := makeEndpointTestCert(t, "service.example.test", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	host, port := startTLSEndpoint(t, cert.tlsCert)
	cases := []struct {
		name        string
		serverName  string
		fingerprint string
		issuer      string
		want        string
	}{
		{name: "fingerprint", serverName: "service.example.test", fingerprint: strings.Repeat("0", 64), issuer: "Endpoint Test Root", want: "fingerprint"},
		{name: "hostname", serverName: "other.example.test", fingerprint: mustFingerprint(t, cert.leafPEM), issuer: "Endpoint Test Root", want: "SANs"},
		{name: "issuer", serverName: "service.example.test", fingerprint: mustFingerprint(t, cert.leafPEM), issuer: "Different CA", want: "issuer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence, err := VerifyEndpoint(context.Background(), EndpointVerifyOptions{
				Host: host, Port: port, ServerName: tc.serverName, ExpectedFingerprint: tc.fingerprint,
				ExpectedIssuer: tc.issuer, TrustPEM: cert.rootPEM,
			})
			if err == nil || evidence.Success || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("evidence=%+v err=%v, want failure containing %q", evidence, err, tc.want)
			}
		})
	}
}

func TestVerifyEndpointRejectsExpiredAndUntrusted(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		cert := makeEndpointTestCert(t, "expired.example.test", time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
		host, port := startTLSEndpoint(t, cert.tlsCert)
		evidence, err := VerifyEndpoint(context.Background(), EndpointVerifyOptions{
			Host: host, Port: port, ServerName: "expired.example.test",
			ExpectedFingerprint: mustFingerprint(t, cert.leafPEM), TrustPEM: cert.rootPEM,
		})
		if err == nil || evidence.ValidityValid {
			t.Fatalf("evidence=%+v err=%v, want expired failure", evidence, err)
		}
	})
	t.Run("untrusted", func(t *testing.T) {
		cert := makeEndpointTestCert(t, "untrusted.example.test", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		host, port := startTLSEndpoint(t, cert.tlsCert)
		evidence, err := VerifyEndpoint(context.Background(), EndpointVerifyOptions{
			Host: host, Port: port, ServerName: "untrusted.example.test",
			ExpectedFingerprint: mustFingerprint(t, cert.leafPEM),
		})
		if err == nil || evidence.ChainTrusted {
			t.Fatalf("evidence=%+v err=%v, want untrusted failure", evidence, err)
		}
	})
}

func TestVerifyEndpointTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			defer conn.Close()
			time.Sleep(time.Second)
		}
	}()
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := net.LookupPort("tcp", portText)
	start := time.Now()
	_, err = VerifyEndpoint(context.Background(), EndpointVerifyOptions{
		Host: host, Port: port, ServerName: "timeout.example.test",
		ExpectedFingerprint: strings.Repeat("0", 64), Timeout: 50 * time.Millisecond,
	})
	if err == nil || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("err=%v duration=%s, want bounded timeout", err, time.Since(start))
	}
}

func TestVerifyEndpointPostgreSQLSSLRequest(t *testing.T) {
	cert := makeEndpointTestCert(t, "postgres.example.test", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		request := make([]byte, 8)
		if _, err := io.ReadFull(conn, request); err != nil {
			return
		}
		if _, err := conn.Write([]byte{'S'}); err != nil {
			return
		}
		tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert.tlsCert}, MinVersion: tls.VersionTLS12})
		_ = tlsConn.Handshake()
	}()
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := net.LookupPort("tcp", portText)
	evidence, err := VerifyEndpoint(context.Background(), EndpointVerifyOptions{
		Host: host, Port: port, ServerName: "postgres.example.test", Protocol: "postgresql",
		ExpectedFingerprint: mustFingerprint(t, cert.leafPEM), TrustPEM: cert.rootPEM,
	})
	if err != nil || !evidence.Success || evidence.Method != "postgresql-sslrequest-tls" {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
}

func mustFingerprint(t *testing.T, certPEM []byte) string {
	t.Helper()
	fingerprint, err := LeafSHA256(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}
