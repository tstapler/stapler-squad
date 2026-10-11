// @feature notification-toast-stack
/**
 * Toast deck on a phone (Story 3.7, ADR-009): docked at the top under the session
 * tab row, one card plus a chip, never over the terminal input or page keys, and
 * a one-line chip with the keyboard open. Playwright can only drive the keyboard
 * through the visual-viewport signal; the real soft-keyboard leg is the
 * operator's device check DV-3.
 */
import { test, expect, type Page } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import { ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
import { profiles, setKeyboardOpen } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';

async function openSessionAndSeed(page: Page, request: Parameters<typeof sendNotification>[0], count: number) {
  const client = new SessionClient(BASE_URL);
  const title = `toast-mobile-${Date.now()}`;
  // Fill the screen first so the prompt sits at the bottom, as it does in a long Claude Code session.
  const session = await client.createSession({ title, path: '/tmp', program: "bash -c 'seq 1 300; exec bash'" });
  await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  await page.goto(`${BASE_URL}/?session=${session.id}`, { waitUntil: 'domcontentloaded' });
  await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached({ timeout: 20_000 });

  const deck = new ToastDeck(page);
  await expect(async () => {
    await sendNotification(request, BASE_URL, { sessionId: `${title}-0`, type: 'ERROR', title: 'Failure 0' });
    await expect(deck.toasts.first()).toBeVisible({ timeout: 1_500 });
  }).toPass({ timeout: 20_000 });
  for (let i = 1; i < count; i++) {
    await sendNotification(request, BASE_URL, { sessionId: `${title}-${i}`, type: 'ERROR', title: `Failure ${i}` });
  }
  return { client, session, deck };
}

test.describe('toast stack (phone)', () => {
  test.beforeEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  test.describe('V2 portrait', () => {
    test.use(profiles.V2.use);

    test('toast_stack_should_dock_top_with_at_most_1_toast_clear_of_input_pagekeys_and_status_pill_when_3_toasts', async ({ page, request }) => {
      const { client, session, deck } = await openSessionAndSeed(page, request, 3);
      try {
        await expect(deck.toasts).toHaveCount(1);
        await expect(deck.chip).toHaveText('+2 more');

        const deckBox = await deck.deck.boundingBox();
        const tabBox = await page.getByRole('tab', { name: /Terminal/ }).first().boundingBox();
        const inputBox = await page.getByRole('textbox', { name: 'Terminal input' }).boundingBox();
        const keysBox = await page.getByTestId('mobile-key').first().boundingBox();
        expect(deckBox).not.toBeNull();
        expect(deckBox!.y).toBeGreaterThanOrEqual(tabBox!.y + tabBox!.height - 1);

        const intersects = (a: { x: number; y: number; width: number; height: number }, b: typeof a) =>
          a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;
        if (inputBox) expect(intersects(deckBox!, inputBox)).toBe(false);
        if (keysBox) expect(intersects(deckBox!, keysBox)).toBe(false);

        const toastBox = await deck.toasts.first().boundingBox();
        expect(toastBox!.height).toBeLessThanOrEqual(96);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });

    test('swipe_on_toast_should_dismiss_pinned_to_tray_without_deleting_history_when_touch', async ({ page, request, context }) => {
      const { client, session, deck } = await openSessionAndSeed(page, request, 2);
      try {
        const row = page.getByTestId('toast-row').first();
        const box = await row.boundingBox();
        const y = box!.y + box!.height / 2;
        const startX = box!.x + 40;
        const cdp = await context.newCDPSession(page);
        const touch = (type: 'touchStart' | 'touchMove' | 'touchEnd', x: number) =>
          cdp.send('Input.dispatchTouchEvent', {
            type,
            touchPoints: type === 'touchEnd' ? [] : [{ x, y }],
          });
        await touch('touchStart', startX);
        for (let step = 1; step <= 6; step++) await touch('touchMove', startX + step * 40);
        await touch('touchEnd', startX + 240);

        // The pinned toast leaves the deck (moved to the tray, not deleted); the next one takes its slot.
        await expect(deck.toasts).toHaveCount(1);
        await expect(deck.toasts.first()).toContainText('Failure 1');
        await expect(deck.chip).toHaveCount(0);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });

    test('swipe_should_start_only_on_row_surface_and_never_from_grabber_tab_strip_or_30px_edge_when_touch_sequences_run', async ({ page, request, context }) => {
      const { client, session, deck } = await openSessionAndSeed(page, request, 2);
      try {
        const cdp = await context.newCDPSession(page);
        const swipeFrom = async (x: number, y: number, dx: number) => {
          const touch = (type: 'touchStart' | 'touchMove' | 'touchEnd', px: number) =>
            cdp.send('Input.dispatchTouchEvent', { type, touchPoints: type === 'touchEnd' ? [] : [{ x: px, y }] });
          await touch('touchStart', x);
          for (let step = 1; step <= 6; step++) await touch('touchMove', x + (step * dx) / 6);
          await touch('touchEnd', x + dx);
        };
        await expect(deck.toasts.first()).toContainText('Failure 0');

        // 1. From the tab strip: no toast is dismissed and the tray stays closed.
        const tabBox = (await page.getByRole('tab', { name: /Terminal/ }).first().boundingBox())!;
        await swipeFrom(tabBox.x + 20, tabBox.y + tabBox.height / 2, 240);
        await expect(deck.toasts.first()).toContainText('Failure 0');

        // 2. From within 30px of the screen edge: the same.
        const rowBox = (await page.getByTestId('toast-row').first().boundingBox())!;
        await swipeFrom(10, rowBox.y + rowBox.height / 2, 240);
        await expect(deck.toasts.first()).toContainText('Failure 0');

        // 3. Open the sheet; a horizontal drag that starts on the grabber never dismisses a tray row.
        await page.getByTestId('toast-overflow-chip').or(page.getByTestId('tray-entry-open')).first().click();
        const grabber = page.getByTestId('tray-grabber');
        await expect(grabber).toBeVisible();
        const rows = page.getByTestId('tray-row');
        const rowsBefore = await rows.count();
        const grabberBox = (await grabber.boundingBox())!;
        await swipeFrom(grabberBox.x + grabberBox.width / 2 - 100, grabberBox.y + grabberBox.height / 2, 240);
        await expect(page.getByTestId('tray-undo-bar')).toHaveCount(0);
        await expect(rows).toHaveCount(rowsBefore);

        // 4. A swipe that does start on the row surface dismisses (the positive control).
        await page.getByRole('button', { name: 'Close notification panel' }).click();
        const target = (await page.getByTestId('toast-row').first().boundingBox())!;
        await swipeFrom(target.x + 40, target.y + target.height / 2, 240);
        await expect(deck.toasts.first()).toContainText('Failure 1');
      } finally {
        await client.deleteSession(session.id, true);
      }
    });

    test('toast_stack_should_hold_the_undo_bar_in_the_chip_row_when_move_all_on_a_phone', async ({ page, request }) => {
      const { client, session, deck } = await openSessionAndSeed(page, request, 3);
      try {
        await deck.moveAll.click();
        await expect(deck.toasts).toHaveCount(0);
        await expect(deck.chip).toHaveCount(0);
        await expect(deck.undoBar).toContainText('Moved 3 to tray');
        const undoBox = await deck.undo.boundingBox();
        expect(undoBox!.height).toBeGreaterThanOrEqual(44);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });
  });

  test.describe('V4 portrait with the keyboard open', () => {
    test.use(profiles.V4.use);

    test('toast_stack_should_show_only_44px_chip_and_keep_textarea_focused_when_keyboard_open', async ({ page, request }) => {
      const { client, session, deck } = await openSessionAndSeed(page, request, 3);
      try {
        const textarea = page.getByRole('textbox', { name: 'Terminal input' });
        await textarea.focus();
        await setKeyboardOpen(page, 300);

        await expect(deck.toasts).toHaveCount(0);
        await expect(deck.chip).toContainText('notifications');
        const chipBox = await deck.chip.boundingBox();
        expect(chipBox!.height).toBeGreaterThanOrEqual(44);
        await expect(textarea).toBeFocused();
      } finally {
        await client.deleteSession(session.id, true);
      }
    });
  });

  test.describe('V3 landscape', () => {
    test.use(profiles.V3.use);

    test('toast_stack_should_cap_1_and_max_height_40pct_when_landscape', async ({ page, request }) => {
      const { client, session, deck } = await openSessionAndSeed(page, request, 3);
      try {
        await expect(deck.toasts).toHaveCount(1);
        const deckBox = await deck.deck.boundingBox();
        const viewport = page.viewportSize()!;
        expect(deckBox!.height).toBeLessThanOrEqual(viewport.height * 0.4 + 1);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });
  });
});
