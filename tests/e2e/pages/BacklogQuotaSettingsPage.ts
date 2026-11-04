import type { Locator, Page } from "@playwright/test";
import { baseUrl } from '../helpers/base-url';

const BASE_URL = baseUrl();

export type QuotaField = "budget" | "pause" | "resume" | "usd";

/** /settings/backlog-quota: the gate switch, the four numeric fields and the save/revert actions. */
export class BacklogQuotaSettingsPage {
  readonly page: Page;
  readonly root: Locator;
  readonly load: Locator;
  readonly loadError: Locator;
  readonly loadRetry: Locator;
  readonly form: Locator;
  readonly effective: Locator;
  readonly notConfigured: Locator;
  readonly warning: Locator;
  readonly gate: Locator;
  readonly save: Locator;
  readonly revert: Locator;
  readonly status: Locator;
  readonly errors: Locator;
  readonly saveError: Locator;
  readonly mismatch: Locator;
  readonly mismatchReload: Locator;
  readonly mismatchSaveAgain: Locator;
  readonly dialog: Locator;

  constructor(page: Page) {
    this.page = page;
    this.root = page.getByTestId("backlog-quota-page");
    this.load = page.getByTestId("backlog-quota-load");
    this.loadError = page.getByTestId("backlog-quota-load-error");
    this.loadRetry = page.getByTestId("backlog-quota-retry");
    this.form = page.getByTestId("backlog-quota-form");
    this.effective = page.getByTestId("backlog-quota-effective");
    this.notConfigured = page.getByTestId("backlog-quota-not-configured");
    this.warning = page.getByTestId("backlog-quota-warning");
    this.gate = page.getByTestId("backlog-quota-enabled");
    this.save = page.getByTestId("backlog-quota-save");
    this.revert = page.getByTestId("backlog-quota-revert");
    this.status = page.getByTestId("backlog-quota-status");
    this.errors = page.getByTestId("backlog-quota-errors");
    this.saveError = page.getByTestId("backlog-quota-save-error");
    this.mismatch = page.getByTestId("backlog-quota-mismatch");
    this.mismatchReload = page.getByTestId("backlog-quota-mismatch-reload");
    this.mismatchSaveAgain = page.getByTestId("backlog-quota-mismatch-save-again");
    this.dialog = page.getByRole("alertdialog");
  }

  async goto(): Promise<void> {
    await this.page.goto(`${BASE_URL}/settings/backlog-quota`, { waitUntil: "domcontentloaded" });
  }

  field(key: QuotaField): Locator {
    return this.page.getByTestId(`backlog-quota-${key}`);
  }

  async fill(key: QuotaField, value: string): Promise<void> {
    await this.field(key).fill(value);
  }

  /** Number of live regions (`role=status`) inside the page body. */
  statusRegions(): Locator {
    return this.root.getByRole("status");
  }
}
