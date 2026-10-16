import { Page, Locator } from '@playwright/test';

export class ImportSessionsPage {
  readonly page: Page;
  readonly container: Locator;
  readonly panel: Locator;
  readonly emptyState: Locator;
  readonly commitError: Locator;

  constructor(page: Page) {
    this.page = page;
    this.container = page.locator('[data-testid="import-sessions-container"]');
    this.panel = page.locator('[data-testid="import-external-sessions-panel"]');
    this.emptyState = page.locator('[data-testid="import-external-sessions-empty"]');
    this.commitError = page.locator('[data-testid="import-commit-error"]');
  }

  async goto() {
    await this.page.goto('/sessions/import');
    await this.page.waitForLoadState('domcontentloaded');
  }

  async waitForPageLoad() {
    await this.page.waitForSelector('[data-testid="import-sessions-container"]', {
      timeout: 10000,
    });
  }

  getRows(): Locator {
    return this.page.locator('[data-testid="import-external-session-row"]');
  }

  getRow(title: string): Locator {
    return this.getRows().filter({ hasText: title });
  }

  async importRow(title: string) {
    const row = this.getRow(title);
    await row.locator('[data-testid="import-row-button"]').click();
    await this.page.waitForSelector('[data-testid="import-preview-dialog"]', { timeout: 5000 });
  }

  getPreviewDialog(): Locator {
    return this.page.locator('[data-testid="import-preview-dialog"]');
  }

  async confirmPreview() {
    await this.page.locator('[data-testid="import-preview-confirm-button"]').click();
  }

  getConfirmKillDialog(): Locator {
    return this.page.locator('[data-testid="confirm-kill-dialog"]');
  }

  async confirmKill() {
    await this.page.locator('[data-testid="confirm-kill-kill-button"]').click();
  }

  async revertKill() {
    await this.page.locator('[data-testid="confirm-kill-revert-button"]').click();
  }
}
