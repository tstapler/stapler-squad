import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: backlog board live updates — mapped from @feature annotation
const _features = [
  FEATURE_CATALOG['backlog-list-items'],
  FEATURE_CATALOG['backlog-transition-status'],
  // FEATURE_CATALOG['backlog:board-page'], // TODO: add to catalog
  // FEATURE_CATALOG['backlog:board'], // TODO: add to catalog
] as const;
// @feature backlog:watch, backlog:board-page, backlog:board

/**
 * E2E tests for project_plans/backlog-event-driven-updates Surface 2
 * (`/backlog/board` Kanban cards) — design/ux.md UX Acceptance Criteria #8, #9.
 *
 * See backlog-live-updates.spec.ts's file header for the debug mutate
 * endpoint pattern used to simulate a server-side transition without a
 * second real browser context.
 */

import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';
import {
  createBacklogItemDirect,
  transitionBacklogItemDirect,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from './pages/BacklogMutations';

test.describe('Backlog live updates (board)', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
    });
  });

  test('a status transition moves a card from its origin column to its destination column without a page reload (UX AC #8)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-board-move-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.gotoBoard();

    const originCard = backlogPage.getCardInColumn('in_progress', itemId);
    await expect(originCard).toBeVisible();
    await expect(backlogPage.getCardInColumn('review', itemId)).toHaveCount(0);

    // Single triggered event moves the card between columns — no manual
    // drag, no page reload.
    await transitionBacklogItemDirect(request, itemId, 'review');

    const destinationCard = backlogPage.getCardInColumn('review', itemId);
    await expect(destinationCard).toBeVisible({ timeout: 5000 });

    // The origin column no longer shows the card at all once its exit
    // transition (~200ms) completes — the durable end state asserted here,
    // matching the "don't need exact animation timing in e2e, just that the
    // card ends up in the correct column" guidance.
    await expect(originCard).toHaveCount(0, { timeout: 3000 });
  });

  test('exactly one connection indicator is visible on the board view (UX AC #9)', async ({ page }) => {
    const backlogPage = new BacklogPage(page);
    await backlogPage.gotoBoard();

    await expect(backlogPage.getConnectionIndicator()).toHaveCount(1);
    await expect(backlogPage.getConnectionIndicator()).toBeVisible();
  });
});
