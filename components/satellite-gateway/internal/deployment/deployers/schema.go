package deployers

import "strings"

// Target schemas.
//
// The deploy endpoint accepted a free-form target and certificate material, so a
// caller had to already know which fields a given deployer reads and what
// credentials it needs — and for the cloud deployers there was no credential
// field at all. Describing each target type as data lets the portal render the
// right form and lets the API reject a request that could not have worked.

// TargetField describes one input a target type needs.
type TargetField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Secret   bool   `json:"secret"`
	// Credential marks a field that belongs in target.credentials rather than on
	// the target itself.
	Credential  bool     `json:"credential"`
	Multiline   bool     `json:"multiline,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Options     []string `json:"options,omitempty"`
	Default     string   `json:"default,omitempty"`
}

// TargetSpec describes a deployable target type.
type TargetSpec struct {
	Type     string        `json:"type"`
	Category string        `json:"category"`
	Title    string        `json:"title"`
	Summary  string        `json:"summary"`
	Fields   []TargetField `json:"fields"`
	// Rollback reports whether reverting is implemented, so the portal does not
	// offer a button that only returns an error.
	Rollback bool `json:"rollback"`
	// RemoteCapable is true when the work can be handed to a vSatellite agent
	// instead of running on the gateway.
	RemoteCapable bool   `json:"remote_capable"`
	Notes         string `json:"notes,omitempty"`
}

// TargetSchemas lists every deployable target type with its inputs.
func TargetSchemas() []TargetSpec {
	specs := []TargetSpec{
		{
			Type: "linux", Category: "os", Title: "Linux host (nginx, Apache, HAProxy)",
			Summary:  "Writes the certificate and key to disk and reloads the service.",
			Rollback: true, RemoteCapable: true,
			Fields: []TargetField{
				{Key: "host", Label: "Host", Required: true, Placeholder: "10.0.0.20"},
				{Key: "cert_path", Label: "Certificate path", Required: true,
					Placeholder: "/etc/ssl/certs/site.pem"},
				{Key: "key_path", Label: "Private key path", Required: true,
					Placeholder: "/etc/ssl/private/site.key"},
				{Key: "service", Label: "Service to reload", Placeholder: "nginx"},
				{Key: "reload_cmd", Label: "Reload command override",
					Placeholder: "systemctl reload nginx",
					Help:        "Only systemctl reload/restart actions for the selected service are accepted."},
				{Key: "ssh_user", Label: "SSH user", Credential: true, Placeholder: "ubuntu",
					Help: "Needed when the target is not this gateway itself."},
				{Key: "ssh_private_key", Label: "SSH private key", Credential: true,
					Secret: true, Multiline: true,
					Help: "PEM. Used only for the deployment and never stored with the certificate."},
			},
			Notes: "Reaching another host requires either SSH credentials or a vSatellite " +
				"agent inside that segment.",
		},
		{
			Type: "postgresql", Category: "data", Title: "PostgreSQL",
			Summary:  "Installs server certificate and key, then signals a config reload.",
			Rollback: true, RemoteCapable: true,
			Fields: []TargetField{
				{Key: "host", Label: "Host", Required: true},
				{Key: "port", Label: "Port", Default: "5432"},
				{Key: "cert_path", Label: "Certificate path", Required: true,
					Placeholder: "/var/lib/postgresql/server.crt"},
				{Key: "key_path", Label: "Key path", Required: true,
					Placeholder: "/var/lib/postgresql/server.key"},
				{Key: "config_file", Label: "postgresql.conf path",
					Placeholder: "/var/lib/postgresql/data/postgresql.conf"},
				{Key: "reload_method", Label: "Reload method",
					Options: []string{"sighup", "restart", "none"}, Default: "sighup"},
				{Key: "ssh_user", Label: "SSH user", Credential: true},
				{Key: "ssh_private_key", Label: "SSH private key", Credential: true,
					Secret: true, Multiline: true},
			},
		},
		{
			Type: "aws-acm", Category: "cloud", Title: "AWS ACM and ELB listener",
			Summary: "Imports into ACM, attaches to an ALB/NLB listener, or both, with a " +
				"choice of whether the superseded certificate stays available.",
			Rollback: true, RemoteCapable: true,
			Fields: []TargetField{
				{Key: "region", Label: "Region", Required: true, Placeholder: "ap-south-1"},
				{Key: "apply_to", Label: "Apply to",
					Options: []string{"acm", "alb", "acm+alb"}, Default: "acm+alb",
					Help: "acm imports only, so nothing serves it yet. alb attaches an " +
						"existing certificate_arn without importing. acm+alb does both."},
				{Key: "version_policy", Label: "ACM version",
					Options: []string{"new-version", "replace-in-place"}, Default: "new-version",
					Help: "new-version imports alongside the old one, which keeps its ARN " +
						"working and makes rollback possible. replace-in-place re-imports over " +
						"certificate_arn, so consumers need no repointing but the old material is gone."},
				{Key: "listener_policy", Label: "Listener change",
					Options: []string{"keep-previous", "replace", "add-only"}, Default: "keep-previous",
					Help: "keep-previous makes the new certificate the default and leaves the " +
						"outgoing one attached, so reverting is one call. replace leaves only the " +
						"new certificate. add-only adds to the SNI list without touching the default."},
				{Key: "listener_arn", Label: "Listener ARN",
					Placeholder: "arn:aws:elasticloadbalancing:ap-south-1:...:listener/app/my-alb/...",
					Help: "Required unless apply_to is acm. Also read from resource_id for " +
						"older requests."},
				{Key: "certificate_arn", Label: "ACM certificate ARN",
					Placeholder: "arn:aws:acm:ap-south-1:...:certificate/...",
					Help: "Required for apply_to=alb and for version_policy=replace-in-place. " +
						"Left empty otherwise; a new-version import fills it in."},
				{Key: "retain_versions", Label: "Superseded versions to keep", Default: "2",
					Help: "Older CLM-imported certificates for the same name beyond this many " +
						"are deleted after a successful deploy. 0 keeps every version. Anything " +
						"AWS still reports as in use is never deleted."},
				{Key: "access_key_id", Label: "Access key ID", Credential: true,
					Help: "Leave the key fields empty to use the gateway's instance role."},
				{Key: "secret_access_key", Label: "Secret access key", Credential: true, Secret: true},
				{Key: "session_token", Label: "Session token", Credential: true, Secret: true,
					Help: "Only for temporary credentials."},
				{Key: "role_arn", Label: "Assume role ARN", Credential: true,
					Placeholder: "arn:aws:iam::123456789012:role/clm-cert-deploy",
					Help: "For cross-account deploys: assumed on top of whichever credentials " +
						"above resolve."},
				{Key: "external_id", Label: "External ID", Credential: true,
					Help: "Only if the assumed role requires one."},
			},
			Notes: "Needs acm:ImportCertificate, acm:DescribeCertificate and acm:ListTagsForCertificate; " +
				"elasticloadbalancing:DescribeListenerCertificates, AddListenerCertificates and " +
				"ModifyListener to attach; acm:DeleteCertificate only if retain_versions is set. " +
				"Note that when two certificates covering the same name are attached to one " +
				"listener, which one a client gets is not defined by AWS — keep-previous is for " +
				"a fast revert, not for serving both.",
		},
		{
			Type: "gcp-lb", Category: "cloud", Title: "GCP load balancer (Compute SSL certificate)",
			Summary:  "Creates a new SSL certificate resource and repoints the target HTTPS proxy.",
			Rollback: false, RemoteCapable: true,
			Fields: []TargetField{
				{Key: "resource_id", Label: "Target HTTPS proxy name",
					Placeholder: "my-https-proxy",
					Help:        "Optional. Without it the certificate is created but nothing serves it."},
				{Key: "service_account_json", Label: "Service account JSON key",
					Required: true, Credential: true, Secret: true, Multiline: true,
					Help: "Needs compute.sslCertificates.create and compute.targetHttpsProxies.setSslCertificates."},
				{Key: "project_id", Label: "Project ID", Credential: true,
					Help: "Defaults to the project in the service account key."},
			},
			Notes: "A Compute SSL certificate is immutable, so every rotation creates a new " +
				"resource and swaps the proxy. The previous resource is left in place.",
		},
		{
			Type: "azure-keyvault", Category: "cloud", Title: "Azure Key Vault",
			Summary:  "Imports the certificate into Key Vault for App Service, App Gateway or Front Door.",
			Rollback: false, RemoteCapable: true,
			Fields: []TargetField{
				{Key: "resource_id", Label: "Certificate name in the vault", Required: true,
					Placeholder: "wildcard-example-com"},
				{Key: "vault_name", Label: "Key Vault name", Required: true,
					Placeholder: "my-keyvault"},
				{Key: "tenant_id", Label: "Tenant ID", Required: true, Credential: true},
				{Key: "client_id", Label: "Client ID (app registration)", Required: true, Credential: true},
				{Key: "client_secret", Label: "Client secret", Required: true,
					Credential: true, Secret: true},
			},
			Notes: "The app registration needs the Key Vault certificates import permission. " +
				"A consumer pinned to a specific certificate version still needs repointing.",
		},
		{
			Type: "windows", Category: "os", Title: "Windows Server (IIS, Exchange, RDP)",
			Summary:  "Imports the certificate into the Windows certificate store and optionally binds it to an IIS site or enables it for RDP/Exchange.",
			Rollback: false, RemoteCapable: true,
			Fields: []TargetField{
				{Key: "host", Label: "Host", Required: true, Placeholder: "10.0.1.50"},
				{Key: "store", Label: "Certificate store",
					Options: []string{"My", "WebHosting", "Root"}, Default: "My",
					Help: "The Windows certificate store to import into. 'My' (Personal) is standard for IIS and RDP."},
				{Key: "binding", Label: "Binding target",
					Options: []string{"none", "iis", "rdp", "exchange"},
					Default: "none",
					Help: "'none' imports without binding. 'iis' binds to site_name:port. " +
						"'rdp' sets the RDP listener certificate. 'exchange' assigns to Exchange services."},
				{Key: "site_name", Label: "IIS site name", Placeholder: "Default Web Site",
					Help: "Required when binding=iis."},
				{Key: "port", Label: "Port", Default: "443"},
				{Key: "winrm_user", Label: "WinRM username", Required: true, Credential: true,
					Placeholder: "administrator",
					Help:        "A local administrator or domain account with certificate import rights."},
				{Key: "winrm_password", Label: "WinRM password", Required: true,
					Credential: true, Secret: true},
				{Key: "winrm_transport", Label: "WinRM transport",
					Options: []string{"ntlm", "kerberos", "basic"}, Default: "ntlm"},
				{Key: "winrm_port", Label: "WinRM port", Default: "5986",
					Help: "5986 for HTTPS (default), 5985 for HTTP."},
			},
			Notes: "Requires WinRM HTTPS enabled on the target. The deployer uses PowerShell " +
				"remoting to import the PFX and run netsh/Set-RDSessionConfiguration as needed. " +
				"A vSatellite agent inside the Windows network is the preferred executor.",
		},
	}

	servingFields := []TargetField{
		{Key: "serving_host", Label: "Serving endpoint host", Placeholder: "service.example.com",
			Help: "DNS name or IP dialed after deployment. Required for AWS listeners and GCP target proxies; Linux/PostgreSQL fall back to host."},
		{Key: "server_name", Label: "TLS server name", Placeholder: "service.example.com",
			Help: "SNI and hostname/SAN to verify. Defaults to serving_host."},
		{Key: "serving_port", Label: "Serving endpoint port", Placeholder: "443",
			Help: "Defaults to port, then 443 (or 5432 for PostgreSQL)."},
		{Key: "expected_issuer", Label: "Expected issuer", Placeholder: "Example Issuing CA",
			Help: "Optional case-insensitive issuer constraint in addition to normal trust validation."},
	}
	for i := range specs {
		specs[i].Fields = append(specs[i].Fields, servingFields...)
	}
	return specs
}

// TargetSchemaFor finds one schema.
func TargetSchemaFor(targetType string) (TargetSpec, bool) {
	for _, s := range TargetSchemas() {
		if s.Type == targetType {
			return s, true
		}
	}
	return TargetSpec{}, false
}

// MissingFields reports the required inputs a target is missing, so the API can
// reject a request before it reaches a vendor and name everything at once.
func MissingFields(targetType string, target *DeployTarget) []string {
	spec, ok := TargetSchemaFor(targetType)
	if !ok || target == nil {
		return nil
	}

	present := func(f TargetField) bool {
		if f.Credential {
			return strings.TrimSpace(target.Credentials[f.Key]) != ""
		}
		switch f.Key {
		case "host":
			return strings.TrimSpace(target.Host) != ""
		case "region":
			return strings.TrimSpace(target.Region) != ""
		case "resource_id":
			return strings.TrimSpace(target.ResourceID) != ""
		case "vault_name":
			return strings.TrimSpace(target.VaultName) != ""
		case "listener_arn":
			// resource_id is the older spelling and is still accepted.
			return strings.TrimSpace(target.ListenerARN) != "" ||
				strings.TrimSpace(target.ResourceID) != ""
		case "certificate_arn":
			return strings.TrimSpace(target.CertificateARN) != ""
		case "apply_to":
			return strings.TrimSpace(target.ApplyTo) != ""
		case "version_policy":
			return strings.TrimSpace(target.VersionPolicy) != ""
		case "listener_policy":
			return strings.TrimSpace(target.ListenerPolicy) != ""
		case "retain_versions":
			return target.RetainVersions != 0
		case "cert_path":
			return strings.TrimSpace(target.CertPath) != ""
		case "key_path":
			return strings.TrimSpace(target.KeyPath) != ""
		case "config_file":
			return strings.TrimSpace(target.ConfigFile) != ""
		case "service":
			return strings.TrimSpace(target.Service) != ""
		}
		return true
	}

	missing := []string{}
	for _, f := range spec.Fields {
		if f.Required && !present(f) {
			missing = append(missing, f.Key)
		}
	}
	return missing
}
