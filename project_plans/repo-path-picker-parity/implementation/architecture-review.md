# Architecture Review: repo-path-picker-parity
**Date**: 2026-08-01
**Verdict**: CLEAN

## Constitution Violations
None — no ADR-000 found (`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository).

## Blockers
None. The previously-blocked item (Task 1.1.1a gating `stopImmediatePropagation()` on `open`
instead of `showDropdown`) is resolved. Verified in the current `plan.md`:

1. **Code snippet now gates on `showDropdown`** — Task 1.1.1a's snippet (lines 147-153) reads
   `if (showDropdown) { e.nativeEvent.stopImmediatePropagation(); }`, not `open`.
2. **`useCallback` dependency-array fix is called out** — Task 1.1.1a explicitly instructs
   updating `handleKeyDown`'s deps from `[open, allEntries, selectedIndex, handleSelect]` to
   include `isLoading` (or depend on `showDropdown` directly), with the reasoning that
   `showDropdown` depends on `isLoading` which `open` alone does not (lines 155-159).
3. **Story 1.1.1's Given/When/Then correctly describes `showDropdown`-gated behavior** —
   the first AC bullet keys off `showDropdown === true` (lines 118-123); the second bullet
   explicitly covers both `open === false` AND the "focused but empty" case
   (`open === true` but `allEntries.length === 0` and not loading), and calls out by name
   that this is "the case a gate on `open` alone would get wrong" (lines 124-133).
4. **Concrete unit test task covers `open=true, showDropdown=false`** — Task 1.1.1c's third
   case (lines 206-216) mocks `useSessionRepoPaths` to `[]` with no filesystem matches,
   focuses the input (dropdown does not render), presses Escape, and asserts BOTH that
   `stopImmediatePropagation` was not triggered and that the parent's `onKeyDown` **was**
   called — i.e. the event bubbles normally. This is exactly the previously-missing case.

The Domain Glossary (`open`/`showDropdown` rows, lines 27-28) and the Pattern Decisions
table (Escape-propagation fix location row, line 38) are also both updated to state the
`showDropdown`-gating rationale consistently with the task/story text. No remaining
inconsistency between the a11y fix (Task 1.1.2a, `aria-expanded={showDropdown}`) and the
Escape fix — both now key off the same variable, closing the internal-inconsistency
observation from the prior pass.

## Concerns
Prior concern — `research/build-vs-buy.md` §3 stating the `id` tiebreak as "desc," contradicting
the plan's ascending choice — is resolved: the research doc now reads "then `id` ascending"
(line 58), consistent with the Domain Glossary, Story 1.2.1's GWT example, and Task 1.2.1a's
comparator code. No open concerns remain.

One residual (harmless) inconsistency: the plan.md Domain Glossary's `Recency tiebreak` row
(line 23) still describes `build-vs-buy.md` §3 as having "stale wording saying 'desc'" —
that description is itself now stale, since the research doc already reads "ascending."
This is a doc-of-a-doc staleness note pointing at an already-fixed issue; it does not affect
implementation (no code or task references the incorrect wording) and is not worth blocking
on — flagged as a nitpick below.

## Nitpicks
- `plan.md` line 23 (Domain Glossary, `Recency tiebreak` row) claims `research/build-vs-buy.md`
  §3 has stale "desc" wording for the `id` tiebreak; that research doc line now correctly
  reads "ascending." Cosmetic only — worth a one-line edit to `plan.md`'s glossary note so it
  doesn't send a future reader chasing an already-fixed discrepancy, but not blocking.
- The `e.nativeEvent.stopImmediatePropagation()` idiom for "suppress my own Escape from
  bubbling to an ancestor's own Escape handler" will exist in 5 places after this change
  (4x already in `Omnibar.tsx` at lines 748/772/820/839, plus the new `RepoPathInput.tsx`
  occurrence). The plan correctly avoids over-abstracting a single call site into a hook
  (matches the codebase's existing inline-repetition idiom, and `RepoPathInput`'s version
  needs its own local `open`/`showDropdown` state anyway), but if a 6th occurrence shows up
  in a future change, a small shared `useEscapeToClose(isOpenAndVisible, onClose)` hook would
  be worth extracting at that point — not before.
- The New Project mode's Parent Directory field loses its prior static hint ("Directory
  where the new project folder will be created") in favor of new copy explaining the
  history-suggestion semantics (Task 2.1.1a). This is a deliberate, explicitly-documented
  product-copy tradeoff (Pattern Decisions table, last row) rather than an oversight —
  flagged here only for visibility, not as a defect.
