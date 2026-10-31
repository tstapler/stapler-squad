// Story 3.2.1: features.json reconciles README.md's 5-category Highlights
// grouping with docs/features.md's flat per-section detail. These tests run
// against small fixture files (mkdtemp'd), not the real repo content -- see
// tests/integration/features-json.integration.test.ts for the real-file run.
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
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

async function writeFixtureFiles(dir: string): Promise<{ readmePath: string; featuresMdPath: string }> {
	const readmePath = join(dir, 'README.md');
	const featuresMdPath = join(dir, 'features.md');
	await writeFile(readmePath, FIXTURE_README, ENCODING);
	await writeFile(featuresMdPath, FIXTURE_FEATURES_MD, ENCODING);
	return { readmePath, featuresMdPath };
}

describe('buildFeaturesJson', () => {
	let dir: string;
	let readmePath: string;
	let featuresMdPath: string;

	beforeEach(async () => {
		dir = await mkdtemp(join(tmpdir(), 'build-features-test-'));
		({ readmePath, featuresMdPath } = await writeFixtureFiles(dir));
	});

	afterEach(async () => {
		await rm(dir, { recursive: true, force: true });
	});

	it('buildFeaturesJson_should_Produce3CardsForReviewApprovalCategory_When_ReconcilingReadmeAndFeaturesMd', () => {
		const cards = buildFeaturesJson({ readmePath, featuresMdPath });

		const reviewApprovalCards = cards.filter((card) => card.category === 'Review & Approval');
		expect(reviewApprovalCards).toHaveLength(3);
		expect(reviewApprovalCards.map((card) => card.heading).sort()).toEqual(
			['Approval Analytics', 'Auto-Approval Rules', 'Review Queue'].sort(),
		);
		for (const card of reviewApprovalCards) {
			expect(card.description.length).toBeGreaterThan(0);
		}
	});

	it('buildFeaturesJson_should_OmitScreenshotField_When_SectionHasNoMatchingPng', () => {
		const cards = buildFeaturesJson({ readmePath, featuresMdPath });

		const liveTerminal = cards.find((card) => card.heading === 'Live Terminal Streaming');
		expect(liveTerminal).toBeDefined();
		expect(liveTerminal).not.toHaveProperty('screenshot');

		const dashboard = cards.find((card) => card.heading === 'Dashboard');
		expect(dashboard?.screenshot).toBe('/features/01-dashboard.png');
	});

	// A clip must never reach features.json before it is published (the deploy
	// gate 404s on it), so it is attached only when the manifest lists it.
	it.each([
		['published', { 'session-dashboard.webm': 'abc123' }, true],
		['not yet published', {}, false],
	])('buildFeaturesJson_should_AttachDemoClipOnlyWhenPublished_When_%s', async (_label, manifest, attached) => {
		const manifestPath = join(dir, 'manifest.json');
		await writeFile(manifestPath, JSON.stringify(manifest), ENCODING);

		const cards = buildFeaturesJson({ readmePath, featuresMdPath, manifestPath });

		const dashboard = cards.find((card) => card.heading === 'Dashboard');
		expect(dashboard?.demoClip?.name).toBe(attached ? 'session-dashboard' : undefined);
		// The screenshot is kept either way: it is the clip's poster.
		expect(dashboard?.screenshot).toBe('/features/01-dashboard.png');
	});
});
