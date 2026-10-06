// @feature insights-sessions-table-sort
// Tests for the Sessions table's extended sort/search (project_plans/insights-cost-intelligence,
// design/ux.md B2): aria-sort on the new columns, signed-vs-unpriced text for
// Cache ROI, sort/search composition, and locating the worst-waste-score
// session in one click. GetInsightsSummary is mocked — see
// InsightsPage.mockGetInsightsSummary's header comment.

import { test, expect } from "@playwright/test";
import { InsightsPage, buildSession } from "./pages/InsightsPage";

test.describe("insights-sessions-table-sort", () => {
  test("insights-sessions-table-sort_should_setAriaSortDescending_when_wasteScoreHeaderClickedFirstTime", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const sessions = [
      buildSession({ sessionId: "s-low", projectPath: "/repo/low-waste", wasteScore: 10 }),
      buildSession({ sessionId: "s-high", projectPath: "/repo/high-waste", wasteScore: 90 }),
    ];
    await insights.mockGetInsightsSummary([{ body: { sessions } }]);
    await insights.goto();

    await insights.getSortableColumnControl(/Waste Score/i).click();

    await expect(insights.getColumnHeader(/Waste Score/i)).toHaveAttribute("aria-sort", "descending");
  });

  test("insights-sessions-table-sort_should_renderSignedTextVsUnpricedBadge_when_negativeRoiAndUnpricedRowsBothVisible", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const sessions = [
      buildSession({ sessionId: "s-negative-roi", projectPath: "/repo/negative-roi", cacheRoiUsd: -0.42 }),
      buildSession({
        sessionId: "s-unpriced-roi",
        projectPath: "/repo/unpriced-roi",
        unpricedModels: ["unlisted-model"],
      }),
    ];
    await insights.mockGetInsightsSummary([{ body: { sessions } }]);
    await insights.goto();

    await expect(page.getByText("-$0.42")).toBeVisible();
    // exact:true excludes the row's own "/repo/unpriced-roi" path-cell text
    // (a substring match on "unpriced" would hit that too); .first() picks
    // one of the (possibly several) "unpriced" badge cells in the row.
    await expect(
      insights.getSessionRow(/unpriced-roi/i).getByText("unpriced", { exact: true }).first()
    ).toBeVisible();
  });

  test("insights-sessions-table-sort_should_keepSameSearchMatchedRows_when_columnHeaderClickedAfterTyping", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const sessions = [
      buildSession({
        sessionId: "s-report-tool",
        projectPath: "/repo/report-tool",
        wasteScore: 30,
        lastMessageAt: "2026-08-01T00:00:00Z",
      }),
      buildSession({
        sessionId: "s-reporting-api",
        projectPath: "/repo/reporting-api",
        wasteScore: 70,
        lastMessageAt: "2026-08-02T00:00:00Z",
      }),
      buildSession({
        sessionId: "s-other-service",
        // Deliberately NOT under "/repo/" like the other two fixtures: Fuse.js's
        // fuzzy threshold (0.4, SessionsTable.tsx) scores any "/repo/*" path
        // ~0.34 against the query "report" from the shared "repo" substring
        // alone, regardless of the suffix — so a same-prefix distractor can
        // never be reliably excluded here (empirically verified against the
        // real fuse.js scorer, not assumed).
        projectPath: "/svc/other-service",
        wasteScore: 50,
        lastMessageAt: "2026-08-03T00:00:00Z",
      }),
    ];
    await insights.mockGetInsightsSummary([{ body: { sessions } }]);
    await insights.goto();

    await insights.getSessionsSearchInput().fill("report");

    await expect(insights.getSessionRow(/report-tool/i)).toBeVisible();
    await expect(insights.getSessionRow(/reporting-api/i)).toBeVisible();
    await expect(insights.getSessionRow(/other-service/i)).toHaveCount(0);

    await insights.getSortableColumnControl(/Waste Score/i).click();

    // Same two matched rows stay visible (search wasn't reset by sorting) —
    // reordered so the higher waste-score row (reporting-api, 70) leads.
    await expect(insights.getSessionRow(/report-tool/i)).toBeVisible();
    await expect(insights.getSessionRow(/reporting-api/i)).toBeVisible();
    await expect(insights.getSessionRow(/other-service/i)).toHaveCount(0);

    // getSessionsTableRows() is scoped to `tbody tr` (no header row), so
    // index 0 is the first data row.
    const rows = insights.getSessionsTableRows();
    await expect(rows.nth(0)).toContainText("reporting-api");
  });

  test("insights-sessions-table-sort_should_surfaceWorstSessionFirst_when_wasteScoreHeaderClickedOnce", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const sessions = [
      buildSession({ sessionId: "s-low", projectPath: "/repo/low-waste", wasteScore: 10 }),
      buildSession({ sessionId: "s-worst", projectPath: "/repo/worst-waste", wasteScore: 95 }),
      buildSession({ sessionId: "s-mid", projectPath: "/repo/mid-waste", wasteScore: 55 }),
    ];
    await insights.mockGetInsightsSummary([{ body: { sessions } }]);
    await insights.goto();

    await insights.getSortableColumnControl(/Waste Score/i).click();

    // getSessionsTableRows() is scoped to `tbody tr` (no header row), so
    // index 0 is the first data row.
    const rows = insights.getSessionsTableRows();
    await expect(rows.nth(0)).toContainText("worst-waste");
  });
});
