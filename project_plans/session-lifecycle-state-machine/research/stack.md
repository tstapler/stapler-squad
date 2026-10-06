# Stack Research: goroutine-lifecycle state machine building block

Scope per requirements.md: research the tech stack for the shared building block that
will represent "should this goroutine stop or continue, and why" — evaluated against
`session/tymux` and `session/tmux`. This is stack/dependency research only; the audit of
*which* subsystems need migration is a separate Phase 2 workstream.

## Repo baseline (verified against this repo's go.mod)

- **Go version**: `go 1.26.6` (`go.mod:3`). Generics, `atomic.Pointer[T]`, etc. all
  available — no version constraint on any candidate approach.
- **No existing FSM/state-machine library** is present anywhere in `go.mod`/`go.sum`.
  `grep -iE "stateless|fsm|statemachine|looplab|qmuntal" go.mod go.sum` returns nothing.
  This means: (a) adopting `qmuntal/stateless` would be a genuinely new dependency, not
  something already pulled in transitively, so the "is a similar tool already present"
  argument does not shortcut the build-vs-buy decision; (b) there's no existing
  third-party FSM convention in the repo to stay consistent with — the only in-repo prior
  art is the hand-rolled `session/instance_state.go` (`TransitionDef`/`lookupTransition`
  table for `session.Status`), which requirements.md already flags as valuable prior art
  to study for *design*, not a migration target.
- **Existing OTel stack** (`go.mod` require block):
  - `go.opentelemetry.io/otel v1.44.0`
  - `go.opentelemetry.io/otel/trace v1.44.0`
  - `go.opentelemetry.io/otel/metric v1.44.0`
  - `go.opentelemetry.io/otel/sdk v1.44.0`
  - `go.opentelemetry.io/otel/sdk/metric v1.44.0`
  - `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0`
  - `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0`
  - `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0` (note: one
    minor behind the `v1.44.0` core line — pre-existing skew in the repo, not something
    to "fix" as part of this project; just don't introduce a *third* version by pinning
    something different for new code)
  - `connectrpc.com/otelconnect v0.8.0`
  - Any new observability code in this project (spans/metrics per lifecycle transition,
    per the Observability Requirements section) **must** use these already-vendored
    versions via the repo's own `telemetry` package wrapper — not add a second OTel SDK
    version or bypass the wrapper. Confirmed access points: `telemetry.GetTracer()`,
    `telemetry.GetMeter()`, `telemetry.StartSpan(ctx, name, opts...)`
    (`telemetry/telemetry.go:263,274,282`).
  - Established per-subsystem pattern to copy directly: `session/tmux/control_mode_observability.go`
    and `session/tmux/exec_gate_observability.go` — both use a package-level
    `sync.Once`-gated `registerXMetricsOnce()` against `telemetry.GetMeter()`, defining
    `metric.Int64Histogram`/`metric.Int64Counter` instruments with
    `metric.WithDescription`/`metric.WithUnit`, called once from `init()` and re-callable
    from tests. `session/streamhub/observability.go` is the third existing instance of
    the same pattern. Any new lifecycle-transition instrumentation in this project should
    follow this exact shape (a `<subsystem>_lifecycle_observability.go` file, package-level
    once-registered instruments, attributes keyed by subsystem/reason — not raw message
    content, to avoid cardinality blowup, per `recordControlModeCommand`'s existing
    comment about not logging full argument lists).

## `github.com/qmuntal/stateless` — concrete evaluation

Checked directly against the GitHub repo/API (2026-09-06), not from memory:

| Property | Finding |
|---|---|
| Module path | `github.com/qmuntal/stateless` |
| Latest release | `v1.8.0`, published 2026-02-10 (~7 months old as of today, but repo has 7 contributors and merged PRs as recently as that release — actively maintained, not abandoned) |
| Go module `go` directive | `go 1.24` — compatible with this repo's `go 1.26.6` |
| Dependency footprint | **Zero.** `go.sum` in the module is a 0-byte file (verified via `gh api repos/qmuntal/stateless/contents/go.sum` — `"size":0`). Stdlib-only (`context`, `sync`, `sync/atomic`, `reflect`). Adopting it adds exactly one `require` line and no transitive dependency tree to audit. |
| License | BSD-2-Clause (verified from `LICENSE` file contents) — permissive, no conflict with this repo. |
| Maintenance signal | 1,383 stars, 69 forks, 23 open issues, 7 contributors, not archived, last commit 2026-02-10 (a merged feature PR, `#102`/`#104`) — healthy small-library signal, not a single-maintainer dead project. |
| Adoption | Go port of the well-known .NET `stateless` library; a recognizable pattern for anyone who's used the .NET original. |

### API shape (from `statemachine.go`, `modes.go`, `README.md` — read directly, not paraphrased from memory)

- Core call is **`(sm *StateMachine) Fire(trigger Trigger, args ...any) error`** and its
  context-carrying twin **`FireCtx(ctx context.Context, trigger Trigger, args ...any) error`**.
  Both are **synchronous**: they run the full transition (guards → exit action → entry
  action → `OnTransitioned` callbacks) before returning, and return an `error` if the
  trigger isn't valid from the current state.
- **Two firing modes** (`modes.go`), configured at construction:
  - `FiringImmediate` (default): a `Fire` call from inside an entry/exit/transition
    callback is **not queued** — the library uses an `atomic.Uint64` op counter purely to
    answer `Firing()`; nested `Fire()` calls execute immediately/reentrantly.
  - `FiringQueued` (opt-in, `stateless.NewStateMachineWithExternalStorage(..., stateless.FiringQueued)`):
    triggers fired while already firing are queued (`[]queuedTrigger` behind a `sync.Mutex`)
    and drained FIFO by the same goroutine, so entry/exit callbacks can safely call `Fire`
    again without reentrancy bugs.
- **"Thread-safe" (README) means safe-for-concurrent-`Fire`-calls, not "subscribes to
  channel events."** There is no built-in event-loop/observer/pub-sub integration — the
  caller's own code is what decides "an event happened" and must explicitly call
  `sm.FireCtx(ctx, trigger, args...)` at that decision point. This maps directly onto the
  requirements.md Rabbit Hole warning: **validate this fits an async, event-driven
  per-goroutine decision before committing** — concretely, it fits the shape where a
  goroutine's own `select`/read loop (e.g. `readAttachLoop`) calls `FireCtx` once per
  received event/error, but it does **not** give you an async dispatcher for free; the
  goroutine's existing loop remains the event source and the library only replaces the ad
  hoc boolean bookkeeping at each decision point.
- Also provides: `PermittedTriggers`/`CanFireCtx` (introspection/guards),
  `OnEntry`/`OnExit`/`OnEntryFrom` (state hooks), `OnUnhandledTrigger` (callback for an
  invalid trigger from the current state — directly relevant to the "fails safe if a
  cause is unhandled" success metric, since an unhandled trigger produces a callback/error
  rather than silently doing nothing), `ToGraph()` (DOT export, matches the "diagram
  export" feature named in requirements.md's Alternatives section), and generic
  `NewStateMachine[S, T]`-style external-storage constructors for storing state outside
  the struct (e.g. keyed by an existing atomic/struct field) if that fits a given
  subsystem's existing shape better than an embedded state.

### Fit assessment against this project's shape (stack-level observation only — the
architecture/plan phase makes the final call)

- The "bounded set of terminal/non-terminal reasons" shape in requirements.md (Rabbit
  Holes: resist over-generalizing) maps cleanly onto `stateless`'s `State`/`Trigger` types,
  which are plain `any`-typed (commonly `int`/`string`/custom named types) — no
  hierarchical-state or guard usage is *required* to get value from it; those features
  (`SubstateOf`, `Permit(...).If(guard)`) are available but optional, so adopting the
  library doesn't force the fuller feature surface requirements.md warns against
  over-relying on.
- Because `Fire`/`FireCtx` are synchronous and return `error`, a call site that forgets to
  register a legal transition for a new cause gets an `error` back (or hits
  `OnUnhandledTrigger`) rather than silently falling through — this is a plausible match
  for the "fails safe... if the author forgets to handle it" success metric, **provided**
  the call site's error handling defaults to the safe (stop) behavior on that error, which
  is a design/plan decision, not something the library enforces on its own.
- Zero dependencies and BSD-2-Clause license mean the "cost proportionate as a shared
  dependency used by several `session/` subsystems" question from Alternatives resolves
  favorably on pure dependency-cost grounds — the open question that remains for
  Phase 3 (plan) is whether the synchronous `Fire`-per-event call shape is *ergonomically*
  a better fit than a bespoke generic `typedfsm.Machine[State, Event]` helper for this
  repo's specific goroutine shapes, not a dependency-risk question.

## Summary of stack implications for planning

- No new Go version requirement, no conflict with existing modules, no OTel version to
  reconcile beyond "reuse what's already there."
- If `qmuntal/stateless` is adopted: exactly one new `go.mod` require line, zero new
  transitive dependencies, permissive license — low-cost from a pure dependency-management
  standpoint. The decision is a design/ergonomics one (validated by the required small
  spike against `readAttachLoop`'s real shape), not a dependency-risk one.
- If a bespoke generic helper is chosen instead: it can be built entirely on this repo's
  existing Go 1.26.6 toolchain (generics, `atomic.Pointer[T]`) with no new dependency at
  all, using `session/instance_state.go`'s `TransitionDef`/`lookupTransition` table shape
  as its closest in-repo design precedent.
- Either path's observability instrumentation must reuse the pinned OTel versions above
  and the `telemetry.GetTracer()`/`GetMeter()`/`StartSpan()` + package-level
  `sync.Once`-registered-instruments pattern already established in
  `session/tmux/control_mode_observability.go`, `session/tmux/exec_gate_observability.go`,
  and `session/streamhub/observability.go`.
