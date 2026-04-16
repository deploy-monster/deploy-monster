import { test, expect } from '@playwright/test';
import { ensureAuthenticated } from './helpers';

/**
 * Applications page E2E tests.
 *
 * Tests run with stored auth state (authenticated user).
 */

test.describe('Applications Page', () => {
  test('loads applications page', async ({ page }) => {
    await page.goto('/apps');
    await ensureAuthenticated(page);

    // Page header should be visible — use exact match to avoid strict mode violation
    // (sidebar also has "Applications" text)
    await expect(page.getByRole('heading', { name: 'Applications', exact: true })).toBeVisible({ timeout: 10_000 });
  });

  test('shows "Deploy New App" button', async ({ page }) => {
    await page.goto('/apps');
    await ensureAuthenticated(page);

    const newAppButton = page.getByRole('link', { name: /new|deploy|create/i }).or(
      page.getByRole('button', { name: /new|deploy|create/i })
    );
    await expect(newAppButton.first()).toBeVisible({ timeout: 10_000 });
  });

  test('has filter tabs', async ({ page }) => {
    await page.goto('/apps');
    await ensureAuthenticated(page);

    // Filter tabs: All, Running, Stopped, Deploying
    await expect(page.getByText('All')).toBeVisible({ timeout: 10_000 });
  });

  test('has search functionality', async ({ page }) => {
    await page.goto('/apps');
    await ensureAuthenticated(page);

    const searchInput = page.getByPlaceholder(/search/i);
    await expect(searchInput.first()).toBeVisible({ timeout: 10_000 });
  });

  test('shows empty state when no apps exist', async ({ page }) => {
    await page.goto('/apps');
    await ensureAuthenticated(page);

    // The empty state shows "No applications yet" heading + "Deploy Your First App" button.
    // Also accepts the stats bar showing "0 applications deployed".
    const emptyHeading = page.getByText('No applications yet');
    const deployBtn = page.getByText(/deploy your first app/i);
    await expect(emptyHeading.or(deployBtn).first()).toBeVisible({ timeout: 10_000 });
  });
});
