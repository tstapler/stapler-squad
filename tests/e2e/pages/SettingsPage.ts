import { Page, expect } from "@playwright/test";

export class SettingsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto(
      (process.env.BASE_URL ?? "http://localhost:8544") + "/settings?tab=general",
      { waitUntil: "domcontentloaded", timeout: 15000 }
    );
  }

  async selectTab(tab: string) {
    await this.page.getByRole("tab", { name: tab }).click();
  }

  async clickNewAlias() {
    await this.page.getByRole("button", { name: "New Alias" }).click();
  }

  async fillAliasName(name: string) {
    await this.page.getByLabel("Name").fill(name);
  }

  async fillAliasDescription(description: string) {
    await this.page.getByLabel("Description").fill(description);
  }

  async saveAlias() {
    await this.page.getByRole("button", { name: "Save" }).click();
    await expect(this.page.getByText(/saved/i)).toBeVisible({ timeout: 5000 });
  }

  async clickEditAlias(name: string) {
    await this.page
      .getByTestId(`alias-row-${name}`)
      .getByRole("button", { name: "Edit" })
      .click();
  }

  async deleteAlias(name: string) {
    await this.page.getByTestId(`alias-delete-${name}`).click();
    await this.page.getByTestId(`alias-confirm-delete-${name}`).click();
  }
}
