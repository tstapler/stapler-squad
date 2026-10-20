// @feature notification-tray, ui:viewport
/**
 * Story 7.8b: iOS-Safari-equivalent checks on the `webkit-mobile` project (iPhone 13, 390x844).
 * Report-only: the project is only defined when WebKit is installed (see playwright.config.ts),
 * and a real iPhone checklist item (tray Back, keyboard over the omnibar) lives in Story 8.1.
 *  1. The tray's `ssqTray` history entry: Back closes it and keeps the URL.
 *  2. `100dvh` panes with the virtual keyboard simulated by shrinking the viewport: the page does
 *     not overflow the visible area and the focused input stays inside the visual viewport.
 */
import { test, expect, webkit } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { NotificationTray, sendNotification } from './pages/NotificationPanel';
import { setKeyboardOpen } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const KEYBOARD_PX = 300;

test.describe('iPhone WebKit (report-only)', () => {
  // The downloaded WebKit build can be present yet unrunnable (this Manjaro host gets Playwright's
  // generic ubuntu20.04 build without its MiniBrowser), so probe a real launch and skip with the reason.
  test.beforeAll(async () => {
    try {
      await (await webkit.launch()).close();
    } catch (err) {
      const reason = String(err instanceof Error ? err.message : err).split('\n')[0];
      test.skip(true, `WebKit cannot launch on this host (record in cutover-gate-results.md): ${reason}`);
    }
  });

  test.beforeEach(async ({ page }) => {
    await new AppShellPage(page).skipOnboarding();
  });

  test('ssqTray: Back closes the tray and leaves the URL unchanged', async ({ page, request }) => {
    await sendNotification(request, BASE_URL, { sessionId: `webkit-tray-${Date.now()}`, type: 'CUSTOM', title: 'WebKit tray note' });
    await page.goto(`${BASE_URL}/`, { waitUntil: 'domcontentloaded' });
    const tray = new NotificationTray(page);
    await expect(tray.entryOpen).toBeVisible();
    const urlBefore = page.url();

    await tray.entryOpen.click();
    await tray.expectOpen();
    await page.goBack();
    await tray.expectClosed();
    expect(page.url()).toBe(urlBefore);
  });

  test('100dvh panes stay inside the viewport when the keyboard is up', async ({ page }) => {
    await page.goto(`${BASE_URL}/history`, { waitUntil: 'domcontentloaded' });
    const input = page.getByRole('textbox').first();
    await expect(input).toBeVisible();
    await input.focus();

    const full = page.viewportSize()!;
    await page.setViewportSize({ width: full.width, height: full.height - KEYBOARD_PX });
    await setKeyboardOpen(page, KEYBOARD_PX);

    const bottomNav = page.getByRole('navigation', { name: 'Bottom navigation' });
    const navBox = await bottomNav.boundingBox();
    if (navBox) expect(navBox.y + navBox.height).toBeLessThanOrEqual(full.height - KEYBOARD_PX + 1);

    const overflow = await page.evaluate(() => document.documentElement.scrollHeight - window.innerHeight);
    expect(overflow, 'document taller than the visible viewport').toBeLessThanOrEqual(1);

    const inputBox = await input.boundingBox();
    expect(inputBox).not.toBeNull();
    expect(inputBox!.y + inputBox!.height).toBeLessThanOrEqual(full.height - KEYBOARD_PX + 1);
  });
});
