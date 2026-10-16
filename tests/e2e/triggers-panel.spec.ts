// @feature triggers:create, triggers:edit, triggers:toggle
import { test, expect } from "@playwright/test";
import { TriggersPage } from "./pages/TriggersPage";

test.describe("triggers-panel", () => {
  let triggersPage: TriggersPage;

  test.beforeEach(async ({ page }) => {
    // Skip the first-run onboarding modal (shows on any fresh browser context after
    // 800ms — see web-app/src/components/onboarding/useOnboarding.ts), matching the
    // convention in rule-builder-ci-passing.spec.ts / approval-ci-block.spec.ts.
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
    triggersPage = new TriggersPage(page);
    await triggersPage.goto();
  });

  test("triggers-panel_should_showNewTriggerInList_When_webhookTriggerCreated", async () => {
    const slug = `e2e-webhook-${Date.now()}`;
    await triggersPage.createWebhookTrigger({
      slug,
      name: `E2E Webhook ${slug}`,
      targetDirectory: "/tmp",
      command: "echo hello",
      eventFilter: "issue_created",
      promptTemplate: "Triage {{.issue.key}}",
    });

    const row = triggersPage.row(slug);
    await expect(row).toBeVisible();
    await expect(row.getByText("Webhook", { exact: true })).toBeVisible();
  });

  test("triggers-panel_should_persistEditedField_When_triggerUpdated", async () => {
    const slug = `e2e-edit-${Date.now()}`;
    await triggersPage.createWebhookTrigger({
      slug,
      name: `E2E Edit ${slug}`,
      targetDirectory: "/tmp",
      command: "echo hello",
    });
    await expect(triggersPage.row(slug)).toBeVisible();

    const newName = `E2E Edit Renamed ${slug}`;
    await triggersPage.editTrigger(slug, { name: newName, eventFilter: "issue_updated" });

    // The row re-renders with the new name once the modal closes and the
    // list refetches (useWorkflows.updateWorkflow awaits refresh()) — no
    // page reload involved.
    await expect(triggersPage.row(slug)).toContainText(newName);
  });

  test("triggers-panel_should_reflectToggleLive_When_enableDisableClicked", async () => {
    const slug = `e2e-toggle-${Date.now()}`;
    await triggersPage.createWebhookTrigger({
      slug,
      name: `E2E Toggle ${slug}`,
      targetDirectory: "/tmp",
      command: "echo hello",
    });
    await expect(triggersPage.row(slug)).toBeVisible();

    // Newly created triggers default to enabled (cronEnabled: true).
    const toggle = triggersPage.toggleButton(slug);
    await expect(toggle).toHaveText("ON");
    await expect(toggle).toHaveAccessibleName(/Disable trigger/);

    await triggersPage.toggleTrigger(slug);
    await expect(toggle).toHaveText("OFF", { timeout: 10000 });
    await expect(toggle).toHaveAccessibleName(/Enable trigger/);

    await triggersPage.toggleTrigger(slug);
    await expect(toggle).toHaveText("ON", { timeout: 10000 });
    await expect(toggle).toHaveAccessibleName(/Disable trigger/);
  });
});

test.describe("callback-settings", () => {
  let triggersPage: TriggersPage;

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
    triggersPage = new TriggersPage(page);
    await triggersPage.goto();
  });

  test("callback-settings_should_roundTripMaskedConfiguredBadge_When_validUrlSaved", async ({ page }) => {
    const fieldTestId = "callback-on-session-complete";

    // The callback config is global, process-wide server state (not per-browser-
    // context) — a prior run of this same spec against the same test-server
    // instance (e.g. the chromium-dom project running after chromium already
    // exercised this exact field) would otherwise leave this field "Configured"
    // before this test even starts, failing the assertion below. Reset it first
    // if needed so the test is self-contained regardless of run order/project.
    // The masked URL input's DOM value is always "" regardless of configured
    // state (the real URL never round-trips), so fill("") on it is always a
    // no-op — use the dedicated "Clear" affordance (only rendered when
    // configured) instead, which calls setEdit directly.
    if ((await triggersPage.callbackStatusBadge(fieldTestId).textContent()) === "Configured") {
      await triggersPage.clearCallbackUrl(fieldTestId);
      await triggersPage.saveCallbackSettings();
      await expect(triggersPage.callbackStatusBadge(fieldTestId)).toHaveText("Not configured");
    }
    await expect(triggersPage.callbackStatusBadge(fieldTestId)).toHaveText("Not configured");

    await triggersPage.fillCallbackUrl(fieldTestId, "https://example.com/hooks/session-complete");
    await triggersPage.saveCallbackSettings();
    await expect(triggersPage.callbackStatusBadge(fieldTestId)).toHaveText("Configured", { timeout: 10000 });

    // The real URL is never echoed back — the input must remain empty after save.
    await expect(page.getByTestId(fieldTestId)).toHaveValue("");

    // Reload to confirm the config-backed state persisted server-side, not just
    // client-side optimism, and that the URL still never round-trips.
    await triggersPage.goto();
    await expect(triggersPage.callbackStatusBadge(fieldTestId)).toHaveText("Configured");
    await expect(page.getByTestId(fieldTestId)).toHaveValue("");
  });

  test("callback-settings_should_rejectSSRFTargetWithVisibleError_When_savingLoopbackUrl", async () => {
    const fieldTestId = "callback-on-session-stale";
    await expect(triggersPage.callbackStatusBadge(fieldTestId)).toHaveText("Not configured");

    await triggersPage.fillCallbackUrl(fieldTestId, "http://127.0.0.1/");
    await triggersPage.saveCallbackSettings();

    await expect(triggersPage.callbackErrorBanner()).toBeVisible({ timeout: 10000 });

    // The SSRF-target URL must not have been persisted (AC11, config-save-time
    // validation reached via the UI, not just the RPC layer).
    await expect(triggersPage.callbackStatusBadge(fieldTestId)).toHaveText("Not configured");
  });
});
