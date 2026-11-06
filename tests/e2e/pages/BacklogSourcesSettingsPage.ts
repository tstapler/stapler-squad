import { Page, Locator, expect } from "@playwright/test";
import { baseUrl } from '../helpers/base-url';

export class BacklogSourcesSettingsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto(
      baseUrl() + "/settings/backlog-sources",
      { waitUntil: "domcontentloaded", timeout: 15000 }
    );
  }

  // `token` is accepted for call-site compatibility but unused: GitHub sources authenticate via
  // the shared GitHub-accounts keychain, so the form has no token input (backlogSourceSchemas.ts).
  async addSource(opts: { displayName: string; owner: string; repo: string; token?: string }) {
    await this.page.getByPlaceholder("Display name (e.g. My Repo Issues)").fill(opts.displayName);
    await this.page.getByPlaceholder("Owner (e.g. acme)").fill(opts.owner);
    await this.page.getByPlaceholder("Repo (e.g. widgets)").fill(opts.repo);
    await this.page.getByRole("button", { name: "Add Source" }).click();
  }

  /** Removes every source whose display name starts with `prefix`, so a test run leaves no rows behind. */
  async deleteSourcesWithPrefix(prefix: string) {
    // Wait for the list RPC to settle (a row, or the empty message), else count() can read 0 pre-load.
    await expect(
      this.page.locator('[data-testid^="source-row-"]').or(this.page.getByText("No sources configured.")).first(),
    ).toBeVisible();
    const buttons = this.page.getByRole("button", { name: new RegExp(`^Remove ${prefix}`) });
    while ((await buttons.count()) > 0) {
      const before = await buttons.count();
      await buttons.first().click();
      await expect(buttons).toHaveCount(before - 1);
    }
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
    const toggle = this.row(displayName).getByRole("switch", { name: /closing GitHub issues/ });
    await toggle.click();
    // Wait for the RPC round-trip: the row ignores clicks on a toggle while one is in flight.
    await expect(toggle).toHaveAttribute("aria-checked", "true");
  }

  /**
   * Clicks the "Reflect GitHub status back here" (backward sync) toggle
   * within a specific source's row (Epic 4.3, backlog-github-two-way-sync).
   * Note: the confirm-with-preview gate (Epic 4.4, BackwardSyncConfirmDialog)
   * ships in this same PR and IS wired up here — this helper's fixture
   * source just has zero linked items, so PreviewBackwardSyncImpact returns
   * 0 and the dialog auto-skips, letting the toggle flip immediately. It is
   * not evidence the gate is absent.
   */
  async enableBackwardSync(displayName: string) {
    // The real preview RPC queries api.github.com for the (fake) owner/repo and returns a 404 error,
    // which blocks the toggle. Stub a zero-impact preview so the test makes no network call.
    await this.page.route("**/*.BacklogService/PreviewBackwardSyncImpact", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ itemCount: 0, sampleTitles: [] }) }),
    );
    const toggle = this.row(displayName).getByRole("switch", { name: /reflecting GitHub status back/ });
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-checked", "true");
  }
}
