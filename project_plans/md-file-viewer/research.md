# Research: Markdown Rendering in File Browser

## Codebase Findings

### Current File Viewer Architecture
- **Entry point**: `web-app/src/app/files/page.tsx` → `LocalFileBrowser` component
- **Main component**: `web-app/src/components/files/LocalFileBrowser.tsx` (509 lines)
- **CSS**: `web-app/src/components/files/LocalFileBrowser.css.ts` (vanilla-extract)

### Current Text Rendering (the gap)
`inferRenderMode()` returns `"text"` for `.md`/`.markdown` extensions (they're in `TEXT_EXTENSIONS`).
The `"text"` render case in `FileViewer` renders as:
```tsx
<pre className={styles.viewerText}>{textContent}</pre>
```
No markdown processing — raw source visible as in the screenshot.

### Dependencies Already Installed
Both packages needed for GFM rendering are already in `web-app/package.json`:
- `react-markdown ^10.1.0` — React component wrapping unified/remark
- `remark-gfm ^4.0.1` — GitHub Flavored Markdown: tables, strikethrough, task lists, autolinks

No new npm dependencies required for the baseline feature.

### Existing Markdown Rendering Pattern
`web-app/src/app/help/page.tsx` already uses the exact pattern needed:
```tsx
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
// ...
<article className={styles.markdownBody}>
  <ReactMarkdown remarkPlugins={[remarkGfm]}>
    {markdownContent}
  </ReactMarkdown>
</article>
```

The test file (`help/__tests__/page.test.tsx`) mocks both packages:
```tsx
jest.mock("react-markdown", () => ({ __esModule: true, default: ({ children }) => <div>{children}</div> }));
jest.mock("remark-gfm", () => ({ __esModule: true, default: () => {} }));
```
This is the pattern to follow in `LocalFileBrowser.test.tsx`.

### Existing Markdown CSS
`web-app/src/app/help/help.css.ts` defines a `markdownBody` style class with full GFM-appropriate
styles (headings, code blocks, tables, blockquotes, lists, links) using the app's `vars` theme tokens.

This CSS is currently private to the help page. It needs to be extracted into a shared location:
- **Best location**: `web-app/src/styles/markdown-body.css.ts` (shared style utility)
- Both help page and file viewer import from there.

### Toolbar Toggle Pattern
The existing `ViewerToolbar` accepts an `openLabel` prop ("Open raw" for text files).
A boolean state `showRaw` with a toggle button in the toolbar is the cleanest addition — same toolbar shape, one extra button.

### Security Analysis
- `react-markdown` v10 does **not** pass raw HTML through by default — it escapes HTML tags in the source. This is safe.
- `rehype-raw` should NOT be added (it allows arbitrary HTML from the markdown file → XSS risk with user-browsed arbitrary files).
- Links: default `react-markdown` renders `<a href="...">` without `target` or `rel`. A custom `components.a` renderer must add `target="_blank" rel="noopener noreferrer"` to prevent tab-napping on external links.
- Relative image `src` attributes in markdown would resolve to the current page URL, not the file system. The custom `components.img` renderer should either suppress images or rewrite the `src` through `serveUrl()`. The simplest safe default: suppress embedded images and render a text placeholder (or just let the broken-image icon show — it won't fetch anything dangerous since it's same-origin).

### Feature Registry
The file browser is already registered under feature `local-file-browser` (marker in `files/page.tsx`). No new feature registry entry needed — markdown rendering is an enhancement to the existing feature.

### Route Meta
`web-app/src/app/route-meta.ts` already has an entry for `/files`. No change needed.

### Test Coverage Gap
`LocalFileBrowser.test.tsx` currently does not test the markdown render path — it tests directory listing and basic file selection. A new test covering the markdown mode would need to mock `react-markdown` and `remark-gfm` and assert that a `.md` file renders via the markdown path rather than the raw `<pre>`.

## Risk Assessment

| Risk | Severity | Mitigation |
|---|---|---|
| XSS via raw HTML in markdown | High | Do not add `rehype-raw`; `react-markdown` escapes HTML by default |
| Tab-napping via markdown links | Medium | Custom `components.a` with `target="_blank" rel="noopener noreferrer"` |
| Relative image paths leaking filesystem paths | Low | Images use broken-image fallback or are suppressed |
| CSS duplication (help page vs file viewer) | Low | Extract to `shared/styles/markdown-body.css.ts` |
| react-markdown v10 API changes from v9 | Low | Already installed; check breaking changes in changelog |
| jscpd duplication gate | Low | Extracting shared CSS prevents triggering it |
