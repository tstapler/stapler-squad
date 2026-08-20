// @feature backlog:item-detail, backlog:deep-link-resolve

import { test, expect } from '@playwright/test';
import { createBacklogItemDirect, enableBacklogFeatureFlag, disableBacklogFeatureFlag } from './pages/BacklogMutations';

/**
 * Story 5.1/5.2, Surface 2 (project_plans/backlog-deep-linking): same-host
 * deep-link resolution. Per web-app/src/app/resolve/page.tsx's doc comment,
 * a same-host `ssq://` link's "in-app equivalent" URL is `/backlog?item=<id>`
 * — the OS scheme handler (--open-url) and the `/resolve` page's "local"
 * branch both land there directly, without ever mounting `/resolve`'s own
 * UI. These tests exercise that in-app-equivalent URL directly, matching
 * validation.md's Happy Path Scenario ("paste ssq:// link in new tab on
 * same host -> detail panel opens with focus on heading, no interstitial").
 * Cross-host and error-path resolution (which DO go through `/resolve`) are
 * covered separately in backlog-cross-host-handoff.spec.ts and
 * backlog-deep-link-errors.spec.ts.
 */
test.describe('Backlog same-host deep link resolution', () => {
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

  test('sameHostLink_should_OpenItemDetailWithNoExtraClicks_When_AppAlreadyOpen', async ({ page, request }) => {
    const title = `e2e-resolve-nofrills-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    const pane = page.getByTestId('backlog-item-detail');
    await expect(pane).toBeVisible();
    await expect(pane).toContainText(title);
    // No intermediate click/dialog is required to reach the detail panel.
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('sameHostLink_should_MoveFocusToItemHeading_When_Resolved', async ({ page, request }) => {
    // GAP (Surface 2 AC3): BacklogItemDetail.tsx's item-title <h2> (lines
    // 1177, 1238) has no `id`/`tabIndex`/imperative focus() call anywhere
    // in the component — nothing ever moves focus to it on
    // mount/resolution. Documenting the gap rather than asserting a UX
    // that doesn't exist.
    test.fail(true, 'Item detail heading is never focused on resolution — no tabIndex/focus() call exists on BacklogItemDetail.tsx\'s <h2>{item.title}</h2> (validation.md Surface 2 AC3)');

    const title = `e2e-resolve-focus-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    await expect(page.getByRole('heading', { name: title })).toBeFocused();
  });

  test('sameHostLink_should_RenderDetailPanelWithoutInterstitial_When_ResolveIsFast', async ({ page, request }) => {
    const title = `e2e-resolve-nointerstitial-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    // The /resolve page's own loading UI (data-testid
    // "deep-link-resolve-loading") is never mounted on this in-app-
    // equivalent path — checked immediately, not after a wait.
    await expect(page.getByTestId('deep-link-resolve-loading')).not.toBeVisible();
    await expect(page.getByRole('status', { name: /resolving/i })).toHaveCount(0);
    await expect(page.getByTestId('backlog-item-detail')).toBeVisible();
  });
});
