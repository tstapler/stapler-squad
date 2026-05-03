// @feature session:create
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8543';

async function openInCreationMode(page: import('@playwright/test').Page) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
  await page.keyboard.press('Control+Shift+K');
  await expect(page.getByRole('radiogroup', { name: 'Session type' })).toBeVisible({ timeout: 5000 });
}

test.describe('new project session creation', () => {
  test('T-E2E-NP-001: New Project radio button is visible in creation panel', async ({ page }) => {
    await openInCreationMode(page);
    await expect(page.getByRole('radio', { name: 'New Project' })).toBeVisible();
  });

  test('T-E2E-NP-002: selecting New Project shows parent dir and project name fields', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'New Project' }).click();
    await expect(page.getByRole('radio', { name: 'New Project' })).toHaveAttribute('aria-checked', 'true');

    await expect(page.getByLabel('Parent Directory *')).toBeVisible();
    await expect(page.getByLabel('Project Name *')).toBeVisible();
  });

  test('T-E2E-NP-003: path preview updates as user types parent dir and project name', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'New Project' }).click();

    await page.getByLabel('Parent Directory *').fill('~/Projects');
    await page.getByLabel('Project Name *').fill('my-e2e-project');

    await expect(page.getByText('~/Projects/my-e2e-project')).toBeVisible({ timeout: 2000 });
  });

  test('T-E2E-NP-004: submit is disabled until both parent dir and project name are filled', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'New Project' }).click();
    await page.getByLabel('Session Name').fill('test-project');

    // Neither field filled — submit disabled
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeDisabled();

    // Only parent dir filled — still disabled
    await page.getByLabel('Parent Directory *').fill('~/Projects');
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeDisabled();

    // Both filled — enabled
    await page.getByLabel('Project Name *').fill('my-project');
    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 2000 });
  });

  test('T-E2E-NP-005: "Open as" radio group defaults to New Worktree', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'New Project' }).click();

    // The "Open as" radio group should show New Worktree selected by default
    const newWorktreeOption = page.getByRole('radio', { name: 'New Worktree' }).nth(1);
    await expect(newWorktreeOption).toHaveAttribute('aria-checked', 'true');
  });

  test('T-E2E-NP-006: switching "Open as" to Directory hides branch field', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'New Project' }).click();

    // Branch field visible by default (New Worktree is default open-as)
    await expect(page.getByLabel(/Git Branch/i)).toBeVisible({ timeout: 3000 });

    // Switch to Directory
    await page.getByRole('radio', { name: 'Directory' }).last().click();

    // Branch field hidden
    await expect(page.getByLabel(/Git Branch/i)).not.toBeVisible();
  });

  test('T-E2E-NP-007: valid new project form sends correct RPC payload', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'New Project' }).click();

    await page.getByLabel('Session Name').fill('e2e-new-project');
    await page.getByLabel('Parent Directory *').fill('/tmp/e2e-projects');
    await page.getByLabel('Project Name *').fill('test-repo');

    // Switch to Directory mode (simpler — no branch required)
    await page.getByRole('radio', { name: 'Directory' }).last().click();

    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });

    const requestPromise = page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST'
    );

    await page.getByRole('button', { name: 'Create Session' }).click();

    const request = await requestPromise;
    const body = request.postDataJSON();

    // sessionType 4 = SESSION_TYPE_NEW_PROJECT
    expect(body.sessionType).toBe(4);
    expect(body.path).toBe('/tmp/e2e-projects/test-repo');
    expect(body.oneOff).toBeFalsy();
  });

  test('T-E2E-NP-008: hides the path detection input when New Project is selected', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'New Project' }).click();

    // The main detection input (used for directory/worktree modes) should not be relevant
    // Working Directory field also hidden for new_project
    await expect(page.getByLabel('Working Directory')).not.toBeVisible();
  });
});

test.describe('directory mode path confirmation', () => {
  test('T-E2E-NP-009: shows confirmation dialog when directory path does not exist', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();

    // Fill in a path that definitely doesn't exist
    await page.locator('input[aria-label="Session source input"]').fill('/tmp/nonexistent-e2e-path-xyz');
    await page.getByLabel('Session Name').fill('e2e-dir-confirm');

    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });
    await page.getByRole('button', { name: 'Create Session' }).click();

    // Confirmation dialog must appear
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/doesn't exist|does not exist/i)).toBeVisible();
    await expect(page.getByRole('button', { name: /create|confirm/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /cancel/i })).toBeVisible();
  });

  test('T-E2E-NP-010: cancelling the confirmation dialog closes it without creating session', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();

    await page.locator('input[aria-label="Session source input"]').fill('/tmp/nonexistent-e2e-path-abc');
    await page.getByLabel('Session Name').fill('e2e-dir-cancel');

    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });
    await page.getByRole('button', { name: 'Create Session' }).click();
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5000 });

    await page.getByRole('button', { name: /cancel/i }).click();
    await expect(page.getByRole('dialog')).not.toBeVisible({ timeout: 3000 });

    // Creation panel should still be open (user can correct the path)
    await expect(page.getByRole('radiogroup', { name: 'Session type' })).toBeVisible();
  });

  test('T-E2E-NP-011: confirming sends request with createIfMissing=true', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();

    await page.locator('input[aria-label="Session source input"]').fill('/tmp/nonexistent-e2e-path-def');
    await page.getByLabel('Session Name').fill('e2e-dir-confirm-create');

    await expect(page.getByRole('button', { name: 'Create Session' })).toBeEnabled({ timeout: 3000 });

    // Intercept the retried request (after confirmation)
    const requestPromise = page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST',
      { timeout: 10000 }
    );

    await page.getByRole('button', { name: 'Create Session' }).click();
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5000 });
    await page.getByRole('button', { name: /create|confirm/i }).click();

    const request = await requestPromise;
    const body = request.postDataJSON();
    expect(body.createIfMissing).toBe(true);
  });
});
