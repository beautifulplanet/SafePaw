import { defineConfig } from '@playwright/test';

const PORT = Number(process.env.WIZARD_PORT ?? 3099);

export default defineConfig({
  testDir: './e2e',
  timeout: 15_000,
  retries: 0,
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    headless: true,
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
  webServer: {
    command: `WIZARD_ADMIN_PASSWORD=e2e-test-password WIZARD_PORT=${PORT} go run ../../cmd/wizard`,
    port: PORT,
    reuseExistingServer: true,
    timeout: 30_000,
  },
});
