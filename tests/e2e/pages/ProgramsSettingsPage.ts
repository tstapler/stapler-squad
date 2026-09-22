import { Page, Locator, expect } from "@playwright/test";

/** Settings > General > Program Configurations form (locators: data-testid / ARIA only). */
export class ProgramsSettingsPage {
  constructor(private page: Page) {}

  get form(): Locator {
    return this.page.getByTestId("program-form");
  }
  get commandInput(): Locator {
    return this.page.getByTestId("prog-command-input");
  }
  get checkButton(): Locator {
    return this.page.getByTestId("prog-command-check");
  }
  get status(): Locator {
    return this.page.getByTestId("prog-command-status");
  }
  get flagsInput(): Locator {
    return this.page.getByTestId("prog-flags-input");
  }
  get flagsWarning(): Locator {
    return this.page.getByTestId("prog-flags-warning");
  }
  get availableFlagsToggle(): Locator {
    return this.page.getByTestId("prog-available-flags-toggle");
  }
  get flagInfoButtons(): Locator {
    return this.page.getByTestId("prog-flag-info-button");
  }
  get saveButton(): Locator {
    return this.page.getByTestId("save-program-btn");
  }

  async gotoAndOpenForm() {
    // The first-run tour overlays and dims the page, which skews contrast checks.
    await this.page.addInitScript(() => localStorage.setItem("stapler-squad:onboarded", "true"));
    await this.page.goto("/settings?tab=general", { waitUntil: "domcontentloaded", timeout: 15000 });
    await this.page.getByTestId("add-program-btn").click();
    await expect(this.form).toBeVisible();
  }

  /** Types the command and moves focus away, which runs the implicit (blur) probe. */
  async enterCommandAndBlur(command: string) {
    await this.commandInput.fill(command);
    await this.flagsInput.focus();
  }

  /** The explicit Check click, the only action allowed to execute a script. */
  async check() {
    await this.checkButton.click();
  }

  async checkFixtureAndAwaitFlags(command: string) {
    await this.enterCommandAndBlur(command);
    await expect(this.status).toContainText("Not checked for flags yet");
    await this.check();
    await expect(this.status).toContainText("flags detected");
  }

  async horizontalOverflow(): Promise<boolean> {
    return this.page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  }
}
