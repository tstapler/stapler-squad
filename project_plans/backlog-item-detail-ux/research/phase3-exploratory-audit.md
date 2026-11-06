# Phase 3 Research: Exploratory UX Audit of BacklogItemDetail
**Date**: 2026-10-17
**Status**: Complete — verified against `BacklogItemDetail.tsx` + all `detail/` sub-components
**Phase 1 + Phase 2**: Fully implemented (confirmed from source)

---

## 1. Current Section Inventory (post Phase 1 + Phase 2)

### Sticky Header (always visible, does not scroll)

| Region | Content |
|---|---|
| Title row | `h2` title + priority badge + created/updated dates |
| Copy row | Copy ID button + Copy Link button |
| Actions row | ConnectionIndicator + Edit button + Close button |
| ItemClaimBanner | Shown only when `item.externalUrl` is set |
| LifecycleSummary | StageTracker → StuckBlock (if stuck) → category badge → pipeline badge → AutomationProfileBadges (up to 4 badges) → ReworkCapBadge → PrBadge → LivenessLine |
| ItemBudgetWarning | Shown only when cost threshold configured and crossed |

### Scroll Area (ordered as rendered in source, lines ~1566–1908)

| # | Section | Visibility gate | Default expand |
|---|---|---|---|
| 1 | PlanVerdictBox | ready/queued, or plan exists | N/A (not collapsible) |
| 2 | ActionsSection | Always | N/A (not collapsible) |
| 3 | GateBlockingSection | Always | N/A (not collapsible) |
| 4 | DependenciesSection | Always | N/A (not collapsible) |
| 5 | GuidanceRequestPanel | Not shown while TriageReviewPanel is visible | N/A (not collapsible) |
| 6 | Error banner | When action error is present | N/A |
| 7 | TriageLoadingIndicator | idea/ready + triage running | N/A |
| 8 | TriageReviewPanel | idea + triage completed + has result | N/A |
| 9 | ChatRefinementPanel | Same gate as TriageReviewPanel | N/A |
| 10 | PlanningSection | Always (renders empty for idea with no result) | N/A (not collapsible) |
| 11 | Triage-failed banner | idea/queued + triage failed + no approved plan | N/A |
| 12 | **AcCriteriaList** | **Always** | **Always expanded (not in CollapsibleGroup)** |
| 13+ | **CollapsibleGroup** (15 sections) | Various gates | See below |

### CollapsibleGroup sections (ordered as rendered)

| # | Section | Gate | Default expanded |
|---|---|---|---|
| 1 | ReviewingSection | `item.status === "review"` | Yes (review status) |
| 2 | LastReviewResultSection | `item.status !== "review" && item.gateVerdict` | No |
| 3 | PullRequestSection | `item.status === "pr_pending"` | Yes (pr_pending) |
| 4 | SourceSection | `item.externalUrl` set | No |
| 5 | DescriptionSection | Always | **No (collapsed by default)** |
| 6 | PlanArtifactsSection | `item.planArtifactsPath` set | No |
| 7 | VersionControlSection | `widgetData !== null` | Yes for in_progress/review/pr_pending or prUrl set |
| 8 | AutonomousHealthStrip | Always (renders null if no data) | No |
| 9 | ItemStageCostTable | Always (loading/error/empty states) | No |
| 10 | SessionsSection | Always | No |
| 11 | WorkflowHistorySection | Always | No |
| 12 | ProgressHistorySection | Always | No |
| 13 | ActivityLogSection | Always | No |
| 14 | NotesSection | Always | No |
| 15 | ManualOverrideSection | Always | No |

---

## 2. Confusion Points Identified

### P3-C1: AcCriteriaList is always-expanded and blocks CollapsibleGroup

AcCriteriaList renders outside the CollapsibleGroup, always-expanded, with no collapse
affordance. For an item with 8+ acceptance criteria (common for well-defined items), the
rendered list is 200+ px tall. This forces users to scroll through all criteria before
reaching the CollapsibleGroup section headers.

- **Frequency**: Every item open with more than ~4 criteria.
- **Impact**: Sessions, Description, Notes — sections a user often wants to navigate to
  directly — are invisible without scrolling past the full criteria list.
- **Current justification in code**: AcCriteriaList was positioned here in Phase 1 as
  "always-visible primary content" (plan.md Task 3.1.2 rationale). The criteria count
  in the header `Acceptance Criteria (N/M)` was designed to show completion progress at
  a glance. Neither rationale requires it to be non-collapsible.

### P3-C2: 15 CollapsibleGroup section headers — no grouping cues

Even collapsed, the CollapsibleGroup shows 15 accordion headers in a straight vertical
list. There is no visual separator, group label, or hierarchy distinguishing:

- **Decision-making** sections (ReviewingSection, LastReviewResult, PullRequestSection)
- **Reference** sections (Source, Description, PlanArtifacts)
- **Monitoring** sections (VersionControl, AutonomousHealthStrip, CostTable)
- **Audit trail** sections (Sessions, WorkflowHistory, ProgressHistory, ActivityLog)
- **Admin** sections (Notes, ManualOverride)

A user looking for "how much did this cost?" must scan 9 section headers before reaching
ItemStageCostTable (position 9 in the group).

### P3-C3: Four overlapping audit/history sections

Sessions, WorkflowHistory, ProgressHistory, and ActivityLog each present history data:

| Section | What it shows |
|---|---|
| SessionsSection | All linked sessions (work, triage, review, headless) with status, pipeline, elapsed time, delete/steer affordances |
| WorkflowHistorySection | Workflow stage transitions (triage completed, plan approved, session started, review completed, etc.) as a timeline |
| ProgressHistorySection | High-level status transitions (idea→ready, ready→queued, etc.) as a simpler timeline |
| ActivityLogSection | Fine-grained event log (every backend event that touched this item) |

A user asking "why did this item fail review last time?" must guess which section to
open. In practice: SessionsSection shows the session that ran, WorkflowHistorySection
shows the review outcome, ActivityLogSection shows the precise event sequence. There
is no single entry point.

### P3-C4: DescriptionSection defaults collapsed

`DescriptionSection` is always at position 5 in the CollapsibleGroup and defaults
collapsed. For a user opening an item they haven't seen before, the description —
the fundamental statement of what the item is — requires an explicit click to read.
All other always-visible primary content (PlanVerdictBox, ActionsSection, PlanningSection,
AcCriteriaList) becomes visible before the item description.

**Impact**: The ordering implies "what state is this item in?" and "what can I do?" are
more important than "what is this item?" This is the opposite of a first-time-open
mental model.

### P3-C5: PlanningSection appears before AcCriteriaList

PlanningSection renders the AI-generated implementation plan for items past `idea` status.
For a `review`-status item, the plan content can be very long (full task list). This
content appears before AcCriteriaList, which shows completion progress. A user in review
mode most wants to check "which criteria are done?" — but must scroll through the full
plan first.

### P3-C6: Triage-flow sections create dead space for idea items

For an `idea`-status item with no triage result yet, the scroll area renders:
1. PlanVerdictBox (renders null for idea with no plan)
2. ActionsSection (shows "Trigger Triage" button — ✓ at top)
3. GateBlockingSection (renders null for fresh idea)
4. DependenciesSection (renders null for fresh idea)
5. GuidanceRequestPanel (may render null)
6. TriageLoadingIndicator (only if triage is running)
7. PlanningSection (renders empty/minimal for idea with no result)

Items 3–7 are all empty or null for a fresh `idea` item, creating ~100–200 px of invisible
dead space below the (correctly positioned) ActionsSection before the AcCriteriaList.

### P3-C7: LifecycleSummary badge accumulation for configured items

A non-default item with category + custom pipeline + skip-planning + rework cap + prUrl
set renders in LifecycleSummary: StageTracker → category badge → pipeline badge →
skip-planning badge → rework cap badge → PR badge → LivenessLine = 5 distinct small
badges in a single row. This is visually dense and may obscure the StageTracker (the
most important signal) in favor of configuration metadata.

---

## 3. What Works Well (Do Not Change)

- **StageTracker + BlockerChip** in the sticky header: the single most effective
  addition from Phase 1. Always-visible, correctly models lifecycle state.
- **ActionsSection at top of scroll area**: Phase 2 reorder. Zero-scroll to primary action.
- **PrBadge in LifecycleSummary**: Phase 2 addition. Works correctly for done/archived
  items; correctly suppressed for pr_pending.
- **VersionControlSection auto-expand with prUrl**: Phase 2. Correct behavior.
- **CollapsibleGroup keyboard nav** (ADR-027 Radix Accordion): roving tabindex works
  correctly across all 15 headers; must be preserved.
- **PlanVerdictBox + ActionsSection co-location**: approve_plan (Actions) and reject_plan
  (PlanVerdictBox) are adjacent — preserves the Phase 1 rationale.

---

## 4. Alternative Layouts

### Alternative A: "Consolidate + Collapsible Criteria" (recommended)

Minimal structural change. Highest confidence, lowest regression risk.

Changes:
1. Move AcCriteriaList INTO the CollapsibleGroup (position 3, after PlanningSection
   always-visible, so the count/progress is accessible without scroll but the full list
   is expandable). Default expanded for `review` and `pr_pending` status (the states where
   criteria completion matters most), collapsed otherwise.
2. Add visual group separators in the CollapsibleGroup between:
   - "Decision" group: ReviewingSection, LastReviewResult, PullRequestSection
   - "Reference" group: Source, Description, PlanArtifacts, AcCriteriaList
   - "Monitoring" group: VersionControl, AutonomousHealthStrip, CostTable
   - "Audit Trail" group: Sessions, WorkflowHistory, ProgressHistory, ActivityLog
   - "Admin" group: Notes, ManualOverride
3. Flip DescriptionSection to default-expanded.
4. Reorder AcCriteriaList to appear after DescriptionSection in the Reference group
   (description before criteria is a natural reading order: "what is this?" before
   "what does it need to achieve?").

What doesn't change: single CollapsibleGroup root (ADR-027 preserved), no new component
types, no new RPCs, CollapsibleGroup key names unchanged.

### Alternative B: "Status-Aware Top Zone"

More invasive. Render a small "now" zone above the CollapsibleGroup that is fully
status-dependent:

- idea: show TriageReviewPanel (if available) or "Trigger Triage" prompt
- ready: show PlanningSection summary + AcCriteriaList (short form, N/M count + 3 items)
- in_progress: show VersionControl widget inline (no collapsible)
- review: show ReviewingSection inline (always visible, not collapsible)
- pr_pending: show PullRequestSection inline
- done/archived: show VersionControl widget + PR link

This reduces "empty section" noise for given statuses but requires per-status rendering
logic that may be fragile as new statuses are added. Higher implementation complexity.

### Alternative C: "Tab Navigation"

Split the panel into 4 tabs: Overview | Plan | Sessions | History.
- Overview: LifecycleSummary + Actions + Gates + Description
- Plan: AcCriteriaList + PlanningSection + PlanArtifacts + Source
- Sessions: SessionsSection with per-session detail
- History: WorkflowHistory + ProgressHistory + ActivityLog + CostTable

Cleanest information architecture. Requires a new tab navigation component, changes
the user's spatial memory of where sections live, and requires careful keyboard nav
integration with the existing Radix Accordion within each tab. Highest regression risk.

---

## 5. Recommendation

**Alternative A** is the right starting point for Phase 3 implementation:

1. It directly addresses the three highest-friction points (C1, C2, C4) with minimal
   structural change.
2. It preserves the ADR-027 single CollapsibleGroup root.
3. Visual group separators/labels can be added as pure CSS + non-interactive `div`
   elements between sections without breaking the Radix Accordion's keyboard contract.
4. Moving AcCriteriaList into the CollapsibleGroup is a straightforward refactor —
   AcCriteriaList already uses `data-testid="ac-criteria-list"` and its tests are
   testid-based, so moving it inside the group doesn't break existing tests.

**Open question deferred to implementation**: Whether the "Audit Trail" group (Sessions,
WorkflowHistory, ProgressHistory, ActivityLog) should be merged into one section with
internal tabs or left as 4 separate sections with a group label. The group-label approach
is safer (zero API/behavior changes, just visual grouping); internal tabs require a new
component. Start with group labels; consider tab consolidation in a Phase 3.5 if
dogfooding shows the group label alone doesn't reduce confusion.
