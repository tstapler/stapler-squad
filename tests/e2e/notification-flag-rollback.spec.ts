// @feature notification-toast-stack
/**
 * Rollback by flag (Story 3.10, ux FG-2/FG-3): toggling notification_tray_v2 in
 * Settings > Features switches the toast list between the capped deck and the
 * legacy uncapped list on the next render, with no reload, and the displayed flag
 * state is the server's read-back.
 */
import { test, expect } from '@playwright/test';
import { ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';
const FLAG_LABEL = 'Notifications: capped toast deck with Move all to tray';

test.use(profiles.V1.use);

test.describe('notification flag rollback', () => {
  test.beforeEach(async ({ request, page }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
    await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  test('toggling_tray_flag_off_should_restore_legacy_list_without_reload_and_show_server_readback', async ({ page, request }) => {
    const flagRow = page.getByTestId('feature-flag-row').filter({
      has: page.getByTestId('feature-flag-name').getByText(FLAG_LABEL),
    });

    // Turn it on from the Settings page and read the state the server reports back.
    await page.goto(`${BASE_URL}/settings/features`, { waitUntil: 'domcontentloaded' });
    await page.getByRole('button', { name: `Enable ${FLAG_LABEL}` }).click();
    await expect(flagRow.getByTestId('feature-flag-status')).toHaveText('On');

    const readBack = await request.post(`${BASE_URL}/api/session.v1.SessionService/GetFeatureFlags`, {
      headers: { 'Content-Type': 'application/json' },
      data: {},
    });
    const flags = ((await readBack.json()) as { flags: Array<{ name: string; enabled?: boolean }> }).flags;
    expect(flags.find((f) => f.name === TRAY_V2_FLAG)?.enabled).toBe(true);

    // With the deck on, a burst is capped at 3. Toasts render on every page, so stay on Settings.
    const prefix = `flag-rollback-${Date.now()}`;
    const deck = new ToastDeck(page);
    await expect(async () => {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-0`, type: 'ERROR', title: 'Failure 0' });
      await expect(deck.toasts.first()).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 20_000 });
    for (let i = 1; i < 5; i++) {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-${i}`, type: 'ERROR', title: `Failure ${i}` });
    }
    await expect(deck.toasts).toHaveCount(3);
    await expect(deck.chip).toHaveText('+2 more');

    // Turn it off in the same page: the legacy uncapped list renders on the next render, no reload.
    await page.getByRole('button', { name: `Disable ${FLAG_LABEL}` }).click();
    await expect(flagRow.getByTestId('feature-flag-status')).toHaveText('Off');
    await expect(deck.toasts).toHaveCount(5);
    await expect(deck.deck).toHaveCount(0);
    await expect(deck.chip).toHaveCount(0);

    const afterOff = await request.post(`${BASE_URL}/api/session.v1.SessionService/GetFeatureFlags`, {
      headers: { 'Content-Type': 'application/json' },
      data: {},
    });
    const offFlags = ((await afterOff.json()) as { flags: Array<{ name: string; enabled?: boolean }> }).flags;
    expect(offFlags.find((f) => f.name === TRAY_V2_FLAG)?.enabled ?? false).toBe(false);
  });
});
