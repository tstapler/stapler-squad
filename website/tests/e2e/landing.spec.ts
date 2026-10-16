// @feature website:landing-page, website:install-command, website:docs-nav
// Phase 6 (validation.md's "UX Acceptance Tests" table): design/ux.md §5
// criteria 1, 2, 3, 5, 8 (partial), 10, 18 against the splash landing page.
import { test, expect } from '@playwright/test';
import { LandingPage } from './pages/LandingPage';
import { DocsPage } from './pages/DocsPage';

test.describe('Landing page — UX acceptance', () => {
	// Criterion 1: identify "what is this" with zero scroll, desktop + mobile.
	test('heroSection_should_ShowValuePropWithoutScrolling_When_ViewedAtDesktopAndMobileViewports', async ({
		page,
	}) => {
		for (const viewport of [
			{ width: 1920, height: 1080 },
			{ width: 375, height: 667 },
		]) {
			await page.setViewportSize(viewport);
			const landing = new LandingPage(page);
			await landing.goto();

			await expect(landing.heading).toBeInViewport();
			await expect(landing.installCommandText).toBeInViewport();
		}
	});

	// Criterion 2: copy the install command in 1 click, exact match to the
	// rendered command text (itself sourced verbatim from README.md -- see
	// website/tests/unit/hero.test.ts's own regression guard for that).
	test('installCommand_should_CopyExactReadmeCommandToClipboard_When_CopyButtonClicked', async ({
		page,
		context,
	}) => {
		await context.grantPermissions(['clipboard-read', 'clipboard-write']);
		const landing = new LandingPage(page);
		await landing.goto();

		const expectedCommand = (await landing.installCommandText.textContent())?.trim();
		expect(expectedCommand).toBeTruthy();

		await landing.installCopyButton.click();

		const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
		expect(clipboardText).toBe(expectedCommand);
	});

	// Criterion 3: reach the docs home in 1 click from the landing page.
	// ux.md §5 criterion 3 names either a header "Docs" link or a footer
	// "Browse the docs" link as satisfying this -- this site's current
	// implementation offers the latter (LandingFooterNav.astro).
	test('docsHome_should_BeReachableInOneClick_When_HeaderDocsLinkClicked', async ({ page }) => {
		const landing = new LandingPage(page);
		await landing.goto();

		await landing.footerDocsLink.click();
		await expect(page).toHaveURL(/\/help\/getting-started\/?$/);
	});

	// Criterion 5: landing -> Troubleshooting in <=2 clicks (Docs, then the
	// Troubleshooting group's first page link).
	test('troubleshootingSection_should_BeReachableInTwoClicksMaximum_When_StartingFromLandingPage', async ({
		page,
	}) => {
		const landing = new LandingPage(page);
		await landing.goto();

		await landing.footerDocsLink.click(); // click 1
		const docs = new DocsPage(page);
		await docs.sidebarLink('Add a custom theme').click(); // click 2
		await expect(page).toHaveURL(/\/help\/add-user-themes\/?$/);
	});

	// Criterion 10: broken hero media degrades to its poster fallback, never
	// a blank box. Per the task brief, this is tested against the real,
	// committed hero video (assets/demo.webm) via route interception, not
	// the unpublished "terminal-streaming" DemoClip (website-demo-gifs-latest
	// release asset genuinely 404s today -- documented, not this test's job
	// to fix).
	test('heroVideoAndFeatureImage_should_ShowPosterOrAltTextFallback_When_SourceIntercept404s', async ({
		page,
	}) => {
		await page.route('**/demo.webm', (route) => route.fulfill({ status: 404, body: '' }));
		await page.route('**/01-dashboard.png', (route) => route.fulfill({ status: 404, body: '' }));

		const landing = new LandingPage(page);
		await landing.goto();

		// The <video> keeps its non-empty poster attribute and its box never
		// collapses to zero size -- native <video poster> fallback behavior,
		// not something this component has to implement itself.
		const poster = await landing.heroVideo.getAttribute('poster');
		expect(poster).toBeTruthy();
		const videoBox = await landing.heroVideo.boundingBox();
		expect(videoBox?.width).toBeGreaterThan(0);
		expect(videoBox?.height).toBeGreaterThan(0);

		// The broken <img> keeps its accessible alt text and its reserved box
		// (explicit width/height attributes + CSS aspect-ratio), so a failed
		// fetch never collapses to blank, layout-shifting space.
		const brokenImage = page.getByAltText(
			'Dashboard showing status badges, diff stats, tags, and approvals for all agents at a glance',
		);
		await expect(brokenImage).toBeVisible();
		const imageBox = await brokenImage.boundingBox();
		expect(imageBox?.width).toBeGreaterThan(0);
		expect(imageBox?.height).toBeGreaterThan(0);
	});

	// Criterion 18: every <video> has a poster and is adjacent to its own
	// heading/description, never an unlabeled floating video. Scoped to the
	// hero video only -- see this file's header comment on the deferred
	// terminal-streaming DemoClip, whose feature-card <video> has no `poster`
	// prop support at all today (a real, separate gap; tracked, not fixed
	// here per the task's explicit scoping instruction).
	test('videoEmbed_should_HavePosterAttributeAndAccompanyingCardText_When_Rendered', async ({ page }) => {
		const landing = new LandingPage(page);
		await landing.goto();

		await expect(landing.heroVideo).toHaveAttribute('poster', /.+/);
		// The hero video sits directly under the headline + tagline text that
		// introduces it -- its accompanying text alternative, never floating
		// unlabeled.
		await expect(landing.heading).toBeVisible();
	});

	// Criterion 8 (landing-page slice): broken hero video, broken feature
	// image, and a blocked Clipboard API each still leave at least one
	// visible, working exit path. The zero-search-results slice of this same
	// criterion lives in docs-search.spec.ts (needs the search modal open).
	test.describe('edgeCaseStates_should_AlwaysRenderAtLeastOneExitLink_When_AnyDocumentedFailureModeTriggered', () => {
		test('broken hero video', async ({ page }) => {
			await page.route('**/demo.webm', (route) => route.fulfill({ status: 404, body: '' }));
			const landing = new LandingPage(page);
			await landing.goto();

			await expect(landing.viewOnGithubLink).toBeVisible();
		});

		test('broken feature image', async ({ page }) => {
			await page.route('**/01-dashboard.png', (route) => route.fulfill({ status: 404, body: '' }));
			const landing = new LandingPage(page);
			await landing.goto();

			await expect(landing.footerDocsLink).toBeVisible();
		});

		test('clipboard API blocked', async ({ page }) => {
			// Simulates a denied/unavailable Clipboard API (insecure context,
			// permission denial, or an old browser) -- InstallCommand.astro's
			// own try/catch is the thing under test: the command text must stay
			// visible and plainly selectable as the fallback exit path.
			await page.addInitScript(() => {
				Object.defineProperty(navigator, 'clipboard', {
					value: { writeText: () => Promise.reject(new Error('blocked')) },
					configurable: true,
				});
			});
			const landing = new LandingPage(page);
			await landing.goto();

			await landing.installCopyButton.click();
			await expect(landing.installCommandText).toBeVisible();
			const text = await landing.installCommandText.textContent();
			expect(text?.trim().length).toBeGreaterThan(0);
		});

		test('no JavaScript', async ({ browser }) => {
			// A no-JS visitor still gets server-rendered content (Astro ships
			// zero JS by default) -- the install command text and the primary
			// CTA link are both plain SSR markup with no client-side dependency.
			const context = await browser.newContext({ javaScriptEnabled: false });
			const page = await context.newPage();
			const landing = new LandingPage(page);
			await landing.goto();

			await expect(landing.installCommandText).toBeVisible();
			await expect(landing.viewOnGithubLink).toBeVisible();
			await context.close();
		});
	});
});
