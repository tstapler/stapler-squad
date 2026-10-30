// Shared client-side logic behind HeroVideo.astro's prefers-reduced-motion
// check (design/ux.md §1.4). Pulled out of the component's inline <script>
// into a plain, directly importable module so it's unit-testable against a
// jsdom video element without needing Astro's script-bundling pipeline.

/**
 * Sets `autoplay` on a video element and attempts to play it, unless the
 * caller's `matchMedia` reports `prefers-reduced-motion: reduce`. Never sets
 * `autoplay` in that case, so a visitor who opted into reduced motion never
 * gets forced motion -- they still get the `poster` frame and visible
 * controls, and can press play manually.
 */
export function applyAutoplayUnlessReducedMotion(
	video: HTMLVideoElement,
	matchMedia: (query: string) => { matches: boolean } = (query) => window.matchMedia(query),
): void {
	const prefersReducedMotion = matchMedia('(prefers-reduced-motion: reduce)').matches;
	if (prefersReducedMotion) {
		return;
	}
	video.setAttribute('autoplay', '');
	video.play?.()?.catch(() => {
		// Autoplay can still be blocked by browser policy even when the user's
		// motion preference allows it -- the poster frame plus visible controls
		// remain the fallback either way.
	});
}

/**
 * Runs `start` once the page has loaded and the main thread is idle, so
 * decoding the hero clip doesn't count as blocking time during load (mobile
 * Lighthouse performance dropped to ~0.75 with autoplay at load).
 */
function whenLoadedAndIdle(start: () => void, win: Window = window): void {
	const afterLoad = () => {
		if (win.requestIdleCallback) {
			win.requestIdleCallback(start);
		} else {
			win.setTimeout(start, 0);
		}
	};
	if (win.document.readyState === 'complete') {
		afterLoad();
	} else {
		win.addEventListener('load', afterLoad, { once: true });
	}
}

/** Applies applyAutoplayUnlessReducedMotion to every hero video on the page, after load. */
export function initHeroVideos(doc: Document = document): void {
	doc.querySelectorAll<HTMLVideoElement>('[data-testid="hero-video"]').forEach((video) => {
		whenLoadedAndIdle(() => applyAutoplayUnlessReducedMotion(video), doc.defaultView ?? window);
	});
}
