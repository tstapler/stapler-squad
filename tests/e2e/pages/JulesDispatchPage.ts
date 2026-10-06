import { APIRequestContext, Page, Locator, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/**
 * Page object for the "Dispatch to Jules" button (BacklogItemDetail's
 * ActionsSection, data-testid "dispatch-to-jules") and the
 * JulesDispatchDialog it opens (google-jules-integration Epic 3.2, Epic
 * 4.1's Story 4.1.3). Follows the `e2e-test-conventions` skill: data-testid/
 * ARIA-role locators only, no waitForTimeout — every wait below is an
 * explicit toBeVisible-style expectation.
 *
 * Assumes the caller already navigated to a backlog item's detail page
 * (BacklogItemDetailPage.openItemByTitle) where the item is eligible for
 * dispatch — see JulesDispatchGate in ActionsSection.tsx for the gating
 * precedence (feature off -> no key -> session already open -> no known
 * branch -> enabled) that controls whether/how the trigger button renders.
 */
export class JulesDispatchPage {
  readonly page: Page;
  readonly trigger: Locator;
  readonly triggerReason: Locator;
  readonly dialog: Locator;
  readonly branchInput: Locator;
  readonly promptInput: Locator;
  readonly egressCheckbox: Locator;
  readonly submitButton: Locator;
  readonly cancelButton: Locator;
  readonly errorBanner: Locator;

  constructor(page: Page) {
    this.page = page;
    this.trigger = page.getByTestId("dispatch-to-jules");
    this.triggerReason = page.getByTestId("dispatch-to-jules-reason");
    this.dialog = page.getByTestId("jules-dispatch-dialog");
    this.branchInput = page.getByTestId("jules-dispatch-branch");
    this.promptInput = page.getByTestId("jules-dispatch-prompt");
    this.egressCheckbox = page.getByTestId("jules-dispatch-egress-checkbox");
    this.submitButton = page.getByTestId("jules-dispatch-submit");
    this.cancelButton = page.getByTestId("jules-dispatch-cancel");
    this.errorBanner = page.getByTestId("jules-dispatch-error");
  }

  /** Clicks the "Dispatch to Jules" trigger and waits for the dialog to open. */
  async openDialog() {
    await this.trigger.click();
    await expect(this.dialog).toBeVisible();
  }

  /**
   * Opens the dialog using only keyboard input — `.focus()` (not a mouse
   * click) followed by `page.keyboard.press("Enter")` — for §7.9's
   * keyboard-only completion scenario. Only meaningful when the trigger is
   * enabled/attached; callers are responsible for that precondition.
   */
  async openDialogViaKeyboard() {
    await this.trigger.focus();
    await this.page.keyboard.press("Enter");
    await expect(this.dialog).toBeVisible();
  }

  async fillBranch(branch: string) {
    await this.branchInput.fill(branch);
  }

  async fillPrompt(prompt: string) {
    await this.promptInput.fill(prompt);
  }

  /** Checks the cloud-egress confirmation box (only present for an
   * unacknowledged repo — JulesDispatchDialog's egressAcknowledged prop). */
  async acknowledgeEgress() {
    await this.egressCheckbox.check();
  }

  dispatchButton(): Locator {
    return this.submitButton;
  }
}

/**
 * Drives Jules feature config through the real GetJulesConfig/UpdateJulesConfig
 * RPCs on the isolated test server (session.v1.SessionService), per Task
 * 4.1.3b — never by editing config.json directly. `apiKey` (when provided)
 * is a stub value only ever seen by this RPC and the OS keychain the test
 * server writes to; no real Jules HTTP call is made by either e2e scenario.
 */
export async function updateJulesConfigDirect(
  request: APIRequestContext,
  opts: { enabled: boolean; apiKey?: string }
): Promise<void> {
  const resp = await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateJulesConfig`, {
    headers: { "Content-Type": "application/json" },
    data: {
      enabled: opts.enabled,
      apiKey: opts.apiKey ?? "",
    },
  });
  if (!resp.ok()) {
    throw new Error(`updateJulesConfigDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
}

/**
 * Calls the real `ConfirmEgressConsent` RPC (session_service.go) — a pure
 * config write (adds repoPath to `JulesConfig.EgressAcknowledgedRepos`), no
 * Jules API call involved — so a test can pre-acknowledge a repo the way
 * JulesDispatchDialog's own checkbox does, without driving that UI first.
 */
export async function confirmEgressConsentDirect(request: APIRequestContext, repoPath: string): Promise<void> {
  const resp = await request.post(`${BASE_URL}/api/session.v1.SessionService/ConfirmEgressConsent`, {
    headers: { "Content-Type": "application/json" },
    data: { repoPath },
  });
  if (!resp.ok()) {
    throw new Error(`confirmEgressConsentDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
}

/**
 * Calls the real `RevokeEgressConsent` RPC (jules_config_service.go) — the
 * removal counterpart to ConfirmEgressConsent above, mirroring how
 * JulesSettings.tsx's "Revoke" button invokes it. Idempotent: revoking a
 * repo that isn't acknowledged is a no-op, not an error.
 */
export async function revokeEgressConsentDirect(request: APIRequestContext, repoPath: string): Promise<void> {
  const resp = await request.post(`${BASE_URL}/api/session.v1.SessionService/RevokeEgressConsent`, {
    headers: { "Content-Type": "application/json" },
    data: { repoPath },
  });
  if (!resp.ok()) {
    throw new Error(`revokeEgressConsentDirect failed (${resp.status()}): ${await resp.text().catch(() => "")}`);
  }
}

/**
 * §7.2's "happy path" scenario needs a real `DispatchToJules` click to
 * observably produce a new session row — but the real RPC's guard chain
 * (jules_dispatch_service.go) calls the real Jules `ListSources` endpoint
 * before it ever reserves an ItemSession, and no e2e-local API key can
 * authenticate against Google's actual Jules API (verified: a stubbed key
 * gets a real 401 from https://jules.googleapis.com). Making that call
 * genuinely succeed would need a live Jules account and a real
 * GitHub-App-connected repo — inappropriate for an automated test.
 *
 * Instead, this intercepts only the one browser-to-our-backend
 * `DispatchToJules` POST and, in its place, creates the exact ItemSession row
 * a successful dispatch would have produced via the debug-only
 * seed-jules-work-session endpoint's `itemId`-attach mode (real storage
 * write, on the real item). Everything downstream of that write — including
 * the real WatchBacklogItems live-update path — runs completely unmocked, so
 * the new row the test asserts on is observed the same way a genuine
 * dispatch's row would be, not injected client-side. The dialog's own
 * `handleDispatch` never reads the RPC response body, so an empty `{}`
 * fulfillment is a faithful stand-in for the real (single-field, rarely-used)
 * `DispatchToJulesResponse`.
 */
export async function interceptDispatchToJulesWithSeededSession(
  page: Page,
  request: APIRequestContext,
  itemId: string
): Promise<void> {
  await page.route(`${BASE_URL}/api/session.v1.BacklogService/DispatchToJules`, async (route) => {
    const seedResp = await request.post(`${BASE_URL}/api/debug/backlog/seed-jules-work-session`, {
      headers: { "Content-Type": "application/json" },
      data: { itemId },
    });
    if (!seedResp.ok()) {
      await route.abort("failed");
      throw new Error(
        `interceptDispatchToJulesWithSeededSession: seeding failed (${seedResp.status()}): ${await seedResp
          .text()
          .catch(() => "")}`
      );
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
  });
}
