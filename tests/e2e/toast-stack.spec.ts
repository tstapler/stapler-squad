// @feature notification-toast-stack
/**
 * Capped toast deck on desktop (Stories 3.3, 3.4, 3.10): at most 3 cards, a
 * "+N more" chip, and one "Move all to tray" control that clears the deck without
 * deleting or reading anything. Notifications go through SendNotification, so the
 * server records history exactly as it does for a real producer.
 */
import { test, expect, type APIRequestContext } from '@playwright/test';
import { ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';

test.use(profiles.V1.use);

interface HistoryRow {
  id: string;
  sessionId: string;
  isRead?: boolean;
  isPendingDecision?: boolean;
}

async function historyFor(request: APIRequestContext, prefix: string): Promise<HistoryRow[]> {
  const response = await request.post(`${BASE_URL}/api/session.v1.SessionService/GetNotificationHistory`, {
    headers: { 'Content-Type': 'application/json' },
    data: { limit: 200 },
  });
  const body = (await response.json()) as { notifications?: HistoryRow[] };
  return (body.notifications ?? []).filter((n) => n.sessionId.startsWith(prefix));
}

test.describe('toast stack (desktop)', () => {
  test.beforeEach(async ({ page, request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
    await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  });

  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  test('toast_stack_should_show_exactly_3_toasts_and_chip_plus7_when_10_errors_seeded', async ({ page, request }) => {
    const prefix = `toast-cap-${Date.now()}`;
    const deck = new ToastDeck(page);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });

    // The first send is retried until the live stream is attached and delivers it.
    await expect(async () => {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-0`, type: 'ERROR', title: 'Failure 0' });
      await expect(deck.toasts.first()).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 20_000 });

    for (let i = 1; i < 10; i++) {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-${i}`, type: 'ERROR', title: `Failure ${i}` });
    }

    await expect(deck.toasts).toHaveCount(3);
    await expect(deck.chip).toHaveText('+7 more');
    await expect(deck.chip).toHaveAccessibleName('7 more notifications, open tray');
  });

  test('toast_stack_should_show_3_chip_plus4_and_keep_history_when_6_info_1_approval_move_all', async ({ page, request }) => {
    const prefix = `toast-move-${Date.now()}`;
    const deck = new ToastDeck(page);
    const historyMutations: string[] = [];
    page.on('request', (r) => {
      if (/MarkNotificationRead|ClearNotificationHistory/.test(r.url())) historyMutations.push(r.url());
    });
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });

    await expect(async () => {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-info-0`, type: 'CUSTOM', title: 'Info 0' });
      await expect(deck.toasts.first()).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 20_000 });
    for (let i = 1; i < 6; i++) {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-info-${i}`, type: 'CUSTOM', title: `Info ${i}` });
    }
    await sendNotification(request, BASE_URL, {
      sessionId: `${prefix}-approval`,
      type: 'APPROVAL_NEEDED',
      title: 'Approve command?',
      metadata: { approval_id: `${prefix}-appr`, tool_name: 'Bash' },
    });

    await expect(deck.toasts).toHaveCount(3);
    await expect(deck.chip).toHaveText('+4 more');
    await expect(deck.moveAll).toHaveText('Move all to tray (7)');

    await deck.moveAll.click();

    await expect(deck.toasts).toHaveCount(0);
    await expect(deck.undoBar).toContainText('Moved 7 to tray');
    // The server persists history asynchronously; wait for all seven rows.
    await expect.poll(async () => (await historyFor(request, prefix)).length, { timeout: 10_000 }).toBe(7);
    const rows = await historyFor(request, prefix);
    const approval = rows.find((r) => r.sessionId === `${prefix}-approval`);
    expect(approval?.isRead ?? false).toBe(false);
    expect(approval?.isPendingDecision).toBe(true);
    expect(historyMutations).toEqual([]);
  });
});
