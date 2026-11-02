// CLM Health Check Script
const http = require('http');

const checks = [
  { url: 'http://localhost:3000/api/v1/health',            label: 'SaaS Control Plane Health' },
  { url: 'http://localhost:3000/',                          label: 'SaaS Admin Portal HTML' },
  { url: 'http://localhost:8080/',                          label: 'Gateway Portal HTML' },
  { url: 'http://localhost:3000/api/v1/tenants',            label: 'Tenants API' },
  { url: 'http://localhost:3000/api/v1/contracts',          label: 'Contracts API' },
  { url: 'http://localhost:3000/api/v1/contracts/metrics',  label: 'Contract Metrics API' },
  { url: 'http://localhost:3000/api/v1/templates',          label: 'Templates API' },
  { url: 'http://localhost:3000/api/v1/approvals/pending',  label: 'Approvals API' },
  { url: 'http://localhost:3000/api/v1/obligations',        label: 'Obligations API' },
  { url: 'http://localhost:3000/api/v1/clm-reports/summary',label: 'CLM Reports API' },
  { url: 'http://localhost:3000/api/v1/identity/status',    label: 'Identity Service' },
  { url: 'http://localhost:3000/api/v1/features',           label: '400 Features Registry' }
];

function get(url) {
  return new Promise(function(resolve) {
    var req = http.get(url, { timeout: 5000 }, function(res) {
      var data = '';
      res.on('data', function(c) { data += c; });
      res.on('end', function() { resolve({ ok: res.statusCode < 400, status: res.statusCode, body: data }); });
    });
    req.on('error', function(e) { resolve({ ok: false, status: 0, error: e.message }); });
    req.on('timeout', function() { req.destroy(); resolve({ ok: false, status: 0, error: 'Timeout' }); });
  });
}

async function main() {
  console.log('');
  console.log('=======================================================');
  console.log('  CLM Platform Health Check');
  console.log('=======================================================');
  var passed = 0, failed = 0;
  for (var i = 0; i < checks.length; i++) {
    var check = checks[i];
    var result = await get(check.url);
    if (result.ok) {
      console.log('  [PASS] ' + check.label);
      passed++;
    } else {
      console.log('  [FAIL] ' + check.label + ' (' + (result.status || result.error) + ')');
      failed++;
    }
  }
  console.log('=======================================================');
  console.log('  Results: ' + passed + ' passed, ' + failed + ' failed');
  console.log('=======================================================');
  console.log('');
  if (failed > 0) {
    console.log('Some checks failed. Ensure the demo is running: node dev-server.js');
    process.exit(1);
  }
}

main().catch(console.error);