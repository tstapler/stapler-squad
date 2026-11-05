// @feature omnibar:detect
import { test, expect } from '@playwright/test';
import { OmnibarFindPrPage, PR_SEARCH_URL } from './pages/OmnibarFindPrPage';
import { ToastDeck } from './pages/NotificationPanel';

const intersects = (a: { x: number; y: number; width: number; height: number }, b: typeof a) =>
  a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;

test.describe('omnibar find sessions for this PR (phone)', () => {
  test('runs on a touch device', async ({ page }) => {
    await page.goto('about:blank');
    expect(await page.evaluate(() => window.matchMedia('(pointer: coarse)').matches)).toBe(true);
    expect(await page.evaluate(() => navigator.maxTouchPoints)).toBeGreaterThan(0);
  });

  test('pasting a PR URL and tapping the action should reach the artifacts list with no hover-only affordance', async ({ page }) => {
    const omnibar = new OmnibarFindPrPage(page);
    await omnibar.mockLookup([
      { sessionId: 'u1', title: 'first', source: 'session_pr' },
      { sessionId: 'u2', title: 'second', source: 'artifact' },
    ]);
    await omnibar.open();
    await omnibar.paste();

    // The action is a visible, labelled, >= 44px control (no hover needed) and nothing overflows sideways.
    const box = await omnibar.action.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);

    await omnibar.action.tap();
    await page.waitForURL(PR_SEARCH_URL, { timeout: 10000 });
  });

  test('a failed lookup should keep the omnibar open with a tappable Retry', async ({ page }) => {
    const omnibar = new OmnibarFindPrPage(page);
    await omnibar.mockLookup(null);
    await omnibar.open();
    await omnibar.paste();
    await omnibar.action.tap();

    await expect(omnibar.error).toBeVisible();
    await expect(page.getByTestId('omnibar')).toBeVisible();
    const retryBox = await omnibar.retry.boundingBox();
    expect(retryBox!.height).toBeGreaterThanOrEqual(44);
    expect(retryBox!.width).toBeGreaterThanOrEqual(44);
    await expect(omnibar.input).toBeEditable();
  });

  test('single-match toast should keep a tappable View all clear of the bottom navigation', async ({ page }) => {
    const omnibar = new OmnibarFindPrPage(page);
    await omnibar.mockLookup([{ sessionId: 'only-one', title: 'only one', source: 'session_pr' }]);
    await omnibar.open();
    await omnibar.paste();
    await omnibar.action.tap();

    const toast = new ToastDeck(page).toasts.filter({ hasText: 'Opened the session tracking acme/app#12 (Tracked)' });
    await expect(toast).toBeVisible({ timeout: 10000 });
    const viewAll = toast.getByRole('button', { name: 'View all' });
    const viewAllBox = await viewAll.boundingBox();
    expect(viewAllBox!.height).toBeGreaterThanOrEqual(44);

    const toastBox = await toast.boundingBox();
    const nav = page.getByRole('navigation', { name: 'Bottom navigation' });
    if (await nav.isVisible()) {
      expect(intersects(toastBox!, (await nav.boundingBox())!)).toBe(false);
    }

    await viewAll.tap();
    await page.waitForURL(PR_SEARCH_URL, { timeout: 10000 });
  });
});
