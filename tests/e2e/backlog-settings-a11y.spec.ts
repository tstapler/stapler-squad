// @feature settings-backlog-stages, settings-features, settings-backlog-quota, backlog:list-stuck, accessibility
/**
 * Cross-surface accessibility for the four backlog-settings surfaces (Stages banner, Features flags,
 * Backlog Quota, stuck auto-merge line) at 360x800 and 1280x800 (backlog-make-it-true, Story 6.1.1;
 * UX-31..34, 41, 54). All states come from helpers/backlog-settings-mocks.ts so failed, inert and
 * dialog states can be reached on demand; the theme is switched through the real picker (ThemeToggle).
 */
import AxeBuilder from "@axe-core/playwright";
import { test, type Page } from "@playwright/test";
import {
  FlagStore,
  addKeyboardScrollRoom,
  QUOTA_DEFAULTS,
  DIFFERENCES,
  STUCK_ITEM_TITLE,
  connectError,
  echoQuotaUpdate,
  mockEngineStatus,
  mockQuotaGet,
  mockQuotaUpdate,
  mockStuckList,
  ok,
  quotaJson,
  setFlagViaApi,
  skipOnboarding,
  statusFollowing,
  stuckReport,
  expect,
} from "./helpers/backlog-settings-mocks";
import { setKeyboardOpen } from "./helpers/viewport-profiles";
import { BacklogQuotaSettingsPage } from "./pages/BacklogQuotaSettingsPage";
import { BacklogStagesSettingsPage } from "./pages/BacklogStagesSettingsPage";
import { FeaturesSettingsPage, FLAG_LABELS } from "./pages/FeaturesSettingsPage";
import { StuckItemsPage } from "./pages/StuckItemsPage";
import { ThemeToggle, type Scheme } from "./pages/ThemeToggle";

const VIEWPORTS = [
  { name: "desktop-1280x800", width: 1280, height: 800 },
  { name: "mobile-360x800", width: 360, height: 800 },
] as const;

const SCHEMES: Scheme[] = ["Light", "Dark"];
const WCAG_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];
const MIN_TARGET_PX = 44;
const MIN_DIALOG_GAP_PX = 16;
const MAX_TAB_STOPS = 120;
const KEYBOARD_PX = 300;
/** Marker for a focus stop outside the page content (the app shell), which is not judged. */
const UNJUDGED = "unjudged";
const FOCUS_OK = "ok";

/** Every mock the four surfaces read, installed once per page. */
async function installMocks(page: Page) {
  const store = await FlagStore.install(page);
  const status = await mockEngineStatus(page, statusFollowing(store, { readiness: { operator: DIFFERENCES } }));
  const quotaGet = await mockQuotaGet(page, ok(quotaJson(QUOTA_DEFAULTS)));
  const quotaUpdate = await mockQuotaUpdate(page, echoQuotaUpdate());
  const stuck = await mockStuckList(page, stuckReport({ lookup: "OK", allowAutoMerge: false, checkedAt: new Date().toISOString() }));
  return { store, status, quotaGet, quotaUpdate, stuck };
}

/** Every WCAG 2.2 AA violation on the page as `rule (impact): nodes, e.g. target - why`. */
async function axeViolations(page: Page): Promise<string[]> {
  const result = await new AxeBuilder({ page }).withTags(WCAG_TAGS).analyze();
  return result.violations
    .map(
      (v) => `${v.id} (${v.impact}): ${v.nodes.length} node(s), e.g. ${v.nodes[0]?.target.join(" ")} - ${v.nodes[0]?.failureSummary?.split("\n")[1] ?? ""}`,
    );
}

/**
 * null when the focused element is inside the viewport and nothing sits on top of its centre. Only the
 * page content (`main`) and an open dialog are judged: the app shell's own nav is not this feature's.
 */
function focusProblem(page: Page): Promise<string> {
  return page.evaluate(([UNJUDGED, FOCUS_OK]) => {
    const el = document.activeElement as HTMLElement | null;
    if (!el || el === document.body || !el.closest("main, [role=alertdialog]")) return UNJUDGED;
    const r = el.getBoundingClientRect();
    const name = el.getAttribute("data-testid") ?? el.getAttribute("aria-label") ?? el.tagName;
    if (r.width === 0 || r.height === 0) return `${name}: zero size`;
    if (r.bottom <= 0 || r.top >= innerHeight || r.right <= 0 || r.left >= innerWidth) return `${name}: outside the viewport`;
    const x = Math.min(Math.max(r.left + r.width / 2, 0), innerWidth - 1);
    const y = Math.min(Math.max(r.top + Math.min(r.height / 2, 12), 0), innerHeight - 1);
    const hit = document.elementFromPoint(x, y);
    const fine = hit && (hit === el || el.contains(hit) || hit.contains(el));
    return fine ? FOCUS_OK : `${name}: covered by ${hit?.getAttribute("data-testid") ?? hit?.tagName}`;
  }, [UNJUDGED, FOCUS_OK]);
}

/** Tabs to the element with this test id; returns every unobscured-focus problem met on the way. */
async function tabToTestId(page: Page, testId: string): Promise<string[]> {
  const problems: string[] = [];
  for (let i = 0; i < MAX_TAB_STOPS; i++) {
    await page.keyboard.press("Tab");
    const problem = await focusProblem(page);
    if (problem) problems.push(problem);
    const current = await page.evaluate(() => document.activeElement?.getAttribute("data-testid") ?? "");
    if (current === testId) return problems;
  }
  throw new Error(`could not Tab to [data-testid=${testId}]`);
}

async function tabToLabel(page: Page, label: string): Promise<string[]> {
  const problems: string[] = [];
  for (let i = 0; i < MAX_TAB_STOPS; i++) {
    await page.keyboard.press("Tab");
    const problem = await focusProblem(page);
    if (problem) problems.push(problem);
    const current = await page.evaluate(() => document.activeElement?.getAttribute("aria-label") ?? "");
    if (current === label) return problems;
  }
  throw new Error(`could not Tab to ${label}`);
}

function noHorizontalScroll(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const root = document.scrollingElement as HTMLElement;
    return root.scrollWidth <= root.clientWidth;
  });
}

for (const vp of VIEWPORTS) {
  test.describe(`backlog settings a11y @ ${vp.name}`, () => {
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

    // ===== UX-32: Axe on the four surfaces, in both themes, including failed / inert / dialog states =====

    for (const scheme of SCHEMES) {
      test(`four_pages_should_have_zero_axe_wcag22aa_violations_in_${scheme.toLowerCase()}`, async ({ page }) => {
        const mocks = await installMocks(page);
        await new ThemeToggle(page).select(scheme);
        const stages = new BacklogStagesSettingsPage(page);
        const features = new FeaturesSettingsPage(page);
        const quota = new BacklogQuotaSettingsPage(page);
        const stuck = new StuckItemsPage(page);
        const found: Record<string, string[]> = {};
        const check = async (state: string) => {
          const violations = await axeViolations(page);
          if (violations.length) found[state] = violations;
        };

        // Stages: shadow, unknown (failed status) and the stage form with its reminder.
        await stages.goto();
        await expect(stages.bannerText).toContainText("Shadow:");
        await check("stages: shadow banner");
        mocks.status.use(connectError("unavailable", "down"));
        await stages.goto();
        await expect(stages.bannerText).toContainText("Couldn't determine");
        await check("stages: unknown banner");
        mocks.status.use(statusFollowing(mocks.store));
        await stages.goto();
        await stages.openStageForm("idea");
        await expect(stages.formReminder).toBeVisible();
        await check("stages: stage form with mode reminder");

        // Features: default, inert row, failed toggle, enforce dialog with unclean evidence.
        mocks.store.set("autonomous_stall_nudge_enabled", true);
        mocks.store.get("autonomous_stall_nudge_enabled").statusDetail = "inactive: Diagnose & Nudge is off; Default: Off (not set explicitly)";
        await features.goto();
        await expect(features.row("stall").getByTestId("feature-flag-inert-note")).toBeVisible();
        await check("features: list with an inert row");
        mocks.store.update.use(connectError("internal", "disk full"));
        await features.switchFor("diagnose").click();
        await expect(features.rowError("diagnose")).toBeVisible();
        await check("features: failed toggle alert");
        mocks.status.use(statusFollowing(mocks.store, { readiness: { operator: DIFFERENCES } }));
        await features.openEnforceDialog("operator");
        await expect(features.ack).toBeVisible();
        await check("features: enforce dialog with unclean evidence");

        // Quota: default, gate on with no budget plus a validation error, load failure.
        await quota.goto();
        await expect(quota.form).toBeVisible();
        await check("quota: default form");
        await quota.gate.check();
        await quota.fill("pause", "60");
        await quota.fill("resume", "50");
        await quota.save.click();
        await expect(quota.errors).toBeVisible();
        await check("quota: warning and validation error");
        mocks.quotaGet.use(connectError("unavailable", "down"));
        await quota.goto();
        await expect(quota.loadError).toBeVisible();
        await check("quota: load failure");

        // Stuck panel: allowed-false line, then a failed lookup.
        await stuck.goto();
        await stuck.openDetail(STUCK_ITEM_TITLE);
        await expect(stuck.autoMergeLine()).toBeVisible();
        await check("stuck: auto-merge not allowed");
        mocks.stuck.use(stuckReport({ lookup: "RATE_LIMITED", checkedAt: new Date().toISOString() }));
        await stuck.goto();
        await stuck.openDetail(STUCK_ITEM_TITLE);
        await expect(stuck.autoMergeRetry()).toBeVisible();
        await check("stuck: failed lookup with Retry");

        expect(found).toEqual({});
      });
    }

    // ===== UX-31: every failed state has a keyboard-reachable exit =====

    test("every_failed_state_should_expose_keyboard_reachable_exit_When_each_surface_errors", async ({ page }) => {
      const mocks = await installMocks(page);
      const stages = new BacklogStagesSettingsPage(page);
      const features = new FeaturesSettingsPage(page);
      const quota = new BacklogQuotaSettingsPage(page);
      const stuck = new StuckItemsPage(page);

      // S1: Stages status failure -> Retry restores the banner.
      mocks.status.use(connectError("unavailable", "down"));
      await stages.goto();
      await expect(stages.bannerText).toContainText("Couldn't determine");
      mocks.status.use(statusFollowing(mocks.store));
      await tabToTestId(page, "engine-mode-banner-retry");
      await page.keyboard.press("Enter");
      await expect(stages.bannerText).toContainText("Shadow:");

      // S2: Features list failure -> Retry.
      mocks.store.list.use(connectError("unavailable", "down"));
      await features.goto();
      await expect(features.loadError).toBeVisible();
      mocks.store.list.use(() => ok({ flags: mocks.store.rows }));
      await features.loadError.getByRole("button", { name: "Retry" }).focus();
      await page.keyboard.press("Enter");
      await expect(features.switchFor("diagnose")).toBeVisible();

      // S3: a toggle failure -> Retry from the row alert.
      mocks.store.update.use(connectError("internal", "disk full"));
      await features.switchFor("diagnose").focus();
      await page.keyboard.press("Space");
      await expect(features.rowError("diagnose")).toBeVisible();
      mocks.store.update.use((_n, body) => mocks.store.applyUpdate(body));
      await features.rowError("diagnose").getByRole("button", { name: "Retry" }).focus();
      await page.keyboard.press("Enter");
      await expect(features.switchFor("diagnose")).toHaveAttribute("aria-checked", "true");

      // S4: quota load failure -> Retry; save failure -> Retry.
      mocks.quotaGet.use(connectError("unavailable", "down"));
      await quota.goto();
      await expect(quota.loadError).toBeVisible();
      mocks.quotaGet.use(ok(quotaJson(QUOTA_DEFAULTS)));
      await tabToTestId(page, "backlog-quota-retry");
      await page.keyboard.press("Enter");
      await expect(quota.form).toBeVisible();
      mocks.quotaUpdate.use(connectError("internal", "disk full"));
      await quota.fill("budget", "5");
      await quota.field("budget").press("Enter"); // Enter moves to the next field, it does not submit
      await quota.save.focus();
      await page.keyboard.press("Enter");
      await expect(quota.saveError).toBeVisible();
      mocks.quotaUpdate.use(echoQuotaUpdate());
      await quota.saveError.getByRole("button", { name: "Retry" }).focus();
      await page.keyboard.press("Enter");
      await expect(quota.status).toHaveText("Applied");

      // S5: auto-merge lookup failure -> Retry.
      mocks.stuck.use(stuckReport({ lookup: "ERROR", checkedAt: new Date().toISOString() }));
      await stuck.goto();
      await stuck.openDetail(STUCK_ITEM_TITLE);
      await expect(stuck.autoMergeRetry()).toBeVisible();
      mocks.stuck.use(stuckReport({ lookup: "OK", allowAutoMerge: true, checkedAt: new Date().toISOString() }));
      await stuck.autoMergeRetry().focus();
      await page.keyboard.press("Enter");
      await expect(stuck.autoMergeLine()).toContainText("Repo auto-merge: allowed");
    });

    // ===== UX-33: keyboard-only primary tasks with visible, unobscured focus =====

    test("five_surfaces_should_be_keyboard_operable_with_visible_unobscured_focus", async ({ page }) => {
      const mocks = await installMocks(page);
      const stages = new BacklogStagesSettingsPage(page);
      const features = new FeaturesSettingsPage(page);
      const quota = new BacklogQuotaSettingsPage(page);
      const stuck = new StuckItemsPage(page);
      const obscured: string[] = [];

      // Stages: open an edit form and cancel it.
      await stages.goto();
      await expect(stages.bannerText).toContainText("Shadow:");
      obscured.push(...(await tabToTestId(page, "backlog-stage-edit-idea")));
      await page.keyboard.press("Enter");
      await expect(stages.form).toBeVisible();
      // The form opens above the list: focus must follow it, not stay on a button it pushed below the fold.
      await expect(stages.form.getByTestId("stage-form-name")).toBeFocused();
      // Its transitions, graph and liveness sections load after mount and grow it; walk the settled form.
      await expect(stages.form).toHaveAttribute("aria-busy", "false");
      await expect(stages.form.getByText("Loading liveness overrides…")).toHaveCount(0);
      obscured.push(...(await tabToTestId(page, "stage-form-cancel")));
      await page.keyboard.press("Enter");
      await expect(stages.form).toHaveCount(0);
      await expect(page.getByTestId("backlog-stage-edit-idea")).toBeFocused();

      // Features: toggle a flag, then open and escape the enforce dialog.
      await features.goto();
      await expect(features.switchFor("diagnose")).toBeVisible();
      obscured.push(...(await tabToLabel(page, FLAG_LABELS.diagnose)));
      await page.keyboard.press("Space");
      await expect(features.rowStatus("diagnose")).toHaveText("Diagnose & Nudge is now On.");
      obscured.push(...(await tabToLabel(page, FLAG_LABELS.operator)));
      await page.keyboard.press("Space");
      await expect(features.dialog).toBeVisible();
      obscured.push((await focusProblem(page)) ?? FOCUS_OK);
      await page.keyboard.press("Escape");
      await expect(features.dialog).toHaveCount(0);

      // Quota: gate, budget and Save without a pointer.
      await quota.goto();
      await expect(quota.form).toBeVisible();
      obscured.push(...(await tabToTestId(page, "backlog-quota-enabled")));
      await page.keyboard.press("Space");
      obscured.push(...(await tabToTestId(page, "backlog-quota-budget")));
      await page.keyboard.type("5000000");
      obscured.push(...(await tabToTestId(page, "backlog-quota-save")));
      await page.keyboard.press("Enter");
      await expect(quota.status).toHaveText("Applied");

      // Stuck: expand the card and reach Retry.
      mocks.stuck.use(stuckReport({ lookup: "RATE_LIMITED", checkedAt: new Date().toISOString() }));
      await stuck.goto();
      await stuck.cardByTitle(STUCK_ITEM_TITLE).getByTestId("stuck-item-toggle").focus();
      await page.keyboard.press("Enter");
      await expect(page.getByTestId("stuck-item-detail")).toBeVisible();
      obscured.push(...(await tabToTestId(page, "stuck-item-auto-merge-retry")));
      expect(await page.evaluate(() => document.activeElement?.getAttribute("data-testid"))).toBe("stuck-item-auto-merge-retry");

      const judged = obscured.filter((p) => p !== UNJUDGED);
      expect(judged.length, "page-content stops were actually judged").toBeGreaterThan(0);
      expect(
        judged.filter((p) => p !== FOCUS_OK),
        "every focus stop in the page content is inside the viewport and not covered",
      ).toEqual([]);
    });

    // ===== UX-34: no h-scroll, truncation or small targets per surface =====

    test("each_surface_should_have_no_hscroll_truncation_and_44px_targets_When_360x800_and_1280x800", async ({ page }) => {
      const mocks = await installMocks(page);
      const stages = new BacklogStagesSettingsPage(page);
      const features = new FeaturesSettingsPage(page);
      const quota = new BacklogQuotaSettingsPage(page);
      const stuck = new StuckItemsPage(page);
      const small: string[] = [];
      const measure = async (name: string, locator: ReturnType<Page["getByTestId"]>) => {
        const box = await locator.boundingBox();
        if (isMobile && (!box || box.height < MIN_TARGET_PX)) small.push(`${name}: ${box?.height ?? "missing"}px high`);
      };
      const clipped = (locator: ReturnType<Page["getByTestId"]>) =>
        locator.evaluate((el) => el.scrollWidth > el.clientWidth + 1);

      mocks.status.use(connectError("unavailable", "down"));
      await stages.goto();
      await expect(stages.bannerRetry).toBeVisible();
      expect(await noHorizontalScroll(page), "stages page").toBe(true);
      expect(await clipped(stages.bannerText), "stages banner text").toBe(false);
      await measure("stages Retry", stages.bannerRetry);
      await measure("stages flags link", stages.bannerFlagsLink);

      mocks.status.use(statusFollowing(mocks.store));
      mocks.store.set("backlog:workflow-engine-enforce", true);
      await features.goto();
      await expect(features.revert("operator")).toBeVisible();
      expect(await noHorizontalScroll(page), "features page").toBe(true);
      await measure("features switch", features.switchFor("operator"));
      await measure("features Revert to Off", features.revert("operator"));
      await measure("features Edit stages", features.editStagesLink("operator"));
      expect(await clipped(features.summary), "features summary").toBe(false);

      await quota.goto();
      await expect(quota.form).toBeVisible();
      expect(await noHorizontalScroll(page), "quota page").toBe(true);
      for (const key of ["budget", "pause", "resume", "usd"] as const) await measure(`quota ${key}`, quota.field(key));
      await measure("quota Save", quota.save);
      expect(await clipped(quota.effective), "quota effective line").toBe(false);

      mocks.stuck.use(stuckReport({ lookup: "RATE_LIMITED", checkedAt: new Date().toISOString() }));
      await stuck.goto();
      await stuck.openDetail(STUCK_ITEM_TITLE);
      expect(await noHorizontalScroll(page), "stuck page").toBe(true);
      await measure("stuck Retry", stuck.autoMergeRetry());
      expect(await clipped(stuck.autoMergeLine()), "stuck auto-merge line").toBe(false);

      expect(small).toEqual([]);
    });

    // ===== UX-41 / UX-54: phone-only layout details =====

    test("mobile_360_should_have_enterkeyhint_scroll_into_view_mode_reminder_and_cancel_first_dialog", async ({ page }) => {
      test.skip(!isMobile, "phone layout only");
      await installMocks(page);
      const quota = new BacklogQuotaSettingsPage(page);
      const features = new FeaturesSettingsPage(page);
      const stages = new BacklogStagesSettingsPage(page);

      await quota.goto();
      await expect(quota.form).toBeVisible();
      for (const key of ["budget", "pause", "resume"] as const) await expect(quota.field(key)).toHaveAttribute("enterkeyhint", "next");
      await expect(quota.field("usd")).toHaveAttribute("enterkeyhint", "done");
      await addKeyboardScrollRoom(page, KEYBOARD_PX);
      await setKeyboardOpen(page, KEYBOARD_PX);
      await quota.field("usd").focus();
      await expect
        .poll(() => quota.field("usd").evaluate((el, keyboard) => el.getBoundingClientRect().bottom <= window.innerHeight - keyboard, KEYBOARD_PX))
        .toBe(true);

      await stages.goto();
      await stages.openStageForm("idea");
      await expect(stages.formReminder).toHaveText("Mode: Shadow. Saved edits are not enforced yet.");

      await features.goto();
      await features.openEnforceDialog("operator");
      await expect(features.evidence).not.toHaveAttribute("aria-busy", "true");
      const order = await page.evaluate(() => {
        const cancel = document.querySelector('[data-testid="alert-confirm-cancel"]')!;
        const confirm = document.querySelector('[data-testid="alert-confirm-confirm"]')!;
        const a = cancel.getBoundingClientRect();
        const b = confirm.getBoundingClientRect();
        return {
          cancelFirstInDom: Boolean(cancel.compareDocumentPosition(confirm) & Node.DOCUMENT_POSITION_FOLLOWING),
          stacked: b.top >= a.bottom - 1,
          gap: b.top >= a.bottom - 1 ? b.top - a.bottom : b.left - a.right,
          cancelAboveOrLeft: a.top < b.top || a.left < b.left,
          heights: [a.height, b.height],
        };
      });
      expect(order.cancelFirstInDom).toBe(true);
      expect(order.cancelAboveOrLeft).toBe(true);
      expect(order.gap).toBeGreaterThanOrEqual(MIN_DIALOG_GAP_PX);
      for (const h of order.heights) expect(h).toBeGreaterThanOrEqual(MIN_TARGET_PX);
    });

    test("ack_label_and_quota_rows_should_have_44px_hit_areas_at_360", async ({ page }) => {
      test.skip(!isMobile, "phone layout only");
      await installMocks(page);
      const features = new FeaturesSettingsPage(page);
      const quota = new BacklogQuotaSettingsPage(page);

      await features.goto();
      await features.openEnforceDialog("operator");
      await expect(features.ack).toBeVisible();
      const label = await features.ack.evaluate((el) => {
        const row = el.closest("label")!.getBoundingClientRect();
        const region = document.querySelector('[data-testid="feature-flag-enforce-evidence"]')!.getBoundingClientRect();
        return { height: row.height, width: row.width, regionWidth: region.width };
      });
      expect(label.height).toBeGreaterThanOrEqual(MIN_TARGET_PX);
      expect(label.width, "the label spans the dialog content width").toBeGreaterThanOrEqual(label.regionWidth - 2);
      await features.ack.locator("xpath=ancestor::label").click(); // the label's own hit area, not the input
      await expect(features.ack).toBeChecked();

      await quota.goto();
      await expect(quota.form).toBeVisible();
      const heights = await page.evaluate(() => {
        const h = (el: Element | null | undefined) => el?.getBoundingClientRect().height ?? 0;
        return {
          gateRow: h(document.querySelector('[data-testid="backlog-quota-enabled"]')?.closest("label")),
          inputs: ["budget", "pause", "resume", "usd"].map((k) => h(document.querySelector(`[data-testid="backlog-quota-${k}"]`))),
          save: h(document.querySelector('[data-testid="backlog-quota-save"]')),
          revert: h(document.querySelector('[data-testid="backlog-quota-revert"]')),
        };
      });
      expect(heights.gateRow).toBeGreaterThanOrEqual(MIN_TARGET_PX);
      for (const input of heights.inputs) expect(input).toBeGreaterThanOrEqual(MIN_TARGET_PX);
      expect(heights.save).toBeGreaterThanOrEqual(MIN_TARGET_PX);
      expect(heights.revert).toBeGreaterThanOrEqual(MIN_TARGET_PX);
      await quota.gate.uncheck();
      await quota.gate.locator("xpath=ancestor::label").click(); // clicking the label toggles the switch
      await expect(quota.gate).toBeChecked();
    });
  });
}
