// CLM Demo Setup Script
// Sets up the local demo environment
const { execSync } = require('child_process');
const http = require('http');
const fs = require('fs');
const path = require('path');

console.log('=======================================================');
console.log('  CLM Platform - Demo Setup');
console.log('=======================================================');

// Check Node.js version
const nodeMajor = parseInt(process.version.slice(1).split('.')[0]);
if (nodeMajor < 18) {
  console.error(`ERROR: Node.js 18+ required. Found ${process.version}`);
  process.exit(1);
}
console.log(`[OK] Node.js ${process.version}`);

// Check required files
const required = [
  'dev-server.js',
  'public/index.html',
  'public/app.js',
  'components/saas-control-plane/web/admin-portal/index.html',
  'components/saas-control-plane/web/admin-portal/app.js',
  'components/satellite-gateway/web/gateway-ui/index.html',
  'components/satellite-gateway/web/gateway-ui/app.js',
  'features/registry.js',
  'scripts/workflow-engine.js'
];

let allPresent = true;
required.forEach(f => {
  const fp = path.join(__dirname, f);
  if (fs.existsSync(fp)) {
    console.log(`[OK] ${f}`);
  } else {
    console.error(`[MISSING] ${f}`);
    allPresent = false;
  }
});

if (!allPresent) {
  console.error('\nERROR: Some required files are missing. Check repository integrity.');
  process.exit(1);
}

// Verify dev-server.js syntax
try {
  execSync(`node -c "${path.join(__dirname, 'dev-server.js')}"`, { stdio: 'pipe' });
  console.log('[OK] dev-server.js syntax valid');
} catch(e) {
  console.error('[FAIL] dev-server.js has syntax errors:', e.stderr?.toString());
  process.exit(1);
}

console.log('');
console.log('=======================================================');
console.log('  Setup complete! Run: node scripts/start-demo.js');
console.log('=======================================================');
