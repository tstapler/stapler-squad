import { expect, Locator, Page } from "@playwright/test";

/**
 * Page-object helper for the Escape Analytics page's tab toggle (per-session vs.
 * all-sessions) and the "All Sessions" per-session breakdown table. Covers only
 * the ARIA-tab and breakdown-table surfaces added in escape-analytics-global-view
 * (Epic 2) — the per-session view's histogram/event-table surfaces predate this
 * feature and are out of scope here.
 */
export class EscapeAnalyticsGlobalViewPage {
  constructor(private readonly page: Page) {}

  async goto(baseUrl: string): Promise<void> {
    await this.page.goto(`${baseUrl}/analytics/escape`, { waitUntil: "domcontentloaded" });
  }

  getTab(mode: "per_session" | "all_sessions"): Locator {
    return this.page.getByTestId(`tab-${mode}`);
  }

  async activateTab(mode: "per_session" | "all_sessions"): Promise<void> {
    await this.getTab(mode).click();
  }

  /** Presses ArrowLeft/ArrowRight on the currently-focused tab button. */
  async pressArrowKey(key: "ArrowLeft" | "ArrowRight"): Promise<void> {
    await this.page.keyboard.press(key);
  }

  getPerSessionPanel(): Locator {
    return this.page.getByRole("tabpanel", { name: "" }).locator("#tabpanel-per_session");
  }

  getPanel(mode: "per_session" | "all_sessions"): Locator {
    return this.page.locator(`#tabpanel-${mode}`);
  }

  /**
   * Excludes Next.js's built-in `#__next-route-announcer__`, which also carries
   * `role="alert"` for screen-reader route-change announcements.
   */
  getErrorBanner(): Locator {
    return this.page.locator('[role="alert"]:not(#__next-route-announcer__)');
  }

  /**
   * The fleet-wide aggregate `MangleRateIndicator` renders before the per-session
   * breakdown table's row instances, which share the same `data-testid` (the
   * component is reused as-is; only this view combines multiple instances on one
   * tab) — `.first()` selects the aggregate one.
   */
  getAggregateMangleRateValue(): Locator {
    return this.getPanel("all_sessions").getByTestId("mangle-rate-value").first();
  }

  /** Same duplicate-testid rationale as {@link getAggregateMangleRateValue}. */
  getAggregateMangleCounts(): Locator {
    return this.getPanel("all_sessions").getByTestId("mangle-counts").first();
  }

  getBreakdownTable(): Locator {
    return this.page.getByTestId("session-escape-breakdown-table");
  }

  getBreakdownRows(): Locator {
    return this.page.getByTestId("session-escape-breakdown-row");
  }

  getSortButton(column: "sessionId" | "totalSequences" | "totalMangled" | "mangleRate"): Locator {
    return this.page.getByTestId(`sort-button-${column}`);
  }

  async sortBy(column: "sessionId" | "totalSequences" | "totalMangled" | "mangleRate"): Promise<void> {
    await this.getSortButton(column).click();
  }

  /** Returns the sessionId cell text for every visible breakdown row, in DOM order. */
  async getRowSessionIds(): Promise<string[]> {
    const rows = this.getBreakdownRows();
    const count = await rows.count();
    const ids: string[] = [];
    for (let i = 0; i < count; i++) {
      const text = await rows.nth(i).locator("td").first().innerText();
      ids.push(text.replace(/^Outlier:\s*/, "").trim());
    }
    return ids;
  }

  async expectTabActive(mode: "per_session" | "all_sessions"): Promise<void> {
    await expect(this.getTab(mode)).toHaveAttribute("aria-selected", "true");
  }
}
