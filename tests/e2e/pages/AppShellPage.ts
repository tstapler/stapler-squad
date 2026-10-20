import { Locator, Page } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

export interface SkipLinkOutcome {
  path: string;
  hash: string;
  mainFocused: boolean;
  outlineStyle: string;
}

/** Page object for the chrome shared by every route: nav, skip link, main landmark, route announcer. */
export class AppShellPage {
  readonly page: Page;
  readonly drawerNav: Locator;
  readonly skipLink: Locator;
  readonly main: Locator;
  /** Any live region; the Next announcer and the Bun one both match. */
  readonly liveRegions: Locator;

  constructor(page: Page) {
    this.page = page;
    this.drawerNav = page.getByTestId("drawer-nav");
    this.skipLink = page.getByRole("link", { name: "Skip to main content" });
    this.main = page.getByRole("main").first();
    this.liveRegions = page.locator("[role=status],[role=alert],[aria-live]");
  }

  /** Skips onboarding so the first-run overlay cannot cover the shell. */
  async skipOnboarding() {
    await this.page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  }

  async goto(path: string) {
    await this.page.goto(`${BASE_URL}${path}`, { waitUntil: "domcontentloaded" });
  }

  /** Drawer nav link by visible label; includeHidden because the drawer is aria-hidden on some routes. */
  navLink(label: string | RegExp): Locator {
    return this.drawerNav.getByRole("link", { name: label, includeHidden: true }).first();
  }

  /** Tab from the top of the document until the skip link has focus; returns the number of Tab presses. */
  async tabToSkipLink(maxTabs = 60): Promise<number> {
    for (let presses = 1; presses <= maxTabs; presses++) {
      await this.page.keyboard.press("Tab");
      if (await this.skipLink.evaluate((el) => el === document.activeElement)) return presses;
    }
    throw new Error(`skip link not reached within ${maxTabs} Tab presses`);
  }

  /** Activates the focused skip link and reports where focus and the URL ended up. */
  async activateSkipLink(): Promise<SkipLinkOutcome> {
    await this.page.keyboard.press("Enter");
    return this.page.evaluate(() => {
      const active = document.activeElement;
      const main = document.getElementById("main-content");
      return {
        path: location.pathname,
        hash: location.hash,
        mainFocused: !!active && active === main,
        outlineStyle: main ? getComputedStyle(main).outlineStyle : "",
      };
    });
  }

  /**
   * The page's scroll container: the element with the largest scrollable overflow (window scroll
   * counts too). It is the inner <main> on Settings and a content <div> on Up Next, so it is found
   * by measurement rather than by selector.
   */
  private static readonly SCROLLER = `(() => {
    let best = null;
    let range = document.scrollingElement ? document.scrollingElement.scrollHeight - innerHeight : 0;
    for (const el of document.querySelectorAll('*')) {
      if (el.closest('[data-testid=tray-scroll],[data-testid=notification-tray]')) continue;
      const overflow = getComputedStyle(el).overflowY;
      if (overflow !== 'auto' && overflow !== 'scroll') continue;
      const r = el.scrollHeight - el.clientHeight;
      if (r > range) { range = r; best = el; }
    }
    return { el: best, range };
  })()`;

  /** How far the page's scroll container can scroll. */
  async scrollRange(): Promise<number> {
    return this.page.evaluate(`${AppShellPage.SCROLLER}.range`) as Promise<number>;
  }

  /** Current scrollTop of the page's scroll container (0 when nothing scrolls). */
  async scrollTop(): Promise<number> {
    return this.page.evaluate(
      `(() => { const s = ${AppShellPage.SCROLLER}; return s.el ? s.el.scrollTop : scrollY; })()`,
    ) as Promise<number>;
  }

  async scrollTo(top: number): Promise<void> {
    await this.page.evaluate(
      `(() => { const s = ${AppShellPage.SCROLLER}; if (s.el) s.el.scrollTop = ${top}; else scrollTo(0, ${top}); })()`,
    );
  }

  /** Text of every live region, excluding empty ones. */
  async liveRegionTexts(): Promise<string[]> {
    const texts = await this.liveRegions.allTextContents();
    return texts.map((t) => t.trim()).filter(Boolean);
  }
}
