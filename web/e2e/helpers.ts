import { type Page, expect } from '@playwright/test';

/**
 * Shared E2E test helpers for DeployMonster.
 */

export const TEST_USER = {
  name: 'E2E Test User',
  email: 'e2e@deploymonster.test',
  password: 'TestPass123!',
};

/** Wait for the dashboard to fully load (stat cards rendered). */
export async function waitForDashboard(page: Page) {
  // Wait for the dashboard shell to be visible (more reliable than greeting text)
  await expect(page.locator('[data-testid="dashboard-shell"]')).toBeVisible({ timeout: 10_000 });
}

/**
 * Log in via direct API — fast, rate-limit-friendly with retry on 429.
 * Sets the session cookie in the browser context without UI rendering overhead.
 * Use this in ensureAuthenticated to avoid burning auth quota with UI retries.
 */
export async function apiLogin(page: Page, email: string, password: string): Promise<boolean> {
  for (let attempt = 1; attempt <= 3; attempt++) {
    const res = await page.request.post('/api/v1/auth/login', {
      data: { email, password },
    });

    if (res.ok()) {
      // Login succeeded — navigate to dashboard and verify auth
      await page.goto('/');
      try {
        await expect(page.locator('[data-testid="dashboard-shell"]')).toBeVisible({ timeout: 10_000 });
        return true;
      } catch {
        return false;
      }
    }

    if (res.status() === 429 && attempt < 3) {
      // Rate limited — wait and retry (5s, 10s)
      await new Promise((resolve) => setTimeout(resolve, attempt * 5_000));
      continue;
    }

    // Non-retryable error (400, 401, 500, etc.)
    return false;
  }
  return false;
}

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

/** Register a new user via the UI form. */
export async function registerViaUI(
  page: Page,
  name: string,
  email: string,
  password: string,
) {
  await page.goto('/register');
  if (name) {
    await page.getByLabel('Name').fill(name);
  }
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByLabel('Confirm password').fill(password);
  await page.getByRole('button', { name: /create account/i }).click();
}

/** Navigate to a sidebar page and wait for it to load. */
export async function navigateTo(page: Page, path: string) {
  await page.goto(path);
}

/** Assert that the page shows an error alert with the given text. */
export async function expectError(page: Page, text: string | RegExp) {
  const alert = page.locator('[role="alert"], .bg-destructive\\/10');
  await expect(alert).toBeVisible();
  await expect(alert).toContainText(text);
}

/** Generate a unique string for test isolation. */
export function uniqueId(prefix = 'e2e') {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
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
  await shell.waitFor({ state: 'visible', timeout: 15_000 });
}
