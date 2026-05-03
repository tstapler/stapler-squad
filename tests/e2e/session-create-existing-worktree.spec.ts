// @feature session:create-existing-worktree
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

async function openInCreationMode(page: import('@playwright/test').Page) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
  await page.keyboard.press('Control+Shift+K');
  await expect(page.getByRole('radiogroup', { name: 'Session type' })).toBeVisible({ timeout: 5000 });
}

test.describe('existing worktree session creation', () => {
  test('existing worktree option is selectable', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Use Worktree' }).click();

    await expect(page.getByRole('radio', { name: 'Use Worktree' })).toHaveAttribute('aria-checked', 'true');
  });

  test('shows worktree path input when existing worktree is selected', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Use Worktree' }).click();

    await expect(page.getByLabel('Existing Worktree Path')).toBeVisible();
  });

  test('hides branch controls when existing worktree is selected', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Use Worktree' }).click();

    await expect(page.getByText(/Use session name as branch name/i)).not.toBeVisible();
  });

  test('shows working directory field for existing worktree mode', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Use Worktree' }).click();

    await expect(page.getByLabel('Working Directory')).toBeVisible();
  });

  test('submit is disabled when worktree path is empty', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Use Worktree' }).click();

    // Provide path + name — but no existingWorktree path
    await page.locator('input[aria-label="Session source input"]').fill('/tmp');
    await page.getByLabel('Session Name').fill('my-worktree-session');

    // canSubmit requires existingWorktree.trim() to be non-empty for this mode
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeDisabled();
  });

  test('sends existing worktree type with worktree path in payload', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Use Worktree' }).click();

    await page.locator('input[aria-label="Session source input"]').fill('/tmp');
    // Wait for detection (auto-fills session name, but does NOT enable submit yet — worktree required)
    await expect(page.getByLabel('Session Name')).not.toHaveValue('', { timeout: 3000 });

    // Fill the existing worktree path (text input, no worktrees pre-seeded in test server)
    await page.getByLabel('Existing Worktree Path').fill('/tmp/worktree');

    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });

    const requestPromise = page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST'
    );
    await page.getByRole('button', { name: 'Create Session' }).click();

    const request = await requestPromise;
    const body = request.postDataJSON();
    // sessionType 3 = SESSION_TYPE_EXISTING_WORKTREE or its string form
    const st = body.sessionType;
    expect(st === 3 || st === 'SESSION_TYPE_EXISTING_WORKTREE').toBe(true);
    expect(body.existingWorktree).toBe('/tmp/worktree');
    expect(body.path).toBeTruthy();
    expect(body.oneOff).toBeFalsy();
  });
});
