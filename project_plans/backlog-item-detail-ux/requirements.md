# Requirements: backlog-item-detail-ux

**Date**: 2026-07-21
**Type**: feature addition (UX redesign of an existing component)
**Complexity**: 3 — system design (multiple epics: information architecture redesign + new read-only session viewer + board card consistency, Large appetite)

## Problem Statement

`BacklogItemDetail.tsx` (1577 lines) renders 12+ sections simultaneously with no progressive disclosure — Header, Planning, Reviewing, Pull Request, Description, Actions (including an inline manual-review verdict form), Plan Artifacts, Version Control, Sessions, Pipeline group, Workflow status-history timeline, and Progress History all render at once, always expanded. This makes it hard to tell an item's current lifecycle state and what it's currently waiting on without reading the whole panel. Separately, the Sessions section (line 1333) renders triage sessions and headless/blocked review sessions as inert `<span>` text instead of clickable links — only work sessions and non-headless review sessions are openable. When an item is stuck in review, there is no way to inspect the triage or headless review session that produced that state.

## Baseline

Today, a user opens a backlog item and sees the entire panel expanded at once — duplicative and overwhelming information with no visual hierarchy signaling "here's what matters right now." To find out why an item is stuck in review, the user must infer the cause indirectly (e.g. from Progress History text) because the triage/headless-review session that actually ran is not clickable — there's no way to open its logs.

## Users / Consumers

Solo user (Tyler) operating the stapler-squad web UI at `localhost:8543`, on both desktop and mobile form factors (per existing mobile+desktop UX requirement for this project).

## Success Metrics

- From the collapsed/default view of an item's detail panel, the user can identify its current lifecycle state and what it's currently blocked on/waiting for without expanding anything.
- Every session linked to an item — work, triage, and review (including headless/blocked review) — is inspectable from the detail view: work/interactive sessions open in the full terminal view as today; triage/headless-review sessions open in a new read-only viewer (output/logs, no interaction).
- Board/list card summaries show status consistent with the detail view's lifecycle model (no duplicated or contradictory state indicators between the two).
- Reduced duplication: information that appears in more than one section today (e.g. status shown in multiple places) is consolidated to a single authoritative location.

## Appetite

Large (3–6 weeks). Explicit user direction: prioritize getting the information architecture right over shipping a partial fix — "as large as it takes, I don't want to have to revisit this."

## Constraints

- All new/modified CSS must use vanilla-extract (`.css.ts`), per `.claude/rules/css-architecture.md` — no new CSS Modules files.
- Any new omnibar-adjacent affordance must follow the existing session-creation/feature-testing registries if applicable (not expected to apply here, since this is a read/display feature, not a new creation mode).
- New/changed RPCs and frontend features must be reflected in `docs/registry/features/` (`make registry-generate`).
- Must work on both mobile and desktop (touch targets, responsive layout) per prior UX requirement for this codebase.

## Non-functional Requirements

- **Performance SLO**: not specified — this is a display/UI feature, not a hot path.
- **Scalability**: not applicable — single-user tool, item detail panels render one item at a time.
- **Security classification**: internal (personal tool, no external users).
- **Data residency**: no special requirements.

## Scope

### In Scope
- Redesign `BacklogItemDetail.tsx`'s information architecture: introduce progressive disclosure (collapsed-by-default secondary sections, an at-a-glance lifecycle/status summary always visible at the top) so the current state and blocking condition are visible without expanding anything.
- Consolidate duplicative information currently repeated across sections into a single authoritative display location.
- Make triage and headless/blocked review sessions inspectable via a new read-only session viewer (output/logs only, no terminal interaction) — extending the Sessions section so all session types (work, triage, review, headless-review) are clickable/openable in some form.
- Update board/list card summaries (`/backlog`, `/backlog/board`) so status/lifecycle indicators are visually consistent with the redesigned detail view (same status vocabulary, same "what's it waiting on" signal), without a full redesign of the board layout itself.

### Out of Scope
- Redesigning the board page's overall layout/columns (only the status/lifecycle indicator consistency on existing cards is in scope).
- Changing the backlog triage/review workflow logic itself (statuses, rework caps, pipeline behavior) — this is a display-layer change only.
- Session-list-level organization/grouping/deletion features — already covered by the separate (already-implemented) `backlog-ux` project.
- The `/unfinished` stuck-item recovery page and its backend logic — already covered by the separate (already-implemented) `backlog-stuck-item-visibility` project.
- Adding a first-class "stuck reason" field to the backend data model — the lifecycle summary should surface derived/inferred state (as `useStuckBacklogItems.ts` already does) rather than requiring a schema change, unless research in Phase 2 finds this is unavoidable.

## Rabbit Holes

- The read-only session viewer for triage/headless-review sessions may require new backend RPC surface to fetch output for `hidden=true` sessions — current session-fetching/streaming may assume interactive sessions. Needs research into `ReadSessionOutput`/`WatchSessions` to confirm hidden sessions' output is already accessible or needs new plumbing.
- "Headless-" and "review-blocked-" prefixed session IDs (`BacklogItemDetail.tsx:1333`) appear to be synthetic/placeholder IDs, not real session IDs — need to confirm whether a real underlying session exists to view, or whether these represent sessions that never actually started (in which case "viewing" them means showing a different kind of diagnostic, not session output).
- Consolidating duplicated info requires an audit of exactly which fields are shown redundantly (e.g. status may appear in header badge, Workflow timeline, and Progress History) — this should be enumerated precisely in Phase 2 research, not assumed.
- Determining the "lifecycle lens" model (what are the canonical stages, and what counts as "waiting on X") needs to reconcile actual backend statuses (`idea/ready/queued/in_progress/review/done/archived`) with derived stuck-state logic — avoid inventing a UI-only status model that drifts from backend truth.

## Alternatives Considered

- Minimal patch (just make triage/review sessions clickable, leave section layout as-is) — rejected per explicit appetite guidance; would likely need revisiting.
- Full page reroute (moving item detail to its own route instead of a panel/drawer) — not requested; current panel/drawer mechanism is assumed to stay, only its internal information architecture changes. To be confirmed/explored in Phase 3 UX design.

## Feasibility Risks

- Read-only session viewer for hidden sessions may uncover that hidden session output isn't currently persisted/retrievable in a form suitable for display (needs Phase 2 research).
- Reworking a 1577-line component's structure risks regressions in the existing manual-review verdict form and other interactive Actions — needs careful test coverage before/after.
- Consolidating "duplicative" info could remove something a future feature depends on if not verified against all consumers of that data first.

## Observability Requirements

Standard request logging sufficient — no new metrics/alerts needed for a personal-tool UI change.

## Risk Control

Not needed — low risk, single-user internal tool, no production rollout process beyond the existing PR/merge flow.

## Open Questions

- What is the exact mechanism for reading hidden/headless session output today (RPC name, whether it already supports non-interactive sessions)? → Phase 2 research.
- What is the precise list of currently-duplicated fields across the panel's 12+ sections? → Phase 2 research / Phase 3 UX design.
- Should the redesigned detail view remain a panel/drawer, or would a different presentation (e.g. tabs, a dedicated page) serve progressive disclosure better? → Phase 3 UX design.

---

## Phase 2 Follow-up: Action Reachability + PR Visibility (2026-10-12)

**Source**: Operator feedback after Phase 1 implementation shipped. Three concrete friction points
reported in use:

### New Problem Statement

Despite the progressive-disclosure redesign, three interaction problems remain after Phase 1:

1. **Actions buried below the fold**: `ActionsSection` (Spawn Session, Approve, etc.) requires
   scrolling past GateBlockingSection, GuidanceRequestPanel, TriageLoadingIndicator,
   TriageReviewPanel, ChatRefinementPanel, PlanningSection, AcCriteriaList, and PlanVerdictBox
   before it is reachable. For a `review`-status item, the primary action ("Approve") is
   the most important thing in the view — it should not require a scroll.

2. **PR link not visible without expansion**: When `item.prUrl` is set and `item.status`
   is `done` or `archived`, `PullRequestSection` does not render (it is status-gated to
   `pr_pending` only) and `VersionControlSection` defaults to collapsed (not in the
   `["in_progress", "review", "pr_pending"]` auto-expand set). The user sees no PR link
   without first expanding a section. For `review` status the VCS auto-expands but the
   PR link is still below the fold.

3. **VersionControl/PullRequest section coordination confusion**: When an item has a PR
   (`item.prUrl` set) but is not `pr_pending`, the user must hunt through sections for
   the PR link. The D4 fix (`showPrLink={false}` in VCS when `pr_pending`) is correct
   but the inverse state — prUrl set, status `done`/`review`/`archived`, no
   `PullRequestSection` rendered — leaves the PR link only in a collapsed VCS section.

4. **Visual flow**: The scroll area ordering deprioritizes decision-making content
   (ActionsSection, PlanVerdictBox) in favor of reference content (description, planning,
   criteria) that rarely changes per review cycle.

### New Success Metrics

- From the sticky header, the user can trigger the primary lifecycle action (triage,
  session start, review approval, mark-done) without scrolling — 0 scrolls to reach the
  primary action button.
- When `item.prUrl` is set, a clickable PR link is visible in the sticky header (or
  immediately adjacent to LifecycleSummary) without expanding any section, regardless of
  `item.status`.
- Section ordering puts ActionsSection + PlanVerdictBox as the first content in the
  scroll area, before any reference/secondary content.

### New Scope

**In Scope (Phase 2 additions)**:
- Add a compact primary-action chip to the sticky header region (inline with LifecycleSummary
  or just below it) that renders the single most-important action for the current lifecycle
  state — not the full ActionsSection, just the primary button.
- Add a compact PR link badge to LifecycleSummary (or sticky header) whenever `item.prUrl`
  is set, regardless of `item.status`. Badge: `PR #<n> →` linking to `item.prUrl`.
- Reorder the scroll area: move ActionsSection + PlanVerdictBox to the top of the scroll
  area (immediately after the sticky header, before GateBlockingSection, TriageReviewPanel,
  etc.), consistent with the principle that decision-making content outranks reference content.
- Expand VersionControlSection by default whenever `item.prUrl` is set (any status), not
  only for `["in_progress", "review", "pr_pending"]` — a PR exists means VCS context is
  relevant.

**Out of Scope**:
- Removing or restructuring the existing ActionsSection in the scroll area — the primary-action
  chip in the header is an *additive* affordance, not a replacement (some statuses have
  multiple relevant actions; the header chip shows the single most-common one, ActionsSection
  shows all of them).
- Changing the D4 `showPrLink` logic in VersionControlSection — the duplication-elimination
  is correct. The PR badge in the header is the primary fix.

### New Constraints

All existing constraints in requirements.md apply. Additional:
- The primary-action chip in the sticky header must not duplicate the full ActionsSection
  UI — it is a compact affordance with a label only (no manual-review form, no Jules
  dispatch dialog trigger, no send-back box) that delegates to the scroll-area ActionsSection
  for anything beyond the single primary action.
- The PR badge must use `GitHubBadge` (already used in PullRequestSection) or equivalent,
  not invent a new PR chip visual language.

---

## Phase 3 Follow-up: Exploratory UX Audit Post Phase 1 + Phase 2 (2026-10-17)

**Source**: Post-implementation exploratory testing request. Phase 1 (progressive disclosure,
LifecycleSummary) and Phase 2 (ActionsSection reorder, PR badge in header, VCS auto-expand)
are fully implemented. A fresh exploratory audit is requested to find remaining confusion
points and produce tested alternative presentations.

### New Problem Statement

After Phase 1 + Phase 2 implementation, `BacklogItemDetail.tsx` has improved progressive
disclosure and action reachability. However, structural issues remain:

1. **AcCriteriaList is always-expanded and outside the CollapsibleGroup** — For items
   with 5+ acceptance criteria, this inserts a long always-visible list between PlanningSection
   and the CollapsibleGroup headers. Users must scroll through all criteria to reach section
   headers like Sessions or Description.

2. **Fifteen sections in the CollapsibleGroup** — Even collapsed, displaying 15 section
   accordions in a vertical list requires users to visually scan a long header stack.
   No grouping cues distinguish "decision-making" sections (Reviewing, PullRequest) from
   "reference" sections (Description, PlanArtifacts) from "audit trail" sections (Sessions,
   WorkflowHistory, ProgressHistory, ActivityLog).

3. **Four overlapping history sections** — Sessions, WorkflowHistory, ProgressHistory, and
   ActivityLog each present historical/audit data in different formats. Users don't have a
   clear mental model for which answers "what happened last time?" or "why did this fail?"

4. **DescriptionSection defaults collapsed** — Item description is the most fundamental
   content for understanding an unfamiliar item, yet it defaults to collapsed.

5. **LifecycleSummary badge accumulation** — For a non-default item (category + custom
   pipeline + skip-planning + rework-cap-override + prUrl set), the header shows 6+ badges.

### New Success Metrics

- A user opening an unfamiliar item's detail panel can identify its lifecycle state AND
  understand what the item is about without expanding more than 1 section.
- The CollapsibleGroup's section list is navigable: users can locate any section in under
  3 seconds of scanning (< 8 visible section headers, or grouped/labeled navigation).
- "What happened last time?" is answerable from a single section or a clearly-labeled
  group without knowing which of Sessions/WorkflowHistory/ProgressHistory/ActivityLog
  to open.
- AcCriteriaList scroll length does not prevent reaching CollapsibleGroup section headers
  without scrolling (criteria list is either collapsible or moved into the group).

### New Scope

**In Scope (Phase 3)**:
- Exploratory audit: enumerate all currently-visible sections per status and identify
  specific confusion points from first-principles use of the live component.
- Design and document 3 alternative layouts (A: flatten+filter, B: status-aware, C: tabs).
- Pick a recommended layout based on complexity-vs-clarity tradeoff.
- Implement the recommended alternative (or the highest-value subset of it).
- Make AcCriteriaList collapsible (or move it into the CollapsibleGroup).
- Group or consolidate the 4 audit/history sections into a single entry point.
- Flip DescriptionSection to default-expanded.
- Write E2E tests covering the changed section ordering and default-expanded states.

**Out of Scope**:
- Changing the LifecycleSummary badge accumulation (separate styling concern).
- Adding new backend data to any section (display-layer changes only).
- ManualOverrideSection visibility/placement — low-frequency power-user tool; leave as-is
  unless the exploratory audit surfaces it as a real friction point.

### New Constraints

All existing Phase 1/Phase 2 constraints apply. Additional:
- Any new section grouping must remain compatible with `useSectionExpandState` persistence
  (expand state per item per section key stored in localStorage).
- Do not introduce nested CollapsibleGroups — the current single Radix Accordion root
  (ADR-027) provides roving-tabindex keyboard nav; nesting would break that contract.
  Use a single flattened group with optional visual separators/headers between groups.

### Open Questions (Phase 3)

- Should Sessions, WorkflowHistory, ProgressHistory, and ActivityLog be merged into
  one collapsible section with internal tab navigation, or remain separate sections
  with a labeled visual grouping line between them? → Phase 3 research.
- Should AcCriteriaList move into the CollapsibleGroup (collapsible) or remain
  always-visible with a "collapse long lists" affordance (e.g. show N, expand to all)?
  → Phase 3 research / dogfooding.
