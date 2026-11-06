// @feature website:docs-search
// design/ux.md §5 criteria 6, 8 (zero-results slice), 9.
import { test, expect } from '@playwright/test';
import { DocsPage } from './pages/DocsPage';
import { SearchModal } from './pages/SearchModal';

test.describe('Docs search — UX acceptance', () => {
	// Criterion 6: search -> a relevant result page in 2 actions (open
	// search, click a result), from any page on the site.
	test('docsSearch_should_ReachResultPageInTwoActions_When_QuerySubmittedAndResultClicked', async ({ page }) => {
		const docs = new DocsPage(page);
		await docs.goto('help/getting-started/');

		const search = new SearchModal(page);
		await page.keyboard.press('Control+k');
		await expect(search.dialog).toBeVisible();

		await search.search('tmux session'); // action 1
		await search.firstResultLink().click(); // action 2

		await expect(page).toHaveURL(/\/help\/.+\/?/);
	});

	// Criterion 9: the zero-results state shows the exact copy pattern
	// 'No results for "{query}"' plus a docs-home link and a file-an-issue
	// link, with the modal staying open. Pagefind's own svelte UI can't embed
	// a link inside its translation string (plain text interpolation, no
	// {@html} -- confirmed by reading @pagefind/default-ui's ui.svelte), so
	// astro.config.mjs's initSearchZeroResultsFallback() appends these two
	// links as a sibling element; this test is what that fix exists for.
	test('searchModal_should_ShowNoResultsMessageWithTwoLinks_When_QueryMatchesNothing', async ({ page }) => {
		const docs = new DocsPage(page);
		await docs.goto('help/getting-started/');

		const search = new SearchModal(page);
		await search.open();
		await search.search('zzqqxyz');

		await expect(page.getByText('No results for "zzqqxyz"')).toBeVisible();
		await expect(search.docsHomeLink).toBeVisible();
		await expect(search.fileIssueLink).toBeVisible();
		await expect(search.dialog).toBeVisible();
	});

	// Criterion 8 (zero-search-results slice): see landing.spec.ts for the
	// other failure modes under this same criterion name.
	test.describe('edgeCaseStates_should_AlwaysRenderAtLeastOneExitLink_When_AnyDocumentedFailureModeTriggered', () => {
		test('zero search results', async ({ page }) => {
			const docs = new DocsPage(page);
			await docs.goto('help/getting-started/');

			const search = new SearchModal(page);
			await search.open();
			await search.search('zzqqxyz');

			await expect(search.docsHomeLink).toBeVisible();
			await expect(search.fileIssueLink).toBeVisible();
		});
	});
});
