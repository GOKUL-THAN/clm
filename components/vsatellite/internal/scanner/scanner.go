package scanner

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DiscoveredCert represents a certificate found during TLS network scanning.
type DiscoveredCert struct {
	Host            string    `json:"host"`
	Port            int       `json:"port"`
	Subject         string    `json:"subject"`
	Issuer          string    `json:"issuer"`
	SANs            []string  `json:"sans"`
	SerialNumber    string    `json:"serial_number"`
	Fingerprint     string    `json:"fingerprint"`
	NotBefore       time.Time `json:"not_before"`
	NotAfter        time.Time `json:"not_after"`
	KeyAlgorithm    string    `json:"key_algorithm"`
	KeySize         int       `json:"key_size"`
	ChainDepth      int       `json:"chain_depth"`
	ChainValid      bool      `json:"chain_valid"`
	SelfSigned      bool      `json:"self_signed"`
	DaysLeft        int       `json:"days_left"`
	Protocol        string    `json:"protocol"`
	ServerName      string    `json:"server_name"`
	ResolvedIP      string    `json:"resolved_ip,omitempty"`
	DiscoveryMethod string    `json:"discovery_method"`
	DiscoveredAt    time.Time `json:"discovered_at"`
}

// Scanner performs concurrent TLS probes across CIDR blocks and IP targets.
type Scanner struct {
	concurrency int
	timeout     time.Duration
	ports       []int
	logger      *zap.Logger
}

// NewScanner creates a new network TLS scanner.
func NewScanner(concurrency int, timeout time.Duration, ports []int, logger *zap.Logger) *Scanner {
	if concurrency <= 0 {
		concurrency = 50
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if len(ports) == 0 {
		ports = []int{443, 8443, 636, 6443}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Scanner{
		concurrency: concurrency,
		timeout:     timeout,
		ports:       ports,
		logger:      logger,
	}
}

type scanTarget struct {
	host string
	port int
}

// ScanCIDR scans a CIDR subnet block concurrently on configured TLS ports.
func (s *Scanner) ScanCIDR(ctx context.Context, cidr string) ([]*DiscoveredCert, error) {
	hosts, err := expandCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	targets := make(chan scanTarget, len(hosts)*len(s.ports))
	results := make(chan *DiscoveredCert, len(hosts)*len(s.ports))

	go func() {
		for _, host := range hosts {
			for _, port := range s.ports {
				targets <- scanTarget{host: host, port: port}
			}
		}
		close(targets)
	}()

	var wg sync.WaitGroup
	for i := 0; i < s.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range targets {
				select {
				case <-ctx.Done():
					return
				default:
					cert := s.probeTarget(target.host, target.port)
					if cert != nil {
						results <- cert
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var found []*DiscoveredCert
	for cert := range results {
		found = append(found, cert)
	}

	s.logger.Info("scan completed",
		zap.String("cidr", cidr),
		zap.Int("hosts_scanned", len(hosts)),
		zap.Int("certs_found", len(found)),
	)

	return found, nil
}

// probeTarget attempts a TLS handshake against host:port and parses the certificate.
func (s *Scanner) probeTarget(host string, port int) *DiscoveredCert {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: s.timeout}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Necessary to discover expired, untrusted, or self-signed certs
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return nil
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil
	}

	leaf := state.PeerCertificates[0]
	h := sha256.Sum256(leaf.Raw)
	fingerprint := hex.EncodeToString(h[:])

	now := time.Now()
	daysLeft := int(leaf.NotAfter.Sub(now).Hours() / 24)

	sans := make([]string, 0, len(leaf.DNSNames)+len(leaf.IPAddresses))
	sans = append(sans, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		sans = append(sans, ip.String())
	}

	keyAlg := leaf.PublicKeyAlgorithm.String()
	keySize := 0
	switch pub := leaf.PublicKey.(type) {
	case interface{ Size() int }:
		keySize = pub.Size() * 8
	}

	selfSigned := leaf.Issuer.CommonName == leaf.Subject.CommonName &&
		leaf.Issuer.String() == leaf.Subject.String()

	proto := "TLS"
	switch state.Version {
	case tls.VersionTLS10:
		proto = "TLS 1.0"
	case tls.VersionTLS11:
		proto = "TLS 1.1"
	case tls.VersionTLS12:
		proto = "TLS 1.2"
	case tls.VersionTLS13:
		proto = "TLS 1.3"
	}

	return &DiscoveredCert{
		Host:            host,
		Port:            port,
		Subject:         leaf.Subject.String(),
		Issuer:          leaf.Issuer.String(),
		SANs:            sans,
		SerialNumber:    leaf.SerialNumber.String(),
		Fingerprint:     fingerprint,
		NotBefore:       leaf.NotBefore,
		NotAfter:        leaf.NotAfter,
		KeyAlgorithm:    keyAlg,
		KeySize:         keySize,
		ChainDepth:      len(state.PeerCertificates),
		ChainValid:      state.VerifiedChains != nil && len(state.VerifiedChains) > 0,
		SelfSigned:      selfSigned,
		DaysLeft:        daysLeft,
		Protocol:        proto,
		ServerName:      state.ServerName,
		DiscoveryMethod: "network",
		DiscoveredAt:    now,
	}
}

// expandCIDR generates the list of IP addresses inside a CIDR block.
func expandCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		// If single IP without CIDR mask, return single IP
		if parsed := net.ParseIP(cidr); parsed != nil {
			return []string{parsed.String()}, nil
		}
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		ips = append(ips, ip.String())
		// Safety bound: prevent scanning more than 65536 IPs per command
		if len(ips) > 65536 {
			return nil, fmt.Errorf("CIDR %s contains too many addresses (max 65536)", cidr)
		}
	}

	// Remove network address and broadcast address for subnets <= /30
	ones, bits := ipnet.Mask.Size()
	if bits-ones > 1 && len(ips) > 2 {
		return ips[1 : len(ips)-1], nil
	}
	return ips, nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

