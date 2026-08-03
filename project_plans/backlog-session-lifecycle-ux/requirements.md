# Requirements: backlog-session-lifecycle-ux

**Date**: 2026-08-01
**Type**: feature addition (frontend UX surfacing + backend audit-trail extension)
**Complexity**: 3 — system design (UI wiring across multiple widgets + a new backend audit-trail table)

## Problem Statement

Users cannot tell, at a glance, what has actually been *tried* on a backlog item or why a session ended the way it did. Two categories of gap, confirmed by code research:

1. **Data that exists but is not surfaced.** `ItemSession.end_reason` (`shutdown`/`timeout`/`process_error`/`claude_not_found`/`other`) is persisted but rendered nowhere in `web-app/`. `Session.pause_reason` (`manual`/`auto:inactivity`/`auto:session_limit`/`auto:resource`) is persisted and has a formatter, but is only a hover-tooltip on `SessionCard.tsx`, not an always-visible badge, and `SessionList.tsx` has no filter/grouping by it. `BacklogStuckState.remediation_attempts`/`next_remediation_at`/`context` are fully modeled but only rendered on the `/unfinished` stuck-items page — invisible from the normal board card (`BacklogItemCard.tsx`) or the item detail's `SessionsSection.tsx`.
2. **Data that does not exist yet.** There is no structured respawn-event audit trail. `AutoRespawnReview`/`AutoRespawnAutonomousWork`/`AutoRespawnTriage`/`RemediateStaleWorkSession` (`server/services/backlog_service_triage.go`) only write to `log.InfoLog`, never to a DB row. Nothing links "session N was (re)spawned because of reason X" as a queryable timeline distinguishable from "this session stalled with no respawn attempted." `BacklogStuckState`'s resolve-in-place model (ADR-referenced) intentionally does not retain per-episode history, so there is no way today to answer "how many times, and why, has this item's work been respawned?" without reading raw logs.

## Baseline

Today a user watching the backlog board or an item's session list sees session status chips and, if they hover, a pause-reason tooltip. To learn *why* a session ended, whether it was auto-respawned, how many times, or whether an item is quietly backing off after repeated remediation attempts, they must open `/unfinished` (only surfaces already-detected "stuck" items) or read server logs directly — the same gap previously documented in `backlog-stuck-item-visibility` and partially addressed by `backlog-item-detail-ux`, both already shipped. This item is the next increment: wire already-modeled data into the widgets that don't show it yet, and add the one piece of data (a respawn audit trail) that doesn't exist at all.

## Users / Consumers

Solo user (Tyler), stapler-squad web UI at `localhost:8543`, desktop and mobile.

## Success Metrics

- From the board card and the item detail's Sessions section, a user can see, without opening `/unfinished` or reading logs: whether a session ended cleanly, was paused (and why), or errored — and whether the item has been auto-remediated/respawned, how many times, and why.
- A user can distinguish "actively being retried (respawned N times, backing off)" from "stalled with no further automatic action" from the widgets alone.
- Every respawn triggered by `AutoRespawnReview`/`AutoRespawnAutonomousWork`/`AutoRespawnTriage`/`RemediateStaleWorkSession` is recorded as a durable, queryable event (reason, timestamp, triggering session, resulting session) — not just a log line.
- New/changed RPCs and UI surfaces are registered per `docs/registry/features/` (`make registry-generate` shows no net-new coverage gap).

## Appetite

Medium (1–3 weeks) — this is UI wiring of already-modeled fields (majority of scope) plus one small, additive schema change (a respawn-events table) with no changes to existing remediation/respawn *logic*, only to what gets recorded about it.

**Fallback increment**: surfacing already-persisted fields (`end_reason`, `pause_reason`, `remediation_attempts`/`context`) on the board card and Sessions section is independently shippable and delivers most of the user-visible value even if the new respawn-event audit trail (net-new schema) slips.

## Constraints

- All new/modified CSS must use vanilla-extract (`.css.ts`) per `.claude/rules/css-architecture.md`.
- New/changed RPCs and frontend features must be reflected in `docs/registry/features/` per `.claude/rules/feature-registry.md`.
- Any ent schema change must use `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` per `.claude/rules/ent-schema-generation.md`.
- Must work on both mobile and desktop (touch targets, responsive layout) per standing UX requirement for this codebase.
- Do not change existing remediation/respawn/rework-cap *decision logic* (when/whether to respawn) — this item is observability-only, same boundary `backlog-stuck-item-visibility` drew.

## Non-functional Requirements

- **Performance SLO**: not specified — low-traffic, single-user internal tool; existing polling cadence is an acceptable baseline.
- **Scalability**: not applicable (single user, backlog size in the tens of items, respawn events per item in the tens).
- **Security classification**: internal.
- **Data residency**: none.

## Scope

### In Scope
- Surface `ItemSession.end_reason` in `SessionsSection.tsx` (item detail). *Scope note: an earlier draft of this bullet also named `SessionList.tsx`/`SessionCard.tsx` for `end_reason`; that was a deliberate narrowing made during planning, not an oversight — see plan.md's Pattern Decisions table, "`end_reason` surfacing scope" row. `end_reason` is structurally an `ItemSession`-only concept, and the generic `Session` proto rendered by `SessionCard.tsx`/`SessionList.tsx` carries no backlog/`ItemSession` linkage, so forcing it through would require an out-of-scope cross-entity join.*
- Promote `Session.pause_reason` from tooltip-only to an always-visible badge on `SessionCard.tsx`; consider a filter/grouping affordance on `SessionList.tsx`.
- Surface `BacklogStuckState.remediation_attempts`, `next_remediation_at`, and `context` on `BacklogItemCard.tsx` (board) and `LifecycleSummary.tsx` (detail), not just `/unfinished`.
- New durable respawn-event record: one row per respawn triggered by any of the four respawn/remediation call sites in `backlog_service_triage.go`, capturing reason, timestamp, triggering session ID, resulting session ID. New RPC(s) to read this per item; UI surface (likely a collapsed-by-default timeline in the item detail, consistent with the progressive-disclosure pattern `backlog-item-detail-ux` already established).
- Progressive-disclosure pass across the touched widgets: default view shows a compact status/reason summary; full history/detail expands on demand (reusing the `Collapsible`/`CollapsibleGroup` primitives already shipped by `backlog-item-detail-ux`).
- Feature registry entries for new/changed surfaces.

### Out of Scope
- Changing respawn/remediation/rework-cap decision logic (thresholds, retry counts, backoff policy).
- Redesigning the board page layout beyond adding the new indicators to existing cards.
- The `/unfinished` page's own logic (already implemented) — this item adds the *same* data to other widgets, not a replacement.
- Aggregate/analytics rollups (mean-time-to-stuck, success-rate-after-remediation) — flagged as a possible future item, not required here.

## Rabbit Holes

- Respawn-event table risks scope creep into a full analytics feature — keep it a flat append-only event log (reason, timestamp, session IDs), no aggregation/derived-metrics computation in this item.
- `BacklogStuckState` is explicitly resolve-in-place (no episode history) by prior ADR — the new respawn-event table is a *separate* concern (audit trail of respawns) and must not be conflated with re-adding episode history to `BacklogStuckState` itself.
- Reusing vs. extending the existing `Collapsible` primitives from `backlog-item-detail-ux` — confirm they generalize to a timeline/list use case before building a parallel component.

## Alternatives Considered

- Log-only (status quo): rejected — logs are not queryable/browsable by the user in the UI, which is the explicit complaint.
- Full analytics dashboard (rollups, charts): rejected as over-scope for a solo-user internal tool; a flat event list is sufficient to answer "what was tried and why."

## Feasibility Risks

- Need to confirm existing RPCs (`ItemSession`, `Session`, `StuckBacklogItem` proto messages) already carry `end_reason`/`pause_reason`/`remediation_attempts` to the frontend (research agent's finding suggests yes, but Phase 2 research should verify field-by-field, not assume).
- New respawn-event table adds a write on every respawn call site (4 locations) — needs verification these are not on a latency-sensitive hot path (they are not; respawn is already a multi-second session-spawn operation).

## Observability Requirements

Standard structured logging for the new respawn-event writes (log + DB write, not log-only). No new metrics/alerting infra needed (single-user tool, no oncall).

## Risk Control

Not needed — low risk, additive observability/UI work, no changes to existing respawn/remediation control flow. Rollback is a simple revert.

## Open Questions

- Exact placement of the respawn-event timeline in the item detail panel (new collapsible section vs. folded into existing Progress History) — leave to planning/UX design phase.
- Whether `pause_reason` needs a dedicated filter control on `SessionList.tsx` or just a visible badge is sufficient for v1 — leave to planning phase, default to badge-only if appetite is tight.
