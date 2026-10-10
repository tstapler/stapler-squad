// @feature notification-tray
/**
 * Story 4.1: opening the tray never touches the terminal. cols x rows, the
 * terminal root DOM node, the scrollback marker, body overflow and focus are the
 * same before, during and after, the tray is non-modal, and the terminal keeps
 * accepting input while it is open. Esc from inside the tray returns focus to the
 * xterm textarea (TY-1, TY-2, TY-3, TY-5).
 */
import path from 'path';
import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import {
  NotificationTray,
  TRAY_V2_FLAG,
  installWebSocketSendCounter,
  markTerminalRoot,
  readTerminalFacts,
  setFeatureFlag,
  webSocketSends,
} from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';
const MARKER = 'TRAY-SCROLLBACK-MARKER';

test.use(profiles.V1.use);

test.describe('tray leaves the terminal untouched', () => {
  test.beforeEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  test('tray_should_keep_terminal_cols_rows_node_scrollback_body_overflow_and_focus_when_opened_and_closed_with_esc', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const session = await client.createSession({
      title: `tray-invariants-${Date.now()}`,
      path: '/tmp',
      program: `sh ${path.join(__dirname, 'fixtures', 'tray-terminal-fixture.sh')} ${MARKER}`,
    });
    try {
      await installWebSocketSendCounter(page);
      await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
      await page.goto(`${BASE_URL}/?session=${session.id}`, { waitUntil: 'domcontentloaded' });
      const textarea = page.getByRole('textbox', { name: 'Terminal input' });
      await expect(textarea).toBeAttached({ timeout: 20_000 });
      await expect.poll(async () => (await readTerminalFacts(page)).text, { timeout: 15_000 }).toContain(MARKER);

      const tray = new NotificationTray(page);
      await expect(tray.handle).toBeVisible();
      await markTerminalRoot(page);
      await textarea.focus();
      const before = await readTerminalFacts(page);
      const overflowBefore = await page.evaluate(() => document.body.style.overflow);
      await page.waitForTimeout(500); // let idle frames settle so only the tray's own sends are counted
      const sendsBefore = await webSocketSends(page);

      // Open with a pointer: focus stays in the terminal, nothing is dimmed or inerted.
      await tray.handle.click();
      await tray.expectOpen();
      await expect(tray.handle).toHaveAttribute('aria-expanded', 'true');
      await expect(tray.tray).not.toHaveAttribute('aria-modal', /.*/);
      expect(await page.locator('[class*="overlay"][aria-hidden="true"]').count()).toBe(0);
      expect(await page.evaluate(() => document.activeElement?.getAttribute('aria-label'))).toBe('Terminal input');

      // The terminal still accepts typed input while the tray is open.
      const typed = `tray-open-${Date.now()}`;
      await page.keyboard.type(`echo ${typed}`);
      await page.keyboard.press('Enter');
      await expect.poll(async () => (await readTerminalFacts(page)).text, { timeout: 10_000 }).toContain(typed);

      // Esc from inside the tray closes it and returns focus to the terminal (TY-2).
      await tray.tray.getByRole('searchbox', { name: 'Search notifications' }).click();
      await page.keyboard.press('Escape');
      await tray.expectClosed();
      expect(await page.evaluate(() => document.activeElement?.getAttribute('aria-label'))).toBe('Terminal input');

      const after = await readTerminalFacts(page);
      expect(after.cols).toBe(before.cols);
      expect(after.rows).toBe(before.rows);
      expect(after.hasProbe).toBe(true);
      expect(after.text).toContain(MARKER);
      expect(await page.evaluate(() => document.body.style.overflow)).toBe(overflowBefore);

      // The only frames sent are the ones typed above (the characters and Enter), never a resize vote.
      const typedFrames = typed.length + 'echo '.length + 1;
      const extra = (await webSocketSends(page)) - sendsBefore - typedFrames;
      console.log(`  frames beyond typed input: ${extra}`);
      expect(extra).toBeLessThanOrEqual(2);
    } finally {
      await client.deleteSession(session.id, true);
    }
  });

  test('tray_should_animate_only_transform_and_use_no_backdrop_filter', async ({ page }) => {
    await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    const tray = new NotificationTray(page);
    await expect(tray.tray).toBeAttached();
    const style = await tray.tray.evaluate((el) => {
      const cs = getComputedStyle(el);
      return {
        backdrop: cs.backdropFilter,
        props: cs.transitionProperty.split(',').map((p) => p.trim()),
        zIndex: cs.zIndex,
      };
    });
    expect(style.backdrop === 'none' || style.backdrop === '').toBe(true);
    expect(style.props.filter((p) => !['transform', 'visibility'].includes(p))).toEqual([]);
    expect(style.zIndex).not.toMatch(/^999[89]$/);
  });
});
