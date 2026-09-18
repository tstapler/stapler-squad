// @feature jules-settings
/**
 * E2E coverage for the Jules Settings panel (google-jules-integration Epic
 * 3.1) -- the two scenarios validation.md's "UX Acceptance Tests" table
 * promised for this file (project_plans/google-jules-integration/
 * implementation/validation.md, §7.1 and §7.14) but that were never
 * actually written, found during a spec-compliance sweep:
 *
 *   1. §7.1 -- the whole "add a key, enable, confirm Test connection
 *      succeeds" round trip completes in the 4 actions ux.md §7 commits to
 *      (open Settings -> Jules, paste key + Save, toggle Enable, Test
 *      connection). A real TestJulesConnection success requires a repo
 *      actually registered as a Jules source with Google's API, which this
 *      test environment has no way to fake server-side -- so, like
 *      slack-notifications.spec.ts does for TestSlackWebhook, the RPC
 *      response is intercepted at the network layer (page.route) rather
 *      than reaching a real 3rd-party service.
 *   2. §7.14 -- after a REAL (unmocked) Save, neither the rendered DOM nor
 *      the GetJulesConfig/UpdateJulesConfig response bodies contain any
 *      substring of the key that was just saved.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * global-setup.ts starts exactly one server for the whole Playwright run,
 * shared across every spec file (see slack-notifications.spec.ts's header
 * for the same caveat) -- both tests below clear the key and disable Jules
 * again afterward so they don't leak "configured" state into specs that run
 * later in the same worker.
 */

import { test, expect } from "@playwright/test";
import { JulesSettingsPage } from "./pages/JulesSettingsPage";
import { updateJulesConfigDirect } from "./pages/JulesDispatchPage";
import { dismissOnboardingIfPresent } from "./pages/OnboardingPage";

test.describe("jules settings", () => {
  test.afterEach(async ({ request }) => {
    await updateJulesConfigDirect(request, { enabled: false, apiKey: "__clear__" });
  });

  test("configures Jules — key, enable, test connection — in 4 actions", async ({ page }) => {
    // TestJulesConnection's success path needs a repo Jules itself
    // recognizes as a registered source -- stub the response so the test
    // exercises the UI's own success rendering, not Google's API.
    await page.route("**/api/session.v1.SessionService/TestJulesConnection", (route) =>
      route.fulfill({ json: { ok: true, message: "" } }),
    );

    const settingsPage = new JulesSettingsPage(page);
    let actionCount = 0;

    // Action 1: open Settings -> Jules.
    await page.goto("/settings?tab=general", { waitUntil: "domcontentloaded" });
    await dismissOnboardingIfPresent(page);
    await page.getByTestId("settings-jules-link").click();
    await expect(settingsPage.apiKeyInput).toBeVisible();
    actionCount++;

    // Action 2: paste key + Save (one bundled action per ux.md §7.1).
    await settingsPage.apiKeyInput.fill("AIzaSyE2E-SETTINGS-ROUNDTRIP-STUB");
    await settingsPage.saveButton.click();
    await expect(settingsPage.saveStatus).toBeVisible();
    actionCount++;

    // Action 3: toggle Enable.
    await settingsPage.enabledCheckbox.check();
    actionCount++;

    // Action 4: Test connection.
    await settingsPage.testRepoPathInput.fill("/home/e2e/code/github.com/tstapler/stapler-squad");
    await settingsPage.testConnectionButton.click();
    actionCount++;

    await expect(settingsPage.testConnectionResult).toBeVisible();
    await expect(settingsPage.testConnectionResult).toHaveAttribute("role", "status");
    await expect(settingsPage.testConnectionResult).toHaveText(
      "Connected — this repo is reachable from Jules.",
    );

    expect(actionCount).toBe(4);
  });

  test("no key substring appears in the DOM or the GetJulesConfig network response after saving", async ({
    page,
  }) => {
    const SECRET_KEY = "AIzaSyE2E-SECRET-DO-NOT-LEAK-998877";

    const settingsPage = new JulesSettingsPage(page);
    await settingsPage.goto();
    await dismissOnboardingIfPresent(page);
    await expect(settingsPage.apiKeyInput).toBeVisible();

    await settingsPage.apiKeyInput.fill(SECRET_KEY);

    // Register these before the click so neither response can resolve
    // (and be missed) before the listener is armed.
    const updateConfigResponse = page.waitForResponse((r) =>
      r.url().includes("/session.v1.SessionService/UpdateJulesConfig"),
    );
    const getConfigResponse = page.waitForResponse((r) =>
      r.url().includes("/session.v1.SessionService/GetJulesConfig"),
    );

    await settingsPage.saveButton.click();
    const [updateResp, getResp] = await Promise.all([updateConfigResponse, getConfigResponse]);

    // Confirms the save cycle (including the post-save reload) has fully
    // settled before inspecting the DOM below.
    await expect(settingsPage.saveStatus).toBeVisible();

    const [updateBody, getBody, domContent] = await Promise.all([
      updateResp.text(),
      getResp.text(),
      page.content(),
    ]);

    expect(updateBody).not.toContain(SECRET_KEY);
    expect(getBody).not.toContain(SECRET_KEY);
    expect(domContent).not.toContain(SECRET_KEY);
  });
});
