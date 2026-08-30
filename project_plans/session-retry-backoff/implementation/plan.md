# Implementation Plan: session-retry-backoff

**Feature**: Replace the hardcoded single-retry crash/stall recovery in `session/session_driver.go`
with a configurable, multi-attempt, exponential-backoff `RetryPolicy` — adding `tmux_exited`
detection, a `permanently_failed` terminal state, retry-count/history UI, and a manual "Retry now"
override.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001](../decisions/ADR-001-permanently-failed-as-status-not-flag.md) — `permanently_failed`
is a new top-level `Status`/`SessionStatus` enum value, not a flag.

All line numbers below were re-verified against the live repo on 2026-08-06 (not copied
verbatim from research docs — two numbers had drifted: the next-free `SessionStatus` enum
value is `10`, not the `8` `research/stack.md` guessed before `HIBERNATED`/`RESTORING` claimed
8/9; the fourth `SetReviewQueue` wiring site is `session_service.go:799`, not `:790`).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `RetryState` | New struct embedded (by value) into `Instance`, holding the automated crash/stall retry lifecycle for one session: attempt count, resolved max, last failure reason, pending-retry timestamp, and history. | `session/retry_state.go` (new file), mirrors `ReviewState` (`session/review_state.go`). Protected by `inst.mu`. |
| `RetryAttemptRecord` | One entry in `RetryState.RetryHistory`: `{Attempt int, Reason string, Timestamp time.Time}`. | Capped at 10 entries. |
| `RetryPolicyConfig` | Config struct (global default in `config/types.go`, optional per-session override via proto) describing `Enabled`, `MaxAttempts`, `Backoff`, `InitialDelaySeconds`, `MaxDelaySeconds`, `RetryOn`, `StaleTriggersRetry`. | Mirrors `SessionRetentionConfig`'s `Enabled *bool` nil-means-unset idiom. |
| `RetryPolicy` (resolved) | The effective, already-merged (global ⊕ per-session-override) `RetryPolicyConfig` value, resolved once at driver start and threaded down as a plain parameter — never re-read from config mid-cycle. | Same shape as `allowedPath` threading today. |
| `MaxAttempts` | Number of automated retries before the session transitions to `PermanentlyFailed`. Default `1` (preserves today's exact single-retry behavior — AC7). | |
| `InitialDelaySeconds` / `MaxDelaySeconds` | Backoff formula inputs: `min(initial * 2^attempt, max)`. Defaults `0`/`300` — `0` preserves today's immediate-restart behavior. | |
| `RetryOn` | Subset of `["crashed", "stalled", "tmux_exited"]` that is eligible for automated retry. A failure whose reason is absent from this list is treated as immediately exhausted (skips straight to `PermanentlyFailed`, no wait). | Empty/nil defaults to all three. |
| `StaleTriggersRetry` | Opt-in `*bool` (default nil→false) on `RetryPolicyConfig` that, once the sibling `stale-session-detection` project's `StaleSessionConfig` exists, treats a crossed staleness threshold as an additional `"stalled"`-classified trigger. | Stubbed in this plan (Phase 5) — sibling config confirmed absent as of 2026-08-06 (`grep -rn "StaleSessionConfig" config/` → no hits). |
| `NextRetryAt` | `time.Time` on `RetryState`; zero means no retry pending. The existing 2s poll-loop ticker checks `time.Now().After(NextRetryAt)` before restarting — no new goroutine/timer. | |
| `LastFailureReason` | Most recent value of `classifyFailureReason`: `"crashed"`, `"stalled"`, or `"tmux_exited"`. Prefixed into the retry continuation prompt. | |
| `RetryAttempt` / `RetryMaxAttempts` | Current attempt count and the policy-resolved cap, snapshotted onto `RetryState` at driver start so a live config edit mid-cycle doesn't change the cap a session is already partway through. | |
| `PermanentlyFailed` | New top-level `Status` (Go) / `SessionStatus` (proto) value — see ADR-001. Not a flag. Session is logically stopped but visually/behaviorally distinct, and transitions back to `Active` via "Retry now." | `Status = 6` (Go), `SESSION_STATUS_PERMANENTLY_FAILED = 10` (proto). |
| `backoffDelay` | Pure function `backoffDelay(attempt int, initial, max time.Duration, jitterFraction float64) time.Duration` computing `min(initial * 2^attempt, max)` plus bounded jitter. Hand-rolled, no new dependency (`research/build-vs-buy.md`). | `session/retry_state.go`. |
| `classifyFailureReason` | `func classifyFailureReason(inst *Instance) string` — returns `"tmux_exited"` when `TmuxProcessManager.DoesSessionExist()` is `false`, else `"crashed"` for a process-exit path, or `"stalled"` for the inactivity-timeout path (which calls it directly with a known reason, not through liveness detection). | `session/session_driver.go`. |
| `tmux_exited` | Failure reason: the tmux *session* itself (not just the pane process) is gone — pane loss, OOM-killed tmux server, laptop sleep. Distinct from `crashed` (process exited, tmux session/remain-on-exit placeholder still alive). | Detected via `session/tmux_process_manager.go:123` `DoesSessionExist()`. |
| `evaluateSessionRetry` | Pure decision function `evaluateSessionRetry(rs RetryState, policy RetryPolicyConfig, reason string, now, bootTime time.Time) retryDecision` mirroring `session/backlog_remediation.go`'s `evaluateRemediation` shape. Returns `scheduled` / `notEligible` (reason ∉ RetryOn) / `exhausted` (attempt ≥ max) / `restartGrace` (tmux_exited within grace window of `bootTime`). | New, DB-independent, exhaustively unit-testable. |
| `retryInFlight` | `atomic.Bool` field on `Instance`, analogous to `driverRunning` — CAS-guards the single restart critical section so the automated backoff-expiry path and a manual "Retry now" RPC can never both call `inst.Restart()`/mutate `RetryState` concurrently. | Closes the double-restart / lost-update races in `research/pitfalls.md` §2. |
| `restartForRetry` | Extracted helper (from today's `handleDriverFailure` restart block, lines ~534-552) shared by both the automated backoff-expiry path and manual `RetryNow()` — single source of the actual `inst.Restart()`/`inst.Start()` call. | |
| `RetryNow` | `func (i *Instance) RetryNow() error` — claims `retryInFlight`, resets `RetryState`, transitions `Status` out of `PermanentlyFailed` (or bypasses `NextRetryAt`), calls `restartForRetry`. Backs the manual "Retry now" UI action and `RetrySession` RPC. | |
| `RetrySession` | New ConnectRPC method (`proto/session/v1/session.proto`), cloned from `RestartSession`'s shape, calling `inst.RetryNow()` instead of `inst.Restart()`. | |
| `markSessionPermanentlyFailed` | Replaces the give-up branch of today's `markSessionNeedsAttention` call inside `handleDriverFailure`: transitions `Status → PermanentlyFailed`, still adds a `ReviewQueue` entry (unchanged `ReasonStale`, per ADR-001), and fires `inst.notifier.Notify(...)` exactly once (edge-triggered). | |
| `Notifier` | Existing interface (`session/backlog_lifecycle.go:28`) + `EventBusNotifier` adapter (`server/services/backlog_notifier.go`). Newly wired onto `Instance` (mirrors `SetReviewQueue`/`GetReviewQueue`) so `markSessionPermanentlyFailed` can push a proactive notification, not just a passive queue entry. | |
| `isOneShot` | Existing gate (`session_driver.go:643`) excluding `backlog:triage`/`backlog:review`-tagged sessions from all auto-retry. Preserved unchanged by this feature — see Unresolved Questions for the `backlog:work` interaction. | |
| `driverMinRuntimeBeforeRetry` | Existing 5-minute floor (`session_driver.go:59`) distinguishing "crashed quickly" from "ran long enough to be a normal completion." Preserved unchanged, sits upstream of `RetryOn` eligibility. | |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Retry state ownership | `RetryState` value struct embedded on `Instance`, protected by `inst.mu` (mirrors `ReviewState`) | `research/architecture.md` §1-2 | (a) Separate `RetryManager` service keyed by session ID, holding its own map + lock | (a) adds a second source of truth that must be kept in sync with `Instance` lifecycle (archive/delete/hibernate), duplicating exactly the bookkeeping `Instance` already does for `ReviewState`; no second real consumer justifies the extra indirection (Step 0.5 creative pass) |
| Retry state persistence | JSON-file-backed via existing `Instance`/config state round-trip (no new datastore) | `requirements.md` NFR, `research/build-vs-buy.md` | (b) Fork `session/backlog_remediation.go`'s Ent-backed `BacklogStuckState` row pattern wholesale | (b) requires a new DB table for a per-session attempt counter + ≤10 history entries — explicitly ruled out by requirements.md, and the schedule it uses (fixed lookup table) isn't the exponential formula requested anyway (Step 0.5 creative pass; also `research/build-vs-buy.md` §4) |
| Backoff scheduling | Poll-loop tick-check: persist `NextRetryAt`, existing 2s ticker in `runSessionDriverWithPrompt` checks `time.Now().After(NextRetryAt)` | `research/pitfalls.md` §1, mirrors `session/backlog_remediation.go`'s `RemediationDue` | `time.Sleep`/`time.Timer`/`time.AfterFunc` inside the retry path | Not interruptible by `Stop`/`Pause` (no `context.Context` exists in this file today); would leak the goroutine or require inventing a new cancellation primitive this file has never needed |
| Backoff math | Hand-rolled `backoffDelay(attempt, initial, max, jitterFraction)` — `min(initial*2^attempt, max)` + bounded jitter | `research/build-vs-buy.md` | `cenkalti/backoff/v5` (already an indirect dep via OTel) | Library wraps a synchronous `Retry(func() error)` call; this feature needs a delay-computation + schedule primitive, not a call-retry wrapper — forcing the library in means using only its 2-line formula method |
| Attempt-counter concurrency | `RetryState.RetryAttempt` stays a plain `int` under `inst.mu`, but every mutation path (automated backoff-expiry restart AND manual `RetryNow()`) is serialized behind a `retryInFlight atomic.Bool` CAS gate before it may touch `RetryState` or call `inst.Restart()` | `research/pitfalls.md` §2, refined from `research/stack.md` §5's literal suggestion | Bare `atomic.Int32` attempt counter | An `atomic.Int32` alone can't atomically carry the counter *and* `LastFailureReason`/`NextRetryAt`/`RetryHistory` together (stack.md itself notes this) — a single CAS-gated critical section around the existing mutex-protected struct gives equivalent atomicity without splitting related state across two synchronization primitives |
| `tmux_exited` detection | `classifyFailureReason(inst)` using `TmuxProcessManager.DoesSessionExist()`, plus a restart-grace exemption keyed off a package-level `serverStartTime` | `research/architecture.md` §5, `research/pitfalls.md` §3 | Naive classification with no restart-grace | A routine `make install-service` deploy (or any tmux-server-wide loss) would otherwise misclassify every live session's pane loss as N independent crashes and fire a synchronized retry storm — this repo has already hit and fixed the analogous problem once (`session/backlog_remediation.go`'s `remediationGrantedRestartGrace`) |
| `permanently_failed` representation | New top-level `Status`/`SessionStatus` enum value | ADR-001, `research/ux.md` §1e option 2 | `PermanentlyFailed bool` flag on `RetryState`, surfaced via `AttentionReason`/`ReviewQueue` | Fails the UX doc's core test: a flag nested in the same `ReviewQueueBadge` slot as routine `NeedsAttention` reasons repeats the exact "mislabeled `ReasonStale`" ambiguity (`session_driver.go:587`) this feature exists to eliminate |
| Config shape | `RetryPolicyConfig{Enabled *bool, ...}` mirroring `SessionRetentionConfig` | `research/stack.md` §2 | Bare `bool` defaulting to `false` | Breaks AC7 — an old `config.json` predating this field must resolve to "enabled, 1 retry, 0 delay" (today's behavior), which only the nil-pointer-means-unset idiom achieves |
| Per-session override | `optional RetryPolicyConfig retry_policy_override` on `CreateSessionRequest`, resolved once (global ⊕ override) at `StartSessionDriver` and threaded down as a value param | `research/architecture.md` §4, mirrors `ReworkCapOverride *int` | Re-reading `*config.Config` from each poll tick | A live global-config edit mid-backoff-cycle would otherwise silently change the cap/delay a session is already partway through, contradicting the "snapshotted at driver start" comment already established for `RetryMaxAttempts` |
| Notification | Reuse `Notifier` interface + `ReviewQueue.Add`, fired edge-triggered (once per exhaustion episode) | `research/architecture.md` §7, `research/pitfalls.md` §5 | Build a new dedicated retry-notification path | Requirements explicitly say "reuse the existing notification bus"; a new path would duplicate `EventBusNotifier`'s existing title/priority/type machinery for no benefit |
| UI retry badge | New peer badge (`RetryBadge`) in `SessionCard.tsx`'s existing header row, `compact`/full split like `ReviewQueueBadge` | `research/ux.md` §1a-b | Nest retry info inside `ReviewQueueBadge`'s `context` field | Retry-in-progress is gated on a different condition (attempt count) than review-queue reasons and needs independent 3-tier severity styling (ux.md §4) — conflating the two badges' conditions would make both harder to reason about |
| Manual "Retry now" | Clone `SessionActionsOverflow.tsx`'s Restart confirm-dialog pattern verbatim (primary-button + overflow-menu-item + `useFocusTrap` dialog) | `research/ux.md` §1c | An instant, unconfirmed action | Every existing sibling override/destructive action (Restart, Delete, Clear Conversation) in this file confirms before acting; skipping confirmation here would be the one inconsistent exception |
| Retry history | Clone `CheckpointList.tsx`'s list shape verbatim (`RetryHistoryList`) | `research/ux.md` §1d | New dedicated "history" tab/view | No dedicated history view exists yet in `SessionDetailView.tsx`; adding one is out of scope when a peer-section clone of the existing checkpoints pattern satisfies AC5 |

---

## Migration Plan

No schema/data migration. All proto changes are additive-only (new message fields at the next
free field number, new enum values at the next free integer) — old clients/old persisted
`Session` protos simply don't populate the new fields, and `google.protobuf.Timestamp`/`int32`
zero-values default correctly. `config.json` gains one new `"retry_policy"` key; the existing
`LoadConfigFromPath` unmarshal-then-default-fill pattern already handles a missing key via the
`Enabled *bool` nil-means-true idiom (AC7) — no explicit migration step, consistent with how
every other `*Config` addition (`SessionRetention`, `TmuxExecGate`, `Hibernation`) was added.

## Observability Plan

- **Logs**: extend the existing `log.Info`/`log.Warn` call sites in `handleDriverFailure` (already
  structured with `"session"`, `"reason"` fields) to also include `"attempt"`/`"max_attempts"` on
  every retry decision, and add one new log line at the `markSessionPermanentlyFailed` transition
  (`"session", "reason", "attempts_exhausted"`). No new logging infrastructure.
- **Metrics**: none added — this repo has no existing metrics-emission convention in
  `session/session_driver.go` to extend (OpenTelemetry setup in `telemetry/` is opt-in and scoped
  elsewhere per `.claude/docs/opentelemetry.md`); out of scope to introduce here.
- **Alerts**: the `Notifier`/`ReviewQueue` wiring (Epic 2.6, 2.5.2) *is* the alerting path for this
  single-user app — no separate alerting system exists or is warranted.

## Risk Control

- **Feature flag**: `RetryPolicyConfig.Enabled` (nil→true). Setting it to `false` in `config.json`
  disables the entire multi-attempt/backoff mechanism; combined with `MaxAttempts:1,
  InitialDelaySeconds:0` (the shipped defaults) the feature is behavior-neutral by default (AC7) —
  no separate rollout flag needed beyond the config field itself.
- **Rollback procedure**: revert `config.json`'s `retry_policy` key (or delete it — nil defaults
  restore today's exact behavior), or set `MaxAttempts:1` / `Enabled:false`. No data migration to
  reverse since persistence is additive JSON fields.
- **Staged rollout**: ship in the phase order below — Phase 1-2 (backend, behavior-neutral by
  default) can merge and run live before Phase 3 (proto/RPC) exists; Phase 4 (frontend) is
  purely additive/read-only until Phase 3's `RetrySession` RPC lands, so a partial deploy (backend
  merged, frontend not yet) is safe and simply shows no new UI.

## Unresolved Questions

- [ ] Should `backlog:work`-tagged sessions get a distinct (lower) `max_attempts` cap, or be fully
  excluded from the new multi-attempt policy, given `BacklogLifecycleListener`'s independent
  remediation gate (`session/backlog_lifecycle.go`) can also act on the same session
  (`research/features.md` "Unstated needs" #1)? — blocks Epic 2.1's gating logic if answered
  "exclude." **Recommended default, not blocking implementation**: apply `RetryPolicyConfig`
  uniformly (no new carve-out beyond the existing `isOneShot()` tags) since the shipped default
  `MaxAttempts:1` reproduces today's exact behavior for every session type including
  `backlog:work` (AC7); the double-remediation risk only newly appears once someone raises the
  global `MaxAttempts` above 1, which is a config change, not a code change — revisit before
  recommending `MaxAttempts > 1` as a new global default. Owner: Tyler.
- [ ] `MaxDelaySeconds` default (300s/5min, `research/architecture.md`'s placeholder) and jitter
  fraction — neither is specified in `requirements.md`'s functional requirements. Recommended:
  ship `MaxDelaySeconds: 300`, jitter `±10%` (`jitterFraction: 0.1`), both easily changed via
  config later without a proto/schema change. Owner: Tyler — blocks Task 1.2.2b only if a
  different value is wanted before merge.
- [ ] A live "retrying in Ns" countdown badge (`research/features.md`/`research/ux.md` both flag
  as a plausible addition, not one of `requirements.md`'s 9 functional requirements). Out of
  scope for this plan — the static "Attempt N/max" badge (AC4) satisfies the literal requirement.
  Owner: Tyler, deferred to a follow-on if wanted.
- [ ] Exact shape of the sibling `stale-session-detection` project's `StaleSessionConfig` —
  confirmed absent from `config/types.go` as of this plan (`grep -rn "StaleSessionConfig"
  config/` → no hits, verified 2026-08-06). Phase 5's `StaleTriggersRetry` consumer wiring is a
  stub (flag exists, unconsumed) until that sibling ships its config struct. Owner: whoever picks
  up `stale-session-detection` next.

## Dependency Visualization

```
Phase 1: Domain & Config Foundations
  Epic 1.1 (RetryState + pure fns)      Epic 1.2 (RetryPolicyConfig)
        │                                        │
        └───────────────┬────────────────────────┘
                         ▼
Phase 2: Driver Integration
  Epic 2.1 (signature migration) ──▶ Epic 2.2 (tmux_exited) ──▶ Epic 2.3 (evaluateSessionRetry +
                                                                  poll-loop backoff wiring)
                         │                                              │
                         ▼                                              ▼
                  Epic 2.4 (manual RetryNow, CAS guard) ◀────── Epic 2.5 (PermanentlyFailed status,
                         │                                        ADR-001)
                         ▼                                              │
                  Epic 2.6 (Notifier wiring) ◀───────────────────────────┘
                         │
                         ▼
                  Epic 2.7 (continuation prompt prefix)
                         │
                         ▼
Phase 3: Proto + RPC
  Epic 3.1 (Session read fields) ──▶ Epic 3.2 (RetrySession RPC) ──▶ Epic 3.3 (defaults wiring)
                         │
                         ▼
Phase 4: Frontend UI (depends on Phase 3's generated bindings)
  Epic 4.1 (badge) ── Epic 4.2 (history) ── Epic 4.3 (Retry now) ── Epic 4.4 (registry)
                         │
                         ▼
Phase 5: Stale-session integration stub (depends on Phase 2, sibling project — deferred)
                         │
                         ▼
Phase 6: Verification (depends on everything above)
```

---

## Phase 1: Domain & Config Foundations

### Epic 1.1: `RetryState` domain struct and pure functions

**Goal**: Introduce the new state shape and backoff/classification math as pure, independently
testable units before touching the driver.

#### Story 1.1.1: Define `RetryState`, `RetryAttemptRecord`, `backoffDelay`, `classifyFailureReason`

**As a** session driver, **I want** a typed, mutex-protected retry state on `Instance` and a pure
backoff-math function, **so that** the multi-attempt policy has somewhere to live and a formula
that's cheap to test in isolation.

**Acceptance Criteria**:
- A `RetryState` struct exists with fields `RetryAttempt int`, `RetryMaxAttempts int`,
  `LastFailureReason string`, `NextRetryAt time.Time`, `RetryHistory []RetryAttemptRecord`.
  - *Given* a fresh `RetryState{}` zero value, *When* `RetryAttempt` is read, *Then* it is `0`
    (no retries yet) and `NextRetryAt.IsZero()` is `true` (no retry pending).
- `backoffDelay(attempt, initial, max, jitterFraction)` returns `min(initial*2^attempt, max)`
  ± jitter, never negative, never exceeding `max*(1+jitterFraction)`.
  - *Given* `initial=10s, max=300s, jitterFraction=0`, *When* `backoffDelay(0, 10s, 300s, 0)`,
    `backoffDelay(1, 10s, 300s, 0)`, `backoffDelay(2, 10s, 300s, 0)` are called, *Then* they
    return `10s`, `20s`, `40s` respectively; `backoffDelay(10, 10s, 300s, 0)` returns exactly
    `300s` (cap boundary, not an overflowed/negative duration).

**Files**: `session/retry_state.go` (new)

##### Task 1.1.1a: Create `session/retry_state.go` with `RetryState`/`RetryAttemptRecord` struct definitions (~4 min)
- Define both structs with doc comments matching `ReviewState`'s style (field purpose,
  mutex-protection note referencing `inst.mu`).
- Files: `session/retry_state.go`

##### Task 1.1.1b: Add `backoffDelay` pure function (~4 min)
- Signature: `func backoffDelay(attempt int, initial, max time.Duration, jitterFraction float64) time.Duration`.
- Guard against `1<<attempt` overflow/negative-duration wraparound per `research/build-vs-buy.md` §1's example.
- Files: `session/retry_state.go`

##### Task 1.1.1c: Add `classifyFailureReason(inst *Instance) string` helper (~5 min)
- Returns `"tmux_exited"` when `inst`'s `TmuxProcessManager.DoesSessionExist()` is `false`; else `"crashed"`.
- Files: `session/retry_state.go` (calls into `session/tmux_process_manager.go:123`)

#### Story 1.1.2: Embed `RetryState` into `Instance`, add `retryInFlight` guard and `Notifier` wiring points

**Acceptance Criteria**:
- `Instance` has an embedded `RetryState` field promoting `inst.RetryAttempt`, etc.
  - *Given* a live `*Instance`, *When* `inst.RetryAttempt` is accessed, *Then* it compiles via
    Go's embedding field-promotion (no `inst.RetryState.RetryAttempt` needed), matching how
    `inst.LastViewed` already works for `ReviewState`.
- `Instance` has a `retryInFlight atomic.Bool` field and a `notifier Notifier` field with
  `SetNotifier`/`GetNotifier` accessors.

**Files**: `session/instance.go`, `session/instance_approval.go`

##### Task 1.1.2a: Embed `RetryState` into `Instance` (~3 min)
- Add `RetryState` as a value field next to `ReviewState` (`session/instance.go:311` region);
  extend the `mu` doc comment (`:365-368`) to also mention `RetryState`.
- Files: `session/instance.go`

##### Task 1.1.2b: Add `retryInFlight atomic.Bool` field (~3 min)
- Add next to `driverRunning atomic.Bool` (`session/instance.go:375` region) with a doc comment
  cross-referencing the CAS-guard rationale (mirrors the `driverRunning` "D3 mitigation" comment style).
- Files: `session/instance.go`

##### Task 1.1.2c: Add `notifier Notifier` field + `SetNotifier`/`GetNotifier` methods (~5 min)
- Clone `SetReviewQueue`/`GetReviewQueue` (`session/instance_approval.go:13,18`) exactly for shape/locking.
- Files: `session/instance_approval.go`

#### Story 1.1.3: Table-driven tests for the pure functions

##### Task 1.1.3a: `backoffDelay` table-driven test (~5 min)
- Cases: attempt 0..N monotonic growth, cap boundary (`d == max`), overflow guard (large `attempt`), jitter bounds.
- Files: `session/retry_state_test.go` (new)

##### Task 1.1.3b: `classifyFailureReason` test with a fake `TmuxProcessManager` (~5 min)
- Cases: `DoesSessionExist()==false` → `"tmux_exited"`; `==true` → `"crashed"`.
- Files: `session/retry_state_test.go`

---

### Epic 1.2: `RetryPolicyConfig` in the `config` package

**Goal**: Global-default config surface, mirroring `SessionRetentionConfig`'s shape exactly.

#### Story 1.2.1: Add `RetryPolicyConfig` struct + resolver methods

**As a** operator, **I want** a `retry_policy` section in `config.json` with a global default,
**so that** I can tune `max_attempts`/backoff without a code change, and so an old config file
silently preserves today's behavior.

**Acceptance Criteria**:
- `RetryPolicyConfig{Enabled *bool, MaxAttempts int, Backoff string, InitialDelaySeconds int,
  MaxDelaySeconds int, RetryOn []string, StaleTriggersRetry *bool}` exists in `config/types.go`.
  - *Given* a `config.json` with no `"retry_policy"` key at all (pre-feature file), *When*
    `LoadConfigFromPath` unmarshals it, *Then* `cfg.RetryPolicy.Enabled` is `nil` and
    `cfg.RetryPolicy.EnabledOrDefault()` returns `true` — matching AC7.

**Files**: `config/types.go`

##### Task 1.2.1a: Add `RetryPolicyConfig` struct (~5 min)
- Mirror `SessionRetentionConfig` (`config/types.go:38-45`) field-for-field shape/JSON tags.
- Files: `config/types.go`

##### Task 1.2.1b: Add `EnabledOrDefault()`, `MaxAttemptsOrDefault()`, `RetryOnOrDefault()` methods (~5 min)
- `EnabledOrDefault()` defaults `true` when nil (mirrors `SessionRetentionConfig.EnabledOrDefault`, `config/types.go:48-51`).
- `MaxAttemptsOrDefault()` defaults `1` when `MaxAttempts <= 0`.
- `RetryOnOrDefault()` defaults `["crashed","stalled","tmux_exited"]` when `RetryOn` is empty.
- Files: `config/types.go`

#### Story 1.2.2: Wire `RetryPolicy` into `Config` + defaults function

**Acceptance Criteria**:
- *Given* a brand-new `DefaultConfig()`-equivalent call, *When* `cfg.RetryPolicy` is inspected,
  *Then* it equals `RetryPolicyConfig{Enabled: boolPtr(true), MaxAttempts: 1, Backoff:
  "exponential", InitialDelaySeconds: 0, MaxDelaySeconds: 300}` — reproducing today's immediate,
  single-retry behavior exactly.

**Files**: `config/config.go`

##### Task 1.2.2a: Add `RetryPolicy RetryPolicyConfig` field to `Config` struct (~2 min)
- Add next to `SessionRetention` (`config/config.go:343` region), `json:"retry_policy,omitempty"`.
- Files: `config/config.go`

##### Task 1.2.2b: Add default assignment in the defaults-building function (~3 min)
- Add `cfg.RetryPolicy = RetryPolicyConfig{Enabled: boolPtr(true), MaxAttempts: 1, Backoff:
  "exponential", InitialDelaySeconds: 0, MaxDelaySeconds: 300}` near `cfg.Hibernation = ...`
  (`config/config.go:457` region).
- Files: `config/config.go`

#### Story 1.2.3: Config-layer tests

##### Task 1.2.3a: Test `EnabledOrDefault`/`MaxAttemptsOrDefault` nil vs. explicit-false/zero cases (~4 min)
- Files: `config/types_test.go` (or existing config test file for `SessionRetentionConfig`)

##### Task 1.2.3b: Test `LoadConfigFromPath` round-trips an old config.json missing `retry_policy` and resolves to AC7 defaults (~5 min)
- Files: `config/config_test.go`

---

## Phase 2: Driver Integration

### Epic 2.1: Replace `atomic.Bool` with `RetryState` + resolved `RetryPolicy`

**Goal**: Migrate the existing single-retry mechanism onto the new state shape without changing
default behavior (AC7).

#### Story 2.1.1: Function signature migration

**As a** session driver, **I want** `handleDriverFailure` and its callers to read/write
`inst.RetryState` instead of threading `*atomic.Bool`, **so that** the retry count survives a
goroutine restart and can express more than one bit of state.

**Acceptance Criteria**:
- *Given* `runSessionDriver`, `runSessionDriverWithPrompt`, `handleDriverFailure` after this
  story, *When* grepped for `atomic.Bool`, *Then* zero occurrences remain in
  `session/session_driver.go` (the parameter is fully replaced by `inst.RetryState` + a passed
  `RetryPolicyConfig` value).

**Files**: `session/session_driver.go`

##### Task 2.1.1a: Change `runSessionDriver`/`runSessionDriverWithPrompt`/`handleDriverFailure` signatures (~5 min)
- Drop `retried *atomic.Bool` param; add `policy RetryPolicyConfig` param to each.
- Files: `session/session_driver.go` (lines 110, 125, 509 regions)

##### Task 2.1.1b: Update the 3 call sites referencing `retried.Load()`/`CompareAndSwap` (~5 min)
- Lines ~203, ~216 (`retried.Load()`) → read `inst.RetryState.RetryAttempt >= inst.RetryState.RetryMaxAttempts` under `inst.mu.RLock()`.
- Files: `session/session_driver.go`

##### Task 2.1.1c: `StartSessionDriver` resolves the effective policy once (~5 min)
- Compute `effectivePolicy := resolveRetryPolicy(globalCfg.RetryPolicy, inst.RetryPolicyOverride)` and pass down.
- Files: `session/session_driver.go` (line 75 region)

#### Story 2.1.2: Per-session override resolution

**Acceptance Criteria**:
- *Given* `Instance.RetryPolicyOverride = &RetryPolicyConfig{MaxAttempts: 5}` and global
  `cfg.RetryPolicy.MaxAttempts = 1`, *When* `resolveRetryPolicy` runs at driver start, *Then* the
  resolved policy's `MaxAttempts` is `5` (override wins, mirrors `ReworkCapOverride`'s
  nil-means-inherit semantics).

**Files**: `session/instance.go`, `session/retry_state.go`

##### Task 2.1.2a: Add `RetryPolicyOverride *RetryPolicyConfig` field to `Instance` (~3 min)
- Mirrors `ReworkCapOverride *int` (`session/repository.go:374-379`).
- Files: `session/instance.go`

##### Task 2.1.2b: Write `resolveRetryPolicy(global config.RetryPolicyConfig, override *config.RetryPolicyConfig) config.RetryPolicyConfig` (~4 min)
- Files: `session/retry_state.go`

#### Story 2.1.3: Migrate existing tests off `atomic.Bool`

##### Task 2.1.3a: Rewrite `TestSessionDriver_SecondFailure_MarksNeedsAttention` (~5 min)
- Assert against `inst.RetryState.RetryAttempt`/`inst.Status == PermanentlyFailed` instead of the removed `atomic.Bool`. Per `.claude/rules/fix-flaky-tests-dont-defer.md`'s spirit: re-derive the equivalent assertion, don't skip/relax it.
- Files: `session/session_driver_test.go`

##### Task 2.1.3b: Update `TestStartSessionDriver_Idempotent` for the new signature (~3 min)
- Files: `session/session_driver_test.go`

---

### Epic 2.2: `tmux_exited` classification + restart-grace exemption

**Goal**: Distinguish pane/session loss from a plain process crash, and don't let a routine
service restart count as N simultaneous crashes.

#### Story 2.2.1: Wire `classifyFailureReason` into the two exit-detection call sites

**As a** session driver, **I want** the failure reason passed to `handleDriverFailure` to be
`"tmux_exited"`, `"crashed"`, or `"stalled"` (never a free-text string), **so that** `RetryOn`
can gate each independently (AC2).

**Acceptance Criteria**:
- *Given* a session whose tmux session no longer exists (`DoesSessionExist()==false`) at the
  moment `st == Stopped` is observed with `sentInitial==true`, *When* the driver reaches the
  exit-after-initial-prompt branch, *Then* `handleDriverFailure` is called with reason
  `"tmux_exited"`, not the old literal `"unexpected exit"`.

**Files**: `session/session_driver.go`

##### Task 2.2.1a: Replace literal reason strings at the two exit branches (~5 min)
- Lines ~209 (`"exit before initial prompt"`) and ~236 (`"unexpected exit"`) call
  `classifyFailureReason(inst)` instead, feeding the result into `handleDriverFailure`.
- Files: `session/session_driver.go`

##### Task 2.2.1b: Rename the inactivity-timeout branch's literal reason to `"stalled"` (~3 min)
- Line ~423 currently passes `"inactivity timeout"` — rename to the 3-value vocabulary's `"stalled"` (unambiguous, not a process-exit path).
- Files: `session/session_driver.go`

#### Story 2.2.2: Restart-grace exemption for `tmux_exited`

**As a** session driver, **I want** a `tmux_exited` failure detected shortly after process start
to not consume a retry attempt, **so that** a `make install-service` deploy killing every tmux
session at once doesn't burn every session's attempt budget simultaneously (AC1 must not
regress into a thundering herd).

**Acceptance Criteria**:
- *Given* `serverStartTime = T0` and a session whose `tmux_exited` failure is detected at
  `T0 + 5s` (within a short grace window), *When* `evaluateSessionRetry` runs, *Then* it returns
  `restartGrace` (retry granted, `RetryAttempt` NOT incremented) rather than consuming attempt 1
  of `MaxAttempts`.

**Files**: `session/session_driver.go` or `session/retry_state.go`

##### Task 2.2.2a: Add package-level `serverStartTime = time.Now()` (~2 min)
- Mirror `session/backlog_remediation.go:51`'s pattern (captured once at init).
- Files: `session/session_driver.go`

##### Task 2.2.2b: Grant restart-grace for `tmux_exited` within the window (~5 min)
- In `evaluateSessionRetry` (Epic 2.3), branch: `reason == "tmux_exited" && failureTime.Sub(serverStartTime) < gracePeriod` → `restartGrace`.
- Files: `session/retry_state.go`

---

### Epic 2.3: `evaluateSessionRetry` decision function + poll-loop backoff wiring

**Goal**: The actual multi-attempt gate — the pure decision function plus the non-blocking
tick-check that replaces immediate restart with a backed-off, interruptible wait.

#### Story 2.3.1: Pure decision function

**As a** session driver, **I want** a pure function that decides "schedule a retry / not
eligible / exhausted / restart-grace" given the current `RetryState` and `RetryPolicyConfig`,
**so that** the exponential-backoff/attempt-cap/`retry_on` logic is unit-testable without a real
tmux session.

**Acceptance Criteria**:
- *Given* `RetryState{RetryAttempt: 2, RetryMaxAttempts: 3}` and `RetryPolicyConfig{RetryOn:
  ["crashed"]}`, *When* `evaluateSessionRetry(rs, policy, "tmux_exited", now, bootTime)` is
  called, *Then* it returns `notEligible` (reason not in `RetryOn`) — AC2.
- *Given* `RetryState{RetryAttempt: 3, RetryMaxAttempts: 3}` and `RetryPolicyConfig{RetryOn:
  ["crashed"]}`, *When* `evaluateSessionRetry(rs, policy, "crashed", now, bootTime)` is called,
  *Then* it returns `exhausted` — AC1's terminal transition.

**Files**: `session/retry_state.go`

##### Task 2.3.1a: Write `evaluateSessionRetry` (~5 min)
- Mirror `evaluateRemediation`'s shape (`session/backlog_remediation.go:96-110`): pure function,
  no I/O, returns one of `retryDecisionScheduled` / `retryDecisionNotEligible` /
  `retryDecisionExhausted` / `retryDecisionRestartGrace`.
- Files: `session/retry_state.go`

##### Task 2.3.1b: Table-driven tests for `evaluateSessionRetry` covering AC1/AC2/AC7 (~5 min)
- Files: `session/retry_state_test.go`

#### Story 2.3.2: Wire the decision into `handleDriverFailure`

**Acceptance Criteria**:
- *Given* a `scheduled` decision with `attempt=1, initial=10s`, *When* `handleDriverFailure`
  processes it, *Then* `inst.RetryState.NextRetryAt` is set to `~now+10s`, a new
  `RetryAttemptRecord{Attempt:1, Reason:"crashed", Timestamp:now}` is appended to
  `RetryHistory`, and no restart happens on this call — AC3's "reuse worktree" restart is
  deferred to the tick-check.

**Files**: `session/session_driver.go`

##### Task 2.3.2a: Branch `handleDriverFailure` on `evaluateSessionRetry`'s result (~5 min)
- `scheduled` → set `NextRetryAt` + append history under `inst.mu`, return (no immediate restart).
- `notEligible`/`exhausted` → call `markSessionPermanentlyFailed` (Epic 2.5).
- `restartGrace` → call `restartForRetry` immediately without incrementing `RetryAttempt`.
- Files: `session/session_driver.go`

#### Story 2.3.3: Poll-loop tick-check for pending backoff

**As a** user, **I want** a session mid-backoff-wait to remain stoppable/manually-retryable,
**so that** requirement 2's "must not block the driver goroutine" constraint holds.

**Acceptance Criteria**:
- *Given* `inst.RetryState.NextRetryAt = now+30s`, *When* the poll loop's ticker fires (every
  2s) before that deadline, *Then* the tick is a no-op (`continue`) — `Stop`/`Pause` checks
  earlier in the loop still run every tick, so a manual stop during the wait works exactly as it
  does today via the loop's existing `st == Paused`/`st == Stopped` branches.
- *Given* `NextRetryAt` has just elapsed, *When* the next tick fires, *Then* `restartForRetry` is
  invoked and `NextRetryAt` is cleared.

**Files**: `session/session_driver.go`

##### Task 2.3.3a: Add the `NextRetryAt` gate near the top of the ticker loop (~4 min)
- After the existing `Paused`/`Stopped` checks (`session_driver.go:194-238` region), add:
  `if !inst.RetryState.NextRetryAt.IsZero() && time.Now().Before(inst.RetryState.NextRetryAt) { continue }`.
- Files: `session/session_driver.go`

##### Task 2.3.3b: On elapsed `NextRetryAt`, invoke `restartForRetry` and clear the timestamp (~5 min)
- Files: `session/session_driver.go`

#### Story 2.3.4: Extract shared `restartForRetry` helper

**Acceptance Criteria**:
- *Given* both the poll-loop's backoff-elapsed path and (Epic 2.4's) `RetryNow()`, *When* either
  calls `restartForRetry(inst, allowedPath, reason)`, *Then* both execute the identical
  `inst.RecoverFromStopped()`/`inst.StopController()`/`inst.Start(false)`/`inst.StartController()`
  (or `inst.Restart(false)`) sequence — no duplicated restart logic between the two entry points.

**Files**: `session/session_driver.go`

##### Task 2.3.4a: Extract `restartForRetry` from `handleDriverFailure`'s restart block (~5 min)
- Move lines ~534-552's restart logic into a standalone function; `handleDriverFailure` and
  `RetryNow()` (Epic 2.4) both call it.
- Files: `session/session_driver.go`

---

### Epic 2.4: Manual "Retry now" with CAS guard

**Goal**: A second trigger path into the same restart machinery that can never race the
automated one (AC6).

#### Story 2.4.1: `RetryNow()` domain method

**As a** user, **I want** a "Retry now" action that works from mid-backoff or from
`PermanentlyFailed`, **so that** I don't have to wait for the computed delay or accept a
terminal give-up.

**Acceptance Criteria**:
- *Given* `Instance{Status: PermanentlyFailed, RetryState:{RetryAttempt:3, RetryMaxAttempts:3}}`,
  *When* `inst.RetryNow()` is called, *Then* `RetryState` resets to `{RetryAttempt:0,
  NextRetryAt: zero}`, `Status` transitions to `Active`, and `restartForRetry` runs — AC6.
- *Given* a `RetryNow()` call racing an automated backoff-expiry restart already in flight
  (`retryInFlight` already `true`), *When* the second caller's CAS fails, *Then* `RetryNow()`
  returns `ErrRetryInFlight` without touching `RetryState` or calling `inst.Restart()` twice.

**Files**: `session/retry_state.go`

##### Task 2.4.1a: Implement `RetryNow()` (~5 min)
- CAS `retryInFlight` (false→true), reset `RetryState` under `inst.mu`, transition `Status`
  `PermanentlyFailed`/`Stopped` → `Active`, call `restartForRetry`, `defer` release `retryInFlight`.
- Files: `session/retry_state.go`

##### Task 2.4.1b: Define and return `ErrRetryInFlight` on CAS failure (~3 min)
- Files: `session/retry_state.go`

#### Story 2.4.2: Concurrency test

##### Task 2.4.2a: Test `RetryNow()` vs. a concurrent automated restart (~5 min)
- Two goroutines racing `retryInFlight`'s CAS; assert exactly one proceeds, the other returns `ErrRetryInFlight`.
- Files: `session/retry_state_test.go`

---

### Epic 2.5: `PermanentlyFailed` terminal state (ADR-001)

**Goal**: Implement the decision recorded in ADR-001 with its full blast-radius sweep.

#### Story 2.5.1: Add the `Status`/`SessionStatus` enum value

**Acceptance Criteria**:
- *Given* `Status.String()` after this story, *When* called on `PermanentlyFailed`, *Then* it
  returns `"PermanentlyFailed"` (not the `default:` `Status(6)` fallback).

**Files**: `session/instance.go`, `session/instance_status.go`, `session/review_queue_poller.go`, `proto/session/v1/types.proto`

##### Task 2.5.1a: Add `PermanentlyFailed Status = 6` (~2 min)
- Files: `session/instance.go` (const block near line 39)

##### Task 2.5.1b: Add case to `Status.String()` (~2 min)
- Files: `session/instance.go` (line 58 region)

##### Task 2.5.1c: Add case to `GetStatusDescription` (~2 min)
- Files: `session/instance_status.go` (line 136 region)

##### Task 2.5.1d: Add case to `reconcileSessions` (~5 min)
- Decision: do **not** auto-revive a `PermanentlyFailed` session even if tmux is alive (unlike
  `Stopped`'s "revive to Active" branch at line 460) — it's a deliberate terminal state pending
  human action via "Retry now," not an incidental stop.
- Files: `session/review_queue_poller.go` (line 460 region)

##### Task 2.5.1e: Add `SESSION_STATUS_PERMANENTLY_FAILED = 10;` to the proto enum (~2 min)
- With a "terminal state" doc comment matching `SESSION_STATUS_STOPPED`'s style.
- Files: `proto/session/v1/types.proto` (line 350 region, after `SESSION_STATUS_RESTORING = 9`)

#### Story 2.5.2: `markSessionPermanentlyFailed`

**Acceptance Criteria**:
- *Given* a session transitioning into `PermanentlyFailed` for the first time this failure
  episode, *When* `markSessionPermanentlyFailed` runs, *Then* `Status` becomes
  `PermanentlyFailed`, a `ReviewQueue.Add` call fires (unchanged `ReasonStale`, per ADR-001), and
  `inst.notifier.Notify(...)` fires exactly once.
- *Given* the session is already `PermanentlyFailed` (e.g. a second read/reconcile pass observes
  the same state), *When* any code path re-evaluates it, *Then* `Notify` is NOT called again
  (edge-triggered, `research/pitfalls.md` §5) — only "Retry now" re-arms the one-shot.

**Files**: `session/session_driver.go`

##### Task 2.5.2a: Implement `markSessionPermanentlyFailed(inst *Instance, reason string)` (~5 min)
- Replaces the give-up branch's `markSessionNeedsAttention` call. Transition `Status` under
  `inst.mu`; keep the existing `rq.Add(&ReviewItem{Reason: ReasonStale, ...})` call unchanged.
- Files: `session/session_driver.go`

##### Task 2.5.2b: Guard the notification to fire only on the transition edge (~4 min)
- Check `inst.Status != PermanentlyFailed` before mutating; only call `Notify` when the check
  was true (i.e. this call is the actual transition, not a re-observation).
- Files: `session/session_driver.go`

#### Story 2.5.3: Frontend/Go blast-radius sweep

##### Task 2.5.3a: Add `PERMANENTLY_FAILED` handling to the status→color/label mapping function (~5 min)
- Locate the function backing `SessionCard.tsx:516-522`'s `getStatusColor`/`getStatusText` (or
  equivalent) and add the new case using `error`/`errorBg` tokens per `research/ux.md` §4.
- Files: `web-app/src/components/sessions/SessionCard.tsx` (or its shared status-mapping module if extracted)

##### Task 2.5.3b: Verify `SessionActionsOverflow`'s `isStopped`/`isRunning` booleans handle `PermanentlyFailed` correctly (~5 min)
- `PermanentlyFailed` should behave like `isStopped` for button-visibility purposes (no
  double-showing both "Restart" and "Retry now"); confirm/adjust the boolean derivations.
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

---

### Epic 2.6: `Notifier` wiring on `Instance`

**Goal**: Give `Instance` its own optional `Notifier` reference at the same 4 sites
`SetReviewQueue` is already wired, per `research/architecture.md` §7.

#### Story 2.6.1: Wire `SetNotifier` at the 4 existing `SetReviewQueue` call sites

**Acceptance Criteria**:
- *Given* the server's dependency-wiring startup path, *When* an `Instance` is constructed and
  `SetReviewQueue` is called on it, *Then* `SetNotifier` is called immediately alongside it at
  the same 4 sites.

**Files**: `server/dependencies.go`, `server/services/session_service.go`

##### Task 2.6.1a: `server/dependencies.go:624` — add `inst.SetNotifier(notifier)` (~3 min)

##### Task 2.6.1b: `server/dependencies.go:832` — same (~3 min)

##### Task 2.6.1c: `server/services/session_service.go:431` — same (~3 min)

##### Task 2.6.1d: `server/services/session_service.go:799` — same (~3 min)
- (Verified actual line — `research/architecture.md` cited `:790`, which had drifted.)

---

### Epic 2.7: Continuation-prompt failure-reason prefix

**Goal**: AC3's "the resumed session's first message states the failure reason."

#### Story 2.7.1: Prepend "Previous attempt failed due to {reason}."

**Acceptance Criteria**:
- *Given* `RetryState.LastFailureReason = "tmux_exited"` and the existing JSONL-derived
  continuation text `"...continuing from the last assistant turn..."`, *When* `restartForRetry`
  builds the prompt for the next attempt, *Then* the injected prompt is `"Previous attempt
  failed due to tmux_exited. ...continuing from the last assistant turn..."`.

**Files**: `session/session_driver.go`

##### Task 2.7.1a: Prepend the reason line in `restartForRetry`/`buildContinuationPrompt`'s caller (~5 min)
- Files: `session/session_driver.go` (line 601 region)

##### Task 2.7.1b: Test both the JSONL-derived and fallback-to-`InitialPrompt` paths get the prefix (~4 min)
- Files: `session/session_driver_test.go`

---

## Phase 3: Proto + RPC

### Epic 3.1: Session read-side proto fields + `RetryPolicyConfig` message

**Goal**: Surface `RetryState` and the resolved policy on the wire.

#### Story 3.1.1: Add fields to `message Session`

**Acceptance Criteria**:
- *Given* the regenerated `Session` proto, *When* a session with `RetryState{RetryAttempt:2,
  RetryMaxAttempts:3}` is converted via the adapter, *Then* `session.retry_attempt == 2` and
  `session.retry_max_attempts == 3` on the wire.

**Files**: `proto/session/v1/types.proto`

##### Task 3.1.1a: Add `retry_attempt=72`, `retry_max_attempts=73`, `last_failure_reason=74`, `next_retry_at=75` (google.protobuf.Timestamp), `retry_history=76` (repeated `RetryAttemptRecord`) (~5 min)
- Verified next-free field number on `message Session` is `72` (highest existing is `71`).
- Files: `proto/session/v1/types.proto`

##### Task 3.1.1b: Add nested `message RetryAttemptRecord { int32 attempt = 1; string reason = 2; google.protobuf.Timestamp timestamp = 3; }` (~3 min)
- Files: `proto/session/v1/types.proto`

#### Story 3.1.2: `RetryPolicyConfig` proto message + wiring into `SessionDefaultsConfig`/`CreateSessionRequest`

**Acceptance Criteria**:
- *Given* the regenerated `SessionDefaultsConfig`, *When* the frontend fetches session defaults,
  *Then* a `retry_policy` field is present carrying the resolved global default.
- *Given* a `CreateSessionRequest` with `retry_policy_override` set, *When* the request is sent,
  *Then* the field round-trips through `make proto-gen`'s generated Go/TS bindings.

**Files**: `proto/session/v1/session.proto`

##### Task 3.1.2a: Add `message RetryPolicyConfig` (~5 min)
- `bool enabled=1; int32 max_attempts=2; string backoff=3; int32 initial_delay_seconds=4; int32 max_delay_seconds=5; repeated string retry_on=6; bool stale_triggers_retry=7;`
- Files: `proto/session/v1/session.proto` (near `message SessionDefaultsConfig`)

##### Task 3.1.2b: Add `RetryPolicyConfig retry_policy = 12;` to `SessionDefaultsConfig` (~2 min)
- Verified next-free field number is `12` (highest existing is `11`, `max_concurrent_backlog_work_items`).
- Files: `proto/session/v1/session.proto`

##### Task 3.1.2c: Add `optional RetryPolicyConfig retry_policy_override = 28;` to `CreateSessionRequest` (~2 min)
- Verified next-free field number is `28` (highest existing is `27`, `alias_name`).
- Files: `proto/session/v1/session.proto`

#### Story 3.1.3: Regenerate bindings

##### Task 3.1.3a: Run `make proto-gen` (~3 min)
- Verify `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` regenerate cleanly; commit both (tracked despite `.gitignore` per CLAUDE.md).
- Files: `session/gen/session/v1/*.go`, `web-app/src/gen/session/v1/*_pb.ts`

##### Task 3.1.3b: `go build ./...` to confirm generated code compiles (~2 min)

---

### Epic 3.2: `RetrySession` RPC

**Goal**: The backend endpoint the "Retry now" UI action calls.

#### Story 3.2.1: Proto RPC definition

**Acceptance Criteria**:
- *Given* the regenerated service client, *When* `RetrySession({id: "sess-6"})` is called,
  *Then* the generated TS client exposes it identically in shape to `RestartSession`.

**Files**: `proto/session/v1/session.proto`

##### Task 3.2.1a: Add `rpc RetrySession(RetrySessionRequest) returns (RetrySessionResponse) {}` (~3 min)
- Next to `RestartSession` (line 106 region).
- Files: `proto/session/v1/session.proto`

##### Task 3.2.1b: Add `message RetrySessionRequest { string id = 1; }` / `message RetrySessionResponse { Session session = 1; bool success = 2; string message = 3; }` (~3 min)
- Near `RestartSessionRequest`/`Response` (line 1173 region).
- Files: `proto/session/v1/session.proto`

#### Story 3.2.2: Go handler

**Acceptance Criteria**:
- *Given* a `RetrySessionRequest{id: "sess-6"}` for a `PermanentlyFailed` session, *When*
  `SessionService.RetrySession` handles it, *Then* it calls `inst.RetryNow()` (not
  `inst.Restart()`), persists via `SaveInstances`, publishes `SessionUpdatedEvent`, and returns
  the updated `Session` proto with `success: true` — mirroring `RestartSession`'s tail exactly.

**Files**: `server/services/session_service.go`

##### Task 3.2.2a: Implement `SessionService.RetrySession` (~5 min)
- Clone `RestartSession`'s instance-lookup shape (`FindLiveInstance` → `loadInstancesWithWiring` fallback), call `inst.RetryNow()`.
- Files: `server/services/session_service.go`

##### Task 3.2.2b: Publish event + persist (~4 min)
- `events.NewSessionUpdatedEvent(instance, []string{"status", "updated_at", "retry_attempt"})`, `s.storage.SaveInstances(...)`.
- Files: `server/services/session_service.go`

##### Task 3.2.2c: Map `ErrRetryInFlight` to `connect.CodeFailedPrecondition` (~3 min)
- Files: `server/services/session_service.go`

#### Story 3.2.3: Handler test

##### Task 3.2.3a: Table-driven test for `RetrySession` (~5 min)
- Cases: from mid-backoff, from `PermanentlyFailed`, concurrent double-call → `FailedPrecondition`.
- Files: `server/services/session_service_test.go`

---

### Epic 3.3: Config → frontend defaults wiring

#### Story 3.3.1: `sessionDefaultsToProto` extension

**Acceptance Criteria**:
- *Given* `cfg.RetryPolicy = RetryPolicyConfig{MaxAttempts: 1, ...}`, *When*
  `sessionDefaultsToProto(cfg)` runs, *Then* the returned `SessionDefaultsConfig.RetryPolicy`
  carries the resolved values (matching the existing `MaxAutoReworkIterationsOrDefault()` idiom).

**Files**: `server/services/defaults_service.go`

##### Task 3.3.1a: Extend `sessionDefaultsToProto` (~5 min)
- Files: `server/services/defaults_service.go` (line 496 region)

##### Task 3.3.1b: Add/extend a settings-update path so the frontend can persist a changed global `RetryPolicy` (~5 min)
- Find and extend whatever existing handler saves `MaxAutoReworkIterations`-style settings.
- Files: `server/services/defaults_service.go`

---

## Phase 4: Frontend UI

### Epic 4.1: Retry-count badge (`SessionCard.tsx`)

#### Story 4.1.1: Compact `RetryBadge`

**As a** user scanning 5-10 session cards, **I want** a compact "Attempt N/max" badge, **so
that** I can tell at a glance which sessions are self-healing without opening them (AC4).

**Acceptance Criteria**:
- *Given* `session.retryAttempt === 0` (no retry yet), *When* `SessionCard` renders, *Then* no
  `RetryBadge` appears at all (per `research/ux.md` §4's "attempt 1, no badge" rule).
- *Given* `session.retryAttempt === 2, session.retryMaxAttempts === 3`, *When* `SessionCard`
  renders, *Then* a compact badge reading `"🔁 2/3"` appears as a peer of `ReviewQueueBadge`.

**Files**: `web-app/src/components/sessions/RetryBadge.tsx` (new), `web-app/src/components/sessions/SessionCard.tsx`

##### Task 4.1.1a: New `RetryBadge` component (~5 min)
- Clone `ReviewQueueBadge.tsx`'s `compact`/full split. Files: `web-app/src/components/sessions/RetryBadge.tsx`

##### Task 4.1.1b: Render as a peer in `SessionCard.tsx`'s header badge row (~5 min)
- Gated on `session.retryAttempt > 0`; position after `ReviewQueueBadge` (line 493-548 region).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 4.1.1c: Apply 3-tier severity styling (~5 min)
- No badge (attempt 1) → neutral (attempt 2+, headroom) → warning tokens (final attempt), reusing the `memoryBadge` CSS-token pattern (`SessionCard.tsx:552-554`).
- Files: `web-app/src/components/sessions/RetryBadge.css.ts` (new, vanilla-extract per `.claude/rules/css-architecture.md`)

#### Story 4.1.2: `PermanentlyFailed` primary status badge

**Acceptance Criteria**:
- *Given* `session.status === SessionStatus.PERMANENTLY_FAILED`, *When* the primary status badge
  renders, *Then* it uses `error`/`errorBg` tokens (not `warning`) with label "Failed — gave up
  after N attempts", in the same leftmost slot `STOPPED`/`PAUSED` use — never sharing color/icon
  with routine `NeedsAttention` reasons (the core correctness requirement of `research/ux.md`).

**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 4.1.2a: Add `PERMANENTLY_FAILED` case to the status→color/label mapping (~5 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx` (line 516-522 region)

#### Story 4.1.3: Accessibility

##### Task 4.1.3a: `role="img"` + `aria-label` on `RetryBadge` (~3 min)
- `aria-label={\`Retry attempt ${n} of ${max}\`}`, icon `aria-hidden="true"`, matching every sibling badge's convention.
- Files: `web-app/src/components/sessions/RetryBadge.tsx`

##### Task 4.1.3b: Jest test: no render at attempt 0, correct `aria-label` at attempt 2/3 (~5 min)
- Files: `web-app/src/components/sessions/RetryBadge.test.tsx` (new)

---

### Epic 4.2: Retry history (`SessionDetailView.tsx`)

#### Story 4.2.1: `RetryHistoryList` component

**As a** user, **I want** to see reason + timestamp per past retry attempt, **so that** I can
tell whether a session has been quietly crash-looping (AC5).

**Acceptance Criteria**:
- *Given* `session.retryHistory = [{attempt:1, reason:"crashed", timestamp:"2026-08-06T10:00:00Z"},
  {attempt:2, reason:"tmux_exited", timestamp:"2026-08-06T10:05:00Z"}]`, *When*
  `RetryHistoryList` renders, *Then* it shows 2 items, newest first: attempt 2 above attempt 1.
- *Given* `session.retryHistory = []`, *When* `RetryHistoryList` renders, *Then* an empty state
  ("No retries yet") is shown, matching `CheckpointList.tsx`'s empty-state convention.

**Files**: `web-app/src/components/sessions/RetryHistoryList.tsx` (new), `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 4.2.1a: New `RetryHistoryList` component (~5 min)
- Clone `CheckpointList.tsx`'s shape verbatim (newest-first, `MAX_VISIBLE=10`, show-all toggle, `formatRelativeTime`, empty state, `aria-label` on `<ul>`).
- Files: `web-app/src/components/sessions/RetryHistoryList.tsx`

##### Task 4.2.1b: Add as a peer section near checkpoints in `SessionDetailView.tsx` (~5 min)
- Files: `web-app/src/components/sessions/SessionDetailView.tsx`

#### Story 4.2.2: Tests

##### Task 4.2.2a: Jest test for empty state + populated ordering (~5 min)
- Files: `web-app/src/components/sessions/RetryHistoryList.test.tsx` (new)

---

### Epic 4.3: Manual "Retry now" action

#### Story 4.3.1: `SessionActionsOverflow` wiring

**As a** user, **I want** a "Retry now" button/menu-item that mirrors the existing "Restart"
interaction, **so that** the confirm-and-act flow feels consistent with every other override
action (AC6).

**Acceptance Criteria**:
- *Given* `session.status === PERMANENTLY_FAILED` and `showPrimaryAction`, *When*
  `SessionActionsOverflow` renders, *Then* a primary "🔁 Retry now" button appears in the same
  slot the "🔄 Restart" button uses for `isStopped` sessions (line 537 region), with confirm
  copy "This session gave up after N attempts — retry anyway?".
- *Given* a session mid-backoff-wait (`session.nextRetryAt` set, status not yet
  `PERMANENTLY_FAILED`), *When* the overflow menu (Group 5) opens, *Then* a "Retry now" menu
  item appears with confirm copy "Skip the wait and retry now?".

**Files**: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 4.3.1a: Add `onRetryNow` prop + `isRetryConfirmOpen` state (clone `isRestartConfirmOpen`) (~5 min)
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 4.3.1b: Primary-button shortcut for `PERMANENTLY_FAILED` (~5 min)
- Mirrors `showPrimaryAction && isStopped && onRestart` (line 537).
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 4.3.1c: Overflow-menu item (Group 5) for mid-backoff sessions (~5 min)
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 4.3.1d: Confirm-dialog copy branching (~5 min)
- Clone `isRestartConfirmOpen`'s `useFocusTrap`/`dangerButton`/portal dialog shape (line 288-311 region); two copy variants per the acceptance criteria above.
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

#### Story 4.3.2: RPC wiring

##### Task 4.3.2a: Add `retrySession` call to `useSessionService.ts` (~5 min)
- Files: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 4.3.2b: Wire `onRetryNow` through `SessionCard.tsx` → `SessionActionsOverflow` (~4 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx`

#### Story 4.3.3: Tests

##### Task 4.3.3a: RTL test: confirm-dialog copy differs between mid-backoff and `PERMANENTLY_FAILED` entry points (~5 min)
- Files: `web-app/src/components/sessions/SessionActionsOverflow.test.tsx`

##### Task 4.3.3b: e2e test `tests/e2e/session-retry.spec.ts` (~5 min)
- `// @feature session:retry`; exercises the "Retry now" button + confirm dialog via `data-testid` locators, no `waitForTimeout`.
- Files: `tests/e2e/session-retry.spec.ts` (new)

---

### Epic 4.4: Feature registry

#### Story 4.4.1: Registry entries

##### Task 4.4.1a: `docs/registry/features/backend/session-retry.json` for `RetrySession` (~4 min)
- With `// +api: session:retry` marker added to the handler.
- Files: `docs/registry/features/backend/session-retry.json`, `server/services/session_service.go`

##### Task 4.4.1b: `docs/registry/features/frontend/retry-badge.json` and `retry-history.json` (~4 min)
- Files: `docs/registry/features/frontend/retry-badge.json`, `docs/registry/features/frontend/retry-history.json`

##### Task 4.4.1c: `make registry-generate` and commit changed aggregate files (~2 min)

---

## Phase 5: Stale-session integration (deferred consumer stub)

### Epic 5.1: `StaleTriggersRetry` stub

#### Story 5.1.1: Confirm sibling config absence and stub the flag

**Acceptance Criteria**:
- *Given* `config/types.go` at implementation time, *When* grepped for `StaleSessionConfig`,
  *Then* either (a) it's still absent — the `StaleTriggersRetry *bool` field exists on
  `RetryPolicyConfig` (added in Task 1.2.1a) but is unconsumed, with a comment pointing at
  `project_plans/stale-session-detection/research/architecture.md` §2 for the exact field to
  wire once it ships; or (b) it has shipped — wire the 4th trigger branch per Task 5.1.1b.

**Files**: `session/session_driver.go`, `config/types.go`

##### Task 5.1.1a: Add TODO-linked comment on `StaleTriggersRetry` if the sibling config is still absent (~4 min)
- Files: `config/types.go`

##### Task 5.1.1b: (Conditional — only if `StaleSessionConfig` has shipped) wire the 4th trigger branch (~5 min)
- Treat a crossed `CardThresholdMinutes` as an additional `"stalled"`-classified failure feeding
  `handleDriverFailure`, gated by `StaleTriggersRetry == true`.
- Files: `session/session_driver.go`

---

## Phase 6: Verification

### Epic 6.1: Build/test/lint gate + manual acceptance-criteria pass

#### Story 6.1.1: Automated gate

##### Task 6.1.1a: `make build && make test && make lint` (~5 min)

##### Task 6.1.1b: `cd web-app && npx jest --no-coverage` for the new component tests (~3 min)

#### Story 6.1.2: Manual smoke test (per CLAUDE.md's manual/interactive testing section — never `make install-service` against the live instance)

##### Task 6.1.2a: `PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test` — force 3 crashes with `max_attempts=3`, confirm `PermanentlyFailed` + one notification (AC1) (~5 min)

##### Task 6.1.2b: Confirm a `["crashed"]`-only policy does not retry a `tmux_exited` failure (AC2), and "Retry now" works from `PermanentlyFailed` (AC6) (~5 min)
