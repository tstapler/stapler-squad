// transform-markdown.mjs
//
// Turns a SourceDoc's raw Markdown body into a CuratedDoc: a Starlight page
// with frontmatter (title derived from the first `# ` heading, description)
// and internal-only cross-references stripped per ADR-003.

/**
 * Paths that are out of scope for the public site (ADR-003 / requirements.md
 * "Out of Scope"). A link target is matched if it contains one of these as a
 * path segment, after resolving `../` segments away — so both
 * `../../project_plans/foo.md` and `project_plans/foo.md` match.
 */
const OUT_OF_SCOPE_PATH_FRAGMENTS = [
	'project_plans/',
	'CLAUDE.md',
	'docs/explanation/concurrency-patterns.md',
];

/**
 * Extracts the title from the SourceDoc's first `# ` (H1) heading.
 * Parse-Don't-Validate: a doc with no H1 is a malformed SourceDoc for our
 * purposes, so this throws rather than returning a guessed fallback title.
 *
 * @param {string} markdown
 * @param {string} sourcePath used only for the error message
 * @returns {string}
 */
export function extractTitle(markdown, sourcePath) {
	const match = markdown.match(/^#\s+(.+?)\s*$/m);
	if (!match) {
		throw new Error(`No "# " H1 heading found in ${sourcePath}; cannot derive a title`);
	}
	return match[1].trim();
}

/**
 * Derives a one-line description for Starlight frontmatter from the first
 * prose paragraph after the H1 (joining its hard-wrapped lines, since these
 * source docs wrap prose at ~80-100 columns rather than one line per
 * paragraph), taking its first sentence. Falls back to the title itself if
 * the doc has no prose paragraph before its next heading/table/list (e.g.
 * the H1 is immediately followed by a sub-heading, or by a table with no
 * lead-in sentence).
 *
 * @param {string} markdown
 * @param {string} title
 * @returns {string}
 */
export function extractDescription(markdown, title) {
	// kibitzer:ignore replace-magic-literal -- '\n' is the literal line-separator
	// used for split/join throughout this file, not a configurable value.
	const lines = markdown.split('\n');
	const h1Index = lines.findIndex((line) => /^#\s+/.test(line));
	const paragraphLines = [];
	for (let i = h1Index + 1; i < lines.length; i++) {
		const line = lines[i].trim();
		if (line === '') {
			if (paragraphLines.length > 0) {
				break;
			}
			continue;
		}
		// kibitzer:ignore replace-magic-literal -- '#'/'|'/'-' are literal Markdown
		// ATX-heading, table-row, and list-item markers, not configurable values.
		if (line.startsWith('#') || line.startsWith('|') || /^[-*]\s/.test(line)) {
			if (paragraphLines.length > 0) {
				break;
			}
			continue;
		}
		paragraphLines.push(line);
	}
	if (paragraphLines.length === 0) {
		return title;
	}
	// Strip inline Markdown emphasis/link syntax for a clean plain-text description.
	const plainParagraph = paragraphLines
		.join(' ')
		.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
		// Strip code-span backticks and **bold**/*italic* asterisks, but not `_` —
		// these source docs use underscores inside code identifiers (e.g.
		// `webhook_triggers`), never as Markdown emphasis, so stripping it would
		// mangle identifiers into `webhooktriggers`.
		.replace(/`/g, '')
		.replace(/\*\*?/g, '')
		.trim();
	const firstSentence = plainParagraph.match(/^(.*?[.!?])(\s|$)/);
	return firstSentence ? firstSentence[1] : plainParagraph;
}

/**
 * Resolves a relative Markdown link target against the SourceDoc's own
 * directory, collapsing `..`/`.` segments, so `../../project_plans/x.md`
 * from `docs/how-to/` resolves to `project_plans/x.md` for matching against
 * OUT_OF_SCOPE_PATH_FRAGMENTS regardless of how many `../` hops it uses.
 *
 * @param {string} sourceDir POSIX-style directory of the SourceDoc, relative to repo root
 * @param {string} linkTarget raw link target as written in the Markdown
 * @returns {string}
 */
function resolveLinkTarget(sourceDir, linkTarget) {
	// External links (http(s), mailto, anchors) are never out-of-scope paths.
	if (/^[a-z]+:/i.test(linkTarget) || linkTarget.startsWith('#')) {
		return linkTarget;
	}
	const [pathPart] = linkTarget.split('#');
	const segments = `${sourceDir}/${pathPart}`.split('/');
	const resolved = [];
	for (const segment of segments) {
		if (segment === '' || segment === '.') {
			continue;
		}
		if (segment === '..') {
			resolved.pop();
			continue;
		}
		resolved.push(segment);
	}
	return resolved.join('/');
}

function isOutOfScope(resolvedPath) {
	return OUT_OF_SCOPE_PATH_FRAGMENTS.some((fragment) => resolvedPath.includes(fragment));
}

/**
 * Removes or rewrites Markdown links that point at an out-of-scope internal
 * path (ADR-003: `project_plans/`, `CLAUDE.md`, dev-only explanation docs).
 * A matching link `[text](target)` is replaced with its bare link text, so
 * the sentence around it still reads naturally without carrying a dead or
 * internal-only reference into the public CuratedDoc.
 *
 * @param {string} markdown
 * @param {string} sourceDir POSIX-style directory of the SourceDoc, relative to repo root
 * @returns {string}
 */
export function stripOutOfScopeLinks(markdown, sourceDir) {
	return markdown.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (fullMatch, text, target) => {
		const resolved = resolveLinkTarget(sourceDir, target);
		if (isOutOfScope(resolved)) {
			return text;
		}
		return fullMatch;
	});
}

/**
 * Removes whole sentences that mention an out-of-scope path as plain prose
 * or inline code (e.g. "See `project_plans/foo/ADR-001.md`."), which
 * `stripOutOfScopeLinks` can't catch since there's no `[text](target)` link
 * syntax to rewrite. Operates per line, skipping fenced code blocks, and
 * drops a line entirely if removing the offending sentence(s) leaves it
 * blank — satisfies ADR-003's "no project_plans/CLAUDE.md reference" bar
 * for references that aren't Markdown links.
 *
 * @param {string} markdown
 * @returns {string}
 */
export function stripOutOfScopeSentences(markdown) {
	const lines = markdown.split('\n');
	let inFencedBlock = false;
	const kept = [];
	for (const line of lines) {
		if (/^\s*```/.test(line)) {
			inFencedBlock = !inFencedBlock;
			kept.push(line);
			continue;
		}
		if (inFencedBlock || !OUT_OF_SCOPE_PATH_FRAGMENTS.some((fragment) => line.includes(fragment))) {
			kept.push(line);
			continue;
		}
		// Split into sentences on ". " and drop only the ones that mention an
		// out-of-scope path, so the rest of the paragraph still reads naturally.
		const sentences = line.split(/(?<=\.)\s+/);
		const remaining = sentences.filter(
			(sentence) => !OUT_OF_SCOPE_PATH_FRAGMENTS.some((fragment) => sentence.includes(fragment)),
		);
		const rejoined = remaining.join(' ').trim();
		if (rejoined !== '') {
			kept.push(rejoined);
		}
	}
	return kept.join('\n');
}

/**
 * Serializes Starlight frontmatter + body into the final CuratedDoc text.
 * Values are single-quoted YAML strings with internal single quotes doubled,
 * since titles/descriptions are plain prose and never need block scalars.
 *
 * @param {{title: string, description: string}} frontmatter
 * @param {string} body
 * @returns {string}
 */
function serialize(frontmatter, body) {
	const yamlString = (value) => `'${value.replace(/'/g, "''")}'`;
	return [
		'---',
		`title: ${yamlString(frontmatter.title)}`,
		`description: ${yamlString(frontmatter.description)}`,
		'---',
		'',
		body.trimEnd(),
		'',
	].join('\n');
}

/**
 * Transforms a SourceDoc's raw Markdown into a complete CuratedDoc file
 * (frontmatter + body), per ADR-003's copy-and-transform decision.
 *
 * @param {string} markdown raw SourceDoc content
 * @param {{sourcePath: string, sourceDir: string}} options
 *   sourcePath is used only for error messages; sourceDir (POSIX, relative to
 *   repo root) anchors relative-link resolution for out-of-scope stripping.
 * @returns {{title: string, description: string, content: string}}
 */
export function transformMarkdown(markdown, { sourcePath, sourceDir }) {
	const title = extractTitle(markdown, sourcePath);
	const description = extractDescription(markdown, title);
	const linksStripped = stripOutOfScopeLinks(markdown, sourceDir);
	const strippedBody = stripOutOfScopeSentences(linksStripped);
	const content = serialize({ title, description }, strippedBody);
	return { title, description, content };
}
