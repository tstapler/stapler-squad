// @feature slack:get-config, slack:update-config, slack:test-webhook
/**
 * E2E tests for the Slack Notification Settings panel.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * These tests use a syntactically-valid but non-functional Slack webhook URL
 * (https://hooks.slack.com/services/T0/B0/E2ETEST) — "Send test message"
 * against it is expected to fail (no real Slack workspace behind it), which
 * is fine: the test only asserts the result region updates with a
 * success-or-failure outcome, not which one.
 */

import { test, expect } from "@playwright/test";
import { SettingsPage } from "./pages/SettingsPage";

const TEST_WEBHOOK_URL = "https://hooks.slack.com/services/T0/B0/E2ETEST";

test.describe("slack-notifications", () => {
  let settings: SettingsPage;

  test.beforeEach(async ({ page }) => {
    settings = new SettingsPage(page);
    await settings.goto();
    await settings.selectTab("Appearance");
  });

  test("configures and verifies webhook in three actions with zero reloads", async ({
    page,
  }) => {
    const urlField = page.getByLabel("Webhook URL");
    await urlField.fill(TEST_WEBHOOK_URL);

    await page.getByRole("button", { name: "Send test message" }).click();

    const result = page.getByTestId("slack-test-webhook-result");
    await expect(result).toHaveText(/./, { timeout: 10000 });
  });

  test("shows specific inline error and blocks save for malformed webhook URL", async ({
    page,
  }) => {
    const urlField = page.getByLabel("Webhook URL");
    await urlField.fill("not-a-url");
    await urlField.blur();

    const error = page.getByText(
      /expected https:\/\/hooks\.slack\.com\/services\//,
    );
    await expect(error).toBeVisible();
    await expect(urlField).toHaveAttribute("aria-invalid", "true");
    await expect(page.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  test("recovers from invalid URL without navigation", async ({ page }) => {
    const urlField = page.getByLabel("Webhook URL");
    await urlField.fill("not-a-url");
    await urlField.blur();
    await expect(
      page.getByText(/expected https:\/\/hooks\.slack\.com\/services\//),
    ).toBeVisible();

    await urlField.fill(TEST_WEBHOOK_URL);
    await expect(
      page.getByText(/expected https:\/\/hooks\.slack\.com\/services\//),
    ).not.toBeVisible();
    await expect(page.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  test("toggle disabled until webhook configured", async ({ page }) => {
    const checkbox = page.getByLabel("Notify on new review-queue item");
    await expect(checkbox).toBeDisabled();

    await page.getByLabel("Webhook URL").fill(TEST_WEBHOOK_URL);
    await expect(checkbox).toBeEnabled();
  });

  test("shows dismissable warning when notify toggle is on and dashboard base URL is empty", async ({
    page,
  }) => {
    await page.getByLabel("Webhook URL").fill(TEST_WEBHOOK_URL);
    await page.getByLabel("Notify on new review-queue item").check();

    const warning = page.getByText(/may not work outside your home network/);
    await expect(warning).toBeVisible();

    await page.getByRole("button", { name: "Dismiss notice" }).click();
    await expect(warning).not.toBeVisible();
  });
});
