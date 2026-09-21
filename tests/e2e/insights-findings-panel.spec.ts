// @feature insights-findings-panel
// Tests for the Findings panel (project_plans/insights-cost-intelligence,
// design/ux.md B1) on the /insights dashboard: the four rendered states
// (loading / computed-empty / unpriced / error), severity conveyed as text,
// keyboard operability of the primary action, and the Retry affordance.
// GetInsightsSummary is mocked (see InsightsPage.mockGetInsightsSummary) —
// see that file's header comment for why real JSONL fixtures aren't used.

import { test, expect } from "@playwright/test";
import { InsightsPage, buildFinding, buildSession } from "./pages/InsightsPage";

test.describe("insights-findings-panel", () => {
  test("insights-findings-panel_should_navigateToSession_when_viewSessionActionActivatedAfterPageLoad", async ({
    page,
  }) => {
    const insights = new InsightsPage(page);
    const finding = buildFinding({ sessionId: "session-nav", conversationId: "conv-nav" });
    const session = buildSession({ sessionId: "session-nav", conversationId: "conv-nav" });

    await insights.mockGetInsightsSummary([{ body: { findings: [finding], sessions: [session] } }]);
    await insights.goto();

    // Top finding card is visible with no additional click.
    await expect(insights.getFindingCards().first()).toBeVisible();

    await insights.getFindingViewSessionLink().click();

    // next.config.ts's trailingSlash: true rewrites the static-export route to
    // a trailing "/" before the query string — allow it optionally.
    await expect(page).toHaveURL(/\/insights\/session-detail\/?\?sessionId=session-nav/);
  });

  test("insights-findings-panel_should_renderDistinctText_when_loadingVsEmptyVsErrorStates", async ({ page }) => {
    const insights = new InsightsPage(page);

    // (a) loading — a slow response leaves the panel on its skeleton long
    // enough to assert against (no waitForTimeout: the assertion itself polls).
    await insights.mockGetInsightsSummary([{ body: {}, delayMs: 3000 }]);
    await insights.goto();
    await expect(insights.getFindingsSkeleton()).toBeVisible();
    const loadingText = await insights.getFindingsPanel().innerText();

    // (b) computed-empty — findings: [], no unpriced sessions.
    await insights.mockGetInsightsSummary([{ body: { findings: [], sessions: [buildSession()] } }]);
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(insights.getFindingsPanel()).toContainText(/No waste patterns detected/i);
    const emptyText = await insights.getFindingsPanel().innerText();

    // (c) error.
    await insights.mockGetInsightsSummary([{ errorMessage: "mock GetInsightsSummary failure" }]);
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(insights.getFindingsPanel()).toContainText(/Couldn.t compute findings/i);
    const errorText = await insights.getFindingsPanel().innerText();

    expect(new Set([loadingText, emptyText, errorText]).size).toBe(3);
  });

  test("insights-findings-panel_should_renderSeverityAsText_when_findingCardDisplayed", async ({ page }) => {
    const insights = new InsightsPage(page);
    const finding = buildFinding({ severity: "SEVERITY_CRITICAL" });

    await insights.mockGetInsightsSummary([{ body: { findings: [finding], sessions: [buildSession()] } }]);
    await insights.goto();

    await expect(insights.getFindingsPanel().getByText("Critical")).toBeVisible();
  });

  test("insights-findings-panel_should_triggerNavigation_when_actionFocusedAndEnterPressed", async ({ page }) => {
    const insights = new InsightsPage(page);
    const finding = buildFinding({ sessionId: "session-kbd", conversationId: "conv-kbd" });
    const session = buildSession({ sessionId: "session-kbd", conversationId: "conv-kbd" });

    await insights.mockGetInsightsSummary([{ body: { findings: [finding], sessions: [session] } }]);
    await insights.goto();

    const link = insights.getFindingViewSessionLink();
    await link.focus();
    await expect(link).toBeFocused();
    await page.keyboard.press("Enter");

    await expect(page).toHaveURL(/\/insights\/session-detail\/?\?sessionId=session-kbd/);
  });

  test("insights-findings-panel_should_refetchFindings_when_retryButtonClickedAfterError", async ({ page }) => {
    const insights = new InsightsPage(page);
    const finding = buildFinding();

    // next.config.ts's reactStrictMode + this e2e harness's dev-mode build
    // (Makefile's web-app/out target, NEXT_BUILD_MODE=development) double-
    // invoke the mount effect, firing two real requests before the Retry
    // click's one — so the error response must be queued twice, or the
    // second mount call would already consume the success response.
    await insights.mockGetInsightsSummary([
      { errorMessage: "mock GetInsightsSummary failure" },
      { errorMessage: "mock GetInsightsSummary failure" },
      { body: { findings: [finding], sessions: [buildSession()] } },
    ]);
    await insights.goto();

    await expect(insights.getFindingsPanel()).toContainText(/Couldn.t compute findings/i);

    await insights.getFindingsRetryButton().click();

    await expect(insights.getFindingCards().first()).toBeVisible();
  });
});
