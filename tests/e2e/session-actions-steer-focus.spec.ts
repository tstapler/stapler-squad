// @feature session:update
/**
 * Real-browser regression coverage for pr-fix-steering Story 1.1.3's 3rd-pass
 * UX finding: disabling the "Give Direction" dialog's focused input the
 * instant a steer RPC becomes in-flight must not rely on the browser's
 * implicit auto-blur-to-document.body behavior — jsdom/RTL (see
 * SessionActionsOverflow.test.tsx's "give direction (steer) dialog" describe
 * block) cannot reproduce that auto-blur, so only a real browser can catch a
 * regression here. See SessionActionsOverflow.tsx's onKeyDown/onClick
 * handlers on the steer dialog's input/Send button for the fix
 * (steerCancelButtonRef.current?.focus() called before setIsSteering(true)
 * takes visual effect).
 *
 * A freshly created session has no autonomousMode field settable via the
 * CreateSession RPC, and flipping it via a real UpdateSession would need a
 * live ClaudeController to make the *second* (steer) UpdateSession call
 * genuinely succeed — unnecessary for a focus-handling test. Instead, this
 * intercepts ListSessions to inject autonomousMode=true onto a real,
 * API-created session (same "mutate a real RPC response" technique
 * create-pull-request.spec.ts's mockSessionFields uses for hasCommitsAhead/
 * githubPrUrl), then intercepts UpdateSession itself to hold the steer
 * request open until the test releases it, and fulfills with that same
 * already-proto-shaped session object — giving a deterministic, genuinely
 * successful resolution without depending on real controller/PTY machinery.
 */

import { test, expect, Page } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { waitUntilSettled } from "./helpers/wait";
import { SessionsPage } from "./pages/SessionsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

interface SteerFixture {
  /** Resolves once ListSessions has been intercepted at least once, so the
   * caller knows capturedSession is populated before navigating. */
  ready: Promise<void>;
  /** Releases the held UpdateSession request, letting it resolve with the
   * captured (autonomousMode=true) session as a genuine success response. */
  releaseSteerRequest: () => void;
}

async function mockAutonomousSessionAndHoldSteer(page: Page, sessionId: string): Promise<SteerFixture> {
  let capturedSession: Record<string, unknown> | null = null;
  let markReady: () => void = () => {};
  const ready = new Promise<void>((resolve) => {
    markReady = resolve;
  });

  await page.route("**/api/session.v1.SessionService/ListSessions", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    const sessions = (json?.sessions ?? []) as Array<Record<string, unknown>>;
    const target = sessions.find((s) => s.id === sessionId);
    if (target) {
      target.autonomousMode = true;
      capturedSession = target;
      markReady();
    }
    await route.fulfill({ response, json });
  });
  // WatchSessions would otherwise clobber the injected autonomousMode field
  // on the next push — same reasoning as create-pull-request.spec.ts.
  await page.route("**/api/session.v1.SessionService/WatchSessions", async (route) => {
    await route.abort();
  });

  let releaseSteerRequest: () => void = () => {};
  const gate = new Promise<void>((resolve) => {
    releaseSteerRequest = resolve;
  });
  await page.route("**/api/session.v1.SessionService/UpdateSession", async (route) => {
    await gate;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ session: capturedSession }),
    });
  });

  return { ready, releaseSteerRequest };
}

test.describe("session-actions-steer-focus", () => {
  // The overflow menu is a long, fixed-position portal anchored to the
  // trigger button; on the default viewport its lower items (including
  // "Give direction") render below the fold — mirrors create-pull-request
  // .spec.ts's identical workaround.
  test.use({ viewport: { width: 1280, height: 1400 } });

  let client: SessionClient;

  test.beforeEach(() => {
    client = new SessionClient(BASE_URL);
  });

  test("moving focus to Cancel before disabling the input keeps Escape and Tab working while a steer is in flight", async ({
    page,
  }) => {
    const ts = Date.now();
    const title = `e2e-steer-focus-${ts}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await waitUntilSettled(client, session.id);

    try {
      const fixture = await mockAutonomousSessionAndHoldSteer(page, session.id);

      await page.addInitScript(() => {
        localStorage.setItem("stapler-squad:onboarded", "true");
      });
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
      await fixture.ready;

      const sessionsPage = new SessionsPage(page);
      const card = sessionsPage.getSessionCard(title);
      await expect(card).toBeVisible({ timeout: 10000 });
      await card.getByRole("button", { name: /more session actions|more actions/i }).click();
      await page.getByRole("menuitem", { name: /give direction/i }).click();

      const dialog = page.getByRole("dialog", { name: "Give Direction" });
      await expect(dialog).toBeVisible();
      const input = dialog.getByRole("textbox");
      // Matches both "Send" and the in-flight "Sending…" label.
      const sendButton = dialog.getByRole("button", { name: /^send/i });
      const cancelButton = dialog.getByRole("button", { name: /^cancel$/i });

      // --- Send while pending: focus, disabled state, Tab cycling, Escape ---
      await input.fill("please run the tests");
      await input.press("Enter");

      // The RPC is now held open by mockAutonomousSessionAndHoldSteer's gate
      // (not released until the end of this test — pressing Escape below
      // dismisses the dialog but does not cancel the in-flight request, so
      // there is exactly one pending request for this test's whole body).
      // The regression this guards: an implicit auto-blur-to-document.body
      // when the input becomes disabled would leave neither the Escape
      // handler (bubbling from the dialog's own onKeyDown) nor the focus trap
      // able to see events dispatched at document.body.
      await expect(cancelButton).toBeFocused();
      await expect(sendButton).toBeDisabled();
      await expect(input).toBeDisabled();

      // Tab from Cancel (the only real focusable control while isSteering is
      // true) must not escape the dialog to document.body.
      await page.keyboard.press("Tab");
      const afterTabIsBody = await page.evaluate(() => document.activeElement === document.body);
      expect(afterTabIsBody).toBe(false);

      await page.keyboard.press("Shift+Tab");
      const afterShiftTabIsBody = await page.evaluate(() => document.activeElement === document.body);
      expect(afterShiftTabIsBody).toBe(false);

      // Escape must still close the dialog while the input/Send are disabled
      // and focus never left the dialog's DOM subtree.
      await page.keyboard.press("Escape");
      await expect(dialog).toBeHidden();

      // Release the still-pending request so it resolves cleanly in the
      // background instead of leaking past the end of the test.
      fixture.releaseSteerRequest();
      await expect(card).toBeVisible();
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });
});
