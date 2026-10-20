// @feature ui:header-nav
/**
 * Story 7.12 (Next baseline). The scroll container is found by measurement (the inner <main> on
 * Settings, a content <div> on Up Next), never the window. Measured on Next and recorded in fixtures/next-baseline.json:
 *  - forward navigation to a new pathname lands at the top (matches the plan);
 *  - Back does NOT restore the previous position (plan expects within 2px: premise false on Next,
 *    the page remounts at 0), so PR 7b asserts the decided Bun policy against this baseline;
 *  - hash-only navigation (#main-content) keeps the same document (no router navigation);
 *  - the Up Next tab change uses history.replaceState, so it adds no history entry and scroll is untouched.
 */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { measureScrollPolicy } from './pages/BaselineProbes';
import { UpNextPage } from './pages/UpNextPage';
import baseline from './fixtures/next-baseline.json';

const SCROLL_OFFSET = 40;

test.describe('scroll policy (Next baseline)', () => {
  test('forward nav, Back and hash nav match the recorded Next values', async ({ page }) => {
    const observed = await measureScrollPolicy(page);
    expect(observed.scrolledTo, 'the long page really scrolled').toBeGreaterThan(0);
    expect(observed).toEqual(baseline.scrollPolicy);
  });

  test('forward navigation resets scroll to the top of the new page', async ({ page }) => {
    const observed = await measureScrollPolicy(page);
    expect(observed.forwardNavScrollTop).toBe(0);
  });

  test('switching the Up Next tab leaves scroll and the history length unchanged', async ({ page }) => {
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
    const before = await shell.scrollTop();
    const historyLength = await page.evaluate(() => history.length);

    await upNext.selectTab('queue');
    await expect(page).toHaveURL(/[?&]tab=queue/);
    expect(await page.evaluate(() => history.length)).toBe(historyLength);
    expect(await shell.scrollTop()).toBe(before);
  });
});
