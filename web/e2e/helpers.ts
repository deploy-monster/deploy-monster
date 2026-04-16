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

  // With retry: wait for redirect up to 5s first attempt, then back off and retry.
  // On rate limit (429), the UI may not redirect — retry with longer backoff.
  for (let attempt = 1; attempt <= 3; attempt++) {
    try {
      await page.waitForURL((url) => !url.pathname.startsWith('/login'), { timeout: 5_000 });
      return; // Success
    } catch {
      if (attempt < 3) {
        // Login didn't redirect — back off and retry.
        // Rate limit waits: 5s, 10s (longer to let rate limit window clear).
        await new Promise((resolve) => setTimeout(resolve, attempt * 5_000));
        // Retry: reload login page and try again.
        await page.goto('/login');
        await page.getByLabel('Email').fill(email);
        await page.getByLabel('Password').fill(password);
        await page.getByRole('button', { name: /sign in/i }).click();
      }
    }
  }
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
 * Self-healing auth: verifies stored session is still valid via /me endpoint,
 * and re-authenticates if needed. More reliable than waiting for dashboard-shell
 * which may not render if auth is in a loading state.
 */
export async function ensureAuthenticated(page: Page): Promise<void> {
  // First check: is dashboard already visible? (fast path for valid sessions)
  try {
    await expect(page.locator('[data-testid="dashboard-shell"]')).toBeVisible({ timeout: 3_000 });
    return;
  } catch {
    // Dashboard not visible
  }

  // Second check: are we at the login page?
  const loginText = page.getByText(/welcome back|sign in to your account/i);
  const isLoginPage = await loginText.isVisible({ timeout: 500 }).catch(() => false);

  if (isLoginPage) {
    // Check if stored session is still valid by calling /me
    const meRes = await page.request.get('/api/v1/auth/me');
    if (!meRes.ok()) {
      // Session invalid or expired — re-authenticate via UI (sets browser cookies)
      await loginViaUI(page, TEST_USER.email, TEST_USER.password);
    }
    // If /me was ok, the session is valid — maybe a timing issue, reload dashboard
    await page.goto('/');
  }

  // Final wait for dashboard shell
  await expect(page.locator('[data-testid="dashboard-shell"]')).toBeVisible({ timeout: 30_000 });
}
