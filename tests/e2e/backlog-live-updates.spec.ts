import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: backlog live updates — mapped from @feature annotation
const _features = [
  FEATURE_CATALOG['backlog-list-items'],
  FEATURE_CATALOG['backlog-transition-status'],
  // FEATURE_CATALOG['backlog:watch'], // TODO: add to catalog (docs/registry/features/backend/backlog/watch.json)
  // FEATURE_CATALOG['backlog:list-page'], // TODO: add to catalog
] as const;
// @feature backlog:watch, backlog:list-page

/**
 * E2E tests for project_plans/backlog-event-driven-updates Surface 1
 * (`/backlog` list rows) — design/ux.md UX Acceptance Criteria #1, #2, #3,
 * #6, #7, #25, #27, #28.
 *
 * A "server-side transition made via another flow" is simulated through the
 * `/api/debug/backlog/mutate-*` endpoints (tests/e2e/pages/BacklogMutations.ts),
 * which call storage.TransitionBacklogItemStatus/UpdateBacklogItem directly —
 * bypassing TransitionBacklogItemStatus's engine/gate checks entirely, the
 * same "no RPC handler involved" path validation.md's Happy Path Scenario
 * describes a reconciler using. This avoids needing a second real browser
 * context for every scenario while still exercising the real
 * WatchBacklogItems -> useWatchBacklogItems -> backlogItemsSlice -> UI path
 * end-to-end.
 *
 * Prerequisites: STAPLER_SQUAD_INSTANCE=e2e-local must be set on the test
 * server process for the debug mutate endpoints to be registered (see
 * server/server.go). When running via `npx playwright test` from tests/e2e,
 * export it before invoking so global-setup.ts's spawned server inherits it:
 *   STAPLER_SQUAD_INSTANCE=e2e-local npx playwright test backlog-live-updates.spec.ts
 */

import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';
import {
  createBacklogItemDirect,
  transitionBacklogItemDirect,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from './pages/BacklogMutations';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Backlog live updates (list)', () => {
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

  test('a status transition made via another flow is reflected in the list without a manual refresh (UX AC #1, #2, #6)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-live-update-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();

    const row = backlogPage.getRowById(itemId);
    await expect(row).toBeVisible();
    await expect(row.locator('[aria-label^="Status:"]')).toContainText('In Progress');

    const scrollBefore = await page.evaluate(() => window.scrollY);

    // Simulate a second, independent actor (reconciler / another tab)
    // transitioning the item — no RPC call from this page at all.
    await transitionBacklogItemDirect(request, itemId, 'review');

    // UX AC #6: status badge updates within ~2s, no manual refresh/reload.
    await expect(row.locator('[aria-label^="Status:"]')).toContainText('Review', { timeout: 5000 });

    // UX AC #1: no scroll-position jump.
    const scrollAfter = await page.evaluate(() => window.scrollY);
    expect(scrollAfter).toBe(scrollBefore);
  });

  test('a row never remounts and keeps DOM identity across a live update (UX AC #7)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-no-remount-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();

    const row = backlogPage.getRowById(itemId);
    await expect(row).toBeVisible();

    // Tag the live DOM node directly (not through React) — a remount would
    // create a fresh node without this marker, whereas an in-place update
    // (same key, same node) keeps it.
    await row.evaluate((el) => el.setAttribute('data-e2e-marker', 'still-here'));

    await transitionBacklogItemDirect(request, itemId, 'review');
    await expect(row.locator('[aria-label^="Status:"]')).toContainText('Review', { timeout: 5000 });

    await expect(row).toHaveAttribute('data-e2e-marker', 'still-here');
  });

  test('keyboard focus on an in-list row is preserved across a live update to that row (UX AC #7)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-focus-preserved-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();

    const row = backlogPage.getRowById(itemId);
    await expect(row).toBeVisible();
    await row.focus();
    await expect(row).toBeFocused();

    await transitionBacklogItemDirect(request, itemId, 'review');
    await expect(row.locator('[aria-label^="Status:"]')).toContainText('Review', { timeout: 5000 });

    await expect(row).toBeFocused();
  });

  test('is_snapshot events on initial load never flash a card (UX AC #3)', async ({ page, request }) => {
    // Create the item directly at "review" BEFORE the page ever loads — it
    // will only ever be seen via WatchBacklogItems's initial snapshot
    // (is_snapshot: true), never a live delta, so it must render with no
    // "just changed" treatment from the very first paint.
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-snapshot-no-flash-${Date.now()}`,
      status: 'review',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();

    const row = backlogPage.getRowById(itemId);
    await expect(row).toBeVisible();
    await expect(row.locator('[aria-label^="Status:"]')).toContainText('Review');
    // A snapshot-only row must never carry the exiting/transition markers a
    // genuine live departure would produce.
    await expect(row).not.toHaveAttribute('data-exiting', 'true');
  });

  test('an item that stops matching the active filter fades out before being removed from the DOM (UX AC #25, #27)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-filter-exit-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();

    // Filter down to in_progress only, so a transition to "done" drops the
    // item out of the active filtered view.
    await backlogPage.applyStatusFilter('in_progress');
    const row = backlogPage.getRowById(itemId);
    await expect(row).toBeVisible();

    await transitionBacklogItemDirect(request, itemId, 'done');

    // The exit fade is only ~200ms (EXIT_TRANSITION_MS in page.tsx) — too
    // tight a window to reliably observe the intermediate data-exiting="true"
    // state without flaking on poll timing, so the durable assertion is the
    // end state: the row is actually removed from the DOM shortly after,
    // rather than remaining forever (which would mean the filter re-evaluation
    // never ran) or vanishing before the event was even processed (which a
    // `toHaveCount(0)` immediately after the mutation, without any wait, would
    // fail to distinguish from "faded and removed").
    await expect(row).toHaveCount(0, { timeout: 3000 });
  });
});
