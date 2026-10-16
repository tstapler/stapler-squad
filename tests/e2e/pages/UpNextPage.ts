import { Locator, Page } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

export type UpNextTabId = "prs" | "stuck" | "worktrees" | "queue";

/** Page object for the tabbed /unfinished ("Up Next") page. */
export class UpNextPage {
  readonly page: Page;
  readonly tabList: Locator;

  constructor(page: Page) {
    this.page = page;
    this.tabList = page.getByRole("tablist", { name: "Up next sections" });
  }

  /** Clears persisted tab state (and skips onboarding) before any page script runs. */
  async resetStorage() {
    await this.page.addInitScript(() => {
      // Only on the first load of this context so a later reload keeps what the test persisted.
      if (!sessionStorage.getItem("e2e-up-next-reset")) {
        localStorage.clear();
        sessionStorage.setItem("e2e-up-next-reset", "1");
      }
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  }

  async goto(tab?: UpNextTabId) {
    const query = tab ? `?tab=${tab}` : "";
    await this.page.goto(`${BASE_URL}/unfinished${query}`, { waitUntil: "domcontentloaded" });
  }

  tab(id: UpNextTabId): Locator {
    return this.page.getByTestId(`up-next-tab-${id}`);
  }

  panel(id: UpNextTabId): Locator {
    return this.page.getByTestId(`up-next-panel-${id}`);
  }

  async selectTab(id: UpNextTabId) {
    await this.tab(id).click();
  }
}
