import { Page, Locator } from "@playwright/test";

export class BacklogSourcesSettingsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto(
      (process.env.TEST_SERVER_URL ?? "http://localhost:8544") + "/settings/backlog-sources",
      { waitUntil: "domcontentloaded", timeout: 15000 }
    );
  }

  async addSource(opts: { displayName: string; owner: string; repo: string; token: string }) {
    await this.page.getByPlaceholder("Display name (e.g. My Repo Issues)").fill(opts.displayName);
    await this.page.getByPlaceholder("Owner (e.g. acme)").fill(opts.owner);
    await this.page.getByPlaceholder("Repo (e.g. widgets)").fill(opts.repo);
    await this.page.getByPlaceholder("GitHub personal access token").fill(opts.token);
    await this.page.getByRole("button", { name: "Add Source" }).click();
  }

  /**
   * Scopes a locator to the row for a specific source by display name, so
   * assertions/clicks are unambiguous even when multiple sources are present
   * (e.g. left over from a retried test in the same suite run).
   */
  row(displayName: string): Locator {
    return this.page.locator('[data-testid^="source-row-"]').filter({ hasText: displayName });
  }

  /**
   * Clicks the "Close GitHub issues when I finish here" (forward sync)
   * toggle within a specific source's row (Epic 4.3, backlog-github-two-way-sync).
   */
  async enableForwardSync(displayName: string) {
    await this.row(displayName).getByRole("switch", { name: /closing GitHub issues/ }).click();
  }

  /**
   * Clicks the "Reflect GitHub status back here" (backward sync) toggle
   * within a specific source's row (Epic 4.3, backlog-github-two-way-sync).
   * Note: as of this wave, enabling backward sync flips the toggle
   * immediately — the confirm-with-preview gate (Epic 4.4) is a later wave.
   */
  async enableBackwardSync(displayName: string) {
    await this.row(displayName).getByRole("switch", { name: /reflecting GitHub status back/ }).click();
  }
}
