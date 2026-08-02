# Requirements: backlog-description-prominence

**Date**: 2026-08-01
**Type**: bug fix / small UX polish (existing component, default-state inversion)
**Complexity**: 1 — single-file-ish fix, no new architecture

## Problem Statement

Item: `5b0e1d57-5244-4847-9b61-029b147f6aab` — "Backlog description should be more prominent."

In `BacklogItemDetail.tsx`, the **Description** section is one of the sibling
`CollapsibleSection`s inside the secondary-info `CollapsibleGroup`, defaulted
collapsed (`useSectionExpandState(itemId, "description", false)`,
`BacklogItemDetail.tsx:323`). The **Acceptance Criteria** section, by
contrast, is rendered outside the collapsible group entirely and is always
visible (`BacklogItemDetail.tsx:1146-1152`, `AcCriteriaList.tsx`).

This is backwards relative to how the data is actually populated: the
description is the field a user (or triage) fills in up front and is
almost always non-empty; acceptance criteria are frequently empty at
creation time and filled in later (if at all) by triage/planning. Today a
user opening an item must click to expand Description to read the one
field that's already there, while Acceptance Criteria — often just "No
acceptance criteria defined." — takes up permanent, always-open space.

## Baseline

- `DescriptionSection` (`web-app/src/components/backlog/detail/DescriptionSection.tsx`)
  renders inside `CollapsibleGroup`, `defaultExpanded` effectively controlled
  by `descriptionExpanded` state seeded `false`.
- `AcCriteriaList` renders unconditionally, always expanded, never
  collapsible, positioned above the `CollapsibleGroup` alongside
  `ActionsSection`.
- Expand/collapse state is per-item, per-section, persisted to
  `localStorage` via `useSectionExpandState` (key
  `backlog-detail-section-${itemId}-${sectionKey}`) — once a user manually
  toggles a section for a given item, that choice sticks for that item.

## Users / Consumers

Solo user (Tyler) operating the stapler-squad web UI, desktop and mobile.

## Success Metrics

- Opening a backlog item's detail view for the first time (no prior stored
  preference for that item) shows the description content without an
  extra click, whenever a description exists.
- No regression to the existing per-item persisted expand/collapse
  preference — a user who has manually collapsed Description on a specific
  item continues to see it collapsed on return visits to that item.
- Acceptance Criteria's current always-visible behavior is preserved (out
  of scope to also collapse it — see Suggestions below for the discussed
  alternative that was descoped).

## Appetite

Extra small (well under a day). This is a one-line default-value flip plus
test coverage, not a redesign.

## Constraints

- Reuse `useSectionExpandState`'s existing default-expanded parameter —
  do not introduce new persistence machinery.
- `defaultExpanded` only affects users with no stored preference yet for
  that item/section (new items, or existing items never manually toggled).
  Existing localStorage entries already recorded as `false` for a given
  item will continue to render collapsed — this is expected, not a bug (the
  hook's contract in `useSectionExpandState.ts:8-13`), and is called out
  explicitly so it isn't mistaken for the fix "not working."
- Follow `.claude/rules/css-architecture.md` if any CSS changes are needed
  (none anticipated — this is a default-prop change).
- Update `docs/registry/features/` only if this touches a registered
  feature marker (unlikely for a default-state change to an existing
  section — verify during planning).

## Non-functional Requirements

- No performance, scalability, or security implications — pure client-side
  default UI state.

## Out of Scope

- Redesigning the collapsible section system itself.
- Changing Acceptance Criteria's always-visible behavior (e.g. collapsing
  it when empty) — flagged as a possible follow-up, not part of this item's
  acceptance criteria.
- Migrating existing users' already-collapsed-by-choice localStorage state.

## Acceptance Criteria (from item, verbatim)

The originating backlog item provided no explicit AC list — only the
description above. AC are derived from the problem statement in Step 3's
output.
