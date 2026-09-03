// @feature session:create, pi-support
/**
 * E2E coverage for Epic 6.2 Story 6.2.1: selecting the "pi" program preset
 * from the Omnibar and creating a session with it. Per plan.md's Phase 6
 * scope, this covers only the AC actually listed there -- picker selection
 * plumbed through to a created session's `program` field -- not the full
 * health-badge/approval-alert UX matrix validation.md sketches elsewhere,
 * which is out of scope for this story.
 */
import { test, expect } from "@playwright/test";
import { SessionsPage } from "./pages/SessionsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const PI_SUPPORT_FLAG_NAME = "pi-support";

async function setPiSupportFlag(request: import("@playwright/test").APIRequestContext, enabled: boolean) {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name: PI_SUPPORT_FLAG_NAME, enabled },
  });
}

test.describe("pi session creation", () => {
  test.beforeEach(async ({ page, request }) => {
    await setPiSupportFlag(request, true);
    // Pre-seed the first-visit onboarding dialog as dismissed so it doesn't
    // intercept clicks on the omnibar's Create button (same pattern as
    // session-auto-approve.spec.ts / ci-status-badge.spec.ts).
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  });

  test.afterEach(async ({ request }) => {
    await setPiSupportFlag(request, false);
  });

  test("creates a session with the pi preset and the created session's program field is pi", async ({ page }) => {
    const sessionsPage = new SessionsPage(page);
    await page.setViewportSize({ width: 1280, height: 2200 });
    await sessionsPage.goto();

    const { sessionId, program } = await sessionsPage.createPiSession("e2e-pi-session-test");

    expect(program).toBe("pi");
    expect(sessionId).toBeTruthy();
  });
});
