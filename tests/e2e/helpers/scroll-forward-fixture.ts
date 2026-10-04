/**
 * Shared boilerplate for the app-scrollback-forwarding E2E suite
 * (tests/e2e/scroll-forward-*.spec.ts) — see
 * project_plans/app-scrollback-forwarding/implementation/validation.md's
 * "UX Acceptance Tests" table and design/ux.md for the surfaces these tests
 * cover.
 *
 * Every scenario needs a Claude Code stand-in: a session whose `program`
 * resolves via session/instance_tmux.go's isClaude (so
 * session/scroll_adapter.go's resolveScrollAdapter and
 * session/claude_scroll_adapter.go wire up) but that runs a scripted fixture
 * script instead of a real `claude` binary
 * (tests/e2e/fixtures/alt-screen-scroll-fixture.sh) — mirroring REQ-4's
 * "golden fixture, no live CLI in CI" strategy. isClaude matches on a
 * whitespace-split token's `filepath.Base(token) == "claude"`, so the
 * fixture is exposed via a directory containing a symlink literally named
 * `claude` pointing at the real script, and that symlink's absolute path is
 * passed as the session's `program` field directly via SessionClient
 * (bypassing the Omnibar's fixed <select>, which can't take an arbitrary
 * program string — same technique terminal-resync-banner.spec.ts and
 * siblings already use for arbitrary `program` values via direct RPC).
 */
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { expect, test, type APIRequestContext, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { SessionClient, type Session } from './session-client';
import { SessionDetailPage } from '../pages/SessionDetailPage';

export const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
export const TEST_DIR = process.env.TEST_SERVER_TESTDIR;

/** config.FeatureAppScrollForwardingClaude (config/config.go). */
export const APP_SCROLL_FORWARD_FLAG = 'terminal:app-scrollback-forwarding:claude';

/**
 * Historical note (kept for anyone grepping old runs/logs): this used to
 * gate on STAPLER_SQUAD_USE_STREAM_HUB, back when PathLegacyPerConnection
 * was the real default and reaching PathHubOwned required both the env var
 * and a seeded config.json rollback-rehearsal timestamp
 * (config.ResolveGlobalStreamHubDefault). Commit d0ab13c30
 * ("feat(rollout): drive stream-hub/tymux defaults from feature flags, not
 * env vars") removed that function, the env var, and the rehearsal gate
 * entirely: config.EffectiveStreamHubEnabled now defaults the "stream_hub"
 * feature flag to `true` unconditionally (config/config.go), so PathHubOwned
 * — not PathLegacyPerConnection — is every session's real default today,
 * with no env var or seeding needed. HUB_PATH_ENABLED is kept as a constant
 * `true` only so existing `!HUB_PATH_ENABLED`-shaped call sites don't need
 * touching; new code should just assume the hub path is live.
 */
export const HUB_PATH_ENABLED = true;

export const ONBOARDED_KEY = 'stapler-squad:onboarded';

export async function setAppScrollForwardFlag(request: APIRequestContext, enabled: boolean): Promise<void> {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { 'Content-Type': 'application/json' },
    data: { name: APP_SCROLL_FORWARD_FLAG, enabled },
  });
}

/**
 * Pins sessionTitle onto PathLegacyPerConnection via the existing
 * SetStreamHubSessionOverride RPC (server/services/stream_hub_rollout_service.go),
 * the "existing test hook for streaming-path selection" validation.md's
 * UX-AC-4 case 2 refers to.
 *
 * Needed because config.EffectiveStreamHubEnabled's global default is `true`
 * (config/config.go), so PathHubOwned -- not PathLegacyPerConnection -- is
 * what a freshly created session actually gets today. The
 * UNSUPPORTED_STREAMING_PATH-only specs in this suite need a *solo*
 * PathLegacyPerConnection session specifically (AppScrollGate's -1
 * subscriberCount sentinel only applies there); without this override they
 * silently run on the hub path instead, where a solo viewer's real
 * subscriberCount of 1 passes AppScrollGate and produces a genuine
 * DELIVERED/AT_TOP outcome -- never BLOCKED -- so the expected toast never
 * appears.
 *
 * Must be called before the session's terminal WebSocket first connects
 * (openAndWaitConnected/StreamOwnershipLock caches the first resolution per
 * tmux session), but the override key resolves from the title alone, so it's
 * safe to call right after session creation.
 */
export async function forceLegacyStreamingPath(request: APIRequestContext, sessionTitle: string): Promise<void> {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/SetStreamHubSessionOverride`, {
    headers: { 'Content-Type': 'application/json' },
    data: { sessionName: sessionTitle, forceHub: false },
  });
}

export async function dismissOnboarding(page: Page): Promise<void> {
  await page.addInitScript((key) => {
    try {
      window.localStorage.setItem(key, 'true');
    } catch {
      /* ignore */
    }
  }, ONBOARDED_KEY);
}

/** @deprecated no longer used — hub path never skips now. Kept only so an old import doesn't break the build. */
export const HUB_SKIP_REASON = '';

/**
 * Shared `test.beforeEach`/`test.afterEach` pair for every
 * UNSUPPORTED_STREAMING_PATH-only spec in this suite: just toggles the
 * feature flag for the test's duration. These specs additionally need
 * forceLegacyStreamingPath (per-session, called from the test body once the
 * session's title is known) to actually land on PathLegacyPerConnection,
 * since config.EffectiveStreamHubEnabled's global default is hub-owned.
 */
export async function soloPathBeforeEach({ page, request }: { page: Page; request: APIRequestContext }): Promise<void> {
  await setAppScrollForwardFlag(request, true);
  await dismissOnboarding(page);
}

export async function soloPathAfterEach({ request }: { request: APIRequestContext }): Promise<void> {
  await setAppScrollForwardFlag(request, false);
}

/**
 * Shared `test.beforeEach`/`test.afterEach` pair for every spec in this
 * suite that needs a DELIVERED/AT_TOP/MULTIPLE_VIEWERS outcome. Used to also
 * skip unless HUB_PATH_ENABLED and seed a rollback-rehearsal gate in
 * config.json -- both dead since commit d0ab13c30 (see HUB_PATH_ENABLED's
 * doc comment): PathHubOwned is the unconditional default now, so this just
 * toggles the app-scroll-forward flag for the test's duration.
 */
export async function hubGatedBeforeEach({
  page,
  request,
}: {
  page: Page;
  request: APIRequestContext;
}): Promise<void> {
  await setAppScrollForwardFlag(request, true);
  await dismissOnboarding(page);
}

export async function hubGatedAfterEach({ request }: { request: APIRequestContext }): Promise<void> {
  await setAppScrollForwardFlag(request, false);
}

// ---------------------------------------------------------------------
// Fixture program (the scripted "claude" stand-in)
// ---------------------------------------------------------------------

const FIXTURE_SCRIPT_PATH = path.resolve(__dirname, '../fixtures/alt-screen-scroll-fixture.sh');

/**
 * Creates a fresh temp directory containing a symlink named `claude` ->
 * the fixture script, and returns the symlink's absolute path for use as a
 * session's `program` field. A fresh directory per call avoids any
 * cross-test interference if two tests happen to run concurrently.
 */
export function createClaudeFixtureProgram(): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'ssq-scroll-fixture-'));
  const programPath = path.join(dir, 'claude');
  fs.symlinkSync(FIXTURE_SCRIPT_PATH, programPath);
  return programPath;
}

export interface AltScreenScrollSession {
  session: Session;
  programPath: string;
}

/** Creates a session whose program is the scripted alt-screen "claude" fixture. */
export async function createAltScreenScrollSession(
  client: SessionClient,
  title: string,
  extra: { path?: string } = {},
): Promise<AltScreenScrollSession> {
  const programPath = createClaudeFixtureProgram();
  const session = await client.createSession({
    title,
    path: extra.path ?? '/tmp',
    program: programPath,
  });
  return { session, programPath };
}

/**
 * Opens the Terminal tab and waits for the standard "attached and streaming"
 * readiness signal used throughout this suite (toolbar visible + "Connected"
 * text) — same idiom as connection-count-indicator.spec.ts /
 * terminal-resync-banner.spec.ts.
 */
export async function openAndWaitConnected(page: Page, detail: SessionDetailPage): Promise<void> {
  await detail.getTerminalTab().click();
  await expect(detail.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
  await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });
  // "Connected" only means the WebSocket is open -- TerminalOutput.tsx's
  // wheel listener (Task 1.4.0b, the trigger every scroll-forward gesture in
  // this suite depends on) isn't attached until isLoadingInitialContent
  // clears, which happens on the stream manager's onFirstOutput callback
  // (the session's first PTY output byte), a later and independent event.
  // A scroll gesture fired between "Connected" and first-output silently
  // no-ops (isLoadingInitialContent gates the listener's own useEffect) --
  // a real, intermittent race observed across several specs in this suite,
  // not specific to any one of them.
  await expect(page.getByText(/Loading terminal content|Initializing terminal|Starting session/)).not.toBeVisible({
    timeout: 15000,
  });
}

/**
 * Story 1.4.0's alt-screen scroll-up trigger — a `wheel` listener
 * (TerminalOutput.tsx's Task 1.4.0b) on xterm.js's internal
 * `.xterm-viewport` element (negative deltaY = scroll up).
 *
 * Dispatches directly on `.xterm-viewport` rather than `page.mouse.wheel()`
 * after `.hover()` on the outer panel: the latter reliably never reached
 * the listener in this headless viewport (likely a coordinate/z-order
 * mismatch against xterm's canvas rendering), verified by direct
 * comparison. `.xterm-viewport` has no ARIA equivalent, so this is a
 * narrow, deliberate exception to the "no CSS selectors" rule for event
 * *dispatch* only — nothing in this suite asserts against this selector.
 */
async function dispatchTerminalWheel(page: Page, detail: SessionDetailPage, deltaY: number): Promise<void> {
  const panel = detail.getTerminalPanel();
  await panel.evaluate((el, dy) => {
    const viewport = el.querySelector('.xterm-viewport') ?? el;
    viewport.dispatchEvent(new WheelEvent('wheel', { deltaY: dy, bubbles: true, cancelable: true }));
  }, deltaY);
}

export async function scrollUpOnTerminal(page: Page, detail: SessionDetailPage): Promise<void> {
  await dispatchTerminalWheel(page, detail, -120);
}

/** Scroll-down counterpart — positive deltaY, used to return to the live tail. */
export async function scrollDownOnTerminal(page: Page, detail: SessionDetailPage): Promise<void> {
  await dispatchTerminalWheel(page, detail, 120);
}

/**
 * Repeats a scroll-up gesture until `check` passes (bounded `.toPass()`
 * retry, not a fixed sleep — see e2e-test-conventions' no-waitForTimeout
 * rule). Needed because the fixture session's AppScrollGate preconditions
 * (AltScreenActive observed from a live PTY byte, and DetectedStatus
 * settling to Idle via session/detection's pattern-match loop, which is
 * driven by an async statusCheckCh signal, not synchronous with session
 * creation) can still be settling in the ~1-2s immediately after
 * `openAndWaitConnected` resolves — "Connected" only means the WebSocket is
 * up, not that server-side status classification has caught up. A single
 * scroll fired too early is silently dropped (AppScrollGate fails closed,
 * same as a real not-yet-idle session), so retrying the gesture itself
 * (not just waiting) is what actually converges once the precondition is
 * met, mirroring terminal-resync-banner.spec.ts's `expect(async () => {...
 * }).toPass(...)` idiom for a real, server-timed race.
 *
 * Retry-safety guard: a naive retry here would re-dispatch the wheel
 * gesture even while a previous ForwardScroll round trip is still
 * genuinely in flight (session/instance_scroll_forward.go's
 * waitForRedrawQuiescence can legitimately take up to its documented ~2s
 * worst case, longer than any one `check()`'s own timeout) — silently
 * turning one intended scroll into two real ones and producing outcomes
 * (e.g. DELIVERED then AT_TOP) that don't match what the test set up. The
 * scroll-loading-pill (TerminalOutput.tsx, data-testid="scroll-loading-pill")
 * is that request's DOM-visible proxy: it's shown for the same span
 * isFetchingScrollbackRef is true and cleared in the exact same
 * handleAppScrollbackFrame call that resolves it.
 *
 * A first version of this guard only waited for the pill to clear before
 * unconditionally re-dispatching — that still double-scrolled: the request
 * that had just resolved while we waited was often already the outcome
 * `check()` wants (e.g. DELIVERED), but nothing re-ran `check()` before
 * firing a brand new gesture on top of it, which could carry the session
 * straight past DELIVERED to AT_TOP before the assertion ever looked.
 * Confirmed live: a 3-run local suite with only the wait-then-redispatch
 * version landed 36/40, with all 4 failures showing exactly this signature
 * (scroll-source-indicator never observed, "No more history available"
 * already showing instead).
 *
 * A second version fixed that by re-running `check()` once *before*
 * dispatching anything new, but bounded the pill-hidden wait at 12s per
 * inner attempt -- combined with up to two `check()` calls (each up to the
 * per-site timeout) in the same `.toPass()` iteration, a single slow-to-
 * settle attempt could burn most of the outer 20s budget, leaving no room
 * left for the plain "gesture fired before AppScrollGate's preconditions
 * settled" retries this helper exists for in the first place (see above).
 * Confirmed live: that version regressed to 33/40, with `.toPass()` itself
 * timing out at 20000ms rather than reproducing the double-scroll symptom.
 *
 * A third version added an explicit bounded wait (3000ms) for the pill to
 * clear before re-checking -- correct (no double-dispatch symptom recurred),
 * but still too expensive: an explicit wait inside a single `.toPass()`
 * attempt is *extra* time on top of that attempt's own `check()` poll, and
 * under this machine's real load (documented run-to-run variance) that
 * extra wait ate into the outer budget on the exact "gate not ready yet"
 * runs this helper's retry loop exists to ride out (see above) — the
 * settling window doc'd there is ~1-2s, needing several cheap ~500-1000ms
 * retries, not a few 3000ms-padded ones. Confirmed live across 3 runs:
 * 36/40, 38/40, 34/40 -- no recurrence of the wrong-outcome symptom, but a
 * new cluster of genuine `.toPass()` timeouts on first-scroll (settling)
 * cases that the pre-guard code didn't have trouble with.
 *
 * This version drops the explicit wait entirely: skip dispatching only on
 * an attempt where the pill is *already* visible right now (a request is
 * genuinely in flight), then fall straight into `check()` regardless --
 * its own polling naturally absorbs any remaining wait for that pending
 * request, and `.toPass()`'s cheap interval (500ms/1000ms) carries the loop
 * forward with no added delay. A fresh gesture only ever fires on an
 * attempt where the pill reads clear going in, which is exactly when one is
 * unambiguously safe to send.
 */
export async function scrollUpUntilVisible(
  page: Page,
  detail: SessionDetailPage,
  check: () => Promise<void>,
  opts: { timeout?: number; intervals?: number[] } = {},
): Promise<void> {
  const pill = detail.getScrollLoadingPill();
  await expect(async () => {
    if (!(await pill.isVisible().catch(() => false))) {
      await scrollUpOnTerminal(page, detail);
    }
    await check();
  }).toPass({ timeout: opts.timeout ?? 25000, intervals: opts.intervals ?? [500, 1000] });
}

export interface SecondViewer {
  contextB: BrowserContext;
  pageB: Page;
  detailB: SessionDetailPage;
}

/**
 * Attaches a second, fully independent browser context to sessionId (the
 * closest Playwright equivalent to a second real tab) and waits for detailA
 * to observe the resulting MULTIPLE_VIEWERS-eligible state (a visible
 * ConnectionCountIndicator) — the shared setup every MULTIPLE_VIEWERS-outcome
 * spec in this suite needs. Caller is responsible for `await contextB.close()`.
 */
export async function attachSecondViewer(
  browser: Browser,
  detailA: SessionDetailPage,
  sessionId: string,
): Promise<SecondViewer> {
  const contextB = await browser.newContext();
  const pageB = await contextB.newPage();
  await dismissOnboarding(pageB);
  const detailB = new SessionDetailPage(pageB);
  await detailB.gotoSession(sessionId);
  await openAndWaitConnected(pageB, detailB);
  // See connection-count-indicator.spec.ts for why this gets generous
  // headroom (server poll + client debounce round trip).
  await expect(detailA.getConnectionCountIndicator()).toBeVisible({ timeout: 30000 });
  return { contextB, pageB, detailB };
}
