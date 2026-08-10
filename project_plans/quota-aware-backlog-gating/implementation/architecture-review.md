# Architecture Review: quota-aware-backlog-gating
**Date**: 2026-08-10
**Verdict**: BLOCKED

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository (verified via
file check). No constitution-derived hard constraints apply — proceeding directly to the
three-lens review.

---

## Blockers

- [ ] **Task 2.3.1a (`foregroundSessionActive`)** — reads `inst.Status` / `inst.Category` directly
  off `*session.Instance` instead of via `inst.Snapshot()`. `session/instance.go:387-389`
  documents the concurrency contract explicitly: *"mu protects Instance's mutable data fields
  (Status, started, Tags, Checkpoints, ReviewState timestamps, GitHub PR fields, Artifacts,
  etc.). Use sendSyncErr / send for writes and **Snapshot() for reads**."* Both `Status` and
  `Category` are present on `InstanceSnapshot` (`session/instance_snapshot.go:88,96`) precisely
  so cross-goroutine readers don't touch the live struct. `QuotaGate.Reconcile` runs on the 60s
  reconcile-ticker goroutine — a different goroutine than whatever mutates a given session's
  `Instance` (tmux control-mode callbacks, RPC handlers, the session driver, etc.), so a direct
  `inst.Status`/`inst.Category` read here is a genuine data race, not a theoretical one. The
  plan's own research explicitly models this component on `capacity_monitor.go`'s
  ticker+threshold skeleton, and that file gets this exactly right at
  `server/services/capacity_monitor.go:149`: `inst.Snapshot().Status != session.Active`. Task
  2.3.1a diverges from its own stated precedent.
  **Remediation**: change Task 2.3.1a to snapshot first: `snap := inst.Snapshot(); if
  snap.Category != session.CategoryBacklog && snap.Status == session.Active { return true }`.
  Add a one-line comment noting *why* (cross-goroutine read) so a future edit doesn't
  "simplify" it back to a direct field read.

- [ ] **Task 1.3.1a / Task 1.3.2b (`recordRateLimitEvent` locking gap)** — Task 1.3.1a defines
  `RateLimitAggregate.recordRateLimitEvent`/`hasRecentRateLimitEvent` with explicitly **no
  locking**, justified only by a code comment ("always accessed under `QuotaGate.mu`"). But Task
  1.3.2b's literal call site is `s.quotaGate.recordRateLimitEvent(time.Now())`, invoked from
  `SessionService.onRateLimitDetected` (`server/services/session_service.go:4136`) — a callback
  that fires independently per session and can run concurrently across sessions on separate
  goroutines whenever more than one session hits a rate limit around the same time. This is
  exactly the "N independent goroutines writing shared state" race class the plan's own Pattern
  Decisions table (Rate-limit hard-signal wiring row) cites `pitfalls.md` §2 as the thing being
  *avoided* by reusing the existing fan-in — but the fan-in reuse only solves "don't subscribe to
  N separate event buses"; it does nothing about "N goroutines still call into `QuotaGate`
  concurrently once fanned in."
  Compounding this: `rateLimits RateLimitAggregate` is declared as a **named** (non-embedded)
  field on `QuotaGate` (Task 2.1.1a: `rateLimits RateLimitAggregate`). Go does not promote methods
  from named, non-embedded fields, so `quotaGate.recordRateLimitEvent(...)` as written in Task
  1.3.2b will not even compile against Task 1.3.1a's definition (which only defines the method on
  `RateLimitAggregate`, not on `*QuotaGate`) unless a `*QuotaGate`-level wrapper method is added —
  and the plan never specs that task. The one documented mitigation (the "no locking here"
  comment) sits on a type whose method the calling code, as written, cannot actually reach.
  **Remediation**: add an explicit task (e.g. 2.1.1c) defining `func (g *QuotaGate)
  recordRateLimitEvent(at time.Time) { g.mu.Lock(); defer g.mu.Unlock();
  g.rateLimits.recordRateLimitEvent(at) }`, and point Task 1.3.2b's call site at this locking
  wrapper explicitly (not at `RateLimitAggregate`'s own unlocked method).

## Concerns

- [ ] **ADR-001 Consequences / Pattern Decisions ("single writer... to avoid the multi-goroutine
  race class")** — overstates the actual guarantee. `Reconcile` does a read-then-conditionally-
  write against `backlogCtrl.IsEnabled()`/`Enable()`/`Disable()`, but `BacklogController`'s
  enabled state is also written directly and concurrently by the `UpdateFeatureFlag` RPC handler
  (verified: `server/services/feature_flag_service.go:120+`, serialized under its own dedicated
  mutex — the handler's own comment says "Serialize the whole persist-toggle-rollback sequence").
  Three independent mutex domains touch this state (`QuotaGate.mu`, `BacklogController`'s own
  internal `mu`, and `FeatureFlagService`'s toggle-serialization mutex), and none of them compose
  to prevent a TOCTOU window where a manual UI toggle lands between `Reconcile`'s `current :=
  backlogCtrl.IsEnabled()` read and its later `Enable()`/`Disable()` call within the same tick.
  The plan is aware of a version of this (Story 2.1.3's second GWT — "manual re-enable while
  quota is still low re-pauses on the next tick") and that self-correcting-within-one-interval
  design is a reasonable mitigation for a low-risk, single-user feature — but the ADR's language
  ("avoid the... race class") should say what's actually true: the design bounds staleness to
  ~1 reconcile interval (60s) via detect-and-correct, it does not eliminate the interleaving.
  **Recommendation**: reword ADR-001's Consequences bullet and add a comment at the top of
  `Reconcile` stating this as a known, accepted, bounded limitation — so it's a documented design
  choice rather than something a future reader discovers as a surprise.

- [ ] **`QuotaGate` (Epic 1.3 → 3.2, all in `quota_gate.go`) — SRP breadth.** The single file/type
  accumulates seven distinct responsibilities: hard-signal aggregation (`RateLimitAggregate`),
  hysteresis/pause-resume decisioning, provenance detection (manual-vs-quota), foreground-throttle
  tracking, notification message composition (`notifyPaused`/`notifyResumed`), and status-string
  rendering (`StatusDetail`). Task 1.3.1a even says outright: *"this becomes the primary file for
  the whole feature — subsequent epics add to it."* The Pattern Decisions table's rejection of a
  multi-actor Domain Model is sound (this genuinely is one linear policy loop, not a
  multi-aggregate domain — a Domain Model would be over-engineering here), but that doesn't
  require all seven responsibilities to live in one type either; the plan already demonstrates the
  right instinct by splitting `computeHeadroom` into its own file (`quota_headroom.go`) instead of
  folding it into `quota_gate.go`.
  **Recommendation**: apply the same split to notification composition — extract
  `notifyPaused`/`notifyResumed` (message templates + `eventBus.Publish` calls) into a small
  sibling file/type (e.g. `quota_notifier.go`), so `Reconcile` stays focused on read-signals →
  hysteresis → decide → call `FeatureController`, and delegates to the notifier only when a
  transition actually occurred. Lower blast radius for future wording/format changes, and keeps
  `quota_gate_test.go`'s fake surface smaller per test.

- [ ] **`HeadroomEstimate.Valid bool` alongside a look-alike sentinel (`PctRemaining == 100.0`
  when invalid)** — Task 1.2.1a's `computeHeadroom` returns a struct that *looks* like a normal,
  trustworthy estimate (100% remaining) even when `Valid == false` (uncalibrated budget). This is
  the classic "boolean flag beside a value that's meaningless when the flag is false" shape
  `type-driven-design` flags under "illegal states unrepresentable" — a future call site that
  forgets to check `Valid` doesn't get a compile error or a nil, it gets a plausible-looking 100%.
  The choice to fail open (100%, never triggers a pause) rather than fail closed is a deliberate
  and reasonable safety call given this ships with `Enabled=false` by default, and today's only
  consumer (`Reconcile`, Task 2.1.2b) does check `Valid` correctly — so this is low blast-radius
  today, not a blocker. **Recommendation**: consider `func computeHeadroom(...) (HeadroomEstimate,
  bool)` or a `*HeadroomEstimate` return (nil = uncalibrated) instead, so a second call site added
  later (e.g. if `StatusDetail()` or a future UI surface calls `computeHeadroom` directly) can't
  silently skip the check.

- [ ] **`gateState.consecutiveBelow`/`consecutiveAbove` — two independent counters with only a
  convention-enforced mutual-exclusion invariant.** Task 2.1.2b's spec relies on every branch
  remembering to reset the *other* counter; nothing in the type prevents both being simultaneously
  nonzero after a future edit misses a reset path, which would silently corrupt the hysteresis
  logic (e.g. a stale `consecutiveAbove` count surviving into a pause evaluation). Well covered by
  Story 2.1.3's table-driven tests today, but hysteresis code is exactly the kind of logic that
  regresses quietly under future edits. **Recommendation**: model as one signed counter
  (negative = consecutive-below run, positive = consecutive-above run, `0` = neutral) or a small
  `hysteresisCounter` type with a `Bump(direction) (fire bool)` method that enforces the exclusion
  internally, so the invariant is structural rather than remembered.

## Nitpicks

- `gateState.lastSetEnabled *bool` (nil = "never set", else the last written value) is a workable
  Go idiom for a tri-state optional, but a small named type (`type provenance int` with
  `unknown`/`setTrue`/`setFalse` constants) would read more intentionally at each call site than
  pointer-nil-checks scattered through `Reconcile`. Low value/low urgency given the field's
  narrow, single-file blast radius.
- `session.Instance.Category`/`session.Status` remain plain `string`/typed-const primitives
  reused as-is by this plan (not introduced by it) — consistent with existing repo convention and
  correctly out of scope to fix here, but worth noting `foregroundSessionActive`'s
  `Category != session.CategoryBacklog` check is a negative string comparison, not an exhaustive
  switch; a future third `Category` value with foreground-adjacent semantics wouldn't be caught by
  the compiler. Not worth a type change for this feature's scope.
- The `FeatureController`/`InstancePoller`/`tokens.TokenStoreReader` reuse (Task 2.1.1a) is a
  genuinely good Dependency Inversion / Interface Segregation call — `QuotaGate` depends on
  exactly the three methods it needs from `FeatureController` (already defined in the *consumer*
  package `services`, matching this repo's own `interface-pollution-checklist.md` guidance), and
  introduces no new interface where one already fits. No action needed; called out here only
  because the review brief specifically asked whether this reuse respects DIP — it does.

---

## Summary of Positive Findings (not gating, noted for completeness)

- The Pattern Decisions table is unusually thorough: every non-trivial component choice records
  the alternative considered and *why* it was rejected, directly against this repo's own
  `interface-pollution-checklist.md` smells (speculative `SignalSource` interface, redundant
  `QuotaFeatureController` wrapper, two-unlinked-booleans provenance — all correctly rejected).
- No persistence/schema work is introduced, correctly scoped given `requirements.md`'s Risk
  Control section frames both failure directions as low-risk and recoverable via the existing
  manual toggle — a Repository/Unit-of-Work pattern would be over-engineering here, and the plan
  doesn't reach for one.
- Feature-flagged rollout (`Quota.Enabled` default `false`) plus a three-stage activation plan is
  a sound, low-risk rollout shape for a single-user instance.
