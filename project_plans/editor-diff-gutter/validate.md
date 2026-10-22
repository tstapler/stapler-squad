# Validation: Editor Diff Gutter Indicators & Background Highlighting

## AC Validation Matrix

| # | Acceptance Criterion | Validation Method | Status |
|---|---|---|---|
| 1 | Changed lines show colored gutter bar (add/modify/delete) | Existing `ChangeGutterMarker` infrastructure handles this; verified wired in `CmEngine.createBuffer()` at line 110. Visually confirm via manual test with an uncommitted change. | WIRED — needs manual confirm |
| 2 | Changed lines have semi-transparent background highlight | `diffHighlightExtension` (Task 3/4) — new code, needs implementation + unit test | NOT YET |
| 3 | Background highlight preserves syntax readability | Opacity 10–15% chosen; verify visually across all 6 themes in manual test | DESIGN — validate post-impl |
| 4 | Works in windowed (large-file) view | WindowedFileView prop threading (Task 5); gap-line mode uses absolute line numbers (verified in research) | NOT YET |
| 5 | No decorations when no diff is active | `buildGutterMarks()` returns empty Map; `diffHighlightExtension(new Map())` returns `[]` early exit; gutter() shows nothing; verified by code reading | SATISFIED BY DESIGN |
| 6 | All 6 themes render correctly | Requires 18 new tokens (Task 1) and visual testing on all themes | NOT YET |
| 7 | Existing unit tests pass | `parseDiff.test.ts`, `buildGutterMarks` tests unchanged; `make test` verifies | EXPECTED — confirm at impl |
| 8 | New unit test for line-background decorations | Task 6 defines the test plan | NOT YET |

## Pre-Implementation Confidence Assessment

### What is already verified (CONFIRMED by source reading):

- `parseDiff.ts:88-106` correctly builds `Map<number, GutterMarkType>` with absolute new-file line numbers.
- `CmEngine.createBuffer()` at line 109 reads `init.gutterMarks` and passes it to the `gutter()` closure; the gutter bar rendering is complete.
- `FileContentViewer.tsx:222-225, 401` wires `buildGutterMarks()` output to `CodeEditor`.
- Theme tokens `gitAdded`, `gitDeleted`, `gitModified` exist in all 6 themes (`theme.css.ts:133, 241, 351, 469, 587, 708`).
- The gap-line lazy controller preserves absolute line numbers (verified: `cm6LazyDoc/gapTable.ts` translates between gap-adjusted and absolute positions; `lineAt().number` returns absolute in gap-line mode).

### What is INFERRED (requires implementation to verify):

- `Decoration.set()` on `visibleRanges` is sorted — standard CM6 pattern, high confidence.
- Semi-transparent backgrounds won't clash with syntax highlighting — plausible, verify visually.
- WindowedFileView internal `CodeEditor` already accepts `gutterMarks` via `ReadSurface` (VERIFIED: `ReadSurface.tsx:21,46` has the prop).

### Open Question

- **Window-swap (A1) mode line numbers**: In `spike-files`, if someone opens a large file in A1 mode, line numbers in the ViewPlugin will be relative to the swapped window, not the absolute file. This is a known limitation. Add a code comment and file a follow-up; do not block this feature.

## Scope Validation

The plan is tightly scoped: 4 modified files + 1 new engine file + 2 test additions. No backend changes. No new feature flags needed (gutter decorations are always shown when diff is present; they are not a beta feature requiring a flag gate). No proto/RPC changes.

## Test Strategy

1. `make test` in fresh worktree after `make build` to get generated code.
2. `cd web-app && bunx jest --testPathPatterns="diffHighlight|CmEngine.conformance" --no-coverage` for targeted test run.
3. Manual: open the Files tab on a file with uncommitted changes — verify gutter bars and row backgrounds appear on changed lines.
4. Manual: open a large file (triggering windowed mode) with changes — verify decorations appear.

## Estimated Complexity

Low-medium. All infrastructure is in place. Primary new code is the `diffHighlight.ts` ViewPlugin (~40 lines) and theme token additions (18 values across 6 themes). No architecture changes.
