// @feature files:range-read, files:large-files, files:accessible-view
// Story 1.6.5: windowed viewer, large-file states and the Accessible view through the transport. files:range-read is off by
// default, so the suite switches it on for the isolated test server (never a live instance) and restores it afterwards.
// Runs in chromium and in the gating mobile-chromium project.
import fs from "fs";
import path from "path";
import { test, expect, type APIRequestContext, type Page } from "@playwright/test";
import { FilesTabView, seedFilesFixture } from "./pages/files-tab.page";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const FLAG = "files:range-read";
const LINES = 1_000_000;
const pad = (n: number) => String(n).padStart(7, "0");
const rpc = (name: string) => `${BASE_URL}/api/session.v1.SessionService/${name}`;

async function setRangeRead(request: APIRequestContext, enabled: boolean): Promise<void> {
  await request.post(rpc("UpdateFeatureFlag"), { headers: { "Content-Type": "application/json" }, data: { name: FLAG, enabled } });
  const res = await request.post(rpc("GetFeatureFlags"), { headers: { "Content-Type": "application/json" }, data: {} });
  const flags = (await res.json()).flags as { name: string; enabled?: boolean }[];
  expect(flags.find((f) => f.name === FLAG)?.enabled ?? false).toBe(enabled); // read back, not assumed
}

/** `large.log`: 1,000,000 lines of 9 bytes (T1, ~9 MB). `huge.log`: 1,000,000 lines of 70 bytes (T2, ~67 MiB, over the 64 MiB hash cap). */
function seedLarge(): string {
  const dir = seedFilesFixture();
  const write = (name: string, width: number) => {
    const file = path.join(dir, name);
    for (let from = 1; from <= LINES; from += 100_000) {
      const chunk = Array.from({ length: 100_000 }, (_, i) => `L${pad(from + i)}`.padEnd(width - 1, "x") + "\n").join("");
      fs.appendFileSync(file, chunk);
    }
  };
  write("large.log", 9);
  write("huge.log", 70);
  return dir;
}

async function goToLine(page: Page, files: FilesTabView, line: number): Promise<void> {
  if ((await files.layout) === "mobile") {
    await files.bottomBarButton("Go to line").click();
  } else {
    await page.getByRole("textbox", { name: /^File: / }).click();
    await page.keyboard.press("Control+g");
  }
  await page.getByRole("textbox", { name: "Go to line" }).fill(String(line));
  await page.keyboard.press("Enter");
}

test.describe("windowed viewer", () => {
  // Wide enough that the tree and the editor sit side by side on desktop; the phone project keeps its own viewport.
  test.beforeEach(async ({ page }, info) => {
    if (info.project.name === "chromium") await page.setViewportSize({ width: 1900, height: 1000 });
  });
  test.beforeAll(async ({ request }) => setRangeRead(request, true));
  test.afterAll(async ({ request }) => setRangeRead(request, false));

  test("windowedOpen_should_PaintFirstWindowShowAboutNLines_And_RestoreAnchorAfterReload", async ({ page }) => {
    const files = await FilesTabView.create(page, seedLarge);
    await files.goto();
    await files.treeFile("large.log").click();
    await expect.poll(() => files.activeLineText()).toBe("L0000001");
    if ((await files.layout) !== "mobile") await expect(page.getByTestId("files-status-bar")).toContainText(/Ln 1 of (about )?[\d.,]+( million)?, Col 1/);

    await goToLine(page, files, 700_000);
    await expect.poll(() => files.activeLineText()).toBe("L0700000");
    await expect(page.getByText("L0700000", { exact: true })).toBeInViewport();

    // Reload: the tab reopens on the same line from a window read; no chunk text was persisted.
    await files.goto();
    await expect.poll(() => files.activeLineText()).toBe("L0700000");
    const stored = await page.evaluate(() => Object.entries(localStorage).filter(([k]) => k.startsWith("ssq.files.")).map(([, v]) => v).join(""));
    expect(stored).toContain('"line":700000');
    expect(stored).not.toContain("L0700000");
  });

  test("farJump_should_LandOnTheExactLine_ClampPastEnd_And_SayWhy", async ({ page }) => {
    const files = await FilesTabView.create(page, seedLarge);
    await files.goto();
    await files.treeFile("large.log").click();
    await expect.poll(() => files.activeLineText()).toBe("L0000001");

    for (const target of [999_999, 123_457, 2]) {
      await goToLine(page, files, target);
      await expect.poll(() => files.activeLineText()).toBe(`L${pad(target)}`);
      await expect(page.getByText(`L${pad(target)}`, { exact: true })).toBeInViewport();
    }
    await goToLine(page, files, 5_000_000);
    await expect.poll(() => files.activeLineText()).toBe(`L${pad(LINES)}`);
    await expect(page.getByText("Line 5,000,000 is past the end; showing line 1,000,000")).toBeVisible();
  });

  test("accessibleView_should_PageThroughTheTransport_When_WindowedFileOpen", async ({ page }) => {
    const files = await FilesTabView.create(page, seedLarge);
    await files.goto();
    await files.treeFile("large.log").click();
    await expect.poll(() => files.activeLineText()).toBe("L0000001");
    if ((await files.layout) === "mobile") {
      await files.overflowButton.click();
      await page.getByRole("menuitem", { name: "Switch to accessible view" }).click();
    } else {
      await page.getByTestId("accessible-skip-link").focus(); // a skip link: reached by keyboard, not by pointer
      await page.keyboard.press("Enter");
    }
    const region = page.getByRole("region", { name: "large.log, accessible view" });
    await expect(region).toBeVisible();
    await expect(region.getByTestId("accessible-view-label")).toHaveText(/^Lines 1-500 of (about )?\d+$/);
    await expect(region.getByTestId("accessible-view-line")).toHaveCount(500);
    await region.getByRole("button", { name: "Next page", exact: true }).click();
    await expect(region.getByTestId("accessible-view-label")).toHaveText(/^Lines 501-1000 of (about )?\d+$/);
    await expect(region.getByTestId("accessible-view-line").first()).toHaveText("501 L0000501");
  });

  test("t2_should_BeViewOnlyWithReasonChip_And_KeepDownload", async ({ page }) => {
    const files = await FilesTabView.create(page, seedLarge);
    await files.goto();
    await files.treeFile("huge.log").click();
    await expect.poll(() => files.activeLineText()).toMatch(/^L0000001x/);
    await expect(page.getByRole("button", { name: "Read-only: file too large to edit" })).toBeVisible();
    await expect(page.getByRole("link", { name: /Download/ })).toHaveAttribute("href", /download=true/);
    await goToLine(page, files, 654_321);
    await expect.poll(() => files.activeLineText()).toMatch(/^L0654321x/);
  });
});
