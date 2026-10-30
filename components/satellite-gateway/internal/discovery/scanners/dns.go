package scanners

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DNSScanner discovers certificates by name rather than by address.
//
// The network scanner sweeps a CIDR and probes whatever answers on each IP.
// That misses two very common cases:
//
//   - Name-based virtual hosts. Several sites share one address and the server
//     picks the certificate from the SNI value in the handshake. Dialling the
//     bare IP returns only the default certificate, so every other certificate
//     on that host is invisible to a CIDR sweep.
//   - Hosts outside the scanned ranges. Anything behind a CDN, a load balancer
//     or a third-party SaaS resolves to addresses that are not in any internal
//     CIDR, so a network sweep never sees them at all.
//
// This scanner starts from a domain, collects candidate names from DNS, and
// probes each one with SNI set, which is what makes the right certificate come
// back.
type DNSScanner struct {
	concurrency int
	timeout     time.Duration
	ports       []int
	resolver    *net.Resolver
	server      string
	logger      *zap.Logger
}

// DNSScanOptions tunes a single domain scan.
type DNSScanOptions struct {
	// Enumerate turns on wordlist-based subdomain probing. Without it only the
	// apex, names found in DNS records, and ExtraNames are tried.
	Enumerate bool
	// Wordlist replaces the built-in subdomain list when non-empty.
	Wordlist []string
	// ExtraNames are additional fully-qualified names to include.
	ExtraNames []string
	// Ports overrides the scanner's configured ports for this scan.
	Ports []int
	// FollowSANs re-probes names found in the subject alternative names of
	// certificates already discovered, limited to the target domain.
	FollowSANs bool
	// MaxNames caps how many candidates are resolved. Zero applies the default.
	MaxNames int
}

// ResolvedName is a candidate name that DNS actually answered for.
type ResolvedName struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	// Source records how the name was found: input, apex, wordlist, mx, ns,
	// srv, cname or san.
	Source  string `json:"source"`
	HasCert bool   `json:"has_cert"`
}

// DNSScanResult is the outcome of scanning one domain.
type DNSScanResult struct {
	Domain    string `json:"domain"`
	DNSServer string `json:"dns_server,omitempty"`
	// NamesLimit is the cap applied to this scan, after clamping.
	NamesLimit int `json:"names_limit"`
	NamesTried int `json:"names_tried"`
	NamesResolved []ResolvedName    `json:"names_resolved"`
	Certs         []*DiscoveredCert `json:"certs"`
	Records       map[string][]string `json:"records,omitempty"`
	// WildcardDNS is set when the zone answers for names that cannot exist. Any
	// wordlist result would then be meaningless, so enumeration is skipped.
	WildcardDNS bool     `json:"wildcard_dns"`
	Notes       []string `json:"notes,omitempty"`
	Errors      []string `json:"errors,omitempty"`
}

// DefaultMaxNames and MaxNamesLimit bound how many candidate names one domain
// scan will resolve.
//
// The previous default of 4000 was arbitrary and never reached: the built-in
// wordlist holds 221 entries, so enumeration stopped at 221 regardless of the
// cap. Raising the cap on its own would therefore have changed nothing — the
// wordlist is now expanded with modifiers (see expandWordlist) so the limit is
// the thing that actually decides the breadth of a scan.
const (
	DefaultMaxNames = 2000
	// MaxNamesLimit is a hard ceiling. Each candidate costs a DNS query, and
	// resolved ones then cost a TLS handshake per port, so an unbounded request
	// would run past the handler's timeout and produce nothing at all.
	MaxNamesLimit = 2000
)

// NewDNSScanner builds a DNS-based scanner. dnsServer is optional; when set,
// every lookup is sent to that resolver instead of the host's configured one,
// which is what allows an internal zone to be scanned from the gateway. It
// accepts "10.0.0.2" or "10.0.0.2:53".
func NewDNSScanner(concurrency int, timeout time.Duration, ports []int, dnsServer string, logger *zap.Logger) *DNSScanner {
	if concurrency <= 0 {
		// Sized so a full 2000-name enumeration completes inside the API's
		// timeout. Most candidates return NXDOMAIN quickly, but at a concurrency
		// of 25 the worst case still runs past four minutes and the caller gets
		// nothing rather than a partial answer.
		concurrency = 64
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if len(ports) == 0 {
		// Deliberately narrower than the network scanner's port list: a name
		// scan multiplies ports by candidate names, and HTTPS is where
		// name-based virtual hosting actually happens.
		ports = []int{443, 8443}
	}

	s := &DNSScanner{
		concurrency: concurrency,
		timeout:     timeout,
		ports:       ports,
		resolver:    net.DefaultResolver,
		logger:      logger,
	}

	if dnsServer = strings.TrimSpace(dnsServer); dnsServer != "" {
		if _, _, err := net.SplitHostPort(dnsServer); err != nil {
			dnsServer = net.JoinHostPort(dnsServer, "53")
		}
		s.server = dnsServer
		s.resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: timeout}
				return d.DialContext(ctx, network, dnsServer)
			},
		}
	}

	return s
}

// DNSServer reports the resolver in use, empty when the system resolver is used.
func (s *DNSScanner) DNSServer() string { return s.server }

// WithDNSServer returns a scanner that sends its queries to the given resolver.
// A request can therefore target an internal DNS server without mutating the
// shared scanner. The receiver is returned unchanged when nothing would differ.
func (s *DNSScanner) WithDNSServer(server string) *DNSScanner {
	server = strings.TrimSpace(server)
	if server == "" || server == s.server {
		return s
	}
	return NewDNSScanner(s.concurrency, s.timeout, s.ports, server, s.logger)
}

// ScanDomain enumerates names under domain and probes each resolved name for a
// TLS certificate.
func (s *DNSScanner) ScanDomain(ctx context.Context, domain string, opts DNSScanOptions) (*DNSScanResult, error) {
	domain = normaliseDomain(domain)
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	clamped := false
	if opts.MaxNames <= 0 {
		opts.MaxNames = DefaultMaxNames
	} else if opts.MaxNames > MaxNamesLimit {
		// Report the clamp rather than silently honouring a smaller number: a
		// caller who asked for 50000 needs to know the scan was not that wide.
		opts.MaxNames = MaxNamesLimit
		clamped = true
	}
	ports := opts.Ports
	if len(ports) == 0 {
		ports = s.ports
	}

	res := &DNSScanResult{
		Domain:     domain,
		DNSServer:  s.server,
		Records:    map[string][]string{},
		NamesLimit: opts.MaxNames,
	}
	if clamped {
		res.Notes = append(res.Notes, fmt.Sprintf(
			"the requested name limit was reduced to the maximum of %d", MaxNamesLimit))
	}

	// candidates keeps the first source that produced each name, so the report
	// shows how a name was found rather than the last thing to touch it.
	candidates := map[string]string{}
	add := func(name, source string) {
		name = normaliseDomain(name)
		if name == "" || len(candidates) >= opts.MaxNames {
			return
		}
		if _, exists := candidates[name]; !exists {
			candidates[name] = source
		}
	}

	add(domain, "apex")
	for _, n := range opts.ExtraNames {
		add(n, "input")
	}

	// Names published in the zone's own records. These are high-confidence
	// hostnames and cost only a handful of queries.
	s.collectFromRecords(ctx, domain, add, res)

	// A wildcard zone answers for everything, so a wordlist cannot distinguish
	// real hosts from noise. Detect it before spending thousands of queries.
	if opts.Enumerate {
		if s.wildcardZone(ctx, domain) {
			res.WildcardDNS = true
			res.Notes = append(res.Notes,
				"the zone resolves names that cannot exist (wildcard DNS), so subdomain "+
					"enumeration was skipped — every guess would appear to resolve")
		} else {
			base := opts.Wordlist
			if len(base) == 0 {
				base = DefaultSubdomains
			}
			// Expanded to whatever the limit allows. Plain names come first, so a
			// small limit still spends its budget on the highest-value candidates.
			for _, sub := range expandWordlist(base, opts.MaxNames) {
				add(sub+"."+domain, "wordlist")
			}
		}
	}

	resolved := s.resolveAll(ctx, candidates)
	res.NamesTried = len(candidates)

	certs, withCert := s.probeNames(ctx, resolved, ports)
	res.Certs = certs

	// Certificates name other hosts. Those SANs are the highest-quality source
	// of further candidates available, because they are names someone actually
	// provisioned a certificate for.
	if opts.FollowSANs && len(certs) > 0 {
		extra := map[string]string{}
		known := func(n string) bool {
			_, ok := candidates[n]
			return ok
		}
		for _, c := range certs {
			for _, san := range c.SANs {
				san = normaliseDomain(strings.TrimPrefix(san, "*."))
				if san == "" || known(san) {
					continue
				}
				// Stay inside the target zone; a shared certificate can name
				// hundreds of unrelated domains.
				if san != domain && !strings.HasSuffix(san, "."+domain) {
					continue
				}
				if _, dup := extra[san]; !dup && len(candidates)+len(extra) < opts.MaxNames {
					extra[san] = "san"
				}
			}
		}
		if len(extra) > 0 {
			for n, src := range extra {
				candidates[n] = src
			}
			res.NamesTried = len(candidates)
			extraResolved := s.resolveAll(ctx, extra)
			extraCerts, extraWithCert := s.probeNames(ctx, extraResolved, ports)
			res.Certs = append(res.Certs, extraCerts...)
			resolved = append(resolved, extraResolved...)
			for n := range extraWithCert {
				withCert[n] = struct{}{}
			}
		}
	}

	for i := range resolved {
		if _, ok := withCert[resolved[i].Name]; ok {
			resolved[i].HasCert = true
		}
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].Name < resolved[j].Name })
	res.NamesResolved = resolved

	res.Notes = append(res.Notes,
		"DNS zone transfer (AXFR) is not attempted; enumeration is wordlist and record based")

	s.logger.Info("dns scan complete",
		zap.String("domain", domain),
		zap.String("dns_server", s.server),
		zap.Int("names_tried", res.NamesTried),
		zap.Int("names_resolved", len(resolved)),
		zap.Int("certs_found", len(res.Certs)),
		zap.Bool("wildcard_dns", res.WildcardDNS),
	)

	return res, nil
}

// collectFromRecords pulls hostnames out of the zone's CNAME, MX, NS and SRV
// records and records what was seen for the report.
func (s *DNSScanner) collectFromRecords(ctx context.Context, domain string, add func(string, string), res *DNSScanResult) {
	lookupCtx, cancel := context.WithTimeout(ctx, s.timeout*3)
	defer cancel()

	if cname, err := s.resolver.LookupCNAME(lookupCtx, domain); err == nil {
		if c := normaliseDomain(cname); c != "" && c != domain {
			add(c, "cname")
			res.Records["cname"] = []string{c}
		}
	}

	if mxs, err := s.resolver.LookupMX(lookupCtx, domain); err == nil && len(mxs) > 0 {
		seen := make([]string, 0, len(mxs))
		for _, mx := range mxs {
			h := normaliseDomain(mx.Host)
			if h == "" {
				continue
			}
			add(h, "mx")
			seen = append(seen, h)
		}
		if len(seen) > 0 {
			res.Records["mx"] = seen
		}
	}

	if nss, err := s.resolver.LookupNS(lookupCtx, domain); err == nil && len(nss) > 0 {
		seen := make([]string, 0, len(nss))
		for _, ns := range nss {
			h := normaliseDomain(ns.Host)
			if h == "" {
				continue
			}
			add(h, "ns")
			seen = append(seen, h)
		}
		if len(seen) > 0 {
			res.Records["ns"] = seen
		}
	}

	// SRV records point at the hosts actually serving a protocol, which is
	// often where an internal TLS endpoint lives.
	var srvSeen []string
	for _, svc := range defaultSRVServices {
		parts := strings.SplitN(svc, ".", 2)
		if len(parts) != 2 {
			continue
		}
		_, addrs, err := s.resolver.LookupSRV(lookupCtx,
			strings.TrimPrefix(parts[0], "_"), strings.TrimPrefix(parts[1], "_"), domain)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			h := normaliseDomain(a.Target)
			if h == "" {
				continue
			}
			add(h, "srv")
			srvSeen = append(srvSeen, svc+" -> "+h+":"+strconv.Itoa(int(a.Port)))
		}
	}
	if len(srvSeen) > 0 {
		res.Records["srv"] = srvSeen
	}
}

// wildcardZone reports whether the zone answers for a name that cannot exist.
func (s *DNSScanner) wildcardZone(ctx context.Context, domain string) bool {
	lookupCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Two distinct improbable labels: one lucky collision should not disable
	// enumeration for a whole scan.
	probes := []string{
		"clm-wildcard-probe-zzq7x1." + domain,
		"clm-wildcard-probe-4k9m2v." + domain,
	}
	hits := 0
	for _, p := range probes {
		if addrs, err := s.resolver.LookupHost(lookupCtx, p); err == nil && len(addrs) > 0 {
			hits++
		}
	}
	return hits == len(probes)
}

// resolveAll resolves candidates concurrently and keeps the ones that answer.
func (s *DNSScanner) resolveAll(ctx context.Context, candidates map[string]string) []ResolvedName {
	type job struct{ name, source string }

	jobs := make(chan job, len(candidates))
	for name, source := range candidates {
		jobs <- job{name: name, source: source}
	}
	close(jobs)

	var (
		mu  sync.Mutex
		out []ResolvedName
		wg  sync.WaitGroup
	)

	workers := s.concurrency
	if workers > len(candidates) {
		workers = len(candidates)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				lookupCtx, cancel := context.WithTimeout(ctx, s.timeout)
				addrs, err := s.resolver.LookupHost(lookupCtx, j.name)
				cancel()
				if err != nil || len(addrs) == 0 {
					continue
				}
				if len(addrs) > 8 {
					addrs = addrs[:8]
				}
				mu.Lock()
				out = append(out, ResolvedName{Name: j.name, Addresses: addrs, Source: j.source})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	return out
}

// probeNames performs an SNI handshake against every resolved name on every
// port and returns the certificates found plus the set of names that served one.
func (s *DNSScanner) probeNames(ctx context.Context, names []ResolvedName, ports []int) ([]*DiscoveredCert, map[string]struct{}) {
	type target struct {
		name string
		addr string
		port int
	}

	jobs := make(chan target, len(names)*len(ports))
	for _, n := range names {
		for _, p := range ports {
			addr := n.Name
			if len(n.Addresses) > 0 {
				// Dial the resolved address but keep SNI as the name, so the
				// result is not at the mercy of a second resolution.
				addr = n.Addresses[0]
			}
			jobs <- target{name: n.Name, addr: addr, port: p}
		}
	}
	close(jobs)

	var (
		mu       sync.Mutex
		certs    []*DiscoveredCert
		withCert = map[string]struct{}{}
		wg       sync.WaitGroup
	)

	workers := s.concurrency
	if total := len(names) * len(ports); workers > total {
		workers = total
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				cert, err := probeTLSSNI(t.addr, t.name, t.port, s.timeout)
				if err != nil || cert == nil {
					continue
				}
				mu.Lock()
				certs = append(certs, cert)
				withCert[t.name] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	sort.Slice(certs, func(i, j int) bool {
		if certs[i].Host == certs[j].Host {
			return certs[i].Port < certs[j].Port
		}
		return certs[i].Host < certs[j].Host
	})

	return certs, withCert
}

// normaliseDomain lower-cases a name and strips the trailing dot that DNS
// answers carry, a scheme if one was pasted in, and any port suffix.
func normaliseDomain(d string) string {
	d = strings.TrimSpace(strings.ToLower(d))
	if d == "" {
		return ""
	}
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	d = strings.TrimSuffix(d, ".")
	d = strings.TrimSuffix(d, "/")
	if i := strings.Index(d, "/"); i >= 0 {
		d = d[:i]
	}
	// Strip a port, but leave bare IPv6 text alone.
	if host, _, err := net.SplitHostPort(d); err == nil && !strings.Contains(host, ":") {
		d = host
	}
	return strings.TrimSuffix(d, ".")
}

// wordlistModifiers are appended to each base name to widen enumeration.
//
// These are the shapes that actually recur in an enterprise estate — numbered
// instances and environment-qualified names — rather than random character
// mutations, which cost the same DNS query and almost never resolve.
//
// Order matters: the earlier a modifier appears, the more likely its names are
// tried when the limit is small.
var wordlistModifiers = []struct {
	prefix string
	suffix string
}{
	{"", "1"}, {"", "2"},
	{"", "-prod"}, {"", "-dev"}, {"", "-test"}, {"", "-uat"},
	{"", "-stage"}, {"", "3"},
	{"prod-", ""}, {"dev-", ""}, {"test-", ""}, {"uat-", ""},
	{"", "01"}, {"", "02"}, {"", "-int"}, {"", "-ext"},
	{"", "-new"}, {"", "-old"}, {"", "-v2"}, {"", "-dr"},
}

// expandWordlist grows a base list up to limit by applying the modifiers.
//
// Plain base names are emitted first because they are by far the most likely to
// exist; the modified forms then fill whatever budget is left. Duplicates are
// dropped, so a caller-supplied list that already contains "api1" does not
// produce it twice.
func expandWordlist(base []string, limit int) []string {
	if limit <= 0 {
		limit = DefaultMaxNames
	}

	out := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)

	push := func(name string) bool {
		if name == "" {
			return true
		}
		if _, dup := seen[name]; dup {
			return true
		}
		seen[name] = struct{}{}
		out = append(out, name)
		return len(out) < limit
	}

	for _, b := range base {
		if !push(b) {
			return out
		}
	}
	for _, m := range wordlistModifiers {
		for _, b := range base {
			if !push(m.prefix + b + m.suffix) {
				return out
			}
		}
	}
	return out
}

// defaultSRVServices are the SRV names worth asking for when looking for TLS
// endpoints.
var defaultSRVServices = []string{
	"_https._tcp",
	"_http._tcp",
	"_ldaps._tcp",
	"_ldap._tcp",
	"_sips._tcp",
	"_sip._tcp",
	"_xmpp-client._tcp",
	"_xmpp-server._tcp",
	"_autodiscover._tcp",
	"_caldavs._tcp",
	"_carddavs._tcp",
	"_imaps._tcp",
	"_submission._tcp",
	"_kerberos._tcp",
}

// DefaultSubdomains is the built-in enumeration wordlist. It favours names that
// tend to terminate TLS in an enterprise estate over generic web hostnames.
var DefaultSubdomains = []string{
	"www", "api", "api2", "apis", "api-gw", "gateway", "gw", "app", "apps",
	"admin", "portal", "dashboard", "console", "manage", "management",
	"auth", "sso", "login", "id", "identity", "idp", "oauth", "keycloak", "adfs",
	"mail", "smtp", "imap", "pop", "pop3", "webmail", "exchange", "owa", "mx", "mx1", "mx2",
	"ns", "ns1", "ns2", "dns", "dns1", "dns2",
	"vpn", "remote", "rdp", "ssh", "bastion", "jump", "proxy", "reverse-proxy",
	"cdn", "static", "assets", "img", "images", "media", "files", "download", "uploads",
	"s3", "storage", "backup", "archive",
	"db", "database", "mysql", "postgres", "postgresql", "oracle", "mssql",
	"redis", "mongo", "mongodb", "cassandra", "elastic", "elasticsearch", "opensearch",
	"kafka", "rabbitmq", "mq", "activemq", "zookeeper", "cache", "memcached",
	"search", "kibana", "grafana", "prometheus", "metrics", "monitor", "monitoring",
	"alerts", "alertmanager", "logs", "logging", "splunk", "loki", "jaeger",
	"jenkins", "ci", "cd", "build", "git", "gitlab", "gitea", "bitbucket",
	"jira", "confluence", "wiki", "docs", "help", "support", "status", "uptime",
	"test", "tests", "testing", "qa", "uat", "sit", "stage", "staging",
	"dev", "develop", "development", "preprod", "pre-prod", "prod", "production",
	"demo", "sandbox", "beta", "alpha", "canary",
	"internal", "intranet", "extranet", "partner", "partners", "b2b", "edi",
	"secure", "pay", "payment", "payments", "billing", "invoice", "checkout",
	"shop", "store", "crm", "erp", "hr", "finance", "legal", "procurement",
	"mobile", "m", "ios", "android", "ws", "wss", "websocket", "socket",
	"stream", "streaming", "live", "video", "voice", "sip", "pbx", "telephony",
	"chat", "bot", "ai", "ml", "data", "analytics", "bi", "report", "reports", "etl",
	"k8s", "kubernetes", "rancher", "openshift", "docker", "registry", "harbor",
	"nexus", "artifactory", "vault", "consul", "nomad", "argocd", "argo",
	"sftp", "ftp", "ftps", "tftp", "nfs", "smb",
	"lb", "loadbalancer", "haproxy", "nginx", "traefik", "envoy", "istio",
	"ldap", "ldaps", "ad", "dc", "radius", "tacacs",
	"cert", "certs", "pki", "ca", "ocsp", "crl", "acme",
}
