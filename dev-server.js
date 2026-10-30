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
    { id: "tenant-m2p-uat", name: "M2P Fintech UAT", plan: "enterprise", status: "active", cert_limit: 100000, created_at: new Date().toISOString() },
    { id: "tenant-dev-lab", name: "Cloud Engineering Lab", plan: "pro", status: "active", cert_limit: 10000, created_at: new Date().toISOString() }
];

let mockSatellites = [
    { id: "sat-m2p-ind-aps1", name: "AWS Mumbai UAT Gateway", region: "ap-south-1", cloud: "aws", ip: "10.125.4.68", status: "online", last_heartbeat: new Date().toISOString() }
];

let mockCerts = [
    { id: "cert-01", subject: "CN=*.m2pfintech.com", issuer: "DigiCert TLS RSA SHA256 2020 CA1", sans: ["*.m2pfintech.com", "m2pfintech.com"], days_left: 42, status: "active", key_algorithm: "RSA", key_size: 2048, not_after: new Date(Date.now() + 42*86400000).toISOString() },
    { id: "cert-02", subject: "CN=api-uat.m2p.in", issuer: "Let's Encrypt Authority X3", sans: ["api-uat.m2p.in"], days_left: 12, status: "expiring_soon", key_algorithm: "ECDSA", key_size: 256, not_after: new Date(Date.now() + 12*86400000).toISOString() },
    { id: "cert-03", subject: "CN=internal-mgmt.local", issuer: "M2P Enterprise Private CA", sans: ["internal-mgmt.local"], days_left: 280, status: "active", key_algorithm: "RSA", key_size: 4096, not_after: new Date(Date.now() + 280*86400000).toISOString() }
];

let mockAgents = [
    { id: "vsat-ind-aps1-uat-01", name: "M2P UAT VPC-01 vSatellite Scanner", host: "ip-10-125-4-121", status: "active", last_heartbeat: new Date().toISOString(), cert_count: 3, version: "1.0.0", cidrs: ["10.125.0.0/16"], capabilities: ["scan_cidr", "deploy_cert", "health_check"] }
];

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
            roles: ["super_admin", "admin"],
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
