import { test, expect, type Page } from '@playwright/test';

/**
 * Deploy Wizard E2E flow.
 *
 * Covers the three-step deploy wizard at /apps/new:
 *   Source -> Configure -> Review -> Deploy
 *
 * The final POST /api/v1/apps call is intercepted with page.route() so the
 * test can exercise the full UI without actually spinning up a container.
 */

/** Stub the POST /api/v1/apps endpoint with a fake success response. */
async function mockDeploySuccess(page: Page, appId = 'e2e-test-app') {
  await page.route('**/api/v1/apps', async (route, request) => {
    if (request.method() === 'POST') {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ id: appId, name: appId, status: 'deploying' }),
      });
      return;
    }
    await route.continue();
  });
}

test.describe('Deploy Wizard', () => {
  test('renders the three-step progress and header', async ({ page }) => {
    await page.goto('/apps/new');

    await expect(
      page.getByRole('heading', { name: /deploy new application/i })
    ).toBeVisible({ timeout: 10_000 });

    // Three step pills: Source / Configure / Review.
    await expect(page.getByText('Source')).toBeVisible();
    await expect(page.getByText('Configure')).toBeVisible();
    await expect(page.getByText('Review')).toBeVisible();

    // The New Deployment badge is part of the header.
    await expect(page.getByText(/new deployment/i)).toBeVisible();
  });

  test('Next button on the source step is disabled until a source is picked', async ({ page }) => {
    await page.goto('/apps/new');

    const next = page.getByRole('button', { name: /^next$/i });
    await expect(next).toBeDisabled();

    await page.getByText(/git repository/i).click();
    await expect(next).toBeEnabled();
  });

  test('advances from Source to Configure and back with Back', async ({ page }) => {
    await page.goto('/apps/new');

    // Source -> Configure
    await page.getByText(/git repository/i).click();
    await page.getByRole('button', { name: /^next$/i }).click();

    await expect(
      page.getByRole('heading', { name: /configure your application/i })
    ).toBeVisible({ timeout: 5_000 });
    await expect(page.getByLabel(/application name/i)).toBeVisible();

    // Back -> Source
    await page.getByRole('button', { name: /^back$/i }).click();
    await expect(
      page.getByRole('heading', { name: /choose deployment source/i })
    ).toBeVisible({ timeout: 5_000 });
  });

  test('Git source shows repository URL + branch fields on Configure', async ({ page }) => {
    await page.goto('/apps/new');
    await page.getByText(/git repository/i).click();
    await page.getByRole('button', { name: /^next$/i }).click();

    await expect(page.getByLabel(/repository url/i)).toBeVisible();
    await expect(page.getByLabel(/branch/i)).toBeVisible();
    // The Docker Image field must NOT render for git source.
    await expect(page.getByLabel(/docker image/i)).not.toBeVisible();
  });

  test('Docker Image source shows image field instead of branch', async ({ page }) => {
    await page.goto('/apps/new');
    await page.getByText(/docker image/i).click();
    await page.getByRole('button', { name: /^next$/i }).click();

    await expect(page.getByLabel(/docker image/i)).toBeVisible();
    // Git-specific fields should not render.
    await expect(page.getByLabel(/repository url/i)).not.toBeVisible();
    await expect(page.getByLabel(/^branch$/i)).not.toBeVisible();
  });

  test('Next on Configure is disabled until a name is provided', async ({ page }) => {
    await page.goto('/apps/new');
    await page.getByText(/docker image/i).click();
    await page.getByRole('button', { name: /^next$/i }).click();

    const next = page.getByRole('button', { name: /^next$/i });
    await expect(next).toBeDisabled();

    await page.getByLabel(/application name/i).fill('e2e-wizard-app');
    await expect(next).toBeEnabled();
  });

  test('review screen lists configuration rows before deploy', async ({ page }) => {
    await page.goto('/apps/new');
    await page.getByText(/git repository/i).click();
    await page.getByRole('button', { name: /^next$/i }).click();

    await page.getByLabel(/application name/i).fill('e2e-review-app');
    await page.getByLabel(/repository url/i).fill('https://github.com/e2e/demo.git');
    await page.getByRole('button', { name: /^next$/i }).click();

    // Review screen.
    await expect(
      page.getByRole('heading', { name: /review and deploy/i })
    ).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/deployment summary/i)).toBeVisible();
    await expect(page.getByText('e2e-review-app')).toBeVisible();
    await expect(page.getByText('https://github.com/e2e/demo.git')).toBeVisible();
  });

  test('Deploy button triggers POST /api/v1/apps and navigates to detail', async ({ page }) => {
    await mockDeploySuccess(page, 'e2e-deployed-123');

    await page.goto('/apps/new');
    await page.getByText(/docker image/i).click();
    await page.getByRole('button', { name: /^next$/i }).click();

    await page.getByLabel(/application name/i).fill('e2e-deploy-target');
    await page.getByLabel(/docker image/i).fill('ghcr.io/e2e/demo:latest');
    await page.getByRole('button', { name: /^next$/i }).click();

    const deployRequest = page.waitForRequest(
      (req) => req.url().endsWith('/api/v1/apps') && req.method() === 'POST',
      { timeout: 10_000 }
    );
    await page.getByRole('button', { name: /deploy application/i }).click();
    const req = await deployRequest;

    const body = req.postDataJSON() as Record<string, unknown>;
    expect(body.name).toBe('e2e-deploy-target');
    expect(body.source_type).toBe('image');

    // Navigates to /apps/{id}
    await expect(page).toHaveURL(/\/apps\/e2e-deployed-123/, { timeout: 10_000 });
  });
});
