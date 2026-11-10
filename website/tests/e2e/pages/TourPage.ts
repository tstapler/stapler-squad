// Page object for /tours/ and a tour page. data-testid/ARIA locators only.
import type { Page, Locator } from '@playwright/test';

export class TourPage {
	readonly page: Page;
	readonly h1: Locator;
	readonly breadcrumb: Locator;
	readonly meta: Locator;
	readonly video: Locator;
	readonly installAfterClip: Locator;
	readonly transcript: Locator;
	readonly transcriptSummary: Locator;
	readonly steps: Locator;
	readonly tryBlock: Locator;
	readonly tryInstall: Locator;
	readonly helpLink: Locator;
	readonly prevNext: Locator;
	readonly allTours: Locator;
	readonly figures: Locator;
	readonly startHere: Locator;
	readonly cards: Locator;
	readonly categories: Locator;

	constructor(page: Page) {
		this.page = page;
		this.h1 = page.getByRole('heading', { level: 1 });
		this.breadcrumb = page.getByRole('navigation', { name: 'Breadcrumb' });
		this.meta = page.getByTestId('tour-meta');
		this.video = page.getByTestId('tour-video');
		this.installAfterClip = page.getByTestId('tour-install-link');
		this.transcript = page.getByTestId('tour-transcript');
		this.transcriptSummary = this.transcript.getByText('Transcript', { exact: true });
		this.steps = page.getByTestId('tour-steps');
		this.tryBlock = page.getByTestId('tour-try');
		this.tryInstall = page.getByTestId('tour-try-install');
		this.helpLink = page.getByTestId('tour-help-link');
		this.prevNext = page.getByTestId('tour-prevnext');
		this.allTours = page.getByTestId('tour-all');
		this.figures = page.locator('figure[data-testid^="tour-"]');
		this.startHere = page.getByTestId('tour-start-here');
		this.cards = page.getByTestId('tour-card');
		this.categories = page.getByTestId('tour-category');
	}

	async gotoIndex(): Promise<void> {
		await this.page.goto('tours/', { waitUntil: 'load' });
	}

	async gotoTour(slug: string): Promise<void> {
		await this.page.goto(`tours/${slug}/`, { waitUntil: 'load' });
	}

	/** Element order in the document for the structure check (UX-2); -1 for absent. */
	async domOrder(): Promise<string[]> {
		return this.page.evaluate(() => {
			const probes: Array<[string, string]> = [
				['breadcrumb', 'nav.tour-breadcrumb'],
				['h1', 'main h1'],
				['meta', '[data-testid="tour-meta"]'],
				['video', '[data-testid="tour-video"]'],
				['install', '[data-testid="tour-install-link"]'],
				['transcript', '[data-testid="tour-transcript"]'],
				['steps', '[data-testid="tour-steps"]'],
				['try', '[data-testid="tour-try"]'],
				['prevnext', '[data-testid="tour-prevnext"]'],
				['all', '[data-testid="tour-all"]'],
			];
			const found = probes
				.map(([name, sel]) => [name, document.querySelector(sel)] as const)
				.filter(([, el]) => el);
			return found
				.sort(([, a], [, b]) => (a!.compareDocumentPosition(b!) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1))
				.map(([name]) => name);
		});
	}
}
