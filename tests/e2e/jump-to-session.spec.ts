// @feature backlog:jump-to-session
/**
 * E2E coverage for the "Jump to session" panel (SessionsSection.tsx via
 * JumpToSessionPanel.tsx) — opening a real, interactive terminal for a
 * backlog item's most recently active jumpable session (work/review)
 * expanded inline in the detail pane, with no page navigation, plus the
 * role picker for switching to a different role's session.
 *
 * Follows backlog-session-steer.spec.ts's precedent: GetBacklogItem has no
 * way to attach an arbitrary real Session to a BacklogItem outside
 * SpawnSessionFromItem (slow/heavy for e2e), so this intercepts the real
 * response to inject fabricated ItemSession rows pointing at real sessions
 * created via SessionClient — real enough that the panel's GetSession RPC
 * resolves a genuine Session and mounts a genuine xterm.js terminal.
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { SessionClient } from "./helpers/session-client";
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();

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
    .catch(() => {});
}

interface FakeItemSession {
  id: string;
  sessionUuid: string;
  sessionRole: string;
}

/** Intercepts GetBacklogItem for `itemId` and appends the given fabricated ItemSession rows, oldest first. */
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

test.describe("backlog-jump-to-session", () => {
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
      // See backlog-session-steer.spec.ts's matching comment — both onboarding
      // tours must be suppressed or a click can get swallowed by a modal.
      localStorage.setItem("stapler-squad:onboarded", "true");
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test("opens a real terminal for the most recent session in place, with no page navigation", async ({
    page,
    request,
  }) => {
    const itemTitle = `jump-to-session-${Date.now()}`;
    let itemId: string | undefined;
    let realSessionId: string | undefined;

    try {
      itemId = await createInProgressItem(request, itemTitle);
      const realSession = await client.createSession({
        title: `jump-to-session-target-${Date.now()}`,
        path: "/tmp",
        program: "bash",
      });
      realSessionId = realSession.id;

      await injectItemSessions(page, itemId, [
        { id: "e2e-work-session-entity", sessionUuid: realSessionId, sessionRole: "work" },
      ]);

      await openDetail(page, itemTitle);
      const urlBeforeClick = page.url();

      const button = page.getByTestId("jump-to-session-button");
      await expect(button).toBeVisible({ timeout: 10000 });
      await expect(button).toHaveText("Jump to Work session");

      await button.click();

      const panel = page.getByTestId("jump-to-session-panel");
      await expect(panel).toBeVisible();
      await expect(panel.locator(".xterm")).toBeVisible({ timeout: 10000 });
      expect(page.url()).toBe(urlBeforeClick);

      // Collapses again on a second click.
      await button.click();
      await expect(panel).not.toBeVisible();
    } finally {
      if (realSessionId) await client.deleteSession(realSessionId, true).catch(() => {});
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("switches the open terminal to a different role via the role picker", async ({ page, request }) => {
    const itemTitle = `jump-to-session-roles-${Date.now()}`;
    let itemId: string | undefined;
    let workSessionId: string | undefined;
    let reviewSessionId: string | undefined;

    try {
      itemId = await createInProgressItem(request, itemTitle);
      const workSession = await client.createSession({
        title: `jump-to-session-work-${Date.now()}`,
        path: "/tmp",
        program: "bash",
      });
      workSessionId = workSession.id;
      const reviewSession = await client.createSession({
        title: `jump-to-session-review-${Date.now()}`,
        path: "/tmp",
        program: "bash",
      });
      reviewSessionId = reviewSession.id;

      // review listed last (most recent) — the default target.
      await injectItemSessions(page, itemId, [
        { id: "e2e-work-session-entity", sessionUuid: workSessionId, sessionRole: "work" },
        { id: "e2e-review-session-entity", sessionUuid: reviewSessionId, sessionRole: "review" },
      ]);

      await openDetail(page, itemTitle);

      const button = page.getByTestId("jump-to-session-button");
      await expect(button).toBeVisible({ timeout: 10000 });
      await expect(button).toHaveText("Jump to Review session");

      const menuTrigger = page.getByTestId("jump-to-session-role-menu-trigger");
      await expect(menuTrigger).toBeVisible();
      await menuTrigger.click();

      await expect(page.getByTestId("jump-to-session-role-menu")).toBeVisible();
      await page.getByTestId("jump-to-session-role-item-work").click();

      await expect(button).toHaveText("Hide Work session");
      const panel = page.getByTestId("jump-to-session-panel");
      await expect(panel).toBeVisible();
      await expect(panel.locator(".xterm")).toBeVisible({ timeout: 10000 });
    } finally {
      if (workSessionId) await client.deleteSession(workSessionId, true).catch(() => {});
      if (reviewSessionId) await client.deleteSession(reviewSessionId, true).catch(() => {});
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("renders a touch-sized jump button at mobile width", async ({ page, request }) => {
    const itemTitle = `jump-to-session-mobile-${Date.now()}`;
    let itemId: string | undefined;
    let realSessionId: string | undefined;

    try {
      itemId = await createInProgressItem(request, itemTitle);
      const realSession = await client.createSession({
        title: `jump-to-session-mobile-target-${Date.now()}`,
        path: "/tmp",
        program: "bash",
      });
      realSessionId = realSession.id;

      await injectItemSessions(page, itemId, [
        { id: "e2e-work-session-entity", sessionUuid: realSessionId, sessionRole: "work" },
      ]);

      // Below theme-contract.css.ts's breakpoints.sm (640px).
      await page.setViewportSize({ width: 390, height: 844 });

      await openDetail(page, itemTitle);

      const button = page.getByTestId("jump-to-session-button");
      await expect(button).toBeVisible({ timeout: 10000 });
      const box = await button.boundingBox();
      expect(box, "Jump button bounding box not found").not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    } finally {
      if (realSessionId) await client.deleteSession(realSessionId, true).catch(() => {});
      if (itemId) await archiveItem(request, itemId);
    }
  });
});
