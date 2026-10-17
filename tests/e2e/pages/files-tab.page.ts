import { Page, Locator } from "@playwright/test";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { SessionClient } from "../helpers/session-client";

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

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/** A directory with a few small files and one 300-line file; returns its path. */
export function seedFilesFixture(): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ssq-files-e2e-"));
  fs.writeFileSync(path.join(dir, "a.txt"), "alpha\n");
  fs.writeFileSync(path.join(dir, "b.txt"), "beta\n");
  fs.writeFileSync(path.join(dir, "c.txt"), "gamma\n");
  fs.writeFileSync(path.join(dir, "long.txt"), Array.from({ length: 300 }, (_, i) => `line ${i + 1}`).join("\n") + "\n");
  return dir;
}

/** Page object for the real Files tab of a session detail view (files:viewer-v2 on). */
export class FilesTabView {
  constructor(readonly page: Page, readonly dir: string) {}

  static async create(page: Page): Promise<FilesTabView> {
    const dir = seedFilesFixture();
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

  /** Text of the editor's active line; the only way to observe the cursor without CSS-class locators. */
  async activeLineText(): Promise<string> {
    return this.page.evaluate(() => document.querySelector(".cm-activeLine")?.textContent ?? "");
  }
}
