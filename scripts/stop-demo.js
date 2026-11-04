// Requests shutdown from the start-demo process; never kills processes by name.
const fs = require('fs');
const http = require('http');
const path = require('path');

const stateDir = path.resolve(__dirname, '..', '.clm-demo');
const sessionPath = path.join(stateDir, 'session.json');
const stopPath = path.join(stateDir, 'stop');

function getJson(url) {
  return new Promise((resolve, reject) => {
    const request = http.get(url, { timeout: 2000 }, response => {
      let text = '';
      response.setEncoding('utf8');
      response.on('data', chunk => { text += chunk; });
      response.on('end', () => {
        let json = null;
        try { json = JSON.parse(text); } catch {}
        resolve({ status: response.statusCode, json });
      });
    });
    request.on('timeout', () => request.destroy(new Error('request timed out')));
    request.on('error', reject);
  });
}

async function main() {
  if (!fs.existsSync(sessionPath)) {
    throw new Error('No managed CLM demo session is running.');
  }
  const session = JSON.parse(fs.readFileSync(sessionPath, 'utf8'));
  if (!session.id || !Number.isInteger(session.pid)) {
    throw new Error('Invalid .clm-demo/session.json; inspect it before proceeding.');
  }

  const api = await getJson('http://127.0.0.1:8081/health');
  if (api.status !== 200 || !api.json || api.json.service !== 'clm-saas-control-plane') {
    throw new Error('The control-plane health check did not match the managed demo.');
  }
  const portal = await getJson('http://127.0.0.1:3000/api/v1/health');
  if (portal.status !== 200 || !portal.json || portal.json.service !== 'clm-saas-control-plane') {
    throw new Error('The portal proxy health check did not match the managed demo.');
  }
  if (fs.existsSync(stopPath)) {
    throw new Error('A shutdown request already exists; wait for the demo process to finish.');
  }

  fs.writeFileSync(stopPath, session.id, { flag: 'wx' });
  const deadline = Date.now() + 12000;
  while (Date.now() < deadline && fs.existsSync(sessionPath)) {
    await new Promise(resolve => setTimeout(resolve, 250));
  }
  if (fs.existsSync(sessionPath)) {
    throw new Error('The demo start process did not stop in time. No other processes were terminated.');
  }
  console.log('CLM demo services stopped.');
}

main().catch(error => {
  console.error(`[FAIL] ${error.message}`);
  process.exitCode = 1;
});
