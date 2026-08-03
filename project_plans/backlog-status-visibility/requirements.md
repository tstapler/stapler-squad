# Requirements: backlog-status-visibility

**Date**: 2026-08-01
**Type**: feature addition (frontend UX surfacing + backend audit-trail extension)
**Complexity**: 3 — system design (multiple widgets + one new backend table)
**Backlog item**: `0a366262-145d-4eb8-81fc-965cf563d7f7` — "Improve the ux of the backlog item workflow and sessions list"

## AC10 Decision Record (2026-08-02)

**Decision**: This backlog item (`0a366262-145d-4eb8-81fc-965cf563d7f7`) is the **canonical delivery vehicle** for `project_plans/backlog-session-lifecycle-ux`'s already-planned work. No independent second plan is produced; this item's implementation session executes `project_plans/backlog-session-lifecycle-ux/implementation/plan.md` directly, carrying forward the two P1 corrections this file's Scope section already flagged (idempotency dedupe, two-counter caption) plus AC7/AC8's fail-open/queued-vs-failed requirements.

**Rationale**:
- No PR exists yet for `backlog-session-lifecycle-ux`, `backlog-status-visibility`, or any respawn-audit-trail work (`gh pr list --search "lifecycle-ux OR respawn OR status-visibility"` on 2026-08-02: zero matches among open/merged/closed PRs). Executing this item against the existing plan produces exactly one PR, satisfying this AC's actual goal ("so two divergent PRs touching the same files are not produced in parallel") — there is nothing to reconcile against because nothing has shipped from the other plan.
- The Phase 0 pre-implementation gate (`project_plans/backlog-session-lifecycle-ux/implementation/plan.md` Story 0.1.1/Task 0.1.1a — merge-base check on commits `0ac676001`/`f7ab0c9ad`) was re-run on 2026-08-02: both commits are now ancestors of `origin/main` (confirmed via `git merge-base --is-ancestor 0ac676001 origin/main` and same for `f7ab0c9ad`; both exit 0). `origin/main` is the relevant target per this repo's PR workflow (backlog PRs are opened against `tstapler/stapler-squad`, i.e. `origin`, not `upstream-fanatics`). The gate is cleared for this branch.
- No operator was reachable to make this call synchronously (autonomous backlog session); this session made the call itself, in auto mode, per the standing instruction to make the reasonable call and proceed rather than block indefinitely, and is recording the decision here rather than acting silently on it.

This decision does not require re-deriving `research/summary.md` or `implementation/plan.md`/`validation.md` above — they already state "adopt the existing plan verbatim" and are left as-is; this section only resolves the one open item they both deferred to "an operator decision."

## ⚠ Duplicate-work finding (read first)

This item's description is, near-verbatim, the problem statement of an **already fully-planned** project sitting in this working tree right now: `project_plans/backlog-session-lifecycle-ux/`. That directory already contains a complete SDD pipeline — `requirements.md`, `research/{stack,features,pitfalls,ux,build-vs-buy}.md`, `implementation/plan.md` (5 phases, 20 stories, full task breakdown), `implementation/architecture-review.md`, `implementation/adversarial-review.md` (verdict: CONCERNS, with P1 items flagged and one already resolved), `implementation/pre-mortem.md` (5 failure modes, 2 P1), and `implementation/validation.md` (full requirement→test mapping + UX acceptance tests) — all dated 2026-08-01 (today), currently **uncommitted** in this checkout.

Concretely, both documents ask for the same three things:
1. Surface `ItemSession.end_reason` / `Session.pause_reason` (already-persisted, under-surfaced fields) on the session widgets.
2. Surface `BacklogStuckState.remediation_attempts` / `next_remediation_at` / `context` (respawned-because-of-inactivity vs. stalled) on the board card and detail.
3. Add a net-new, durable, queryable **respawn-event audit trail** (the "increase the amount of data that is tracked" ask) — currently only `log.InfoLog` lines at the 4 auto-respawn/remediation call sites.
4. Apply progressive disclosure to whichever widgets gain new data (reusing the `Collapsible`/`useShowMore` primitives already shipped by `backlog-item-detail-ux`).

**Recommendation**: do not plan or implement this item independently. Either (a) close this backlog item as a duplicate once `backlog-session-lifecycle-ux`'s already-committed-locally work lands, or (b) if this item's triage is what's supposed to drive that work, redirect execution to consume `project_plans/backlog-session-lifecycle-ux/implementation/plan.md` directly rather than generating a second, divergent plan. This document intentionally does not re-derive independent research/plan/validation — it cross-references the existing artifacts and flags the two gaps found while reviewing them (see Scope below).

## Problem Statement

Users cannot tell, at a glance, what has actually been *tried* on a backlog item or why a session ended the way it did:
1. **Data that exists but isn't surfaced**: `ItemSession.end_reason`, `Session.pause_reason` (tooltip-only, not an always-visible badge), and `BacklogStuckState.remediation_attempts`/`next_remediation_at`/`context` (only rendered on `/unfinished`, not the board card or detail Sessions section).
2. **Data that doesn't exist yet**: no structured respawn-event audit trail. `AutoRespawnReview`/`AutoRespawnAutonomousWork`/`AutoRespawnTriage`/`RemediateStaleWorkSession` only log, never persist a queryable "session N was respawned because of reason X" row.

## Baseline

Today a user watching the board or an item's session list sees status chips and, on hover, a pause-reason tooltip. To learn why a session ended, whether it was auto-respawned, or how many times, they must read server logs. See `project_plans/backlog-session-lifecycle-ux/requirements.md` for the full baseline writeup (identical situation).

## Users / Consumers

Solo user (Tyler), stapler-squad web UI, desktop and mobile.

## Success Metrics

Same three as `backlog-session-lifecycle-ux/requirements.md`:
- From the board card and item detail Sessions section, a user can see whether a session ended cleanly/paused/errored, and whether the item has been auto-respawned, how many times, and why — without opening `/unfinished` or reading logs.
- A user can distinguish "actively being retried, backing off" from "stalled with no further automatic action."
- Every respawn is a durable, queryable event (reason, timestamp, triggering/resulting session), not just a log line.

## Appetite

Medium (1–3 weeks) — same as the existing plan; UI wiring of already-modeled fields plus one small additive schema change.

## Constraints

- vanilla-extract only for new/changed CSS (`.claude/rules/css-architecture.md`).
- Feature registry entries required for new/changed RPCs/UI (`.claude/rules/feature-registry.md`).
- ent schema changes must use `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (`.claude/rules/ent-schema-generation.md`).
- Mobile + desktop parity required (touch targets, responsive layout).
- Do not change respawn/remediation/rework-cap *decision logic* — observability only.

## Scope

### In Scope
Identical to `backlog-session-lifecycle-ux/implementation/plan.md`'s 5 phases — see that file, not re-listed here. Two gaps this review adds on top of that plan (found while reading its own adversarial-review.md/pre-mortem.md, which already flag them as unresolved P1/P2 concerns — carrying them forward here so they aren't lost if this item's artifacts are what gets executed):
- **Idempotency guard on `CreateRespawnEvent`** (pre-mortem #1, P1) — the 4 call sites this plan instruments have a *recent, documented* history (commit `0ac676001`) of double-firing from overlapping reconciliation sweeps; the plan's Task 4.2.1e already designs a dedupe guard for this — treat it as required, not optional polish.
- **Two-counter reconciliation** (pre-mortem #2, P1) — `BlockerChip`'s `×N` (episode-scoped `remediation_attempts`, resets on resolve) and `RespawnHistorySection`'s count (all-time, append-only) will structurally diverge for any item that cycles stuck→resolved→stuck; the plan's Story 4.5.2 already requires an explanatory caption for this — do not drop it during implementation.

### Out of Scope
Same as `backlog-session-lifecycle-ux/requirements.md`: respawn/rework-cap decision-logic changes, board layout redesign, `/unfinished` page logic, analytics rollups.

## Rabbit Holes / Feasibility Risks / Observability / Risk Control

See `backlog-session-lifecycle-ux/requirements.md` — identical, not re-derived.

## Open Questions

- Is this backlog item (`0a366262…`) meant to be a fresh, independently-tracked delivery vehicle for the same feature, or should it be merged/closed against the work already sitting in `project_plans/backlog-session-lifecycle-ux/`? This is an operator decision, not something further research resolves.
- `backlog-session-lifecycle-ux/implementation/plan.md`'s own Unresolved Questions section flags a **hard pre-implementation gate**: as of 2026-08-01 the `end_reason` prerequisite commits (`0ac676001`, `f7ab0c9ad`) are committed locally but not yet an ancestor of `origin/main` or `upstream-fanatics/main`. Whoever executes either plan must re-run that merge-base check first (Story 0.1.1/Task 0.1.1a in the existing plan).
