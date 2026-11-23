// ==============================================================================
// services/persistence-engine.js
// Enterprise CLM Persistent Dynamic Database Engine
// Replaces in-memory mock variables with dynamic, persistent storage across:
// - Organization Management & Domain Verifications (DCV & D-U-N-S)
// - SSO Configuration & Identity Federation
// - Cloud Accounts & Resource Discovery
// - Satellite Gateways & Automated Deployment Jobs
// - PKI Certificates Lifecycle (Issue, Renew, Revoke, Download)
// - Network Port Discovery & Certificate Transparency (CT) Stream
// - Cryptographic Governance Policies & Compliance Audit
// - Platform Health Telemetry & Systems Monitoring
// - Global SaaS AuditUsage Analytics
// - Subscriptions, Cloud Billing & Invoices
// - Real-time Alerts Engine & Incident Response
// - Enterprise Platform Settings & Security Controls
// ==============================================================================

'use strict';

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

const DB_FILE = path.join(__dirname, '..', 'data', 'clm_database.json');

// Initial seed dataset (Production-grade realistic enterprise baseline)
const defaultSeed = {
    tenants: [
        {
            id: "tenant-enterprise-uat",
            name: "Enterprise Global Holdings Inc.",
            slug: "enterprise-global",
            plan: "enterprise",
            status: "active",
            max_users: 50,
            max_satellites: 10,
            max_certs: 500,
            data_region: "ap-south-1",
            created_at: "2026-01-15T08:00:00.000Z"
        },
        {
            id: "tenant-fintech-prod",
            name: "Apex Financial Technologies",
            slug: "apex-fintech",
            plan: "pro",
            status: "active",
            max_users: 25,
            max_satellites: 5,
            max_certs: 200,
            data_region: "us-east-1",
            created_at: "2026-03-20T11:30:00.000Z"
        }
    ],
    verifications: {
        legal_name: "Enterprise Global Holdings Inc.",
        duns_number: "98-234-5819",
        duns_status: "Verified",
        verification_level: "EV (Extended Validation)",
        incorporation_country: "United States",
        ca_prevalidation: { digicert: true, lets_encrypt: true, sectigo: true },
        domains: [
            { domain: "enterprise.internal", status: "Verified", method: "DNS_TXT", token: "clm-dcv-9f82a17c24e8", verified_at: "2026-08-15T10:00:00.000Z" },
            { domain: "aiforgers.tech", status: "Verified", method: "DNS_TXT", token: "clm-dcv-4a7b9e11c38d", verified_at: "2026-09-01T14:30:00.000Z" },
            { domain: "payments.enterprise.internal", status: "Verified", method: "HTTP_TOKEN", token: "clm-dcv-77c8e9f02a31", verified_at: "2026-10-12T09:15:00.000Z" }
        ]
    },
    sso_config: {
        provider: "saml",
        sp_entity_id: "https://clm-website-1cc7.vercel.app/api/v1/auth/saml/metadata",
        sp_acs_url: "https://clm-website-1cc7.vercel.app/api/v1/auth/saml/callback",
        idp_entity_id: "https://identity.enterprise.internal/realms/enterprise",
        idp_sso_url: "https://identity.enterprise.internal/realms/enterprise/protocol/saml",
        idp_cert: "-----BEGIN CERTIFICATE-----\nMIIC+DCCAeCgAwIBAgIUQW6p6k6c1X7e7P...\n-----END CERTIFICATE-----",
        oidc_issuer: "https://identity.enterprise.internal/realms/enterprise",
        oidc_client_id: "clm-portal-client",
        enforce_sso: false,
        allowed_domains: ["enterprise.internal", "aiforgers.tech"],
        auto_provision: true,
        default_role: "operator",
        status: "active"
    },
    cloud_accounts: [
        {
            id: "ca-01",
            name: "AWS Production (Mumbai)",
            provider: "aws",
            region: "ap-south-1",
            credential_version: 2,
            role_arn: "arn:aws:iam::279078306945:role/CLM-Deploy",
            status: "active",
            created_at: "2026-04-10T10:00:00.000Z"
        },
        {
            id: "ca-02",
            name: "Azure Cloud Enterprise",
            provider: "azure",
            region: "centralindia",
            credential_version: 1,
            status: "active",
            created_at: "2026-05-18T14:20:00.000Z"
        }
    ],
    satellites: [
        {
            id: "vsat-ind-aps1-uat-01",
            name: "Enterprise UAT VPC-01 vSatellite Scanner",
            cloud: "AWS",
            region: "ap-south-1",
            ip_address: "10.125.4.121",
            status: "online",
            agent_version: "2.4.1",
            last_heartbeat: new Date().toISOString(),
            pki_vault_status: "sealed_healthy"
        }
    ],
    deploy_jobs: [
        {
            id: "job-001",
            name: "Deploy Wildcard SSL to Nginx Ingress",
            cloud: "AWS",
            region: "ap-south-1",
            mode: "ec2",
            state: "completed",
            public_ip: "10.125.4.121",
            target_cert: "cert-01",
            target_endpoint: "Nginx Ingress Controller",
            current_step: 4,
            steps: [
                { name: "Verify In-VPC mTLS Transport", status: "completed" },
                { name: "Compile Signed Evidence Bundle", status: "completed" },
                { name: "Push Private Key to Hardware Enclave", status: "completed" },
                { name: "Hot Reload Target Webserver", status: "completed" }
            ],
            created_at: "2026-11-20T10:00:00.000Z"
        }
    ],
    certificates: [
        {
            id: "cert-01",
            domain: "*.enterprise.internal",
            subject: "CN=*.enterprise.internal",
            issuer: "DigiCert TLS RSA SHA256 2020 CA1",
            sans: ["*.enterprise.internal", "enterprise.internal"],
            days_left: 42,
            status: "active",
            key_algorithm: "RSA",
            key_size: 2048,
            environment: "prod",
            not_after: new Date(Date.now() + 42 * 86400000).toISOString()
        },
        {
            id: "cert-02",
            domain: "api-uat.enterprise.internal",
            subject: "CN=api-uat.enterprise.internal",
            issuer: "Let's Encrypt Authority X3",
            sans: ["api-uat.enterprise.internal"],
            days_left: 12,
            status: "expiring_soon",
            key_algorithm: "ECDSA",
            key_size: 256,
            environment: "uat",
            not_after: new Date(Date.now() + 12 * 86400000).toISOString()
        },
        {
            id: "cert-03",
            domain: "internal-mgmt.local",
            subject: "CN=internal-mgmt.local",
            issuer: "Enterprise Private CA",
            sans: ["internal-mgmt.local"],
            days_left: 280,
            status: "active",
            key_algorithm: "RSA",
            key_size: 4096,
            environment: "prod",
            not_after: new Date(Date.now() + 280 * 86400000).toISOString()
        }
    ],
    discovery_scans: [
        {
            id: "scan-001",
            target: "10.125.0.0/16",
            ports: "443, 8443, 9443",
            scanned_hosts: 256,
            active_tls_endpoints: 14,
            valid_certificates: 13,
            expiring_certificates: 1,
            rogue_certificates: 0,
            duration_ms: 1850,
            status: "completed",
            completed_at: "2026-11-22T16:00:00.000Z"
        }
    ],
    ct_logs: [
        { id: "ct-01", domain: "auth.enterprise.internal", issuer: "Let's Encrypt Authority X3", logged_at: new Date(Date.now() - 120000).toISOString(), ct_log: "Cloudflare Nimbus2026", status: "Authorized", san_count: 2 },
        { id: "ct-02", domain: "payments.enterprise.internal", issuer: "DigiCert TLS RSA SHA256", logged_at: new Date(Date.now() - 3600000).toISOString(), ct_log: "Google Argon2026", status: "Authorized", san_count: 1 },
        { id: "ct-03", domain: "unknown-sub.enterprise.internal", issuer: "ZeroSSL RSA Domain Secure Site CA", logged_at: new Date(Date.now() - 7200000).toISOString(), ct_log: "DigiCert Yeti2026", status: "Warning: Unrecognized CA", san_count: 1 }
    ],
    policies: [
        { id: "POL-001", name: "Minimum Key Length", severity: "High", condition: "RSA >= 2048 or ECDSA >= P-256", enforced: true },
        { id: "POL-002", name: "Maximum Validity Period", severity: "High", condition: "Max 397 Days (CAB Forum)", enforced: true },
        { id: "POL-003", name: "SHA-1 / MD5 Deprecation", severity: "Critical", condition: "Disallow weak hashes", enforced: true },
        { id: "POL-004", name: "Wildcard Scope Guard", severity: "Medium", condition: "Internal domains only", enforced: true },
        { id: "POL-005", name: "Segregation of Duties", severity: "Critical", condition: "Prohibit self-approval", enforced: true }
    ],
    subscription: {
        current_plan: "enterprise",
        amount: 4999.00,
        currency: "USD",
        billing_cycle: "annual",
        renewal_date: "2027-01-01",
        plans: [
            { id: "starter", name: "Starter", price: 499, interval: "month", max_users: 5, max_gateways: 2, max_certs: 50, max_contracts: 20, features: ["SAML / SSO", "Basic Discovery", "Email Alerts"] },
            { id: "pro", name: "Professional", price: 1499, interval: "month", max_users: 25, max_gateways: 10, max_certs: 500, max_contracts: 100, features: ["Multi-Cloud Accounts", "In-VPC Satellites", "Custom Playbooks", "Three-Way Match"] },
            { id: "enterprise", name: "Enterprise", price: 4999, interval: "month", max_users: "Unlimited", max_gateways: "Unlimited", max_certs: "Unlimited", max_contracts: "Unlimited", features: ["Zero-Knowledge Hardware PKI", "Custom CA Integrations", "99.99% SLA", "Dedicated Account Manager"] }
        ]
    },
    invoices: [
        { id: "INV-2026-1101", date: "2026-11-01", amount: 4999.00, currency: "USD", status: "Paid", items: "Enterprise CLM Platform (Annual Subscription)" },
        { id: "INV-2026-1001", date: "2026-10-01", amount: 4999.00, currency: "USD", status: "Paid", items: "Enterprise CLM Platform (Annual Subscription)" },
        { id: "INV-2026-0901", date: "2026-09-01", amount: 4999.00, currency: "USD", status: "Paid", items: "Enterprise CLM Platform (Annual Subscription)" }
    ],
    payment_methods: {
        card: { brand: "Visa", last4: "4242", exp_month: 12, exp_year: 2028, holder: "Corporate Finance" },
        billing_email: "billing@enterprise.internal"
    },
    alerts: [
        { id: "alt-001", severity: "critical", category: "Certificate Expiration", title: "api-uat.enterprise.internal expires in 12 days", timestamp: new Date(Date.now() - 3600000).toISOString(), status: "active", target: "cert-02" },
        { id: "alt-002", severity: "warning", category: "Compliance Drift", title: "Shadow certificate detected via CT Log on unknown-sub.enterprise.internal", timestamp: new Date(Date.now() - 14400000).toISOString(), status: "active", target: "ct-03" },
        { id: "alt-003", severity: "info", category: "Gateway Health", title: "Satellite vsat-ind-aps1-uat-01 latency spiked to 48ms", timestamp: new Date(Date.now() - 28800000).toISOString(), status: "muted", target: "vsat-ind-aps1-uat-01" }
    ],
    settings: {
        organization_name: "Enterprise Global Holdings Inc.",
        support_email: "support@enterprise.internal",
        session_timeout_minutes: 60,
        mfa_enforced: true,
        password_min_length: 12,
        password_require_special: true,
        ip_allowlist_enabled: true,
        allowed_ips: ["127.0.0.1", "10.0.0.0/8", "192.168.1.0/24"],
        maintenance_mode: false,
        audit_retention_days: 365,
        slack_webhook_alerts: true,
        email_expiry_alerts: true
    },
    audit_chain: [
        {
            id: "aud-001",
            action: "system_initialized",
            actor: "system@enterprise.internal",
            target: "clm-platform",
            prev_hash: "0000000000000000000000000000000000000000000000000000000000000000",
            hash: "c8678deb94e397b0553754e3895e54619d8031d24e13ec97e4e1a067645bf0e3",
            timestamp: "2026-01-01T00:00:00.000Z"
        }
    ]
};

class PersistenceEngine {
    constructor() {
        this.data = null;
        this.init();
    }

    init() {
        try {
            if (fs.existsSync(DB_FILE)) {
                const raw = fs.readFileSync(DB_FILE, 'utf8');
                this.data = JSON.parse(raw);
            } else {
                this.data = JSON.parse(JSON.stringify(defaultSeed));
                this.persist();
            }
        } catch (e) {
            console.error('[PersistenceEngine] Failed to load DB file, initializing seed:', e.message);
            this.data = JSON.parse(JSON.stringify(defaultSeed));
        }
    }

    persist() {
        try {
            const dir = path.dirname(DB_FILE);
            if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
            fs.writeFileSync(DB_FILE, JSON.stringify(this.data, null, 2), 'utf8');
        } catch (e) {
            console.error('[PersistenceEngine] Failed to write DB file:', e.message);
        }
    }

    get(collection) {
        return this.data[collection] || null;
    }

    set(collection, value) {
        this.data[collection] = value;
        this.persist();
        return this.data[collection];
    }

    find(collection, predicate) {
        const arr = this.data[collection];
        if (!Array.isArray(arr)) return null;
        return arr.find(predicate);
    }

    insert(collection, item) {
        if (!Array.isArray(this.data[collection])) {
            this.data[collection] = [];
        }
        this.data[collection].push(item);
        this.logAudit('insert_' + collection, item.id || 'unidentified', item);
        this.persist();
        return item;
    }

    update(collection, id, updates) {
        if (Array.isArray(this.data[collection])) {
            const idx = this.data[collection].findIndex(i => i.id === id);
            if (idx !== -1) {
                Object.assign(this.data[collection][idx], updates);
                this.logAudit('update_' + collection, id, updates);
                this.persist();
                return this.data[collection][idx];
            }
        } else if (this.data[collection] && typeof this.data[collection] === 'object') {
            Object.assign(this.data[collection], updates);
            this.logAudit('update_' + collection, id, updates);
            this.persist();
            return this.data[collection];
        }
        return null;
    }

    delete(collection, id) {
        if (Array.isArray(this.data[collection])) {
            const prevLen = this.data[collection].length;
            this.data[collection] = this.data[collection].filter(i => i.id !== id);
            if (this.data[collection].length !== prevLen) {
                this.logAudit('delete_' + collection, id, { deleted: true });
                this.persist();
                return true;
            }
        }
        return false;
    }

    logAudit(action, target, details) {
        if (!this.data.audit_chain) this.data.audit_chain = [];
        const last = this.data.audit_chain[this.data.audit_chain.length - 1];
        const prev_hash = last ? last.hash : '0000000000000000000000000000000000000000000000000000000000000000';
        const timestamp = new Date().toISOString();
        const payload = `${prev_hash}:${action}:${target}:${timestamp}`;
        const hash = crypto.createHash('sha256').update(payload).digest('hex');

        this.data.audit_chain.push({
            id: 'aud-' + Date.now() + '-' + Math.random().toString(36).slice(2, 6),
            action,
            actor: 'operator@enterprise.internal',
            target,
            prev_hash,
            hash,
            timestamp,
            details: typeof details === 'string' ? details : JSON.stringify(details)
        });
    }
}

const engineInstance = new PersistenceEngine();
module.exports = engineInstance;
