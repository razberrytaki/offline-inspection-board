import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'node:url';

const baseURL = 'http://127.0.0.1:' + (process.env.E2E_PORT ?? '18091');

export default defineConfig({
  testDir: '.',
  // 테스트 서버의 장애 설정과 보드 순서는 모든 브라우저 컨텍스트에서 공유된다.
  workers: 1,
  outputDir: '../test-results',
  reporter: 'list',
  use: { ...devices['Desktop Chrome'], baseURL },
  webServer: {
    command: 'node tests/server.ts',
    cwd: fileURLToPath(new URL('../', import.meta.url)),
    url: baseURL + '/',
    timeout: 60_000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 5000 }
  }
});
