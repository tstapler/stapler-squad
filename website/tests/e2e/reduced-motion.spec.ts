// @feature website:reduced-motion
// design/ux.md §5 criterion 17: prefers-reduced-motion suppresses autoplay
// on every video embed site-wide (hero and any feature-card clips).
import { test, expect } from '@playwright/test';
import { LandingPage } from './pages/LandingPage';
import { TourPage } from './pages/TourPage';
import { loadTourExpectations } from '../helpers/tour-expectations';

test.describe('prefers-reduced-motion — UX acceptance', () => {
	test('allVideoEmbeds_should_SuppressAutoplay_When_PrefersReducedMotionEmulated', async ({ page }) => {
		await page.emulateMedia({ reducedMotion: 'reduce' });
		const landing = new LandingPage(page);
		await landing.goto();

		// reduced-motion-video.ts's applyAutoplayUnlessReducedMotion() must not
		// have set autoplay on the hero video.
		await expect(landing.heroVideo).not.toHaveAttribute('autoplay');

		// Feature-card clips (present only in the deploy build) never get an
		// autoplay attribute at all, with or without reduced motion --
		// FeatureCard.astro's markup simply never sets it.
		const featureCardVideo = page.getByTestId('feature-card-video');
		if (await featureCardVideo.count() > 0) {
			await expect(featureCardVideo.first()).not.toHaveAttribute('autoplay');
		}
	});

	test('heroVideo_should_SetAutoplay_When_MotionNotReduced', async ({ page }) => {
		// Control case for the test above: with no reduced-motion preference,
		// the hero video DOES get autoplay -- proving the first test's
		// assertion is actually exercising the reduced-motion branch, not just
		// observing a video that never autoplays regardless.
		const landing = new LandingPage(page);
		await landing.goto();

		await expect(landing.heroVideo).toHaveAttribute('autoplay', '');
	});
});

// Story 3.1.1 UX-5 (G11): reduced motion on the landing page and every tour page.
test.describe('prefers-reduced-motion -- tours and landing', () => {
	const exp = loadTourExpectations();
	const paths = ['', ...exp.built.map((t) => `tours/${t.slug}/`)];

	for (const path of paths) {
		test(`videos_should_NotAutoplayAndHavePosterAndControls_When_ReducedMotionOn ${path || '/'}`, async ({ page }) => {
			await page.emulateMedia({ reducedMotion: 'reduce' });
			await page.goto(path, { waitUntil: 'load' });
			const videos = await page.locator('video').evaluateAll((vs) =>
				vs.map((v) => ({ autoplay: v.hasAttribute('autoplay'), poster: !!v.getAttribute('poster'), controls: v.hasAttribute('controls'), paused: (v as HTMLVideoElement).paused })),
			);
			expect(videos.length).toBeGreaterThan(0);
			for (const v of videos) {
				expect(v).toEqual({ autoplay: false, poster: true, controls: true, paused: true });
			}
		});
	}

	test('tourPage_should_ComputeZeroDurations_When_ReducedMotionOn', async ({ page }) => {
		test.skip(exp.built.length === 0, 'no tour content authored yet (Epic 3.2)');
		await page.emulateMedia({ reducedMotion: 'reduce' });
		const tours = new TourPage(page);
		await tours.gotoIndex();
		await tours.gotoTour(exp.built[0].slug);
		const durations = await page.evaluate(() =>
			[...document.querySelectorAll('.tour-cta, .tour-link, .tour-shot img')].map((el) => {
				const cs = getComputedStyle(el);
				return [cs.transitionDuration, cs.animationDuration];
			}),
		);
		expect(durations.length).toBeGreaterThan(0);
		for (const [transition, animation] of durations) {
			expect(transition).toBe('0s');
			expect(animation).toBe('0s');
		}
	});

	test('hero_should_RenderNoPauseButton_When_ReducedMotionOn', async ({ page }) => {
		await page.emulateMedia({ reducedMotion: 'reduce' });
		const landing = new LandingPage(page);
		await landing.goto();
		await expect(landing.heroPauseButton).toBeHidden();
	});
});
