import { Page, Locator, expect } from "@playwright/test";

export interface WebhookTriggerFields {
  slug: string;
  name: string;
  targetDirectory: string;
  command: string;
  eventFilter?: string;
  labelFilter?: string;
  promptTemplate?: string;
}

/**
 * Page helper for /triggers — wraps both TriggersPanel (list + TriggerFormModal
 * create/edit) and CallbackSettings (outbound callback URL config), which share
 * the single /triggers route (web-app/src/app/triggers/page.tsx).
 */
export class TriggersPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto((process.env.TEST_SERVER_URL ?? "http://localhost:8544") + "/triggers", {
      waitUntil: "domcontentloaded",
      timeout: 15000,
    });
    await expect(this.page.getByTestId("triggers-panel")).toBeVisible({ timeout: 10000 });
  }

  // ── TriggersPanel: list + create/edit ─────────────────────────────────────

  /** Row locator for a trigger by its slug (matches the visible desktop table row). */
  row(slug: string): Locator {
    return this.page.getByRole("row").filter({ hasText: slug });
  }

  async openCreateModal() {
    await this.page.getByTestId("add-trigger-button").click();
    await expect(this.page.getByTestId("trigger-form")).toBeVisible();
  }

  /** Fills and submits the create form for a webhook-type trigger. */
  async createWebhookTrigger(fields: WebhookTriggerFields) {
    await this.openCreateModal();
    await this.page.getByTestId("trigger-type-webhook").click();
    await this.page.getByTestId("trigger-slug-input").fill(fields.slug);
    await this.page.getByTestId("trigger-name-input").fill(fields.name);
    await this.page.getByTestId("trigger-target-directory-input").fill(fields.targetDirectory);
    await this.page.getByTestId("trigger-command-input").fill(fields.command);
    await this.page.getByTestId("trigger-webhook-slug-input").fill(fields.slug);
    if (fields.eventFilter !== undefined) {
      await this.page.getByTestId("trigger-event-filter-input").fill(fields.eventFilter);
    }
    if (fields.labelFilter !== undefined) {
      await this.page.getByTestId("trigger-label-filter-input").fill(fields.labelFilter);
    }
    if (fields.promptTemplate !== undefined) {
      await this.page.getByTestId("trigger-prompt-template-input").fill(fields.promptTemplate);
    }
    await this.page.getByTestId("trigger-form-submit").click();
    await expect(this.page.getByTestId("trigger-form")).toHaveCount(0, { timeout: 10000 });
  }

  /** Opens the edit modal for the trigger identified by `slug`. */
  async openEdit(slug: string) {
    await this.row(slug).getByRole("button", { name: /Edit trigger/ }).click();
    await expect(this.page.getByTestId("trigger-form")).toBeVisible();
  }

  /** Edits an existing trigger's name and target directory, then saves. */
  async editTrigger(slug: string, opts: { name?: string; targetDirectory?: string; eventFilter?: string }) {
    await this.openEdit(slug);
    if (opts.name !== undefined) {
      await this.page.getByTestId("trigger-name-input").fill(opts.name);
    }
    if (opts.targetDirectory !== undefined) {
      await this.page.getByTestId("trigger-target-directory-input").fill(opts.targetDirectory);
    }
    if (opts.eventFilter !== undefined) {
      await this.page.getByTestId("trigger-event-filter-input").fill(opts.eventFilter);
    }
    await this.page.getByTestId("trigger-form-submit").click();
    await expect(this.page.getByTestId("trigger-form")).toHaveCount(0, { timeout: 10000 });
  }

  /** Toggle button within a trigger's row (ON/OFF label). */
  toggleButton(slug: string): Locator {
    return this.row(slug).getByRole("button", { name: /Disable trigger|Enable trigger/ });
  }

  async toggleTrigger(slug: string) {
    await this.toggleButton(slug).click();
  }

  // ── CallbackSettings ───────────────────────────────────────────────────────

  async fillCallbackUrl(fieldTestId: string, url: string) {
    await this.page.getByTestId(fieldTestId).fill(url);
  }

  async saveCallbackSettings() {
    await this.page.getByTestId("callback-settings-save").click();
  }

  callbackStatusBadge(fieldTestId: string): Locator {
    return this.page.getByTestId(`${fieldTestId}-status`);
  }

  callbackErrorBanner(): Locator {
    return this.page.getByRole("alert");
  }
}
