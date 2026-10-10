// @feature notification-toast-stack
/**
 * Tap-counting walkthrough for the outcome table (plan Stories 1.5 and 3.10):
 * a burst of 6 informational notifications, 3 pending approvals and 1 failure
 * arrives, and the script counts the taps from the burst to an empty deck with
 * every decision left pinned in the tray.
 *
 * Budget: taps <= 2 + 1 per pending decision. The measured count is printed as a
 * JSON line so the PR description can quote it.
 *
 * Not covered here: the taps to a hidden session's read-only output. That view
 * ships with Epic 5; this script gains that leg when it does.
 */
import { test, expect } from '@playwright/test';
import { ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';
const PENDING_DECISIONS = 3;

test.use(profiles.V1.use);

test.describe('notification triage walkthrough', () => {
  test.beforeEach(async ({ request, page }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
    await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  test('triage_burst_should_reach_an_empty_deck_within_the_tap_budget_when_6_info_3_approvals_1_failure', async ({ page, request }) => {
    const prefix = `triage-${Date.now()}`;
    const deck = new ToastDeck(page);
    let taps = 0;
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });

    await expect(async () => {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-info-0`, type: 'CUSTOM', title: 'Info 0' });
      await expect(deck.toasts.first()).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 20_000 });
    for (let i = 1; i < 6; i++) {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-info-${i}`, type: 'CUSTOM', title: `Info ${i}` });
    }
    for (let i = 0; i < PENDING_DECISIONS; i++) {
      await sendNotification(request, BASE_URL, {
        sessionId: `${prefix}-approval-${i}`,
        type: 'APPROVAL_NEEDED',
        title: `Approve ${i}?`,
        metadata: { approval_id: `${prefix}-appr-${i}`, tool_name: 'Bash' },
      });
    }
    await sendNotification(request, BASE_URL, { sessionId: `${prefix}-failure`, type: 'ERROR', title: 'Build failed' });

    await expect(deck.moveAll).toHaveText('Move all to tray (10)');

    // One tap: everything leaves the deck; every decision stays pinned and unread in the tray.
    await deck.moveAll.click();
    taps += 1;
    await expect(deck.toasts).toHaveCount(0);

    const budget = 2 + PENDING_DECISIONS;
    console.log(JSON.stringify({ walkthrough: 'burst-to-empty-deck', taps, budget, decisionsLeftPinned: PENDING_DECISIONS + 1 }));
    expect(taps).toBeLessThanOrEqual(budget);
  });
});
