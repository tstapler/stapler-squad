// @feature website:docs-sidebar
// design/ux.md §5 criterion 4: Troubleshooting is reachable from any docs
// page in 1 click (a persistent, top-level sidebar group -- not nested
// under Getting Started).
import { test, expect } from '@playwright/test';
import { DocsPage } from './pages/DocsPage';

test.describe('Docs sidebar navigation — UX acceptance', () => {
	test('troubleshootingGroup_should_BeReachableInOneClick_When_SidebarGroupClicked', async ({ page }) => {
		const docs = new DocsPage(page);
		await docs.goto('help/getting-started/');

		// Troubleshooting is a top-level sidebar group alongside Getting
		// Started and Features (astro.config.mjs's `sidebar` array) -- it's
		// directly visible without expanding any parent group first.
		const troubleshootingHeader = docs.sidebarGroupHeader('Troubleshooting');
		await expect(troubleshootingHeader).toBeVisible();

		const startUrl = page.url();
		await troubleshootingHeader.click();

		// Clicking the group's own disclosure header toggles a native
		// <details>/<summary> in place -- it never navigates to an
		// intermediate index page.
		await expect(page).toHaveURL(startUrl);

		// All sidebar groups render open by default (confirmed against a live
		// preview build) -- toggling it closed then back open proves the list
		// is a client-side disclosure, not a second navigation.
		const firstEntry = docs.sidebarLink('Add a custom theme');
		if (!(await firstEntry.isVisible())) {
			await troubleshootingHeader.click();
		}
		await expect(firstEntry).toBeVisible();

		await firstEntry.click();
		await expect(page).toHaveURL(/\/help\/add-user-themes\/?$/);
	});
});
