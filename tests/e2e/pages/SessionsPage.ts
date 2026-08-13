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
    await this.page.waitForSelector('[data-testid="session-list"], .session-list', { timeout: 10000 });
  }

  // Matches both densities: SessionCard.tsx (grid view, "session-card") and
  // SessionRow.tsx (the default list view, "session-row" — see ci-status-badge.spec.ts
  // for the same pattern). A title-only selector would previously silently match
  // nothing against the app's default view.
  getSessionCard(title: string): Locator {
    return this.page
      .locator('[data-testid="session-card"], [data-testid="session-row"]')
      .filter({ hasText: title });
  }

  getSessionCards(): Locator {
    return this.page.locator('[data-testid="session-card"], [data-testid="session-row"]');
  }
}
