// @feature files:mobile, files:content-search
// Story 1.5.3b. Runs in the mobile-chromium (gating) and mobile-webkit projects. The visualViewport stub
// emulates a soft keyboard; real keyboard behavior is the owner's phone check (Task 1.5.3c / 2.5.2).
// Written against the real Files tab; not run at the time of writing (needs a built server).
import fs from "fs";
import path from "path";
import { test, expect } from "@playwright/test";
import { FilesTabView, seedSearchFixture } from "./pages/files-tab.page";
import { assertReflow, setVisualViewportHeight, stubVisualViewport } from "./helpers/a11y";

const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };
const NARROW = { viewport: { width: 320, height: 568 }, isMobile: true, hasTouch: true };

function seedLarge(): string {
  const dir = seedSearchFixture();
  fs.writeFileSync(path.join(dir, "big.js"), ("const x = 1; // ".padEnd(79, "x") + "\n").repeat(7500));
  fs.writeFileSync(path.join(dir, "wide.txt"), "w".repeat(4000) + "\n");
  return dir;
}

test.describe("phone 390x844", () => {
  test.use(PHONE);

  test("filesMobile_should_OpenSearchJumpAndReturnToResults_When_390x844", async ({ page }) => {
    const files = await FilesTabView.create(page, seedSearchFixture);
    await files.goto();
    expect(await files.layout).toBe("mobile");
    await files.treeFile("a.txt").click();
    await expect(files.topBar).toBeVisible();
    await files.bottomBarButton("Search").click();
    await files.searchQuery.fill("needle_fn");
    const hit = files.searchResults.getByRole("treeitem", { name: /pkg\/a\.go line 12/ }).first();
    await hit.click();
    await expect(files.topBar).toContainText("pkg/a.go");
    await expect.poll(() => files.activeLineText()).toContain("needle_fn");
    await files.paneBack.click();
    await expect(files.searchResults).toBeVisible();
    await expect(hit).toBeVisible();
  });

  test("filesMobile_should_PopEditorResultsFilesInOrder_And_KeepDeepLink_When_BackTwice", async ({ page }) => {
    const files = await FilesTabView.create(page, seedSearchFixture);
    await files.goto();
    const sessionUrl = new URL(page.url());
    await files.bottomBarButton("Search").click();
    await files.searchQuery.fill("needle_fn");
    await files.searchResults.getByRole("treeitem", { name: /pkg\/a\.go line 12/ }).first().click();
    expect(await files.pane).toBe("editor");

    await page.goBack();
    await expect.poll(() => files.pane).toBe("search");
    await expect(files.searchResults).toBeVisible();
    await page.goBack();
    await expect.poll(() => files.pane).toBe("files");
    expect(new URL(page.url()).searchParams.get("session")).toBe(sessionUrl.searchParams.get("session"));

    await files.goto("&file=pkg/a.go&line=12");
    await expect.poll(() => files.activeLineText()).toContain("needle_fn");
    await page.goBack();
    await page.goForward();
    await expect.poll(() => files.activeLineText()).toContain("needle_fn");
  });

  test("mobileNav_should_PopPaneWithoutLeavingSession_When_BrowserBack", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await stubVisualViewport(page);
    await files.treeFile("a.txt").click();
    expect(await files.pane).toBe("editor");
    // Keyboard up then down with no popstate: the pane does not move (Android Back, ux 7.4 rule 6).
    await setVisualViewportHeight(page, 500);
    await setVisualViewportHeight(page, 844);
    expect(await files.pane).toBe("editor");
    await page.goBack();
    await expect.poll(() => files.pane).toBe("files");
    expect(page.url()).toContain("session=");
  });

  test("filesMobile_should_NotScrollHorizontallyAndMeet44px_When_390x844And320x568", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    expect(await assertReflow(page)).toEqual([]);
    await files.treeFile("a.txt").click();
    for (const bar of [files.topBar, files.bottomBar]) {
      for (const b of await bar.getByRole("button").all()) {
        const box = (await b.boundingBox())!;
        expect(box.height).toBeGreaterThanOrEqual(44);
        expect(box.width).toBeGreaterThanOrEqual(44);
      }
    }
    const padding = await files.bottomBar.evaluate((el) => getComputedStyle(el).paddingBottom);
    expect(padding).toBeDefined();
    expect(await assertReflow(page)).toEqual([]);
  });

  test("filesMobile_should_HideBottomBarAndSizeEditor_When_VisualViewportStubbedTo500", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await stubVisualViewport(page);
    await files.treeFile("a.txt").click();
    await expect(files.bottomBar).toBeVisible();
    await setVisualViewportHeight(page, 500);
    await expect(files.bottomBar).toBeHidden();
    const h = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--viewport-height"));
    expect(h).toBe("500px");
  });

  test("codeEditor_should_NotFocusOrResizeViewport_When_TapInReadModeOnCoarse", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await files.treeFile("a.txt").click();
    await expect(files.codeContent).toHaveAttribute("inputmode", "none");
    const before = await page.evaluate(() => window.visualViewport?.height);
    await files.codeContent.tap();
    expect(await page.evaluate(() => window.visualViewport?.height)).toBe(before);
  });

  test("tabControls_should_MeasureAtLeast44x44_When_CoarsePointer", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await files.treeFile("a.txt").click();
    await files.tabCountButton.click();
    await expect(files.tabSheet).toBeVisible();
    for (const row of await files.tabSheet.getByRole("listitem").all()) {
      expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    }
    for (const name of ["Close tab list"]) {
      const box = (await files.tabSheet.getByRole("button", { name }).boundingBox())!;
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
  });

  test("sheets_should_TrapFocusDismissAndDisableAnimation_When_ReducedMotion", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    const files = await FilesTabView.create(page);
    await files.goto();
    await files.treeFile("a.txt").click();
    await files.tabCountButton.click();
    await expect(files.tabSheet).toBeVisible();
    expect(await files.tabSheet.evaluate((el) => getComputedStyle(el).animationName)).toBe("none");
    for (let i = 0; i < 8; i++) {
      await page.keyboard.press("Tab");
      expect(await files.tabSheet.evaluate((el) => el.contains(document.activeElement))).toBe(true);
    }
    await page.keyboard.press("Escape");
    await expect(files.tabSheet).toBeHidden();
    await expect(files.tabCountButton).toBeFocused();
  });

  test("searchRow_should_OpenOnSingleTap_When_KeyboardOpenViewportStubbed", async ({ page }) => {
    const files = await FilesTabView.create(page, seedSearchFixture);
    await files.goto();
    await stubVisualViewport(page);
    await files.bottomBarButton("Search").click();
    await files.searchQuery.fill("line");
    await files.searchResults.getByRole("treeitem").nth(1).waitFor();
    await files.searchQuery.focus();
    await setVisualViewportHeight(page, 500);
    await files.searchResults.getByRole("treeitem", { level: 2 }).nth(2).tap();
    await expect.poll(() => files.pane).toBe("editor");
  });

  test("largeFile_should_DegradeWithNotice_And_ScrollLongLine_When_Coarse600Kb", async ({ page }) => {
    const files = await FilesTabView.create(page, seedLarge);
    await files.goto();
    await files.treeFile("big.js").click();
    await expect(page.getByTestId("degrade-notice")).toBeVisible();
    await files.paneBack.click();
    await files.treeFile("wide.txt").click();
    await expect(files.codeContent).toBeVisible();
    const scrolls = await files.codeContent.evaluate((el) => (el.closest(".cm-scroller") as HTMLElement).scrollWidth > (el.closest(".cm-scroller") as HTMLElement).clientWidth);
    expect(scrolls).toBe(true);
  });
});

test.describe("narrow phone 320x568", () => {
  test.use(NARROW);

  test("bottomBar_should_CollapseToFivePrimaryTargetsWithMore_When_320x568", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await files.treeFile("a.txt").click();
    const names = await files.bottomBar.getByRole("button").evaluateAll((els) => els.map((e) => e.getAttribute("aria-label")));
    expect(names).toEqual(["Files", "Search", "Quick open", "Previous location", "More"]);
    for (const b of await files.bottomBar.getByRole("button").all()) expect((await b.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    expect(await assertReflow(page)).toEqual([]);
    await files.bottomBarButton("More").click();
    await expect(page.getByRole("menuitem", { name: "Go to line" })).toBeVisible();
    await expect(page.getByRole("menuitem", { name: "Next location" })).toBeVisible();
  });

  test("filesMobile_should_NotScrollHorizontally_When_320x568", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    expect(await assertReflow(page)).toEqual([]);
  });
});

test.describe("layout rule", () => {
  test("layout_should_BeMobileAt844x390Coarse_And_DesktopAt1180x820CoarseWithViewportHeight470", async ({ browser }) => {
    const landscape = await browser.newContext({ viewport: { width: 844, height: 390 }, isMobile: true, hasTouch: true });
    const p1 = await landscape.newPage();
    const f1 = await FilesTabView.create(p1);
    await f1.goto();
    expect(await f1.layout).toBe("mobile");
    await landscape.close();

    const tablet = await browser.newContext({ viewport: { width: 1180, height: 820 }, isMobile: true, hasTouch: true });
    const p2 = await tablet.newPage();
    const f2 = await FilesTabView.create(p2);
    await f2.goto();
    expect(await f2.layout).toBe("desktop");
    await p2.setViewportSize({ width: 1180, height: 470 });
    expect(await f2.layout).toBe("desktop");
    await tablet.close();
  });

  test("tablet_should_UseDesktopLayoutWithTouchSizing_When_820x1180CoarsePointer", async ({ browser }) => {
    const ctx = await browser.newContext({ viewport: { width: 820, height: 1180 }, isMobile: true, hasTouch: true });
    const page = await ctx.newPage();
    const files = await FilesTabView.create(page);
    await files.goto();
    expect(await files.layout).toBe("desktop");
    await files.treeFile("a.txt").click();
    await expect(files.bottomBar).toHaveCount(0);
    await expect(files.tabCountButton).toHaveCount(0);
    for (const name of ["Quick open", "Go to line", "Search"]) {
      const box = (await page.getByRole("button", { name, exact: true }).first().boundingBox())!;
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    await ctx.close();
  });
});
