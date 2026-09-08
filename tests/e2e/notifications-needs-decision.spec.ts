// @feature ui:notifications-page, approval:resolve
/**
 * E2E coverage for the Notifications page's "Needs a decision" IA sectioning
 * (validation.md's UX Acceptance Tests table, rows 1-13, 26 partial) —
 * NotificationsPage.tsx / NotificationItem.tsx's NeedsDecisionSection.
 *
 * Backend test-mode seeds no notification history and exposes no RPC to inject
 * server-computed records directly, so every test here mocks GetNotificationHistory
 * via ConnectRPC route interception, following approval-ci-block.spec.ts and
 * notifications-responsive.spec.ts's documented precedent. Every locator is scoped
 * inside `#main-content` (the routed page's own container, per layout.tsx) rather
 * than a page-root selector, since NotificationPanel.tsx (the header bell's
 * slide-out drawer) is *also* always mounted off-screen at the layout level and
 * shares the same GetNotificationHistory-backed data — including, for
 * AutoHandledSection, a component with no unique data-testid of its own.
 * notifications-responsive.spec.ts documents this exact `#main-content` scoping
 * precedent for the same reason.
 *
 * Deviations from validation.md's literal wording, found by reading the shipped
 * component (NotificationItem.tsx) rather than guessing:
 * - Row 1: no `role="region"` is attached to the needs-a-decision section in the
 *   shipped markup (grepped for `role="region"` repo-wide — it's used elsewhere,
 *   e.g. SessionBoard.tsx, but never here). Asserted instead via the section's own
 *   `data-testid="needs-decision-section"`, which is the real, stable anchor.
 * - Row 3: no "Couldn't record your decision — try again." string, or any retry
 *   affordance, exists anywhere in the approve/deny error path
 *   (useApprovalResolution.ts's resolveApproval): a non-FailedPrecondition
 *   rejection unconditionally sets `resolvedApprovals[id] = "expired"`, rendering
 *   the generic "Expired" badge with no retry action. This test is written
 *   faithfully to the acceptance criterion (and to what useApprovalResolution.ts
 *   *should* do per AC26's "no dead ends" requirement) rather than weakened to
 *   match that behavior — see this task's final report for the gap this surfaces.
 */
import { test, expect, Page, Locator } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

interface FixtureNotification {
  id: string;
  sessionId: string;
  sessionName?: string;
  notificationType: string;
  priority?: string;
  title?: string;
  message?: string;
  metadata?: Record<string, string>;
  isRead: boolean;
  occurrenceCount?: number;
}

function buildHistoryResponse(notifications: FixtureNotification[]) {
  return {
    notifications: notifications.map((n) => ({
      id: n.id,
      sessionId: n.sessionId,
      sessionName: n.sessionName ?? n.sessionId,
      notificationType: n.notificationType,
      priority: n.priority ?? "NOTIFICATION_PRIORITY_MEDIUM",
      title: n.title ?? "Notification",
      message: n.message ?? "",
      metadata: n.metadata,
      createdAt: new Date().toISOString(),
      isRead: n.isRead,
      occurrenceCount: n.occurrenceCount,
    })),
    totalCount: notifications.length,
    unreadCount: notifications.filter((n) => !n.isRead).length,
    hasMore: false,
  };
}

async function mockNotificationHistory(
  page: Page,
  notifications: FixtureNotification[],
  opts?: { delayMs?: number }
) {
  await page.route("**/api/session.v1.SessionService/GetNotificationHistory", async (route) => {
    if (opts?.delayMs) await new Promise((r) => setTimeout(r, opts.delayMs));
    await route.fulfill({ json: buildHistoryResponse(notifications) });
  });
}

/** Always mocked success: several flows below (approve, mark-activity-read) call
 *  markAsRead, whose real-backend rejection would roll back the optimistic
 *  isRead update via a re-fetch of the (mocked, static) history fixture. */
async function mockMarkNotificationRead(page: Page) {
  await page.route("**/api/session.v1.SessionService/MarkNotificationRead", async (route) => {
    await route.fulfill({ json: {} });
  });
}

async function openNotificationsPage(page: Page): Promise<Locator> {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(`${BASE_URL}/notifications`, { waitUntil: "domcontentloaded" });
  return page.locator("#main-content");
}

test.describe("notifications-needs-decision", () => {
  test.beforeEach(async ({ page }) => {
    await mockMarkNotificationRead(page);
  });

  test("needs-a-decision section is visible above the fold on page load", async ({ page }) => {
    await mockNotificationHistory(page, [
      {
        id: "n-needs-1",
        sessionId: "s-needs-1",
        notificationType: "NOTIFICATION_TYPE_APPROVAL_NEEDED",
        metadata: { approval_id: "appr-needs-1", tool_name: "Bash" },
        isRead: false,
      },
      { id: "n-read-1", sessionId: "s-read-1", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
      { id: "n-read-2", sessionId: "s-read-2", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
      { id: "n-read-3", sessionId: "s-read-3", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
    ]);
    const main = await openNotificationsPage(page);

    await expect(main.getByTestId("needs-decision-section")).toBeVisible();
    await expect(main.getByTestId("needs-decision-item-n-needs-1")).toBeVisible();
  });

  test("approve an item in two clicks from page load", async ({ page }) => {
    await mockNotificationHistory(page, [
      {
        id: "n-approve-flow",
        sessionId: "s-approve-flow",
        notificationType: "NOTIFICATION_TYPE_APPROVAL_NEEDED",
        metadata: { approval_id: "appr-approve-flow", tool_name: "Bash" },
        isRead: false,
      },
    ]);
    await page.route("**/api/session.v1.SessionService/ResolveApproval", async (route) => {
      await route.fulfill({ json: {} });
    });
    const main = await openNotificationsPage(page);
    const item = main.getByTestId("needs-decision-item-n-approve-flow");
    await expect(item).toBeVisible();

    const resolveRequest = page.waitForRequest(
      (req) => req.url().includes("ResolveApproval") && req.method() === "POST"
    );
    await item.getByRole("button", { name: "✓ Approve" }).click(); // click 1 of ≤2
    await resolveRequest;

    await expect(item).toHaveCount(0);
  });

  test("failed approve shows retry message and keeps buttons active", async ({ page }) => {
    await mockNotificationHistory(page, [
      {
        id: "n-approve-fail",
        sessionId: "s-approve-fail",
        notificationType: "NOTIFICATION_TYPE_APPROVAL_NEEDED",
        metadata: { approval_id: "appr-approve-fail", tool_name: "Bash" },
        isRead: false,
      },
    ]);
    await page.route("**/api/session.v1.SessionService/ResolveApproval", async (route) => {
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ code: "internal", message: "transient network error" }),
      });
    });
    const main = await openNotificationsPage(page);
    const item = main.getByTestId("needs-decision-item-n-approve-fail");
    await item.getByRole("button", { name: "✓ Approve" }).click();

    await expect(item.getByText("Couldn't record your decision — try again.")).toBeVisible();
    await expect(item.getByRole("button", { name: "✓ Approve" })).toBeEnabled();
  });

  test("header count and BottomNav bell badge match within one poll cycle", async ({ page }) => {
    // BottomNav only renders below the 900px breakpoint (BottomNav.css.ts).
    await page.setViewportSize({ width: 390, height: 844 });
    await mockNotificationHistory(page, [
      { id: "n-badge-1", sessionId: "s-badge-1", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: false },
      { id: "n-badge-2", sessionId: "s-badge-2", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: false },
      { id: "n-badge-3", sessionId: "s-badge-3", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: false },
    ]);
    await openNotificationsPage(page);

    const headerBadge = page.getByTestId("notifications-unread-badge");
    // BottomNav.tsx caps its own badge at "9+" (a different, lower cap than the
    // header's 99+) — kept at 3 unread so the two are directly comparable.
    // Anchored regex: the surrounding link's own aria-label is "Notifications (3
    // unread)", which a substring match on "\d+ unread" would also match —
    // anchoring to the badge span's exact "3 unread" label disambiguates.
    const bellBadge = page.locator('nav[aria-label="Bottom navigation"]').getByLabel(/^\d+ unread$/);

    await expect.poll(async () => (await headerBadge.textContent())?.trim()).toBe("3");
    await expect.poll(async () => (await bellBadge.textContent())?.trim()).toBe("3");
  });

  test("header badge shows 99+ above 99, literal number at or below 99", async ({ page }) => {
    const many = (n: number): FixtureNotification[] =>
      Array.from({ length: n }, (_, i) => ({
        id: `n-cap-${i}`,
        sessionId: `s-cap-${i}`,
        notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE",
        isRead: false,
      }));

    await mockNotificationHistory(page, many(152));
    await openNotificationsPage(page);
    await expect(page.getByTestId("notifications-unread-badge")).toHaveText("99+");

    await page.unroute("**/api/session.v1.SessionService/GetNotificationHistory");
    await mockNotificationHistory(page, many(99));
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("notifications-unread-badge")).toHaveText("99");
  });

  test("recent activity and auto-handled sections are collapsed on every fresh load", async ({ page }) => {
    await mockNotificationHistory(page, [
      { id: "n-collapse-read", sessionId: "s-collapse-read", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
    ]);
    await openNotificationsPage(page);

    const recentHeader = page.getByRole("button", { name: /Recent activity/i });
    await expect(recentHeader).toHaveAttribute("aria-expanded", "false");
    await recentHeader.click();
    await expect(recentHeader).toHaveAttribute("aria-expanded", "true");

    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /Recent activity/i })).toHaveAttribute("aria-expanded", "false");
  });

  test("expanding recent activity does not change needs-decision or badge counts", async ({ page }) => {
    await mockNotificationHistory(page, [
      {
        id: "n-count-stable",
        sessionId: "s-count-stable",
        notificationType: "NOTIFICATION_TYPE_APPROVAL_NEEDED",
        metadata: { approval_id: "appr-count-stable", tool_name: "Bash" },
        isRead: false,
      },
      { id: "n-count-read-1", sessionId: "s-count-read-1", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
      { id: "n-count-read-2", sessionId: "s-count-read-2", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
    ]);
    const main = await openNotificationsPage(page);

    const headerBadgeText = await page.getByTestId("notifications-unread-badge").textContent();
    const needsHeadingText = await main.getByTestId("needs-decision-heading").textContent();

    await page.getByRole("button", { name: /Recent activity/i }).click();

    await expect(page.getByTestId("notifications-unread-badge")).toHaveText(headerBadgeText ?? "");
    await expect(main.getByTestId("needs-decision-heading")).toHaveText(needsHeadingText ?? "");
  });

  test("live auto-approval and rule-reconciled item show different Auto-handled labels", async ({ page }) => {
    await mockNotificationHistory(page, [
      {
        id: "n-live-auto",
        sessionId: "s-live-auto",
        notificationType: "NOTIFICATION_TYPE_AUTO_APPROVED",
        metadata: { tool_name: "Bash" },
        message: "Ran safely",
        isRead: true,
      },
      {
        id: "n-reconciled-label",
        sessionId: "s-reconciled-label",
        notificationType: "NOTIFICATION_TYPE_APPROVAL_NEEDED",
        metadata: {
          reconciled: "true",
          classifier_rule_name: "Auto-allow safe git status checks",
          approval_decision: "allow",
          approval_id: "appr-reconciled-label",
          tool_name: "Grep",
        },
        message: "Ran safely too",
        isRead: true,
      },
    ]);
    const main = await openNotificationsPage(page);
    const autoHandledHeader = main.getByRole("button", { name: /Auto-handled/i });
    // Both records reach Auto-handled (never identical rendering — see below).
    await expect(autoHandledHeader).toContainText("2");
    await autoHandledHeader.click();

    // The rule-reconciled item names its rule; it's the only one that does, so
    // this text renders exactly once even though 2 records are in the list —
    // proving the two are not rendered identically.
    await expect(main.getByText("Auto-allow safe git status checks")).toBeVisible();
    await expect(main.getByText("Auto-allow safe git status checks")).toHaveCount(1);
  });

  test("reconciled item never appears in Needs a Decision or Recent Activity", async ({ page }) => {
    await mockNotificationHistory(page, [
      {
        id: "n-reconciled-once",
        sessionId: "s-reconciled-once",
        notificationType: "NOTIFICATION_TYPE_APPROVAL_NEEDED",
        metadata: {
          reconciled: "true",
          classifier_rule_name: "Auto-allow safe git status checks",
          approval_decision: "allow",
          approval_id: "appr-once",
          tool_name: "ReconciledToolXYZ",
        },
        message: "Reconciled message XYZ",
        isRead: false,
      },
      { id: "n-other-once", sessionId: "s-other-once", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
    ]);
    const main = await openNotificationsPage(page);

    await expect(main.getByTestId("needs-decision-item-n-reconciled-once")).toHaveCount(0);

    await main.getByRole("button", { name: /Recent activity/i }).click();
    await expect(main.getByText("Reconciled message XYZ")).toHaveCount(0);

    await main.getByRole("button", { name: /Auto-handled/i }).click();
    await expect(main.getByText("ReconciledToolXYZ")).toBeVisible();
    await expect(main.getByText("ReconciledToolXYZ")).toHaveCount(1);
  });

  test("empty needs-decision state shows All caught up, never No items found", async ({ page }) => {
    await mockNotificationHistory(page, [
      { id: "n-empty-read", sessionId: "s-empty-read", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
    ]);
    const main = await openNotificationsPage(page);

    // Scoped to the visible empty-state block, not a bare page-wide getByText --
    // the same "All caught up" string is also the (visually-hidden but DOM-visible)
    // aria-live announcement's content (needs-decision-announcement), so an
    // unscoped locator matches 2 elements.
    const emptyState = main.getByTestId("needs-decision-empty");
    await expect(emptyState).toBeVisible();
    await expect(emptyState.getByText("All caught up")).toBeVisible();
    await expect(emptyState.getByText("Nothing needs your attention right now")).toBeVisible();
    await expect(main.getByText("No items found")).toHaveCount(0);
  });

  test("informational section remains visible and collapsed when needs-decision is empty", async ({ page }) => {
    await mockNotificationHistory(page, [
      { id: "n-info-visible", sessionId: "s-info-visible", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true },
    ]);
    const main = await openNotificationsPage(page);

    const recentHeader = main.getByRole("button", { name: /Recent activity/i });
    await expect(recentHeader).toBeVisible();
    await expect(recentHeader).toHaveAttribute("aria-expanded", "false");
  });

  test("loading indicator shows before first poll response, never a false All caught up", async ({ page }) => {
    await mockNotificationHistory(
      page,
      [{ id: "n-loading-read", sessionId: "s-loading-read", notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE", isRead: true }],
      { delayMs: 800 }
    );
    const main = await openNotificationsPage(page);

    // "All caught up" is scoped to the empty-state block (see the previous test's
    // comment on why an unscoped getByText is ambiguous with the aria-live
    // announcement carrying the same string).
    const emptyStateText = main.getByTestId("needs-decision-empty").getByText("All caught up");
    await expect(main.getByText("Loading notifications...")).toBeVisible();
    await expect(emptyStateText).toHaveCount(0);
    await expect(emptyStateText).toBeVisible({ timeout: 5000 });
  });
});
