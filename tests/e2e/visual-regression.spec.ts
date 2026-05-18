// @feature session:list, review-queue:list
//
// Visual regression tests — one spec, 4 theme projects (visual-matrix,
// visual-cyberpunk77, visual-wh40k, visual-clean).
//
// To capture baselines (first run or after intentional visual changes):
//   npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-matrix
//   npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-cyberpunk77
//   npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-wh40k
//   npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-clean
//
// Snapshots are stored in tests/snapshots/{projectName}/... and committed to git.
// CI runs without --update-snapshots; any pixel diff > 1% fails the test.

import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
});

test("session list empty state", async ({ page }) => {
  await page.goto("/");
  // Use "load" instead of "networkidle" — live seeded sessions create persistent
  // WebSocket connections that prevent networkidle from ever resolving.
  await page.waitForLoadState("load");
  await page.waitForTimeout(500);
  await expect(page).toHaveScreenshot("session-list-empty.png", {
    maxDiffPixelRatio: 0.01,
    animations: "disabled",
  });
});

test("omnibar open", async ({ page }) => {
  await page.goto("/");
  await page.waitForLoadState("load");
  await page.waitForTimeout(500);
  await page.keyboard.press("Meta+k");
  await page.waitForSelector('[data-testid="omnibar-input"]', { timeout: 3000 }).catch(() => {});
  await expect(page).toHaveScreenshot("omnibar-open.png", {
    maxDiffPixelRatio: 0.01,
    animations: "disabled",
  });
});
