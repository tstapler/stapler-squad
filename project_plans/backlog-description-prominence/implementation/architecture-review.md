# Architecture Review: backlog-description-prominence
**Date**: 2026-08-02
**Verdict**: CLEAN

## Constitution Violations

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository (checked `docs/adr/`). No constitution to check against — section skipped per instructions.

## Blockers

None.

## Concerns

None.

## Nitpicks

- **Story 1.1.1 / Task 1.1.1b — `defaultExpanded: boolean` required prop is dead weight outside the group, but that's inherent to the existing convention, not new debt.** `DescriptionSection` is always rendered inside `BacklogItemDetail`'s page-level `CollapsibleGroup` in production (verified: `Collapsible.tsx:137-163`, the `insideGroup` branch ignores `defaultExpanded` except for a dev-mode divergence warning). The plan correctly identifies this in its own Domain Glossary and chooses to match the 8-sibling convention rather than invent a special case for `DescriptionSection` — the right call, since a bespoke exception here would itself be a SOLID/consistency violation (Liskov-ish surprise: one section behaving differently from its siblings for no domain reason). No action needed; flagging only so the "why does this required prop do nothing at the real call site" question doesn't resurface as a false positive in a future review.

- **Requirements.md / plan.md — "type-driven design" lens has effectively nothing to grip here, correctly.** `defaultExpanded: boolean` is a plain boolean, not a primitive standing in for a richer domain concept (unlike, say, a raw `string` doing duty as an `Email`). A boolean toggle for "is this UI section open" is the correct type — introducing an enum/sum type (`Expanded | Collapsed`) here would be over-engineering for a value that has no other legal states, no domain invariant beyond "true or false," and no risk of illegal-state combination. Confirmed no primitive-obsession smell.

- **Task 1.1.2c — the plan's decision not to add a `collapseSection()` helper to `BacklogItemDetailPage.ts` is correct and matches the Page Object pattern's actual purpose.** A raw `.click()` on an existing public locator (`detailPage.sectionHeader("description")`) is the appropriate weight for a one-off test action; wrapping it in a page-object method would be premature abstraction (the "unjustified generic"/speculative-interface smell generalized to test helpers) since no second call site exists yet. If a second spec later needs to force-collapse a section, promote it to a helper then — not preemptively.

## Lens-by-Lens Notes (no findings beyond the above)

- **SOLID**: `DescriptionSection` goes from an unconfigurable component (hardcoded `defaultExpanded={false}`, violating Open/Closed — the only way to change its behavior was editing its source) to one that receives its initial-open state as a prop, exactly mirroring 8 existing siblings. This closes an existing SOLID gap rather than opening one.
- **Layer coupling**: Pure presentational-component prop threading, one level deep (`BacklogItemDetail` → `DescriptionSection`). No new layer, no new boundary crossed.
- **DDD aggregate boundaries**: N/A, confirmed — no persisted domain aggregate is touched; `localStorage` key semantics are unchanged (only the seed default passed into `useSectionExpandState` changes, verified at `BacklogItemDetail.tsx:323`).
- **Testability**: Verified via `Collapsible.tsx` that `CollapsibleSection` rendered standalone (no wrapping group, as in `DescriptionSection.test.tsx`) genuinely honors `defaultExpanded` through its own implicit `Accordion.Root` (`Collapsible.tsx:165-176`) — so Story 1.1.2's unit tests are testing real, live behavior, not a value that's dead in the harness the way it is in production. Each proposed change is testable in isolation as designed.
- **Illegal states**: `DescriptionSectionProps` going from `{ item }` to `{ item, defaultExpanded: boolean }` (required, no `?`) cannot represent an invalid combination — matches `NotesSectionProps`'s shape exactly (verified: `NotesSection.tsx:8-13`, `defaultExpanded: boolean` required, no default).
- **Parse-at-boundary**: N/A — no raw external input is being parsed here; the boolean already originates as a proven `boolean` from `useState`/`useSectionExpandState`.
- **GoF/PoEAA pattern fit**: Correctly flagged N/A in the plan. This is "follow the existing convention," not a new pattern application. No missing pattern, no unneeded pattern introduced.
- **Build-vs-buy consistency**: Verified `research/build-vs-buy.md` recommends Build with the same reasoning the plan restates (one-line default flip + existing prop-threading convention, no new dependency — `@radix-ui/react-accordion` already in `web-app/package.json`). Plan is consistent with this.
- **API contract stability**: `DescriptionSectionProps` gains one required field. This is a breaking change to the component's prop contract, but its only consumer is the single call site in `BacklogItemDetail.tsx:1215`, which the plan updates in the same atomic unit (Task 1.1.1c). No other call site exists (verified via grep — `DescriptionSection` is imported and used exactly once). Stable and correctly scoped.
