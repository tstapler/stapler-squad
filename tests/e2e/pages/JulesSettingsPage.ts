import { Page, Locator } from "@playwright/test";

/**
 * Page object for the Jules Settings panel (`/settings/jules`,
 * `JulesSettings.tsx`, google-jules-integration Epic 3.1). Follows the
 * `e2e-test-conventions` skill: data-testid/ARIA-role locators only, no
 * waitForTimeout.
 */
export class JulesSettingsPage {
  readonly page: Page;
  readonly enabledCheckbox: Locator;
  readonly apiKeyInput: Locator;
  readonly saveButton: Locator;
  readonly saveStatus: Locator;
  readonly testRepoPathInput: Locator;
  readonly testConnectionButton: Locator;
  readonly testConnectionResult: Locator;

  constructor(page: Page) {
    this.page = page;
    this.enabledCheckbox = page.getByLabel("Enable Jules integration");
    this.apiKeyInput = page.getByLabel("API key");
    this.saveButton = page.getByRole("button", { name: "Save" });
    // The save-result message (JulesSettings.tsx's saveStatusMessage span)
    // has no data-testid of its own -- distinguish it from the separate
    // "Test connection" result region (which does have one) by its text.
    this.saveStatus = page.getByRole("status").filter({ hasText: /^(Key saved\.|Saved\.)$/ });
    this.testRepoPathInput = page.getByLabel(/Test connection/);
    this.testConnectionButton = page.getByRole("button", { name: "Test connection" });
    this.testConnectionResult = page.getByTestId("jules-test-connection-result");
  }

  /** Direct navigation to /settings/jules. */
  async goto() {
    await this.page.goto("/settings/jules", { waitUntil: "domcontentloaded", timeout: 15000 });
  }

  /**
   * Navigates the way a real user does per ux.md §7.1's "open Settings ->
   * Jules" step: land on the Settings hub, then follow the "Jules (Google
   * cloud agent)" link (data-testid "settings-jules-link") from its General
   * tab.
   */
  async gotoViaSettingsHub() {
    await this.page.goto("/settings?tab=general", { waitUntil: "domcontentloaded", timeout: 15000 });
    await this.page.getByTestId("settings-jules-link").click();
  }
}
