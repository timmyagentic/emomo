import { defineConfig, devices } from '@playwright/test';

const requestedPort = Number(process.env.E2E_PORT);
const e2ePort = Number.isInteger(requestedPort) && requestedPort > 0 && requestedPort < 65_536
  ? String(requestedPort)
  : '5173';
const e2eBaseUrl = `http://localhost:${e2ePort}`;

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: e2eBaseUrl,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: `npm run dev -- --port ${e2ePort}`,
    url: e2eBaseUrl,
    reuseExistingServer: !process.env.CI,
  },
});
