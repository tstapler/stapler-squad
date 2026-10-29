import { Page, expect } from "@playwright/test";

export class SettingsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto("/settings?tab=general", {
      waitUntil: "domcontentloaded",
      timeout: 15000,
    });
  }

  async selectTab(tab: string) {
    await this.page.getByRole("tab", { name: tab }).click();
  }

  async clickNewAlias() {
    await this.page.getByRole("button", { name: "New Alias" }).click();
  }

  async clickAliasFormSave() {
    await this.aliasFormSave().click();
  }

  async fillAliasName(name: string) {
    await this.page.getByRole("textbox", { name: /^Name(?! prefix)/ }).fill(name);
  }

  async fillAliasDescription(description: string) {
    await this.page.getByLabel("Description").fill(description);
  }

  /** The alias form's Save button; the General tab behind the form has its own Save. */
  private aliasFormSave() {
    return this.page.getByLabel(/^(New|Edit) Alias/).getByRole("button", { name: "Save" });
  }

  async saveAlias() {
    await this.aliasFormSave().click();
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
