# Requirements: Backlog description should be more prominent

Source: backlog item `5b0e1d57-5244-4847-9b61-029b147f6aab`. Derived directly
from the item's description and acceptance criteria (no interactive ideate
interview — none was run per pipeline instructions).

## Problem

In `BacklogItemDetail`, `DescriptionSection` is a `CollapsibleSection` with
`defaultExpanded={false}` hardcoded internally (`DescriptionSection.tsx:20`).
Description is the field a user actually fills in when creating an item, so
hiding it by default buries the most load-bearing content behind a click.
Acceptance Criteria, by contrast, is rendered unconditionally, always visible,
outside any `CollapsibleSection` (`BacklogItemDetail.tsx:1147-1152`) even
though it's frequently empty at creation time. This item only changes
Description's default-expanded behavior — it does not touch Acceptance
Criteria's rendering.

## Functional Requirements

Numbered to match the item's acceptance criteria 1:1.

1. **Description expanded by default on first view.** Opening a backlog
   item's detail view for the first time, with no stored per-item
   `localStorage` preference for the `description` section key, must render
   Description's content already visible — zero clicks.
2. **Stored per-item preference still wins.** If a user previously collapsed
   Description on a specific item (stored under
   `backlog-detail-section-${itemId}-description` in `localStorage`), that
   item must still show Description collapsed on return visits. The stored
   preference must take precedence over the new default, exactly as
   `useSectionExpandState` already does for every other section — this is
   existing, unmodified behavior that must not regress.
3. **Acceptance Criteria unaffected.** Acceptance Criteria's always-visible
   rendering and layout must be provably unaffected by this change — a test
   must directly assert this (not just "no diff touched it").
4. **`DescriptionSection` takes `defaultExpanded` as a prop.** Currently
   `DescriptionSection` hardcodes `defaultExpanded={false}` internally and
   only accepts `item` as a prop. It must instead accept `defaultExpanded:
   boolean` as a prop, threaded from `BacklogItemDetail` exactly like the
   ~8 sibling sections (`NotesSection`, `SessionsSection`,
   `VersionControlSection`, `PlanArtifactsSection`, `WorkflowHistorySection`,
   `ProgressHistorySection`, `LastReviewResultSection`,
   `ReviewingSection`/`PullRequestSection`). The component's stale docstring
   ("secondary info, collapsed by default") must be updated to match the new
   behavior.
5. **E2E spec updated.** `tests/e2e/backlog-item-detail-redesign.spec.ts`
   currently asserts Description starts `aria-expanded="false"` (the old
   default) in its "expands a top-level section from its own
   default-collapsed state" test. This assertion must be replaced: assert
   Description starts `aria-expanded="true"` by default, then exercise
   collapse (click to `false`) and re-expand (click back to `true`).
6. **No collateral changes.** No new npm dependencies, no CSS changes (no
   `.css.ts` edits), and no new/changed `docs/registry/features/` entries.
   `make quick-check` and the targeted Jest suites (`DescriptionSection.test`,
   `BacklogItemDetail.markdown.test`, `BacklogItemDetail.test`) must all pass.

## Non-Functional / Constraints

- Persistence mechanism (`useSectionExpandState`, `localStorage` key
  `backlog-detail-section-${itemId}-${sectionKey}`) is not modified — only
  the *default* value passed into it for the `description` key changes from
  `false` to `true`. This is the same mechanism `PullRequestSection`
  (`true`) and `SessionsSection` (`true`) already use for a non-`false`
  default, so no new pattern is introduced.
- `CollapsibleGroup`/`CollapsibleSection` component API and roving-tabindex
  keyboard navigation (ADR-027) are untouched.
- Change is confined to: `DescriptionSection.tsx` (prop signature + doc),
  `BacklogItemDetail.tsx` (one `useSectionExpandState` default value +
  passing the new prop), `DescriptionSection.test.tsx`,
  `BacklogItemDetail.markdown.test.tsx`, and
  `tests/e2e/backlog-item-detail-redesign.spec.ts`. No proto, no Go backend,
  no registry involvement — this is UI-default-only.

## Out of Scope

- Any change to Acceptance Criteria's structure, styling, or collapsibility.
- Any change to other sections' default-expanded values.
- Any change to `useSectionExpandState`, `CollapsibleSection`, or
  `CollapsibleGroup` implementations.
- Feature registry updates (no new/changed RPC or UI feature — an existing
  UI default is being flipped, not registering a new capability). This
  matches AC6's explicit "no docs/registry/features/ entries" requirement.

## Acceptance Criteria (verbatim from backlog item)

1. Opening a backlog item's detail view for the first time (no stored
   per-item preference) shows the Description section expanded with its
   content visible, with zero clicks.
2. A user who previously manually collapsed Description on a specific item
   still sees it collapsed on return visits to that item — the stored
   per-item preference always wins over the new default.
3. Acceptance Criteria's existing always-visible rendering and layout are
   unaffected by this change (directly asserted, not just assumed).
4. `DescriptionSection` accepts `defaultExpanded` as a prop threaded from the
   parent (matching ~8 sibling detail sections) instead of a hardcoded value,
   and its stale docstring is updated to match.
5. `tests/e2e/backlog-item-detail-redesign.spec.ts` no longer asserts the old
   collapsed-by-default precondition for Description; it asserts the new
   expanded default and exercises collapse/re-expand.
6. No new dependencies, CSS changes, or `docs/registry/features/` entries are
   introduced; `make quick-check` and the targeted Jest suite
   (DescriptionSection, BacklogItemDetail.markdown, BacklogItemDetail.test)
   pass.
