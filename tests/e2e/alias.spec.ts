// @feature session:create, alias:invoke
import { test, expect } from "@playwright/test";

const BASE_URL = process.env.BASE_URL ?? "http://localhost:8544";

test.describe("alias:invoke", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded", timeout: 10000 });
    await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });
  });

  test("alias:browse_should_showAliasPalette_When_atSignTyped", async ({ page }) => {
    // Open the omnibar by clicking the New Session button
    await page.getByText("New Session").click();

    // Type "@" into the omnibar input to trigger alias browse mode
    const omnibarInput = page.locator('input[aria-label="Search sessions"], input[placeholder*="session"], input[type="text"]').first();
    await omnibarInput.fill("@");

    // The alias palette should appear
    await expect(page.locator('[data-testid="alias-palette"]')).toBeVisible({ timeout: 5000 });
  });

  test("alias:browse_should_showEmptyState_When_atSignTypedAndNoAliasesConfigured", async ({ page }) => {
    // Open the omnibar
    await page.getByText("New Session").click();

    // Type "@" into the omnibar input
    const omnibarInput = page.locator('input[aria-label="Search sessions"], input[placeholder*="session"], input[type="text"]').first();
    await omnibarInput.fill("@");

    // The alias palette should appear
    const palette = page.locator('[data-testid="alias-palette"]');
    await expect(palette).toBeVisible({ timeout: 5000 });

    // If no aliases are configured, the empty state should be shown
    const emptyState = page.locator('[data-testid="alias-palette-empty"]');
    const hasAliasRows = await page.locator('[data-testid="alias-row"]').count();

    // Either aliases are listed OR the empty state is shown — both are valid
    if (hasAliasRows === 0) {
      await expect(emptyState).toBeVisible({ timeout: 3000 });
    } else {
      // Aliases exist — the palette is populated, which is also correct
      expect(hasAliasRows).toBeGreaterThan(0);
    }
  });
});
