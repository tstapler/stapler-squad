// Story 3.3.1: the hero <video> never carries `autoplay` in its server-
// rendered markup (an Astro component has no access to the client's media
// query at render time); the client-side check in src/lib/reduced-motion-video.ts
// adds it only when `prefers-reduced-motion` doesn't match. The component's
// inline <script> is a thin call into that module (see HeroVideo.astro) --
// pulling the logic out keeps it testable against a jsdom element directly,
// without depending on Astro's script-bundling pipeline.
import { JSDOM } from 'jsdom';
import { describe, expect, it } from 'vitest';
import { experimental_AstroContainer as AstroContainer } from 'astro/container';
import HeroVideo from '../../src/components/HeroVideo.astro';
import { applyAutoplayUnlessReducedMotion } from '../../src/lib/reduced-motion-video.ts';

const PROPS = { src: '/stapler-squad/demo.webm', poster: '/stapler-squad/screenshot.png' };
// Boolean attributes HeroVideo.astro always sets server-side, named once so
// both the markup assertions and the jsdom fixture below share one spelling.
// kibitzer:ignore replace-magic-literal -- these are the literal HTML
// attribute names under test ('loop'/'muted'/'poster'), not configurable
// values; the array above is already the single source for the two
// boolean ones, and the `expect(html).toContain('loop')`-style assertions
// necessarily repeat the literal they're asserting against.
const STATIC_VIDEO_ATTRIBUTES = ['loop', 'muted', 'controls'];

function createVideoElement(prefersReducedMotion: boolean) {
	const dom = new JSDOM('<!doctype html><html><body></body></html>', {
		beforeParse(window) {
			// jsdom doesn't implement media playback.
			window.HTMLMediaElement.prototype.play = () => Promise.resolve();
		},
	});
	const video = dom.window.document.createElement('video');
	video.setAttribute('data-testid', 'hero-video');
	for (const attribute of STATIC_VIDEO_ATTRIBUTES) {
		video.setAttribute(attribute, '');
	}
	video.setAttribute('poster', PROPS.poster);
	dom.window.document.body.appendChild(video);
	applyAutoplayUnlessReducedMotion(video, () => ({ matches: prefersReducedMotion }));
	return video;
}

describe('HeroVideo component markup', () => {
	it('heroVideo_should_SetPosterAndAutoplayLoopMuted_When_MotionNotReduced', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(HeroVideo, { props: PROPS });

		expect(html).toContain('loop');
		expect(html).toContain('muted');
		expect(html).toContain('controls');
		expect(html).toContain(`poster="${PROPS.poster}"`);
		expect(html).toContain(`src="${PROPS.src}"`);

		const video = createVideoElement(false);
		expect(video.hasAttribute('autoplay')).toBe(true);
		expect(video.hasAttribute('loop')).toBe(true);
		expect(video.hasAttribute('muted')).toBe(true);
		expect(video.getAttribute('poster')).toBe(PROPS.poster);
	});

	it('heroVideo_should_OmitAutoplayAttribute_When_PrefersReducedMotionMatches', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(HeroVideo, { props: PROPS });

		// Server-rendered markup never carries autoplay at all -- it's only
		// ever added client-side, after the reduced-motion check below.
		expect(html).not.toMatch(/<video[^>]*\bautoplay\b/);

		const video = createVideoElement(true);
		expect(video.hasAttribute('autoplay')).toBe(false);
		expect(video.getAttribute('poster')).toBe(PROPS.poster);
	});
});
