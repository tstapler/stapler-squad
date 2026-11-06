# Phase 2 Research: Action Reachability + PR Visibility
**Date**: 2026-10-12

## 1. Current scroll-area ordering audit

Current scroll area content order (verified against `BacklogItemDetail.tsx` lines 1563–1899):

1. GateBlockingSection
2. GuidanceRequestPanel
3. Inline action error banner
4. TriageLoadingIndicator (status-gated: idea/ready with running triage)
5. TriageReviewPanel (status-gated: idea + triage completed)
6. ChatRefinementPanel (same gate as TriageReviewPanel)
7. PlanningSection (always-rendered when item is past idea)
8. TriageErrorBanner (status-gated: failed triage)
9. ReviewChangesModal / BacklogFileBrowserModal / JulesDispatchDialog (portals, not in flow)
10. AcCriteriaList (always-rendered)
11. PlanVerdictBox (status-gated: ready/queued or plan exists)
12. **ActionsSection** (always-rendered, primary actions here)
13. CollapsibleGroup: Reviewing → LastReviewResult → PullRequest → Source → Description →
    PlanArtifacts → VersionControl → AutonomousHealthStrip → ItemStageCostTable → Sessions →
    WorkflowHistory → ProgressHistory → ActivityLog → Notes

**Finding**: For a `review`-status item (the most action-intensive state), the scroll path to
reach `ActionsSection` passes through: GateBlockingSection, GuidanceRequestPanel, PlanningSection
(may be expanded showing full triage content), AcCriteriaList (all criteria), PlanVerdictBox.
This is a significant scroll for any item with substantive planning content.

**Existing justification in plan.md** (Task 3.1.3b comment): ActionsSection was deliberately
positioned before the CollapsibleGroup to "stay reachable without expanding anything" — but
"reachable without expanding" and "reachable without scrolling" are different bars. The plan met
the first but not the second.

## 2. Sticky header content audit

Current sticky header (`BacklogItemDetail.tsx:1442–1561`):

- `h2` title
- Priority badge + created/updated dates
- Copy ID / Copy Link buttons
- ItemClaimBanner (if externalUrl)
- `div.headerActions`: ConnectionIndicator, Edit button, Close button
- LifecycleSummary (StageTracker + BlockerChip + LivenessLine + pipeline badge + rework cap badge)
- ItemBudgetWarning (cost threshold)

**Missing**: No primary action button. No PR link.

**Space available**: LifecycleSummary's row layout (flex-row in `LifecycleSummary.css.ts`) has
existing precedent for additive badges (pipeline, rework cap). A compact PR badge slots naturally
after the StageTracker row. A primary-action button slots into the `headerActions` row alongside
Edit/Close, or as a full-width button immediately below LifecycleSummary.

## 3. Primary action mapping by status

Derived from `lib/backlog/itemActions.ts` + `ActionsSection.tsx` inspection:

| status | primary action | button label (ActionsSection today) |
|---|---|---|
| `idea` | trigger triage | "Trigger Triage" |
| `ready` | spawn session | "Start Session" |
| `queued` | none (auto-starts) | — (shows "at capacity" message) |
| `in_progress` | none routinely | — (restart shown only if stale) |
| `review` | approve or send back | "Approve" / "Request Changes" |
| `pr_pending` | mark done | "Mark Done" |
| `done` | none | — |
| `archived` | unarchive | "Unarchive" |

**Finding**: 4 of 8 statuses have a single unambiguous primary action that would benefit from
zero-scroll reachability: `idea`, `ready`, `review`, `pr_pending`. The `queued`/`in_progress`/`done`
cases have no primary single action, so a header chip can be omitted for those without loss.

**Constraint**: `review` has TWO primary actions (Approve + Request Changes / Re-review). The
header chip must pick one or show both compactly. The safer choice: show only the positive-path
action ("Approve") in the header — the send-back / re-review path is the deliberate dissent path
and can tolerate a scroll. Alternatively, show "Review Actions ↓" as a scroll-anchor link rather
than a direct action button.

## 4. PR link placement analysis

Relevant code:
- `PullRequestSection`: renders only when `item.status === "pr_pending"` — always expanded.
  Contains `GitHubBadge` (prNumber + prUrl).
- `VersionControlSection`: renders when `widgetData !== null`. `showPrLink = item.status !== "pr_pending"`.
  Auto-expands for `in_progress`, `review`, `pr_pending`.
- `LifecycleSummary`: no PR link today.

**Status × PR link visibility table** (current state):

| status | prUrl set | PullRequestSection rendered? | VCS auto-expands? | PR visible without user action? |
|---|---|---|---|---|
| `idea` | no | No | No | N/A |
| `ready` | no | No | No | N/A |
| `queued` | no | No | No | N/A |
| `in_progress` | rare | No | Yes | Yes (in VCS, below fold) |
| `review` | common | No | Yes | Yes (in VCS, below fold) |
| `pr_pending` | yes | Yes (always expanded) | Yes | Yes (PullRequest section) |
| `done` | yes | No | No | **No — must expand VCS manually** |
| `archived` | sometimes | No | No | **No — must expand VCS manually** |

**Finding**: The gap is primarily `done` and `archived` status with a PR. These are the "item is
finished" states — the exact moment when the user most wants to quickly confirm the PR was merged
or link someone to the PR. A collapsed VCS section is the wrong default here.

**Fix option A (header badge)**: Add `item.prUrl && <GitHubBadge prNumber={item.prNumber} prUrl={item.prUrl} />` to LifecycleSummary. Renders whenever prUrl exists. Zero-click PR access for any status. The badge is identical to what PullRequestSection already uses — no new visual language.

**Fix option B (VCS expand rule)**: Change the auto-expand condition from
`["in_progress", "review", "pr_pending"].includes(item.status)` to also include
`!!item.prUrl` (any status where prUrl is set). For `done`/`archived` with prUrl, VCS
auto-expands so the PR link is visible on first render. Simpler, but still below the fold.

**Verdict**: Fix option A (header badge) is strictly better — it's zero-scroll *and* zero-click,
and it reuses `GitHubBadge` which is already imported in `BacklogItemDetail.tsx`'s dependency graph.
Fix option B can be applied as a complementary improvement (expand VCS when prUrl is set
regardless of status) but option A is the primary fix.

## 5. Section reordering — moving ActionsSection up

The plan's current ordering rationale: ActionsSection was positioned after PlanVerdictBox
so that `approve_plan` (in ActionsSection) and `reject_plan` (in PlanVerdictBox) are both
reachable without any collapsible expansion — they're co-located in the visual flow for
`ready`/`queued` items. This is a legitimate UX reason to keep PlanVerdictBox just above
ActionsSection.

**Proposed reorder**: Move **both** PlanVerdictBox and ActionsSection to the TOP of the
scroll area (positions 1–2), before GateBlockingSection, GuidanceRequestPanel, and everything
else. Rationale:

1. GateBlockingSection answers "what is blocking the next stage" — useful context but not
   an action item itself.
2. GuidanceRequestPanel shows open guidance threads — important but navigational, not
   decision-making.
3. TriageReviewPanel / ChatRefinementPanel are relevant only for `idea` status — when they
   render, ActionsSection (for idea) has minimal content anyway.
4. AcCriteriaList is reference content — useful for context while approving but not itself
   an action target.

The move keeps PlanVerdictBox immediately before ActionsSection (preserving the `approve_plan`/
`reject_plan` co-location rationale) while making both zero-scroll-reachable.

**Risk**: TriageReviewPanel renders for `idea` status, and it currently appears before ActionsSection.
Moving ActionsSection to the top means it appears BEFORE TriageReviewPanel for `idea` items.
For `idea` status, ActionsSection's primary action is "Trigger Triage" — if triage is running,
`TriageLoadingIndicator` shows there too. This ordering is acceptable (action first, then
triage results below) — the user doesn't need to see triage output before deciding to trigger
triage.

## 6. Compatibility with existing tests

The following test files assert on ActionsSection's position relative to other elements:
- `BacklogItemDetail.test.tsx` — queries by testid, not DOM order; reordering is safe
- `BacklogItemDetail.sendBackFeedback.test.tsx` — same
- `BacklogItemDetail.shipPR.test.tsx` — same

No test asserts a specific vertical ordering of sections (confirmed by grepping for
`previousSibling`/`nextSibling`/`insertBefore`/`nth-child`). Reordering is DOM-order only and
does not break any testid-based assertions.
