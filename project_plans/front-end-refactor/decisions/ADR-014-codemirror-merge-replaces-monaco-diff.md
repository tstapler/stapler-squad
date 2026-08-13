# ADR-014: CodeMirror 6 Merge Addon Replaces Monaco for Diff View on Mobile

## Status
Accepted

## Context

`web-app/src/components/sessions/DiffViewer.tsx` uses `@monaco-editor/react` to render git diffs. Monaco Editor is the engine behind VS Code — it is a powerful desktop-first editor. On mobile (Pixel 9 Pro Fold), Monaco has three problems:

### Problem 1: Bundle size

`@monaco-editor/react` + `monaco-editor` add approximately 500–700 KB (uncompressed) to the JavaScript bundle. The current `size-limit` configuration allows a 5 MB total JS bundle. Monaco's share of that budget is disproportionate given that the diff viewer is a secondary panel — it is not loaded on the initial page render but it does appear in the bundle.

### Problem 2: Mobile pointer model conflict

Monaco uses its own pointer capture and scroll management, designed for desktop mouse input. On Android, this conflicts with the browser's touch scroll handling. The result is:
- Pinch-to-zoom triggers Monaco's text zoom instead of the browser's zoom
- Vertical scroll in a diff view is frequently hijacked, making it impossible to scroll past the diff panel
- Touch selection (for copy) has incorrect selection anchors

### Problem 3: Canvas-based rendering is opaque to accessibility

Monaco renders text on a canvas layer for performance. On mobile, screen readers (TalkBack) cannot read canvas content. The accessible text layer Monaco provides is incomplete and not reliable on Android.

### Why CodeMirror 6 merge addon is the right replacement

The project already has **extensive CodeMirror 6 dependencies**:

```json
"@codemirror/lang-css": "^6.3.1",
"@codemirror/lang-go": "^6.0.1",
"@codemirror/lang-html": "^6.4.11",
"@codemirror/lang-java": "^6.0.2",
"@codemirror/lang-javascript": "^6.2.5",
"@codemirror/lang-json": "^6.0.2",
"@codemirror/lang-markdown": "^6.5.0",
"@codemirror/lang-python": "^6.2.1",
"@codemirror/lang-rust": "^6.0.2",
"@codemirror/state": "^6.6.0",
"@codemirror/view": "^6.41.0",
"codemirror": "^6.0.2"
```

`@codemirror/merge` is an official CodeMirror package that provides a side-by-side or unified diff view using `MergeView`. It is:

- **DOM-based** (not canvas): text is rendered as real DOM nodes, accessible to TalkBack and screen readers
- **Touch-native**: CodeMirror 6 uses standard DOM scroll — `touch-action: pan-y` enables native vertical scroll
- **~150–200 KB lighter** than Monaco (rough estimate based on package sizes; exact delta measured with `npm run size-limit` post-migration)
- **No new language dependency**: all language extensions already installed for the FileContentViewer and other CodeMirror uses

### What is lost by removing Monaco

Monaco provides:
- IntelliSense / autocomplete — not used in the diff viewer (display-only)
- Bracket matching, folding — not critical for diff display
- The VS Code color theme ecosystem — `@codemirror/theme-one-dark` provides equivalent for the dark theme case

Nothing that Monaco provides in the `DiffViewer.tsx` context is actually used beyond syntax highlighting and side-by-side diff display. Both are available in `@codemirror/merge`.

### Options considered

| Option | Mobile scroll | Bundle | A11y | Syntax HL | Verdict |
|--------|--------------|--------|------|-----------|---------|
| Monaco Editor (current) | Broken | ~600 KB | Canvas-opaque | Excellent | Rejected — mobile incompatible |
| **CodeMirror 6 + @codemirror/merge** | Native DOM | ~50 KB incremental | Full DOM | Excellent (existing lang-* packages) | **Selected** |
| react-diff-view (lightweight) | Good | ~30 KB | Good | Limited | Rejected — doesn't reuse existing CodeMirror investment |
| Custom `<pre>` diff renderer | Perfect | 0 KB | Perfect | Manual | Rejected — syntax highlighting would require Shiki (already installed but adds complexity) |

## Decision

Replace `@monaco-editor/react` with `@codemirror/merge` in `DiffViewer.tsx`.

### Implementation pattern

```tsx
// DiffViewer.tsx
"use client";
import { useEffect, useRef } from 'react';
import { MergeView } from '@codemirror/merge';
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { oneDark } from '@codemirror/theme-one-dark';
import { go } from '@codemirror/lang-go';
import { javascript } from '@codemirror/lang-javascript';
// ... other language imports

interface DiffViewerProps {
  original: string;
  modified: string;
  language: 'go' | 'typescript' | 'javascript' | 'python' | 'rust' | 'json' | 'markdown';
}

export function DiffViewer({ original, modified, language }: DiffViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current) return;
    const view = new MergeView({
      parent: containerRef.current,
      a: { doc: original, extensions: [languageExtension(language), oneDark] },
      b: { doc: modified, extensions: [languageExtension(language), oneDark] },
      orientation: 'a-b',
    });
    return () => view.destroy();
  }, [original, modified, language]);

  return <div ref={containerRef} className={container} />;
}
```

The container `.css.ts` applies `touch-action: pan-y; overscroll-behavior: contain` to enable native vertical scroll on mobile.

### Package changes

**Add**: `@codemirror/merge` (install `--save`)

**Remove**: `@monaco-editor/react` (uninstall after confirming no other component imports it)

Monaco Editor itself (`monaco-editor`) is a transitive dependency of `@monaco-editor/react` — it will be removed automatically when the React wrapper is uninstalled. Verify with `npm ls monaco-editor` post-uninstall.

### Language extension mapping

```ts
function languageExtension(lang: string) {
  const map: Record<string, LanguageSupport> = {
    go: go(),
    typescript: javascript({ typescript: true }),
    javascript: javascript(),
    python: python(),
    rust: rust(),
    json: json(),
    markdown: markdown(),
    css: css(),
  };
  return map[lang] ?? [];
}
```

All language packages are already installed — no new installs required beyond `@codemirror/merge`.

## Consequences

### Positive
- ~200 KB or more reduction in JS bundle weight (Monaco removal)
- Diff view scrolls natively on Android — no pointer capture conflict
- DOM-based rendering — TalkBack can read diff content
- No new language dependencies — `@codemirror/lang-*` packages already present
- CodeMirror 6 is already the standard for `FileContentViewer.tsx` — consistent authoring model

### Negative / Constraints
- `MergeView` API is imperative (DOM-based `useEffect` pattern) rather than declarative JSX — this is standard for CodeMirror 6 in React and already the pattern used in `FileContentViewer.tsx`
- Monaco's minimap, breadcrumb navigation, and code action features are unavailable — these were not used in the diff-only context, so there is no functional regression
- The CodeMirror merge view shows a side-by-side layout by default; on narrow viewports (outer Pixel 9 Pro Fold ~390px), the side-by-side layout is too narrow. A `unified` mode (single column with inline additions/deletions) should be used below the fold breakpoint (600px) via a responsive prop.

## References
- CodeMirror merge addon: https://codemirror.net/docs/ref/#merge
- xterm.js mobile touch issue (related mobile scroll context): https://github.com/xtermjs/xterm.js/issues/5377
- Research: `project_plans/front-end-refactor/research/findings-features.md`
- Implementation: Phase 4, Story 4.2 in `docs/tasks/front-end-refactor.md`
