// @feature session:update, backlog-session-steer
/**
 * E2E coverage for Gap 2 (project_plans/backlog-operator-feedback-loop,
 * Epic 2) — steering a live work/review session inline from
 * BacklogItemDetail's Sessions section (SessionsSection.tsx), and its
 * absence for headless-triage (Synthetic Session) rows per ADR-002.
 *
 * A backlog item's linked sessions (ItemSession rows) are a separate
 * concept from a session.Instance — no existing RPC links an arbitrary real
 * Session to a BacklogItem without going through SpawnSessionFromItem (a
 * real tmux/git-worktree work session, slow/heavy for e2e). Following
 * backlog-pipeline-mode.spec.ts's precedent, this spec intercepts the real
 * GetBacklogItem response to inject a fabricated ItemSession entry.
 *
 * For the tests that actually invoke Send (steering a live session, Enter
 * submits), the injected ItemSession's sessionUuid is a *real* session
 * created via SessionClient (a real tmux pane) — the real UpdateSession RPC
 * then genuinely reaches a live Instance and calls SendKeys, exercising the
 * real backend widened-UpdateSession path (Epic 2.1) end to end rather than
 * mocking the RPC entirely. Tests that never click Send (ended session,
 * touch target, headless-triage absence) use a fully synthetic sessionUuid
 * since no RPC call is ever made against it.
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { SessionClient } from "./helpers/session-client";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function waitForBacklogRPCsEnabled(request: APIRequestContext) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListPipelineModes`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

async function createInProgressItem(request: APIRequestContext, title: string): Promise<string> {
  const res = await request.post(`${BASE_URL}/api/debug/backlog/mutate-create`, {
    headers: { "Content-Type": "application/json" },
    data: { title, status: "in_progress", priority: 3 },
  });
  if (!res.ok()) {
    throw new Error(`mutate-create failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { itemId: string };
  return body.itemId;
}

async function archiveItem(request: APIRequestContext, itemId: string) {
  await request
    .post(`${BASE_URL}/api/session.v1.BacklogService/ArchiveBacklogItem`, {
      headers: { "Content-Type": "application/json" },
      data: { id: itemId },
    })
    .catch(() => {
      // Best-effort cleanup — do not fail the test on cleanup errors.
    });
}

interface FakeItemSession {
  id: string;
  sessionUuid: string;
  sessionRole: string;
  endedAt?: string;
  estimatedCostUsd?: number;
}

/** Intercepts GetBacklogItem for `itemId` and appends the given fabricated ItemSession rows. */
async function injectItemSessions(page: Page, itemId: string, sessions: FakeItemSession[]) {
  await page.route("**/api/session.v1.BacklogService/GetBacklogItem", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    if (json?.item?.id === itemId) {
      json.item.itemSessions = [
        ...(json.item.itemSessions ?? []),
        ...sessions.map((s) => ({ estimatedCostUsd: 0, ...s })),
      ];
    }
    await route.fulfill({ response, json });
  });
}

async function openDetail(page: Page, itemTitle: string): Promise<BacklogPage> {
  const backlogPage = new BacklogPage(page);
  await backlogPage.goto();
  await backlogPage.waitForPageLoad();
  await backlogPage.openItemDetail(itemTitle);
  await expect(backlogPage.getItemDetailPane()).toBeVisible();
  return backlogPage;
}

test.describe("backlog-session-steer", () => {
  let client: SessionClient;

  test.beforeAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { "Content-Type": "application/json" },
      data: { name: "backlog", enabled: true },
    });
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { "Content-Type": "application/json" },
      data: { name: "backlog", enabled: false },
    });
  });

  test.beforeEach(async ({ page }) => {
    client = new SessionClient(BASE_URL);
    await page.addInitScript(() => {
      // Two separate onboarding modals can intercept pointer events on first
      // load: the app-wide "One place for all your AI coding sessions" tour
      // (useOnboarding.ts's ONBOARDED_KEY) and the backlog-specific "How
      // backlog items work" tour (useBacklogTour.ts's BACKLOG_ONBOARDED_KEY).
      // Both must be suppressed or a Steer click can get swallowed by
      // whichever modal is still showing (see backlog-plan-approval-flicker
      // .spec.ts, which documents this same pair).
      localStorage.setItem("stapler-squad:onboarded", "true");
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  // Parametrized over both Real-Session roles (sessionKind.ts:52-55 treats
  // "work" and "review" identically for isSteerable()) — AC6 requires
  // steering to work for "triage, review, or work" sessions, and this is the
  // e2e proof that a review-role row genuinely reaches the live Instance via
  // the real UpdateSession RPC, not just the work-role case.
  for (const role of ["work", "review"] as const) {
    test(`steers a live ${role} session and hides the control for headless triage sessions`, async ({
      page,
      request,
    }) => {
      const itemTitle = `session-steer-live-${role}-${Date.now()}`;
      let itemId: string | undefined;
      let realSessionId: string | undefined;
      const headlessSessionId = `headless-triage-e2e-fake-${role}-${Date.now()}`;

      try {
        itemId = await createInProgressItem(request, itemTitle);
        const realSession = await client.createSession({
          title: `session-steer-live-target-${role}-${Date.now()}`,
          path: "/tmp",
          program: "bash",
        });
        realSessionId = realSession.id;

        await injectItemSessions(page, itemId, [
          { id: "e2e-session-entity", sessionUuid: realSessionId, sessionRole: role },
          { id: "e2e-headless-session-entity", sessionUuid: headlessSessionId, sessionRole: "triage" },
        ]);

        await openDetail(page, itemTitle);

        const toggle = page.getByTestId(`session-steer-toggle-${realSessionId}`);
        await expect(toggle).toBeVisible();
        await expect(toggle).toBeEnabled();

        await toggle.click();
        await page.getByTestId(`session-steer-input-${realSessionId}`).fill("please run the tests");
        await page.getByTestId(`session-steer-submit-${realSessionId}`).click();

        await expect(page.getByRole("alert").filter({ hasText: "Steering message sent." })).toBeVisible();

        // Absent, not just hidden — a headless-triage row never renders a
        // steer toggle at all (ADR-002).
        await expect(page.getByTestId(`session-steer-toggle-${headlessSessionId}`)).toHaveCount(0);
      } finally {
        if (realSessionId) await client.deleteSession(realSessionId, true).catch(() => {});
        if (itemId) await archiveItem(request, itemId);
      }
    });
  }

  test("submits on Enter and cancels with focus return on Escape", async ({ page, request }) => {
    const itemTitle = `session-steer-keyboard-${Date.now()}`;
    let itemId: string | undefined;
    let realSessionId: string | undefined;

    try {
      itemId = await createInProgressItem(request, itemTitle);
      const realSession = await client.createSession({
        title: `session-steer-keyboard-target-${Date.now()}`,
        path: "/tmp",
        program: "bash",
      });
      realSessionId = realSession.id;

      await injectItemSessions(page, itemId, [
        { id: "e2e-work-session-entity", sessionUuid: realSessionId, sessionRole: "work" },
      ]);

      await openDetail(page, itemTitle);

      const toggle = page.getByTestId(`session-steer-toggle-${realSessionId}`);
      const input = page.getByTestId(`session-steer-input-${realSessionId}`);

      // This test spawns a real tmux-backed session via client.createSession
      // (unlike the fake-sessionId tests elsewhere in this file), so the
      // detail pane's first render can occasionally take longer than the
      // default action timeout under load — wait explicitly before the
      // first interaction rather than letting toggle.click() itself be the
      // (shorter) implicit wait. See the matching comment on the
      // ended-session test below.
      await expect(toggle).toBeVisible({ timeout: 10000 });

      // Enter submits, same success path as clicking Send.
      await toggle.click();
      await input.fill("run the linter");
      await input.press("Enter");
      await expect(page.getByRole("alert").filter({ hasText: "Steering message sent." })).toBeVisible();

      // Escape cancels and returns focus to the toggle.
      await expect(toggle).toBeVisible({ timeout: 5000 });
      await toggle.click();
      await expect(input).toBeVisible();
      await input.fill("partial draft, never sent");
      await input.press("Escape");

      await expect(page.getByTestId(`session-steer-input-${realSessionId}`)).toHaveCount(0);
      const activeTestId = await page.evaluate(() => document.activeElement?.getAttribute("data-testid"));
      expect(activeTestId).toBe(`session-steer-toggle-${realSessionId}`);
    } finally {
      if (realSessionId) await client.deleteSession(realSessionId, true).catch(() => {});
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("keeps the composer open and shows the error when steering fails", async ({ page, request }) => {
    const itemTitle = `session-steer-fails-${Date.now()}`;
    let itemId: string | undefined;
    const fakeSessionId = `e2e-fake-work-session-${Date.now()}`;

    try {
      itemId = await createInProgressItem(request, itemTitle);
      await injectItemSessions(page, itemId, [
        { id: "e2e-work-session-entity", sessionUuid: fakeSessionId, sessionRole: "work" },
      ]);

      await page.route("**/api/session.v1.SessionService/UpdateSession", async (route) => {
        await route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify({ code: "failed_precondition", message: "session is not accepting input" }),
        });
      });

      await openDetail(page, itemTitle);

      await page.getByTestId(`session-steer-toggle-${fakeSessionId}`).click();
      const input = page.getByTestId(`session-steer-input-${fakeSessionId}`);
      await input.fill("this must not be lost");
      await page.getByTestId(`session-steer-submit-${fakeSessionId}`).click();

      // useSessionService.updateSession() swallows the RPC error and returns
      // null; handleSteerSession re-throws the real error message read from
      // the sessions-error redux slice (fix commit 255d22505 — previously a
      // hardcoded "Failed to steer session." for every failure cause) so
      // SessionsSection's composer catch can keep itself open (ADR-001) with
      // the actual failure reason. Both the composer's own inline steerError
      // span and BacklogItemDetail's action toast render this same text with
      // role="alert" — assert the composer's specifically (scoped to the
      // steer form).
      await expect(
        page
          .locator(`#session-steer-composer-${fakeSessionId}`)
          .locator('[role="alert"]', { hasText: "session is not accepting input" })
      ).toBeVisible();
      await expect(input).toBeVisible();
      await expect(input).toHaveValue("this must not be lost");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("shows a disabled Steer button with an explanatory title for an ended session", async ({
    page,
    request,
  }) => {
    const itemTitle = `session-steer-ended-${Date.now()}`;
    let itemId: string | undefined;
    const endedSessionId = `e2e-fake-ended-session-${Date.now()}`;

    try {
      itemId = await createInProgressItem(request, itemTitle);
      await injectItemSessions(page, itemId, [
        {
          id: "e2e-ended-session-entity",
          sessionUuid: endedSessionId,
          sessionRole: "work",
          endedAt: new Date().toISOString(),
        },
      ]);

      await openDetail(page, itemTitle);

      const toggle = page.getByTestId(`session-steer-toggle-${endedSessionId}`);
      // See triage-question-answer.spec.ts's matching comment: the injected
      // GetBacklogItem response adds an extra network round trip to the
      // detail pane's first load, which can occasionally exceed the default
      // 5s timeout under load — a longer timeout here avoids a flaky false
      // negative without masking a real absence.
      await expect(toggle).toBeVisible({ timeout: 10000 });
      await expect(toggle).toBeDisabled();
      await expect(toggle).toHaveAttribute("title", "Session has ended — steering is unavailable");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("renders a touch-sized Steer button and stacked composer below the sm breakpoint", async ({
    page,
    request,
  }) => {
    const itemTitle = `session-steer-mobile-${Date.now()}`;
    let itemId: string | undefined;
    const fakeSessionId = `e2e-fake-mobile-session-${Date.now()}`;

    try {
      itemId = await createInProgressItem(request, itemTitle);
      await injectItemSessions(page, itemId, [
        { id: "e2e-mobile-session-entity", sessionUuid: fakeSessionId, sessionRole: "work" },
      ]);

      // Below theme-contract.css.ts's breakpoints.sm (640px).
      await page.setViewportSize({ width: 390, height: 844 });

      await openDetail(page, itemTitle);

      const toggle = page.getByTestId(`session-steer-toggle-${fakeSessionId}`);
      await expect(toggle).toBeVisible();
      const box = await toggle.boundingBox();
      expect(box, "Steer toggle bounding box not found").not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);

      await toggle.click();
      const composer = page.locator(`#session-steer-composer-${fakeSessionId}`);
      await expect(composer).toBeVisible();
      const flexDirection = await composer.evaluate((el) => getComputedStyle(el).flexDirection);
      expect(flexDirection).toBe("column");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });
});
