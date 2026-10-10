import { expect, type APIRequestContext, type Locator, type Page } from '@playwright/test';

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
// Toast deck (notification_tray_v2)
// ---------------------------------------------------------------------------


export const TRAY_V2_FLAG = 'notification_tray_v2';

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
  type: 'ERROR' | 'WARNING' | 'APPROVAL_NEEDED' | 'CUSTOM';
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
// Notification tray (notification_tray_v2)
// ---------------------------------------------------------------------------

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
    this.entryOpen = page.getByTestId('tray-entry-open');
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

/** The tray's Background segment (Story 5.4). */
export class BackgroundSegment {
  readonly tab: Locator;
  readonly notificationsTab: Locator;
  readonly rows: Locator;
  readonly summary: Locator;
  readonly refresh: Locator;
  readonly error: Locator;
  readonly stale: Locator;
  readonly emptyHealthy: Locator;

  constructor(page: Page) {
    this.tab = page.getByTestId('tray-tab-background');
    this.notificationsTab = page.getByTestId('tray-tab-notifications');
    this.rows = page.getByTestId('background-row');
    this.summary = page.getByTestId('background-summary');
    this.refresh = page.getByTestId('background-refresh');
    this.error = page.getByTestId('background-error');
    this.stale = page.getByTestId('background-stale');
    this.emptyHealthy = page.getByTestId('background-empty-healthy');
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
