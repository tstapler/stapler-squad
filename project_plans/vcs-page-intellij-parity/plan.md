# Implementation Plan: VCS Page IntelliJ Parity

**Date:** 2026-10-27
**Phase:** SDD Phase 3 — Plan
**Feature ID:** 746a7d66-417f-4cc7-8abc-30e48d587f15

---

## Architecture Decisions

### Decision 1: Hunk navigation state lives in DiffViewer, not DiffRenderer

`DiffRenderer` is a pure display component that accepts `parseDiff` output and renders it. Adding mutable hunk-index state to it would couple two concerns and prevent `DiffRenderer` from being reused in `FileDiffView` (AC-3) without carrying session context. Instead, `DiffViewer` owns `activeHunkIndex` state and `navigateHunk(direction)` logic, and passes `activeHunkIndex`, `onPrevHunk`, `onNextHunk` as props to `DiffRenderer`. `DiffRenderer` only reads these to render the active highlight and invoke the callbacks on button click.

### Decision 2: FileDiffView is a thin wrapper around DiffRenderer, not a parallel implementation

`FileDiffView` accepts a pre-parsed `DiffFile` object (from `parseDiff(diff.content)`, already available in context) and delegates entirely to `DiffRenderer` with that single file. This avoids duplicating the render logic and keeps `DiffRenderer` as the single source of truth for diff layout. The inline-file diff path is therefore: `VcsPanel → FileDiffView → DiffRenderer(files=[selectedDiffFile])`.

### Decision 3: Split view is CSS grid inside DiffRenderer, no new component

The split layout is a different render mode of the same data, not a different data shape. Adding a `viewMode: "unified" | "split"` prop to `DiffRenderer` and toggling a CSS grid class avoids a second component tree while keeping the toggle cheap. Placeholder rows (empty `<div>` cells) are used on the opposite side for asymmetric hunks (delete-only or add-only) to maintain visual row alignment without scroll-sync arithmetic.

### Decision 4: LezerHighlighter is a new file in the existing engine/ directory

The `Highlighter.ts` interface (`web-app/src/components/sessions/files/engine/Highlighter.ts`) was designed explicitly for this use case: "a non-CodeMirror engine can reuse Lezer parsers for syntax highlighting." Implementing `LezerHighlighter.ts` in the same directory follows the established pattern, avoids touching `CmEngine.ts` or the full editor stack, and the `loadCodemirrorLang` function already handles all language package loading. Tokenization returns `TokenSpan[]` async, so the diff renderer shows unhighlighted text immediately and updates in place once tokenization completes (no flash-of-unstyled blocker on initial render).

### Decision 5: Context expansion uses server-side proto parameter (Strategy A)

The backend's `NewUnifiedEncoder` already accepts arbitrary `contextLines int`. Adding `int32 context_lines = 2` to `GetSessionDiffRequest` is a one-line proto change. The alternative (Strategy B: fetch a large context and hide lines client-side) imposes permanent payload cost and cannot support "expand all." The proto field uses zero-value = server default (3 lines), so all existing callers remain unaffected without any code change.

### Decision 6: A new "diff" ShortcutContext scopes n/p to the diff pane

Using `context: "global"` for `n`/`p` would cause those keys to fire even when the user is in a terminal or text input. A dedicated `"diff"` context requires a `data-context="diff"` attribute on the diff pane root and a two-file update (`shortcutRegistry.ts` + `KeyboardShortcutOverlay.tsx`). This is the correct pattern already established by `"terminal"`, `"cockpit"`, etc.

---

## Implementation Tasks

### Phase 1: Hunk Navigation (AC-1 + AC-6)

**T1 — Hunk navigation state and logic in DiffViewer**
- Add `activeHunkIndex: number | null` state and `hunkList: Array<{fileIndex, hunkIndex}>` derived value (flat list of all hunks across all files from `parseDiff(diff.content)`).
- Implement `navigateHunk(direction: "next" | "prev")`: wraps around, updates `activeHunkIndex`, scrolls the active hunk into view via a ref map.
- Files: `web-app/src/components/sessions/DiffViewer.tsx`
- Complexity: **M**
- Dependencies: none

**T2 — Active hunk highlight and navigation props in DiffRenderer**
- Add `activeHunkId?: string`, `onPrevHunk?: () => void`, `onNextHunk?: () => void` to `DiffRendererProps`.
- Assign stable `id` attributes to each hunk `<div>` (e.g., `diff-hunk-{fileIdx}-{hunkIdx}`) so DiffViewer can call `document.getElementById(id)?.scrollIntoView(...)`.
- Add `activeHunk` CSS class to the active hunk's container div; define the style in `DiffRenderer.css.ts` (a subtle left-border accent in the session's accent color, consistent with existing highlight styles).
- Files: `web-app/src/components/shared/DiffRenderer.tsx`, `web-app/src/components/shared/DiffRenderer.css.ts`
- Complexity: **S**
- Dependencies: T1

**T3 — Prev/Next hunk buttons in DiffRenderer toolbar**
- Add icon buttons (◀ ▶ or ↑ ↓, matching existing toolbar button style) to the existing toolbar row in `DiffRenderer`.
- Buttons are disabled when `onPrevHunk`/`onNextHunk` are undefined (graceful degradation for `FileDiffView` without navigation state) or when there are no hunks.
- Files: `web-app/src/components/shared/DiffRenderer.tsx`
- Complexity: **S**
- Dependencies: T2

**T4 — Keyboard shortcuts for hunk navigation**
- Add `"diff"` to `ShortcutContext` union type and add label entry in `CONTEXT_LABELS`.
- Add `data-context="diff"` to the diff pane root element in `DiffViewer.tsx`.
- Register `useShortcut("diff.hunk.next", { key: "n", context: "diff" }, enabled)` and `useShortcut("diff.hunk.prev", { key: "p", context: "diff" }, enabled)` in `DiffViewer`, calling `navigateHunk("next"/"prev")`.
- Files: `web-app/src/components/sessions/DiffViewer.tsx`, `web-app/src/lib/shortcuts/shortcutRegistry.ts`, `web-app/src/components/layout/KeyboardShortcutOverlay.tsx`
- Complexity: **S**
- Dependencies: T1

**T5 — Shortcut documentation (AC-6)**
- The `useShortcut` registrations in T4 automatically populate the `?` overlay via `registry.getAll()`. No separate work, but verify the shortcut descriptions render with human-readable labels (e.g., `"Next change"`, `"Previous change"`) by passing a `description` field to `useShortcut`.
- Files: `web-app/src/components/sessions/DiffViewer.tsx` (description strings only)
- Complexity: **S**
- Dependencies: T4

---

### Phase 2: Inline File Diff from VCS Tab (AC-3)

**T6 — Add onViewFileDiff prop through VCS component chain**
- Add `onViewFileDiff?: (path: string) => void` to `VcsWidgetFileList` props; render a "View diff" button (or make the file row itself clickable) that calls it.
- Thread the prop up through `VcsWidget.tsx` and `VcsPanel.tsx`.
- Do not remove `onNavigateToFile` — it remains available as a fallback when no diff content is loaded yet.
- Files: `web-app/src/components/shared/VcsWidgetFileList.tsx` (if it exists as a subcomponent) or `web-app/src/components/shared/VcsWidget.tsx`, `web-app/src/components/sessions/VcsPanel.tsx`
- Complexity: **S**
- Dependencies: none

**T7 — Create FileDiffView.tsx**
- New component accepting `file: DiffFile` and `onBack: () => void` props.
- Renders a back button row (`← Back to file list`) followed by `DiffRenderer` with `files={[file]}`.
- No hunk navigation state — `FileDiffView` is a simple scope-reducing wrapper; it does not own `activeHunkIndex` in Phase 2 (hunk nav within `FileDiffView` can be added in Phase 1 if desired, via DiffViewer-style local state).
- Files: `web-app/src/components/shared/FileDiffView.tsx` (new)
- Complexity: **S**
- Dependencies: T2 (for the stable hunk IDs, though not required for basic rendering)

**T8 — Wire selectedDiffFile state in VcsPanel**
- Add `selectedDiffFile: string | null` state in `VcsPanel.tsx`.
- When set: hide the `VcsWidget` and render `FileDiffView` with the matching `DiffFile` from `parseDiff(diff.content)`.
- When `diff.content` is not yet loaded, fall back to `onNavigateToFile(path)` (the existing Files-tab navigation).
- Parse `diff.content` with `useMemo` so `parseDiff` is not called on every render — the memo depends on `diff.content`.
- Files: `web-app/src/components/sessions/VcsPanel.tsx`
- Complexity: **M**
- Dependencies: T6, T7

---

### Phase 3: Split (Side-by-Side) Diff View (AC-2)

**T9 — CSS grid two-column layout in DiffRenderer.css.ts**
- Add a `splitContainer` class: CSS grid with two equal columns, each `overflow-x: auto`.
- Add `splitLeft` / `splitRight` pane classes.
- Add `splitRow` paired-cell classes for synchronized row rendering.
- Add `splitPlaceholder` for empty cells on asymmetric hunks.
- Files: `web-app/src/components/shared/DiffRenderer.css.ts`
- Complexity: **S**
- Dependencies: none

**T10 — Split mode rendering in DiffRenderer.tsx**
- Add `viewMode: "unified" | "split"` prop to `DiffRendererProps` (default `"unified"`).
- In split mode, for each hunk, build two aligned arrays:
  - Left side: lines with `type === "delete"` or `type === "context"`, interleaved with `splitPlaceholder` cells where the right side has add-only lines.
  - Right side: lines with `type === "add"` or `type === "context"`, interleaved with `splitPlaceholder` cells where the left side has delete-only lines.
- Use a row-pairing algorithm: iterate lines; context lines emit a paired row; for a block of consecutive delete/add lines, pair them index-by-index, padding the shorter side with placeholders.
- Enable the "Split" button in the `viewModeToggle` (remove `disabled` attribute); the existing segmented control calls `onViewModeChange` (or similar), which is now wired to update parent state.
- Files: `web-app/src/components/shared/DiffRenderer.tsx`
- Complexity: **L**
- Dependencies: T9

**T11 — Synchronized scroll between split panes**
- Each pane is a `div` with `ref`. A shared `handleScroll` closure reads `scrollTop` from the scrolling pane and sets it on the other pane via `ref.current.scrollTop = ...`, guarded by a `isSyncing` flag to prevent infinite scroll event loops.
- Files: `web-app/src/components/shared/DiffRenderer.tsx`
- Complexity: **M**
- Dependencies: T10

**T12 — Wire viewMode state up to DiffViewer**
- Add `viewMode` state to `DiffViewer.tsx`; pass to `DiffRenderer`.
- Register `useShortcut("diff.view.split", { key: "s", context: "diff" })` to toggle split/unified from keyboard.
- Files: `web-app/src/components/sessions/DiffViewer.tsx`
- Complexity: **S**
- Dependencies: T10, T4

---

### Phase 4: Syntax Highlighting (AC-4)

**T13 — Implement LezerHighlighter.ts**
- New file at `web-app/src/components/sessions/files/engine/LezerHighlighter.ts`.
- Implements the existing `Highlighter` interface: `tokenize(code: string, languageId: string): Promise<TokenSpan[]>`.
- Uses `loadCodemirrorLang(languageId)` to get the `LanguageSupport`, then instantiates its parser via `lang.language.parser.parse(code)`, walks the syntax tree with `@lezer/highlight`'s `highlightTree`, and maps each tagged range to the matching `classNameForNode` (the same CSS class names `cmTheme.ts` already defines for the full editor).
- Returns `[]` immediately for unrecognized languages (no fallback to Monaco; the diff renderer renders plain text).
- Files: `web-app/src/components/sessions/files/engine/LezerHighlighter.ts` (new)
- Complexity: **M**
- Dependencies: none (reuses existing packages already in bundle)

**T14 — Wire LezerHighlighter into DiffRenderer**
- `DiffRenderer` receives an optional `highlighter?: Highlighter` prop.
- On first render (or when `files` prop changes), fire async `highlighter.tokenize(line.content, languageId)` for each file's lines, grouped by file.
- Language ID is derived from the filename via the `detectLanguage(filename)` function already exported from `FileContentViewer.tsx` (or its underlying `EXT_TO_LANG` map).
- Store token spans in a `Map<lineKey, TokenSpan[]>` via `useState`; lines render as `<span>` elements with classes from `TokenSpan.className` when available, plain text otherwise.
- Tokenization is kicked off lazily per file using `IntersectionObserver` on each file's container div — only tokenize files currently visible in the viewport to avoid blocking the main thread for large multi-file diffs.
- `DiffViewer.tsx` instantiates `LezerHighlighter` once (via `useMemo`) and passes it down.
- Files: `web-app/src/components/shared/DiffRenderer.tsx`, `web-app/src/components/sessions/DiffViewer.tsx`
- Complexity: **L**
- Dependencies: T13

---

### Phase 5: Context Line Expansion (AC-5)

**T15 — Proto change: add context_lines to GetSessionDiffRequest**
- Add `int32 context_lines = 2;` to `GetSessionDiffRequest` in `proto/session/v1/session.proto`.
- Run `make proto-gen` to regenerate Go and TypeScript bindings.
- Convention: `0` = use server default (3 lines). `-1` = expand all (backend maps to `math.MaxInt`). Positive values are literal context line counts.
- Files: `proto/session/v1/session.proto`; generated output is auto-regenerated by Make and not committed per repo convention.
- Complexity: **S**
- Dependencies: none

**T16 — Thread context_lines through backend**
- In `server/services/session_service_watch.go` (or wherever `GetSessionDiff` is handled): read `req.ContextLines`, default `0` → `3`, map `-1` → `math.MaxInt`, pass to the diff computation function.
- In `session/git/diff.go`: update the call to `fdiff.NewUnifiedEncoder` to use the passed `contextLines int` parameter instead of `fdiff.DefaultContextLines`.
- Ensure caching is invalidated when `contextLines` differs from the last cached value (add `contextLines` to the cache key or store the last-used value alongside the cached diff).
- Files: `server/services/session_service_watch.go` (or `session_service.go`), `session/git/diff.go`
- Complexity: **M**
- Dependencies: T15

**T17 — Update SessionVcsContext to accept contextLines**
- Add `refreshDiff(contextLines?: number): void` overload (or add parameter to existing `refreshDiff`) in `SessionVcsContext.tsx`.
- The context stores `currentContextLines: number` (default 3) alongside the diff content, so the UI can display "showing N lines of context."
- Files: `web-app/src/lib/contexts/SessionVcsContext.tsx`
- Complexity: **S**
- Dependencies: T15

**T18 — Expand context UI in DiffRenderer**
- Add "▼ Show N more lines" buttons above and below each hunk (rendered when there are hidden context lines between this hunk and the previous or next one).
- Clicking fires `onExpandContext(fileIndex, hunkIndex, direction: "above"|"below", count: number)` callback prop.
- Add "Expand all" button in the toolbar; clicking fires `onExpandAll()`.
- `DiffViewer.tsx` wires these callbacks to call `refreshDiff(contextLines + 5)` (incremental expansion) or `refreshDiff(-1)` (expand all).
- While the re-fetch is in flight, the current diff content remains displayed (no spinner overlay — latency is low and content stays valid).
- Files: `web-app/src/components/shared/DiffRenderer.tsx`, `web-app/src/components/sessions/DiffViewer.tsx`
- Complexity: **M**
- Dependencies: T16, T17

---

## Phasing

### Phase 1 — Ship: Hunk Navigation (T1–T5)

**Scope:** T1, T2, T3, T4, T5

Standalone shippable: hunk navigation is fully self-contained within `DiffViewer` + `DiffRenderer`. No backend changes, no new components. Delivers the highest-value IntelliJ parity item. Keyboard shortcuts register immediately in the `?` overlay.

**AC delivered:** AC-1, AC-6 (partial — shortcuts for AC-2/AC-5 added in later phases)

**Verification gate before merge:** `make quick-check` passes; manually navigate a multi-file diff with `n`/`p` and buttons; `?` overlay shows new shortcuts.

---

### Phase 2 — Ship: Inline File Diff (T6–T8)

**Scope:** T6, T7, T8

Standalone shippable: entirely client-side, no proto/backend changes. `FileDiffView` reuses `DiffRenderer` from Phase 1 (gets active hunk highlight for free). Eliminates the disruptive tab switch.

**AC delivered:** AC-3

**Verification gate before merge:** `make quick-check`; click a file in VCS tab, verify inline diff renders, click back, verify file list is restored; verify fallback to Files tab when diff not yet loaded.

---

### Phase 3 — Ship: Split View (T9–T12)

**Scope:** T9, T10, T11, T12

Can be shipped after Phase 1 (T4 keyboard infra used in T12). Highest implementation complexity. Should be tested on asymmetric hunks (delete-only, add-only, mixed) before merge.

**AC delivered:** AC-2

**Verification gate before merge:** `make quick-check`; toggle split mode on diffs with delete-only, add-only, and mixed hunks; scroll one pane and verify the other tracks; toggle back to unified; verify `s` keyboard shortcut works.

---

### Phase 4 — Ship: Syntax Highlighting (T13–T14)

**Scope:** T13, T14

Can be shipped independently after Phase 1 (needs `DiffRenderer` props infrastructure from T2). No backend changes. The `LezerHighlighter` is independently testable.

**AC delivered:** AC-4

**Verification gate before merge:** `make quick-check`; open a diff for a `.go` and a `.ts` file; verify token colors match the full-file editor; verify that a language with no grammar (`EXT_TO_LANG` miss) renders as plain text without error.

---

### Phase 5 — Ship: Context Expansion (T15–T18)

**Scope:** T15, T16, T17, T18

Requires proto change (`make proto-gen`) and backend plumbing. Ship last because it has the most cross-cutting scope and a backend risk window. Can be developed in parallel with Phase 4.

**AC delivered:** AC-5, AC-6 (expand shortcut if keyboard is wired in T18)

**Verification gate before merge:** `make ci`; request diff with context 3 vs. context 10 and verify the correct number of context lines; click "Expand all" and verify full-file context; verify cache invalidation (second call with same context_lines is served from cache; call with different context_lines triggers a new backend diff).

---

## Test Strategy

### Unit tests

**parseDiff edge cases (`web-app/src/lib/utils/parseDiff.ts`):**
No new tests required — the parser is unchanged. Existing tests cover the hunk data shape. If edge cases are encountered during implementation, add regression fixtures there.

**LezerHighlighter.ts (`web-app/src/components/sessions/files/engine/`):**
- `tokenize(code, "go")` on a known snippet returns `TokenSpan[]` with non-empty `className` values.
- `tokenize(code, "unknown-lang")` returns `[]` without throwing.
- `tokenize("", "go")` returns `[]` without throwing.
These can be Jest unit tests with the existing `@codemirror` packages available in test scope.

**navigateHunk logic (DiffViewer):**
- With `N` total hunks across `M` files, `navigateHunk("next")` from `activeHunkIndex = N-1` wraps to `0`.
- `navigateHunk("prev")` from `0` wraps to `N-1`.
- With zero hunks, navigation does not change state.
Unit-testable as a pure function extracted from `DiffViewer` (extract a `buildHunkList(files: DiffFile[]): HunkRef[]` function and test it separately from the component).

**Split view row-pairing algorithm:**
Extract the pairing logic (delete/add line alignment with placeholder insertion) as a pure function and test it with mixed, delete-only, and add-only hunks. This is the highest-complexity logic in the plan and benefits most from isolated unit tests.

### Component tests (Jest + React Testing Library)

**DiffRenderer — unified mode (existing behavior, regression guard):**
Snapshot or behavior tests confirming line count, hunk header text, and gutter numbers are unchanged after adding `viewMode` and `activeHunkIndex` props with `undefined` defaults.

**DiffRenderer — active hunk highlight:**
Given `activeHunkId = "diff-hunk-0-1"`, the matching hunk container has the `activeHunk` CSS class.

**DiffRenderer — split mode:**
Given a diff with a delete-only hunk, render in split mode and verify a placeholder cell appears on the right side; verify no placeholder on the left.

**FileDiffView:**
Given a single `DiffFile`, renders one file's hunks and a back button; clicking back calls `onBack`.

**VcsPanel — inline diff selection:**
When `onViewFileDiff` is invoked with a path that exists in `diff.content`, `FileDiffView` is shown; when `onBack` fires, the file list is restored.

### Integration tests

**SessionVcsContext re-fetch with contextLines:**
Mock the `GetSessionDiff` RPC stub; call `refreshDiff(10)` and verify the stub received `context_lines = 10`. Call again with `10` and verify the stub was not called again (cache hit). Call with `20` and verify the stub was called.

### E2E tests (`tests/e2e/`)

**`vcs-diff-hunk-navigation.spec.ts`:**
```
// @feature session:vcs-diff
```
- Navigate to a session with a multi-file diff.
- Press `n` and verify the viewport scrolls to the next hunk.
- Press `p` and verify it returns to the previous hunk.
- Click Prev/Next buttons and verify equivalent behavior.
- Verify `?` overlay lists "Next change" and "Previous change" shortcuts.

**`vcs-inline-file-diff.spec.ts`:**
```
// @feature session:vcs-diff
```
- Navigate to a session with changed files.
- Click a file in the VCS tab file list.
- Verify the diff view for that file is shown (file-specific hunk headers).
- Click Back and verify the file list is restored.

**`vcs-split-view.spec.ts`:**
```
// @feature session:vcs-diff
```
- Click the Split toggle button in the diff toolbar.
- Verify two column panes are visible.
- Scroll the left pane and verify the right pane scrolls in sync.
- Click Unified and verify single-column view is restored.

E2E test conventions: `data-testid` locators only (no CSS class selectors), no `waitForTimeout`, helpers in `tests/e2e/pages/`.

### Feature registry

After implementation, run `make registry-generate` to emit per-feature files for any new `// +api:` or `// +feature:` markers added to handlers and React components. The `vcs-diff` feature ID should cover all new components.

---

## Adversarial Pass

### Risk 1: Split view row misalignment on asymmetric hunks causes visual desynchronization

**Scenario:** A hunk contains 5 delete lines and 2 add lines. The naive pairing emits 5 left-column rows and 2 right-column rows. Since the two panes share `scrollTop` via ref mirroring, they are always synchronized in scroll position — but the visual row heights differ, so after the hunk the next context line appears at different vertical offsets in each pane. Over a large diff this misalignment compounds, making the side-by-side comparison unusable.

**Mitigation:** The row-pairing algorithm (T10) must render paired `<div>` rows as siblings within a shared grid container, not as independent lists in separate `overflow: auto` divs. Each CSS grid row contains both the left and right cells; a `splitPlaceholder` cell occupies the gap. This way, the row heights are naturally equal (tallest cell determines the row height), and there is no `scrollTop` synchronization at all — scroll happens on the grid container, not on two separate panes. T11's scroll-sync implementation should use this grid approach rather than two-pane `scrollTop` mirroring. The research doc's scroll-sync approach (two `onScroll` refs) should be reconsidered in favor of a single scrollable grid container.

**Revised T11 scope:** If a single grid container with paired rows is used, synchronized scroll is free (one container, one scroll event). The implementation of T11 becomes adding `overflow-x: auto` per column via a sticky-column technique rather than a scroll event listener. This simplifies implementation and eliminates the `isSyncing` flag antipattern entirely.

### Risk 2: Lezer tokenization on large diffs blocks the main thread and makes the tab unresponsive

**Scenario:** A session diff touches 40 files, each with 200+ lines. On tab open, `DiffRenderer` fires 8,000+ async tokenization calls. Even though `loadCodemirrorLang` is async, the Lezer `parser.parse(code)` step is synchronous and CPU-intensive. With 40 files × 200 lines, firing all tokenizations immediately causes a jank spike measurable in hundreds of milliseconds.

**Mitigation:** T14 specifies lazy per-file tokenization via `IntersectionObserver`. Tokenize a file's lines only when its container `div` intersects the viewport, and do so inside a `requestIdleCallback` (or `scheduler.postTask` with `background` priority where available). Cap concurrent tokenizations to 2 files at a time using a simple semaphore in `LezerHighlighter.ts`. For the specific case of `FileDiffView` (single file), fire tokenization eagerly since the file is always visible.

### Risk 3: Proto field zero-value (context_lines = 0) is indistinguishable from "use default" at the call site, causing the backend to silently serve 0-line context diffs

**Scenario:** The TypeScript protobuf-generated request object initializes `contextLines` to `0` (proto3 default). When `refreshDiff()` is called without an argument, the generated client sends `context_lines: 0`. If the backend treats `0` as a literal "zero context lines," the returned diff contains only the changed lines with no surrounding context — the exact opposite of the intended default.

**Mitigation:** Define the zero-value convention explicitly in both the proto comment and the backend handler: `0` MUST mean "use server default (3)." The backend handler checks `if req.ContextLines == 0 { req.ContextLines = 3 }` before passing to `NewUnifiedEncoder`. This is a one-line guard in `session_service_watch.go`. Document the convention in the proto field comment:
```protobuf
// Number of context lines to include around each hunk.
// 0 = server default (3). -1 = include all lines (expand all).
int32 context_lines = 2;
```
Add a backend unit test asserting that a `GetSessionDiff` request with `context_lines = 0` returns the same diff as one with `context_lines = 3`.

### Risk 4: The "diff" ShortcutContext doesn't fire because data-context attribute is never active

**Scenario:** The `ShortcutRegistry` dispatches to the `"diff"` context by walking `data-context` attributes up from `document.activeElement`. If the diff pane is a `div` without `tabIndex`, it cannot receive keyboard focus, so `document.activeElement` is always `<body>` (or the previously-focused element), and the `"diff"` context is never resolved. Pressing `n`/`p` fires nothing.

**Mitigation:** The diff pane root element must have `tabIndex={0}` (or `tabIndex={-1}` with `focus()` called on mount) so it can receive focus. Add `tabIndex={0}` to the diff pane container in `DiffViewer.tsx` alongside `data-context="diff"`. Optionally, auto-focus the pane when the diff tab is activated (call `paneRef.current?.focus()` in a `useEffect` keyed on tab visibility). Cover this in the E2E test by verifying that pressing `n` without a prior click still navigates (requires the auto-focus behavior).

### Risk 5: Registry generate creates duplicate feature markers after adding new components

**Scenario:** New `// +feature: vcs-diff` markers in `FileDiffView.tsx` and updated markers in `VcsPanel.tsx` overlap with an existing `vcs` feature entry in `docs/registry/features/`. `make registry-generate` emits a new per-feature file that conflicts with a pre-existing one, breaking `make registry-diff`.

**Mitigation:** Before adding any `// +feature:` markers, run `make registry-diff` to inspect the current state of the registry. Use the same feature ID slug already used for existing VCS components (inspect `docs/registry/features/` to confirm the naming pattern). If a new `vcs-diff` feature entry is warranted, create the per-feature JSON file before adding the markers and confirm `make registry-generate` is idempotent on a second run.

---

## Implementation Notes (post-implementation)

### Deviations from the plan (all intentional)

| Plan | As built | Why |
|---|---|---|
| D1: hunk state in `DiffViewer` | New `components/sessions/DiffPane.tsx` owns hunk cursor, `n`/`p`/`s` shortcuts, view mode, highlighter; used by `DiffViewer` and `FileDiffView` | One implementation shared by the Diff tab and the inline file diff |
| T7: `FileDiffView` in `shared/vcs-widget/`, takes a `DiffFile` | `components/sessions/FileDiffView.tsx`, takes the file's raw diff text | `DiffPane`/`LezerHighlighter` live in `sessions/`; eslint `boundaries` forbids `shared/` → `sessions/` |
| T11: synchronized scroll | None needed | Split view is one grid per hunk, so both sides share a scroll container (per adversarial Risk 1) |
| T14: `IntersectionObserver` lazy tokenizing | Hunks tokenized sequentially with a macrotask yield, results published in batches of 8 | Simpler; **deferred**: viewport gating for very large diffs |
| T16: `Instance.RefreshDiffWithContextLines` | Handler calls `GitWorktree.DiffWithContextLines` directly for non-default context (bypasses the diff-stats cache); `Diff()` unchanged | Keeps the 5+ other `Diff()` callers untouched (validation Gap 1) |
| T17: state in `SessionVcsContext.tsx` | State in `useSessionVcs` (`diffContextLines`/`setDiffContextLines`), exposed through the existing context | The context just forwards the hook's value |
| T18: per-hunk expand callbacks | One global `contextLines` value; every "Show N more lines" click adds 10 lines of context to all hunks; reset to default when the diff view unmounts | Matches the proto (a single `context_lines`); a per-gap fetch would need a richer RPC |

### sdd:6-verify review disposition

Four reviewers (architecture, refactor candidates, TS/React idioms, Go/proto idioms) ran against the feature diff. No BLOCKER and no MUST FIX that survived; findings were handled as follows.

**Fixed:** `context_lines` validated in the handler (< -1 → `InvalidArgument`, > 10 000 → whole file) with a unit test; whole-file sentinel halved (`MaxInt32/2`) to avoid go-git overflow; diff errors on the cache-bypass path now surface as `Internal` instead of an empty success; `DiffPane` no longer steals focus on mount (opt-in `autoFocus`, set by the Diff tab and the inline file view), has a `:focus-visible` ring, `role="region"` and an accessible name; token state published in batches instead of one `Map` copy per hunk; `buildHunkList` computed once; `pairSplitRows` returns line indices (identity `Map` removed); `prefers-reduced-motion` respected for hunk scrolling; refresh button `aria-label`/`type`; hunk counter is a live region; inline toolbar style moved to vanilla-extract; `viewMode` prop honoured (no half-controlled state); `Highlighter`/`TokenSpan` single definition in `lib/highlight/types.ts`; token class names single definition in `lib/highlight/tokenClasses.ts`; context-line constants in `lib/diff/contextLines.ts`; `extractFileDiff` matches on `+++ b/` / `--- a/` (handles deleted files and paths containing ` b/`); `VcsPanel` leaves the inline diff if a refresh drops the file; "Expand all" no longer sticks after the diff view goes away.

**Deferred (not required by the acceptance criteria):**
- Splitting `DiffRenderer.tsx` (~500 lines) into `HunkView`/toolbar/`useHunkTokens` modules — structural refactor, tracked as follow-up.
- Viewport-gated tokenizing (`IntersectionObserver`) — only matters for multi-hundred-hunk diffs.
- Moving `detectLanguage` out of `FileContentViewer.tsx` into `engine/languages.ts`.
- Live directory sessions ignore `context_lines` (documented in the proto comment); a pre-existing unsynchronized `baseCommitSHA` write in `GitWorktree` (not introduced here) is untouched.
- A service-level `GetSessionDiff` test with a live-instance fake; the clamp helper and the git-level behaviour are unit-tested.
- Split view conveys add/delete by background tint only (the `+`/`-` prefix is dropped); revisit with a marker column.
- Duplicate shortcut ids are not an issue today: the Diff tab and the VCS tab never mount together (`SessionDetailView` renders them conditionally).
