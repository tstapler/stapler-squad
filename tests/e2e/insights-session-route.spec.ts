// @feature insights-session-detail-route
// Tests for the session drill-down route vs. modal (project_plans/insights-cost-intelligence,
// design/ux.md B3) and the per-tool estimated-value marker (ux.md A1).
//
// validation.md's UX Acceptance Tests table specified these against a
// `/insights/session/[sessionId]` dynamic route. That route was reworked
// after validation.md was written to `/insights/session-detail?sessionId=`
// (a query param, not a path segment) — see SessionDetailPageClient.tsx and
// session-detail/page.tsx's doc comments: this app builds with Next.js
// `output: "export"`, and a dynamic path segment has no pre-renderable
// params at build time, so a cold navigation to it rendered the root
// dashboard instead of the session page. These tests target the real query-param
// URL; every other assertion (focus management, "Back to dashboard", orphan
// href resolution, focus-trap parity) is unchanged from the table's intent.
//
// GetInsightsSummary is mocked — see InsightsPage.mockGetInsightsSummary's
// header comment for why.

import { test, expect } from "@playwright/test";
import { InsightsPage, buildSession, buildTopTool, assertTabWrapsWithinDialog } from "./pages/InsightsPage";
import { SessionDetailPage } from "./pages/SessionDetailPage";

test.describe("insights-session-route", () => {
  test("insights-session-route_should_showBackToDashboardLink_when_sessionNotFoundOrFetchErrors", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const detail = new SessionDetailPage(page);

    // Not-found: response has no session matching the requested ID.
    await insights.mockGetInsightsSummary([{ body: { sessions: [] } }]);
    await detail.gotoInsightsSessionRoute("does-not-exist");
    await expect(detail.getInsightsSessionNotFound()).toBeVisible();
    await expect(detail.getInsightsBackToDashboardLink()).toBeVisible();

    // Fetch error: the request itself fails.
    await insights.mockGetInsightsSummary([{ errorMessage: "mock GetInsightsSummary failure" }]);
    await page.reload({ waitUntil: "domcontentloaded" });
    // getByRole("alert") alone also matches Next.js's own
    // #__next-route-announcer__ (role="alert", always present) — scope to
    // the app's own error text to avoid that ambiguity.
    await expect(page.getByRole("alert").filter({ hasText: /Couldn.t load session/i })).toBeVisible();
    await expect(detail.getInsightsBackToDashboardLink()).toBeVisible();
  });

  test("insights-session-route_should_renderSessionDetailContent_when_navigatedDirectlyWithNoPriorClientNavigation", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const detail = new SessionDetailPage(page);
    const session = buildSession({
      sessionId: "session-cold",
      conversationId: "conv-cold",
      primaryModel: "claude-opus-4",
    });

    await insights.mockGetInsightsSummary([{ body: { sessions: [session] } }]);

    // Cold direct navigation — no prior page.goto("/insights") in this test,
    // so there's no dashboard/client-router state to rely on.
    await detail.gotoInsightsSessionRoute("session-cold");

    await expect(page.getByRole("heading", { name: "Metadata" })).toBeVisible();
    await expect(page.getByText("claude-opus-4")).toBeVisible();
  });

  test("insights-session-route_should_moveFocusToCloseButtonThenRestoreToTriggerRow_when_modalOpenedAndClosed", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const detail = new SessionDetailPage(page);
    const session = buildSession({ sessionId: "session-modal", projectPath: "/repo/modal-focus" });

    await insights.mockGetInsightsSummary([{ body: { sessions: [session] } }]);
    await insights.goto();

    const row = insights.getSessionRow(/modal-focus/i);
    await row.click();

    await expect(detail.getInsightsModalCloseButton()).toBeFocused();

    await page.keyboard.press("Escape");

    await expect(row).toBeFocused();
  });

  test("insights-session-route_should_moveFocusToHeading_when_routeMounts", async ({ page }) => {
    const insights = new InsightsPage(page);
    const detail = new SessionDetailPage(page);
    const session = buildSession({ sessionId: "session-focus-heading" });

    await insights.mockGetInsightsSummary([{ body: { sessions: [session] } }]);
    await detail.gotoInsightsSessionRoute("session-focus-heading");

    await expect(detail.getInsightsHeading()).toBeFocused();
  });

  test("insights-session-route_should_resolveHrefUsingConversationId_when_orphanSessionOpenFullPageClicked", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const detail = new SessionDetailPage(page);
    const orphan = buildSession({
      sessionId: "",
      conversationId: "conv-999",
      projectPath: "/repo/orphan-project",
      isOrphan: true,
    });

    await insights.mockGetInsightsSummary([{ body: { sessions: [orphan] } }]);
    await insights.goto();

    await insights.getSessionRow(/orphan-project/i).click();

    // next.config.ts's trailingSlash: true rewrites this static-export route's
    // href to a trailing "/" before the query string.
    await expect(detail.getInsightsOpenFullPageLink()).toHaveAttribute(
      "href",
      "/insights/session-detail/?sessionId=conv-999"
    );
  });

  test("insights-session-route_should_cycleFocusBackToFirstElement_when_tabbedFromLastFocusableInModal", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const detail = new SessionDetailPage(page);
    const session = buildSession({ sessionId: "session-trap", projectPath: "/repo/trap-focus" });

    await insights.mockGetInsightsSummary([{ body: { sessions: [session] } }]);
    await insights.goto();

    await insights.getSessionRow(/trap-focus/i).click();

    const dialog = detail.getInsightsModal();
    await expect(dialog).toBeVisible();

    await assertTabWrapsWithinDialog(page, dialog);
  });

  test("insights-session-route_should_showEstimatedMarkerOnlyOnDoubleCountedTool_when_toolsBreakdownTableRenders", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const detail = new SessionDetailPage(page);
    const session = buildSession({
      sessionId: "session-tools",
      topTools: [
        buildTopTool({ toolName: "Bash", costUsd: 1.23, costMayDoubleCount: true }),
        buildTopTool({ toolName: "Read", costUsd: 0.05, costMayDoubleCount: false }),
      ],
    });

    await insights.mockGetInsightsSummary([{ body: { sessions: [session] } }]);
    await detail.gotoInsightsSessionRoute("session-tools");

    const toolsTable = detail.getInsightsToolsBreakdownTable();
    await expect(toolsTable.getByRole("row", { name: /Bash/ })).toContainText("~$1.23");
    const readRow = toolsTable.getByRole("row", { name: /Read/ });
    await expect(readRow).toBeVisible();
    await expect(readRow).not.toContainText("~$");
  });
});
