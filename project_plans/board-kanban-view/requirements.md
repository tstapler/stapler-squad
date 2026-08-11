# Requirements: Board/Kanban View Toggle for Sessions

Source: backlog item `6e048b5b-b4de-4ecc-a96f-fee5b904bd37` ("feat: board/kanban view
toggle for sessions"), migrated from `TylerStaplerAtFanatics/stapler-squad#46`. No
interactive ideation was run (non-interactive triage session) — this document is derived
directly from the item's description plus a scan of the existing session-list codebase.

## Problem

The dashboard currently only offers a flat/grouped list view (`SessionList.tsx` +
`SessionRow.tsx`, grouped via `web-app/src/lib/grouping/strategies.ts`). A list is
efficient for scanning many sessions but doesn't convey workflow state at a glance
(how many are blocked vs. running, where attention is needed). A kanban board view,
organizing sessions into status-based columns, is a common alternative paradigm in
competing agent-orchestration tools (Vibe-Kanban, Agent-Kanban, Dorothy per the item's
competitive scan) and would give users a second, complementary mental model over the
same underlying session data.

## Target User

The solo/small-team developer running many concurrent Claude Code / Aider agent sessions
via this dashboard (this project's only user population today — see `list_workspace_peers`
for the multi-peer case). Specifically: a user who currently has to either scan a long
flat/grouped list or open the separate Review Queue panel to answer "how many sessions
need my attention right now, and of what kind" — the person this feature is *for* is
whoever asks that question more than a few times per session.

## Success Metrics

Added during triad review (2026-08-07) — requirements.md previously had no measurable
success criteria, which the PM-lens triad pass flagged. Since this is a solo-developer
tool with no analytics/experimentation platform, metrics are self-reportable rather than
instrumented:
1. **Adoption**: the user voluntarily keeps Board as their persisted per-workspace default
   (AC9) rather than reverting to List within the first week of availability — a signal
   read informally (no telemetry required), not a hard KPI.
2. **Task success**: a user can answer "how many sessions need my attention" by glancing at
   the Needs Review column's count badge alone, with zero clicks, matching the Problem
   statement's stated gap.
3. **No regression**: List view's existing test suite and manual workflows are unaffected
   (AC11) — a strictly additive feature must not cost anything to the users who never touch
   Board view.

## Risky Assumption

The core bet this feature makes is that a kanban board is a genuinely better mental model
for *this specific* tool's session-management task, not just a pattern borrowed because
competitors (Vibe-Kanban, Agent-Kanban, Dorothy) have it. That assumption is untested
against this app's actual users — the Competitive Context in the source item is a feature
inventory, not user research. If the assumption is wrong, the mitigation is already
structural: Board ships as a strictly additive, reversible-per-session toggle (Non-Goal:
does not replace or regress List view), so a wrong bet costs implementation effort but not
a forced migration — the "Adoption" success metric above is the intended fast feedback
signal for whether the bet paid off.

## Goals

1. Add a "Board" view as an alternative to the existing "List" view, toggleable from
   the dashboard header, without removing or regressing the current list experience.
2. Board view organizes sessions into columns by status by default: Running, Needs
   Review, Paused, Complete (exact column set to be reconciled against actual
   `SessionStatus` enum values during research/planning — see Open Questions).
3. Sessions can be dragged between columns to change their status, where that
   transition is a valid/supported state change on the backend.
4. Board view composes with existing session-list features rather than duplicating or
   forking them:
   - The existing grouping-strategy axis (`GroupingStrategy` — category, tag, branch,
     etc.) can be used as the swimlane axis instead of status.
   - Instant search filters cards across all columns.
   - Bulk select works across columns.
5. The user's last-used view (list vs. board) is remembered per workspace.
6. A keyboard shortcut (`b`) toggles between list and board view.

## Non-Goals

- Custom/user-defined columns beyond the status-based default and the existing
  grouping-strategy axes (e.g. no free-form "add a column" builder).
- Cross-workspace board layouts or shared/team board customization.
- Adding new session-state-machine edges or new backend permission models. Planning
  surfaced one narrow, in-scope backend addition that does *not* violate this Non-Goal:
  `Instance.StopByUser()` (plan.md Phase 0) exposes the drag-into-"Complete" gesture via
  a new `UpdateSession` branch, but it only reaches `Active/Paused/Hibernated → Stopped`,
  an edge already valid in `session/state_machine.go` today — no new state-machine edge,
  no new permission field (reuses `CanPause`). Anything beyond wiring an already-valid
  edge to a drag gesture remains out of scope.

## Acceptance Criteria

1. A "List / Board" toggle control is present in the dashboard header; clicking it (or
   pressing `b` when the dashboard has focus and no text input is focused) switches
   between the two views without a full page reload or loss of current filters.
2. In board view, sessions render as cards grouped into columns by status, matching (as
   closely as the backend's actual `SessionStatus` values allow) Running / Needs Review
   / Paused / Complete.
3. Each column header shows a count badge of the sessions currently in it.
4. Dragging a session card from one column to another triggers the corresponding
   session status/state mutation via the existing session-mutation RPCs — `UpdateSession`
   for status changes, plus `ResolveApproval` (Needs Review → Running) and
   `ResumeHibernatedSession` (Hibernated → Running) for the two transitions that are not
   plain status writes on the backend (see plan.md's Domain Glossary/`ApprovalResolution`
   and Task 3.1.1a) — and the card moves to reflect the new state (optimistically or on
   confirmed response — decided in planning). No new RPC is added other than the
   `StopByUser`-backed `→ Stopped` branch on the existing `UpdateSession` handler.
5. Dragging a card to a column that does not represent a valid state transition for
   that session is rejected (card returns to its original column) with a visible error
   indication.
6. Switching the swimlane axis (via the existing grouping-strategy selector) re-groups
   board columns by that axis instead of status, reusing `GroupingStrategy` from
   `web-app/src/lib/grouping/strategies.ts`.
7. Typing in the existing instant-search box filters cards across all columns
   identically to how it filters rows in list view.
8. Selecting cards via the existing bulk-select mechanism works across columns, and
   bulk actions (e.g. pause, stop) apply to the full cross-column selection.
9. The last-used view mode (list vs. board) persists per workspace (survives a reload)
   and is restored on next visit to that workspace.
10. Board view respects mobile/responsive layout — columns become independently
    horizontally scrollable or stack per the existing mobile+desktop UX requirement
    (see `feedback_mobile_desktop_ux` memory); touch targets and drag interaction must
    have a viable non-drag fallback on touch devices (e.g. a per-card "move to..."
    action) since drag-and-drop UX on mobile is unreliable.
11. Existing list view and its own tests/behavior are unaffected (no regression).
12. Dragging (or using "Move to...") a session into the "Complete" column requires an
    explicit confirmation step before the stop mutation fires — added 2026-08-07 per the
    UX-lens triad review BLOCKER: stopping a session (killing its tmux pane, potentially
    removing its worktree) via a single accidental drag gesture, with
    `legalBoardTransitions["complete"] = []` giving no in-board way to undo it, violates
    error-prevention/user-control expectations for a materially more costly action than
    every other column-to-column move this feature supports.

## Open Questions (for research/planning phases)

- Exact mapping from the item's proposed columns (Running / Needs Review / Paused /
  Complete) to the real `SessionStatus` enum (`session/instance.go`) and any
  approval-queue-derived "needs review" sub-state — this may not be a single backend
  enum value but a derived condition (e.g. active + pending approval).
  the review queue" data source, is board view's "Needs Review" the same computed
  set, or a new query?
- What library (if any) provides drag-and-drop — no `@dnd-kit`/`react-beautiful-dnd`
  equivalent is currently in `web-app/package.json`; research phase must evaluate
  whether to add one dependency (ladder rung 4/6) vs. hand-rolling HTML5 DnD.
- Whether column-change-via-drag maps to an existing RPC (`UpdateSession`,
  `PauseSession`, `ResumeSession`, `StopSession`) per column pair, or requires new
  server-side validation of which drags are legal.
- Persistence mechanism for "last-used view per workspace" — likely same pattern as
  existing per-workspace UI prefs (check for an existing workspace-scoped local
  storage/settings pattern before adding a new one).
