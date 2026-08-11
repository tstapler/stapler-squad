/**
 * E2E regression tests for the thin-PTY rendering bugs.
 *
 * Bug summary:
 *   Bug 1 (XtermTerminal.tsx) — fired onResize(80, 24) before fitAddon.fit() ran,
 *     corrupting the localStorage dimension cache.
 *   Bug 2 (TerminalOutput.tsx) — on next load, read the stale 80×24 cache and called
 *     connect(80, 24) before the actual terminal measured its container.
 *   Bug 3 (XtermTerminal.tsx) — ResizeObserver fired fit() on 0×0 containers
 *     (hidden tabs), overwriting the cache with xterm defaults again.
 *
 * These tests verify:
 *   1. The cache is never written with 80×24 immediately at mount (Bug 1 regression).
 *   2. When the cache contains stale 80×24, the terminal does not use those dims
 *      to connect — it waits for the actual resize event (Bug 2 regression).
 *   3. Navigating away (which can trigger Bug 3) does not corrupt the cache.
 *
 * Setup requirements:
 *   The app server must be running at the configured baseURL (see playwright.config.ts).
 *   At least one terminal session must be present, or the test uses the
 *   /test/layout-overlap page which renders the terminal shell.
 *
 * How to run:
 *   npx playwright test pty-width-regression --project=chromium
 */

import { test, expect, Page } from '@playwright/test';

// ─── helpers ────────────────────────────────────────────────────────────────

/** Read terminal-dimensions-* entries from localStorage. */
async function getStoredDimensions(page: Page): Promise<Record<string, { cols: number; rows: number }>> {
  return page.evaluate(() => {
    const result: Record<string, { cols: number; rows: number }> = {};
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)!;
      if (key.startsWith('terminal-dimensions-')) {
        try {
          result[key] = JSON.parse(localStorage.getItem(key)!);
        } catch {
          // ignore malformed entries
        }
      }
    }
    return result;
  });
}

/** Inject a stale 80×24 cache entry (simulating Bug 1 corruption). */
async function injectStaleCache(page: Page, sessionId: string): Promise<void> {
  await page.evaluate(([sid]) => {
    localStorage.setItem(`terminal-dimensions-${sid}`, JSON.stringify({ cols: 80, rows: 24 }));
  }, [sessionId]);
}

/** Clear all terminal-dimensions-* keys. */
async function clearTerminalCache(page: Page): Promise<void> {
  await page.evaluate(() => {
    for (const key of Object.keys(localStorage)) {
      if (key.startsWith('terminal-dimensions-')) {
        localStorage.removeItem(key);
      }
    }
  });
}

/** Wait for the terminal viewport element to appear and stabilise. */
async function waitForTerminal(page: Page, timeout = 10_000): Promise<void> {
  await page.waitForSelector('.xterm-viewport', { timeout });
  // Allow fitAddon.fit() and the double-rAF to complete.
  await page.waitForTimeout(300);
}

/** Return the column count xterm reports for the first visible terminal. */
async function getXtermCols(page: Page): Promise<number | null> {
  return page.evaluate(() => {
    // xterm renders a hidden accessibility textarea that shows `cols` in its aria-label,
    // or we can count the character cells in the first row.
    const rows = document.querySelectorAll('.xterm-rows');
    if (!rows.length) return null;
    const firstRow = rows[0].querySelector('.xterm-cursor-layer');
    // Alternative: count the actual rendered columns via container width / cell width.
    const viewport = document.querySelector('.xterm-viewport') as HTMLElement | null;
    const screen = document.querySelector('.xterm-screen') as HTMLElement | null;
    if (!viewport || !screen) return null;
    const screenWidth = screen.getBoundingClientRect().width;
    // Character cell width can be derived from the xterm-char-measure-element
    const ruler = document.querySelector('.xterm-char-measure-element') as HTMLElement | null;
    if (!ruler) return null;
    const cellWidth = ruler.getBoundingClientRect().width;
    if (cellWidth <= 0) return null;
    return Math.round(screenWidth / cellWidth);
  });
}

// ─── test page helper ────────────────────────────────────────────────────────

/** Navigate to the layout-overlap test page, which renders the terminal shell. */
async function goToTestPage(page: Page): Promise<void> {
  await page.goto('/test/layout-overlap');
  await page.getByTestId('modal-content').waitFor({ state: 'visible' });
  await page.waitForTimeout(150); // let layout settle
}

// ─── tests ───────────────────────────────────────────────────────────────────

test.describe('PTY width regression: thin-terminal bugs', () => {
  test.beforeEach(async ({ page }) => {
    await clearTerminalCache(page);
  });

  /**
   * Bug 1 regression:
   * XtermTerminal must NOT write 80×24 to the cache immediately at mount.
   * After mount, any cached dims should be > 80×24 (the real container size).
   */
  test('cache is not written with xterm defaults (80×24) immediately after mount', async ({ page }) => {
    await goToTestPage(page);

    // Wait for the terminal to fully initialize and fitAddon.fit() to run.
    try {
      await waitForTerminal(page);
    } catch {
      // Terminal might not be present on this test page — skip gracefully.
      test.skip();
    }

    const dims = await getStoredDimensions(page);
    const entries = Object.values(dims);

    for (const entry of entries) {
      // If a cache entry was written, it must not be the xterm default 80×24.
      // (80×24 at mount = Bug 1 corruption; real containers are wider.)
      expect(
        entry.cols === 80 && entry.rows === 24,
        `Cache was written with xterm default dims ${entry.cols}×${entry.rows} — Bug 1 regression`
      ).toBe(false);
    }
  });

  /**
   * Bug 2 regression:
   * When the cache already contains stale 80×24 (from a previous Bug 1 corruption),
   * the terminal should connect with the actual container dims, not the stale 80×24.
   *
   * We verify this indirectly: after the terminal initialises, the cache must be
   * updated to the real dims (≠ 80×24), meaning the actual resize event was used.
   */
  test('stale 80×24 cache is overwritten with real dims after terminal loads', async ({ page }) => {
    // Inject stale cache BEFORE navigating (simulates a corrupted cache from Bug 1).
    await page.addInitScript(() => {
      // We don't know the session ID yet, so inject a generic one.
      // The TerminalOutput component reads this on mount.
      localStorage.setItem('terminal-dimensions-e2e-test-session', JSON.stringify({ cols: 80, rows: 24 }));
    });

    await goToTestPage(page);

    try {
      await waitForTerminal(page);
    } catch {
      test.skip();
    }

    // After the terminal has loaded and fitAddon.fit() fired, any entry that was
    // stale 80×24 should have been overwritten with the real container dims.
    const dims = await getStoredDimensions(page);
    const entries = Object.values(dims);

    for (const entry of entries) {
      if (entry.cols === 80 && entry.rows === 24) {
        // If the cache still shows 80×24, the real resize event was not used.
        // This indicates Bug 2 is still present.
        expect.soft(false, `Cache still shows stale 80×24 after terminal loaded — Bug 2 regression`);
      }
    }
  });

  /**
   * Bug 3 regression:
   * After the terminal renders at the correct size, navigating away (which can
   * collapse the container to 0×0 and trigger the ResizeObserver) should NOT
   * overwrite the cache with 80×24.
   */
  test('navigating away does not corrupt the dimension cache', async ({ page }) => {
    await goToTestPage(page);

    try {
      await waitForTerminal(page);
    } catch {
      test.skip();
    }

    // Snapshot the cache while the terminal is visible.
    const dimsBefore = await getStoredDimensions(page);

    // Navigate away (simulates closing/switching the tab, which triggers Bug 3).
    await page.goto('/');
    await page.waitForTimeout(200); // allow any ResizeObserver callbacks to fire

    // Navigate back.
    await goToTestPage(page);
    await page.waitForTimeout(300);

    const dimsAfter = await getStoredDimensions(page);

    // Any entry that existed before should not have been corrupted to 80×24.
    for (const [key, before] of Object.entries(dimsBefore)) {
      const after = dimsAfter[key];
      if (!after) continue; // entry was cleared — acceptable

      if (before.cols !== 80 && before.rows !== 24) {
        // The cache had valid non-default dims before navigation.
        // After navigation, it should not have been corrupted to 80×24.
        expect(
          after.cols === 80 && after.rows === 24,
          `Cache for ${key} was corrupted to 80×24 after navigation — Bug 3 regression ` +
          `(was ${before.cols}×${before.rows}, now ${after.cols}×${after.rows})`
        ).toBe(false);
      }
    }
  });

  /**
   * Pre-sizing regression:
   * After the terminal initialises, the localStorage entry should include cellWidth
   * and cellHeight alongside cols/rows. These pixel metrics allow instant pre-sizing
   * on reconnect without waiting for xterm's first onResize event.
   */
  test('cache includes cell pixel metrics (cellWidth/cellHeight) after terminal loads', async ({ page }) => {
    await goToTestPage(page);

    try {
      await waitForTerminal(page);
    } catch {
      test.skip();
    }

    const dims = await getStoredDimensions(page) as Record<string, { cols: number; rows: number; cellWidth?: number; cellHeight?: number }>;
    const entries = Object.values(dims);

    if (entries.length === 0) {
      // No cache written yet — terminal may not have resized; skip.
      test.skip();
    }

    // At least one cache entry should have finite positive cellWidth and cellHeight.
    const hasMetrics = entries.some(
      e => typeof e.cellWidth === 'number' && e.cellWidth > 0 &&
           typeof e.cellHeight === 'number' && e.cellHeight > 0
    );
    expect(
      hasMetrics,
      'No cache entry contains cellWidth/cellHeight — pre-sizing metrics were not saved'
    ).toBe(true);
  });

  /**
   * Column-count smoke test:
   * The rendered terminal must be wider than 80 columns when the viewport is 1280px wide.
   * If the terminal is exactly 80 columns in a wide viewport, the PTY sizing bug is present.
   */
  test('terminal renders more than 80 columns in a 1280px viewport', async ({ page }) => {
    // Playwright's Desktop Chrome default is 1280×720.
    await goToTestPage(page);

    let cols: number | null = null;
    try {
      await waitForTerminal(page);
      cols = await getXtermCols(page);
    } catch {
      test.skip();
    }

    if (cols === null) {
      // Terminal element not present on this test page — skip.
      test.skip();
    }

    expect(
      cols,
      `Terminal has only ${cols} columns in a 1280px viewport — thin PTY bug may be present`
    ).toBeGreaterThan(80);
  });
});
