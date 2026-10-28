// @feature ui:header-nav, notification-tray, up-next-tabs
/**
 * Story 7.3: Back and Forward keep working. Runs unchanged on Next and Bun.
 *  - Back x2 / Forward x1 over / -> /history?sessionId=a -> /logs;
 *  - on a phone, Back closes the notification tray (its `ssqTray` history entry) and keeps the URL;
 *  - the Up Next tab control rewrites ?tab= without adding a history entry (history.replaceState).
 */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { NotificationTray, sendNotification } from './pages/NotificationPanel';
import { UpNextPage } from './pages/UpNextPage';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

const pathOf = (url: string) => new URL(url).pathname.replace(/(.)\/$/, '$1');

test.describe('history behaviour', () => {
  test.describe('desktop Back and Forward', () => {
    test('Back twice then Forward once retraces / -> /history?sessionId=a -> /logs', async ({ page }) => {
      const shell = new AppShellPage(page);
      await shell.skipOnboarding();
      await page.setViewportSize({ width: 1400, height: 900 });

      await shell.goto('/');
      await expect(shell.drawerNav).toBeVisible();
      await shell.goto('/history?sessionId=a');
      await expect(page).toHaveTitle('History - Stapler Squad');
      await shell.navLink(/logs/i).click({ force: true });
      await expect(page).toHaveURL(/\/logs/);

      await page.goBack();
      await expect.poll(() => pathOf(page.url())).toBe('/history');
      expect(new URL(page.url()).searchParams.get('sessionId')).toBe('a');

      await page.goBack();
      await expect.poll(() => pathOf(page.url())).toBe('/');

      await page.goForward();
      await expect.poll(() => pathOf(page.url())).toBe('/history');
      expect(new URL(page.url()).searchParams.get('sessionId')).toBe('a');
    });
  });

  test.describe('phone notification tray', () => {
    test.use(profiles.V2.use);

    test('Back closes the tray and leaves the URL unchanged', async ({ page, request }) => {
      const shell = new AppShellPage(page);
      await shell.skipOnboarding();
      await sendNotification(request, BASE_URL, { sessionId: `history-tray-${Date.now()}`, type: 'CUSTOM', title: 'History tray note' });

      await shell.goto('/');
      const tray = new NotificationTray(page);
      await expect(tray.entryOpen).toBeVisible();
      // useWindowUrlSync self-heals `?window=<id>` once windows restore; capture the URL after it
      // settles or Back would be compared against the pre-heal URL.
      await expect(page).toHaveURL(/[?&]window=/);
      const urlBefore = page.url();
      await tray.entryOpen.click();
      await tray.expectOpen();

      await page.goBack();
      await tray.expectClosed();
      expect(page.url()).toBe(urlBefore);
    });
  });

  test.describe('Up Next tab control', () => {
    test('?tab= changes in place without adding a history entry', async ({ page }) => {
      const upNext = new UpNextPage(page);
      await upNext.resetStorage();
      await upNext.goto('prs');
      await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');
      const entries = await page.evaluate(() => history.length);

      await upNext.selectTab('queue');
      await expect(page).toHaveURL(/[?&]tab=queue/);
      await expect(upNext.tab('queue')).toHaveAttribute('aria-selected', 'true');
      expect(await page.evaluate(() => history.length)).toBe(entries);
    });
  });
});
