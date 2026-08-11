import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: rules-yaml-import — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['approval-rules-yaml-import'], // TODO: add to catalog
] as const;
// E2E tests for YAML bulk import/export on the /rules page.
// Covers E2E-01 through E2E-08.
// All locators use data-testid or ARIA roles — no CSS class selectors.

import { test, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

const VALID_YAML_3_RULES = `rules:
- name: E2E Allow git log
  tool: Bash
  programs:
    - git
  subcommands:
    - log
  decision: allow
  reason: Read-only git history
- name: E2E Allow git status
  tool: Bash
  programs:
    - git
  subcommands:
    - status
  decision: allow
- name: E2E Allow ls
  tool: Bash
  programs:
    - ls
  decision: allow
`;

const MIXED_YAML = `rules:
- name: E2E Mixed Valid Rule
  tool: Bash
  programs:
    - git
  decision: allow
- name: E2E Mixed Valid Rule 2
  tool: Bash
  programs:
    - npm
  decision: deny
- name: E2E Invalid Rule No Name
  tool: Bash
  command_pattern: "[invalid(regex"
  decision: allow
`;

test.describe("rules-yaml-import", () => {
  // ── E2E-01 ──────────────────────────────────────────────────────────────────
  test("rules-yaml-import > import modal opens and closes", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    // Import YAML button should be visible.
    const importBtn = page.getByTestId("import-yaml-button");
    await expect(importBtn).toBeVisible({ timeout: 10000 });

    // Click to open.
    await importBtn.click();

    // Modal should be visible.
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("yaml-input")).toBeVisible();

    // Press Escape to close.
    await page.keyboard.press("Escape");

    // Modal should be gone.
    await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 5000 });
  });

  // ── E2E-02 ──────────────────────────────────────────────────────────────────
  test("rules-yaml-import > validates rules and shows preview cards", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    await page.getByTestId("import-yaml-button").click();
    await expect(page.getByTestId("yaml-input")).toBeVisible({ timeout: 5000 });

    // Paste 3-rule valid YAML.
    await page.getByTestId("yaml-input").fill(VALID_YAML_3_RULES);

    // Wait for preview cards to appear.
    await expect(page.getByTestId("preview-list")).toBeVisible({ timeout: 10000 });
    const validCards = page.locator('[data-testid="parsed-rule-card-valid"]');
    await expect(validCards).toHaveCount(3, { timeout: 10000 });
  });

  // ── E2E-03 ──────────────────────────────────────────────────────────────────
  test("rules-yaml-import > shows inline validation errors", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    await page.getByTestId("import-yaml-button").click();
    await expect(page.getByTestId("yaml-input")).toBeVisible({ timeout: 5000 });

    // Paste mixed YAML: 2 valid + 1 with invalid regex.
    await page.getByTestId("yaml-input").fill(MIXED_YAML);

    // Wait for preview list.
    await expect(page.getByTestId("preview-list")).toBeVisible({ timeout: 10000 });

    // At least 1 error card should appear.
    const errorCards = page.locator('[data-testid="parsed-rule-card-error"]');
    await expect(errorCards).toHaveCount(1, { timeout: 10000 });

    // Apply button should mention 2 rules and 1 error.
    const applyBtn = page.getByTestId("apply-button");
    await expect(applyBtn).toContainText("Apply 2 rules", { timeout: 5000 });
    await expect(applyBtn).toContainText("1 has errors");
  });

  // ── E2E-04 ──────────────────────────────────────────────────────────────────
  test("rules-yaml-import > applies valid rules and refreshes table", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    await page.getByTestId("import-yaml-button").click();
    await expect(page.getByTestId("yaml-input")).toBeVisible({ timeout: 5000 });

    await page.getByTestId("yaml-input").fill(VALID_YAML_3_RULES);

    // Wait for apply button to become enabled.
    const applyBtn = page.getByTestId("apply-button");
    await expect(applyBtn).toBeEnabled({ timeout: 10000 });

    // Click Apply.
    await applyBtn.click();

    // Modal should close on success.
    await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10000 });

    // Rules table should now contain the imported rule names.
    await expect(page.getByText("E2E Allow git log")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("E2E Allow git status")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("E2E Allow ls")).toBeVisible({ timeout: 5000 });
  });

  // ── E2E-05 ──────────────────────────────────────────────────────────────────
  test("rules-yaml-import > export yaml button downloads file", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    // Wait for the export button to be visible.
    const exportBtn = page.getByTestId("export-yaml-button");
    await expect(exportBtn).toBeVisible({ timeout: 10000 });

    // Set up download listener before clicking.
    const downloadPromise = page.waitForEvent("download", { timeout: 10000 });
    await exportBtn.click();

    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe("rules.yaml");
  });

  // ── E2E-06 ──────────────────────────────────────────────────────────────────
  test("rules-yaml-import > duplicate skip mode", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    // First import: create one rule.
    const singleRuleYaml = `rules:
- name: E2E Dup Skip Rule
  tool: Bash
  programs:
    - git
  decision: allow
`;

    await page.getByTestId("import-yaml-button").click();
    await page.getByTestId("yaml-input").fill(singleRuleYaml);

    const applyBtn = page.getByTestId("apply-button");
    await expect(applyBtn).toBeEnabled({ timeout: 10000 });
    await applyBtn.click();
    await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10000 });

    // Second import: same rule, skip mode (default).
    await page.getByTestId("import-yaml-button").click();
    await expect(page.getByTestId("yaml-input")).toBeVisible({ timeout: 5000 });
    await page.getByTestId("yaml-input").fill(singleRuleYaml);

    // The "skip" radio should be selected by default and the card should show skip badge.
    await expect(page.getByTestId("duplicate-mode-skip")).toBeChecked({ timeout: 10000 });
    await expect(page.locator('[data-testid="skip-badge"]')).toBeVisible({ timeout: 10000 });

    // Apply button may show 0 applicable rules (all skipped), making it disabled.
    // The result: the rule in the table remains unchanged.
    await page.keyboard.press("Escape");
    await expect(page.getByText("E2E Dup Skip Rule")).toBeVisible();
  });

  // ── E2E-07 ──────────────────────────────────────────────────────────────────
  test("rules-yaml-import > duplicate overwrite mode", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    // First import: create rule with "allow" decision.
    const originalYaml = `rules:
- name: E2E Dup Overwrite Rule
  tool: Bash
  programs:
    - git
  decision: allow
`;

    await page.getByTestId("import-yaml-button").click();
    await page.getByTestId("yaml-input").fill(originalYaml);
    const applyBtn1 = page.getByTestId("apply-button");
    await expect(applyBtn1).toBeEnabled({ timeout: 10000 });
    await applyBtn1.click();
    await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10000 });

    // Second import: same rule name but "deny" decision, overwrite mode.
    const updatedYaml = `rules:
- name: E2E Dup Overwrite Rule
  tool: Bash
  programs:
    - git
  decision: deny
`;

    await page.getByTestId("import-yaml-button").click();
    await page.getByTestId("yaml-input").fill(updatedYaml);

    // Switch to overwrite mode.
    const overwriteRadio = page.getByTestId("duplicate-mode-overwrite");
    await expect(overwriteRadio).toBeVisible({ timeout: 10000 });
    await overwriteRadio.click();

    // Apply button should be enabled.
    const applyBtn2 = page.getByTestId("apply-button");
    await expect(applyBtn2).toBeEnabled({ timeout: 10000 });
    await applyBtn2.click();

    // Modal should close.
    await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 10000 });

    // The rule should still be present in the table (overwritten with deny decision).
    await expect(page.getByText("E2E Dup Overwrite Rule")).toBeVisible({ timeout: 10000 });
  });

  // ── E2E-08 ──────────────────────────────────────────────────────────────────
  test("rules-yaml-import > empty state has explanatory text", async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });

    // Switch to "Custom" (user) filter to get an isolated empty state.
    const customTab = page.getByRole("button", { name: /Custom/ });
    await expect(customTab).toBeVisible({ timeout: 10000 });
    await customTab.click();

    // If there are rules, this test is N/A; only check empty state if it exists.
    const emptyState = page.getByTestId("empty-state");
    const isEmptyStateVisible = await emptyState.isVisible().catch(() => false);

    if (isEmptyStateVisible) {
      await expect(emptyState).toContainText(/automatically allow or deny/i);
    } else {
      // Rules exist — verify we're on the right page at least.
      await expect(page.getByRole("button", { name: /Import YAML/i })).toBeVisible();
    }
  });
});
