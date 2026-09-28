# ADR-003: Nudge Cap/Cooldown State Is a Persisted Row Guarded by a Package-Level Mutex, Not Per-Dispatcher-Local State

**Status**: Accepted
**Date**: 2026-09-27

## Context

Three prior incidents bound this decision (`research/pitfalls.md` §1):

- **Crash-loop restart storm** (`b28e78fc0`): a cap that only *logged* without
  blocking caused a 93%-of-7.6GB-heap incident. A log-only cap is not a cap.
- **JulesDispatchService spend-cap race** (`5a187795b`): a **per-item** mutex was not
  enough — two different items' dispatches could each observe a stale under-limit
  count and both proceed past a shared ceiling. Fixed with a **package-level** mutex
  serializing check-then-reserve.
- `AutonomousDriver`'s `lastNudge`/`nudgeCooldown` is a **per-loop-local, in-memory**
  tracker — sufficient for a single self-driving loop, not sufficient here because
  this feature's nudge is dispatched externally and multiple dispatches (manual
  re-click, a future reconciler-triggered dispatch) could race for the same item.

Separately, `research/ux.md` requires nudge-attempt history to be visible,
chronological, and to survive navigation away and back — meaning cap/cooldown state
cannot be purely in-memory even if concurrency safety weren't in question.

## Decision

Nudge cap/cooldown state is:

1. **Durable**: a new ent-backed row, `NudgeCapRecord` (schema:
   `session/ent/schema/nudgecaprecord.go`), keyed by backlog item ID, holding
   `NudgeCount`, `WindowStartAt`, `LastNudgeAt`. This satisfies the UX history
   requirement (a page refresh sees the same state) and survives process restart.
2. **Atomically check-then-reserved**: a package-level `diagnoseNudgeGuardMu
   sync.Mutex` (mirroring `julesSpendGuardMu`'s precedent) wraps the entire
   read-check-increment-write sequence against the `NudgeCapRecord` row, for the
   lifetime of this single-process, self-hosted deployment (per requirements.md's
   Constraints: "no multi-tenant... beyond what already exists"). A single in-process
   mutex is sufficient because there is exactly one `stapler-squad` process; this is
   not a distributed-systems problem today.
3. **Actually blocking**: the gate check returns
   `SafetyGateReasonNudgeCapReached`/`SafetyGateReasonNudgeCooldownActive` and the
   caller MUST treat that as a hard stop on the write path — never a log-only
   soft-fail, directly addressing the crash-loop-storm precedent.
4. **Live-settable limits, not a guessed hardcoded number**: the numeric cap and
   cooldown duration live in `config.Config.DiagnoseNudge`
   (`MaxNudgesPerItemOrDefault()`, `CooldownSecondsOrDefault()`), following the
   `AutonomousMaxTurnsOrDefault()` shape — adjustable without a redeploy, shipped
   conservative (default `MaxNudgesPerItem = 2`, `CooldownSeconds = 900` — 15 minutes,
   five times `AutonomousDriver`'s 3-minute production cooldown, chosen deliberately
   more conservative since this write is fully unattended with no approval step,
   unlike the self-driving loop's own internal nudge).

## Consequences

- One new ent schema + a narrow repository (`NudgeCapStore`, mirroring
  `supersededSessionStore`'s narrow-interface style) is added (Phase 3, Story 3.3.1).
- Every check-then-increment must go through the same guarded function; a future call
  site that increments the counter without holding `diagnoseNudgeGuardMu` reopens the
  exact race this ADR closes — flagged in code review guidance for Phase 3.
- The cap is per-item (not per-session), since a "session" here can be superseded by
  a new round mid-cycle; the cap tracks how many times *this backlog item* has been
  nudged, which is the quantity Tyler actually wants bounded and visible.

## Alternatives Considered

- **In-memory-only per-dispatcher tracker** (mirroring `AutonomousDriver.lastNudge`
  directly) — rejected: doesn't survive a process restart (loses history, reopens the
  crash-loop-storm shape on redeploy) and doesn't coordinate across concurrent
  dispatchers for the same item, which the JulesDispatchService precedent shows a
  per-item-scoped structure alone cannot do.
- **DB-only optimistic concurrency (CAS on a row version), no in-process mutex** —
  rejected as unnecessary complexity for this deployment: a single-process app can
  serialize with a plain mutex more simply and just as correctly; CAS-based retry
  logic would be solving a distributed-systems problem this system doesn't have.
  Revisit if `stapler-squad` is ever horizontally scaled.
- **Reuse `effectiveReworkCap`'s exact mechanism (`BacklogItemData.ReworkCapOverride`)
  wholesale** — rejected as the sole mechanism: it's the right *shape* to imitate
  (live-settable, per-item-override precedent) but tracks rework rounds, a different
  quantity than nudge attempts; a dedicated `NudgeCapRecord` keeps the two concerns
  from being silently conflated in one counter.
