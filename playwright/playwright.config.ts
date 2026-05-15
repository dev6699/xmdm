import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.XMDM_DASHBOARD_URL ?? 'http://127.0.0.1:39092';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  reporter: process.env.CI ? 'dot' : 'list',
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  ...(process.env.XMDM_DASHBOARD_URL
    ? {}
    : {
        webServer: {
          command: 'bash ./playwright/scripts/start-real-server.sh',
          cwd: '..',
          url: `${baseURL}/admin/login`,
          reuseExistingServer: !process.env.CI,
          timeout: 180_000,
        },
      }),
});
