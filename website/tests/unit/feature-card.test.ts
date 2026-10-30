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
				heading: 'Live Terminal Streaming',
				description: 'Full xterm.js terminal per session with real-time output streaming.',
				category: 'Visibility',
			},
		});

		expect(html).not.toContain('<img');
		expect(html).toContain('Live Terminal Streaming');
	});

	// Story 3.3.1's second AC: a screenshot-less card with a `demoClip` field
	// renders a <video> whose src is releaseAssetUrl()'s output, never a
	// hardcoded literal duplicated from that helper.
	it('featureCard_should_RenderVideoViaReleaseAssetUrl_When_DemoClipFieldPresent', async () => {
		const container = await AstroContainer.create();
		const html = await container.renderToString(FeatureCard, {
			props: {
				heading: 'Live Terminal Streaming',
				description: 'Full xterm.js terminal per session with real-time output streaming.',
				category: 'Visibility',
				demoClip: { tag: 'website-demo-gifs-latest', name: 'terminal-streaming', ext: 'webm' },
			},
		});

		expect(html).not.toContain('<img');
		expect(html).toContain(
			'src="https://github.com/tstapler/stapler-squad/releases/download/website-demo-gifs-latest/terminal-streaming.webm"',
		);
		expect(html).toContain('<video');
	});
});
