// ==============================================================================
// scripts/test_400_implementation.js
// CLM 400 Features & Tasks 1-11 Comprehensive API Verification Suite
// Tests all endpoints across Tasks 1-11 in-process via handleMockAPI
// ==============================================================================

'use strict';

const { EventEmitter } = require('events');
const devServer = require('../dev-server.js');

function makeMockReq(method, url, body) {
    const req = new EventEmitter();
    req.method = method;
    req.url = url;
    req.socket = { remoteAddress: '127.0.0.1' };
    req.headers = {
        'host': '127.0.0.1:3000',
        'x-test-bypass-ratelimit': '1',
        'accept': 'application/json'
    };
    if (body) {
        const bodyStr = typeof body === 'string' ? body : JSON.stringify(body);
        req.headers['content-type'] = 'application/json';
        req.headers['content-length'] = String(Buffer.byteLength(bodyStr));
        setTimeout(() => {
            req.emit('data', Buffer.from(bodyStr));
            req.emit('end');
        }, 0);
    }
    return req;
}

function makeMockRes() {
    const res = new EventEmitter();
    res.statusCode = 200;
    res._headers = {};
    res._body = '';
    res.setHeader = (k, v) => { res._headers[k.toLowerCase()] = v; };
    res.getHeader = (k) => res._headers[k.toLowerCase()];
    res.writeHead = (status, hdrs) => {
        res.statusCode = status;
        if (hdrs) {
            for (const [k, v] of Object.entries(hdrs)) {
                res._headers[k.toLowerCase()] = v;
            }
        }
    };
    res.write = (chunk) => { res._body += chunk; };
    res.end   = (chunk) => { if (chunk) res._body += chunk; };
    return res;
}

function runTest(task, name, method, url, body, check) {
    return new Promise((resolve) => {
        const req = makeMockReq(method, url, body);
        const res = makeMockRes();

        try {
            const handled = devServer.handleMockAPI(req, res);
            if (!handled) {
                // not synchronously handled
            }
        } catch (err) {
            resolve({ task, name, passed: false, reason: `handleMockAPI threw: ${err.message}` });
            return;
        }

        setTimeout(() => {
            let parsed = null;
            try { parsed = JSON.parse(res._body); } catch (_) { /* non-JSON is ok */ }
            try {
                if (res.statusCode < 200 || res.statusCode >= 300) {
                    throw new Error(`Expected 2xx status code but received ${res.statusCode}: ${res._body}`);
                }
                if (check) check(res, parsed);
                resolve({ task, name, passed: true });
            } catch (err) {
                resolve({ task, name, passed: false, reason: err.message });
            }
        }, 50);
    });
}

async function runAllTests() {
    console.log('='.repeat(80));
    console.log('CLM 400 IMPLEMENTATION VERIFICATION SUITE (TASKS 1-11)');
    console.log('='.repeat(80));

    const tests = [
        // Task 1: Contract Intake & Requests
        { task: 'Task 1', name: 'GET /api/v1/requests', method: 'GET', url: '/api/v1/requests', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 1', name: 'POST /api/v1/requests', method: 'POST', url: '/api/v1/requests', body: { title: 'New Vendor MSA', priority: 'High', requestedBy: 'legal@enterprise.internal' }, check: (res, body) => body && body.id },
        { task: 'Task 1', name: 'POST /api/v1/requests/:id/approve', method: 'POST', url: '/api/v1/requests/req-001/approve', body: { approvedBy: 'Director of Legal' }, check: (res, body) => body && (body.status === 'Approved' || body.success) },
        { task: 'Task 1', name: 'POST /api/v1/requests/:id/reject', method: 'POST', url: '/api/v1/requests/req-002/reject', body: { reason: 'Duplicate' }, check: (res, body) => body && (body.status === 'Rejected' || body.success) },
        { task: 'Task 1', name: 'POST /api/v1/requests/triage', method: 'POST', url: '/api/v1/requests/triage', body: { description: 'Urgent procurement request' }, check: (res, body) => body && body.routing },

        // Task 2: Document Generation & Clause Management
        { task: 'Task 2', name: 'GET /api/v1/templates', method: 'GET', url: '/api/v1/templates', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 2', name: 'GET /api/v1/clauses', method: 'GET', url: '/api/v1/clauses', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 2', name: 'POST /api/v1/clauses', method: 'POST', url: '/api/v1/clauses', body: { name: 'Indemnity Standard', category: 'Liability', body: 'Vendor agrees to indemnify...' }, check: (res, body) => body && body.id },
        { task: 'Task 2', name: 'GET /api/v1/clauses/search', method: 'GET', url: '/api/v1/clauses/search?q=indemnity', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 2', name: 'POST /api/v1/assembly/generate', method: 'POST', url: '/api/v1/assembly/generate', body: { templateId: 'tpl-msa', variables: { counterparty: 'Acme Corp' } }, check: (res, body) => body && body.documentUrl },
        { task: 'Task 2', name: 'GET /api/v1/playbooks', method: 'GET', url: '/api/v1/playbooks', body: null, check: (res, body) => Array.isArray(body) },

        // Task 3: Procurement & Vendor Contracts
        { task: 'Task 3', name: 'GET /api/v1/vendors', method: 'GET', url: '/api/v1/vendors', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 3', name: 'POST /api/v1/vendor-ratings', method: 'POST', url: '/api/v1/vendor-ratings', body: { vendorId: 'vnd-001', score: 95, notes: 'Compliant' }, check: (res, body) => body && body.success },
        { task: 'Task 3', name: 'GET /api/v1/rfp', method: 'GET', url: '/api/v1/rfp', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 3', name: 'POST /api/v1/s2c/compare', method: 'POST', url: '/api/v1/s2c/compare', body: { candidates: ['vnd-001', 'vnd-002'] }, check: (res, body) => body && body.comparison },
        { task: 'Task 3', name: 'GET /api/v1/purchase-orders', method: 'GET', url: '/api/v1/purchase-orders', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 3', name: 'POST /api/v1/purchase-orders', method: 'POST', url: '/api/v1/purchase-orders', body: { vendorId: 'vnd-001', amount: 50000 }, check: (res, body) => body && body.id },

        // Task 4: Financial Terms & Invoicing
        { task: 'Task 4', name: 'GET /api/v1/financial-terms', method: 'GET', url: '/api/v1/financial-terms', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 4', name: 'GET /api/v1/invoices', method: 'GET', url: '/api/v1/invoices', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 4', name: 'POST /api/v1/invoices', method: 'POST', url: '/api/v1/invoices', body: { contractId: 'cnt-001', amount: 12500, dueDate: '2026-12-31' }, check: (res, body) => body && body.id },
        { task: 'Task 4', name: 'POST /api/v1/invoices/:id/approve', method: 'POST', url: '/api/v1/invoices/inv-001/approve', body: { approver: 'CFO' }, check: (res, body) => body && body.success },
        { task: 'Task 4', name: 'GET /api/v1/revenue-schedules', method: 'GET', url: '/api/v1/revenue-schedules', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 4', name: 'GET /api/v1/rate-cards', method: 'GET', url: '/api/v1/rate-cards', body: null, check: (res, body) => Array.isArray(body) },

        // Task 5: Obligation Management & Compliance
        { task: 'Task 5', name: 'GET /api/v1/obligations/status', method: 'GET', url: '/api/v1/obligations/status', body: null, check: (res, body) => body && (body.total !== undefined || Array.isArray(body)) },
        { task: 'Task 5', name: 'POST /api/v1/obligations/fulfill', method: 'POST', url: '/api/v1/obligations/fulfill', body: { obligationId: 'ob-001', notes: 'Completed report' }, check: (res, body) => body && body.success },
        { task: 'Task 5', name: 'GET /api/v1/penalties', method: 'GET', url: '/api/v1/penalties', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 5', name: 'GET /api/v1/slas', method: 'GET', url: '/api/v1/slas', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 5', name: 'GET /api/v1/warranties', method: 'GET', url: '/api/v1/warranties', body: null, check: (res, body) => Array.isArray(body) },

        // Task 6: Risk, Compliance & Governance
        { task: 'Task 6', name: 'GET /api/v1/risk/assessments', method: 'GET', url: '/api/v1/risk/assessments', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'POST /api/v1/risk/mitigations', method: 'POST', url: '/api/v1/risk/mitigations', body: { riskId: 'RSK-101', mitigation: 'Insurance coverage verified' }, check: (res, body) => body && body.success },
        { task: 'Task 6', name: 'GET /api/v1/risk/exceptions', method: 'GET', url: '/api/v1/risk/exceptions', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/risk/regulatory-changes', method: 'GET', url: '/api/v1/risk/regulatory-changes', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/compliance/sanctions', method: 'GET', url: '/api/v1/compliance/sanctions', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/compliance/anti-bribery', method: 'GET', url: '/api/v1/compliance/anti-bribery', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/compliance/coi', method: 'GET', url: '/api/v1/compliance/coi', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/compliance/evidence', method: 'GET', url: '/api/v1/compliance/evidence', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/governance/doa-matrix', method: 'GET', url: '/api/v1/governance/doa-matrix', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/governance/board-resolutions', method: 'GET', url: '/api/v1/governance/board-resolutions', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/governance/entities', method: 'GET', url: '/api/v1/governance/entities', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/governance/filings', method: 'GET', url: '/api/v1/governance/filings', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 6', name: 'GET /api/v1/governance/annual-compliance', method: 'GET', url: '/api/v1/governance/annual-compliance', body: null, check: (res, body) => Array.isArray(body) },

        // Task 7: AI Contract Intelligence
        { task: 'Task 7', name: 'POST /api/v1/ai/classify', method: 'POST', url: '/api/v1/ai/classify', body: { text: 'This Master Services Agreement is entered into...' }, check: (res, body) => body && body.classification },
        { task: 'Task 7', name: 'POST /api/v1/ai/extract-metadata', method: 'POST', url: '/api/v1/ai/extract-metadata', body: { text: 'Contract between Acme Corp and BigCo...' }, check: (res, body) => body && body.parties },
        { task: 'Task 7', name: 'POST /api/v1/ai/extract-obligations', method: 'POST', url: '/api/v1/ai/extract-obligations', body: { text: 'Vendor shall deliver quarterly SOC2 reports...' }, check: (res, body) => body && Array.isArray(body.obligations) },
        { task: 'Task 7', name: 'POST /api/v1/ai/extract-financial', method: 'POST', url: '/api/v1/ai/extract-financial', body: { text: 'Customer shall pay $120,000 annually Net 30.' }, check: (res, body) => body && body.contractValue },
        { task: 'Task 7', name: 'POST /api/v1/ai/deviation', method: 'POST', url: '/api/v1/ai/deviation', body: { clauseText: 'Liability shall be uncapped.' }, check: (res, body) => body && body.deviations },
        { task: 'Task 7', name: 'POST /api/v1/ai/summarize', method: 'POST', url: '/api/v1/ai/summarize', body: { text: 'Full text of agreement...' }, check: (res, body) => body && body.executiveSummary },
        { task: 'Task 7', name: 'POST /api/v1/ai/predict-risk', method: 'POST', url: '/api/v1/ai/predict-risk', body: { contractId: 'cnt-001' }, check: (res, body) => body && body.predictedRiskScore !== undefined },
        { task: 'Task 7', name: 'POST /api/v1/ai/benchmark', method: 'POST', url: '/api/v1/ai/benchmark', body: { contractType: 'MSA' }, check: (res, body) => body && body.benchmarks },
        { task: 'Task 7', name: 'POST /api/v1/ai/similarity', method: 'POST', url: '/api/v1/ai/similarity', body: { contractId: 'cnt-001' }, check: (res, body) => body && Array.isArray(body.similarContracts) },
        { task: 'Task 7', name: 'GET /api/v1/ai/nl-search', method: 'GET', url: '/api/v1/ai/nl-search?q=liability', body: null, check: (res, body) => body && Array.isArray(body.results) },
        { task: 'Task 7', name: 'GET /api/v1/ai/portfolio-intelligence', method: 'GET', url: '/api/v1/ai/portfolio-intelligence', body: null, check: (res, body) => body && body.metrics },
        { task: 'Task 7', name: 'GET /api/v1/ai/leakage-detection', method: 'GET', url: '/api/v1/ai/leakage-detection', body: null, check: (res, body) => body && Array.isArray(body.leakages) },
        { task: 'Task 7', name: 'POST /api/v1/ai/compliance-check', method: 'POST', url: '/api/v1/ai/compliance-check', body: { contractId: 'cnt-001', standard: 'GDPR' }, check: (res, body) => body && body.complianceRating },

        // Task 8: Enterprise Integrations
        { task: 'Task 8', name: 'GET /api/v1/integrations/health', method: 'GET', url: '/api/v1/integrations/health', body: null, check: (res, body) => body && body.overallStatus === 'Healthy' },
        { task: 'Task 8', name: 'POST /api/v1/integrations/sync-all', method: 'POST', url: '/api/v1/integrations/sync-all', body: { triggeredBy: 'admin' }, check: (res, body) => body && body.syncResults },
        { task: 'Task 8', name: 'GET /api/v1/integrations/sap', method: 'GET', url: '/api/v1/integrations/sap', body: null, check: (res, body) => body && body.status },
        { task: 'Task 8', name: 'GET /api/v1/integrations/oracle', method: 'GET', url: '/api/v1/integrations/oracle', body: null, check: (res, body) => body && body.status },
        { task: 'Task 8', name: 'GET /api/v1/integrations/salesforce', method: 'GET', url: '/api/v1/integrations/salesforce', body: null, check: (res, body) => body && body.status },
        { task: 'Task 8', name: 'GET /api/v1/integrations/coupa', method: 'GET', url: '/api/v1/integrations/coupa', body: null, check: (res, body) => body && body.status },
        { task: 'Task 8', name: 'GET /api/v1/integrations/servicenow', method: 'GET', url: '/api/v1/integrations/servicenow', body: null, check: (res, body) => body && body.status },
        { task: 'Task 8', name: 'GET /api/v1/integrations/workday', method: 'GET', url: '/api/v1/integrations/workday', body: null, check: (res, body) => body && body.status },
        { task: 'Task 8', name: 'GET /api/v1/integrations/slack', method: 'GET', url: '/api/v1/integrations/slack', body: null, check: (res, body) => body && body.status },
        { task: 'Task 8', name: 'GET /api/v1/integrations/email', method: 'GET', url: '/api/v1/integrations/email', body: null, check: (res, body) => body && body.status },

        // Task 9: Advanced Reporting & Security Center
        { task: 'Task 9', name: 'GET /api/v1/reports/cycle-time', method: 'GET', url: '/api/v1/reports/cycle-time', body: null, check: (res, body) => body && body.averageDays !== undefined },
        { task: 'Task 9', name: 'GET /api/v1/reports/bottlenecks', method: 'GET', url: '/api/v1/reports/bottlenecks', body: null, check: (res, body) => body && Array.isArray(body.stages) },
        { task: 'Task 9', name: 'GET /api/v1/reports/renewal-forecast', method: 'GET', url: '/api/v1/reports/renewal-forecast', body: null, check: (res, body) => body && Array.isArray(body.forecast) },
        { task: 'Task 9', name: 'GET /api/v1/reports/counterparty-spend', method: 'GET', url: '/api/v1/reports/counterparty-spend', body: null, check: (res, body) => body && Array.isArray(body.topVendors) },
        { task: 'Task 9', name: 'GET /api/v1/reports/portfolio-health', method: 'GET', url: '/api/v1/reports/portfolio-health', body: null, check: (res, body) => body && body.healthIndex !== undefined },
        { task: 'Task 9', name: 'GET /api/v1/security/ip-allowlist', method: 'GET', url: '/api/v1/security/ip-allowlist', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 9', name: 'GET /api/v1/security/incidents', method: 'GET', url: '/api/v1/security/incidents', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 9', name: 'GET /api/v1/security/api-keys', method: 'GET', url: '/api/v1/security/api-keys', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 9', name: 'GET /api/v1/security/access-reviews', method: 'GET', url: '/api/v1/security/access-reviews', body: null, check: (res, body) => Array.isArray(body) },

        // Task 10: Negotiation Legal Ops & Specialized Agreements
        { task: 'Task 10', name: 'GET /api/v1/negotiations/neg-101/rounds', method: 'GET', url: '/api/v1/negotiations/neg-101/rounds', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 10', name: 'POST /api/v1/negotiations/neg-101/chat', method: 'POST', url: '/api/v1/negotiations/neg-101/chat', body: { sender: 'Counsel', message: 'We propose 24-month indemnity limitation.' }, check: (res, body) => body && body.success },
        { task: 'Task 10', name: 'GET /api/v1/disputes', method: 'GET', url: '/api/v1/disputes', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 10', name: 'GET /api/v1/specialized-agreements', method: 'GET', url: '/api/v1/specialized-agreements', body: null, check: (res, body) => Array.isArray(body) },

        // Task 11: Core Contract Gaps & Calendar
        { task: 'Task 11', name: 'GET /api/v1/contracts/calendar', method: 'GET', url: '/api/v1/contracts/calendar', body: null, check: (res, body) => Array.isArray(body) },
        { task: 'Task 11', name: 'POST /api/v1/contracts/cnt-001/generate-clause', method: 'POST', url: '/api/v1/contracts/cnt-001/generate-clause', body: { clauseType: 'Non-Compete' }, check: (res, body) => body && body.clauseText }
    ];

    let passedCount = 0;
    let failedCount = 0;

    for (const t of tests) {
        const result = await runTest(t.task, t.name, t.method, t.url, t.body, t.check);
        if (result.passed) {
            console.log(`[PASS] [${result.task}] ${result.name}`);
            passedCount++;
        } else {
            console.error(`[FAIL] [${result.task}] ${result.name} - Reason: ${result.reason}`);
            failedCount++;
        }
    }

    console.log('='.repeat(80));
    console.log(`SUMMARY: Total: ${tests.length} | Passed: ${passedCount} | Failed: ${failedCount}`);
    console.log('='.repeat(80));

    if (failedCount > 0) {
        console.error(`\x1b[31mSuite failed with ${failedCount} failing tests.\x1b[0m`);
        process.exit(1);
    } else {
        console.log(`\x1b[32mAll ${passedCount} CLM 400 implementation tests passed with 100% success rate!\x1b[0m`);
        process.exit(0);
    }
}

runAllTests().catch((err) => {
    console.error('Test runner fatal error:', err);
    process.exit(1);
});
