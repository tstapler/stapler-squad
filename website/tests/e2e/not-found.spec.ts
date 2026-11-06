// @feature website:not-found-page
// design/ux.md §5 criteria 11, 12: the 404 page renders the site's own
// branded chrome with a docs-home link and a way to search, including when
// reached via a stale/excluded internal link that bypasses BrokenLinkCheck.
import { test, expect } from '@playwright/test';

test.describe('404 page — UX acceptance', () => {
	// Criterion 11. ux.md §5's own wording is "a link to the docs home and a
	// way to search" -- this implementation's search affordance is
	// Starlight's header search button (role=button, not a link), so this
	// test follows the authoritative ux.md wording over validation.md's
	// paraphrased "Search the docs" link text.
	test('notFoundPage_should_RenderBrandedChromeWithDocsHomeAndSearchLinks_When_UnmatchedRouteVisited', async ({
		page,
	}) => {
		const response = await page.goto('does-not-exist', { waitUntil: 'load' });
		expect(response?.status()).toBe(404);

		await expect(page.getByRole('banner')).toBeVisible();
		await expect(page.getByRole('link', { name: 'Go to the docs home' })).toBeVisible();
		await expect(page.getByRole('button', { name: /^Search/ })).toBeVisible();
	});

	// Criterion 12: a stale/excluded internal link that bypasses
	// BrokenLinkCheck still falls back to this branded page, never a raw
	// GitHub Pages default 404. Simulated per validation.md's own Steps
	// column: navigate directly to a slug known never to have existed.
	test('notFoundPage_should_BeReachedAsFallback_When_StaleCuratedLinkBypassesBrokenLinkCheck', async ({
		page,
	}) => {
		const response = await page.goto('help/does-not-exist/', { waitUntil: 'load' });
		expect(response?.status()).toBe(404);

		await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible();
		await expect(page.getByRole('link', { name: 'Go to the docs home' })).toBeVisible();
	});
});
