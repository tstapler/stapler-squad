// @feature website:docs-pager
// design/ux.md §5 criterion 7: the prev/next pager never dead-links -- it
// omits the control at either end of the flattened sidebar order rather
// than disabling or dead-linking it.
import { test, expect } from '@playwright/test';
import { DocsPage } from './pages/DocsPage';

test.describe('Docs pager — UX acceptance', () => {
	test('pager_should_OmitNextControl_When_OnLastPageOfFlattenedOrder', async ({ page }) => {
		// "use-up-next-pr-tabs-and-ask-to-fix" is the last entry in
		// astro.config.mjs's Troubleshooting group, itself the last sidebar
		// group -- the last page in flattened order.
		const docs = new DocsPage(page);
		await docs.goto('help/use-up-next-pr-tabs-and-ask-to-fix/');

		await expect(docs.pagerNext).toHaveCount(0);
		await expect(docs.pagerPrev).toBeVisible();
	});

	test('pager_should_OmitPrevControl_When_OnFirstPageOfFlattenedOrder', async ({ page }) => {
		// "Getting started" is the first sidebar group's only entry -- the
		// first page in flattened order. Included as the Next-side mirror of
		// the test above, same criterion.
		const docs = new DocsPage(page);
		await docs.goto('help/getting-started/');

		await expect(docs.pagerPrev).toHaveCount(0);
		await expect(docs.pagerNext).toBeVisible();
	});
});
