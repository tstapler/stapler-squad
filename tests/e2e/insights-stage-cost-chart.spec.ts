// @feature insights-dashboard, backlog-stage-execution-costs
// Tests for StageCostChart / the sessions-table role cross-filter
// (project_plans/backlog-stage-execution-costs, design/ux.md): initial-load
// rendering, bar/legend click and keyboard drill-down into SessionsTable,
// zero-session-role omission, the role="img" wrapper's aria-label parity
// with rendered bar values, legend keyboard reachability, the chart being a
// convenience path (not the only path) into the sessions table, and the
// bars summing to the total-cost summary card. GetInsightsSummary is mocked
// — see InsightsPage.mockGetInsightsSummary's header comment.
//
// Recharts renders each bar as an anonymous SVG shape with no
// data-testid/ARIA role of its own, so "clicking the bar" is driven through
// the legend entry instead — see InsightsPage.getStageCostLegendEntry's doc
// comment for why that's behaviorally identical (same onRoleClick handler,
// same role argument).

import { test, expect } from "@playwright/test";
import { InsightsPage, buildSession, buildRoleBreakdown, tabUntilFocused } from "./pages/InsightsPage";
import { fmtCost } from "../../web-app/src/app/insights/insightsFormatters";

/** Three sessions (one non-work, two work) + a matching role breakdown, shared by the click/keyboard drill-down tests. */
function buildWorkDrilldownFixture() {
  return {
    sessions: [
      buildSession({ sessionId: "s-triage", conversationId: "c-triage", projectPath: "/repo/triage-only", sessionRole: "triage" }),
      buildSession({ sessionId: "s-work-a", conversationId: "c-work-a", projectPath: "/repo/work-alpha", sessionRole: "work" }),
      buildSession({ sessionId: "s-work-b", conversationId: "c-work-b", projectPath: "/repo/work-beta", sessionRole: "work" }),
    ],
    roleBreakdown: [
      buildRoleBreakdown({ sessionRole: "work", estimatedCostUsd: 10 }),
      buildRoleBreakdown({ sessionRole: "triage", estimatedCostUsd: 5 }),
    ],
    totalCostUsd: 15,
  };
}

/** Asserts the sessions table shows exactly the two work sessions and neither the triage one nor a drill-down modal. */
async function assertFilteredToWork(insights: InsightsPage, page: import("@playwright/test").Page) {
  await expect(insights.getSessionRow(/work-alpha/i)).toBeVisible();
  await expect(insights.getSessionRow(/work-beta/i)).toBeVisible();
  await expect(insights.getSessionRow(/triage-only/i)).toHaveCount(0);
  await expect(insights.getSessionsTableRows()).toHaveCount(2);
  // Scoped to SessionDetailDrawer specifically (aria-label "Session details")
  // — NotificationPanel also has role="dialog" but is permanently mounted
  // (CSS-hidden, not conditionally rendered) regardless of interaction, so
  // an unscoped getByRole("dialog") would always find it.
  await expect(page.getByRole("dialog", { name: "Session details" })).toHaveCount(0);
}

/** Tabs to `role`'s legend entry, activates it with `key`, then asserts the cross-filter narrowed to that role's one seeded session. */
async function tabActivateAndAssertFiltered(
  page: import("@playwright/test").Page,
  insights: InsightsPage,
  role: string,
  key: "Enter" | "Space"
) {
  expect(await tabUntilFocused(page, insights.getStageCostLegendEntry(role))).toBe(true);
  await page.keyboard.press(key);
  await expect(insights.getRoleFilterChip()).toContainText(role);
  await expect(insights.getSessionRow(new RegExp(`${role}-only`, "i"))).toBeVisible();
  await expect(insights.getSessionsTableRows()).toHaveCount(1);
}

test.describe("insights-stage-cost-chart", () => {
  test("renders stage cost chart alongside model breakdown on initial insights page load", async ({ page }) => {
    const insights = new InsightsPage(page);
    await insights.mockGetInsightsSummary([
      {
        body: {
          totalCostUsd: 12,
          sessions: [
            buildSession({ sessionId: "s-triage", projectPath: "/repo/triage-1", sessionRole: "triage", estimatedCostUsd: 5 }),
            buildSession({ sessionId: "s-work", projectPath: "/repo/work-1", sessionRole: "work", estimatedCostUsd: 7 }),
          ],
          roleBreakdown: [
            buildRoleBreakdown({ sessionRole: "work", estimatedCostUsd: 7 }),
            buildRoleBreakdown({ sessionRole: "triage", estimatedCostUsd: 5 }),
          ],
        },
      },
    ]);

    // Single navigation, no prior interaction.
    await insights.goto();

    await expect(insights.getStageCostChart()).toBeVisible();
    await expect(page.getByText(/Cost by Model/i)).toBeVisible();
  });

  test("clicking the work bar filters the sessions table to work-role sessions in one click", async ({ page }) => {
    const insights = new InsightsPage(page);
    await insights.mockGetInsightsSummary([{ body: buildWorkDrilldownFixture() }]);
    await insights.goto();

    await insights.getStageCostLegendEntry("work").click();

    await assertFilteredToWork(insights, page);
  });

  test("activating the work legend entry via Tab and Enter filters sessions identically to a click", async ({ page }) => {
    const insights = new InsightsPage(page);
    await insights.mockGetInsightsSummary([{ body: buildWorkDrilldownFixture() }]);
    await insights.goto();

    const workEntry = insights.getStageCostLegendEntry("work");
    expect(await tabUntilFocused(page, workEntry)).toBe(true);
    await page.keyboard.press("Enter");

    await assertFilteredToWork(insights, page);
  });

  test("omits a zero-session role from the stage cost chart entirely rather than showing a zero bar", async ({ page }) => {
    const insights = new InsightsPage(page);
    await insights.mockGetInsightsSummary([
      {
        body: {
          totalCostUsd: 13,
          sessions: [
            buildSession({ sessionId: "s-triage", projectPath: "/repo/triage-only", sessionRole: "triage", estimatedCostUsd: 4 }),
            buildSession({ sessionId: "s-work", projectPath: "/repo/work-only", sessionRole: "work", estimatedCostUsd: 9 }),
          ],
          // No "review" entry at all — not merely a zero-cost/zero-session one.
          roleBreakdown: [
            buildRoleBreakdown({ sessionRole: "work", estimatedCostUsd: 9 }),
            buildRoleBreakdown({ sessionRole: "triage", estimatedCostUsd: 4 }),
          ],
        },
      },
    ]);
    await insights.goto();

    await expect(insights.getStageCostLegendEntry("work")).toBeVisible();
    await expect(insights.getStageCostLegendEntry("triage")).toBeVisible();
    await expect(insights.getStageCostLegendEntry("review")).toHaveCount(0);
    await expect(page.getByTestId(/^stage-cost-legend-/)).toHaveCount(2);
    await expect(insights.getStageCostChartImg()).not.toHaveAccessibleName(/review/i);
  });

  test("stage cost chart wrapper role=img aria-label matches the visually rendered bar values", async ({ page }) => {
    const insights = new InsightsPage(page);
    await insights.mockGetInsightsSummary([
      {
        body: {
          totalCostUsd: 19,
          sessions: [
            buildSession({ sessionId: "s-triage", projectPath: "/repo/triage-only", sessionRole: "triage" }),
            buildSession({ sessionId: "s-review", projectPath: "/repo/review-only", sessionRole: "review" }),
            buildSession({ sessionId: "s-work", projectPath: "/repo/work-only", sessionRole: "work" }),
          ],
          roleBreakdown: [
            buildRoleBreakdown({ sessionRole: "triage", estimatedCostUsd: 5 }),
            buildRoleBreakdown({ sessionRole: "review", estimatedCostUsd: 3.25 }),
            buildRoleBreakdown({ sessionRole: "work", estimatedCostUsd: 10.75 }),
          ],
        },
      },
    ]);
    await insights.goto();

    const ariaLabel = await insights.getStageCostChartImg().getAttribute("aria-label");

    // Each visible legend role/cost pair must appear in the aria-label...
    for (const { role, cost } of [
      { role: "work", cost: 10.75 },
      { role: "triage", cost: 5 },
      { role: "review", cost: 3.25 },
    ]) {
      expect(ariaLabel).toContain(`${role} ${fmtCost(cost)}`);
    }
    // ...generated from the same sorted-by-cost-descending array the bars render from.
    expect(ariaLabel).toBe(
      `Cost by stage: work ${fmtCost(10.75)}, triage ${fmtCost(5)}, review ${fmtCost(3.25)}`
    );
  });

  test("legend entries are reachable via Tab and activatable via both Enter and Space", async ({ page }) => {
    const insights = new InsightsPage(page);
    await insights.mockGetInsightsSummary([
      {
        body: {
          totalCostUsd: 17,
          sessions: [
            buildSession({ sessionId: "s-work", projectPath: "/repo/work-only", sessionRole: "work" }),
            buildSession({ sessionId: "s-triage", projectPath: "/repo/triage-only", sessionRole: "triage" }),
            buildSession({ sessionId: "s-review", projectPath: "/repo/review-only", sessionRole: "review" }),
          ],
          roleBreakdown: [
            buildRoleBreakdown({ sessionRole: "work", estimatedCostUsd: 10 }),
            buildRoleBreakdown({ sessionRole: "triage", estimatedCostUsd: 5 }),
            buildRoleBreakdown({ sessionRole: "review", estimatedCostUsd: 2 }),
          ],
        },
      },
    ]);
    await insights.goto();

    // Enter on the first entry filters exactly like a click.
    await tabActivateAndAssertFiltered(page, insights, "work", "Enter");
    // Space on a second entry filters identically.
    await tabActivateAndAssertFiltered(page, insights, "triage", "Space");
    // The third entry is reachable too (every legend entry, not just two of three).
    expect(await tabUntilFocused(page, insights.getStageCostLegendEntry("review"))).toBe(true);
  });

  test("sessions table role and text search remain fully usable without any interaction with the chart", async ({ page }) => {
    const insights = new InsightsPage(page);
    await insights.mockGetInsightsSummary([
      {
        body: {
          totalCostUsd: 6,
          sessions: [
            buildSession({ sessionId: "s-triage", projectPath: "/repo/apex-triage", sessionRole: "triage" }),
            buildSession({ sessionId: "s-work", projectPath: "/repo/apex-work", sessionRole: "work" }),
            buildSession({ sessionId: "s-other", projectPath: "/svc/unrelated-service", sessionRole: "review" }),
          ],
          roleBreakdown: [
            buildRoleBreakdown({ sessionRole: "triage", estimatedCostUsd: 1 }),
            buildRoleBreakdown({ sessionRole: "work", estimatedCostUsd: 2 }),
            buildRoleBreakdown({ sessionRole: "review", estimatedCostUsd: 3 }),
          ],
        },
      },
    ]);
    await insights.goto();

    // The chart is never touched — only SessionsTable's own search box.
    await insights.getSessionsSearchInput().fill("apex");

    await expect(insights.getSessionRow(/apex-triage/i)).toBeVisible();
    await expect(insights.getSessionRow(/apex-work/i)).toBeVisible();
    await expect(insights.getSessionRow(/unrelated-service/i)).toHaveCount(0);
    await expect(insights.getSessionsTableRows()).toHaveCount(2);
  });

  test("stage cost chart bar values sum to the total cost card for the same time range", async ({ page }) => {
    const insights = new InsightsPage(page);
    const totalCostUsd = 19; // 5 + 3.25 + 10.75
    await insights.mockGetInsightsSummary([
      {
        body: {
          totalCostUsd,
          sessions: [
            buildSession({ sessionId: "s-triage", projectPath: "/repo/triage-only", sessionRole: "triage", estimatedCostUsd: 0.4 }),
            buildSession({ sessionId: "s-review", projectPath: "/repo/review-only", sessionRole: "review", estimatedCostUsd: 0.4 }),
            buildSession({ sessionId: "s-work", projectPath: "/repo/work-only", sessionRole: "work", estimatedCostUsd: 0.4 }),
          ],
          roleBreakdown: [
            buildRoleBreakdown({ sessionRole: "triage", estimatedCostUsd: 5 }),
            buildRoleBreakdown({ sessionRole: "review", estimatedCostUsd: 3.25 }),
            buildRoleBreakdown({ sessionRole: "work", estimatedCostUsd: 10.75 }),
          ],
        },
      },
    ]);
    await insights.goto();

    // The role="img" aria-label is the authoritative textual record of each
    // bar's rendered value (AC13) — parse the dollar figures out of it
    // rather than reading pixel heights.
    const ariaLabel = (await insights.getStageCostChartImg().getAttribute("aria-label")) ?? "";
    const barSum = [...ariaLabel.matchAll(/\$([0-9.]+)/g)]
      .map((m) => Number(m[1]))
      .reduce((sum, v) => sum + v, 0);

    await expect(page.getByText(fmtCost(totalCostUsd), { exact: true })).toBeVisible();
    expect(barSum).toBeCloseTo(totalCostUsd, 2);
  });
});
