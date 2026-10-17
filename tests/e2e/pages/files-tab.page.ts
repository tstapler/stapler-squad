import { Page, Locator } from "@playwright/test";

/**
 * Page object for the Files viewer spike pages (web-app/src/app/spike-files, THROWAWAY) and,
 * later, the Files tab. Locators use data-testid or ARIA roles only. Reads that need live
 * CodeMirror state (scrollTop, undo depth) go through the page's `window.__spike.api`.
 */
/** Marks first-run onboarding done before any page script runs, so its modal never covers the page. */
export async function suppressOnboarding(page: Page): Promise<void> {
  await page.addInitScript(() => localStorage.setItem("stapler-squad:onboarded", "true"));
}

export class FilesTabPage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  async gotoSpike(variant: "input" | "stateswap" | "a11y") {
    await suppressOnboarding(this.page);
    await this.page.goto(`/spike-files/?v=${variant}`);
    await this.page.getByTestId(`spike-variant-${variant}`).waitFor();
  }

  tab(which: "a" | "b"): Locator {
    return this.page.getByTestId(`spike-tab-${which}`);
  }

  get editor(): Locator {
    return this.page.getByTestId("spike-editor");
  }

  /** Calls a function on the spike page's window.__spike.api and returns its JSON result. */
  async api<T>(fn: string, ...args: unknown[]): Promise<T> {
    return this.page.evaluate(
      async ([name, a]) => {
        const api = (window as unknown as { __spike: { api: Record<string, (...x: unknown[]) => unknown> } }).__spike.api;
        const out = api[name as string](...(a as unknown[]));
        // Let CodeMirror measure and settle scroll before the caller reads layout.
        await new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r())));
        return out;
      },
      [fn, args] as const
    ) as Promise<T>;
  }
}
