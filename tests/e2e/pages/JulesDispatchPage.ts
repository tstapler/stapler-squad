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
