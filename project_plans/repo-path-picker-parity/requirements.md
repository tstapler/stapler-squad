# Requirements: Repo Path Picker Parity

Backlog item: `5dbbd510-035f-47e3-bba5-9b7f7ad73ffa`
Source: "The repository path field does not support all of the features of our other path
selection fields" (priority 3). No interactive ideation — derived directly from the item
description and its 7 acceptance criteria, per SDD pipeline instructions (no user present).

## Problem

`OmnibarCreationPanel.tsx` has two repository-path text fields that are plain
`<input type="text">` elements, duplicating logic that already exists in the shared
`RepoPathInput` component used everywhere else in the app:

1. **Parent Directory** (New Project mode) — `OmnibarCreationPanel.tsx:488-501`
2. **Existing Worktree Path fallback** (Existing Branch mode, shown only when the
   worktree-discovery `<select>` has no options or errors) — `OmnibarCreationPanel.tsx:651-660`

Neither surfaces recent/session-history path suggestions the way `RepoPathInput` does for
`BacklogItemForm`, `NewShellDialog`, `WorkflowForm`, and `LocalFileBrowser`.

## Current-state findings (from codebase exploration)

- `RepoPathInput` (`web-app/src/components/ui/RepoPathInput.tsx`) already implements
  history-suggestion + filesystem-completion dropdown behavior via `useSessionRepoPaths()`
  + `usePathCompletions()` + `PathCompletionDropdown`. It is the correct reuse target — no
  new picker component should be built.
- `useSessionRepoPaths()` (`web-app/src/lib/hooks/useSessionRepoPaths.ts`) currently sources
  from `selectAllSessions` (Redux entity-adapter's raw insertion order) — **not**
  recency-ordered. `selectActiveSessionsSortedByUpdatedAt` already exists in
  `sessionsSlice.ts:117-123` (filters out `SessionStatus.UNSPECIFIED`, sorts by
  `updatedAt.seconds` descending) but has **no defined tiebreak** when `updatedAt` is
  missing/zero for two or more sessions — `Array.prototype.sort` falls back to whatever
  order the entity adapter returns, which is not a meaningful recency signal.
- **Escape-key hazard**: `Omnibar.tsx`'s outer `modal` div attaches `onKeyDown={handleKeyDown}`
  (`Omnibar.tsx:1210`), which every descendant keydown bubbles into by React's synthetic
  event delegation. `Omnibar.tsx` also has a `document`-level native listener
  (`Omnibar.tsx:891-901`) that closes the omnibar entirely (`onClose()`) on any bubbled
  Escape. Existing dropdown-closing code paths inside `Omnibar.tsx` (lines 748, 772, 820, 839)
  all call `e.nativeEvent.stopImmediatePropagation()` specifically to stop a "close my own
  dropdown" Escape from also resetting/closing the omnibar. `RepoPathInput`'s own Escape
  handler (`RepoPathInput.tsx:134-137`) does **not** stop propagation today — none of its
  current call sites are nested inside a keydown-capturing ancestor, so it has never needed
  to. Embedding `RepoPathInput` inside `OmnibarCreationPanel` (a descendant of `modal`)
  without fixing this would regress: pressing Escape to close just the suggestion dropdown
  would also reset the creation panel / close the omnibar. This is exactly what AC6 guards
  against and must be fixed as part of this change.
- The "Existing Worktree Path" `<select>` (populated when git worktrees are discovered) is
  a distinct, legitimate UI and is **out of scope** — only its fallback plain-text `<input>`
  (shown when discovery finds nothing or errors) is in scope, matching the AC1 wording.
- Existing e2e test `tests/e2e/session-create-new-project.spec.ts` uses
  `page.getByLabel('Parent Directory *')` — the `<label htmlFor="omnibar-parent-dir">` /
  `id="omnibar-parent-dir"` wiring lives in `OmnibarCreationPanel.tsx`, not inside
  `RepoPathInput` itself, so swapping the `<input>` for `<RepoPathInput id="omnibar-parent-dir" />`
  preserves that wiring and should not break existing label-based locators.
- No feature registry entry currently exists for the omnibar repo-path fields or for
  `RepoPathInput` itself under `docs/registry/features/frontend/`.

## Requirements (mapped to acceptance criteria)

**R1 (AC1).** Replace the plain-text Parent Directory input and the Existing Worktree Path
fallback input in `OmnibarCreationPanel.tsx` with the shared `RepoPathInput` component.
No new/duplicated picker implementation.

**R2 (AC2).** Both fields must surface existing-session-path suggestions consistent with
`RepoPathInput`'s other consumers (same dropdown, same history-entry styling/behavior).

**R3 (AC3).** `useSessionRepoPaths` must source from `selectActiveSessionsSortedByUpdatedAt`
instead of `selectAllSessions`, and `selectActiveSessionsSortedByUpdatedAt`'s sort must gain
a defined, deterministic tiebreak for sessions sharing a missing/zero `updatedAt` (e.g. fall
back to `createdAt` descending, then session `id`) so suggestion order is stable and
meaningfully most-recent-first rather than incidental adapter order.

**R4 (AC4).** Manual free-text entry must keep working in both fields — typing an arbitrary
path not present in the suggestion list must not be overridden, cleared, or blocked by the
dropdown/selection logic.

**R5 (AC5).** Dropdown rendering (visibility, no horizontal overflow, tappable row targets)
must be verified for both fields at a standard desktop viewport and at 390×844 (phone).

**R6 (AC6).** Fix the Escape-key propagation hazard identified above: dismissing a
`RepoPathInput` dropdown's own suggestions via Escape must not bubble to
`OmnibarCreationPanel`/`Omnibar.tsx`'s reset-to-discovery or close-omnibar handlers. No
regression to existing repo-path validation, `canSubmit` gating, or Omnibar's existing
Escape-to-reset behavior when no `RepoPathInput` dropdown is open.

**R7 (AC7).** Add/update e2e tests per `.claude/rules/e2e-test-conventions.md` covering the
above, register the feature(s) under `docs/registry/features/frontend/`, run
`make registry-generate`, and confirm `docs/registry/coverage-gaps.json` does not grow.

## Out of scope

- The "Existing Worktree Path" `<select>` (populated path) — unaffected.
- Any change to `RepoPathInput`'s filesystem-completion (`usePathCompletions`) behavior,
  GitHub-URL detection, or its use in `BacklogItemForm` / `NewShellDialog` / `WorkflowForm` /
  `LocalFileBrowser` beyond the shared hook/selector fix in R3 (which benefits all of them
  incidentally — recency ordering was never differentiated per-consumer).
- Any change to the main Omnibar text input's own separate path-completion dropdown
  (`isDropdownVisible` / `mergedEntries` in `Omnibar.tsx`) — that is a different, older
  completion system, not `RepoPathInput`.
