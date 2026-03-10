import { test, expect } from '@playwright/test';

// The wizard is started by playwright.config.ts webServer with
// WIZARD_ADMIN_PASSWORD=e2e-test-password on the configured port.

test.describe('Login page', () => {
  test('renders the login form', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('h2')).toHaveText('Welcome to SafePaw');
    await expect(page.locator('#password')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
  });

  test('shows error on wrong password', async ({ page }) => {
    await page.goto('/');
    await page.locator('#password').fill('wrong-password');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByText('Invalid password')).toBeVisible();
  });

  test('successful login navigates away from login', async ({ page }) => {
    await page.goto('/');
    await page.locator('#password').fill('e2e-test-password');
    await page.getByRole('button', { name: 'Sign In' }).click();

    // After login the app goes to prerequisites or setup — either way the
    // login heading disappears and a post-login heading appears.
    await expect(page.locator('h2', { hasText: 'Welcome to SafePaw' })).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();
  });

  test('logout returns to login page', async ({ page }) => {
    // First log in
    await page.goto('/');
    await page.locator('#password').fill('e2e-test-password');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page.getByRole('button', { name: 'Logout' })).toBeVisible();

    // Then log out
    await page.getByRole('button', { name: 'Logout' }).click();
    await expect(page.locator('h2')).toHaveText('Welcome to SafePaw');
    await expect(page.locator('#password')).toBeVisible();
  });
});
