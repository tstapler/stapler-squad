// @feature website:tours
// Story 3.1.1 UX-1/2/3/6 plus WCAG 2.4.11 and 1.4.12. Expectations come from
// the build's own inputs (tour-expectations.ts), so the same spec runs against
// the full deploy build and the smoke subset.
import { test, expect } from '@playwright/test';
import { TourPage } from './pages/TourPage';
import { loadTourExpectations } from '../helpers/tour-expectations';

const exp = loadTourExpectations();
const MIN_TARGET_PX = 44;

test.describe('Tours', () => {
	test.skip(exp.sources === 0, 'no tour content authored yet (Epic 3.2)');

	test('toursBuilt_should_BeNonZero_When_ToursExist', () => {
		expect(exp.built.length, 'smoke must always build the hero tour').toBeGreaterThan(0);
	});

	test('index_should_ShowStartHereStripAndOneH2PerCategory_When_Loaded (UX-1)', async ({ page }) => {
		const tours = new TourPage(page);
		await tours.gotoIndex();
		await expect(tours.h1).toHaveCount(1);
		if (exp.startHere.length === 0) {
			await expect(tours.startHere).toHaveCount(0);
		} else {
			await expect(tours.startHere.getByRole('listitem')).toHaveCount(exp.startHere.length);
			for (const t of exp.startHere) {
				await expect(tours.startHere.getByRole('link', { name: `See the tour: ${t.title}` })).toBeVisible();
			}
		}
		await expect(tours.categories).toHaveCount(exp.categories.length);
		for (const category of exp.categories) {
			await expect(tours.categories.getByRole('heading', { level: 2, name: category })).toHaveCount(1);
		}
		const hrefs = await tours.cards.getByRole('link').evaluateAll((as) => as.map((a) => (a as HTMLAnchorElement).getAttribute('href')));
		for (const t of exp.built) expect(hrefs.some((h) => h?.endsWith(`/tours/${t.slug}/`))).toBe(true);
		expect(hrefs.length).toBeGreaterThan(0);
	});

	test('tourPage_should_FollowDocumentedDomOrder_When_Loaded (UX-2)', async ({ page }) => {
		const tours = new TourPage(page);
		await tours.gotoTour(exp.built[0].slug);
		expect(await tours.domOrder()).toEqual(['breadcrumb', 'h1', 'meta', 'video', 'install', 'transcript', 'steps', 'try', 'prevnext', 'all']);
	});

	test('tourPage_should_KeepTextAndFigureBoxes_When_AllMediaBlocked (UX-3)', async ({ page }) => {
		const slug = exp.built[0].slug;
		const tours = new TourPage(page);
		await tours.gotoTour(slug);
		const boxes = async (t: TourPage) =>
			t.figures.evaluateAll((els) =>
				els.map((e) => {
					const r = e.getBoundingClientRect();
					return [Math.round(r.width), Math.round(r.height)];
				}),
			);
		const before = await boxes(tours);

		await page.route(/\.(webm|webp|png)(\?.*)?$/, (route) => route.abort());
		await page.reload({ waitUntil: 'load' });
		await expect(tours.steps).toBeVisible();
		await expect(tours.transcript).toBeAttached();
		await expect(tours.tryBlock).toBeVisible();
		expect(await boxes(tours)).toEqual(before);
		const alts = await tours.figures.locator('img').evaluateAll((imgs) => imgs.map((i) => i.getAttribute('alt')));
		expect(alts.length).toBeGreaterThan(0);
		for (const alt of alts) expect(alt?.trim().length).toBeGreaterThan(0);
		await expect(tours.video).toContainText("doesn't support embedded video");
	});

	for (const viewport of [
		{ width: 390, height: 844 },
		{ width: 320, height: 640 },
	]) {
		test(`pages_should_NotOverflowAndHave44pxTargets_When_Viewport${viewport.width}x${viewport.height} (UX-6)`, async ({ page }) => {
			await page.setViewportSize(viewport);
			const tours = new TourPage(page);
			for (const open of [() => tours.gotoIndex(), () => tours.gotoTour(exp.built[0].slug)]) {
				await open();
				expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
			}
			const targets = [tours.transcriptSummary, tours.installAfterClip, tours.tryInstall, tours.helpLink, tours.allTours, tours.prevNext.getByRole('link').first()];
			for (const target of targets) {
				const box = await target.boundingBox();
				expect(box, 'target visible').not.toBeNull();
				expect(box!.height).toBeGreaterThanOrEqual(MIN_TARGET_PX);
				expect(box!.width).toBeGreaterThanOrEqual(MIN_TARGET_PX);
			}
			await tours.gotoIndex();
			const cardLinks = tours.cards.getByRole('link');
			for (let i = 0; i < (await cardLinks.count()); i++) {
				expect((await cardLinks.nth(i).boundingBox())!.height).toBeGreaterThanOrEqual(MIN_TARGET_PX);
			}
			await tours.gotoTour(exp.built[0].slug);
			const phoneAlts = await page.getByTestId('tour-phone-figure').locator('img').evaluateAll((i) => i.map((x) => x.getAttribute('alt')));
			for (const alt of phoneAlts) expect(alt?.trim()).toBeTruthy();
		});
	}

	test('focusedLinks_should_ClearStickyHeader_When_ScrolledIntoView (2.4.11)', async ({ page }) => {
		const tours = new TourPage(page);
		await tours.gotoTour(exp.built[0].slug);
		const headerBottom = await page.locator('header').first().evaluate((h) => h.getBoundingClientRect().bottom);
		for (const target of [tours.transcriptSummary, tours.helpLink, tours.allTours]) {
			await target.focus();
			await target.evaluate((el) => el.scrollIntoView());
			const top = (await target.boundingBox())!.y;
			expect(top).toBeGreaterThanOrEqual(headerBottom);
		}
	});

	test('textSpacingOverride_should_NotClipOrOverflow_When_AppliedAt390px (1.4.12)', async ({ page }) => {
		await page.setViewportSize({ width: 390, height: 844 });
		const tours = new TourPage(page);
		for (const open of [() => tours.gotoIndex(), () => tours.gotoTour(exp.built[0].slug)]) {
			await open();
			await page.addStyleTag({
				content: '*{line-height:1.5 !important;letter-spacing:0.12em !important;word-spacing:0.16em !important} p{margin-bottom:2em !important}',
			});
			expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
			const clipped = await page.evaluate(() =>
				[...document.querySelectorAll('.tour-card, .tour-meta li, .tour-cta, .tour-link')].filter((e) => e.scrollWidth > e.clientWidth + 1).length,
			);
			expect(clipped).toBe(0);
		}
	});
});
