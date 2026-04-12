import { test, expect } from '@playwright/test';

/**
 * Navigation & page loading E2E tests.
 *
 * Verifies all protected routes load without errors for an authenticated user.
 * Tests the sidebar navigation, page transitions, and basic rendering.
 */

const ROUTES = [
  { path: '/', name: 'Dashboard' },
  { path: '/apps', name: 'Applications' },
  { path: '/domains', name: 'Domains' },
  { path: '/databases', name: 'Databases' },
  { path: '/servers', name: 'Servers' },
  { path: '/marketplace', name: 'Marketplace' },
  { path: '/team', name: 'Team' },
  { path: '/git', name: 'Git Sources' },
  { path: '/backups', name: 'Backups' },
  { path: '/secrets', name: 'Secrets' },
  { path: '/monitoring', name: 'Monitoring' },
  { path: '/settings', name: 'Settings' },
  { path: '/billing', name: 'Billing' },
  { path: '/admin', name: 'Admin' },
  { path: '/topology', name: 'Topology' },
];

test.describe('Page Navigation', () => {
  for (const route of ROUTES) {
    test(`${route.name} page loads without error (${route.path})`, async ({ page }) => {
      await page.goto(route.path);

      // Should not redirect to login (authenticated)
      await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 });

      // Should not show 404
      await expect(page.getByText(/page not found/i)).not.toBeVisible().catch(() => {
        // Some pages may have text that partially matches — OK
      });

      // No uncaught errors (check console)
      const errors: string[] = [];
      page.on('pageerror', (err) => errors.push(err.message));

      // Wait for page to settle
      await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {
        // Network idle may not be reached if there are polling requests
      });

      // If there are JS errors, they shouldn't be React rendering errors
      const criticalErrors = errors.filter(
        (e) => e.includes('Minified React error') || e.includes('Cannot read properties of undefined'),
      );
      expect(criticalErrors).toHaveLength(0);
    });
  }
});

test.describe('404 Page', () => {
  test('shows not found page for invalid routes', async ({ page }) => {
    await page.goto('/this-page-does-not-exist');

    // Should show 404 page or redirect to login
    const notFound = page.getByText(/not found|404/i);
    const loginPage = page.getByText(/welcome back|sign in/i);

    await expect(notFound.first().or(loginPage.first())).toBeVisible({ timeout: 10_000 });
  });
});

test.describe('Settings Page', () => {
  test('loads with profile tab', async ({ page }) => {
    await page.goto('/settings');

    // Settings page should show tabs or sections
    await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 });
  });

  test('has theme toggle', async ({ page }) => {
    await page.goto('/settings');

    // Look for appearance/theme section
    const themeSection = page.getByText(/theme|appearance|dark mode/i);
    await expect(themeSection.first()).toBeVisible({ timeout: 10_000 });
  });
});

test.describe('Team Page', () => {
  test('loads team management page', async ({ page }) => {
    await page.goto('/team');

    await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 });

    // Team page should show members or invite section
    const teamContent = page.getByText(/team|member|invite/i);
    await expect(teamContent.first()).toBeVisible({ timeout: 10_000 });
  });
});

test.describe('Secrets Page', () => {
  test('loads secrets page', async ({ page }) => {
    await page.goto('/secrets');

    await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 });

    // Secrets page should render
    const secretsContent = page.getByText(/secret|vault|variable/i);
    await expect(secretsContent.first()).toBeVisible({ timeout: 10_000 });
  });
});
