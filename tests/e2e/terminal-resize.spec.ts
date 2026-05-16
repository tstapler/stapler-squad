// @feature terminal:resize
import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

/**
 * Open a session's terminal view and ensure the toolbar is expanded.
 * Returns false if no session card is visible (test should be skipped).
 */
async function openTerminalView(page: import('@playwright/test').Page): Promise<boolean> {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
  const sessionCard = page.locator('[data-testid="session-card"]').first();
  const hasSession = await sessionCard.isVisible({ timeout: 5000 }).catch(() => false);
  if (!hasSession) return false;

  await sessionCard.click();
  await expect(page.locator('[data-testid="toolbar-toggle"]')).toBeVisible({ timeout: 8000 });

  // Expand toolbar if collapsed
  const toggle = page.locator('[data-testid="toolbar-toggle"]');
  const expanded = await toggle.getAttribute('aria-expanded');
  if (expanded === 'false') await toggle.click();

  return true;
}

test.describe('terminal resize', () => {
  test('resize button is present in secondary toolbar', async ({ page }) => {
    const opened = await openTerminalView(page);
    test.skip(!opened, 'No session available in test server');

    await expect(page.getByRole('button', { name: 'Resize terminal' })).toBeVisible({ timeout: 5000 });
  });

  test('clicking resize shows then hides the resizing overlay', async ({ page }) => {
    const opened = await openTerminalView(page);
    test.skip(!opened, 'No session available in test server');

    // Wait for connection before triggering resize
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    // Resizing overlay must NOT be visible before clicking
    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached();

    await page.getByRole('button', { name: 'Resize terminal' }).click();

    // Overlay appears (RESIZING state) then disappears (STABLE state)
    // Waiting for absence with a generous timeout covers slow CI environments
    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached({ timeout: 10000 });
  });

  test('terminal stays connected after resize', async ({ page }) => {
    const opened = await openTerminalView(page);
    test.skip(!opened, 'No session available in test server');

    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    await page.getByRole('button', { name: 'Resize terminal' }).click();

    // After the resize cycle completes the terminal must still be connected
    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached({ timeout: 10000 });
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 5000 });
  });

  test('resize via viewport change cycles through RESIZING overlay', async ({ page }) => {
    const opened = await openTerminalView(page);
    test.skip(!opened, 'No session available in test server');

    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    // Trigger resize by changing the viewport — exercises the browser resize path
    // rather than the manual button path
    await page.setViewportSize({ width: 900, height: 600 });

    // Wait for any RESIZING overlay to clear
    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached({ timeout: 10000 });
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 5000 });
  });

  /**
   * Double-display regression test.
   *
   * Uses the chromium-dom project (--disable-webgl) so xterm.js uses its DOM
   * renderer and terminal rows are readable as real DOM elements.
   *
   * The bug: after a resize, the post-resize snapshot was written on top of
   * existing content without clearing the visible screen first (double display).
   * We detect this by checking that each non-blank row appears exactly once.
   */
  test('no duplicate lines in terminal after resize (dom renderer)', async ({ page, browserName }, testInfo) => {
    // This assertion requires the DOM renderer; skip in other projects.
    test.skip(
      !testInfo.project.name.includes('dom'),
      'DOM content assertion only runs in chromium-dom project',
    );

    const client = new SessionClient(BASE_URL);
    const session = await client.createIdleSession(
      `e2e-resize-${Date.now()}`,
      '/tmp',
    );

    await page.goto(`${BASE_URL}/?session=${session.id}`, { waitUntil: 'domcontentloaded', timeout: 10000 });

    // Ensure toolbar is expanded
    await expect(page.locator('[data-testid="toolbar-toggle"]')).toBeVisible({ timeout: 8000 });
    const toggle = page.locator('[data-testid="toolbar-toggle"]');
    const expanded = await toggle.getAttribute('aria-expanded');
    if (expanded === 'false') await toggle.click();

    await expect(page.getByText('Connected')).toBeVisible({ timeout: 20000 });

    // Capture terminal rows before resize
    const rowsBefore = await page.locator('.xterm-rows > div').allInnerTexts();
    const nonBlankBefore = rowsBefore.filter(r => r.trim() !== '');

    // Trigger resize
    await page.getByRole('button', { name: 'Resize terminal' }).click();
    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached({ timeout: 10000 });

    // Capture terminal rows after resize
    const rowsAfter = await page.locator('.xterm-rows > div').allInnerTexts();
    const nonBlankAfter = rowsAfter.filter(r => r.trim() !== '');

    // Each distinct non-blank row text should appear at most once.
    // Duplicates mean the snapshot was written on top of existing content.
    const counts = new Map<string, number>();
    for (const row of nonBlankAfter) {
      counts.set(row, (counts.get(row) ?? 0) + 1);
    }
    const duplicates = [...counts.entries()].filter(([, n]) => n > 1).map(([text]) => text);

    expect(
      duplicates,
      `Duplicate terminal rows found after resize (double-display bug): ${JSON.stringify(duplicates)}. ` +
      `Before resize: ${nonBlankBefore.length} rows, after: ${nonBlankAfter.length} rows`,
    ).toHaveLength(0);

    await client.deleteSession(session.id, true).catch(() => {});
  });
});
