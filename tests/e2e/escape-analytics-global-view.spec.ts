// @feature analytics:get-escape-global-summary, escape-analytics-global-view
/**
 * E2E coverage for the "All Sessions" tab of the Escape Analytics page
 * (project_plans/escape-analytics-global-view, Epic 3 / Story 3.1).
 *
 * GetEscapeAnalyticsGlobalSummary's response shape (fleet-wide histogram + a
 * per-session breakdown) is cheapest to exercise by mocking the RPC directly
 * rather than seeding real EscapeEvent rows across multiple sessions — there is
 * no existing fixture-seeding helper for escape-sequence events in this repo.
 * This follows the same documented precedent as ci-status-badge.spec.ts's
 * mockCIStatus and vcs-widget.spec.ts's mockShipStatus (intercept a real RPC to
 * inject server-computed data), exercising the real frontend rendering,
 * sorting, and tab-switching logic end to end.
 *
 * Scope note (plan.md Story 3.1): the time-range filter is listed as
 * "if a UI control exists — else RPC-level only." EscapeAnalyticsPage.tsx (as
 * shipped) calls useEscapeAnalyticsGlobalSummary with no filter arguments and
 * renders no From/To/Clear controls, so the time-range-narrowing and
 * validation-message behaviors have no UI surface to drive in this build.
 * Those scenarios are intentionally omitted here rather than testing against
 * UI that doesn't exist — see the PR description for the tracked gap.
 */

import { test, expect, Page } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { EscapeAnalyticsGlobalViewPage } from "./pages/EscapeAnalyticsGlobalViewPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

interface SessionEscapeSummaryFixture {
  sessionId: string;
  totalSequences: number;
  totalMangled: number;
  mangleRate: number;
}

interface GlobalSummaryFixture {
  histogram?: Array<{ sequenceType: string; count: number; mangledCount: number }>;
  totalSequences: number;
  totalMangled: number;
  mangleRate: number;
  perSession: SessionEscapeSummaryFixture[];
}

/** Builds a GetEscapeAnalyticsGlobalSummaryResponse JSON body (Connect wire format: int64 as strings). */
function buildGlobalSummaryResponse(fixture: GlobalSummaryFixture) {
  return {
    histogram: (fixture.histogram ?? []).map((h) => ({
      sequenceType: h.sequenceType,
      count: String(h.count),
      mangledCount: String(h.mangledCount),
    })),
    totalSequences: String(fixture.totalSequences),
    totalMangled: String(fixture.totalMangled),
    mangleRate: fixture.mangleRate,
    perSession: fixture.perSession.map((row) => ({
      sessionId: row.sessionId,
      totalSequences: String(row.totalSequences),
      totalMangled: String(row.totalMangled),
      mangleRate: row.mangleRate,
    })),
  };
}

/** Intercepts GetEscapeAnalyticsGlobalSummary and fulfills the given fixture. */
async function mockGlobalSummary(page: Page, fixture: GlobalSummaryFixture) {
  await page.route(
    "**/api/session.v1.SessionService/GetEscapeAnalyticsGlobalSummary",
    async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(buildGlobalSummaryResponse(fixture)),
      });
    }
  );
}

/** Intercepts GetEscapeAnalyticsGlobalSummary and fulfills a Connect-protocol error envelope. */
async function mockGlobalSummaryError(page: Page, message: string) {
  await page.route(
    "**/api/session.v1.SessionService/GetEscapeAnalyticsGlobalSummary",
    async (route) => {
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ code: "internal", message }),
      });
    }
  );
}

/** Counts how many times GetEscapeAnalyticsSummary (the per-session RPC) has been requested. */
function trackPerSessionSummaryCalls(page: Page): { count: number } {
  const tracker = { count: 0 };
  page.on("request", (request) => {
    if (request.url().includes("/api/session.v1.SessionService/GetEscapeAnalyticsSummary")) {
      tracker.count += 1;
    }
  });
  return tracker;
}

async function gotoEscapeAnalytics(page: Page): Promise<EscapeAnalyticsGlobalViewPage> {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  const pageObject = new EscapeAnalyticsGlobalViewPage(page);
  await pageObject.goto(BASE_URL);
  await expect(pageObject.getTab("per_session")).toBeVisible({ timeout: 10000 });
  return pageObject;
}

const THREE_ROW_FIXTURE: GlobalSummaryFixture = {
  totalSequences: 600,
  totalMangled: 60,
  mangleRate: 0.1,
  perSession: [
    { sessionId: "session-alpha", totalSequences: 100, totalMangled: 5, mangleRate: 0.05 },
    { sessionId: "session-bravo", totalSequences: 200, totalMangled: 40, mangleRate: 0.2 },
    { sessionId: "session-charlie", totalSequences: 300, totalMangled: 15, mangleRate: 0.05 },
  ],
};

test.describe("escape-analytics-global-view", () => {
  test("EscapeAnalyticsGlobalView_should_ActivateAllSessionsTab_When_ClickedOnce", async ({
    page,
  }) => {
    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");

    await analytics.expectTabActive("all_sessions");
    await expect(analytics.getPanel("all_sessions")).toBeVisible();
    await expect(analytics.getBreakdownTable()).toBeVisible();
  });

  test("EscapeAnalyticsGlobalView_should_HideTable_When_EmptyStateActive_ForPerSessionView", async ({
    page,
  }) => {
    // Per-session panel is hidden entirely (not rendered) while "All Sessions" is active.
    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");

    await expect(analytics.getPanel("per_session")).toHaveCount(0);
  });

  test("EscapeAnalyticsGlobalView_should_WireAriaControlsAndLabelledby_When_TabsRendered", async ({
    page,
  }) => {
    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);

    await expect(analytics.getTab("per_session")).toHaveAttribute(
      "aria-controls",
      "tabpanel-per_session"
    );
    await expect(analytics.getTab("all_sessions")).toHaveAttribute(
      "aria-controls",
      "tabpanel-all_sessions"
    );

    await analytics.activateTab("all_sessions");

    await expect(analytics.getPanel("all_sessions")).toHaveAttribute(
      "aria-labelledby",
      "tab-all_sessions"
    );
  });

  test("EscapeAnalyticsGlobalView_should_ActivateAdjacentTab_When_ArrowKeyPressed", async ({
    page,
  }) => {
    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.getTab("per_session").focus();
    await analytics.pressArrowKey("ArrowRight");

    await analytics.expectTabActive("all_sessions");
    await expect(analytics.getTab("all_sessions")).toBeFocused();
  });

  test("EscapeAnalyticsGlobalView_should_NotRequestPerSessionSummary_When_AllSessionsTabActive", async ({
    page,
  }) => {
    const client = new SessionClient(BASE_URL);
    const ts = Date.now();
    const session = await client.createSession({
      title: `e2e-escape-global-${ts}`,
      path: "/tmp",
      program: "bash",
    });

    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);
    const tracker = trackPerSessionSummaryCalls(page);

    // Select a real session on the per-session tab to trigger at least one
    // GetEscapeAnalyticsSummary call, establishing a non-zero baseline.
    await page.getByLabel("Select session for escape analytics").selectOption(session.id);
    await expect
      .poll(() => tracker.count, { timeout: 10000 })
      .toBeGreaterThan(0);
    const baseline = tracker.count;

    await analytics.activateTab("all_sessions");
    await expect(analytics.getBreakdownTable()).toBeVisible();

    // No further per-session summary requests should fire while on the
    // all-sessions tab.
    expect(tracker.count).toBe(baseline);
  });

  test("EscapeAnalyticsGlobalView_should_PreserveSelection_When_ReturningToPerSessionTab", async ({
    page,
  }) => {
    const client = new SessionClient(BASE_URL);
    const ts = Date.now();
    const session = await client.createSession({
      title: `e2e-escape-global-preserve-${ts}`,
      path: "/tmp",
      program: "bash",
    });

    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);

    const selector = page.getByLabel("Select session for escape analytics");
    await selector.selectOption(session.id);
    await expect(selector).toHaveValue(session.id);

    await analytics.activateTab("all_sessions");
    await analytics.activateTab("per_session");

    await expect(selector).toHaveValue(session.id);
  });

  test("EscapeAnalyticsGlobalView_should_ShowAggregateSummary_When_TabSwitched", async ({
    page,
  }) => {
    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");

    await expect(analytics.getPanel("all_sessions").getByTestId("mangle-rate-value")).toBeVisible();
    await expect(analytics.getPanel("all_sessions").getByTestId("mangle-counts")).toContainText(
      "60"
    );
    await expect(analytics.getPanel("all_sessions").getByTestId("mangle-counts")).toContainText(
      "600"
    );
  });

  test("EscapeAnalyticsGlobalView_should_ShowTopContributor_When_OneSessionExceedsThreshold", async ({
    page,
  }) => {
    const dominantFixture: GlobalSummaryFixture = {
      totalSequences: 1000,
      totalMangled: 100,
      mangleRate: 0.1,
      perSession: [
        { sessionId: "session-dominant", totalSequences: 800, totalMangled: 80, mangleRate: 0.1 },
        { sessionId: "session-minor", totalSequences: 200, totalMangled: 20, mangleRate: 0.1 },
      ],
    };
    await mockGlobalSummary(page, dominantFixture);
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");

    await expect(analytics.getPanel("all_sessions")).toContainText(
      "session-dominant accounts for the majority of mangled sequences fleet-wide."
    );
  });

  test("EscapeAnalyticsGlobalView_should_ShowErrorBanner_When_RpcFails", async ({ page }) => {
    await mockGlobalSummaryError(page, "database unavailable");
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");

    await expect(analytics.getErrorBanner()).toBeVisible();
    await expect(analytics.getErrorBanner()).toContainText("Failed to load global summary:");
    await expect(analytics.getBreakdownTable()).toHaveCount(0);
  });

  test("EscapeAnalyticsGlobalView_should_ShowDistinctEmptyCopy_When_GlobalEmpty", async ({
    page,
  }) => {
    // Only the "no events at all" empty copy is reachable in this build — the
    // "filtered empty" variant depends on the time-range filter UI, which does
    // not exist yet (see file-level doc comment).
    await mockGlobalSummary(page, { totalSequences: 0, totalMangled: 0, mangleRate: 0, perSession: [] });
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");

    await expect(analytics.getPanel("all_sessions")).toContainText(
      "No escape sequence events recorded across any session yet."
    );
    await expect(analytics.getBreakdownTable()).toHaveCount(0);
  });

  test("EscapeAnalyticsGlobalView_should_DefaultSortByMangleRateDescending_When_TableFirstRendered", async ({
    page,
  }) => {
    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");
    await expect(analytics.getBreakdownTable()).toBeVisible();

    await expect(analytics.getSortButton("mangleRate")).toHaveAttribute(
      "data-testid",
      "sort-button-mangleRate"
    );
    const header = page.locator('th[aria-sort]').filter({ has: analytics.getSortButton("mangleRate") });
    await expect(header).toHaveAttribute("aria-sort", "descending");

    // session-bravo has the highest mangle rate (0.2) and must render first.
    const ids = await analytics.getRowSessionIds();
    expect(ids[0]).toBe("session-bravo");
  });

  test("EscapeAnalyticsGlobalView_should_ResortColumn_When_HeaderButtonActivated", async ({
    page,
  }) => {
    await mockGlobalSummary(page, THREE_ROW_FIXTURE);
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");
    await expect(analytics.getBreakdownTable()).toBeVisible();

    await analytics.sortBy("sessionId");

    const header = page.locator('th[aria-sort]').filter({ has: analytics.getSortButton("sessionId") });
    await expect(header).toHaveAttribute("aria-sort", "descending");

    // Clicking a new column defaults to descending — session-charlie sorts last
    // alphabetically, so it renders first in descending order.
    const ids = await analytics.getRowSessionIds();
    expect(ids[0]).toBe("session-charlie");
  });

  test("EscapeAnalyticsGlobalView_should_HideTable_When_EmptyStateActive", async ({ page }) => {
    // Fleet-wide totals are non-zero (so the "no events at all" branch does not
    // fire) but the per-session breakdown itself is empty.
    await mockGlobalSummary(page, {
      totalSequences: 50,
      totalMangled: 5,
      mangleRate: 0.1,
      perSession: [],
    });
    const analytics = await gotoEscapeAnalytics(page);

    await analytics.activateTab("all_sessions");

    await expect(analytics.getBreakdownTable()).toHaveCount(0);
    await expect(analytics.getPanel("all_sessions")).toContainText(
      "No per-session breakdown available."
    );
  });
});
