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
    command: 'go run ./cmd/wizard',
    cwd: '..', // wizard module root (go.mod lives here)
    port: PORT,
    reuseExistingServer: true,
    timeout: 30_000,
    env: {
      WIZARD_ADMIN_PASSWORD: 'e2e-test-password',
      WIZARD_OPERATOR_PASSWORD: 'e2e-operator',
      WIZARD_VIEWER_PASSWORD: 'e2e-viewer',
      WIZARD_PORT: String(PORT),
      WIZARD_E2E: '1', // allow prerequisites to pass so RBAC E2E can reach dashboard
    },
  },
});
