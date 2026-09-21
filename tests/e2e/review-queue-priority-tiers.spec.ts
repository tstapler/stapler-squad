// @feature review-queue-priority-tiers, session-status-substatus
/**
 * E2E coverage for Epic 3.2's Review Queue priority tiering (validation.md's UX
 * Acceptance Tests table, rows 19, 6(RQ), Bulk skip, 8(RQ)) — the always-expanded
 * "Needs a decision" tier (Urgent/High/Medium) vs. the collapsed-by-default
 * "Informational" tier (Low), and Epic 3.2.2's removal of idle-reason items from
 * the queue entirely (ADR-002).
 *
 * Following review-queue-severity.spec.ts's documented precedent (GetReviewQueue's
 * live server-side ordering/enrichment is slow/flaky to reproduce end-to-end and
 * already covered by Go tests + ReviewQueuePanel.test.tsx), this spec mocks
 * session.v1.SessionService/GetReviewQueue with a fabricated ReviewQueue payload
 * and drives the real ReviewQueuePanel UI against it.
 */
import { test, expect, Page } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

interface QueueFixtureItem {
  sessionId: string;
  sessionName: string;
  reason?: string;
  priority?: string;
  metadata?: Record<string, string>;
}

function makeReviewQueueResponse(items: QueueFixtureItem[]) {
  return {
    reviewQueue: {
      totalItems: items.length,
      items: items.map((it) => ({
        sessionId: it.sessionId,
        sessionName: it.sessionName,
        reason: it.reason ?? "ATTENTION_REASON_APPROVAL_PENDING",
        priority: it.priority ?? "PRIORITY_HIGH",
        detectedAt: new Date().toISOString(),
        context: "e2e fixture context",
        program: "claude",
        branch: "main",
        path: "/tmp/e2e-repo",
        tags: [],
        category: "",
        metadata: it.metadata ?? {},
      })),
      byPriority: {},
      byReason: {},
      averageAgeSeconds: "0",
      oldestItemId: items[0]?.sessionId ?? "",
      oldestAgeSeconds: "0",
    },
  };
}

async function mockReviewQueue(page: Page, items: QueueFixtureItem[]) {
  await page.route("**/api/session.v1.SessionService/GetReviewQueue", async (route) => {
    await route.fulfill({ json: makeReviewQueueResponse(items) });
  });
  await page.route("**/api/session.v1.SessionService/WatchReviewQueue", (route) => route.abort());
}

async function openReviewQueue(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(`${BASE_URL}/review-queue`, { waitUntil: "domcontentloaded" });
  await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000, state: "attached" });
}

test.describe("review-queue-priority-tiers", () => {
  test("Urgent/High items render expanded, Low items collapsed under Informational", async ({ page }) => {
    await mockReviewQueue(page, [
      {
        sessionId: "s-tier-urgent",
        sessionName: "Urgent Item",
        priority: "PRIORITY_URGENT",
        metadata: { pending_approval_id: "appr-tier-urgent", tool_name: "Bash" },
      },
      { sessionId: "s-tier-low", sessionName: "Low Item", priority: "PRIORITY_LOW", reason: "ATTENTION_REASON_TASK_COMPLETE" },
    ]);
    await openReviewQueue(page);

    await expect(page.getByTestId("review-item-s-tier-urgent")).toBeVisible();
    await expect(page.getByTestId("review-item-s-tier-low")).not.toBeAttached();

    await page.getByTestId("collapsible-header-review-queue-informational").click();
    await expect(page.getByTestId("review-item-s-tier-low")).toBeVisible();
  });

  test("Skip all only targets the Needs a Decision tier", async ({ page }) => {
    await mockReviewQueue(page, [
      { sessionId: "s-bulk-nd-1", sessionName: "ND One", priority: "PRIORITY_HIGH", reason: "ATTENTION_REASON_TASK_COMPLETE" },
      { sessionId: "s-bulk-nd-2", sessionName: "ND Two", priority: "PRIORITY_MEDIUM", reason: "ATTENTION_REASON_TASK_COMPLETE" },
      { sessionId: "s-bulk-info-1", sessionName: "Info One", priority: "PRIORITY_LOW", reason: "ATTENTION_REASON_TASK_COMPLETE" },
      { sessionId: "s-bulk-info-2", sessionName: "Info Two", priority: "PRIORITY_LOW", reason: "ATTENTION_REASON_TASK_COMPLETE" },
      { sessionId: "s-bulk-info-3", sessionName: "Info Three", priority: "PRIORITY_LOW", reason: "ATTENTION_REASON_TASK_COMPLETE" },
    ]);
    const acknowledgedIds: string[] = [];
    // acknowledgeSessions (useReviewQueue.ts) fans out one AcknowledgeSession RPC
    // per sessionId — there is no plural bulk "AcknowledgeSessions" endpoint.
    await page.route("**/api/session.v1.SessionService/AcknowledgeSession", async (route) => {
      const body = route.request().postDataJSON() as { id?: string };
      if (body.id) acknowledgedIds.push(body.id);
      await route.fulfill({ json: {} });
    });

    await openReviewQueue(page);
    page.once("dialog", (dialog) => dialog.accept());
    await page.getByTestId("skip-all-visible").click();

    await expect.poll(() => acknowledgedIds.length).toBe(2);
    expect(acknowledgedIds.sort()).toEqual(["s-bulk-nd-1", "s-bulk-nd-2"]);
  });

  test("Review Queue empty needs-decision state matches Notifications page's calm treatment", async ({ page }) => {
    await mockReviewQueue(page, [
      { sessionId: "s-empty-info-only", sessionName: "Info Only", priority: "PRIORITY_LOW", reason: "ATTENTION_REASON_TASK_COMPLETE" },
    ]);
    await openReviewQueue(page);

    await expect(page.getByTestId("needs-decision-empty")).toBeVisible();
    await expect(page.getByText("All caught up")).toBeVisible();
    await expect(page.getByText("Nothing needs your attention right now")).toBeVisible();
    await expect(page.getByTestId("collapsible-header-review-queue-informational")).toHaveAttribute(
      "aria-expanded",
      "false"
    );
  });

  test("idle session shows on Sessions list chip and is absent from Review Queue simultaneously", async ({ page }) => {
    const sessionId = "e2e-idle-shared-session";
    const title = "Idle Shared Fixture Session";
    await page.addInitScript(() => localStorage.setItem("stapler-squad:onboarded", "true"));

    await page.route("**/api/session.v1.SessionService/ListSessions", async (route) => {
      // Augments the real ListSessions response rather than replacing it -- the
      // shared test-mode instance seeds its own demo sessions on startup (see
      // idle-session-chip.spec.ts's identical note); a fully-fabricated body
      // does not survive whatever repopulates that data.
      const response = await route.fetch();
      const json = await response.json();
      const sessions = (json?.sessions ?? []) as Array<Record<string, unknown>>;
      sessions.push({
        id: sessionId,
        title,
        status: "SESSION_STATUS_ACTIVE",
        subStatus: "SUB_STATUS_IDLE",
        program: "claude",
        path: "/tmp/e2e-idle-shared",
        branch: "main",
      });
      await route.fulfill({ response, json: { ...json, sessions } });
    });
    await page.route("**/api/session.v1.SessionService/WatchSessions", (route) => route.abort());
    await mockReviewQueue(page, [
      { sessionId, sessionName: title, reason: "ATTENTION_REASON_IDLE", priority: "PRIORITY_LOW" },
    ]);

    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    // Filtered by path, not title -- see idle-session-chip.spec.ts's identical
    // note on why SessionRow.tsx's displayName may not render the title as a
    // visible text node (its branch-column-visibility fallback).
    const sessionRow = page
      .locator('[data-testid="session-card"], [data-testid="session-row"]')
      .filter({ hasText: "/tmp/e2e-idle-shared" });
    await expect(sessionRow).toBeVisible({ timeout: 10000 });
    await expect(sessionRow.getByRole("status", { name: "Session is idle" })).toBeVisible();

    await openReviewQueue(page);
    // isReviewQueueVisible() (reviewQueueVisibility.ts) filters ATTENTION_REASON_IDLE
    // items client-side before the panel ever renders them.
    await expect(page.getByTestId(`review-item-${sessionId}`)).toHaveCount(0);
  });
});
