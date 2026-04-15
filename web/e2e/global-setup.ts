import { test as setup, expect } from '@playwright/test';

/**
 * Global setup: registers a test user (or logs in if already registered),
 * verifies the resulting cookies actually authenticate, and saves the
 * browser state to e2e/.auth/user.json.
 *
 * All other test projects depend on this via `dependencies: ['setup']`
 * and reuse the stored auth state via `storageState`.
 *
 * We use UI-based login (not page.request API calls) because Playwright's
 * page.request is a separate API client — cookies from page.request responses
 * don't automatically propagate to the browser page's cookie jar.
 */

const TEST_USER = {
  name: 'E2E Test User',
  email: 'e2e@deploymonster.test',
  password: 'TestPass123!',
};

setup('authenticate', async ({ page }) => {
  // Try to register first. If user already exists (409), fall back to login.
  const registerRes = await page.request.post('/api/v1/auth/register', {
    data: {
      name: TEST_USER.name,
      email: TEST_USER.email,
      password: TEST_USER.password,
    },
  });

  // Always log in via the UI — this guarantees cookies land in the browser context.
  await page.goto('/login');
  await page.getByLabel('Email').fill(TEST_USER.email);
  await page.getByLabel('Password').fill(TEST_USER.password);
  await page.getByRole('button', { name: /sign in/i }).click();

  // Wait for auth initialization to complete and dashboard to render.
  await expect(page.getByText(/good (morning|afternoon|evening)/i)).toBeVisible({
    timeout: 15_000,
  });

  // Confirm we're not stuck on login page.
  await expect(page).toHaveURL(/^(?!.*\/login)(?!.*\/register).+/);

  // Sanity check: cookies must actually authenticate against /me.
  // Use retry loop to handle rate limiting (100 req/min per tenant).
  let meRes: Awaited<ReturnType<typeof page.request.get>> | null = null;
  for (let attempt = 1; attempt <= 3; attempt++) {
    meRes = await page.request.get('/api/v1/auth/me');
    if (meRes.ok()) break;
    if (meRes.status() === 429) {
      // Rate limited — wait 2 seconds and retry
      await page.waitForTimeout(2_000);
      continue;
    }
    break;
  }
  if (!meRes?.ok()) {
    const cookies = await page.context().cookies();
    const body = await meRes?.text().catch(() => '<unreadable>');
    throw new Error(
      `e2e setup: /api/v1/auth/me returned ${meRes?.status()} after login; ` +
        `cookies=${JSON.stringify(cookies)}; body=${body}`,
    );
  }

  await page.context().storageState({ path: './e2e/.auth/user.json' });
});
