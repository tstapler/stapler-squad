// @feature files:tabs
// Story 1.3.5 / U-CHORD-1: each documented Files chord reaches its handler with focus inside the Files tab and is
// cancelled exactly once, and is left alone with focus outside it. Playwright injects keys below the browser's
// accelerator layer, so this proves the page handler, not real delivery; the owner's per-browser manual check
// (Chrome, Firefox, Safari) is a Story 2.5.2 checklist line. Runs on desktop Chromium (gating), Firefox and WebKit.
import { test, expect, type Page } from "@playwright/test";
import { FilesTabView } from "./pages/files-tab.page";

const MOD = process.platform === "darwin" ? "Meta" : "Control";

/** Counts keydowns and how many were cancelled, as seen after the app's own window listener ran. */
async function watchKeydowns(page: Page): Promise<void> {
  await page.evaluate(() => {
    const w = window as unknown as { __chord: { seen: number; prevented: number } };
    w.__chord = { seen: 0, prevented: 0 };
    window.addEventListener("keydown", (e) => {
      w.__chord.seen += 1;
      if (e.defaultPrevented) w.__chord.prevented += 1;
    });
  });
}
const tally = (page: Page) => page.evaluate(() => (window as unknown as { __chord: { seen: number; prevented: number } }).__chord);
const reset = (page: Page) => page.evaluate(() => { (window as unknown as { __chord: object }).__chord = { seen: 0, prevented: 0 }; });

test.describe("Files shortcuts", () => {
  test("shortcuts_should_ReachHandlerAndPreventDefaultOnce_When_FocusInsideFilesTabOnChromiumFirefoxWebkit", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await files.treeFile("long.txt").click();
    await watchKeydowns(page);

    // Alt+W closes the tab; open another first so the chord has something to act on.
    await files.treeFile("a.txt").click();
    await expect(page.getByRole("tab", { name: /^a\.txt/ })).toBeVisible();
    await files.searchBox.focus();
    await reset(page);
    await page.keyboard.press("Alt+KeyW");
    await expect(page.getByRole("tab", { name: /^a\.txt/ })).toHaveCount(0);
    expect(await tally(page)).toEqual({ seen: 1, prevented: 1 });

    // Alt+[ and Alt+] by physical key: Back, then Forward, each cancelled once.
    await files.searchBox.focus();
    await reset(page);
    await page.keyboard.press("Alt+BracketLeft");
    expect(await tally(page)).toEqual({ seen: 1, prevented: 1 });
    await reset(page);
    await page.keyboard.press("Alt+BracketRight");
    expect(await tally(page)).toEqual({ seen: 1, prevented: 1 });

    // Ctrl/Cmd+G opens go-to-line.
    await reset(page);
    await page.keyboard.press(`${MOD}+g`);
    await expect(page.getByRole("textbox", { name: "Go to line" })).toBeVisible();
    expect(await tally(page)).toEqual({ seen: 1, prevented: 1 });
    await page.keyboard.press("Escape");

    // Ctrl/Cmd+P opens the palette.
    await files.searchBox.focus();
    await reset(page);
    await page.keyboard.press(`${MOD}+p`);
    await expect(page.getByRole("combobox", { name: "Quick open file" })).toBeVisible();
    expect(await tally(page)).toEqual({ seen: 1, prevented: 1 });
  });

  test("shortcuts_should_NotBePrevented_When_FocusOutsideTheFilesTab", async ({ page }) => {
    const files = await FilesTabView.create(page);
    await files.goto();
    await files.treeFile("a.txt").click();
    // Another session tab hides the Files panel; the chords must belong to the browser again.
    await page.getByRole("tab", { name: "Info", exact: true }).click();
    await watchKeydowns(page);
    for (const chord of ["Alt+BracketLeft", "Alt+BracketRight", "Alt+KeyW", `${MOD}+g`, `${MOD}+p`, `${MOD}+Shift+f`, "F4"]) {
      await reset(page);
      await page.keyboard.press(chord);
      expect(await tally(page), chord).toMatchObject({ prevented: 0 });
    }
    await expect(page.getByRole("combobox", { name: "Quick open file" })).toHaveCount(0);
  });
});
