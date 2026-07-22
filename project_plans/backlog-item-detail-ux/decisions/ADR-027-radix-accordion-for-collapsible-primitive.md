# ADR-027: Adopt `@radix-ui/react-accordion` for the Shared Collapsible Primitive

**Status**: Accepted
**Date**: 2026-07-21
**Project**: backlog-item-detail-ux

## Context

`backlog-item-detail-ux` requires a shared collapsible/disclosure primitive to progressively
disclose 8+ secondary sections of `BacklogItemDetail.tsx` (Plan Artifacts, Version Control,
Workflow history, Progress History, etc.). No dedicated shared accordion/collapsible *primitive*
exists in `web-app/src/components/ui/` today. Three partial, divergent patterns exist:

- `GoalPanel.tsx:120` — native `<details>`/`<summary>`, vanilla-extract styled, correct
  semantics, but only ever used for a single independent disclosure (no grouped/coordinated
  expand-collapse, no nested interactive controls in the body).
- `WorkflowsPanel.tsx`'s `RecentRuns` (~lines 34–86) — a hand-rolled `<button>` + conditional
  `<div>` toggle with **no `aria-expanded`, `aria-controls`, or `id` pairing** — a real, already-
  shipped a11y bug in this codebase, and the concrete evidence that hand-rolling this class of
  widget is easy to get subtly wrong even for an experienced Go/TS shop.
- `StuckItem.tsx`/`UnfinishedItem.tsx` (`web-app/src/components/backlog-stuck/`, from the
  already-shipped `backlog-stuck-item-visibility` project) — the **strongest** existing
  precedent: `isExpanded`/`onToggleExpand` lifted to parent state, `aria-expanded`,
  Enter/Space/Escape keyboard handling, a `wasExpandedRef`-driven focus-return effect on
  collapse, and a `cardExpanded` CSS-variant class (not inline style). `research/pitfalls.md` §2
  names this pattern explicitly and recommends reusing it over inventing a new one. It is
  evaluated as a first-class alternative below, not omitted — its one structural gap (the header
  is a `<div role="button">`, not a real `<button>`) is exactly the kind of thing this project's
  own Story 1.1.1 acceptance criterion is designed to prevent going forward.

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
- **Extract `StuckItem.tsx`/`UnfinishedItem.tsx`'s pattern into a shared primitive**
  (`isExpanded: boolean` prop lifted to a parent `useState<Set<string>>`, `aria-expanded`, a
  `wasExpandedRef` to detect collapse transitions and return focus, Escape-to-collapse, a
  `cardExpanded` CSS-variant class) — the strongest alternative, and the one
  `research/pitfalls.md` §2 explicitly recommends reusing ("Reuse this pattern rather than
  inventing a new accordion primitive"), since it is already shipped, proven in this exact
  codebase across two independent consumers (`StuckItem.tsx`, `UnfinishedItem.tsx`), and adds
  zero new dependency.
  - **Pros**: already shipped and battle-tested against real daily use; zero new dependency;
    `wasExpandedRef`-driven focus return on collapse is a genuinely nice touch this ADR's
    original Radix-only analysis didn't call out; matches this codebase's general preference
    (per `research/features.md` §4) for the cheapest pattern that solves the problem over new
    generic infrastructure.
  - **Cons, and why they tip the decision back to Radix anyway**: (1) `StuckItem.tsx`'s header
    is `<div role="button" tabIndex={0}>`, not a real `<button>` — under this plan's own Story
    1.1.1 acceptance criterion ("a real `<button aria-expanded>`, never a `<div onClick>`"),
    reusing it verbatim would itself fail the bar this project is setting for every other new
    disclosure header; adopting it would require first patching `StuckItem.tsx`/
    `UnfinishedItem.tsx` to swap the `div`+`role="button"` for a real `<button>`, which is
    out-of-scope refactor work in two files this project doesn't otherwise touch (`StuckItem.tsx`
    belongs to the separate, already-shipped `backlog-stuck-item-visibility` project). (2) No
    built-in keyboard *group* navigation — each `StuckItem` instance manages its own
    Enter/Space/Escape handling independently; there's no roving-tabindex Home/End/Arrow
    traversal across a list of headers, which this redesign's 8+ sibling sections would
    otherwise benefit from (see the resolved Accordion-vs-Collapsible question below). (3) The
    pattern lives inline in `StuckItem.tsx` today, not as an extractable primitive — turning it
    into `web-app/src/components/ui/Collapsible.tsx` is itself nontrivial refactor work (pulling
    the `Set<string>`-keyed parent state, the ref-based focus-return effect, and the CSS variant
    convention out of a component that also owns unrelated stuck-item concerns like snooze
    pickers and retry buttons), not a drop-in reuse.
  - **Net call**: `StuckItem.tsx`'s pattern is the right reference for *behavior* (this ADR's
    Decision section already borrows its collapse/expand state-machine shape), but adopting it
    as the literal implementation would both inherit its `role="button"` a11y gap and require
    non-trivial extraction work for no correctness gain over Radix. Radix Accordion remains the
    chosen primitive; `StuckItem.tsx` is not migrated to it by this project (that's a separate,
    optional follow-up for the `backlog-stuck-item-visibility` component tree, not something this
    ADR's scope requires).
- **`@radix-ui/react-collapsible`** (single-panel primitive; no `Root`-level coordination across
  multiple panels) for each section independently, instead of `@radix-ui/react-accordion`.
  **Revised reasoning (the original version of this ADR rejected this option while also
  calling Accordion's group semantics "deliberately unused" — those two claims contradict each
  other, and this revision resolves it honestly rather than leaving it circular):**
  "Group semantics" bundles two genuinely separate things, and this project uses one but not the
  other:
  1. **Exclusive-open coordination** (only one panel open at a time) — **not used**. This plan's
     own sections are independently open by design (no section ever needs to force-close
     another; Sessions, Version Control, and Progress History are frequently relevant
     *simultaneously* per the Step 0.5 rejection of the tabs alternative). `type="multiple"` mode
     is used specifically to opt out of this.
  2. **Roving-tabindex keyboard navigation across a `Root`'s headers** (Home/End/Arrow move
     focus between `AccordionTrigger`s without needing Tab to walk through every intervening
     element) — **used, and this is the actual differentiator over `-collapsible`.** With 8+
     independent `Collapsible` instances (the `-collapsible` alternative), a keyboard user
     driving from the "Plan Artifacts" header to the "Notes" header at the bottom must Tab
     through every header (and, for expanded sections, every focusable element inside their
     bodies) in between. A single `Accordion.Root` wrapping all 8+ sections gives Home/End/Arrow
     traversal directly between headers regardless of expand state or body content, for the same
     install cost as `-collapsible`. That is a real, concrete keyboard-ergonomics win for a panel
     this section-dense, not a hypothetical "might want it later" — so it is not accurate to call
     the group behavior "deliberately unused"; only the *exclusivity* half of it is unused, and
     `type="multiple"` mode gives the group's navigation behavior without imposing exclusivity.
  - **Conclusion**: `@radix-ui/react-accordion` (`type="multiple"`) is kept over
    `@radix-ui/react-collapsible` specifically for the header-to-header keyboard navigation,
    not for exclusive-panel coordination this project doesn't want. No change to the package
    choice in `Task 1.1.1a`/`1.1.1c` or the Pattern Decisions table row as a result of this
    revision — the original package choice was correct, but the original justification for
    rejecting `-collapsible` was not, and is replaced by the reasoning above.
- **Native `<details>` for all 8+ sections.** Rejected as the *sole* mechanism: fine for the
  `GoalPanel.tsx`-style trivial case, but has documented cross-browser focus/interaction quirks
  with nested interactive elements in some browsers, which several of the new sections have; also
  gets none of the Home/End/Arrow group navigation described above.

## Consequences

- New dependency: `@radix-ui/react-accordion` in `web-app/package.json`.
- `web-app/src/components/ui/Collapsible.tsx` + `Collapsible.css.ts` become the canonical
  disclosure primitive for all new multi-section work; `<details>` remains acceptable for new,
  genuinely single/independent/non-nested disclosures elsewhere in the codebase (not this
  project's concern to migrate).
- `WorkflowsPanel.tsx`'s `RecentRuns` ARIA gap is *not* fixed by this project (out of scope,
  per the plan's task list) — flagged here so it isn't mistaken for addressed.
- `StuckItem.tsx`/`UnfinishedItem.tsx`'s `<div role="button">` header pattern is *not* migrated
  to `Collapsible`/`Accordion` by this project either — it was evaluated (see Alternatives
  Considered above) and kept as-is; a future project touching
  `web-app/src/components/backlog-stuck/` could migrate it to the new shared primitive to also
  close its `role="button"`-vs-`<button>` a11y gap, but that migration is not this ADR's scope.
