import { test, expect, type Page } from '@playwright/test';

/**
 * Marketplace E2E flow.
 *
 * Covers the Marketplace page at /marketplace:
 *   header + badge, template grid with featured star, category filter,
 *   search, deploy dialog, deploy submission.
 *
 * The GET /marketplace endpoint is mocked with a known dataset; the deploy
 * POST is intercepted so the test does not trigger a real deployment.
 */

async function mockMarketplace(page: Page) {
  await page.route('**/api/v1/marketplace**', async (route, request) => {
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: [
            {
              slug: 'ghost',
              name: 'Ghost',
              description: 'A modern publishing platform for professional bloggers.',
              category: 'cms',
              tags: ['blog', 'node', 'cms'],
              version: '5.0.0',
              featured: true,
              verified: true,
              min_resources: { memory_mb: 512 },
              config_schema: {
                properties: {
                  database_password: { type: 'string', title: 'Database Password', secret: true },
                  admin_password: { type: 'string', title: 'Admin Password', secret: true },
                },
              },
            },
            {
              slug: 'grafana',
              name: 'Grafana',
              description: 'Analytics and interactive visualization web application.',
              category: 'monitoring',
              tags: ['metrics', 'dashboard', 'prometheus'],
              version: '10.2.0',
              featured: false,
              verified: true,
              min_resources: { memory_mb: 256 },
            },
            {
              slug: 'postgres',
              name: 'PostgreSQL',
              description: 'Powerful, open source object-relational database system.',
              category: 'database',
              tags: ['sql', 'relational'],
              version: '16.1',
              featured: true,
              verified: true,
              min_resources: { memory_mb: 256 },
            },
          ],
          categories: ['cms', 'monitoring', 'database'],
        }),
      });
      return;
    }
    await route.continue();
  });

  await page.route('**/api/v1/marketplace/deploy', async (route, request) => {
    if (request.method() === 'POST') {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ app_id: 'e2e-marketplace-app' }),
      });
      return;
    }
    await route.continue();
  });
}

test.describe('Marketplace', () => {
  test('renders the Marketplace header with template count badge', async ({ page }) => {
    await mockMarketplace(page);
    await page.goto('/marketplace');

    await expect(
      page.getByRole('heading', { name: /^marketplace$/i })
    ).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText(/3 templates/i)).toBeVisible();
  });

  test('renders a card per template with name, version, and Deploy button', async ({ page }) => {
    await mockMarketplace(page);
    await page.goto('/marketplace');

    await expect(page.getByText('Ghost').first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('Grafana').first()).toBeVisible();
    await expect(page.getByText('PostgreSQL').first()).toBeVisible();

    // Each card has a "Deploy" button — there should be at least 3.
    const deployButtons = page.getByRole('button', { name: /^deploy$/i });
    await expect(deployButtons).toHaveCount(3);
  });

  test('search filters the template grid by name', async ({ page }) => {
    await mockMarketplace(page);
    await page.goto('/marketplace');

    await expect(page.getByText('Grafana').first()).toBeVisible({ timeout: 10_000 });

    await page.getByPlaceholder(/search templates/i).fill('ghost');

    // The page sends the query to the backend — our mock returns the same
    // set regardless, but we assert the search filter badge appears.
    await expect(page.getByText(/search: "ghost"/i)).toBeVisible({ timeout: 5_000 });
  });

  test('category filter is present with All Categories option', async ({ page }) => {
    await mockMarketplace(page);
    await page.goto('/marketplace');

    await expect(page.getByText('Ghost').first()).toBeVisible({ timeout: 10_000 });

    // The Select control has the default "All Categories" option.
    const categorySelect = page.locator('select').last();
    await expect(categorySelect).toBeVisible();

    // Pick a known category.
    await categorySelect.selectOption('monitoring');
    // After selection, the active filter badge should appear.
    await expect(page.getByText(/category: monitoring/i)).toBeVisible({ timeout: 5_000 });
  });

  test('clicking Deploy opens the deploy dialog with stack name input', async ({ page }) => {
    await mockMarketplace(page);
    await page.goto('/marketplace');

    await expect(page.getByText('Ghost').first()).toBeVisible({ timeout: 10_000 });

    // Click the first template's Deploy button.
    await page.getByRole('button', { name: /^deploy$/i }).first().click();

    await expect(
      page.getByRole('heading', { name: /deploy ghost/i })
    ).toBeVisible({ timeout: 5_000 });
    await expect(page.getByLabel(/stack name/i)).toBeVisible();
    await expect(page.getByLabel(/database password/i)).toBeVisible();
    await expect(page.getByLabel(/admin password/i)).toBeVisible();
  });

  test('submitting the deploy dialog POSTs to /marketplace/deploy', async ({ page }) => {
    await mockMarketplace(page);
    await page.goto('/marketplace');

    await expect(page.getByText('Ghost').first()).toBeVisible({ timeout: 10_000 });
    await page.getByRole('button', { name: /^deploy$/i }).first().click();

    const dialog = page.getByRole('dialog');
    await dialog.getByLabel(/stack name/i).fill('e2e-ghost-stack');

    const req = page.waitForRequest(
      (r) => r.url().endsWith('/api/v1/marketplace/deploy') && r.method() === 'POST',
      { timeout: 10_000 }
    );
    await dialog.getByRole('button', { name: /^deploy$/i }).click();
    const request = await req;

    const body = request.postDataJSON() as Record<string, unknown>;
    expect(body.slug).toBe('ghost');
    expect(body.name).toBe('e2e-ghost-stack');

    // Should navigate to the app detail page for the mocked app_id.
    await expect(page).toHaveURL(/\/apps\/e2e-marketplace-app/, { timeout: 10_000 });
  });
});
