// @feature session:create-new-worktree
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8543';

async function openInCreationMode(page: import('@playwright/test').Page) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
  await page.keyboard.press('Control+Shift+K');
  await expect(page.getByRole('radiogroup', { name: 'Session type' })).toBeVisible({ timeout: 5000 });
}

test.describe('new worktree session creation', () => {
  test('new worktree is the default selection', async ({ page }) => {
    await openInCreationMode(page);

    await expect(page.getByRole('radio', { name: 'New Worktree' })).toHaveAttribute('aria-checked', 'true');
  });

  test('shows use-title-as-branch checkbox for new worktree mode', async ({ page }) => {
    await openInCreationMode(page);

    // new_worktree is already selected by default
    await expect(page.getByText(/Use session name as branch name/i)).toBeVisible();
    // Checkbox should be checked by default
    await expect(page.locator('input[type="checkbox"]').filter({ hasText: /branch/i })).not.toBeVisible();
    // Find it by its sibling text
    const checkbox = page.locator('label').filter({ hasText: /Use session name as branch name/i }).locator('input[type="checkbox"]');
    await expect(checkbox).toBeChecked();
  });

  test('branch field is read-only while use-title-as-branch is checked', async ({ page }) => {
    await openInCreationMode(page);

    await page.getByLabel('Session Name').fill('my-feature');
    const branchInput = page.getByLabel(/Git Branch/i);
    await expect(branchInput).toBeVisible();
    await expect(branchInput).toBeDisabled();
    // Placeholder shows the session name
    await expect(branchInput).toHaveValue('my-feature');
  });

  test('branch field becomes editable when use-title-as-branch is unchecked', async ({ page }) => {
    await openInCreationMode(page);

    const checkbox = page.locator('label').filter({ hasText: /Use session name as branch name/i }).locator('input[type="checkbox"]');
    await checkbox.uncheck();

    const branchInput = page.getByLabel(/Git Branch/i);
    await expect(branchInput).toBeEnabled();
    await branchInput.fill('feature/my-branch');
    await expect(branchInput).toHaveValue('feature/my-branch');
  });

  test('sends new worktree type with branch in payload', async ({ page }) => {
    await openInCreationMode(page);

    // Type a local path — triggers detection and auto-fills session name (used as branch)
    await page.locator('input[aria-label="Session source input"]').fill('/tmp');
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });

    const requestPromise = page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST'
    );
    await page.getByRole('button', { name: 'Create Session' }).click();

    const request = await requestPromise;
    const body = request.postDataJSON();
    // sessionType 2 = SESSION_TYPE_NEW_WORKTREE or its string form
    const st = body.sessionType;
    expect(st === 2 || st === 'SESSION_TYPE_NEW_WORKTREE').toBe(true);
    expect(body.path).toBeTruthy();
    // Branch is set (from session name via useTitleAsBranch)
    expect(body.branch).toBeTruthy();
    expect(body.oneOff).toBeFalsy();
  });
});
