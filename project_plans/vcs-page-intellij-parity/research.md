# Research: VCS Page IntelliJ Parity

**Date:** 2026-10-27  
**Phase:** SDD Phase 2 — Research

---

## 1. Existing Code Analysis

### 1.1 DiffRenderer (`web-app/src/components/shared/DiffRenderer.tsx`)

The renderer is ~190 lines, pure display, no data fetching. Key existing capabilities:

- Parses raw unified diff via `parseDiff(content)` into `DiffFile[]`.
- Renders a file-tree sidebar that jumps to a file on click (scrollIntoView).
- Per-hunk `<div className={hunk}>` blocks with a `@@ -a,b +c,d @@` header line.
- Per-line render: `lineAdd` / `lineDelete` / `lineContext` CSS classes; two gutter columns (old + new line numbers); one `<span className={lineContent}>` for text.
- Has a `viewModeToggle` with "Unified" active and "Split" **disabled** (`disabled title="Split view coming soon"`).
- No hunk navigation, no scroll-to-hunk state.
- Uses vanilla-extract CSS (`DiffRenderer.css.ts`); dark terminal-palette aesthetic.

**Gap summary:** split view, hunk navigation state/buttons, per-line syntax highlighting.

### 1.2 parseDiff (`web-app/src/lib/utils/parseDiff.ts`)

Parses git unified diff format into:

```
DiffFile { filename, additions, deletions, changes: DiffHunk[] }
DiffHunk { oldStart, oldLines, newStart, newLines, lines: DiffLine[] }
DiffLine { type: "add"|"delete"|"context", content, oldLineNumber?, newLineNumber? }
```

Also exports `buildGutterMarks(diffContent, filePath)` for use by the CodeMirror file editor.

**Reuse potential:** The structured `DiffHunk[]` per file is exactly what hunk navigation, context expansion, and split-view layout need — no new parser work required.

### 1.3 DiffViewer (`web-app/src/components/sessions/DiffViewer.tsx`)

Thin session-aware wrapper (~38 lines): reads `diff` and `diffLoading` from `SessionVcsContext` and passes them to `DiffRenderer`. Also renders a `CIStatusBadge` when a PR is linked.

### 1.4 VcsPanel + VcsWidget

`VcsPanel` reads VCS status from `SessionVcsContext` and renders `VcsWidget`. The file list in `VcsWidget` → `VcsWidgetFileList` renders `FileRow` items that call `onNavigateToFile(path)` — today that callback switches to the Files tab. No inline diff or per-file diff view is present.

**Reuse potential:** The existing `FileChangeSummary[]` (with `path`, `status`, `additions`, `deletions`) is sufficient to drive an inline-diff entry point without any new backend data.

### 1.5 VCS Types (`web-app/src/lib/vcs/types.ts`)

`FileChangeSummary`, `DiffFile`, `VcsWidgetData` are clean and stable. No changes needed for ACs 1–4; AC-5 context expansion may warrant an optional `contextLines` field on the request.

### 1.6 SessionVcsContext (`web-app/src/lib/contexts/SessionVcsContext.tsx`)

Provides `{ diff: SessionDiff|null, diffLoading, status, statusLoading, refresh, refreshDiff, refreshStatus }` across all consumers (DiffViewer, VcsPanel, FilesTab). `SessionDiff = { content, added, removed }` where `content` is the raw unified diff string.

The VCS tab already has access to the full diff content through this context without any additional fetching.

---

## 2. CodeMirror Integration

CodeMirror 6 is **already the primary code editor**. The dependency footprint is substantial:

| Package | Purpose |
|---|---|
| `codemirror` `^6.0.2` | Core bundle |
| `@codemirror/view` `^6.41.0` | EditorView, decorations, ViewPlugin, gutter |
| `@codemirror/state` `^6.6.0` | EditorState, Compartment, Extension |
| `@codemirror/language` `^6.12.3` | HighlightStyle, syntaxHighlighting |
| `@lezer/highlight` `1.2.3` | Token tags |
| `@codemirror/lang-{go,ts,js,python,rust,java,json,html,css,markdown}` | Per-language Lezer grammars |
| `@monaco-editor/react` + `monaco-editor` | Monaco (used separately, not relevant here) |

The engine layer (`web-app/src/components/sessions/files/engine/`) already:

- Wraps CodeMirror via `CmEngine.ts` behind a stable `EditorEngine` port interface.
- Uses `HighlightStyle` + `syntaxHighlighting` from `@codemirror/language` (in `cmTheme.ts`).
- Loads language grammars lazily by language string (`loadCodemirrorLang` in `languages.ts`).
- Defines a `Highlighter` interface (`Highlighter.ts`) specifically designed so "a non-CodeMirror engine can reuse Lezer parsers for syntax highlighting" — a `tokenize(range, languageId): Promise<TokenSpan[]>` API that returns `{ from, to, className }` spans.

**For AC-4 syntax highlighting in diff lines:** the `Highlighter` interface is the right seam. Implement a `LezerHighlighter` that, given a line string and `languageId`, uses the Lezer parser (via `loadCodemirrorLang`) to produce `TokenSpan[]`, then renders each line's content as `<span>` elements with those classes. This avoids mounting a full CodeMirror view per diff line and reuses the existing language-loading infrastructure. The language is detected via `detectLanguage(filename)` already exported from `FileContentViewer.tsx`.

---

## 3. Split Diff Precedents and Library Landscape

No split-diff library is currently in `package.json` (`react-diff-viewer`, `diff2html`, `@uiw/react-codemirror`, etc. are all absent).

Options evaluated:

| Option | Pros | Cons |
|---|---|---|
| **Custom CSS two-column layout** | Zero new dependencies; full control over styling and scroll sync; uses existing `DiffHunk[]` data | Implementation cost: synchronized scroll, paired-row alignment (a deletion-only hunk has no right-side row) |
| **`@codemirror/merge`** | Native CM6; `unifiedMergeView` extension is in CM6 proper | Designed for 3-way merge / conflict resolution; overkill; not for displaying an arbitrary unified diff |
| **`react-diff-viewer`** | Ready-made | Adds a dependency with its own styling assumptions; no CodeMirror integration; no active maintenance signal needed |
| **`diff2html`** | Widely used | HTML-string output, not React; heavy CSS import; no direct integration with existing theme |

**Recommendation:** custom two-column layout built on top of the existing `parseDiff` output. Each hunk is laid out as a CSS grid row: left pane shows `delete` + `context` lines (original), right pane shows `add` + `context` lines (modified). Context lines appear in both columns. Synchronized scroll is handled with a shared `onScroll` handler updating both pane refs via `scrollTop`. No new runtime dependency.

---

## 4. Keyboard Shortcut Infrastructure

A mature `ShortcutRegistry` singleton exists at `web-app/src/lib/shortcuts/shortcutRegistry.ts`:

- **`useShortcut(id, shortcut, enabled?)`** — registers/deregisters on mount/unmount.
- Context-sensitive dispatch: active context is detected by walking `data-context` attributes up from `document.activeElement`. Existing contexts: `"global"`, `"session-list"`, `"approval"`, `"terminal"`, `"cockpit"`, `"omnibar"`.
- Shortcuts appear in the `?` overlay (`KeyboardShortcutOverlay.tsx`) automatically via `registry.getAll()`.
- IME composition events are skipped; input elements are excluded unless a modifier key is held.

**For hunk navigation (AC-1):**  
Register `n` / `p` (or `]c` / `[c` — IntelliJ's default) with `context: "global"` (or a new `"diff"` context if the diff pane gets a `data-context` attribute). The action calls a `navigateHunk(direction: "next"|"prev")` callback passed down from `DiffRenderer`. Because `DiffRenderer` is a pure display component, the hunk-index state and navigation logic live one level up in `DiffViewer`, and the renderer receives `activeHunkIndex`, `onPrevHunk`, `onNextHunk` props.

**New `"diff"` context consideration:** Adding a `"diff"` context requires a one-line addition to `ShortcutContext` type in `shortcutRegistry.ts` and a label entry in `CONTEXT_LABELS` in `KeyboardShortcutOverlay.tsx`. This is the cleaner option because it avoids `n`/`p` firing when the user is not in the diff pane.

---

## 5. Context Expansion Strategy

The backend computes the unified diff using `fdiff.NewUnifiedEncoder(&buf, fdiff.DefaultContextLines)` where `fdiff.DefaultContextLines = 3` (`session/git/diff.go:138`). The `NewUnifiedEncoder` signature is `func NewUnifiedEncoder(w io.Writer, contextLines int) *UnifiedEncoder` — it already accepts an arbitrary integer.

`GetSessionDiffRequest` proto (`proto/session/v1/session.proto:1074`) currently has only `string id = 1;`. There is no `context_lines` field.

Two viable strategies:

### Strategy A — Server-side context parameter (recommended)
Add `int32 context_lines = 2;` to `GetSessionDiffRequest`. Default value `0` means "use server default (3)". The backend reads it in `GetSessionDiff`, passes it through to `instance.RefreshDiffStatsIfStale(contextLines)` or a dedicated `GetDiffWithContext(contextLines int)` path, and `Diff()` receives the parameter to pass to `NewUnifiedEncoder`. This is the cleanest approach:

- Diff content grows linearly with context but is already fetched on demand.
- The frontend can request e.g. 10 or "all" context by sending a large value (the encoder handles it naturally — it just includes more unchanged lines).
- "Expand all" = re-fetch with `context_lines = -1` (or a sentinel like `999999`), which the backend maps to `math.MaxInt`.
- Requires: proto change → `make proto-gen` → threaded backend change (small) → frontend re-fetch on expand click.

### Strategy B — Client-side expansion only
Always fetch with a large fixed context (e.g., 20 lines) and hide lines beyond the default window in the renderer state. Expand = unhide locally. No proto change, no backend change. Downside: 20-line context diffs are 3-5× larger; "expand all" is impossible without the full file content.

**Recommendation:** Strategy A for "expand N more lines" via re-fetch; the proto change is one line and the backend plumbing is ~5 lines. Strategy B can be used as a fallback or for the "show N more" micro-interaction while the re-fetch is in flight.

---

## 6. Inline File Diff (AC-3)

The full diff content is already in `SessionVcsContext.diff.content`. The approach is entirely client-side:

1. `VcsWidgetFileList` gains an `onViewFileDiff?: (path: string) => void` prop alongside the existing `onNavigateToFile`.
2. The VCS tab in `SessionDetailView` wires `onViewFileDiff` to set a `selectedDiffFile` state variable.
3. When `selectedDiffFile` is set, the VCS tab renders a scoped `DiffRenderer` (or a new `FileDiffView` component) filtered to that file's `DiffFile` entry from `parseDiff(diff.content)`.
4. A "← Back" button clears `selectedDiffFile`.

No new RPC, no proto change. The `parseDiff` result is already computed in `DiffRenderer`; a shared `useMemo` call at the `DiffViewer`/`VcsPanel` level can avoid re-parsing.

---

## 7. Key Risks and Unknowns

| Risk | Likelihood | Mitigation |
|---|---|---|
| **Split view scroll synchronization is fragile** — delete-only hunks have no right-side row, so naive scrollTop mirroring drifts | Medium | Use a shared hunk-by-hunk position table rather than raw scrollTop; render paired placeholder rows to keep visual alignment |
| **Lezer tokenization async latency for syntax highlighting** — `loadCodemirrorLang` returns a Promise; large diffs with many files could cause visible flash | Low-Medium | Tokenize lazily (on scroll into view) using an IntersectionObserver or virtualized list |
| **Context re-fetch latency** — adding a context_lines proto field causes `RefreshDiffStatsIfStale` to always re-compute even for cached diffs | Low | Gate re-computation on `contextLines != current cached contextLines` |
| **parseDiff regex is not robust against all edge cases** (e.g., `@@ -1 +1 @@` without count, binary diffs, rename headers) | Low | The existing parser already handles common cases; edge cases are pre-existing tech debt, not introduced by this feature |
| **`ShortcutContext` type is shared** — adding `"diff"` context to a union type requires updating `CONTEXT_LABELS` and the `getAll()` record — easy to miss one | Low | Both are in `shortcutRegistry.ts` and `KeyboardShortcutOverlay.tsx`; updating both is a 2-file change |

---

## 8. Library Recommendations

| AC | Recommendation | Rationale |
|---|---|---|
| AC-1 Hunk navigation | No new library; use existing `parseDiff` + `useShortcut` | All data structures already present |
| AC-2 Split view | No new library; custom CSS grid layout | Zero dependencies; full theme control |
| AC-3 Inline file diff | No new library; filter existing `parseDiff` output | Full diff already in context |
| AC-4 Syntax highlighting | Implement `LezerHighlighter` using `@lezer/highlight` + existing language packages | Already in bundle; `Highlighter.ts` interface is the designed seam |
| AC-5 Context expansion | Strategy A: add `context_lines` to proto | Clean, minimal backend change; encoder already supports arbitrary context |
| AC-6 Shortcut docs | No new library; existing `ShortcutRegistry` + `useShortcut` | Auto-appears in `?` overlay |

---

## 9. Implementation Touchpoints Summary

| AC | Files to create or modify |
|---|---|
| AC-1 Hunk navigation | `DiffRenderer.tsx` (add nav buttons + `activeHunkIndex` prop), `DiffViewer.tsx` (hunk state + `useShortcut`), `DiffRenderer.css.ts` (active-hunk highlight), `shortcutRegistry.ts` (add `"diff"` context), `KeyboardShortcutOverlay.tsx` (add context label) |
| AC-2 Split view | `DiffRenderer.tsx` (split mode layout), `DiffRenderer.css.ts` (two-column grid) |
| AC-3 Inline file diff | `VcsWidgetFileList.tsx` (add `onViewFileDiff` prop), `VcsWidget.tsx` (thread prop), `VcsPanel.tsx` (thread prop), `SessionDetailView.tsx` (wire inline diff state), new `FileDiffView.tsx` (scoped single-file diff renderer) |
| AC-4 Syntax highlighting | New `web-app/src/components/sessions/files/engine/LezerHighlighter.ts`, `DiffRenderer.tsx` (render `TokenSpan[]` per line) |
| AC-5 Context expansion | `proto/session/v1/session.proto` (add `context_lines`), `make proto-gen`, `session/git/diff.go` (thread through), `server/services/session_service_watch.go` (read field), `DiffRenderer.tsx` (expand button), `useSessionVcs.ts` (pass context_lines on re-fetch) |
| AC-6 Shortcut docs | Covered by `useShortcut` registrations in AC-1; no separate work |
