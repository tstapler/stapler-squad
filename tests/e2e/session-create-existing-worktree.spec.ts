import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: session-create-existing-worktree — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['session-create-existing-worktree'], // TODO: add to catalog
  FEATURE_CATALOG['session-create'],
] as const;
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

  // AC1: a hung ListWorktrees backend request must not leave the dropdown
  // (or the surrounding Create button) stuck loading forever — it must fall
  // back to a bounded error/manual-entry state within useWorktreeSuggestions'
  // client-side timeout.
  test('falls back to manual entry when ListWorktrees hangs', async ({ page }) => {
    await openInCreationMode(page);

    // Never fulfill the request — simulates a hung backend.
    await page.route('**/session.v1.SessionService/ListWorktrees', () => {
      // Intentionally never calls route.fulfill/continue/abort.
    });

    await page.getByRole('radio', { name: 'Use Worktree' }).click();
    await page.locator('input[aria-label="Session source input"]').fill('/tmp');

    // Loading state shows first...
    await expect(page.getByText(/Scanning for git worktrees/i)).toBeVisible({ timeout: 3000 });

    // ...but must resolve to the manual-entry fallback within the client
    // timeout, not stay stuck loading indefinitely.
    await expect(page.getByLabel('Existing Worktree Path')).toBeEnabled({ timeout: 8000 });
    await expect(page.getByText(/Scanning for git worktrees/i)).not.toBeVisible();
  });
});
