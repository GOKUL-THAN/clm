// Starts the isolated Go/PostgreSQL API and the local portal in the foreground.
const { spawn } = require('child_process');
const crypto = require('crypto');
const fs = require('fs');
const http = require('http');
const net = require('net');
const path = require('path');

const root = path.resolve(__dirname, '..');
const backend = path.join(root, 'components', 'saas-control-plane');
const stateDir = path.join(root, '.clm-demo');
const sessionPath = path.join(stateDir, 'session.json');
const stopPath = path.join(stateDir, 'stop');
const demoBinary = path.join(backend, 'bin', process.platform === 'win32' ? 'clm-server.exe' : 'clm-server');

function readDemoEnv() {
  const file = path.join(root, '.env.demo.local');
  if (!fs.existsSync(file)) throw new Error('Run npm run demo:setup before starting the database-backed demo.');
  const values = {};
  for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const match = line.match(/^([A-Z0-9_]+)=(.*)$/);
    if (match) values[match[1]] = match[2];
  }
  for (const key of ['CLM_DEMO_DB_PASSWORD', 'CLM_DEMO_JWT_SECRET', 'CLM_DEMO_CLOUD_MASTER_KEY',
    'CLM_DEMO_SETUP_ID', 'CLM_DEMO_ADMIN_EMAIL', 'CLM_DEMO_ADMIN_PASSWORD', 'CLM_DEMO_ORGANIZATION']) {
    if (!values[key]) throw new Error(`Missing ${key} in .env.demo.local`);
  }
  return values;
}

function portOpen(port) {
  return new Promise(resolve => {
    const socket = net.createConnection({ host: '127.0.0.1', port });
    socket.setTimeout(800);
    socket.once('connect', () => { socket.destroy(); resolve(true); });
    socket.once('timeout', () => { socket.destroy(); resolve(false); });
    socket.once('error', () => resolve(false));
  });
}

function requestJson(url, method = 'GET', body, token) {
  return new Promise((resolve, reject) => {
    const data = body === undefined ? null : Buffer.from(JSON.stringify(body));
    const headers = {};
    if (data) {
      headers['Content-Type'] = 'application/json';
      headers['Content-Length'] = data.length;
    }
    if (token) headers.Authorization = `Bearer ${token}`;
    const request = http.request(url, {
      method,
      timeout: 3000,
      headers
    }, response => {
      let text = '';
      response.setEncoding('utf8');
      response.on('data', chunk => { text += chunk; });
      response.on('end', () => {
        let json = null;
        try { json = JSON.parse(text); } catch {}
        resolve({ status: response.statusCode, json, text });
      });
    });
    request.on('timeout', () => request.destroy(new Error('request timed out')));
    request.on('error', reject);
    if (data) request.write(data);
    request.end();
  });
}

async function waitForHealth(port, pathName, child, expectedService) {
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error(`Service exited before becoming healthy (exit ${child.exitCode}).`);
    try {
      const response = await requestJson(`http://127.0.0.1:${port}${pathName}`);
      if (response.status === 200 && response.json && response.json.service === expectedService) return;
    } catch {}
    await new Promise(resolve => setTimeout(resolve, 500));
  }
  throw new Error(`Service did not become healthy on port ${port} within 30 seconds.`);
}

function spawnManaged(command, args, options) {
  const child = spawn(command, args, { ...options, stdio: 'inherit', windowsHide: true });
  child.once('error', error => console.error(`Service failed to start: ${error.message}`));
  return child;
}

function demoBackendEnv(values) {
  const env = {};
  for (const key of ['PATH', 'SystemRoot', 'WINDIR', 'TEMP', 'TMP', 'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'HOME']) {
    if (process.env[key]) env[key] = process.env[key];
  }
  return {
    ...env,
    CLM_CONFIG: path.join(backend, 'configs', 'server-demo.yaml'),
    CLM_DEMO_DB_PASSWORD: values.CLM_DEMO_DB_PASSWORD,
    CLM_DEMO_JWT_SECRET: values.CLM_DEMO_JWT_SECRET,
    CLM_DEMO_CLOUD_MASTER_KEY: values.CLM_DEMO_CLOUD_MASTER_KEY
  };
}

async function seedDemoUser(values) {
  const base = 'http://127.0.0.1:3000';
  const credentials = { email: values.CLM_DEMO_ADMIN_EMAIL, password: values.CLM_DEMO_ADMIN_PASSWORD };
  let login = await requestJson(`${base}/api/v1/auth/login`, 'POST', credentials);
  if (login.status !== 200 || !login.json || !login.json.access_token) {
    if (login.status !== 401) {
      throw new Error(`Demo login check failed (${login.status}): ${login.text}`);
    }
    const registration = await requestJson(`${base}/api/v1/auth/register`, 'POST', {
      ...credentials,
      name: 'CLM Demo Administrator',
      organization: values.CLM_DEMO_ORGANIZATION
    });
    if (registration.status !== 201) {
      throw new Error(`Demo account initialization failed (${registration.status}): ${registration.text}`);
    }
    login = await requestJson(`${base}/api/v1/auth/login`, 'POST', credentials);
    if (login.status !== 200 || !login.json || !login.json.access_token) {
      throw new Error(`Demo account could not log in after registration (${login.status}): ${login.text}`);
    }
  }
  const token = login.json.access_token;
  const adminClaims = JSON.parse(Buffer.from(token.split('.')[1], 'base64url').toString('utf8'));
  if (!adminClaims.tenant_id) throw new Error('Demo admin token is missing its tenant identity.');
  const approver = { email: 'approver@enterprise.internal', password: values.CLM_DEMO_ADMIN_PASSWORD };
  let createdApprover = await requestJson(`${base}/api/v1/users`, 'POST', {
    ...approver,
    name: 'CLM Demo Approver',
    role: 'approver'
  }, token);
  if (createdApprover.status === 409) {
    const approverLogin = await requestJson(`${base}/api/v1/auth/login`, 'POST', approver);
    if (approverLogin.status !== 200 || !approverLogin.json || !approverLogin.json.access_token) {
      throw new Error(`Existing demo approver could not log in (${approverLogin.status}): ${approverLogin.text}`);
    }
    const approverClaims = JSON.parse(Buffer.from(approverLogin.json.access_token.split('.')[1], 'base64url').toString('utf8'));
    if (approverClaims.tenant_id !== adminClaims.tenant_id) {
      throw new Error('Existing demo approver belongs to another organization; refusing to reassign it.');
    }
  } else if (createdApprover.status !== 201) {
    throw new Error(`Demo approver initialization failed (${createdApprover.status}): ${createdApprover.text}`);
  }

  const templates = await requestJson(`${base}/api/v1/contracts/templates`, 'GET', undefined, token);
  if (templates.status !== 200 || !templates.json || !Array.isArray(templates.json.templates)) {
    throw new Error(`Could not load contract templates (${templates.status}): ${templates.text}`);
  }
  if (!templates.json.templates.some(item => item.name === 'CLM Demo MSA Template')) {
    const template = await requestJson(`${base}/api/v1/contracts/templates`, 'POST', {
      name: 'CLM Demo MSA Template',
      category: 'MSA',
      description: 'Local demonstration template; not legal advice.',
      content_body: 'MASTER SERVICES AGREEMENT\n\nThis agreement is between {{Customer}} and CLM Demo Organization. It is a non-binding demonstration document.',
      variables: ['Customer'],
      status: 'active'
    }, token);
    if (template.status !== 201) {
      throw new Error(`Demo template initialization failed (${template.status}): ${template.text}`);
    }
  }

  const contracts = await requestJson(`${base}/api/v1/contracts?search=CLM%20Demo%20MSA`, 'GET', undefined, token);
  if (contracts.status !== 200 || !contracts.json || !Array.isArray(contracts.json.contracts)) {
    throw new Error(`Could not load the demo contract repository (${contracts.status}): ${contracts.text}`);
  }
  if (!contracts.json.contracts.some(item => item.title === 'CLM Demo MSA')) {
    const contract = await requestJson(`${base}/api/v1/contracts`, 'POST', {
      title: 'CLM Demo MSA',
      type: 'MSA',
      status: 'draft',
      stage: 'authoring',
      counterparty_name: 'Northwind Demonstration Services',
      counterparty_email: 'contracts@northwind.example',
      department: 'Legal',
      total_value: 125000,
      currency: 'USD',
      content_body: 'Demonstration agreement for the CLM lifecycle. This text is fictional and non-binding.'
    }, token);
    if (contract.status !== 201) {
      throw new Error(`Demo contract initialization failed (${contract.status}): ${contract.text}`);
    }
  }
}

function stopChild(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return Promise.resolve();
  return new Promise(resolve => {
    let timeout;
    child.once('exit', () => { clearTimeout(timeout); resolve(); });
    if (!child.kill('SIGTERM')) {
      resolve();
      return;
    }
    timeout = setTimeout(() => child.kill(), 5000);
  });
}

async function main() {
  const values = readDemoEnv();
  if (!fs.existsSync(demoBinary)) throw new Error('Control-plane binary is missing; rerun npm run demo:setup.');
  fs.mkdirSync(stateDir, { recursive: true });
  if (fs.existsSync(sessionPath)) {
    throw new Error('A CLM demo session file already exists. Inspect .clm-demo/session.json before removing stale state.');
  }
  if (fs.existsSync(stopPath)) {
    throw new Error('A stale .clm-demo/stop request exists. Inspect it before starting a new session.');
  }
  for (const port of [3000, 8080, 8081]) {
    if (await portOpen(port)) throw new Error(`Port ${port} is already in use; stop that service before starting the demo.`);
  }

  const sessionId = crypto.randomUUID();
  const children = [];
  let sessionWritten = false;
  const webEnv = { ...process.env, CLM_DEMO_API: 'go' };
  try {
    const api = spawnManaged(demoBinary, [], { cwd: backend, env: demoBackendEnv(values) });
    children.push(api);
    await waitForHealth(8081, '/health', api, 'clm-saas-control-plane');

    const portal = spawnManaged(process.execPath, ['dev-server.js'], { cwd: root, env: webEnv });
    children.push(portal);
    const session = { id: sessionId, pid: process.pid, started_at: new Date().toISOString() };
    fs.writeFileSync(sessionPath, JSON.stringify(session, null, 2), { flag: 'wx' });
    sessionWritten = true;
    await waitForHealth(3000, '/api/v1/health', portal, 'clm-saas-control-plane');
    await waitForHealth(8080, '/api/v1/health', portal, 'clm-saas-control-plane');
    await seedDemoUser(values);

    console.log('');
    console.log('CLM database-backed demo is running.');
    console.log('  Admin portal:  http://127.0.0.1:3000');
    console.log('  Gateway portal: http://127.0.0.1:8080 (local portal preview)');
    console.log('  Go API health: http://127.0.0.1:8081/health');
    console.log(`  Demo login:    ${values.CLM_DEMO_ADMIN_EMAIL} / ${values.CLM_DEMO_ADMIN_PASSWORD}`);
    console.log('Use npm run demo:stop in another terminal or press Ctrl+C to stop.');

    await new Promise(resolve => {
      let done = false;
      const finish = () => {
        if (done) return;
        done = true;
        clearInterval(poll);
        process.removeListener('SIGINT', finish);
        process.removeListener('SIGTERM', finish);
        resolve();
      };
      const poll = setInterval(() => {
        try {
          const requestedSession = fs.readFileSync(stopPath, 'utf8').trim();
          if (requestedSession === sessionId) finish();
        } catch (error) {
          if (error.code !== 'ENOENT') console.error(`Could not read stop request: ${error.message}`);
        }
        const exited = children.find(child => child.exitCode !== null || child.signalCode !== null);
        if (exited) {
          console.error('A CLM demo service exited unexpectedly.');
          finish();
        }
      }, 500);
      process.once('SIGINT', finish);
      process.once('SIGTERM', finish);
    });
  } finally {
    await Promise.all(children.map(stopChild));
    if (sessionWritten) {
      try {
        const current = JSON.parse(fs.readFileSync(sessionPath, 'utf8'));
        if (current.id === sessionId) fs.unlinkSync(sessionPath);
      } catch (error) {
        if (error.code !== 'ENOENT') console.error(`Could not remove CLM session state: ${error.message}`);
      }
    }
    try {
      if (fs.readFileSync(stopPath, 'utf8').trim() === sessionId) fs.unlinkSync(stopPath);
    } catch (error) {
      if (error.code !== 'ENOENT') console.error(`Could not remove CLM stop request: ${error.message}`);
    }
  }
}

main().catch(error => {
  console.error(`[FAIL] ${error.message}`);
  process.exitCode = 1;
});
