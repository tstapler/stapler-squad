// @feature notification-toast-stack
/**
 * Bulk actions across two open tabs (Stories 3.4 and 3.9). "Move all to tray" in
 * one tab drops exactly the same toasts from the other, leaves its history alone,
 * and a toast that arrives afterwards is unaffected. Epic 4 adds the tray's
 * "Clear informational" cases to this file.
 */
import { test, expect } from '@playwright/test';
import { ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
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
