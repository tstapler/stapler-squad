// Story 3.3.1: the hero <video> never carries `autoplay` in its server-
// rendered markup (an Astro component has no access to the client's media
// query at render time); the client-side check in src/lib/reduced-motion-video.ts
// adds it only when `prefers-reduced-motion` doesn't match. The component's
// inline <script> is a thin call into that module (see HeroVideo.astro) --
// pulling the logic out keeps it testable against a jsdom element directly,
// without depending on Astro's script-bundling pipeline.
import { JSDOM } from 'jsdom';
import { describe, expect, it, vi } from 'vitest';
import { experimental_AstroContainer as AstroContainer } from 'astro/container';
import HeroVideo from '../../src/components/HeroVideo.astro';
import { applyAutoplayUnlessReducedMotion, startHeroPlayback, whenLoadedAndIdle } from '../../src/lib/reduced-motion-video.ts';

const PROPS = { src: '/stapler-squad/clips/dashboard-lifecycle.webm', poster: '/stapler-squad/shots/dashboard-lifecycle-poster.webp' };
const MP4 = '/stapler-squad/clips/dashboard-lifecycle.mp4';
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

describe('HeroVideo renditions (Cross-cutting 7e)', () => {
	it('heroVideo_should_ListWebmThenMp4Sources_When_Mp4Given', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(HeroVideo, { props: { ...PROPS, mp4: MP4 } });
		const webm = html.indexOf(`<source src="${PROPS.src}" type="video/webm"`);
		const mp4 = html.indexOf(`<source src="${MP4}" type="video/mp4"`);
		expect(webm).toBeGreaterThan(-1);
		expect(mp4).toBeGreaterThan(webm);
		expect(html).toContain(`poster="${PROPS.poster}"`);
	});
});

describe('whenLoadedAndIdle', () => {
	function fakeWindow(readyState: string, withIdle: boolean) {
		const listeners: Record<string, () => void> = {};
		const idle = vi.fn((cb: () => void) => cb());
		const win = {
			document: { readyState },
			addEventListener: (type: string, cb: () => void) => {
				listeners[type] = cb;
			},
			setTimeout: vi.fn((cb: () => void) => cb()),
			...(withIdle ? { requestIdleCallback: idle } : {}),
		};
		return { win: win as unknown as Window, listeners, idle };
	}

	it('whenLoadedAndIdle_should_WaitForLoadEvent_When_DocumentStillLoading', () => {
		const { win, listeners } = fakeWindow('interactive', true);
		const start = vi.fn();
		whenLoadedAndIdle(start, win);
		expect(start).not.toHaveBeenCalled();
		listeners.load();
		expect(start).toHaveBeenCalledTimes(1);
	});

	it('whenLoadedAndIdle_should_StartWithoutLoadEvent_When_DocumentAlreadyComplete', () => {
		const { win, listeners } = fakeWindow('complete', true);
		const start = vi.fn();
		whenLoadedAndIdle(start, win);
		expect(start).toHaveBeenCalledTimes(1);
		expect(listeners.load).toBeUndefined();
	});

	it('whenLoadedAndIdle_should_PassIdleTimeout_When_RequestIdleCallbackAvailable', () => {
		const { win, idle } = fakeWindow('complete', true);
		whenLoadedAndIdle(vi.fn(), win);
		expect(idle).toHaveBeenCalledWith(expect.any(Function), { timeout: 2000 });
	});

	it('whenLoadedAndIdle_should_FallBackToSetTimeout_When_RequestIdleCallbackMissing', () => {
		const { win } = fakeWindow('complete', false);
		const start = vi.fn();
		whenLoadedAndIdle(start, win);
		expect(start).toHaveBeenCalledTimes(1);
	});
});

describe('HeroVideo pause control and sources', () => {
	it('heroVideo_should_ListWebmThenMp4Sources_When_Mp4Provided', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(HeroVideo, { props: { ...PROPS, mp4: '/stapler-squad/demo.mp4' } });
		expect(html.indexOf('type="video/webm"')).toBeGreaterThan(-1);
		expect(html.indexOf('type="video/webm"')).toBeLessThan(html.indexOf('type="video/mp4"'));
		expect(html).toContain(`poster="${PROPS.poster}"`);
	});

	it('heroVideo_should_RenderHiddenPauseButtonBeforeVideo_When_Rendered', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(HeroVideo, { props: PROPS });
		const button = html.indexOf('data-testid="hero-pause"');
		expect(button).toBeGreaterThan(-1);
		expect(button).toBeLessThan(html.indexOf('data-testid="hero-video"'));
		expect(html).toMatch(/data-testid="hero-pause"[^>]*\bhidden\b/);
	});

	function setup(playImpl: () => Promise<void>) {
		const dom = new JSDOM('<!doctype html><body><button hidden></button><video></video></body>', {
			beforeParse(window) {
				const proto = window.HTMLMediaElement.prototype;
				let paused = true;
				Object.defineProperty(proto, 'paused', { get: () => paused, configurable: true });
				proto.play = function () {
					return playImpl().then(() => {
						paused = false;
						this.dispatchEvent(new window.Event('play'));
					});
				};
				proto.pause = function () {
					paused = true;
					this.dispatchEvent(new window.Event('pause'));
				};
			},
		});
		const doc = dom.window.document;
		return { video: doc.querySelector('video')!, button: doc.querySelector('button')! };
	}

	it('startHeroPlayback_should_ShowPauseAndToggle_When_AutoplaySucceeds', async () => {
		const { video, button } = setup(() => Promise.resolve());
		await startHeroPlayback(video, button, () => ({ matches: false }));
		expect(button.hidden).toBe(false);
		expect(button.textContent).toBe('Pause');
		expect(button.getAttribute('aria-pressed')).toBe('true');
		button.click();
		expect(button.textContent).toBe('Play');
		expect(button.getAttribute('aria-pressed')).toBe('false');
		button.click();
		await Promise.resolve();
		await Promise.resolve();
		expect(button.textContent).toBe('Pause');
	});

	it('startHeroPlayback_should_StayPausedReadingPlay_When_PlayRejected', async () => {
		const { video, button } = setup(() => Promise.reject(new Error('NotAllowedError')));
		await startHeroPlayback(video, button, () => ({ matches: false }));
		expect(button.hidden).toBe(false);
		expect(button.textContent).toBe('Play');
		expect(button.getAttribute('aria-pressed')).toBe('false');
	});

	it('startHeroPlayback_should_RenderNoButton_When_PrefersReducedMotion', async () => {
		const { video, button } = setup(() => Promise.resolve());
		await startHeroPlayback(video, button, () => ({ matches: true }));
		expect(button.hidden).toBe(true);
		expect(video.hasAttribute('autoplay')).toBe(false);
	});
});
