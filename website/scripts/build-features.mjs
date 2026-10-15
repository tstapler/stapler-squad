#!/usr/bin/env node
// build-features.mjs
//
// Reconciles README.md's 5-category "Highlights" grouping with
// docs/features.md's flat 17-section detail (research/features.md §4's
// "two different taxonomies of the same feature set") into one
// FeatureCard[] array, written to website/src/content/features.json.
//
// Headings and descriptions are parsed live from docs/features.md so this
// never drifts from the real file; category and alt-text are a hand-curated
// lookup (CATEGORY_BY_HEADING / ALT_TEXT_BY_HEADING below) since reconciling
// two human-authored taxonomies and writing a real descriptive sentence
// isn't mechanically derivable. README.md is also read and its five category
// names cross-checked against CATEGORY_BY_HEADING's values, so a renamed
// README category is caught at build time instead of silently drifting.

import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
const WEBSITE_ROOT = join(SCRIPT_DIR, '..');
// Repo root is one level up from website/.
const REPO_ROOT = join(WEBSITE_ROOT, '..');
const DEFAULT_README_PATH = join(REPO_ROOT, 'README.md');
const DEFAULT_FEATURES_MD_PATH = join(REPO_ROOT, 'docs', 'features.md');
const DEFAULT_OUTPUT_PATH = join(WEBSITE_ROOT, 'src', 'content', 'features.json');
const PUBLIC_FEATURES_DIR = join(WEBSITE_ROOT, 'public', 'features');
const ENCODING = 'utf8';

// README.md's 5 Highlights category names, named once here so
// CATEGORY_BY_HEADING below can't typo/duplicate one as a fresh string.
const CATEGORY = {
	VISIBILITY: 'Visibility',
	ORGANISATION: 'Organisation',
	REVIEW_APPROVAL: 'Review & Approval',
	HISTORY_DEBUGGING: 'History & Debugging',
	INFRASTRUCTURE: 'Infrastructure',
};

// Display order for FeatureGrid.astro's category headings. Derived from
// CATEGORY (not re-declared) so a renamed/added/removed category can't drift
// out of sync between the two files the way it did before.
export const CATEGORY_ORDER = Object.values(CATEGORY);

/**
 * README's 5 Highlights categories, reconciling docs/features.md's 17
 * sections against them. Two sections have no direct README bullet of their
 * own (README folds them into a sibling's one-liner) and needed a judgment
 * call, noted here rather than left implicit:
 *   - "Status Filtering" -> Organisation (it's a filtering/organisation
 *     feature, grouped with Instant Search/Tag-Based Grouping).
 *   - "Custom Rules" -> Organisation, not Review & Approval. README's
 *     "Review & Approval" bullet list names exactly 3 items (Auto-approval
 *     rules engine, Review Queue, Approval analytics), and Custom Rules is
 *     mentioned only inline inside the Auto-approval bullet's one-liner, not
 *     as its own top-level item -- so grouping it under Review & Approval
 *     here would silently inflate that category past what README documents.
 *     Organisation is the next-best fit: it's about customizing/organizing
 *     rule behavior, same family as the other user-configuration features.
 *
 * @type {Record<string, string>}
 */
const CATEGORY_BY_HEADING = {
	Dashboard: CATEGORY.VISIBILITY,
	'Live Terminal Streaming': CATEGORY.VISIBILITY,
	'Diff Viewer': CATEGORY.VISIBILITY,
	Notifications: CATEGORY.VISIBILITY,
	'Instant Search': CATEGORY.ORGANISATION,
	'Status Filtering': CATEGORY.ORGANISATION,
	'Tag-Based Grouping': CATEGORY.ORGANISATION,
	'Workspace Switcher': CATEGORY.ORGANISATION,
	'Bulk Actions': CATEGORY.ORGANISATION,
	'Custom Rules': CATEGORY.ORGANISATION,
	'Review Queue': CATEGORY.REVIEW_APPROVAL,
	'Auto-Approval Rules': CATEGORY.REVIEW_APPROVAL,
	'Approval Analytics': CATEGORY.REVIEW_APPROVAL,
	'History Search': CATEGORY.HISTORY_DEBUGGING,
	'Logs Viewer': CATEGORY.HISTORY_DEBUGGING,
	'Config Viewer': CATEGORY.HISTORY_DEBUGGING,
	'Isolated Git Workspaces': CATEGORY.INFRASTRUCTURE,
};

/**
 * Descriptive alt-text sentences for the 8 sections with a real screenshot,
 * reusing docs/features.md's own prose (per research/ux.md §3) rather than
 * the screenshot's filename or its short Markdown image alt text.
 *
 * @type {Record<string, string>}
 */
const ALT_TEXT_BY_HEADING = {
	Dashboard:
		'Dashboard showing status badges, diff stats, tags, and approvals for all agents at a glance',
	'Instant Search':
		'Instant search narrowing the session list from 6 agents down to 2 as the query "payment" is typed',
	'Status Filtering':
		'Status filter showing every session with a Needs Approval gate, with other statuses hidden',
	'Tag-Based Grouping':
		'Sessions grouped by tag, with a single session appearing under more than one tag group',
	'Review Queue':
		'Review Queue listing pending agent actions with diffs ready to approve or reject before they reach the codebase',
	'Auto-Approval Rules':
		'List of 42 built-in auto-approval rules covering read-only commands, safe git operations, and common dev tasks',
	'Custom Rules':
		'Adding a custom approval-rule pattern like ^git log so it is never flagged for manual review again',
	'Isolated Git Workspaces':
		'Each agent session running in its own isolated git worktree with no branch conflicts between sessions',
};

/**
 * Story 3.3.1's second AC: exactly one of the 9 screenshot-less sections gets
 * a `DemoClip` video embed (via `ReleaseAssetRef`), proving the embed wiring
 * exists and is exercised -- the other 8 stay text-only per
 * research/features.md §4. "Live Terminal Streaming" is the plan's own
 * example. The clip itself (`terminal-streaming.webm`) isn't published yet
 * (Phase 5 is deferred), so `check-asset-health.mjs` is expected to fail on
 * it until then.
 *
 * @type {Record<string, {tag: string, name: string, ext: string}>}
 */
const DEMO_CLIP_BY_HEADING = {
	'Live Terminal Streaming': { tag: 'website-demo-gifs-latest', name: 'terminal-streaming', ext: 'webm' },
};

/**
 * @typedef {{heading: string, description: string, category: string, screenshot?: string, alt?: string, demoClip?: {tag: string, name: string, ext: string}}} FeatureCard
 */

/**
 * Splits docs/features.md into its `## ` sections.
 *
 * @param {string} markdown
 * @returns {{heading: string, body: string}[]}
 */
export function parseFeaturesMdSections(markdown) {
	// kibitzer:ignore replace-magic-literal -- '\n' is the literal line-separator
	// used for split/join throughout this file, not a configurable value.
	const lines = markdown.split('\n');
	/** @type {{heading: string, body: string}[]} */
	const sections = [];
	let currentHeading = null;
	let currentBody = [];
	for (const line of lines) {
		const headingMatch = line.match(/^##\s+(.+?)\s*$/);
		if (headingMatch) {
			if (currentHeading !== null) {
				sections.push({ heading: currentHeading, body: currentBody.join('\n') });
			}
			currentHeading = headingMatch[1].trim();
			currentBody = [];
			continue;
		}
		if (currentHeading !== null) {
			currentBody.push(line);
		}
	}
	if (currentHeading !== null) {
		sections.push({ heading: currentHeading, body: currentBody.join('\n') });
	}
	return sections;
}

/**
 * Extracts a section's prose description: its body with the Markdown image
 * reference and the `---` separator stripped, collapsed to a single
 * trimmed line.
 *
 * @param {string} body
 * @returns {string}
 */
export function extractSectionDescription(body) {
	return body
		.split('\n')
		.map((line) => line.trim())
		.filter((line) => line !== '' && line !== '---' && !line.startsWith('!['))
		.join(' ')
		.trim();
}

/**
 * Extracts a section's screenshot reference (if any), rewritten from the
 * source's `../assets/demo-recording/0N-*.png` relative path to the
 * `/features/0N-*.png` path it's copied to under website/public/features/.
 * Throws if the rewritten path's target file is missing from
 * website/public/features/, so a stale/typo'd filename in docs/features.md
 * fails the build instead of producing a silently dead <img> on the live site.
 *
 * @param {string} body
 * @param {string} heading
 * @returns {string | undefined}
 */
export function extractSectionScreenshot(body, heading) {
	const match = body.match(/!\[[^\]]*\]\(\.\.\/assets\/demo-recording\/([\w.-]+)\)/);
	if (!match) return undefined;
	const filename = match[1];
	if (!existsSync(join(PUBLIC_FEATURES_DIR, filename))) {
		throw new Error(
			`"${heading}" references screenshot "${filename}", which does not exist under ` +
				`website/public/features/`,
		);
	}
	return `/features/${filename}`;
}

/**
 * Reads README.md's five Highlights category names (the bold lines directly
 * under `### Highlights`), used only to cross-check CATEGORY_BY_HEADING's
 * values below -- so a renamed/added/removed README category is caught here
 * instead of silently drifting out of sync.
 *
 * @param {string} readmeMarkdown
 * @returns {string[]}
 */
export function extractReadmeCategoryNames(readmeMarkdown) {
	const lines = readmeMarkdown.split('\n');
	const startIndex = lines.findIndex((line) => line.trim() === '### Highlights');
	if (startIndex === -1) {
		throw new Error('Could not find "### Highlights" section in README.md');
	}
	const names = [];
	for (let i = startIndex + 1; i < lines.length; i++) {
		const line = lines[i].trim();
		if (line.startsWith('###')) break; // next top-level subsection ends Highlights
		const boldMatch = line.match(/^\*\*(.+?)\*\*$/);
		if (boldMatch) names.push(boldMatch[1]);
	}
	return names;
}

/**
 * Builds the full FeatureCard[] data set, reconciling README.md's category
 * names with docs/features.md's 17 sections.
 *
 * @param {{readmePath?: string, featuresMdPath?: string}} [options]
 * @returns {FeatureCard[]}
 */
export function buildFeaturesJson(options = {}) {
	const readmePath = options.readmePath ?? DEFAULT_README_PATH;
	const featuresMdPath = options.featuresMdPath ?? DEFAULT_FEATURES_MD_PATH;

	const readmeMarkdown = readFileSync(readmePath, ENCODING);
	const featuresMarkdown = readFileSync(featuresMdPath, ENCODING);

	const readmeCategories = new Set(extractReadmeCategoryNames(readmeMarkdown));
	const sections = parseFeaturesMdSections(featuresMarkdown);

	return sections.map(({ heading, body }) => {
		const category = CATEGORY_BY_HEADING[heading];
		if (!category) {
			throw new Error(
				`docs/features.md section "${heading}" has no CATEGORY_BY_HEADING entry in build-features.mjs`,
			);
		}
		if (!readmeCategories.has(category)) {
			throw new Error(
				`CATEGORY_BY_HEADING maps "${heading}" to "${category}", which is not one of README.md's ` +
					`Highlights category names (${[...readmeCategories].join(', ')})`,
			);
		}
		/** @type {FeatureCard} */
		const card = {
			heading,
			description: extractSectionDescription(body),
			category,
		};
		const screenshot = extractSectionScreenshot(body, heading);
		if (screenshot) {
			card.screenshot = screenshot;
			card.alt = ALT_TEXT_BY_HEADING[heading];
			if (!card.alt) {
				throw new Error(`"${heading}" has a screenshot but no ALT_TEXT_BY_HEADING entry`);
			}
		} else {
			const demoClip = DEMO_CLIP_BY_HEADING[heading];
			if (demoClip) {
				card.demoClip = demoClip;
			}
		}
		return card;
	});
}

function main() {
	const outputPath = DEFAULT_OUTPUT_PATH;
	try {
		const cards = buildFeaturesJson();
		writeFileSync(outputPath, `${JSON.stringify(cards, null, '\t')}\n`, ENCODING);
		console.log(`wrote ${cards.length} FeatureCard entries to ${outputPath}`);
	} catch (err) {
		console.error(err.message);
		process.exitCode = 1;
	}
}

// Only run the CLI when invoked directly (`node build-features.mjs`), not when imported by tests.
if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
	main();
}
