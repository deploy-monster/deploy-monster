import { type Page, expect } from '@playwright/test';

/**
 * Shared E2E test helpers for DeployMonster.
 */

export const TEST_USER = {
  name: 'E2E Test User',
  email: 'e2e@deploymonster.test',
  password: 'TestPass123!',
};

const dashboardShell = '[data-testid="dashboard-shell"]';
const loginPageText = /welcome back|sign in to your account/i;

async function waitForShellOrLogin(page: Page, timeout: number): Promise<'shell' | 'login' | 'timeout'> {
  const shell = page.locator(dashboardShell);
  const login = page.getByText(loginPageText).first();

  return Promise.race([
    shell.waitFor({ state: 'visible', timeout }).then(() => 'shell' as const),
    login.waitFor({ state: 'visible', timeout }).then(() => 'login' as const),
  ]).catch(() => 'timeout' as const);
}

/** Log in via the UI form (for tests that start without stored auth). */
export async function loginViaUI(page: Page, email: string, password: string) {
  const shell = page.locator(dashboardShell);

  for (let attempt = 1; attempt <= 2; attempt++) {
    await page.goto('/login');
    await page.getByLabel('Email').fill(email);
    await page.getByLabel('Password').fill(password);
    await page.getByRole('button', { name: /sign in/i }).click();

    const result = await waitForShellOrLogin(page, 10_000);
    if (result === 'shell') return;

    await page.waitForTimeout(1_000 * attempt);
  }

  await expect(shell).toBeVisible({ timeout: 15_000 });
}

/**
 * Self-healing auth: detects login page and re-authenticates via UI.
 * Simple, robust, no /me calls to avoid rate limit noise.
 */
export async function ensureAuthenticated(page: Page): Promise<void> {
  const shell = page.locator(dashboardShell);

  for (let attempt = 1; attempt <= 3; attempt++) {
    const result = await waitForShellOrLogin(page, attempt === 1 ? 3_000 : 10_000);
    if (result === 'shell') return;
    if (result === 'login') {
      await loginViaUI(page, TEST_USER.email, TEST_USER.password);
      return;
    }

    await page.waitForLoadState('domcontentloaded', { timeout: 5_000 }).catch(() => {});
    await page.reload({ waitUntil: 'domcontentloaded' }).catch(() => {});
  }

  // Wait for auth initialization (calls /auth/me which may be slow on cold start).
  await shell.waitFor({ state: 'visible', timeout: 30_000 });
}
