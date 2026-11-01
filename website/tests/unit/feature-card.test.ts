// Story 3.2.2: FeatureCard.astro renders real descriptive alt text when a
// screenshot is provided, and renders no <img> element at all when it isn't
// (most features.json entries have no screenshot).
import { describe, expect, it } from 'vitest';
import { experimental_AstroContainer as AstroContainer } from 'astro/container';
import FeatureCard from '../../src/components/FeatureCard.astro';

describe('FeatureCard', () => {
	it('featureCard_should_RenderDescriptiveAltText_When_ScreenshotProvided', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(FeatureCard, {
			props: {
				heading: 'Dashboard',
				description: 'See all your AI agents at a glance.',
				category: 'Visibility',
				screenshot: '/features/01-dashboard.png',
				alt: 'Dashboard showing status badges, diff stats, tags, and approvals for all agents at a glance',
			},
		});

		expect(html).toContain('src="/features/01-dashboard.png"');
		expect(html).toContain(
			'alt="Dashboard showing status badges, diff stats, tags, and approvals for all agents at a glance"',
		);
		// Regression guard: alt text must be a real sentence, not the filename.
		expect(html).not.toContain('alt="01-dashboard.png"');
	});

	it('featureCard_should_RenderWithoutImageElement_When_ScreenshotFieldAbsent', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(FeatureCard, {
			props: {
				heading: 'Dashboard',
				description: 'Full xterm.js terminal per session with real-time output streaming.',
				category: 'Visibility',
			},
		});

		expect(html).not.toContain('<img');
		expect(html).toContain('Dashboard');
	});

	// Story 3.3.1's second AC: a screenshot-less card with a `demoClip` field
	// renders a <video> served from the site's own /clips/ path.
	it('featureCard_should_RenderVideoFromSiteClipsPath_When_DemoClipFieldPresent', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(FeatureCard, {
			props: {
				heading: 'Dashboard',
				description: 'Full xterm.js terminal per session with real-time output streaming.',
				category: 'Visibility',
				demoClip: { name: 'session-dashboard', ext: 'webm' },
			},
		});

		expect(html).not.toContain('<img');
		expect(html).toMatch(/src="[^"]*\/clips\/session-dashboard\.webm"/);
		expect(html).toContain('<video');
	});

	// A published clip replaces the screenshot, which stays as the video poster.
	it('featureCard_should_PlayClipWithScreenshotAsPoster_When_BothScreenshotAndDemoClipPresent', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(FeatureCard, {
			props: {
				heading: 'Dashboard',
				description: 'All agents at a glance.',
				category: 'Visibility',
				screenshot: '/features/01-dashboard.png',
				alt: 'Dashboard',
				demoClip: { name: 'session-dashboard', ext: 'webm' },
			},
		});

		expect(html).toContain('<video');
		expect(html).toContain('session-dashboard.webm');
		// Lazy: an eager poster= on every card starved the landing page's LCP (see FeatureCard.astro).
		expect(html).toMatch(/data-poster="[^"]*01-dashboard\.png"/);
		expect(html).not.toMatch(/\sposter=/);
		expect(html).not.toContain('<img');
	});
});
