// @feature review-queue-severity-sort-filter, approval-analytics-risk-breakdown
/**
 * E2E coverage for review-queue-severity (plan.md Task 8.2.1): severity badges render on
 * the review queue, the default sort is severity-first, and the severity filter narrows
 * the list — the three top-level UX guarantees from design/ux.md and validation.md.
 *
 * Following the same "intercept and fulfill a fabricated response" precedent
 * approval-ci-block.spec.ts documents (GetReviewQueue's live server-side ordering/enrichment
 * depends on real approval/classifier state that's slow and flaky to reproduce end-to-end,
 * and is already covered by Go tests in server/services and ReviewQueuePanel.test.tsx's
 * "ReviewQueuePanel — severity" describe block): this spec mocks
 * session.v1.SessionService/GetReviewQueue with a fabricated ReviewQueue payload and drives
 * the real ReviewQueuePanel UI against it.
 */

import { test, expect, Page } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

type FixtureItem = {
  sessionId: string;
  sessionName: string;
  riskLevel?: string; // omit for "not recorded"
};

function makeReviewQueueResponse(items: FixtureItem[]) {
  return {
    reviewQueue: {
      totalItems: items.length,
      items: items.map((it) => ({
        sessionId: it.sessionId,
        sessionName: it.sessionName,
        reason: "ATTENTION_REASON_APPROVAL_PENDING",
        priority: "PRIORITY_HIGH",
        detectedAt: new Date().toISOString(),
        context: "Claude Code file permission prompt",
        program: "claude",
        branch: "main",
        path: "/tmp/e2e-repo",
        tags: [],
        category: "",
        metadata: {
          pending_approval_id: `appr-${it.sessionId}`,
          tool_name: "Bash",
          tool_input_command: "rm -rf /tmp/build",
          ...(it.riskLevel !== undefined ? { risk_level: it.riskLevel } : {}),
        },
      })),
      byPriority: { "2": items.length },
      byReason: { "1": items.length },
      averageAgeSeconds: "0",
      oldestItemId: items[0]?.sessionId ?? "",
      oldestAgeSeconds: "0",
    },
  };
}

async function mockReviewQueue(page: Page, items: FixtureItem[]) {
  await page.route("**/api/session.v1.SessionService/GetReviewQueue", async (route) => {
    await route.fulfill({ json: makeReviewQueueResponse(items) });
  });
  // WatchReviewQueue is a WS-upgraded stream for push updates; the panel falls back to the
  // GetReviewQueue poll above when it errors/is unavailable, so aborting it here is enough —
  // no separate WS fixture is needed for these read-only scenarios.
  await page.route("**/api/session.v1.SessionService/WatchReviewQueue", (route) => route.abort());
}

async function openReviewQueue(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(`${BASE_URL}/review-queue`, { waitUntil: "domcontentloaded" });
  await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000, state: "attached" });
}

test.describe("review-queue-severity", () => {
  test("should identify highest-severity item as the first row with no sort interaction", async ({ page }) => {
    await mockReviewQueue(page, [
      { sessionId: "s-low", sessionName: "Low Item", riskLevel: "low" },
      { sessionId: "s-critical", sessionName: "Critical Item", riskLevel: "critical" },
      { sessionId: "s-medium", sessionName: "Medium Item", riskLevel: "medium" },
    ]);
    await openReviewQueue(page);

    const firstRow = page.locator('[data-testid^="review-item-"]').first();
    await expect(firstRow).toHaveAttribute("data-testid", "review-item-s-critical");
    await expect(firstRow.getByTestId("severity-badge-critical")).toBeVisible();
  });

  test("should render unrecorded-severity item between Critical and Medium in default sort", async ({ page }) => {
    await mockReviewQueue(page, [
      { sessionId: "s-medium", sessionName: "Medium Item", riskLevel: "medium" },
      { sessionId: "s-unrecorded", sessionName: "Unrecorded Item" },
      { sessionId: "s-critical", sessionName: "Critical Item", riskLevel: "critical" },
    ]);
    await openReviewQueue(page);

    const ids = await page.locator('[data-testid^="review-item-"]').evaluateAll((els) =>
      els.map((el) => el.getAttribute("data-testid"))
    );
    expect(ids).toEqual(["review-item-s-critical", "review-item-s-unrecorded", "review-item-s-medium"]);
  });

  test("should narrow to Critical items within two clicks", async ({ page }) => {
    await mockReviewQueue(page, [
      { sessionId: "s-critical", sessionName: "Critical Item", riskLevel: "critical" },
      { sessionId: "s-low", sessionName: "Low Item", riskLevel: "low" },
    ]);
    await openReviewQueue(page);

    await page.getByRole("button", { name: /^Filter/ }).click(); // click 1
    const criticalChip = page.getByRole("button", { name: /Critical \(1\)/ });
    await criticalChip.click(); // click 2

    await expect(criticalChip).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByTestId("review-item-s-critical")).toBeVisible();
    await expect(page.getByTestId("review-item-s-low")).not.toBeAttached();
  });

  test("should restore the unfiltered queue with a single Clear click", async ({ page }) => {
    await mockReviewQueue(page, [
      { sessionId: "s-critical", sessionName: "Critical Item", riskLevel: "critical" },
      { sessionId: "s-low", sessionName: "Low Item", riskLevel: "low" },
    ]);
    await openReviewQueue(page);

    await page.getByRole("button", { name: /^Filter/ }).click();
    await page.getByRole("button", { name: /Critical \(1\)/ }).click();
    await expect(page.getByTestId("review-item-s-low")).not.toBeAttached();

    await page.getByRole("button", { name: /clear active filter/i }).click();

    await expect(page.getByTestId("review-item-s-critical")).toBeVisible();
    await expect(page.getByTestId("review-item-s-low")).toBeVisible();
  });
});
