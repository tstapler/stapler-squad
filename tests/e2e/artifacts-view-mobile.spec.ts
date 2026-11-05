// @feature session:artifacts
/**
 * The /artifacts page at 390px on a touch device (T10.1.3, UX-S8-4, UX-S8-8): one column of cards, filters behind a
 * button that opens a focus-trapped sheet, 44px targets, no sideways scroll, nothing that needs hover, light and
 * dark Axe, and the route reached from the mobile menu with two real taps. Runs on `mobile-chromium` only
 * (playwright.config.ts MOBILE_ONLY_SPECS). Browse is answered by a route mock (pages/ArtifactsPage.ts).
 */
import { test, expect } from "@playwright/test";
import { baseUrl } from "./helpers/base-url";
import { assertReflow, axeLightAndDark, mobilePreconditionFacts, touchTargetProblems } from "./helpers/a11y";
import { collectConsoleErrors, skipOnboarding } from "./helpers/multi-pr";
import { enableBacklogFeatureFlag } from "./pages/StuckItemsPage";
import { ArtifactsPage, rowJson, type MockRow } from "./pages/ArtifactsPage";

const BASE_URL = baseUrl();
let consoleErrors: string[];
let artifacts: ArtifactsPage;

const ROWS: MockRow[] = [
  { sessionId: "s1", sessionTitle: "refactor-auth-with-a-long-title-that-must-wrap-instead-of-pushing-the-page-sideways", kind: "pr", value: 12, state: "open", source: "session_pr", at: "2026-11-05T10:00:00Z" },
  { sessionId: "s1", sessionTitle: "refactor-auth", kind: "commit", value: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", at: "2026-11-05T09:00:00Z" },
  { sessionId: "s2", sessionTitle: "other-repo-work", kind: "branch", value: "feat/a-branch-name-long-enough-to-need-wrapping-on-a-phone", at: "2026-11-03T10:00:00Z" },
];

test.describe("artifacts-view-mobile", () => {
  test.beforeEach(async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    consoleErrors = collectConsoleErrors(page);
    await skipOnboarding(page);
    artifacts = new ArtifactsPage(page);
    await artifacts.mockBrowse(() => ({ rows: ROWS.map(rowJson), totalCount: ROWS.length, unscannedSessionCount: 0 }));
  });

  test.afterEach(() => {
    expect(consoleErrors, "no console or page errors").toEqual([]);
  });

  test("precondition_should_beACoarsePointerTouchDevice", async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    const facts = await mobilePreconditionFacts(page);
    expect(facts.coarse).toBe(true);
    expect(facts.touchPoints).toBeGreaterThan(0);
    expect(facts.ua).toMatch(/Mobile|Android/);
  });

  test("page should be single column with filters behind a button and no sideways scroll", async ({ page }) => {
    await artifacts.goto();
    await expect(artifacts.rows).toHaveCount(3);
    await expect(page.getByTestId("artifacts-cards")).toBeVisible();
    await expect(page.getByTestId("artifacts-table")).toHaveCount(0);
    await expect(page.getByTestId("artifacts-filter-bar")).toHaveCount(0);

    const lefts = await Promise.all((await artifacts.rows.all()).map(async (r) => (await r.boundingBox())?.x ?? -1));
    expect(new Set(lefts.map((x) => Math.round(x))).size, "all cards share one left edge").toBe(1);
    expect(await assertReflow(page)).toEqual([]);
    expect(await page.evaluate(() => document.scrollingElement!.scrollWidth <= window.innerWidth)).toBe(true);
  });

  test("filters button should open a focus-trapped sheet and return focus on Done", async ({ page }) => {
    await artifacts.goto("?type=pr");
    await expect(artifacts.filtersButton).toHaveText("Filters (1)");
    await expect(artifacts.filtersButton).toHaveAttribute("aria-expanded", "false");
    await artifacts.filtersButton.tap();
    await expect(artifacts.filterSheet).toBeVisible();
    await expect(artifacts.filterSheet).toHaveAttribute("role", "dialog");
    expect(await touchTargetProblems(artifacts.filterType, artifacts.filterRepo, artifacts.filterState, page.getByTestId("artifacts-filter-clear"), page.getByTestId("artifacts-filter-done"))).toEqual([]);

    await artifacts.filterState.selectOption("open");
    await expect(page).toHaveURL(/state=open/);
    await page.getByTestId("artifacts-filter-clear").tap();
    await expect(page).not.toHaveURL(/type=|state=/);
    await page.getByTestId("artifacts-filter-done").tap();
    await expect(artifacts.filterSheet).toHaveCount(0);
    await expect(artifacts.filtersButton).toBeFocused();
    await expect(artifacts.filtersButton).toHaveText("Filters");
  });

  test("targets should be at least 44px and nothing should depend on hover", async ({ page }) => {
    await artifacts.goto();
    await expect(artifacts.rows).toHaveCount(3);
    const targets = [artifacts.search, artifacts.searchSubmit, artifacts.filtersButton];
    for (const row of await artifacts.rows.all()) {
      targets.push(artifacts.sessionLink(row));
      const chip = row.getByTestId("pr-chip");
      if (await chip.count()) targets.push(chip);
    }
    expect(await touchTargetProblems(...targets)).toEqual([]);
    // Everything a hover could reveal on desktop is already in the card: the kind, the artifact, the session and the age.
    await expect(artifacts.rows.first()).toContainText("Tracked");
    await expect(artifacts.rows.first()).toContainText("ago");
  });

  test("artifacts page should be reachable from the mobile menu in two taps", async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "More navigation options" }).tap();
    await page.getByRole("navigation", { name: "More navigation" }).getByRole("link", { name: "Artifacts" }).tap();
    await expect(page).toHaveURL(/\/artifacts$/);
    await expect(artifacts.heading).toBeVisible();
    await expect(artifacts.rows).toHaveCount(3);
  });

  test("page should have no WCAG AA violations in light and dark", async ({ page }) => {
    await artifacts.goto();
    await expect(artifacts.rows).toHaveCount(3);
    const violations = await axeLightAndDark(page, "[data-testid=artifacts-page]", async () => {
      await expect(artifacts.rows).toHaveCount(3);
    });
    expect(violations).toEqual([]);
  });
});
