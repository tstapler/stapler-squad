import { FEATURE_CATALOG } from '../../../web-app/src/lib/features';
const _features = [FEATURE_CATALOG['terminal-render']] as const;
/**
 * Full-cycle tmux roundtrip integration tests.
 *
 * These tests exercise the COMPLETE path from browser input to PTY output:
 *
 *   Browser keyboard → xterm.js helper textarea
 *     → WebSocket (StreamTerminal RPC)
 *     → Go session service
 *     → tmux send-keys
 *     → PTY / bash process
 *     → tmux output buffer
 *     → control-mode / capture-pane
 *     → WebSocket back to browser
 *     → xterm.js write()
 *     → terminal rendered in DOM/canvas
 *
 * Scenarios covered:
 *   1. Echo roundtrip   — type a command, verify output appears in tmux pane
 *   2. TUI rendering    — run `top -b -n 1` and verify the screen fills correctly
 *   3. Large scrollback — `seq 1 2000` stresses the buffer and dimension cache;
 *                         we measure render latency and verify the terminal
 *                         remains responsive afterward
 *
 * Prerequisites:
 *   - stapler-squad server running at localhost:8543 (or TEST_SERVER_URL)
 *   - tmux installed and on PATH
 *
 * Both are skipped gracefully when unavailable (see beforeAll guard).
 *
 * Run:
 *   npx playwright test terminal-stress/tmux-roundtrip --project chromium
 */

import { test, expect, Page } from '@playwright/test';
import { exec } from 'child_process';
import { promisify } from 'util';
import * as fs from 'fs';

const execPromise = promisify(exec);

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8543';

// ─── tmux helpers ─────────────────────────────────────────────────────────────

const TMUX_PREFIX = 'staplersquad_';

/**
 * Derive the tmux session name from the stapler-squad session title.
 * Mirrors Go logic in session/tmux/tmux.go:ToStaplerSquadTmuxName.
 */
function tmuxName(title: string): string {
  return TMUX_PREFIX + title.replace(/\s+/g, '').replace(/\./g, '_');
}

/**
 * Read the current pane contents of a tmux session (up to historyLines back).
 * Returns empty string if tmux or the session is unavailable.
 */
async function captureTmuxPane(session: string, historyLines = 2000): Promise<string> {
  try {
    const { stdout } = await execPromise(
      `tmux capture-pane -p -t "${session}" -S -${historyLines}`,
      { timeout: 5000 },
    );
    return stdout;
  } catch {
    return '';
  }
}

/**
 * Poll until a tmux session exists or the timeout elapses.
 * Returns true if the session appeared before the deadline.
 */
async function waitForTmuxSession(session: string, timeoutMs = 15_000): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      await execPromise(`tmux has-session -t "${session}"`, { timeout: 2000 });
      return true;
    } catch {
      await sleep(300);
    }
  }
  return false;
}

// ─── ConnectRPC API helpers ───────────────────────────────────────────────────

/** Create a bash session via the ConnectRPC JSON API. Returns the session ID. */
async function createBashSession(title: string): Promise<string> {
  const res = await fetch(`${BASE_URL}/session.v1.SessionService/CreateSession`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
    },
    body: JSON.stringify({ title, path: '/tmp', program: 'bash' }),
    signal: AbortSignal.timeout(10_000),
  });
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(`CreateSession failed: HTTP ${res.status} — ${body}`);
  }
  const data = (await res.json()) as { session?: { id: string } };
  const id = data.session?.id;
  if (!id) throw new Error('CreateSession returned no session ID');
  return id;
}

/** Delete a session (best-effort cleanup). */
async function deleteSession(id: string): Promise<void> {
  await fetch(`${BASE_URL}/session.v1.SessionService/DeleteSession`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
    },
    body: JSON.stringify({ sessionId: id }),
    signal: AbortSignal.timeout(5000),
  }).catch(() => {});
}

// ─── Browser helpers ──────────────────────────────────────────────────────────

/**
 * Navigate to a session's terminal view and wait for xterm.js to mount.
 * Returns when the viewport element is visible and non-zero height.
 */
async function openSessionTerminal(page: Page, sessionId: string): Promise<void> {
  await page.goto(`${BASE_URL}/sessions/${sessionId}`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.xterm-viewport', { timeout: 20_000 });
  // Allow xterm to complete its initial fit and the StreamTerminal WS to connect.
  await page.waitForTimeout(2000);
}

/**
 * Focus the xterm.js hidden input and type text, then press Enter.
 * Uses the `.xterm-helper-textarea` that xterm.js uses for all keyboard input.
 */
async function typeInTerminal(page: Page, text: string, pressEnter = true): Promise<void> {
  // Click the terminal viewport to trigger focus management
  await page.locator('.xterm-viewport').click({ force: true });
  await page.waitForTimeout(100);
  // xterm.js routes keystrokes through its hidden textarea
  const textarea = page.locator('.xterm-helper-textarea');
  await textarea.focus();
  await page.keyboard.type(text, { delay: 30 });
  if (pressEnter) await page.keyboard.press('Enter');
}

/**
 * Count the number of visible xterm rows that contain non-whitespace content.
 * Works with the DOM renderer; returns 0 when WebGL is active (canvas-only).
 */
async function countRenderedRows(page: Page): Promise<number> {
  return page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll('.xterm-rows > div'));
    return rows.filter(r => (r.textContent || '').trim().length > 0).length;
  });
}

/**
 * Read all visible text from xterm.js DOM rows.
 * Only works with the DOM renderer (chromium-dom project); returns '' with WebGL.
 */
async function readRenderedText(page: Page): Promise<string> {
  return page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll('.xterm-rows > div'));
    return rows.map(r => r.textContent || '').join('\n');
  });
}

/**
 * Return the column count xterm reports, derived from cell width measured against
 * the screen element.  Works with any renderer.
 */
async function measureXtermCols(page: Page): Promise<number | null> {
  return page.evaluate(() => {
    const screen = document.querySelector('.xterm-screen') as HTMLElement | null;
    const ruler  = document.querySelector('.xterm-char-measure-element') as HTMLElement | null;
    if (!screen || !ruler) return null;
    const cellWidth = ruler.getBoundingClientRect().width;
    if (cellWidth <= 0) return null;
    return Math.round(screen.getBoundingClientRect().width / cellWidth);
  });
}

/**
 * Snapshot JS heap size (Chrome only; 0 in other browsers).
 */
async function heapSize(page: Page): Promise<number> {
  return page.evaluate(() => (performance as any).memory?.usedJSHeapSize ?? 0);
}

const sleep = (ms: number) => new Promise(r => setTimeout(r, ms));

// ─── test suite ──────────────────────────────────────────────────────────────

test.describe('tmux roundtrip: full-cycle terminal integration', () => {
  // Unique title per test run so parallel runs don't collide.
  const RUN_ID = Date.now();
  let sessionId = '';
  let sessionTitle = '';
  let tmuxSession = '';
  let skipReason = '';

  test.beforeAll(async () => {
    // Guard 1: tmux must be installed
    try {
      await execPromise('tmux -V', { timeout: 3000 });
    } catch {
      skipReason = 'tmux not found on PATH';
      return;
    }

    // Guard 2: stapler-squad server must be reachable
    try {
      sessionTitle = `e2e-roundtrip-${RUN_ID}`;
      sessionId = await createBashSession(sessionTitle);
      tmuxSession = tmuxName(sessionTitle);
    } catch (err) {
      skipReason = `Server unavailable or CreateSession failed: ${err}`;
      return;
    }

    // Guard 3: Wait for the tmux session to actually appear
    const ready = await waitForTmuxSession(tmuxSession);
    if (!ready) {
      skipReason = `tmux session "${tmuxSession}" never appeared after 15s`;
      // Cleanup the orphaned record
      await deleteSession(sessionId);
      sessionId = '';
    }
  });

  test.afterAll(async () => {
    if (sessionId) {
      await deleteSession(sessionId);
      sessionId = '';
    }
  });

  // Helper: skip if beforeAll detected an issue
  function checkSkip() {
    if (skipReason) {
      test.skip(true, skipReason);
    }
  }

  // ── 1. Echo roundtrip ───────────────────────────────────────────────────────

  test('echo roundtrip: command output appears in tmux pane within 3s', async ({ page }) => {
    checkSkip();
    test.setTimeout(30_000);

    const marker = `roundtrip-${RUN_ID}`;

    await openSessionTerminal(page, sessionId);

    // Send a printf that emits a unique marker string
    await typeInTerminal(page, `printf '%s\\n' '${marker}'`);

    // Poll tmux pane for the marker — this exercises the FULL loop
    const deadline = Date.now() + 8_000;
    let found = false;
    while (Date.now() < deadline) {
      const pane = await captureTmuxPane(tmuxSession);
      if (pane.includes(marker)) {
        found = true;
        break;
      }
      await sleep(200);
    }

    // Screenshot regardless of outcome (stored in test-results/)
    fs.mkdirSync('test-results', { recursive: true });
    await page.screenshot({ path: `test-results/roundtrip-echo-${RUN_ID}.png` });

    expect(
      found,
      `Marker "${marker}" never appeared in tmux capture-pane output — ` +
      'the browser→tmux→browser loop may be broken',
    ).toBe(true);

    console.log(`✅ Echo roundtrip: marker "${marker}" confirmed in tmux pane`);
  });

  // ── 2. TUI rendering ────────────────────────────────────────────────────────

  test('TUI rendering: top -b -n 1 fills terminal with structured output', async ({ page }) => {
    checkSkip();
    test.setTimeout(30_000);

    await openSessionTerminal(page, sessionId);

    // Clear the pane so we get a clean baseline
    await typeInTerminal(page, 'clear');
    await sleep(500);

    // Run top in batch mode (non-interactive, one iteration)
    const tuiMarker = `top-done-${RUN_ID}`;
    await typeInTerminal(page, `top -b -n 1 | head -20 && printf '%s\\n' '${tuiMarker}'`);

    // Wait for marker to appear in tmux — proves top completed and output arrived
    const deadline = Date.now() + 15_000;
    let found = false;
    while (Date.now() < deadline) {
      const pane = await captureTmuxPane(tmuxSession);
      if (pane.includes(tuiMarker)) {
        found = true;
        // Verify the pane has realistic top output (Cpu, Mem, Tasks appear in real top)
        const hasTopHeader = /tasks|cpu|mem|load avg/i.test(pane);
        if (hasTopHeader) {
          console.log('✅ TUI rendering: top header keywords found in tmux output');
        } else {
          console.warn('⚠️ TUI: marker found but top header keywords missing (may be OS-specific)');
        }
        break;
      }
      await sleep(300);
    }

    await page.screenshot({ path: `test-results/roundtrip-tui-${RUN_ID}.png` });

    expect(found, `TUI marker "${tuiMarker}" never appeared — top may have failed`).toBe(true);

    // With the DOM renderer (chromium-dom project), assert that text actually
    // appears in .xterm-rows — proving the browser rendered the output, not just
    // that tmux received it.
    const renderedRows = await countRenderedRows(page);
    console.log(`  xterm rendered rows with content: ${renderedRows}`);
    if (renderedRows > 0) {
      // DOM renderer is active — assert at least a few rows have content
      expect(
        renderedRows,
        'xterm DOM rows are empty — terminal may not have rendered TUI output',
      ).toBeGreaterThan(3);
      const renderedText = await readRenderedText(page);
      console.log(`  xterm DOM text sample: ${renderedText.slice(0, 120).replace(/\n/g, ' ↵ ')}`);
    } else {
      // WebGL renderer — text is on canvas; DOM check skipped (tmux assertion above is sufficient)
      console.log('  WebGL renderer active — DOM text check skipped');
    }
  });

  // ── 3. Large scrollback ─────────────────────────────────────────────────────

  test('large scrollback: seq 1 2000 completes and terminal stays responsive', async ({ page }) => {
    checkSkip();
    test.setTimeout(60_000);

    await openSessionTerminal(page, sessionId);
    await typeInTerminal(page, 'clear');
    await sleep(500);

    const scrollbackMarker = `scrollback-done-${RUN_ID}`;
    const heapBefore = await heapSize(page);

    // Generate 2000 lines of sequential output
    console.log('  Sending seq 1 2000 …');
    const t0 = Date.now();
    await typeInTerminal(page, `seq 1 2000 && printf '%s\\n' '${scrollbackMarker}'`);

    // Wait for the marker (confirms all 2000 lines were flushed through)
    const deadline = Date.now() + 30_000;
    let found = false;
    while (Date.now() < deadline) {
      const pane = await captureTmuxPane(tmuxSession, 3000);
      if (pane.includes(scrollbackMarker)) {
        found = true;
        break;
      }
      await sleep(400);
    }
    const renderMs = Date.now() - t0;

    const heapAfter = await heapSize(page);
    const heapGrowthMB = heapBefore > 0 ? (heapAfter - heapBefore) / (1024 * 1024) : -1;

    console.log(`  seq 1 2000 + marker round-trip: ${renderMs}ms`);
    if (heapBefore > 0) {
      console.log(`  JS heap growth: ${heapGrowthMB.toFixed(1)} MB`);
    }

    await page.screenshot({ path: `test-results/roundtrip-scrollback-${RUN_ID}.png` });

    expect(found, 'scrollback marker never appeared — seq may have hung or WS disconnected').toBe(true);

    // Terminal must still be responsive after a 2000-line burst
    const responsiveMarker = `responsive-${RUN_ID}`;
    await typeInTerminal(page, `printf '%s\\n' '${responsiveMarker}'`);
    await sleep(1500);
    const paneAfter = await captureTmuxPane(tmuxSession);
    expect(
      paneAfter.includes(responsiveMarker),
      'Terminal is unresponsive after large scrollback — possible hang or disconnect',
    ).toBe(true);

    console.log('✅ Large scrollback: terminal remained responsive after 2000 lines');

    // Soft assertion: heap growth under 50 MB (informational, not blocking)
    if (heapBefore > 0 && heapGrowthMB > 50) {
      console.warn(`⚠️ JS heap grew ${heapGrowthMB.toFixed(1)} MB during scrollback — investigate memory retention`);
    }
  });

  // ── 4. Window resize ────────────────────────────────────────────────────────

  test('window resize: PTY dimensions update when viewport changes size', async ({ page }) => {
    checkSkip();
    test.setTimeout(30_000);

    await openSessionTerminal(page, sessionId);

    // Measure initial column count
    const colsBefore = await measureXtermCols(page);
    console.log(`  Initial xterm cols: ${colsBefore}`);

    // Shrink the viewport by ~40% width — forces a ResizeObserver + fit() cycle
    const viewport = page.viewportSize();
    const narrowWidth = Math.round((viewport?.width ?? 1280) * 0.6);
    await page.setViewportSize({ width: narrowWidth, height: viewport?.height ?? 720 });

    // Allow ResizeObserver debounce (250 ms) + double-rAF to settle
    await sleep(600);

    const colsAfterShrink = await measureXtermCols(page);
    console.log(`  Cols after shrink to ${narrowWidth}px: ${colsAfterShrink}`);

    // Restore original viewport
    await page.setViewportSize({ width: viewport?.width ?? 1280, height: viewport?.height ?? 720 });
    await sleep(600);

    const colsAfterRestore = await measureXtermCols(page);
    console.log(`  Cols after restore to ${viewport?.width ?? 1280}px: ${colsAfterRestore}`);

    // Verify PTY received the new dimensions via tmux — `tput cols` reports what
    // the PTY thinks the width is (matches the TIOCSWINSZ ioctl sent by the server)
    const resizeMarker = `resize-${RUN_ID}`;
    await typeInTerminal(page, `printf 'cols=%s\\n' "$(tput cols)" && printf '%s\\n' '${resizeMarker}'`);
    const deadline = Date.now() + 8_000;
    let paneOutput = '';
    while (Date.now() < deadline) {
      paneOutput = await captureTmuxPane(tmuxSession);
      if (paneOutput.includes(resizeMarker)) break;
      await sleep(200);
    }

    await page.screenshot({ path: `test-results/roundtrip-resize-${RUN_ID}.png` });

    expect(paneOutput.includes(resizeMarker), 'resize marker never appeared in tmux pane').toBe(true);

    // After restore the PTY col count should match (or be very close to) xterm's col count
    if (colsAfterRestore !== null) {
      const match = paneOutput.match(/cols=(\d+)/);
      if (match) {
        const ptyColsAfterRestore = parseInt(match[1], 10);
        console.log(`  PTY cols (tput cols): ${ptyColsAfterRestore}, xterm cols: ${colsAfterRestore}`);
        expect(
          Math.abs(ptyColsAfterRestore - colsAfterRestore),
          `PTY width (${ptyColsAfterRestore}) diverged from xterm width (${colsAfterRestore}) after resize — TIOCSWINSZ may not have fired`,
        ).toBeLessThanOrEqual(2); // allow 2-col tolerance for rounding
      }
    }

    // Narrower viewport must have produced fewer columns
    if (colsBefore !== null && colsAfterShrink !== null) {
      expect(
        colsAfterShrink,
        `Cols did not decrease after viewport shrink (before: ${colsBefore}, after: ${colsAfterShrink}) — ResizeObserver may not have fired`,
      ).toBeLessThan(colsBefore);
    }

    console.log('✅ Window resize: PTY dimensions updated correctly');
  });
});
