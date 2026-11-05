package scanners

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

// DiscoveredCert represents a certificate found during network scanning.
type DiscoveredCert struct {
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	Subject      string    `json:"subject"`
	Issuer       string    `json:"issuer"`
	SANs         []string  `json:"sans"`
	SerialNumber string    `json:"serial_number"`
	Fingerprint  string    `json:"fingerprint"`
	NotBefore    time.Time `json:"not_before"`
	NotAfter     time.Time `json:"not_after"`
	KeyAlgorithm string    `json:"key_algorithm"`
	KeySize      int       `json:"key_size"`
	ChainDepth   int       `json:"chain_depth"`
	ChainValid   bool      `json:"chain_valid"`
	SelfSigned   bool      `json:"self_signed"`
	DaysLeft     int       `json:"days_left"`
	Protocol     string    `json:"protocol"`
	ServerName   string    `json:"server_name"`
	// ResolvedIP is the address that was dialled when the certificate was found
	// by name. Empty for address-based scans, where Host is already the address.
	ResolvedIP string `json:"resolved_ip,omitempty"`
	// DiscoveryMethod records how the certificate was found: "network" for a
	// CIDR/host probe, "dns" for a name-based probe with SNI.
	DiscoveryMethod string    `json:"discovery_method,omitempty"`
	DiscoveredAt    time.Time `json:"discovered_at"`
}

// NetworkScanner performs TLS handshakes on network targets to discover certificates.
type NetworkScanner struct {
	concurrency int
	timeout     time.Duration
	ports       []int
	logger      *zap.Logger
}

// NewNetworkScanner creates a new network scanner.
func NewNetworkScanner(concurrency int, timeout time.Duration, ports []int, logger *zap.Logger) *NetworkScanner {
	if len(ports) == 0 {
		ports = []int{443, 8443, 636, 993, 995, 5671, 6443, 9093}
	}
	return &NetworkScanner{
		concurrency: concurrency,
		timeout:     timeout,
		ports:       ports,
		logger:      logger,
	}
}

// ScanNetwork scans a CIDR range for TLS certificates.
func (s *NetworkScanner) ScanNetwork(ctx context.Context, cidr string) ([]*DiscoveredCert, error) {
	hosts, err := expandCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	// Create work channel
	type scanTarget struct {
		host string
		port int
	}
	targets := make(chan scanTarget, len(hosts)*len(s.ports))
	results := make(chan *DiscoveredCert, 100)

	// Feed targets
	go func() {
		for _, host := range hosts {
			for _, port := range s.ports {
				targets <- scanTarget{host: host, port: port}
			}
		}
		close(targets)
	}()

	// Workers
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
				}

				cert, err := s.probeTLS(ctx, target.host, target.port)
				if err == nil && cert != nil {
					results <- cert
				}
			}
		}()
	}

	// Close results when all workers done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var discovered []*DiscoveredCert
	for cert := range results {
		discovered = append(discovered, cert)
	}

	s.logger.Info("network scan complete",
		zap.String("cidr", cidr),
		zap.Int("hosts_scanned", len(hosts)),
		zap.Int("certs_found", len(discovered)),
	)

	return discovered, nil
}

// ScanHost scans a single host for TLS certificates on all configured ports.
func (s *NetworkScanner) ScanHost(ctx context.Context, host string) ([]*DiscoveredCert, error) {
	var certs []*DiscoveredCert

	for _, port := range s.ports {
		cert, err := s.probeTLS(ctx, host, port)
		if err == nil && cert != nil {
			certs = append(certs, cert)
		}
	}

	return certs, nil
}

// probeTLS performs a TLS handshake and extracts the certificate.
func (s *NetworkScanner) probeTLS(ctx context.Context, host string, port int) (*DiscoveredCert, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 -- discovery must collect expired and self-signed certificates without sending credentials.
	})
	if err != nil {
		return nil, err // Host not responding on this port
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, nil
	}

	return buildDiscoveredCert(state, host, port, "network", ""), nil
}

// probeTLSSNI performs a handshake against dialAddr while presenting serverName
// in the SNI extension, and attributes the result to serverName rather than to
// the address.
//
// This is the difference that makes name-based discovery work. A server hosting
// several names on one address selects its certificate from the SNI value, so
// dialling the address without SNI only ever yields the default certificate.
func probeTLSSNI(dialAddr, serverName string, port int, timeout time.Duration) (*DiscoveredCert, error) {
	addr := net.JoinHostPort(dialAddr, strconv.Itoa(port))

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		// Discovery must report certificates that are expired, self-signed or
		// misissued, so verification is intentionally not performed here.
		InsecureSkipVerify: true, // #nosec G402 -- this probe only retrieves certificate metadata.
		ServerName:         serverName,
	})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, nil
	}

	resolved := ""
	if dialAddr != serverName {
		resolved = dialAddr
	}
	return buildDiscoveredCert(state, serverName, port, "dns", resolved), nil
}

// buildDiscoveredCert turns a completed handshake into a DiscoveredCert. Shared
// by the address-based and name-based probes so both report identical fields.
func buildDiscoveredCert(state tls.ConnectionState, host string, port int, method, resolvedIP string) *DiscoveredCert {
	leaf := state.PeerCertificates[0]
	fingerprint := sha256.Sum256(leaf.Raw)

	// Check chain validity
	chainValid := false
	if len(state.PeerCertificates) > 1 {
		intermediates := x509.NewCertPool()
		for _, cert := range state.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		opts := x509.VerifyOptions{
			Intermediates: intermediates,
		}
		_, err := leaf.Verify(opts)
		chainValid = err == nil
	}

	selfSigned := leaf.Issuer.CommonName == leaf.Subject.CommonName

	daysLeft := int(time.Until(leaf.NotAfter).Hours() / 24)

	keySize := 0
	switch pub := leaf.PublicKey.(type) {
	case interface{ Size() int }:
		keySize = pub.Size() * 8
	}

	return &DiscoveredCert{
		Host:            host,
		Port:            port,
		Subject:         leaf.Subject.CommonName,
		Issuer:          leaf.Issuer.CommonName,
		SANs:            leaf.DNSNames,
		SerialNumber:    leaf.SerialNumber.Text(16),
		Fingerprint:     hex.EncodeToString(fingerprint[:]),
		NotBefore:       leaf.NotBefore,
		NotAfter:        leaf.NotAfter,
		KeyAlgorithm:    leaf.PublicKeyAlgorithm.String(),
		KeySize:         keySize,
		ChainDepth:      len(state.PeerCertificates),
		ChainValid:      chainValid,
		SelfSigned:      selfSigned,
		DaysLeft:        daysLeft,
		Protocol:        "https",
		ServerName:      leaf.Subject.CommonName,
		ResolvedIP:      resolvedIP,
		DiscoveryMethod: method,
		DiscoveredAt:    time.Now(),
	}
}

// expandCIDR converts a CIDR notation to a list of host IPs.
func expandCIDR(cidr string) ([]string, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var hosts []string
	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incrementIP(ip) {
		hosts = append(hosts, ip.String())
		// Limit to prevent scanning huge ranges
		if len(hosts) > 65536 {
			break
		}
	}

	// Remove network and broadcast addresses
	if len(hosts) > 2 {
		hosts = hosts[1 : len(hosts)-1]
	}

	return hosts, nil
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
