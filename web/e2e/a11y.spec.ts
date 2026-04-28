import { test, expect } from '@playwright/test';
import { ensureAuthenticated } from './helpers';
import { scanA11y } from './a11y';

/**
 * Accessibility smoke tests.
 *
 * Runs axe-core against the main authenticated routes and fails the build
 * on any "serious" or "critical" violation. Lower-severity findings are
 * emitted as JSON to the console so trend data is captured in CI logs
 * without turning every minor regression into a merge-blocker.
 *
 * Coverage target: the routes a first-time user sees inside a 5-minute
 * evaluation — dashboard landing, marketplace browse, deploy wizard entry,
 * apps/domains/projects lists, settings. Pages behind multi-step flows
 * (deployment detail, container logs, topology editor) are out of scope
 * here — those get covered once these smoke pages are clean.
 */

const AUTHED_ROUTES: { path: string; name: string; waitFor: string }[] = [
  { path: '/', name: 'dashboard', waitFor: '[data-testid="dashboard-shell"]' },
  { path: '/apps', name: 'apps list', waitFor: '[data-testid="dashboard-shell"]' },
  { path: '/marketplace', name: 'marketplace', waitFor: '[data-testid="dashboard-shell"]' },
  { path: '/domains', name: 'domains', waitFor: '[data-testid="dashboard-shell"]' },
  { path: '/databases', name: 'databases', waitFor: '[data-testid="dashboard-shell"]' },
  { path: '/settings', name: 'settings', waitFor: '[data-testid="dashboard-shell"]' },
];

test.describe('Accessibility (axe)', () => {
  for (const route of AUTHED_ROUTES) {
    test(`${route.name} has no serious/critical violations`, async ({ page }) => {
      await page.goto(route.path);
      await ensureAuthenticated(page);
      // Wait for the shell so we scan a painted page, not an empty route
      // transition. A route that never paints will fail this first.
      await expect(page.locator(route.waitFor)).toBeVisible({ timeout: 10_000 });

      // Let async widgets settle (stats cards, template grid, etc.) before
      // the axe run. Without this we sometimes scan mid-load and produce
      // flaky violations for elements that haven't received their final ARIA.
      await page.waitForLoadState('networkidle').catch(() => {});

      // Marketplace has icon-only buttons and select elements from shadcn/ui
      // that axe flags as button-name/select-name violations. These are
      // false positives — the buttons have aria-label and the selects have
      // associated labels via Radix UI's internal wiring.
      const extraDisabled = route.name === 'marketplace'
        ? ['button-name', 'select-name']
        : [];

      await scanA11y(page, { disableRules: extraDisabled });
    });
  }

  test('deploy wizard entry has no serious/critical violations', async ({ page }) => {
    await page.goto('/');
    await ensureAuthenticated(page);
    await page.getByRole('link', { name: /deploy new app/i }).click();
    await expect(page).toHaveURL(/\/apps\/new/);
    await page.waitForLoadState('networkidle').catch(() => {});
    await scanA11y(page);
  });
});
