// ==============================================================================
// CLM Unified Development Web Server
// ==============================================================================
// Serves both CLM web portals and provides lightweight mock APIs for instant
// local preview and interactive testing without requiring Docker or PostgreSQL.
//
// Ports:
//   - http://localhost:3000 -> SaaS Control Plane Admin Portal
//   - http://localhost:8080 -> Satellite Gateway Portal UI & Mock APIs
// ==============================================================================

const http = require('http');
const fs = require('fs');
const path = require('path');
const url = require('url');

const SAAS_DIR = path.join(__dirname, 'components', 'saas-control-plane', 'web', 'admin-portal');
const GATEWAY_DIR = path.join(__dirname, 'components', 'satellite-gateway', 'web', 'gateway-ui');

const MIME_TYPES = {
    '.html': 'text/html; charset=utf-8',
    '.js': 'application/javascript; charset=utf-8',
    '.css': 'text/css; charset=utf-8',
    '.json': 'application/json',
    '.png': 'image/png',
    '.jpg': 'image/jpeg',
    '.svg': 'image/svg+xml',
    '.ico': 'image/x-icon'
};

// Mock data stores
let mockTenants = [
    { id: "tenant-m2p-uat", name: "M2P Fintech UAT", slug: "m2p-uat", plan: "enterprise", status: "active", max_certs: 100000, max_satellites: 50, max_users: 100, data_region: "ap-south-1", created_at: new Date().toISOString() },
    { id: "tenant-dev-lab", name: "Cloud Engineering Lab", slug: "cloud-eng", plan: "pro", status: "active", max_certs: 10000, max_satellites: 5, max_users: 15, data_region: "ap-south-1", created_at: new Date().toISOString() }
];

let mockSatellites = [
    { id: "sat-m2p-ind-aps1", name: "AWS Mumbai UAT Gateway", region: "ap-south-1", cloud: "aws", ip: "10.125.4.68", status: "online", last_heartbeat: new Date().toISOString() }
];

let mockCerts = [
    { id: "cert-01", domain: "*.m2pfintech.com", subject: "CN=*.m2pfintech.com", issuer: "DigiCert TLS RSA SHA256 2020 CA1", sans: ["*.m2pfintech.com", "m2pfintech.com"], days_left: 42, status: "active", key_algorithm: "RSA", key_size: 2048, is_self_signed: false, environment: "prod", not_after: new Date(Date.now() + 42*86400000).toISOString() },
    { id: "cert-02", domain: "api-uat.m2p.in", subject: "CN=api-uat.m2p.in", issuer: "Let's Encrypt Authority X3", sans: ["api-uat.m2p.in"], days_left: 12, status: "expiring_soon", key_algorithm: "ECDSA", key_size: 256, is_self_signed: false, environment: "uat", not_after: new Date(Date.now() + 12*86400000).toISOString() },
    { id: "cert-03", domain: "internal-mgmt.local", subject: "CN=internal-mgmt.local", issuer: "M2P Enterprise Private CA", sans: ["internal-mgmt.local"], days_left: 280, status: "active", key_algorithm: "RSA", key_size: 4096, is_self_signed: false, environment: "prod", not_after: new Date(Date.now() + 280*86400000).toISOString() }
];

let mockAgents = [
    { id: "vsat-ind-aps1-uat-01", name: "M2P UAT VPC-01 vSatellite Scanner", host: "ip-10-125-4-121", status: "active", last_heartbeat: new Date().toISOString(), cert_count: 3, version: "1.0.0", cidrs: ["10.125.0.0/16"], capabilities: ["scan_cidr", "deploy_cert", "health_check"] }
];

let mockApprovals = [
    {
        id: "appr-001",
        tenant_id: "tenant-m2p-uat",
        request_type: "cert_create",
        level: "L2",
        status: "pending",
        requested_by: "user-eng-02",
        requested_by_email: "eng-team@m2pfintech.com",
        justification: "Production wildcard SSL for new payment routing gateway",
        payload: JSON.stringify({ domain: "*.payments.m2p.in", key_algorithm: "RSA", key_size: 2048 }),
        estimated_cost: 249.00,
        sla_deadline: new Date(Date.now() + 6 * 3600000).toISOString(),
        created_at: new Date(Date.now() - 2 * 3600000).toISOString(),
        comment: "L1 Approved by tech-lead@m2pfintech.com [Advanced to L2 Security Review]"
    },
    {
        id: "appr-002",
        tenant_id: "tenant-m2p-uat",
        request_type: "cert_purchase",
        level: "L3",
        status: "pending",
        requested_by: "user-infra-01",
        requested_by_email: "infra-lead@m2pfintech.com",
        justification: "EV Multi-Domain Certificate for Core Banking Interface",
        payload: JSON.stringify({ domain: "core.banking.m2p.in", san_count: 5 }),
        estimated_cost: 899.00,
        sla_deadline: new Date(Date.now() + 18 * 3600000).toISOString(),
        created_at: new Date(Date.now() - 5 * 3600000).toISOString(),
        comment: "L2 Approved by infosec@m2pfintech.com [Advanced to L3 CISO Signoff]"
    }
];

let mockSyncStatus = {
    config: {
        aws: {
            enabled: true,
            regions: ["ap-south-1", "us-east-1", "me-south-1"],
            secrets_prefix: "AWS/SSL/",
            tag_managed_by: "CLM",
            compliance_scope: "PCI-DSS-v4"
        },
        gcp: {
            enabled: true,
            project_id: "m2p-hyc-prd",
            locations: ["global", "asia-south1"],
            secrets_prefix: "gcp-ssl/"
        }
    },
    last_result: {
        job_id: "sync-job-latest",
        started_at: new Date(Date.now() - 15 * 60000).toISOString(),
        completed_at: new Date(Date.now() - 14 * 60000).toISOString(),
        dry_run: false,
        total_certs: 3,
        synced_certs: 2,
        drifted_certs: 1,
        failed_certs: 0,
        items: [
            {
                cert_id: "cert-01",
                domain: "*.m2pfintech.com",
                fingerprint: "a9f8b2c4e1d35467001a4e5f7823901bce45781a",
                days_remaining: 42,
                aws_sync_status: { "ap-south-1": "arn:aws:acm:ap-south-1:279078306945:certificate/clm-01", "us-east-1": "arn:aws:acm:us-east-1:279078306945:certificate/clm-01" },
                gcp_sync_status: { "global": "projects/m2p-hyc-prd/locations/global/certificates/clm-01" },
                overall_status: "in_sync"
            },
            {
                cert_id: "cert-02",
                domain: "api-uat.m2p.in",
                fingerprint: "6b38f12a88e9102c118944fa90de0188ef4920ba",
                days_remaining: 12,
                aws_sync_status: { "ap-south-1": "arn:aws:acm:ap-south-1:279078306945:certificate/clm-02" },
                gcp_sync_status: { "global": "projects/m2p-hyc-prd/locations/global/certificates/clm-02" },
                overall_status: "drifted",
                drift_details: ["Certificate expiring in 12 days; automated cloud renewal required"]
            },
            {
                cert_id: "cert-03",
                domain: "internal-mgmt.local",
                fingerprint: "f0178a9c332e4d9b891a27419e072bba56711200",
                days_remaining: 280,
                aws_sync_status: { "ap-south-1": "arn:aws:acm:ap-south-1:279078306945:certificate/clm-03" },
                gcp_sync_status: { "global": "projects/m2p-hyc-prd/locations/global/certificates/clm-03" },
                overall_status: "in_sync"
            }
        ]
    }
};

let mockGroupMappings = [
    { id: "map-01", group_name: "AWSReservedSSO_AdministratorAccess", target_role: "super_admin", priority: 95, description: "Cloud platform super admins", created_at: new Date().toISOString() },
    { id: "map-02", group_name: "M2P-SECURITY-OPERATIONS", target_role: "operator", priority: 80, description: "SecOps team (Approver L2)", created_at: new Date().toISOString() },
    { id: "map-03", group_name: "M2P-COMPLIANCE-AUDITORS", target_role: "auditor", priority: 50, description: "External & internal auditors", created_at: new Date().toISOString() }
];

let mockMigrationHistory = [
    { batch_id: "mig-batch-init-01", source: "AWS Secrets Manager (AWS/SSL/*)", environment: "prod", total_discovered: 3, valid_count: 3, weak_key_count: 0, expired_count: 0, cloud_sync_queued: true, created_at: new Date().toISOString() }
];

let mockComplianceSummary = {
    evaluated_at: new Date().toISOString(),
    overall_score: 94,
    overall_grade: "A",
    framework_scores: {
        "PCI-DSS-v4.0": 95,
        "SOC2-Type-II": 96,
        "ISO-27001": 90
    },
    total_certificates: 3,
    compliant_certs: 2,
    non_compliant_certs: 1,
    critical_findings: 0,
    high_findings: 1,
    medium_findings: 0,
    integrity_hash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    findings: [
        {
            rule_id: "ISO27001-A.8.24-EXPIRING-URGENT",
            framework: "ISO-27001",
            title: "Critical Impending Certificate Expiration",
            severity: "HIGH",
            target_id: "cert-02",
            target_label: "api-uat.m2p.in",
            status: "NON_COMPLIANT",
            description: "Certificate expires in 12 days",
            remediation: "Schedule automated ACME or CA renewal immediately."
        }
    ]
};

function serveStatic(baseDir, req, res) {
    let parsedUrl = url.parse(req.url);
    let filePath = path.join(baseDir, parsedUrl.pathname === '/' ? 'index.html' : parsedUrl.pathname);

    fs.stat(filePath, (err, stats) => {
        if (err || !stats.isFile()) {
            filePath = path.join(baseDir, 'index.html');
        }

        const ext = path.extname(filePath).toLowerCase();
        const contentType = MIME_TYPES[ext] || 'application/octet-stream';

        fs.readFile(filePath, (readErr, content) => {
            if (readErr) {
                res.writeHead(500, { 'Content-Type': 'text/plain' });
                res.end('500 Internal Server Error');
                return;
            }
            res.writeHead(200, {
                'Content-Type': contentType,
                'Cache-Control': 'no-cache, no-store, must-revalidate'
            });
            res.end(content);
        });
    });
}

function handleMockAPI(req, res) {
    const parsed = url.parse(req.url, true);
    const pathname = parsed.pathname;

    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Agent-Secret');

    if (req.method === 'OPTIONS') {
        res.writeHead(204);
        res.end();
        return true;
    }

    // Auth proxy mock
    if (pathname.includes('/token') || pathname === '/auth/login' || pathname.startsWith('/auth/')) {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
            access_token: "mock-jwt-token-clm-admin-access",
            refresh_token: "mock-jwt-refresh-token",
            expires_in: 3600,
            token_type: "Bearer"
        }));
        return true;
    }

    // User profile mock
    if (pathname === '/api/v1/auth/me' || pathname === '/api/v1/users/me') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
            id: "user-admin-01",
            username: "admin@m2pfintech.com",
            email: "admin@m2pfintech.com",
            roles: ["super_admin", "admin", "approver"],
            tenant_id: "tenant-m2p-uat"
        }));
        return true;
    }

    // Health
    if (pathname === '/api/v1/health' || pathname === '/health') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ status: "healthy", mode: "local-dev-mock", timestamp: new Date().toISOString() }));
        return true;
    }

    // Tenants
    if (pathname === '/api/v1/tenants') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ tenants: mockTenants, total: mockTenants.length }));
        return true;
    }

    // Customer Onboarding APIs (Patterned after customer-onboarding repo)
    if (pathname === '/api/v1/onboarding/preflight' && req.method === 'POST') {
        let body = '';
        req.on('data', chunk => { body += chunk; });
        req.on('end', () => {
            let input = {};
            try { input = JSON.parse(body); } catch(e) {}
            let checks = [
                { name: "Organization Slug Format", passed: !!(input.slug && /^[a-z][a-z0-9-]{2,30}$/.test(input.slug)), message: "3-30 chars, lowercase alphanumeric" },
                { name: "Slug Availability", passed: !mockTenants.some(t => t.slug === input.slug), message: "Slug is available" },
                { name: "Contact Email Validation", passed: !!(input.contact_email && input.contact_email.includes('@')), message: "Valid corporate email" },
                { name: "AWS Account ID Format", passed: !!(input.aws_account_id && /^\d{12}$/.test(input.aws_account_id)), message: "12 digits required" },
                { name: "VPC ID Format", passed: !!(input.vpc_id && /^vpc-[0-9a-fA-F]{8,17}$/.test(input.vpc_id)), message: "Valid VPC format" },
                { name: "STS External ID Entropy", passed: !!(input.external_id && input.external_id.length >= 16), message: ">= 16 chars required" }
            ];
            let isValid = checks.every(c => c.passed);
            res.writeHead(isValid ? 200 : 422, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ valid: isValid, checks: checks, plan: input.plan || "enterprise" }));
        });
        return true;
    }

    if (pathname === '/api/v1/onboarding/bootstrap' && req.method === 'POST') {
        let body = '';
        req.on('data', chunk => { body += chunk; });
        req.on('end', () => {
            let input = {};
            try { input = JSON.parse(body); } catch(e) {}
            const tenantID = "tenant-" + Math.floor(Math.random() * 89999 + 10000);
            const satID = "sat-" + Math.floor(Math.random() * 89999 + 10000);
            const newTenant = {
                id: tenantID,
                name: input.name || "Enterprise Customer",
                slug: input.slug || "tenant-new",
                plan: input.plan || "enterprise",
                status: "active",
                max_certs: 10000,
                max_satellites: 50,
                max_users: 100,
                data_region: input.region || "ap-south-1",
                created_at: new Date().toISOString()
            };
            mockTenants.push(newTenant);
            res.writeHead(201, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({
                tenant_id: tenantID,
                slug: newTenant.slug,
                name: newTenant.name,
                plan: newTenant.plan,
                admin_email: input.contact_email || "admin@customer.com",
                temporary_secret: "clm-sec-" + Math.random().toString(36).substring(2, 10),
                primary_gateway: {
                    satellite_id: satID,
                    pairing_token: "PAIR-" + Math.random().toString(36).substring(2, 14),
                    install_script: `curl -sSL https://clm.m2p.in/install-gateway.sh | bash -s -- --satellite-id ${satID} --region ${input.region || 'ap-south-1'}`
                }
            }));
        });
        return true;
    }

    // Approvals APIs (Multi-Tier L1/L2/L3 & Segregation of Duties ARC-004)
    if (pathname === '/api/v1/approvals/pending' || pathname === '/api/v1/approvals') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ approvals: mockApprovals, total: mockApprovals.length }));
        return true;
    }

    if (pathname.startsWith('/api/v1/approvals/') && pathname.endsWith('/approve') && req.method === 'POST') {
        const parts = pathname.split('/');
        const id = parts[4];
        const item = mockApprovals.find(a => a.id === id);
        if (item) {
            // ARC-004 Check
            if (item.requested_by === "user-admin-01") {
                res.writeHead(403, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ error: "segregation of duties violation: requester cannot approve their own request (ARC-004)" }));
                return true;
            }
            // Multi-tier progress
            if (item.level === 'L1') {
                item.level = 'L2';
                item.comment = "L1 Approved. Escalated to L2 Security Review.";
            } else if (item.level === 'L2' && item.estimated_cost >= 500) {
                item.level = 'L3';
                item.comment = "L2 Approved. Escalated to L3 CISO Review.";
            } else {
                item.status = "approved";
                item.resolved_at = new Date().toISOString();
                item.resolved_by = "user-admin-01";
            }
            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ message: "decision recorded", approval: item }));
        } else {
            res.writeHead(404, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: "approval request not found" }));
        }
        return true;
    }

    // Multi-Cloud Sync APIs (Patterned after aws-ssl-automation and gcp-ssl-cert-sync)
    if (pathname === '/api/v1/sync/multicloud/status') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(mockSyncStatus));
        return true;
    }

    if (pathname === '/api/v1/sync/multicloud/run' && req.method === 'POST') {
        mockSyncStatus.last_result.started_at = new Date().toISOString();
        mockSyncStatus.last_result.completed_at = new Date().toISOString();
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(mockSyncStatus.last_result));
        return true;
    }

    if (pathname === '/api/v1/sync/multicloud/history') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ history: [mockSyncStatus.last_result], total: 1 }));
        return true;
    }

    // Corporate Compliance & Audit Posture
    if (pathname === '/api/v1/reports/corporate-compliance') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(mockComplianceSummary));
        return true;
    }

    if (pathname === '/api/v1/reports/audit-bundle') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
            gateway_id: "sat-m2p-ind-aps1",
            exported_at: new Date().toISOString(),
            compliance: mockComplianceSummary,
            audit_event_count: 42,
            bundle_seal: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
            assertions: {
                "ARC-001": "Customer private keys reside exclusively within gateway boundary and are never exported",
                "ARC-004": "Segregation of duties enforced; requesters cannot approve high-risk certificates",
                "PCI-DSS": "Cryptographic key lengths meet PCI-DSS v4.0 Requirement 8 and 12",
                "SOC2": "Full immutable audit trail with non-repudiation integrity seal"
            }
        }));
        return true;
    }

    
    // Directory Group Mappings APIs (AWS IAM Identity Center / SAML)
    if (pathname.includes('/sso/mappings')) {
        if (req.method === 'GET') {
            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ mappings: mockGroupMappings, count: mockGroupMappings.length }));
            return true;
        }
        if (req.method === 'POST') {
            let body = '';
            req.on('data', chunk => { body += chunk; });
            req.on('end', () => {
                let input = {};
                try { input = JSON.parse(body); } catch(e) {}
                const newRule = {
                    id: "map-" + Math.floor(Math.random() * 8999 + 1000),
                    group_name: input.group_name || "EnterpriseGroup",
                    target_role: input.target_role || "viewer",
                    priority: parseInt(input.priority, 10) || 50,
                    description: input.description || "",
                    created_at: new Date().toISOString()
                };
                mockGroupMappings.unshift(newRule);
                res.writeHead(201, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ mapping: newRule }));
            });
            return true;
        }
        if (req.method === 'DELETE') {
            const parts = pathname.split('/');
            const id = parts[parts.length - 1];
            mockGroupMappings = mockGroupMappings.filter(m => m.id !== id);
            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ message: "group mapping deleted" }));
            return true;
        }
    }

    // Bulk Certificate Migration APIs
    if (pathname === '/api/v1/migration/validate' && req.method === 'POST') {
        let body = '';
        req.on('data', chunk => { body += chunk; });
        req.on('end', () => {
            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({
                batch_id: "mig-dryrun-" + Date.now(),
                total_discovered: 1,
                valid_count: 1,
                weak_key_count: 0,
                expired_count: 0,
                dry_run: true,
                certificates: [
                    {
                        id: "mig-cert-dryrun-01",
                        domain: "migrated-payment.m2pfintech.com",
                        sans: ["migrated-payment.m2pfintech.com"],
                        key_algorithm: "RSA",
                        key_size: 2048,
                        days_remaining: 180,
                        has_private_key: true,
                        key_match_passed: true,
                        compliance_pass: true
                    }
                ]
            }));
        });
        return true;
    }

    if (pathname === '/api/v1/migration/import' && req.method === 'POST') {
        let body = '';
        req.on('data', chunk => { body += chunk; });
        req.on('end', () => {
            let input = {};
            try { input = JSON.parse(body); } catch(e) {}
            const batchId = "mig-batch-" + Date.now();
            const imported = {
                batch_id: batchId,
                source: input.source || "Corporate Upload",
                environment: input.environment || "prod",
                total_discovered: 1,
                valid_count: 1,
                weak_key_count: 0,
                expired_count: 0,
                cloud_sync_queued: true,
                created_at: new Date().toISOString(),
                certificates: [
                    {
                        id: "mig-cert-" + Math.floor(Math.random() * 8999 + 1000),
                        domain: "imported-core.m2pfintech.com",
                        key_algorithm: "RSA",
                        key_size: 2048,
                        days_remaining: 210,
                        key_match_passed: true,
                        compliance_pass: true
                    }
                ]
            };
            mockMigrationHistory.unshift(imported);
            mockCerts.push({
                id: imported.certificates[0].id,
                subject: "CN=" + imported.certificates[0].domain,
                issuer: "Corporate Ingested CA",
                sans: [imported.certificates[0].domain],
                days_left: 210,
                status: "active",
                key_algorithm: "RSA",
                key_size: 2048,
                not_after: new Date(Date.now() + 210*86400000).toISOString()
            });
            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ batch: imported, imported_count: 1 }));
        });
        return true;
    }

    if (pathname === '/api/v1/migration/history') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ history: mockMigrationHistory, total: mockMigrationHistory.length }));
        return true;
    }

    // Automated Rotation APIs
    if (pathname === '/api/v1/rotation/status') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
            total_monitored: mockCerts.length,
            in_rotation_window: 1,
            auto_renew_eligible: 1,
            active_rotations: 0,
            completed_30_days: 4
        }));
        return true;
    }

    if (pathname.startsWith('/api/v1/rotation/execute/') && req.method === 'POST') {
        const parts = pathname.split('/');
        const id = parts[parts.length - 1];
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
            id: "rot-" + Date.now(),
            cert_id: id,
            stage: "completed",
            health_verified: true,
            rollback_occurred: false,
            new_serial: "0x" + Math.random().toString(16).substring(2, 14),
            completed_at: new Date().toISOString()
        }));
        return true;
    }

    // Satellites / Gateways
    if (pathname === '/api/v1/satellites') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ satellites: mockSatellites, total: mockSatellites.length }));
        return true;
    }

    // Certificates
    if (pathname === '/api/v1/certificates') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ certificates: mockCerts, total: mockCerts.length }));
        return true;
    }

    // vSatellite agents listing
    if (pathname === '/vsatellite/agents' || pathname === '/api/v1/vsatellites') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ agents: mockAgents, total: mockAgents.length }));
        return true;
    }

    // vSatellite heartbeat
    if (pathname === '/vsatellite/heartbeat') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ status: "ok", commands: [] }));
        return true;
    }

    // Stats
    if (pathname === '/api/v1/stats' || pathname === '/api/v1/dashboard') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
            total_certificates: mockCerts.length,
            active_gateways: mockSatellites.length,
            active_agents: mockAgents.length,
            expiring_30_days: 1
        }));
        return true;
    }

    return false;
}

// 1. SaaS Control Plane Admin Portal (Port 3000)
const adminServer = http.createServer((req, res) => {
    if (handleMockAPI(req, res)) return;
    serveStatic(SAAS_DIR, req, res);
});

adminServer.listen(3000, () => {
    console.log('\x1b[32m%s\x1b[0m', '=======================================================');
    console.log('\x1b[36m%s\x1b[0m', '  CLM SaaS Admin Portal is running!');
    console.log('\x1b[33m%s\x1b[0m', '  URL: http://localhost:3000');
    console.log('\x1b[32m%s\x1b[0m', '=======================================================');
});

// 2. Gateway Portal & Local APIs (Port 8080)
const gatewayServer = http.createServer((req, res) => {
    if (handleMockAPI(req, res)) return;
    serveStatic(GATEWAY_DIR, req, res);
});

gatewayServer.listen(8080, () => {
    console.log('\x1b[32m%s\x1b[0m', '=======================================================');
    console.log('\x1b[36m%s\x1b[0m', '  CLM Satellite Gateway Portal is running!');
    console.log('\x1b[33m%s\x1b[0m', '  URL: http://localhost:8080');
    console.log('\x1b[32m%s\x1b[0m', '=======================================================');
});
