import { spawn, execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Client } from 'pg';

const root = fileURLToPath(new URL('../../', import.meta.url));
const staticDir = fileURLToPath(new URL('../build', import.meta.url));
const dbUrl = new URL(process.env.TEST_DATABASE_URL ?? 'postgres://postgres:dev@127.0.0.1:5433/oib?sslmode=disable');
const admin = new Client({ connectionString: dbUrl.href });
const database = 'oib_e2e_' + randomUUID().replaceAll('-', '');
const tempDir = await mkdtemp(join(tmpdir(), 'oib-e2e-'));
let databaseCreated = false;

try {
  const executable = join(tempDir, 'server');
  execFileSync('go', ['build', '-o', executable, './cmd/server'], { cwd: root, stdio: 'inherit' });
  await admin.connect();
  await admin.query('CREATE DATABASE ' + database);
  databaseCreated = true;

  dbUrl.pathname = '/' + database;
  dbUrl.searchParams.delete('database');
  dbUrl.searchParams.delete('dbname');
  const server = spawn(executable, {
    cwd: root,
    env: {
      ...process.env,
      DATABASE_URL: dbUrl.href,
      FAULT_INJECTION: process.env.E2E_DEMO === '1' ? '1' : undefined,
      STATIC_DIR: staticDir
    },
    stdio: 'inherit'
  });
  function stop() { server.kill('SIGTERM'); }
  process.once('SIGTERM', stop);
  process.once('SIGINT', stop);
  try {
    await once(server, 'close');
    if (server.exitCode !== 0 && server.signalCode !== 'SIGTERM' && server.signalCode !== 'SIGINT') {
      throw new Error('Playwright용 Go 서버가 중단되었습니다.');
    }
  } finally {
    process.off('SIGTERM', stop);
    process.off('SIGINT', stop);
  }
} finally {
  try {
    if (databaseCreated) await admin.query('DROP DATABASE ' + database + ' WITH (FORCE)');
  } finally {
    await admin.end();
    await rm(tempDir, { recursive: true, force: true });
  }
}
