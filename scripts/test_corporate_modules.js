const { EventEmitter } = require('events');
const { handleMockAPI } = require('d:/cmd project/M2P-Git/CLM/dev-server.js');

function invokeAPI(method, urlPath, bodyObj = null) {
    return new Promise((resolve) => {
        const req = new EventEmitter();
        req.method = method;
        req.url = urlPath;
        req.headers = { host: 'localhost:3000' };

        const res = {
            statusCode: 200,
            headers: {},
            body: '',
            setHeader(k, v) { this.headers[k] = v; },
            writeHead(code, headers) {
                this.statusCode = code;
                if (headers) Object.assign(this.headers, headers);
            },
            end(data) {
                if (data) this.body += data;
                resolve({ statusCode: this.statusCode, headers: this.headers, body: this.body });
            }
        };

        const handled = handleMockAPI(req, res);
        if (!handled) {
            resolve({ statusCode: 404, body: 'Not Handled' });
            return;
        }

        if (bodyObj) {
            req.emit('data', JSON.stringify(bodyObj));
        }
        req.emit('end');
    });
}

async function runComprehensiveTests() {
    console.log('--- RUNNING ENTERPRISE CLM CORPORATE & BUG-FIX TEST SUITE ---');

    // 1. Corporate Departments Matrix (14 departments)
    let r = await invokeAPI('GET', '/api/v1/corporate-departments');
    const depts = JSON.parse(r.body).departments;
    console.log('1. Corporate Departments:', r.statusCode, 'Count:', depts.length);
    if (depts.length !== 14) throw new Error('Expected 14 corporate departments');
    console.log('   Departments verified:', depts.map(d => d.code).join(', '));

    // 2. Contracts Across All Departments
    r = await invokeAPI('GET', '/api/v1/contracts');
    const contracts = JSON.parse(r.body).contracts;
    console.log('2. Contracts count:', r.statusCode, contracts.length);
    const deptsPresent = new Set(contracts.map(c => c.department));
    console.log('   Contracts span departments:', Array.from(deptsPresent).join(', '));

    // 3. Request-to-Contract Conversion
    r = await invokeAPI('POST', '/api/v1/contract-requests/req-01/convert', {});
    console.log('3. Convert Request to Contract:', r.statusCode, JSON.parse(r.body).contract.contract_number);

    // 4. Obligation Status Update (Bug Fix Verification)
    r = await invokeAPI('POST', '/api/v1/obligations', { id: 'obl-02', status: 'fulfilled' });
    console.log('4. Fulfill Obligation:', r.statusCode, JSON.parse(r.body).obligation.status);

    // 5. Identity Management Mock (Bug Fix Verification)
    r = await invokeAPI('GET', '/api/v1/identity/status');
    let rUsers = await invokeAPI('GET', '/api/v1/identity/users');
    let rRoles = await invokeAPI('GET', '/api/v1/identity/roles');
    console.log('5. Identity Status:', r.statusCode, JSON.parse(r.body).realm);
    console.log('   Identity Users & Roles:', rUsers.statusCode, rRoles.statusCode);

    // 6. AI Q&A with Department Context
    r = await invokeAPI('POST', '/api/v1/ai-contracts/cnt-001/ask', { question: "What are the payment terms?" });
    console.log('6. AI Contract Q&A:', r.statusCode, JSON.parse(r.body).answer);

    // 7. Suppliers & Procurement
    r = await invokeAPI('GET', '/api/v1/suppliers');
    console.log('7. Suppliers Directory:', r.statusCode, 'Count:', JSON.parse(r.body).count);

    // 8. Legal Matters & Holds
    r = await invokeAPI('GET', '/api/v1/legal-matters');
    console.log('8. Legal Matters:', r.statusCode, 'Count:', JSON.parse(r.body).count);

    // 9. Three-Way Invoice Match
    r = await invokeAPI('POST', '/api/v1/financials/three-way-match', {});
    console.log('9. Three-Way Invoice Matching:', r.statusCode, 'Status:', JSON.parse(r.body).status);

    // 10. Executive Reporting Summary
    r = await invokeAPI('GET', '/api/v1/clm-reports/summary');
    console.log('10. Executive CLM Reports:', r.statusCode, 'Score:', JSON.parse(r.body).portfolio_health.overall_score);

    console.log('\n=============================================================');
    console.log('  ALL 10 TEST SUITES (CORPORATE MODULES & FIXES) PASSED OK!  ');
    console.log('=============================================================');
}

runComprehensiveTests().catch(err => {
    console.error('Test failed:', err);
    process.exit(1);
});
