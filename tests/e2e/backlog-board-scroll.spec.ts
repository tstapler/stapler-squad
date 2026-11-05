// @feature backlog:board-page, backlog:board
/**
 * The board's columns must be reachable by horizontal scroll: boardWrapper needs
 * minWidth:0 or the inner overflow-x scroller never engages and columns are clipped.
 */

import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';
import {
  createBacklogItemDirect,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from './pages/BacklogMutations';

test.describe('Backlog board scroll', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test('board scrolls horizontally when columns exceed the viewport', async ({ page, request }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
    });
    await page.setViewportSize({ width: 900, height: 700 });
    await createBacklogItemDirect(request, {
      title: `e2e-board-scroll-${Date.now()}`,
      status: 'in_progress',
    });

    await new BacklogPage(page).gotoBoard();
    const board = page.getByTestId('backlog-board');

    await expect(board).toHaveCSS('overflow-x', 'auto');
    const dims = await board.evaluate((el) => ({ sw: el.scrollWidth, cw: el.clientWidth }));
    expect(dims.sw).toBeGreaterThan(dims.cw);

    // The container must itself be bounded by the viewport, not blown out to content width.
    const boardWidth = await board.evaluate((el) => el.getBoundingClientRect().width);
    expect(boardWidth).toBeLessThanOrEqual(900);

    await board.evaluate((el) => {
      el.scrollLeft = el.scrollWidth;
    });
    expect(await board.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0);
  });
});
