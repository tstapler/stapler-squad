// @feature session:create, alias:settings
/**
 * E2E tests for the Alias Settings Manager UI.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 */

import { test, expect } from "@playwright/test";
import { SettingsPage } from "./pages/SettingsPage";

const BASE_URL = process.env.BASE_URL ?? "http://localhost:8544";

test.describe("alias-settings", () => {
  let settings: SettingsPage;

  test.beforeEach(async ({ page }) => {
    settings = new SettingsPage(page);
    await settings.goto();
  });

  test("AC-01 create alias with name and path only", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName("e2e-create");
    await page.getByLabel("Path").fill("~/code/e2e");
    await settings.saveAlias();
    await expect(page.getByTestId("alias-row-e2e-create")).toBeVisible();
  });

  test("AC-02 edit alias description", async ({ page }) => {
    // Pre-create alias
    await settings.clickNewAlias();
    await settings.fillAliasName("e2e-edit");
    await settings.saveAlias();

    // Edit it
    await settings.clickEditAlias("e2e-edit");
    await settings.fillAliasDescription("Updated E2E");
    await settings.saveAlias();
    await expect(page.getByTestId("alias-row-e2e-edit")).toContainText("Updated E2E");
  });

  test("AC-03 delete alias via inline confirmation", async ({ page }) => {
    // Pre-create alias
    await settings.clickNewAlias();
    await settings.fillAliasName("e2e-delete");
    await settings.saveAlias();

    // Delete it
    await page.getByTestId("alias-delete-e2e-delete").click();
    await expect(page.getByTestId("alias-confirm-delete-e2e-delete")).toBeVisible();
    await page.getByTestId("alias-confirm-delete-e2e-delete").click();
    await expect(page.getByTestId("alias-row-e2e-delete")).not.toBeVisible();
  });

  test("AC-04 add environment variable via Advanced section", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName("e2e-envvar");
    await page.getByLabel("Advanced options").check();
    await page.getByRole("button", { name: "Add Variable" }).click();
    await page.getByLabel("Environment variable 1 key").fill("E2E_VAR");
    await page.getByLabel("Environment variable 1 value").fill("hello");
    await settings.saveAlias();
    await expect(page.getByTestId("alias-row-e2e-envvar")).toBeVisible();
  });

  test("AC-05 add tag via tag input Enter key", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName("e2e-tags");
    await page.getByPlaceholder("Add a tag...").fill("e2e-tag");
    await page.getByPlaceholder("Add a tag...").press("Enter");
    await expect(page.getByText("e2e-tag")).toBeVisible();
  });

  test("AC-06 cancel does not persist data", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName("should-not-exist");
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByTestId("alias-row-should-not-exist")).not.toBeVisible();
  });

  test("AC-09 save with empty name shows inline error", async ({ page }) => {
    await settings.clickNewAlias();
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("Name is required.")).toBeVisible();
  });

  test("AC-10 save with invalid name shows regex error", async ({ page }) => {
    await settings.clickNewAlias();
    await page.getByLabel("Name").fill("my project");
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText(/letters, digits, hyphens, and underscores/i)).toBeVisible();
  });

  test("AC-18 success banner appears after save", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName("e2e-banner");
    await settings.saveAlias();
    await expect(page.getByText(/Alias "@e2e-banner" saved/)).toBeVisible();
  });
});
