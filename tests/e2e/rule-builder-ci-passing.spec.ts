// @feature approval:upsert-rule
/**
 * E2E coverage for the "Require CI passing" checkbox in the rule builder (AC6/Task
 * 1.1.4a; validation.md UX criteria 3 and 10). Uses the real UI + real UpsertApprovalRule
 * RPC — no network mocking needed, unlike ci-status-badge.spec.ts, since this flow has no
 * dependency on live-only (unpersisted) GitHub fields.
 */

import { test, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function deleteRuleByName(request: import("@playwright/test").APIRequestContext, name: string) {
  const resp = await request.post(`${BASE_URL}/api/session.v1.SessionService/ListApprovalRules`, {
    headers: { "Content-Type": "application/json" },
    data: {},
  });
  const body = (await resp.json()) as { rules?: Array<{ id: string; name: string }> };
  const match = body.rules?.find((r) => r.name === name);
  if (match) {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/DeleteApprovalRule`, {
      headers: { "Content-Type": "application/json" },
      data: { id: match.id },
    });
  }
}

test.describe("rule-builder-ci-passing", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  });

  test("RuleBuilder_should_SubmitRequireCiPassingTrue_When_CheckboxCheckedAndSaved", async ({ page, request }) => {
    const ruleName = `e2e-ci-passing-rule-${Date.now()}`;
    try {
      await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

      await page.getByTestId("add-rule-button").click();
      await page.getByTestId("form-name-input").fill(ruleName);
      await page.getByTestId("form-tool-name-input").fill("Bash");
      await page.getByTestId("require-ci-passing-checkbox").getByRole("checkbox").check();
      await page.getByRole("button", { name: /Save Rule/ }).click();

      // Form closes on successful save.
      await expect(page.getByTestId("form-name-input")).not.toBeVisible({ timeout: 10000 });

      // Verify the submitted payload actually carried requireCiPassing: true (not just
      // that the checkbox visually toggled) by reading it back from the real RPC.
      await expect(async () => {
        const resp = await request.post(`${BASE_URL}/api/session.v1.SessionService/ListApprovalRules`, {
          headers: { "Content-Type": "application/json" },
          data: {},
        });
        const body = (await resp.json()) as { rules?: Array<{ name: string; requireCiPassing?: boolean }> };
        const created = body.rules?.find((r) => r.name === ruleName);
        expect(created?.requireCiPassing).toBe(true);
      }).toPass({ timeout: 5000 });
    } finally {
      await deleteRuleByName(request, ruleName);
    }
  });

  test("RuleBuilder_should_RevertCheckboxState_When_FormCancelled", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    await page.getByTestId("add-rule-button").click();
    const checkbox = page.getByTestId("require-ci-passing-checkbox").getByRole("checkbox");
    await checkbox.check();
    await expect(checkbox).toBeChecked();

    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByTestId("form-name-input")).not.toBeVisible();

    await page.getByTestId("add-rule-button").click();
    await expect(page.getByTestId("require-ci-passing-checkbox").getByRole("checkbox")).not.toBeChecked();
  });
});
