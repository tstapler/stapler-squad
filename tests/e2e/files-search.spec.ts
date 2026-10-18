// @feature files:content-search, files:tabs
// Story 1.4.3: the flagship search-to-line flow on the real Files tab. Needs a built server; see global-setup.ts.
import { test, expect } from "@playwright/test";
import { FilesTabView, seedSearchFixture } from "./pages/files-tab.page";

test.describe("content search", () => {
  test("search_should_LandOnHitLine_In3Interactions_When_QueryTypedAndEnterPressed", async ({ page }) => {
    const files = await FilesTabView.create(page, seedSearchFixture);
    await files.goto();

    await files.openSearch(); // interaction 1
    await files.typeQuery("needle_fn", "a.go"); // interaction 2
    await page.keyboard.press("Enter"); // interaction 3 (Enter in the query box opens the first hit)

    await expect(files.editorTab("a.go")).toHaveAttribute("aria-selected", "true");
    await expect.poll(() => files.activeLineText()).toContain("needle_fn");
    await expect(page.getByText("func needle_fn() {}", { exact: true })).toBeInViewport();
  });

  test("search_should_OfferHitStepping_When_HitOpened", async ({ page }) => {
    const files = await FilesTabView.create(page, seedSearchFixture);
    await files.goto();
    await files.openSearch();
    await files.typeQuery("needle_fn", "a.go");
    await page.keyboard.press("Enter");
    await expect(files.quickfixChip).toBeVisible();
    await expect(files.quickfixChip).toContainText("Hit 1/1");
  });
});
