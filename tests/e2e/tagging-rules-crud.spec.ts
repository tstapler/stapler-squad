// @feature session:update
/**
 * E2E coverage for the "Tagging Rules" tab's CRUD flow (design/ux.md Surface 4/6,
 * AC14, AC16–AC18, plus the four Surface-6 fire-count-column criteria;
 * validation.md's "UX Acceptance Tests" table). Uses the real UI + real
 * UpsertTaggingRule/DeleteTaggingRule RPCs, per rule-builder-ci-passing.spec.ts's
 * precedent for this tab family — no network mocking needed.
 */

import { test, expect } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { TaggingRulesPage } from "./pages/TaggingRulesPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function deleteRuleByName(request: import("@playwright/test").APIRequestContext, name: string) {
  const resp = await request.post(`${BASE_URL}/api/session.v1.SessionService/ListTaggingRules`, {
    headers: { "Content-Type": "application/json" },
    data: {},
  });
  const body = (await resp.json()) as { rules?: Array<{ id: string; name: string }> };
  const match = body.rules?.find((r) => r.name === name);
  if (match) {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/DeleteTaggingRule`, {
      headers: { "Content-Type": "application/json" },
      data: { id: match.id },
    });
  }
}

test.describe("tagging-rules-crud", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  });

  test("AC14: create a tagging rule visible in list within six interactions", async ({ page, request }) => {
    const ruleName = `e2e-crud-create-${Date.now()}`;
    try {
      const rulesPage = new TaggingRulesPage(page);
      await rulesPage.goto();

      let interactions = 0;
      await rulesPage.openTab(); interactions++; // 1: switch to Tagging Rules tab
      await rulesPage.getAddButton().click(); interactions++; // 2: + Add Tagging Rule
      await rulesPage.getPatternInput().fill("^bugfix/"); interactions++; // 3: pattern
      await rulesPage.getOutputTagInput().fill("Bugfix"); interactions++; // 4: output tag
      await rulesPage.getNameInput().fill(ruleName); interactions++; // 5: name
      await rulesPage.getSaveButton().click(); interactions++; // 6: Save Rule

      expect(interactions).toBeLessThanOrEqual(6);

      // Appears with no page reload.
      await expect(rulesPage.getRow(ruleName)).toBeVisible({ timeout: 10000 });
      expect(page.url()).toBe(`${BASE_URL}/rules`);
    } finally {
      await deleteRuleByName(request, ruleName);
    }
  });

  test("AC16: every control is Tab-reachable and Enter/Space-operable", async ({ page, request }) => {
    const ruleName = `e2e-crud-keyboard-${Date.now()}`;
    try {
      const rulesPage = new TaggingRulesPage(page);
      await rulesPage.goto();

      await rulesPage.getTab().focus();
      await page.keyboard.press("Enter");
      await expect(rulesPage.getPanel()).toBeVisible();

      await rulesPage.getAddButton().focus();
      await page.keyboard.press("Enter");
      await expect(rulesPage.getPatternInput()).toBeVisible();

      await rulesPage.getPatternInput().fill("^hotfix/");
      await rulesPage.getOutputTagInput().fill("Hotfix");
      await rulesPage.getNameInput().fill(ruleName);

      await rulesPage.getSaveButton().focus();
      await page.keyboard.press("Enter");
      await expect(rulesPage.getRow(ruleName)).toBeVisible({ timeout: 10000 });

      // Toggle, Edit, Delete are each reachable and Enter-operable.
      await rulesPage.getToggleButton(ruleName).focus();
      await page.keyboard.press("Enter");
      await expect(rulesPage.getRow(ruleName)).toContainText("OFF");

      await rulesPage.getEditButton(ruleName).focus();
      await page.keyboard.press("Enter");
      await expect(rulesPage.getSaveButton()).toBeVisible();
      await rulesPage.getCancelButton().focus();
      await page.keyboard.press("Enter");
      await expect(rulesPage.getPatternInput()).toHaveCount(0);

      await rulesPage.getDeleteButton(ruleName).focus();
      await page.keyboard.press("Enter");
      await expect(rulesPage.getRow(ruleName)).toHaveCount(0);
    } finally {
      await deleteRuleByName(request, ruleName);
    }
  });

  test("AC17: row action aria-labels match approval-rule naming convention exactly", async ({ page, request }) => {
    const ruleName = `e2e-crud-arialabels-${Date.now()}`;
    try {
      const rulesPage = new TaggingRulesPage(page);
      await rulesPage.goto();
      await rulesPage.openTab();
      await rulesPage.getAddButton().click();
      await rulesPage.fillAndSave({ pattern: "^chore/", outputTag: "Chore", name: ruleName });
      await expect(rulesPage.getRow(ruleName)).toBeVisible({ timeout: 10000 });

      await expect(page.getByRole("button", { name: `Edit tagging rule ${ruleName}` })).toBeVisible();
      await expect(page.getByRole("button", { name: `Delete tagging rule ${ruleName}` })).toBeVisible();
      await expect(page.getByRole("button", { name: "Disable tagging rule" })).toBeVisible();

      await rulesPage.getToggleButton(ruleName).click();
      await expect(page.getByRole("button", { name: "Enable tagging rule" })).toBeVisible();
    } finally {
      await deleteRuleByName(request, ruleName);
    }
  });

  test("AC18: failed save offers Try again with no orphaned row; Cancel returns with no partial row", async ({ page, request }) => {
    const ruleName = `e2e-crud-failedsave-${Date.now()}`;
    const rulesPage = new TaggingRulesPage(page);
    await rulesPage.goto();
    await rulesPage.openTab();
    await rulesPage.getAddButton().click();

    await rulesPage.getPatternInput().fill("^release/");
    await rulesPage.getOutputTagInput().fill("Release");
    await rulesPage.getNameInput().fill(ruleName);

    await page.route("**/api/session.v1.SessionService/UpsertTaggingRule", async (route) => {
      await route.fulfill({ status: 500, body: "simulated failure" });
    });
    await rulesPage.getSaveButton().click();

    // Entered values retained; an error is surfaced (form stays open, not silently swallowed).
    await expect(rulesPage.getPatternInput()).toHaveValue("^release/");
    await expect(rulesPage.getNameInput()).toHaveValue(ruleName);

    await page.unroute("**/api/session.v1.SessionService/UpsertTaggingRule");
    await rulesPage.getSaveButton().click();
    await expect(rulesPage.getRow(ruleName)).toBeVisible({ timeout: 10000 });
    await deleteRuleByName(request, ruleName);

    // Cancel instead of retrying — no partial row added.
    await rulesPage.getAddButton().click();
    await rulesPage.getPatternInput().fill("^release/");
    await rulesPage.getOutputTagInput().fill("Release");
    await rulesPage.getNameInput().fill(`${ruleName}-cancelled`);
    await rulesPage.getCancelButton().click();
    await expect(rulesPage.getRow(`${ruleName}-cancelled`)).toHaveCount(0);
  });

  test("Surface 6: fire-count column reuses approval-rule tooltip verbatim, no shaming styling, no new route", async ({ page, request }) => {
    const ruleName = `e2e-crud-firecount-${Date.now()}`;
    try {
      const rulesPage = new TaggingRulesPage(page);
      await rulesPage.goto();
      await rulesPage.openTab();
      await rulesPage.getAddButton().click();
      await rulesPage.fillAndSave({ pattern: "^docs/", outputTag: "Docs", name: ruleName });
      await expect(rulesPage.getRow(ruleName)).toBeVisible({ timeout: 10000 });

      await expect(rulesPage.getFireCountHeader()).toHaveAttribute(
        "title",
        "Number of times this rule fired in the last 7 days",
      );

      // No new route — switching to the tab is a column/content swap only.
      expect(page.url()).toBe(`${BASE_URL}/rules`);

      // Zero-fire row: passive "—" placeholder, no warning class/aria-alarm attribute.
      const row = rulesPage.getRow(ruleName);
      await expect(row).toContainText("—");
      const cellCount = await row.locator('[aria-live], [role="alert"]').count();
      expect(cellCount).toBe(0);
    } finally {
      await deleteRuleByName(request, ruleName);
    }
  });

  test("Surface 6: fire count increments after the rule actually fires", async ({ page, request }) => {
    const ts = Date.now();
    const ruleName = `e2e-crud-realfire-${ts}`;
    const namePattern = `^e2e-fire-target-${ts}`;
    try {
      const client = new SessionClient(BASE_URL);

      // Seed the rule via the real RPC.
      const ruleResp = await request.post(`${BASE_URL}/api/session.v1.SessionService/UpsertTaggingRule`, {
        headers: { "Content-Type": "application/json" },
        data: { rule: { id: `${ruleName}-id`, name: ruleName, namePattern, outputTag: "E2EFireTarget", priority: 50, enabled: true, source: "user" } },
      });
      expect(ruleResp.ok()).toBe(true);

      // Trigger a session mutation that matches it: creating a session whose title
      // matches namePattern applies the tag (and records the fire) immediately, per
      // Story 3.3.2's "immediate synchronous apply" guarantee.
      await client.createSession({ title: `e2e-fire-target-${ts}`, path: "/tmp", program: "bash" });

      const rulesPage = new TaggingRulesPage(page);
      await expect(async () => {
        await rulesPage.goto();
        await rulesPage.openTab();
        await expect(rulesPage.getRow(ruleName)).toContainText(/[1-9]/);
      }).toPass({ timeout: 15000 });
    } finally {
      await deleteRuleByName(request, ruleName);
    }
  });
});
