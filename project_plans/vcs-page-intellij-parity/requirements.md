# Requirements: VCS Page IntelliJ Parity

**Item ID:** 746a7d66-417f-4cc7-8abc-30e48d587f15  
**Date:** 2026-10-27  
**Category:** feature

## Problem Statement

The current VCS/Diff UI in stapler-squad has two separate tabs:
- **Diff tab** — `DiffViewer` → `DiffRenderer`: shows a unified diff with a file tree sidebar, a disabled "Split" toggle, no hunk navigation, and no syntax highlighting.
- **VCS tab** — `VcsPanel` → `VcsWidget`: shows file-change status, commits, PR checks, and a "navigate to Files tab" action per file — but no inline diff preview.

Users familiar with IntelliJ's VCS/diff tooling expect to:
1. Navigate between diff hunks quickly (keyboard shortcuts, prev/next buttons).
2. View a side-by-side (split) diff to see old and new content simultaneously.
3. Open a specific file's diff directly from the file list — not be redirected to a separate tab.
4. See syntax-highlighted diff content (not plain colored `+`/`-` lines).
5. Expand or collapse surrounding context lines to focus on changes.

The goal is near-parity with the IntelliJ experience without replicating every feature — focus on the highest-impact items that speed up reviewing AI-generated changes.

## Current State (as of 2026-10-27)

| Component | File | Current capability |
|---|---|---|
| `DiffViewer` | `web-app/src/components/sessions/DiffViewer.tsx` | Wraps `DiffRenderer`; no hunk nav, no split |
| `DiffRenderer` | `web-app/src/components/shared/DiffRenderer.tsx` | Unified only; split toggle is disabled ("coming soon"); file sidebar with jump-to-file scroll; no syntax highlight |
| `VcsPanel` | `web-app/src/components/sessions/VcsPanel.tsx` | File list with status icons; click navigates to Files tab; no inline diff |
| `VcsWidget` | `web-app/src/components/shared/VcsWidget.tsx` | Full VCS metadata widget; `onNavigateToFile` prop exits to Files tab |
| `parseDiff` | `web-app/src/lib/utils/parseDiff.ts` | Parses unified diff into `DiffFile[]` with hunks and line-level typed lines |
| `buildGutterMarks` | same | Builds line→mark map for gutter decorations |
| `GetSessionDiff` RPC | `proto/session/v1/session.proto:52` | Returns `DiffStats { added, removed, content }` — full unified diff string |
| `SessionVcsContext` | `web-app/src/lib/contexts/SessionVcsContext.tsx` | Shared context; exposes `diff` (SessionDiff) and `status` (VCSStatus) |

## Acceptance Criteria

### AC-1: Hunk Navigation (Prev/Next Change)
Users can press a "Next Change" and "Previous Change" button (or keyboard shortcuts **n** / **p** while focused in the diff pane) to jump between diff hunks across files. The currently-focused hunk is visually highlighted. This is the highest-value single feature from IntelliJ's diff tool.

### AC-2: Split (Side-by-Side) Diff View
The "Split" toggle in `DiffRenderer` is implemented and functional. Side-by-side view shows two columns — the original (left, red deletions) and modified (right, green additions) — with synchronized scroll. The existing "Unified" / "Split" segmented control switches between modes.

### AC-3: Inline File Diff from VCS Tab File List
Clicking a file in the VCS tab's file list opens an inline diff for that specific file within the same tab (not navigating away to the Files tab). The inline diff is the unified diff view scoped to that file's hunks. A "← Back" or "✕" action returns to the full file list.

### AC-4: Syntax Highlighting in Diff Lines
Diff lines are syntax-highlighted using the file's language (detected from its extension via the existing `EXT_TO_LANG` map in `FileContentViewer.tsx`). Highlighting applies within the diff renderer, not just the full-file viewer. Both unified and split modes respect syntax highlighting.

### AC-5: Expand/Collapse Context Lines
Context lines around each hunk can be expanded ("Show N more lines above/below") to give more surrounding context. The default context window matches git's default (3 lines). A "Expand all" action shows the complete file in diff view.

### AC-6: Keyboard Shortcut Documentation
All new keyboard shortcuts (hunk navigation, toggle split/unified, expand context) are listed in the existing keyboard-shortcuts help panel or tooltip.

## Out of Scope (explicitly)

- Inline comment authoring on diff lines (PR review workflow — separate feature).
- Word-level diff highlighting within a changed line (nice-to-have, not IntelliJ-critical).
- Three-way merge / conflict resolution view.
- Live diff streaming (diff is already fetched once per refresh cycle; real-time update is a separate concern).
- Reverting individual hunks from the UI.

## Priority Ordering

1. **AC-1 Hunk Navigation** — highest velocity improvement; cheap to implement with existing `parseDiff` data.
2. **AC-3 Inline file diff** — eliminates the disruptive tab-switch when reviewing a specific file.
3. **AC-2 Split view** — visually complete but implementation cost is highest (two-column layout with scroll sync).
4. **AC-4 Syntax highlighting** — improves readability; feasible via CodeMirror integration already present.
5. **AC-5 Context expansion** — quality-of-life; requires server-side or client-side context-widening strategy.
6. **AC-6 Keyboard docs** — required before shipping, low cost.
