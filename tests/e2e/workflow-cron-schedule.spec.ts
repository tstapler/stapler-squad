// @feature workflows-management, workflow-cron-schedule-input
import { test, expect } from "@playwright/test";

const BASE_URL = process.env.BASE_URL ?? "http://localhost:8544";

test.describe("workflow-cron-schedule-input", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(`${BASE_URL}/workflows`, { waitUntil: "domcontentloaded", timeout: 10000 });
    await page.getByRole("button", { name: "+ New Workflow" }).click();
    await expect(page.getByLabel("Slug", { exact: false })).toBeVisible({ timeout: 5000 });
  });

  async function fillRequiredFields(page: import("@playwright/test").Page, slug: string) {
    await page.getByLabel("Slug", { exact: false }).fill(slug);
    await page.getByLabel("Name", { exact: false }).fill(`Cron e2e ${slug}`);
    await page.getByLabel("Command / Prompt", { exact: false }).fill("echo hello");
    await page.getByLabel("Target Directory", { exact: false }).fill("/tmp");
  }

  test("workflow-cron-schedule-input_should_acceptRawCronText_When_powerUserTypesInAdvancedMode", async ({ page }) => {
    await fillRequiredFields(page, `cron-e2e-adv-${Date.now()}`);

    // Simple mode is the default; switch to Advanced to type raw cron directly.
    await page.getByLabel("Advanced").check();
    const rawInput = page.getByLabel("Advanced cron expression");
    await rawInput.fill("0 9 * * 1-5");

    await expect(page.getByText(/Monday through Friday/i)).toBeVisible();
    await expect(page.getByTestId("wf-cron-error")).toHaveCount(0);

    await page.getByRole("button", { name: "Create Workflow" }).click();
    await expect(page.getByRole("heading", { name: "New Workflow" })).toHaveCount(0, { timeout: 10000 });
  });

  test("workflow-cron-schedule-input_should_buildCronFromDropdowns_When_userHasNoCronKnowledge", async ({ page }) => {
    await fillRequiredFields(page, `cron-e2e-simple-${Date.now()}`);

    // Simple mode is the default landing state — no cron syntax typed at all.
    await expect(page.getByLabel("Simple")).toBeChecked();
    await page.getByLabel("Frequency").selectOption("weekly");
    await page.getByLabel("Day of week").selectOption("1");
    await page.getByLabel("Time").fill("09:00");

    await expect(page.getByText(/09:00 AM.*Monday|Monday.*09:00 AM/i)).toBeVisible();

    await page.getByRole("button", { name: "Create Workflow" }).click();
    await expect(page.getByRole("heading", { name: "New Workflow" })).toHaveCount(0, { timeout: 10000 });
  });

  test("workflow-cron-schedule-input_should_blockSubmit_When_advancedExpressionIsInvalid", async ({ page }) => {
    await fillRequiredFields(page, `cron-e2e-invalid-${Date.now()}`);

    await page.getByLabel("Advanced").check();
    await page.getByLabel("Advanced cron expression").fill("99 9 * * *");
    await expect(page.getByTestId("wf-cron-error")).toBeVisible();

    await page.getByRole("button", { name: "Create Workflow" }).click();
    // Still on the form — submission was blocked client-side, no backend round-trip.
    await expect(page.getByLabel("Slug", { exact: false })).toBeVisible();
  });
});
