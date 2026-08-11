// @feature session:ci-status-badge
/**
 * E2E coverage for the diff-viewer CI status badge (AC1, AC2, AC7; Task 4.1.1b).
 *
 * GitHubCheckConclusion/GitHubPRNumber/GitHubPRUrl are only ever populated server-side
 * by PRStatusPoller against a real GitHub PR (see plan.md's Implementation Deviations —
 * these fields are not persisted, only held on the live in-memory Instance). Reproducing
 * that live-GitHub-API path in e2e would be slow/flaky. Following the same documented
 * precedent as backlog-pipeline-mode.spec.ts's injectFakeSessionWithPipelineSnapshot and
 * vcs-widget.spec.ts's mockShipStatus (intercepting a real RPC response to inject
 * server-computed-only fields onto a real, API-created session), this spec creates a
 * real session via the API and intercepts ListSessions to inject the CI fields —
 * exercising the real frontend rendering path end to end, not the poller itself (which
 * has its own Go coverage in session/pr_status_poller_test.go).
 *
 * WatchSessions is blocked for the session-list page's lifetime in these tests so its
 * real (fixture-less) events can't race with and clobber the ListSessions-injected
 * fields — SessionEvent.sessionUpdated payloads are a full Session replace per Redux
 * entity-adapter semantics, not a per-field merge.
 */

import { test, expect, Page } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { SessionDetailPage } from "./pages/SessionDetailPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

interface CIFixture {
  githubPrNumber: number;
  githubPrUrl: string;
  githubCheckConclusion: string;
}

/**
 * Intercepts ListSessions and injects `fields` onto the session matching `sessionId`.
 * Also blocks WatchSessions so its unrelated real-time events can't race with and
 * overwrite the injected fixture (see file header).
 */
async function mockCIStatus(page: Page, sessionId: string, fields: CIFixture) {
  await page.route("**/api/session.v1.SessionService/ListSessions", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    const sessions = (json?.sessions ?? []) as Array<Record<string, unknown>>;
    const target = sessions.find((s) => s.id === sessionId);
    if (target) {
      Object.assign(target, fields);
    }
    await route.fulfill({ response, json });
  });
  await page.route("**/api/session.v1.SessionService/WatchSessions", async (route) => {
    await route.abort();
  });
}

async function openSessionDiffTab(page: Page, title: string) {
  // Pre-seed the first-visit onboarding dialog as dismissed so it doesn't intercept
  // clicks on the session list (same pattern as backlog-pipeline-mode.spec.ts).
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
  // The session list renders either "card" (grid) or "row" (list) view depending on the
  // user's saved view-mode preference — match both, mirroring shell-tabs.spec.ts.
  const card = page.locator('[data-testid="session-card"], [data-testid="session-row"]').filter({ hasText: title });
  await expect(card).toBeVisible({ timeout: 10000 });
  await card.click();
  await page.getByRole("tab", { name: "Diff" }).click();
}

test.describe("ci-status-badge", () => {
  let client: SessionClient;

  test.beforeEach(() => {
    client = new SessionClient(BASE_URL);
  });

  test("shows the expected badge text for each CI conclusion fixture", async ({ page }) => {
    const ts = Date.now();
    const cases: Array<{ conclusion: string; expectedText: string }> = [
      { conclusion: "success", expectedText: "Passing" },
      { conclusion: "failure", expectedText: "Failing" },
      { conclusion: "pending", expectedText: "Pending" },
      { conclusion: "", expectedText: "No checks" },
    ];

    for (const { conclusion, expectedText } of cases) {
      const title = `e2e-ci-badge-${conclusion || "none"}-${ts}`;
      const session = await client.createSession({ title, path: "/tmp", program: "bash" });

      await mockCIStatus(page, session.id, {
        githubPrNumber: 42,
        githubPrUrl: "https://github.com/acme/widgets/pull/42",
        githubCheckConclusion: conclusion,
      });

      const sessionDetailPage = new SessionDetailPage(page);
      await openSessionDiffTab(page, title);

      await expect(sessionDetailPage.getCIStatusBadge()).toBeVisible();
      await expect(sessionDetailPage.getCIStatusBadge()).toContainText(expectedText);
    }
  });

  test("CIBadge_should_OpenChecksPageInNewTab_When_Clicked", async ({ page, context }) => {
    const ts = Date.now();
    const title = `e2e-ci-badge-link-${ts}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });

    await mockCIStatus(page, session.id, {
      githubPrNumber: 42,
      githubPrUrl: "https://github.com/acme/widgets/pull/42",
      githubCheckConclusion: "success",
    });

    const sessionDetailPage = new SessionDetailPage(page);
    await openSessionDiffTab(page, title);

    const badge = sessionDetailPage.getCIStatusBadge();
    await expect(badge).toBeVisible();
    await expect(badge).toHaveAttribute("href", "https://github.com/acme/widgets/pull/42/checks");
    await expect(badge).toHaveAttribute("target", "_blank");
  });

  test("shows no badge (not an empty placeholder) for the no-PR fixture", async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-ci-badge-nopr-${ts}`;
    await client.createSession({ title, path: "/tmp", program: "bash" });

    const sessionDetailPage = new SessionDetailPage(page);
    await openSessionDiffTab(page, title);

    await expect(sessionDetailPage.getCIStatusBadge()).not.toBeVisible();
  });
});
