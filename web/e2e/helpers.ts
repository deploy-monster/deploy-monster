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

/** Log in via the UI form (for tests that start without stored auth). */
export async function loginViaUI(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: /sign in/i }).click();
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
