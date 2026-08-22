// @feature connection-count-indicator
import * as fs from 'fs';
import * as path from 'path';
import { test, expect } from '@playwright/test';
import { SessionsPage } from './pages/SessionsPage';
import { SessionDetailPage } from './pages/SessionDetailPage';

/**
 * ConnectionCountIndicator (Epic 4.2, Story 4.2.2) — Surface 1 of
 * project_plans/terminal-multi-connection-streaming/design/ux.md.
 *
 * ConnectionCountIndicator.test.tsx (Jest) already covers everything that
 * can be verified from a single rendered component in isolation: render
 * gating on count, role/aria attributes, icon aria-hidden-ness, tooltip
 * content branching on `sizeMismatch`, debounce/coalescing of rapid count
 * changes, and the departure-announcement/unmount timing — all driven by
 * feeding synthetic `count` props directly to the component.
 *
 * What Jest cannot cover, because it never renders two real connections:
 * whether the indicator actually appears when a *second real browser
 * connection* attaches to the same session's StreamHub, and disappears (via
 * the departure state) when that connection actually goes away. This spec
 * covers exactly that gap, using two independent Playwright browser
 * contexts (separate cookie/localStorage jars, like two real tabs) against
 * the same session.
 *
 * `connection_count` is only populated on the PathHubOwned stream path
 * (STAPLER_SQUAD_USE_STREAM_HUB) — see terminal-hub-path.spec.ts's file
 * header for why this repo's shared e2e server can't yet isolate a single
 * hub-owned session per-test (Story 3.1.1's per-session override hasn't
 * landed). This spec gracefully skips unless an operator explicitly opts in:
 *
 *   STAPLER_SQUAD_USE_STREAM_HUB=true npx playwright test connection-count-indicator.spec.ts
 */

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const HUB_PATH_ENABLED = process.env.STAPLER_SQUAD_USE_STREAM_HUB === 'true';
const ONBOARDED_KEY = 'stapler-squad:onboarded';
const TEST_DIR = process.env.TEST_SERVER_TESTDIR;

/**
 * Even with STAPLER_SQUAD_USE_STREAM_HUB=true, config.ResolveGlobalStreamHubDefault
 * (server/services/connectrpc_websocket.go's useStreamHub) refuses to let the
 * *global* default resolve to true until config.json's
 * rollback_rehearsal_completed_at is set (Story 3.3.2's mechanical
 * rollback-rehearsal gate, pre-mortem P1 #4) — a fresh e2e config never has
 * this set, so without seeding it every session on this server silently
 * stays on PathLegacyPerConnection (connection_count never populated) no
 * matter the env var. Seed it directly onto the shared test server's
 * isolated config.json, the same direct-disk-seeding pattern
 * launcher-presets.spec.ts uses for launcher-presets.json — config.LoadConfig
 * re-reads this file fresh on every call (no caching), so writing it just
 * before creating a session is sufficient, no server restart needed.
 */
function configPath(): string {
  if (!TEST_DIR) {
    throw new Error('TEST_SERVER_TESTDIR is not set — global-setup.ts must export it');
  }
  return path.join(TEST_DIR, 'config.json');
}

function readConfig(): Record<string, unknown> {
  const p = configPath();
  if (!fs.existsSync(p)) return {};
  try {
    return JSON.parse(fs.readFileSync(p, 'utf-8'));
  } catch {
    return {};
  }
}

let originalConfigRaw: string | null = null;

function seedRollbackRehearsalCompleted() {
  const p = configPath();
  originalConfigRaw = fs.existsSync(p) ? fs.readFileSync(p, 'utf-8') : null;
  const cfg = readConfig();
  cfg.rollback_rehearsal_completed_at = new Date().toISOString();
  fs.writeFileSync(p, JSON.stringify(cfg));
}

function restoreConfig() {
  const p = configPath();
  if (originalConfigRaw !== null) {
    fs.writeFileSync(p, originalConfigRaw);
  } else {
    fs.rmSync(p, { force: true });
  }
  originalConfigRaw = null;
}

async function dismissOnboarding(page: import('@playwright/test').Page) {
  // useOnboarding.ts's 800ms timer otherwise pops a full-viewport onboarding
  // modal mid-test on a fresh (empty-localStorage) context — see
  // repo-path-picker-parity.spec.ts / session-completion-summary.spec.ts.
  await page.addInitScript((key) => {
    try {
      window.localStorage.setItem(key, 'true');
    } catch {
      /* ignore */
    }
  }, ONBOARDED_KEY);
}

test.describe('connection count indicator (multi-connection)', () => {
  // Default is 30s (playwright.config.ts). These tests drive two independent
  // browser contexts through session creation, a terminal-tab connect, and a
  // multi-step assertion chain each — more sequential UI actions than a
  // typical spec — so give them more headroom against the same hydration/
  // slow-CPU action-timeout flake documented on SessionsPage.createBashSession.
  // 120s covers the worst-case sum of every individual assertion's own
  // timeout below (several 15s/30s waits back to back) plus setup, rather
  // than relying on them overlapping favorably.
  test.describe.configure({ timeout: 120000 });

  test.beforeEach(() => {
    test.skip(
      !HUB_PATH_ENABLED,
      'STAPLER_SQUAD_USE_STREAM_HUB is not set for this run — connection_count is only ' +
        'populated on the PathHubOwned stream path. Re-run with ' +
        'STAPLER_SQUAD_USE_STREAM_HUB=true to exercise this spec.',
    );
    if (!HUB_PATH_ENABLED) return;
    seedRollbackRehearsalCompleted();
  });

  test.afterEach(() => {
    if (!HUB_PATH_ENABLED) return;
    restoreConfig();
  });

  test('indicator appears when a second tab attaches, and is accessible per design/ux.md UX-AC-2/4/6/8', async ({
    page,
    browser,
  }) => {
    await dismissOnboarding(page);

    const sessionsPage = new SessionsPage(page);
    const detailA = new SessionDetailPage(page);

    await sessionsPage.goto();
    await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });
    const sessionId = await sessionsPage.createBashSession('e2e-conn-count');

    await detailA.getTerminalTab().click();
    await expect(detailA.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    // Single-tab case: zero added visual surface (UX-AC-1).
    await expect(detailA.getConnectionCountIndicator()).not.toBeAttached();

    // Second, fully independent browser context — a separate cookie/
    // localStorage jar, the closest Playwright equivalent to a second real
    // browser tab/window attaching to the same session.
    const contextB = await browser.newContext();
    try {
      const pageB = await contextB.newPage();
      await dismissOnboarding(pageB);
      const detailB = new SessionDetailPage(pageB);
      await detailB.gotoSession(sessionId);

      await detailB.getTerminalTab().click();
      await expect(detailB.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
      await expect(pageB.getByText('Connected')).toBeVisible({ timeout: 15000 });

      // Operator (tab A) can tell another connection attached in 0 extra
      // steps (UX-AC-2) — the indicator is near the terminal chrome, no
      // navigation required.
      const indicatorA = detailA.getConnectionCountIndicator();
      // The server only pushes a fresh connection_count frame on tab A's
      // side-channel when SubscriberCount() changes (server/services/
      // connectrpc_websocket.go's sendConnectionCountUpdates), polled once a
      // second, plus the frontend's own 500ms coalesce debounce
      // (ConnectionCountIndicator.tsx) — under a slow/contended CPU that
      // whole round trip (server tick -> WS frame -> browser JS processing
      // -> React re-render) has been observed taking well past 15s, so this
      // wait gets extra headroom rather than assuming a healthy machine.
      await expect(indicatorA).toBeVisible({ timeout: 30000 });
      await expect(indicatorA).toHaveAttribute('role', 'status');
      await expect(indicatorA).toHaveAttribute('aria-live', 'polite');
      await expect(indicatorA).toHaveAttribute('aria-label', '2 connections active');

      // Never role="alert" — this is not an error state (UX-AC-4).
      await expect(page.getByRole('alert', { name: /connections? active/i })).toHaveCount(0);

      // Icon glyph is aria-hidden; visible/announced text carries all
      // meaning (UX-AC-6).
      await expect(indicatorA.locator('[aria-hidden="true"]')).toBeAttached();
      await expect(indicatorA).toContainText('2');

      // Keyboard-navigable: reachable via focus, tooltip revealable via
      // keyboard focus, not hover-only (UX-AC-3/8).
      await indicatorA.focus();
      const tooltipA = detailA.getConnectionCountTooltip();
      await expect(tooltipA).toBeVisible();
      await expect(tooltipA).toHaveText('2 connections active');
      await indicatorA.blur();
      await expect(tooltipA).not.toBeAttached();

      // Also reachable via hover (mouse path), same content.
      await indicatorA.hover();
      await expect(tooltipA).toBeVisible();
      await expect(tooltipA).toHaveText('2 connections active');
    } finally {
      await contextB.close();
    }
  });

  test('indicator announces departure and then disappears when the second tab closes (design/ux.md Step 5)', async ({
    page,
    browser,
  }) => {
    await dismissOnboarding(page);

    const sessionsPage = new SessionsPage(page);
    const detailA = new SessionDetailPage(page);

    await sessionsPage.goto();
    await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });
    const sessionId = await sessionsPage.createBashSession('e2e-conn-count-departure');

    await detailA.getTerminalTab().click();
    await expect(detailA.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    const contextB = await browser.newContext();
    const pageB = await contextB.newPage();
    await dismissOnboarding(pageB);
    const detailB = new SessionDetailPage(pageB);
    await detailB.gotoSession(sessionId);
    await detailB.getTerminalTab().click();
    await expect(detailB.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
    await expect(pageB.getByText('Connected')).toBeVisible({ timeout: 15000 });

    const indicatorA = detailA.getConnectionCountIndicator();
    // See the sibling test's identical wait for why this gets extra headroom
    // beyond the mechanism's normal ~1.5s (1s server poll + 500ms debounce).
    await expect(indicatorA).toHaveAttribute('aria-label', '2 connections active', { timeout: 30000 });

    // Close the second tab — hub's SubscriberCount() goes 2 -> 1.
    await contextB.close();

    // A screen-reader user who was told "2 connections active" must not go
    // silent without a signal: the live region updates its text to a
    // departure announcement before the node unmounts (a content mutation
    // within a still-present live region is reliably announced, unlike a
    // node's disappearance in the same commit).
    await expect(indicatorA).toHaveAttribute('aria-label', '1 connection active', { timeout: 15000 });
    await expect(indicatorA).toHaveAttribute('role', 'status');
    await expect(indicatorA).toHaveAttribute('aria-live', 'polite');

    // Then, once the departure announcement has been held long enough to be
    // read, the indicator actually unmounts — back to the single-tab,
    // zero-added-visual-surface state (UX-AC-1).
    await expect(indicatorA).not.toBeAttached({ timeout: 5000 });
  });
});
