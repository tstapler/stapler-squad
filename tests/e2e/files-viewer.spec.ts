// @feature files:tabs
// Phase 0 spike section of the files-viewer-revamp plan. Order: Task 0.1.1, Task 0.1.4a, Task 0.1.6a.
import { test, expect } from "@playwright/test";
import { FilesTabPage } from "./pages/files-tab.page";

test.describe("Task 0.1.1: server-streaming RPC delivers incrementally", () => {
  // The throwaway handler is registered by the Go lane (Task 0.1.1b); until it exists this stays skipped.
  const streamPath = process.env.SPIKE_STREAM_PATH;
  test.skip(!streamPath, "SPIKE_STREAM_PATH not set: throwaway streaming handler (Task 0.1.1b) is not available");

  test("streamingTransport_should_DeliverMessagesIncrementally_On_RealServerPath", async ({ page }) => {
    await page.goto("/");
    const arrivals = await page.evaluate(async (path) => {
      const body = new Uint8Array(5 + 2);
      body[4] = 2;
      body.set(new TextEncoder().encode("{}"), 5);
      const start = performance.now();
      const res = await fetch(path as string, {
        method: "POST",
        headers: { "content-type": "application/connect+json", "connect-protocol-version": "1" },
        body,
      });
      const reader = res.body!.getReader();
      const times: number[] = [];
      for (;;) {
        const { done } = await reader.read();
        if (done) break;
        times.push(performance.now() - start);
      }
      return times;
    }, streamPath);
    console.log(`SPIKE 0.1.1 chunk arrivals (ms): ${JSON.stringify(arrivals.map(Math.round))}`);
    expect(arrivals.length).toBeGreaterThanOrEqual(3);
    expect(arrivals[arrivals.length - 1] - arrivals[0]).toBeGreaterThanOrEqual(150);
  });
});

test.describe("Task 0.1.4a: state swap keeps scroll, selection and undo", () => {
  test("codeEditor_should_RestoreScrollSelectionUndo_When_TabSwitchedAndBack", async ({ page }) => {
    const files = new FilesTabPage(page);
    await files.gotoSpike("stateswap");
    await expect(files.editor).toBeVisible();

    await files.api("goto", 900, 2, 6);
    await files.api("type", "!!");
    const lineHeight = await files.api<number>("lineHeight");
    const scrollA = await files.api<number>("scrollTop");
    const selectionA = await files.api<number[][]>("selection");
    const depthA = await files.api<number>("undoDepth");
    expect(scrollA).toBeGreaterThan(lineHeight * 800);
    expect(depthA).toBeGreaterThan(0);

    await files.tab("b").click();
    await files.api("goto", 1400, 1, 4);
    expect(await files.api<number>("undoDepth")).toBe(0);
    expect(await files.api<number>("scrollTop")).toBeGreaterThan(scrollA);

    await files.tab("a").click();
    await expect.poll(async () => Math.abs((await files.api<number>("scrollTop")) - scrollA)).toBeLessThanOrEqual(lineHeight);
    expect(await files.api<number[][]>("selection")).toEqual(selectionA);
    expect(await files.api<number>("undoDepth")).toBe(depthA);
  });
});

test.describe("Task 0.1.6a: accessibility tree of the Accessible view versus CodeMirror 6", () => {
  test("accessibleView_should_ExposeEveryPageLine_And_Cm6OnlyViewportLines_When_AccessibilityTreeRead", async ({ page, browserName }) => {
    test.skip(browserName !== "chromium", "reads Chromium's accessibility tree over CDP");
    const files = new FilesTabPage(page);
    await files.gotoSpike("a11y");
    await expect(page.getByTestId("spike-page-label")).toHaveText("Lines 1-500 of 3000");

    const cdp = await page.context().newCDPSession(page);
    const { nodes } = await cdp.send("Accessibility.getFullAXTree");
    const names = nodes.filter((n) => n.role?.value === "StaticText").map((n) => String(n.name?.value ?? ""));
    const accessible = names.filter((n) => /^(\d+) L\1 text$/.test(n));
    const cm6 = names.filter((n) => /^L\d+ text$/.test(n));

    console.log(`SPIKE 0.1.6a: accessible-view lines in AX tree=${accessible.length}, CM6 lines in AX tree=${cm6.length} of 3000`);
    expect(accessible).toHaveLength(500);
    expect(cm6.length).toBeGreaterThan(0);
    expect(cm6.length).toBeLessThan(500);
  });
});
