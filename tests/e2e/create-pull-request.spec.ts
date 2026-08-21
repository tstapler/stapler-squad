// @feature session:draft-pull-request, session:create-pull-request, session-create-pr
/**
 * E2E coverage for the mechanical one-click PR-creation flow (AC8; Epic 3.2).
 *
 * hasCommitsAhead/githubPrUrl on Session are only ever populated server-side from a
 * real git worktree ahead of its base branch (see session/git_worktree_manager.go).
 * Reproducing that live-git path in e2e for every scenario would be slow/flaky.
 * Following the same documented precedent as ci-status-badge.spec.ts's mockCIStatus
 * and vcs-widget.spec.ts's mockShipStatus (intercepting a real RPC response to inject
 * server-computed-only fields onto a real, API-created session), this spec creates a
 * real session via the API and intercepts ListSessions to inject those fields, then
 * intercepts DraftPullRequest/CreatePullRequest themselves so the happy-path and
 * existing-PR tests don't depend on a real `gh` CLI/GitHub call — exercising the real
 * frontend rendering + submit path end to end. The disabled-trigger test needs no
 * mocking: a freshly created session with no real git worktree naturally has
 * hasCommitsAhead=false.
 *
 * WatchSessions is blocked for the session-list page's lifetime so its real
 * (fixture-less) events can't race with and clobber the ListSessions-injected fields.
 */

import { test, expect, Page } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

interface SessionFixture {
  hasCommitsAhead?: boolean;
  githubPrUrl?: string;
  githubPrNumber?: number;
}

async function mockSessionFields(page: Page, sessionId: string, fields: SessionFixture) {
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

async function mockDraftPullRequest(page: Page, fields: Record<string, unknown>) {
  await page.route("**/api/session.v1.SessionService/DraftPullRequest", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fields),
    });
  });
}

async function mockCreatePullRequest(page: Page, fields: Record<string, unknown>) {
  await page.route("**/api/session.v1.SessionService/CreatePullRequest", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fields),
    });
  });
}

// A freshly created session starts in SESSION_STATUS_CREATING and the sidebar list
// excludes it until it settles — WatchSessions is aborted in these tests (see file
// header), so there is no live update to pick up the transition after page load.
// Poll until it leaves Creating before navigating.
async function waitUntilSettled(client: SessionClient, sessionId: string, timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const session = await client.getSession(sessionId);
    if (session.status !== "SESSION_STATUS_CREATING") return session;
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`Session ${sessionId} still SESSION_STATUS_CREATING after ${timeoutMs}ms`);
}

async function openSessionCardMenu(page: Page, title: string) {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
  const card = page.locator('[data-testid="session-card"], [data-testid="session-row"]').filter({ hasText: title });
  await expect(card).toBeVisible({ timeout: 10000 });
  await card.getByRole("button", { name: /more session actions|more actions/i }).click();
  return card;
}

test.describe("create-pull-request", () => {
  // The overflow menu is a long, fixed-position portal anchored to the trigger
  // button; on the default viewport its lower items render below the fold and
  // normal scroll-into-view can't reach them (the menu isn't in document flow).
  test.use({ viewport: { width: 1280, height: 1400 } });

  let client: SessionClient;

  test.beforeEach(() => {
    client = new SessionClient(BASE_URL);
  });

  test("CreatePullRequest_should_PrefillEditSubmitAndShowPrLink_When_HappyPath", async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-create-pr-happy-${ts}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await waitUntilSettled(client, session.id);

    try {
      await mockSessionFields(page, session.id, { hasCommitsAhead: true, githubPrUrl: "" });
      await mockDraftPullRequest(page, {
        title: `feat: ${title}`,
        body: "Auto-generated summary of the session's diff.",
        baseBranch: "main",
        hasCommitsAhead: true,
        existingPrUrl: "",
        existingPrNumber: 0,
      });
      await mockCreatePullRequest(page, {
        prUrl: "https://github.com/tstapler/stapler-squad/pull/999",
        prNumber: 999,
        alreadyExisted: false,
        persisted: true,
        persistError: "",
      });

      await openSessionCardMenu(page, title);
      await page.getByTestId(`create-pr-trigger-${session.id}`).click();

      const titleInput = page.getByTestId("create-pr-title-input");
      await expect(titleInput).not.toHaveValue("", { timeout: 5000 });
      await expect(titleInput).toHaveValue(`feat: ${title}`);

      await titleInput.fill(`feat: ${title} (edited)`);

      await page.getByTestId("create-pr-submit").click();

      const link = page.getByTestId("github-pr-link");
      await expect(link).toBeVisible();
      await expect(link).toHaveAttribute("href", "https://github.com/tstapler/stapler-squad/pull/999");
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });

  test("CreatePullRequest_should_ShowViewPrLink_When_SessionAlreadyHasPr", async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-create-pr-existing-${ts}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await waitUntilSettled(client, session.id);

    try {
      await mockSessionFields(page, session.id, {
        hasCommitsAhead: true,
        githubPrUrl: "https://github.com/tstapler/stapler-squad/pull/500",
        githubPrNumber: 500,
      });

      // The overflow menu renders in a portal outside the card's DOM subtree,
      // so its items are located page-wide, not scoped to `card`.
      await openSessionCardMenu(page, title);

      const link = page.getByTestId("github-pr-link");
      await expect(link).toBeVisible();
      await expect(link).toHaveAttribute("href", "https://github.com/tstapler/stapler-squad/pull/500");
      await expect(page.getByTestId(`create-pr-trigger-${session.id}`)).toHaveCount(0);
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });

  test("CreatePullRequest_should_DisableTrigger_When_NoCommitsAhead", async ({ page }) => {
    const ts = Date.now();
    const title = `e2e-create-pr-disabled-${ts}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await waitUntilSettled(client, session.id);

    try {
      await mockSessionFields(page, session.id, { hasCommitsAhead: false, githubPrUrl: "" });

      await openSessionCardMenu(page, title);

      const trigger = page.getByTestId(`create-pr-trigger-${session.id}`);
      await expect(trigger).toBeVisible();
      await expect(trigger).toBeDisabled();
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });
});
