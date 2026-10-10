// @feature notification:hidden-session-gate, notification-tray
/**
 * T-E2-14 (SM-1, SC-1): when a hidden session finishes, nothing about it reaches the
 * toast deck, the tray or the Notifications page, and the server never stores a row.
 *
 * The hidden session is real: tests/e2e/seed-hidden writes it into the test database before
 * boot, so the delivery gate's visibility index resolves it as hidden. A completion of a
 * visible session is sent right after as the control, which also proves the stream and the
 * history pipeline were alive when the hidden one was dropped.
 */
import { test, expect, type APIRequestContext } from '@playwright/test';
import {
  HIDDEN_GATE_FLAG,
  NotificationTray,
  SEEDED_HIDDEN_SESSION,
  TRAY_V2_FLAG,
  sendNotification,
  setFeatureFlag,
} from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';

test.use(profiles.V1.use);

/** The stored history as one string; SendNotification resolves a title to the session UUID, so rows are matched by text. */
async function storedHistoryText(request: APIRequestContext): Promise<string> {
  const response = await request.post(`${BASE_URL}/api/session.v1.SessionService/GetNotificationHistory`, {
    headers: { 'Content-Type': 'application/json' },
    data: { limit: 500 },
  });
  return JSON.stringify(await response.json());
}

test.describe('hidden session stays quiet', () => {
  test.beforeEach(async ({ page, request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
    await setFeatureFlag(request, BASE_URL, HIDDEN_GATE_FLAG, true);
    await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  });

  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, HIDDEN_GATE_FLAG, false);
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  test('hidden_session_completion_should_leave_toast_panel_and_notifications_page_empty_when_session_finishes', async ({
    page,
    request,
  }) => {
    const stamp = Date.now();
    const hiddenTitle = `Review finished ${stamp}`;
    const controlTitle = `Visible finished ${stamp}`;
    const controlSession = `e2e-visible-complete-${stamp}`;

    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });

    // A visible failure is retried until the live stream is attached and shows its toast.
    await expect(async () => {
      await sendNotification(request, BASE_URL, {
        sessionId: `${controlSession}-probe`,
        type: 'ERROR',
        title: `Stream probe ${stamp}`,
      });
      await expect(page.getByTestId('toast').filter({ hasText: `Stream probe ${stamp}` }).first()).toBeVisible({
        timeout: 1_500,
      });
    }).toPass({ timeout: 20_000 });

    await sendNotification(request, BASE_URL, { sessionId: controlSession, type: 'TASK_COMPLETE', title: controlTitle });
    // The versioned stamp is what a current ssq-notify sends; without it the gate treats the
    // event's type as untrusted and delivers it (fail open).
    await sendNotification(request, BASE_URL, {
      sessionId: SEEDED_HIDDEN_SESSION,
      type: 'TASK_COMPLETE',
      title: hiddenTitle,
      metadata: { ssq_notify_schema: '2' },
    });
    // A visible failure sent after the hidden completion is the ordering marker: once its
    // toast shows, the hidden completion has had every chance to arrive.
    const marker = `Marker failure ${stamp}`;
    await sendNotification(request, BASE_URL, { sessionId: `${controlSession}-marker`, type: 'ERROR', title: marker });
    await expect(page.getByTestId('toast').filter({ hasText: marker })).toBeVisible({ timeout: 15_000 });
    await expect.poll(() => storedHistoryText(request)).toContain(controlTitle);

    await expect(page.getByTestId('toast').filter({ hasText: hiddenTitle })).toHaveCount(0);
    expect(await storedHistoryText(request)).not.toContain(hiddenTitle);

    const tray = new NotificationTray(page);
    await tray.handle.click();
    await tray.expectOpen();
    await tray.dismissWhatChanged();
    await expect(tray.tray.getByText(controlTitle).first()).toBeVisible();
    await expect(tray.tray.getByText(hiddenTitle)).toHaveCount(0);

    await page.goto(`${BASE_URL}/notifications`, { waitUntil: 'domcontentloaded' });
    await expect(page.getByText(controlTitle).first()).toBeVisible();
    await expect(page.getByText(hiddenTitle)).toHaveCount(0);
  });
});
