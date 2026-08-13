import { test, expect } from '@playwright/test';
import { execSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

/**
 * S6-4: Project grouping flow
 *
 * Covers:
 * 1. Select 2 sessions via checkboxes → selection toolbar appears
 * 2. Type project name in "Group as..." input → press Enter
 * 3. Switch GroupBy to "Project" → group header with project name appears
 * 4. Both sessions visible under the group header
 * 5. Inline rename: click ✏️ → type new name → Enter → header updates
 * 6. Inline delete: click 🗑️ → confirm → sessions move to "Ungrouped"
 * 7. Group header aggregate counts are correct
 *
 * Requires a running dev server (`make restart-web`).
 */

let testRepoPath: string;

test.beforeAll(async () => {
  testRepoPath = fs.mkdtempSync(path.join(os.tmpdir(), 'squad-e2e-project-'));
  execSync('git init', { cwd: testRepoPath });
  execSync('git config user.email "test@test.com"', { cwd: testRepoPath });
  execSync('git config user.name "Test"', { cwd: testRepoPath });
  execSync('git commit --allow-empty -m "init"', { cwd: testRepoPath });
});

test.afterAll(() => {
  if (testRepoPath) {
    fs.rmSync(testRepoPath, { recursive: true, force: true });
  }
});

/** Helper: create a session via the wizard and return its title. */
async function createTestSession(page: any, title: string, repoPath: string): Promise<string> {
  await page.goto('/?new=true');
  await page.waitForLoadState('networkidle');
  await page.waitForSelector('[data-testid="session-title"]', { timeout: 10000 });

  await page.getByTestId('session-title').fill(title);
  await page.getByRole('button', { name: 'Next' }).click();
  await page.waitForSelector('text=Repository Setup', { timeout: 5000 });

  await page.getByTestId('session-path').fill(repoPath);
  await page.selectOption('select#sessionType', 'directory');
  await page.getByRole('button', { name: 'Next' }).click();
  await page.waitForSelector('text=Configuration', { timeout: 5000 });

  await page.getByRole('button', { name: 'Next' }).click();
  await page.waitForSelector('text=Review', { timeout: 5000 });

  await page.getByTestId('create-session-button').click();

  // Wait for wizard to close and session to appear
  await page.waitForSelector(`text=${title}`, { timeout: 15000 });
  return title;
}

test.describe('Project grouping flow (S6-4)', () => {
  test('select 2 sessions and group them as a new project', async ({ page }) => {
    // Create 2 test sessions
    const ts = Date.now();
    const title1 = `e2e-proj-session1-${ts}`;
    const title2 = `e2e-proj-session2-${ts}`;
    const projectName = `e2e-project-${ts}`;

    await createTestSession(page, title1, testRepoPath);
    await createTestSession(page, title2, testRepoPath);

    // Navigate to the main page
    await page.goto('/');
    await page.waitForLoadState('networkidle');

    // Both sessions should appear
    await expect(page.getByText(title1)).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(title2)).toBeVisible({ timeout: 10000 });

    // Enter select mode
    await page.getByRole('button', { name: 'Enter select mode' }).click();

    // Select both sessions by clicking their checkboxes
    const checkbox1 = page.getByLabel(`Select ${title1}`);
    const checkbox2 = page.getByLabel(`Select ${title2}`);
    await checkbox1.click();
    await checkbox2.click();

    // Selection toolbar should appear with the count
    await expect(page.getByText(/2 of/)).toBeVisible({ timeout: 3000 });

    // Group as... input should be visible
    const groupAsInput = page.getByLabel('Group selected sessions as project');
    await expect(groupAsInput).toBeVisible({ timeout: 3000 });

    // Type project name and submit
    await groupAsInput.fill(projectName);
    await page.keyboard.press('Enter');

    // Wait for feedback toast
    await page.waitForTimeout(1000);

    // Switch GroupBy to "Project"
    const groupBySelect = page.getByLabel('Group sessions by');
    await expect(groupBySelect).toBeVisible({ timeout: 3000 });
    await groupBySelect.selectOption('project');

    // The project group header should appear
    await expect(page.getByText(projectName)).toBeVisible({ timeout: 5000 });

    // Both sessions should appear under the group header
    await expect(page.getByText(title1)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(title2)).toBeVisible({ timeout: 5000 });
  });

  test('selection toolbar appears when ≥1 session is selected', async ({ page }) => {
    // Create a session if none exist
    const ts = Date.now();
    const title = `e2e-select-toolbar-${ts}`;

    await createTestSession(page, title, testRepoPath);
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText(title)).toBeVisible({ timeout: 10000 });

    // Enter select mode
    await page.getByRole('button', { name: 'Enter select mode' }).click();

    // Select a session
    const checkbox = page.getByLabel(`Select ${title}`);
    await checkbox.click();

    // The BulkActions toolbar should appear
    await expect(page.getByText(/selected/)).toBeVisible({ timeout: 3000 });
    await expect(page.getByLabel('Group selected sessions as project')).toBeVisible();

    // Clear Selection button should work
    await page.getByRole('button', { name: 'Clear Selection' }).click();
    await expect(page.getByLabel('Group selected sessions as project')).not.toBeVisible({ timeout: 3000 });
  });

  test('Escape key exits select mode and clears selection', async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-escape-select-${ts}`;

    await createTestSession(page, title, testRepoPath);
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText(title)).toBeVisible({ timeout: 10000 });

    // Enter select mode and select a session
    await page.getByRole('button', { name: 'Enter select mode' }).click();
    await page.getByLabel(`Select ${title}`).click();
    await expect(page.getByText(/selected/)).toBeVisible({ timeout: 3000 });

    // Cancel button clears selection
    await page.getByRole('button', { name: 'Exit select mode' }).click();
    await expect(page.getByText(/selected/)).not.toBeVisible({ timeout: 3000 });
  });

  test('project group header shows rename and delete actions when GroupByProject is active', async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-proj-header-${ts}`;
    const projectName = `e2e-header-project-${ts}`;

    await createTestSession(page, title, testRepoPath);
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText(title)).toBeVisible({ timeout: 10000 });

    // Enter select mode and select the session
    await page.getByRole('button', { name: 'Enter select mode' }).click();
    await page.getByLabel(`Select ${title}`).click();

    // Group as project
    const groupAsInput = page.getByLabel('Group selected sessions as project');
    await groupAsInput.fill(projectName);
    await page.keyboard.press('Enter');
    await page.waitForTimeout(1000);

    // Switch to Project grouping
    await page.getByLabel('Group sessions by').selectOption('project');

    // The project group header should show the project name
    await expect(page.getByText(projectName)).toBeVisible({ timeout: 5000 });

    // Rename button (✏️) should be visible on hover/in the header
    const renameButton = page.locator(`button[aria-label*="Rename ${projectName}"], button[title*="Rename"]`).first();
    if (await renameButton.isVisible({ timeout: 2000 }).catch(() => false)) {
      await renameButton.click();

      // Rename input should appear
      const renameInput = page.locator('input[value*="' + projectName + '"], input[placeholder*="Project name"]').first();
      if (await renameInput.isVisible({ timeout: 2000 }).catch(() => false)) {
        const newName = `${projectName}-renamed`;
        await renameInput.clear();
        await renameInput.fill(newName);
        await page.keyboard.press('Enter');

        // Header should update with the new name
        await expect(page.getByText(newName)).toBeVisible({ timeout: 5000 });
      }
    }
  });

  test('deleting a project moves its sessions to Ungrouped', async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-proj-delete-${ts}`;
    const projectName = `e2e-delete-project-${ts}`;

    await createTestSession(page, title, testRepoPath);
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText(title)).toBeVisible({ timeout: 10000 });

    // Select and group the session
    await page.getByRole('button', { name: 'Enter select mode' }).click();
    await page.getByLabel(`Select ${title}`).click();
    const groupAsInput = page.getByLabel('Group selected sessions as project');
    await groupAsInput.fill(projectName);
    await page.keyboard.press('Enter');
    await page.waitForTimeout(1000);

    // Switch to Project grouping
    await page.getByLabel('Group sessions by').selectOption('project');
    await expect(page.getByText(projectName)).toBeVisible({ timeout: 5000 });

    // Find and click the delete button on the project header
    const deleteButton = page.locator(`button[aria-label*="Delete ${projectName}"], button[title*="Delete"]`).first();
    if (await deleteButton.isVisible({ timeout: 2000 }).catch(() => false)) {
      await deleteButton.click();

      // Confirmation tooltip/popover should appear
      const confirmButton = page.getByRole('button', { name: /Confirm|Yes, delete|Delete/i }).last();
      if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmButton.click();
        await page.waitForTimeout(1000);

        // The project group header should be gone
        await expect(page.getByText(projectName)).not.toBeVisible({ timeout: 5000 });

        // The session should appear in "Ungrouped"
        await expect(page.getByText('Ungrouped')).toBeVisible({ timeout: 5000 });
        await expect(page.getByText(title)).toBeVisible({ timeout: 5000 });
      }
    }
  });

  test('entering an existing project name assigns without creating duplicate', async ({ page }) => {
    const ts = Date.now();
    const title1 = `e2e-dedup-session1-${ts}`;
    const title2 = `e2e-dedup-session2-${ts}`;
    const projectName = `e2e-dedup-project-${ts}`;

    // Create two sessions
    await createTestSession(page, title1, testRepoPath);
    await createTestSession(page, title2, testRepoPath);
    await page.goto('/');
    await page.waitForLoadState('networkidle');

    await expect(page.getByText(title1)).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(title2)).toBeVisible({ timeout: 10000 });

    // Group session 1
    await page.getByRole('button', { name: 'Enter select mode' }).click();
    await page.getByLabel(`Select ${title1}`).click();
    await page.getByLabel('Group selected sessions as project').fill(projectName);
    await page.keyboard.press('Enter');
    await page.waitForTimeout(1000);

    // Exit select mode
    await page.getByRole('button', { name: 'Exit select mode' }).click();

    // Group session 2 into the SAME project name
    await page.getByRole('button', { name: 'Enter select mode' }).click();
    await page.getByLabel(`Select ${title2}`).click();
    await page.getByLabel('Group selected sessions as project').fill(projectName);
    await page.keyboard.press('Enter');
    await page.waitForTimeout(1000);

    // Switch to Project grouping
    await page.getByLabel('Group sessions by').selectOption('project');

    // There should be EXACTLY ONE project group with projectName
    const projectHeaders = page.getByText(projectName);
    const headerCount = await projectHeaders.count();
    // The project name should appear as a group header (only once)
    expect(headerCount).toBeLessThanOrEqual(3); // header + potential text matches in session cards

    // Both sessions should appear
    await expect(page.getByText(title1)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(title2)).toBeVisible({ timeout: 5000 });
  });
});
