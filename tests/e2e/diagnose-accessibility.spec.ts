// @feature backlog:diagnose, backlog:list-diagnose-dispatches, backlog:diagnose-outcome-display, backlog:diagnose-history-list
/**
 * Accessibility acceptance coverage for the "Diagnose" action
 * (project_plans/backlog-diagnose-and-nudge, design/ux.md's Accessibility
 * criteria 14-20 and no-false-confidence criterion 22). Maps to
 * validation.md's "UX Acceptance Tests" table rows for
 * tests/e2e/diagnose-accessibility.spec.ts.
 *
 * Same route-interception technique as diagnose-action.spec.ts (see that
 * file's header for why: no test-mode hook exists to force a real
 * diagnostic session to a given outcome).
 *
 * Contrast (AC 19) IS Playwright-testable here via the repo's existing
 * @axe-core/playwright gate, scoped to the rendered outcome/history DOM
 * (mirrors accessibility.spec.ts's "WCAG AA contrast for priority tokens"
 * test) -- not skipped. What genuinely isn't testable here: whether a
 * removed-color rendering (AC 17) is *visually* legible to a human is a
 * design-review judgment; this file's AC 17 test instead asserts the
 * structural precondition axe-core's contrast rule and a sighted reviewer
 * both depend on -- every state pairs an `aria-hidden` icon with real text
 * content, and no two states share identical text -- which is what
 * Playwright can mechanically verify.
 */

import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { enableBacklogFeatureFlag, disableBacklogFeatureFlag } from "./pages/StuckItemsPage";
import { BacklogPage } from "./pages/BacklogPage";
import { createBacklogItemDirect } from "./pages/BacklogMutations";
import {
  DiagnoseActionPage,
  diagnoseDispatchFixture,
  setDiagnoseNudgeExecutionFlag,
  waitForBacklogRPCsEnabled,
  uniqueDiagnoseTitle,
  openStuckItemDetail,
} from "./pages/DiagnoseActionPage";

function uniqueTitle(label: string): string {
  return uniqueDiagnoseTitle("diagnose-a11y", label);
}

test.describe("diagnose-accessibility", () => {
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

  test("should_expose_diagnose_button_as_tab_reachable_button_with_correct_aria_label", async ({ page, request }) => {
    const title = uniqueTitle("aria-label-stuck");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([]);
    await openStuckItemDetail(page, request, title);

    const stuckButton = page.getByRole("button", { name: "Diagnose this stuck item" });
    await expect(stuckButton).toBeVisible();
    expect(await stuckButton.evaluate((el) => el.tagName)).toBe("BUTTON");
    await stuckButton.focus();
    await expect(stuckButton).toBeFocused();

    // BacklogItemDetail's own call site (Surface 2) -- same contract, a
    // distinct aria-label per design/ux.md AC 14. Seeded directly via the
    // debug mutate endpoint (createBacklogItemDirect) rather than through
    // the "New Item" modal form -- that form's submit-then-wait-for-modal-
    // to-close path timed out in practice (a pre-existing form/validation
    // concern unrelated to Diagnose), and this test only needs a plain item
    // to exist, not to exercise item creation itself.
    const itemTitle = uniqueTitle("aria-label-backlog");
    await createBacklogItemDirect(request, { title: itemTitle });
    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.openItemDetail(itemTitle);

    const backlogButton = page.getByRole("button", { name: "Diagnose this item" });
    await expect(backlogButton).toBeVisible();
    expect(await backlogButton.evaluate((el) => el.tagName)).toBe("BUTTON");
  });

  test("should_expose_aria_busy_and_diagnosing_label_while_dispatch_in_flight", async ({ page, request }) => {
    const title = uniqueTitle("busy-state");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([]);
    const deferred = await dap.mockDiagnoseBacklogItemDeferred();

    await openStuckItemDetail(page, request, title);
    const button = dap.diagnoseButton("stuck");
    await button.click();

    await expect(button).toHaveAttribute("aria-busy", "true");
    await expect(button).toBeDisabled();
    await expect(button).toHaveText("Diagnosing…");
    // Not a spinner-only state: real visible text, asserted above via toHaveText.

    deferred.resolve();
    await expect(button).toHaveText("Diagnose");
  });

  test("should_use_polite_live_region_for_routine_outcomes_and_assertive_alert_for_failures", async ({ page, request }) => {
    const cases: Array<{
      name: string;
      fixture: Record<string, unknown>;
      locator: (dap: DiagnoseActionPage) => import("@playwright/test").Locator;
      expectAlert: boolean;
    }> = [
      { name: "pending", fixture: diagnoseDispatchFixture({ status: "DIAGNOSE_DISPATCH_STATUS_PENDING" }), locator: (d) => d.pendingOutcome(), expectAlert: false },
      { name: "stalled", fixture: diagnoseDispatchFixture({ status: "DIAGNOSE_DISPATCH_STATUS_STALLED" }), locator: (d) => d.stalledOutcome(), expectAlert: false },
      { name: "nudged", fixture: diagnoseDispatchFixture({ outcomeKind: "nudged" }), locator: (d) => d.nudgedOutcome(), expectAlert: false },
      { name: "skipped_safety_gate", fixture: diagnoseDispatchFixture({ outcomeKind: "skipped_safety_gate", safetyGateReason: "not_idle" }), locator: (d) => d.skippedSafetyGateOutcome(), expectAlert: false },
      { name: "bug_filed", fixture: diagnoseDispatchFixture({ outcomeKind: "bug_filed", bugItemId: "itm_bug_a11y" }), locator: (d) => d.bugFiledOutcome(), expectAlert: false },
      { name: "inconclusive_note_filed", fixture: diagnoseDispatchFixture({ outcomeKind: "inconclusive_note_filed" }), locator: (d) => d.inconclusiveOutcome(), expectAlert: false },
      { name: "dispatch_failed", fixture: diagnoseDispatchFixture({ outcomeKind: "dispatch_failed", failureReason: "x" }), locator: (d) => d.dispatchFailedOutcome(), expectAlert: true },
    ];

    for (const c of cases) {
      const title = uniqueTitle(`live-region-${c.name}`);
      const dap = new DiagnoseActionPage(page);
      await dap.mockListDiagnoseDispatches([c.fixture]);
      await openStuckItemDetail(page, request, title);

      const el = c.locator(dap);
      await expect(el).toBeVisible();
      if (c.expectAlert) {
        await expect(el).toHaveAttribute("role", "alert");
      } else {
        await expect(el).toHaveAttribute("aria-live", "polite");
        await expect(el).not.toHaveAttribute("role", "alert");
      }
    }
  });

  test("should_remain_distinguishable_by_text_alone_when_color_is_removed", async ({ page, request }) => {
    // hasIcon: false only for "pending" -- PendingBanner (DiagnoseOutcomeDisplay.tsx)
    // is plain "Diagnosing…" text with no icon at all, confirmed against the
    // real DOM (no spinner-only state to begin with, so there's no icon/color
    // signal to be color-independent of in the first place).
    const states: Array<{
      name: string;
      fixture: Record<string, unknown>;
      locator: (dap: DiagnoseActionPage) => import("@playwright/test").Locator;
      hasIcon: boolean;
    }> = [
      { name: "nudged", fixture: diagnoseDispatchFixture({ outcomeKind: "nudged" }), locator: (d) => d.nudgedOutcome(), hasIcon: true },
      { name: "skipped_safety_gate", fixture: diagnoseDispatchFixture({ outcomeKind: "skipped_safety_gate", safetyGateReason: "not_idle" }), locator: (d) => d.skippedSafetyGateOutcome(), hasIcon: true },
      { name: "bug_filed", fixture: diagnoseDispatchFixture({ outcomeKind: "bug_filed", bugItemId: "itm_bug_a11y2" }), locator: (d) => d.bugFiledOutcome(), hasIcon: true },
      { name: "inconclusive_note_filed", fixture: diagnoseDispatchFixture({ outcomeKind: "inconclusive_note_filed" }), locator: (d) => d.inconclusiveOutcome(), hasIcon: true },
      { name: "dispatch_failed", fixture: diagnoseDispatchFixture({ outcomeKind: "dispatch_failed", failureReason: "x" }), locator: (d) => d.dispatchFailedOutcome(), hasIcon: true },
      { name: "pending", fixture: diagnoseDispatchFixture({ status: "DIAGNOSE_DISPATCH_STATUS_PENDING" }), locator: (d) => d.pendingOutcome(), hasIcon: false },
      { name: "stalled", fixture: diagnoseDispatchFixture({ status: "DIAGNOSE_DISPATCH_STATUS_STALLED" }), locator: (d) => d.stalledOutcome(), hasIcon: true },
    ];

    const seenTexts = new Set<string>();
    for (const s of states) {
      const title = uniqueTitle(`color-independent-${s.name}`);
      const dap = new DiagnoseActionPage(page);
      await dap.mockListDiagnoseDispatches([s.fixture]);
      await openStuckItemDetail(page, request, title);

      const el = s.locator(dap);
      await expect(el).toBeVisible();
      if (s.hasIcon) {
        // Icon is decorative only -- the accessible name/text must carry the
        // meaning, never the glyph or a CSS color alone.
        const icon = el.locator('[aria-hidden="true"]').first();
        await expect(icon).toBeAttached();
      }
      const text = (await el.textContent())?.trim() ?? "";
      expect(text.length).toBeGreaterThan(0);
      expect(seenTexts.has(text)).toBe(false);
      seenTexts.add(text);
    }
  });

  test("should_expose_history_list_as_role_list_with_keyboard_focusable_listitems", async ({ page, request }) => {
    const title = uniqueTitle("history-list-a11y");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([
      diagnoseDispatchFixture({ outcomeKind: "nudged", diagnosticSessionId: "headless-diagnose-e2e-a11y-hist" }),
      diagnoseDispatchFixture({ outcomeKind: "skipped_safety_gate", safetyGateReason: "not_idle" }),
    ]);
    await openStuckItemDetail(page, request, title);

    const list = dap.historyList().getByRole("list", { name: "Diagnose dispatch history" });
    await expect(list).toBeVisible();
    const items = dap.historyItems();
    await expect(items).toHaveCount(2);

    // Fixtures were given oldest-first (matching ListDiagnoseDispatches'
    // real ordering); the component reverses to newest-first, so the nudged
    // fixture (given first) renders LAST -- it's the one carrying the
    // focusable link child.
    const link = items.last().getByRole("link", { name: "View diagnosis session" });
    await expect(link).toBeVisible();
    await link.focus();
    await expect(link).toBeFocused();
  });

  test("should_pass_axe_core_contrast_check_for_all_diagnose_outcome_chips", async ({ context, request }) => {
    const outcomeFixtures: Record<string, unknown>[] = [
      diagnoseDispatchFixture({ outcomeKind: "nudged" }),
      diagnoseDispatchFixture({ outcomeKind: "skipped_safety_gate", safetyGateReason: "not_idle" }),
      diagnoseDispatchFixture({ outcomeKind: "bug_filed", bugItemId: "itm_bug_contrast" }),
      diagnoseDispatchFixture({ outcomeKind: "inconclusive_note_filed" }),
      diagnoseDispatchFixture({ outcomeKind: "dispatch_failed", failureReason: "MCP server unreachable" }),
      diagnoseDispatchFixture({ status: "DIAGNOSE_DISPATCH_STATUS_STALLED" }),
    ];

    for (const themeName of ["light", "dark"] as const) {
      const page = await context.newPage();
      const title = uniqueTitle(`contrast-${themeName}`);
      const dap = new DiagnoseActionPage(page);
      await dap.mockListDiagnoseDispatches(outcomeFixtures);
      await page.addInitScript((name) => {
        localStorage.setItem("stapler-theme", name);
        localStorage.setItem("stapler-squad:onboarded", "true");
        localStorage.setItem("stapler-squad:backlog-onboarded", "true");
      }, themeName);
      await page.emulateMedia({ reducedMotion: "reduce" });
      await openStuckItemDetail(page, request, title);
      await expect(dap.stalledOutcome()).toBeVisible();

      const results = await new AxeBuilder({ page })
        .include('[data-testid^="diagnose-outcome-"]')
        .include('[data-testid="diagnose-history-list"]')
        .withRules(["color-contrast"])
        .analyze();

      expect(results.violations, `color-contrast violations in ${themeName} theme`).toHaveLength(0);
      await page.close();
    }
  });

  test("should_complete_full_diagnose_flow_via_keyboard_only_with_no_focus_loss_to_body", async ({ page, request }) => {
    const title = uniqueTitle("keyboard-only-flow");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([]);
    const deferred = await dap.mockDiagnoseBacklogItemDeferred();

    await openStuckItemDetail(page, request, title);

    const button = dap.diagnoseButton("stuck");
    await button.focus();
    await expect(button).toBeFocused();
    await page.keyboard.press("Enter");

    await expect(button).toHaveAttribute("aria-busy", "true");
    // KNOWN RISK, checked rather than assumed: `disabled={diagnoseBusy}` on
    // this same button (StuckItemDetail.tsx) can cause the browser to move
    // focus to <body> the instant a focused control becomes disabled -- the
    // exact failure this AC exists to catch. Assert it directly instead of
    // assuming design/ux.md's stated intent was implemented correctly.
    const activeDuringBusy = await page.evaluate(() => document.activeElement?.tagName ?? null);
    expect(activeDuringBusy).not.toBe("BODY");

    deferred.resolve({ dispatchId: "kb-d1", diagnosticSessionId: "headless-diagnose-e2e-kb" });
    await expect(button).toHaveText("Diagnose");

    const activeAfterSettle = await page.evaluate(() => document.activeElement?.tagName ?? null);
    expect(activeAfterSettle).not.toBe("BODY");
  });

  test("should_never_render_nudged_outcome_with_resolved_or_all_good_implication", async ({ page, request }) => {
    const title = uniqueTitle("no-resolved-implication");
    const dap = new DiagnoseActionPage(page);
    await dap.mockListDiagnoseDispatches([diagnoseDispatchFixture({ outcomeKind: "nudged" })]);
    await openStuckItemDetail(page, request, title);

    const nudged = dap.nudgedOutcome();
    await expect(nudged).toBeVisible();
    const text = (await nudged.textContent()) ?? "";
    expect(text).toContain("nudged the session");
    expect(text).not.toMatch(/resolved|fixed|all good|done\b/i);
  });
});
