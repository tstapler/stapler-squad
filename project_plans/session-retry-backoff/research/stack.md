# Research: Stack — session-retry-backoff

## 1. Existing backoff/retry patterns

**No general-purpose backoff library is actually used.** `go.mod` lists
`github.com/cenkalti/backoff/v5 v5.0.3 // indirect` (line pulled in transitively by
some other dependency), but `grep -rln "cenkalti/backoff" --include="*.go" .` returns
zero hits — nothing in this codebase imports it. Don't reach for it; it's not an
established project convention, just an indirect transitive dep.

**The real precedent is `session/backlog_remediation.go`** — the shared exponential
backoff gate for backlog-item stuck-state remediation (a different concept per the
requirements doc's "out of scope" note, but structurally the closest analog in the repo):

- `remediationBackoffSchedule []time.Duration` — a **hardcoded slice of gaps**, not a
  formula (`{30m, 2h, 8h, 24h, 72h}`), indexed by `attemptNumber-1`
  (`session/backlog_remediation.go:31-37`).
- `MaxRemediationAttempts = int32(len(remediationBackoffSchedule))` — attempt cap
  derived from schedule length, not a separate config field
  (`session/backlog_remediation.go:45`).
- `evaluateRemediation(row, now, bootTime) remediationDecision` — a **pure, table-driven
  function** returning one of `remediationSkippedParked` /
  `remediationSkippedNotDue` / `remediationGranted` /
  `remediationGrantedRestartGrace` (`session/backlog_remediation.go:96-110`). Kept
  DB-independent specifically so it's exhaustively unit-testable — same rationale
  cited for `session/stuck_decisions.go`.
- `Storage.RemediationDue(ctx, itemID, reason)` — the DB-integrated gate every caller
  goes through; atomically records the attempt (or restart-grace pass) **before**
  returning `due=true`, so concurrent/async callers can't double-count
  (`session/backlog_remediation.go:168-193`).
- Attempt state (`RemediationAttempts`, `NextRemediationAt`, `GraceBootTime`) is
  persisted per-row via the Ent-backed `Storage`/`EntRepository`, not the JSON config
  file — this is the one place the analogy breaks for our feature, since the
  requirements doc explicitly rules out a new SQLite/Ent-backed queue ("reuse existing
  config/state file persistence").

**Recommendation:** mirror the *shape* (hardcoded schedule slice + pure
`evaluate*`-style decision function + attempt counter), not the *storage backend*.
For session-level retry, attempt count + last-failure metadata should live on the
`Instance`/session state persisted via `config/state.go`'s JSON file pattern (or
wherever per-session runtime state already round-trips — check `session/instance.go`
for existing per-instance persisted fields before adding new ones).

## 2. Config persistence pattern (`config/` package)

- `config/config.go:229` — `type Config struct` is a **flat struct**, one field per
  setting, JSON-tagged with `,omitempty` for anything added after the original schema
  (e.g. `ConfigVersion int \`json:"config_version,omitempty"\`` at line 283 — present
  but not actively used as a migration gate in what was inspected; new fields are
  additive and just default to Go zero values on old config files, no explicit
  migration step observed).
- Nested config blocks are typed sub-structs embedded by value with `omitempty`,
  e.g. `Hibernation HibernationConfig \`json:"hibernation,omitempty"\`` (line 337),
  `TmuxExecGate TmuxExecGateConfig` (341), `SessionRetention SessionRetentionConfig`
  (343). **A new `RetryPolicy` config block should follow this exact shape**: a
  `RetryPolicyConfig` struct in `config/types.go`, embedded on `Config` as
  `RetryPolicy RetryPolicyConfig \`json:"retry_policy,omitempty"\``, mirroring
  `SessionRetentionConfig`'s use of `*bool` for `Enabled` (`config/types.go:42`) so
  "unset" is distinguishable from "explicitly false" — needed for a
  global-default-with-per-session-override design (requirement #1).
- `config/types.go:38-45` (`SessionRetentionConfig`) is the closest existing analog
  to what's being asked for (a *_Config struct with `Enabled *bool` + numeric knobs
  like `RetentionDays int`) — copy its shape for `RetryPolicyConfig` (`Enabled *bool`,
  `MaxAttempts int`, `InitialDelaySeconds int`, `MaxDelaySeconds int`,
  `RetryOn []string` or a typed enum slice).
- Load/save: `LoadConfig()` (line 782), `SaveConfig(config *Config)` (841),
  `LoadConfigFromPath(path string)` (847) — plain JSON marshal/unmarshal to a file
  path, no schema versioning/migration framework beyond the unused-looking
  `ConfigVersion` field. No DB, no ent — confirms the requirements doc's "reuse
  existing config/state file persistence" is realistic and low-effort.
- Per-session override: none of the inspected structs show an existing
  global-default + per-session-override pattern for a policy-shaped config value in
  this codebase; this would be new territory — check whether per-session config
  already exists in `session/instance.go` (not confirmed in this pass; worth a
  follow-up read before implementation).

## 3. Proto pattern for session state (`proto/session/v1/`)

- `proto/session/v1/types.proto:325-345` — `enum SessionStatus` uses `allow_alias =
  true` and has deliberately **left gaps in the deprecated values** (1-3, 5) while
  preserving wire compatibility; current live terminal value is
  `SESSION_STATUS_STOPPED = 7` (comment: "terminal state, cannot transition
  further"). A new `SESSION_STATUS_PERMANENTLY_FAILED` terminal state fits at the next
  free integer (`= 8`), following the same "terminal state" comment convention as
  `STOPPED`.
- `proto/session/v1/types.proto:410-434` — `enum SubStatus` is the **fine-grained,
  derived-at-read-time** companion enum layered on top of the coarse `SessionStatus`
  (comment: "Derived at read time from the detection layer; never stored in the
  database"). `SubStatus sub_status = 54;` and `= 21;` are wired into two different
  messages (`types.proto:179`, `621`). This is the existing precedent for "fine detail
  under a coarse status" — retry-count/attempt info could ride alongside as new fields
  on the session message (e.g. `int32 retry_attempt = N; int32 retry_max_attempts =
  N+1;`) rather than needing a new enum value itself, since attempt count is a number,
  not a state.
- A new `RetryPolicy` message belongs in `proto/session/v1/types.proto` near
  `SessionDefaults`-equivalent structures, or in `session.proto` alongside
  `CreateSessionRequest` if it's only used at creation/override time — check
  `.claude/rules/session-creation-registry.md` touchpoint #2 for the exact home
  (`CreateSessionRequest` gets new fields via next available field number, e.g.
  `RetryPolicy retry_policy = 15;` following the `bool one_off = 14;` precedent
  documented in that rule file).
- Regen command per `CLAUDE.md`: **`make proto-gen`**, which regenerates
  `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`. Do not hand
  edit generated files.
- For retry **history** (reason + timestamp per attempt, requirement #7), no existing
  proto message was found for a similar "event log" shape in `types.proto` in this
  pass — closest pattern is likely `proto/session/v1/events.proto` (not inspected in
  detail this pass; worth checking whether a generic session-event log already exists
  there before inventing a new `RetryAttempt` repeated message).

## 4. Non-blocking delayed-restart goroutine: stdlib primitive recommendation

**Recommend: `select { case <-ctx.Done(): ...; case <-manualRetryCh: ...; case
<-time.After(delay): ... }`** — not `time.Timer`/`time.AfterFunc` directly. This is
the dominant idiom already used throughout `session/` for exactly this
shape (cancellable wait + external interrupt + timeout):

- `session/autonomous_driver.go:434-450` — the closest direct analog: a bounded wait
  loop that re-checks a settle window, interruptible by `ctx.Done()` (cancel) or an
  external `statusCh` event, falling through to `time.After(wait)` as the timeout arm.
  This is structurally identical to what requirement #2 asks for (backoff delay,
  "non-blocking for manual retry/stop during the wait") — swap `statusCh` for a
  manual-retry-now signal channel and `wait` for the computed backoff duration.
- Other examples of the same `select` + `time.After` idiom: `session/actor.go:89,96`,
  `session/command_executor.go:194/257/317`, `session/hibernation_sweeper.go:190/271`,
  `session/backlog_sync.go:79`, `session/history_watcher.go:88`,
  `session/external_streamer.go:300/476`.
- Why not `time.AfterFunc`: it fires on its own goroutine outside the driver's existing
  `select` loop, which is harder to make cancellable/interruptible by a manual-retry
  signal without adding a second synchronization primitive (channel + timer.Stop()
  race). The existing `session_driver.go` goroutine-per-session model already owns a
  loop with a `select`-friendly shape (see `runSessionDriverWithPrompt`,
  `session/session_driver.go:125+`), so extending `handleDriverFailure`
  (`session/session_driver.go:509`) to compute a backoff duration and block on
  `select{ctx.Done(), manualRetryCh, time.After(delay)}` before calling `inst.Restart`
  fits the existing structure with no new primitive.
- Why not a bare `time.Sleep`: blocks the goroutine uninterruptibly, which directly
  violates requirement #2 ("non-blocking for manual retry/stop during the wait").

## 5. Attempt-counting pattern: reuse vs. mirror

- Current single-retry state: `var retried atomic.Bool` (`session/session_driver.go:111`),
  passed by pointer through `runSessionDriverWithPrompt` and `handleDriverFailure`, with
  `retried.CompareAndSwap(false, true)` as the idempotency guard
  (`session/session_driver.go:509-510`). This must become a counter
  (`atomic.Int32` or similar) gated against a configurable `max_attempts`, not a
  boolean — the CompareAndSwap-based single-shot guard doesn't generalize to N
  attempts directly; needs an atomic increment + compare-against-max, or a
  small mutex-guarded struct if backoff timestamp/reason history needs to travel
  alongside the count (plain `atomic.Int32` can't carry a timestamp + reason string
  atomically — follow `.claude/rules/go-double-checked-locking.md`'s "return the
  locally-computed value" discipline if a lock is introduced here).
- `session/backlog_remediation.go`'s `evaluateRemediation`/`nextRemediationAt` pure-function
  split (decision logic separate from the DB write) is the right pattern to mirror for
  a new `evaluateSessionRetry(attempts, lastFailureAt, policy, now) retryDecision`-style
  helper — keeps the exponential math (`initial_delay * 2^attempt`, capped at
  `max_delay_seconds`, per requirement #2) unit-testable without spinning up a real
  session driver.
- `MaxRemediationAttempts` derives from `len(schedule)`; the new feature's
  `max_attempts` is explicitly configurable per the requirements doc, so the retry
  helper should take `policy RetryPolicyConfig` as a parameter rather than hardcoding
  a schedule slice — compute `initial_delay * 2^(attempt-1)` capped at `max_delay`
  instead of indexing into a fixed slice.

## Summary of concrete file touchpoints for planning

| Concern | File | Pattern to follow |
|---|---|---|
| Retry policy config struct | `config/types.go` | Mirror `SessionRetentionConfig` (`Enabled *bool` + numeric fields) |
| Config embed | `config/config.go` (`Config` struct ~line 343) | `omitempty`-tagged nested struct field |
| New terminal proto enum value | `proto/session/v1/types.proto:325-345` (`SessionStatus`) | Next free int (`= 8`), "terminal state" comment like `STOPPED` |
| RetryPolicy proto message + request field | `proto/session/v1/session.proto` (`CreateSessionRequest`) | Next available field number, per `.claude/rules/session-creation-registry.md` |
| Proto regen | — | `make proto-gen` (never hand-edit `session/gen/`, `web-app/src/gen/`) |
| Attempt counter + backoff decision | `session/session_driver.go` (`handleDriverFailure`, `~509`) | Replace `atomic.Bool` with counter; extract pure decision fn like `evaluateRemediation` |
| Delayed non-blocking restart | `session/session_driver.go` | `select{ctx.Done(), manualRetryCh, time.After(delay)}`, mirroring `session/autonomous_driver.go:434-450` |
| tmux_exited vs crashed distinction | `session/session_driver.go` failure-detection sites (`~202-236`) | Not yet researched in depth — needs a look at how tmux pane-liveness is currently checked elsewhere (e.g. `session/tmux/`) in a follow-up research pass |
