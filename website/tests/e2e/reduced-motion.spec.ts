// @feature website:reduced-motion
// design/ux.md §5 criterion 17: prefers-reduced-motion suppresses autoplay
// on every video embed site-wide (hero and any feature-card clips).
import { test, expect } from '@playwright/test';
import { LandingPage } from './pages/LandingPage';

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
