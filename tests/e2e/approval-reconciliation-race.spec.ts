// @feature approval:resolve
/**
 * E2E coverage for Epic 2.1's human-vs-reconciliation arbitration UX
 * (validation.md's UX Acceptance Tests table, rows 14-17, 26 partial) — what a
 * reviewer sees when their Approve/Deny click loses the race against a
 * rule-reconciliation pass that auto-resolved the same pending approval moments
 * earlier.
 *
 * The backend's real arbitration path (ApprovalStore.MarkHumanResolving /
 * ResolveApprovalReconciled's CodeAborted branch) is a goroutine-level race
 * already covered by server/services/rules_service_test.go's
 * TestReconcilePendingApprovals_HumanWinsRace (real concurrency, not a
 * simulation) — reproducing that exact interleaving against a live backend from
 * Playwright would be slow/flaky. Following approval-ci-block.spec.ts's
 * documented "intercept and fulfill a fabricated response" precedent, this spec
 * mocks GetNotificationHistory (one pending approval) and ResolveApproval
 * (rejecting with the real losing-race error shape: FailedPrecondition + the
 * exact message text useApprovalResolution.test.ts pins) — exercising the
 * reviewer-facing UI end to end, not the backend race itself.
 *
 * Deviation from validation.md's literal wording for rows 14/15 ("simulate the
 * reconciliation event landing via a mocked poll response now carrying
 * reconciled: true"): NotificationsPage.tsx has no client-side poll loop —
 * history is fetched once on mount (useNotificationHistory.ts) and only
 * re-fetched on a push event (SessionServiceContext's onApprovalResponse ->
 * refreshHistory()) delivered over the WatchSessions stream, which has no
 * established route-interception fixture pattern in this repo for injecting a
 * live push event (grepped every *.spec.ts file — every WatchSessions mock
 * either aborts the route or leaves it live against the real backend; none
 * fulfills a synthetic push). What IS directly, faithfully testable and real is
 * the moment a reviewer's own click loses that race: the buttons disable in
 * place (isPending) while the RPC is in flight, the item is never silently
 * removed from the DOM, and the response names the specific rule — exactly what
 * useApprovalResolution.ts's resolveApproval()/blockedApprovals implements.
 * Rows 14/15 below test that reachable path instead of an unmockable live-poll
 * landing.
 */
import { test, expect, Page } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const RULE_NAME = "Auto-allow safe git status checks";
const RACE_MESSAGE = `already auto-resolved by rule "${RULE_NAME}" while you were reviewing it — no action needed`;

async function mockPendingApproval(page: Page, approvalId: string, sessionId: string) {
  await page.route("**/api/session.v1.SessionService/GetNotificationHistory", async (route) => {
    await route.fulfill({
      json: {
        notifications: [
          {
            id: `n-${approvalId}`,
            sessionId,
            sessionName: sessionId,
            notificationType: "NOTIFICATION_TYPE_APPROVAL_NEEDED",
            priority: "NOTIFICATION_PRIORITY_HIGH",
            title: "Permission Required",
            message: "Bash tool wants to run a command",
            metadata: { approval_id: approvalId, tool_name: "Bash" },
            createdAt: new Date().toISOString(),
            isRead: false,
          },
        ],
        totalCount: 1,
        unreadCount: 1,
        hasMore: false,
      },
    });
  });
}

/** Rejects ResolveApproval with the real losing-race error shape (FailedPrecondition
 *  + the exact message useApprovalResolution.test.ts pins), after an optional delay
 *  so the in-flight (isPending, disabled) state is observable before it settles. */
async function mockResolveApprovalLosesRace(page: Page, delayMs = 0) {
  await page.route("**/api/session.v1.SessionService/ResolveApproval", async (route) => {
    if (delayMs) await new Promise((r) => setTimeout(r, delayMs));
    await route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({ code: "failed_precondition", message: RACE_MESSAGE }),
    });
  });
}

async function openNotificationsPage(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(`${BASE_URL}/notifications`, { waitUntil: "domcontentloaded" });
  // Scoped to the routed page's own container, not NotificationPanel's
  // always-mounted off-screen drawer (see approval-ci-block.spec.ts's identical
  // documented reason for this scoping).
  return page.getByTestId("notifications-content");
}

test.describe("approval-reconciliation-race", () => {
  test("reconciled-while-open item's action buttons become disabled, never silently removed", async ({ page }) => {
    const approvalId = `appr-race-disable-${Date.now()}`;
    const sessionId = "e2e-race-session-disable";
    await mockPendingApproval(page, approvalId, sessionId);
    await mockResolveApprovalLosesRace(page, 500);
    const content = await openNotificationsPage(page);
    const itemWrapper = content.getByTestId(`needs-decision-item-n-${approvalId}`);
    await expect(itemWrapper).toBeVisible();

    // Located by their static `title` attribute rather than accessible name --
    // the button's visible text/name flips from "✓ Approve" to "…" the instant
    // isPending becomes true (NotificationItem.tsx), so a name-based locator
    // would stop matching at exactly the moment this test needs to observe.
    const approveButton = itemWrapper.getByTitle("Approve this tool use");
    const denyButton = itemWrapper.getByTitle("Deny this tool use");
    await approveButton.click();

    // While the RPC is in flight (the 500ms delay above), both buttons disable
    // in place — the "…" pending label useApprovalResolution.ts's
    // pendingApprovals state drives — rather than the item vanishing.
    await expect(approveButton).toBeDisabled();
    await expect(denyButton).toBeDisabled();
    await expect(itemWrapper).toBeVisible();

    // Settles into the losing-race message once the RPC resolves; still present.
    await expect(itemWrapper.getByTestId("ci-block-message")).toBeVisible();
    await expect(itemWrapper).toBeVisible();
  });

  test("mid-race banner names the specific rule, never generic 'This item was resolved'", async ({ page }) => {
    const approvalId = `appr-race-banner-${Date.now()}`;
    const sessionId = "e2e-race-session-banner";
    await mockPendingApproval(page, approvalId, sessionId);
    await mockResolveApprovalLosesRace(page);
    const content = await openNotificationsPage(page);

    await content.getByRole("button", { name: "✓ Approve" }).click();

    const blockMessage = content.getByTestId("ci-block-message");
    await expect(blockMessage).toBeVisible();
    await expect(blockMessage).toContainText(RULE_NAME);
    await expect(content.getByText("This item was resolved")).toHaveCount(0);
  });

  test("human's losing click shows 'already auto-resolved by rule ... no action needed'", async ({ page }) => {
    const approvalId = `appr-race-message-${Date.now()}`;
    const sessionId = "e2e-race-session-message";
    await mockPendingApproval(page, approvalId, sessionId);
    await mockResolveApprovalLosesRace(page);
    const content = await openNotificationsPage(page);

    await content.getByRole("button", { name: "✓ Approve" }).click();

    const blockMessage = content.getByTestId("ci-block-message");
    await expect(blockMessage).toContainText(RACE_MESSAGE);
    await expect(content.getByText("Expired")).toHaveCount(0);
    await expect(content.getByText("failed_precondition")).toHaveCount(0);
  });

  test("reconciliation-race message offers only Deny, never Approve anyway", async ({ page }) => {
    const approvalId = `appr-race-denyonly-${Date.now()}`;
    const sessionId = "e2e-race-session-denyonly";
    await mockPendingApproval(page, approvalId, sessionId);
    await mockResolveApprovalLosesRace(page);
    const content = await openNotificationsPage(page);

    await content.getByRole("button", { name: "✓ Approve" }).click();
    await expect(content.getByTestId("ci-block-message")).toBeVisible();

    // No CI-checks URL in a reconciliation-race message — splitCIBlockMessage
    // returns no checksUrl, so "Approve anyway" (gated on checksUrl) never renders.
    await expect(content.getByRole("button", { name: "Approve anyway" })).toHaveCount(0);
    await expect(content.getByRole("button", { name: "✗ Deny" })).toBeVisible();
  });
});
