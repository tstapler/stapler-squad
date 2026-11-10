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
): Promise<void> {
	const prefersReducedMotion = matchMedia('(prefers-reduced-motion: reduce)').matches;
	if (prefersReducedMotion) {
		return Promise.resolve();
	}
	video.setAttribute('autoplay', '');
	return Promise.resolve(video.play?.()).then(
		() => undefined,
		() => {
			// Autoplay can still be blocked by browser policy even when the user's
			// motion preference allows it -- the poster frame plus visible controls
			// remain the fallback either way.
		},
	);
}

/**
 * Pause/Play control for an autoplaying hero (WCAG 2.2.2). The button exists
 * only when the clip can autoplay: under reduced motion it stays hidden and the
 * native controls are the only affordance. Its state follows the video, so a
 * rejected play() leaves it reading "Play" with aria-pressed="false".
 */
export function startHeroPlayback(
	video: HTMLVideoElement,
	button: HTMLButtonElement | null,
	matchMedia: (query: string) => { matches: boolean } = (query) => window.matchMedia(query),
): Promise<void> {
	if (matchMedia('(prefers-reduced-motion: reduce)').matches) {
		return Promise.resolve();
	}
	const sync = () => {
		if (!button) return;
		button.hidden = false;
		button.textContent = video.paused ? 'Play' : 'Pause';
		button.setAttribute('aria-pressed', String(!video.paused));
	};
	video.addEventListener('play', sync);
	video.addEventListener('pause', sync);
	button?.addEventListener('click', () => {
		if (video.paused) {
			void Promise.resolve(video.play?.()).catch(() => sync());
		} else {
			video.pause();
		}
	});
	return applyAutoplayUnlessReducedMotion(video, matchMedia).then(sync);
}

// Caps how long a busy page can postpone the hero autoplay.
const IDLE_TIMEOUT_MS = 2000;

/**
 * Runs `start` once the page has loaded and the main thread is idle, so
 * decoding the hero clip doesn't count as blocking time during load (mobile
 * Lighthouse performance dropped to ~0.75 with autoplay at load).
 */
export function whenLoadedAndIdle(start: () => void, win: Window = window): void {
	const afterLoad = () => {
		if (win.requestIdleCallback) {
			win.requestIdleCallback(start, { timeout: IDLE_TIMEOUT_MS });
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

/** Starts every hero video (and its Pause/Play button) on the page, after load. */
export function initHeroVideos(doc: Document = document): void {
	doc.querySelectorAll<HTMLVideoElement>('[data-testid="hero-video"]').forEach((video) => {
		const button = video.parentElement?.querySelector<HTMLButtonElement>('[data-testid="hero-pause"]') ?? null;
		whenLoadedAndIdle(() => void startHeroPlayback(video, button), doc.defaultView ?? window);
	});
}
