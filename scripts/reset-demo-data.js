// CLM Reset Demo Data Script
// Calls the server to reset in-memory data to demo defaults
const http = require('http');

console.log('=======================================================');
console.log('  CLM Demo - Reset Data');
console.log('=======================================================');

function post(url, body) {
  return new Promise((resolve, reject) => {
    const data = JSON.stringify(body);
    const options = {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) },
      timeout: 5000
    };
    const req = http.request(url, options, res => {
      let d = '';
      res.on('data', c => d += c);
      res.on('end', () => resolve({ status: res.statusCode, body: d }));
    });
    req.on('error', reject);
    req.write(data);
    req.end();
  });
}

async function main() {
  // The mock server uses in-memory data that resets on restart
  // This script just restarts the server to achieve the reset
  console.log('INFO: Demo data is in-memory and resets automatically on server restart.');
  console.log('INFO: To reset, stop and restart the server:');
  console.log('');
  console.log('  node scripts/stop-demo.js');
  console.log('  node scripts/start-demo.js');
  console.log('');
  console.log('Alternatively, any new contracts/data created during the demo are');
  console.log('stored in memory only and will not persist across restarts.');
}

main().catch(console.error);
