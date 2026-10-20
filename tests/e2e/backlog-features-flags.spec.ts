// @feature settings-features, feature-flag:get, feature-flag:update, GetWorkflowEngineStatus
/**
 * Features page flag rows and the workflow-engine enforce dialog (backlog-make-it-true, Story 6.1.1;
 * UX-09..17, 35, 36, 39, 43..46, 48). Switch semantics, hit areas and the default/explicit source
 * line run against the real isolated server (flags restored afterwards); readiness evidence variants,
 * failing or pending writes and "changed in another tab" come from helpers/backlog-settings-mocks.ts.
 */
import { test, type Browser, type Page } from "@playwright/test";
import {
  CLEAN,
  DIFFERENCES,
  ENFORCE_FLAGS,
  FlagStore,
  connectError,
  deferred,
  engineStatus,
  mockEngineStatus,
  ok,
  pastRefetchWindow,
  setFlagViaApi,
  skipOnboarding,
  statusFollowing,
  wakeTab,
  type EngineStatusSpec,
  type Readiness,
  expect,
} from "./helpers/backlog-settings-mocks";
import { FeaturesSettingsPage, FLAG_LABELS, FLAG_ROW_HASH } from "./pages/FeaturesSettingsPage";
import { BacklogStagesSettingsPage } from "./pages/BacklogStagesSettingsPage";

const VIEWPORTS = [
  { name: "desktop-1280x800", width: 1280, height: 800 },
  { name: "mobile-360x800", width: 360, height: 800 },
] as const;

const MIN_TARGET_PX = 44;
const MAX_TABS_TO_REACH_SWITCH = 80;
const NOT_READY: Readiness = { divergence24h: 0, verdict: "NOT_READY", strandedItemCount: 0 };
const NEEDS_ACK: Readiness = { divergence24h: 0, verdict: "NEEDS_OWNER_ACK", strandedItemCount: 0 };
const STRANDED: Readiness = { divergence24h: 0, verdict: "READY", strandedItemCount: 2 };
const OPERATOR_ENFORCE = ENFORCE_FLAGS.operator;
const AGENT_ENFORCE = ENFORCE_FLAGS.agent;

/** Store, status and page object for a test that needs full control of flags and readiness. */
async function mocked(page: Page, operatorReadiness: Readiness = CLEAN) {
  const store = await FlagStore.install(page);
  const readiness: EngineStatusSpec["readiness"] = { operator: operatorReadiness, agent: operatorReadiness, system: operatorReadiness };
  const status = await mockEngineStatus(page, statusFollowing(store, { readiness }));
  return { store, status, readiness, features: new FeaturesSettingsPage(page) };
}

/** Presses Tab until the switch with this accessible name has focus. */
async function tabToSwitch(page: Page, key: keyof typeof FLAG_LABELS): Promise<void> {
  for (let i = 0; i < MAX_TABS_TO_REACH_SWITCH; i++) {
    await page.keyboard.press("Tab");
    const label = await page.evaluate(() => document.activeElement?.getAttribute("aria-label") ?? "");
    if (label === FLAG_LABELS[key]) return;
  }
  throw new Error(`could not Tab to ${FLAG_LABELS[key]}`);
}

/** True while keyboard focus is inside the open alertdialog. */
function focusInsideDialog(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const dialog = document.querySelector('[role="alertdialog"]');
    return Boolean(dialog && dialog.contains(document.activeElement));
  });
}

async function openDialogByTap(browser: Browser, width: number, height: number) {
  const context = await browser.newContext({ hasTouch: true, viewport: { width, height } });
  const page = await context.newPage();
  await skipOnboarding(page);
  return { context, page };
}

for (const vp of VIEWPORTS) {
  test.describe(`backlog features flags @ ${vp.name}`, () => {
    test.use({ viewport: { width: vp.width, height: vp.height } });
    test.describe.configure({ timeout: 90_000 }); // multi-step flows on a loaded machine outrun the 30 s default

    test.beforeEach(async ({ page }) => {
      await skipOnboarding(page);
    });

    // ===== Real server: UX-09, UX-10, UX-11, UX-12, UX-13 =====

    test.describe("real flags", () => {
      test.afterEach(async ({ request }) => {
        await setFlagViaApi(request, "diagnose_nudge_enabled", false);
        await setFlagViaApi(request, "autonomous_stall_nudge_enabled", false);
      });

      test("flag_controls_should_expose_switch_semantics_When_rendered", async ({ page }) => {
        const features = new FeaturesSettingsPage(page);
        await features.goto();
        const switches = page.getByTestId("feature-flag-switch");
        await expect(switches.first()).toBeVisible();
        expect(await switches.count()).toBeGreaterThan(5);

        const problems = await switches.evaluateAll((els) =>
          els.flatMap((el) => {
            const out: string[] = [];
            const name = el.getAttribute("aria-label") ?? "";
            if (el.getAttribute("role") !== "switch") out.push(`${name}: role is not switch`);
            if (!name.trim()) out.push("a switch has no accessible name");
            if (!["true", "false"].includes(el.getAttribute("aria-checked") ?? "")) out.push(`${name}: no aria-checked`);
            if (el.hasAttribute("aria-pressed")) out.push(`${name}: has aria-pressed`);
            for (const id of (el.getAttribute("aria-describedby") ?? "").split(" ").filter(Boolean)) {
              if (!document.getElementById(id)) out.push(`${name}: aria-describedby ${id} is missing`);
            }
            return out;
          }),
        );
        expect(problems).toEqual([]);

        const diagnose = features.switchFor("diagnose");
        const before = await diagnose.getAttribute("aria-checked");
        await diagnose.click();
        await expect(diagnose).not.toHaveAttribute("aria-checked", before ?? "");
        await expect(page.getByRole("switch", { name: FLAG_LABELS.diagnose, exact: true })).toBeVisible();
        await expect(features.rowStatus("diagnose")).toContainText("is now On");
      });

      test("flag_switch_should_have_44px_hit_area_and_focus_ring_When_360_and_1280", async ({ page }) => {
        const features = new FeaturesSettingsPage(page);
        await features.goto();
        for (const key of ["shadow", "operator", "diagnose"] as const) {
          const box = await features.switchFor(key).boundingBox();
          expect(box?.width, `${key} switch width`).toBeGreaterThanOrEqual(MIN_TARGET_PX);
          expect(box?.height, `${key} switch height`).toBeGreaterThanOrEqual(MIN_TARGET_PX);
        }
        await tabToSwitch(page, "diagnose");
        const ring = await features.switchFor("diagnose").evaluate((el) => {
          const style = getComputedStyle(el);
          return { outlineStyle: style.outlineStyle, outlineWidth: parseFloat(style.outlineWidth), boxShadow: style.boxShadow };
        });
        expect(ring.outlineStyle !== "none" && ring.outlineWidth > 0 || ring.boxShadow !== "none").toBe(true);
      });

      test("stall_row_should_read_on_inactive_and_nested_When_diagnose_off_and_stall_on", async ({ page, request }) => {
        await setFlagViaApi(request, "diagnose_nudge_enabled", false);
        await setFlagViaApi(request, "autonomous_stall_nudge_enabled", true);
        const features = new FeaturesSettingsPage(page);
        await features.goto();

        await expect(page.getByRole("heading", { name: "Autonomous backlog", level: 2 })).toBeVisible();
        const stall = features.row("stall");
        await expect(stall.getByTestId("feature-flag-status")).toHaveText("On (inactive)");
        await expect(stall.getByTestId("feature-flag-inert-note")).toHaveText("Has no effect until Diagnose & Nudge is On.");
        await features.switchFor("stall").click();
        await expect(features.switchFor("stall")).toHaveAttribute("aria-checked", "false");
      });

      // Whether the server still says "Default" depends on which earlier test wrote the flag, so the
      // real-server half asserts the explicit state; the default wording is checked on a stored row below.
      test("source_line_should_read_set_explicitly_and_live_note_should_name_the_delay_When_a_flag_was_toggled", async ({ page }) => {
        const features = new FeaturesSettingsPage(page);
        await features.goto();
        await expect(features.row("shadow").getByTestId("flag-live-note")).toContainText("within a few seconds");

        await features.switchFor("diagnose").click();
        await expect(features.rowStatus("diagnose")).toContainText("is now On");
        await page.reload();
        await expect(features.row("diagnose").getByTestId("feature-flag-source")).toContainText("Set explicitly");
      });

      test("toggle_should_announce_status_and_keep_focus_When_toggled_by_keyboard", async ({ page }) => {
        const features = new FeaturesSettingsPage(page);
        await features.goto();
        await expect(features.switchFor("diagnose")).toBeVisible();
        await tabToSwitch(page, "diagnose");
        await expect(features.switchFor("diagnose")).toBeFocused();

        await page.keyboard.press("Space");
        await expect(features.rowStatus("diagnose")).toHaveText("Diagnose & Nudge is now On.");
        await expect(features.rowStatus("diagnose")).toHaveAttribute("role", "status");
        await expect(features.switchFor("diagnose")).toBeFocused();
        await expect(features.switchFor("diagnose")).toHaveAttribute("aria-checked", "true");
      });
    });

    test("source_line_should_read_default_not_set_explicitly_When_flag_untouched", async ({ page }) => {
      const { features } = await mocked(page);
      await features.goto();
      await expect(features.row("diagnose").getByTestId("feature-flag-source")).toHaveText("Default: Off (not set explicitly)");
    });

    // ===== UX-14 / UX-17: failures and loading =====

    test("flags_should_revert_and_alert_When_toggle_fails_and_hide_switches_When_list_load_fails", async ({ page }) => {
      const { store, features } = await mocked(page);
      await features.goto();
      const diagnose = features.switchFor("diagnose");
      await expect(diagnose).toHaveAttribute("aria-checked", "false");

      store.update.use(connectError("internal", "disk full"));
      await diagnose.click();
      const alert = features.rowError("diagnose");
      await expect(alert).toBeVisible();
      await expect(alert).toContainText("Couldn't change Diagnose & Nudge");
      await expect(diagnose).toHaveAttribute("aria-checked", "false"); // the server's value, not the attempt
      expect(await alert.getAttribute("role")).toBe("alert");

      store.update.use((_n, body) => store.applyUpdate(body));
      await alert.getByRole("button", { name: "Retry" }).click();
      await expect(diagnose).toHaveAttribute("aria-checked", "true");
      await expect(features.rowError("diagnose")).toHaveCount(0);

      // List-load failure: an alert with Retry, and no switch that could read as Off.
      store.list.use(connectError("unavailable", "flags unavailable"));
      await features.goto();
      await expect(features.loadError).toBeVisible();
      await expect(features.loadError.getByRole("button", { name: "Retry" })).toBeVisible();
      await expect(page.getByTestId("feature-flag-switch")).toHaveCount(0);
      expect(await features.loadError.getAttribute("role")).toBe("alert");
    });

    test("loading_skeleton_should_be_aria_busy_with_no_switch_as_Off_When_flags_are_pending", async ({ page }) => {
      const { store, features } = await mocked(page);
      const held = deferred();
      store.list.use(() => held.promise);
      await features.goto();

      await expect(features.loading).toHaveAttribute("aria-busy", "true");
      await expect(page.getByTestId("feature-flag-switch")).toHaveCount(0);
      held.resolve(ok({ flags: store.rows }));
      await expect(features.switchFor("diagnose")).toBeVisible();
      await expect(features.loading).toHaveCount(0);
    });

    // ===== UX-15 / UX-43: the enforce dialog contract =====

    test("enforce_dialog_should_trap_focus_cancel_on_escape_and_overlay_and_change_flag_only_When_confirmed", async ({
      page,
    }) => {
      const { store, features } = await mocked(page);
      await features.goto();
      const operator = features.switchFor("operator");

      await operator.click();
      await expect(features.dialog).toBeVisible(); // role=alertdialog, not dialog
      await expect(page.getByRole("dialog")).toHaveCount(0);
      await expect(features.dialogCancel).toBeFocused(); // the initial focus is never the primary button
      for (let i = 0; i < 6; i++) {
        await page.keyboard.press("Tab");
        expect(await focusInsideDialog(page), `Tab ${i + 1} stays inside`).toBe(true);
      }
      for (let i = 0; i < 6; i++) {
        await page.keyboard.press("Shift+Tab");
        expect(await focusInsideDialog(page), `Shift+Tab ${i + 1} stays inside`).toBe(true);
      }

      await page.keyboard.press("Escape");
      await expect(features.dialog).toHaveCount(0);
      await expect(operator).toBeFocused();
      expect(store.update.calls).toBe(0);

      await operator.click();
      await features.dialogOverlay.click({ position: { x: 4, y: 4 } });
      await expect(features.dialog).toHaveCount(0);
      await expect(operator).toBeFocused();
      expect(store.update.calls).toBe(0);

      // A failing write leaves the flag Off with an alert.
      store.update.use(connectError("internal", "write failed"));
      await operator.click();
      await expect(features.evidence).not.toHaveAttribute("aria-busy", "true");
      await features.confirmEnforce();
      await expect(features.dialog).toHaveCount(0);
      await expect(features.rowError("operator")).toContainText("Couldn't change Enforce for operators (this UI)");
      await expect(operator).toHaveAttribute("aria-checked", "false");

      // A successful confirm changes only that flag.
      store.update.use((_n, body) => store.applyUpdate(body));
      await operator.click();
      await expect(features.evidence).not.toHaveAttribute("aria-busy", "true");
      await features.confirmEnforce();
      await expect(operator).toHaveAttribute("aria-checked", "true");
      expect(store.get(AGENT_ENFORCE).enabled).toBe(false);
      expect(store.get(ENFORCE_FLAGS.system).enabled).toBe(false);
    });

    test("enforce_dialog_should_cancel_on_overlay_click_and_return_focus_to_switch_When_opened_by_mouse_touch_or_keyboard", async ({
      page,
      browser,
    }) => {
      const { features } = await mocked(page);
      await features.goto();
      const operator = features.switchFor("operator");

      // Keyboard (Space) and mouse: Cancel, Escape, overlay click and Confirm all return focus to the switch.
      await operator.focus(); // reached by Tab in the real-flags keyboard test; here the key press is what is under test
      await page.keyboard.press("Space");
      await expect(features.dialog).toBeVisible();
      await features.dialog.getByRole("heading").click(); // a click inside does not close it
      await expect(features.dialog).toBeVisible();
      await features.dialogCancel.click();
      await expect(features.dialog).toHaveCount(0);
      await expect(operator).toBeFocused();

      await operator.click();
      await features.dialogOverlay.click({ position: { x: 4, y: 4 } });
      await expect(operator).toBeFocused();

      await operator.click();
      await page.keyboard.press("Escape");
      await expect(operator).toBeFocused();

      await operator.click();
      await expect(features.evidence).not.toHaveAttribute("aria-busy", "true");
      await features.confirmEnforce();
      await expect(operator).toHaveAttribute("aria-checked", "true");
      await expect(operator).toBeFocused();

      // Touch: the same overlay contract from a tap.
      const touch = await openDialogByTap(browser, vp.width, vp.height);
      try {
        const touched = await mocked(touch.page);
        await touched.features.goto();
        await touched.features.switchFor("agent").tap();
        await expect(touched.features.dialog).toBeVisible();
        await touched.features.dialogOverlay.tap({ position: { x: 4, y: 4 } });
        await expect(touched.features.dialog).toHaveCount(0);
        await expect(touched.features.switchFor("agent")).toBeFocused();
        expect(touched.store.update.calls).toBe(0);
      } finally {
        await touch.context.close();
      }
    });

    // ===== UX-35 / UX-44: readiness evidence =====

    test("enforce_dialog_should_show_evidence_and_gate_enforce_on_acknowledgement_When_differences_or_not_ready_or_load_failed", async ({
      page,
    }) => {
      const { store, status, features } = await mocked(page);
      await features.goto();
      const operator = features.switchFor("operator");

      // Clean: Enforce is enabled at once, no acknowledgement row.
      await operator.click();
      await expect(features.evidence).toContainText("Shadow differences in the last 24h: 0");
      await expect(features.ack).toHaveCount(0);
      await expect(features.dialogConfirm).not.toHaveAttribute("aria-disabled", "true");
      await page.keyboard.press("Escape");

      const variants: Array<[string, Readiness, RegExp]> = [
        ["differences", DIFFERENCES, /Shadow differences in the last 24h: 3/],
        ["not ready", NOT_READY, /Soak verdict: NOT READY/],
        ["needs ack", NEEDS_ACK, /Soak verdict: needs your acknowledgement/],
        ["stranded", STRANDED, /Items that would lose an exit: 2/],
      ];
      for (const [label, readiness, expected] of variants) {
        status.use(statusFollowing(store, { readiness: { operator: readiness } }));
        await operator.click();
        await expect(features.evidence, label).toContainText(expected);
        await expect(features.evidence, label).toContainText("Not clean");
        await expect(features.dialogConfirm, label).toHaveAttribute("aria-disabled", "true");
        // aria-disabled stays focusable: pressing it by keyboard must do nothing.
        await features.dialogConfirm.focus();
        await page.keyboard.press("Enter");
        await expect(features.dialog, `${label}: a disabled Enforce does nothing`).toBeVisible();
        await features.ack.check();
        await expect(features.dialogConfirm, label).not.toHaveAttribute("aria-disabled", "true");
        await page.keyboard.press("Escape");
        await expect(features.dialog).toHaveCount(0);
      }
      expect(store.update.calls).toBe(0);

      // A failed evidence load: message, Retry, and the acknowledgement route.
      status.use(connectError("unavailable", "status down"));
      await operator.click();
      await expect(features.evidence).toContainText("Couldn't load readiness evidence");
      await expect(features.evidenceRetry).toBeVisible();
      await expect(features.dialogConfirm).toHaveAttribute("aria-disabled", "true");
      await features.ack.check();
      await expect(features.dialogConfirm).not.toHaveAttribute("aria-disabled", "true");
    });

    test("enforce_dialog_should_keep_focus_on_cancel_announce_once_and_explain_disabled_enforce_When_evidence_resolves", async ({
      page,
    }) => {
      const { store, status, features } = await mocked(page, DIFFERENCES);
      await features.goto();
      const operator = features.switchFor("operator");

      // The dialog's own status read is held; the page's earlier read already resolved.
      const held = deferred();
      status.use(() => held.promise);
      await operator.click();
      await expect(features.dialogConfirm).toHaveAttribute("aria-disabled", "true");
      await expect(features.ack).toHaveCount(0);
      await expect(features.evidence).toHaveAttribute("aria-busy", "true");
      const heightBefore = (await features.evidence.boundingBox())?.height ?? 0;

      held.resolve(engineStatus({ modes: store.modes(), readiness: { operator: DIFFERENCES } }));
      await expect(features.evidence).not.toHaveAttribute("aria-busy", "true");
      await expect(features.evidence).toContainText(
        "Readiness evidence: 3 shadow differences, soak verdict READY, 0 items would lose an exit. Acknowledgement required.",
      );
      await expect(features.dialogCancel).toBeFocused();
      await expect(features.ack).toBeVisible();
      expect((await features.evidence.boundingBox())?.height ?? -1).toBeGreaterThanOrEqual(heightBefore - 1);

      const hintId = await features.dialogConfirm.getAttribute("aria-describedby");
      expect(hintId).toBeTruthy();
      await expect(features.hint).toHaveText("Tick the acknowledgement above to enable Enforce.");
      expect(await features.hint.getAttribute("id")).toBe(hintId);
      await features.dialogConfirm.focus();
      await page.keyboard.press("Enter");
      await expect(features.dialog).toBeVisible();
      expect(store.update.calls).toBe(0);
      await page.keyboard.press("Escape");

      // A failed load: Retry keeps focus through a repeat failure; focus moves to Cancel after success.
      status.use(connectError("unavailable", "status down"));
      await operator.click();
      await expect(features.evidenceRetry).toBeVisible();
      const callsBeforeRetry = status.calls;
      await features.evidenceRetry.click();
      // Let the repeat failure finish (the button is aria-disabled while it runs), then check focus stayed.
      await expect.poll(() => status.calls).toBeGreaterThan(callsBeforeRetry);
      await expect(features.evidenceRetry).not.toHaveAttribute("aria-disabled", "true");
      await expect(features.evidenceRetry).toBeFocused();
      status.use(engineStatus({ modes: store.modes(), readiness: { operator: CLEAN } }));
      await features.evidenceRetry.click();
      await expect(features.dialogCancel).toBeFocused();
      await expect(features.evidence).toContainText("Shadow differences in the last 24h: 0");
    });

    // ===== UX-45: the re-check at confirm =====

    test("enforce_dialog_should_not_write_flag_and_should_refocus_ack_When_recheck_shows_worse_evidence", async ({ page }) => {
      const { store, status, features } = await mocked(page);
      await features.goto();
      const operator = features.switchFor("operator");

      // Clean at open, three differences by confirm: nothing is written, the ack is required and focused.
      await operator.click();
      await expect(features.evidence).toContainText("Shadow differences in the last 24h: 0");
      status.use(statusFollowing(store, { readiness: { operator: DIFFERENCES } }));
      await features.dialogConfirm.click();
      await expect(page.getByTestId("feature-flag-enforce-evidence-changed")).toContainText(
        "Readiness evidence changed since you opened this dialog: shadow differences rose from 0 to 3.",
      );
      await expect(features.ack).toBeFocused();
      await expect(features.dialogConfirm).toHaveAttribute("aria-disabled", "true");
      expect(store.update.calls).toBe(0);
      await page.keyboard.press("Escape");

      // Clean then clean: one write.
      status.use(statusFollowing(store, { readiness: { operator: CLEAN } }));
      await operator.click();
      await expect(features.evidence).toContainText("Shadow differences in the last 24h: 0");
      await features.dialogConfirm.click();
      await expect(operator).toHaveAttribute("aria-checked", "true");
      expect(store.update.calls).toBe(1);

      // A failing re-check: no write, an in-dialog alert with Retry.
      store.set(OPERATOR_ENFORCE, false);
      await features.goto();
      await features.switchFor("operator").click();
      await expect(features.evidence).toContainText("Shadow differences in the last 24h: 0");
      status.use(connectError("unavailable", "status down"));
      await features.dialogConfirm.click();
      const recheck = page.getByTestId("feature-flag-enforce-recheck-alert");
      await expect(recheck).toContainText("Couldn't re-check readiness");
      await expect(recheck.getByRole("button", { name: "Retry" })).toBeVisible();
      expect(store.update.calls).toBe(1);
    });

    // ===== UX-46: independent flags and the recommended order =====

    test("enforce_rows_should_be_independent_with_step_lines_and_order_advisory_When_agents_enforced_before_operators", async ({
      page,
    }) => {
      const { features } = await mocked(page);
      await features.goto();

      await expect(
        page.getByText("These three switches are independent. Recommended order: operators, then agents, then automation."),
      ).toBeVisible();
      await expect(features.row("operator").getByTestId("feature-flag-step")).toHaveText("Step 1 of 3 (recommended first).");
      await expect(features.row("agent").getByTestId("feature-flag-step")).toHaveText("Step 2 of 3. Enforce after operators have soaked.");
      await expect(features.row("system").getByTestId("feature-flag-step")).toContainText("Step 3 of 3. Enforce last");
      for (const key of ["operator", "agent", "system"] as const) {
        await expect(features.switchFor(key)).not.toHaveAttribute("aria-disabled", "true");
      }

      // Agents before operators: an advisory, and Enforce is still enabled by clean evidence alone.
      await features.openEnforceDialog("agent");
      await expect(page.getByTestId("feature-flag-enforce-order-advisory")).toHaveText(
        "Operators are not enforced yet; the recommended order is operators first.",
      );
      await expect(features.evidence).not.toHaveAttribute("aria-busy", "true");
      await expect(features.dialogConfirm).not.toHaveAttribute("aria-disabled", "true");
      await features.dialogConfirm.click();
      await expect(features.switchFor("agent")).toHaveAttribute("aria-checked", "true");
      await expect(features.switchFor("operator")).toHaveAttribute("aria-checked", "false");

      // The Stages banner links to the first class not yet enforced in the recommended order.
      const stages = new BacklogStagesSettingsPage(page);
      await stages.goto();
      await expect(stages.bannerText).toContainText("Partly enforced.");
      await expect(stages.bannerFlagsLink).toHaveAttribute("href", new RegExp(`#${FLAG_ROW_HASH.operator}$`));
    });

    // ===== UX-36 / UX-39: refetch on wake and the summary line =====

    test("banner_and_rows_should_refetch_on_visibility_and_show_stale_copy_When_flag_changed_elsewhere", async ({ page }) => {
      await page.clock.install();
      const { store, features } = await mocked(page);
      await features.goto();
      await expect(features.summary).toHaveText(
        "0 of 3 caller classes enforced. Shadow: edits are compared with the built-in rules but not applied.",
      );
      await expect(features.row("operator").getByTestId("flag-live-note")).toContainText("within a few seconds");
      await expect(features.switchFor("operator")).toHaveAttribute("aria-checked", "false");

      store.set(OPERATOR_ENFORCE, true); // "another tab"
      await pastRefetchWindow(page);
      await wakeTab(page);

      await expect(features.switchFor("operator")).toHaveAttribute("aria-checked", "true");
      await expect(features.summary).toHaveText(
        "1 of 3 caller classes enforced (Partly enforced): operators enforced; agents and automation still in shadow.",
      );
    });

    test("workflow_engine_summary_should_state_enforced_count_in_plain_words_When_each_mode", async ({ page }) => {
      const { store, status, features } = await mocked(page);
      await features.goto();
      await expect(features.summary).toHaveText(
        "0 of 3 caller classes enforced. Shadow: edits are compared with the built-in rules but not applied.",
      );

      store.set(OPERATOR_ENFORCE, true);
      await features.goto();
      await expect(features.summary).toHaveText(
        "1 of 3 caller classes enforced (Partly enforced): operators enforced; agents and automation still in shadow.",
      );

      store.set(AGENT_ENFORCE, true);
      store.set(ENFORCE_FLAGS.system, true);
      await features.goto();
      await expect(features.summary).toHaveText("All 3 caller classes enforced.");

      status.use(connectError("unavailable", "status down"));
      await features.goto();
      await expect(features.summary).toHaveText("Mode unavailable: couldn't read the engine status.");

      // The same words as the Stages banner for the partial state.
      store.set(AGENT_ENFORCE, false);
      store.set(ENFORCE_FLAGS.system, false);
      status.use(statusFollowing(store));
      const stages = new BacklogStagesSettingsPage(page);
      await stages.goto();
      await expect(stages.bannerText).toContainText("Partly enforced.");
    });

    // ===== UX-48: pending semantics =====

    test("switch_should_ignore_space_enter_and_revert_While_toggle_pending", async ({ page }) => {
      const { store, features } = await mocked(page);
      store.set(OPERATOR_ENFORCE, true);
      await features.goto();

      // A pending flip of a plain flag: aria-disabled, focus kept, keys ignored.
      const held = deferred();
      store.update.use(() => held.promise);
      const diagnose = features.switchFor("diagnose");
      await diagnose.focus();
      await page.keyboard.press("Space");
      await expect(features.rowStatus("diagnose")).toHaveText("Saving Diagnose & Nudge...");
      await expect(diagnose).toHaveAttribute("aria-disabled", "true");
      await expect(diagnose).toBeFocused();
      await page.keyboard.press("Space");
      await page.keyboard.press("Enter");
      await expect.poll(() => store.update.calls).toBe(1);
      held.resolve(ok({ flag: { ...store.get("diagnose_nudge_enabled"), enabled: true } }));
      await expect(features.rowStatus("diagnose")).toHaveText("Diagnose & Nudge is now On.");
      await expect(diagnose).not.toHaveAttribute("aria-disabled", "true");

      // A pending Revert to Off: aria-disabled, a click adds no request, then it completes normally.
      const heldRevert = deferred();
      const callsBefore = store.update.calls;
      store.update.use(() => heldRevert.promise);
      const revert = features.revert("operator");
      await revert.click();
      await expect(features.rowStatus("operator")).toHaveText("Saving Enforce for operators (this UI)...");
      await expect(revert).toHaveAttribute("aria-disabled", "true");
      await revert.focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => store.update.calls).toBe(callsBefore + 1);
      store.set(OPERATOR_ENFORCE, false);
      heldRevert.resolve(ok({ flag: store.get(OPERATOR_ENFORCE) }));
      await expect(features.rowStatus("operator")).toContainText("is now Off.");
      await expect(features.revert("operator")).toHaveCount(0);
    });
  });
}
