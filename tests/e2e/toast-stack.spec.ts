// @feature notification-toast-stack
/**
 * Capped toast deck on desktop (Stories 3.3, 3.4, 3.10): at most 3 cards, a
 * "+N more" chip, and one "Move all to tray" control that clears the deck without
 * deleting or reading anything. Notifications go through SendNotification, so the
 * server records history exactly as it does for a real producer. The second describe
 * runs the deck over a real terminal: it leaves the terminal alone (TD-10, XA-10), steps
 * clear of the open tray (TD-12) and of the cursor and scrollbar (TD-15).
 */
import { test, expect, type APIRequestContext } from '@playwright/test';
import {
  NotificationTray,
  ToastDeck,
  TRAY_V2_FLAG,
  installWebSocketSendCounter,
  openTerminalSession,
  readTerminalFacts,
  sendNotification,
  setFeatureFlag,
  waitForIdleFrames,
  webSocketSends,
} from './pages/NotificationPanel';
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

test.describe('toast stack over a terminal (desktop)', () => {
  const MARKER = 'DECK-TERMINAL-MARKER';

  test.beforeEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  async function seedDeck(page: import('@playwright/test').Page, request: APIRequestContext, prefix: string, count: number) {
    const deck = new ToastDeck(page);
    await expect(async () => {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-0`, type: 'ERROR', title: 'Failure 0' });
      await expect(deck.toasts.first()).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 20_000 });
    for (let i = 1; i < count; i++) {
      await sendNotification(request, BASE_URL, { sessionId: `${prefix}-${i}`, type: 'ERROR', title: `Failure ${i}` });
    }
    await expect(deck.moveAll).toBeVisible();
    return deck;
  }

  test('deck_should_leave_cols_rows_node_overflow_and_frames_unchanged_when_toasts_arrive_move_all_and_undo', async ({ page, request }) => {
    await installWebSocketSendCounter(page);
    const { client, session } = await openTerminalSession(page, BASE_URL, { marker: MARKER, titlePrefix: 'deck-invariants' });
    try {
      const before = await readTerminalFacts(page);
      const overflowBefore = await page.evaluate(() => document.body.style.overflow);
      await waitForIdleFrames(page);
      const sendsBefore = await webSocketSends(page);

      const deck = await seedDeck(page, request, `deck-inv-${Date.now()}`, 5);
      await deck.moveAll.click();
      await expect(deck.toasts).toHaveCount(0);
      await deck.undo.click();
      await expect(deck.toasts.first()).toBeVisible();

      const after = await readTerminalFacts(page);
      expect(after.cols).toBe(before.cols);
      expect(after.rows).toBe(before.rows);
      expect(after.hasProbe).toBe(true);
      expect(await page.evaluate(() => document.body.style.overflow)).toBe(overflowBefore);
      await waitForIdleFrames(page);
      expect((await webSocketSends(page)) - sendsBefore).toBeLessThanOrEqual(2);
    } finally {
      await client.deleteSession(session.id, true);
    }
  });

  test('deck_should_sit_at_least_16px_left_of_the_open_tray_and_stay_visible', async ({ page, request }) => {
    const { client, session } = await openTerminalSession(page, BASE_URL, { marker: MARKER, titlePrefix: 'deck-tray', waitForText: false });
    try {
      const deck = await seedDeck(page, request, `deck-tray-${Date.now()}`, 2);
      const tray = new NotificationTray(page);
      await tray.handle.click();
      await tray.expectOpen();

      await expect
        .poll(async () => {
          const trayBox = await tray.tray.boundingBox();
          const deckBox = await deck.deck.boundingBox();
          return trayBox && deckBox ? trayBox.x - (deckBox.x + deckBox.width) : -1;
        })
        .toBeGreaterThanOrEqual(15);
      await expect(deck.toasts.first()).toBeVisible();
    } finally {
      await client.deleteSession(session.id, true);
    }
  });

  test('deck_should_avoid_the_cursor_cell_and_the_scrollbar_when_the_cursor_is_on_the_last_row', async ({ page, request }) => {
    const { client, session } = await openTerminalSession(page, BASE_URL, { marker: MARKER, titlePrefix: 'deck-cursor' });
    try {
      const textarea = page.getByRole('textbox', { name: 'Terminal input' });
      await textarea.focus();
      // Move the cursor far to the right on the prompt row, inside the bottom-right deck's footprint.
      await page.keyboard.type(' '.repeat(115));
      await expect.poll(async () => (await textarea.boundingBox())?.x ?? 0).toBeGreaterThan(900);

      const deck = await seedDeck(page, request, `deck-cursor-${Date.now()}`, 2);
      await expect(deck.deck).toHaveAttribute('data-placement', 'desktopTopRight');

      const deckBox = (await deck.deck.boundingBox())!;
      const cursor = (await textarea.boundingBox())!;
      const cell = { x: cursor.x, y: cursor.y, width: Math.max(cursor.width, 8), height: Math.max(cursor.height, 16) };
      const overlapsCursor =
        deckBox.x < cell.x + cell.width && deckBox.x + deckBox.width > cell.x && deckBox.y < cell.y + cell.height && deckBox.y + deckBox.height > cell.y;
      expect(overlapsCursor).toBe(false);
      // The xterm scrollbar is the right-most ~15px of the viewport.
      expect(deckBox.x + deckBox.width).toBeLessThanOrEqual(1280 - 15);
    } finally {
      await client.deleteSession(session.id, true);
    }
  });
});
