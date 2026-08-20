// @feature backlog:item-detail, backlog:deep-link-resolve

import { test, expect } from '@playwright/test';
import { createBacklogItemDirectWithPublicId, enableBacklogFeatureFlag, disableBacklogFeatureFlag } from './pages/BacklogMutations';

/**
 * Story 5.2 (project_plans/backlog-deep-linking): regression coverage
 * proving the pre-existing `?item=<uuid>` deep link still resolves exactly
 * as it did before the ssq:// deep-linking feature (Epics 1-5) shipped —
 * no warning banner, no migration prompt, item detail loads the same way.
 * The new DeepLinkErrorBanner/`/resolve` page (Story 5.1) is a separate,
 * additive entry point (web-app/src/app/resolve/page.tsx) that this legacy
 * flow never touches.
 */
test.describe('Backlog legacy ?item= deep link backcompat', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test('legacy_item_query_param_should_ResolveIdenticallyToBeforeFeatureShipped', async ({ page, request }) => {
    const title = `e2e-legacy-backcompat-${Date.now()}`;
    const { itemId, publicId } = await createBacklogItemDirectWithPublicId(request, { title });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    const pane = page.getByTestId('backlog-item-detail');
    await expect(pane).toBeVisible();
    await expect(pane).toContainText(title);
    // The legacy ?item=<uuid> URL still resolves the same item -- but every
    // item (old or new) now displays its publicId (bl_...) per Story 2.3's
    // universal public_id-first display, not the raw UUID. That's the
    // intentional, additive UI change; backcompat here is about the URL/
    // routing mechanics below, not the displayed ID text.
    await expect(page.getByTestId('backlog-item-id')).toHaveText(publicId);

    // No new deep-link error/warning UI (Story 5.1's DeepLinkErrorBanner)
    // and no migration prompt appear on this legacy path. Scoped to the
    // banner's own testid/copy rather than role="status"/"alert" generally —
    // both roles are legitimately used elsewhere on this page (e.g.
    // ConnectionIndicator, StatusBadge, InlineError), so a blanket
    // toHaveCount(0) on those roles is a false positive, not a real check.
    await expect(page.getByTestId('deep-link-error-banner')).toHaveCount(0);
    await expect(page.getByText(/isn't reachable right now/i)).toHaveCount(0);
    await expect(page.getByText(/lives on/i)).toHaveCount(0);
    await expect(page.getByText(/no longer exists/i)).toHaveCount(0);
    await expect(page.getByText(/has been archived/i)).toHaveCount(0);
    await expect(page.getByText(/migrat/i)).toHaveCount(0);

    // URL itself is unchanged by resolution — never redirected through the
    // new /resolve page.
    await expect(page).toHaveURL(new RegExp(`/backlog\\?item=${itemId}$`));
  });
});
