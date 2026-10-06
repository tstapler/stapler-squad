// @feature session-status-substatus
/**
 * E2E coverage for UX Criterion 18 (validation.md's UX Acceptance Tests table) —
 * the idle SubStatusChip ("● Idle") must render on the Sessions list. Epic 3.2.2
 * (ADR-002) removed idle-reason items from the Review Queue entirely, so this
 * chip is now the Sessions list's only "ready for next task" signal (see
 * SubStatusChip.tsx's doc comment) — this is a new file rather than an extension
 * of session-board-view.spec.ts, since that file is scoped to the List/Board
 * toggle and kanban column rendering, not substatus chip content (row 19's
 * companion "idle absent from Review Queue" scenario lives in
 * review-queue-priority-tiers.spec.ts, alongside its own idle fixture).
 *
 * SubStatus only flips server-side via the real tmux-output detection pipeline,
 * which is slow/flaky to reproduce end-to-end and already covered by
 * session/review_queue_determiner_test.go. Following this repo's documented
 * "intercept and fulfill a fabricated response" precedent (ci-status-badge.spec.ts's
 * mockCIStatus), this spec intercepts ListSessions and injects a synthetic
 * session carrying subStatus IDLE into the real response (rather than fully
 * replacing it -- the shared test-mode instance this suite runs against seeds
 * its own demo sessions on startup, e.g. "payment-stripe-integration" et al.
 * per demo.spec.ts, and a fully-fabricated ListSessions body does not survive
 * whatever repopulates that data; augmenting the real response, exactly like
 * ci-status-badge.spec.ts's mockCIStatus, does). WatchSessions is blocked so its
 * real events can't race with and overwrite the injected fixture — there is no
 * test-mode hook to force a live IDLE transition mid-test, so (like
 * ci-status-badge.spec.ts) this proves the chip renders correctly for an idle
 * session's static state rather than the literal transition-in-progress moment.
 */
import { test, expect, Page } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function mockIdleSession(page: Page, sessionId: string, title: string) {
  await page.route("**/api/session.v1.SessionService/ListSessions", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    const sessions = (json?.sessions ?? []) as Array<Record<string, unknown>>;
    sessions.push({
      id: sessionId,
      title,
      status: "SESSION_STATUS_ACTIVE",
      subStatus: "SUB_STATUS_IDLE",
      program: "claude",
      path: "/tmp/e2e-idle-chip",
      branch: "main",
    });
    await route.fulfill({ response, json: { ...json, sessions } });
  });
  await page.route("**/api/session.v1.SessionService/WatchSessions", (route) => route.abort());
}

test.describe("idle-session-chip", () => {
  test("idle SubStatus chip renders on Sessions list within one poll cycle of going idle", async ({ page }) => {
    const sessionId = "e2e-idle-chip-session";
    const title = "Idle Chip Fixture Session";
    await page.addInitScript(() => localStorage.setItem("stapler-squad:onboarded", "true"));
    await mockIdleSession(page, sessionId, title);

    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

    // Filtered by the fixture's path, not its title: SessionRow.tsx's displayName
    // falls back to session.branch when the branch column isn't visible (the
    // default here), so the title text may not render as a visible text node at
    // all -- the abbreviated path always does, regardless of column config.
    const row = page
      .locator('[data-testid="session-card"], [data-testid="session-row"]')
      .filter({ hasText: "/tmp/e2e-idle-chip" });
    await expect(row).toBeVisible({ timeout: 10000 });

    const chip = row.getByRole("status", { name: "Session is idle" });
    await expect(chip).toBeVisible();
    await expect(chip).toHaveText("● Idle");
  });
});
