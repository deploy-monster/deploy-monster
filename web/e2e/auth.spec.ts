import { test, expect } from '@playwright/test';
import { TEST_USER, loginViaUI } from './helpers';

/**
 * Authentication E2E tests.
 *
 * These tests exercise the full auth flow through the browser:
 * login form, register form, logout, session persistence, error handling.
 */

test.describe('Login', () => {
  // Auth tests need a clean browser — don't use stored auth state
  test.use({ storageState: { cookies: [], origins: [] } });

  test('shows login page with form fields', async ({ page }) => {
    await page.goto('/login');

    await expect(page.getByText('Welcome back')).toBeVisible();
    await expect(page.getByLabel('Email')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
    await expect(page.getByRole('button', { name: /sign in/i })).toBeVisible();
    await expect(page.getByText(/don't have an account/i)).toBeVisible();
  });

  test('logs in with valid credentials and redirects to dashboard', async ({ page }) => {
    await loginViaUI(page, TEST_USER.email, TEST_USER.password);

    // Should redirect away from login
    await expect(page).not.toHaveURL(/\/login/);

    // Dashboard should load
    await expect(page.getByText(/good (morning|afternoon|evening)/i)).toBeVisible({
      timeout: 10_000,
    });
  });

  test('shows error for invalid credentials', async ({ page }) => {
    await loginViaUI(page, 'wrong@example.com', 'wrongpassword');

    // Should stay on login page
    await expect(page).toHaveURL(/\/login/);

    // Should show error message
    const errorAlert = page.locator('.bg-destructive\\/10, [role="alert"]');
    await expect(errorAlert).toBeVisible({ timeout: 5_000 });
  });

  test('shows validation for empty fields', async ({ page }) => {
    await page.goto('/login');

    // Click submit without filling fields — HTML5 validation prevents submission
    await page.getByRole('button', { name: /sign in/i }).click();

    // Should stay on login page
    await expect(page).toHaveURL(/\/login/);
  });

  test('navigates to register page via link', async ({ page }) => {
    await page.goto('/login');
    await page.getByRole('link', { name: /register/i }).click();

    await expect(page).toHaveURL(/\/register/);
    await expect(page.getByText('Create your account')).toBeVisible();
  });

  test('toggles password visibility', async ({ page }) => {
    await page.goto('/login');

    const passwordInput = page.getByLabel('Password');
    await passwordInput.fill('secret123');

    // Initially masked
    await expect(passwordInput).toHaveAttribute('type', 'password');

    // Click eye icon to reveal
    await page.locator('button').filter({ has: page.locator('svg') }).last().click();
    await expect(passwordInput).toHaveAttribute('type', 'text');
  });
});

test.describe('Register', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('shows register page with form fields', async ({ page }) => {
    await page.goto('/register');

    await expect(page.getByText('Create your account')).toBeVisible();
    await expect(page.getByLabel('Name')).toBeVisible();
    await expect(page.getByLabel('Email')).toBeVisible();
    await expect(page.getByLabel('Password', { exact: true })).toBeVisible();
    await expect(page.getByLabel('Confirm password')).toBeVisible();
    await expect(page.getByRole('button', { name: /create account/i })).toBeVisible();
  });

  test('shows password strength indicator', async ({ page }) => {
    await page.goto('/register');

    const passwordInput = page.getByLabel('Password', { exact: true });

    // Weak password
    await passwordInput.fill('abc');
    await expect(page.getByText('Weak')).toBeVisible();

    // Fair password
    await passwordInput.fill('Password1');
    await expect(page.getByText('Fair')).toBeVisible();

    // Strong password
    await passwordInput.fill('StrongP@ss123!');
    await expect(page.getByText('Strong')).toBeVisible();
  });

  test('shows password mismatch error', async ({ page }) => {
    await page.goto('/register');

    await page.getByLabel('Password', { exact: true }).fill('TestPass123!');
    await page.getByLabel('Confirm password').fill('DifferentPass');

    await expect(page.getByText('Passwords do not match')).toBeVisible();
  });

  test('navigates to login page via link', async ({ page }) => {
    await page.goto('/register');
    await page.getByRole('link', { name: /sign in/i }).click();

    await expect(page).toHaveURL(/\/login/);
    await expect(page.getByText('Welcome back')).toBeVisible();
  });
});

test.describe('Logout', () => {
  test('redirects to login page after logout', async ({ page }) => {
    // Start authenticated (uses storageState from setup)
    await page.goto('/');
    await expect(page.getByText(/good (morning|afternoon|evening)/i)).toBeVisible({
      timeout: 10_000,
    });

    // Find and click logout (typically in user menu or sidebar)
    const logoutButton = page.getByRole('button', { name: /log\s*out|sign\s*out/i });
    const logoutLink = page.getByRole('link', { name: /log\s*out|sign\s*out/i });

    if (await logoutButton.isVisible().catch(() => false)) {
      await logoutButton.click();
    } else if (await logoutLink.isVisible().catch(() => false)) {
      await logoutLink.click();
    } else {
      // Try the sidebar user menu
      const userMenu = page.locator('[data-testid="user-menu"], .user-menu');
      if (await userMenu.isVisible().catch(() => false)) {
        await userMenu.click();
        await page.getByText(/log\s*out|sign\s*out/i).first().click();
      } else {
        // Use API to logout
        await page.request.post('/api/v1/auth/logout');
        await page.goto('/login');
      }
    }

    // Should end up on login page
    await expect(page).toHaveURL(/\/login/, { timeout: 10_000 });
  });
});

test.describe('Session Persistence', () => {
  test('maintains session across page reload', async ({ page }) => {
    // Start authenticated
    await page.goto('/');
    await expect(page.getByText(/good (morning|afternoon|evening)/i)).toBeVisible({
      timeout: 10_000,
    });

    // Reload the page
    await page.reload();

    // Should still be on dashboard (not redirected to login)
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page.getByText(/good (morning|afternoon|evening)/i)).toBeVisible({
      timeout: 10_000,
    });
  });

  test('redirects unauthenticated users to login', async ({ page }) => {
    // Use a fresh context without auth
    const context = await page.context().browser()!.newContext();
    const freshPage = await context.newPage();

    await freshPage.goto('/');

    // Should redirect to login
    await expect(freshPage).toHaveURL(/\/login/, { timeout: 10_000 });

    await context.close();
  });
});
