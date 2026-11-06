// @feature backlog:flow
/**
 * E2E for /backlog/flow (project_plans/backlog-flow-observability, Story 4.3.2).
 *
 * Data: the isolated test server is seeded in beforeAll through the e2e-local debug mutate endpoints
 * (seedFlowData in ./pages/BacklogFlowPage.ts): one item walked idea -> done, and one item flipped
 * ready <-> in_progress eight times, which the collapse rules count as one thrash episode.
 * Failure, stale, empty and partial states are produced with page.route over the real response.
 */
import { test, expect } from "@playwright/test";
import { BacklogFlowPage, seedFlowData, type SeededFlowData } from "./pages/BacklogFlowPage";
import { disableBacklogFeatureFlag, enableBacklogFeatureFlag } from "./pages/BacklogMutations";

let seeded: SeededFlowData;

test.beforeAll(async ({ request }) => {
  await enableBacklogFeatureFlag(request);
  seeded = await seedFlowData(request);
});

test.afterAll(async ({ request }) => {
  await disableBacklogFeatureFlag(request);
});

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
    localStorage.setItem("stapler-squad:backlog-onboarded", "true");
  });
});

test.describe("flow page happy path", () => {
  test("opens the drawer from the FPY tile and shows the singular thrash banner", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.goto("?window=30");

    await expect(flow.thrashBanner).toContainText("1 item thrashing");
    await flow.click(flow.tileButton("fpy"));

    await expect(flow.drawer).toBeVisible();
    await expect(page).toHaveURL(/drill=fpy\.first_pass/);
  });

  test("number opens drawer in one click and item link in two", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.goto("?window=30");

    await flow.click(flow.tileButton("throughput"));
    await expect(flow.drawer).toBeVisible();
    expect(flow.clicks).toBe(1);

    await flow.click(flow.drawer.getByRole("link", { name: "Flow e2e merged item" }).first());
    expect(flow.clicks).toBe(2);
    await expect(page).toHaveURL(new RegExp(`item=${seeded.mergedItemId}`));
  });

  // GAP (UX criterion 2, "detail panel in 2 clicks"): the drawer's item link is `?item=<id>` on /backlog/flow, but only
  // the List page (web-app/src/app/backlog/page.tsx) reads `item`, so no detail panel opens from the Flow route.
  test.fixme("item link opens the existing detail panel from the Flow route", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.goto(`?window=30&item=${seeded.mergedItemId}`);
    await expect(page.getByTestId("backlog-item-detail")).toBeVisible();
  });

  test("compare previous period shows a delta on the tiles in one click", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.goto("?window=30");
    await expect(flow.tile("throughput")).toBeVisible();

    await flow.click(flow.compareRadio("prev"));

    expect(flow.clicks).toBe(1);
    await expect(flow.compareRadio("prev")).toBeChecked();
    for (const metric of ["merged_items", "lead_time_p50_hours", "first_pass_yield", "review_adjusted_fail_rate"]) {
      await expect(page.getByTestId(`flow-delta-${metric}`)).toBeVisible();
    }
  });

  test("thrash banner View focuses the anomalies heading in one click", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.goto("?window=30");
    await expect(flow.thrashBanner).toBeVisible();

    await flow.click(flow.thrashBanner.getByRole("button", { name: "View" }));

    expect(flow.clicks).toBe(1);
    await expect(flow.sectionHeading("Anomalies & thrash")).toBeFocused();
    await expect(flow.sectionHeading("Anomalies & thrash")).toBeInViewport();
  });

  test("Flow tab navigates from the list in one click", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await page.goto("/backlog", { waitUntil: "domcontentloaded" });

    await flow.click(flow.viewTabs.getByRole("link", { name: "Flow" }));

    expect(flow.clicks).toBe(1);
    await expect(page).toHaveURL(/\/backlog\/flow\/?$/);
  });

  test("funnel is visible within a 1080p viewport with lower sections collapsed", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await page.setViewportSize({ width: 1920, height: 1080 });
    await flow.goto("?window=30");

    await expect(page.getByTestId("flow-funnel-stage-in_progress")).toBeInViewport();
    await expect(page.getByTestId("flow-toggle-stuck")).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByTestId("flow-toggle-anomalies")).toHaveAttribute("aria-expanded", "false");
  });

  test("jump list link opens the section and moves focus to its heading", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.goto("?window=30");

    await flow.click(flow.jumpList.getByRole("link", { name: "Review" }));

    await expect(flow.sectionHeading("Review gate")).toBeFocused();
  });
});

test.describe("flow page non-happy states", () => {
  test("a failed refresh keeps data behind a stale banner and a later success clears it", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.goto("?window=30");
    await expect(flow.tile("throughput")).toBeVisible();

    await flow.failSummary();
    await flow.click(page.getByTestId("flow-refresh"));
    await expect(flow.staleBanner).toBeVisible();
    await expect(flow.staleBanner).toContainText("refresh failed");
    await expect(flow.tile("throughput")).toBeVisible();

    await flow.unstubSummary();
    await flow.click(flow.staleBanner.getByRole("button", { name: "Retry" }));
    await expect(flow.staleBanner).toBeHidden();
  });

  test("a hard error on first load shows server text, Retry and the Board link", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.failSummary("flow aggregation exploded");
    await flow.goto("?window=30");

    await expect(flow.hardError).toContainText("flow aggregation exploded");
    await expect(flow.hardError.getByRole("button", { name: "Retry" })).toBeVisible();
    await expect(flow.hardError.getByRole("link", { name: "Back to Board" })).toHaveAttribute("href", /\/backlog\/board/);

    await flow.unstubSummary();
    await flow.click(flow.hardError.getByRole("button", { name: "Retry" }));
    await expect(flow.tile("throughput")).toBeVisible();
  });

  test("an empty 7-day window offers Widen to 30d, which sets window=30", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.stubSummary((summary) => {
      (summary.tiles as { meta: { status: string } }).meta.status = "empty";
    });
    await flow.goto("?window=7");

    await expect(page.getByText("No items finished in the last 7 days")).toBeVisible();
    await flow.unstubSummary();
    await flow.click(page.getByRole("button", { name: "Widen to 30d" }));

    await expect(page).toHaveURL(/window=30/);
    await expect(flow.tile("throughput")).toBeVisible();
  });

  test("reload restores window, compare and drill, and Back restores the prior selection", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.goto("?window=30&compare=prev&drill=rework.review_fail");

    await expect(flow.windowRadio("30")).toBeChecked();
    await expect(flow.compareRadio("prev")).toBeChecked();
    await expect(flow.drawer).toBeVisible();

    // The drawer is modal, so the window can only change once it is closed; Back then walks both steps back.
    await page.keyboard.press("Escape");
    await expect(flow.drawer).toBeHidden();
    await flow.click(flow.windowRadio("7"));
    await expect(page).toHaveURL(/window=7/);

    await page.goBack();
    await expect(page).toHaveURL(/window=30/);
    await expect(flow.windowRadio("30")).toBeChecked();
    await expect(flow.compareRadio("prev")).toBeChecked();
    await page.goBack();
    await expect(flow.drawer).toBeVisible();
    await expect(page).toHaveURL(/drill=rework\.review_fail/);
  });

  test("a failed drawer call shows 'Could not load items' with Retry and Close", async ({ page }) => {
    const flow = new BacklogFlowPage(page);
    await flow.failDrilldown();
    await flow.goto("?window=30&drill=tiles.merged");

    await expect(flow.drawer).toContainText("Could not load items");
    await expect(flow.drawer.getByRole("button", { name: "Retry" })).toBeVisible();

    await flow.unstubDrilldown();
    await flow.click(flow.drawer.getByRole("button", { name: "Retry" }));
    await expect(flow.drawer.getByRole("link", { name: "Flow e2e merged item" }).first()).toBeVisible();

    await flow.click(flow.drawer.getByRole("button", { name: "Close" }).first());
    await expect(flow.drawer).toBeHidden();
    await expect(page).not.toHaveURL(/drill=/);
  });
});
