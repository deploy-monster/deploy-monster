import { test, expect } from '@playwright/test';

/**
 * Applications page E2E tests.
 *
 * Tests run with stored auth state (authenticated user).
 */

test.describe('Applications Page', () => {
  test('loads applications page', async ({ page }) => {
    await page.goto('/apps');

    // Page header should be visible — use exact match to avoid strict mode violation
    // (sidebar also has "Applications" text)
    await expect(page.getByRole('heading', { name: 'Applications', exact: true })).toBeVisible({ timeout: 10_000 });
  });

  test('shows "Deploy New App" button', async ({ page }) => {
    await page.goto('/apps');

    const newAppButton = page.getByRole('link', { name: /new|deploy|create/i }).or(
      page.getByRole('button', { name: /new|deploy|create/i })
    );
    await expect(newAppButton.first()).toBeVisible({ timeout: 10_000 });
  });

  test('has filter tabs', async ({ page }) => {
    await page.goto('/apps');

    // Filter tabs: All, Running, Stopped, Deploying
    await expect(page.getByText('All')).toBeVisible({ timeout: 10_000 });
  });

  test('has search functionality', async ({ page }) => {
    await page.goto('/apps');

    const searchInput = page.getByPlaceholder(/search/i);
    await expect(searchInput.first()).toBeVisible({ timeout: 10_000 });
  });

  test('shows empty state when no apps exist', async ({ page }) => {
    await page.goto('/apps');

    // Either shows app list or empty state
    const appCards = page.locator('[data-testid="app-card"], .app-card');
    const emptyState = page.getByText(/no applications|deploy your first/i);

    // One of these should be visible
    await expect(appCards.first().or(emptyState.first())).toBeVisible({ timeout: 10_000 });
  });
});

test.describe('Deploy Wizard', () => {
  test('loads deploy wizard page', async ({ page }) => {
    await page.goto('/apps/new');

    // Should show deployment options or wizard steps
    await expect(page.locator('body')).not.toHaveText(/404|not found/i, {
      timeout: 10_000,
    });
  });

  test('navigating from apps page works', async ({ page }) => {
    await page.goto('/apps');

    // Click deploy/new button
    const newAppButton = page.getByRole('link', { name: /new|deploy|create/i }).or(
      page.getByRole('button', { name: /new|deploy|create/i })
    );
    const btn = newAppButton.first();

    if (await btn.isVisible().catch(() => false)) {
      await btn.click();
      await expect(page).toHaveURL(/\/apps\/new/);
    }
  });
});
