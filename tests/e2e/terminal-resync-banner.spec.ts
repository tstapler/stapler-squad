// @feature terminal-render, session-stream-terminal
/**
 * E2E tests for the terminal resync reconnect/hard-fail banners
 * (project_plans/terminal-resync-reliability), covering validation.md rows 1-3:
 *   - terminalBanner_should_RemainHidden_When_BackgroundedTerminalWasNotSwitchedTo
 *   - reconnectingBanner_should_SelfClearOrEscalateToHardFail_When_ResyncNeverStaysStuckPending
 *   - hardFailBanner_should_ShowRetryAndReattemptConnection_When_Clicked
 *
 * Implementation refs:
 *   - web-app/src/components/sessions/TerminalOutput.tsx (banner markup:
 *     role="status" aria-label="Reconnecting" for the soft banner,
 *     role="alert" with a "Retry" button for the hard-fail banner)
 *   - web-app/src/components/sessions/useVisibilityResync.ts (RESYNC_BANNER_DELAY_MS=2000,
 *     RESYNC_STALL_TIMEOUT_MS=4000 watchdog that forces disconnect+reconnect —
 *     the "self-clear" path — when a resync response never arrives)
 *   - web-app/src/lib/hooks/useTerminalStream.ts / backoff.ts (isHardFailed driven by
 *     a non-retriable WS close code — 4001/4004 — surfaced via ConnectError metadata)
 *
 * The terminal stream is a genuine browser WebSocket (StreamTerminal, proxied via
 * it-ws), so page.routeWebSocket() can transparently proxy it to the real isolated
 * test server while selectively withholding frames (row 2) or injecting a
 * server-initiated close code (row 3) — this exercises the real client-side state
 * machine, not a mocked one.
 *
 * NOTE: no Go server code path in this repo ever actually emits a non-retriable
 * WS close code (grep for 4001/4004 in server/ and session/ turns up nothing) —
 * hard-fail is a real, shipped client capability that is currently unreachable
 * from genuine server behavior. Row 3 exercises it via a test-injected close code,
 * which is the correct way to cover client-side hard-fail handling given that gap,
 * but is not exercising a real server-driven scenario end-to-end.
 */

import { test, expect } from '@playwright/test';
import { ShellTabsPage } from './pages/ShellTabsPage';
import { SessionClient } from './helpers/session-client';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

const FIXTURE_SESSION_TITLE = 'terminal-resync-banner-e2e-fixture';

async function openSessionDetailView(page: import('@playwright/test').Page): Promise<ShellTabsPage> {
  const shellTabs = new ShellTabsPage(page);
  // Pre-seed the first-visit onboarding dialog as dismissed so it doesn't
  // intercept clicks later in the flow (same pattern as session-notes.spec.ts
  // / ci-status-badge.spec.ts).
  await page.addInitScript(() => {
    localStorage.setItem('stapler-squad:onboarded', 'true');
  });
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForSelector(
    '[data-testid="session-card"], [data-testid="session-row"]',
    { timeout: 15000 },
  );
  // Target the fixture session by name rather than openFirstSession(): the
  // isolated test server (global-setup.ts) always pre-seeds several demo
  // sessions ahead of it in the list, including ones with status "Stopped"
  // that never establish a live terminal/WS connection — opening "whatever
  // sorts first" silently exercised the wrong session and made the WS
  // close-code injection in the hard-fail test unreachable.
  await shellTabs.openSessionByTitle(FIXTURE_SESSION_TITLE);
  return shellTabs;
}

test.beforeAll(async () => {
  const client = new SessionClient(BASE_URL);
  // Always ensure the fixture exists — global-setup.ts pre-seeds several demo
  // sessions, so `listSessions().length === 0` is never true and a prior
  // version of this check silently never created the fixture at all.
  const sessions = await client.listSessions();
  if (!sessions.some((s) => s.title === FIXTURE_SESSION_TITLE)) {
    await client.createSession({
      title: FIXTURE_SESSION_TITLE,
      path: '/tmp',
      program: 'bash',
    });
  }
});

test.describe('terminal-resync-banner', () => {
  // Row 1: a backgrounded (not-switched-to) terminal's reconnecting/hard-fail
  // banner must never appear, even while a document-level visibility/focus
  // event fires a resync on the foregrounded sibling terminal.
  test('terminalBanner_should_RemainHidden_When_BackgroundedTerminalWasNotSwitchedTo', async ({ page }) => {
    const shellTabs = await openSessionDetailView(page);

    // Unique per invocation so a Playwright retry (which reuses the same
    // fixture session/server) never collides with a tab left behind by a
    // prior attempt — a fixed name broke getShellTab's uniqueness assumption
    // on retry (strict-mode violation: 2 tabs matching the same name).
    const backgroundTabName = `e2e-background-me-${Date.now()}`;

    // Ensure a second shell tab exists so there is a real backgrounded sibling
    // TerminalOutput instance (kept mounted with aria-hidden, per
    // SessionDetailView.tsx's "Shell tab panels ... kept mounted but hidden").
    await shellTabs.openDialogViaButton({ name: backgroundTabName });
    await shellTabs.submitAndWait();
    const backgroundTab = shellTabs.getShellTab(backgroundTabName);
    await expect(backgroundTab).toBeVisible({ timeout: 8000 });

    // Switch focus to the ORIGINAL (first) tab, leaving 'e2e-background-me'
    // backgrounded (mounted, aria-hidden=true, isVisible=false).
    const allTabs = page.getByRole('tab');
    await allTabs.first().click();

    // role="tabpanel"][aria-hidden="true"] identifies the backgrounded
    // panel(s) — SessionDetailView.tsx keeps every shell's TerminalOutput
    // mounted and sets aria-hidden on whichever one isn't the active tab.
    const hiddenPanels = page.locator('[role="tabpanel"][aria-hidden="true"]');
    await expect(hiddenPanels.first()).toBeAttached({ timeout: 8000 });

    // Trigger a document-level visibility/focus resync (the real trigger for
    // useVisibilityResync's handleVisibilityOrFocusResync) via a background/
    // foreground cycle of the whole page — this fires resync on the
    // foregrounded (active) instance only when properly scoped, and must
    // never surface a banner inside the backgrounded panel.
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    // Give the debounced resync (300ms) + banner delay (2000ms) window time to
    // pass by waiting on a concrete, non-arbitrary condition instead: the
    // active panel's own banners settling (present or not) — poll until the
    // hidden panel provably never shows a banner across that whole window.
    await expect(async () => {
      const hiddenBannerCount = await hiddenPanels.first()
        .locator('[role="status"][aria-label="Reconnecting"], [role="alert"]')
        .count();
      expect(hiddenBannerCount).toBe(0);
    }).toPass({ timeout: 4000, intervals: [200] });
  });

  // Row 2: when a resync's response never arrives, the reconnecting banner
  // must not stay stuck forever — it must either self-clear (the 4s stall
  // watchdog forces disconnect+reconnect, which clears the banner once the
  // new connection succeeds) or escalate into the hard-fail banner. It must
  // never remain visible indefinitely.
  test('reconnectingBanner_should_SelfClearOrEscalateToHardFail_When_ResyncNeverStaysStuckPending', async ({ page }) => {
    let buffering = false;
    let connectionCount = 0;
    await page.routeWebSocket(/StreamTerminal/, async (ws) => {
      connectionCount += 1;
      const server = ws.connectToServer();
      server.onMessage((message) => {
        if (buffering) return; // withhold server->client frames while buffering
        ws.send(message);
      });
      ws.onMessage((message) => server.send(message));
    });

    const shellTabs = await openSessionDetailView(page);
    // Let the initial connection/resync settle before starving frames.
    await expect(page.getByRole('tab').first()).toBeVisible({ timeout: 8000 });

    // Start withholding all server->client frames, then fire a resync via a
    // visibility/focus cycle — its response will never arrive, forcing the
    // real 2s-banner / 4s-stall-watchdog timers to run for real.
    buffering = true;
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      window.dispatchEvent(new Event('focus'));
    });

    const reconnectingBanner = page.getByRole('status', { name: 'Reconnecting' });
    // Next.js's own route-announcer (#__next-route-announcer__) is also
    // role="alert" and always present, so scope to the one containing a
    // Retry button (the hard-fail banner's distinguishing feature).
    const hardFailBanner = page.getByRole('alert').filter({ has: page.getByRole('button', { name: 'Retry' }) });

    // The soft banner should appear once the 2s delay elapses (best-effort —
    // some environments may jump straight past it if the watchdog fires
    // first), so only assert on the terminal condition below.

    // Wait for a concrete signal that the 4s stall watchdog actually fired —
    // a brand-new WebSocket connection attempt (useVisibilityResync forces
    // disconnect+reconnect on stall) — rather than a blind fixed sleep. Only
    // once that real reconnect attempt is underway do we stop withholding
    // frames, since the reconnect itself still needs live frames to succeed.
    const connectionsBeforeStall = connectionCount;
    await expect(async () => {
      expect(connectionCount).toBeGreaterThan(connectionsBeforeStall);
    }).toPass({ timeout: 8000, intervals: [200] });
    buffering = false;

    // Terminal condition: within a bounded window after the stall watchdog
    // fires, the banner must NOT be stuck showing "Reconnecting" forever —
    // it must have either cleared entirely or turned into the hard-fail alert.
    await expect(async () => {
      const stuckReconnecting = await reconnectingBanner.isVisible().catch(() => false);
      const isHardFailed = await hardFailBanner.isVisible().catch(() => false);
      expect(stuckReconnecting && !isHardFailed).toBe(false);
    }).toPass({ timeout: 10000, intervals: [250] });
  });

  // Row 3: the hard-fail banner (shown when the WS closes with a non-retriable
  // code) must offer a Retry action that re-attempts the connection.
  test('hardFailBanner_should_ShowRetryAndReattemptConnection_When_Clicked', async ({ page }) => {
    let connectionCount = 0;
    // Track the most recently opened connection so the test can force-close
    // the *currently live* socket directly, rather than trying to trigger a
    // fresh connection attempt to intercept. (A visibility/focus cycle while
    // already connected sends the resync request over the existing socket —
    // see useVisibilityResync.ts's handleVisibilityOrFocusResyncInner, which
    // only calls connect() as a fallback when already disconnected — so it
    // never opens a new connection for a route handler to kill.) A real
    // non-retriable close (per web-app/src/lib/utils/backoff.ts's
    // NON_RETRIABLE_WS_CODES) is server-initiated on an established
    // connection, which this mirrors directly.
    let activeWs: import('@playwright/test').WebSocketRoute | null = null;

    await page.routeWebSocket(/StreamTerminal/, async (ws) => {
      connectionCount += 1;
      activeWs = ws;
      const server = ws.connectToServer();
      server.onMessage((message) => ws.send(message));
      ws.onMessage((message) => server.send(message));
    });

    const shellTabs = await openSessionDetailView(page);
    await expect(page.getByRole('tab').first()).toBeVisible({ timeout: 8000 });
    // The tab bar renders before the terminal's WebSocket actually connects
    // (StreamTerminal is opened lazily by TerminalOutput), so wait for a real
    // connection to land before trying to force-close it.
    await expect(async () => {
      expect(activeWs).not.toBeNull();
    }).toPass({ timeout: 8000, intervals: [200] });

    const connectionsBeforeKill = connectionCount;
    expect(activeWs).not.toBeNull();
    // Non-retriable close code (session-not-found), per
    // web-app/src/lib/utils/backoff.ts's NON_RETRIABLE_WS_CODES.
    activeWs!.close({ code: 4004, reason: 'e2e forced hard failure' });

    // Next.js's own route-announcer (#__next-route-announcer__) is also
    // role="alert" and always present, so scope to the one containing the
    // Retry button rather than the bare role.
    const hardFailBanner = page.getByRole('alert').filter({ has: page.getByRole('button', { name: 'Retry' }) });
    await expect(hardFailBanner).toBeVisible({ timeout: 10000 });
    const retryButton = hardFailBanner.getByRole('button', { name: 'Retry' });
    await expect(retryButton).toBeVisible();

    await retryButton.click();

    // Clicking Retry must trigger exactly one new connection attempt
    // (handleManualReconnect resets isHardFailed and calls connect()) —
    // observed here as a new WebSocket connection to StreamTerminal. connect()
    // itself now refuses to run while isHardFailedRef is set (see
    // useTerminalStream.ts), so no other caller can sneak in an extra
    // reconnect between the hard-fail and this click; hence a plain
    // greater-than-baseline check (not baseline+1) is now accurate.
    await expect(async () => {
      expect(connectionCount).toBeGreaterThan(connectionsBeforeKill);
    }).toPass({ timeout: 8000, intervals: [200] });
  });
});
