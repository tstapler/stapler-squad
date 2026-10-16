// @feature import:preview, import:commit, import:confirm_kill, import:cancel_pending_kill
import { test, expect } from "@playwright/test";
import { ImportSessionsPage } from "./pages/ImportSessionsPage";

test.describe("import-external-session", () => {
  test("import-external-session_should_navigateToImportPage_When_navImportClicked", async ({
    page,
  }) => {
    await page.goto("/", { waitUntil: "domcontentloaded", timeout: 10000 });
    await page.getByRole("link", { name: "Import" }).click();
    await expect(page).toHaveURL(/\/sessions\/import\/?$/);

    const importPage = new ImportSessionsPage(page);
    await importPage.waitForPageLoad();
    await expect(importPage.container).toBeVisible();
    await expect(importPage.panel).toBeVisible();
  });

  test("import-external-session_should_showEmptyState_When_noExternalSessionsDiscovered", async ({
    page,
  }) => {
    const importPage = new ImportSessionsPage(page);
    await importPage.goto();
    await importPage.waitForPageLoad();

    await expect(importPage.emptyState).toBeVisible({ timeout: 5000 });
    await expect(importPage.getRows()).toHaveCount(0);
  });
});
