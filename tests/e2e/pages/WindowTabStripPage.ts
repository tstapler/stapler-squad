import { Page, Locator, expect } from '@playwright/test';

/**
 * Page object for the `WindowTabStrip` (role="tablist" aria-label="Window
 * switcher") — see web-app/src/components/window/WindowTabStrip.tsx for the
 * underlying markup. Renders one `role="tab"` button per window (visible
 * text == accessible name == window name) plus a trailing "New window"
 * button; a "×" close button per tab is only rendered once 2+ windows exist.
 */
export class WindowTabStripPage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The `role="tablist"` container itself. */
  get tabList(): Locator {
    return this.page.getByRole('tablist', { name: 'Window switcher' });
  }

  /** All window tabs, in window order. */
  get tabs(): Locator {
    return this.tabList.getByRole('tab');
  }

  /** Wait for the tab strip to be rendered at all. */
  async waitForLoaded(): Promise<void> {
    await expect(this.tabList).toBeVisible({ timeout: 15000 });
  }

  /** The tab button for a window by its current display name. */
  getTab(name: string): Locator {
    return this.tabList.getByRole('tab', { name });
  }

  /** The tab currently marked `aria-selected="true"`. */
  getActiveTab(): Locator {
    return this.tabList.locator('[role="tab"][aria-selected="true"]');
  }

  /** The Nth tab in window order (0-indexed) — for identifying a tab before/after a rename changes its name. */
  getTabByIndex(index: number): Locator {
    return this.tabs.nth(index);
  }

  /** Click a window's tab to switch to it. */
  async switchToTab(name: string): Promise<void> {
    await this.getTab(name).click();
  }

  /** Click the "+" button to create a new window (per page.tsx wiring, this also switches to it). */
  async createWindow(): Promise<void> {
    await this.page.getByRole('button', { name: 'New window' }).click();
  }

  /** Click the "×" close button for a window's tab (only present once 2+ windows exist). */
  async closeTab(name: string): Promise<void> {
    await this.page.getByRole('button', { name: `Close ${name}` }).click();
  }

  /**
   * Double-click a tab to enter inline-rename mode, replace its text, and
   * commit with Enter — mirrors WindowTabStrip.tsx's WindowTabEditor.
   */
  async renameTab(oldName: string, newName: string): Promise<void> {
    await this.getTab(oldName).dblclick();
    const input = this.tabList.getByRole('textbox');
    await expect(input).toBeVisible();
    await input.fill(newName);
    await input.press('Enter');
  }

  /** Parses the `?window=<id>` param off a page URL (e.g. `page.url()`). */
  static windowIdFromUrl(url: string): string | null {
    return new URL(url).searchParams.get('window');
  }
}
