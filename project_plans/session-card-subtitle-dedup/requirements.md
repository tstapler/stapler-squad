# Requirements: session-card-subtitle-dedup

## Source

- Migrated from GitHub issue [TylerStaplerAtFanatics/stapler-squad#175](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/175), opened 2026-07-10.
- Backlog item id `e6f644c7-9088-4a84-a7e4-4bb6f107def6`.

## Problem

`SessionCard.tsx` (`web-app/src/components/sessions/SessionCard.tsx`) renders a
primary title (`session.title`, line ~454) followed by a block of secondary
info rows (`Program`, `Branch`, `Path`, `Working Dir`, `Repository`, `Pull
Request`, `Cloned To`, `Goal` — lines 700–778). None of these rows are
currently deduplicated against the title. When a session's title happens to
equal (or trivially match) one of these values — e.g. the session was named
after its branch, or the title is the same as the working-directory
basename — the secondary line(s) repeat information already visible in the
primary line, adding visual noise to the session list.

Note: as of this triage, `SessionCard` does NOT have a single consolidated
"subtitle" line the way `herdr-web`'s `paneMeta()` example builds one — it has
several discrete labeled info rows in the card body. The issue's proposed
`paneMeta()`-style helper (build one joined line, skip parts equal to the
title) is the referenced pattern, but the concrete integration point in this
codebase is per-row suppression (or consolidation) inside `SessionCard.tsx`,
not a literal port of the referenced function's shape. Research phase should
confirm the best integration shape (per-row suppression vs. a new
consolidated subtitle line) against actual production session data.

## Goal

Apply title-aware deduplication so that any secondary display element whose
value is identical (or a case-insensitive / whitespace-normalized match) to
the visible session title is not rendered a second time, reducing redundancy
in the session list UI. Scope is display-only — no changes to underlying
session data, proto fields, or session creation flow.

## Out of scope

- Changing what data is stored on `Session` (proto fields untouched).
- Changing the 7-touchpoint session-creation-mode registry.
- Redesigning the card's visual layout beyond removing/collapsing duplicate rows.
- Deduplicating against fields not currently rendered on the card.

## Acceptance Criteria (initial — refined further in plan.md)

1. Given `session.title` equals `session.branch` (exact match, case-sensitive
   comparison per current title/branch normalization), the `Branch:` row is
   not rendered.
2. Given `session.title` equals the basename of `session.path` or
   `session.workingDir`, the corresponding row's redundant value is not
   rendered (row itself may be hidden or its value suppressed — plan.md
   decides which, consistently).
3. Given `session.title` equals `session.program`, the `Program:` row is not
   rendered.
4. Given none of the secondary fields duplicate the title, `SessionCard`
   renders exactly as it does today (no visual regression for the common
   case).
5. Deduplication logic is implemented as a pure, unit-testable helper
   function (mirroring the existing `hasPendingProgramChange` pattern already
   in `SessionCard.tsx`, line 22) rather than inlined ad hoc into JSX.
6. Comparison is resilient to trivial formatting differences the UI itself
   would consider "the same" (surrounding whitespace at minimum); exact
   normalization rules (case-folding, path separators) are decided in
   plan.md and documented in the helper's doc comment.
7. Existing `SessionCard` tests (`web-app/src/components/sessions/__tests__/*`)
   continue to pass; new unit tests cover the dedup helper directly.
8. No change to `aria-label`/accessibility text that would remove
   information available to screen-reader users beyond what's visually
   deduped (i.e., don't strip data that's the *only* source of some other
   fact just because it string-matches the title incidentally).

## References

- Issue: https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/175
- Prior art: `herdr-web`'s `paneMeta()` in `state.ts` (external repo, referenced in issue body only — not vendored here).
- Current implementation: `web-app/src/components/sessions/SessionCard.tsx:700-778` (info rows), `:454` (title).
