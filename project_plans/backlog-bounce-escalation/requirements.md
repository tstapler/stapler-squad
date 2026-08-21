# Requirements: backlog-bounce-escalation

**Date**: 2026-08-11
**Type**: feature addition (backend reconciliation + notification/escalation; UI surfacing likely additive to existing stuck-state view)
**Complexity**: 2-3 — targeted extension of existing durable stuck-state infrastructure, not a new subsystem
**Source item**: `8baf63ea-d8b4-4ad2-b3bd-d07cfa0f39a6` — "Backlog review/work items can bounce indefinitely without converging or escalating"

## Problem Statement

Backlog items in `review`/`in_progress` can bounce (rework → re-review → rework...) without
ever converging on a resolution. The existing `maxAutoReworkIterations` cap (3,
`server/services/backlog_service_triage.go`) and the newer `MaxRemediationAttempts` /
`backlog_stuck_states` infrastructure (built by the `backlog-stuck-item-visibility` project)
already give durable, queryable per-reason stuck rows and a one-time parking notification
("...has been retried N times... use Reset to try again automatically"). What's still
missing, confirmed live against the running instance on 2026-08-11:

1. **No escalation for multi-reason accumulation.** Two items (`ccbfe7a6`, `e271db3d`) are
   simultaneously flagged by 2-3 independent stuck detectors at once (`bouncing` +
   `abandoned_review`, or `bouncing` + `autonomous_stuck` + `abandoned_review`). Each reason
   is tracked as its own durable row, but nothing treats "N simultaneous reasons on one item"
   as a distinct, higher-severity signal — it surfaces identically to a single-reason stuck
   item.
2. **Hitting the remediation cap while still bouncing produces the same notification as any
   other park.** Item `92d679fd` is `in_progress`, `bouncing` since 2026-08-11 01:35, and
   `remediation_attempts=3` (at `MaxRemediationAttempts`). The existing "use Reset to try
   again" notification is generic — it doesn't distinguish "capped after N non-converging
   review cycles" (a signal the item may need a fundamentally different approach, not just a
   retry) from an ordinary transient-failure cap-out.
3. **No differentiated review strategy for inherently non-deterministic fixes.** All three
   currently-bouncing items are flaky-test root-cause fixes. Root-causing a flaky test is
   harder for an automated LLM reviewer to verify than a deterministic bug fix — a review
   pass can pass by "getting lucky" on a single non-repro run, or fail by hitting the same
   flake the fix was meant to resolve. The review pipeline currently applies the same
   verdict process regardless of whether the underlying change addresses non-deterministic
   behavior.

## Baseline

Today: `backlog_stuck_states` rows are durable and queryable per-reason (confirmed via the
`backlog_stuck_states` table itself, `session/stuck_decisions.go`,
`server/services/backlog_service_stuck.go`), and `MaxRemediationAttempts` parking fires one
notification when the cap is hit (`session/backlog_lifecycle_review.go:208,660`,
`session/backlog_lifecycle_pr.go:881,1210`, `session/backlog_lifecycle_stale.go:240`). There
is no code today that reads "how many distinct stuck reasons does this item have right now"
as an input to severity/escalation, and no code that treats a flaky-test-classified item
differently anywhere in the triage/review pipeline.

## Users / Consumers

Single user (Tyler), via the stapler-squad web UI and its notification surface. No other
consumers.

## Success Metrics

- An item that accumulates 2+ simultaneous open stuck-state reasons is visibly distinguished
  (in the UI and/or notification severity) from a single-reason stuck item, without requiring
  the user to cross-reference the stuck-state table by hand.
- An item that hits `MaxRemediationAttempts` while its `bouncing` reason is still open
  produces a notification/signal that is distinguishable from an ordinary cap-out, and that
  signal persists (queryable) rather than being a one-time toast.
- Verification: re-run the live queries used in this item's investigation (stuck-state count
  per item, `remediation_attempts` vs cap, bouncing reason open/closed) against the shipped
  feature and confirm `ccbfe7a6`, `e271db3d`, and `92d679fd` (or their then-current
  equivalents) surface with elevated/distinct signals.

## Appetite

Small–Medium (1–2 weeks). This is an incremental extension of existing, already-durable
stuck-state infrastructure — not new storage or a new subsystem.

**Fallback increment**: if scope tightens, ship multi-reason escalation (item 1) alone;
review-strategy differentiation for flaky-test items (item 3) is explicitly the more
speculative, lower-confidence piece and can be deferred to its own follow-up without
blocking the escalation mechanism.

## Constraints

- Must build on the existing `backlog_stuck_states` schema/detectors
  (`session/stuck_decisions.go`, `session/backlog_remediation.go`,
  `server/services/backlog_service_stuck.go`) rather than introducing parallel tracking.
- Existing `docs/registry/` feature-registry rule applies to any new RPC/UI surface added
  (`.claude/rules/feature-registry.md`).
- Single-developer, self-hosted instance — no multi-tenant/auth considerations.

## Non-functional Requirements

- **Performance SLO**: none specified; reuses the existing 60s reconcile ticker cadence.
- **Scalability**: not applicable (single user, backlog size in the tens of items).
- **Security classification**: internal.

## Scope

### In Scope

- Compute and surface a per-item "stuck severity" (or equivalent escalation signal) driven by
  (a) count of simultaneously open stuck-state reasons, and (b) reaching
  `MaxRemediationAttempts` while a `bouncing` reason remains open.
- A durable (DB-backed) escalation marker/notification distinct from the existing one-time
  "use Reset" toast, so a multi-reason or capped-while-bouncing item stays visibly flagged
  until resolved or explicitly acknowledged — not just a point-in-time event.
- Investigate whether flaky-test-classified items warrant a distinct review strategy (e.g.
  requiring the fix to demonstrate N consecutive non-flaky passes, or a different verdict
  threshold) and produce a recommendation — full implementation of a new review strategy may
  be out of scope for this pass depending on planning-phase findings (see Fallback increment).

### Out of Scope

- Root-causing the three specific flaky tests named in the source item (`ccbfe7a6`,
  `e271db3d`, `92d679fd`'s underlying test fixes) — those are the existing items' own job.
- Changing the value of `maxAutoReworkIterations` / `MaxRemediationAttempts` themselves.
- Any new remediation *action* (auto-retry-differently, auto-escalate-to-different-reviewer)
  beyond visibility/notification — consistent with the prior `backlog-stuck-item-visibility`
  project's explicit "visibility, not a control panel" scope decision.
- Rebuilding or replacing the existing `backlog_stuck_states` schema/detector framework.

## Rabbit Holes

- Defining "flaky-test-classified item" requires either an LLM classification step or a
  heuristic (title/description keyword match, e.g. "flaky", "-race", "intermittent") —
  needs an explicit, cheap decision in planning; don't let this become a general
  intent-classification subsystem.
- Multi-reason severity scoring could balloon into a full scoring/weighting system — keep the
  initial version to a simple count/threshold, not a tunable rubric.

## Alternatives Considered

- Leave the existing per-reason notification as-is and rely on the user periodically querying
  `backlog_stuck_states` directly: rejected — this is exactly the gap the source item reports
  (the evidence itself was gathered by manual DB inspection, which shouldn't be required).
- Auto-escalate by taking a remediation action (e.g. auto-abandon after cap+bounce): rejected
  as out of scope — matches the prior project's "visibility, not control panel" precedent, and
  the source item explicitly frames this as a bounce/escalation *signal* problem, not asking
  for new automated remediation.

## Feasibility Risks

- The three live example items may already resolve (get merged, get reset) before this ships,
  removing the concrete verification data points; the plan should verify against current
  live state at implementation time, not hardcode these specific item IDs into tests.

## Open Questions

- Exact severity threshold (2 reasons? 3?) for elevated signal — leave to planning to pick a
  defensible default informed by the current live data (both currently-flagged items have 2-3
  reasons; a naive threshold of "≥2" would have caught both).
- Whether flaky-test review differentiation belongs in this project or should be split into
  its own follow-up item once this project's planning phase assesses its actual scope —
  recommend planning make this call explicitly rather than defaulting to "in scope."
