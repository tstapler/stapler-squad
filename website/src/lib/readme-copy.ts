// Extracts the hero's two pieces of lifted copy (Task 3.1.1a/b: "don't invent
// new copy, lift it" per design/ux.md §2) straight from the real README.md,
// so hero.test.ts can regression-guard index.mdx's hardcoded hero strings
// against README drift instead of trusting a one-time copy-paste forever.

// Matches README.md's opening sentence after stripping Markdown link syntax --
// anchored on the distinctive "web-based mission control" phrasing so it
// survives edits to the parenthetical agent list or trailing sentence.
const VALUE_PROP_PATTERN =
	/is a web-based mission control for running multiple AI coding agents[\s\S]*?structured review queue\./;

// Matches the Homebrew install command's fenced bash code block under the
// "#### Homebrew" heading.
const INSTALL_COMMAND_PATTERN = /```bash\n(brew tap tstapler\/stapler-squad[^\n]*)\n```/;

function stripMarkdownLinks(markdown: string): string {
	return markdown.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1');
}

/**
 * Extracts README.md's value-prop sentence ("is a web-based mission
 * control... structured review queue."), stripped of Markdown link syntax.
 * Throws if the README's wording has drifted enough that the anchor phrase
 * is gone, rather than silently returning stale text.
 */
export function extractValueProp(markdown: string): string {
	const stripped = stripMarkdownLinks(markdown);
	const match = stripped.match(VALUE_PROP_PATTERN);
	if (!match) {
		throw new Error(
			'Could not find the value-prop sentence in README.md (expected text starting ' +
				'"is a web-based mission control for running multiple AI coding agents...")',
		);
	}
	return match[0];
}

/** Extracts README.md's exact Homebrew install command (the `brew tap ... && brew install ...` line). */
export function extractInstallCommand(markdown: string): string {
	const match = markdown.match(INSTALL_COMMAND_PATTERN);
	if (!match) {
		throw new Error('Could not find the Homebrew install command in README.md');
	}
	return match[1].trim();
}
