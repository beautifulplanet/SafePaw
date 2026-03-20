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

  // Professor-grade: brute-force protection — 5 wrong passwords trigger lockout (429).
  // Server uses NewLoginGuard(5, 1min, 15min). We submit 6 times so we definitely hit lockout
  // even if another test (same IP, shared server) already recorded failures.
  test('lockout after 5 wrong passwords', async ({ page }) => {
    await page.goto('/');
    const wrongPassword = 'wrong-password-e2e';
    for (let i = 0; i < 6; i++) {
      await page.locator('#password').fill(wrongPassword);
      await page.getByRole('button', { name: 'Sign In' }).click();
      await page.waitForTimeout(600); // server has 500ms delay on failure/lockout response
    }
    await expect(page.getByText(/too many|locked out/i)).toBeVisible({ timeout: 5_000 });
  });
});
