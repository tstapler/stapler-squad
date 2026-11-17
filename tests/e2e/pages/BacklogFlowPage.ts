import { APIRequestContext, Locator, Page, Route } from "@playwright/test";
import { baseUrl } from "../helpers/base-url";
import { seedReviewVerdictItem } from "./BacklogItemDetailPage";
import { createBacklogItemDirect, deleteBacklogItemsByTitlePrefix, transitionBacklogItemDirect } from "./BacklogMutations";

const BASE_URL = baseUrl();
const SUMMARY_PATH = "**/session.v1.BacklogFlowService/GetFlowSummary";
const DRILLDOWN_PATH = "**/session.v1.BacklogFlowService/ListFlowDrilldown";

/** Eight ready <-> in_progress flips inside the 15 minute gap form one thrash episode (session/flow/config.go). */
const THRASH_FLIPS = 8;

export interface SeededFlowData {
  mergedItemId: string;
  thrashItemId: string;
}

/**
 * Seeds the isolated test server through the e2e-local debug endpoints (BacklogMutations.ts): one item walked
 * through the happy path to done, one item that flips ready <-> in_progress (a thrash item), and one item with a
 * PARTIAL review verdict so the verdict-mix chart has bars.
 */
export async function seedFlowData(request: APIRequestContext): Promise<SeededFlowData> {
  // beforeAll runs once per worker and each Playwright project gets its own worker on the same slot server, so a
  // second project would otherwise add a second thrash item and break the singular "1 item thrashing" banner.
  await deleteBacklogItemsByTitlePrefix(request, ["Flow e2e "]);
  const mergedItemId = await createBacklogItemDirect(request, { title: "Flow e2e merged item" });
  for (const status of ["ready", "in_progress", "review", "pr_pending", "done"]) {
    await transitionBacklogItemDirect(request, mergedItemId, status);
  }
  const thrashItemId = await createBacklogItemDirect(request, { title: "Flow e2e thrash item" });
  await transitionBacklogItemDirect(request, thrashItemId, "ready");
  for (let flip = 0; flip < THRASH_FLIPS; flip++) {
    await transitionBacklogItemDirect(request, thrashItemId, flip % 2 === 0 ? "in_progress" : "ready");
  }
  await seedReviewVerdictItem(request, { title: "Flow e2e verdict item" });
  return { mergedItemId, thrashItemId };
}

type JsonObject = Record<string, unknown>;

/** Page object for /backlog/flow. Every click goes through `click()` so tests can assert click counts. */
export class BacklogFlowPage {
  readonly page: Page;
  clicks = 0;

  constructor(page: Page) {
    this.page = page;
  }

  async goto(search = "?window=30"): Promise<void> {
    await this.page.goto(`${BASE_URL}/backlog/flow${search}`, { waitUntil: "domcontentloaded" });
  }

  /** Clicks and counts; use for every interaction whose click budget a criterion asserts. */
  async click(locator: Locator): Promise<void> {
    this.clicks++;
    await locator.click();
  }

  resetClicks(): void {
    this.clicks = 0;
  }

  tile(name: string): Locator {
    return this.page.getByTestId(`flow-tile-${name}`);
  }

  /** The drill button inside a tile, found by role so the test does not depend on its markup. */
  tileButton(name: string): Locator {
    return this.tile(name).getByRole("button").first();
  }

  get drawer(): Locator {
    return this.page.getByTestId("flow-drilldown-drawer");
  }

  get thrashBanner(): Locator {
    return this.page.getByTestId("flow-thrash-banner");
  }

  get staleBanner(): Locator {
    return this.page.getByTestId("flow-stale-banner");
  }

  get hardError(): Locator {
    return this.page.getByTestId("flow-hard-error");
  }

  get viewTabs(): Locator {
    return this.page.getByRole("navigation", { name: "Backlog views" });
  }

  get jumpList(): Locator {
    return this.page.getByRole("navigation", { name: "Jump to section" });
  }

  windowRadio(value: "7" | "30" | "90" | "all"): Locator {
    return this.page.getByTestId(`flow-window-${value}`);
  }

  compareRadio(value: "none" | "prev"): Locator {
    return this.page.getByTestId(`flow-compare-${value}`);
  }

  sectionHeading(name: string): Locator {
    return this.page.getByRole("heading", { level: 2, name });
  }

  /** Fetches the real response, lets `mutate` rewrite it, and fulfils the request with the result. */
  async stubSummary(mutate: (body: JsonObject) => void): Promise<void> {
    await this.page.route(SUMMARY_PATH, async (route: Route) => {
      const response = await route.fetch();
      const body = (await response.json()) as { summary: JsonObject };
      mutate(body.summary);
      await route.fulfill({ response, json: body });
    });
  }

  /** Fails every GetFlowSummary call with a Connect error until `unstubSummary()`. */
  async failSummary(message = "flow aggregation exploded"): Promise<void> {
    await this.page.route(SUMMARY_PATH, (route) => route.fulfill({ status: 500, json: { code: "internal", message } }));
  }

  async unstubSummary(): Promise<void> {
    await this.page.unroute(SUMMARY_PATH);
  }

  async failDrilldown(): Promise<void> {
    await this.page.route(DRILLDOWN_PATH, (route) => route.fulfill({ status: 500, json: { code: "internal", message: "drill exploded" } }));
  }

  async unstubDrilldown(): Promise<void> {
    await this.page.unroute(DRILLDOWN_PATH);
  }

  /** Focuses the Flow view tab, then presses Tab `count` times; each stop records whether it follows the previous one in DOM order. */
  async tabThrough(count: number): Promise<{ label: string; followsPrevious: boolean }[]> {
    await this.viewTabs.getByRole("link", { name: "Flow" }).focus();
    await this.page.evaluate(() => {
      (window as unknown as { __flowPrev: Element | null }).__flowPrev = document.activeElement;
    });
    const stops: { label: string; followsPrevious: boolean }[] = [];
    for (let i = 0; i < count; i++) {
      await this.page.keyboard.press("Tab");
      stops.push(
        await this.page.evaluate(() => {
          const w = window as unknown as { __flowPrev: Element | null };
          const el = document.activeElement as HTMLElement;
          const label = el.getAttribute("aria-label") ?? el.textContent?.trim() ?? el.tagName;
          const follows = w.__flowPrev === null || Boolean(w.__flowPrev.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING);
          w.__flowPrev = el;
          return { label, followsPrevious: follows };
        }),
      );
    }
    return stops;
  }

  /** Visible buttons, links and radios in the page whose box is smaller than `min` CSS px on either side. */
  async undersizedTargets(min: number): Promise<string[]> {
    const main = this.page.locator("main");
    const targets = main.getByRole("button").or(main.getByRole("link")).or(main.getByRole("radio"));
    const problems: string[] = [];
    for (const target of await targets.all()) {
      const box = await target.boundingBox();
      if (box && (box.width < min || box.height < min)) {
        problems.push(`${(await target.textContent())?.trim() || (await target.getAttribute("aria-label"))}: ${Math.round(box.width)}x${Math.round(box.height)}`);
      }
    }
    return problems;
  }

  /** Computed transition-duration, in seconds, of every element inside the page's charts. */
  async chartTransitionSeconds(): Promise<number[]> {
    return this.page.evaluate(() =>
      [...document.querySelectorAll('[data-testid="flow-chart"] *, [data-testid^="flow-sparkline"] *')].map((el) => parseFloat(getComputedStyle(el).transitionDuration.split(",")[0])),
    );
  }

  /** Scroll regions (tables overflowing their box) that lack an accessible name. */
  async unlabelledScrollRegions(): Promise<string[]> {
    return this.page.evaluate(() =>
      [...document.querySelectorAll<HTMLElement>("[role=region]")]
        .filter((el) => el.scrollWidth > el.clientWidth && !el.getAttribute("aria-label") && !el.getAttribute("aria-labelledby"))
        .map((el) => el.outerHTML.slice(0, 80)),
    );
  }
}
