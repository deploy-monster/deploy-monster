import { test as setup, expect } from '@playwright/test';

/**
 * Global setup: registers a test user (or logs in if already registered)
 * and saves the authenticated browser state to e2e/.auth/user.json.
 *
 * All other test projects depend on this via `dependencies: ['setup']`
 * and reuse the stored auth state via `storageState`.
 */

const TEST_USER = {
  name: 'E2E Test User',
  email: 'e2e@deploymonster.test',
  password: 'TestPass123!',
};

setup('authenticate', async ({ page }) => {
  // Try to register first (may fail if user already exists)
  const registerRes = await page.request.post('/api/v1/auth/register', {
    data: {
      name: TEST_USER.name,
      email: TEST_USER.email,
      password: TEST_USER.password,
    },
  });

  if (registerRes.ok()) {
    // Registration succeeded — we're authenticated via cookies
    await page.goto('/');
    await page.waitForURL(/^(?!.*\/login)(?!.*\/register)/);
  } else {
    // User likely already exists — log in
    const loginRes = await page.request.post('/api/v1/auth/login', {
      data: {
        email: TEST_USER.email,
        password: TEST_USER.password,
      },
    });
    expect(loginRes.ok()).toBeTruthy();

    // Visit app to let cookies + state settle
    await page.goto('/');
    await page.waitForURL(/^(?!.*\/login)(?!.*\/register)/);
  }

  // Save authenticated state
  await page.context().storageState({ path: './e2e/.auth/user.json' });
});
