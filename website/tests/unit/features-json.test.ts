// Story 3.2.1: features.json reconciles README.md's 5-category Highlights
// grouping with docs/features.md's flat per-section detail. These tests run
// against small fixture files (mkdtemp'd), not the real repo content -- see
// tests/integration/features-json.integration.test.ts for the real-file run.
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { buildFeaturesJson } from '../../scripts/build-features.mjs';

const ENCODING = 'utf8';

const FIXTURE_README = `# Fixture

### Highlights

**Visibility**
- **Real-time dashboard** — status badges

**Review & Approval**
- **Auto-approval rules engine** — blocks dangerous operations
- **Review Queue** — structured triage
- **Approval analytics** — visualise trends

### Installation
`;

const FIXTURE_FEATURES_MD = `# Fixture Feature Overview

## Dashboard

See all your AI agents at a glance.

![Dashboard](../assets/demo-recording/01-dashboard.png)

---

## Live Terminal Streaming

Full xterm.js terminal per session.

---

## Review Queue

Structured triage for every pending agent action.

![Review Queue](../assets/demo-recording/05-review-queue.png)

---

## Auto-Approval Rules

42 built-in rules block dangerous operations automatically.

---

## Approval Analytics

Visualise decision trends and classifier performance over time.
`;

const FIXTURE_MEDIA = {
	Dashboard: { clip: 'session-dashboard', shot: '01-dashboard', alt: 'Dashboard alt' },
	'Review Queue': { shot: '05-review-queue', alt: 'Review queue alt' },
};

async function writeFixtureFiles(dir: string): Promise<{ readmePath: string; featuresMdPath: string; mediaPath: string }> {
	const readmePath = join(dir, 'README.md');
	const featuresMdPath = join(dir, 'features.md');
	const mediaPath = join(dir, 'feature-media.json');
	await writeFile(readmePath, FIXTURE_README, ENCODING);
	await writeFile(featuresMdPath, FIXTURE_FEATURES_MD, ENCODING);
	await writeFile(mediaPath, JSON.stringify(FIXTURE_MEDIA), ENCODING);
	return { readmePath, featuresMdPath, mediaPath };
}

describe('buildFeaturesJson', () => {
	let dir: string;
	let readmePath: string;
	let featuresMdPath: string;
	let mediaPath: string;

	beforeEach(async () => {
		dir = await mkdtemp(join(tmpdir(), 'build-features-test-'));
		({ readmePath, featuresMdPath, mediaPath } = await writeFixtureFiles(dir));
	});

	afterEach(async () => {
		await rm(dir, { recursive: true, force: true });
	});

	it('buildFeaturesJson_should_Produce3CardsForReviewApprovalCategory_When_ReconcilingReadmeAndFeaturesMd', () => {
		const cards = buildFeaturesJson({ readmePath, featuresMdPath, mediaPath });

		const reviewApprovalCards = cards.filter((card) => card.category === 'Review & Approval');
		expect(reviewApprovalCards).toHaveLength(3);
		expect(reviewApprovalCards.map((card) => card.heading).sort()).toEqual(
			['Approval Analytics', 'Auto-Approval Rules', 'Review Queue'].sort(),
		);
		for (const card of reviewApprovalCards) {
			expect(card.description.length).toBeGreaterThan(0);
		}
	});

	it('buildFeaturesJson_should_OmitScreenshotField_When_HeadingHasNoMediaEntry', () => {
		const cards = buildFeaturesJson({ readmePath, featuresMdPath, mediaPath });

		const liveTerminal = cards.find((card) => card.heading === 'Live Terminal Streaming');
		expect(liveTerminal).toBeDefined();
		expect(liveTerminal).not.toHaveProperty('screenshot');

		const dashboard = cards.find((card) => card.heading === 'Dashboard');
		expect(dashboard?.screenshot).toBe('/features/01-dashboard.png');
		expect(dashboard?.alt).toBe('Dashboard alt');
	});

	// Media is declared by name, so no file on disk changes the output (Story 3.1.2).
	it('buildFeaturesJson_should_AttachDemoClipByName_When_MediaEntryDeclaresClip', () => {
		const cards = buildFeaturesJson({ readmePath, featuresMdPath, mediaPath });

		expect(cards.find((card) => card.heading === 'Dashboard')?.demoClip).toEqual({ name: 'session-dashboard', ext: 'webm' });
		expect(cards.find((card) => card.heading === 'Review Queue')).not.toHaveProperty('demoClip');
	});

	it('buildFeaturesJson_should_BeIdentical_When_ClipFilesPresentOrAbsent', async () => {
		const before = buildFeaturesJson({ readmePath, featuresMdPath, mediaPath });
		await mkdir(join(dir, 'public', 'clips'), { recursive: true });
		await writeFile(join(dir, 'public', 'clips', 'session-dashboard.webm'), 'x');
		expect(buildFeaturesJson({ readmePath, featuresMdPath, mediaPath })).toEqual(before);
	});

	it('buildFeaturesJson_should_NameHeading_When_MediaEntryHasNoMatchingSection', async () => {
		await writeFile(mediaPath, JSON.stringify({ 'No Such Section': { shot: 's', alt: 'a' } }), ENCODING);
		expect(() => buildFeaturesJson({ readmePath, featuresMdPath, mediaPath })).toThrow(/"No Such Section".*not a section heading/);
	});

	it('buildFeaturesJson_should_NameHeading_When_MediaEntryMissingAlt', async () => {
		await writeFile(mediaPath, JSON.stringify({ Dashboard: { shot: 's' } }), ENCODING);
		expect(() => buildFeaturesJson({ readmePath, featuresMdPath, mediaPath })).toThrow(/"Dashboard" needs a non-empty "shot" and "alt"/);
	});
});
