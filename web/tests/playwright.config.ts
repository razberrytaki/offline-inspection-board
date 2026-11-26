import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import { demoURL, normalURL } from './ports';

export default defineConfig({
  testDir: '.',
  // 테스트 서버의 장애 설정과 보드 순서는 모든 브라우저 컨텍스트에서 공유된다.
  workers: 1,
  outputDir: '../test-results',
  reporter: 'list',
  use: { ...devices['Desktop Chrome'], baseURL: demoURL },
  webServer: [demoURL, normalURL].map((url) => ({
    command: 'node tests/server.ts',
    cwd: fileURLToPath(new URL('../', import.meta.url)),
    url: url + '/',
    env: { ADDR: new URL(url).host, E2E_DEMO: url === demoURL ? '1' : '0' },
    timeout: 60_000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 5000 }
  }))
});
