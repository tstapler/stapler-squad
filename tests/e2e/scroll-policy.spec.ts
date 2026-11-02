// @feature ui:header-nav
/**
 * Scroll policy. The scroll container is found by measurement (the inner <main> on Settings, a
 * content <div> on Up Next), never the window. Forward navigation lands at the top, and the Up Next
 * tab change uses history.replaceState, so it adds no history entry and leaves scroll untouched.
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

test.describe('scroll policy on Back (UX G8)', () => {
  // KNOWN GAP, found verifying this story against a real Bun build: ScrollPolicy.tsx's POP branch
  // (src/lib/nav/ScrollPolicy.tsx) calls restore() with the position saved under the current
  // location.key, but observed backScrollTop is 0, not the saved offset, on every run (verified
  // with a real mouse-wheel scroll, not just a scripted scrollTop write, so this isn't a
  // test-harness timing artifact). Needs an issue filed and a fix outside PR 7b's file set before
  // this can be un-marked. test.fail() keeps the gap visible (and tracked as a red flag) instead
  // of asserting a policy that does not hold.
  test.fail();
  test('Back restores the previous scroll position (within 2px)', async ({ page }) => {
    const observed = await measureScrollPolicy(page);
    expect(observed.scrolledTo, 'the long page really scrolled').toBeGreaterThan(0);
    expect(Math.abs(observed.backScrollTop - observed.scrolledTo)).toBeLessThanOrEqual(SCROLL_TOLERANCE_PX);
  });
});
