import { Page, Locator } from "@playwright/test";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { SessionClient } from "../helpers/session-client";
import { baseUrl } from '../helpers/base-url';

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

const BASE_URL = baseUrl();

/** A directory with a few small files and one 300-line file; returns its path. */
export function seedFilesFixture(): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ssq-files-e2e-"));
  fs.writeFileSync(path.join(dir, "a.txt"), "alpha\n");
  fs.writeFileSync(path.join(dir, "b.txt"), "beta\n");
  fs.writeFileSync(path.join(dir, "c.txt"), "gamma\n");
  fs.writeFileSync(path.join(dir, "long.txt"), Array.from({ length: 300 }, (_, i) => `line ${i + 1}`).join("\n") + "\n");
  return dir;
}

/** Worktree for the search e2e: `pkg/a.go` holds `needle_fn` on line 12; `big_a.go` and `big_b.go` are 1000 lines each. */
export function seedSearchFixture(): string {
  const dir = seedFilesFixture();
  fs.mkdirSync(path.join(dir, "pkg"));
  const a = Array.from({ length: 40 }, (_, i) => `// a.go line ${i + 1}`);
  a[11] = "func needle_fn() {}";
  fs.writeFileSync(path.join(dir, "pkg", "a.go"), a.join("\n") + "\n");
  fs.writeFileSync(path.join(dir, "pkg", "b.go"), "package pkg\n");
  for (const name of ["big_a.go", "big_b.go"]) {
    fs.writeFileSync(path.join(dir, name), Array.from({ length: 1000 }, (_, i) => `${name} line ${i + 1}`).join("\n") + "\n");
  }
  return dir;
}

/** Page object for the real Files tab of a session detail view (files:viewer-v2 on). */
export class FilesTabView {
  constructor(readonly page: Page, readonly dir: string) {}

  static async create(page: Page, seed: () => string = seedFilesFixture): Promise<FilesTabView> {
    const dir = seed();
    await suppressOnboarding(page);
    const session = await new SessionClient(BASE_URL).createSession({ title: `files-e2e-${Date.now()}`, path: dir, program: "bash" });
    const view = new FilesTabView(page, dir);
    view.sessionId = session.id;
    return view;
  }

  sessionId = "";

  async goto(query = ""): Promise<void> {
    await this.page.goto(`${BASE_URL}/?session=${this.sessionId}&tab=files${query}`);
    await this.page.getByRole("tab", { name: "Files", exact: true }).click();
  }

  get separator(): Locator { return this.page.getByRole("separator", { name: "Resize file panel" }); }
  get treePane(): Locator { return this.page.getByTestId("files-tree-pane"); }
  get searchBox(): Locator { return this.page.getByRole("searchbox", { name: "Search files" }); }
  get staleBanner(): Locator { return this.page.getByTestId("files-stale-banner"); }
  treeFile(file: string): Locator { return this.page.getByText(file, { exact: true }).first(); }

  get explorerTab(): Locator { return this.page.getByRole("tab", { name: "Explorer", exact: true }); }
  get searchTab(): Locator { return this.page.getByRole("tab", { name: "Search", exact: true }); }
  get searchQuery(): Locator { return this.page.getByRole("searchbox", { name: "Search file contents" }); }
  get searchResults(): Locator { return this.page.getByRole("tree", { name: "Search results" }); }
  get quickfixChip(): Locator { return this.page.getByRole("group", { name: "Search hits" }); }
  editorTab(file: string): Locator { return this.page.getByRole("tab", { name: new RegExp(`^${file.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`) }); }

  /** Interaction 1: open the Search pane. */
  async openSearch(): Promise<void> {
    await this.searchTab.click();
  }

  /** Interaction 2: type the query and wait for the streamed hits to land. */
  async typeQuery(pattern: string, expectedHit: string): Promise<void> {
    await this.searchQuery.fill(pattern);
    await this.searchResults.getByRole("treeitem", { name: new RegExp(expectedHit) }).first().waitFor();
  }

  /** Text of the editor's active line; the only way to observe the cursor without CSS-class locators. */
  async activeLineText(): Promise<string> {
    return this.page.evaluate(() => document.querySelector(".cm-activeLine")?.textContent ?? "");
  }

  // Mobile layout (nav-stack panes, top bar, bottom bar, tab sheet).
  get layoutRoot(): Locator { return this.page.getByTestId("files-tab"); }
  get topBar(): Locator { return this.page.getByTestId("mobile-top-bar"); }
  get bottomBar(): Locator { return this.page.getByRole("navigation", { name: "Files actions" }); }
  bottomBarButton(name: string): Locator { return this.bottomBar.getByRole("button", { name, exact: true }); }
  get paneBack(): Locator { return this.topBar.getByRole("button", { name: /^Back to / }); }
  get tabCountButton(): Locator { return this.topBar.getByRole("button", { name: /^Open files, \d+$/ }); }
  get tabSheet(): Locator { return this.page.getByRole("dialog", { name: "Open files" }); }
  get overflowButton(): Locator { return this.topBar.getByRole("button", { name: "More actions" }); }
  get layout(): Promise<string | null> { return this.layoutRoot.getAttribute("data-layout"); }
  get pane(): Promise<string | null> { return this.layoutRoot.getAttribute("data-pane"); }
  get codeContent(): Locator { return this.page.getByRole("textbox", { name: /^File: / }); }
}
