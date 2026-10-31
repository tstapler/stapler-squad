// @feature session:keyboard-leader
/**
 * Leader-key command bar: arm, navigate, execute, cancel, and "the leader never
 * reaches the PTY". Default leader is Alt+; (matched on event.code).
 */
import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import {
  NotificationTray,
  installWebSocketSendCounter,
  openTerminalSession,
  waitForIdleFrames,
  webSocketSends,
} from './pages/NotificationPanel';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const ONBOARDED_KEY = 'stapler-squad:onboarded';
const LEADER = 'Alt+Semicolon';

async function gotoHome(page: Page): Promise<void> {
  await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  await page.goto(`${BASE_URL}/`, { waitUntil: 'domcontentloaded' });
  // The leader listener is attached in an effect; wait for the bar's readiness marker.
  await page.waitForSelector('body[data-command-bar-ready="true"]');
  // The Windows group registers from the page's effects, which can land after the layout-level bar is ready.
  await expect(page.getByRole('tablist', { name: 'Window switcher' })).toBeVisible();
}

const bar = (page: Page) => page.getByRole('dialog', { name: 'Command bar' });
const barInput = (page: Page) => page.getByTestId('command-bar-input');

test.describe('leader-key command bar', () => {
  test('leader_should_openCommandBarShowingNextKeyGroups', async ({ page }) => {
    await gotoHome(page);
    await page.keyboard.press(LEADER);
    await expect(bar(page)).toBeVisible();
    await expect(bar(page)).toHaveAttribute('data-mode', 'whichkey');
    await expect(page.getByTestId('command-bar-item').first()).toBeVisible();
  });

  test('jk_should_moveAriaActiveDescendantThroughTheList', async ({ page }) => {
    await gotoHome(page);
    await page.keyboard.press(LEADER);
    await expect(bar(page)).toBeVisible();
    const count = await page.getByTestId('command-bar-item').count();
    // Windows (w) and Notifications (n) are always registered; fewer means a registration regression.
    expect(count).toBeGreaterThan(1);

    const first = await barInput(page).getAttribute('aria-activedescendant');
    await page.keyboard.press('j');
    await expect(barInput(page)).not.toHaveAttribute('aria-activedescendant', first ?? '');
    await page.keyboard.press('k');
    await expect(barInput(page)).toHaveAttribute('aria-activedescendant', first ?? '');
    await page.keyboard.press('ArrowDown');
    await expect(barInput(page)).not.toHaveAttribute('aria-activedescendant', first ?? '');
  });

  test('sequence_should_executeNotificationTrayToggle', async ({ page }) => {
    await gotoHome(page);
    const tray = new NotificationTray(page);
    await page.keyboard.press(LEADER);
    await expect(bar(page)).toBeVisible();
    await page.keyboard.press('n');
    await expect(page.getByTestId('command-bar-path')).toContainText('Notifications');
    await page.keyboard.press('t');
    await expect(bar(page)).toBeHidden();
    await tray.expectOpen();
  });

  test('esc_should_closeBarAndReturnFocusToPreviousElement', async ({ page }) => {
    await gotoHome(page);
    const skip = page.getByRole('link', { name: 'Skip to main content' });
    await skip.focus();
    await expect(skip).toBeFocused();

    await page.keyboard.press(LEADER);
    await expect(bar(page)).toBeVisible();
    await expect(barInput(page)).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(bar(page)).toBeHidden();
    await expect(skip).toBeFocused();
  });

  test('leader_should_neverReachTheTerminalPty', async ({ page }) => {
    await installWebSocketSendCounter(page);
    await openTerminalSession(page, BASE_URL, { marker: 'LEADER-PTY-MARKER', titlePrefix: 'leader-pty', waitForText: false });
    const textarea = page.getByRole('textbox', { name: 'Terminal input' });
    await textarea.focus();

    // The keydown spy sits on xterm's own input element: a captured+stopped event never reaches it.
    // Re-installable because React may remount the terminal; each install targets the live element.
    const installSpy = () =>
      page.evaluate(() => {
        const w = window as unknown as { __xtermKeydowns?: number };
        w.__xtermKeydowns ??= 0;
        const el = document.querySelector('textarea[aria-label="Terminal input"]') as
          | (HTMLTextAreaElement & { __ssqSpy?: boolean })
          | null;
        if (!el || el.__ssqSpy) return;
        el.__ssqSpy = true;
        el.addEventListener('keydown', (e) => {
          // Bare modifier keydowns (the Alt half of Alt+;) carry no input; xterm emits nothing for them.
          if (!['Alt', 'Control', 'Shift', 'Meta'].includes(e.key)) w.__xtermKeydowns! += 1;
        });
      });
    await installSpy();
    const spy = () => page.evaluate(() => (window as unknown as { __xtermKeydowns?: number }).__xtermKeydowns ?? 0);

    await waitForIdleFrames(page);
    const sendsBefore = await webSocketSends(page);

    await page.keyboard.press(LEADER);
    await expect(bar(page)).toBeVisible();
    // Keys typed while the bar is open are swallowed too.
    await page.keyboard.press('x');
    await page.keyboard.press('Escape');
    await expect(bar(page)).toBeHidden();

    expect(await spy()).toBe(0);
    expect(await webSocketSends(page)).toBe(sendsBefore);

    // Control: with the bar closed, the same keystroke does reach the PTY (xterm onData -> WebSocket
    // frame), so the unchanged send count above is meaningful rather than a dead counter.
    await textarea.focus();
    await page.keyboard.press('x');
    await expect.poll(() => webSocketSends(page)).toBeGreaterThan(sendsBefore);
  });

  test('openBar_should_haveNoAxeWcagAaViolations', async ({ page }) => {
    await gotoHome(page);
    await page.keyboard.press(LEADER);
    await expect(bar(page)).toBeVisible();
    const results = await new AxeBuilder({ page })
      .include('[data-testid="command-bar"]')
      .withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'])
      .analyze();
    expect(results.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  });
});
