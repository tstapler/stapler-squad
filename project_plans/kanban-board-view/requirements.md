# Requirements: kanban-board-view

**Date**: 2026-08-06
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement
`SessionList.tsx` (the dashboard's only view, `web-app/src/components/sessions/SessionList.tsx`) presents sessions as a single flat/grouped list. Users managing many concurrent sessions have no way to see workflow state at a glance — how many sessions are blocked on review, running, paused, or done — without scanning the whole list. Migrated from upstream issue [TylerStaplerAtFanatics/stapler-squad#46](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/46).

## Baseline
Today, status visibility comes from the "Status" grouping strategy (one of 8 in `GroupingStrategy`, see `.claude/docs/tag-organization.md`) or the separate Review Queue panel (`ReviewQueuePanel.tsx` / `ReviewQueueContext.tsx`) for approval-pending sessions. There is no single-screen "board" view, and no drag-and-drop way to change a session's state — state changes happen through per-session action buttons (pause/resume/stop) or the review-approval flow.

## Users / Consumers
Existing web-app dashboard users (solo dev + any workspace peers, see `list_workspace_peers`) managing multiple concurrent AI agent sessions across tmux/worktrees.

## Success Metrics
- A user can toggle to a board view and identify, without scrolling or filtering, how many sessions are in each of Running / Needs Review / Paused / Complete.
- Existing list-view capabilities (search, tag grouping, bulk select) are not degraded or duplicated with divergent behavior when board view ships.
- View choice (list vs. board) persists across reloads per workspace, so users don't re-toggle every session.

## Appetite
Medium (1–2 weeks) — new view mode alongside a 1601-line existing component, plus drag-and-drop state-change wiring through existing session-control RPCs (pause/resume/stop), not new RPCs.

## Constraints
- Must reuse existing `SessionService` RPCs for state transitions (`pause_session`, `resume_session`, `stop_session` equivalents) — no new backend session-state RPC.
- Must follow `.claude/rules/css-architecture.md` (vanilla-extract, no new CSS Modules) and `.claude/rules/e2e-test-conventions.md` for any new e2e spec.
- `SESSION_STATUS_NEEDS_APPROVAL` is deprecated in `proto/session/v1/types.proto` (types.proto:341) — "Needs Review" is NOT a first-class `SessionStatus` value today; it's derived from the review-queue/approvals subsystem (`ReviewQueueContext`, `useApprovalsContext`). The board's "Needs Review" column must be defined against that derived state, not a proto enum value.

## Non-functional Requirements
- **Performance SLO**: board render must stay responsive with the same session counts the virtualized list already handles (`SessionList.tsx` uses `react-virtuoso`/`@tanstack/react-virtual`); board columns should be virtualized or windowed if a column can grow unbounded.
- **Scalability**: not applicable beyond current single-workspace session counts.
- **Security classification**: internal (local dashboard, no new external surface).
- **Data residency**: not applicable.

## Scope
### In Scope
- "List / Board" view toggle in the dashboard header, keyboard shortcut `b`.
- Default board columns: Running, Needs Review, Paused, Complete, organized as swimlanes.
- Drag-and-drop a session card between columns to trigger the corresponding state-change action (e.g., drag to Paused → pause_session).
- Board view can switch its swimlane axis to any existing `GroupingStrategy` (tag, category, etc.), reusing `groupSessions()`.
- Instant search filters cards across all columns (reuse existing search state/logic from `SessionList.tsx`).
- Bulk select works across columns (reuse `BulkActions.tsx`).
- Persist last-used view (list vs. board) per workspace.

### Out of Scope
- New backend RPCs or `SessionStatus` enum values (e.g., promoting "needs review" to a real status) — this migrates existing states into a board layout, it does not redesign the state model.
- Customizable/user-defined columns beyond the default four + existing grouping strategies.
- Mobile-specific board interaction redesign beyond existing responsive patterns (touch drag-and-drop is a rabbit hole — see below).
- Cross-workspace board views.

## Rabbit Holes
- **Touch/mobile drag-and-drop**: `.claude/CLAUDE.md`'s mobile+desktop UX requirement applies, but real drag-and-drop on touch screens (long-press-to-drag, scroll-vs-drag conflict) is a known deep rabbit hole. Phase 3 planning must explicitly decide: full touch drag support, tap-to-move fallback, or board view desktop-only for v1.
- **Column ↔ status mapping ambiguity**: sessions can be simultaneously "Running" and "has a pending approval" (needs review is derived, not exclusive with active). Planning must define precedence rules for which single column a session lands in.
- **Drag-and-drop failure/rollback UX**: what the card does if the underlying pause/resume/stop RPC fails mid-drag (snap back, error toast, optimistic vs. pessimistic update) — easy to underscope.
- **Virtualization inside drag-and-drop columns**: combining `react-virtuoso` windowing with a drag-and-drop library is non-trivial; a naively unvirtualized board could regress performance on large session counts.

## Alternatives Considered
- Extend the existing "Status" `GroupingStrategy` view with collapsible sections instead of a new board — rejected in the original issue in favor of a dedicated swimlane/kanban paradigm matching competitor tools (Vibe-Kanban, Agent-Kanban, Dorothy).
- Read-only board (no drag-and-drop, click to open a status-change menu) as a smaller first cut — flagged as a scope option for Phase 3 planning to size against the Medium appetite.

## Feasibility Risks
- No drag-and-drop library currently in `web-app/package.json` (to be confirmed in Phase 2 research) — introducing one is a new dependency decision (`code-architecture-best-practices` / build-vs-buy applies).
- `SessionList.tsx` is already 1601 lines; bolting board rendering into the same file risks further bloating a hotspot — Phase 3 planning should default to a sibling `SessionBoard.tsx` component sharing hooks/state via extraction, not inline branching.

## Observability Requirements
Standard client-side error logging on failed drag-drop state-change RPC calls (existing session-action error handling pattern in `SessionList.tsx`). No new metrics/alerts — this is a client UX feature with no new backend surface.

## Risk Control
Not needed — low risk, additive UI feature behind a view toggle; no feature flag required since falling back to list view is always one click/keypress away. No schema or RPC changes to roll back.

## Open Questions
- Should "Needs Review" column count include sessions with *any* pending approval, or only sessions whose primary session status computation currently surfaces as blocked? (Feeds Phase 2 architecture research into `ReviewQueueContext`/`useApprovalsContext`.)
- Which drag-and-drop library (if any) best fits the existing `react-virtuoso` virtualization already in use? (Feeds Phase 2 build-vs-buy research.)
- Desktop-only v1 vs. touch-drag v1 — resolved in Phase 3 planning per the Rabbit Holes note above, not before.
