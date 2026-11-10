// @feature ui:header-nav
/**
 * Scroll policy. The scroll container is found by measurement (the inner <main> on Settings, a
 * content <div> on Up Next), never the window. Forward navigation lands at the top, and the Up Next
 * tab change uses history.replaceState, so it adds no history entry.
 */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { measureScrollPolicy } from './pages/BaselineProbes';
import { UpNextPage } from './pages/UpNextPage';

const SCROLL_OFFSET = 40;
const SCROLL_TOLERANCE_PX = 2;

test.describe('scroll policy', () => {
  test('forward navigation resets scroll to the top of the new page', async ({ page }) => {
    const observed = await measureScrollPolicy(page);
    expect(observed.forwardNavScrollTop).toBe(0);
  });

  test('switching the Up Next tab adds no history entry', async ({ page }) => {
    const shell = new AppShellPage(page);
    const upNext = new UpNextPage(page);
    await upNext.resetStorage();
    await page.setViewportSize({ width: 1280, height: 360 });
    await upNext.goto('prs');
    await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');

    await expect.poll(() => shell.scrollRange()).toBeGreaterThan(SCROLL_OFFSET);
    // The panel re-renders as data loads and can reset the offset, so retry until it holds.
    await expect
      .poll(async () => {
        await shell.scrollTo(SCROLL_OFFSET);
        return shell.scrollTop();
      })
      .toBe(SCROLL_OFFSET);
    const historyLength = await page.evaluate(() => history.length);

    await upNext.selectTab('queue');
    await expect(page).toHaveURL(/[?&]tab=queue/);
    expect(await page.evaluate(() => history.length)).toBe(historyLength);
    // No scrollTop assertion: the scroller lives inside each tab's panel, so the PRs scroller unmounts on
    // the switch and the short queue panel doesn't scroll (the page object then measures an unrelated
    // element). "A search-only change keeps position" is covered in lib/nav/__tests__/scroll-policy.test.tsx.
  });
});

test.describe('scroll policy on Back (UX G8)', () => {
  test('Back restores the previous scroll position (within 2px)', async ({ page }) => {
    const observed = await measureScrollPolicy(page);
    expect(observed.scrolledTo, 'the long page really scrolled').toBeGreaterThan(0);
    expect(Math.abs(observed.backScrollTop - observed.scrolledTo)).toBeLessThanOrEqual(SCROLL_TOLERANCE_PX);
  });
});
