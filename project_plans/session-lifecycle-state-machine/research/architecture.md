# Architecture Research: session-lifecycle-state-machine

Agent 3 (Architecture), SDD Phase 2. Scope per `requirements.md`: patterns for representing
a goroutine's continue-vs-stop decision as an explicit, exhaustively-checked state machine;
integration points; consistency requirements.

## Prior-analysis check (Step 2.75)

Skimmed `project_plans/engineering-excellence/research/architecture.md` (209 lines) for
concurrency-model / goroutine-lifecycle findings, per the 2-minute budget. Its only
`session/`-adjacent finding is that `session/instance.go` is a ~105KB god-file mixing
lifecycle, state machine, tmux integration, git worktree, terminal state, and approval logic,
recommended for a feature-line split (lifecycle/approval/terminal/git) to fix an import-cycle
problem with `server/analytics`/`server/events`. **Nothing there addresses goroutine
continue-vs-stop classification, reader-goroutine architecture, or the actor pattern** — it's
a file-size/import-cycle finding, not a concurrency-shape finding. Not otherwise relevant to
this project; did not open the other 9 listed files per the instruction (async-session-creation,
cold-restart-uuid-recovery, ci-hookurl-race-flake, etc. are incidental `session/tmux` mentions
by file path, not architecture analysis of this smell).

## 1. Prior art: `session.Status`'s TransitionDef/lookupTransition table

Lives in `session/state_machine.go` (108 lines total) — **not** `instance_state.go`, which
only *consumes* it (`instance_state.go`'s `transitionTo`/`transitionToLocked` call
`lookupTransition`). Full shape:

```go
type transitionKey struct{ from, to Status }

type TransitionDef struct {
    From  Status
    To    Status
    Guard func(ctx context.Context, i *Instance) error   // nil = unconditional
    After func(ctx context.Context, i *Instance)          // nil = no side effect
}

var transitionDefs = []TransitionDef{
    {From: Creating, To: Active},
    {From: Active, To: Hibernated, After: func(ctx context.Context, i *Instance) {
        go i.hibernateProcess(ctx)   // heavy I/O dispatched off the lock-holding path
    }},
    // ... 14 edges total, out of 6 states x 6 states = 36 possible pairs
}

var transitionIndex map[transitionKey]TransitionDef   // built once in init()

func lookupTransition(from, to Status) (TransitionDef, bool) {
    def, ok := transitionIndex[transitionKey{from, to}]
    return def, ok
}

func CanTransition(from, to Status) bool { _, ok := lookupTransition(from, to); return ok }
```

**How illegal transitions are rejected:** not by an exhaustive `switch` over `Status` at all —
by table *absence*. `transitionDefs` enumerates only the 14 valid `(From, To)` edges out of 36
possible pairs; everything not listed is illegal by construction. `lookupTransition`'s `ok`
return is the sole gate: `instance_state.go:43-46`'s `transitionTo` and `:81-84`'s
`transitionToLocked` both do:

```go
def, ok := lookupTransition(i.Status, to)
if !ok {
    return ErrInvalidTransition{From: i.Status, To: to}
}
```

So there is no compile-time exhaustiveness (no `exhaustive` linter coverage here — confirmed
per requirements.md's Feasibility Risks, this package is in the linter's excluded-packages
regex chain), but there *is* a single source of truth: `CanTransition`, `canTransitionLocked`,
and `transitionToLocked` all resolve through the same `lookupTransition` call, so they cannot
drift apart from each other (the comment at `state_machine.go:82-85` says this explicitly).
The "exhaustiveness" property this design actually buys is edge-completeness relative to the
table, not switch-arm completeness relative to the `Status` enum — a materially different (and
weaker) guarantee than what requirements.md's Success Metrics ask for ("adding a future
legitimate cause... fails safe... if the author forgets to handle it"). A missing edge here
fails safe already (`ErrInvalidTransition`, not silent pass-through), which is the right
default, but a missing *default classification of a new stop-reason value* is exactly the
failure mode a `switch` with no `default:` (or a `default: return unknown` that isn't "stop")
would need to guard against — this table shape doesn't structurally need that guard because
"absent = illegal" already is its default. **Takeaway for Phase 3**: the table-driven shape
(closed set of legal transitions, single lookup function, guard/after hooks) is worth
generalizing, but the goroutine-continue-vs-stop problem is shaped differently — it's not
"is (from, to) a legal edge," it's "given this stop-*reason*, do we stop or continue, and does
every possible reason have an assigned answer." That argues for an exhaustive `switch` over a
**closed reason enum** (with a compile-time-checked `default` case failing to "stop"), not a
transition-edge lookup table. `TransitionDef`'s `Guard`/`After` hook shape (function fields on
a struct, resolved via a map) is still a reasonable pattern to borrow if the new mechanism ends
up needing per-reason side effects (e.g. "on stop-reason=daemon-restart, call ReviveSession").

Also relevant to Phase 3's build-vs-buy question: this in-repo prior art is a **synchronous,
directly-invoked** table (`transitionTo` is called inline by whatever wants to transition), not
an async/event-driven one — it does not by itself demonstrate whether the same idiom fits
`readAttachLoop`'s "background goroutine reacting to a `Receive()` error" shape, which is the
`qmuntal/stateless` evaluation's specific concern per requirements.md's Alternatives section.

## 2. Current architecture: `session/tymux`'s reader goroutine

### `tymuxGRPCSession` struct (`session/tymux/session.go:42-176`)

One struct, `sync.RWMutex`-guarded (`mu`) for most fields, plus two lock-free fields used
specifically because the reader goroutine must check them without risking contending a lock a
`Close()`-in-progress call might already hold:

| Field | Type | Purpose |
|---|---|---|
| `stream` / `cancelAttach` / `streamDone` | `attachStream` / `context.CancelFunc` / `chan struct{}` | The one standing Attach stream for the session's whole lifetime; `streamDone` is closed by the reader goroutine when the stream ends for any reason |
| `closing` | `atomic.Bool` | Set by `Close()`/`DetachSafely()` **before** cancelling — the "whole session is closing" signal |
| `abortReconnect` | `chan struct{}` (closed once) | Interrupts an in-progress `ReconnectLoop` backoff wait or in-flight dial |
| `exited` / `exitReason` / `exitFired` | `bool` / `string` / `bool`, under `mu` | Whether/why a clean `Exited` event was observed; `exitFired` guards one-shot callback delivery |
| `reconnecting` / `reconnectAttempt` / `reconnectCause` / `reconnectSince` | mixed, under `mu` | `ReconnectState()` introspection |
| `backendRestarted` / `backendRestartedAt` | `bool` / `time.Time`, under `mu` | Distinguishes "still alive" from "alive but a fresh replacement process" |
| `teardownWait` | `time.Duration` | Bounds `teardownStandingStream`'s wait (test-shrinkable) |

### Goroutines and ownership

- **`readAttachLoop(ctx, paneID, stream, done)`** (`stream.go:191-232`) — one goroutine per
  stream *generation*, started by `openStandingStream`. Owns: the `for { stream.Receive() }`
  loop, dispatch to `handleAttachEvent`, and — on error — the exact 3-clause boolean this
  project exists to replace:
  ```go
  if s.closing.Load() || exited || ctx.Err() != nil {
      close(done)
      return
  }
  ```
  Three independently-set signals, three independent incidents (per requirements.md): `exited`
  (added after incident #1 — a clean pane exit misclassified as a drop), `ctx.Err() != nil`
  (added after incident #2 — a deliberate *reopen* wasn't distinguishable from a *close*), and
  `closing` (the original signal — deliberate `Close()`/`DetachSafely()`). These three are
  **not fully redundant** (requirements.md's Feasibility Risks confirms this): `exited` fires
  on natural clean-exit *before* any `cancel()`; `ctx.Err()` only covers deliberate
  cancellation; `closing` scopes to "whole session ending," not "this stream generation
  ending." A correct replacement must preserve exactly these three *origins* as distinguishable
  classified reasons, not collapse them.
- **`ReconnectLoop(paneID, cause)`** (`stream.go:523-607`) — invoked synchronously *from*
  `readAttachLoop` (same goroutine, no separate goroutine spawned) when the above check falls
  through to "genuine drop." Blocking here is fine specifically because nothing else waits on
  this goroutine except via `streamDone`. Returns `(stream, event, ok)`; `ok=false` means
  either exhaustion (attempts) or interruption (`closing` observed mid-loop) — the *caller*
  (`readAttachLoop`) cannot currently tell these two `ok=false` causes apart, though
  `ReconnectLoop` internally logs them differently (`"reconnect abandoned (deliberate close)"` vs
  `"reconnect exhausted, giving up"`) and only calls `deliverExit` (the terminal-failure
  notification path) on the exhaustion case, not the interrupted case. This is itself an
  instance of the same smell one layer up: the *caller's* only signal is a bare `bool`.
- **`teardownStandingStream()`** (`stream.go:148-165`) — called from `openStandingStream`
  (tear-down-before-reopen) and from `Close()`/`DetachSafely()` (full close, after
  `beginClosing()` sets `closing`). Cancels `cancelAttach`, then waits on `done` bounded by
  `teardownWait` (5s), logging and abandoning (leaking) the old reader goroutine on timeout —
  the incident #3 mitigation. This bounded-wait code is explicitly named in scope
  ("fold them into the new design rather than leaving both old and new mechanisms present").

### Ownership/ordering invariant this all sits inside

This whole call chain (`RestoreWithWorkDir`/`cacheFromSession` → `openStandingStream` →
`teardownStandingStream` → blocked `<-done`) runs **inside the owning `Instance`'s serialized
actor** (`session/actor.go`'s `runActor`, one goroutine per `LiveInstance`, draining a mailbox).
That is *why* incident #2 was severe: one wedged reader goroutine wedges the single actor
goroutine that processes every future command on that `Instance` — including the commands that
would destroy it. The new mechanism does not need to solve actor-wedging generally (out of
scope per requirements.md — actor shutdown ordering is a "genuinely different problem shape"
candidate, confirm before assuming same class), but it does need its bounded-wait/abandon
behavior (the incident #3 fix) to remain intact, since that specific mitigation is what
currently prevents a class of decision-classification bug from becoming an actor-wide hang.

## 3. `session/tmux/control_mode.go`: same smell or different shape?

Confirmed **structurally different**, not a copy of the tymux smell — worth stating precisely
rather than assuming it needs the identical fix:

- **`readControlModeOutput()`** (`control_mode.go:386-498`) uses a single `select { case
  <-doneCh: return; default: ... }` per scanned line, where `doneCh` is closed exactly once by
  `StopControlMode` (deliberate) — there is no OR-chain of independently-set booleans at this
  decision point. The exit-classification signal here is `t.intentionalStop` (`atomic.Bool`,
  read at `control_mode.go:491` and `:656`/`:668`), a single flag, not three.
  `t.onExitOnce` (`sync.Once`) guards firing the exit callback exactly once — a different
  one-shot idiom than tymux's `exited`/`exitFired` pair, but solving the same problem
  (check-before-and-after-registration race, per `control_mode.go:96-102`'s comment referencing
  the same `pane.rs` pattern tymux's `deliverExit` comment cites).
- There *is* an ad hoc-ish multi-branch shape, but it's spread across **three separate call
  sites** each independently checking `!t.intentionalStop.Load()` before firing `onExitOnce`
  (scanner-EOF fallback at `:491`, `%exit` at `:656`, `%session-closed` at `:668`) rather than
  one boolean-OR condition at one decision point. This is a different bug shape than tymux's
  (three *causes* merged into one condition) — it's the same *guard* duplicated at three
  independent trigger points. A shared mechanism could still help here (one classification
  call site instead of three copies of `if !intentionalStop.Load() { onExitOnce.Do(...) }`),
  but it is not the same incident pattern and doesn't have tymux's "which of these overlapping
  signals actually applies" ambiguity — `intentionalStop` alone is authoritative.
- **`TmuxServerRegistry.reconnectLoop()`** (`session/tmux/server_registry.go:416-509`) is the
  actual analogue of tymux's `ReconnectLoop`+`readAttachLoop` combination (own backoff, own
  "exited vs. reconnect" cycle) and is architecturally **cleaner already**: its only
  stop-vs-continue check is `select { case <-r.ctx.Done(): return; default: }`, evaluated at
  every loop iteration boundary — a single-cause classification via `ctx`, not a multi-flag OR.
  No incident history was found for this file in the grep sweep. This suggests the audit should
  treat `control_mode.go` and `server_registry.go` as two separate candidates with different
  verdicts, not lump "the classic backend" together as one smell/no-smell answer.

**Recommendation to the audit/planning phase:** `session/tmux` likely does *not* need the
tymux-identical fix applied uniformly. `control_mode.go`'s three-call-site duplication is a
much smaller, lower-risk refactor (consolidate a guard, not reclassify overlapping causes) than
tymux's problem, and `server_registry.go`'s loop doesn't obviously need touching at all. This
matters directly for the Risk Control section's default-backend caution: if the real fix needed
in `session/tmux` is narrow, that argues for treating it as lower-effort/lower-risk than
`session/tymux`'s fix, not higher, despite being the default backend — worth stating explicitly
in the plan rather than assuming symmetric migration cost.

## 4. Integration surface for a shared "lifecycle decision" type

Based on the two concrete reader-goroutine shapes above, a usable shared mechanism needs to
expose, at minimum:

1. **A closed reason type** representing "why did this generation/stream/process end," with at
   least these variants demonstrated as real, distinct origins by the tymux incidents:
   `DeliberateClose` (whole session ending — tymux's `closing`), `DeliberateSupersede` (this
   generation specifically being torn down for a reopen — tymux's `ctx.Err()` case,
   incident #2), `CleanExit` (the remote side reported a clean exit — tymux's `exited`,
   incident #1), `TransportDrop` (unexpected — triggers reconnect), plus room for a
   `ReconnectExhausted` terminal variant (today conflated into `TransportDrop`'s failure path
   via `deliverExit`). `control_mode.go`'s shape only needs `DeliberateClose` vs. everything
   else — the type must not force it to pretend it has three origins it doesn't have.
2. **One classification entry point** callable from inside a `Receive()`/scan error branch,
   taking whatever local signals that call site has (a boolean, a `context.Context`, an atomic
   flag) and returning the closed reason + a stop/continue decision — replacing the inline
   `||` chain with one function call whose return type cannot be silently ignored (Go idiom:
   return the enum, not a bool, and require the caller's `switch` to be exhaustive-checked by
   `go vet`'s `exhaustive` carve-out per requirements.md's Feasibility Risks).
3. **No imposed goroutine ownership or channel shape.** tymux's `done chan struct{}` (closed by
   the reader, waited on by `teardownStandingStream`) and `control_mode.go`'s `doneCh` (closed
   by the stopper, checked by the reader) are *opposite* directions of the same handshake — a
   shared mechanism that only supports one direction would force an awkward inversion on
   whichever backend doesn't already use it. The mechanism should classify a *decision*, not
   own the synchronization primitive that signals it.
4. **No coupling to `ReconnectLoop`'s retry/backoff algorithm** (explicitly out of scope) — the
   mechanism answers "stop or continue," not "how many times has it tried and how long should
   it wait." `ReconnectLoop`'s own `ok=false` ambiguity (exhaustion vs. interruption, §2 above)
   is a candidate for the *same* reason type, though — it should be able to express "stop
   because interrupted" vs. "stop because exhausted" as distinguishable reasons the caller
   (`readAttachLoop`) can log/span differently, which today it structurally cannot.
5. **Optional, not mandatory, observability hook.** `control_mode_observability.go`'s pattern
   (`telemetry.GetMeter()` → `meter.Int64Counter(name, ...)` registered once via an
   idempotent `sync.Once`-guarded `RegisterXMetrics()`, called from `init()`) and
   `telemetry.StartSpan(ctx, name, opts...)` (`telemetry/telemetry.go:282`) are the two hooks
   every migrated subsystem needs to call at the classification point and at generation
   start/end — but the mechanism's core type should not *require* a telemetry import to be
   usable (keeps it viable as a candidate for genuinely hot paths elsewhere later, and keeps
   the core testable without an OTel SDK dependency in unit tests).

## 5. Failure modes specific to a cross-cutting, two-backend migration

Beyond the incident-specific risks requirements.md already names, the migration *shape itself*
(shared new abstraction, two independently-owned reader-goroutine call sites, different risk
tiers) introduces these additional failure modes:

- **Partial migration leaves the two backends observably inconsistent.** If `session/tymux`
  moves to the new mechanism first (lower blast radius, per Risk Control) and `session/tmux`'s
  narrower fix (§3) lands later or not at all, a future maintainer reading `control_mode.go`
  and `stream.go` side-by-side sees two different idioms for "did this goroutine stop or
  continue" and may reasonably (but wrongly) assume `control_mode.go` also needs the full
  reason-enum treatment, triggering unnecessary rework. Mitigation: the ADR this project
  produces should explicitly record *why* `control_mode.go` got a narrower fix (or none) so
  the asymmetry reads as a decision, not an oversight — directly serving the Success Metrics
  requirement that any subsystem found and not migrated has "an explicit, reviewed reason."
- **The shared mechanism itself introduces a new concurrency bug that neither backend's
  existing test suite catches**, because both suites were written against the old ad hoc-flag
  shape and may implicitly assume its exact interleavings (e.g. a test asserting `exited` gets
  set synchronously before `deliverExit` fires — a coincidental ordering, not a documented
  contract). A generic `typedfsm`-style helper shared across packages is exactly the kind of
  code most likely to get a subtle bug from being exercised by two different call patterns
  (tymux's "check inline in an error branch" vs. control_mode's "check inline in a scan loop")
  that its own unit tests, written against only one shape, don't cover. Mitigation (already
  implied by requirements.md's Feasibility Risks and Phase 4 pre-mortem requirement): the new
  mechanism needs its own concurrency-focused test suite independent of either backend's,
  including a `-race` exercise of concurrent classification calls if the type is meant to be
  shared across goroutines (check whether it must be — tymux's `readAttachLoop` is
  single-goroutine-per-generation, so the classification call itself may not need to be
  thread-safe at all; forcing thread-safety it doesn't need is its own unnecessary-generality
  risk per the Rabbit Holes section).
- **`session/tmux` regression risk is asymmetric with `session/tymux`'s and easy to
  under-weight because the *code change* there is smaller.** A smaller diff in the
  default-traffic backend is not proportionally lower risk — `control_mode.go`'s consolidation
  of three `intentionalStop`-check call sites into one still touches the exact code path every
  production tmux session's exit detection runs through. Risk Control's call for
  extra-scrutiny/staged-rollout for any `session/tmux` change should apply by *which backend*,
  not by diff size.
- **Two backends migrating to one shared type creates a hidden coupling that makes future
  backend-specific changes harder to land independently**, e.g. adding a tymux-only stop reason
  later requires touching a type `session/tmux` also depends on, re-triggering its test suite
  and review even though tmux-specific behavior is unchanged. Mitigation direction (for
  planning, not decided here): keep the reason enum backend-agnostic and generic (transport
  drop / clean exit / deliberate close / deliberate supersede / exhausted), and let each
  backend define its *own* mapping from local signals onto those generic reasons, rather than
  growing backend-specific variants into one shared enum.
- **Rollback granularity risk**: Risk Control calls for separable per-subsystem commits/PRs so
  a bad migration in one backend doesn't force reverting the other — but if the shared type
  itself needs a fix after `session/tymux` ships and *before* `session/tmux` migrates, that fix
  necessarily touches code both backends will eventually depend on (or already do, if
  sequenced as one PR touching the shared package plus one consumer). The plan should decide
  whether the shared type's package ships and stabilizes (with its own tests) *before* either
  backend consumes it, specifically to avoid a revert of "shared type v1" cascading into an
  unwanted revert of whichever backend already adopted it.

## Files referenced

- `/home/tstapler/Programming/stapler-squad/session/state_machine.go` — `TransitionDef`/`lookupTransition` prior art
- `/home/tstapler/Programming/stapler-squad/session/instance_state.go` — consumer of the above (`transitionTo`, `transitionToLocked`)
- `/home/tstapler/Programming/stapler-squad/session/actor.go` — the generic actor (`runActor`, `sendSyncErr`/`send`/`sendCtx`) every backend's lifecycle commands serialize through
- `/home/tstapler/Programming/stapler-squad/session/tymux/stream.go` — `readAttachLoop`, `teardownStandingStream`, `openStandingStream`, `ReconnectLoop`
- `/home/tstapler/Programming/stapler-squad/session/tymux/session.go` — `tymuxGRPCSession` struct (lines 42-176)
- `/home/tstapler/Programming/stapler-squad/session/tmux/control_mode.go` — `readControlModeOutput`, `%exit` handling, `intentionalStop`/`onExitOnce`
- `/home/tstapler/Programming/stapler-squad/session/tmux/server_registry.go` — `TmuxServerRegistry.reconnectLoop` (lines 416-509), the cleaner analogue
- `/home/tstapler/Programming/stapler-squad/session/tmux/control_mode_observability.go`, `/home/tstapler/Programming/stapler-squad/telemetry/telemetry.go` — established span/metric pattern (`telemetry.GetMeter()`, `telemetry.StartSpan`)
- `/home/tstapler/Programming/stapler-squad/project_plans/engineering-excellence/research/architecture.md` — skimmed per Step 2.75; no relevant findings beyond the unrelated `instance.go` god-file note
