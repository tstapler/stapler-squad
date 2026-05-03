import { Page, Locator } from '@playwright/test';

export class SessionsPage {
  readonly page: Page;
  readonly searchInput: Locator;
  readonly statusFilter: Locator;
  readonly groupBySelect: Locator;
  readonly newSessionButton: Locator;

  constructor(page: Page) {
    this.page = page;
    this.searchInput = page.locator('input[aria-label="Search sessions"]').first();
    this.statusFilter = page.locator('select[aria-label="Filter by status"]').first();
    this.groupBySelect = page.locator('select[aria-label="Group sessions by"]').first();
    this.newSessionButton = page.getByRole('button', { name: /new session/i }).first();
  }

  async goto() {
    await this.page.goto('/');
    await this.page.waitForLoadState('domcontentloaded');
  }

  async waitForSessionList() {
    await this.page.waitForSelector('[data-testid="session-list"], .session-list, [class="sessionList"]', { timeout: 10000 });
  }

  getSessionCard(title: string): Locator {
    return this.page.locator('[data-testid="session-card"]').filter({ hasText: title });
  }

  getSessionCards(): Locator {
    return this.page.locator('[data-testid="session-card"]');
  }
}
