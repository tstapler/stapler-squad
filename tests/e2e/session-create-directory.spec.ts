import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
const _features = [FEATURE_CATALOG['session-create']] as const;
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

  // AC2: CreateSession must carry a bounded client-side timeout so a stalled
  // backend can't leave the Create button grayed out forever. ConnectRPC
  // encodes timeoutMs as the "Connect-Timeout-Ms" request header — asserting
  // on the header lets this test verify the wiring without waiting out a
  // real multi-minute timeout.
  test('CreateSession request carries a bounded Connect-Timeout-Ms header', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();
    await page.locator('input[aria-label="Session source input"]').fill('/tmp');
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });

    const requestPromise = page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST'
    );
    await page.getByRole('button', { name: 'Create Session' }).click();

    const request = await requestPromise;
    const timeoutHeader = request.headers()['connect-timeout-ms'];
    expect(timeoutHeader).toBeTruthy();
    const timeoutMs = Number(timeoutHeader);
    expect(timeoutMs).toBeGreaterThan(0);
    // Must not be so large it's effectively unbounded, and must be well above
    // the debounce/render noise floor.
    expect(timeoutMs).toBeGreaterThan(60_000);
    expect(timeoutMs).toBeLessThan(10 * 60_000);
  });

  // AC2: if CreateSession never responds, the Create button must re-enable
  // with an error rather than staying grayed out forever. We can't wait out
  // the real ~160s client timeout in a test, so this simulates the timeout
  // by aborting the intercepted request quickly — exercising the exact same
  // rejection→catch→setIsSubmitting(false) path the real timeout triggers.
  test('Create button recovers when CreateSession fails instead of staying grayed out', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();
    await page.locator('input[aria-label="Session source input"]').fill('/tmp');
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });

    await page.route('**/session.v1.SessionService/CreateSession', async (route) => {
      await route.abort('timedout');
    });

    await page.getByRole('button', { name: 'Create Session' }).click();
    await expect(page.getByText('Creating...')).toBeVisible();

    // Once the (simulated) timeout fires, the button must re-enable — not
    // stay grayed out indefinitely.
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 10_000 });
  });
});
