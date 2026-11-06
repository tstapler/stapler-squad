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
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();

// The Global Defaults section has its own tag/env-var controls; scope to the alias form.
const aliasForm = (page: import("@playwright/test").Page) => page.getByLabel(/^(New|Edit) Alias/);

test.describe("alias-settings", () => {
  let settings: SettingsPage;
  // chromium and chromium-dom share one server; fixed alias names collide across projects.
  let sfx: string;

  test.beforeEach(async ({ page }, testInfo) => {
    sfx = testInfo.project.name;
    settings = new SettingsPage(page);
    await settings.goto();
  });

  test("AC-01 create alias with name and path only", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName(`e2e-create-${sfx}`);
    await page.getByLabel("Path").fill("~/code/e2e");
    await settings.saveAlias();
    await expect(page.getByTestId(`alias-row-e2e-create-${sfx}`)).toBeVisible();
  });

  test("AC-02 edit alias description", async ({ page }) => {
    // Pre-create alias
    await settings.clickNewAlias();
    await settings.fillAliasName(`e2e-edit-${sfx}`);
    await settings.saveAlias();

    // Edit it
    await settings.clickEditAlias(`e2e-edit-${sfx}`);
    await settings.fillAliasDescription("Updated E2E");
    await settings.saveAlias();
    await expect(page.getByTestId(`alias-row-e2e-edit-${sfx}`)).toContainText("Updated E2E");
  });

  test("AC-03 delete alias via inline confirmation", async ({ page }) => {
    // Pre-create alias
    await settings.clickNewAlias();
    await settings.fillAliasName(`e2e-delete-${sfx}`);
    await settings.saveAlias();

    // Delete it
    await page.getByTestId(`alias-delete-e2e-delete-${sfx}`).click();
    await expect(page.getByTestId(`alias-confirm-delete-e2e-delete-${sfx}`)).toBeVisible();
    await page.getByTestId(`alias-confirm-delete-e2e-delete-${sfx}`).click();
    await expect(page.getByTestId(`alias-row-e2e-delete-${sfx}`)).not.toBeVisible();
  });

  test("AC-04 add environment variable via Advanced section", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName(`e2e-envvar-${sfx}`);
    await page.getByLabel("Advanced").check();
    await aliasForm(page).getByRole("button", { name: "Add variable" }).click();
    await page.getByLabel("Environment variable key 1").fill("E2E_VAR");
    await page.getByLabel("Environment variable value 1").fill("hello");
    await settings.saveAlias();
    await expect(page.getByTestId(`alias-row-e2e-envvar-${sfx}`)).toBeVisible();
  });

  test("AC-05 add tag via tag input Enter key", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName(`e2e-tags-${sfx}`);
    await aliasForm(page).getByPlaceholder("Add a tag...").fill("e2e-tag");
    await aliasForm(page).getByPlaceholder("Add a tag...").press("Enter");
    await expect(aliasForm(page).getByRole("button", { name: "Remove tag e2e-tag" })).toBeVisible();
  });

  test("AC-06 cancel does not persist data", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName("should-not-exist");
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByTestId("alias-row-should-not-exist")).not.toBeVisible();
  });

  test("AC-09 save with empty name shows inline error", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.clickAliasFormSave();
    await expect(page.getByText("Name is required.")).toBeVisible();
  });

  test("AC-10 save with invalid name shows regex error", async ({ page }) => {
    await settings.clickNewAlias();
    await page.getByRole("textbox", { name: /^Name(?! prefix)/ }).fill("my project");
    await settings.clickAliasFormSave();
    await expect(page.getByText(/letters, digits, hyphens, and underscores/i)).toBeVisible();
  });

  test("AC-18 success banner appears after save", async ({ page }) => {
    await settings.clickNewAlias();
    await settings.fillAliasName(`e2e-banner-${sfx}`);
    await settings.saveAlias();
    await expect(page.getByText(new RegExp(`Alias "@e2e-banner-${sfx}" saved`))).toBeVisible();
  });
});
