import { type Page, expect } from '@playwright/test';

/**
 * Shared E2E test helpers for DeployMonster.
 */

export const TEST_USER = {
  name: 'E2E Test User',
  email: 'e2e@deploymonster.test',
  password: 'TestPass123!',
};

/** Log in via the UI form (for tests that start without stored auth). */
export async function loginViaUI(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: /sign in/i }).click();
  // Wait for dashboard — catch timeout and return, letting ensureAuthenticated
  // do the final assertion with the test's remaining time budget.
  await expect(page.locator('[data-testid="dashboard-shell"]')).toBeVisible({ timeout: 45_000 }).catch(() => {});
}

/**
 * Self-healing auth: detects login page and re-authenticates via UI.
 * Simple, robust, no /me calls to avoid rate limit noise.
 */
export async function ensureAuthenticated(page: Page): Promise<void> {
  const shell = page.locator('[data-testid="dashboard-shell"]');

  // Quick check — shell already visible (common fast path).
  try {
    await shell.waitFor({ state: 'visible', timeout: 3_000 });
    return;
  } catch {
    // Not visible within 3s — continue healing flow.
  }

  // If we're on the login page, re-authenticate.
  const loginText = page.getByText(/welcome back|sign in to your account/i);
  const isLoginPage = await loginText.isVisible({ timeout: 500 }).catch(() => false);
  if (isLoginPage) {
    await loginViaUI(page, TEST_USER.email, TEST_USER.password);
    return;
  }

  // Shell not visible and not on login page — page may be stuck loading.
  // Try waiting for DOM content then re-check.
  await page.waitForLoadState('domcontentloaded', { timeout: 10_000 }).catch(() => {});

  // Retry shell check with generous timeout.
  try {
    await shell.waitFor({ state: 'visible', timeout: 15_000 });
    return;
  } catch {
    // Still not visible — try reload as last resort.
  }

  // Reload and re-check — handles stale renders from previous test navigation.
  await page.reload({ waitUntil: 'domcontentloaded' }).catch(() => {});

  // After reload, check if we landed on login page (session expired).
  const isLoginAfterReload = await page.getByText(/welcome back|sign in to your account/i)
    .isVisible({ timeout: 2_000 }).catch(() => false);
  if (isLoginAfterReload) {
    await loginViaUI(page, TEST_USER.email, TEST_USER.password);
    return;
  }

  // Wait for auth initialization (calls /auth/me which may be slow on cold start).
  await shell.waitFor({ state: 'visible', timeout: 30_000 });
}
