import { test as setup, expect } from '@playwright/test';

/**
 * Global setup: registers a test user (or logs in if already registered),
 * verifies the resulting cookies actually authenticate, and saves the
 * browser state to e2e/.auth/user.json.
 *
 * All other test projects depend on this via `dependencies: ['setup']`
 * and reuse the stored auth state via `storageState`.
 *
 * The setup is deliberately loud: if auth fails at *any* step, it throws
 * with enough context (HTTP status, cookies, /me response) to diagnose
 * the failure without having to rerun with more instrumentation. The
 * prior version used a negative-lookahead `waitForURL` that silently
 * passed when the page hung on Suspense, which hid broken-auth bugs
 * behind 77 mysteriously-unauthenticated test failures downstream.
 */

const TEST_USER = {
  name: 'E2E Test User',
  email: 'e2e@deploymonster.test',
  password: 'TestPass123!',
};

setup('authenticate', async ({ page }) => {
  const registerRes = await page.request.post('/api/v1/auth/register', {
    data: {
      name: TEST_USER.name,
      email: TEST_USER.email,
      password: TEST_USER.password,
    },
  });

  if (!registerRes.ok()) {
    // User likely exists from a prior run — log in instead.
    const loginRes = await page.request.post('/api/v1/auth/login', {
      data: {
        email: TEST_USER.email,
        password: TEST_USER.password,
      },
    });
    if (!loginRes.ok()) {
      const body = await loginRes.text().catch(() => '<unreadable>');
      throw new Error(
        `e2e setup: register returned ${registerRes.status()} AND login returned ${loginRes.status()}; login body=${body}`,
      );
    }
  }

  // Sanity check: cookies must actually authenticate against /me.
  // If this fails the cookie pipeline is broken (Secure flag, path,
  // HttpOnly handling, etc.) and every downstream test would fail the
  // same way — fail LOUDLY here with cookie dump so the cause is
  // visible in the CI log instead of propagating as 77 timeouts.
  const meRes = await page.request.get('/api/v1/auth/me');
  if (!meRes.ok()) {
    const cookies = await page.context().cookies();
    const body = await meRes.text().catch(() => '<unreadable>');
    throw new Error(
      `e2e setup: /api/v1/auth/me returned ${meRes.status()} after auth; ` +
        `cookies=${JSON.stringify(cookies)}; body=${body}`,
    );
  }

  // Visit the SPA so the browser context matches what tests will see.
  // networkidle fires when all chunks have loaded, then we wait for the
  // greeting text to confirm auth initialization completed.
  await page.goto('/');
  await page.waitForLoadState('networkidle', { timeout: 15_000 });

  // Confirm the dashboard shell rendered with greeting visible
  await expect(page.getByText(/good (morning|afternoon|evening)/i)).toBeVisible({
    timeout: 10_000,
  });

  // THEN check URL (confirm we're not on login/register)
  await expect(page).toHaveURL(/^(?!.*\/login)(?!.*\/register).+/);

  await page.context().storageState({ path: './e2e/.auth/user.json' });
});
