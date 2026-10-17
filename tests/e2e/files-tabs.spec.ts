// @feature files:tabs
// Stories 1.3.1-1.3.4: tab strip, jumplist / go-to-line / deep links, stale banner. Written against the real
// Files tab (files:viewer-v2 on, the default). Needs a built server; see global-setup.ts.
import * as fs from "fs";
import * as path from "path";
import { test, expect } from "@playwright/test";
import { FilesTabView } from "./pages/files-tab.page";

test.describe("tab strip", () => {
  test("tabs_should_OpenPreviewThenPin_And_CycleWithArrows_And_CloseWithAltW", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await files.treeFile("a.txt").click();
    await expect(page.getByRole("tab", { name: "a.txt, preview" })).toBeVisible();
    await files.treeFile("b.txt").click();
    await expect(page.getByRole("tab", { name: "b.txt, preview" })).toBeVisible();
    await expect(page.getByRole("tab", { name: /^a\.txt/ })).toHaveCount(0);

    await page.getByRole("tab", { name: "b.txt, preview" }).dblclick();
    await expect(page.getByRole("tab", { name: "b.txt, pinned" })).toBeVisible();
    await files.treeFile("c.txt").click();
    await expect(page.getByRole("tablist")).toContainText("c.txt");

    await page.getByRole("tab", { name: "b.txt, pinned" }).focus();
    await page.keyboard.press("ArrowRight");
    await expect(page.getByRole("tab", { name: /^c\.txt/ })).toHaveAttribute("aria-selected", "true");

    await page.keyboard.press("Alt+w");
    await expect(page.getByRole("tab", { name: /^c\.txt/ })).toHaveCount(0);
  });

  test.describe("touch sizing", () => {
    test.use({ hasTouch: true, isMobile: true, viewport: { width: 820, height: 1180 } });
    test("tabs_should_GiveCloseButtonA44pxHitArea_When_PointerCoarse", async ({ page }) => {
      const files = await FilesTabView.create(page);
      await files.goto();
      await files.treeFile("a.txt").tap();
      const box = await page.getByRole("button", { name: "Close a.txt" }).boundingBox();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
    });
  });
});

test.describe("jumplist, go to line and deep links", () => {
  test("nav_should_GoToLineClamped_And_StepBackWithAltBracket_And_OpenDeepLinkAtLine", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await files.treeFile("long.txt").click();
    await page.keyboard.press("Control+g");
    await page.getByRole("textbox", { name: "Go to line" }).fill("250");
    await page.keyboard.press("Enter");
    await expect.poll(() => files.activeLineText()).toContain("line 250");

    await page.keyboard.press("Control+g");
    await page.getByRole("textbox", { name: "Go to line" }).fill("9999");
    await page.keyboard.press("Enter");
    await expect.poll(() => files.activeLineText()).toContain("line 300");

    await files.treeFile("a.txt").click();
    await page.keyboard.press("Alt+[");
    await expect(page.getByRole("tab", { name: /^long\.txt/ })).toHaveAttribute("aria-selected", "true");
    await expect.poll(() => files.activeLineText()).toContain("line 300");
  });

  test("deepLink_should_OpenPinnedAtLine_When_QueryFileAndLine", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto("&file=long.txt&line=42");
    await expect(page.getByRole("tab", { name: "long.txt, pinned" })).toBeVisible();
    await expect.poll(() => files.activeLineText()).toContain("line 42");
  });
});

test.describe("stale banner", () => {
  test("staleBanner_should_AppearWithoutMovingFocus_And_ReloadKeepLineScroll", async ({ page }) => {
    test.setTimeout(60_000);
    const files = await FilesTabView.create(page);
    await files.goto("&file=long.txt&line=200");
    await expect.poll(() => files.activeLineText()).toContain("line 200");

    await files.searchBox.focus();
    const target = path.join(files.dir, "long.txt");
    fs.writeFileSync(target, fs.readFileSync(target, "utf8").replace("line 1\n", "line one rewritten\n"));

    // The active-buffer poll runs every 10 s and confirms a change one second later.
    await expect(files.staleBanner).toContainText("Changed on disk", { timeout: 20_000 });
    await expect(files.searchBox).toBeFocused();
    await expect(page.getByText("line one rewritten")).toHaveCount(0);

    await files.staleBanner.getByRole("button", { name: "Reload" }).click();
    await expect(files.staleBanner).toHaveCount(0);
    await expect(page.getByText("line one rewritten")).toBeVisible();
    await expect.poll(() => files.activeLineText()).toContain("line 200");
  });
});
