// Initializes only the isolated clm_demo database and builds the Go API.
const { spawnSync } = require('child_process');
const crypto = require('crypto');
const fs = require('fs');
const os = require('os');
const path = require('path');

const root = path.resolve(__dirname, '..');
const backend = path.join(root, 'components', 'saas-control-plane');
const migrationsDir = path.join(backend, 'deploy', 'migrations');
const envPath = path.join(root, '.env.demo.local');
const demoConfig = path.join(backend, 'configs', 'server-demo.yaml');
const psqlPath = findPsql();
const goPath = findGo();

function findPsql() {
  if (process.env.PSQL_PATH && fs.existsSync(process.env.PSQL_PATH)) {
    return process.env.PSQL_PATH;
  }
  if (process.platform === 'win32') {
    for (const version of ['18', '17', '16', '15', '14']) {
      const candidate = path.join(
        process.env.ProgramFiles || 'C:\\Program Files',
        'PostgreSQL',
        version,
        'bin',
        'psql.exe'
      );
      if (fs.existsSync(candidate)) return candidate;
    }
    return 'psql.exe';
  }
  return 'psql';
}

function findGo() {
  if (process.env.CLM_GO_PATH) return process.env.CLM_GO_PATH;
  if (process.platform === 'win32') {
    const candidate = path.join(process.env.ProgramFiles || 'C:\\Program Files', 'Go', 'bin', 'go.exe');
    if (fs.existsSync(candidate)) return candidate;
  }
  return 'go';
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd || root,
    env: options.env || process.env,
    encoding: 'utf8',
    input: options.input,
    maxBuffer: 10 * 1024 * 1024,
    windowsHide: true
  });
  if (result.error) {
    throw new Error(`${command} could not run: ${result.error.message}`);
  }
  if (result.status !== 0) {
    const details = (result.stderr || result.stdout || '').trim();
    throw new Error(`${command} exited with code ${result.status}${details ? `:\n${details}` : ''}`);
  }
  return (result.stdout || '').trim();
}

function psqlArgs(user, database, extra = []) {
  return ['-X', '-w', '-h', '127.0.0.1', '-p', '5432', '-v', 'ON_ERROR_STOP=1', '-U', user, '-d', database, ...extra];
}

function psqlEnv(password) {
  const env = { ...process.env };
  for (const key of ['PGHOST', 'PGPORT', 'PGUSER', 'PGDATABASE', 'PGSSLMODE', 'PGOPTIONS', 'PGPASSWORD']) {
    delete env[key];
  }
  if (password) env.PGPASSWORD = password;
  return env;
}

function runPsql(user, database, extra, options = {}) {
  return run(psqlPath, psqlArgs(user, database, extra), {
    cwd: options.cwd,
    env: options.env || psqlEnv(options.password),
    input: options.input
  });
}

function readDemoEnv() {
  const values = {};
  for (const line of fs.readFileSync(envPath, 'utf8').split(/\r?\n/)) {
    const match = line.match(/^([A-Z0-9_]+)=(.*)$/);
    if (match) values[match[1]] = match[2];
  }
  const required = [
    'CLM_DEMO_DB_PASSWORD',
    'CLM_DEMO_JWT_SECRET',
    'CLM_DEMO_CLOUD_MASTER_KEY',
    'CLM_DEMO_SETUP_ID',
    'CLM_DEMO_ADMIN_EMAIL',
    'CLM_DEMO_ADMIN_PASSWORD',
    'CLM_DEMO_ORGANIZATION'
  ];
  for (const key of required) {
    if (!values[key]) throw new Error(`Missing ${key} in .env.demo.local`);
  }
  if (!/^[0-9a-f]{32}$/.test(values.CLM_DEMO_SETUP_ID.replace(/-/g, ''))) {
    throw new Error('Invalid CLM_DEMO_SETUP_ID in .env.demo.local');
  }
  return values;
}

function createDemoEnv() {
  const values = {
    CLM_DEMO_DB_PASSWORD: crypto.randomBytes(32).toString('hex'),
    CLM_DEMO_JWT_SECRET: crypto.randomBytes(32).toString('hex'),
    CLM_DEMO_CLOUD_MASTER_KEY: crypto.randomBytes(32).toString('base64url').slice(0, 32),
    CLM_DEMO_SETUP_ID: crypto.randomUUID(),
    CLM_DEMO_ADMIN_EMAIL: 'admin@enterprise.internal',
    CLM_DEMO_ADMIN_PASSWORD: 'Demo@2024!',
    CLM_DEMO_ORGANIZATION: 'CLM Demonstration Organization'
  };
  const contents = Object.entries(values).map(([key, value]) => `${key}=${value}`).join('\n') + '\n';
  fs.writeFileSync(envPath, contents, { flag: 'wx', mode: 0o600 });
  return values;
}

function checkPrerequisites() {
  const nodeMajor = Number(process.versions.node.split('.')[0]);
  if (nodeMajor < 18) throw new Error(`Node.js 18+ required; found ${process.version}`);
  if (process.env.PSQL_PATH && !fs.existsSync(process.env.PSQL_PATH)) {
    throw new Error(`PSQL_PATH does not exist: ${process.env.PSQL_PATH}`);
  }
  if (process.env.CLM_GO_PATH && !fs.existsSync(process.env.CLM_GO_PATH)) {
    throw new Error(`CLM_GO_PATH does not exist: ${process.env.CLM_GO_PATH}`);
  }
  if (!fs.existsSync(demoConfig)) throw new Error(`Missing demo config: ${demoConfig}`);
  if (!fs.existsSync(path.join(backend, 'go.mod'))) throw new Error(`Missing Go module: ${backend}`);
  const version = run(goPath, ['version']);
  console.log(`[OK] ${version}`);
  console.log(`[OK] PostgreSQL client: ${psqlPath}`);
}

function main() {
  console.log('CLM database-backed demo setup');
  checkPrerequisites();

  const adminUser = process.env.CLM_DEMO_PG_ADMIN_USER || 'postgres';
  const adminPassword = process.env.CLM_DEMO_PG_ADMIN_PASSWORD || '';
  const adminEnv = psqlEnv(adminPassword);
  runPsql(adminUser, 'postgres', ['-c', 'SELECT 1'], { env: adminEnv });

  const state = runPsql(adminUser, 'postgres', [
    '-A', '-t', '-F', '|',
    '-c',
    "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='clm_demo'), EXISTS(SELECT 1 FROM pg_database WHERE datname='clm_demo')"
  ], { env: adminEnv }).split('|');
  const roleExists = state[0] === 't';
  const databaseExists = state[1] === 't';
  const envExists = fs.existsSync(envPath);

  if (!envExists && (roleExists || databaseExists)) {
    throw new Error('clm_demo role/database already exists without .env.demo.local; refusing to modify or reuse it.');
  }
  if (roleExists !== databaseExists) {
    throw new Error('Only one of the clm_demo role/database exists; refusing partial setup. Inspect it manually before continuing.');
  }

  const values = envExists ? readDemoEnv() : createDemoEnv();
  if (!roleExists) {
    const password = values.CLM_DEMO_DB_PASSWORD.replace(/'/g, "''");
    runPsql(adminUser, 'postgres', [], {
      env: adminEnv,
      input: `CREATE ROLE clm_demo LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '${password}';\n`
    });
    runPsql(adminUser, 'postgres', ['-c', 'CREATE DATABASE clm_demo OWNER clm_demo'], { env: adminEnv });
    const setupID = values.CLM_DEMO_SETUP_ID.replace(/'/g, "''");
    const markerSQL = `BEGIN;
      CREATE TABLE clm_demo_metadata (key text PRIMARY KEY, value text NOT NULL);
      INSERT INTO clm_demo_metadata (key, value) VALUES ('setup_id', '${setupID}');
      CREATE TABLE clm_demo_schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
      COMMIT;`;
    runPsql('clm_demo', 'clm_demo', [], {
      password: values.CLM_DEMO_DB_PASSWORD,
      input: markerSQL
    });
  } else {
    const setupID = runPsql('clm_demo', 'clm_demo', [
      '-A', '-t', '-c', "SELECT value FROM clm_demo_metadata WHERE key='setup_id'"
    ], { password: values.CLM_DEMO_DB_PASSWORD });
    if (setupID !== values.CLM_DEMO_SETUP_ID) {
      throw new Error('clm_demo metadata does not match .env.demo.local; refusing to reuse this database.');
    }
  }
  console.log('[OK] Isolated clm_demo database is ready');

  const tableOwnerState = runPsql(adminUser, 'postgres', [
    '-A', '-t', '-F', '|', '-c',
    "SELECT rolsuper, rolbypassrls, rolcanlogin FROM pg_roles WHERE rolname='clm_table_owner'"
  ], { env: adminEnv });
  if (!tableOwnerState) {
    runPsql(adminUser, 'postgres', [], {
      env: adminEnv,
      input: 'CREATE ROLE clm_table_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;\n'
    });
  } else if (tableOwnerState !== 'f|f|f') {
    throw new Error('Existing clm_table_owner role is not a no-login, non-superuser, non-BYPASSRLS role; refusing to change it.');
  }

  const applied = runPsql('clm_demo', 'clm_demo', [
    '-A', '-t', '-c', 'SELECT version FROM clm_demo_schema_migrations ORDER BY version'
  ], { password: values.CLM_DEMO_DB_PASSWORD }).split(/\r?\n/).filter(Boolean);
  const migrations = fs.readdirSync(migrationsDir)
    .filter(name => /^00[1-8]_.*\.sql$/.test(name))
    .sort();
  if (migrations.length !== 8) {
    throw new Error(`Expected eight ordered CLM migrations, found ${migrations.length}`);
  }
  for (const migration of migrations) {
    if (applied.includes(migration)) {
      console.log(`[SKIP] ${migration} already applied`);
      continue;
    }
    const migrationSQL = fs.readFileSync(path.join(migrationsDir, migration), 'utf8');
    const version = migration.replace(/'/g, "''");
    runPsql('clm_demo', 'clm_demo', [], {
      password: values.CLM_DEMO_DB_PASSWORD,
      input: `BEGIN;\n${migrationSQL}\nINSERT INTO clm_demo_schema_migrations (version) VALUES ('${version}');\nCOMMIT;\n`
    });
    console.log(`[OK] Applied ${migration}`);
  }

  const binDir = path.join(backend, 'bin');
  fs.mkdirSync(binDir, { recursive: true });
  const testEnv = { ...process.env };
  testEnv.CLM_TEST_DB_DSN = `host=127.0.0.1 port=5432 user=clm_demo password=${values.CLM_DEMO_DB_PASSWORD} dbname=clm_demo sslmode=disable`;
  testEnv.CLM_TEST_DB_REQUIRED = '1';
  const stageDir = fs.mkdtempSync(path.join(os.tmpdir(), 'clm-demo-go-'));
  try {
    fs.cpSync(backend, stageDir, { recursive: true });
    run(goPath, ['mod', 'edit', '-module=github.com/m2p/clm'], { cwd: stageDir });
    run(goPath, ['mod', 'tidy'], { cwd: stageDir });
    run(goPath, ['test', './internal/saas/contract', './internal/store/postgres', './pkg/config', './internal/api/rest/handlers'], {
      cwd: stageDir,
      env: testEnv
    });
    const binaryName = process.platform === 'win32' ? 'clm-server.exe' : 'clm-server';
    run(goPath, ['build', '-o', path.join(binDir, binaryName), './cmd/clm-server'], { cwd: stageDir });
  } finally {
    fs.rmSync(stageDir, { recursive: true, force: true });
  }
  console.log('[OK] Go tests passed and the control plane built');
  console.log('[OK] Generated local configuration: .env.demo.local');
  console.log('Next: npm run demo:start');
}

try {
  main();
} catch (error) {
  console.error(`[FAIL] ${error.message}`);
  process.exitCode = 1;
}
