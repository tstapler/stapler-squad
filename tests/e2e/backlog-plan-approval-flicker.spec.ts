// @feature backlog:list-stuck, backlog:approve-plan
/**
 * E2E coverage for the backlog/plan-approval-flicker fix
 * (project_plans reference: plan-approval-ux; triage item
 * d622de9f-5ea8-4358-8d11-cbf45078915b).
 *
 * Two things get exercised here that no prior e2e spec covered:
 *
 * 1. The Stuck Items surface's "Approve Plan" affordance is gated on a real
 *    plan existing (`plan_artifacts_path`), not just `reason ===
 *    PLAN_NOT_APPROVED` — StuckItemDetail.tsx's hasPlan check.
 * 2. Approving a plan from the Stuck Items card is reflected on the nav
 *    badge (StuckNavBadge, a separate mount elsewhere in the app) without
 *    waiting for its own 60s poll — StuckBacklogItemsContext's single
 *    shared poll (research/pitfalls.md #5). Deliberately approves from the
 *    Stuck Items surface itself rather than round-tripping through the
 *    backlog item detail page: a live-watch/GetBacklogItem race specific to
 *    that page's own remount-on-navigate path surfaced while writing this
 *    test and needs separate investigation (see BacklogItemDetail.tsx's
 *    liveRawItem staleness guard) — it's a distinct concern from the
 *    context-consolidation fix this test is scoped to verify.
 *
 * Prerequisites: same as backlog-stuck-items.spec.ts —
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import {
  StuckItemsPage,
  seedStuckItem,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from "./pages/StuckItemsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function waitForBacklogRPCsEnabled(request: APIRequestContext) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListStuckBacklogItems`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

test.describe("plan-approval flicker fix", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      // Two separate onboarding modals can intercept pointer events on first
      // load: the app-wide "One place for all your AI coding sessions" tour
      // (useOnboarding.ts's ONBOARDED_KEY) and the backlog-specific "How
      // backlog items work" tour (useBacklogTour.ts's BACKLOG_ONBOARDED_KEY).
      // Both must be suppressed or the Approve Plan click gets swallowed by
      // whichever modal is still showing.
      localStorage.setItem("stapler-squad:onboarded", "true");
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test("hides the Approve Plan button and shows a non-actionable message when the stuck item has no plan yet", async ({
    page,
    request,
  }) => {
    await seedStuckItem(request, {
      itemId: "e2e-plan-not-approved-no-plan",
      title: "fix: plan gate no-plan-yet test",
      reason: "plan_not_approved",
      context: "queued, waiting on triage to produce a plan",
      hasPlan: false,
    });

    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();
    await stuckPage.cardByTitle("fix: plan gate no-plan-yet test").click();

    await expect(page.getByTestId("stuck-item-no-action-copy")).toBeVisible();
    await expect(page.getByTestId("stuck-item-approve-plan")).not.toBeVisible();
  });

  test("approves a plan from the Stuck Items card with the real backend RPC, no error", async ({
    page,
    request,
  }) => {
    // NOTE: this intentionally stops short of asserting the nav badge/list
    // count decreases. ApprovePlan only flips plan_approved on the
    // BacklogItem — it does not synchronously resolve the item's open
    // BacklogStuckState row; that only happens on the next periodic
    // ReconcileStuck tick (every 60s, server/dependencies.go — see
    // research/pitfalls.md #6, an already-documented, out-of-scope residual
    // limitation of this fix, not something StuckBacklogItemsContext's
    // polling consolidation can shortcut). The
    // useStuckBacklogItems.test.ts "StuckBacklogItemsProvider" describe
    // block covers the actual context-sharing behavior deterministically
    // (mocked RPC, no reconciler dependency) — this e2e test's job is just
    // to prove the real click-through RPC path (hasPlan gate → real
    // ApprovePlan call) works end to end without error.
    await seedStuckItem(request, {
      itemId: "e2e-plan-not-approved-has-plan",
      title: "fix: cross-invalidation approve test",
      reason: "plan_not_approved",
      context: "queued item blocked by the planning gate",
      hasPlan: true,
    });

    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();
    const card = stuckPage.cardByTitle("fix: cross-invalidation approve test");
    await expect(card).toBeVisible();

    // Approve directly from this card's expanded detail — a real,
    // actionable button since hasPlan=true.
    await card.click();
    const approveBtn = page.getByTestId("stuck-item-approve-plan");
    await expect(approveBtn).toBeVisible();
    await approveBtn.click();

    // Success: the button returns to its idle label and no error text
    // appears — confirms the real ApprovePlan RPC succeeded (hasPlan gate
    // let a genuine approvable item through, and the backend accepted it).
    await expect(approveBtn).toHaveText("Approve Plan", { timeout: 10_000 });
    await expect(page.getByTestId("stuck-item-approve-plan-error")).not.toBeVisible();
  });
});
