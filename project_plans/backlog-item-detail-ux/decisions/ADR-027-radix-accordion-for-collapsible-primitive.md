# ADR-027: Adopt `@radix-ui/react-accordion` for the Shared Collapsible Primitive

**Status**: Accepted
**Date**: 2026-07-21
**Project**: backlog-item-detail-ux

## Context

`backlog-item-detail-ux` requires a shared collapsible/disclosure primitive to progressively
disclose 8+ secondary sections of `BacklogItemDetail.tsx` (Plan Artifacts, Version Control,
Workflow history, Progress History, etc.). No accordion/collapsible component exists in
`web-app/src/` today. Two partial, divergent patterns exist:

- `GoalPanel.tsx:120` — native `<details>`/`<summary>`, vanilla-extract styled, correct
  semantics, but only ever used for a single independent disclosure (no grouped/coordinated
  expand-collapse, no nested interactive controls in the body).
- `WorkflowsPanel.tsx`'s `RecentRuns` (~lines 34–86) — a hand-rolled `<button>` + conditional
  `<div>` toggle with **no `aria-expanded`, `aria-controls`, or `id` pairing** — a real, already-
  shipped a11y bug in this codebase, and the concrete evidence that hand-rolling this class of
  widget is easy to get subtly wrong even for an experienced Go/TS shop.

The default instinct for a solo-maintained personal tool is "a few lines of `useState` + a
`<button>` + a conditional render is enough — don't add a dependency for that." This ADR exists
because that instinct is wrong for this specific case, and the reasoning needs to be on record
so a future session doesn't re-litigate it from scratch.

## Decision

Adopt `@radix-ui/react-accordion` as the base for a new shared primitive,
`web-app/src/components/ui/Collapsible.tsx`, styled entirely with vanilla-extract `selectors`
targeting Radix's `data-state="open"|"closed"` attributes — the same integration pattern already
used for this codebase's existing `@radix-ui/react-dialog` and `@radix-ui/react-tabs` usage.

Reasons this beats "a few lines of `useState`":

1. **The redesign needs 8+ sections, several with interactive bodies.** `ActionsSection`
   contains buttons and an inline manual-review form; `SessionsSection` contains delete buttons
   and (after Epic 4) a nested diagnostic panel. Radix Accordion handles focus retention and
   keyboard nav (Home/End/Arrow between headers) around nested interactive content for free;
   hand-rolling this correctly across 8+ instances is exactly the surface area
   `WorkflowsPanel.tsx`'s `RecentRuns` got wrong with a single instance.
2. **This repo's own CI enforces the bar this ADR would otherwise skip.** Per `CLAUDE.md`, Axe
   Core UX-analysis CI blocks on WCAG AA violations for any PR touching `web-app/src/`. "It's a
   personal tool, nobody else uses it" does not remove that gate — it just means the gate is the
   only thing catching an ARIA mistake, which raises rather than lowers the value of using a
   library that gets `aria-expanded`/`aria-controls`/roving tabindex right by construction.
3. **Radix is already the house pattern for this exact problem shape** (`react-dialog`,
   `react-tabs`, `react-tooltip` are all unstyled/composable Radix primitives already vendored
   and already integrated with vanilla-extract `selectors` elsewhere in this codebase). Adding
   `react-accordion` is a same-family extension, not a new dependency category — no new update
   cadence, license review, or bundle-analysis precedent to establish.
4. **One consistent pattern beats three divergent ones.** Without this decision the codebase
   would have `<details>` (`GoalPanel.tsx`), a hand-rolled toggle (`WorkflowsPanel.tsx`), and a
   third bespoke implementation for this project — three places to remember three different
   correctness properties. Standardizing new work on Radix Accordion (while leaving `<details>`
   alone for genuinely trivial single, non-nested disclosures elsewhere) reduces that to two.

## Alternatives Considered

- **Promote `RecentFilesSection.tsx`'s pattern** (`<button aria-expanded>` + chevron +
  `localStorage`) into a shared primitive, zero new dependency. Rejected: it already gets
  `aria-expanded` right for a *single* toggle, but has no group-coordination/keyboard-nav
  story, and extending it to handle nested interactive bodies safely converges on
  re-implementing what Radix Accordion already provides — paying the engineering cost without
  the correctness guarantee.
- **`@radix-ui/react-collapsible`** (single-panel primitive, no group semantics) for each
  section independently, skipping the accordion's group coordination. Rejected: still a new
  dependency, and buys none of the (deliberately unused, but available if wanted later)
  multi-section keyboard-group behavior `react-accordion` provides for the same install cost.
- **Native `<details>` for all 8+ sections.** Rejected as the *sole* mechanism: fine for the
  `GoalPanel.tsx`-style trivial case, but has documented cross-browser focus/interaction quirks
  with nested interactive elements in some browsers, which several of the new sections have.

## Consequences

- New dependency: `@radix-ui/react-accordion` in `web-app/package.json`.
- `web-app/src/components/ui/Collapsible.tsx` + `Collapsible.css.ts` become the canonical
  disclosure primitive for all new multi-section work; `<details>` remains acceptable for new,
  genuinely single/independent/non-nested disclosures elsewhere in the codebase (not this
  project's concern to migrate).
- `WorkflowsPanel.tsx`'s `RecentRuns` ARIA gap is *not* fixed by this project (out of scope,
  per the plan's task list) — flagged here so it isn't mistaken for addressed.
