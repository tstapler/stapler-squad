# Validation: VCS Page IntelliJ Parity

**Date:** 2026-10-27
**Phase:** SDD Phase 4 — Validate
**Feature ID:** 746a7d66-417f-4cc7-8abc-30e48d587f15

---

## Verdict

**GO with corrections.** The architecture is sound, all ACs have task coverage, and no fundamental blockers exist. Three corrections are required before implementation starts (two fix incorrect assumptions, one adds a missing task); three others are lower-priority but should be addressed before merge.

---

## Open Questions Answered

### Q1: Is `GetSessionDiffRequest` field 2 available?

**VERIFIED CLEAR.** `GetSessionDiffRequest` (proto line 1074) has exactly one field: `string id = 1;`. Field 2 is free.

The `int32 context_lines = 2;` that appears at line 2700 is in a different message — `SearchLimits` — and does not conflict.

### Q2: Is the ShortcutRegistry / ShortcutContext approach sound?

**Sound, but the plan undercounts the update locations.**

The current `ShortcutContext` union type in `web-app/src/lib/shortcuts/shortcutRegistry.ts` (line 1) is:
```typescript
export type ShortcutContext = "global" | "session-list" | "approval" | "terminal" | "cockpit" | "omnibar";
```

Adding `"diff"` to this union triggers a TypeScript compile error in `getAll()` (lines 108–115 of the same file), because that method initializes a hardcoded `Record<ShortcutContext, Shortcut[]>` object with all six keys as literal properties:
```typescript
const result: Record<ShortcutContext, Shortcut[]> = {
  global: [], "session-list": [], approval: [], terminal: [], cockpit: [], omnibar: [],
};
```
Without adding `diff: []`, TypeScript rejects the assignment.

In `web-app/src/components/ui/KeyboardShortcutOverlay.tsx` (found at `components/ui/`, not `components/layout/`), there are **two** update locations, not one:
- `CONTEXT_LABELS` record (line 23) — named in the plan
- `contexts` array (line 101): `const contexts: ShortcutContext[] = ["global", "session-list", "approval", "terminal", "cockpit", "omnibar"];` — **not named in the plan**. Omitting `"diff"` from this array means diff shortcuts will never render in the `?` overlay even if `CONTEXT_LABELS` is updated.

### Q3: Does `VcsWidgetFileList.tsx` support an `onViewFileDiff` callback, or are the type changes more invasive?

**No existing support; changes are slightly more invasive than T6 implies.**

Actual file path: `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx` — **not** `web-app/src/components/shared/VcsWidgetFileList.tsx` as T6 states.

The prop interface currently accepts only `fileChanges` and `onNavigateToFile?`. The component has three layers:
1. `VcsWidgetFileList` (exported) — passes `onNavigateToFile` to `FileSection`
2. `FileSection` — passes `onNavigateToFile` to `FileRow`
3. `FileRow` — conditionally renders a `<button>` if `onNavigateToFile` is set, otherwise a `<span>`

Adding `onViewFileDiff?: (path: string) => void` requires threading it through all three layers (interface, `FileSection` signature, `FileRow` signature and render logic). This is a 3-point prop-threading change per layer, not a 1-point addition at the top. The existing `VcsWidgetFileList.test.tsx` has 5 test cases covering the `onNavigateToFile` rendering patterns; the new prop needs at least one new test case.

---

## Plan Gaps Found

### Gap 1 (Blocker): T16 changes `Diff()` signature, breaking 5+ callers

**Root cause:** T16 says to "update the call to `fdiff.NewUnifiedEncoder`" in `session/git/diff.go` to accept a `contextLines` parameter. This implies changing the `GitWorktree.Diff()` method signature. But `Diff()` is called from six production call sites:

| File | Line | Caller |
|---|---|---|
| `server/services/session_service_watch.go` | 182, 188 | `GetSessionDiff` (completed-session paths) |
| `server/mcp/tools_vcs.go` | 102 | MCP VCS tool |
| `server/services/pr_creation_service.go` | 191 | PR creation |
| `session/git_worktree_manager.go` | 444, 455, 470 | Internal diff cache |

All six would need a `contextLines` argument added with a default value. Similarly, `RefreshDiffStatsIfStale()` (live-session path) delegates to `UpdateDiffStats()` → `git_worktree_manager.go`'s internal `wt.Diff()` calls (lines 444, 455, 470), spreading the parameter further.

**Correction for T16:** Add a new method `DiffWithContextLines(contextLines int) *DiffStats` to `GitWorktree` that calls `fdiff.NewUnifiedEncoder(contextLines)` instead of `fdiff.DefaultContextLines`. Leave `Diff()` untouched. In `GetSessionDiff`:
- For the completed-session path: call `wt.DiffWithContextLines(contextLines)` instead of `wt.Diff()`.
- For the live-session path: add a new `Instance.RefreshDiffWithContextLines(contextLines int) error` method (or call `UpdateDiffStats` with the context lines overriding parameter) that bypasses the `DiffStatsFresh()` cache check when `contextLines != defaultContextLines`, then stores the result.

This keeps the surface change local to `GetSessionDiff` and avoids touching the 5+ unrelated callers.

**T16 size reclassification:** M → **L**. The diff-stats caching on the live-session path needs special handling: `RefreshDiffStatsIfStale` skips recomputation when fresh, but a context-lines change should force recomputation. The cache key must include `contextLines`, or the non-default-context response must bypass caching entirely.

### Gap 2 (Blocker): T4 missing two update locations for ShortcutContext

T4 states: "Add `'diff'` to `ShortcutContext` union type and add label entry in `CONTEXT_LABELS`." Missing:

1. **`getAll()` body in `shortcutRegistry.ts`:** The result object initialization at lines 108–115 must include `diff: []` or TypeScript will fail to compile (`Property 'diff' is missing in type`).
2. **`contexts` array in `KeyboardShortcutOverlay.tsx`:** Line 101 hardcodes the list of contexts to render in the overlay. `"diff"` must be added here or the section is silently absent from the `?` overlay.

**Correction for T4:** Add both locations explicitly to the file-change list. T4 complexity remains S.

### Gap 3 (Correctable): T6 wrong file path for VcsWidgetFileList

T6 lists `web-app/src/components/shared/VcsWidgetFileList.tsx`. Correct path is `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx`.

T6 also needs to note that `FileRow` and `FileSection` (private sub-components within the same file) require their parameter signatures to be updated alongside the exported interface — this is still within the S complexity estimate.

### Gap 4 (Test coverage): T6 has no unit test task for VcsWidgetFileList

Five existing tests in `VcsWidgetFileList.test.tsx` cover the `onNavigateToFile` rendering path. Adding `onViewFileDiff` needs at least one test:
- `VcsWidgetFileList_should_CallOnViewFileDiff_When_FileRowClicked_And_PropProvided`
- Confirming `onNavigateToFile` is still invoked for its own button (no regression)

**Add sub-task T6a:** Update `VcsWidgetFileList.test.tsx` to cover the new callback. Complexity: S.

### Gap 5 (Placement consistency): FileDiffView.tsx placement

T7 proposes `web-app/src/components/shared/FileDiffView.tsx`. VCS-specific subcomponents are colocated under `web-app/src/components/shared/vcs-widget/`. The correct placement is `web-app/src/components/shared/vcs-widget/FileDiffView.tsx` to stay consistent with `VcsWidgetFileList.tsx`, `VcsWidgetFileList.css.ts`, etc.

---

## AC Coverage Check

| AC | Tasks | Covered? |
|---|---|---|
| AC-1: Hunk Navigation | T1 (state), T2 (highlight props), T3 (buttons), T4 (keyboard), T5 (docs) | Yes |
| AC-2: Split View | T9 (CSS), T10 (render), T11 (scroll), T12 (state + keyboard) | Yes |
| AC-3: Inline File Diff | T6 (prop), T7 (FileDiffView), T8 (VcsPanel state) | Yes |
| AC-4: Syntax Highlighting | T13 (LezerHighlighter), T14 (wire into DiffRenderer) | Yes |
| AC-5: Context Expansion | T15 (proto), T16 (backend), T17 (context), T18 (UI) | Yes — pending Gap 1 correction |
| AC-6: Keyboard Docs | T4 (diff.hunk.next/prev), T5 (descriptions), T12 (split shortcut) | Yes |

---

## Tasks to Add

| ID | Description | Phase | Complexity |
|---|---|---|---|
| T6a | Update `VcsWidgetFileList.test.tsx` to cover `onViewFileDiff` callback behavior | Phase 2 | S |

---

## Tasks to Resize

| ID | Current | Revised | Reason |
|---|---|---|---|
| T16 | M | L | Must add new `DiffWithContextLines` method on `GitWorktree` + handle live-session cache key extension; cannot simply change `Diff()` signature without touching 5+ unrelated callers |

---

## Missing Touchpoints (CLAUDE.md rules)

### route-meta.ts

**Not applicable.** This feature adds no new `app/**/page.tsx` files. `FileDiffView`, `DiffRenderer`, and `VcsPanel` are components rendered inside the existing session detail view, not standalone Next.js routes. The nav-facade rule does not apply.

### Feature registry (`make registry-generate`)

The plan correctly notes this at the end of the test strategy section. One prerequisite: check `docs/registry/features/` before adding `// +feature: vcs-diff` markers to confirm whether a `vcs` or `vcs-diff` feature entry already exists (run `make registry-diff` first, per the plan's adversarial pass Risk 5). If creating a new entry, create the per-feature JSON file before running `make registry-generate` to avoid duplicate-entry conflicts.

### Session creation registry (7 touchpoints)

**Not applicable.** No new session creation mode is introduced.

### Omnibar action registry

**Not applicable.** No new Omnibar actions are introduced.

---

## Dependency Issues

None found. Phase ordering is internally consistent:
- Phase 2 (T7 `FileDiffView`) depends on Phase 1's T2 for stable hunk IDs, but T7 notes this and renders without hunk navigation if T2 is not yet complete.
- Phase 3 (T12) depends on T4's keyboard infrastructure (correct).
- Phase 5 (T18) depends on T16 and T17 (correct), and T16's reclassification to L does not affect phase ordering.

---

## Feasibility Assessment

| Area | Assessment |
|---|---|
| Hunk navigation (AC-1) | Low risk. All data structures exist; state lives in `DiffViewer`; `useShortcut` is mature. |
| Inline file diff (AC-3) | Low risk after Gap 3 correction. Entirely client-side; diff content already in context. |
| Split view (AC-2) | Medium risk. The adversarial pass correctly identifies the single-scrollable-grid approach (not two-pane `scrollTop` sync) — this is the right call. Row-pairing algorithm complexity warrants the isolated unit test noted in the test strategy. |
| Syntax highlighting (AC-4) | Medium risk. `IntersectionObserver` lazy-load + `requestIdleCallback` mitigates the main-thread concern. `Highlighter.ts` interface is the designed seam. |
| Context expansion (AC-5) | Medium risk (revised upward from plan). The `Diff()` multi-caller issue makes T16 larger than estimated. The new `DiffWithContextLines` approach is correct but requires careful cache-key design for the live-session path. |
| tabIndex for "diff" context | Low risk. Adversarial pass Risk 4 identifies the `tabIndex={0}` requirement; include it as a required implementation note in T4. |

---

## Summary of Required Corrections Before Implementation

1. **T16 (Blocker):** Do not change `Diff()` signature. Add `GitWorktree.DiffWithContextLines(contextLines int) *DiffStats` and call it from `GetSessionDiff` only. Add `Instance.RefreshDiffWithContextLines(contextLines int)` for the live-session path. Resize T16 to L.

2. **T4 (Blocker):** Add `diff: []` to `getAll()` body in `shortcutRegistry.ts` (line ~108) and add `"diff"` to the `contexts` array in `KeyboardShortcutOverlay.tsx` (line 101). Both are in the two files T4 already names — add explicit line-level callouts to avoid missing them.

3. **T6 (Correctable):** Update file path to `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx`; note that `FileRow` and `FileSection` sub-components need prop-threading updates.

4. **T6a (New task):** Update `VcsWidgetFileList.test.tsx` to cover the `onViewFileDiff` callback rendering.

5. **T7 (Correctable):** Update `FileDiffView.tsx` target path to `web-app/src/components/shared/vcs-widget/FileDiffView.tsx`.
