// @feature backlog:diagnose, backlog:list-diagnose-dispatches, backlog:diagnose-outcome-display, backlog:diagnose-history-list
/**
 * UX acceptance coverage for the "Diagnose" action's core trigger -> in-flight
 * -> outcome flows (project_plans/backlog-diagnose-and-nudge). Maps to
 * validation.md's "UX Acceptance Tests" table rows for
 * tests/e2e/diagnose-action.spec.ts (UX criteria 1, 2, 3, 5, 6, 7, 8, 9, 11,
 * 12, 13 from design/ux.md).
 *
 * There is no test-mode hook to force a real diagnostic session to a given
 * outcome without a real LLM call (mirroring handoff-summary.spec.ts's and
 * approval-ci-block.spec.ts's identical situation), so every settled outcome
 * state below is injected via page.route() against
 * session.v1.DiagnoseService's JSON-over-HTTP ConnectRPC endpoints. Locators,
 * exact copy, and data-testid values were read directly from the current
 * DiagnoseOutcomeDisplay.tsx/DiagnoseHistoryList.tsx/StuckItemDetail.tsx
 * source, not from validation.md's/design/ux.md's wireframe prose -- see this
 * spec's individual test comments for confirmed drift between the two.
 *
 * Known drift from design/ux.md (reported, not silently fixed or hidden):
 * AC 7's required copy for the cap/cooldown skip variants is "nudge cap
 * reached: N/cap" and "cooldown active, next eligible <time>" -- the actual
 * implementation (diagnoseOutcomeCopy.ts's safetyGateReasonCopy) renders only
 * "nudge skipped (nudge cap reached)" / "nudge skipped (cooldown active)",
 * with no numeric/time detail at all. The DiagnoseDispatchProto wire message
 * (proto/session/v1/diagnose.proto) has no field to carry that number or
 * timestamp in the first place, so this is a real, structural implementation
 * gap, not a copy typo. Tests below assert the actual copy.
 *
 * Prerequisites: same as backlog-stuck-items.spec.ts --
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 * (tests/e2e/global-setup.ts spins up an isolated instance automatically for
 * a normal `npx playwright test` run -- this is only needed when running
 * against a manually-started server via TEST_SERVER_URL.)
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
  return uniqueDiagnoseTitle("diagnose", label);
}

test.describe("diagnose-action", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page, request }) => {
    // Default ON so a completed outcome fixture is never contradicted by the
    // nudging-disabled banner unless a specific test needs it OFF.
    await setDiagnoseNudgeExecutionFlag(request, true);
    await page.addInitScript(() => {
      // Two separate onboarding tours can intercept pointer events on first
      // load (see backlog-plan-approval-flicker.spec.ts's identical note).
      localStorage.setItem("stapler-squad:onboarded", "true");
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test.afterEach(async ({ request }) => {
    await setDiagnoseNudgeExecutionFlag(request, false);
  });

  test("should_dispatch_diagnosis_with_a_single_click_and_no_confirmation_dialog", async ({ page, request }) => {
    const title = uniqueTitle("single-click");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([]);
    const deferred = await dap.mockDiagnoseBacklogItemDeferred();

    await openStuckItemDetail(page, request, title);

    // NotificationPanel is globally mounted app-wide and always renders its
    // own `role="dialog"` content regardless of open/closed state (only a
    // CSS transform hides it -- see approval-ci-block.spec.ts's identical
    // note), so a bare "0 dialogs" assertion would fail on that unrelated,
    // pre-existing dialog. Compare the count before/after the click instead:
    // no *new* confirmation dialog is what this AC actually requires.
    const dialogCountBefore = await page.getByRole("dialog").count();

    await dap.diagnoseButton("stuck").click();

    await expect(page.getByRole("dialog")).toHaveCount(dialogCountBefore);
    await expect(dap.diagnoseButton("stuck")).toBeDisabled();
    await expect(dap.diagnoseButton("stuck")).toHaveAttribute("aria-busy", "true");
    await expect(dap.diagnoseButton("stuck")).toHaveText("Diagnosing…");

    deferred.resolve();
    await expect(dap.diagnoseButton("stuck")).toHaveText("Diagnose");
  });

  test("should_navigate_to_diagnostic_session_record_via_view_diagnosis_link", async ({ page, request }) => {
    const title = uniqueTitle("view-diagnosis-link");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({ outcomeKind: "nudged", diagnosticSessionId: "headless-diagnose-e2e-view-link" }),
    ]);

    await openStuckItemDetail(page, request, title);

    const nudged = dap.nudgedOutcome();
    await expect(nudged).toBeVisible();
    const link = nudged.getByRole("link", { name: "View diagnosis" });
    await expect(link).toHaveAttribute("href", "/?session=headless-diagnose-e2e-view-link");

    await link.click();
    await expect(page).toHaveURL(/session=headless-diagnose-e2e-view-link/);
  });

  test("should_retry_failed_dispatch_via_retry_button_without_page_reload", async ({ page, request }) => {
    const title = uniqueTitle("retry-no-reload");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({ outcomeKind: "dispatch_failed", failureReason: "MCP server unreachable" }),
    ]);
    let dispatchCallCount = 0;
    await page.route("**/api/session.v1.DiagnoseService/DiagnoseBacklogItem", async (route) => {
      dispatchCallCount++;
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ dispatchId: "retry-d1", diagnosticSessionId: "headless-diagnose-retry" }),
      });
    });

    await openStuckItemDetail(page, request, title);
    await expect(dap.dispatchFailedOutcome()).toBeVisible();
    expect(dispatchCallCount).toBe(0);

    let loadCount = 0;
    page.on("load", () => loadCount++);

    await dap.outcomeRetryButton().click();

    await expect.poll(() => dispatchCallCount).toBe(1);
    expect(loadCount).toBe(0);
  });

  test("should_show_nudging_disabled_banner_proactively_when_flag_is_off", async ({ page, request }) => {
    await setDiagnoseNudgeExecutionFlag(request, false);
    const title = uniqueTitle("flag-off-banner");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([]);

    await openStuckItemDetail(page, request, title);

    // 0 clicks: the banner is visible as soon as the section renders, before
    // any interaction with the Diagnose button.
    await expect(dap.nudgingDisabledBanner()).toBeVisible();
    await expect(dap.nudgingDisabledBanner()).toContainText(
      "Nudging is currently disabled — diagnosis will file a bug or post a note, but won't act on the session directly."
    );
  });

  test("should_show_exact_dispatch_failed_copy_and_retry_button", async ({ page, request }) => {
    const title = uniqueTitle("dispatch-failed-copy");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({ outcomeKind: "dispatch_failed", failureReason: "MCP server unreachable" }),
    ]);

    await openStuckItemDetail(page, request, title);

    const banner = dap.dispatchFailedOutcome();
    await expect(banner).toBeVisible();
    await expect(banner).toHaveAttribute("role", "alert");
    await expect(banner).toContainText("Couldn't start diagnosis — MCP server unreachable. Try again.");
    const retry = dap.outcomeRetryButton();
    await expect(retry).toBeVisible();
    await expect(retry).toBeEnabled();
    expect(await retry.evaluate((el) => el.tagName)).toBe("BUTTON");
  });

  test("should_name_specific_safety_gate_reason_for_each_skip_variant", async ({ page, request }) => {
    const cases: Array<{ reason: string; expected: string }> = [
      { reason: "not_idle", expected: "nudge skipped (session wasn't idle)" },
      { reason: "identity_mismatch_instance", expected: "nudge skipped (identity check failed)" },
      // Drift from design/ux.md (see file header): actual copy omits the
      // "N/cap" and "next eligible <time>" detail entirely.
      { reason: "nudge_cap_reached", expected: "nudge skipped (nudge cap reached)" },
      { reason: "nudge_cooldown_active", expected: "nudge skipped (cooldown active)" },
    ];

    for (const { reason, expected } of cases) {
      const title = uniqueTitle(`gate-${reason}`);
      const dap = new DiagnoseActionPage(page);
      await dap.mockListDiagnoseDispatches([
        diagnoseDispatchFixture({ outcomeKind: "skipped_safety_gate", safetyGateReason: reason }),
      ]);

      await openStuckItemDetail(page, request, title);

      const el = dap.skippedSafetyGateOutcome();
      await expect(el).toBeVisible();
      await expect(el).toContainText(expected);
      // Never the generic fallback: this is the "never a generic 'skipped'" AC.
      await expect(el).not.toContainText("nudge skipped (safety gate)");
    }
  });

  test("should_link_bug_filed_outcome_to_filed_bug_and_inconclusive_outcome_to_note", async ({ page, request }) => {
    const bugTitle = uniqueTitle("bug-filed-link");
    const dapBug = new DiagnoseActionPage(page);
    await dapBug.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({ outcomeKind: "bug_filed", bugItemId: "itm_bug_e2e_1" }),
    ]);
    await openStuckItemDetail(page, request, bugTitle);
    const bugOutcome = dapBug.bugFiledOutcome();
    await expect(bugOutcome).toBeVisible();
    // Next's <Link> normalizes the rendered href to a trailing slash before
    // the query string (this project's routing convention -- confirmed
    // against the real DOM), even though DiagnoseOutcomeDisplay.tsx builds
    // the href as `${routes.backlog}?item=...` with no trailing slash.
    await expect(bugOutcome.getByRole("link", { name: "View bug" })).toHaveAttribute(
      "href",
      "/backlog/?item=itm_bug_e2e_1"
    );

    const noteTitle = uniqueTitle("inconclusive-link");
    const dapNote = new DiagnoseActionPage(page);
    await dapNote.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({ outcomeKind: "inconclusive_note_filed", noteText: "couldn't confirm the write landed" }),
    ]);
    await openStuckItemDetail(page, request, noteTitle);
    const noteOutcome = dapNote.inconclusiveOutcome();
    await expect(noteOutcome).toBeVisible();
    await expect(noteOutcome.getByRole("link", { name: "View diagnostic note" })).toHaveAttribute(
      "href",
      "#backlog-activity-log"
    );
  });

  test("should_show_distinct_stalled_session_copy_with_diagnose_again_lever", async ({ page, request }) => {
    const title = uniqueTitle("stalled-copy");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({
        status: "DIAGNOSE_DISPATCH_STATUS_STALLED",
        diagnosticSessionId: "headless-diagnose-e2e-stalled",
      }),
    ]);

    await openStuckItemDetail(page, request, title);

    const stalled = dap.stalledOutcome();
    await expect(stalled).toBeVisible();
    // Distinct from dispatch-failed: not role=alert, different testid/copy.
    await expect(stalled).not.toHaveAttribute("role", "alert");
    await expect(dap.dispatchFailedOutcome()).not.toBeVisible();
    await expect(stalled).toContainText("Diagnosis stopped without a completion signal");
    await expect(stalled).toContainText("diagnose again");

    // The recovery lever is the Diagnose button itself, still functional.
    await expect(dap.diagnoseButton("stuck")).toBeEnabled();
    await expect(stalled.getByRole("link", { name: "View diagnosis session" })).toHaveAttribute(
      "href",
      "/?session=headless-diagnose-e2e-stalled"
    );
  });

  test("should_provide_a_working_next_action_for_every_degraded_state_without_reload", async ({ page, request }) => {
    // Surface 9: dispatch-failed's Retry.
    {
      const title = uniqueTitle("degraded-dispatch-failed");
      const dap = new DiagnoseActionPage(page);
      await dap.mockListDiagnoseDispatches([diagnoseDispatchFixture({ outcomeKind: "dispatch_failed", failureReason: "x" })]);
      let loadCount = 0;
      page.on("load", () => loadCount++);
      await openStuckItemDetail(page, request, title);
      loadCount = 0;
      const retry = dap.outcomeRetryButton();
      await expect(retry).toBeVisible();
      await expect(retry).toBeEnabled();
      expect(loadCount).toBe(0);
    }

    // Surface 15: stalled's "View diagnosis session" link + Diagnose-again button.
    {
      const title = uniqueTitle("degraded-stalled");
      const dap = new DiagnoseActionPage(page);
      await dap.mockListDiagnoseDispatches([
        diagnoseDispatchFixture({ status: "DIAGNOSE_DISPATCH_STATUS_STALLED", diagnosticSessionId: "headless-diagnose-degraded" }),
      ]);
      await openStuckItemDetail(page, request, title);
      await expect(dap.diagnoseButton("stuck")).toBeEnabled();
      await expect(dap.stalledOutcome().getByRole("link", { name: "View diagnosis session" })).toBeVisible();
    }

    // Surface 11: history fetch error's own Retry.
    {
      const title = uniqueTitle("degraded-history-error");
      const dap = new DiagnoseActionPage(page);
      await dap.mockListDiagnoseDispatchesError("simulated backend outage");
      let loadCount = 0;
      page.on("load", () => loadCount++);
      await openStuckItemDetail(page, request, title);
      loadCount = 0;
      const retry = dap.historyRetryButton();
      await expect(retry).toBeVisible();
      await expect(retry).toBeEnabled();
      expect(loadCount).toBe(0);
    }
  });

  test("should_reuse_in_flight_state_for_racing_dispatch_never_show_contradictory_outcomes", async ({ page, request }) => {
    const title = uniqueTitle("racing-dispatch");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([]);
    // Simulates the server-side race: DiagnoseBacklogItem rejects with
    // FailedPrecondition because a dispatch is already in flight for this
    // item (useDiagnoseAction's documented "already-diagnosing" branch,
    // mirroring Surface 14's cross-tab/server race case).
    await dap.mockDiagnoseBacklogItemError("failed_precondition", "diagnosis already in progress for this item");

    await openStuckItemDetail(page, request, title);
    await dap.diagnoseButton("stuck").click();

    const button = dap.diagnoseButton("stuck");
    await expect(button).toHaveText("Diagnosing… (already in progress)");
    await expect(button).toBeDisabled();
    await expect(button).toHaveAttribute("aria-live", "polite");
    // Never a second, contradictory error outcome alongside the reused busy state.
    await expect(dap.diagnoseError("stuck")).not.toBeVisible();
  });

  test("should_keep_diagnose_button_fully_functional_while_nudging_disabled_banner_shown", async ({ page, request }) => {
    await setDiagnoseNudgeExecutionFlag(request, false);
    const title = uniqueTitle("flag-off-still-functional");
    const dap = new DiagnoseActionPage(page);
    const controller = await dap.mockListDiagnoseDispatches([]);
    await dap.mockDiagnoseBacklogItemSuccess("d-flagoff", "headless-diagnose-e2e-flagoff");

    await openStuckItemDetail(page, request, title);
    await expect(dap.nudgingDisabledBanner()).toBeVisible();

    await dap.diagnoseButton("stuck").click();
    await expect(dap.diagnoseButton("stuck")).toHaveText("Diagnose", { timeout: 10_000 });
    await expect(dap.diagnoseError("stuck")).not.toBeVisible();

    // Bug-filed/inconclusive outcomes remain reachable while the flag is off
    // (only the nudge-shaped subset is foreclosed) -- verified via the
    // persisted-state path (Surface 16), since a live mid-session refresh of
    // DiagnoseOutcomeDisplay has no trigger in the current implementation
    // (useDiagnoseDispatches only polls once already-Pending; see
    // diagnose-history.spec.ts's persistence test for the same mechanism).
    controller.set([diagnoseDispatchFixture({ outcomeKind: "bug_filed", bugItemId: "itm_bug_flagoff" })]);
    await page.reload();
    // A reload re-collapses every stuck-item card (StuckItemsSection remounts
    // with none expanded), so the detail pane -- and this banner -- must be
    // re-opened before it can be asserted again.
    await new StuckItemsPage(page).cardByTitle(title).click();
    await expect(dap.nudgingDisabledBanner()).toBeVisible();
    await expect(dap.bugFiledOutcome()).toBeVisible();
  });
});
