// @feature settings-backlog-stages, backlog:list-stages, settings-features, feature-flag:update
/**
 * Backlog Stages engine-mode banner and the Features-page round trip (backlog-make-it-true,
 * Story 6.1.1; UX-01..08, 16, 37, 47, 52, 55). The default-mode and flag-toggle path runs against
 * the real isolated server; states it cannot produce on demand (a class per mode, a failing or
 * pending status RPC, review dates) come from helpers/backlog-settings-mocks.ts.
 *
 * Banner copy is per caller class (Enforced / Partly enforced / Shadow / Off / Unknown): the plan's
 * single global "Enforced" assumption is replaced by what the UI does, see the AC notes in the report.
 */
import { test } from "@playwright/test";
import {
  BASE_URL,
  FlagStore,
  allModes,
  connectError,
  deferred,
  engineStatus,
  mockEngineStatus,
  ok,
  pastRefetchWindow,
  resetEngineFlags,
  sequence,
  setFlagViaApi,
  skipOnboarding,
  statusFollowing,
  wakeTab,
  whenHydrated,
  type EngineStatusSpec,
  expect,
} from "./helpers/backlog-settings-mocks";
import { BacklogStagesSettingsPage } from "./pages/BacklogStagesSettingsPage";
import { FeaturesSettingsPage, FLAG_ROW_HASH } from "./pages/FeaturesSettingsPage";

const VIEWPORTS = [
  { name: "desktop-1280x800", width: 1280, height: 800 },
  { name: "mobile-360x800", width: 360, height: 800 },
] as const;

const ENFORCED_COPY =
  "Enforced: edits here change which transitions and gates the backlog allows, for operators, agents and automation. Edges that automation relies on are fixed; their gates are editable.";
const PARTLY_COPY =
  "Partly enforced. Operators (this UI): enforced. Agents: shadow, not enforced. Automation: shadow, not enforced.";
const SHADOW_COPY =
  "Shadow: edits are compared against the built-in rules but are not enforced. No differences in the last 24h.";
const OFF_COPY = "Not in use: the built-in 9-stage workflow is active. Edits are saved but have no effect.";
const UNKNOWN_COPY = "Couldn't determine whether these settings are enforced. Treat edits as not applied.";
const LOADING_COPY = "Checking whether these settings are enforced...";
const MIN_TARGET_PX = 44;
const SCROLL_CLEARANCE_PX = 16;

const PARTLY_SPEC: EngineStatusSpec = { modes: { operator: "ENFORCE", agent: "SHADOW", system: "SHADOW" } };

const MODE_CASES = [
  { name: "Enforced", spec: { modes: allModes("ENFORCE") }, copy: ENFORCED_COPY, icon: "✓" },
  { name: "Partly enforced", spec: PARTLY_SPEC, copy: PARTLY_COPY, icon: "⚠" },
  { name: "Shadow", spec: { modes: allModes("SHADOW") }, copy: SHADOW_COPY, icon: "⚠" },
  { name: "Off", spec: { modes: allModes("OFF") }, copy: OFF_COPY, icon: "⚠" },
  { name: "Unknown", spec: { modes: allModes("SHADOW"), gateWired: false }, copy: UNKNOWN_COPY, icon: "⚠" },
] as const;

for (const vp of VIEWPORTS) {
  test.describe(`backlog engine mode @ ${vp.name}`, () => {
    test.use({ viewport: { width: vp.width, height: vp.height } });
    test.describe.configure({ timeout: 90_000 }); // multi-step flows on a loaded machine outrun the 30 s default
    const isMobile = vp.width < 400;

    test.beforeAll(async ({ request }) => {
      await setFlagViaApi(request, "backlog", true);
    });

    test.afterAll(async ({ request }) => {
      await resetEngineFlags(request);
      await setFlagViaApi(request, "backlog", false);
    });

    test.beforeEach(async ({ page }) => {
      await skipOnboarding(page);
    });

    // ----- Acceptance criterion: default Shadow -> Partly enforced -> Enforced through the real Features page -----

    test("banner_should_show_shadow_then_partly_enforced_then_enforced_When_class_flags_are_toggled_on_the_features_page", async ({
      page,
      request,
    }) => {
      await resetEngineFlags(request);
      const stages = new BacklogStagesSettingsPage(page);
      const features = new FeaturesSettingsPage(page);

      // The server's mode reader caches for about a second, so re-open until it reports the new mode.
      const expectBanner = async (text: RegExp | string) =>
        expect(async () => {
          await stages.goto();
          await expect(stages.bannerText).toContainText(text, { timeout: 4000 });
        }).toPass({ timeout: 20_000 });

      await expectBanner("Shadow: edits are compared against the built-in rules but are not enforced.");
      expect(await stages.hasNoHorizontalScroll()).toBe(true);

      // UX-08: two clicks from the banner to the operator flag row and back.
      await stages.bannerFlagsLink.click();
      await expect(page).toHaveURL(new RegExp(`/settings/features/?#${FLAG_ROW_HASH.operator}$`));
      await expect(features.row("operator")).toBeFocused(); // UX-37

      await features.enforce("operator");
      await expect(features.rowStatus("operator")).toContainText("is now On. 1 of 3 caller classes enforced.");
      await features.editStagesLink("operator").click();
      await expect(page).toHaveURL(/\/settings\/backlog-stages\/?$/);
      await expectBanner(/^Partly enforced\. Operators \(this UI\): enforced\. Agents: shadow, not enforced\./);

      await features.goto();
      await features.enforce("agent");
      await features.enforce("system");
      await expect(features.summary).toContainText("All 3 caller classes enforced.");
      await expectBanner("Enforced: edits here change which transitions and gates the backlog allows");
    });

    // ----- UX-01 / UX-02: one test per mode -----

    for (const mode of MODE_CASES) {
      test(`banner_should_show_exact_copy_icon_and_one_empty_live_region_before_the_stage_list_When_${mode.name.replace(/ /g, "_")}`, async ({
        page,
      }) => {
        await mockEngineStatus(page, engineStatus(mode.spec));
        const stages = new BacklogStagesSettingsPage(page);
        await stages.goto();

        await expect(stages.bannerText).toHaveText(mode.copy);
        await expect(stages.banner).toContainText(mode.icon);
        await expect(stages.banner).not.toHaveAttribute("role", /.+/);
        await expect(stages.banner.getByRole("status")).toHaveCount(1);
        await expect(stages.bannerLive).toBeEmpty();
        await expect(stages.stageRow("idea")).toBeVisible();
        expect(await stages.bannerPrecedesStageList()).toBe(true);
        if (mode.name !== "Enforced") expect(await stages.banner.innerText()).not.toContain("Enforced:");
      });
    }

    // ----- UX-02 loading / failed never claim Enforced; UX-03 Retry updates in place -----

    test("banner_should_not_claim_enforced_While_loading_and_resolve_in_place_When_the_status_arrives", async ({ page }) => {
      const pending = deferred();
      await mockEngineStatus(page, () => pending.promise);
      const stages = new BacklogStagesSettingsPage(page);
      await stages.goto();

      await expect(stages.bannerText).toHaveText(LOADING_COPY);
      await expect(stages.banner).toHaveAttribute("aria-busy", "true");
      expect(await stages.banner.innerText()).not.toContain("Enforced:");

      pending.resolve(engineStatus({ modes: allModes("SHADOW") }));
      await expect(stages.bannerText).toHaveText(SHADOW_COPY);
      await expect(stages.banner).not.toHaveAttribute("aria-busy", "true");
      await expect(stages.bannerLive).toBeEmpty(); // the first resolved value is never announced
    });

    test("banner_should_show_unknown_copy_and_retry_updates_in_place_When_status_rpc_fails", async ({ page }) => {
      const status = await mockEngineStatus(page, connectError("unavailable", "engine status unavailable"));
      const stages = new BacklogStagesSettingsPage(page);
      await stages.goto();
      const urlBefore = page.url();

      await expect(stages.bannerText).toHaveText(UNKNOWN_COPY);
      expect(await stages.banner.innerText()).not.toContain("Enforced:");
      await expect(stages.bannerLive).toBeEmpty();
      await expect(stages.bannerFlagsLink).toBeVisible();
      if (isMobile) {
        expect((await stages.bannerRetry.boundingBox())?.height).toBeGreaterThanOrEqual(MIN_TARGET_PX);
        expect((await stages.bannerFlagsLink.boundingBox())?.height).toBeGreaterThanOrEqual(MIN_TARGET_PX);
      }

      status.use(engineStatus({ modes: allModes("SHADOW") }));
      await stages.bannerRetry.click();

      await expect(stages.bannerText).toHaveText(SHADOW_COPY);
      await expect(stages.bannerRetry).toHaveCount(0);
      await expect(stages.bannerLive).toHaveText(SHADOW_COPY); // UX-01: the change is announced once
      expect(page.url()).toBe(urlBefore);
      expect(status.calls).toBe(2);
    });

    test("banner_should_announce_the_new_mode_once_When_the_tab_wakes_and_the_status_changed", async ({ page }) => {
      await page.clock.install();
      const status = await mockEngineStatus(page, engineStatus({ modes: allModes("SHADOW") }));
      const stages = new BacklogStagesSettingsPage(page);
      await stages.goto();
      await expect(stages.bannerText).toHaveText(SHADOW_COPY);
      await expect(stages.bannerLive).toBeEmpty();

      status.use(engineStatus({ modes: allModes("ENFORCE") }));
      await pastRefetchWindow(page);
      await wakeTab(page);

      await expect(stages.bannerText).toHaveText(ENFORCED_COPY);
      await expect(stages.bannerLive).toHaveText(ENFORCED_COPY);
    });

    // ----- UX-04 / UX-05 -----

    test("banner_should_not_render_old_description_that_asserted_enforcement", async ({ page }) => {
      await mockEngineStatus(page, engineStatus({ modes: allModes("SHADOW") }));
      await new BacklogStagesSettingsPage(page).goto();
      await expect(page.getByRole("heading", { name: "Backlog Stages", level: 1 })).toBeVisible();
      await expect(page.getByText("the gates that must pass before a transition is allowed")).toHaveCount(0);
      await expect(page.getByText("Whether these settings are enforced is shown below.")).toBeVisible();
    });

    test("banner_should_not_scroll_horizontally_and_have_44px_targets_When_360_and_1280", async ({ page }) => {
      await mockEngineStatus(page, engineStatus({ modes: allModes("SHADOW") }));
      const stages = new BacklogStagesSettingsPage(page);
      await stages.goto();
      await expect(stages.bannerText).toHaveText(SHADOW_COPY);
      await expect(stages.banner.getByText("not enforced").first()).toBeVisible();
      expect(await stages.hasNoHorizontalScroll()).toBe(true);
      expect((await stages.bannerFlagsLink.boundingBox())?.height).toBeGreaterThanOrEqual(MIN_TARGET_PX);
    });

    // ----- UX-06: stage-save-status carries the mode -----

    test("stage_save_status_should_read_not_enforced_yet_When_saved_in_shadow_and_flag_a_stale_engine", async ({
      page,
      request,
    }) => {
      const slug = `e2e-engine-${vp.width}`;
      await mockEngineStatus(
        page,
        sequence(
          engineStatus({ modes: allModes("SHADOW") }),
          engineStatus({ modes: allModes("OFF"), invalidatorWired: false }),
        ),
      );
      const stages = new BacklogStagesSettingsPage(page);
      await stages.goto();
      await expect(stages.bannerText).toHaveText(SHADOW_COPY);

      try {
        await page.getByTestId("backlog-stage-new").click();
        await page.getByTestId("stage-form-slug").fill(slug);
        await page.getByTestId("stage-form-name").fill("E2E engine stage");
        await stages.formSubmit.click();
        await expect(stages.saveStatus).toHaveText("Saved. Not enforced yet (shadow)");
        await expect(stages.bannerText).toHaveText(OFF_COPY);

        await stages.openStageForm(slug);
        await stages.formSubmit.click();
        await expect(stages.saveStatus).toHaveText("Saved. Not in use (off) Saved, but the running engine was not refreshed.");
      } finally {
        await deleteStageBySlug(request, slug);
      }
    });

    // ----- UX-07: a system-required edge cannot be removed -----

    test("stage_form_should_alert_edge_name_keep_values_and_allow_cancel_When_system_required_edge_removed", async ({
      page,
    }) => {
      const stages = new BacklogStagesSettingsPage(page);
      await mockEngineStatus(page, engineStatus({ modes: allModes("SHADOW") }));
      await stages.goto();
      await stages.openStageForm("ready");
      await expect(stages.form).toBeVisible();

      const targets = page.getByTestId(/^stage-transition-\d+-to$/);
      await expect(targets.first()).toBeVisible();
      const queuedIndex = await targets.evaluateAll((els) => els.findIndex((el) => (el as HTMLSelectElement).value === "queued"));
      expect(queuedIndex, "ready should have an outgoing ready->queued transition").toBeGreaterThanOrEqual(0);
      const nameBefore = await page.getByTestId("stage-form-name").inputValue();
      await page.getByTestId(`stage-transition-remove-${queuedIndex}`).click();
      await stages.formSubmit.click();

      const alert = page.getByTestId("stage-form-edge-rejected");
      await expect(alert).toBeVisible();
      await expect(alert).toContainText("ready->queued");
      await expect(alert).toContainText("Automation relies on this transition");
      await expect(page.getByTestId("stage-form-name")).toHaveValue(nameBefore);

      await stages.formCancel.click();
      await expect(stages.form).toHaveCount(0);
    });

    // ----- UX-16: Revert to Off and Edit stages while enforced -----

    test("enforce_row_should_offer_revert_without_dialog_and_edit_stages_link_When_enforced", async ({ page }) => {
      const store = await FlagStore.install(page);
      store.set("backlog:workflow-engine-enforce", true);
      await mockEngineStatus(page, statusFollowing(store));
      const features = new FeaturesSettingsPage(page);
      await features.goto();

      await expect(features.switchFor("operator")).toHaveAttribute("aria-checked", "true");
      await expect(features.editStagesLink("operator")).toBeVisible();
      await features.revert("operator").click();

      await expect(features.dialog).toHaveCount(0);
      await expect(features.switchFor("operator")).toHaveAttribute("aria-checked", "false");
      await expect(features.rowStatus("operator")).toContainText("is now Off. 0 of 3 caller classes enforced.");
      await expect(features.revert("operator")).toHaveCount(0);
      await expect(features.editStagesLink("operator")).toHaveCount(0);
    });

    // ----- UX-37 / UX-47: hash focus -----

    test("banner_link_should_focus_the_first_unenforced_class_row_unobscured_and_unknown_hash_should_not_error", async ({
      page,
    }) => {
      const store = await FlagStore.install(page);
      store.set("backlog:workflow-engine-enforce", true); // operators done: the link must point at agents
      await mockEngineStatus(page, statusFollowing(store));
      const stages = new BacklogStagesSettingsPage(page);
      const features = new FeaturesSettingsPage(page);
      const consoleErrors: string[] = [];
      page.on("pageerror", (err) => consoleErrors.push(err.message));

      await stages.goto();
      await expect(stages.bannerText).toContainText("Partly enforced.");
      await stages.bannerFlagsLink.click();
      await expect(page).toHaveURL(new RegExp(`#${FLAG_ROW_HASH.agent}$`));
      const row = features.row("agent");
      await expect(row).toBeFocused();
      const covered = await row.evaluate((el) => {
        const r = el.getBoundingClientRect();
        const top = document.elementFromPoint(r.x + r.width / 2, r.y + Math.min(r.height / 2, 24));
        return !el.contains(top);
      });
      expect(covered, "sticky chrome must not cover the focused row").toBe(false);

      await features.goto("no-such-row");
      await expect(features.switchFor("agent")).toBeVisible();
      expect(consoleErrors).toEqual([]);
    });

    // UX-47 says Retry after a failed list focuses the hash row. It does not today: useHashRowFocus
    // counts the click (or Enter) on Retry as "the user already moved", so the mount-time hash attempt
    // never focuses. test.fail() keeps CI green while the defect stands and goes red the day it is fixed.
    test("hash_focus_should_focus_the_row_once_When_the_list_failed_then_Retry_succeeds", async ({ page }) => {
      test.fail(true, "UX-47 defect: a Retry click is treated as user interaction, so the hash row is never focused");
      const store = await FlagStore.install(page);
      const features = new FeaturesSettingsPage(page);
      // Answered by state, not by call count: the app shell also reads the list.
      let listFails = true;
      store.list.use(() => (listFails ? connectError("unavailable", "flags unavailable") : ok({ flags: store.rows })));
      await features.goto(FLAG_ROW_HASH.agent);
      await expect(features.loadError).toBeVisible();
      listFails = false;
      await features.loadError.getByRole("button", { name: "Retry" }).click();
      await expect(features.row("agent")).toBeVisible();
      await expect(features.row("agent")).toBeFocused({ timeout: 2000 });
    });

    test("hash_focus_should_not_steal_focus_follow_hashchange_and_clear_sticky_chrome", async ({ page }) => {
      const store = await FlagStore.install(page);
      const features = new FeaturesSettingsPage(page);

      // hashchange is the user's own navigation: it focuses the row.
      await features.goto(FLAG_ROW_HASH.agent);
      await expect(features.row("agent")).toBeFocused();
      await page.evaluate((id) => {
        window.location.hash = id;
      }, FLAG_ROW_HASH.system);
      await expect(features.row("system")).toBeFocused();

      // Scroll margins clear the sticky header and bottom nav (read from the shell's tokens).
      const margins = await features.row("system").evaluate((el) => {
        // The tokens are in rem; resolve them to pixels with a probe element.
        const px = (token: string) => {
          const probe = document.createElement("div");
          probe.style.cssText = `position:absolute;visibility:hidden;height:var(${token})`;
          document.body.appendChild(probe);
          const height = probe.getBoundingClientRect().height;
          probe.remove();
          return height;
        };
        const style = getComputedStyle(el);
        return {
          top: parseFloat(style.scrollMarginTop),
          bottom: parseFloat(style.scrollMarginBottom),
          header: px("--header-height"),
          nav: px("--bottom-nav-height"),
        };
      });
      expect(margins.top).toBeCloseTo(margins.header + SCROLL_CLEARANCE_PX, 0);
      expect(margins.bottom).toBeCloseTo(margins.nav + SCROLL_CLEARANCE_PX, 0);
      expect(margins.header, "the header token resolves").toBeGreaterThan(0);

      // A key press before the list renders means the mount-time hash must not take focus.
      const held = deferred();
      store.list.use(() => held.promise);
      await page.goto("about:blank"); // a hash-only goto from the same page would not reload it
      await features.goto(FLAG_ROW_HASH.agent);
      await expect(features.loading).toBeVisible();
      await whenHydrated(page, "feature-flags-loading"); // the skeleton is prerendered; a key press before hydration is invisible to the hook
      await page.keyboard.press("Shift");
      held.resolve(ok({ flags: store.rows }));
      await expect(features.row("agent")).toBeVisible();
      await expect(features.row("agent")).not.toBeFocused();
      expect(await page.evaluate(() => window.scrollY)).toBe(0);
    });

    // ----- UX-52: StageForm reminder in every state -----

    test("stage_form_reminder_should_match_banner_state_for_all_seven_states", async ({ page }) => {
      const stages = new BacklogStagesSettingsPage(page);
      const REMINDERS = [
        { spec: { modes: allModes("ENFORCE") }, text: "Mode: Enforced." },
        {
          spec: PARTLY_SPEC,
          text: "Mode: Partly enforced. Saved edits apply to operators only; agents and automation are in shadow.",
        },
        { spec: { modes: allModes("SHADOW") }, text: "Mode: Shadow. Saved edits are not enforced yet." },
        { spec: { modes: allModes("OFF") }, text: "Mode: Off. Edits are saved but have no effect." },
      ] as const;
      const status = await mockEngineStatus(page, engineStatus(REMINDERS[0].spec));

      for (const state of REMINDERS) {
        status.use(engineStatus(state.spec));
        await stages.goto();
        await stages.openStageForm("idea");
        await expect(stages.formReminder).toHaveText(state.text);
        await expectReminderAboveSave(stages);
      }

      // Loading: the form opens while the status is still pending.
      const pending = deferred();
      status.use(() => pending.promise);
      await stages.goto();
      await stages.openStageForm("idea");
      await expect(stages.formReminder).toHaveText("Mode: checking...");
      pending.resolve(engineStatus({ modes: allModes("SHADOW") }));
      await expect(stages.formReminder).toHaveText("Mode: Shadow. Saved edits are not enforced yet.");

      // Unknown: the RPC fails.
      status.use(connectError("unavailable", "down"));
      await stages.goto();
      await stages.openStageForm("idea");
      await expect(stages.formReminder).toHaveText("Mode: couldn't be determined. Treat saved edits as not applied.");
      await expect(stages.bannerText).toHaveText(UNKNOWN_COPY);
    });

    test("stage_form_reminder_should_say_stale_and_never_enforced_only_When_a_refetch_fails_after_a_good_value", async ({
      page,
    }) => {
      await page.clock.install();
      const status = await mockEngineStatus(page, engineStatus({ modes: allModes("SHADOW") }));
      const stages = new BacklogStagesSettingsPage(page);
      await stages.goto();
      await expect(stages.bannerText).toHaveText(SHADOW_COPY);
      await stages.openStageForm("idea");

      status.use(connectError("unavailable", "down"));
      await pastRefetchWindow(page);
      await wakeTab(page);

      await expect(stages.formReminder).toHaveText(
        /^Mode: Shadow \(last checked .+; couldn't refresh\)\. Saved edits may not be enforced\.$/,
      );
      await expect(stages.bannerText).toContainText("couldn't refresh");
      await expect(stages.bannerRetry).toBeVisible();
      await expect(stages.formReminder).not.toContainText("Mode: Enforced");
    });

    // ----- UX-55: review line in the banner and the Features summary -----

    test("review_line_should_show_due_overdue_neutral_or_absent_in_banner_and_features_summary", async ({ page }) => {
      const stages = new BacklogStagesSettingsPage(page);
      const features = new FeaturesSettingsPage(page);
      const status = await mockEngineStatus(
        page,
        engineStatus({
          modes: allModes("SHADOW"),
          review: { dueAt: "2026-10-01T12:00:00Z", state: "OVERDUE" },
        }),
      );
      const OVERDUE =
        "Review overdue: the soak review date (2026-10-01) has passed and no caller class is enforced. Review the soak evidence in docs/how-to/flip-workflow-engine-enforcement.md.";
      await FlagStore.install(page);

      await stages.goto();
      await expect(stages.review).toHaveText(OVERDUE);
      await expect(stages.bannerLive.getByTestId("engine-mode-review")).toHaveCount(0);
      await features.goto();
      await expect(features.review).toHaveText(OVERDUE);

      status.use(engineStatus({ modes: allModes("SHADOW"), review: { dueAt: "2026-11-03T12:00:00Z", state: "NOT_DUE" } }));
      await stages.goto();
      await expect(stages.review).toHaveText("Soak review due 2026-11-03.");
      await features.goto();
      await expect(features.review).toHaveText("Soak review due 2026-11-03.");

      status.use(engineStatus({ modes: allModes("ENFORCE"), review: { dueAt: "2026-11-03T12:00:00Z", state: "ENFORCED" } }));
      await stages.goto();
      await expect(stages.review).toHaveText("Soak review date 2026-11-03.");

      status.use(engineStatus({ modes: allModes("SHADOW"), review: { dueAt: "2026-11-03T12:00:00Z", state: "UNAVAILABLE" } }));
      await stages.goto();
      await expect(stages.bannerText).toHaveText(SHADOW_COPY);
      await expect(stages.review).toHaveCount(0);
      await features.goto();
      await expect(features.review).toHaveCount(0);
    });
  });
}

async function expectReminderAboveSave(stages: BacklogStagesSettingsPage): Promise<void> {
  // One evaluate, so a late layout shift (the liveness section loads above) cannot separate the two reads.
  const gap = await stages.formReminder.evaluate((reminder) => {
    const save = document.querySelector('[data-testid="stage-form-submit"]');
    if (!save) return Number.NaN;
    return save.getBoundingClientRect().top - reminder.getBoundingClientRect().bottom;
  });
  expect(gap, "the reminder sits directly above Save").toBeGreaterThanOrEqual(0);
  expect(gap).toBeLessThan(100);
}

async function deleteStageBySlug(request: import("@playwright/test").APIRequestContext, slug: string): Promise<void> {
  const headers = { "Content-Type": "application/json" };
  const list = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListStages`, { headers, data: {} });
  const body = (await list.json()) as { items?: Array<{ id: string; slug: string }> };
  const stage = body.items?.find((s) => s.slug === slug);
  if (stage) await request.post(`${BASE_URL}/api/session.v1.BacklogService/DeleteStage`, { headers, data: { id: stage.id } });
}
