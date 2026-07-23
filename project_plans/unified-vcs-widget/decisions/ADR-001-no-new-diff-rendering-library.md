# ADR-001: No New Diff-Rendering Library — VcsWidget Delegates to Existing Modals

**Status**: Accepted
**Date**: 2026-07-18
**Project**: unified-vcs-widget

**Promoted to `docs/adr/ADR-024-no-new-diff-rendering-library.md`.**

## Context

Requirements' Rabbit Holes section flags "`ReviewChangesModal` vs. inline expandable diff" as a decision that must be made explicitly, not assumed away. Build-vs-buy research (`research/build-vs-buy.md` §1) evaluated two options for the diff-rendering gap (`DiffRenderer.tsx`/`parseDiff.ts` is hand-rolled, no syntax highlighting, split-view toggle disabled):

- **Option A**: extend the existing hand-rolled `DiffRenderer`/`parseDiff.ts`.
- **Option B**: adopt `react-diff-view` (headless, vanilla-extract-compatible, has hunk expand/collapse and split-view built in) and migrate off `parseDiff.ts`.

Build-vs-buy research made Option B conditional: "recommended *if and only if* the UX research agent confirms the 'inline expandable diff' requirement needs side-by-side view and/or syntax highlighting." Architecture research (`research/architecture.md` §4) resolved that condition: the `VcsWidget` component's `onViewDiff` prop opens whichever diff surface already exists at each call site (`ReviewChangesModal` on Backlog detail, `WorktreeDiffModal` on Unfinished item detail) — `VcsWidget` itself never renders a diff inline. UX research's "one interaction away" tier (§2) explicitly assigns diff viewing to "deep-dive without separate navigation... already the pattern in `UnfinishedItemDetail.tsx`," i.e. modal-based, not inline-in-widget.

## Decision

`VcsWidget` does not render diffs itself in v1. It exposes `onViewDiff?: () => void`, and each call site wires it to its existing modal (`ReviewChangesModal` for Backlog item detail, `WorktreeDiffModal` for Unfinished item detail; Session detail's Files tab navigation via `onNavigateToFile` covers the live-session case). No new diff-rendering dependency is introduced. `DiffRenderer.tsx`/`parseDiff.ts` are left as-is — this project does not touch them.

## Consequences

- Zero new npm dependency, no bundle-size review needed against `web-app/package.json`'s `size-limit` budgets.
- Split-view and syntax highlighting remain unimplemented (pre-existing gap, unchanged by this project) — out of scope for unified-vcs-widget.
- If a future project needs an inline expandable diff (not just "open a modal"), re-open build-vs-buy research Option B (`react-diff-view`) at that time rather than reusing this ADR's decision — the condition that made B unnecessary here (no inline diff surface in `VcsWidget`) will no longer hold.
- `parseDiff.ts`'s untested regex parser (noted in build-vs-buy research as having no located test file) is not addressed by this project; flagging here so it isn't assumed fixed.

## Alternatives Considered

- **Adopt `react-diff-view`, build inline expand/collapse diff inside `VcsWidget`**: rejected — duplicates two already-working modal diff viewers, adds a new dependency, and the UX research's own disclosure-tier analysis places diff-viewing at "modal/deep-dive," not "always-visible or one-click-inline."
