# Architecture Research: session-retry-backoff

**Date**: 2026-08-06
**Agent**: architecture research pass (ad hoc, not a full sdd:2-research run)

## Prior analysis: none found — first architecture pass for this area

Checked `project_plans/*/research/architecture.md` across the repo; no existing doc analyzes
`session/session_driver.go` or `session/backlog_lifecycle.go`'s retry/crash-recovery machinery.
The closest prior art is the sibling `project_plans/stale-session-detection/research/architecture.md`
(2026-08-06, same day) — reused extensively below for config-threading and proto-wiring precedent,
per requirements.md's explicit instruction to consume that project's config surface rather than
duplicate it.

---

## 1. Where the RetryPolicy / attempt counter / backoff step gets threaded through the driver

### Current shape (confirmed by reading `session/session_driver.go:75-596`)

- `StartSessionDriver(inst *Instance, allowedPath string)` (line 75) — public entry point. Uses
  `inst.driverRunning.CompareAndSwap(false, true)` for idempotency, then spawns
  `runSessionDriver(inst, allowedPath)` in a goroutine.
- `runSessionDriver` (line 110) — thin wrapper: allocates `var retried atomic.Bool` **on the Go
  stack of this call**, resolves the initial prompt, and delegates to
  `runSessionDriverWithPrompt(inst, allowedPath, initialPrompt, &retried)`.
- `runSessionDriverWithPrompt(inst *Instance, allowedPath string, initialPrompt string, retried *atomic.Bool)`
  (line 125) — the actual poll loop. On detecting a failure (three call sites: line 209 "exit
  before initial prompt", line 423 "inactivity timeout", and a third exit-after-initial-prompt path
  around line 236) it calls `handleDriverFailure(inst, allowedPath, retried, reason)` and returns.
- `handleDriverFailure` (line 509) — the single/multi-retry gate. Today:
  `retried.CompareAndSwap(false, true)` — first call flips it and restarts; second call
  (`retried.Load()==true`, CAS fails) calls `markSessionNeedsAttention` and stops. The restart path
  spawns a **new** `runSessionDriverWithPrompt` goroutine, passing the **same** `retried` pointer
  forward (line 569) — this is exactly why today's cap is "retry once": the flag is monotonic and
  shared across every generation of the goroutine chain for one session's failure lifecycle.

### The key structural fact: `retried *atomic.Bool` is function-call state, not Instance state

It lives on `runSessionDriver`'s stack, threaded by pointer through every subsequent call in the
chain (including across goroutine restarts). It is **not** on `Instance` today, and critically: it
resets to zero every time `StartSessionDriver` is called fresh (e.g., a manual restart, a service
restart, or the user clicking "Start" again) — a new `var retried atomic.Bool` is allocated. This
is actually a **latent bug alongside the feature gap**: if the driver goroutine chain is torn down
and `StartSessionDriver` is called again from outside (any external restart trigger), the retry
counter silently resets to zero even mid-failure-cycle. A counter living on `Instance` fixes this
for free as a side effect of making it persistent/observable.

### Proposed minimal structural change

Replace `*atomic.Bool` with a small **`RetryState` struct field on `Instance`**, not scattered loose
fields, mirroring the established `ReviewState` embedding pattern (`session/instance.go:311`,
`session/review_state.go:41` — a dedicated file, a plain struct of timestamps/counters, embedded
by value into `Instance` so its fields are promoted, e.g. `inst.LastViewed` reads directly).
Concretely:

- **New file `session/retry_state.go`**, mirroring `session/review_state.go`'s shape:
  ```go
  package session

  // RetryState holds the automated crash/stall retry lifecycle for one session.
  // Embedded (by value) into Instance so fields are promoted (inst.RetryAttempt, etc.).
  // Protected by Instance.mu — see the same convention as ReviewState.
  type RetryState struct {
      // RetryAttempt is the number of automated retries already performed for the
      // current failure cycle. Reset to 0 on a successful Ready transition (i.e.
      // once the session produces meaningful output again) or a manual "Retry now".
      RetryAttempt int `json:"retry_attempt,omitempty"`
      // RetryMaxAttempts is the resolved (policy-applied) cap for this session,
      // snapshotted at driver start so a live config edit mid-cycle doesn't change
      // the cap the session is already partway through. 0 == policy disabled.
      RetryMaxAttempts int `json:"retry_max_attempts,omitempty"`
      // LastFailureReason is the most recent retry-triggering reason: "crashed",
      // "stalled", or "tmux_exited" (see §3 below).
      LastFailureReason string `json:"last_failure_reason,omitempty"`
      // NextRetryAt is when the pending backoff delay elapses and the retry fires.
      // Zero when no retry is currently pending.
      NextRetryAt time.Time `json:"next_retry_at,omitempty"`
      // PermanentlyFailed is true once RetryAttempt has exhausted RetryMaxAttempts
      // and no further automatic retry will occur (see §2/§6 for the terminal-state
      // question — kept as a flag, not a new Status value).
      PermanentlyFailed bool `json:"permanently_failed,omitempty"`
      // RetryHistory is an append-only log of past attempts (reason + timestamp),
      // capped (see below) so it can't grow unbounded across a very long-lived session.
      RetryHistory []RetryAttemptRecord `json:"retry_history,omitempty"`
  }

  // RetryAttemptRecord is one entry in RetryState.RetryHistory.
  type RetryAttemptRecord struct {
      Attempt   int       `json:"attempt"`
      Reason    string    `json:"reason"`
      Timestamp time.Time `json:"timestamp"`
  }
  ```
  Cap `RetryHistory` at a small fixed length (e.g. 10, matching the existing
  `driverContinuationMaxMessages = 10` style constant) — `max_attempts` itself is
  small (single-digit per requirements.md's example of 3), so history never grows large in
  practice, but a cap is still cheap insurance against a pathological config.
- **Embed into `Instance`** the same way `ReviewState` is (`session/instance.go:311`, right next to
  it) — a plain value field, protected by the existing `inst.mu sync.RWMutex` (already documented at
  `session/instance.go:365-368` as covering "ReviewState timestamps" — extend that doc comment to
  also mention RetryState).
- **Function signature changes**: `handleDriverFailure`'s `retried *atomic.Bool` parameter is
  replaced by reading/writing `inst.RetryState` directly (under `inst.mu`), so the parameter can be
  dropped entirely from `handleDriverFailure`, `runSessionDriverWithPrompt`, and `runSessionDriver`.
  This is a larger diff than adding a field, but it's the right one: `*atomic.Bool` cannot express
  "attempt 2 of 5" or a reason string, and per the pointer analysis above, moving the state onto
  `Instance` also fixes the goroutine-restart reset bug as a side effect. Threading a
  `*RetryPolicy` (resolved config, effectively read-only after driver start) alongside is simplest
  as a second explicit parameter (not stored on Instance — it's derived/resolved config, not
  session-owned mutable state), e.g.:
  ```go
  func runSessionDriverWithPrompt(inst *Instance, allowedPath string, initialPrompt string, policy RetryPolicy) { ... }
  func handleDriverFailure(inst *Instance, allowedPath string, policy RetryPolicy, reason string) { ... }
  ```
  `RetryPolicy` itself (the resolved config value — enabled/max_attempts/backoff/delays/retry_on)
  is a **value type** passed down, not stored on `Instance` — analogous to how `allowedPath` is
  already threaded as a plain parameter rather than re-read from config on every call. It gets
  resolved once in `StartSessionDriver` (global config, with an optional per-session override
  read off `Instance` — see §4) and passed down unchanged through the whole call chain, including
  across the restart-spawns-new-goroutine hop (replacing today's `&retried` pointer being forwarded
  at line 569).

### Backoff delay without blocking `Stop`/manual-retry

Requirement 2 explicitly forbids blocking the driver goroutine in a way that prevents manual stop/
retry during the wait. The existing poll loop (`ticker := time.NewTicker(driverPollInterval)`,
2s cadence) is the natural place to implement this **without a blocking `time.Sleep`**: on entering
a pending-retry state, set `inst.RetryState.NextRetryAt = time.Now().Add(delay)` and keep the
*existing* ticker loop running — each tick checks `if !NextRetryAt.IsZero() && time.Now().Before(NextRetryAt) { continue }`
before proceeding to the actual restart call. This reuses the poll loop's existing pattern instead
of introducing a second timer/goroutine, and manual "Retry now" (§UI) becomes trivial: it just
zeroes `NextRetryAt` (or sets it to the past) under `inst.mu`, and the very next tick (≤2s later)
fires the restart — no cancellation channel or context needed. `Stop`/`Pause` already short-circuit
the loop via the existing `st == Paused` / `st == Stopped` checks at the top of each tick
(lines 194-196), so a stop request during the backoff wait is already handled by the pre-existing
control flow — no new interruption mechanism required.

---

## 2. Where retry-related fields live — Instance fields vs. owned `RetryState`, and mutex discipline

**Recommendation: owned `RetryState` struct embedded in `Instance` (see §1), not loose top-level
fields.** Reasons, grounded in what's already in the file:

- `Instance` already uses exactly this pattern for a closely analogous concern — `ReviewState`
  (`session/instance.go:308-311`) groups "all review queue and terminal activity timestamps" as one
  embedded struct rather than 8 individual top-level fields, with a comment explicitly noting fields
  are "Protected by mu (via sendSyncErr / Snapshot)." `RetryState` is the same shape of concern
  (a cohesive bundle of timestamps + a counter + a flag, all related to one subsystem) and should
  follow the same convention for consistency and to avoid further flattening an already
  360-line struct definition.
- `PauseReason`/`hibernateReason` are the **other** precedent, and they cut the other way (loose
  top-level/unexported fields, not a sub-struct) — but they're single scalar fields with no
  companion timestamps or counters; `RetryState` needs 6 related fields, past the point where a
  loose-field approach stays readable. `ReviewState`'s bundling is the better-fitting precedent
  once the field count exceeds ~2-3.

### Mutex discipline

- `inst.mu sync.RWMutex` (line 369) already protects `ReviewState`, `Status`, `Tags`,
  `Checkpoints`, and GitHub PR fields per its doc comment — extend that same lock to cover
  `RetryState`. The driver goroutine (single writer per session, since `driverRunning`'s CAS already
  guarantees only one driver goroutine runs at a time) writes `RetryState` fields under `inst.mu.Lock()`;
  HTTP handlers / UI-polling reads go through `inst.mu.RLock()` — or, following the codebase's
  broader convention of lock-free reads for polling-heavy consumers (`session_driver.go:459-462`
  itself takes `inst.mu.Lock()` briefly just to set `GitHubPRURL`/`GitHubPRNumber`, a two-field
  write, then unlocks — the identical shape a `RetryState` write would take), consider whether
  `RetryState` needs to be in the existing `snapshot atomic.Pointer[InstanceSnapshot]`
  (`session/instance.go:355-358`) that "every mutator publishes... before it releases mu" so that
  HTTP-handler/UI-polling reads can call `Snapshot()` lock-free rather than taking `inst.mu.RLock()`
  on every poll — this is the established zero-contention-read pattern for exactly this kind of
  "driver writes occasionally, UI polls frequently" access shape. Check `InstanceSnapshot`'s struct
  definition (`session/instance.go`, search `type InstanceSnapshot struct`) at implementation time
  and add the `RetryState` fields there too, updating whatever `Snapshot()` builder function
  currently populates it from `ReviewState`.
- **Do not** use a second `atomic.Bool`/`atomic.Pointer` per field the way `started`/`driverRunning`
  do — those are justified by the doc comment at line 328-334 as a response to ~30+ uncoordinated
  access sites across the whole package predating any lock discipline (a BUG-025 postmortem, not a
  pattern to imitate for brand-new state). `RetryState` has exactly one writer (the driver
  goroutine) and reads from a small, known set of call sites (HTTP handlers, UI-polling snapshot),
  so the existing `mu`+`Snapshot()` combination is the correct, already-established fit — introducing
  raw atomics per-field here would be over-engineering relative to the actual concurrency shape.

---

## 3. Proto changes — `retry_on`/`tmux_exited`/`permanently_failed`/attempt count

Per `.claude/rules/session-creation-registry.md`'s adjacent precedent (a similar "wire a new
concept through proto → Go → web-app" chain, though this feature does **not** need a new
`SessionType` enum value or new session-creation touchpoint — it's a post-creation lifecycle
concern, not a creation mode). Two separate proto surfaces are needed:

### 3a. Per-session retry state (read side — surfaces on `Session` message)

`proto/session/v1/types.proto`'s `message Session { ... }` (starts line 9) currently runs field
numbers up to ~71 (verify exact highest at implementation time — do not hardcode a number in the
plan without re-checking, since sibling in-flight proto work in other project_plans/ branches may
have already claimed adjacent numbers). Add, at the next available field number:
```protobuf
int32 retry_attempt = <next>;
int32 retry_max_attempts = <next+1>;
string last_failure_reason = <next+2>;       // "crashed" | "stalled" | "tmux_exited"
google.protobuf.Timestamp next_retry_at = <next+3>;
bool permanently_failed = <next+4>;
repeated RetryAttemptRecord retry_history = <next+5>;

message RetryAttemptRecord {
  int32 attempt = 1;
  string reason = 2;
  google.protobuf.Timestamp timestamp = 3;
}
```
This mirrors the existing `GitHubPRState`/`GitHubPRIsDraft`/etc. block on `Instance`
(`session/instance.go:195-211`) — a set of "populated by a background process, not set on session
creation" fields grouped together, which is exactly `RetryState`'s shape too. `tmux_exited` as "a
distinct condition" (requirement 3) only needs to exist as a **string value** on
`last_failure_reason`/in the `retry_on` policy list (§3b) — it does not need its own proto enum
unless the plan phase decides a `RetryFailureReason` enum is preferable to a bare string for
type-safety on the wire (consistent with how `AutonomousOutcome string` on `Instance`, line 172, is
also a bare string rather than an enum for a small fixed value set — a reasonable precedent to
follow rather than introduce a new proto enum for a 3-value set).

### 3b. RetryPolicy config (write side — global default + optional per-session override)

Needs its own message, referenced from two places:
1. `SessionDefaultsConfig` (`proto/session/v1/session.proto:1713-1731`) — the stale-session-detection
   architecture doc (§2 there) already identifies this exact message as the precedent for exposing
   a new global-default config value to the frontend (it already has the "0 means use server
   default, response always echoes resolved value" idiom via `max_auto_rework_iterations`). Add a
   nested `RetryPolicyConfig retry_policy = <next>;` field here.
2. `CreateSessionRequest` (`proto/session/v1/session.proto:472`, fields currently run to ~27) — for
   a **per-session override**, following the `session-creation-registry.md` pattern of "new bool/
   field on `CreateSessionRequest`" (e.g. its own `one_off = 14` example). Add
   `optional RetryPolicyConfig retry_policy_override = <next>;` (using `optional` — proto3
   explicit-presence — so "not set" is distinguishable from "explicitly disabled," matching the
   config package's own `*bool` nil-means-unset idiom described in §4 below).

```protobuf
message RetryPolicyConfig {
  bool enabled = 1;
  int32 max_attempts = 2;
  string backoff = 3;               // "exponential" — only value today, string not enum per YAGNI
  int32 initial_delay_seconds = 4;
  int32 max_delay_seconds = 5;
  repeated string retry_on = 6;     // subset of "crashed", "stalled", "tmux_exited"
}
```

After any proto edit: `make proto-gen` (regenerates `session/gen/session/v1/*.go` and
`web-app/src/gen/session/v1/*_pb.ts` — per CLAUDE.md, `web-app/src/gen` is tracked despite
`.gitignore`, commit it).

---

## 4. `config/` package — where `RetryPolicy` config lives

Confirmed pattern via `config/types.go` (read in full for `HibernationConfig`,
`SessionRetentionConfig`, `TmuxExecGateConfig`) and `config/config.go:229` (`Config` struct) /
`:450-470` (`DefaultConfig()`-equivalent defaults function):

- Config sections are **plain structs in `config/types.go`**, JSON-tagged, referenced as a named
  field on the top-level `Config` struct (`config/config.go:229`), with defaults assigned inline
  inside the function that builds the zero-value default config (around line 455-470, e.g.
  `cfg.Hibernation = HibernationConfig{...}`).
- The **nil-pointer-means-unset** idiom (`SessionRetentionConfig.Enabled *bool` +
  `EnabledOrDefault()`, `config/types.go:41,48-51`) is the established way to distinguish "field
  didn't exist when this config.json was saved" from "explicitly set false" — required here too,
  since `RetryPolicy` is a new field being added to an existing persisted config file format.

Proposed:
```go
// RetryPolicyConfig holds the global default automated-retry policy for crashed/stalled/
// tmux-exited sessions. See session.RetryState for the per-session runtime counters this
// policy gates. A per-session override can be set via CreateSessionRequest.retry_policy_override
// (proto) — Instance stores the resolved, session-specific policy, not this global struct,
// once the driver starts (see architecture.md §1).
type RetryPolicyConfig struct {
    // Enabled controls whether the automated retry mechanism runs at all. A pointer so a
    // config saved before this field existed (nil) defaults to true — preserving today's
    // existing single-retry behavior rather than silently downgrading to zero retries
    // (requirements.md Acceptance Criteria #7).
    Enabled *bool `json:"enabled,omitempty"`
    // MaxAttempts is the number of automated retries before permanently_failed. Default: 1
    // (matches today's hardcoded single-retry behavior — see MaxAttemptsOrDefault).
    MaxAttempts int `json:"max_attempts,omitempty"`
    // Backoff is the delay strategy. Only "exponential" is supported (YAGNI per requirements.md).
    Backoff string `json:"backoff,omitempty"`
    // InitialDelaySeconds is the delay before the first retry. Default: 0 (matches today's
    // immediate-restart behavior for the existing single retry).
    InitialDelaySeconds int `json:"initial_delay_seconds,omitempty"`
    // MaxDelaySeconds caps the exponential growth. Default: e.g. 300 (5 min) — a Phase 3
    // decision, not derived from any existing constant.
    MaxDelaySeconds int `json:"max_delay_seconds,omitempty"`
    // RetryOn is the subset of "crashed"/"stalled"/"tmux_exited" that triggers a retry.
    // Empty/nil defaults to all three (preserving today's behavior, which doesn't distinguish
    // them at all).
    RetryOn []string `json:"retry_on,omitempty"`
}
```
Add `RetryPolicy RetryPolicyConfig` to `Config` (`config/config.go:229` region) and a default
assignment in the defaults function (`config/config.go:~459` region), e.g.
`cfg.RetryPolicy = RetryPolicyConfig{Enabled: boolPtr(true), MaxAttempts: 1, Backoff: "exponential", MaxDelaySeconds: 300}`
— **the default `MaxAttempts: 1` with `InitialDelaySeconds: 0` is what satisfies Acceptance
Criteria #7** ("a sane default policy preserves at least today's retry-once behavior").

Per-session override resolution: `StartSessionDriver` resolves
`effectivePolicy := cfg.RetryPolicy; if inst has an override, merge/replace` once, at driver-start
time, and passes the resolved `RetryPolicy` value down the call chain (§1) — this mirrors how
`allowedPath` is already resolved once by the caller and threaded down rather than re-read from
config on every poll tick.

---

## 5. `tmux_exited` as a distinct condition — detection point

Today, `handleDriverFailure` is reached from exactly two `Stopped`-status branches
(`session_driver.go:199-236`) and one inactivity branch (`:389-425`) — none currently distinguish
*why* the tmux session is gone. The existing codebase already has the primitive needed to tell
"tmux pane/session itself is gone" apart from "process exited but tmux server/session survives"
(the remain-on-exit case, explicitly called out in a comment at `session/instance.go:791` and
`session/health.go:211`: "tmux session alive but pane process has exited (remain-on-exit
placeholder)"):

- `TmuxProcessManager.DoesSessionExist()` (`session/tmux_process_manager.go:122-128`) — checks
  whether the tmux *session* itself (not just the pane's process) is still registered with the
  server. This is the exact liveness check needed: if `DoesSessionExist()` is `false` when the
  driver detects `st == Stopped`, that's `tmux_exited` (session/pane lost entirely — machine sleep,
  OOM-killed tmux server, etc., per requirements.md's own framing). If `DoesSessionExist()` is
  `true` but the pane process has exited (the remain-on-exit placeholder case), that's a plain
  `crashed`.
- Wire this as a new reason string passed into `handleDriverFailure`'s existing `reason` parameter
  — e.g. replace the two call sites' literal `"exit before initial prompt"` /
  inactivity-timeout string with a helper `classifyFailureReason(inst) string` that returns
  `"tmux_exited"`, `"crashed"`, or `"stalled"` (the third for the existing inactivity-timeout
  branch, which is unambiguous already — it's not a process-exit path at all). This reason string
  is what gets compared against the resolved `RetryPolicy.RetryOn` list (§4) to decide whether this
  particular failure is retry-eligible, and what gets recorded into `RetryState.LastFailureReason`
  / `RetryHistory` (§1) and prefixed into the continuation prompt per requirement 4 ("Previous
  attempt failed due to {reason}").

---

## 6. Terminal state: `permanently_failed` — flag on `RetryState`, not a new `Status` enum value

`session/instance.go:24-46` defines `Status` as a small closed `int` enum (`Creating`, `Active`,
`Paused`, `Stopped`, `Hibernated`, `Restoring`) with `Stopped` already documented as "a terminal
state." Two options:

- **(a) New `Status` value**, e.g. `PermanentlyFailed Status = 6` — would require updating every
  exhaustive `switch st { ... }` over `Status` across the codebase (the driver loop itself does
  this at `st == Paused`/`st == Stopped`; likely also true in `server/services/session_service.go`,
  UI status-icon mapping, etc.) — a wide blast radius for what requirement 5 actually asks for
  (something the UI can *badge*, not a new lifecycle branch every consumer must handle).
- **(b) `PermanentlyFailed bool` flag on `RetryState`** (already proposed in §1), read *alongside*
  `Status` (which stays `Stopped` — the tmux/process reality doesn't change) — mirrors exactly how
  `PauseReason string` (`session/instance.go:284`) and `hibernateReason string`
  (`session/instance.go:280`) already layer a *reason/sub-state* on top of the existing `Status`
  enum rather than minting new `Status` values for "paused because idle" vs. "paused because
  resource pressure." **Recommended** — same precedent, same rationale (narrower blast radius, no
  new exhaustive-switch cases to audit across the whole codebase), and it directly matches how the
  existing code already distinguishes "generic `NeedsAttention`" (today's only outcome, via
  `ReviewQueue`) from a new, more specific outcome: `permanently_failed` becomes a **second, more
  specific `ReviewItem.Reason`** value (see `session/review_queue.go:23`,
  `ReasonStale = queue.ReasonStale` — the existing `Reason` enum in `session/queue/` already has
  multiple named values; add `ReasonPermanentlyFailed` alongside `ReasonStale`) rather than a new
  top-level session lifecycle state. `markSessionNeedsAttention`'s existing call at
  `handleDriverFailure`'s give-up branch (line 515) is the one line that changes: pass the new
  reason once `RetryState.RetryAttempt >= RetryState.RetryMaxAttempts`, instead of always
  `ReasonStale`.

### Manual "Retry now" resetting from `permanently_failed`

Acceptance criterion 6 requires resetting from `permanently_failed`. Since it's a flag (not a
`Status`), reset is just: clear `RetryState.PermanentlyFailed = false`, `RetryState.RetryAttempt = 0`,
`RetryState.NextRetryAt = time.Time{}` under `inst.mu`, then call the same restart path
`handleDriverFailure` already uses today (`inst.RecoverFromStopped()` /
`inst.Restart(false)`/`inst.Start(false)`, lines 536-552) and re-invoke `StartSessionDriver` — no
new restart machinery needed, this is the existing crash-recovery restart path invoked manually
instead of from the poll loop.

---

## 7. Notification bus integration for `permanently_failed`

Two existing, live patterns for "fire a notification when a session-lifecycle event needing
attention occurs" — neither is currently wired from `session_driver.go` itself:

- **`ReviewQueue` observer pattern** (`session/queue/queue.go:192-203`,
  `ReviewQueueObserver.OnItemAdded`) — `markSessionNeedsAttention`'s existing `rq.Add(&ReviewItem{...})`
  call (line 584) already triggers this for *every* existing NeedsAttention case (including
  today's single-retry give-up), so whatever currently renders a UI toast/badge when a session
  lands in the Review Queue already fires for the existing generic case, with zero new code needed
  beyond setting the new `Reason: ReasonPermanentlyFailed` (§6) on the same `Add` call.
- **`session.Notifier` interface + `EventBusNotifier` adapter**
  (`session/backlog_lifecycle.go:22-30`, `server/services/backlog_notifier.go:12-35`) — the pattern
  requirements.md explicitly points to ("reuse the existing notification bus... per
  stale-session-detection's research"). This is **richer** than the ReviewQueue observer path (a
  titled/prioritized/typed message via `events.NewNotificationEvent`, matching
  `notifyIfActiveWorkSessionStale`'s exact call shape at
  `server/services/backlog_service_triage.go:1232-1239`) but is **not currently wired to
  `Instance`/`session_driver.go` at all** — today `Notifier` is only held by
  `BacklogLifecycleListener` (a different, backlog-item-scoped consumer), not by `Instance` itself.
  To reuse it for a generic (not backlog-item-specific) "this session permanently failed"
  notification, `Instance` needs its own optional `Notifier` reference, following the exact same
  wiring shape as `SetReviewQueue`/`GetReviewQueue` (`session/instance_approval.go:12-19`,
  wired at 4 call sites in `server/dependencies.go:624,832` and
  `server/services/session_service.go:431,790`): add `SetNotifier(n Notifier)` /
  `notifier Notifier` field to `Instance`, wire it at the same 4 call sites (same file, same
  function, right next to the existing `SetReviewQueue` call — they're already co-located), and
  call `inst.notifier.Notify(inst.UUID, "Session permanently failed", ..., NOTIFICATION_TYPE_ERROR, NOTIFICATION_PRIORITY_HIGH)`
  from `handleDriverFailure`'s give-up branch, guarded with a nil check (`if inst.notifier != nil`,
  matching `EventBusNotifier.Notify`'s own nil-receiver guard style).
  **Recommendation: do both** — the ReviewQueue `Add` (needed regardless, for the badge/queue
  surfacing itself) plus the richer `Notifier.Notify` call (needed for a proactive
  toast/push-notification per requirement 5's explicit "with a user notification" wording, which
  a passive queue entry alone doesn't satisfy — the user has to be looking at the Review Queue to
  see it, whereas `Notifier` pushes proactively via the event bus / web push).

---

## 8. Stale-session-detection integration (requirement 9)

Per requirements.md: "an optional setting (default off) that, when a session crosses the
stale-session-detection threshold, triggers a retry attempt instead of only notifying — consuming
that feature's threshold/config rather than adding a second one."

From `project_plans/stale-session-detection/research/architecture.md` (§2, §3): that project's
config surface is `StaleSessionConfig` (proposed, `config/types.go`) with
`CardThresholdMinutes`/`NotifyEnabled`, sourced to the frontend via `SessionDefaultsConfig`
(`sessionDefaultsToProto`, `server/services/defaults_service.go:496`), and the canonical Go-side
computation is `Instance.GetTimeSinceLastMeaningfulOutput()` (`session/instance_approval.go:112`) —
**the same call `session_driver.go`'s own inactivity-timeout branch already uses indirectly** (it
reads `inst.LastMeaningfulOutputTime()`, line 390 — check whether that's the same or a sibling
helper to `GetTimeSinceLastMeaningfulOutput` at implementation time; they likely wrap the same
underlying timestamp).

Given that overlap, the driver's *existing* inactivity-timeout branch
(`session_driver.go:414-425`, `driverInactivityTimeout = 10 * time.Minute` today, a package
constant not yet config-driven) is architecturally the **same signal** stale-session-detection's
`CardThresholdMinutes` threshold measures — just currently hardcoded and independently maintained.
Two integration shapes:
- **(a) Minimal, decoupled** (recommended for this project's scope): add a new
  `RetryPolicyConfig.StaleTriggersRetry *bool` (default nil→false) field. When true, and when
  `stale-session-detection`'s `StaleSessionConfig.CardThresholdMinutes` is crossed (read directly
  from `cfg.StaleSession.CardThresholdMinutes` — both configs are already loaded from the same
  `*config.Config` the driver already has access to via whatever already threads config into
  `StartSessionDriver`/`runSessionDriverWithPrompt` for the `RetryPolicy` resolution in §1), treat
  it as an additional `"stalled"`-classified failure reason feeding into the same
  `handleDriverFailure` path — i.e., the stale-session threshold becomes a 4th trigger alongside
  the existing 10-minute inactivity check, gated by this new opt-in flag, rather than the driver
  re-deriving its own separate staleness computation.
- **(b) Full unification** (out of scope per requirements.md's non-functional section and the
  sibling project's own explicit "do NOT attempt to unify the 3 existing Go implementations... a
  separate, optional follow-on" note) — replacing `driverInactivityTimeout` itself with
  `cfg.StaleSession.CardThresholdMinutes` outright. Not recommended for this project; sequence
  after both projects ship if ever pursued, per the sibling doc's own guidance.

This item should be **sequenced after `stale-session-detection`'s `StaleSessionConfig` struct
exists** (per requirements.md's own sequencing note) — this project's `StaleTriggersRetry` flag is
a pure consumer of a config field this project does not itself define.

---

## Is this a multi-actor domain needing an Event-Command-Policy table?

**Yes — build one.** Unlike the sibling stale-session-detection project (which explicitly declined
one, being mostly "add a read/render path" changes), this feature has genuine actor/policy/command
flow: a background goroutine detects failures and autonomously decides to retry or give up, a timer
gates the decision, a human can override via a UI action, and a downstream notification/consumer
system reacts to the terminal state.

| Domain Event | Policy (trigger condition) | Command | Actor / System |
|---|---|---|---|
| `SessionExitedUnexpectedly` (process gone, tmux session still exists) | `RetryPolicy.Enabled && "crashed" ∈ RetryOn && RetryAttempt < MaxAttempts` | `ScheduleRetry(delay=backoff(attempt))` | Session Driver goroutine (`session_driver.go`) |
| `TmuxSessionLost` (tmux session itself gone — `DoesSessionExist()==false`) | `RetryPolicy.Enabled && "tmux_exited" ∈ RetryOn && RetryAttempt < MaxAttempts` | `ScheduleRetry(delay=backoff(attempt))` | Session Driver goroutine |
| `SessionStalled` (inactivity timeout OR stale-session threshold, if opted in) | `RetryPolicy.Enabled && "stalled" ∈ RetryOn && RetryAttempt < MaxAttempts` | `ScheduleRetry(delay=backoff(attempt))` | Session Driver goroutine |
| `RetryBackoffElapsed` (`NextRetryAt` reached) | none (unconditional once scheduled) | `RestartSessionWithContinuationPrompt(reason)` | Session Driver goroutine (poll-loop tick) |
| `SessionExitedUnexpectedly` / `TmuxSessionLost` / `SessionStalled` | `RetryAttempt >= MaxAttempts` (exhausted) | `MarkPermanentlyFailed` + `NotifyOperator` | Session Driver goroutine → `session.Notifier` / `ReviewQueue` |
| `ManualRetryRequested` (user clicks "Retry now") | none (always honored, incl. from `permanently_failed`) | `ClearRetryState` + `RestartSessionImmediately` (bypasses `NextRetryAt`) | User → HTTP handler → `Instance` |
| `SessionPermanentlyFailed` | `Notifier != nil` | `PublishNotificationEvent` | `session.Notifier` (`EventBusNotifier`) → event bus → web push / UI toast |
| `StaleSessionThresholdCrossed` (from sibling project, consumed here) | `RetryPolicy.StaleTriggersRetry == true` | treated as `SessionStalled` (feeds the same retry path) | Session Driver goroutine, consuming `config.StaleSessionConfig` |

---

## Integration points summary

| Area | Files |
|---|---|
| Retry state (NEW) | `session/retry_state.go` (new file, `RetryState` + `RetryAttemptRecord` structs) |
| Instance embedding | `session/instance.go:311` region (embed `RetryState` next to `ReviewState`), `:365-368` (extend `mu` doc comment) |
| Driver call chain (signature changes) | `session/session_driver.go:75` `StartSessionDriver`, `:110` `runSessionDriver`, `:125` `runSessionDriverWithPrompt`, `:509` `handleDriverFailure` — replace `retried *atomic.Bool` param with `RetryState` (on Instance) + `RetryPolicy` (passed value) |
| tmux_exited detection | `session/tmux_process_manager.go:122` `DoesSessionExist()` — new `classifyFailureReason(inst)` helper in `session_driver.go` |
| Terminal state | `session/review_queue.go:23` / `session/queue/` `Reason` enum — add `ReasonPermanentlyFailed`; `handleDriverFailure`'s give-up branch (line 515 today) |
| Manual retry reset | Same restart path `handleDriverFailure` already uses (lines 536-552), invoked from a new HTTP handler instead of the poll loop |
| Notifier wiring on Instance (NEW) | `session/instance_approval.go` (mirror `SetReviewQueue`/`GetReviewQueue`), 4 wiring call sites: `server/dependencies.go:624,832`, `server/services/session_service.go:431,790` |
| Proto — session read side | `proto/session/v1/types.proto` `message Session` (next field after ~71) — `retry_attempt`, `retry_max_attempts`, `last_failure_reason`, `next_retry_at`, `permanently_failed`, `retry_history` |
| Proto — policy config | `proto/session/v1/session.proto:1713-1731` `SessionDefaultsConfig` (global default), `:472` `CreateSessionRequest` (next field after ~27, per-session override) |
| Config | `config/types.go` (new `RetryPolicyConfig`), `config/config.go:229` (new field on `Config`), `:459` region (defaults, `MaxAttempts:1` to preserve AC#7) |
| Config → frontend | `server/services/defaults_service.go:496` `sessionDefaultsToProto` (extend, same function stale-session-detection already targets) |
| Stale-session integration | `config.StaleSessionConfig` (sibling project, not yet built) — consume `CardThresholdMinutes` as a 4th `handleDriverFailure` trigger gated by new `RetryPolicyConfig.StaleTriggersRetry` |
| UI — retry badge | `web-app/src/components/sessions/SessionCard.tsx` (no existing retry code — confirmed by requirements.md grep) |
| UI — retry history | Existing session history view (not yet located — grep `web-app/src/components/sessions/` for whatever renders `Checkpoints`/JSONL history at implementation time) |
| UI — manual retry action | New button, likely alongside existing session-card action buttons in `SessionCard.tsx`; needs a new RPC (not scoped here — plan phase should decide REST/ConnectRPC shape, likely a new `RetrySession` RPC on the session service) |
| Notification bus (reuse) | `session/backlog_lifecycle.go:22-30` `Notifier` interface, `server/services/backlog_notifier.go:12-35` `EventBusNotifier` — reuse as-is, wire onto `Instance` |
| Existing single-retry behavior (preserve, do not regress) | `session/session_driver.go:509-570` `handleDriverFailure`'s current restart/continuation-prompt logic — becomes attempt 1 of N under the new policy, default `MaxAttempts:1` preserves today's exact behavior |
