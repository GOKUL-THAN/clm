// Drops only the CLM demo database after explicit confirmation and ownership checks.
const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const envPath = path.join(root, '.env.demo.local');
const sessionPath = path.join(root, '.clm-demo', 'session.json');

function findPsql() {
  if (process.env.PSQL_PATH) {
    if (!fs.existsSync(process.env.PSQL_PATH)) throw new Error(`PSQL_PATH does not exist: ${process.env.PSQL_PATH}`);
    return process.env.PSQL_PATH;
  }
  if (process.platform === 'win32') {
    for (const version of ['18', '17', '16', '15', '14']) {
      const candidate = path.join(process.env.ProgramFiles || 'C:\\Program Files', 'PostgreSQL', version, 'bin', 'psql.exe');
      if (fs.existsSync(candidate)) return candidate;
    }
    return 'psql.exe';
  }
  return 'psql';
}

function loadEnv() {
  if (!fs.existsSync(envPath)) throw new Error('Missing .env.demo.local; there is no configured demo database to reset.');
  const values = {};
  for (const line of fs.readFileSync(envPath, 'utf8').split(/\r?\n/)) {
    const match = line.match(/^([A-Z0-9_]+)=(.*)$/);
    if (match) values[match[1]] = match[2];
  }
  if (!values.CLM_DEMO_SETUP_ID || !values.CLM_DEMO_DB_PASSWORD) {
    throw new Error('.env.demo.local is missing its demo ownership marker or database password.');
  }
  return values;
}

function runPsql(psql, user, database, args, input, password) {
  const env = { ...process.env };
  for (const key of ['PGHOST', 'PGPORT', 'PGUSER', 'PGDATABASE', 'PGSSLMODE', 'PGOPTIONS', 'PGPASSWORD']) delete env[key];
  if (password) env.PGPASSWORD = password;
  const result = spawnSync(psql, [
    '-X', '-w', '-h', '127.0.0.1', '-p', '5432', '-v', 'ON_ERROR_STOP=1', '-U', user, '-d', database, ...args
  ], { encoding: 'utf8', input, env, windowsHide: true, maxBuffer: 1024 * 1024 });
  if (result.error) throw new Error(`psql could not run: ${result.error.message}`);
  if (result.status !== 0) {
    throw new Error(`psql exited with code ${result.status}: ${(result.stderr || result.stdout || '').trim()}`);
  }
  return (result.stdout || '').trim();
}

async function main() {
  if (!process.argv.includes('--confirm')) {
    throw new Error('Reset is destructive. Run npm run demo:reset -- --confirm to remove only the marked clm_demo database.');
  }
  if (fs.existsSync(sessionPath)) {
    throw new Error('The managed demo is running; stop it with npm run demo:stop before resetting.');
  }

  const values = loadEnv();
  const psql = findPsql();
  const adminUser = process.env.CLM_DEMO_PG_ADMIN_USER || 'postgres';
  const adminPassword = process.env.CLM_DEMO_PG_ADMIN_PASSWORD || '';
  runPsql(psql, adminUser, 'postgres', ['-c', 'SELECT 1'], undefined, adminPassword);
  const exists = runPsql(psql, adminUser, 'postgres', [
    '-A', '-t', '-F', '|', '-c',
    "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='clm_demo'), EXISTS(SELECT 1 FROM pg_database WHERE datname='clm_demo')"
  ], undefined, adminPassword).split('|');
  if (exists[0] !== 't' || exists[1] !== 't') {
    throw new Error('Both clm_demo role and database must exist; refusing to remove a partial setup.');
  }

  const marker = runPsql(psql, 'clm_demo', 'clm_demo', [
    '-A', '-t', '-c', "SELECT value FROM clm_demo_metadata WHERE key='setup_id'"
  ], undefined, values.CLM_DEMO_DB_PASSWORD);
  if (marker !== values.CLM_DEMO_SETUP_ID) {
    throw new Error('Database ownership marker does not match .env.demo.local; refusing to reset it.');
  }

  runPsql(psql, adminUser, 'postgres', ['-c',
    "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='clm_demo' AND pid<>pg_backend_pid()"
  ], undefined, adminPassword);
  runPsql(psql, adminUser, 'postgres', ['-c', 'DROP DATABASE clm_demo'], undefined, adminPassword);
  runPsql(psql, adminUser, 'postgres', ['-c', 'DROP ROLE clm_demo'], undefined, adminPassword);
  console.log('The marked clm_demo database and role were removed. .env.demo.local was preserved.');
  console.log('Run npm run demo:setup to recreate the isolated demo database.');
}

main().catch(error => {
  console.error(`[FAIL] ${error.message}`);
  process.exitCode = 1;
});
