import { Page, Locator, expect } from '@playwright/test';

export class SessionsPage {
  readonly page: Page;
  readonly searchInput: Locator;
  readonly statusFilter: Locator;
  readonly groupBySelect: Locator;
  readonly newSessionButton: Locator;
  readonly createSessionSubmitButton: Locator;

  constructor(page: Page) {
    this.page = page;
    this.searchInput = page.locator('input[aria-label="Search sessions"]').first();
    this.statusFilter = page.locator('select[aria-label="Filter by status"]').first();
    this.groupBySelect = page.locator('select[aria-label="Group sessions by"]').first();
    this.newSessionButton = page.getByRole('button', { name: /new session/i }).first();
    this.createSessionSubmitButton = page.getByTestId('omnibar-create-session-button');
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

  /**
   * Creates a temporary (no-git) plain-shell session via the Omnibar and
   * waits for the app to navigate to it. A plain "bash" program is used
   * instead of a real AI CLI so the session is cheap and its lifecycle is
   * fully controllable from the test — mirrors
   * session-completion-summary.spec.ts's creation flow.
   *
   * Returns the new session's id, parsed out of the resulting
   * `/?session=<id>` URL, so callers (e.g. connection-count-indicator.spec.ts)
   * can open that same session from a second browser context/tab.
   */
  async createBashSession(namePrefix: string): Promise<string> {
    await this.newSessionButton.click();
    await this.page.getByRole('radio', { name: /temporary \(no git\)/i }).click();

    const sessionTitle = `${namePrefix}-${Date.now()}`;
    await this.page.getByLabel('Session Name').fill(sessionTitle);

    await this.page.getByText('Advanced Options').click();
    await this.page.getByLabel('Program', { exact: true }).selectOption('bash');

    const createRequest = this.page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST',
    );
    await this.createSessionSubmitButton.click();
    await createRequest;

    await this.page.waitForURL(/[?&]session=/, { timeout: 15000 });
    const url = new URL(this.page.url());
    const sessionId = url.searchParams.get('session');
    expect(sessionId).not.toBeNull();
    return sessionId!;
  }
}
