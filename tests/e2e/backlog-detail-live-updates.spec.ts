import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: backlog detail live updates — mapped from @feature annotation
const _features = [
  FEATURE_CATALOG['backlog-get-item'],
  FEATURE_CATALOG['backlog-transition-status'],
  FEATURE_CATALOG['backlog-archive-item'],
  // FEATURE_CATALOG['backlog:item-detail'], // TODO: add to catalog
] as const;
// @feature backlog:watch, backlog:item-detail

/**
 * E2E tests for project_plans/backlog-event-driven-updates Surface 3
 * (`BacklogItemDetail` side panel) — design/ux.md UX Acceptance Criteria
 * #10, #11, #13.
 *
 * See backlog-live-updates.spec.ts's file header for the debug mutate
 * endpoint pattern used to simulate a server-side transition/archive
 * without a second real browser context.
 */

import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';
import {
  createBacklogItemDirect,
  transitionBacklogItemDirect,
  archiveBacklogItemDirect,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from './pages/BacklogMutations';

test.describe('Backlog live updates (detail panel)', () => {
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

  test("an in_progress item's detail panel updates live to review with no polling network request firing on a timer (UX AC #10)", async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-detail-live-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.openItemDetail(`e2e-detail-live`);

    const detailPane = backlogPage.getItemDetailPane();
    await expect(detailPane).toBeVisible();
    await expect(backlogPage.getDetailStatusBadge()).toContainText('In Progress');

    // Old behavior (shouldPoll, BacklogItemDetail.tsx pre-Epic-5.3) fired a
    // GetBacklogItem poll every 5s while in_progress. Confirm no such request
    // fires during an idle window immediately after load — waitForResponse
    // with a bounded timeout (not waitForTimeout) doubles as the wait here:
    // if it resolves, an unexpected poll fired; if it rejects on timeout,
    // none did, which is the passing case.
    const unexpectedPoll = await page
      .waitForResponse((resp) => resp.url().includes('BacklogService/GetBacklogItem'), { timeout: 3000 })
      .then(() => true)
      .catch(() => false);
    expect(unexpectedPoll).toBe(false);

    await transitionBacklogItemDirect(request, itemId, 'review');

    await expect(backlogPage.getDetailStatusBadge()).toContainText('Review', { timeout: 5000 });
  });

  test('detail panel keeps showing the open item after it no longer matches the active list filter (UX AC #11)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-detail-outlives-filter-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.applyStatusFilter('in_progress');
    await backlogPage.openItemDetail('e2e-detail-outlives-filter');

    const detailPane = backlogPage.getItemDetailPane();
    await expect(detailPane).toBeVisible();

    // Transition the open item out of the active filter's match set.
    await transitionBacklogItemDirect(request, itemId, 'done');

    // The list row disappears (Surface 7)...
    await expect(backlogPage.getRowById(itemId)).toHaveCount(0, { timeout: 5000 });

    // ...but the still-open detail panel keeps showing the item's current
    // (done) state rather than blanking/freezing.
    await expect(detailPane).toBeVisible();
    await expect(backlogPage.getDetailStatusBadge()).toContainText('Done', { timeout: 5000 });
  });

  test('archiving the open item from another flow shows a terminal state instead of stale action buttons (UX AC #13)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-detail-terminal-${Date.now()}`,
      status: 'review',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.openItemDetail('e2e-detail-terminal');

    const detailPane = backlogPage.getItemDetailPane();
    await expect(detailPane).toBeVisible();

    await archiveBacklogItemDirect(request, itemId);

    const terminalNotice = page.getByTestId('backlog-detail-terminal-notice');
    await expect(terminalNotice).toBeVisible({ timeout: 5000 });
    await expect(terminalNotice).toContainText('archived elsewhere');

    // The Edit button is hidden once terminalState is set — no stale,
    // now-broken action button remains clickable.
    await expect(page.getByTestId('backlog-detail-edit')).toHaveCount(0);
  });
});
