// Page object for a Starlight docs page (sidebar nav + prev/next pager).
// Locators are data-testid/ARIA-role only, with one documented exception:
// Starlight's sidebar group disclosure control is a bare <summary> that
// Chromium exposes in its accessibility tree with role "generic" (confirmed
// empirically against a live preview build, not assumed from the HTML-AAM
// spec table) -- there is no ARIA role to filter on, and it's vendored
// @astrojs/starlight markup this project doesn't own, so a data-testid
// can't be added to it either. getByText is the documented fallback for
// that one control only; every other locator here is role/testid-based.
import type { Page, Locator } from '@playwright/test';

export class DocsPage {
	readonly page: Page;
	readonly sidebarNav: Locator;
	readonly pagerNext: Locator;
	readonly pagerPrev: Locator;

	constructor(page: Page) {
		this.page = page;
		this.sidebarNav = page.getByRole('navigation', { name: 'Main' });
		this.pagerNext = page.getByRole('link', { name: /^Next/ });
		this.pagerPrev = page.getByRole('link', { name: /^Previous/ });
	}

	async goto(slug: string): Promise<void> {
		// Relative, non-"/"-prefixed: see playwright.config.ts's baseURL comment.
		await this.page.goto(slug, { waitUntil: 'load' });
	}

	/** A sidebar group's disclosure header (e.g. "Troubleshooting"). */
	sidebarGroupHeader(groupLabel: string): Locator {
		return this.sidebarNav.getByText(groupLabel, { exact: true });
	}

	/** A sidebar entry's own page link, by its visible link text. */
	sidebarLink(linkName: string | RegExp): Locator {
		return this.sidebarNav.getByRole('link', { name: linkName });
	}
}
