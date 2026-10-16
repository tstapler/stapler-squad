// @feature terminal-render, session-stream-terminal
/**
 * E2E coverage for validation.md row 8 (Surface 2, terminal-resync-reliability):
 *   staggerCoordinator_should_FireResyncImmediatelyWithNoJitter_When_StaggerFlagOff
 *
 * Implementation refs:
 *   - web-app/src/components/sessions/SessionDetailView.tsx's ResyncStaggerQueue /
 *     useResyncStaggerQueue: `useResyncStaggerQueue(enabled, ...)` returns
 *     `undefined` for the per-instance scheduler when `enabled` is false — the
 *     queue class is never even instantiated, so there is no code path capable
 *     of injecting jitter (`Math.random() * RESYNC_STAGGER_JITTER_MAX_MS`).
 *     Every enqueue also logs `console.debug('[resync-stagger] burst size=...')`,
 *     which makes "the stagger path never ran" directly observable.
 *   - web-app/src/components/sessions/useVisibilityResync.ts's
 *     handleVisibilityOrFocusResyncInner: builds a `fire` closure and calls it
 *     synchronously when `scheduleResyncRef.current` is unset (flag off) —
 *     "unchanged from pre-Epic-6.1 behavior" per the inline comment there.
 *
 * This test asserts absence of any `[resync-stagger]` log during a real
 * visibility/focus resync cycle with the flag off — a deterministic,
 * code-path-level check tied to the actual jitter source, rather than a
 * timing-threshold assertion (the `[resync] ... delay=0ms` log text is a
 * static label, not a measured value, so it can't be used to prove timing).
 *
 * Row 6 (terminalSwitch_should_ShowNoAddedLatencyVsSingleSessionBaseline_...)
 * is NOT covered here: it requires a live review-queue with 4+ real sessions
 * simultaneously in NeedsApproval state to exercise SessionDetailView's
 * showNavigation Next/Previous flow (the only caller of the terminal pool
 * this row is about). Standing up multiple sessions that a real Claude Code
 * process has driven into NeedsApproval is exactly the "requires tmux
 * sessions, git worktrees, and program execution" backend setup that
 * review-queue.spec.ts already documents as infeasible for E2E without mock
 * infrastructure (see its "Advanced Review Queue Tests (Require Backend)"
 * skipped describe block) — mocking GetReviewQueue's response (as
 * review-queue-severity.spec.ts does) produces fabricated session IDs with no
 * real WebSocket-backed terminal behind them, which is exactly the mechanism
 * row 6 needs to measure.
 *
 * Row 7 (resyncPreemption_...) is also not covered here — already established
 * as skipped (sibling shell-tab TerminalOutput instances share
 * `effectiveSessionId` in console logs with no per-instance identifier to
 * verify ordering claims).
 */

import { test, expect } from '@playwright/test';
import { ShellTabsPage } from './pages/ShellTabsPage';
import { SessionClient } from './helpers/session-client';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const STAGGER_FLAG = 'terminal:resync-stagger';
const FIXTURE_SESSION_TITLE = 'terminal-resync-stagger-e2e-fixture';

async function resetStaggerFlag(request: import('@playwright/test').APIRequestContext) {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { 'Content-Type': 'application/json' },
    data: { name: STAGGER_FLAG, enabled: false },
  });
}

async function openSessionDetailView(page: import('@playwright/test').Page): Promise<ShellTabsPage> {
  const shellTabs = new ShellTabsPage(page);
  await page.addInitScript(() => {
    localStorage.setItem('stapler-squad:onboarded', 'true');
  });
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForSelector(
    '[data-testid="session-card"], [data-testid="session-row"]',
    { timeout: 15000 },
  );
  await shellTabs.openSessionByTitle(FIXTURE_SESSION_TITLE);
  return shellTabs;
}

test.beforeAll(async () => {
  const client = new SessionClient(BASE_URL);
  const sessions = await client.listSessions();
  if (!sessions.some((s) => s.title === FIXTURE_SESSION_TITLE)) {
    await client.createSession({
      title: FIXTURE_SESSION_TITLE,
      path: '/tmp',
      program: 'bash',
    });
  }
});

test.describe('terminal-resync-stagger', () => {
  test.beforeEach(async ({ request }) => {
    await resetStaggerFlag(request);
  });

  test.afterEach(async ({ request }) => {
    await resetStaggerFlag(request);
  });

  test('staggerCoordinator_should_FireResyncImmediatelyWithNoJitter_When_StaggerFlagOff', async ({ page }) => {
    const consoleMessages: string[] = [];
    page.on('console', (msg) => consoleMessages.push(msg.text()));

    const shellTabs = await openSessionDetailView(page);
    await expect(page.getByRole('tab').first()).toBeVisible({ timeout: 8000 });

    // Wait for the terminal to be genuinely connected and outputting before
    // triggering a resync. The tab bar itself renders well before the
    // WebSocket finishes connecting, so gating on tab count (as an earlier
    // version of this test did) races useVisibilityResync's own
    // `terminalStateRef.current === 'CONNECTING' || 'LOADING'` guard: firing
    // visibilitychange mid-handshake silently drops the resync with no log at
    // all (see useVisibilityResync.ts's early-return branch), which made the
    // "fired" assertion below flaky whenever sibling specs in the same run
    // shifted this fixture session's connect timing. `[TerminalMetrics]`
    // (TerminalOutput.tsx's logTerminalMetrics) only logs once firstOutputTime
    // is set — i.e. the WS is connected and has actually streamed output — so
    // it is a real, deterministic "connected" signal instead of a proxy for one.
    await expect(async () => {
      const connected = consoleMessages.some((m) => m.includes('[TerminalMetrics]'));
      expect(connected).toBe(true);
    }).toPass({ timeout: 15000, intervals: [200] });

    const messagesBeforeTrigger = consoleMessages.length;

    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      window.dispatchEvent(new Event('focus'));
    });

    // A resync must actually fire (proves the trigger worked at all) — logged
    // synchronously inside the `fire` closure regardless of stagger state.
    await expect(async () => {
      const fired = consoleMessages
        .slice(messagesBeforeTrigger)
        .some((m) => m.includes('[resync]') && m.includes('trigger=visibility-or-focus'));
      expect(fired).toBe(true);
    }).toPass({ timeout: 4000, intervals: [200] });

    // The deterministic "no jitter" check: with the flag off,
    // useResyncStaggerQueue never instantiates a scheduler, so
    // ResyncStaggerQueue.schedule() — the only place that computes
    // `Math.random() * RESYNC_STAGGER_JITTER_MAX_MS` — must never run. Its
    // `[resync-stagger] burst size=...` log is the direct, observable proof of
    // that code path executing.
    const staggerLogs = consoleMessages.filter((m) => m.includes('[resync-stagger]'));
    expect(staggerLogs).toEqual([]);
  });
});
