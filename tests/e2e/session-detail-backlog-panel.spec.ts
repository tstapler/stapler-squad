import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: session detail backlog panel — mapped from @feature annotation
const _features = [
  FEATURE_CATALOG['backlog-spawn-session'],
  FEATURE_CATALOG['backlog-get-item'],
  // FEATURE_CATALOG['backlog-item-panel'], // TODO: add to catalog (docs/registry/features/frontend/ui/backlog-item-panel.json)
] as const;
// @feature backlog:watch, backlog:item-panel

/**
 * E2E tests for project_plans/backlog-event-driven-updates Surface 4
 * (`BacklogItemPanel` embedded in `SessionDetail`) — design/ux.md UX
 * Acceptance Criteria #14, #15, #16.
 *
 * ============================ KNOWN GAP =================================
 * `BacklogItemPanel` (web-app/src/components/backlog/BacklogItemPanel.tsx)
 * and its live-update wiring (Task 5.4.1b/c) are fully implemented and
 * covered at the unit level (BacklogItemPanel.test.tsx,
 * docs/registry/features/frontend/ui/backlog-item-panel.json). But
 * `SessionDetail`/`SessionDetailView`'s `backlogItemId` prop — the only
 * thing that makes this panel render at all — is never populated by ANY
 * real call site in the current app:
 *
 *   - web-app/src/components/pane/PaneSplitRenderer.tsx:262 renders
 *     <SessionDetail ... embedded={true} /> (the main `/` workspace pane
 *     view) with no `backlogItemId` prop at all.
 *   - web-app/src/app/review-queue/page.tsx:286 renders <SessionDetail>
 *     with no `backlogItemId` prop.
 *   - web-app/src/components/sessions/SessionPeekModal.tsx:58 renders
 *     <SessionDetail> with no `backlogItemId` prop.
 *
 * `grep -rn "\.backlogItemId\b" web-app/src` (excluding the two files that
 * only *define* the prop and their own tests) returns zero matches —
 * confirmed during this sweep (2026-07-22). Plan Task 5.4.1a explicitly
 * flagged "confirm current data-fetching approach during implementation" as
 * an unresolved discovery item (design/ux.md Surface 4); that discovery was
 * apparently never followed by actually wiring a value through from a real
 * session's linked backlog item (e.g. a selector over backlogItemsSlice
 * keyed by `linkedSessions[].sessionId === session.id`).
 *
 * This means Surface 4 is NOT reachable through any real page navigation
 * today — no amount of e2e test cleverness can open a real SessionDetail
 * view and see a live BacklogItemPanel, because nothing ever tells it which
 * backlog item to show. The tests below are written to the intended
 * behavior and marked `test.fixme` with this comment so they show up as
 * "known gap," not silently skipped or falsely green — flip them to real
 * tests once a real caller threads `backlogItemId` through (the natural
 * fix: PaneSplitRenderer.tsx deriving it from a
 * `selectBacklogItemBySessionId`-style selector over the same
 * backlogItemsSlice store the backlog pages already use).
 * ==========================================================================
 */

import { test, expect } from '@playwright/test';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { enableBacklogFeatureFlag, disableBacklogFeatureFlag } from './pages/BacklogMutations';

test.describe('Session detail backlog panel', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.fixme(
    'linked backlog item panel updates live without the surrounding session detail page reloading (UX AC #14)',
    async ({ page }) => {
      // Intended flow once the wiring gap above is fixed:
      //   1. Spawn a real session linked to a backlog item (SpawnSessionFromItem).
      //   2. Open that session in the main pane view (SessionDetailPage.gotoSession).
      //   3. Switch to a non-default tab (e.g. Diff) and scroll.
      //   4. Transition the linked item's status via the debug mutate endpoint.
      //   5. Assert the panel updates while the active tab/scroll position is unchanged.
      const sessionDetailPage = new SessionDetailPage(page);
      await sessionDetailPage.gotoSession('placeholder-session-id');
      await expect(sessionDetailPage.getBacklogPanel()).toBeVisible();
    }
  );

  test.fixme(
    'at most one Live/connection indicator is visible per SessionDetail page (UX AC #15)',
    async ({ page }) => {
      const sessionDetailPage = new SessionDetailPage(page);
      await sessionDetailPage.gotoSession('placeholder-session-id');
      await expect(sessionDetailPage.getConnectionIndicators()).toHaveCount(1);
    }
  );

  test.fixme(
    'archiving the linked item elsewhere shows a terminal state in the embedded panel instead of stale action buttons (UX AC #16)',
    async ({ page }) => {
      const sessionDetailPage = new SessionDetailPage(page);
      await sessionDetailPage.gotoSession('placeholder-session-id');
      await expect(sessionDetailPage.getBacklogPanelTerminalNotice()).toBeVisible();
    }
  );
});
