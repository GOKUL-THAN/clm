// CLM Demo Start Script
const { spawn } = require('child_process');
const http = require('http');
const path = require('path');

function checkPort(port) {
  return new Promise(resolve => {
    const req = http.get(`http://localhost:${port}/api/v1/health`, { timeout: 3000 }, res => {
      resolve(res.statusCode === 200);
    });
    req.on('error', () => resolve(false));
    req.on('timeout', () => { req.destroy(); resolve(false); });
  });
}

async function main() {
  console.log('=======================================================');
  console.log('  CLM Platform - Starting Demo');
  console.log('=======================================================');

  // Check if already running
  const alreadyRunning = await checkPort(3000);
  if (alreadyRunning) {
    console.log('[INFO] CLM server already running on port 3000');
    console.log('');
    console.log('  SaaS Admin Portal:  http://localhost:3000');
    console.log('  Gateway Portal:     http://localhost:8080');
    console.log('  Login:              admin@enterprise.internal / Demo@2024!');
    return;
  }

  const root = path.join(__dirname, '..');
  const proc = spawn('node', ['dev-server.js'], {
    cwd: root,
    stdio: 'inherit',
    detached: false
  });

  proc.on('error', err => {
    console.error('Failed to start server:', err.message);
    process.exit(1);
  });

  // Wait for server to be ready
  let attempts = 0;
  const maxAttempts = 30;
  while (attempts < maxAttempts) {
    await new Promise(r => setTimeout(r, 500));
    const ready = await checkPort(3000);
    if (ready) {
      console.log('');
      console.log('=======================================================');
      console.log('  CLM Demo is RUNNING');
      console.log('=======================================================');
      console.log('');
      console.log('  SaaS Admin Portal:  http://localhost:3000');
      console.log('  Gateway Portal:     http://localhost:8080');
      console.log('  Health Check:       http://localhost:3000/api/v1/health');
      console.log('');
      console.log('  Demo Credentials:');
      console.log('    Email:    admin@enterprise.internal');
      console.log('    Password: Demo@2024!');
      console.log('');
      console.log('  Press Ctrl+C to stop.');
      console.log('=======================================================');
      return;
    }
    attempts++;
  }

  console.error('Server did not start in time. Check for errors above.');
}

main().catch(console.error);
