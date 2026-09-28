// @feature backlog:diagnose, backlog:list-diagnose-dispatches, backlog:diagnose-history-list, backlog:diagnose-outcome-display
/**
 * UX acceptance coverage for DiagnoseHistoryList (project_plans/
 * backlog-diagnose-and-nudge, design/ux.md Surfaces 11-12, 16). Maps to
 * validation.md's "UX Acceptance Tests" table rows for
 * tests/e2e/diagnose-history.spec.ts (UX criteria 4, 10, 21, 23).
 *
 * Same route-interception technique and known-drift note as
 * diagnose-action.spec.ts (see that file's header) -- read before editing
 * this one.
 *
 * Known drift relevant to this file: AC 23 ("nudge-cap/cooldown counter
 * visible on the item itself... via the outcome display and history list")
 * cannot be satisfied by the current implementation at all, not just
 * incompletely -- DiagnoseDispatchProto (proto/session/v1/diagnose.proto)
 * carries no numeric nudge-count/cap field or cooldown-eligible timestamp on
 * the wire, so there is nothing for either component to render even if the
 * copy function were fixed. See this file's test for AC 23, written as a
 * documented test.skip() rather than asserting against fabricated data the
 * real backend could never send.
 */

import { test, expect } from "@playwright/test";
import { StuckItemsPage, enableBacklogFeatureFlag, disableBacklogFeatureFlag } from "./pages/StuckItemsPage";
import {
  DiagnoseActionPage,
  diagnoseDispatchFixture,
  setDiagnoseNudgeExecutionFlag,
  waitForBacklogRPCsEnabled,
  uniqueDiagnoseTitle,
  openStuckItemDetail,
} from "./pages/DiagnoseActionPage";

function uniqueTitle(label: string): string {
  return uniqueDiagnoseTitle("diagnose-history", label);
}

test.describe("diagnose-history", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page, request }) => {
    await setDiagnoseNudgeExecutionFlag(request, true);
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test.afterEach(async ({ request }) => {
    await setDiagnoseNudgeExecutionFlag(request, false);
  });

  test("should_render_diagnose_history_list_inline_without_additional_navigation", async ({ page, request }) => {
    const title = uniqueTitle("populated-inline");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({ outcomeKind: "skipped_safety_gate", safetyGateReason: "not_idle" }),
      diagnoseDispatchFixture({ outcomeKind: "bug_filed", bugItemId: "itm_bug_hist_1" }),
    ]);

    // 0 additional clicks beyond the one that expands the item's card.
    await openStuckItemDetail(page, request, title);

    const list = dap.historyList();
    await expect(list).toBeVisible();
    // role="list" lives on the inner <ul>, not the outer data-testid div
    // (DiagnoseHistoryList.tsx) -- confirmed against the real DOM.
    await expect(list.getByRole("list", { name: "Diagnose dispatch history" })).toBeVisible();
    const items = dap.historyItems();
    await expect(items).toHaveCount(2);
    for (const item of await items.all()) {
      await expect(item).toHaveAttribute("role", "listitem");
    }
    // Newest-first: the fixtures were given in oldest-first order (matching
    // ListDiagnoseDispatches' real ordering), so the bug-filed row (2nd
    // fixture) renders first.
    await expect(items.first()).toContainText("filed a bug instead of nudging");
    await expect(items.last()).toContainText("nudge skipped (session wasn't idle)");
  });

  test("should_show_history_fetch_error_with_retry_never_a_silently_empty_list", async ({ page, request }) => {
    const errorTitle = uniqueTitle("fetch-error");
    const dapError = new DiagnoseActionPage(page);
    await dapError.mockListDiagnoseDispatchesError("simulated backend outage");
    await openStuckItemDetail(page, request, errorTitle);

    const errorBox = dapError.historyError();
    await expect(errorBox).toBeVisible();
    await expect(errorBox.getByRole("alert")).toContainText("Couldn't load diagnosis history");
    await expect(dapError.historyRetryButton()).toBeVisible();
    // Distinct testid from the empty state -- never rendered simultaneously.
    await expect(dapError.historyEmpty()).not.toBeVisible();

    const emptyTitle = uniqueTitle("empty-distinct");
    const dapEmpty = new DiagnoseActionPage(page);
    await dapEmpty.mockListDiagnoseDispatches([]);
    await openStuckItemDetail(page, request, emptyTitle);
    await expect(dapEmpty.historyEmpty()).toBeVisible();
    await expect(dapEmpty.historyEmpty()).toContainText("This item hasn't been diagnosed yet.");
    await expect(dapEmpty.historyError()).not.toBeVisible();
  });

  test("should_show_identical_outcome_after_page_refresh_sourced_from_persisted_dispatch_state", async ({ page, request }) => {
    const title = uniqueTitle("refresh-identical");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({ outcomeKind: "nudged", diagnosticSessionId: "headless-diagnose-e2e-refresh" }),
    ]);

    await openStuckItemDetail(page, request, title);
    const before = await dap.nudgedOutcome().textContent();
    expect(before).toContain("nudged the session");

    await page.reload();
    // Re-expand: StuckItemsSection re-collapses all cards on a fresh mount.
    await new StuckItemsPage(page).cardByTitle(title).click();

    const after = dap.nudgedOutcome();
    await expect(after).toBeVisible();
    const afterText = await after.textContent();
    expect(afterText).toBe(before);
    await expect(after.getByRole("link", { name: "View diagnosis" })).toHaveAttribute(
      "href",
      "/?session=headless-diagnose-e2e-refresh"
    );
  });

  test.skip(
    "should_show_visible_nudge_count_and_cap_on_cap_reached_outcome_and_in_history -- SKIPPED: DiagnoseDispatchProto " +
      "(proto/session/v1/diagnose.proto) has no field carrying a nudge count, cap, or cooldown-eligible timestamp. " +
      "safetyGateReasonCopy() (web-app/src/lib/backlog/diagnoseOutcomeCopy.ts) can only ever render the fixed string " +
      "'nudge skipped (nudge cap reached)' -- there is no 'N/cap' data anywhere on the wire for either " +
      "DiagnoseOutcomeDisplay or DiagnoseHistoryList to surface, even via a page.route() mock, without fabricating a " +
      "response shape the real backend can never produce. This is a genuine, unimplemented gap against design/ux.md's " +
      "AC 23 (and the 'no silent auto-retry with no visible counter' anti-pattern fix it cites) -- fixing it requires " +
      "adding a nudge_count/nudge_cap (and/or cooldown_eligible_at) field to the proto and to " +
      "session/diagnose.SafetyGateReason's cap/cooldown construction sites, which is outside an e2e test's scope.",
    async () => {}
  );
});
