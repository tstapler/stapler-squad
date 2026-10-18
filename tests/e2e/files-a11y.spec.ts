// @feature files:tabs, files:mobile
// Story 1.3.6b: the Explorer|Search panel handle is a keyboard-operable separator. The axe cases for
// U-A11Y-1/2/3/5 are added to this file by Story 1.5.3b. Written against the real Files tab; not yet run
// in CI at the time of writing (needs a built server).
import { test, expect } from "@playwright/test";
import { FilesTabView, seedSearchFixture } from "./pages/files-tab.page";
import { assertReflow, axeAaViolations, tabOrderNames } from "./helpers/a11y";

// Wide enough that the tree and the editor show side by side; a narrow container shows one at a time.
test.use({ viewport: { width: 1900, height: 1000 } });

test.describe("Files panel separator (U-A11Y-4)", () => {
  test("separator_should_ResizeByKeyboard_And_CollapseUnchanged_When_Focused", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();

    const sep = files.separator;
    await expect(sep).toBeVisible();
    await expect(sep).toHaveAttribute("aria-orientation", "vertical");
    const widthOf = async () => Math.round((await files.treePane.boundingBox())!.width);
    const now = async () => Number(await sep.getAttribute("aria-valuenow"));
    const min = Number(await sep.getAttribute("aria-valuemin"));
    const max = Number(await sep.getAttribute("aria-valuemax"));
    expect(max).toBeGreaterThan(min);

    await sep.focus();
    await sep.press("Home");
    await expect.poll(now).toBe(min);
    expect(await widthOf()).toBe(min);

    await sep.press("ArrowRight");
    await expect.poll(now).toBe(min + 16);
    expect(await widthOf()).toBe(min + 16);

    await sep.press("Shift+ArrowRight");
    await expect.poll(now).toBe(min + 80);

    await sep.press("ArrowLeft");
    await expect.poll(now).toBe(min + 64);

    await sep.press("End");
    await expect.poll(now).toBe(max);
    expect(await widthOf()).toBe(max);

    // Collapse is unchanged: the button hides the handle, expand restores it.
    await page.getByRole("button", { name: "Collapse file tree panel" }).click();
    await expect(sep).toHaveCount(0);
    await page.getByRole("button", { name: "Expand file tree panel" }).click();
    await expect(sep).toBeVisible();
  });
});

test.describe("Files tab axe and keyboard (U-A11Y-1/2/3/5/8)", () => {
  test("filesTab_should_HaveZeroAxeAaViolations_When_DesktopResultsOpen", async ({ page }) => {
    const files = await FilesTabView.create(page, seedSearchFixture);
    await files.goto();
    await files.openSearch();
    await files.typeQuery("needle_fn", "a.go");
    expect(await axeAaViolations(page)).toEqual([]);
  });

  test.describe("mobile", () => {
    test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

    test("filesTab_should_HaveZeroAxeAaViolations_When_MobileResultsAndSheetOpen", async ({ page }) => {
      const files = await FilesTabView.create(page, seedSearchFixture);
      await files.goto();
      await files.bottomBarButton("Search").click();
      await files.searchQuery.fill("needle_fn");
      await files.searchResults.getByRole("treeitem").first().waitFor();
      expect(await axeAaViolations(page)).toEqual([]);
      await files.searchResults.getByRole("treeitem", { name: /pkg\/a\.go line 12/ }).first().click();
      await files.tabCountButton.click();
      expect(await axeAaViolations(page)).toEqual([]);
    });

    test("filesTab_should_ReflowWithoutLossAt320px_And_200PercentZoom", async ({ page }) => {
      await page.setViewportSize({ width: 320, height: 568 });
      const files = await FilesTabView.create(page);
      await files.goto();
      await page.addStyleTag({ content: "html { font-size: 200% }" });
      expect(await assertReflow(page)).toEqual([]);
      await files.treeFile("a.txt").click();
      expect(await assertReflow(page)).toEqual([]);
    });

    test("filesTab_should_TabThroughTopBarThenPaneThenBottomBar_When_KeyboardOnly", async ({ page }) => {
      const files = await FilesTabView.create(page);
      await files.goto();
      await files.treeFile("a.txt").click();
      await files.paneBack.focus();
      const names = await tabOrderNames(page, 3);
      expect(names[0]).toMatch(/^Open files, \d+$/);
      expect(names[1]).toBe("More actions");
    });
  });
});
