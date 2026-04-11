import { test, expect, type Page } from '@playwright/test';

/**
 * Domain setup E2E flow.
 *
 * Covers the Domains page at /domains:
 *   header + Add Domain CTA, dialog form, DNS hint preview,
 *   POST /domains submission, search, table rendering.
 *
 * The POST /api/v1/domains request is intercepted so the test can verify
 * the submitted body without depending on an actual backend accepting it.
 */

/** Stub the domains list + add endpoints with a controlled dataset. */
async function mockDomains(page: Page, existing: unknown[] = []) {
  const list = [...existing];

  await page.route('**/api/v1/domains', async (route, request) => {
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(list),
      });
      return;
    }
    if (request.method() === 'POST') {
      const body = JSON.parse(request.postData() || '{}');
      list.push({
        id: `dom-${list.length + 1}`,
        fqdn: body.fqdn,
        app_id: body.app_id || '',
        verified: false,
        dns_synced: false,
        type: 'custom',
        created_at: new Date().toISOString(),
      });
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ id: `dom-${list.length}` }),
      });
      return;
    }
    await route.continue();
  });
}

test.describe('Domain Setup', () => {
  test('renders the Domains header and Add Domain CTA', async ({ page }) => {
    await mockDomains(page, []);
    await page.goto('/domains');

    await expect(
      page.getByRole('heading', { name: /^domains$/i })
    ).toBeVisible({ timeout: 10_000 });
    await expect(
      page.getByRole('button', { name: /add domain/i }).first()
    ).toBeVisible();
  });

  test('shows empty state when the domain list is empty', async ({ page }) => {
    await mockDomains(page, []);
    await page.goto('/domains');

    await expect(
      page.getByRole('heading', { name: /no domains configured/i })
    ).toBeVisible({ timeout: 10_000 });
    await expect(
      page.getByRole('button', { name: /add your first domain/i })
    ).toBeVisible();
  });

  test('opens the Add Domain dialog with FQDN input and DNS hint', async ({ page }) => {
    await mockDomains(page, []);
    await page.goto('/domains');

    await page.getByRole('button', { name: /add your first domain/i }).click();

    await expect(
      page.getByRole('heading', { name: /add custom domain/i })
    ).toBeVisible();
    await expect(page.getByLabel(/domain \(fqdn\)/i)).toBeVisible();
    await expect(page.getByLabel(/application id/i)).toBeVisible();
    // The placeholder preview reads "app.example.com" before typing.
    await expect(page.getByText(/dns configuration/i)).toBeVisible();
  });

  test('live DNS hint reflects the FQDN as the user types', async ({ page }) => {
    await mockDomains(page, []);
    await page.goto('/domains');

    await page.getByRole('button', { name: /add your first domain/i }).click();
    await page.getByLabel(/domain \(fqdn\)/i).fill('e2e.deploymonster.test');

    // The <code> block echoes the typed domain as "fqdn  A  -> your-server-ip".
    const hint = page.locator('code').filter({ hasText: 'e2e.deploymonster.test' });
    await expect(hint).toBeVisible();
  });

  test('Add Domain button is disabled until FQDN is filled', async ({ page }) => {
    await mockDomains(page, []);
    await page.goto('/domains');

    await page.getByRole('button', { name: /add your first domain/i }).click();

    // Dialog has two buttons with "Add Domain" text (header CTA + dialog footer).
    // Grab the one inside the dialog by scoping to the dialog role.
    const dialog = page.getByRole('dialog');
    const addBtn = dialog.getByRole('button', { name: /add domain/i });
    await expect(addBtn).toBeDisabled();

    await dialog.getByLabel(/domain \(fqdn\)/i).fill('e2e.example.com');
    await expect(addBtn).toBeEnabled();
  });

  test('submitting the dialog POSTs to /api/v1/domains with the FQDN', async ({ page }) => {
    await mockDomains(page, []);
    await page.goto('/domains');

    await page.getByRole('button', { name: /add your first domain/i }).click();

    const dialog = page.getByRole('dialog');
    await dialog.getByLabel(/domain \(fqdn\)/i).fill('created.example.com');

    const req = page.waitForRequest(
      (r) => r.url().endsWith('/api/v1/domains') && r.method() === 'POST',
      { timeout: 10_000 }
    );
    await dialog.getByRole('button', { name: /add domain/i }).click();
    const request = await req;

    const body = request.postDataJSON() as Record<string, unknown>;
    expect(body.fqdn).toBe('created.example.com');
  });

  test('renders the domain table and summary cards when domains exist', async ({ page }) => {
    await mockDomains(page, [
      {
        id: 'd1',
        fqdn: 'one.example.com',
        app_id: 'app_one',
        verified: true,
        dns_synced: true,
        type: 'custom',
        created_at: new Date().toISOString(),
      },
      {
        id: 'd2',
        fqdn: 'two.example.com',
        app_id: '',
        verified: false,
        dns_synced: false,
        type: 'custom',
        created_at: new Date().toISOString(),
      },
    ]);
    await page.goto('/domains');

    await expect(page.getByText('one.example.com').first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('two.example.com').first()).toBeVisible();
    await expect(page.getByText(/total domains/i)).toBeVisible();
    await expect(page.getByText(/ssl active/i)).toBeVisible();
    await expect(page.getByText(/pending verification/i)).toBeVisible();
  });

  test('search filters the domain list', async ({ page }) => {
    await mockDomains(page, [
      {
        id: 'd1',
        fqdn: 'alpha.example.com',
        app_id: '',
        verified: true,
        dns_synced: true,
        type: 'custom',
        created_at: new Date().toISOString(),
      },
      {
        id: 'd2',
        fqdn: 'beta.example.com',
        app_id: '',
        verified: false,
        dns_synced: false,
        type: 'custom',
        created_at: new Date().toISOString(),
      },
    ]);
    await page.goto('/domains');

    await expect(page.getByText('alpha.example.com').first()).toBeVisible({ timeout: 10_000 });

    await page.getByPlaceholder(/search domains/i).fill('alpha');

    // Beta should disappear from the visible table rows after debounce.
    await expect(page.getByText('beta.example.com')).not.toBeVisible({ timeout: 5_000 });
    await expect(page.getByText('alpha.example.com').first()).toBeVisible();
  });
});
