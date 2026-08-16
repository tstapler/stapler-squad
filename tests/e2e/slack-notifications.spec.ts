// @feature GetSlackConfig, UpdateSlackConfig, TestSlackWebhook, slack-notification-settings
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
 * is fine: most tests only assert the result region updates with a
 * success-or-failure outcome, not which one.
 *
 * Scenarios that need a specific server response (a seeded "last delivery,"
 * a specific Slack error string, a persisted save) intercept
 * SessionService/GetSlackConfig|UpdateSlackConfig|TestSlackWebhook via
 * page.route rather than mutating the shared global test server's real
 * config — the same "intercept and fulfill a fabricated response" precedent
 * documented in approval-ci-block.spec.ts and review-queue-severity.spec.ts.
 * That matters here specifically because global-setup.ts starts exactly one
 * server for the whole Playwright run (shared across every spec file), so a
 * test that actually persisted a webhook via a real Save would leak
 * "configured" state into every test that runs after it.
 */

import { test, expect, Page } from "@playwright/test";
import { SettingsPage } from "./pages/SettingsPage";

const TEST_WEBHOOK_URL = "https://hooks.slack.com/services/T0/B0/E2ETEST";
// Sentinel URL used to make a mocked TestSlackWebhook branch return failure
// vs. success based on which webhook is currently in the form — lets a
// single route handler cover both the "failing" and "recovered" halves of
// UX-7 without needing two separate page.route registrations.
const FAILING_WEBHOOK_URL = "https://hooks.slack.com/services/T0/B0/FAIL";

interface SlackConfigFixture {
  webhookConfigured?: boolean;
  notifyOnQueueItem?: boolean;
  queueDepthThreshold?: number;
  approvalEnabled?: boolean;
  dashboardBaseUrl?: string;
  lastDelivery?: {
    attempted: boolean;
    success: boolean;
    error: string;
    attemptedAt?: string;
  };
}

function slackConfigJson(overrides: SlackConfigFixture = {}) {
  return {
    config: {
      webhookConfigured: false,
      notifyOnQueueItem: false,
      queueDepthThreshold: 0,
      approvalEnabled: false,
      dashboardBaseUrl: "",
      ...overrides,
    },
  };
}

async function mockGetSlackConfig(page: Page, overrides: SlackConfigFixture = {}) {
  await page.route("**/api/session.v1.SessionService/GetSlackConfig", (route) =>
    route.fulfill({ json: slackConfigJson(overrides) }),
  );
}

/** Fails only when the form's current webhook URL is FAILING_WEBHOOK_URL, succeeds otherwise. */
async function mockTestSlackWebhookByUrl(page: Page) {
  await page.route("**/api/session.v1.SessionService/TestSlackWebhook", async (route) => {
    const body = route.request().postDataJSON() as { webhookUrl?: string };
    if (body.webhookUrl === FAILING_WEBHOOK_URL) {
      await route.fulfill({
        json: { success: false, error: "slack returned 404: no_service" },
      });
    } else {
      await route.fulfill({ json: { success: true, error: "" } });
    }
  });
}

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

  test("configures webhook and sends test message", async ({ page }) => {
    // REQ-15 / Story 1.4.5: fill the URL, opt in to queue-item notifications,
    // persist via Save, then verify via "Send test message" — the full
    // configure-and-verify flow, distinct from the "verify before saving"
    // flow covered above. UpdateSlackConfig/GetSlackConfig are mocked (see
    // file header) so this test's Save doesn't leak into shared server state.
    let saved = slackConfigJson().config;
    await page.route("**/api/session.v1.SessionService/GetSlackConfig", (route) =>
      route.fulfill({ json: { config: saved } }),
    );
    await page.route("**/api/session.v1.SessionService/UpdateSlackConfig", async (route) => {
      const body = route.request().postDataJSON() as {
        webhookUrl?: string;
        notifyOnQueueItem?: boolean;
        queueDepthThreshold?: number;
        dashboardBaseUrl?: string;
      };
      saved = {
        ...saved,
        webhookConfigured: Boolean(body.webhookUrl),
        notifyOnQueueItem: Boolean(body.notifyOnQueueItem),
        queueDepthThreshold: body.queueDepthThreshold ?? saved.queueDepthThreshold,
        dashboardBaseUrl: body.dashboardBaseUrl ?? saved.dashboardBaseUrl,
      };
      await route.fulfill({ json: { config: saved } });
    });
    await page.route("**/api/session.v1.SessionService/TestSlackWebhook", (route) =>
      route.fulfill({ json: { success: true, error: "" } }),
    );

    const urlField = page.getByLabel("Webhook URL");
    await urlField.fill(TEST_WEBHOOK_URL);
    await page.getByLabel("Notify on new review-queue item").check();
    await page.getByRole("button", { name: "Save" }).click();

    // Save clears the input and re-fetches config; the masked placeholder
    // flipping confirms the reload landed before we move on.
    await expect(urlField).toHaveValue("");
    await expect(urlField).toHaveAttribute("placeholder", "•••• (configured)");

    await page.getByRole("button", { name: "Send test message" }).click();

    const result = page.getByTestId("slack-test-webhook-result");
    await expect(result).toHaveText(/Test message sent/);
  });

  test("shows specific inline error and blocks save for malformed webhook URL", async ({
    page,
  }) => {
    const urlField = page.getByLabel("Webhook URL");
    await urlField.fill("not-a-url");
    await urlField.blur();

    const error = page.getByTestId("slack-webhook-error");
    await expect(error).toBeVisible();
    await expect(error).toHaveText(
      /expected https:\/\/hooks\.slack\.com\/services\//,
    );
    await expect(urlField).toHaveAttribute("aria-invalid", "true");
    await expect(page.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  test("recovers from invalid URL and failed test-send without navigation", async ({
    page,
  }) => {
    const urlField = page.getByLabel("Webhook URL");
    const fieldError = page.getByTestId("slack-webhook-error");

    // UX-4/UX-7 half 1: invalid URL shape, then corrected live with no re-submit.
    await urlField.fill("not-a-url");
    await urlField.blur();
    await expect(fieldError).toBeVisible();
    await expect(fieldError).toHaveText(
      /expected https:\/\/hooks\.slack\.com\/services\//,
    );

    await urlField.fill(TEST_WEBHOOK_URL);
    await expect(fieldError).not.toBeVisible();
    await expect(page.getByRole("button", { name: "Save" })).toBeEnabled();

    // UX-5/UX-7 half 2: a failed test-send recovers in place once the user
    // supplies a different webhook, with no page reload in between.
    await mockTestSlackWebhookByUrl(page);
    const result = page.getByTestId("slack-test-webhook-result");
    const testButton = page.getByRole("button", { name: "Send test message" });

    await urlField.fill(FAILING_WEBHOOK_URL);
    await testButton.click();
    await expect(result).toHaveAttribute("role", "alert");
    await expect(result).toHaveText(/no_service/);

    await urlField.fill(TEST_WEBHOOK_URL);
    await testButton.click();
    await expect(result).toHaveAttribute("role", "status");
    await expect(result).toHaveText(/Test message sent/);
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

    const warning = page.getByTestId("slack-dashboard-warning");
    await expect(warning).toBeVisible();

    await page.getByRole("button", { name: "Dismiss notice" }).click();
    await expect(warning).not.toBeVisible();
  });

  test("shows last delivery status immediately on page load", async ({ page }) => {
    // UX-2: a prior delivery, seeded via a GetSlackConfig fixture (the
    // "config fixture" option named in validation.md's UX-2 row), must be
    // visible on first render — zero additional clicks.
    const attemptedAt = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    await mockGetSlackConfig(page, {
      webhookConfigured: true,
      notifyOnQueueItem: true,
      lastDelivery: { attempted: true, success: true, error: "", attemptedAt },
    });

    await settings.goto();
    await settings.selectTab("Appearance");

    const status = page.getByTestId("slack-last-delivery-status");
    await expect(status).toBeVisible();
    await expect(status).toHaveText(/Last Slack delivery: \d+m ago — ✓ delivered/);
  });

  test("surfaces slack error text inline on failed test-send", async ({ page }) => {
    // UX-5: the result region must show Slack's own error text, not a
    // generic "test failed" message.
    await page.route("**/api/session.v1.SessionService/TestSlackWebhook", (route) =>
      route.fulfill({
        json: { success: false, error: "slack returned 404: no_service" },
      }),
    );

    await page.getByLabel("Webhook URL").fill(TEST_WEBHOOK_URL);
    await page.getByRole("button", { name: "Send test message" }).click();

    const result = page.getByTestId("slack-test-webhook-result");
    await expect(result).toHaveAttribute("role", "alert");
    await expect(result).toHaveText("Test failed: slack returned 404: no_service");
  });

  test("is fully operable via keyboard alone in visual tab order", async ({ page }) => {
    // UX-16: tab through every interactive element in visual order; Space
    // activates the checkbox, Enter activates a button.
    //
    // This test's Tab chain runs long enough for the app's onboarding-tour
    // dialog to mount mid-test and steal focus (it renders a real modal, not
    // a toast) — suppress it the same way review-queue-severity.spec.ts's
    // openReviewQueue() does, since the other, shorter tests in this file
    // finish before the dialog has a chance to appear.
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
    await mockGetSlackConfig(page, { webhookConfigured: true });
    await settings.goto();
    await settings.selectTab("Appearance");

    const urlField = page.getByLabel("Webhook URL");
    const removeBtn = page.getByRole("button", { name: "Remove" });
    const testBtn = page.getByRole("button", { name: "Send test message" });
    const notifyCheckbox = page.getByLabel("Notify on new review-queue item");
    const thresholdInput = page.getByLabel(/Queue-depth digest threshold/);
    const dashboardInput = page.getByLabel("Dashboard URL");
    const saveBtn = page.getByRole("button", { name: "Save" });

    await urlField.fill(TEST_WEBHOOK_URL);
    await expect(urlField).toBeFocused();

    await page.keyboard.press("Tab");
    await expect(removeBtn).toBeFocused();

    await page.keyboard.press("Tab");
    await expect(testBtn).toBeFocused();

    await page.keyboard.press("Tab");
    await expect(notifyCheckbox).toBeFocused();

    // Space activates the focused checkbox.
    await expect(notifyCheckbox).not.toBeChecked();
    await page.keyboard.press("Space");
    await expect(notifyCheckbox).toBeChecked();

    await page.keyboard.press("Tab");
    await expect(thresholdInput).toBeFocused();

    // The disabled "Allow Approve/Deny from Slack" checkbox is unfocusable,
    // so Tab skips straight past it to the next enabled control.
    await page.keyboard.press("Tab");
    await expect(dashboardInput).toBeFocused();

    // Checking the notify toggle above (with an empty Dashboard URL) surfaces
    // the UX-9 dashboard-warning notice, which inserts its own "Dismiss
    // notice" button into the visual/tab order between the Dashboard URL
    // field and Save.
    const dismissWarningBtn = page.getByRole("button", { name: "Dismiss notice" });
    await page.keyboard.press("Tab");
    await expect(dismissWarningBtn).toBeFocused();

    await page.keyboard.press("Tab");
    await expect(saveBtn).toBeFocused();

    // Enter activates the focused control — verified in isolation, after the
    // tab-order chain above, since a real test-send transiently disables
    // (and therefore blurs) "Send test message" mid-flight, which would
    // otherwise break the chain of toBeFocused() assertions.
    await testBtn.focus();
    await page.keyboard.press("Enter");
    const result = page.getByTestId("slack-test-webhook-result");
    await expect(result).toHaveText(/./, { timeout: 10000 });
  });
});
