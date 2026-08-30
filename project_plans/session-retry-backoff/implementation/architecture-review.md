# Architecture Review: session-retry-backoff
**Date**: 2026-08-06
**Verdict**: BLOCKED

## Constitution Violations

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo (verified:
`test -f` returns not-found). No constitution to check against — none applicable.

## Blockers

- [ ] **Epic 2.3, Stories 2.3.2/2.3.3 — the backoff tick-check has no goroutine left to run
  it.** The plan's core design ("existing 2s poll-loop ticker checks
  `time.Now().After(NextRetryAt)` — no new goroutine/timer", Pattern Decisions table,
  "Backoff scheduling" row) requires the *same* ticker goroutine that detected the failure
  to keep running through the backoff wait and later notice `NextRetryAt` has elapsed. But
  the two call sites this plan touches (`session_driver.go:209-210`, `:236-237`) both read:
  ```go
  handleDriverFailure(inst, allowedPath, retried, "...")
  return
  ```
  `return` here exits `runSessionDriverWithPrompt`'s goroutine entirely, stopping its own
  ticker (`defer ticker.Stop()`, line 141) unconditionally — regardless of what
  `evaluateSessionRetry` decides. Task 2.1.1a/2.1.1b only change the signature and the
  `retried.Load()` read; neither task touches this `return`. Per Story 2.3.2's own AC
  ("no restart happens on this call... AC3's restart is deferred to the tick-check"), the
  `scheduled` decision needs the ticker to survive past this point — it doesn't, so nothing
  is ever running to observe `NextRetryAt` elapse. The backoff mechanism as specified cannot
  fire.
  - **Compounding bug if "fixed" by simply not returning**: `st == Stopped` stays true on
    every subsequent tick (nothing transitions `Status` away from `Stopped` while backoff is
    pending — no new status is introduced for "retry scheduled"). Task 2.3.3a places the
    `NextRetryAt` gate *"after the existing Paused/Stopped checks"* — i.e., **after** the
    `st == Stopped` branch, which itself unconditionally re-enters and re-calls
    `handleDriverFailure` (or the `isOneShot(inst) || retried.Load()` exhaustion check) on
    every single tick while `Status` is still `Stopped`. That means every 2s tick during a
    (potentially 300s) backoff wait would re-run `evaluateSessionRetry`, re-append a
    `RetryAttemptRecord`, and re-increment `RetryAttempt` — exhausting `MaxAttempts` and
    transitioning to `PermanentlyFailed` within seconds of the first failure, not after the
    intended delay.
  - **Remediation**: restructure the `st == Stopped` branch explicitly, before writing any
    code: check `NextRetryAt` pending *first* (top of the branch, not after it) — if
    non-zero and not yet elapsed, `continue` (no re-evaluation, no re-append); if elapsed,
    call `restartForRetry` and clear it; only call `evaluateSessionRetry`/`handleDriverFailure`
    at all when `NextRetryAt` is zero (i.e., this is a genuinely new failure episode). Add
    an explicit task for this reordering, and a table-driven test that asserts
    `RetryAttempt`/`RetryHistory` length stay constant across N ticks while `NextRetryAt` is
    still in the future (this is cheap to write and would have caught the bug immediately).

- [ ] **Epic 2.4's `retryInFlight` CAS guard is only wired into `RetryNow()` — the automated
  paths it's supposed to guard against never touch it.** The Domain Glossary and Pattern
  Decisions table both assert the guard closes races between "the automated backoff-expiry
  path and a manual 'Retry now' RPC" and that "every mutation path (automated backoff-expiry
  restart AND manual `RetryNow()`) is serialized behind a `retryInFlight` CAS gate." But:
  - Task 2.3.3b (poll-loop tick-check, elapsed `NextRetryAt` → `restartForRetry`) never
    mentions `retryInFlight`.
  - Task 2.3.2a's `restartGrace` branch (immediate restart, no backoff) never mentions
    `retryInFlight`.
  - Only Task 2.4.1a (`RetryNow()`) actually CASes it.

  Concretely: if a user clicks "Retry now" at the exact moment the automated tick-check's
  `NextRetryAt` elapses, `RetryNow()`'s CAS succeeds (nothing else ever sets `retryInFlight`
  to `true` on the automated path), and **both** the automated path and `RetryNow()` call
  `restartForRetry` concurrently — the exact "double-restart / lost-update" race
  (`research/pitfalls.md` §2) this field exists to close, per the plan's own framing.
  - **Remediation**: don't rely on every call site remembering to CAS the guard. Move the
    CAS into `restartForRetry` itself (or a thin wrapper every caller must go through) so
    the guard is structurally impossible to bypass — a single choke point rather than three
    call sites (tick-check, `restartGrace` branch, `RetryNow()`) each independently
    responsible for remembering it. Add Epic 2.4.2a's concurrency test against this
    single-choke-point version, not just `RetryNow()` racing itself.

- [ ] **Epic 2.1.1b — `inst.mu.RLock()` read immediately preceding a call into
  `handleDriverFailure` (which takes `inst.mu.Lock()`) is a self-deadlock waiting to
  happen.** Task 2.1.1b directs: replace `retried.Load()` at lines ~203/~216 with a read of
  `inst.RetryState.RetryAttempt >= inst.RetryState.RetryMaxAttempts` under
  `inst.mu.RLock()`. The very next statement in both branches (unchanged by this story) is
  `handleDriverFailure(inst, allowedPath, retried, "...")`, and per Task 2.3.2a/2.5.2a that
  function now takes `inst.mu.Lock()` internally to set `NextRetryAt`/append history/flip
  `Status`. Go's idiomatic (and extremely common) way to write a scoped read-lock is
  `inst.mu.RLock(); defer inst.mu.RUnlock()` — if an implementer writes it that way here
  (nothing in the task text warns against it), the deferred `RUnlock()` doesn't run until
  the enclosing function returns, i.e. *after* `handleDriverFailure` has already tried to
  take `inst.mu.Lock()` — `sync.RWMutex` is not reentrant, so this deadlocks the driver
  goroutine on literally the first exhausted-retry failure exercised in a test or in
  production.
  - **Remediation**: don't inline the lock at the call site. Add a small accessor method
    (e.g. `func (i *Instance) retryExhausted() bool` that internally does
    `i.mu.RLock(); defer i.mu.RUnlock(); return ...` and returns before the caller ever
    reaches `handleDriverFailure`), matching the existing `Snapshot()`-style read pattern
    elsewhere in `instance.go`. Add this as an explicit acceptance criterion ("the RLock is
    released before `handleDriverFailure` is called on the same goroutine") and a
    `-race`-run test that exercises the exhausted path, since a self-deadlock won't show up
    under `-race` but will hang the specific test — call it out so CI timeout, not silent
    merge, catches it.

## Concerns

- [ ] **Epic 2.1.2 — `RetryPolicyOverride *RetryPolicyConfig` (Task 2.1.2a) claims to
  mirror `ReworkCapOverride *int` but is a materially different shape.** `ReworkCapOverride`
  (`session/repository.go:379`) is a single nilable primitive — no internal optionality.
  `RetryPolicyOverride *RetryPolicyConfig` nests a *second* layer of optionality inside the
  first: the pointer itself can be nil (no override), but if non-nil, every field inside
  (`Enabled *bool`, `MaxAttempts int`, `RetryOn []string`, ...) still needs its own
  unset-vs-explicit-zero resolution. Task 2.1.2b's only acceptance criterion tests a
  single-field override (`&RetryPolicyConfig{MaxAttempts: 5}`) resolving `MaxAttempts` to
  `5` — it never specifies or tests what happens to the override's other zero-value fields
  (`RetryOn: nil`, `Backoff: ""`). Given `RetryOnOrDefault()` treats an empty `RetryOn` as
  "all three reasons," a partial override that only sets `MaxAttempts` would silently widen
  `RetryOn` back to `["crashed","stalled","tmux_exited"]` even if the *global* policy had
  deliberately narrowed it to `["crashed"]` — a silent behavior change nobody asked for.
  - **Remediation**: specify field-by-field merge semantics explicitly in the plan
    (override-field-set → override wins; override-field-zero → inherit global's *resolved*
    value, not the field's own default) and add a test with a non-default global
    (`RetryOn: ["crashed"]`) plus a partial override (`MaxAttempts` only), asserting the
    resolved `RetryOn` is still `["crashed"]`, not the all-three fallback default.

- [ ] **`RetryPolicyConfig.Backoff string` is a field with no behavior behind it.**
  `backoffDelay(attempt, initial, max, jitterFraction)` (Task 1.1.1b) always computes
  exponential backoff — nothing in any task branches on the `Backoff` string's value. So
  `Backoff: "exponential"` and `Backoff: "linear"` (or a typo) produce byte-identical
  behavior; the field is decorative. This is an illegal state made representable for no
  reason (Lens 2, point 6): an operator can set a value that looks meaningful in
  `config.json` and get silently ignored math.
  - **Remediation**: either (a) validate at config-load time that `Backoff`, if set, must
    equal `"exponential"` — reject/log and fall back to the default otherwise, consistent
    with the plan's own "no new strategies" YAGNI framing — or (b) drop the field entirely
    until a second backoff mode actually exists, and hardcode "exponential" as an internal
    constant/comment instead of a user-facing knob that does nothing.

- [ ] **No validation on `RetryOn []string` entries.** `RetryOnOrDefault()` (Task 1.2.1b)
  only handles the *empty* case (defaults to all three); nothing validates that each
  provided entry is one of `"crashed"/"stalled"/"tmux_exited"`. A config typo (e.g.
  `"crashd"`) silently produces a policy that never matches `classifyFailureReason`'s output
  for that reason — the session simply never retries for that failure mode, with no error,
  warning, or UI indication anywhere. Given this repo's plan explicitly rejected a full
  typed enum here as YAGNI (justifiably, for a 3-value set), the missing piece isn't the
  enum — it's the validation that would exist regardless of representation.
  - **Remediation**: add a lightweight validator in `LoadConfigFromPath` (or
    `RetryOnOrDefault`) that logs a warning and drops any entry not in the known set, so a
    typo degrades to "ignored with a log line" rather than "silently wrong forever."

- [ ] **Layering: `evaluateSessionRetry`'s parameter type is `config.RetryPolicyConfig`, not
  a resolved session-domain type.** The Domain Glossary explicitly distinguishes
  `RetryPolicyConfig` (raw, config-package, nilable fields) from `RetryPolicy` *(resolved)* —
  "the effective, already-merged... value... threaded down as a plain parameter." But
  `resolveRetryPolicy` (Task 2.1.2b) is typed to *return* `config.RetryPolicyConfig` again,
  and `evaluateSessionRetry` (Task 2.3.1a, described as "pure, DB-independent, exhaustively
  unit-testable" domain logic) takes that same config-package type as a parameter. This
  means the "resolved" policy retains config's nil-pointer-means-unset representation all
  the way into the domain layer's pure decision function, which then has to call
  `.EnabledOrDefault()`-style methods internally rather than working off a fully-resolved
  plain value — and every unit test for `evaluateSessionRetry` (Task 2.3.1b) is coupled to
  `config` package's struct shape instead of a `session`-local type.
  - **Remediation**: introduce a distinct `session.RetryPolicy` (or similar) with
    non-pointer, already-defaulted fields; have `resolveRetryPolicy` be the one place that
    converts `config.RetryPolicyConfig` → `session.RetryPolicy`, and have
    `evaluateSessionRetry` depend only on the session-local type. This is the Dependency
    Inversion direction ADR-001 itself argues for elsewhere (domain state shouldn't leak
    config's representation choices) and costs one small struct + one conversion function.

- [ ] **ADR-001's blast-radius safety claim doesn't hold uniformly across the 3 switches it
  cites.** ADR-001 states "All three have a `default:` fallback, so a missed case degrades
  to a wrong label... rather than a crash." Verified: `Status.String()`
  (`session/instance.go:50`) and `GetStatusDescription` (`session/instance_status.go:127`)
  both do have a real `default:` branch (`"Status(%d)"` / `"Unknown"`). But
  `reconcileSessions`'s `switch inst.Status` (`session/review_queue_poller.go:442`) has **no
  `default:` clause at all** — it's `case Active:`/`case Stopped:`/`case Hibernated:` with no
  catch-all; an unhandled status (including today's `Paused`/`Restoring`, and tomorrow's
  `PermanentlyFailed` if Task 2.5.1d were skipped) simply matches no case and the switch is
  a silent no-op. That happens to be exactly the desired behavior for `PermanentlyFailed`
  (per Task 2.5.1d's own reasoning: don't auto-revive), but it's safe *by coincidence*
  (no-action being correct here), not by a `default:` fallback the way the other two
  switches are. This is a documentation-accuracy issue now, but the next person who adds a
  `Status` value and reads ADR-001 as "all switches degrade safely by construction" could
  reasonably skip auditing `reconcileSessions` and add a status that actually *needed*
  corrective action there — silently getting the coincidental no-op instead.
  - **Remediation**: correct ADR-001's wording for this one switch (no default clause;
    safe only because "no action" happens to be correct for every currently-unhandled
    status) so it isn't cited as a uniform safety guarantee in future status additions.

## Nitpicks

- Domain Glossary's `NextRetryAt` zero-means-pending invariant is explicitly tested (Task
  1.1.1a's AC) — good; no action needed, just noting the one implicit invariant the task
  list explicitly called out was in fact covered.
- `evaluateSessionRetry` modeled on `evaluateRemediation`'s shape is an appropriate level of
  indirection given the existing precedent in `session/backlog_remediation.go` — not
  over-engineered, no concern here.
- Build-vs-buy consistency check: plan matches `research/build-vs-buy.md`'s recommendation
  (hand-rolled `backoffDelay`, no `cenkalti/backoff` dependency) — compliant, well-justified
  rejection reasoning included in the Pattern Decisions table.
- Story 2.5.1's phrasing "3 exhaustive Go switches" is slightly imprecise — Go doesn't
  enforce switch exhaustiveness on named int types, and one of the three has no `default:`
  at all (see Concerns above); "3 switches over `Status`, each degrading safely for a
  missed case (2 via explicit `default:`, 1 via no-op fallthrough)" would be more accurate
  for whoever implements Epic 2.5.
