import { Page, expect } from "@playwright/test";
import { dismissOnboardingIfPresent } from "./OnboardingPage";

/**
 * Page object for Settings -> Remotes (web-app/src/app/settings/remotes/page.tsx) and its
 * Add Remote form / host-key trust dialog (ssh-remote-workspaces Phase 6 Epic 6.1). Follows
 * .claude/rules/e2e-test-conventions.md: data-testid/ARIA-role locators only, no
 * waitForTimeout -- every wait below is an explicit toBeVisible/toHaveText-style expectation.
 */
export class RemotesSettingsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto("/settings/remotes", {
      waitUntil: "domcontentloaded",
      timeout: 15000,
    });
    await dismissOnboardingIfPresent(this.page);
  }

  async clickAddRemote() {
    await this.page.getByTestId("remotes-add-button").click();
    await expect(this.page.getByTestId("add-remote-form")).toBeVisible();
  }

  /** Fills the Add Remote form's Name/Host/User/Port/Base path fields. */
  async fillRemoteForm(remote: { name: string; host: string; user: string; port?: number; basePath: string }) {
    await this.page.getByTestId("add-remote-name").fill(remote.name);
    await this.page.getByTestId("add-remote-host").fill(remote.host);
    await this.page.getByTestId("add-remote-user").fill(remote.user);
    if (remote.port) {
      await this.page.getByTestId("add-remote-port").fill(String(remote.port));
    }
    await this.page.getByTestId("add-remote-base-path").fill(remote.basePath);
  }

  /** Submits the form (generates an identity + calls TestRemoteConnection). */
  async submitTestConnection() {
    await this.page.getByTestId("add-remote-submit").click();
  }

  /** Waits for the generated-identity authorized_keys block to render (proves
   * GenerateRemoteIdentity round-tripped for real). */
  async waitForIdentityGenerated() {
    await expect(this.page.getByTestId("add-remote-authorized-keys")).toBeVisible({ timeout: 10000 });
  }

  /** Waits for the host-key trust dialog (host_key_unknown=true response) and confirms it. */
  async trustHostKey() {
    await expect(this.page.getByTestId("host-key-trust-overlay")).toBeVisible({ timeout: 10000 });
    await this.page.getByTestId("host-key-trust-confirm").click();
  }

  /** Row-level "Test" button for an already-saved remote. */
  async testSavedRemote(name: string) {
    await this.page.getByTestId(`remote-test-${name}`).click({ timeout: 20000 });
  }

  async expectStatus(name: string, text: string | RegExp) {
    await expect(this.page.getByTestId(`remote-status-${name}`)).toHaveText(text, { timeout: 15000 });
  }

  async expectRemoteRowVisible(name: string) {
    await expect(this.page.getByTestId(`remote-row-${name}`)).toBeVisible({ timeout: 15000 });
  }

  async deleteRemote(name: string) {
    await this.page.getByTestId(`remote-delete-${name}`).click();
    await this.page.getByTestId(`remote-confirm-delete-${name}`).click();
    await expect(this.page.getByTestId(`remote-row-${name}`)).not.toBeVisible();
  }
}
