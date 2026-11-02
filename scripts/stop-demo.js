// CLM Demo Stop Script
const { execSync } = require('child_process');
const http = require('http');

console.log('Stopping CLM demo servers...');

function tryStop(port) {
  return new Promise(resolve => {
    const req = http.get(`http://localhost:${port}/api/v1/health`, { timeout: 2000 }, res => {
      console.log(`Port ${port} was running, stopping...`);
      resolve(true);
    });
    req.on('error', () => { console.log(`Port ${port} not running.`); resolve(false); });
  });
}

async function main() {
  await Promise.all([tryStop(3000), tryStop(8080)]);

  try {
    if (process.platform === 'win32') {
      execSync('taskkill /F /IM node.exe /T', { stdio: 'pipe' });
    } else {
      execSync("pkill -f 'node dev-server.js'", { stdio: 'pipe' });
    }
    console.log('CLM demo servers stopped.');
  } catch(e) {
    console.log('No running CLM server processes found.');
  }
}

main().catch(console.error);
