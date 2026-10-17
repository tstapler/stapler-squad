// @feature files:tabs
// Story 1.3.6b: the Explorer|Search panel handle is a keyboard-operable separator. The axe cases for
// U-A11Y-1/2/3/5 are added to this file by Story 1.5.3b. Written against the real Files tab; not yet run
// in CI at the time of writing (needs a built server).
import { test, expect } from "@playwright/test";
import { FilesTabView } from "./pages/files-tab.page";

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
