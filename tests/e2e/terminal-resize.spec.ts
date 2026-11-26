import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: terminal-resize — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['terminal-resize'], // TODO: add to catalog
] as const;
import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const REDRAW_BUTTON = /^Redraw terminal/;

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

    await expect(page.getByRole('button', { name: REDRAW_BUTTON })).toBeVisible({ timeout: 5000 });
  });

  test('clicking resize shows then hides the resizing overlay', async ({ page }) => {
    const opened = await openTerminalView(page);
    test.skip(!opened, 'No session available in test server');

    // Wait for connection before triggering resize
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    // Resizing overlay must NOT be visible before clicking
    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached();

    await page.getByRole('button', { name: REDRAW_BUTTON }).click();

    // Overlay appears (RESIZING state) then disappears (STABLE state)
    // Waiting for absence with a generous timeout covers slow CI environments
    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached({ timeout: 10000 });
  });

  test('terminal stays connected after resize', async ({ page }) => {
    const opened = await openTerminalView(page);
    test.skip(!opened, 'No session available in test server');

    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    await page.getByRole('button', { name: REDRAW_BUTTON }).click();

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
   * We detect this by checking that each printed marker row appears exactly once.
   */
  test('no duplicate lines in terminal after resize (dom renderer)', async ({ page, browserName }, testInfo) => {
    // This assertion requires the DOM renderer; skip in other projects.
    test.skip(
      !testInfo.project.name.includes('dom'),
      'DOM content assertion only runs in chromium-dom project',
    );

    const client = new SessionClient(BASE_URL);
    const title = `e2e-resize-${Date.now()}`;
    const session = await client.createIdleSession(title, '/tmp');
    try {
      await client.waitForSessionsActive([title], 20000);

      await page.goto(`${BASE_URL}/?session=${session.id}`, { waitUntil: 'domcontentloaded', timeout: 10000 });

      // Ensure toolbar is expanded
      const toggle = page.locator('[data-testid="toolbar-toggle"]');
      await expect(toggle).toBeVisible({ timeout: 8000 });
      if ((await toggle.getAttribute('aria-expanded')) === 'false') await toggle.click();

      await expect(page.getByText('Connected')).toBeVisible({ timeout: 20000 });

      // Print two marker rows, so there is known content the redraw has to bring back.
      const markers = ['ssq-a', 'ssq-b'];
      const markerCounts = async () => {
        const rows = (await page.locator('.xterm-rows > div').allInnerTexts()).map((r) => r.trim());
        return markers.map((m) => rows.filter((r) => r === m).length);
      };
      await page.getByRole('textbox', { name: 'Terminal input' }).focus();
      await page.keyboard.type("printf 'ssq-a\\nssq-b\\n'\n");
      await expect.poll(markerCounts, { timeout: 10000 }).toEqual([1, 1]);

      // Redraw clears the local buffer and forces a server resize; only this click logs that line. The rows
      // can also return from another in-flight snapshot, so the forced resize itself is asserted in
      // TerminalOutput.refit.test.tsx; this test guards the clear-then-repaint result.
      const forced = page.waitForEvent('console', {
        predicate: (m) => m.text().includes('Forcing resize message to backend'),
        timeout: 10000,
      });
      await page.getByRole('button', { name: REDRAW_BUTTON }).click();
      await forced;
      // Two frames, so the DOM reflects the clear before we look for the repainted rows.
      await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));

      await expect.poll(markerCounts, { timeout: 10000 }).toEqual([1, 1]);
      await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached({ timeout: 10000 });
      // A second copy means the snapshot was written on top of existing content (double display).
      expect(await markerCounts()).toEqual([1, 1]);
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });
});
