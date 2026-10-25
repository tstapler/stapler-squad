# Plan: Markdown Rendering in File Browser

## Overview

Add GitHub-flavored markdown rendering to the file browser's file viewer. No new npm dependencies — `react-markdown` and `remark-gfm` are already installed. The change is isolated to three files plus one new shared CSS file.

## Implementation Steps

### Task 1 — Extract shared Markdown CSS
**File**: `web-app/src/styles/markdown-body.css.ts` (new file)

Move the `markdownBody` style and all its `globalStyle` children from `help/help.css.ts` into the new shared file. Export `markdownBody` from the shared file.

Update `help/help.css.ts` to import `markdownBody` from the new location instead of defining it.

This prevents CSS duplication between help page and file viewer, which keeps jscpd happy.

**Why shared file instead of component**: The pattern in `help/page.tsx` is `<article className={styles.markdownBody}>` — a plain CSS class applied to the wrapper. A shared CSS file is the right granularity; a full React component wrapper adds unnecessary abstraction.

### Task 2 — Add `"markdown"` render mode to `inferRenderMode()`
**File**: `web-app/src/components/files/LocalFileBrowser.tsx`

Change:
```typescript
type RenderMode = "html" | "image" | "svg" | "pdf" | "video" | "text" | "binary";
```
To:
```typescript
type RenderMode = "html" | "image" | "svg" | "pdf" | "video" | "text" | "markdown" | "binary";
```

In `inferRenderMode()`, add a check **before** the `TEXT_EXTENSIONS.has(ext)` line:
```typescript
if (["md", "markdown", "mdx"].includes(ext)) return "markdown";
```

Remove `"md"` and `"markdown"` from `TEXT_EXTENSIONS` (they no longer need the text fallback since `inferRenderMode` handles them first). Keep `"mdx"` out of `TEXT_EXTENSIONS` since it wasn't there before.

### Task 3 — Add `MarkdownViewer` rendering case in `FileViewer`
**File**: `web-app/src/components/files/LocalFileBrowser.tsx`

Add imports:
```typescript
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { markdownBody } from "@/styles/markdown-body.css";
```

Add state for the raw/rendered toggle (inside `FileViewer`):
```typescript
const [showRaw, setShowRaw] = useState(false);
```

Add the `"markdown"` case to the switch statement (before `default`):
```tsx
case "markdown": {
  const toggleLabel = showRaw ? "Rendered" : "Raw";
  if (textError) {
    return (
      <div className={styles.viewerWrapper}>
        <ViewerToolbar name={entry.name} url={url} openLabel="Open raw" onClose={onClose} />
        <div className={styles.viewerEmpty}>Failed to load: {textError}</div>
      </div>
    );
  }
  if (textContent === null) {
    return (
      <div className={styles.viewerWrapper}>
        <ViewerToolbar name={entry.name} url={url} openLabel="Open raw" onClose={onClose} />
        <div className={styles.viewerEmpty}>Loading…</div>
      </div>
    );
  }
  return (
    <div className={styles.viewerWrapper}>
      <ViewerToolbar
        name={entry.name}
        url={url}
        openLabel="Open raw"
        onClose={onClose}
        extraActions={
          <button
            onClick={() => setShowRaw((v) => !v)}
            className={styles.externalButton}
            data-testid="file-browser-viewer-toggle-raw"
          >
            {toggleLabel}
          </button>
        }
      />
      {showRaw ? (
        <pre className={styles.viewerText}>{textContent}</pre>
      ) : (
        <div className={styles.viewerMarkdown}>
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              a({ href, children }) {
                return (
                  <a href={href} target="_blank" rel="noopener noreferrer">
                    {children}
                  </a>
                );
              },
            }}
          >
            {textContent}
          </ReactMarkdown>
        </div>
      )}
    </div>
  );
}
```

The `FileViewer` effect already fetches text for `mode === "text"` — update the condition to `mode === "text" || mode === "markdown"` so the content is fetched for markdown files too.

### Task 4 — Update `ViewerToolbar` to accept extra actions
**File**: `web-app/src/components/files/LocalFileBrowser.tsx`

Add optional `extraActions?: React.ReactNode` prop to `ViewerToolbarProps`. Render it between the label and the open/close buttons:
```tsx
function ViewerToolbar({ name, url, openLabel, onClose, extraActions }: ViewerToolbarProps) {
  return (
    <div className={styles.viewerToolbar}>
      <span className={styles.viewerLabel}>{name}</span>
      {extraActions}
      <button ...>{/* open */}</button>
      {onClose && <button ...>{/* close */}</button>}
    </div>
  );
}
```

### Task 5 — Add `viewerMarkdown` CSS class
**File**: `web-app/src/components/files/LocalFileBrowser.css.ts`

Add:
```typescript
import { markdownBody } from "@/styles/markdown-body.css";

export const viewerMarkdown = style([
  markdownBody,
  {
    flex: 1,
    overflow: "auto",
    padding: vars.space["4"],
    background: vars.color.background,
    // Override help page's 720px max-width to fill the viewer panel
    maxWidth: "none",
  },
]);
```

### Task 6 — Update `LocalFileBrowser.test.tsx`
**File**: `web-app/src/components/files/LocalFileBrowser.test.tsx`

Add mocks at the top (same pattern as `help/__tests__/page.test.tsx`):
```typescript
jest.mock("react-markdown", () => ({
  __esModule: true,
  default: ({ children }: { children: string }) => <div data-testid="mock-markdown">{children}</div>,
}));
jest.mock("remark-gfm", () => ({ __esModule: true, default: () => {} }));
```

Add a test case:
- Mock a `.md` file entry
- Mock `fetch` to return a markdown string
- Assert the viewer renders via the markdown path (not `<pre>`)
- Assert the raw/rendered toggle button exists
- Toggle to raw and assert `<pre>` appears

### Task 7 — Update help page import
**File**: `web-app/src/app/help/help.css.ts`

Replace the local `markdownBody` definition with an import from the shared file. Run `make lint` to confirm the styles are identical and no visual regression.

## Files Changed

| File | Change |
|---|---|
| `web-app/src/styles/markdown-body.css.ts` | **New** — extracted shared GFM CSS |
| `web-app/src/app/help/help.css.ts` | Import `markdownBody` from shared file instead of defining it |
| `web-app/src/components/files/LocalFileBrowser.tsx` | Add `"markdown"` mode, `MarkdownViewer` case, toolbar extra-actions prop |
| `web-app/src/components/files/LocalFileBrowser.css.ts` | Add `viewerMarkdown` style |
| `web-app/src/components/files/LocalFileBrowser.test.tsx` | Mock react-markdown; add markdown render test |

## Adversarial Review

**Q: Why not use `@codemirror/lang-markdown` for rendering instead?**  
A: CodeMirror is an editor, not a document renderer. It would give us a syntax-highlighted editor experience (good for editing), not a rendered preview (good for reading). The user wants GitHub-style preview. `react-markdown` is correct here.

**Q: Does adding `rehype-highlight` for code block syntax highlighting make sense?**  
A: Not in this pass. It adds a dependency, bundle weight, and complexity for a feature the user didn't request. The current styled code block (monospace, distinct background) is sufficient. This can be a follow-on.

**Q: Should `.mdx` be rendered as markdown?**  
A: Yes — strip JSX tags and render the markdown portions. `react-markdown` will just skip the JSX syntax it doesn't understand (or escape it safely). The user gets readable prose without the JSX noise.

**Q: What if the markdown file is very large (> 2 MB)?**  
A: The existing `MAX_TEXT_PREVIEW_BYTES = 2 MB` guard already covers this — the error path shows "File too large to preview" for both text and markdown modes.

**Q: Does the Raw/Rendered toggle state persist across file selections?**  
A: No — `useState(false)` inside `FileViewer` resets on each file selection (new component mount). This is the right default; each file should open in rendered mode.
