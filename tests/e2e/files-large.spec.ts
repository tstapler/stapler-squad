// @feature files:range-read, files:large-files, files:accessible-view
// Story 1.6.5: windowed viewer, large-file states and the Accessible view through the transport. files:range-read is off by
// default, so the suite switches it on for the isolated test server (never a live instance) and restores it afterwards.
// Runs in chromium and in the gating mobile-chromium project.
import fs from "fs";
import { test, expect, type APIRequestContext, type Page } from "@playwright/test";
import { FilesTabView, seedFilesFixture } from "./pages/files-tab.page";
import { FIXTURES, pad, writeNumberedLog } from "./helpers/largeFile";
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const FLAG = "files:range-read";
const LINES = 1_000_000;
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
  writeNumberedLog(dir, { name: "large.log", lines: LINES, width: 9 });
  writeNumberedLog(dir, { name: "huge.log", lines: LINES, width: 70 });
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

// Task 1.6.7a: the Story 1.6.7 matrix. Fixtures are generated on the host (5/50/200 MB) and removed after each test.
const MIB = 1024 * 1024;
const HEAP_BUDGET_BYTES = 256 * MIB; // chosen here: the plan names a "phone-class budget" without a number

function seedFixture(spec: keyof typeof FIXTURES): () => string {
  return () => {
    const dir = seedFilesFixture();
    writeNumberedLog(dir, FIXTURES[spec]);
    return dir;
  };
}

/** Sizes (decoded body bytes) of every ReadFileRange response the page receives. */
function watchRangeResponses(page: Page): { sizes: number[]; flush: () => Promise<void> } {
  const sizes: number[] = [];
  const pending: Promise<void>[] = [];
  page.on("response", (res) => {
    if (!res.url().endsWith("/ReadFileRange")) return;
    pending.push(res.body().then((b) => { sizes.push(b.length); }, () => undefined));
  });
  return { sizes, flush: async () => { await Promise.all(pending); } };
}

async function usedHeapBytes(page: Page): Promise<number | null> {
  return page.evaluate(() => (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory?.usedJSHeapSize ?? null);
}

test.describe("range-read matrix (T1 5/50 MB, T2 200 MB)", () => {
  test.beforeEach(async ({ page }, info) => {
    if (info.project.name === "chromium") await page.setViewportSize({ width: 1900, height: 1000 });
  });
  test.beforeAll(async ({ request }) => setRangeRead(request, true));
  test.afterAll(async ({ request }) => setRangeRead(request, false));

  test("matrix_should_ScrollJumpTo800000_AndOpenSearchHit_WithinResponseSizeAndHeapBudget_On50MbFixture", async ({ page }) => {
    const ranges = watchRangeResponses(page);
    const files = await FilesTabView.create(page, seedFixture("mb50"));
    try {
      await files.goto();
      await files.treeFile(FIXTURES.mb50.name).click();
      await expect.poll(() => files.activeLineText()).toBe("L0000001".padEnd(FIXTURES.mb50.width - 1, "x"));

      await goToLine(page, files, 800_000);
      await expect.poll(() => files.activeLineText()).toBe(`L${pad(800_000)}`.padEnd(FIXTURES.mb50.width - 1, "x"));
      await expect(page.getByText(`L${pad(800_000)}`)).toBeInViewport();

      // Scrolling: keyboard paging from the landing line moves the cursor forward through windows that are not in memory yet.
      await page.getByRole("textbox", { name: /^File: / }).click();
      for (let i = 0; i < 4; i++) await page.keyboard.press("PageDown");
      await expect.poll(async () => Number((await files.activeLineText()).slice(1, 8))).toBeGreaterThan(800_000);

      // Search hit: "Include large files" is what lets the scan reach a 50 MB file.
      if ((await files.layout) === "mobile") await files.bottomBarButton("Search").click();
      else await files.openSearch();
      await page.getByRole("button", { name: "Filters", exact: true, expanded: false }).last().click();
      await page.getByRole("checkbox", { name: "Include large files" }).check();
      await files.typeQuery("NEEDLE_MID", "NEEDLE_MID");
      await files.searchResults.getByRole("treeitem", { name: /NEEDLE_MID/ }).first().click();
      await expect.poll(() => files.activeLineText()).toMatch(/^NEEDLE_MID_640000x/);

      await ranges.flush();
      expect(ranges.sizes.length).toBeGreaterThan(0); // the windowed path served the file, not a whole-file read
      expect(Math.max(...ranges.sizes)).toBeLessThan(MIB);
      const heap = await usedHeapBytes(page);
      test.info().annotations.push({ type: "measured", description: `ranges=${ranges.sizes.length} maxResponseBytes=${Math.max(...ranges.sizes)} usedJSHeapBytes=${heap}` });
      if (heap !== null) expect(heap).toBeLessThan(HEAP_BUDGET_BYTES);
    } finally {
      fs.rmSync(files.dir, { recursive: true, force: true });
    }
  });

  test("matrix_should_OpenAndJumpExactly_On5MbFixture_WithEveryResponseUnder1MiB", async ({ page }) => {
    const ranges = watchRangeResponses(page);
    const files = await FilesTabView.create(page, seedFixture("mb5"));
    try {
      await files.goto();
      await files.treeFile(FIXTURES.mb5.name).click();
      await expect.poll(() => files.activeLineText()).toMatch(/^L0000001x/);
      await goToLine(page, files, 499_999);
      await expect.poll(() => files.activeLineText()).toMatch(/^L0499999x/);
      await ranges.flush();
      expect(ranges.sizes.length).toBeGreaterThan(0);
      expect(Math.max(...ranges.sizes)).toBeLessThan(MIB);
    } finally {
      fs.rmSync(files.dir, { recursive: true, force: true });
    }
  });

  test("matrix_should_BeViewOnlyWithReasonChipAndExactJump_On200MbFixture", async ({ page }) => {
    const ranges = watchRangeResponses(page);
    const files = await FilesTabView.create(page, seedFixture("mb200"));
    try {
      await files.goto();
      await files.treeFile(FIXTURES.mb200.name).click();
      await expect.poll(() => files.activeLineText()).toMatch(/^L0000001x/);
      await expect(page.getByRole("button", { name: "Read-only: file too large to edit" })).toBeVisible();
      await goToLine(page, files, 2_900_001);
      await expect.poll(() => files.activeLineText()).toMatch(/^L2900001x/);
      await ranges.flush();
      expect(Math.max(...ranges.sizes)).toBeLessThan(MIB);
    } finally {
      fs.rmSync(files.dir, { recursive: true, force: true });
    }
  });
});

test.describe("range failure falls back to the full adapter", () => {
  test.beforeEach(async ({ page }, info) => {
    if (info.project.name === "chromium") await page.setViewportSize({ width: 1900, height: 1000 });
  });
  test.beforeAll(async ({ request }) => setRangeRead(request, true));
  test.afterAll(async ({ request }) => setRangeRead(request, false));

  test("rangeFailure_should_FallBackToTruncatedReadOnlyRead_When_EveryReadFileRangeCallFails", async ({ page }) => {
    await page.route("**/ReadFileRange", (route) => route.fulfill({ status: 503, contentType: "application/json", body: '{"code":"unavailable","message":"injected"}' }));
    const files = await FilesTabView.create(page, seedFixture("mb5"));
    try {
      await files.goto();
      await files.treeFile(FIXTURES.mb5.name).click();
      await expect(page.getByText(/^L0000001x/).first()).toBeVisible();
      await expect(page.getByRole("button", { name: /Read-only: .*(first part|too large)/ })).toBeVisible();
    } finally {
      fs.rmSync(files.dir, { recursive: true, force: true });
    }
  });
});

// The `full` adapter run: with files:range-read off the base plan applies. A 5 MB file opens truncated and read-only; a file over
// the 10 MiB GetFileContent limit is refused with failed_precondition (the plan's "truncated" wording covers only the first case).
test.describe("full adapter (files:range-read off)", () => {
  test.beforeEach(async ({ page }, info) => {
    if (info.project.name === "chromium") await page.setViewportSize({ width: 1900, height: 1000 });
  });
  test.beforeAll(async ({ request }) => setRangeRead(request, false));

  test("fullAdapter_should_OpenTruncatedReadOnly_On5Mb_RefuseOver10MiB_And_MakeNoRangeCalls", async ({ page }) => {
    const ranges = watchRangeResponses(page);
    const files = await FilesTabView.create(page, () => {
      const dir = seedFixture("mb5")();
      writeNumberedLog(dir, FIXTURES.mb50);
      return dir;
    });
    try {
      await files.goto();
      await files.treeFile(FIXTURES.mb5.name).click();
      await expect(page.getByText(/^L0000001x/).first()).toBeVisible();
      await expect(page.getByRole("button", { name: /Read-only: .*(first part|too large)/ })).toBeVisible();
      if ((await files.layout) === "mobile") await files.paneBack.click();
      await files.treeFile(FIXTURES.mb50.name).click();
      await expect(page.getByText(/file too large \(\d+ bytes\); maximum is \d+ bytes/)).toBeVisible();
      await ranges.flush();
      expect(ranges.sizes).toHaveLength(0);
    } finally {
      fs.rmSync(files.dir, { recursive: true, force: true });
    }
  });
});
