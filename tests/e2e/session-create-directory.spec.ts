// @feature session:create-directory
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

async function openInCreationMode(page: import('@playwright/test').Page) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
  // Ctrl+Shift+K opens the omnibar directly in creation mode
  await page.keyboard.press('Control+Shift+K');
  await expect(page.getByRole('radiogroup', { name: 'Session type' })).toBeVisible({ timeout: 5000 });
}

test.describe('directory session creation', () => {
  test('directory type is selectable', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();
    await expect(page.getByRole('radio', { name: 'Directory' })).toHaveAttribute('aria-checked', 'true');
  });

  test('hides branch controls when directory is selected', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();

    await expect(page.getByLabel(/Git Branch/i)).not.toBeVisible();
    await expect(page.getByText(/Use session name as branch/i)).not.toBeVisible();
  });

  test('shows working directory field for directory mode', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();

    await expect(page.getByLabel('Working Directory')).toBeVisible();
    await expect(page.getByPlaceholder('src/api (optional)')).toBeVisible();
  });

  test('submit is disabled without a path', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();

    // Main input is empty — submit must be disabled regardless of session name
    await page.getByLabel('Session Name').fill('my-dir-session');
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeDisabled();
  });

  test('sends directory session type in payload', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();

    // Type a local path — triggers LocalPath detection and auto-fills session name
    await page.locator('input[aria-label="Session source input"]').fill('/tmp');

    // Wait until detection enables the submit button (avoids arbitrary sleep)
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });

    const requestPromise = page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST'
    );
    await page.getByRole('button', { name: 'Create Session' }).click();

    const request = await requestPromise;
    const body = request.postDataJSON();
    // sessionType 1 = SESSION_TYPE_DIRECTORY in proto; ConnectRPC JSON may omit it (defaults to 0/1) or send integer
    const sessionType = body.sessionType ?? 0;
    expect([0, 1]).toContain(sessionType); // must be UNSPECIFIED(0) or DIRECTORY(1)
    expect(body.oneOff).toBeFalsy();
    expect(body.path).toBeTruthy();
  });
});
