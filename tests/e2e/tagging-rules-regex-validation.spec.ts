// @feature session:update
/**
 * E2E coverage for the tagging-rule pattern field's regex-validation UX (design/ux.md
 * Surface 5, AC15, AC19–AC21; validation.md's "UX Acceptance Tests" table). Uses the
 * real UI; AC15 additionally intercepts the save RPC to prove client-side validation
 * blocks the network call rather than merely showing an error alongside it.
 */

import { test, expect } from "@playwright/test";
import { TaggingRulesPage } from "./pages/TaggingRulesPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const INVALID_PATTERN = "^(unterminated[";

test.describe("tagging-rules-regex-validation", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  });

  test("AC15: invalid pattern caught before save round-trip", async ({ page }) => {
    const rulesPage = new TaggingRulesPage(page);
    await rulesPage.goto();
    await rulesPage.openTab();
    await rulesPage.getAddButton().click();

    await rulesPage.getPatternInput().fill(INVALID_PATTERN);
    await rulesPage.getOutputTagInput().fill("Whatever");
    await rulesPage.getNameInput().fill(`e2e-regex-blocked-${Date.now()}`);

    let saveCalled = false;
    await page.route("**/api/session.v1.SessionService/UpsertTaggingRule", async (route) => {
      saveCalled = true;
      await route.continue();
    });

    await rulesPage.getSaveButton().click();

    await expect(rulesPage.getPatternError()).toBeVisible();
    expect(saveCalled).toBe(false);
    await expect(rulesPage.getPatternInput()).toBeFocused();
  });

  test("AC19: invalid regex shows inline error on blur, before any Save click", async ({ page }) => {
    const rulesPage = new TaggingRulesPage(page);
    await rulesPage.goto();
    await rulesPage.openTab();
    await rulesPage.getAddButton().click();

    await expect(rulesPage.getPatternError()).toHaveCount(0);
    await rulesPage.getPatternInput().fill(INVALID_PATTERN);
    await rulesPage.getPatternInput().blur();

    await expect(rulesPage.getPatternError()).toBeVisible();
  });

  test("AC20: error is programmatically associated via aria-invalid/aria-describedby", async ({ page }) => {
    const rulesPage = new TaggingRulesPage(page);
    await rulesPage.goto();
    await rulesPage.openTab();
    await rulesPage.getAddButton().click();

    await rulesPage.getPatternInput().fill(INVALID_PATTERN);
    await rulesPage.getPatternInput().blur();

    const input = rulesPage.getPatternInput();
    await expect(input).toHaveAttribute("aria-invalid", "true");
    const describedBy = await input.getAttribute("aria-describedby");
    expect(describedBy).toBeTruthy();
    await expect(page.locator(`#${describedBy}`)).toBeVisible();
    await expect(page.locator(`#${describedBy}`)).toHaveText(await rulesPage.getPatternError().textContent() as string);
  });

  test("AC21: correcting the pattern clears error state in the same render", async ({ page }) => {
    const rulesPage = new TaggingRulesPage(page);
    await rulesPage.goto();
    await rulesPage.openTab();
    await rulesPage.getAddButton().click();

    const input = rulesPage.getPatternInput();
    await input.fill(INVALID_PATTERN);
    await input.blur();
    await expect(rulesPage.getPatternError()).toBeVisible();
    await expect(input).toHaveAttribute("aria-invalid", "true");

    await input.fill("^bugfix/");
    await expect(rulesPage.getPatternError()).toHaveCount(0);
    await expect(input).not.toHaveAttribute("aria-invalid", "true");
  });
});
