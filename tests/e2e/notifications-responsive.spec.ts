// @feature ui:notifications-page
/**
 * Covers the mobile-resize fix for the Notifications page (header row wrapping
 * instead of clipping on narrow viewports, and the page honoring the Page Scroll
 * Convention so long lists scroll vertically). Notification history is fully
 * mocked via route interception on GetNotificationHistory — the real backend has
 * no seeded notifications in test mode and no RPC exists to inject
 * server-computed history records directly, so this follows the same documented
 * "intercept and fulfill a fabricated response" precedent as
 * backlog-pipeline-mode.spec.ts's injectFakeSessionWithPipelineSnapshot.
 */

import { test, expect, Page } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

interface FakeNotification {
  id: string;
  sessionId: string;
  title: string;
  message: string;
  isRead: boolean;
}

function buildHistoryResponse(notifications: FakeNotification[]) {
  const now = new Date();
  return {
    notifications: notifications.map((n, i) => ({
      id: n.id,
      sessionId: n.sessionId,
      sessionName: n.title,
      notificationType: "NOTIFICATION_TYPE_TASK_COMPLETE",
      priority: "NOTIFICATION_PRIORITY_MEDIUM",
      title: n.title,
      message: n.message,
      createdAt: new Date(now.getTime() - i * 1000).toISOString(),
      isRead: n.isRead,
    })),
    totalCount: notifications.length,
    unreadCount: notifications.filter((n) => !n.isRead).length,
    hasMore: false,
  };
}

async function mockNotificationHistory(page: Page, notifications: FakeNotification[]) {
  const body = buildHistoryResponse(notifications);
  await page.route("**/api/session.v1.SessionService/GetNotificationHistory", async (route) => {
    await route.fulfill({ json: body });
  });
}

test.describe("notifications-responsive", () => {
  test("notifications-responsive > header title, unread badge, and action buttons stay within a 375px viewport", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await mockNotificationHistory(page, [
      { id: "n1", sessionId: "s1", title: "First session", message: "Task finished", isRead: false },
      { id: "n2", sessionId: "s2", title: "Second session", message: "Task finished", isRead: false },
    ]);
    await page.goto(`${BASE_URL}/notifications`, { waitUntil: "load" });

    const title = page.getByTestId("notifications-title");
    const badge = page.getByTestId("notifications-unread-badge");
    const markAllRead = page.getByTestId("notifications-mark-all-read");
    const clearAll = page.getByTestId("notifications-clear-all");

    await expect(title).toBeInViewport({ ratio: 1 });
    await expect(badge).toBeInViewport({ ratio: 1 });
    await expect(markAllRead).toBeInViewport({ ratio: 1 });
    await expect(clearAll).toBeInViewport({ ratio: 1 });
  });

  for (const width of [390, 414]) {
    test(`notifications-responsive > header row has no horizontal clipping at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 844 });
      await mockNotificationHistory(page, [
        { id: "n1", sessionId: "s1", title: "First session", message: "Task finished", isRead: false },
        { id: "n2", sessionId: "s2", title: "Second session", message: "Task finished", isRead: false },
      ]);
      await page.goto(`${BASE_URL}/notifications`, { waitUntil: "load" });

      await expect(page.getByTestId("notifications-header")).toBeVisible();

      const hasHorizontalOverflow = await page.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth
      );
      expect(hasHorizontalOverflow).toBe(false);
    });
  }

  test("notifications-responsive > header row wraps onto a second line instead of overflowing at 375px", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await mockNotificationHistory(page, [
      { id: "n1", sessionId: "s1", title: "First session", message: "Task finished", isRead: false },
    ]);
    await page.goto(`${BASE_URL}/notifications`, { waitUntil: "load" });

    const header = page.getByTestId("notifications-header");
    const title = page.getByTestId("notifications-title");
    const markAllRead = page.getByTestId("notifications-mark-all-read");

    const [headerBox, titleBox, actionBox] = await Promise.all([
      header.boundingBox(),
      title.boundingBox(),
      markAllRead.boundingBox(),
    ]);

    expect(headerBox).not.toBeNull();
    expect(titleBox).not.toBeNull();
    expect(actionBox).not.toBeNull();
    // A wrapped header row is taller than a single line of its own content —
    // the reliable signal that flexWrap kicked in rather than clipping.
    expect(headerBox!.height).toBeGreaterThan(titleBox!.height);
    // The action row starts below the title row once wrapped.
    expect(actionBox!.y).toBeGreaterThan(titleBox!.y);
  });

  test("notifications-responsive > notification list scrolls vertically when content exceeds viewport height", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 600 });
    const notifications: FakeNotification[] = Array.from({ length: 30 }, (_, i) => ({
      id: `n${i}`,
      sessionId: `s${i}`,
      title: `Session ${i}`,
      message: `Task ${i} finished`,
      isRead: i % 2 === 0,
    }));
    await mockNotificationHistory(page, notifications);
    await page.goto(`${BASE_URL}/notifications`, { waitUntil: "load" });

    // Scope to the page content, not the (also-mounted, off-screen) NotificationPanel
    // drawer, which shares the same GetNotificationHistory-backed data.
    const lastItem = page.locator("#main-content").getByText("Task 29 finished");
    await lastItem.scrollIntoViewIfNeeded();
    await expect(lastItem).toBeInViewport();

    const listScrolls = await page.evaluate(() => {
      const el = document.querySelector('[data-testid="notifications-content"]');
      return el ? el.scrollHeight > el.clientHeight : false;
    });
    expect(listScrolls).toBe(true);
  });
});
