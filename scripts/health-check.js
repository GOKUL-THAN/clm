// Verifies the local database-backed CLM demo through its real API.
const fs = require('fs');
const http = require('http');
const path = require('path');

const root = path.resolve(__dirname, '..');

function loadDemoEnv() {
  const file = path.join(root, '.env.demo.local');
  if (!fs.existsSync(file)) throw new Error('Missing .env.demo.local; run npm run demo:setup first.');
  const values = {};
  for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const match = line.match(/^([A-Z0-9_]+)=(.*)$/);
    if (match) values[match[1]] = match[2];
  }
  if (!values.CLM_DEMO_ADMIN_EMAIL || !values.CLM_DEMO_ADMIN_PASSWORD) {
    throw new Error('Demo login credentials are missing from .env.demo.local.');
  }
  return values;
}

function request(url, method = 'GET', body, token) {
  return new Promise((resolve, reject) => {
    const data = body === undefined ? null : Buffer.from(JSON.stringify(body));
    const headers = {};
    if (data) {
      headers['Content-Type'] = 'application/json';
      headers['Content-Length'] = data.length;
    }
    if (token) headers.Authorization = `Bearer ${token}`;
    const req = http.request(url, { method, headers, timeout: 5000 }, res => {
      let text = '';
      res.setEncoding('utf8');
      res.on('data', chunk => { text += chunk; });
      res.on('end', () => {
        let json = null;
        try { json = JSON.parse(text); } catch {}
        resolve({ status: res.statusCode, json, text });
      });
    });
    req.on('timeout', () => req.destroy(new Error('request timed out')));
    req.on('error', reject);
    if (data) req.write(data);
    req.end();
  });
}

async function main() {
  const env = loadDemoEnv();
  const checks = [];
  const check = async (label, fn) => {
    try {
      await fn();
      checks.push({ label, ok: true });
    } catch (error) {
      checks.push({ label, ok: false, error: error.message });
    }
  };
  const expectStatus = (response, status) => {
    if (response.status !== status) {
      throw new Error(`expected HTTP ${status}, received ${response.status}: ${response.text}`);
    }
  };

  await check('Go control plane and database startup', async () => {
    const response = await request('http://127.0.0.1:8081/health');
    expectStatus(response, 200);
    if (!response.json || response.json.service !== 'clm-saas-control-plane') {
      throw new Error('unexpected control-plane health response');
    }
  });
  await check('Admin portal', async () => {
    const response = await request('http://127.0.0.1:3000/');
    expectStatus(response, 200);
    if (!response.text.toLowerCase().includes('<html')) throw new Error('portal did not return HTML');
  });
  await check('Gateway portal preview', async () => {
    const response = await request('http://127.0.0.1:8080/');
    expectStatus(response, 200);
    if (!response.text.toLowerCase().includes('<html')) throw new Error('gateway portal did not return HTML');
  });
  await check('Admin portal proxies to the Go API', async () => {
    const response = await request('http://127.0.0.1:3000/api/v1/health');
    expectStatus(response, 200);
    if (!response.json || response.json.service !== 'clm-saas-control-plane') {
      throw new Error('health request did not reach the Go API');
    }
  });

  let token = '';
  await check('Demo authentication', async () => {
    const response = await request('http://127.0.0.1:3000/api/v1/auth/login', 'POST', {
      email: env.CLM_DEMO_ADMIN_EMAIL,
      password: env.CLM_DEMO_ADMIN_PASSWORD
    });
    expectStatus(response, 200);
    if (!response.json || !response.json.access_token) throw new Error('API returned no access token');
    token = response.json.access_token;
  });

  if (token) {
    await check('Tenant-scoped contract repository', async () => {
      const response = await request('http://127.0.0.1:3000/api/v1/contracts', 'GET', undefined, token);
      expectStatus(response, 200);
      if (!response.json || !Array.isArray(response.json.contracts)) {
        throw new Error('contract list response has an unexpected shape');
      }
    });
    await check('Contract analytics', async () => {
      const response = await request('http://127.0.0.1:3000/api/v1/contracts/analytics', 'GET', undefined, token);
      expectStatus(response, 200);
      if (!response.json || typeof response.json.total_contracts !== 'number') {
        throw new Error('analytics response has an unexpected shape');
      }
    });
    await check('Approval workflow API', async () => {
      const response = await request('http://127.0.0.1:3000/api/v1/approvals/pending', 'GET', undefined, token);
      expectStatus(response, 200);
    });
    await check('Audit history API', async () => {
      const response = await request('http://127.0.0.1:3000/api/v1/audits', 'GET', undefined, token);
      expectStatus(response, 200);
    });
  }

  console.log('CLM database-backed demo health check');
  let failed = 0;
  for (const result of checks) {
    console.log(`  [${result.ok ? 'PASS' : 'FAIL'}] ${result.label}${result.error ? `: ${result.error}` : ''}`);
    if (!result.ok) failed++;
  }
  console.log(`Results: ${checks.length - failed} passed, ${failed} failed`);
  if (failed) process.exitCode = 1;
}

main().catch(error => {
  console.error(`[FAIL] ${error.message}`);
  process.exitCode = 1;
});
