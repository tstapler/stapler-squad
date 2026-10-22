# Validation: Markdown Rendering in File Browser

## Risk Checklist

### Security
- [x] No `rehype-raw` — raw HTML in markdown source is escaped, not rendered. XSS risk mitigated.
- [x] Custom `components.a` renderer adds `target="_blank" rel="noopener noreferrer"` — tab-napping prevented.
- [x] Relative image paths: react-markdown will try to resolve them relative to the page URL (not the filesystem). Since the app is same-origin, the worst outcome is a broken image; no sensitive data is leaked. A follow-on can add `components.img` to suppress or rewrite relative images via `serveUrl()`.
- [x] The `serveUrl()` path-encoding function (`/api/local/serve${encoded}`) uses `encodeURIComponent` per segment — already present, no change needed for image rewriting.

### Correctness
- [x] `inferRenderMode()` checks `"markdown"` before `TEXT_EXTENSIONS.has(ext)` — no risk of `.md` falling through to `"text"` mode.
- [x] Text-fetch effect condition updated to `mode === "text" || mode === "markdown"` — markdown files get fetched.
- [x] `showRaw` state is inside `FileViewer`, not `LocalFileBrowser` — resets to rendered (false) on each file open.
- [x] 2 MB size guard applies to markdown files (same path) — no unbounded memory risk.

### Duplication Gate (jscpd)
- [x] Shared CSS extracted to `web-app/src/styles/markdown-body.css.ts` — no duplication between help page and file viewer. The jscpd gate (threshold 0.14%) should not be tripped.

### Existing Tests
- [x] `LocalFileBrowser.test.tsx` gets `react-markdown` and `remark-gfm` mocks — no ESM/CJS transform issues at test time.
- [x] `help/__tests__/page.test.tsx` already mocks both — help page tests unaffected by shared CSS extraction (the CSS import path changes; the mock does not).
- [x] `route-meta.test.ts` — no new page added; no change needed.

### Lint / Build
- [x] `make quick-check` (build + test + lint) is the local validation gate.
- [x] `dupl` complexity gate: the new markdown case in `FileViewer` is similar in structure to the existing `"text"` case but uses different JSX content. If dupl flags it, a small `renderTextContent(content, showRaw)` helper extraction would resolve it.
- [x] `@codemirror/lang-markdown` is already installed — if `import` is needed in future for a CodeMirror-based editor, no conflict with `react-markdown`.

## Open Questions

1. **Syntax highlighting in code blocks**: Not in scope for this pass, but the most common follow-on request. `rehype-highlight` (CSS-only, no JS runtime) would be the safest addition. Defer until user requests it.
2. **`.mdx` rendering**: Treating MDX as markdown (JSX tags escaped) is safe but visually noisy for files with heavy JSX. Should we suppress MDX and fall back to raw text? Leaning toward rendering it — the prose is still readable.
3. **Relative image embedding**: Should `components.img` rewrite `src` through `serveUrl()` so local images render inline? This is a UX improvement but requires path resolution logic (the markdown file's directory + the relative src). Defer.
4. **Table width in narrow viewer panels**: `maxWidth: "none"` in `viewerMarkdown` overrides the help page's 720px cap. Verify on mobile breakpoints that wide tables scroll horizontally (`overflowX: "auto"` is inherited from the markdown CSS via `pre` rule but not on `table` itself — may need `overflow-x: auto` on a table wrapper).

## Estimated Cost
- No new npm dependencies
- ~5 files changed, ~150 lines added/modified
- 1 new shared CSS file (~60 lines)
- Existing test patterns make the new test straightforward
- Low implementation risk: all patterns are already proven in the codebase
