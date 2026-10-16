// Page object for Starlight's built-in Pagefind search modal
// (<dialog aria-label="Search">, mounted into #starlight__search).
import type { Page, Locator } from '@playwright/test';
import { expect } from '@playwright/test';

export class SearchModal {
	readonly page: Page;
	readonly trigger: Locator;
	readonly dialog: Locator;
	readonly input: Locator;
	readonly resultListItems: Locator;
	/** astro.config.mjs's zero-results fallback (data-testid="search-zero-results-links"). */
	readonly zeroResultsLinks: Locator;
	readonly docsHomeLink: Locator;
	readonly fileIssueLink: Locator;

	constructor(page: Page) {
		this.page = page;
		this.trigger = page.getByRole('button', { name: /^Search/ });
		this.dialog = page.getByRole('dialog', { name: 'Search' });
		this.input = this.dialog.getByRole('textbox', { name: 'Search' });
		this.resultListItems = this.dialog.getByRole('listitem');
		this.zeroResultsLinks = page.getByTestId('search-zero-results-links');
		this.docsHomeLink = this.zeroResultsLinks.getByRole('link', { name: 'Go to the docs home' });
		this.fileIssueLink = this.zeroResultsLinks.getByRole('link', { name: 'File a GitHub issue' });
	}

	async open(): Promise<void> {
		// Ctrl+K is the documented shortcut (and works cross-platform in
		// Chromium regardless of the Mac Cmd+K hint swapped in client-side);
		// clicking the trigger button is the mouse-equivalent fallback used
		// when the shortcut alone isn't reliable in a given test context.
		await this.trigger.click();
		await expect(this.dialog).toBeVisible();
	}

	async search(query: string): Promise<void> {
		await this.input.fill(query);
		// Pagefind debounces input before re-querying; wait for the message
		// paragraph to reflect this exact query rather than a fixed timeout.
		await expect(this.page.getByText(query, { exact: false }).first()).toBeVisible();
	}

	/** The first real result link inside the first result listitem. */
	firstResultLink(): Locator {
		return this.resultListItems.first().getByRole('link').first();
	}
}
