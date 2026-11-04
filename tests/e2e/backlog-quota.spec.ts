// @feature settings-backlog-quota, settings-features, GetQuotaConfig, UpdateQuotaConfig
/**
 * Settings > Backlog Quota (backlog-make-it-true, Story 6.1.1; UX-18..26, 38, 49..51). The one task
 * the real server can prove end to end (gate On, budget 5,000,000, one Save, persisted after reload;
 * UX-26) runs against the isolated e2e server and restores the defaults afterwards. Read-backs that
 * differ, failing or pending saves and `quota_source` variants come from helpers/backlog-settings-mocks.ts.
 */
import { test, type Page } from "@playwright/test";
import {
  BASE_URL,
  FlagStore,
  QUOTA_DEFAULTS,
  connectError,
  deferred,
  echoQuotaUpdate,
  mockQuotaGet,
  mockQuotaUpdate,
  ok,
  quotaJson,
  quotaUpdateStoring,
  setFlagViaApi,
  skipOnboarding,
  type ProcedureMock,
  type QuotaValues,
  expect,
} from "./helpers/backlog-settings-mocks";
import { BacklogQuotaSettingsPage } from "./pages/BacklogQuotaSettingsPage";
import { FeaturesSettingsPage } from "./pages/FeaturesSettingsPage";

const VIEWPORTS = [
  { name: "desktop-1280x800", width: 1280, height: 800 },
  { name: "mobile-360x800", width: 360, height: 800 },
] as const;

const MIN_TARGET_PX = 44;
const NO_BUDGET_WARNING = "Gate is on but no budget is set, so the proactive signal never fires";
const SUM_ERROR = "Pause threshold plus resume margin must be at most 100";

interface Mocked {
  quota: BacklogQuotaSettingsPage;
  get: ProcedureMock;
  update: ProcedureMock;
}

/** Quota page with both RPCs mocked: loads `initial`, and an update stores what was sent. */
async function mockedQuota(
  page: Page,
  initial: QuotaValues = QUOTA_DEFAULTS,
  source: "NOT_CONFIGURED" | "CONFIGURED" = "CONFIGURED",
): Promise<Mocked> {
  const get = await mockQuotaGet(page, ok(quotaJson(initial, source)));
  const update = await mockQuotaUpdate(page, echoQuotaUpdate());
  return { quota: new BacklogQuotaSettingsPage(page), get, update };
}

/** The first visible link to another app page (bottom nav on a phone, side nav on desktop). */
function otherPageLink(page: Page) {
  return page.getByRole("link", { name: /^Sessions/ }).filter({ visible: true }).first();
}

/** True when a `beforeunload` listener asks the browser to confirm leaving. */
function beforeUnloadGuarded(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const event = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(event);
    return event.defaultPrevented;
  });
}

for (const vp of VIEWPORTS) {
  test.describe(`backlog quota @ ${vp.name}`, () => {
    test.use({ viewport: { width: vp.width, height: vp.height } });
    test.describe.configure({ timeout: 90_000 }); // multi-step flows on a loaded machine outrun the 30 s default
    const isMobile = vp.width < 400;

    test.beforeAll(async ({ request }) => {
      await setFlagViaApi(request, "backlog", true);
    });

    test.afterAll(async ({ request }) => {
      await setFlagViaApi(request, "backlog", false);
    });

    test.beforeEach(async ({ page }) => {
      await skipOnboarding(page);
    });

    // ===== UX-26 (real server): one save, no detour, persisted =====

    test("quota_task_should_apply_and_persist_with_one_save_and_no_detour_When_changing_gate_budget", async ({
      page,
      request,
    }) => {
      const quota = new BacklogQuotaSettingsPage(page);
      const dialogs: string[] = [];
      const detours: string[] = [];
      page.on("dialog", (d) => dialogs.push(d.message()));
      try {
        await quota.goto();
        await expect(quota.form).toBeVisible();
        const url = page.url();
        // The leave guard's same-URL history sentinel also fires `framenavigated`; a detour is another URL or a new document.
        page.on("framenavigated", (frame) => {
          if (frame === page.mainFrame() && frame.url() !== url) detours.push(frame.url());
        });
        page.on("load", () => detours.push("document load"));

        await quota.gate.check();
        await quota.fill("budget", "5000000");
        await quota.save.click();

        await expect(quota.status).toHaveText("Applied");
        await expect(quota.effective).toHaveText("Effective now: gate On, budget 5,000,000 tokens");
        expect(dialogs).toEqual([]);
        expect(detours).toEqual([]);

        page.removeAllListeners("load");
        await page.reload();
        await expect(quota.gate).toBeChecked();
        await expect(quota.field("budget")).toHaveValue("5000000");
        await expect(quota.effective).toHaveText("Effective now: gate On, budget 5,000,000 tokens");
      } finally {
        await request.post(`${BASE_URL}/api/session.v1.QuotaService/UpdateQuotaConfig`, {
          headers: { "Content-Type": "application/json" },
          data: { settings: { enabled: false, pauseBelowHeadroomPct: 20, resumeMarginPct: 15, assumedWindowTokenBudget: "0" } },
        });
      }
    });

    // ===== UX-18: reachable from the Settings index and the Features row =====

    test("quota_page_should_be_reachable_from_settings_index_link_and_features_row", async ({ page }) => {
      const quota = new BacklogQuotaSettingsPage(page);
      await page.goto(`${BASE_URL}/settings`, { waitUntil: "domcontentloaded" });
      await page.getByTestId("settings-backlog-quota-link").click();
      await expect(page).toHaveURL(/\/settings\/backlog-quota\/?$/);
      await expect(page.getByRole("heading", { name: "Backlog Quota", level: 1 })).toBeVisible();

      const features = new FeaturesSettingsPage(page);
      await features.goto();
      await page.getByTestId("feature-flag-quota-link").click();
      await expect(page).toHaveURL(/\/settings\/backlog-quota\/?$/);
      await expect(quota.root).toBeVisible();
    });

    // ===== UX-19 / UX-20 / UX-21 =====

    test("quota_should_show_static_no_budget_warning_and_zero_helper_When_gate_on_and_budget_zero", async ({ page }) => {
      const { quota, update } = await mockedQuota(page);
      await quota.goto();
      await expect(quota.form).toBeVisible();
      await expect(quota.warning).toHaveCount(0);
      await expect(page.getByText("0 = proactive headroom signal disabled")).toBeVisible();

      await quota.gate.check();
      await expect(quota.warning).toBeVisible();
      await expect(quota.warning).toContainText(NO_BUDGET_WARNING);
      await expect(quota.warning).not.toHaveAttribute("role", /.+/); // static text: only the status region announces it
      await expect(quota.status).toHaveText(NO_BUDGET_WARNING);
      expect(await quota.field("budget").getAttribute("aria-describedby")).toContain("backlog-quota-warning");

      // Saving budget 0 with the gate on keeps the warning.
      await quota.save.click();
      await expect(quota.status).toHaveText("Applied");
      await expect(quota.warning).toBeVisible();
      expect(update.calls).toBe(1);

      await quota.fill("budget", "5000000");
      await expect(quota.warning).toHaveCount(0);
      await expect(quota.status).toHaveText("Budget warning cleared");
    });

    test("quota_should_show_inline_error_keep_values_and_link_summary_When_pause_60_resume_50", async ({ page }) => {
      const { quota, update } = await mockedQuota(page);
      await quota.goto();
      await quota.fill("pause", "60");
      await quota.fill("resume", "50");
      await quota.save.click();

      await expect(quota.errors).toBeVisible();
      expect(await quota.errors.getAttribute("role")).toBe("alert");
      await expect(page.getByText(SUM_ERROR).first()).toBeVisible();
      await expect(quota.field("pause")).toHaveAttribute("aria-invalid", "true");
      await expect(quota.field("resume")).toHaveAttribute("aria-invalid", "true");
      const describedText = await quota.field("pause").evaluate((el) =>
        (el.getAttribute("aria-describedby") ?? "")
          .split(" ")
          .map((id) => document.getElementById(id)?.textContent ?? "")
          .join(" "),
      );
      expect(describedText).toContain(SUM_ERROR);
      await expect(quota.field("pause")).toHaveValue("60");
      await expect(quota.field("resume")).toHaveValue("50");
      expect(update.calls).toBe(0);

      await quota.errors.getByRole("button", { name: SUM_ERROR }).click();
      await expect(quota.field("pause")).toBeFocused();

      await quota.fill("pause", "40");
      await expect(quota.errors).toHaveCount(0);
      await quota.save.click();
      await expect(quota.status).toHaveText("Applied");
    });

    test("quota_save_should_show_applied_When_read_back_matches", async ({ page }) => {
      const { quota } = await mockedQuota(page);
      await quota.goto();
      await quota.gate.check();
      await quota.fill("budget", "2500000");
      await quota.save.click();
      await expect(quota.status).toHaveText("Applied");
      await expect(quota.effective).toHaveText("Effective now: gate On, budget 2,500,000 tokens");
    });

    // ===== UX-22 / UX-23 =====

    test("quota_save_failure_should_keep_values_with_retry_and_revert_should_restore", async ({ page }) => {
      const { quota, update } = await mockedQuota(page);
      await quota.goto();
      update.use(connectError("internal", "disk full"));
      await quota.fill("budget", "123");
      await quota.save.click();

      await expect(quota.saveError).toBeVisible();
      expect(await quota.saveError.getAttribute("role")).toBe("alert");
      await expect(quota.saveError).toContainText("Couldn't save");
      await expect(quota.field("budget")).toHaveValue("123");
      await expect(quota.saveError.getByRole("button", { name: "Retry" })).toBeVisible();

      await quota.revert.click();
      await expect(quota.field("budget")).toHaveValue("0");
      await expect(quota.saveError).toHaveCount(0);
      await expect(quota.errors).toHaveCount(0);

      await quota.fill("budget", "77");
      update.use(echoQuotaUpdate());
      await quota.save.click();
      await expect(quota.status).toHaveText("Applied");
    });

    test("quota_load_failure_should_hide_form_and_loading_should_be_busy", async ({ page }) => {
      const { quota, get } = await mockedQuota(page);
      const held = deferred();
      get.use(() => held.promise);
      await quota.goto();
      await expect(quota.load).toHaveAttribute("aria-busy", "true");
      await expect(quota.form).toHaveCount(0);
      await expect(page.getByTestId("backlog-quota-budget")).toHaveCount(0);

      held.resolve(connectError("unavailable", "quota unavailable"));
      await expect(quota.loadError).toBeVisible();
      expect(await quota.loadError.getAttribute("role")).toBe("alert");
      await expect(quota.loadRetry).toBeVisible();
      await expect(quota.form).toHaveCount(0);

      get.use(ok(quotaJson(QUOTA_DEFAULTS)));
      await quota.loadRetry.click();
      await expect(quota.form).toBeVisible();
    });

    // ===== UX-24: layout at both widths =====

    test("quota_form_should_meet_mobile_and_desktop_layout_When_360_and_1280", async ({ page }) => {
      const { quota } = await mockedQuota(page);
      await quota.goto();
      await expect(quota.form).toBeVisible();

      const layout = await page.evaluate(() => {
        const box = (id: string) => document.querySelector(`[data-testid="${id}"]`)!.getBoundingClientRect();
        const label = (id: string) => document.querySelector(`label[for="${id}"]`)!.getBoundingClientRect();
        const root = document.scrollingElement as HTMLElement;
        return {
          noHScroll: root.scrollWidth <= root.clientWidth,
          inputHeights: ["budget", "pause", "resume", "usd"].map((k) => box(`backlog-quota-${k}`).height),
          labelAbove: ["budget", "pause", "resume", "usd"].every(
            (k) => label(`backlog-quota-${k}`).bottom <= box(`backlog-quota-${k}`).top + 1,
          ),
          pause: box("backlog-quota-pause"),
          resume: box("backlog-quota-resume"),
          usd: box("backlog-quota-usd"),
          save: box("backlog-quota-save"),
        };
      });
      expect(layout.noHScroll).toBe(true);
      expect(layout.labelAbove).toBe(true);
      expect(layout.save.top, "Save follows the last field").toBeGreaterThanOrEqual(layout.usd.bottom);
      if (isMobile) {
        for (const h of layout.inputHeights) expect(h).toBeGreaterThanOrEqual(MIN_TARGET_PX);
        expect(layout.resume.top).toBeGreaterThan(layout.pause.top); // one column
        await expect(quota.field("budget")).toHaveAttribute("inputmode", "numeric");
        await expect(quota.field("budget")).toHaveAttribute("pattern", "[0-9]*");
        await expect(quota.field("pause")).toHaveAttribute("inputmode", "numeric");
        await expect(quota.field("usd")).toHaveAttribute("inputmode", "decimal");
      } else {
        expect(layout.resume.left, "pause and resume sit in two columns").toBeGreaterThan(layout.pause.left);
        expect(Math.abs(layout.resume.top - layout.pause.top)).toBeLessThan(2);
      }
    });

    // ===== UX-25 =====

    test("quota_should_show_backlog_resumed_and_help_text_When_disabling_paused_gate", async ({ page }) => {
      const store = await FlagStore.install(page);
      store.get("backlog").statusDetail = "Paused by the quota gate until the window resets";
      const { quota } = await mockedQuota(page, { ...QUOTA_DEFAULTS, enabled: true, budget: "5000000" });
      await quota.goto();
      await expect(quota.gate).toBeChecked();
      await expect(
        page.getByText(
          "Turning the gate on also throttles foreground dispatch. Toggling counts as a manual override with a notification grace window. Turning it off resumes backlog if the gate had paused it.",
        ),
      ).toBeVisible();

      await quota.gate.uncheck();
      await quota.save.click();
      await expect(quota.status).toHaveText("Applied. Backlog resumed");
    });

    // ===== UX-38 / UX-51: leave guard =====

    test("quota_should_guard_navigation_When_unsaved_edits", async ({ page }) => {
      const { quota } = await mockedQuota(page);
      await quota.goto();
      expect(await beforeUnloadGuarded(page)).toBe(false);

      await quota.fill("budget", "42");
      expect(await beforeUnloadGuarded(page)).toBe(true);
      await otherPageLink(page).click();

      await expect(quota.dialog).toBeVisible();
      await expect(quota.dialog).toContainText("Discard unsaved changes?");
      await expect(page.getByTestId("alert-confirm-cancel")).toHaveText("Keep editing");
      await expect(page.getByTestId("alert-confirm-cancel")).toBeFocused();

      await page.getByTestId("alert-confirm-cancel").click();
      await expect(quota.dialog).toHaveCount(0);
      await expect(page).toHaveURL(/\/settings\/backlog-quota\/?$/);
      await expect(quota.field("budget")).toHaveValue("42");

      await otherPageLink(page).click();
      await expect(quota.dialog).toBeVisible();
      await page.getByTestId("alert-confirm-confirm").click();
      await expect(page).not.toHaveURL(/backlog-quota/);
    });

    test("quota_should_guard_back_navigation_keep_url_and_skip_sentinel_When_clean", async ({ page }) => {
      const { quota, update } = await mockedQuota(page);
      await page.goto(`${BASE_URL}/settings`, { waitUntil: "domcontentloaded" });
      await page.getByTestId("settings-backlog-quota-link").click();
      await expect(quota.form).toBeVisible();
      const quotaUrl = page.url();

      // Dirty + Back: the dialog, no navigation, values intact.
      await quota.fill("budget", "42");
      const historyLength = await page.evaluate(() => history.length);
      await page.goBack();
      await expect(quota.dialog).toBeVisible();
      await expect(page.getByTestId("alert-confirm-cancel")).toBeFocused();
      expect(page.url()).toBe(quotaUrl);
      await expect(quota.field("budget")).toHaveValue("42");

      // Keep editing re-arms the guard without growing the history.
      await page.getByTestId("alert-confirm-cancel").click();
      expect(await page.evaluate(() => history.length)).toBe(historyLength);
      await page.goBack();
      await expect(quota.dialog).toBeVisible();
      expect(await page.evaluate(() => history.length)).toBe(historyLength);

      // Discard reaches the page before the form.
      await page.getByTestId("alert-confirm-confirm").click();
      await expect(page).toHaveURL(/\/settings\/?$/);

      // A clean form skips the sentinel: one Back reaches the previous page.
      await page.getByTestId("settings-backlog-quota-link").click();
      await expect(quota.form).toBeVisible();
      await quota.fill("budget", "43");
      await quota.save.click();
      await expect(quota.status).toHaveText("Applied");
      expect(update.calls).toBe(1);
      await page.goBack();
      await expect(page).toHaveURL(/\/settings\/?$/);
    });

    test("quota_discard_via_link_should_replace_the_sentinel_and_beforeunload_should_hold_during_a_pending_save", async ({
      page,
    }) => {
      const { quota, update } = await mockedQuota(page);
      await page.goto(`${BASE_URL}/settings`, { waitUntil: "domcontentloaded" });
      await page.getByTestId("settings-backlog-quota-link").click();
      await expect(quota.form).toBeVisible();

      await quota.fill("budget", "42");
      await otherPageLink(page).click();
      await page.getByTestId("alert-confirm-confirm").click();
      await expect(page).not.toHaveURL(/backlog-quota/);
      await page.goBack();
      await expect(page).toHaveURL(/backlog-quota\/?$/); // replace: Back returns to the form exactly once

      // beforeunload stays registered while a save is in flight.
      const held = deferred();
      update.use(() => held.promise);
      await expect(quota.form).toBeVisible();
      await quota.gate.check();
      await quota.fill("budget", "9");
      await quota.save.click();
      await expect(quota.status).toHaveText("Saving...");
      expect(await beforeUnloadGuarded(page)).toBe(true);
      held.resolve(ok(quotaJson({ ...QUOTA_DEFAULTS, enabled: true, budget: "9" })));
      await expect(quota.status).toHaveText("Applied");
      expect(await beforeUnloadGuarded(page)).toBe(false);
    });

    // ===== UX-49 / UX-50 =====

    test("quota_should_have_one_status_region_and_recover_from_mismatch_with_focus_handoff", async ({ page }) => {
      const { quota, update } = await mockedQuota(page);
      await quota.goto();
      const held = deferred();
      const oneStatusAtMostOneAlert = async (step: string) => {
        await expect(quota.statusRegions(), step).toHaveCount(1);
        expect(await quota.root.getByRole("alert").count(), `${step}: alerts`).toBeLessThanOrEqual(1);
      };

      await oneStatusAtMostOneAlert("loaded");
      await quota.gate.check();
      await oneStatusAtMostOneAlert("warning appeared");
      await quota.fill("budget", "7000000");
      await oneStatusAtMostOneAlert("edited");
      update.use(() => held.promise);
      await quota.save.click();
      await expect(quota.status).toHaveText("Saving...");
      await oneStatusAtMostOneAlert("save pending");

      // The server stores a different budget: the mismatch alert takes focus.
      held.resolve(ok(quotaJson({ ...QUOTA_DEFAULTS, enabled: true, budget: "6000000" })));
      await expect(quota.mismatch).toBeVisible();
      await expect(quota.mismatch).toBeFocused();
      await expect(quota.mismatch).toContainText("Assumed window token budget: Entered 7000000, effective now 6000000");
      await oneStatusAtMostOneAlert("mismatch");

      // Reload effective values replaces the fields, clears dirty and focuses the first differing field.
      await quota.mismatchReload.click();
      await expect(quota.field("budget")).toHaveValue("6000000");
      await expect(quota.field("budget")).toBeFocused();
      await expect(quota.revert).toBeDisabled();
      await expect(quota.mismatch).toHaveCount(0);

      // Save again re-submits the entered values.
      update.use(quotaUpdateStoring({ assumedWindowTokenBudget: "6000000" }));
      await quota.fill("budget", "7000000");
      await quota.save.click();
      await expect(quota.mismatch).toBeVisible();
      const callsBefore = update.calls;
      update.use(echoQuotaUpdate());
      await quota.mismatchSaveAgain.click();
      await expect(quota.status).toHaveText("Applied");
      expect(update.calls).toBe(callsBefore + 1);
      await oneStatusAtMostOneAlert("recovered");
    });

    test("quota_should_show_not_configured_only_for_not_configured_source", async ({ page }) => {
      const { quota, get } = await mockedQuota(page, QUOTA_DEFAULTS, "NOT_CONFIGURED");
      await quota.goto();
      await expect(quota.notConfigured).toHaveText("Quota is not configured. Defaults apply: gate Off, no budget.");
      await expect(quota.form).toBeVisible();

      get.use(ok(quotaJson(QUOTA_DEFAULTS, "CONFIGURED")));
      await quota.goto();
      await expect(quota.form).toBeVisible();
      await expect(quota.notConfigured).toHaveCount(0);

      get.use(ok(quotaJson(QUOTA_DEFAULTS, "UNSPECIFIED")));
      await quota.goto();
      await expect(quota.loadError).toContainText("the server response was incomplete");
      await expect(quota.form).toHaveCount(0);
      await expect(quota.notConfigured).toHaveCount(0);
    });
  });
}
