import { test, expect } from '@playwright/test';

/**
 * Dashboard E2E tests.
 *
 * Tests run with stored auth state (authenticated user).
 */

test.describe('Dashboard', () => {
  test('loads with welcome greeting', async ({ page }) => {
    await page.goto('/');

    // Welcome banner with time-based greeting
    await expect(page.getByText(/good (morning|afternoon|evening)/i)).toBeVisible({
      timeout: 10_000,
    });
  });

  test('displays stat cards', async ({ page }) => {
    await page.goto('/');

    // Wait for stat cards to load (should have 5 cards)
    await expect(page.getByText('Applications')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('Running')).toBeVisible();
    await expect(page.getByText('Containers')).toBeVisible();
    await expect(page.getByText('Domains')).toBeVisible();
    await expect(page.getByText('Projects')).toBeVisible();
  });

  test('displays quick action cards', async ({ page }) => {
    await page.goto('/');

    await expect(page.getByText('Deploy from Git')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('Deploy Docker Image')).toBeVisible();
    await expect(page.getByText('Browse Marketplace')).toBeVisible();
  });

  test('shows "Deploy New App" button', async ({ page }) => {
    await page.goto('/');

    const deployButton = page.getByRole('link', { name: /deploy new app/i });
    await expect(deployButton).toBeVisible({ timeout: 10_000 });
  });

  test('"Deploy New App" navigates to deploy wizard', async ({ page }) => {
    await page.goto('/');

    await page.getByRole('link', { name: /deploy new app/i }).click();
    await expect(page).toHaveURL(/\/apps\/new/);
  });

  test('quick action "Deploy from Git" navigates correctly', async ({ page }) => {
    await page.goto('/');

    await page.getByText('Deploy from Git').click();
    await expect(page).toHaveURL(/\/apps\/new/);
  });

  test('quick action "Browse Marketplace" navigates correctly', async ({ page }) => {
    await page.goto('/');

    await page.getByText('Browse Marketplace').click();
    await expect(page).toHaveURL(/\/marketplace/);
  });

  test('shows recent applications section', async ({ page }) => {
    await page.goto('/');

    await expect(page.getByText('Recent Applications')).toBeVisible({ timeout: 10_000 });
  });

  test('shows activity feed section', async ({ page }) => {
    await page.goto('/');

    await expect(page.getByText('Activity')).toBeVisible({ timeout: 10_000 });
  });

  test('has working search input', async ({ page }) => {
    await page.goto('/');

    // Desktop search (hidden on mobile)
    const searchInput = page.getByPlaceholder('Search apps, domains...');
    await expect(searchInput.first()).toBeVisible({ timeout: 10_000 });
  });
});
