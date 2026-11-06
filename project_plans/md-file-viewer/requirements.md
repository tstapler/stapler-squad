# Requirements: Markdown Rendering in File Browser

## Source
Backlog item f639afcf-9129-46de-9619-320c53ec994f

## Problem Statement
The Files tab (`/files`) file browser renders all text files — including `.md` and `.markdown` files — as raw monospaced text inside a `<pre>` tag. The attached screenshot shows a planning document with visible `#`, `**`, and `|` markdown syntax that should be rendered as formatted headings, bold text, and tables.

The user wants GitHub-flavored markdown (GFM) rendering for markdown files in the file browser, matching the quality of GitHub's review UI: styled headings, tables with borders, bold/italic, code blocks with monospace formatting, task lists, blockquotes, and links.

## Scope

**In scope:**
- Detect `.md` / `.markdown` / `.mdx` files in `inferRenderMode()` and return a new `"markdown"` render mode
- Render the markdown body using `react-markdown` + `remark-gfm` (already installed)
- Style the rendered output (headings, tables, code blocks, blockquotes, lists, links) consistent with the existing app theme
- Add a Raw / Rendered toggle in the viewer toolbar (users should still be able to see source)
- Security: links in rendered markdown must open in a new tab with `rel="noopener noreferrer"`; no raw HTML pass-through (`rehype-raw` is NOT included to prevent XSS)
- Shared `MarkdownBody` CSS extracted from `help.css.ts` so both help page and file viewer use the same styles

**Out of scope (first pass):**
- Syntax highlighting within fenced code blocks (no `rehype-highlight` or similar; styled code blocks only)
- `.mdx` JSX execution
- Anchor link navigation within document
- Inline image rendering from relative paths (images link out; they don't embed)

## Acceptance Criteria

1. Opening a `.md` or `.markdown` file in the file browser renders it as formatted HTML, not raw text.
2. GFM tables are rendered with visible column borders and alternating row styling.
3. Headings (h1–h4), bold, italic, strikethrough, and blockquotes all render correctly.
4. Fenced code blocks render in a styled `<pre><code>` block (monospace, distinct background).
5. The viewer toolbar has a "Raw" / "Rendered" toggle; "Rendered" is the default.
6. Switching to "Raw" displays the unmodified source in the existing `<pre>` text viewer.
7. External links in rendered markdown open with `target="_blank" rel="noopener noreferrer"`.
8. The markdown body CSS is extracted into a shared location reusable by both the help page and the file viewer.
9. The feature passes existing `LocalFileBrowser` tests (no regressions).
10. The feature does not render raw HTML from the markdown source (no `rehype-raw`; XSS prevention).
