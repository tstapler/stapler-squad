// @feature notification-toast-stack, notification-tray
/**
 * Bulk actions across two open tabs (Stories 3.4 and 3.9). "Move all to tray" in
 * one tab drops exactly the same toasts from the other, leaves its history alone,
 * and a toast that arrives afterwards is unaffected. Epic 4 adds the tray's
 * "Clear informational" cases to this file.
 */
import { test, expect, type APIRequestContext } from '@playwright/test';
import { NotificationTray, ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';

test.use(profiles.V1.use);

test.describe('notification bulk actions across tabs', () => {
  test.beforeEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  test('second_tab_should_drop_exactly_broadcast_ids_and_keep_late_arrival_when_move_all_in_first_tab', async ({ context, request }) => {
    await context.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
    const prefix = `toast-xtab-${Date.now()}`;
    const tabA = await context.newPage();
    const tabB = await context.newPage();
    const deckA = new ToastDeck(tabA);
    const deckB = new ToastDeck(tabB);
    await Promise.all([
      tabA.goto(BASE_URL, { waitUntil: 'domcontentloaded' }),
      tabB.goto(BASE_URL, { waitUntil: 'domcontentloaded' }),
    ]);

    // Retry the first send until both tabs' streams are attached and show it.
    await expect(async () => {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-0`, type: 'ERROR', title: 'Failure 0' });
      await expect(deckA.toasts.first()).toBeVisible({ timeout: 1_500 });
      await expect(deckB.toasts.first()).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 20_000 });
    await sendNotification(request, BASE_URL, { sessionId: `${prefix}-1`, type: 'ERROR', title: 'Failure 1' });
    await expect(deckA.moveAll).toBeVisible();
    await expect(deckB.moveAll).toBeVisible();

    await deckA.moveAll.click();
    await expect(deckA.toasts).toHaveCount(0);
    await expect(deckB.toasts).toHaveCount(0);
    await expect(deckB.moveAll).toHaveCount(0);

    // A toast that arrives after the click is shown in both tabs.
    await sendNotification(request, BASE_URL, { sessionId: `${prefix}-late`, type: 'ERROR', title: 'Late failure' });
    await expect(deckA.toasts).toHaveCount(1);
    await expect(deckB.toasts).toHaveCount(1);
  });
});

interface HistoryRow {
  id: string;
  sessionId: string;
  isRead?: boolean;
  isPendingDecision?: boolean;
}

async function historyFor(request: APIRequestContext, prefix: string): Promise<HistoryRow[]> {
  const response = await request.post(`${BASE_URL}/api/session.v1.SessionService/GetNotificationHistory`, {
    headers: { 'Content-Type': 'application/json' },
    data: { limit: 500 },
  });
  const body = (await response.json()) as { notifications?: HistoryRow[] };
  return (body.notifications ?? []).filter((n) => (n.sessionId ?? '').startsWith(prefix));
}

const CLEAR_RPC = 'ClearNotificationHistory';

test.describe('tray bulk actions with seeded decisions (Story 4.4)', () => {
  test.beforeEach(async ({ page, request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
    await page.addInitScript(
      ([onboarded, undoKey]) => {
        localStorage.setItem(onboarded, 'true');
        localStorage.setItem(undoKey, '5000'); // the shortest undo window keeps the spec fast
      },
      [ONBOARDED_KEY, 'ssq.notifications.undoWindow'],
    );
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  async function seedMixed(request: APIRequestContext, prefix: string) {
    for (let i = 0; i < 5; i++) {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-info-${i}`, type: 'CUSTOM', title: `Info ${i}` });
    }
    for (let i = 0; i < 2; i++) {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-err-${i}`, type: 'ERROR', title: `Failure ${i}` });
    }
    // The subscriber coalesces for ~500ms before persisting; wait until every row is stored.
    await expect.poll(async () => (await historyFor(request, prefix)).length, { timeout: 10_000 }).toBe(7);
  }

  async function openTray(page: import('@playwright/test').Page) {
    const tray = new NotificationTray(page);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await expect(tray.handle).toBeVisible();
    await tray.handle.click();
    await tray.expectOpen();
    await tray.dismissWhatChanged();
    return tray;
  }

  test('clear_informational_should_delete_only_informational_rows_after_undo_window_and_keep_pending_decisions', async ({ page, request }) => {
    const prefix = `tray-clear-${Date.now()}`;
    await seedMixed(request, prefix);
    const tray = await openTray(page);
    await expect(tray.tray.getByTestId('tray-needs-attention')).toContainText('need attention');

    await tray.clickMenuItem('clear-informational');
    await expect(tray.confirm).toContainText(/\d+ awaiting decision kept/);
    await expect(tray.confirm.getByTestId('tray-confirm-cancel')).toBeFocused();
    await tray.confirm.getByTestId('tray-confirm-ok').click();
    await expect(tray.undoBar).toBeVisible();

    // Nothing is deleted while the window is open.
    expect((await historyFor(request, prefix)).filter((n) => (n.sessionId ?? '').includes('-info-'))).toHaveLength(5);
    await expect(tray.undoBar).toBeHidden({ timeout: 15_000 });

    await expect.poll(async () => (await historyFor(request, prefix)).filter((n) => (n.sessionId ?? '').includes('-info-')).length).toBe(0);
    const remaining = await historyFor(request, prefix);
    expect(remaining.filter((n) => (n.sessionId ?? '').includes('-err-'))).toHaveLength(2);
    expect(remaining.every((n) => n.isPendingDecision)).toBe(true);
  });

  test('clear_informational_should_send_no_rpc_when_undo_is_tapped', async ({ page, request }) => {
    const prefix = `tray-undo-${Date.now()}`;
    await seedMixed(request, prefix);
    const clears: string[] = [];
    page.on('request', (r) => {
      if (r.url().includes(CLEAR_RPC)) clears.push(r.url());
    });
    const tray = await openTray(page);
    await tray.clickMenuItem('clear-informational');
    await tray.confirm.getByTestId('tray-confirm-ok').click();
    await tray.undo.click();
    await page.waitForTimeout(6_000); // longer than the 5s window
    expect(clears).toHaveLength(0);
    expect((await historyFor(request, prefix)).filter((n) => (n.sessionId ?? '').includes('-info-'))).toHaveLength(5);
  });

  test('clear_informational_should_complete_with_keepalive_when_the_page_is_hidden_during_the_window', async ({ page, request }) => {
    const prefix = `tray-hide-${Date.now()}`;
    await seedMixed(request, prefix);
    const tray = await openTray(page);
    await tray.clickMenuItem('clear-informational');
    const clearRequest = page.waitForRequest((r) => r.url().includes(CLEAR_RPC), { timeout: 4_000 });
    await tray.confirm.getByTestId('tray-confirm-ok').click();
    await expect(tray.undoBar).toBeVisible();

    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await clearRequest; // sent immediately, long before the 5s window would end
    await expect
      .poll(async () => (await historyFor(request, prefix)).filter((n) => (n.sessionId ?? '').includes('-info-')).length)
      .toBe(0);
  });

  test('clear_history_should_use_inline_confirm_never_the_native_dialog_and_keep_unread_decisions', async ({ page, request }) => {
    const prefix = `tray-history-${Date.now()}`;
    await seedMixed(request, prefix);
    let dialogs = 0;
    page.on('dialog', async (d) => {
      dialogs += 1;
      await d.dismiss();
    });
    const tray = await openTray(page);
    // Read the informational rows first so "Clear history" has something to clear.
    await tray.tray.getByRole('button', { name: 'Mark activity read' }).click();

    await tray.clickMenuItem('clear-history');
    await expect(tray.confirm).toContainText("This can't be undone");
    await expect(tray.confirm.getByTestId('tray-confirm-cancel')).toBeFocused();
    await tray.confirm.getByTestId('tray-confirm-ok').click();
    await expect(tray.confirm).toHaveCount(0);

    expect(dialogs).toBe(0);
    await expect.poll(async () => (await historyFor(request, prefix)).filter((n) => (n.sessionId ?? '').includes('-info-')).length).toBe(0);
    expect((await historyFor(request, prefix)).filter((n) => (n.sessionId ?? '').includes('-err-'))).toHaveLength(2);
  });

  test('stale_client_pending_id_should_be_kept_by_the_server_and_reported', async ({ request }) => {
    const prefix = `tray-guard-${Date.now()}`;
    await seedMixed(request, prefix);
    const rows = await historyFor(request, prefix);
    const err = rows.find((n) => (n.sessionId ?? '').includes('-err-'))!;
    const info = rows.find((n) => (n.sessionId ?? '').includes('-info-'))!;
    const response = await request.post(`${BASE_URL}/api/session.v1.SessionService/ClearNotificationHistory`, {
      headers: { 'Content-Type': 'application/json' },
      data: { notificationIds: [err.id, info.id] },
    });
    const body = (await response.json()) as { clearedCount?: number; keptIds?: string[] };
    expect(body.clearedCount).toBe(1);
    expect(body.keptIds).toEqual([err.id]);
  });
});
