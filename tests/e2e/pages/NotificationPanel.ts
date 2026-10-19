import path from 'path';
import { expect, test, type APIRequestContext, type Locator, type Page } from '@playwright/test';
import { SessionClient } from '../helpers/session-client';

// Closes anything that can steal keyboard focus from the session list: the
// Notification Panel (if auto-opened) and any visible toast alerts. Both
// fire on the same "session creation failed" event accessibility.spec.ts's
// Cancel/Retry test provokes, and either can intercept a Tab-key walk before
// it reaches the session list (BUG-097). Call before (and periodically
// during) any Tab walk that exercises the session list.
export async function dismissNotificationInterference(page: Page): Promise<void> {
  const panelClose = page.getByRole('button', { name: 'Close notification panel' });
  if (await panelClose.isVisible().catch(() => false)) {
    await panelClose.click().catch(() => {});
  }

  // Toasts can stack; dismiss all currently visible ones. Bounded at 5
  // (independent of any caller's own retry count) so a stuck/reappearing
  // toast can't hang the test -- warn rather than fail if it's still there,
  // since this is best-effort mitigation, not the assertion under test.
  const dismissButtons = page.getByTestId('toast').getByRole('button', { name: /Dismiss|Close notification|Move to tray/ });
  for (let i = 0; i < 5; i++) {
    if ((await dismissButtons.count()) === 0) return;
    await dismissButtons.first().click().catch(() => {});
  }
  if ((await dismissButtons.count()) > 0) {
    console.warn('[a11y] dismissNotificationInterference: a toast is still visible after 5 dismiss attempts');
  }
}

// ---------------------------------------------------------------------------
// Toast deck
// ---------------------------------------------------------------------------


export const HIDDEN_GATE_FLAG = 'hidden_session_gate';
/** Title of the hidden review session seeded before the test server boots (tests/e2e/seed-hidden). */
export const SEEDED_HIDDEN_SESSION = 'e2e-hidden-review';

/** Live-sets a feature flag through the same RPC Settings > Features calls. */
export async function setFeatureFlag(
  request: APIRequestContext,
  baseUrl: string,
  name: string,
  enabled: boolean,
): Promise<void> {
  const response = await request.post(`${baseUrl}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { 'Content-Type': 'application/json' },
    data: { name, enabled },
  });
  if (!response.ok()) throw new Error(`UpdateFeatureFlag ${name}=${enabled} failed: ${response.status()}`);
}

export interface SeededNotification {
  sessionId: string;
  type: 'ERROR' | 'WARNING' | 'APPROVAL_NEEDED' | 'CUSTOM' | 'INFO' | 'TASK_COMPLETE';
  title: string;
  message?: string;
  metadata?: Record<string, string>;
}

/** Publishes one notification through SendNotification (localhost only; external session ids are accepted). */
export async function sendNotification(
  request: APIRequestContext,
  baseUrl: string,
  n: SeededNotification,
): Promise<void> {
  const response = await request.post(`${baseUrl}/api/session.v1.SessionService/SendNotification`, {
    headers: { 'Content-Type': 'application/json' },
    data: {
      sessionId: n.sessionId,
      notificationType: `NOTIFICATION_TYPE_${n.type}`,
      priority: 'NOTIFICATION_PRIORITY_HIGH',
      title: n.title,
      message: n.message ?? n.title,
      metadata: n.metadata ?? {},
    },
  });
  if (!response.ok()) throw new Error(`SendNotification ${n.title} failed: ${response.status()}`);
}

/** The capped toast deck: cards, the "+N more" chip and the single bulk control. */
export class ToastDeck {
  readonly deck: Locator;
  readonly toasts: Locator;
  readonly chip: Locator;
  readonly moveAll: Locator;
  readonly undoBar: Locator;
  readonly undo: Locator;

  constructor(private readonly page: Page) {
    this.deck = page.getByTestId('toast-stack');
    this.toasts = page.getByTestId('toast');
    this.chip = page.getByTestId('toast-overflow-chip');
    this.moveAll = page.getByTestId('toast-move-all-to-tray');
    this.undoBar = page.getByTestId('toast-undo-bar');
    this.undo = page.getByTestId('toast-undo-move');
  }
}

// ---------------------------------------------------------------------------
// Notification tray
// ---------------------------------------------------------------------------

/**
 * Resolves once the tray's scroll region has been quiet for `quietMs`. The tray swallows a press
 * that starts within 120 ms of a scroll event (fling guard, useScrollSettle), and a layout shift
 * (a strip, banner or rows appearing) fires one, so a click straight after such a shift is dropped.
 */
export async function waitForTrayScrollIdle(page: Page, quietMs = 200): Promise<void> {
  await page.getByTestId('notification-tray').getByRole('tabpanel').evaluate(
    (el, quiet) =>
      new Promise<void>((resolve) => {
        let timer = window.setTimeout(done, quiet);
        function done() {
          el.removeEventListener('scroll', onScroll);
          resolve();
        }
        function onScroll() {
          window.clearTimeout(timer);
          timer = window.setTimeout(done, quiet);
        }
        el.addEventListener('scroll', onScroll);
      }),
    quietMs,
  );
}

/** Clicks a control inside the tray once the scroll region is quiet, so the fling guard cannot eat the press. */
export async function clickWhenScrollIdle(page: Page, control: Locator): Promise<void> {
  await waitForTrayScrollIdle(page);
  await control.click();
}

/** The non-modal tray, its entry points and the terminal facts its invariants are about. */
export class NotificationTray {
  readonly tray: Locator;
  readonly handle: Locator;
  readonly entry: Locator;
  readonly entryOpen: Locator;
  readonly rows: Locator;
  readonly needsAttentionHeader: Locator;
  readonly undoBar: Locator;
  readonly undo: Locator;
  readonly overflow: Locator;
  readonly keptLine: Locator;
  readonly confirm: Locator;

  constructor(private readonly page: Page) {
    this.tray = page.getByTestId('notification-tray');
    this.handle = page.getByTestId('tray-handle');
    this.entry = page.getByTestId('tray-entry');
    // The phone has no bell chip; the bottom nav's Alerts tab is its standing way into the tray.
    this.entryOpen = page.getByTestId('nav-alerts');
    this.rows = this.tray.getByTestId('tray-row');
    this.needsAttentionHeader = this.tray.getByTestId('tray-needs-attention-header');
    this.undoBar = this.tray.getByTestId('tray-undo-bar');
    this.undo = this.tray.getByTestId('tray-undo');
    this.overflow = this.tray.getByTestId('tray-overflow');
    this.keptLine = this.tray.getByTestId('tray-kept-line');
    this.confirm = this.tray.getByTestId('tray-confirm');
  }

  async expectOpen(): Promise<void> {
    await expect(this.tray).toHaveAttribute('data-state', 'open');
  }

  async expectClosed(): Promise<void> {
    await expect(this.tray).toHaveAttribute('data-state', 'closed');
  }

  /** A phone opens the tray as a ~220px peek sheet with no room for rows; expand it to reach them. */
  async expandIfPeek(): Promise<void> {
    if ((await this.tray.getAttribute('data-sheet')) === 'peek') {
      await this.tray.getByTestId('tray-expand').click();
      await expect(this.tray).toHaveAttribute('data-sheet', 'expanded');
    }
  }

  /** See `waitForTrayScrollIdle`. */
  async waitForScrollIdle(quietMs = 200): Promise<void> {
    await waitForTrayScrollIdle(this.page, quietMs);
  }

  async clickMenuItem(item: 'clear-informational' | 'clear-history' | 'settings'): Promise<void> {
    await this.overflow.click();
    await this.tray.getByTestId(`tray-menu-${item}`).click();
  }

  /** Dismisses the one-time "What changed" card so it does not sit above the list in layout checks. */
  async dismissWhatChanged(): Promise<void> {
    const dismiss = this.tray.getByTestId('tray-what-changed-dismiss');
    if (await dismiss.isVisible().catch(() => false)) await dismiss.click();
  }
}

const ONBOARDED_STORAGE_KEY = 'stapler-squad:onboarded';

/** Marks the app onboarded for this page, loads it and opens the tray (expanded on a phone). */
export async function openNotificationTray(page: Page, baseUrl: string): Promise<NotificationTray> {
  await page.addInitScript((k) => localStorage.setItem(k, 'true'), ONBOARDED_STORAGE_KEY);
  const tray = new NotificationTray(page);
  await page.goto(baseUrl, { waitUntil: 'domcontentloaded' });
  const entry = tray.handle.or(new ToastDeck(page).chip).or(tray.entryOpen).locator('visible=true').first();
  // The entry is server-rendered before it hydrates, so an early click can be lost; click until open.
  await expect
    .poll(
      async () => {
        if ((await tray.tray.getAttribute('data-state')) === 'open') return true;
        await entry.click();
        return false;
      },
      { timeout: 15_000 },
    )
    .toBe(true);
  await tray.expandIfPeek();
  await tray.dismissWhatChanged();
  return tray;
}

export const INLINE_ACTIONS_FLAG = 'notification_inline_actions';

/** Resolves once the element's box has not moved or resized for `quietMs` (the row's settled-layout window is 300 ms). */
async function waitForStableBox(control: Locator, quietMs = 400): Promise<void> {
  await control.evaluate(
    (el, quiet) =>
      new Promise<void>((resolve) => {
        let key = '';
        let since = performance.now();
        const tick = () => {
          const r = el.getBoundingClientRect();
          const now = `${r.x},${r.y},${r.width},${r.height}`;
          if (now !== key) {
            key = now;
            since = performance.now();
          }
          if (performance.now() - since >= quiet) resolve();
          else requestAnimationFrame(tick);
        };
        tick();
      }),
    quietMs,
  );
}

/**
 * One tray row's inline actions (ActionRow). A control that just rendered or moved ignores a press for
 * 300 ms and the tray drops a press right after a scroll, so `press` scrolls, then waits for both before it clicks.
 */
export class InlineActionRow {
  readonly row: Locator;
  readonly actions: Locator;
  readonly status: Locator;
  readonly approve: Locator;
  readonly deny: Locator;
  readonly sendDenial: Locator;
  readonly undo: Locator;
  readonly unavailableReason: Locator;

  constructor(
    private readonly page: Page,
    private readonly text: string,
  ) {
    this.row = page.getByTestId('notification-tray').getByTestId('tray-row').filter({ hasText: text });
    this.actions = this.row.getByTestId('action-row');
    this.status = this.row.getByTestId('action-status');
    this.approve = this.row.getByTestId('action-approve');
    this.deny = this.row.getByTestId('action-deny');
    this.sendDenial = this.row.getByTestId('action-send-denial');
    this.undo = this.row.getByTestId('action-undo');
    this.unavailableReason = this.row.getByTestId('action-unavailable-reason');
  }

  /** The list is virtualized and ranks by urgency, so search for the row's text to bring it into the DOM. */
  async reveal(): Promise<void> {
    await this.page.getByTestId('notification-tray').getByRole('searchbox', { name: 'Search notifications' }).fill(this.text);
    await expect(this.row).toHaveCount(1);
  }

  /** An answer or decision option button, by its label. */
  option(label: string | RegExp): Locator {
    return this.row.locator('[data-action-option]').filter({ hasText: label });
  }

  async press(control: Locator, { tap = false }: { tap?: boolean } = {}): Promise<void> {
    await expect(control).toBeVisible();
    // Scroll first: the click's own scroll-into-view would fire a scroll event right before the press.
    await control.scrollIntoViewIfNeeded();
    await waitForStableBox(control);
    await waitForTrayScrollIdle(this.page);
    if (tap) await control.tap();
    else await control.click();
  }
}

/** The tray's Background segment (Story 5.4). */
export class BackgroundSegment {
  readonly tab: Locator;
  readonly notificationsTab: Locator;
  readonly rows: Locator;
  readonly rollup: Locator;
  readonly stateChips: Locator;
  readonly connectivity: Locator;
  readonly refresh: Locator;
  readonly error: Locator;
  readonly emptyHealthy: Locator;
  readonly failureStrip: Locator;
  readonly markAll: Locator;
  readonly older: Locator;

  constructor(page: Page) {
    this.tab = page.getByTestId('tray-tab-background');
    this.notificationsTab = page.getByTestId('tray-tab-notifications');
    this.rows = page.getByTestId('background-row');
    this.rollup = page.getByTestId('background-rollup');
    this.stateChips = page.getByTestId('background-state-chip');
    this.connectivity = page.getByTestId('background-connectivity');
    this.refresh = page.getByTestId('background-refresh');
    this.error = page.getByTestId('background-error');
    this.emptyHealthy = page.getByTestId('background-empty-healthy');
    this.failureStrip = page.getByTestId('background-failure-strip');
    this.markAll = page.getByRole('button', { name: 'Mark all reviewed' });
    this.older = page.getByTestId('background-older-toggle');
  }
}

/** Counts frames the page sends over any WebSocket; a terminal resize vote is one frame. */
export async function installWebSocketSendCounter(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const w = window as unknown as { __wsSends: number };
    w.__wsSends = 0;
    const original = WebSocket.prototype.send;
    WebSocket.prototype.send = function (this: WebSocket, data: Parameters<WebSocket['send']>[0]) {
      w.__wsSends += 1;
      return original.call(this, data);
    };
  });
}

export function webSocketSends(page: Page): Promise<number> {
  return page.evaluate(() => (window as unknown as { __wsSends: number }).__wsSends);
}

export interface TerminalFacts {
  cols: number | null;
  rows: number;
  hasProbe: boolean;
  text: string;
}

/** Marks the terminal root so a later read can prove it is the same DOM node. */
export async function markTerminalRoot(page: Page): Promise<void> {
  await page.evaluate(() => {
    document.querySelector('.xterm')?.setAttribute('data-ssq-probe', 'same-node');
  });
}

/**
 * Terminal text and row counts are read from xterm's DOM renderer. The `chromium` project loads
 * SwiftShader WebGL, which paints a canvas and leaves `.xterm-rows` empty, so a spec that waits
 * for terminal text runs only in the `chromium-dom` project (the repo convention).
 */
export function skipWithoutDomRenderer(): void {
  test.skip(
    !test.info().project.name.includes('dom'),
    'terminal text is read from the DOM renderer; runs in the chromium-dom project',
  );
}

export async function readTerminalFacts(page: Page): Promise<TerminalFacts> {
  return page.evaluate(() => {
    const screen = document.querySelector('.xterm-screen') as HTMLElement | null;
    const ruler = document.querySelector('.xterm-char-measure-element') as HTMLElement | null;
    const cell = ruler ? ruler.getBoundingClientRect().width : 0;
    const rowEls = Array.from(document.querySelectorAll('.xterm-rows > div'));
    return {
      cols: screen && cell > 0 ? Math.round(screen.getBoundingClientRect().width / cell) : null,
      rows: rowEls.length,
      hasProbe: document.querySelector('.xterm')?.getAttribute('data-ssq-probe') === 'same-node',
      text: rowEls.map((r) => r.textContent || '').join('\n'),
    };
  });
}

const ONBOARDED_KEY = 'stapler-squad:onboarded';

/**
 * Opens a real session whose terminal is filled to the bottom (prompt on the last row) and waits
 * for it to render. Call `installWebSocketSendCounter` first when the spec counts frames.
 */
export async function openTerminalSession(
  page: Page,
  baseUrl: string,
  { marker, titlePrefix, waitForText = true }: { marker: string; titlePrefix: string; waitForText?: boolean },
): Promise<{ client: SessionClient; session: { id: string } }> {
  if (waitForText) skipWithoutDomRenderer();
  const client = new SessionClient(baseUrl);
  const session = await client.createSession({
    title: `${titlePrefix}-${Date.now()}`,
    path: '/tmp',
    program: `sh ${path.join(__dirname, '..', 'fixtures', 'tray-terminal-fixture.sh')} ${marker}`,
  });
  await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  await page.goto(`${baseUrl}/?session=${session.id}`, { waitUntil: 'domcontentloaded' });
  await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached({ timeout: 20_000 });
  if (waitForText) {
    await expect.poll(async () => (await readTerminalFacts(page)).text, { timeout: 15_000 }).toContain(marker);
  }
  await markTerminalRoot(page);
  return { client, session };
}

/** Waits until the page's WebSocket send counter has stopped moving (two samples apart are equal). */
export async function waitForIdleFrames(page: Page, intervalMs = 300): Promise<number> {
  let last = -1;
  await expect
    .poll(
      async () => {
        const sends = await webSocketSends(page);
        const settled = sends === last;
        last = sends;
        return settled;
      },
      { intervals: [intervalMs], timeout: 10_000 },
    )
    .toBe(true);
  return last;
}
