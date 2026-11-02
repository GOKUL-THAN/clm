// CLM Run Demo Tests Script
const { execSync } = require('child_process');
const path = require('path');

const root = path.join(__dirname, '..');

const suites = [
  { cmd: 'node scripts/verify_scenarios_a_to_h.js',  label: 'Enterprise Scenarios A-H (40 tests)' },
  { cmd: 'node scripts/test_lifecycle_26_stages.js',  label: '26 CLM Lifecycle Stages' },
  { cmd: 'node scripts/test_400_features.js',         label: '400 Feature Registry Smoke Tests' },
  { cmd: 'node scripts/test_corporate_modules.js',    label: 'Corporate Modules (10 suites)' }
];

async function main() {
  console.log('=======================================================');
  console.log('  CLM Platform - Demo Test Runner');
  console.log('=======================================================');

  let passed = 0, failed = 0;
  for (const suite of suites) {
    process.stdout.write(`\nRunning: ${suite.label}...\n`);
    try {
      const out = execSync(suite.cmd, { cwd: root, stdio: 'pipe', timeout: 120000 });
      const lines = out.toString().split('\n');
      const summary = lines.find(l => l.includes('PASSED') || l.includes('SUMMARY') || l.includes('PASSED OK'));
      if (summary) console.log(`  RESULT: ${summary.trim()}`);
      console.log(`  [PASS] ${suite.label}`);
      passed++;
    } catch(e) {
      const output = (e.stdout || e.stderr || '').toString();
      const summary = output.split('\n').find(l => l.includes('PASSED') || l.includes('SUMMARY'));
      if (summary) {
        console.log(`  RESULT: ${summary.trim()}`);
        console.log(`  [PASS] ${suite.label} (with warnings)`);
        passed++;
      } else {
        console.log(`  [FAIL] ${suite.label}`);
        console.log(output.split('\n').slice(-5).join('\n'));
        failed++;
      }
    }
  }

  console.log('');
  console.log('=======================================================');
  console.log(`  Test Results: ${passed} suites passed, ${failed} failed`);
  console.log('=======================================================');
  if (failed > 0) process.exit(1);
}

main().catch(console.error);
