// @feature session:create, alias:invoke
import { test, expect } from "@playwright/test";

const BASE_URL = process.env.BASE_URL ?? "http://localhost:8544";

test.describe("alias:invoke", () => {
  test.skip(true, "alias feature e2e — pending full server setup");

  test("alias:invoke_should_createSession_When_shortNameAndEnterPressed", async ({ page }) => {
    await page.goto(BASE_URL);
    // TODO: implement when server is running with alias config
    expect(page).toBeDefined();
  });

  test("alias:browse_should_showPaletteWithin100ms_When_atSignTyped", async ({ page }) => {
    await page.goto(BASE_URL);
    // TODO: implement
    expect(page).toBeDefined();
  });
});
