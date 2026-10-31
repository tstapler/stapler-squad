import type { Locator, Page } from "@playwright/test";
import { expect } from "../helpers/backlog-settings-mocks";
import { baseUrl } from '../helpers/base-url';

const BASE_URL = baseUrl();

export type Scheme = "Light" | "Dark";

/**
 * Drives the real theme control: the ThemePicker radio group on /settings. The picker has no
 * data-testid, so it is located by role and name. The app applies a theme as a vanilla-extract
 * class on <html> (ThemeContext), not as `data-theme`, so the proof that the theme changed is a
 * different <html> class list plus `aria-checked="true"` on the chosen radio.
 */
export class ThemeToggle {
  readonly page: Page;
  readonly group: Locator;

  constructor(page: Page) {
    this.page = page;
    this.group = page.getByRole("radiogroup", { name: "Theme selection" });
  }

  private htmlClasses(): Promise<string> {
    return this.page.evaluate(() => document.documentElement.className);
  }

  /** Selects Light or Dark on /settings; the choice persists for later navigations in this context. */
  async select(scheme: Scheme): Promise<void> {
    // The picker lives in the Appearance tab of /settings.
    await this.page.goto(`${BASE_URL}/settings?tab=appearance`, { waitUntil: "domcontentloaded" });
    const radio = this.group.getByRole("radio", { name: new RegExp(`^${scheme}`) });
    await expect(radio).toBeVisible();
    if ((await radio.getAttribute("aria-checked")) === "true") return;
    const before = await this.htmlClasses();
    await radio.click();
    await expect(radio).toHaveAttribute("aria-checked", "true");
    await expect.poll(() => this.htmlClasses()).not.toBe(before);
  }
}
