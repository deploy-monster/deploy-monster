import { test, expect, type Page } from '@playwright/test';

/**
 * Team management E2E flow.
 *
 * Covers the Team page at /team:
 *   header + Invite CTA, member table, invite dialog (email + role),
 *   POST /team/invites submission, tab switch between Members and Audit Log.
 *
 * The list endpoints are intercepted so the test isn't dependent on the
 * real user fixture state; the invite POST is also captured to assert the
 * submitted body.
 */

async function mockTeam(
  page: Page,
  {
    members = [],
    audit = [],
  }: { members?: unknown[]; audit?: unknown[] } = {}
) {
  await page.route('**/api/v1/team/members', async (route, request) => {
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(members),
      });
      return;
    }
    await route.continue();
  });

  await page.route('**/api/v1/team/audit-log', async (route, request) => {
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(audit),
      });
      return;
    }
    await route.continue();
  });

  await page.route('**/api/v1/team/invites', async (route, request) => {
    if (request.method() === 'POST') {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'inv-e2e-1', status: 'sent' }),
      });
      return;
    }
    await route.continue();
  });
}

function fakeMember(overrides: Partial<{
  id: string;
  name: string;
  email: string;
  role: string;
  joined_at: string;
}> = {}) {
  return {
    id: 'm-1',
    name: 'Alice Admin',
    email: 'alice@deploymonster.test',
    role: 'role_admin',
    joined_at: new Date(Date.now() - 86_400 * 1000).toISOString(),
    ...overrides,
  };
}

test.describe('Team Management', () => {
  test('renders the Team page header and Invite Member CTA', async ({ page }) => {
    await mockTeam(page, { members: [fakeMember()] });
    await page.goto('/team');

    await expect(
      page.getByRole('heading', { name: /team management/i })
    ).toBeVisible({ timeout: 10_000 });
    await expect(
      page.getByRole('button', { name: /invite member/i })
    ).toBeVisible();
  });

  test('shows the empty state when there are no members', async ({ page }) => {
    await mockTeam(page, { members: [] });
    await page.goto('/team');

    await expect(
      page.getByRole('heading', { name: /no team members yet/i })
    ).toBeVisible({ timeout: 10_000 });
    await expect(
      page.getByRole('button', { name: /invite your first member/i })
    ).toBeVisible();
  });

  test('renders the member row with the member name and email', async ({ page }) => {
    await mockTeam(page, {
      members: [
        fakeMember({ id: 'a', name: 'Alice Admin', email: 'alice@test.dev', role: 'role_admin' }),
        fakeMember({ id: 'b', name: 'Bob Dev', email: 'bob@test.dev', role: 'role_developer' }),
      ],
    });
    await page.goto('/team');

    await expect(page.getByText('Alice Admin')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('alice@test.dev')).toBeVisible();
    await expect(page.getByText('Bob Dev')).toBeVisible();
    await expect(page.getByText('bob@test.dev')).toBeVisible();
  });

  test('opens the Invite Member dialog with email + role fields', async ({ page }) => {
    await mockTeam(page, { members: [fakeMember()] });
    await page.goto('/team');

    await page.getByRole('button', { name: /invite member/i }).click();

    await expect(
      page.getByRole('heading', { name: /invite team member/i })
    ).toBeVisible();
    await expect(page.getByLabel(/email address/i)).toBeVisible();
    await expect(page.getByLabel(/^role$/i)).toBeVisible();

    // Default role should be Developer.
    const select = page.getByLabel(/^role$/i);
    await expect(select).toHaveValue('role_developer');
  });

  test('Send Invite button is disabled until email is filled', async ({ page }) => {
    await mockTeam(page, { members: [fakeMember()] });
    await page.goto('/team');

    await page.getByRole('button', { name: /invite member/i }).click();

    const dialog = page.getByRole('dialog');
    const sendBtn = dialog.getByRole('button', { name: /send invite/i });
    await expect(sendBtn).toBeDisabled();

    await dialog.getByLabel(/email address/i).fill('new@dev.test');
    await expect(sendBtn).toBeEnabled();
  });

  test('submitting the invite dialog POSTs to /api/v1/team/invites', async ({ page }) => {
    await mockTeam(page, { members: [fakeMember()] });
    await page.goto('/team');

    await page.getByRole('button', { name: /invite member/i }).click();

    const dialog = page.getByRole('dialog');
    await dialog.getByLabel(/email address/i).fill('invitee@dev.test');
    await dialog.getByLabel(/^role$/i).selectOption('role_operator');

    const req = page.waitForRequest(
      (r) => r.url().endsWith('/api/v1/team/invites') && r.method() === 'POST',
      { timeout: 10_000 }
    );
    await dialog.getByRole('button', { name: /send invite/i }).click();
    const request = await req;

    const body = request.postDataJSON() as Record<string, unknown>;
    expect(body.email).toBe('invitee@dev.test');
    expect(body.role_id).toBe('role_operator');
  });

  test('switching to Audit Log tab reveals the audit section', async ({ page }) => {
    await mockTeam(page, {
      members: [fakeMember()],
      audit: [],
    });
    await page.goto('/team');

    await page.getByRole('tab', { name: /audit log/i }).click();

    // Empty audit state heading.
    await expect(
      page.getByRole('heading', { name: /no audit log entries/i })
    ).toBeVisible({ timeout: 5_000 });
  });
});
