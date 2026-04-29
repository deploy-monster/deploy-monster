import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright E2E test configuration for DeployMonster.
 *
 * Tests run against the full stack: Go backend (port 8443) + embedded React UI.
 * Start the server with `make dev` before running tests.
 *
 * Usage:
 *   pnpm test:e2e              # Run all e2e tests (headless)
 *   pnpm test:e2e --ui         # Run with Playwright UI
 *   pnpm test:e2e --headed     # Run with visible browser
 */
export default defineConfig({
  testDir: './e2e',
  outputDir: './e2e-results',

  /* Retry flaky tests — 1 retry in CI, 1 locally */
  retries: 1,

  /* Parallel workers */
  workers: process.env.CI ? 1 : undefined,

  /* Reporter */
  reporter: process.env.CI
    ? [['html', { open: 'never', outputFolder: 'e2e-report' }]]
    : [['list']],

  /* Shared settings for all projects */
  use: {
    baseURL: process.env.E2E_BASE_URL || 'http://localhost:8443',

    /* Collect traces on first retry */
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',

    /* Sensible timeouts */
    actionTimeout: 10_000,
    navigationTimeout: 15_000,
  },

  /* Global timeout per test — 60s to accommodate auth retry in slow suites */
  timeout: 60_000,

  /* Expect timeout */
  expect: {
    timeout: 5_000,
  },

  projects: [
    /* Setup project: creates test user + authenticates */
    {
      name: 'setup',
      testMatch: /global-setup\.ts/,
    },

    /* Login — runs auth.spec.ts in its own browser context to avoid
       exhausting the auth rate limit quota shared with other tests.
       No setup dependency since it uses empty storageState (fresh auth each test). */
    {
      name: 'login',
      use: {
        ...devices['Desktop Chrome'],
        storageState: { cookies: [], origins: [] },
      },
      testMatch: /.*auth\.spec\.ts/,
    },

    /* Chromium — main test browser (excludes auth.spec.ts, handled by 'login') */
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        storageState: './e2e/.auth/user.json',
      },
      testIgnore: /.*auth\.spec\.ts/,
      dependencies: ['setup'],
    },
  ],
});
