// Page object for the splash landing page (website/src/content/docs/index.mdx).
// Locators are data-testid/ARIA-role only, per e2e-test-conventions.
import type { Page, Locator } from '@playwright/test';

export class LandingPage {
	readonly page: Page;
	readonly heading: Locator;
	readonly installCommandText: Locator;
	readonly installCopyButton: Locator;
	readonly viewOnGithubLink: Locator;
	readonly heroVideo: Locator;
	readonly featureGrid: Locator;
	readonly searchTrigger: Locator;
	/** Header site-title link -- NOT a "Docs" link; it points back at this same page. */
	readonly siteTitleLink: Locator;
	/** LandingFooterNav.astro's two docs exit links (ux.md §1.1/§1.2 wireframe). */
	readonly footerBrowseDocsLink: Locator;
	readonly footerDocsLink: Locator;

	constructor(page: Page) {
		this.page = page;
		this.heading = page.getByRole('heading', { level: 1 });
		this.installCommandText = page.getByTestId('install-command-text');
		this.installCopyButton = page.getByTestId('install-copy-button');
		this.viewOnGithubLink = page.getByRole('link', { name: 'View on GitHub' });
		this.heroVideo = page.getByTestId('hero-video');
		this.featureGrid = page.getByTestId('feature-grid');
		this.searchTrigger = page.getByRole('button', { name: /^Search/ });
		this.siteTitleLink = page.getByRole('link', { name: 'Stapler Squad', exact: true });
		this.footerBrowseDocsLink = page.getByRole('link', { name: 'Browse the docs' });
		this.footerDocsLink = page.getByRole('link', { name: 'Docs', exact: true });
	}

	async goto(): Promise<void> {
		// Relative, non-"/"-prefixed: see playwright.config.ts's baseURL comment
		// -- a leading "/" would silently drop the "/stapler-squad" base path.
		await this.page.goto('', { waitUntil: 'load' });
	}

	/** All `data-testid="feature-card"` articles in the feature grid. */
	featureCards(): Locator {
		return this.page.getByTestId('feature-card');
	}
}
