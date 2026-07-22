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
