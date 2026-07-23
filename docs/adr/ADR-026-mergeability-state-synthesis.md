# ADR-026: Client-Side `MergeabilityState` Synthesis Fixes the Closed/Merged Conflation Bug

**Status**: Accepted
**Date**: 2026-07-18
**Project**: unified-vcs-widget

**Promoted from**: `project_plans/unified-vcs-widget/decisions/ADR-003-mergeability-state-synthesis.md`

**Note**: `project_plans/unified-vcs-widget/implementation/plan.md` amends this decision with a 10th `MergeabilityState` member, `"snapshot_unavailable"`, per an architecture-review BLOCKER finding (a durable-snapshot capture failure must have its own explicit branch, not silently misclassify into `ci_pending`) — see plan.md's Story 1.1.5/Task 1.1.5c and Pattern Decisions table for the amendment. This ADR's original 9-state decision is preserved below as accepted history.

## Context

UX research (`research/ux.md` §2) identifies the single highest-value new UX element: none of the three current surfaces synthesizes CI conclusion + review counts + conflict state + local clean/dirty into one glanceable "is this ready" signal — each surface makes the viewer mentally combine 3-4 separate fields. Features research (`research/features.md` edge case #2) and pitfalls research both flag a live bug this synthesis must not inherit: `github/priority.go:29-31`'s `DerivePRPriority` maps both `state == "merged"` and `state == "closed"` to `PRPriorityComplete`, and `GitHubBadge.tsx`'s `priorityLabel()` renders `PRPriorityComplete` as "Merged" unconditionally — so a PR closed without merging currently displays as "✓ Merged," which is false. The durable `shipped`/`shippedVia` fields (from `git.IsCommitOnMain`, unaffected by this bug) already give a trustworthy independent signal.

## Decision

Introduce a new frontend sum type, `MergeabilityState`, computed by a single pure function `deriveMergeabilityState(data: VcsWidgetData): MergeabilityState` in `web-app/src/lib/vcs/mergeability.ts`. It is a closed set of specific states (not booleans combined ad hoc per render site):

```ts
type MergeabilityState =
  | "draft"
  | "conflicted"
  | "changes_requested"
  | "ci_failing"
  | "ci_pending"
  | "ready_to_merge"
  | "shipped"          // git-history-confirmed on main — wins over any GitHub PR state
  | "closed_unshipped" // PR closed without merging AND not on main — the bug-fix case
  | "no_pr";
```

Precedence rule, evaluated top-down: `data.branchExists === false && !onMain` cases aside, **`shipped` (derived from durable git-history `Shipped`/`ShippedVia`) always wins over a GitHub `prState` of "merged" or "closed"** — if the durable ship-status says not-yet-on-main but the GitHub PR shows `state === "closed"`, the widget must render `closed_unshipped`, never a stale "Merged" label. This directly fixes the bug: `github/priority.go` is not modified (other call sites of `DerivePRPriority`/`PRPriorityComplete` outside this widget are out of scope), but `VcsWidget` does not call `DerivePRPriority`/`priorityLabel` at all — it computes its own state from `GithubSummary.prState` + `data.branchExists`/git-derived shipped fields directly, sidestepping the buggy function entirely for this surface.

## Consequences

- `MergeabilityPill` (a `VcsWidget` sub-component) renders exactly one of the 9 states above as a single colored+labeled pill, satisfying the UX research's "one synthesized token" recommendation.
- `github/priority.go`'s existing bug remains present for `GitHubBadge.tsx`'s other call sites (`SessionCard.tsx`, `SessionRow.tsx`) — this project does not fix it there; a future backlog item should, since the same false "Merged" label persists on Sessions list/row views. Flagged as an explicit non-goal here, not silently left inconsistent.
- Because `MergeabilityState` is a closed union, the render switch in `MergeabilityPill` is exhaustive (TypeScript compile error on a missing case) — the same "compile-time-enforced exhaustiveness" guard already used by `dispatch.ts`'s `OmnibarAction` switch (`.claude/rules/feature-testing-registry.md`).

## Alternatives Considered

- **Reuse `github.DerivePRPriority`/`priorityLabel` as-is inside `VcsWidget`**: rejected — inherits the closed/merged conflation bug directly into the new widget, which the requirements' Feature Landscape research explicitly calls out as a bug the new widget "must not inherit."
- **Fix `github/priority.go` in place** (distinguish `closed` from `merged`): rejected for this project's scope — `DerivePRPriority` is consumed by `GitHubBadge.tsx` at 3 call sites across Sessions list/row/detail that are out of scope for unified-vcs-widget; changing shared backend logic to fix a frontend-only widget's display risks an untested behavior change on unrelated surfaces. `VcsWidget` computing its own state independently avoids this blast radius while still fixing the bug where it matters for this project.
