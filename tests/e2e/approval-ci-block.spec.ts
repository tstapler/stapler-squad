// @feature approval:resolve
/**
 * E2E coverage for AC5/AC6 (Story 2.2.4) — blocking manual Approve when the session's
 * branch has failing CI, with an inline explanation, a "View CI run" link, and an
 * audited "Approve anyway" override. validation.md's requirement-to-test map assigns
 * this behaviour to tests/e2e/approval-ci-block.spec.ts.
 *
 * ResolveApproval's CI-red guard reads the session's live (unpersisted) GitHub check
 * state via liveFinder.FindLiveInstance — reproducing that server-side path in e2e
 * would require a real session wired to a real failing PR, which is slow/flaky and
 * already covered by Go tests (approval_service_test.go, approval_handler_integration_test.go).
 * Following the same "intercept and fulfill a fabricated response" precedent as
 * ci-status-badge.spec.ts and notifications-responsive.spec.ts, this spec mocks
 * GetNotificationHistory (to seed one pending approval) and ResolveApproval (to return
 * the real CI-blocked error shape) — exercising the approve/deny UI end to end, not the
 * server-side guard itself.
 *
 * Exercises the /notifications page (NotificationsPage.tsx), not the header bell's
 * slide-out NotificationPanel drawer: Header.tsx/ConditionalHeader is not mounted by
 * the root layout (layout.tsx renders CockpitShell + DrawerNav instead), so its
 * "Open notifications" button never reaches the DOM. NotificationPanel.tsx *is* still
 * globally mounted (layout.tsx renders it directly) and always renders its dialog
 * content regardless of open/closed state (only a CSS transform hides it), so it
 * shares the same GetNotificationHistory-backed data as this page and duplicates every
 * testid/role used here. Every locator below is scoped inside "notifications-content"
 * to avoid matching that also-mounted, off-screen duplicate — the same scoping
 * precedent notifications-responsive.spec.ts documents for the same reason.
 */

import { test, expect, Page, Locator } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const CI_BLOCK_FLAG = "review:block-approval-on-ci-failure";
const CHECKS_URL = "https://github.com/acme/widgets/pull/42/checks";
const BLOCK_TEXT = "Approval blocked: CI is failing on this branch — review before approving.";

async function setCIBlockFlag(request: import("@playwright/test").APIRequestContext, enabled: boolean) {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name: CI_BLOCK_FLAG, enabled },
  });
}

async function mockPendingApproval(page: Page, approvalId: string) {
  await page.route("**/api/session.v1.SessionService/GetNotificationHistory", async (route) => {
    await route.fulfill({
      json: {
        notifications: [
          {
            id: approvalId,
            sessionId: "e2e-ci-block-session",
            sessionName: "e2e-ci-block-session",
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

/** Mocks ResolveApproval to reproduce ApprovalService's real CI-red guard shape. */
async function mockResolveApproval(page: Page) {
  await page.route("**/api/session.v1.SessionService/ResolveApproval", async (route) => {
    const body = route.request().postDataJSON() as { decision?: string; overrideCiBlock?: boolean };
    if (body.decision === "allow" && !body.overrideCiBlock) {
      await route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ code: "failed_precondition", message: `${BLOCK_TEXT} ${CHECKS_URL}` }),
      });
      return;
    }
    await route.fulfill({ json: {} });
  });
}

/** Navigates to /notifications and returns the page-content locator (see file header). */
async function openNotificationsPage(page: Page): Promise<Locator> {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(`${BASE_URL}/notifications`, { waitUntil: "domcontentloaded" });
  return page.getByTestId("notifications-content");
}

test.describe("approval-ci-block", () => {
  test.beforeEach(async ({ request }) => {
    await setCIBlockFlag(request, true);
  });
  test.afterEach(async ({ request }) => {
    await setCIBlockFlag(request, false);
  });

  test("ApprovalBlock_should_ShowInlineExplanation_When_ApproveClickedWithFailingCIAndFlagOn", async ({ page }) => {
    const approvalId = `e2e-ci-block-${Date.now()}`;
    await mockPendingApproval(page, approvalId);
    await mockResolveApproval(page);
    const content = await openNotificationsPage(page);

    await content.getByRole("button", { name: "✓ Approve" }).click();

    const blockMessage = content.getByTestId("ci-block-message");
    await expect(blockMessage).toBeVisible();
    await expect(blockMessage).toContainText(BLOCK_TEXT);
    // Re-enabled (not a disabled button) — the reviewer can still act via the override.
    await expect(content.getByRole("button", { name: "Approve anyway" })).toBeEnabled();
  });

  test("ApprovalBlock_should_OfferViewCIRunLink_When_Blocked", async ({ page }) => {
    const approvalId = `e2e-ci-block-${Date.now()}`;
    await mockPendingApproval(page, approvalId);
    await mockResolveApproval(page);
    const content = await openNotificationsPage(page);

    await content.getByRole("button", { name: "✓ Approve" }).click();

    const link = content.getByTestId("ci-block-view-run-link");
    await expect(link).toBeVisible();
    await expect(link).toHaveText("View CI run");
    await expect(link).toHaveAttribute("href", CHECKS_URL);
    await expect(link).toHaveAttribute("target", "_blank");
  });

  test("ApprovalBlock_should_AllowApproveAnyway_When_ReviewerOverridesBlock", async ({ page }) => {
    const approvalId = `e2e-ci-block-${Date.now()}`;
    await mockPendingApproval(page, approvalId);
    await mockResolveApproval(page);
    const content = await openNotificationsPage(page);

    await content.getByRole("button", { name: "✓ Approve" }).click();
    await expect(content.getByTestId("ci-block-message")).toBeVisible();

    await content.getByRole("button", { name: "Approve anyway" }).click();

    const resolved = content.getByText("✓ Approved");
    await expect(resolved).toBeVisible();
    await expect(resolved).toHaveAttribute("data-decision", "allow");
  });

  test("ApprovalBlock_should_RemainKeyboardOperable_When_ApproveDenyButtonsRendered", async ({ page }) => {
    const approvalId = `e2e-ci-block-${Date.now()}`;
    await mockPendingApproval(page, approvalId);
    await mockResolveApproval(page);
    const content = await openNotificationsPage(page);

    await content.getByRole("button", { name: "✓ Approve" }).click();
    await expect(content.getByTestId("ci-block-message")).toBeVisible();

    const approveAnyway = content.getByRole("button", { name: "Approve anyway" });
    await approveAnyway.focus();
    await expect(approveAnyway).toBeFocused();
    await page.keyboard.press("Enter");

    await expect(content.getByText("✓ Approved")).toBeVisible();
  });

  test("ApprovalBlock_should_PairWarningIconWithText_When_BlockMessageRenders", async ({ page }) => {
    const approvalId = `e2e-ci-block-${Date.now()}`;
    await mockPendingApproval(page, approvalId);
    await mockResolveApproval(page);
    const content = await openNotificationsPage(page);

    await content.getByRole("button", { name: "✓ Approve" }).click();

    const blockMessage = content.getByTestId("ci-block-message");
    const text = (await blockMessage.textContent()) ?? "";
    // Not color-only or glyph-only: the warning glyph and the text label are both present.
    expect(text).toContain("⚠️");
    expect(text).toContain(BLOCK_TEXT);
  });
});
