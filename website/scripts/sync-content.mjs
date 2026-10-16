#!/usr/bin/env node
// sync-content.mjs
//
// Reads website/scripts/content-manifest.json (the ContentManifest, ADR-003)
// and, for every `include: true` entry, copies + transforms its SourceDoc
// into a CuratedDoc under website/src/content/docs/help/.
//
// Modes:
//   generate (default) — write the transformed CuratedDocs to disk.
//   verify (--check)   — compare what generate mode would produce against
//     the committed CuratedDocs and exit non-zero on any drift, without
//     writing anything. Intended for Story 6.3.1's deploy-time drift check;
//     not wired into the deploy workflow yet (out of scope for Epic 2.1).
//
// Parse-Don't-Validate: every manifest entry is validated into a typed
// IncludeDecision at load time (loadManifest), so a malformed entry is
// rejected immediately instead of failing deep inside the transform pass.

import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { dirname, join, posix } from 'node:path';
import { fileURLToPath } from 'node:url';
import { transformMarkdown } from './transform-markdown.mjs';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
// Repo root is three levels up from website/scripts/: website/scripts -> website -> repo root.
const REPO_ROOT = join(SCRIPT_DIR, '..', '..');
const DEFAULT_MANIFEST_PATH = join(SCRIPT_DIR, 'content-manifest.json');
const ENCODING = 'utf8';

/**
 * CuratedDocs always live at <repoRoot>/website/src/content/docs/help/,
 * derived from whichever repoRoot was passed to syncContent — never from a
 * module-level constant — so a test-fixture repoRoot can't accidentally
 * write into this repo's real website/ tree.
 *
 * @param {string} repoRoot
 * @returns {string}
 */
function curatedDocsDir(repoRoot) {
	return join(repoRoot, 'website', 'src', 'content', 'docs', 'help');
}

/**
 * @typedef {{source: string, include: true, slug: string}} IncludedDecision
 * @typedef {{source: string, include: false, reason: string}} ExcludedDecision
 * @typedef {IncludedDecision | ExcludedDecision} IncludeDecision
 */

/**
 * Validates one raw JSON entry into an IncludeDecision, throwing with a
 * precise, actionable message on the first problem found. This is the
 * Parse-Don't-Validate boundary: nothing past this function ever sees a
 * malformed entry.
 *
 * @param {unknown} entry
 * @param {number} index position in the manifest array, for error messages
 * @returns {IncludeDecision}
 */
export function parseIncludeDecision(entry, index) {
	const where = `content-manifest.json entry ${index}`;
	if (entry === null || typeof entry !== 'object') {
		throw new Error(`${where} must be an object, got ${JSON.stringify(entry)}`);
	}
	const { source, include, slug, reason } = /** @type {Record<string, unknown>} */ (entry);
	if (typeof source !== 'string' || source.trim() === '') {
		throw new Error(`${where} ("source") must be a non-empty string`);
	}
	if (typeof include !== 'boolean') {
		throw new Error(`${where} ("${source}") "include" must be a boolean`);
	}
	if (include) {
		if (typeof slug !== 'string' || slug.trim() === '') {
			throw new Error(`${where} ("${source}") has include: true but no non-empty "slug"`);
		}
		return { source, include: true, slug };
	}
	if (typeof reason !== 'string' || reason.trim() === '') {
		throw new Error(
			`${where} ("${source}") has include: false but no non-empty "reason" — ` +
				'every exclusion must be justified in writing (ADR-003)',
		);
	}
	return { source, include: false, reason };
}

/**
 * Loads and validates the full ContentManifest from disk.
 *
 * @param {string} manifestPath
 * @returns {IncludeDecision[]}
 */
export function loadManifest(manifestPath) {
	const raw = readFileSync(manifestPath, ENCODING);
	/** @type {unknown} */
	let parsed;
	try {
		parsed = JSON.parse(raw);
	} catch (err) {
		throw new Error(`${manifestPath} is not valid JSON: ${err.message}`);
	}
	if (!Array.isArray(parsed)) {
		throw new Error(`${manifestPath} must contain a JSON array of IncludeDecision entries`);
	}
	return parsed.map((entry, index) => parseIncludeDecision(entry, index));
}

/**
 * Maps every include:true decision's repo-root-relative source path to its
 * CuratedDoc slug (e.g. "docs/how-to/debug-with-logs.md" -> "help/debug-with-logs"),
 * so transformMarkdown can rewrite a sibling SourceDoc cross-reference
 * (`[Debugging logs](debug-with-logs.md)`, the normal GitHub-relative-link
 * convention for docs/how-to/*.md) into that doc's actual published site
 * path instead of leaving a dead `.md` link in the CuratedDoc (Starlight
 * renders routes, not literal `.md` files).
 *
 * @param {IncludeDecision[]} decisions
 * @returns {Map<string, string>}
 */
export function buildSiblingSlugMap(decisions) {
	const map = new Map();
	for (const decision of decisions) {
		if (decision.include) {
			map.set(decision.source, decision.slug);
		}
	}
	return map;
}

/**
 * Transforms a single IncludedDecision into its CuratedDoc text, reading the
 * SourceDoc from disk. Fails loudly (throws, naming the missing path) if the
 * source no longer exists — ADR-003's explicit requirement, so a stale
 * manifest reference is a build-time error, not a silent skip.
 *
 * @param {IncludedDecision} decision
 * @param {string} repoRoot
 * @param {Map<string, string>} [siblingSlugMap]
 * @returns {{slug: string, content: string}}
 */
export function buildCuratedDoc(decision, repoRoot, siblingSlugMap = new Map()) {
	const absoluteSourcePath = join(repoRoot, decision.source);
	if (!existsSync(absoluteSourcePath)) {
		throw new Error(`Manifest source does not exist: ${decision.source}`);
	}
	const markdown = readFileSync(absoluteSourcePath, ENCODING);
	const sourceDir = posix.dirname(decision.source.split('/').join(posix.sep));
	const { content } = transformMarkdown(markdown, {
		sourcePath: decision.source,
		sourceDir,
		siblingSlugMap,
	});
	return { slug: decision.slug, content };
}

function slugToOutputPath(slug, repoRoot) {
	// Slugs are "help/<name>"; CuratedDocs live under src/content/docs/help/.
	const basename = slug.startsWith('help/') ? slug.slice('help/'.length) : slug;
	return join(curatedDocsDir(repoRoot), `${basename}.md`);
}

/**
 * Runs the sync in generate mode: writes every include:true entry's
 * CuratedDoc to disk. Throws (and the CLI entrypoint below turns that into a
 * non-zero exit + printed path) on the first missing SourceDoc or malformed
 * manifest entry.
 *
 * @param {{manifestPath?: string, repoRoot?: string, check?: boolean}} [options]
 * @returns {{slug: string, outputPath: string, content: string, changed: boolean}[]}
 */
export function syncContent(options = {}) {
	const manifestPath = options.manifestPath ?? DEFAULT_MANIFEST_PATH;
	const repoRoot = options.repoRoot ?? REPO_ROOT;
	const check = options.check ?? false;

	const decisions = loadManifest(manifestPath);
	const siblingSlugMap = buildSiblingSlugMap(decisions);
	const results = [];
	for (const decision of decisions) {
		if (!decision.include) {
			continue;
		}
		const { slug, content } = buildCuratedDoc(decision, repoRoot, siblingSlugMap);
		const outputPath = slugToOutputPath(slug, repoRoot);
		const existing = existsSync(outputPath) ? readFileSync(outputPath, ENCODING) : null;
		const changed = existing !== content;
		if (!check && changed) {
			mkdirSync(dirname(outputPath), { recursive: true });
			writeFileSync(outputPath, content, ENCODING);
		}
		results.push({ slug, outputPath, content, changed });
	}
	return results;
}

function main() {
	const args = process.argv.slice(2);
	const check = args.includes('--check') || args.includes('--verify');
	// Test-only overrides so integration tests can run the real CLI entrypoint
	// (exit code + stdout/stderr) against a fixture manifest/repo root instead
	// of this repo's real content-manifest.json and docs/ tree.
	const manifestPath = process.env.SYNC_CONTENT_MANIFEST || undefined;
	const repoRoot = process.env.SYNC_CONTENT_REPO_ROOT || undefined;
	try {
		const results = syncContent({ check, manifestPath, repoRoot });
		if (check) {
			const drifted = results.filter((r) => r.changed);
			if (drifted.length > 0) {
				for (const r of drifted) {
					console.error(`Drift detected: ${r.outputPath} does not match its SourceDoc`);
				}
				process.exitCode = 1;
				return;
			}
			console.log(`verify: ${results.length} CuratedDoc(s) up to date`);
			return;
		}
		for (const r of results) {
			console.log(`${r.changed ? 'wrote' : 'unchanged'}: ${r.outputPath}`);
		}
		console.log(`generate: ${results.length} CuratedDoc(s) synced`);
	} catch (err) {
		console.error(err.message);
		process.exitCode = 1;
	}
}

// Only run the CLI when invoked directly (`node sync-content.mjs`), not when imported by tests.
if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
	main();
}
