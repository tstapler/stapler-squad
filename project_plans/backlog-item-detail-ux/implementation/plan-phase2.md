# Implementation Plan — Phase 2: Action Reachability + PR Visibility
**Date**: 2026-10-12
**Status**: Ready for implementation
**Extends**: `implementation/plan.md` (Phase 1 implementation, already complete or in progress)

---

## Background

Operator-reported friction after Phase 1 shipped. Three concrete UX gaps remain:
1. ActionsSection requires scrolling past substantial content for review/ready items.
2. PR link (`item.prUrl`) is invisible without expanding `VersionControlSection` when
   `item.status` is `done` or `archived`.
3. General section ordering buries decision-making content below reference content.

Full analysis: `research/phase2-action-reachability.md`.

---

## Step 0: Alternatives Considered

| # | Approach | Strength | Weakness |
|---|---|---|---|
| A | **Scroll area reorder + PR header badge** (chosen) | Directly addresses both complaints; additive, no behavioral change to existing sections; consistent with LifecycleSummary's established "always-visible header" model | Requires updating the ordering comment in `BacklogItemDetail.tsx` to match new rationale |
| B | Floating "sticky actions" bar (CSS `position: sticky` on ActionsSection) | Near-zero code change — just a CSS addition | `position: sticky` interacts poorly with the existing `overflow-y: auto` scroll container (a child cannot `position: sticky` inside a `overflow: auto` ancestor without explicit height — common trap; see MDN sticky-in-overflow caveat). Also leaves the PR link problem unsolved. |
| C | Add a separate "Quick Actions" row to the sticky header, fully replacing ActionsSection | Clean UX — actions are always in the header | ActionsSection has complex content (manual review form, Jules dispatch trigger, send-back box, conditional rendering per status) — fully duplicating it in the header is a large, risky refactor with the duplication problem Phase 1 set out to eliminate. Incompatible with requirements. |

**Chosen: Approach A.** Minimal code surface, directly addresses both complaints, no regression
risk to the existing collapsible section machinery.

---

## Epic P2.1: PR Badge in LifecycleSummary

### Story P2.1.1: Add PR badge to LifecycleSummary
**Goal**: Make the PR link visible in the sticky header whenever `item.prUrl` is set, zero
clicks and zero scrolls.

**Acceptance Criteria**:
- When `item.prUrl` is set, a `GitHubBadge` with `prNumber`/`prUrl` renders inside
  `LifecycleSummary` regardless of `item.status`.
  - *Given* `item.status === "done"` and `item.prUrl === "https://github.com/x/y/pull/42"`,
    *When* `LifecycleSummary` renders, *Then* `data-testid="lifecycle-pr-badge"` is present
    and its link target is `item.prUrl`.
  - *Given* `item.status === "pr_pending"` and `item.prUrl` set, *When* `LifecycleSummary`
    renders, *Then* NO badge is rendered — `PullRequestSection` already renders expanded
    with the PR link + "Mark Done" button; a duplicate header badge adds noise (adversarial
    review finding #1).
  - *Given* `item.prUrl` is `undefined`, *When* `LifecycleSummary` renders, *Then* no PR
    badge or link is rendered — the badge is omitted entirely, not shown as a placeholder.

**Files**:
- `web-app/src/components/backlog/detail/LifecycleSummary.tsx`
- `web-app/src/components/backlog/detail/LifecycleSummary.css.ts`
- `web-app/src/components/backlog/detail/LifecycleSummary.test.tsx`

#### Task P2.1.1a: Add `prUrl`/`prNumber` props to `LifecycleSummaryProps` (~2 min)
- Add `prUrl?: string; prNumber?: number;` to `LifecycleSummaryProps`.
- Wire from `BacklogItemDetail.tsx`'s `<LifecycleSummary>` call site:
  `prUrl={item.prUrl} prNumber={item.prNumber}`.
- Files: `LifecycleSummary.tsx`, `BacklogItemDetail.tsx`

#### Task P2.1.1b: Render `GitHubBadge` when `prUrl` set (~4 min)
- Import `GitHubBadge` from `@/components/shared/GitHubBadge`.
- Add `{prUrl && item.status !== "pr_pending" && <GitHubBadge prNumber={prNumber} prUrl={prUrl} data-testid="lifecycle-pr-badge" />}`
  inside the LifecycleSummary layout, positioned after the StageTracker row (same row as
  pipeline/rework-cap badges if present, or as its own row if space is tight).
- **Adversarial review fix**: suppress badge when `status === "pr_pending"` because
  `PullRequestSection` already renders expanded with the PR badge + Mark Done button — a
  second badge in the header for that status adds duplication, not value.
- Files: `LifecycleSummary.tsx`

#### Task P2.1.1c: Style integration (~3 min)
- If GitHubBadge's existing styles fit the sticky header, no new CSS needed. If it needs
  size/spacing adjustment for the header context, add a wrapper class in `LifecycleSummary.css.ts`
  — never change `GitHubBadge`'s own styles for one call site.
- Files: `LifecycleSummary.css.ts`

#### Task P2.1.1d: Tests (~4 min)
- Add 3 cases to `LifecycleSummary.test.tsx`:
  1. prUrl set, any status → badge renders with correct href
  2. prUrl set, status `pr_pending` → badge still renders (not suppressed because PullRequestSection exists)
  3. prUrl undefined → no badge, no broken link
- Files: `LifecycleSummary.test.tsx`

---

## Epic P2.2: Scroll Area Reorder

### Story P2.2.1: Move ActionsSection + PlanVerdictBox to top of scroll area
**Goal**: Make the primary decision-making content the first thing in the scroll area.

**Acceptance Criteria**:
- ActionsSection renders before GateBlockingSection in the scroll area DOM.
  - *Given* any item, *When* `BacklogItemDetail` renders, *Then* `ActionsSection`'s DOM node
    appears before `GateBlockingSection`'s DOM node in document order (verified via
    `compareDocumentPosition` in the test).
- PlanVerdictBox remains immediately before ActionsSection (co-location of `approve_plan` +
  `reject_plan` preserved).
- GateBlockingSection, GuidanceRequestPanel, and triage-specific content (TriageLoadingIndicator,
  TriageReviewPanel, ChatRefinementPanel) remain after ActionsSection.

**Files**:
- `web-app/src/components/backlog/BacklogItemDetail.tsx`
- `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

#### Task P2.2.1a: Reorder JSX in scroll area (~5 min)
Move the `PlanVerdictBox` block and `ActionsSection` block to be the FIRST rendered content
inside `<div className={styles.scrollArea}>`, before the `GateBlockingSection` call:

```tsx
<div className={styles.scrollArea}>
  {/* Decision-making content FIRST — zero scroll to reach primary action */}
  {/* Plan review + Actions: PlanVerdictBox immediately precedes ActionsSection
      so approve_plan (in ActionsSection) and reject_plan (in PlanVerdictBox) are
      co-located per plan.md Task 3.1.3b's co-location rationale. */}
  {(item.status === "ready" ||
    item.status === "queued" ||
    derivePlanReviewStatus(item) !== "no_plan") && (
    <PlanVerdictBox ... />
  )}
  <ActionsSection ... />

  {/* Reference/context content below */}
  <GateBlockingSection item={item} />
  {/* GuidanceRequestPanel, TriageLoadingIndicator, TriageReviewPanel, ... */}
  ...
```

- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

#### Task P2.2.1b: Regression test for DOM order (~4 min)
- Add one test asserting `ActionsSection` precedes `GateBlockingSection` in document order.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

#### Task P2.2.1c: Update `DescriptionSection` default-expanded decision (~3 min)
Re-evaluate per pre-mortem #1: now that the reorder places ActionsSection at the top,
DescriptionSection is further from the natural landing zone. Flip `DescriptionSection`'s
default from collapsed to expanded if dogfooding data confirms it is expanded on near-every
open (see pre-mortem failure mode #1). This is a judgment call: if the default should stay
collapsed, document why in a comment.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

## Epic P2.3: VersionControl Auto-Expand When PR Exists

### Story P2.3.1: Expand VersionControlSection by default when `item.prUrl` is set
**Goal**: For `done`/`archived` items where prUrl is set, auto-expand VersionControlSection
on first render (without overriding a user's manual collapse choice on subsequent renders).

**Acceptance Criteria**:
- For `item.status === "done"` and `item.prUrl` set, `VersionControlSection` defaults to
  expanded on first render for an item the user hasn't previously interacted with.
  - *Given* no stored preference (`localStorage` key absent) for `version-control` for this
    itemId, and `item.status === "done"` with `item.prUrl` set, *When* the detail view
    renders, *Then* `VersionControlSection`'s `Collapsible` has `aria-expanded="true"`.
  - *Given* the user has manually collapsed the section (stored preference `"false"`), *When*
    the same item opens on a subsequent render, *Then* `VersionControlSection` stays collapsed —
    the stored preference wins over the prUrl default.
- The change does NOT affect `VersionControlSection` for items with no prUrl and non-active
  statuses — they still default collapsed.

**Note**: This is a complementary improvement to the PR badge (P2.1.1). The badge provides
zero-scroll PR access; the VCS auto-expand ensures the full VCS widget (CI status, diff link)
is also available without a click for done items with PRs.

**Files**:
- `web-app/src/components/backlog/BacklogItemDetail.tsx` (the auto-expand `useEffect`, lines ~494–513)
- `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

#### Task P2.3.1a: Extend VersionControl auto-expand condition (~3 min)
- In the `useEffect` at lines ~500–504, change the auto-expand condition from:
  ```js
  ["in_progress", "review", "pr_pending"].includes(item.status)
  ```
  to:
  ```js
  ["in_progress", "review", "pr_pending"].includes(item.status) || !!item.prUrl
  ```
- `!!item.prUrl` covers `done`/`archived` (and any other status) where a PR was created.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

#### Task P2.3.1b: Test for done + prUrl auto-expand (~3 min)
- Add a test: render with `status:"done"` + `prUrl` set + no stored preference → VCS section
  `aria-expanded="true"`.
- Add the inverse: `status:"done"` + no prUrl → VCS section `aria-expanded="false"`.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

---

## Risk Control

- No backend changes — pure frontend display adjustments.
- No new collapsible sections or primitives — reuses existing `LifecycleSummary`,
  `GitHubBadge`, `useSectionExpandState`, `ActionsSection` without modification.
- Rollback: each epic is independently revertable (badge, reorder, VCS expand are
  three separate, independent changes with no cross-dependency).

## Observability

No new metrics — personal tool, no production rollout process.

## Unresolved Questions

1. **DescriptionSection default (Task P2.2.1c)**: Should Description flip to expanded-by-default
   after the reorder changes the spatial context? Pre-mortem #1 flagged this as a dogfooding call.
   Resolve before Task P2.2.1c: count Description expands on the first N item opens after P2.2
   ships; if ≥90% of opens expand Description, flip the default.

2. **Primary action button in header (deferred)**: The initial scope included a compact
   primary-action chip in the sticky header. After analysis, the scroll-area reorder (Epic P2.2)
   addresses the same problem with less code (no new header component, no per-status action
   mapping extraction). The header chip adds value only if the scroll area still has a noticeable
   scroll before ActionsSection — which after the reorder it does not. Leave the header chip as a
   future improvement if dogfooding after the reorder still shows action-finding friction.
