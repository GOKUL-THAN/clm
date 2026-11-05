package dns01

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// RFC2136Provider applies dynamic DNS updates through nsupdate. The TSIG key is
// read by nsupdate from CredentialRef; key contents never enter gateway config,
// inventory, logs, or this process's command line.
type RFC2136Provider struct {
	zone       string
	nameserver string
	keyPath    string
	run        func(context.Context, string) error
}

func newRFC2136Provider(cfg Config) (*RFC2136Provider, error) {
	zone := canonicalZone(cfg.Zone)
	if zone == "" {
		return nil, fmt.Errorf("RFC2136 DNS-01 requires zone metadata")
	}
	if !validDNSName(zone) {
		return nil, fmt.Errorf("RFC2136 zone %q is invalid", cfg.Zone)
	}
	nameserver, err := nsupdateServer(cfg.Nameserver)
	if err != nil {
		return nil, err
	}
	keyPath, err := credentialFile(cfg.CredentialRef)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(keyPath); err != nil {
		return nil, fmt.Errorf("reading RFC2136 credential reference %q: %w", cfg.CredentialRef, err)
	}

	provider := &RFC2136Provider{zone: zone, nameserver: nameserver, keyPath: keyPath}
	provider.run = provider.runNSUpdate
	return provider, nil
}

func (p *RFC2136Provider) Name() string { return ProviderRFC2136 }

func (p *RFC2136Provider) Present(ctx context.Context, record Record) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	script := fmt.Sprintf("server %s\nzone %s\nupdate add %s %d TXT %s\nsend\n",
		p.nameserver, p.zone, record.FQDN, record.TTL, strconv.Quote(record.Value))
	return p.run(ctx, script)
}

func (p *RFC2136Provider) Cleanup(ctx context.Context, record Record) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	// Delete only this challenge value. Deleting the whole RRset can break a
	// concurrent authorization for the apex and wildcard of the same domain.
	script := fmt.Sprintf("server %s\nzone %s\nupdate delete %s TXT %s\nsend\n",
		p.nameserver, p.zone, record.FQDN, strconv.Quote(record.Value))
	return p.run(ctx, script)
}

func (p *RFC2136Provider) runNSUpdate(ctx context.Context, script string) error {
	// #nosec G204 -- executable is fixed and the validated key path is passed as one argument; no shell is used.
	command := exec.CommandContext(ctx, "nsupdate", "-k", p.keyPath)
	command.Stdin = strings.NewReader(script)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(output.String())
		if detail == "" {
			return fmt.Errorf("nsupdate failed: %w", err)
		}
		return fmt.Errorf("nsupdate failed: %w: %s", err, detail)
	}
	return nil
}

func validateRecord(record Record) error {
	if record.TTL <= 0 {
		return fmt.Errorf("DNS-01 TTL must be positive")
	}
	identifier := strings.TrimPrefix(record.FQDN, "_acme-challenge.")
	canonical, err := RecordName(identifier)
	if err != nil || canonical != record.FQDN {
		return fmt.Errorf("invalid DNS-01 record name %q", record.FQDN)
	}
	if record.Value == "" || strings.ContainsAny(record.Value, "\r\n\x00") {
		return fmt.Errorf("invalid DNS-01 TXT value")
	}
	return nil
}

func canonicalZone(zone string) string {
	zone = strings.ToLower(strings.TrimSpace(zone))
	if zone == "" {
		return ""
	}
	return strings.TrimSuffix(zone, ".") + "."
}

func credentialFile(reference string) (string, error) {
	// file:// is intentionally not accepted: satellite config resolves that
	// scheme by reading the file contents. file-ref:// preserves a path-only
	// reference all the way to nsupdate and keeps TSIG material out of memory.
	const prefix = "file-ref://"
	if !strings.HasPrefix(reference, prefix) {
		return "", fmt.Errorf("RFC2136 credential_ref must be a file-ref:// reference to an nsupdate TSIG key; raw key material is forbidden")
	}
	path := strings.TrimSpace(strings.TrimPrefix(reference, prefix))
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("RFC2136 credential_ref must contain an absolute file path")
	}
	if strings.ContainsAny(path, "\r\n\x00") {
		return "", fmt.Errorf("RFC2136 credential_ref contains invalid characters")
	}
	return filepath.Clean(path), nil
}

func nsupdateServer(server string) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" || strings.ContainsAny(server, "\r\n\t ") {
		return "", fmt.Errorf("RFC2136 DNS-01 requires a valid nameserver")
	}
	if host, port, err := net.SplitHostPort(server); err == nil {
		portNumber, portErr := strconv.Atoi(port)
		if !validServerHost(host) || portErr != nil || portNumber < 1 || portNumber > 65535 {
			return "", fmt.Errorf("RFC2136 nameserver %q is invalid", server)
		}
		return host + " " + port, nil
	}
	host := strings.Trim(server, "[]")
	if !validServerHost(host) {
		return "", fmt.Errorf("RFC2136 nameserver %q is invalid", server)
	}
	return host + " 53", nil
}

func validServerHost(host string) bool {
	return net.ParseIP(host) != nil || validDNSName(host)
}
