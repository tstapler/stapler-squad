import { expect, type Locator, type Page } from '@playwright/test';
import { baseUrl } from '../helpers/base-url';

export const PR_URL = 'https://github.com/acme/app/pull/12';
export const PR_SEARCH_URL = /\/artifacts\?pr=acme\/app%2312$/;

const LOOKUP_ROUTE = '**/api/session.v1.SessionService/FindSessionsByPullRequest';

export interface MockHit {
  sessionId: string;
  title: string;
  source: 'session_pr' | 'artifact';
}

/** The omnibar's "Find sessions for this PR" action (design/ux.md S9). */
export class OmnibarFindPrPage {
  readonly input: Locator;
  readonly action: Locator;
  readonly error: Locator;
  readonly retry: Locator;
  readonly lookupRequests: string[] = [];

  constructor(private readonly page: Page) {
    this.input = page.getByRole('combobox', { name: /session source input/i });
    this.action = page.getByTestId('omnibar-find-pr-action');
    this.error = page.getByTestId('omnibar-find-pr-error');
    this.retry = page.getByTestId('omnibar-find-pr-retry');
  }

  /** Answers the lookup RPC with `hits`, or with an error when `hits` is null. */
  async mockLookup(hits: MockHit[] | null): Promise<void> {
    await this.page.route(LOOKUP_ROUTE, async (route) => {
      this.lookupRequests.push(route.request().postData() ?? '');
      if (hits === null) {
        await route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ code: 'internal', message: 'lookup failed' }),
        });
        return;
      }
      await route.fulfill({ status: 200, contentType: 'application/json', json: { hits, unscannedSessionCount: 0 } });
    });
  }

  async open(): Promise<void> {
    // Suppress the app-wide onboarding walkthrough; its overlay intercepts clicks.
    await this.page.addInitScript(() => localStorage.setItem('stapler-squad:onboarded', 'true'));
    await this.page.goto(baseUrl(), { waitUntil: 'domcontentloaded', timeout: 15000 });
    const isPhone = (this.page.viewportSize()?.width ?? 1280) < 768;
    const opener = isPhone
      ? this.page.getByText('New', { exact: true })
      : this.page.getByRole('button', { name: 'Create new session' });
    await opener.first().click();
    await expect(this.input).toBeVisible({ timeout: 10000 });
  }

  async paste(url: string = PR_URL): Promise<void> {
    await this.input.fill(url);
    await expect(this.action).toBeVisible({ timeout: 5000 });
  }
}
