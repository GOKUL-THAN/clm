// Runs repository suites, PostgreSQL integration tests, and the live CLM API workflow.
const { spawnSync } = require('child_process');
const fs = require('fs');
const http = require('http');
const path = require('path');

const root = path.resolve(__dirname, '..');
const backend = path.join(root, 'components', 'saas-control-plane');
const scripts = [
  ['scripts/verify_scenarios_a_to_h.js', 'In-memory enterprise scenarios A-H'],
  ['scripts/test_lifecycle_26_stages.js', 'In-memory 26-stage lifecycle suite'],
  ['scripts/test_400_features.js', '400-feature registry suite'],
  ['scripts/test_corporate_modules.js', 'Corporate module suite']
];

function loadDemoEnv() {
  const file = path.join(root, '.env.demo.local');
  if (!fs.existsSync(file)) throw new Error('Missing .env.demo.local; run npm run demo:setup first.');
  const values = {};
  for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const match = line.match(/^([A-Z0-9_]+)=(.*)$/);
    if (match) values[match[1]] = match[2];
  }
  for (const key of ['CLM_DEMO_DB_PASSWORD', 'CLM_DEMO_ADMIN_EMAIL', 'CLM_DEMO_ADMIN_PASSWORD']) {
    if (!values[key]) throw new Error(`Missing ${key} in .env.demo.local`);
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
    const req = http.request(url, { method, headers, timeout: 10000 }, res => {
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

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

async function api(method, endpoint, body, token, expectedStatus = 200) {
  const response = await request(`http://127.0.0.1:3000${endpoint}`, method, body, token);
  assert(response.status === expectedStatus,
    `${method} ${endpoint}: expected ${expectedStatus}, received ${response.status}: ${response.text}`);
  return response.json;
}

function tokenClaims(token) {
  const payload = token.split('.')[1];
  if (!payload) throw new Error('Malformed access token');
  return JSON.parse(Buffer.from(payload, 'base64url').toString('utf8'));
}

async function loginOrRegister(email, password, name, organization) {
  let response = await request('http://127.0.0.1:3000/api/v1/auth/login', 'POST', { email, password });
  if (response.status === 401) {
    response = await request('http://127.0.0.1:3000/api/v1/auth/register', 'POST', {
      email, password, name, organization
    });
    assert(response.status === 201, `registration failed for ${email}: ${response.text}`);
    response = await request('http://127.0.0.1:3000/api/v1/auth/login', 'POST', { email, password });
  }
  assert(response.status === 200 && response.json && response.json.access_token,
    `login failed for ${email}: ${response.text}`);
  return response.json.access_token;
}

async function runLiveWorkflow(values) {
  const stages = [];
  const mark = async (label, action) => {
    await action();
    stages.push(label);
  };
  const adminToken = await loginOrRegister(
    values.CLM_DEMO_ADMIN_EMAIL, values.CLM_DEMO_ADMIN_PASSWORD,
    'CLM Demo Administrator', values.CLM_DEMO_ORGANIZATION || 'CLM Demonstration Organization'
  );
  const claims = tokenClaims(adminToken);
  assert(claims.tenant_id && claims.user_id, 'login token is missing user or tenant identity');
  const tenantID = claims.tenant_id;
  const unique = new Date().toISOString().replace(/[-:.TZ]/g, '');
  const title = `Automated CLM workflow ${unique}`;
  let contractID = '';

  await mark('Tenant-authenticated contract creation', async () => {
    const result = await api('POST', '/api/v1/contracts', {
      title,
      type: 'MSA',
      status: 'draft',
      stage: 'authoring',
      counterparty_name: 'Contoso Demo Services',
      counterparty_email: 'legal@contoso.example',
      department: 'Legal',
      total_value: 250000,
      currency: 'USD',
      content_body: 'Fictional contract text created by the local API test.'
    }, adminToken, 201);
    assert(result.contract && result.contract.id, 'contract create response omitted its ID');
    assert(result.contract.tenant_id === tenantID, 'contract was not stored in the authenticated tenant');
    contractID = result.contract.id;
  });

  await mark('Persisted contract read and version history', async () => {
    const found = await api('GET', `/api/v1/contracts/${contractID}`, undefined, adminToken);
    assert(found.contract && found.contract.title === title, 'created contract was not persisted');
    const version = await api('POST', `/api/v1/contracts/${contractID}/versions`, {
      title,
      content_body: 'Version two of the fictional demo agreement.',
      changelog_summary: 'Automated demo version',
      author_email: values.CLM_DEMO_ADMIN_EMAIL
    }, adminToken, 201);
    assert(version.version && version.version.version_number === 2, 'contract version was not persisted');
  });

  await mark('Template assembly and negotiation redline', async () => {
    const templates = await api('GET', '/api/v1/contracts/templates', undefined, adminToken);
    const template = templates.templates.find(item => item.name === 'CLM Demo MSA Template');
    assert(template, 'database-backed demo template is missing');
    const assembled = await api('POST', '/api/v1/contracts/templates/assemble', {
      template_id: template.id,
      variables: { Customer: 'Contoso' },
      selected_clause_ids: []
    }, adminToken);
    assert(assembled.assembled_document.includes('Contoso'), 'template variables were not assembled');
    const redline = await api('POST', `/api/v1/contracts/${contractID}/negotiations`, {
      author_name: 'Contoso Legal',
      author_type: 'counterparty',
      change_type: 'modification',
      clause_title: 'Limitation of liability',
      original_text: 'Liability is unlimited.',
      proposed_text: 'Liability is capped at fees paid.'
    }, adminToken, 201);
    assert(redline.redline && redline.redline.id, 'redline was not persisted');
  });

  await mark('Internal contract risk analysis', async () => {
    const analysis = await api('POST', `/api/v1/contracts/${contractID}/ai-analyze`, {}, adminToken);
    assert(analysis.analysis, 'risk analysis returned no result');
  });

  const approverEmail = 'approver@enterprise.internal';
  const approverPassword = values.CLM_DEMO_ADMIN_PASSWORD;
  let approverLogin = await request('http://127.0.0.1:3000/api/v1/auth/login', 'POST', {
    email: approverEmail,
    password: approverPassword
  });
  if (approverLogin.status === 401) {
    await api('POST', '/api/v1/users', {
      email: approverEmail,
      password: approverPassword,
      name: 'CLM Demo Approver',
      role: 'approver'
    }, adminToken, 201);
    approverLogin = await request('http://127.0.0.1:3000/api/v1/auth/login', 'POST', {
      email: approverEmail,
      password: approverPassword
    });
  }
  assert(approverLogin.status === 200 && approverLogin.json && approverLogin.json.access_token,
    `demo approver login failed: ${approverLogin.text}`);
  const approverToken = approverLogin.json.access_token;
  assert(tokenClaims(approverToken).tenant_id === tenantID, 'approver is not in the demo tenant');

  await mark('Approval request and separate approver decision', async () => {
    const requestBody = {
      request_type: 'cert_create',
      justification: `Automated approval for ${title}`,
      estimated_cost: 250,
      cert_type: 'DV',
      environment: 'prod',
      payload: JSON.stringify({ contract_id: contractID })
    };
    const approval = await api('POST', '/api/v1/approvals', requestBody, adminToken, 201);
    assert(approval.id, 'approval request was not persisted');
    const decision = await api('POST', `/api/v1/approvals/${approval.id}/approve`, {
      comment: 'Approved by the separate demo approver.'
    }, approverToken);
    assert(decision.approval && decision.approval.status === 'approved', 'approval workflow did not complete');
  });

  await mark('Demo signature and obligation fulfillment', async () => {
    await api('POST', `/api/v1/contracts/${contractID}/transition`, {
      target_status: 'ready_for_signature',
      target_stage: 'signature',
      comment: 'Approval complete; ready for demo signing.'
    }, adminToken);
    const signer = await api('POST', `/api/v1/contracts/${contractID}/signatures`, {
      signer_name: 'CLM Demo Administrator',
      signer_email: values.CLM_DEMO_ADMIN_EMAIL,
      signer_role: 'authorized_demo_signer',
      signing_order: 1
    }, adminToken, 201);
    assert(signer.signature && signer.signature.id, 'signer record was not persisted');
    const signed = await api('POST', `/api/v1/contracts/signatures/${signer.signature.id}/execute`, {
      auth_method: 'demo_hash_attestation'
    }, adminToken);
    assert(signed.signature.status === 'signed' && /^[a-f0-9]{64}$/.test(signed.signature.digital_signature_hash),
      'demo signature evidence was not generated');
    const obligation = await api('POST', `/api/v1/contracts/${contractID}/obligations`, {
      title: 'Demo quarterly compliance review',
      category: 'Regulatory',
      party_responsible: 'internal',
      owner_email: values.CLM_DEMO_ADMIN_EMAIL,
      due_date: new Date(Date.now() + 86400000).toISOString(),
      recurrence: 'quarterly'
    }, adminToken, 201);
    assert(obligation.obligation && obligation.obligation.id, 'obligation was not persisted');
    const fulfilled = await api('POST',
      `/api/v1/contracts/obligations/${obligation.obligation.id}/fulfill`,
      { verified_by: values.CLM_DEMO_ADMIN_EMAIL, evidence_notes: 'Completed by automated demo workflow.' },
      adminToken);
    assert(fulfilled.obligation.status === 'fulfilled', 'obligation was not fulfilled');
  });

  await mark('Cross-tenant contract isolation', async () => {
    const isolationToken = await loginOrRegister(
      'isolation-probe@enterprise.internal',
      values.CLM_DEMO_ADMIN_PASSWORD,
      'CLM Isolation Probe',
      'CLM Isolation Probe'
    );
    assert(tokenClaims(isolationToken).tenant_id !== tenantID, 'isolation probe unexpectedly shares the demo tenant');
    const response = await request(`http://127.0.0.1:3000/api/v1/contracts/${contractID}`,
      'GET', undefined, isolationToken);
    assert(response.status === 404, `other tenant received an unexpected response (${response.status})`);
  });

  await mark('Audit event persistence and API authorization', async () => {
    const audits = await api('GET', '/api/v1/audits', undefined, adminToken);
    assert(Array.isArray(audits.audit_events) && audits.audit_events.length > 0, 'workflow mutations produced no audit events');
    const denied = await request('http://127.0.0.1:3000/api/v1/contracts');
    assert(denied.status === 401, `unauthenticated contract request returned ${denied.status}`);
  });

  console.log('Live PostgreSQL-backed API workflow:');
  for (const stage of stages) console.log(`  [PASS] ${stage}`);
}

function findGo() {
  if (process.env.CLM_GO_PATH) return process.env.CLM_GO_PATH;
  if (process.platform === 'win32') {
    const candidate = path.join(process.env.ProgramFiles || 'C:\\Program Files', 'Go', 'bin', 'go.exe');
    if (fs.existsSync(candidate)) return candidate;
  }
  return 'go';
}

function execute(command, args, cwd, env) {
  const result = spawnSync(command, args, {
    cwd,
    env,
    encoding: 'utf8',
    timeout: 300000,
    maxBuffer: 20 * 1024 * 1024,
    windowsHide: true
  });
  if (result.error) throw new Error(`${command} could not run: ${result.error.message}`);
  if (result.status !== 0) {
    const details = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
    throw new Error(`${command} exited with code ${result.status}${details ? `:\n${details}` : ''}`);
  }
  return result.stdout || '';
}

async function main() {
  const values = loadDemoEnv();
  const results = [];
  for (const [file, label] of scripts) {
    try {
      const output = execute(process.execPath, [file], root);
      const summary = output.split(/\r?\n/).find(line => /SUMMARY|PASSED|RESULTS/i.test(line));
      results.push({ label, ok: true, detail: summary && summary.trim() });
    } catch (error) {
      results.push({ label, ok: false, detail: error.message });
    }
  }

  const testEnv = { ...process.env };
  delete testEnv.PGHOST;
  delete testEnv.PGPORT;
  delete testEnv.PGUSER;
  delete testEnv.PGDATABASE;
  delete testEnv.PGPASSWORD;
  testEnv.CLM_TEST_DB_DSN = `host=127.0.0.1 port=5432 user=clm_demo password=${values.CLM_DEMO_DB_PASSWORD} dbname=clm_demo sslmode=disable`;
  testEnv.CLM_TEST_DB_REQUIRED = '1';
  try {
    const output = execute(findGo(), ['test', './...'], backend, testEnv);
    results.push({ label: 'Go tests with isolated PostgreSQL integration tests', ok: true,
      detail: output.split(/\r?\n/).filter(line => /^ok\s|^FAIL\s/.test(line)).slice(-4).join('; ') });
  } catch (error) {
    results.push({ label: 'Go tests with isolated PostgreSQL integration tests', ok: false, detail: error.message });
  }

  try {
    await runLiveWorkflow(values);
    results.push({ label: 'Live PostgreSQL-backed CLM API workflow', ok: true });
  } catch (error) {
    results.push({ label: 'Live PostgreSQL-backed CLM API workflow', ok: false, detail: error.message });
  }

  console.log('\nCLM demo test results');
  let failed = 0;
  for (const result of results) {
    console.log(`  [${result.ok ? 'PASS' : 'FAIL'}] ${result.label}${result.detail ? ` — ${result.detail}` : ''}`);
    if (!result.ok) failed++;
  }
  console.log(`Suites: ${results.length - failed} passed, ${failed} failed`);
  if (failed) process.exitCode = 1;
}

main().catch(error => {
  console.error(`[FAIL] ${error.message}`);
  process.exitCode = 1;
});
