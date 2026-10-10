// @feature notification-tray
/**
 * Stories 4.2 and 4.3 at the four viewport profiles (XA-13): the desktop edge
 * handle and overlay widths (TH-1, TY-11), the phone entry chip and its stable
 * node (TH-2, TH-7..TH-9), the bottom sheet's peek and expanded heights (TS-1),
 * its keyboard cap (TK-2, TS-9), 500-row virtualization (TR-1) and the offline
 * policy (TE-2). Real soft keyboard and device behavior is the operator's DV-3.
 */
import { test, expect, type Page } from '@playwright/test';
import {
  NotificationTray,
  TRAY_V2_FLAG,
  installWebSocketSendCounter,
  readTerminalFacts,
  sendNotification,
  setFeatureFlag,
  webSocketSends,
  waitForIdleFrames,
  openTerminalSession,
} from './pages/NotificationPanel';
import { profiles, setKeyboardOpen } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';
const MARKER = 'TRAY-VARIANT-MARKER';

function openSession(page: Page, { waitForText = true } = {}) {
  return openTerminalSession(page, BASE_URL, { marker: MARKER, titlePrefix: 'tray-variants', waitForText });
}

async function seed(request: Parameters<typeof sendNotification>[0], prefix: string, count: number) {
  for (let i = 0; i < count; i++) {
    await sendNotification(request, BASE_URL, { sessionId: `${prefix}-${i}`, type: 'CUSTOM', title: `Note ${i}` });
  }
}

test.describe('notification tray variants', () => {
  test.beforeEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  test.describe('V1 desktop', () => {
    test.use(profiles.V1.use);

    test('handle_should_show_99plus_with_aria_and_open_a_400px_overlay_when_120_unread', async ({ page, request }) => {
      const prefix = `tray-v1-${Date.now()}`;
      await seed(request, prefix, 105);
      const { client, session } = await openSession(page);
      try {
        const tray = new NotificationTray(page);
        await expect(tray.handle).toBeVisible();
        await expect(tray.handle).toContainText('99+');
        await expect(tray.handle).toHaveAttribute('aria-expanded', 'false');
        await expect(tray.handle).toHaveAttribute('aria-controls', 'notification-tray');
        const handleBox = await tray.handle.boundingBox();
        expect(handleBox!.width).toBeGreaterThanOrEqual(44);
        expect(handleBox!.x + handleBox!.width).toBeGreaterThan(1280 - 2);

        const before = await readTerminalFacts(page);
        await tray.handle.click();
        await tray.expectOpen();
        await expect.poll(async () => (await tray.tray.boundingBox())!.x).toBeCloseTo(1280 - 400, 0);
        expect((await tray.tray.boundingBox())!.width).toBeCloseTo(400, 0);
        expect(await tray.tray.getAttribute('data-variant')).toBe('side-overlay');
        await tray.dismissWhatChanged();

        // 105 rows are grouped by session but only a window of them is in the DOM (TR-1).
        await expect(tray.rows.first()).toBeVisible();
        expect(await tray.rows.count()).toBeLessThan(60);
        const after = await readTerminalFacts(page);
        expect(after.cols).toBe(before.cols);
        expect(after.hasProbe).toBe(true);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });

    for (const width of [900, 1000, 1100]) {
      test(`tray_should_overlay_min_400px_40vw_without_resizing_terminal_when_${width}px_wide`, async ({ page }) => {
        await page.setViewportSize({ width, height: 800 });
        const { client, session } = await openSession(page);
        try {
          const tray = new NotificationTray(page);
          const before = await readTerminalFacts(page);
          await tray.handle.click();
          await tray.expectOpen();
          const expectedWidth = Math.min(400, width * 0.4);
          // boundingBox includes the slide-in transform; poll until the 250ms transition settles.
          await expect.poll(async () => (await tray.tray.boundingBox())!.x).toBeCloseTo(width - expectedWidth, 0);
          const box = await tray.tray.boundingBox();
          expect(box!.width).toBeCloseTo(expectedWidth, 0);
          console.log(`  ${width}px: tray left edge ${box!.x}px (width ${box!.width}px), terminal cols ${before.cols}`);
          const during = await readTerminalFacts(page);
          expect(during.cols).toBe(before.cols);
          expect(during.rows).toBe(before.rows);
          expect(during.hasProbe).toBe(true);
          // Esc closes from inside the tray; with focus in the terminal it deliberately does nothing.
          await tray.tray.getByRole('searchbox', { name: 'Search notifications' }).click();
          await page.keyboard.press('Escape');
          await tray.expectClosed();
          expect((await readTerminalFacts(page)).cols).toBe(before.cols);
        } finally {
          await client.deleteSession(session.id, true);
        }
      });
    }

    test('tray_should_show_offline_banner_and_disable_dismiss_when_the_watch_stream_drops', async ({ page, request }) => {
      const prefix = `tray-offline-${Date.now()}`;
      await seed(request, prefix, 2);
      // Route every WebSocket so the stream can be cut and kept down, the way a network loss looks to the app.
      const sockets: Array<{ close: () => Promise<void> }> = [];
      let blocked = false;
      await page.routeWebSocket(/.*/, async (ws) => {
        if (blocked) {
          await ws.close();
          return;
        }
        ws.connectToServer();
        sockets.push(ws);
      });
      await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
      await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
      const tray = new NotificationTray(page);
      await expect(tray.handle).toBeVisible();
      await tray.handle.click();
      await tray.expectOpen();
      await tray.dismissWhatChanged();

      blocked = true;
      for (const ws of sockets) await ws.close();
      await expect(tray.tray.getByTestId('tray-banner-offline')).toBeVisible({ timeout: 30_000 });
      await expect(tray.tray.getByRole('button', { name: 'Mark activity read' })).toHaveAttribute('aria-disabled', 'true');
      await expect(tray.tray.getByText('All caught up')).toHaveCount(0);
      // Nothing is queued: the dismiss control reports why it is disabled and does nothing.
      const remove = tray.tray.getByLabel('Remove notification').first();
      if (await remove.count()) await expect(remove).toHaveAttribute('aria-disabled', 'true');
    });

    test('pin_should_dock_the_tray_narrowing_the_terminal_once_per_toggle_without_a_remount', async ({ page }) => {
      await installWebSocketSendCounter(page);
      const { client, session } = await openSession(page);
      try {
        const tray = new NotificationTray(page);
        await tray.handle.click();
        await tray.expectOpen();
        await tray.dismissWhatChanged();
        const overlayFacts = await readTerminalFacts(page);

        const pin = tray.tray.getByTestId('tray-pin');
        await expect(pin).toHaveAttribute('aria-pressed', 'false');
        const sendsBefore = await webSocketSends(page);
        await pin.click();
        await expect(pin).toHaveAttribute('aria-pressed', 'true');
        await expect.poll(async () => (await readTerminalFacts(page)).cols).toBeLessThan(overlayFacts.cols!);
        const pinnedFacts = await readTerminalFacts(page);
        // The resize vote is debounced by the settling hook, so wait for it, then let anything extra arrive.
        await expect.poll(async () => (await webSocketSends(page)) - sendsBefore).toBeGreaterThanOrEqual(1);
        // Anything extra from the same settle has arrived once the frame count stops moving.
        await waitForIdleFrames(page, 500);
        const resizeFrames = (await webSocketSends(page)) - sendsBefore;
        console.log(`  pin on: cols ${overlayFacts.cols} -> ${pinnedFacts.cols}, frames sent ${resizeFrames}`);
        expect(pinnedFacts.hasProbe).toBe(true);
        expect(resizeFrames).toBeLessThanOrEqual(3);

        await pin.click();
        await expect(pin).toHaveAttribute('aria-pressed', 'false');
        await expect.poll(async () => (await readTerminalFacts(page)).cols).toBe(overlayFacts.cols);
        expect((await readTerminalFacts(page)).hasProbe).toBe(true);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });
  });

  test.describe('V2 portrait', () => {
    test.use(profiles.V2.use);

    test('entry_chip_should_be_the_single_stable_node_and_sheet_should_peek_then_expand_when_phone', async ({ page, request }) => {
      const { client, session } = await openSession(page);
      try {
        const tray = new NotificationTray(page);
        await expect(tray.entry).toHaveCount(1);
        await expect(tray.entry).toHaveAttribute('data-content', 'bell');
        await expect(tray.handle).toHaveCount(0);

        // Observe the dock: the entry node must never be removed or inserted while content changes.
        await page.evaluate(() => {
          const w = window as unknown as { __entryMutations: number; __entryNode: Element | null };
          w.__entryNode = document.querySelector('[data-testid="tray-entry"]');
          w.__entryMutations = 0;
          new MutationObserver((records) => {
            for (const r of records) {
              const nodes = [...Array.from(r.removedNodes), ...Array.from(r.addedNodes)];
              if (nodes.some((n) => n instanceof Element && (n.matches?.('[data-testid="tray-entry"]') || n.querySelector?.('[data-testid="tray-entry"]')))) {
                w.__entryMutations += 1;
              }
            }
          }).observe(document.body, { childList: true, subtree: true });
        });

        const prefix = `tray-v2-${Date.now()}`;
        await seed(request, prefix, 3);
        await expect(tray.entry).toHaveAttribute('data-content', 'more-row');
        await expect(tray.entry).toHaveCount(1);
        await setKeyboardOpen(page, 300);
        await expect(tray.entry).toHaveAttribute('data-content', 'keyboard-chip');
        await setKeyboardOpen(page, 0);
        await expect(tray.entry).toHaveAttribute('data-content', 'more-row');

        const stable = await page.evaluate(() => {
          const w = window as unknown as { __entryMutations: number; __entryNode: Element | null };
          return { mutations: w.__entryMutations, same: w.__entryNode === document.querySelector('[data-testid="tray-entry"]') };
        });
        expect(stable.mutations).toBe(0);
        expect(stable.same).toBe(true);

        // The entry never covers the input line or page keys (TH-2).
        const entryBox = await tray.entry.boundingBox();
        const inputBox = await page.getByRole('textbox', { name: 'Terminal input' }).boundingBox();
        if (inputBox && entryBox) {
          const overlap = entryBox.y < inputBox.y + inputBox.height && entryBox.y + entryBox.height > inputBox.y;
          expect(overlap).toBe(false);
        }

        const before = await readTerminalFacts(page);
        await page.getByTestId('toast-overflow-chip').click();
        await tray.expectOpen();
        await expect(tray.tray).toHaveAttribute('data-variant', 'bottom-sheet');
        await expect(tray.tray).toHaveAttribute('data-sheet', 'peek');
        const viewportHeight = page.viewportSize()!.height;
        const peek = await tray.tray.boundingBox();
        expect(peek!.height / viewportHeight).toBeGreaterThanOrEqual(0.22);
        expect(peek!.height / viewportHeight).toBeLessThanOrEqual(0.28);

        await tray.tray.getByTestId('tray-expand').click();
        await expect(tray.tray).toHaveAttribute('data-sheet', 'expanded');
        const expanded = await tray.tray.boundingBox();
        expect(expanded!.height / viewportHeight).toBeGreaterThanOrEqual(0.84);
        expect(expanded!.height / viewportHeight).toBeLessThanOrEqual(0.86);
        // On touch the expanded sheet is non-trapping: no aria-modal, background not inert.
        expect(await tray.tray.getAttribute('aria-modal')).toBeNull();
        expect(await page.evaluate(() => document.getElementById('main-content')?.hasAttribute('inert'))).toBe(false);

        await tray.tray.getByRole('button', { name: 'Close notification panel' }).click();
        await tray.expectClosed();
        const after = await readTerminalFacts(page);
        expect(after.cols).toBe(before.cols);
        expect(after.rows).toBe(before.rows);
        expect(after.hasProbe).toBe(true);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });

    test('hardware_back_should_close_the_sheet_without_changing_url_or_session', async ({ page, request }) => {
      const { client, session } = await openSession(page);
      try {
        const tray = new NotificationTray(page);
        await seed(request, `tray-back-${Date.now()}`, 1);
        const url = page.url();
        await tray.entryOpen.or(page.getByTestId('toast-overflow-chip')).first().click();
        await tray.expectOpen();
        await page.goBack();
        await tray.expectClosed();
        expect(page.url()).toBe(url);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });

    test('sheet_open_expand_and_close_should_keep_cols_rows_and_node_and_send_no_resize_when_phone', async ({ page, request }) => {
      await installWebSocketSendCounter(page);
      const { client, session } = await openSession(page);
      try {
        const tray = new NotificationTray(page);
        await seed(request, `tray-ts2-${Date.now()}`, 2);
        await expect(tray.entry).toHaveAttribute('data-content', 'more-row');
        const before = await readTerminalFacts(page);
        await waitForIdleFrames(page);
        const sendsBefore = await webSocketSends(page);

        await page.getByTestId('toast-overflow-chip').click();
        await tray.expectOpen();
        await tray.tray.getByTestId('tray-expand').click();
        await expect(tray.tray).toHaveAttribute('data-sheet', 'expanded');
        await tray.tray.getByTestId('tray-expand').click();
        await expect(tray.tray).toHaveAttribute('data-sheet', 'peek');
        await tray.tray.getByRole('button', { name: 'Close notification panel' }).click();
        await tray.expectClosed();

        const after = await readTerminalFacts(page);
        expect(after.cols).toBe(before.cols);
        expect(after.rows).toBe(before.rows);
        expect(after.hasProbe).toBe(true);
        // Same budget as the desktop invariants spec: no resize vote beyond idle chatter.
        await waitForIdleFrames(page);
        expect((await webSocketSends(page)) - sendsBefore).toBeLessThanOrEqual(2);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });

    test('entry_should_be_one_visible_element_in_every_frame_and_hold_its_top_right_anchor_across_state_changes', async ({ page, request }) => {
      const { client, session } = await openSession(page, { waitForText: false });
      try {
        const tray = new NotificationTray(page);
        await page.evaluate(() => {
          const w = window as unknown as { __entryFrames: Array<{ visible: number; right: number; top: number }>; __entryStop: boolean };
          w.__entryFrames = [];
          w.__entryStop = false;
          const sample = () => {
            const boxes = Array.from(document.querySelectorAll('[data-testid="tray-entry"]'))
              .map((el) => el.getBoundingClientRect())
              .filter((r) => r.width > 0 && r.height > 0);
            const first = boxes[0];
            w.__entryFrames.push({ visible: boxes.length, right: first ? first.right : -1, top: first ? first.top : -1 });
            if (!w.__entryStop) requestAnimationFrame(sample);
          };
          requestAnimationFrame(sample);
        });

        // bell (0 toasts) -> more-row -> keyboard-chip -> more-row -> bell with undo -> bell
        await expect(tray.entry).toHaveAttribute('data-content', 'bell');
        await seed(request, `tray-th8-${Date.now()}`, 3);
        await expect(tray.entry).toHaveAttribute('data-content', 'more-row');
        await setKeyboardOpen(page, 300);
        await expect(tray.entry).toHaveAttribute('data-content', 'keyboard-chip');
        await setKeyboardOpen(page, 0);
        await expect(tray.entry).toHaveAttribute('data-content', 'more-row');
        await page.getByTestId('toast-move-all-to-tray').click();
        await expect(tray.entry).toHaveAttribute('data-undo', 'true');
        await expect(tray.entry).toHaveAttribute('data-undo', 'false', { timeout: 15_000 });

        const frames = await page.evaluate(() => {
          const w = window as unknown as { __entryFrames: Array<{ visible: number; right: number; top: number }>; __entryStop: boolean };
          w.__entryStop = true;
          return w.__entryFrames;
        });
        expect(frames.length).toBeGreaterThan(30);
        expect(frames.filter((f) => f.visible !== 1)).toEqual([]);
        const rights = frames.map((f) => f.right);
        expect(Math.max(...rights) - Math.min(...rights)).toBeLessThanOrEqual(1);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });

    test('touchstart_within_30px_of_the_edge_should_not_open_the_tray', async ({ page }) => {
      const { client, session } = await openSession(page);
      try {
        const tray = new NotificationTray(page);
        await page.touchscreen.tap(390 - 10, 400);
        await tray.expectClosed();
      } finally {
        await client.deleteSession(session.id, true);
      }
    });
  });

  test.describe('V4 portrait with the keyboard open', () => {
    test.use(profiles.V4.use);

    test('top_sheet_should_sit_above_the_keyboard_and_keep_textarea_focus_when_keyboard_open', async ({ page, request }) => {
      const { client, session } = await openSession(page);
      try {
        const tray = new NotificationTray(page);
        await seed(request, `tray-v4-${Date.now()}`, 2);
        const textarea = page.getByRole('textbox', { name: 'Terminal input' });
        await textarea.focus();
        await setKeyboardOpen(page, 300);
        await expect(tray.entry).toHaveAttribute('data-content', 'keyboard-chip');

        await page.getByTestId('toast-overflow-chip').click();
        await tray.expectOpen();
        await expect(tray.tray).toHaveAttribute('data-variant', 'top-sheet');
        // Pointer open on a touch device never takes focus from the terminal (TK-1).
        expect(await page.evaluate(() => document.activeElement?.getAttribute('aria-label'))).toBe('Terminal input');

        // boundingBox includes the slide-in transform, so poll until the 250ms transition settles.
        const vp = page.viewportSize()!;
        const keyboardTop = vp.height - 300;
        await expect
          .poll(async () => {
            const b = await tray.tray.boundingBox();
            return b!.y + b!.height;
          })
          .toBeLessThanOrEqual(keyboardTop + 1);
        // Anchored at the top: the keyboard can never cover it (TK-2).
        expect((await tray.tray.boundingBox())!.y).toBeLessThan(vp.height / 2);
        expect(await page.evaluate(() => document.activeElement?.getAttribute('aria-label'))).toBe('Terminal input');
      } finally {
        await client.deleteSession(session.id, true);
      }
    });
  });

  test.describe('V3 landscape', () => {
    test.use(profiles.V3.use);

    test('landscape_panel_should_be_at_most_360px_and_50vw_and_leave_terminal_size_unchanged', async ({ page }) => {
      const { client, session } = await openSession(page, { waitForText: false });
      try {
        const tray = new NotificationTray(page);
        const before = await readTerminalFacts(page);
        await tray.entryOpen.or(page.getByTestId('toast-overflow-chip')).first().click();
        await tray.expectOpen();
        await expect(tray.tray).toHaveAttribute('data-variant', 'landscape-panel');
        await expect.poll(async () => (await tray.tray.boundingBox())!.x).toBeCloseTo(844 - Math.min(360, 844 * 0.5), 0);
        const box = await tray.tray.boundingBox();
        expect(box!.width).toBeLessThanOrEqual(360);
        expect(box!.width).toBeLessThanOrEqual(844 * 0.5 + 1);

        const buttons = tray.tray.getByRole('button');
        const count = await buttons.count();
        for (let i = 0; i < count; i++) {
          const b = buttons.nth(i);
          if (!(await b.isVisible())) continue;
          const bb = await b.boundingBox();
          if (bb && bb.width > 0) expect(bb.height).toBeGreaterThanOrEqual(43);
        }
        // The close control is visible without scrolling (TL-2) and the list scrolls inside the panel (TL-4).
        await expect(tray.tray.getByRole('button', { name: 'Close notification panel' })).toBeInViewport();
        const overflowY = await tray.tray.getByTestId('tray-scroll').evaluate((el) => getComputedStyle(el).overflowY);
        expect(['auto', 'scroll']).toContain(overflowY);
        const after = await readTerminalFacts(page);
        expect(after.cols).toBe(before.cols);
        expect(after.rows).toBe(before.rows);
      } finally {
        await client.deleteSession(session.id, true);
      }
    });
  });
});
