import { Page, Locator, expect } from '@playwright/test';

/**
 * Page object for the `WindowTabStrip` (role="tablist" aria-label="Window
 * switcher") — see web-app/src/components/window/WindowTabStrip.tsx for the
 * underlying markup. Renders one `role="tab"` button per window (visible
 * text == accessible name == window name) plus a trailing "New window"
 * button; a "×" close button per tab is only rendered once 2+ windows exist.
 */
/** localStorage key `useWindowManager`/`windowPersistence.ts` persist windows under. */
const WINDOW_LAYOUT_KEY = 'cockpit.windowLayout';

/**
 * Runs inside the page via `page.evaluate` — must be fully self-contained
 * (Playwright ships only this function's own source into the browser
 * context, not any module-scope helpers it might otherwise close over) —
 * reads window `id`'s persisted pane-leaf count (see `PaneNode` in
 * paneTypes.ts) out of the `key` localStorage entry.
 */
function readPersistedPaneLeafCountInBrowser({ key, id }: { key: string; id: string }): number {
  function countLeaves(node: unknown): number {
    if (!node || typeof node !== 'object') return 0;
    const n = node as { type?: string; first?: unknown; second?: unknown };
    if (n.type === 'leaf') return 1;
    if (n.type !== 'split') return 0;
    return countLeaves(n.first) + countLeaves(n.second);
  }
  const raw = localStorage.getItem(key);
  if (!raw) return 0;
  try {
    const parsed = JSON.parse(raw) as { windows?: Array<{ id: string; paneState?: { root?: unknown } }> };
    const win = (parsed.windows ?? []).find((w) => w.id === id);
    if (!win || !win.paneState) return 0;
    return countLeaves(win.paneState.root);
  } catch {
    return 0;
  }
}

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

  /**
   * Wait for the tab strip to be rendered AND for `useWindowUrlSync`'s
   * self-healing `router.replace` to land the `?window=` param in the URL.
   * That param write happens in a `useEffect` after the tablist's first
   * paint, so waiting on tablist visibility alone races the URL update —
   * reading `page.url()` immediately after can observe a still-missing
   * param even though the tab strip is already interactive.
   */
  async waitForLoaded(): Promise<void> {
    await expect(this.tabList).toBeVisible({ timeout: 15000 });
    await expect
      .poll(() => WindowTabStripPage.windowIdFromUrl(this.page.url()), { timeout: 15000 })
      .not.toBeNull();
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

  /** All rendered pane leaves in the currently-viewed window (see PaneSplitRenderer.tsx's `pane-leaf-<id>` testid). */
  get paneLeaves(): Locator {
    return this.page.locator('[data-testid^="pane-leaf-"]');
  }

  /** The sorted set of `pane-leaf-<id>` testids currently rendered — a fingerprint of the pane tree's identity. */
  async getPaneLeafIds(): Promise<(string | null)[]> {
    return this.paneLeaves.evaluateAll((els) => els.map((el) => el.getAttribute('data-testid')).sort());
  }

  /**
   * Split the first rendered pane side-by-side via its header's "Split pane
   * side by side" button and wait for the resulting extra leaf to render.
   * Every window (including a freshly created one) already starts with 2
   * leaves — initialPaneState()'s default session-list/session-detail split
   * — so this asserts one more than whatever was there before, not a fixed 2.
   */
  async splitFirstPaneVertically(): Promise<void> {
    const before = await this.paneLeaves.count();
    await this.page.getByTestId('pane-split-vertical-btn').first().click();
    await expect(this.paneLeaves).toHaveCount(before + 1);
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

  /**
   * Wait for `useWindowManager`'s debounced (~300ms) localStorage save to
   * actually persist `count` windows, not just for React state/the DOM to
   * reflect `count` tabs. A second real browser tab reads `cockpit.windowLayout`
   * from localStorage independently on its own mount — if it loads faster
   * than the debounce window, it sees a stale (fewer-window) snapshot and
   * can resolve to the wrong window. A real user opening a second tab takes
   * far longer than 300ms, but an automated test easily beats it — this
   * makes the test wait for the same real-world precondition instead of
   * assuming DOM-visible state implies persisted state.
   */
  async waitForPersistedWindowCount(count: number): Promise<void> {
    await expect
      .poll(
        () =>
          this.page.evaluate((key: string) => {
            const raw = localStorage.getItem(key);
            if (!raw) return 0;
            try {
              return (JSON.parse(raw).windows ?? []).length;
            } catch {
              return 0;
            }
          }, WINDOW_LAYOUT_KEY),
        { timeout: 5000 }
      )
      .toBe(count);
  }

  /**
   * Wait for a specific window's persisted pane tree to actually contain
   * `count` leaf panes — the pane-split counterpart to
   * waitForPersistedWindowCount above. A pane split (e.g. via the "Split
   * pane" buttons) goes through the same debounced (~300ms) localStorage
   * save as a window create/rename, so reading a second real tab's copied
   * URL immediately after building a layout can otherwise race a stale,
   * pre-split snapshot.
   */
  async waitForPersistedPaneLeafCount(windowId: string, count: number): Promise<void> {
    await expect
      .poll(
        () => this.page.evaluate(readPersistedPaneLeafCountInBrowser, { key: WINDOW_LAYOUT_KEY, id: windowId }),
        { timeout: 5000 }
      )
      .toBe(count);
  }
}
