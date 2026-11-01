const fs = require('fs');
const path = require('path');

const filePath = path.join(__dirname, '..', 'components/saas-control-plane/internal/saas/integrations/service.go');
let content = fs.readFileSync(filePath, 'utf8');

const isCRLF = content.includes('\r\n');
const eol = isCRLF ? '\r\n' : '\n';

// Replace imports
const oldImport = [
    'import (',
    '\t"bytes"',
    '\t"context"',
    '\t"crypto/hmac"',
    '\t"crypto/sha256"',
    '\t"encoding/hex"',
    '\t"encoding/json"',
    '\t"errors"',
    '\t"fmt"',
    '\t"net/http"',
    '\t"sync"',
    '\t"time"',
    '',
    '\t"github.com/google/uuid"',
    '\t"go.uber.org/zap"',
    ')'
].join(eol);

const newImport = [
    'import (',
    '\t"bytes"',
    '\t"context"',
    '\t"crypto/hmac"',
    '\t"crypto/sha256"',
    '\t"encoding/hex"',
    '\t"encoding/json"',
    '\t"errors"',
    '\t"fmt"',
    '\t"net"',
    '\t"net/http"',
    '\t"net/url"',
    '\t"strings"',
    '\t"sync"',
    '\t"time"',
    '',
    '\t"github.com/google/uuid"',
    '\t"go.uber.org/zap"',
    ')'
].join(eol);

content = content.replace(oldImport, newImport);

// Add security functions
const securityBlock = [
    'var (',
    '\tErrIntegrationNotFound = errors.New("integration subscription not found")',
    '\tErrInvalidEndpointURL  = errors.New("endpoint URL must be a valid HTTPS URL (private IPs, localhost, and cloud metadata prohibited)")',
    ')',
    '',
    '// IsPrivateOrReservedIP verifies whether an IP address belongs to loopback, private RFC 1918,',
    '// link-local, cloud metadata (169.254.0.0/16), carrier-grade NAT, or IPv6 unique local space.',
    'func IsPrivateOrReservedIP(ip net.IP) bool {',
    '\tif ip == nil {',
    '\t\treturn true',
    '\t}',
    '\tif v4 := ip.To4(); v4 != nil {',
    '\t\tip = v4',
    '\t}',
    '\tif ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {',
    '\t\treturn true',
    '\t}',
    '\tmetadataNet := net.IPNet{',
    '\t\tIP:   net.ParseIP("169.254.0.0"),',
    '\t\tMask: net.CIDRMask(16, 32),',
    '\t}',
    '\tif metadataNet.Contains(ip) {',
    '\t\treturn true',
    '\t}',
    '\tcgnatNet := net.IPNet{',
    '\t\tIP:   net.ParseIP("100.64.0.0"),',
    '\t\tMask: net.CIDRMask(10, 32),',
    '\t}',
    '\tif cgnatNet.Contains(ip) {',
    '\t\treturn true',
    '\t}',
    '\tif len(ip) == net.IPv6len {',
    '\t\tif ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {',
    '\t\t\treturn true',
    '\t\t}',
    '\t\tif (ip[0] & 0xfe) == 0xfc {',
    '\t\t\treturn true',
    '\t\t}',
    '\t}',
    '\treturn false',
    '}',
    '',
    '// ValidateWebhookURL strictly validates URL scheme, internal domains, and IP literals.',
    'func ValidateWebhookURL(endpoint string) error {',
    '\tif endpoint == "" {',
    '\t\treturn ErrInvalidEndpointURL',
    '\t}',
    '\tparsed, err := url.ParseRequestURI(endpoint)',
    '\tif err != nil {',
    '\t\treturn fmt.Errorf("%w: malformed URL: %v", ErrInvalidEndpointURL, err)',
    '\t}',
    '\tif strings.ToLower(parsed.Scheme) != "https" {',
    '\t\treturn fmt.Errorf("%w: scheme must strictly be https", ErrInvalidEndpointURL)',
    '\t}',
    '\thost := strings.ToLower(parsed.Hostname())',
    '\tif host == "localhost" || strings.HasSuffix(host, ".localhost") ||',
    '\t\tstrings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") ||',
    '\t\thost == "metadata.google.internal" || host == "instance-data" {',
    '\t\treturn fmt.Errorf("%w: internal hostname %s is forbidden", ErrInvalidEndpointURL, host)',
    '\t}',
    '\tif ip := net.ParseIP(host); ip != nil {',
    '\t\tif IsPrivateOrReservedIP(ip) {',
    '\t\t\treturn fmt.Errorf("%w: destination IP %s is prohibited", ErrInvalidEndpointURL, ip.String())',
    '\t\t}',
    '\t}',
    '\treturn nil',
    '}',
    '',
    '// NewSafeHTTPClient constructs an HTTP client that validates destination IPs at connection time',
    '// and prohibits redirects, completely neutralizing DNS rebinding and redirect SSRF attacks.',
    'func NewSafeHTTPClient() *http.Client {',
    '\tdialer := &net.Dialer{',
    '\t\tTimeout:   5 * time.Second,',
    '\t\tKeepAlive: 30 * time.Second,',
    '\t}',
    '\ttransport := &http.Transport{',
    '\t\tDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {',
    '\t\t\thost, port, err := net.SplitHostPort(addr)',
    '\t\t\tif err != nil {',
    '\t\t\t\treturn nil, fmt.Errorf("invalid destination address: %w", err)',
    '\t\t\t}',
    '\t\t\tips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)',
    '\t\t\tif err != nil {',
    '\t\t\t\treturn nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)',
    '\t\t\t}',
    '\t\t\tif len(ips) == 0 {',
    '\t\t\t\treturn nil, fmt.Errorf("zero IP addresses returned for host %s", host)',
    '\t\t\t}',
    '\t\t\tfor _, ip := range ips {',
    '\t\t\t\tif IsPrivateOrReservedIP(ip) {',
    '\t\t\t\t\treturn nil, fmt.Errorf("SSRF defense: resolved IP %s is private/metadata/prohibited", ip.String())',
    '\t\t\t\t}',
    '\t\t\t}',
    '\t\t\tpinnedAddr := net.JoinHostPort(ips[0].String(), port)',
    '\t\t\treturn dialer.DialContext(ctx, network, pinnedAddr)',
    '\t\t},',
    '\t\tTLSHandshakeTimeout: 5 * time.Second,',
    '\t}',
    '\treturn &http.Client{',
    '\t\tTransport: transport,',
    '\t\tTimeout:   10 * time.Second,',
    '\t\tCheckRedirect: func(req *http.Request, via []*http.Request) error {',
    '\t\t\treturn errors.New("SSRF defense: HTTP redirects are strictly prohibited for webhook delivery")',
    '\t\t},',
    '\t}',
    '}'
].join(eol);

const oldVars = [
    'var (',
    '\tErrIntegrationNotFound = errors.New("integration subscription not found")',
    '\tErrInvalidEndpointURL   = errors.New("endpoint URL must be a valid HTTP/HTTPS URL")',
    ')'
].join(eol);

content = content.replace(oldVars, securityBlock);

// Update NewIntegrationController
content = content.replace(
    'httpClient:    &http.Client{Timeout: 10 * time.Second},',
    'httpClient:    NewSafeHTTPClient(),'
);

// Update RegisterSubscription
const oldCheck = [
    '\tif sub.EndpointURL == "" {',
    '\t\treturn nil, ErrInvalidEndpointURL',
    '\t}'
].join(eol);

const newCheck = [
    '\tif err := ValidateWebhookURL(sub.EndpointURL); err != nil {',
    '\t\treturn nil, err',
    '\t}'
].join(eol);

content = content.replace(oldCheck, newCheck);

fs.writeFileSync(filePath, content, 'utf8');
console.log('Successfully updated service.go with connection-time SSRF defenses');

