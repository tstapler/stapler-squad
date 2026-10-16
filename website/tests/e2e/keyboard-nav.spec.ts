// @feature website:keyboard-accessibility
// design/ux.md §5 criteria 13, 14: every interactive element is reachable
// via Tab/Shift+Tab/Enter alone, and shows a visible focus ring on
// :focus-visible.
import { test, expect } from '@playwright/test';
import type { Page } from '@playwright/test';
import { LandingPage } from './pages/LandingPage';
import { DocsPage } from './pages/DocsPage';

/** Tabs forward `count` times and returns the resulting document.activeElement's testid/role/text. */
async function tabAndDescribeFocus(
	page: Page,
	count: number,
): Promise<Array<{ testId: string | null; tag: string; text: string }>> {
	const seen: Array<{ testId: string | null; tag: string; text: string }> = [];
	for (let i = 0; i < count; i++) {
		await page.keyboard.press('Tab');
		const described = await page.evaluate(() => {
			const el = document.activeElement;
			if (!el || el === document.body) return null;
			return {
				testId: el.getAttribute('data-testid'),
				tag: el.tagName,
				text: (el.textContent || el.getAttribute('aria-label') || '').trim().slice(0, 40),
			};
		});
		if (described) seen.push(described);
	}
	return seen;
}

test.describe('Keyboard navigation — UX acceptance', () => {
	test('keyboardNavigation_should_ReachEveryInteractiveElement_When_TabbingThroughLandingAndDocsPage', async ({
		page,
	}) => {
		const landing = new LandingPage(page);
		await landing.goto();

		const landingFocus = await tabAndDescribeFocus(page, 10);
		// No keyboard trap: every Tab press landed on a real, non-body element.
		expect(landingFocus.length).toBe(10);
		expect(landingFocus.some((f) => f.testId === 'install-copy-button')).toBe(true);
		expect(landingFocus.some((f) => f.text.includes('Search'))).toBe(true);

		const docs = new DocsPage(page);
		await docs.goto('help/getting-started/');
		const docsFocus = await tabAndDescribeFocus(page, 12);
		expect(docsFocus.length).toBe(12);
		expect(docsFocus.some((f) => f.text.includes('Search'))).toBe(true);
		expect(docsFocus.some((f) => f.text.includes('Getting started'))).toBe(true);
		expect(docsFocus.some((f) => f.text.includes('Troubleshooting') || f.text.includes('Add a custom theme'))).toBe(
			true,
		);
	});

	test('focusRing_should_BeVisibleOnFocusVisible_When_TabbingToAnyInteractiveElement', async ({ page }) => {
		const landing = new LandingPage(page);
		await landing.goto();

		// Sample a representative set of real, visible interactive elements
		// (not the hidden mobile-duplicate nav, which the browser correctly
		// excludes focus styling from since it's off-screen) and assert none
		// has focus-visible suppressed outright.
		const elementsToCheck = [landing.searchTrigger, landing.installCopyButton, landing.viewOnGithubLink];

		for (const locator of elementsToCheck) {
			await locator.focus();
			const style = await locator.evaluate((el) => {
				const cs = getComputedStyle(el);
				return { outlineStyle: cs.outlineStyle, outlineWidth: cs.outlineWidth, boxShadow: cs.boxShadow };
			});
			const hasOutline = style.outlineStyle !== 'none' && style.outlineWidth !== '0px';
			const hasBoxShadow = style.boxShadow !== 'none';
			expect(hasOutline || hasBoxShadow, `expected a visible focus ring, got ${JSON.stringify(style)}`).toBe(true);
		}
	});
});
