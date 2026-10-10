// @feature session-detail, notification:deep-link
/**
 * Story 5.3 (client read-only mode and reliable deep link): RO-1, RO-3, RO-4, RO-7, RO-10.
 *
 * The shared test-mode instance has no hidden-session seeding hook (the e2e session client
 * cannot create `Hidden: true` sessions), so, following idle-session-chip.spec.ts's
 * precedent, ListSessions and GetSession are fulfilled with fabricated responses. The
 * server-side guarantee (RO-2: 0 bytes, 0 resizes) is covered by the Go stream tests.
 * AWAITING OPERATOR: Spike 1.1 (live "View Session" repros against a real hidden session).
 */
import { test, expect, Page } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const HIDDEN_ID = "e2e-hidden-review-session";

const hiddenSession = {
  id: HIDDEN_ID,
  title: "review:ee1b4be0",
  status: "SESSION_STATUS_ACTIVE",
  program: "claude",
  path: "/tmp/e2e-hidden-view",
  branch: "main",
  hidden: true,
};

async function emptyList(page: Page) {
  await page.route("**/api/session.v1.SessionService/ListSessions", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [] }) }),
  );
  await page.route("**/api/session.v1.SessionService/WatchSessions", (route) => route.abort());
}

async function getSessionOk(page: Page) {
  await page.route("**/api/session.v1.SessionService/GetSession", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ session: hiddenSession }) }),
  );
}

async function getSessionNotFound(page: Page) {
  await page.route("**/api/session.v1.SessionService/GetSession", (route) =>
    route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ code: "not_found", message: "session not found" }),
    }),
  );
}

test.describe("hidden-session-view", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem("stapler-squad:onboarded", "true"));
  });

  test("ro4_should_open_on_cold_deep_link_with_empty_list_within_budget", async ({ page }) => {
    await emptyList(page);
    await getSessionOk(page);

    const started = Date.now();
    await page.goto(`${BASE_URL}/?session=${HIDDEN_ID}&tab=terminal`, { waitUntil: "domcontentloaded" });
    const banner = page.getByTestId("readonly-banner");
    await expect(banner).toBeVisible({ timeout: 4000 });
    expect(Date.now() - started).toBeLessThan(4000);
    await expect(banner).toContainText("Background session - read-only");
  });

  test("ro1_should_hide_input_chrome_for_a_hidden_session", async ({ page }) => {
    await emptyList(page);
    await getSessionOk(page);

    await page.goto(`${BASE_URL}/?session=${HIDDEN_ID}&tab=terminal`, { waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("readonly-banner")).toBeVisible();
    await expect(page.getByTestId("readonly-banner-secondary")).toHaveText("Terminal input is disabled in this view.");
    await expect(page.getByRole("button", { name: /mobile keyboard/i })).toHaveCount(0);
    await expect(page.getByTestId("mobile-key")).toHaveCount(0);
  });

  test("ro10_should_show_card_without_notification_text_when_the_session_is_gone", async ({ page }) => {
    await emptyList(page);
    await getSessionNotFound(page);

    await page.goto(`${BASE_URL}/?session=${HIDDEN_ID}&tab=terminal&notification=pruned-id`, {
      waitUntil: "domcontentloaded",
    });
    const card = page.getByTestId("session-unavailable-card");
    await expect(card).toBeVisible();
    await expect(card).toContainText("Session no longer available");
    await expect(page.getByTestId("session-unavailable-open-notifications")).toBeVisible();
    await expect(page.getByTestId("session-unavailable-go-to-sessions")).toBeVisible();
    await expect(page.getByTestId("session-unavailable-notification")).toHaveCount(0);
  });

  test("ro7_should_retry_get_session_without_reload", async ({ page }) => {
    await emptyList(page);
    let calls = 0;
    await page.route("**/api/session.v1.SessionService/GetSession", (route) => {
      calls += 1;
      if (calls === 1) {
        return route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ code: "unavailable", message: "try later" }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ session: hiddenSession }),
      });
    });

    await page.goto(`${BASE_URL}/?session=${HIDDEN_ID}&tab=terminal`, { waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("session-unavailable-card")).toContainText("Could not load session.");
    await page.getByTestId("session-unavailable-retry").click();
    await expect(page.getByTestId("readonly-banner")).toBeVisible();
  });
});
