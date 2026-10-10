import type { APIRequestContext, Locator, Page } from '@playwright/test';

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
  const dismissButtons = page.getByTestId('toast').getByRole('button', { name: /Dismiss|Close notification/ });
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
