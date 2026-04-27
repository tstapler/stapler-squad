import { test, expect } from '@playwright/test';

const BASE_URL = 'http://localhost:8544';

test.describe('one-off session creation', () => {
  test('shows one-off option in creation panel', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });

    // Open omnibar / new session panel
    await page.getByText('New Session').click();

    // The One-off radio option must be visible
    await expect(page.getByRole('radio', { name: /one.off/i })).toBeVisible({ timeout: 5000 });
  });

  test('hides path input when one-off is selected', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    await page.getByText('New Session').click();

    // Select the One-off option
    await page.getByRole('radio', { name: /one.off/i }).click();

    // Path / directory input must not be visible
    await expect(page.getByPlaceholder(/path|directory|repo/i)).not.toBeVisible();

    // Session name input must still be visible and required
    await expect(page.getByPlaceholder(/session name|title/i)).toBeVisible();
  });

  test('creates session with one_off flag', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    await page.getByText('New Session').click();

    // Select One-off and fill title
    await page.getByRole('radio', { name: /one.off/i }).click();
    await page.getByPlaceholder(/session name|title/i).fill('e2e-one-off-test');

    // Intercept the CreateSession RPC to verify one_off is set
    const requestPromise = page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST'
    );

    await page.getByRole('button', { name: /create|start/i }).click();

    const request = await requestPromise;
    const body = request.postDataJSON();
    expect(body).toMatchObject({ oneOff: true });
    expect(body.path ?? '').toBe('');
  });
});
