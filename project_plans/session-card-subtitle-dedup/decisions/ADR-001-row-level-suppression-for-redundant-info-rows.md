# ADR-001: Row-Level Suppression, Not Value-Only, for Redundant `SessionCard` Info Rows

**Status**: Accepted
**Date**: 2026-08-06
**Project**: session-card-subtitle-dedup

## Context

`SessionCard.tsx` renders a primary title (`session.title`, line 454) followed by up to 8 labeled info rows (`web-app/src/components/sessions/SessionCard.tsx:700-778`). This feature suppresses 5 of those rows (Branch, Path, Working Dir, Cloned To, Goal) when their value exactly duplicates the title, to reduce visual redundancy (issue TylerStaplerAtFanatics/stapler-squad#175).

For 3 of those 5 rows — Path, Working Dir, Cloned To — the redundancy check is a *basename* comparison (`basenameOf(session.path) === session.title`), not a full-string comparison. That means when a row is suppressed, the value being hidden is not fully redundant: only its last path segment matches the title. The parent-directory portion (e.g. `/tmp/clones/` in `/tmp/clones/shared-fixes`) is information the title alone does not carry. Two of these three rows (Path at `:713`, Cloned To at `:760`) also carry a `title=` HTML attribute (hover tooltip) unique to that row, which is lost if the row doesn't render at all.

requirements.md's AC2 explicitly left this open: *"the corresponding row's redundant value is not rendered (row itself may be hidden or its value suppressed — plan.md decides which, consistently)."*

## Decision

Suppress the entire row (label, value, and any tooltip) — never render a row with a blanked or partially-redacted value. Applied uniformly across all 5 in-scope rows (Branch, Path, Working Dir, Cloned To, Goal), via the existing `session.branch && (...)` conditional-JSX idiom already used elsewhere in the file.

The resulting information loss (parent-directory prefix + hover tooltip, only for the 3 basename-compared rows) is accepted as a bounded, deliberate tradeoff, not an oversight, because the full untruncated value remains available one click away: `SessionDetailView.tsx` renders `session.path` (`:881-884`), `session.workingDir` (`:906`), and `session.clonedRepoPath` (`:1180-1183`) unconditionally whenever present, independent of any title match.

## Alternatives Considered

- **Keep the row, blank or truncate only the value.** Rejected: no existing row in this file is ever rendered with an empty/placeholder value — every optional row today is either fully present or fully absent (`session.branch && (...)`, `session.workingDir && (...)`, etc.). Introducing a new "present but valueless" row state would be a new visual pattern the rest of the card doesn't use, and would require deciding what a blanked row even displays (an empty value span reads as a rendering bug, not an intentional dedup).
- **Keep the row, replace the value with just the non-redundant remainder** (e.g. show `/tmp/clones/` instead of `/tmp/clones/shared-fixes` when the basename matches). Rejected: meaningfully more logic (path-splitting + partial-string rendering) for marginal benefit, and produces a visually truncated-looking path with no trailing segment that reads as broken/cut-off rather than intentional. Not worth the complexity for a v1 display-polish feature; the full value is one click away regardless.
- **Suppress only for the exact-string-match rows (Branch, Goal) and never suppress the 3 basename-compared rows at all**, avoiding the tradeoff entirely. Rejected: this would leave the most common real-world case for Path/Working Dir/Cloned To (a worktree or clone directory named exactly after the session) unaddressed, undermining the feature's stated goal, for a loss (parent-directory detail) that's already recoverable via `SessionDetailView`.

## Consequences

- All 5 in-scope info rows use one consistent suppression mechanism (`isRedundantWithTitle`/`basenameOf` + `&&`-guarded JSX) — easy to reason about, easy to test via the pure predicates alone (`SessionCard.subtitle-dedup.test.tsx`).
- Path and Cloned To rows lose their `title=` hover tooltip specifically in the (uncommon) case where their basename exactly matches the session title. This is scoped to that one condition — the tooltip is unaffected whenever the row still renders.
- If a future need arises to preserve the parent-directory detail directly on the card (rather than requiring a click into `SessionDetailView`), revisit with the "replace value with non-redundant remainder" alternative above — not blocking for this change.
