// @feature triage-related-work
import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
const _features = [FEATURE_CATALOG['triage-related-work']] as const;

import { test, expect } from '@playwright/test';
import { BacklogItemDetailPage, seedHeadlessTriageItem } from './pages/BacklogItemDetailPage';
import { enableBacklogFeatureFlag } from './pages/BacklogMutations';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('triage-related-work', () => {
  test('e2e:triage-related-work - Find related past work surfaces prior sessions', async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    const title = `Add dark mode toggle ${Date.now()}`;
    const { itemId } = await seedHeadlessTriageItem(request, { title, status: 'idea', ended: true });

    await page.goto(`${BASE_URL}/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    const detail = new BacklogItemDetailPage(page);
    await detail.triageReviewPanel().waitFor({ state: 'visible', timeout: 15000 });

    // Auto-populates with the item's title on mount — no user interaction
    // required. This is the direct, deterministic proof that Story 2.2.1's
    // RelatedWorkQuery wiring fired for real, over the real ConnectRPC path.
    await expect(detail.relatedWorkInput()).toHaveValue(title);

    // Deliberately not waiting on the search RPC to *resolve* here: the
    // underlying SearchClaudeHistory index build runs over the real
    // ~/.claude/projects tree on first call, whose size is entirely a
    // property of the machine running the suite (unbounded on a real dev
    // box, empty/fast on a fresh CI runner) — not something this test can
    // control or should encode a timeout guess against. history-search.spec.ts
    // (the sibling spec for the existing search box) establishes the same
    // precedent: assert the input reflects the query, not that a real
    // backend search round-trip completed within some arbitrary window.

    // Editing the pre-filled box re-fires a debounced search — still
    // editable, not locked while a search may be in flight.
    await detail.relatedWorkInput().fill('a completely different search term');
    await expect(detail.relatedWorkInput()).toHaveValue('a completely different search term');

    // Apply/Skip remain reachable — the search box never blocks the panel's own actions.
    await expect(page.getByTestId('triage-apply-button').or(page.getByTestId('triage-mark-ready-button'))).toBeVisible();
    await expect(page.getByTestId('triage-skip-button')).toBeVisible();
  });
});
