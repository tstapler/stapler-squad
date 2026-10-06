// @feature session:crashed-status, session:resume_crashed
/**
 * E2E coverage for the Crashed session status (dead tmux pane detected by
 * SessionHealthChecker, session/health.go) — overlay, resume button, and list
 * badge (Task: "Dead tmux pane not surfaced as a session state" backlog item).
 *
 * SESSION_STATUS_CRASHED is only ever set server-side by SessionHealthChecker
 * polling a real tmux pane's remain-on-exit placeholder — reproducing an actual
 * dead pane in e2e would require killing a real process mid-test, which is slow
 * and flaky. Following ci-status-badge.spec.ts's precedent, this spec creates a
 * real session via the API and intercepts ListSessions to inject the
 * server-computed status/exitReason fields, exercising the real frontend
 * rendering path end to end (the health-check detection logic itself has its
 * own Go coverage in session/health_test.go).
 *
 * WatchSessions is blocked for the session-list page's lifetime so its real
 * (fixture-less) events can't race with and clobber the ListSessions-injected
 * fields — SessionEvent.sessionUpdated payloads are a full Session replace,
 * not a per-field merge.
 */

import { test, expect, Page } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function mockCrashedStatus(page: Page, sessionId: string, exitReason: string) {
  await page.route("**/api/session.v1.SessionService/ListSessions", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    const sessions = (json?.sessions ?? []) as Array<Record<string, unknown>>;
    const target = sessions.find((s) => s.id === sessionId);
    if (target) {
      Object.assign(target, { status: "SESSION_STATUS_CRASHED", exitReason });
    }
    await route.fulfill({ response, json });
  });
  await page.route("**/api/session.v1.SessionService/WatchSessions", async (route) => {
    await route.abort();
  });
}

async function openSession(page: Page, title: string) {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
  const card = page.locator('[data-testid="session-card"], [data-testid="session-row"]').filter({ hasText: title });
  await expect(card).toBeVisible({ timeout: 10000 });
  return card;
}

test.describe("crashed-session-ux", () => {
  let client: SessionClient;

  test.beforeEach(() => {
    client = new SessionClient(BASE_URL);
  });

  test("crashed-session-ux_should_showCrashedBadge_When_sessionListHasCrashedSession", async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-crashed-badge-${ts}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });

    await mockCrashedStatus(page, session.id, "signal SIGKILL (exit code 137)");

    const card = await openSession(page, title);
    // Row view surfaces the status via aria-label (compact dot, no visible text);
    // card view renders it as visible text. Check both so this holds regardless
    // of the user's saved view-mode preference.
    const accessibleText = (await card.getAttribute("aria-label")) ?? (await card.textContent()) ?? "";
    expect(accessibleText).toContain("Crashed");
  });

  test("crashed-session-ux_should_showOverlay_When_crashedSessionOpened", async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-crashed-overlay-${ts}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });

    await mockCrashedStatus(page, session.id, "signal SIGKILL (exit code 137)");

    const card = await openSession(page, title);
    await card.click();

    await expect(page.getByRole("status", { name: "Session has crashed" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("signal SIGKILL (exit code 137)")).toBeVisible();
    await expect(page.getByRole("button", { name: "Resume this session" })).toBeVisible();
  });

  test("crashed-session-ux_should_resumeSession_When_overlayResumeClicked", async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-crashed-resume-${ts}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });

    await mockCrashedStatus(page, session.id, "signal SIGKILL (exit code 137)");

    const card = await openSession(page, title);
    await card.click();

    await expect(page.getByRole("status", { name: "Session has crashed" })).toBeVisible({ timeout: 5000 });

    // The ResumeCrashedSession RPC fires when the button is clicked -- assert the
    // request is actually made with this session's id, rather than requiring the
    // real (non-fixture) tmux resume to succeed end to end.
    const resumeRequest = page.waitForRequest(
      (req) =>
        req.url().includes("/api/session.v1.SessionService/ResumeCrashedSession") &&
        req.method() === "POST"
    );
    await page.getByRole("button", { name: "Resume this session" }).click();
    const request = await resumeRequest;
    const body = request.postDataJSON() as { id?: string };
    expect(body.id).toBe(session.id);
  });
});
