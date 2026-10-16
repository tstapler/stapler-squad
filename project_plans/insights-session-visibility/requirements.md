# Requirements: insights-session-visibility

**Date**: 2026-09-12
**Type**: feature addition (bundled: 1 backend proto extension + 3 frontend UX gaps)
**Backlog item**: `16ed8ad1-19cc-4336-92b4-b87a8a882bb2`

## Problem Statement

The Insights page (`/insights`, `/insights/session-detail`) surfaces per-session token/cost
data via `SessionTokenSummary` (`proto/session/v1/insights.proto:69-96`, verified current as
of commit `474eba13273f`), but four related gaps prevent the user from actually assessing
session efficiency at a glance:

1. Token-type composition (input/output/cache-creation/cache-read) is rendered as plain text
   in `SessionDetailContent.tsx` and collapsed into a single `cacheHitRate` percentage in
   `SessionsTable.tsx` — raw cache numbers only appear in a hover tooltip
   (`SessionsTable.tsx:309`). No visual (bar/stacked-bar) breakdown exists anywhere on the page.
2. `SessionsTable.tsx`'s Fuse.js search covers only `projectPath`/`backlogTitle` plus a
   `primaryModel` dropdown — no tag filter exists, and `SessionTokenSummary` has no `tags`
   field at all (verified: proto has no tags field; `session/instance_tags.go` confirms
   sessions already carry a persisted `Tags []string` via `Instance.GetTags()`/`SetTags()`,
   unused by Insights).
3. Triage/review session role labels (`SessionsTable.tsx:298`, `backlogEntry.sessionRole`)
   are computed via a client-side join against `backlogIndex`, populated by
   `useBacklogSessionIndex()` (`web-app/src/lib/hooks/useBacklogService.ts:1339-1370`) calling
   `BacklogService.GetSessionBacklogIndex`.
   **Correction to the original hypothesis (Phase 2 research, verified independently by 3
   research agents plus direct inspection of `server/services/backlog_service_query.go:533-545`
   and `session/ent_repository_backlog.go:2782-2804`):** this join is NOT scoped to
   currently-open items — `GetAllItemSessionsWithBacklogInfo` does an explicitly unfiltered
   `ItemSession.Query().WithBacklogItem().All(ctx)` scan (comment: "feeds the Insights
   dashboard, which needs the full join across all item sessions"), and archival only flips
   `BacklogItem.status` — it does not touch `ItemSession` rows. So role labels do **not**
   currently disappear on archive/close as originally believed. They **do** disappear
   permanently when a backlog item is hard-**deleted** — `DeleteBacklogItem`
   (`session/ent_repository_backlog.go:1438-1490`) cascades and removes the `ItemSession` rows
   entirely, and that RPC is reachable in the real app
   (`server/services/backlog_service_lifecycle.go:548`). The real, remaining problems this
   project should fix are therefore: (a) role survives archival today but not hard-deletion —
   attaching it to `SessionTokenSummary` at build time is still the right fix for
   deletion-proofing it going forward (a session's summary can carry its role even after the
   source `ItemSession` row is later deleted, if captured once and treated as historical
   fact rather than live-joined), and (b) architecturally, role lives only in a *separate*
   RPC/client-side Map today, so it can't be filtered, sorted, or exported server-side, and
   is subject to a load-order race (session data can render before the second `backlogIndex`
   fetch resolves) — consolidating it onto `SessionTokenSummary` fixes both regardless of the
   archival non-bug. Role, tags, and waste-score data end up consistently co-located and
   filterable in one place, which is what actually serves the user's stated goal of judging
   triage/review session efficiency.
4. The "Waste Score" column header (`SessionsTable.tsx:276`) has no tooltip, unlike sibling
   columns (e.g. cache-hit-rate has a hover title showing raw read/write counts). Users can't
   tell it's a weighted 0-100 "badness" blend (cache-shortfall/token-ceiling/context-size
   penalties; see `session/tokens/findings.go` and ADR-002), not a dollar amount, that higher
   is worse, or what a blank/nil value means (too few turns to evaluate,
   `ComputeWasteScore` returns nil).

These four are bundled into one SDD project because #1-#4 all read/extend the same
`SessionTokenSummary` proto message and the same two frontend components
(`SessionsTable.tsx`, `SessionDetailContent.tsx`) — uncoordinated separate changes would
produce conflicting edits to the same message and files.

## Users / Consumers

- End users: the Insights page's only user today is Tyler himself (personal dev-tooling
  dashboard), assessing which sessions/session-types burn tokens inefficiently.
- No downstream/automated consumers of the Insights UI. `SessionTokenSummary` itself is
  consumed only by `server/services/insights_service.go` (single builder function) and the
  Insights frontend surface (`SessionsTable.tsx`, `SessionDetailContent.tsx`,
  `SessionDetailDrawer.tsx`, `FindingsPanel.tsx`, `InsightsDashboard.tsx`,
  `useInsightsService.ts`, plus their tests and `tests/e2e/pages/InsightsPage.ts`) — verified
  via repo-wide grep, no other feature area depends on this message.

## Success Metrics

- User can visually see, per session (both in the table and in session detail), the
  proportion of input/output/cache-creation/cache-read tokens without reading raw numbers.
- User can filter/search the session list by tag, using the same tag data the main session
  list already uses (no second tagging system introduced).
- A triage/review session's role is a first-class field on `SessionTokenSummary` itself (not
  a separate client-side join), and survives even if its source `ItemSession` row is later
  hard-deleted, so historical triage/review efficiency remains assessable without depending
  on a second RPC's load timing or the backlog item's continued existence. "Filterable" here
  means the field exists on the wire type for any consumer (present or future) to query —
  this PR does not itself ship a role-filter UI control; that's tracked as a deferred
  follow-up in plan.md pending explicit confirmation, since requirements Scope §3 only asked
  for the role *badge*'s source to change, not for a new filter control. Sorting by role is
  likewise not required.
- Every metric column on the Insights page that isn't self-explanatory (Waste Score,
  confirmed missing; others audited during research) has a hover explanation.
- `make ci` / `make ready` pass; `gofmt -w .` applied; existing Insights tests
  (`insights_service_test.go`, `SessionsTable.test.tsx`, `SessionDetailContent.test.tsx`,
  etc.) still pass, plus new tests for the added behavior.

## Constraints

- No hard deadline. Personal-project cadence.
- `SessionTokenSummary` is a shared proto message — verified single Go consumer
  (`insights_service.go`) and Insights-scoped TS consumers only, so field additions are
  additive/backward-compatible (new fields appended after `activity_type = 20`), but the
  plan must re-confirm no other consumer appears in Phase 2 research before finalizing
  field numbers.
- Tags must reuse the existing `Instance.Tags`/`TagManager` mechanism
  (`session/instance_tags.go`, `docs/reference/tag-organization.md`) — not a new tagging
  system.
- Session role must come from the session's own persisted `Role` field
  (`session.SessionRoleWork/Triage/Review/JulesWork`, `session/backlog.go`), sourced at
  summary-build time, not via a live join against currently-open backlog items.
- Repo conventions apply: Conventional Commits, `make ci`/`make ready` before shipping each
  PR, PRs ready-for-review (not draft) per this repo's override, `--repo owner/repo` on any
  `gh pr merge`, `gofmt -w .`, dupl/jscpd gates, pnpm (not npm) in `web-app/`.
- If proto/backend changes (tags, session_role) and frontend changes (viz, filter UI, role
  display, tooltips) are large enough to review independently, split into 2+ PRs — Phase 3
  planning decides this explicitly with reasoning, not forced into one PR.

## Scope

### In Scope

1. Frontend token-type visual breakdown: compact per-row visual in `SessionsTable.tsx` +
   fuller breakdown in `SessionDetailContent.tsx`, using existing `SessionTokenSummary`
   fields (no proto change needed for this part). Follow the `dataviz` skill for any chart.
2. Backend: add a `tags` field to `SessionTokenSummary`, populated from the session's
   persisted `Instance.Tags` in `buildSessionSummary()`. Frontend: tag chips/multi-select
   filter UI in `SessionsTable.tsx`, and extend the existing Fuse.js search to match tags.
3. Backend: add a `session_role` (or `session_type`) field to `SessionTokenSummary`,
   populated at summary-build time from the session's actual persisted role — surviving
   backlog item closure/archival. Frontend: use this field instead of (or in addition to,
   for a transition) the `backlogIndex` join for the role badge, so role display no longer
   depends on the parent backlog item still being open.
4. Frontend: tooltip on the "Waste Score" column header explaining it's a weighted blend
   (not a dollar sum), higher = worse, and what a blank/nil value means. Audit other
   Insights fields for missing explanations during research/planning and add tooltips only
   where genuinely missing — no redesign of already-explained fields.

### Out of Scope

- Backend changes to how Waste Score, cache hit rate, or any other existing metric is
  *computed* — formula is confirmed correct and already documented (ADR-002); this project
  only adds visualization/filtering/attribution/explanation, not new metrics.
- A general-purpose tagging system — reuse what exists.
- Full page redesign of Insights — additive UI changes only (chips, tooltips, a breakdown
  visual), not a layout overhaul.
- Changing backlog item lifecycle/archival behavior itself.
- Any change to `isOrphan` filtering semantics (item #3 is about role attribution, not
  orphan detection).

## Open Questions

- Exact mechanism for looking up a session's persisted `Role` at summary-build time when
  the backlog item may be closed/archived (ent query shape, whether `ItemSession` rows
  survive archival, whether a new indexed lookup by `SessionUUID` across all backlog items —
  open or closed — is needed) — resolve in Phase 2 research.
- Whether `session_role` should carry the raw `session.SessionRoleXxx` string or a richer
  enum matching proto conventions elsewhere (`ActivityType`) — resolve in Phase 3 planning.
- Chart type for the token-type breakdown (stacked bar vs. donut vs. sparkline-style) —
  resolve per `dataviz` skill guidance in Phase 3.
- Whether this splits into a backend-foundation PR (tags + session_role fields) followed by
  frontend PR(s) — Phase 3 planning decides explicitly.
