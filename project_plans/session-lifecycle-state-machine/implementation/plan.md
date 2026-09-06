# Implementation Plan: session-lifecycle-state-machine

**Feature**: A shared, exhaustively-checked `session/lifecycle` building block that replaces
ad hoc boolean-OR / repeated-flag classification of "should this goroutine stop or continue,
and why." **2 subsystems** (`session/tymux`, `session/tmux`) migrate onto the building
block's `lifecycle.Reason` mechanism; `session/external_streamer.go` gets **1 adjacent fix**
riding along — a swap of its timeout classifier for the building block's
`lifecycle.IsBenignTimeout` helper only, with no `Reason` adoption. See Phase 4's scope note
and the Domain Glossary/Dependency Visualization sections for the distinction.
**Date**: 2026-09-06
**Status**: Ready for implementation
**ADRs**: ADR-001-bespoke-lifecycle-reason-over-fsm-library.md

---

## Step 0.5 — Alternatives considered (creative pass)

Three distinct shapes were brainstormed for the shared mechanism before committing (full
evaluation: `research/build-vs-buy.md`):

1. **Adopt a caller-triggered FSM library** (`qmuntal/stateless` or `looplab/fsm`).
   *Strength*: mature, zero/low transitive dependencies, BSD/Apache license, free DOT/mermaid
   diagram export. *Weakness*: both libraries model *legal edges between named states fired by
   an external trigger* — `readAttachLoop`'s bug was never "an illegal transition was
   attempted," it was "the classification of *why* `Receive()` failed was incomplete," and
   neither library performs that classification; adopting one requires writing the exact same
   classification logic by hand first, then translating it into a trigger name — net new code
   with no corresponding risk reduction.
2. **Generalize `session/instance_state.go`'s `TransitionDef`/`lookupTransition` table** into a
   shared `from→to` graph type. *Strength*: proven in this exact repo under production load,
   zero new dependency, same team already understands it. *Weakness*: it's the identical
   graph-shaped mismatch as option 1 (a caller invokes a named transition; nothing here
   self-classifies a `Receive()` error), and generalizing it risks exactly the "conflating
   `session.Status`'s lifecycle with this project's" outcome `requirements.md`'s Rabbit Holes
   section explicitly forbids.
3. **A small bespoke closed-enum + exhaustive-switch helper**, package `session/lifecycle`.
   *Strength*: directly matches the actual problem shape — one already-known combination of
   local signals (closing/exited/ctx.Err(), or intentionalStop) gets classified, once, into a
   closed set of reasons, with a compile-time-checked (via the repo's existing `exhaustive`
   linter mechanism) safe-by-default zero value. *Weakness*: no free transition-graph tooling
   (guards, diagram export) if a future subsystem turns out to need real multi-state graphs —
   accepted, since none of the confirmed targets need that generality: the 2 subsystems
   migrating to `Reason` (`session/tymux`, `session/tmux`) plus `session/external_streamer.go`'s
   adjacent `IsBenignTimeout` swap (not a `Reason` migration — see Phase 4's scope note).

**Chosen: Option 3.** Recorded as ADR-001, with option 1/2 as the rejected alternatives (see
Pattern Decisions below).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `lifecycle.Reason` | A closed `int`-based type in the new `session/lifecycle` package representing "why did this goroutine generation end." | Mirrors `session/detection.DetectedStatus`'s existing iota-plus-`exhaustive`-linter idiom — the repo's one other linter-enforced closed enum. |
| `ReasonUnknown` | `Reason`'s zero value. Safe-by-default: any code path that never explicitly classifies a reason (a forgotten assignment, a future unhandled case) gets this, and `ShouldContinue()`/`ShouldFireExitCallback()` both treat it as "stop." | Never returned by any of this plan's classify functions in normal operation — it exists purely as the safety net for future omissions. |
| `ReasonDeliberateClose` | The whole session/session-owner is ending on purpose (tymux's `s.closing`; tmux's `t.intentionalStop`). | |
| `ReasonDeliberateSupersede` | This specific generation is being torn down deliberately for a reopen, but the owning session itself is *not* closing (tymux's `ctx.Err() != nil` when `s.closing` is false — incident #2's exact signal). | tymux-only; no `session/tmux` analogue exists today. |
| `ReasonCleanExit` | The remote side already reported a clean exit before this `Receive()`/read failed (tymux's `s.exited` — incident #1's exact signal). | |
| `ReasonTransportDrop` | An unexpected, non-deliberate stream/process end — triggers reconnect where the subsystem supports it. | Also the classification `control_mode.go`'s three call sites use for every *non*-deliberate stop, since that backend never reconnects mid-generation — see Story 3.1.1's Given/When/Then. |
| `ReasonReconnectExhausted` | `ReconnectLoop` ran out of attempts without reconnecting (as opposed to being interrupted by a deliberate close). | tymux-only. |
| `(Reason).ShouldContinue()` | Method answering "should the calling goroutine attempt to reconnect/retry" for a given `Reason`. Exhaustive `switch`, `default: return false`. | Used by `session/tymux`. |
| `(Reason).ShouldFireExitCallback()` | Method answering "should the one-shot exit callback fire" for a given `Reason` — a distinct predicate from `ShouldContinue()` because `session/tmux` never reconnects mid-generation but still needs the deliberate-vs-not distinction. Exhaustive `switch`, `default: return true` (fail toward *notifying*, since a silently-swallowed exit is the worse failure mode for a callback contract). | Used by `session/tmux`. |
| `(Reason).String()` | Human-readable label used as the `reason` span/metric attribute value. | |
| `lifecycle.StartGeneration(ctx, subsystem string) (context.Context, trace.Span)` | Opens one observability span for one goroutine-generation's lifetime, via `telemetry.StartLinkedBackgroundSpan` (the repo's existing pattern for a goroutine that outlives its triggering request/call). | Called once per generation (e.g. once per `readAttachLoop` invocation). |
| `lifecycle.EndGeneration(span trace.Span, subsystem string, reason Reason)` | Tags `span` with `reason`, ends it, and increments `session_lifecycle_ends_total{subsystem,reason}`. | Pairs with `StartGeneration`. |
| `lifecycle.RecordEnd(ctx context.Context, subsystem string, reason Reason)` | Lighter-weight alternative to `EndGeneration` for a lifecycle-ending decision that does *not* own a dedicated generation span (e.g. `ReconnectLoop`'s own give-up branches, which run mid-generation on `readAttachLoop`'s already-open span): adds a span event on `ctx`'s current span (a no-op if none) and increments the same counter. | |
| `session_lifecycle_ends_total` | The OTel counter (`metric.Int64Counter`), labeled `subsystem` and `reason`, registered once via `sync.Once` against `telemetry.GetMeter()` — this project's answer to "how often does each stop-reason fire, in which subsystem." | Mirrors `tmux_control_mode_timeouts_total`'s registration shape in `control_mode_observability.go`. |
| `session_lifecycle_active_generations` | A companion OTel `metric.Int64UpDownCounter` (gauge), labeled `subsystem`, incremented once by `StartGeneration` and decremented once by `EndGeneration`. | Exists specifically because incident #3's failure mode is a generation that never *returns* (abandoned, not killed) — `session_lifecycle_ends_total` and the per-generation span are both structurally silent for that case (nothing ever calls `EndGeneration`), so this gauge is the one signal that keeps moving: "active count not decreasing" is the trace-visible symptom of a recurrence. See Epic 1.3, Story 1.3.3. |
| `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` | Env-var-backed config flag (`config.TmuxLifecycleV2Enabled()`, mirroring the existing `STAPLER_SQUAD_USE_CONTROL_MODE` boolean-flag convention), default `false`. | Gates only `session/tmux/control_mode.go`'s migration (Phase 3, the default backend) — when `false`, the 3 call sites run the literal pre-migration `!t.intentionalStop.Load()` checks unchanged; when `true`, they run the consolidated `classifyControlModeExit()`-based path. `session/tymux` (Phase 2) is not gated — it's already opt-in via `config.TymuxSessionOverrides`. See Epic 3.1, Story 3.1.2 and Risk Control. |
| `lifecycle.AwaitBounded(done <-chan struct{}, wait time.Duration) bool` | Generalizes `teardownStandingStream`'s bounded-wait-then-abandon idiom: waits for `done` to close, returns `true` if it does within `wait`, `false` (caller logs + abandons the goroutine) otherwise. | Replaces the inline `select { case <-done: ; case <-time.After(...): }` in `session/tymux/stream.go`. |
| `lifecycle.IsBenignTimeout(err error) bool` | The canonical, typed "is this error an expected poll-timeout, not a real disconnect" classifier — `net.Error.Timeout()`, `os.ErrDeadlineExceeded`, `io.ErrUnexpectedEOF`, nothing else. | Replaces `session/external_streamer.go`'s `strings.Contains(err.Error(), "i/o timeout")` fallback (Phase 4). This is the only thing Phase 4 borrows from `session/lifecycle` — `external_streamer.go` does not adopt `Reason`, so it is not a third migrated subsystem: this project migrates **2 subsystems** to `Reason` (`session/tymux`, `session/tmux`) plus **1 adjacent fix** (`session/external_streamer.go`). See Phase 4's scope note. |
| `classifyStreamEnd` | `(*tymuxGRPCSession)` method in `session/tymux/stream.go` mapping `s.closing`/`s.exited`/`ctx.Err()` onto exactly one `lifecycle.Reason`, replacing the 3-clause `||`. | Precedence: `closing` > `exited` > `ctx.Err()` > default `ReasonTransportDrop`. |
| `classifyControlModeExit` | `(*TmuxSession)` method in `session/tmux/control_mode.go` mapping `t.intentionalStop` onto exactly one `lifecycle.Reason`, replacing the 3 independently-repeated `!t.intentionalStop.Load()` checks. | Only ever returns `ReasonDeliberateClose` or `ReasonTransportDrop` — this subsystem has no generation/supersede/clean-exit distinction to make. |
| Generation | One incarnation of a subsystem's reader/reconnect goroutine — a fresh `readAttachLoop`/`readControlModeOutput` invocation. Distinct from "the whole session," which can outlive many generations (tymux) or exactly one (tmux, which never reconnects mid-generation). | Not a new type this project introduces — the term names an existing concept (tymux's `ctx`/`done` pair, tmux's `doneCh`) so plan/code/tests use one word for it. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Shared lifecycle mechanism | Closed enum (`iota` + exhaustive `switch`, safe-default zero value) — Type-driven design | `type-driven-design` skill; `session/detection.DetectedStatus` in-repo precedent | `qmuntal/stateless` / `looplab/fsm` (caller-triggered FSM libraries) | Both model synchronous, externally-fired named-event transitions; `readAttachLoop`'s bug is *async self-classification of an already-known combination of local signals*, which neither library performs — adopting either would still require hand-writing the classification, then wrapping it in an adapter that translates the result into a trigger name. Net new code, zero risk reduction (`research/build-vs-buy.md` Option 1/2). |
| Shared lifecycle mechanism (alternate rejection) | (same as above) | | Generalizing `session/instance_state.go`'s `TransitionDef`/`lookupTransition` into a shared `from→to` graph | Same graph-shaped mismatch as the FSM libraries (a caller invokes a named transition; nothing here classifies a `Receive()` error), plus it risks conflating `session.Status`'s lifecycle with this project's — explicitly forbidden by `requirements.md`'s Rabbit Holes. Only its *idioms* (typed error on an unhandled edge, the `exhaustive` carve-out mechanics) are reused, not its transition-graph machinery. |
| `Reason`'s exhaustiveness guarantee | Plain `type Reason int` + `iota` constants, enforced via the repo's existing `.golangci.yml` `exhaustive` carve-out mechanism (mirroring `session/detection.DetectedStatus` exactly) | GoF/idiomatic-Go closed-type discussion in `research/pitfalls.md` §2 | An unexported-field-gated "true" closed type (`type Reason struct{ v int }` with unexported `v`) | The repo already has one working, linter-enforced convention for exactly this shape (`DetectedStatus`); diverging to a stricter idiom for only this one type would be an unjustified inconsistency. The "untyped-int leakage" risk `pitfalls.md` flags is mitigated in practice because `Reason` values are only ever *produced* by this plan's `classify*` functions (never constructed ad hoc by calling code) and are never exported for external construction beyond the six named constants. |
| Cross-subsystem read of "why did this end" | Consuming packages never `switch` over `Reason` themselves — they only call `Reason.ShouldContinue()`/`ShouldFireExitCallback()`, whose exhaustiveness is enforced once, inside `session/lifecycle` | `.claude/rules/instance-lock-free-reads.md`'s "one sanctioned accessor" pattern, generalized | Exposing `Reason`'s underlying value for callers to `switch` on directly | Keeps the `exhaustive` linter carve-out scoped to exactly one package (`session/lifecycle`) instead of needing new carve-out entries (and review of every pre-existing switch) in `session/tmux` and `session/tymux`, both large packages full of unrelated `iota`+`default:` switches the repo's own `.golangci.yml` comment says are intentionally *not* exhaustive-checked. |
| Generation-fencing ("is this event/error from my generation or a superseded one") | Left as each subsystem's existing mechanism (tymux: per-generation `ctx`/`done` pair; tmux: `doneCh` identity comparison, `control_mode.go:476-481`) | `research/features.md` §5 — `tryInstallPTYTriple`'s generation-counter idiom and `control_mode.go`'s `doneCh`-identity check are both already-correct prior art | A shared generic `Generation`/fencing-token type in `session/lifecycle` | `requirements.md`'s Rabbit Holes explicitly warns against generalizing beyond what confirmed findings need; both existing mechanisms are correct today (confirmed by direct reading, `research/architecture.md` §2-3) and solve a *different* concern (which generation) from `Reason` (why did it end) — folding them into one type would be the "single flat enum won't fit all three" mistake `research/features.md` edge case #1 warns against. |
| Bounded-wait-then-abandon (incident #3's mitigation) | Generalized into `lifecycle.AwaitBounded`, a first-class helper in the shared package | `research/pitfalls.md` §1 ("a timeout... doesn't even live in the `if`" — explicitly named as a design smell to fix, not preserve) | Leaving `maxTeardownWait`'s inline `select`/`time.After` as a `session/tymux`-local pattern | Per the project-specific guidance, incident #3's mitigation must be folded into the new design, not left standing beside it; per `pitfalls.md`, any future subsystem that adopts `Reason` and also blocks on a goroutine's exit needs this same contract, so it belongs in the shared package now rather than being re-invented per subsystem later. |
| Timeout classification (`external_streamer.go`) | One canonical `lifecycle.IsBenignTimeout(err error) bool`, typed-error-only (no string matching) | `research/features.md` §4 ("a classifier that's *already* falling through to `strings.Contains`... is a distinct failure mode from boolean-OR proliferation") | Wrapping the existing string-matching chain in a nicer-looking type without removing the string match | The string-matching fallback is itself the smell the Success Metrics target (a classification that silently breaks if a wrapped library changes its error text) — wrapping it without removing it would "adopt an exhaustive-checked enum" in name only, per `pitfalls.md`'s explicit warning against exactly that. |
| `session/actor.go` | Not migrated; documented as an explicit, reviewed out-of-scope decision (Phase 5) | `research/pitfalls.md` §3, `research/features.md` §3 — confirmed by direct reading: `runActor`'s own `select` is single-signal and correct | Treating it as a 4th migration target because it was named as a starting candidate in `requirements.md` | Two independent research agents (architecture.md §3, pitfalls.md §3) confirm `runActor`'s gap is "a command can block the actor indefinitely," a liveness/preemption problem, not a multi-signal classification problem — forcing it into the same `Reason` type would be the "conflating problem shapes" rabbit hole `requirements.md` names explicitly. |

---

## Migration Plan

**N/A.** Confirmed from `requirements.md`'s Risk Control section: "no data migration, no
persisted-state format change, no wire-protocol change anticipated." This project changes only
in-process goroutine classification logic and its observability; no schema, config format, or
on-disk/wire state is touched.

## Observability Plan
- **Logs**: existing `log.Info`/`log.Warn` call sites keep firing unchanged (e.g.
  `teardownStandingStream`'s abandon-warning, `ReconnectLoop`'s abandoned/exhausted logs); no
  log-volume increase — the new signal is span/metric, per requirements.md's explicit ask that
  a future incident be diagnosable from a trace, not a log/pprof dump.
- **Metrics**: `session_lifecycle_ends_total{subsystem, reason}` (new `metric.Int64Counter`,
  `session/lifecycle/observability.go`) — one increment per classified generation end across
  all three migrated subsystems (`subsystem` values: `tymux_stream`, `tymux_reconnect`,
  `tmux_control_mode`). Answers "how often does each stop-reason fire, in which subsystem"
  without per-file log archaeology (Success Metrics).
  Alongside it, `session_lifecycle_active_generations{subsystem}` (new
  `metric.Int64UpDownCounter`, same file) — incremented at `StartGeneration`, decremented at
  `EndGeneration` — exists specifically because incident #3's generation never returns at all:
  an abandoned/wedged generation is invisible to `session_lifecycle_ends_total` and its span
  (neither ever fires), but shows up here as "active count not decreasing." See Epic 1.3,
  Story 1.3.3.
- **Spans**: one span per generation via `lifecycle.StartGeneration`/`EndGeneration`
  (`telemetry.StartLinkedBackgroundSpan` under the hood — the repo's existing pattern for a
  goroutine outliving its triggering request), tagged with the terminal `reason` attribute.
- **Alerts**: no new alerting/oncall requirement for `session/tymux` (still opt-in, internal,
  per requirements.md). For `session/tmux` (default backend): no new alert either —
  `session_lifecycle_ends_total{subsystem="tmux_control_mode"}` is a low-cardinality counter a
  future dashboard panel can chart if the team decides it's worth one post-migration; this plan
  does not itself add a panel or alert (requirements.md leaves that decision open, not assumed).

## Risk Control
- **Feature flag**: `session/tymux` remains not gated by this project — it's already opt-in via
  `config.TymuxSessionOverrides` (existing, unrelated to this project), which is itself the
  risk-control mechanism requirements.md accepts for that side. `session/tmux` (Phase 3, the
  default backend every currently-running session restarts onto) **is** gated:
  `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` (env-var-backed, mirroring the existing
  `STAPLER_SQUAD_USE_CONTROL_MODE` convention), default `false` for one full release. Default
  `false` keeps `control_mode.go`'s 3 call sites running the literal, already-proven-in-
  production `!t.intentionalStop.Load()` checks unchanged; `true` opts a build into the new
  consolidated `classifyControlModeExit()`-based path (Epic 3.1). This is the distinct
  mechanism requirements.md's Risk Control section requires beyond "own PR, own regression
  tests" — see Epic 3.1, Story 3.1.2 for the flag's own tasks, and Epic 5.3, Task 5.3.2a for
  its removal once bake time has passed and the new path is proven stable.
- **Rollback procedure**: standard `git revert` **per phase's own commit/PR** — Phase 1 (shared
  package, no consumer yet), Phase 2 (`session/tymux`), Phase 3 (`session/tmux`), and Phase 4
  (`session/external_streamer.go`) each land as separable commits/PRs specifically so a bad
  migration in one subsystem doesn't force reverting another (requirements.md Risk Control).
  Phase 1 ships and stabilizes (with its own full test suite) *before* any consumer lands, so a
  post-hoc fix to `session/lifecycle` itself never straddles a partially-migrated consumer. For
  Phase 3 specifically, flipping `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` back to `false` is a faster
  rollback lever than a full `git revert`/binary rollback if the new path misbehaves after the
  systemd unit picks it up.
- **Staged rollout**: land per-phase, in the stated Phase 1→2→3→4 order — `session/tymux`
  (opt-in, well-tested, 3-times-proven target) migrates before `session/tmux` (default,
  highest-traffic backend), which per requirements.md's Risk Control gets proportionally more
  scrutiny (its own from-scratch regression tests, its own commit, no riding on
  `session/tymux`'s existing suite, *and* the `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` bake-time flag
  above) despite its diff being smaller.

## Unresolved Questions

None. Every question `requirements.md` left open (`exhaustive` carve-out scope, `session/tmux`
vs. `session/tymux` sequencing, exact migration list) is resolved by this plan: carve-out is
scoped to `session/lifecycle` only (Pattern Decisions); sequencing is
tymux-then-tmux-then-external_streamer (Risk Control above); the migration list is **2
subsystems migrated to `lifecycle.Reason`** — `session/tymux/stream.go`,
`session/tmux/control_mode.go` — **plus 1 adjacent fix riding along**,
`session/external_streamer.go`'s `readLoop` (swaps to `lifecycle.IsBenignTimeout` only, no
`Reason` adoption — see Phase 4's scope note), with `session/actor.go` and
`session/tmux/server_registry.go` explicitly out of scope (Pattern Decisions, Phase 5).

## Dependency Visualization

```
Phase 1: session/lifecycle (shared package, no consumer)
  Epic 1.1 Reason type ──┬──> Epic 1.2 exhaustive carve-out
                         ├──> Epic 1.3 observability (StartGeneration/EndGeneration/RecordEnd
                         |       + Story 1.3.3's active_generations gauge)
                         ├──> Epic 1.4 AwaitBounded
                         └──> Epic 1.5 IsBenignTimeout
                                        |
              (Phase 1 must be fully green, incl. -race, before Phase 2 starts)
                                        |
        +-------------------------------+-------------------------------+
        v                               v                               v
Phase 2: session/tymux            Phase 3: session/tmux           Phase 4: external_streamer
  (migrates to Reason)              (migrates to Reason)             (adjacent fix — NOT a
  Epic 2.1 classifyStreamEnd        Epic 3.1 classifyControlModeExit  Reason migration; borrows
    (+ Story 2.2.2 gauge regr.)         (Story 3.1.2: gated behind      only IsBenignTimeout)
  Epic 2.2 observability                 STAPLER_SQUAD_TMUX_LIFECYCLE_V2, Epic 4.1 IsBenignTimeout swap
  Epic 2.3 AwaitBounded fold-in           default false)                Epic 4.2 regression tests
  Epic 2.4 regression tests         Epic 3.2 observability (same flag)  Epic 4.3 verification
  Epic 2.5 verification             Epic 3.3 from-scratch tests
                                     Epic 3.4 verification (own PR, both flag values)
        |                               |                               |
        +-------------------------------+-------------------------------+
                                        v
                    Phase 5: scope documentation + ADR + follow-on tracking
                      (session/actor.go, server_registry.go noted out-of-scope;
                       ADR-001 written; command-execution-SLA gap tracked)
```

**Project totals**: **2 subsystems migrated to the `lifecycle.Reason` mechanism**
(`session/tymux`, `session/tmux`) **+ 1 adjacent fix riding along**
(`session/external_streamer.go`'s `IsBenignTimeout` swap) — not "3 subsystems migrated." See
Phase 4's scope note for why it's counted separately.

Phases 2, 3, and 4 have no dependency on each other (independent subsystems, independent
files) and could run in parallel once Phase 1 ships — sequenced 2→3→4 above per Risk Control's
"opt-in backend before default backend" preference, not a hard technical dependency.

---

## Phase 1: Shared mechanism — `session/lifecycle` (built and tested in isolation, no consumer yet)

### Epic 1.1: `Reason` closed type

**Goal**: A single, safe-by-default closed enum representing "why did this goroutine
generation end," usable by every migrated subsystem without any of them needing their own
`exhaustive`-linted switch.

#### Story 1.1.1: Define `Reason` and its two predicate methods
**As a** developer migrating a reader-goroutine's error branch, **I want** one type with
`ShouldContinue()`/`ShouldFireExitCallback()` methods, **so that** I replace a boolean-OR/
repeated-flag check with one function call whose omission-safety is enforced by the type,
not by my memory.
**Acceptance Criteria**:
- `Reason`'s zero value (`ReasonUnknown`) makes both predicates return their safe answer.
  - *Given* `var r lifecycle.Reason` (never assigned), *When* `r.ShouldContinue()` is called,
    *Then* it returns `false`. *When* `r.ShouldFireExitCallback()` is called, *Then* it
    returns `true` (fail toward notifying — a silently-swallowed exit is the worse outcome
    for a one-shot callback contract).
- A future, currently-unhandled `Reason` value also fails safe.
  - *Given* `r := lifecycle.Reason(999)` (simulating a variant added later without updating
    the switch), *When* `r.ShouldContinue()` is called, *Then* it returns `false` (the
    `default:` branch, not a compile error, since Go has no sum types — see ADR-001).
- The six named reasons round-trip through `String()` for use as span/metric attribute values.
  - *Given* `lifecycle.ReasonCleanExit`, *When* `.String()` is called, *Then* it returns
    `"clean_exit"` (snake_case, matching `control_mode_observability.go`'s existing
    `attribute.String("command", ...)` label convention).

**Files**: `session/lifecycle/reason.go`

##### Task 1.1.1a: Create `session/lifecycle/reason.go` with the `Reason` type and 6 constants (~3 min)
- New file, `package lifecycle`. Declare `type Reason int` and:
  `ReasonUnknown` (iota 0), `ReasonDeliberateClose`, `ReasonDeliberateSupersede`,
  `ReasonCleanExit`, `ReasonTransportDrop`, `ReasonReconnectExhausted`.
- Doc comment on `Reason` states the zero-value safety contract explicitly (mirroring
  `session/detection.DetectedStatus`'s doc comment style).
- Files: `session/lifecycle/reason.go`

##### Task 1.1.1b: Implement `String()` (~2 min)
- `switch` over all 6 constants returning the snake_case labels above; `default: return
  "unknown"`.
- Files: `session/lifecycle/reason.go`

##### Task 1.1.1c: Implement `ShouldContinue()` (~3 min)
- `switch r { case ReasonTransportDrop: return true; case ReasonDeliberateClose,
  ReasonDeliberateSupersede, ReasonCleanExit, ReasonReconnectExhausted, ReasonUnknown: return
  false; default: return false }`.
- Files: `session/lifecycle/reason.go`

##### Task 1.1.1d: Implement `ShouldFireExitCallback()` (~3 min)
- `switch r { case ReasonDeliberateClose: return false; case ReasonDeliberateSupersede,
  ReasonCleanExit, ReasonTransportDrop, ReasonReconnectExhausted, ReasonUnknown: return true;
  default: return true }`.
- Files: `session/lifecycle/reason.go`

#### Story 1.1.2: Prove the safe-default guarantee with tests, not just code review
**As a** future maintainer adding a 7th `Reason` variant, **I want** a test that already
fails if I forget to update `ShouldContinue()`/`ShouldFireExitCallback()`'s switch, **so that**
the omission is caught in CI, not in a 4th production incident.
**Acceptance Criteria**:
- An unhandled `Reason` value stops (per Success Metrics), verified by test, not inspection.
  - *Given* `lifecycle.Reason(999)`, *When* `ShouldContinue()` and `ShouldFireExitCallback()`
    are both called, *Then* the test asserts `false` and `true` respectively (see Story
    1.1.1's criteria) — this is the literal test the Success Metrics section requires.

**Files**: `session/lifecycle/reason_test.go`

##### Task 1.1.2a: `TestReason_UnhandledValue_FailsSafe` (~4 min)
- Table test over `[]lifecycle.Reason{lifecycle.Reason(999), lifecycle.Reason(-1)}`, asserting
  `ShouldContinue() == false` and `ShouldFireExitCallback() == true` for each.
- Files: `session/lifecycle/reason_test.go`

##### Task 1.1.2b: `TestReason_ZeroValue_IsSafeDefault` and `TestReason_String_AllNamedConstants` (~4 min)
- First: `var r lifecycle.Reason` (no assignment) asserts the same two predicates as above.
- Second: table test over all 6 named constants asserting `String()` returns the documented
  label (no empty string, no `"unknown"` for a named constant).
- Files: `session/lifecycle/reason_test.go`

### Epic 1.2: Wire the `exhaustive` linter onto `session/lifecycle` only

**Goal**: Make `Reason`'s exhaustiveness a linter-enforced fact, not just an idiom, scoped
narrowly enough to avoid breaking every unrelated `iota`+`default:` switch already living in
`session/tmux`/`session/tymux` (per `research/pitfalls.md`'s explicit warning about getting
this regex chain wrong in either direction).

#### Story 1.2.1: Add `session/lifecycle` to the `exhaustive`-enforced set
**As a** reviewer of a future PR adding a 7th `Reason` value, **I want** `make lint` to fail
if `ShouldContinue()`'s switch doesn't handle it, **so that** the enforcement is real, not
just structurally possible.
**Acceptance Criteria**:
- `session/lifecycle`'s switches are checked by `exhaustive`; no other `session/*` package's
  checked status changes.
  - *Given* `.golangci.yml`'s current exclusion chain (`^session/[^d]` through
    `^session/dete[^c]`, excluding `exhaustive` from every `session/*` package except
    `session/detection`), *When* the chain is extended with an equivalent `l`-branch, *Then*
    `golangci-lint run --enable-only exhaustive ./session/...` reports `session/lifecycle`
    and `session/detection` as checked and every other `session/*` package as excluded
    (verified by running the command, not by reading the regex).

**Files**: `.golangci.yml`

##### Task 1.2.1a: Extend the exclusion regex chain for `session/lifecycle` (~5 min)
- In `.golangci.yml`'s `linters.exclusions.rules`, change the existing
  `- path: "^session/[^d]"` entry to `- path: "^session/[^dl]"`, and add 5 new entries
  mirroring the existing `d`-branch exactly, spelling out `lifecycle`:
  `^session/l[^i]`, `^session/li[^f]`, `^session/lif[^e]`, `^session/life[^c]`,
  `^session/lifec[^y]` (each `linters: [exhaustive]`).
- Update the existing comment above the chain ("exhaustive enforces DetectedStatus switch
  coverage in session/detection/ only...") to also name `session/lifecycle`'s `Reason` type.
- Files: `.golangci.yml`

##### Task 1.2.1b: Manually verify the carve-out actually fires, and record the verification (~5 min)
- Temporarily add an unhandled constant (`ReasonHalfClose`) to `reason.go` without adding it
  to `ShouldContinue()`'s switch; run `make lint` (or
  `golangci-lint run --enable-only exhaustive ./session/lifecycle/...`); confirm it fails with
  an exhaustive-missing-case error naming `ShouldContinue`. Revert the temporary addition.
- Not a permanent automated test (a permanent test can't leave the switch intentionally
  broken) — record the verification command and its failing output in the PR
  description/commit message for this task, per `research/pitfalls.md` §6.3's explicit ask
  ("write a test that actually proves the carve-out fires... don't accept 'we used an
  exhaustive-shaped idiom' as equivalent to 'the linter enforces it here'"). Story 1.1.2's
  `TestReason_UnhandledValue_FailsSafe` is the permanent regression complement (proves the
  runtime *behavior* stays safe even if a future switch omission slips past review).
- Files: none committed (verification-only; no file change survives this task)

### Epic 1.3: Observability plumbing

**Goal**: `session_lifecycle_ends_total{subsystem,reason}`, a per-generation span, and a
`session_lifecycle_active_generations{subsystem}` gauge that stays elevated if a generation is
abandoned rather than returning (Story 1.3.3 — closing the specific blind spot incident #3
would otherwise leave in this new observability), following the exact
`sync.Once`-registered-instruments pattern already established in
`control_mode_observability.go`/`exec_gate_observability.go`, callable by any subsystem without
forcing an OTel import into `session/lifecycle`'s core `Reason` type (Story 1.1's tests have no
OTel dependency).

#### Story 1.3.1: Register the shared counter
**As an** operator debugging a future incident, **I want** one counter labeled by subsystem and
reason, **so that** "how often does each stop-reason fire, in which subsystem" is a query, not
log archaeology.
**Acceptance Criteria**:
- The counter registers exactly once, idempotently, safe to call from `init()` and again from
  tests.
  - *Given* `lifecycle.RegisterMetrics()` has already been called once (e.g. via package
    `init()`), *When* it is called a second time (e.g. from a test's setup), *Then* it returns
    the same cached `nil` error without re-registering the instrument with
    `telemetry.GetMeter()` (verified via `sync.Once`'s single-execution guarantee, matching
    `RegisterControlModeMetrics`'s existing shape).

**Files**: `session/lifecycle/observability.go`

##### Task 1.3.1a: `registerOnce`/`registerErr`/`endCounter` + `RegisterMetrics()`/`init()` (~5 min)
- Mirror `control_mode_observability.go`'s exact shape: package-level `sync.Once`,
  `registerMetricsOnce()` calling `telemetry.GetMeter().Int64Counter("session_lifecycle_ends_total",
  metric.WithDescription("Count of goroutine-lifecycle-generation endings, by subsystem and reason"))`,
  `init()` logging (via `log.Error`) if registration fails.
- Files: `session/lifecycle/observability.go`

#### Story 1.3.2: `StartGeneration`/`EndGeneration`/`RecordEnd`
**As a** subsystem author instrumenting a reader goroutine, **I want** one call at generation
start and one at generation end, **so that** I get a span + counter increment without hand-
rolling OTel boilerplate per subsystem.
**Acceptance Criteria**:
- `StartGeneration` never panics or blocks even with no `telemetry.Initialize` call (matching
  `GetTracer()`'s documented no-op-when-disabled contract).
  - *Given* telemetry is not initialized (test environment, no `telemetry.Initialize()` call),
    *When* `lifecycle.StartGeneration(context.Background(), "tymux_stream")` is called, *Then*
    it returns a non-nil `context.Context` and a non-nil (no-op) `trace.Span` without error or
    panic.
- `EndGeneration` tags the span with the reason and increments the counter exactly once.
  - *Given* a span from `StartGeneration(ctx, "tymux_stream")`, *When*
    `lifecycle.EndGeneration(span, "tymux_stream", lifecycle.ReasonCleanExit)` is called,
    *Then* `session_lifecycle_ends_total{subsystem="tymux_stream",reason="clean_exit"}`
    increments by exactly 1 (asserted via a test-local OTel SDK `metric.Reader`, matching
    `control_mode_observability_test.go`'s existing test-instrumentation pattern if one
    exists, else a minimal in-test `metric.NewManualReader()`).

**Files**: `session/lifecycle/observability.go`

##### Task 1.3.2a: `StartGeneration(ctx, subsystem) (context.Context, trace.Span)` (~4 min)
- `telemetry.StartLinkedBackgroundSpan(ctx, "session.lifecycle."+subsystem)`, then
  `span.SetAttributes(attribute.String("lifecycle.subsystem", subsystem))` before returning.
- Files: `session/lifecycle/observability.go`

##### Task 1.3.2b: `EndGeneration(span trace.Span, subsystem string, reason Reason)` (~4 min)
- `span.SetAttributes(attribute.String("lifecycle.reason", reason.String()))`; `span.End()`;
  increment `endCounter` with `attribute.String("subsystem", subsystem)`,
  `attribute.String("reason", reason.String())` (nil-guarded, matching
  `recordControlModeCommand`'s existing `if cmCommandDurationHist != nil` guard).
- Files: `session/lifecycle/observability.go`

##### Task 1.3.2c: `RecordEnd(ctx context.Context, subsystem string, reason Reason)` (~4 min)
- For lifecycle-ending decisions with no dedicated generation span (e.g. `ReconnectLoop`'s
  give-up branches): `trace.SpanFromContext(ctx).AddEvent("session.lifecycle.end",
  trace.WithAttributes(...))` (no-op on a context with no active span) plus the same counter
  increment as `EndGeneration`.
- Files: `session/lifecycle/observability.go`

##### Task 1.3.2d: `observability_test.go` (~5 min)
- `TestStartGeneration_NoTelemetryInitialized_ReturnsUsableSpan`,
  `TestEndGeneration_IncrementsCounterWithCorrectLabels`,
  `TestRegisterMetrics_Idempotent_SecondCallReturnsCachedResult`.
- Files: `session/lifecycle/observability_test.go`

#### Story 1.3.3: `session_lifecycle_active_generations` gauge — closes the "abandoned generation" blind spot
**As an** operator, **I want** a signal that keeps moving even when a generation's goroutine
never returns, **so that** a recurrence of incident #3 (the reader "abandoned, not
force-killed... Go has no API to forcibly interrupt a blocked network read" per `stream.go`'s
own doc comment) is visible in a trace/dashboard instead of looking identical to "nothing
happened" — `session_lifecycle_ends_total` and the per-generation span are both structurally
silent for a generation that never calls `EndGeneration` (Adversarial Review Blocker: this is
the single most severe incident class this project exists to make diagnosable, and the
plan's `ends_total`-only design was blind to exactly that case).
**Acceptance Criteria**:
- The gauge increments on `StartGeneration` and decrements on `EndGeneration`, for every
  subsystem, with no other trigger.
  - *Given* `lifecycle.StartGeneration(ctx, "tymux_stream")` is called, *When* the resulting
    metric state is inspected via a test-local `metric.ManualReader`, *Then*
    `session_lifecycle_active_generations{subsystem="tymux_stream"}` reads `1` higher than
    before the call; *When* `lifecycle.EndGeneration(span, "tymux_stream", <any reason>)` is
    subsequently called for that same generation, *Then* it reads back to its prior value.
- A generation that starts but never ends leaves the gauge elevated indefinitely — this is the
  literal safety property the Story exists for, not an incidental side effect.
  - *Given* `lifecycle.StartGeneration(ctx, "tymux_stream")` is called and `EndGeneration` is
    deliberately never called for it (simulating an abandoned goroutine), *When* the gauge is
    read at any later point, *Then* it still reflects that generation as active — proven by a
    unit test in `session/lifecycle` (Task 1.3.3c) and, end to end against the real motivating
    scenario, by Epic 2.2's Story 2.2.2.

**Files**: `session/lifecycle/observability.go`

##### Task 1.3.3a: Register `activeGenerationsGauge` alongside `endCounter` (~4 min)
- In the same `registerMetricsOnce()` from Task 1.3.1a, add
  `telemetry.GetMeter().Int64UpDownCounter("session_lifecycle_active_generations",
  metric.WithDescription("Count of currently-active (started, not yet ended) goroutine-lifecycle generations, by subsystem — stays elevated if a generation is abandoned rather than returning"))`,
  stored in the same package-level var block as `endCounter`.
- Files: `session/lifecycle/observability.go`

##### Task 1.3.3b: Wire the gauge into `StartGeneration`/`EndGeneration` (~4 min)
- Edit Task 1.3.2a's `StartGeneration` body to call
  `activeGenerationsGauge.Add(ctx, 1, metric.WithAttributes(attribute.String("subsystem", subsystem)))`
  (nil-guarded, same convention as `endCounter`) right before it returns.
- Edit Task 1.3.2b's `EndGeneration` body to call
  `activeGenerationsGauge.Add(ctx, -1, metric.WithAttributes(attribute.String("subsystem", subsystem)))`
  right after `span.End()`. `RecordEnd` (Task 1.3.2c) does **not** touch the gauge — it exists
  for lifecycle-ending decisions with no dedicated generation span (e.g. `ReconnectLoop`'s
  give-up branches), which never called `StartGeneration` in the first place, so there is
  nothing for it to decrement.
- Files: `session/lifecycle/observability.go`

##### Task 1.3.3c: `TestActiveGenerationsGauge_StartIncrements_EndDecrements_AbandonedStaysElevated` (~5 min)
- Three cases in one test: (1) after `StartGeneration` alone, the gauge reads 1 for that
  subsystem; (2) after the matching `EndGeneration`, it reads back to 0; (3) after a second
  `StartGeneration` with `EndGeneration` deliberately never called, the gauge still reads 1 —
  this third case is the literal proof of the Story's safety property, isolated from any real
  goroutine/wedge scenario (Epic 2.2's Story 2.2.2 provides the end-to-end proof against the
  actual wedged-reader test).
- Files: `session/lifecycle/observability_test.go`

### Epic 1.4: `AwaitBounded` — generalized bounded-wait-then-abandon

**Goal**: Fold incident #3's `maxTeardownWait` mitigation into the shared package as a
first-class, independently-tested helper, so it's available to any future subsystem that
adopts `Reason` and also blocks on a goroutine's exit (`research/pitfalls.md`'s explicit ask).

#### Story 1.4.1: `AwaitBounded(done <-chan struct{}, wait time.Duration) bool`
**As a** subsystem author tearing down a generation, **I want** one call that waits for a
done-channel with a bound and tells me whether it timed out, **so that** I don't re-derive
`teardownStandingStream`'s inline `select`/`time.After` race by hand.
**Acceptance Criteria**:
- Returns `true` when `done` closes before `wait` elapses.
  - *Given* a `chan struct{}` closed by a separate goroutine after 10ms, *When*
    `lifecycle.AwaitBounded(done, 200*time.Millisecond)` is called, *Then* it returns `true`
    in well under 200ms.
- Returns `false` when `wait` elapses first, without blocking longer than `wait`.
  - *Given* a `chan struct{}` that is never closed, *When*
    `lifecycle.AwaitBounded(done, 50*time.Millisecond)` is called, *Then* it returns `false`
    after approximately 50ms (test asserts elapsed time is within `[50ms, 500ms)`, matching
    this repo's existing timing-test tolerance convention).

**Files**: `session/lifecycle/await.go`

##### Task 1.4.1a: Implement `AwaitBounded` (~3 min)
- `select { case <-done: return true; case <-time.After(wait): return false }`.
- Files: `session/lifecycle/await.go`

##### Task 1.4.1b: `await_test.go` covering both branches, run under `-race` (~4 min)
- `TestAwaitBounded_ClosesInTime_ReturnsTrue`, `TestAwaitBounded_NeverCloses_ReturnsFalseAfterBound`.
- Files: `session/lifecycle/await_test.go`

### Epic 1.5: `IsBenignTimeout` — canonical typed timeout classifier

**Goal**: One typed, non-string-matching "is this a benign poll timeout" classifier, ready for
`session/external_streamer.go`'s Phase 4 migration (`research/features.md`'s explicit
recommendation).

#### Story 1.5.1: `IsBenignTimeout(err error) bool`
**As a** reader-loop author, **I want** one function classifying `net.Error.Timeout()`,
`os.ErrDeadlineExceeded`, and `io.ErrUnexpectedEOF` as benign, **so that** I never need
`strings.Contains(err.Error(), "i/o timeout")` again.
**Acceptance Criteria**:
- Each of the three known-benign error shapes returns `true`.
  - *Given* `err := fmt.Errorf("read: %w", os.ErrDeadlineExceeded)`, *When*
    `lifecycle.IsBenignTimeout(err)` is called, *Then* it returns `true`.
- A genuine non-timeout error returns `false`.
  - *Given* `err := errors.New("connection reset by peer")`, *When*
    `lifecycle.IsBenignTimeout(err)` is called, *Then* it returns `false`.

**Files**: `session/lifecycle/timeout.go`

##### Task 1.5.1a: Implement `IsBenignTimeout` (~4 min)
- Port exactly the three typed checks from `external_streamer.go:413-426`
  (`errors.As(err, &netErr) && netErr.Timeout()`, `errors.Is(err, os.ErrDeadlineExceeded)`,
  `errors.Is(err, io.ErrUnexpectedEOF)`) into one function; deliberately omit the
  `strings.Contains` fallback (Pattern Decisions).
- Files: `session/lifecycle/timeout.go`

##### Task 1.5.1b: `timeout_test.go` covering all 3 positive cases + 1 negative case (~4 min)
- `TestIsBenignTimeout_NetErrorTimeout_ReturnsTrue`,
  `TestIsBenignTimeout_DeadlineExceeded_ReturnsTrue`,
  `TestIsBenignTimeout_UnexpectedEOF_ReturnsTrue`,
  `TestIsBenignTimeout_GenuineDisconnectError_ReturnsFalse`.
- Files: `session/lifecycle/timeout_test.go`

### Epic 1.6: Phase gate

##### Task 1.6a: Full-package verification before any consumer migrates (~3 min)
- `go test ./session/lifecycle/... -race -v`; `golangci-lint run ./session/lifecycle/...`.
  Both must be green before Phase 2 starts (Risk Control: the shared package stabilizes
  before any consumer depends on it).
- Files: none (verification only)

---

## Phase 2: Migrate `session/tymux/stream.go` (proven 3-incident case, opt-in backend)

### Epic 2.1: Replace the 3-clause `||` with `classifyStreamEnd`

**Goal**: `readAttachLoop`'s `if s.closing.Load() || exited || ctx.Err() != nil` becomes one
classified `lifecycle.Reason`, preserving all three distinguishable origins
(`research/architecture.md` §2's explicit non-redundancy finding).

#### Story 2.1.1: `classifyStreamEnd` preserves origin precedence
**As a** maintainer reading `readAttachLoop`, **I want** one method that returns *which*
origin ended the stream, **so that** the 24-line comment explaining three provenances by hand
(`research/pitfalls.md` §1) is replaced by a name, not more prose.
**Acceptance Criteria**:
- `closing` takes precedence over `exited` and `ctx.Err()` when more than one is true
  simultaneously (the common case: `Close()` sets `closing` before canceling `ctx`).
  - *Given* a `*tymuxGRPCSession` with `s.closing.Load() == true` and a `ctx` already
    canceled (both true, as `Close()` produces), *When* `s.classifyStreamEnd(ctx)` is called,
    *Then* it returns `lifecycle.ReasonDeliberateClose`, not `lifecycle.ReasonDeliberateSupersede`.
- `exited` (incident #1's signal) is preserved as distinct from `ctx.Err()`.
  - *Given* `s.exited == true` (set by `deliverExit`) and `s.closing.Load() == false` and
    `ctx.Err() == nil` (the pane sent `Exited` and closed the stream on its own, no
    cancellation involved yet), *When* `s.classifyStreamEnd(ctx)` is called, *Then* it
    returns `lifecycle.ReasonCleanExit`.
- `ctx.Err() != nil` alone (incident #2's signal — `openStandingStream`'s own
  tear-down-before-reopen, which does *not* set `closing`) is preserved as distinct.
  - *Given* `s.closing.Load() == false`, `s.exited == false`, and `ctx.Err() != nil` (a
    `teardownStandingStream` call from `openStandingStream`'s reopen path, not from
    `Close()`), *When* `s.classifyStreamEnd(ctx)` is called, *Then* it returns
    `lifecycle.ReasonDeliberateSupersede`.
- None of the three signals fired: a genuine transport drop.
  - *Given* `s.closing.Load() == false`, `s.exited == false`, `ctx.Err() == nil`, *When*
    `s.classifyStreamEnd(ctx)` is called, *Then* it returns `lifecycle.ReasonTransportDrop`.

**Files**: `session/tymux/stream.go`

##### Task 2.1.1a: Add `classifyStreamEnd` method and `lifecycle` import (~5 min)
- Add `"github.com/tstapler/stapler-squad/session/lifecycle"` import; add method
  `func (s *tymuxGRPCSession) classifyStreamEnd(ctx context.Context) lifecycle.Reason`
  implementing the precedence in Story 2.1.1 (read `s.exited` under `s.mu.RLock()`, matching
  the existing snapshot pattern at `stream.go:195-197`).
- Files: `session/tymux/stream.go`

##### Task 2.1.1b: Update `readAttachLoop` to call `classifyStreamEnd` (~5 min)
- Replace `if s.closing.Load() || exited || ctx.Err() != nil { close(done); return }` with
  `if reason := s.classifyStreamEnd(ctx); !reason.ShouldContinue() { close(done); return
  reason }`. Change `readAttachLoop`'s signature from `func(...)` to
  `func(...) lifecycle.Reason`. Both of the function's existing exit points (this one and
  Task 2.1.1c's `ReconnectLoop`-exhausted branch) already return explicitly under the new
  signature — the `for` loop itself has no implicit fallthrough return to add.
- Files: `session/tymux/stream.go`

##### Task 2.1.1c: Classify `ReconnectLoop`'s `ok=false` outcome (~4 min)
- Where `readAttachLoop` currently does `newStream, first, ok := s.ReconnectLoop(paneID,
  "error"); if !ok { close(done); return }`, change the return to
  `return reasonForReconnectFailure(s)` where a small local helper (or inline
  `if s.closing.Load() { return lifecycle.ReasonDeliberateClose }; return
  lifecycle.ReasonReconnectExhausted`) distinguishes "interrupted by a deliberate close" from
  "genuinely exhausted," per `research/architecture.md` §2's identified gap ("the *caller's*
  only signal is a bare `bool`").
- Files: `session/tymux/stream.go`

### Epic 2.2: Observability wiring

**Goal**: One span per stream generation, tagged with its terminal `Reason`, plus counter
increments for `ReconnectLoop`'s own give-up paths.

#### Story 2.2.1: Span per generation, counter per terminal reason
**As an** operator, **I want** `session_lifecycle_ends_total{subsystem="tymux_stream",...}` and
a per-generation span, **so that** a future tymux incident is diagnosable from a trace, not a
live `pprof` dump (the exact gap incident #2 exposed).
**Acceptance Criteria**:
- Every `openStandingStream` call starts exactly one generation span, ended exactly once with
  the classified reason.
  - *Given* `openStandingStream` is called and the resulting `readAttachLoop` goroutine later
    returns `lifecycle.ReasonCleanExit`, *When* the goroutine exits, *Then*
    `session_lifecycle_ends_total{subsystem="tymux_stream",reason="clean_exit"}` has
    incremented by exactly 1 for that generation.
- `ReconnectLoop`'s two give-up branches are separately counted.
  - *Given* `ReconnectLoop` exhausts all attempts without `s.closing.Load()` ever being true,
    *When* it returns `false`, *Then*
    `session_lifecycle_ends_total{subsystem="tymux_reconnect",reason="reconnect_exhausted"}`
    increments; *given* it instead observes `s.closing.Load() == true` mid-loop, *Then*
    `...{subsystem="tymux_reconnect",reason="deliberate_close"}` increments instead.

**Files**: `session/tymux/stream.go`, `session/tymux/session.go`

##### Task 2.2.1a: Wrap the `readAttachLoop` goroutine launch with `StartGeneration`/`EndGeneration` (~5 min)
- In `openStandingStream` (`session/tymux/stream.go:112`), replace
  `go s.readAttachLoop(ctx, paneID, stream, done)` with a wrapping closure:
  ```go
  go func() {
      genCtx, span := lifecycle.StartGeneration(ctx, "tymux_stream")
      reason := s.readAttachLoop(genCtx, paneID, stream, done)
      lifecycle.EndGeneration(span, "tymux_stream", reason)
  }()
  ```
  (`genCtx` is passed to `readAttachLoop` in place of `ctx` so `classifyStreamEnd`'s
  `ctx.Err()` check still observes the same cancellation — `StartLinkedBackgroundSpan`
  derives its returned context from the same cancellation tree via `trace.WithNewRoot`, which
  is assumed, not yet proven, not to detach `ctx.Done()`. **This task is not done until Task
  2.2.1a-verify's direct propagation test exists and passes** — the `-race` run in Task
  2.5.1a proves absence of data races, not correct cancellation propagation, and is not a
  substitute (pre-mortem P1 #1).
- Files: `session/tymux/stream.go`

##### Task 2.2.1a-verify: Prove `StartGeneration`'s derived context propagates parent cancellation — merge gate on Task 2.2.1a (~4 min)
- Direct unit test (not inference from `-race`): call
  `genCtx, _ := lifecycle.StartGeneration(parentCtx, "test")` where `parentCtx` is a
  cancelable `context.Context`; cancel the *parent* (`cancel()`); assert `<-genCtx.Done()`
  fires and `genCtx.Err() != nil` within a bounded wait (e.g. `AwaitBounded`-style, or a plain
  `select`/`time.After` in the test itself). This is the literal proof that
  `telemetry.StartLinkedBackgroundSpan`'s derived context still observes the parent's
  cancellation tree — pre-mortem P1 #1's exact finding: if this assumption is wrong,
  `classifyStreamEnd`'s `ctx.Err()` check (Task 2.2.1a) permanently reads `nil` under the new
  mechanism, silently reintroducing incident #2's deadlock behind the abstraction built to
  prevent it.
- **Acceptance criterion, stated as a merge gate**: Task 2.2.1a's wrapping closure must not be
  merged until this test exists in the codebase and passes. A green `-race` run alone (Task
  2.5.1a) does not satisfy this gate.
- Files: `session/lifecycle/observability_test.go` (test `StartGeneration` directly, isolated
  from any `session/tymux` fixture)

##### Task 2.2.1b: Add `lifecycle.RecordEnd` calls to `ReconnectLoop`'s two give-up branches (~4 min)
- At `stream.go:599-606` (the `s.closing.Load()` branch and the exhaustion branch), add
  `lifecycle.RecordEnd(context.Background(), "tymux_reconnect", lifecycle.ReasonDeliberateClose)`
  and `lifecycle.RecordEnd(context.Background(), "tymux_reconnect",
  lifecycle.ReasonReconnectExhausted)` respectively, right before each existing log line.
  (`context.Background()` because `ReconnectLoop` has no `ctx` parameter of its own — a
  deliberate, documented scope cut: the span-event half of `RecordEnd` is a no-op here, only
  the counter fires. Threading a real `ctx` through `ReconnectLoop`'s signature would touch 3+
  call sites for a span-only benefit not required by Success Metrics.)
- Files: `session/tymux/stream.go`

#### Story 2.2.2: Prove the active-generations gauge actually catches incident #3's exact shape
**As an** operator, **I want** the wedged-reader scenario this project already reproduced live
to leave a visible, non-decrementing gauge, **so that** Story 1.3.3's isolated unit proof is
also verified against the real abandoned-goroutine mechanism this project was built to make
diagnosable (Adversarial Review Blocker: the plan's original `ends_total`-only design was
structurally blind to a non-returning generation — this is the end-to-end regression test
proving the gauge fix actually closes that gap for `session/tymux`, not just in isolation).
**Acceptance Criteria**:
- After `TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged`'s
  existing wedge setup runs (the old reader goroutine is abandoned, per that test's own
  design), the old generation's slot in `session_lifecycle_active_generations{subsystem="tymux_stream"}`
  has not decremented.
  - *Given* the exact wedged-reader setup already exercised by
    `TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged`
    (`session/tymux/stream_test.go`) — a `Receive()` that never returns, so `readAttachLoop`'s
    goroutine is abandoned once `teardownStandingStream`'s bounded wait (`AwaitBounded`) times
    out — *When* the test's assertions run (it already asserts `RestoreWithWorkDir` proceeds
    within 2s despite the old reader being stuck), *Then* an additional assertion via a
    test-local `metric.ManualReader` confirms
    `session_lifecycle_active_generations{subsystem="tymux_stream"}` still counts that old
    generation as active (has not been decremented, since its goroutine's `EndGeneration` call
    — Task 2.2.1a — is never reached).

**Files**: `session/tymux/stream_test.go`

##### Task 2.2.2a: Extend `TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged` with a gauge assertion (~5 min)
- Add a test-local `metric.ManualReader` (matching Task 1.3.2d's convention) wired into the
  test's `telemetry` setup before the wedge is created; after the existing assertions
  (`RestoreWithWorkDir` returns within 2s), read the collected metrics and assert
  `session_lifecycle_active_generations{subsystem="tymux_stream"}` is still `>= 1` for the
  abandoned generation — proving the gauge, not just the span/counter, reflects the leak this
  test already reproduces.
- Files: `session/tymux/stream_test.go`

### Epic 2.3: Fold incident #3's bounded-wait into `lifecycle.AwaitBounded`

**Goal**: Supersede, not duplicate, `teardownStandingStream`'s inline `select`/`time.After`.

#### Story 2.3.1: `teardownStandingStream` uses the shared helper
**As a** maintainer, **I want** the bounded-wait idiom to live in one place, **so that** a
future subsystem needing the same "wait for a done-channel, abandon after N" contract doesn't
re-derive it.
**Acceptance Criteria**:
- Behavior is unchanged: still logs the same warning on timeout, still returns promptly on a
  timely close.
  - *Given* `s.teardownWait == 50*time.Millisecond` and a `done` channel that never closes
    (the wedged-reader scenario `TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged`
    exercises), *When* `teardownStandingStream` runs, *Then* it logs
    `"tymux: teardownStandingStream: old reader goroutine did not exit in time, abandoning
    it"` and returns within ~50ms, exactly as before this refactor.

**Files**: `session/tymux/stream.go`

##### Task 2.3.1a: Replace the inline `select`/`time.After` with `lifecycle.AwaitBounded` (~4 min)
- At `stream.go:157-163`, replace
  ```go
  select {
  case <-done:
  case <-time.After(s.teardownWait):
      log.Warn(...)
  }
  ```
  with
  ```go
  if !lifecycle.AwaitBounded(done, s.teardownWait) {
      log.Warn("tymux: teardownStandingStream: old reader goroutine did not exit in time, abandoning it",
          "timeout", s.teardownWait)
  }
  ```
- Files: `session/tymux/stream.go`

##### Task 2.3.1b: Trim `maxTeardownWait`'s doc comment to point at `lifecycle.AwaitBounded` (~3 min)
- Keep the incident narrative (why 5s, what's abandoned) but remove the mechanism-duplicating
  prose that now belongs on `lifecycle.AwaitBounded`'s own doc comment (Task 1.4.1a) — one
  cross-reference sentence, not a re-explanation.
- Files: `session/tymux/stream.go`

### Epic 2.4: Regression tests

**Goal**: All 3 documented incidents stay covered against the refactored mechanism, per
Success Metrics.

#### Story 2.4.1: Re-verify the 2 existing fix #1/#2 regression tests, unmodified
**As a** reviewer, **I want** confirmation that the two existing deadlock regression tests
still pass against the new `classifyStreamEnd`-based code, **so that** the refactor is proven
non-regressive on the exact incidents it was built to fix.
**Acceptance Criteria**:
- Both tests pass unmodified (they assert black-box behavior — `RestoreWithWorkDir` returning
  within 2s — not any internal call shape this refactor changes).
  - *Given* the refactored `session/tymux/stream.go`, *When*
    `go test ./session/tymux/... -run
    'TestOpenStandingStream_TearDownForReopen_(DoesNotDeadlock_WhenOldStreamWouldOtherwiseReconnect|ProceedsAnyway_WhenOldReaderIsWedged)'
    -race -v` is run, *Then* both tests pass with no source changes required to the test file
    itself.

**Files**: `session/tymux/stream_test.go` (read/run only, no edits expected)

##### Task 2.4.1a: Run and confirm both existing deadlock regression tests pass unmodified (~3 min)
- `go test ./session/tymux/... -run TestOpenStandingStream_TearDownForReopen -race -v`;
  attach the output to the PR. If either fails, that is new information requiring a design
  fix, not a test edit (Engineering Discipline: root-cause before symptom fix).
- Files: none (verification only)

##### Task 2.4.1b: Confirm the existing clean-exit test already covers incident #1; add one focused unit assertion on the new classifier (~4 min)
- `TestReadAttachLoop_CleanExitThenStreamEnd_DoesNotReconnectOrRevive`
  (`session/tymux/stream_test.go:707`) already reproduces commit `477eacd6a`'s incident #1
  end-to-end (asserts `attachCalls == 1`, `revived == 0`, `BackendRestarted() == false` after
  a clean `Exited` event followed by stream close) — no duplicate test is needed. Add one new,
  narrower unit test tying the existing behavioral proof to the new mechanism: assert
  `classifyStreamEnd` itself returns `ReasonCleanExit` in the exact `exited`-before-`Receive()`-
  error sequence, not just that the end-to-end behavior happens to be correct.
- Files: `session/tymux/stream_test.go`

#### Story 2.4.2: New table test for `classifyStreamEnd` itself
**As a** reviewer, **I want** a direct unit test of the classification function in isolation,
**so that** the four Given/When/Then cases in Story 2.1.1 are each independently verifiable
without needing a full `readAttachLoop`/RPC-mock setup.
**Acceptance Criteria**:
- All 4 cases from Story 2.1.1 (closing-wins, clean-exit, supersede, transport-drop) pass as a
  single table-driven test.
  - *Given* the table `{closing, exited, ctxCanceled bool; want lifecycle.Reason}` with the 4
    rows from Story 2.1.1, *When* `classifyStreamEnd` is called for each row's constructed
    `*tymuxGRPCSession`/`ctx`, *Then* each row's `want` matches.

**Files**: `session/tymux/stream_test.go`

##### Task 2.4.2a: `TestClassifyStreamEnd_AllFourOrigins` (~5 min)
- Table-driven test constructing a minimal `*tymuxGRPCSession` (via `NewTymuxGRPCSession` +
  direct field pokes under a helper, matching `setReconnectBackoff`/`setTeardownWait`'s
  existing test-helper convention) and a `context.Context` canceled or not per row.
- Files: `session/tymux/stream_test.go`

### Epic 2.5: Full-suite verification

##### Task 2.5.1a: `go test ./session/tymux/... -race -v` (~2 min)
- Full existing 46+2(new-from-this-session)+3(new-from-this-plan) test suite green under
  `-race`. Attach output to the PR (Engineering Discipline: green first, then "done").
- Files: none (verification only)

---

## Phase 3: Migrate `session/tmux/control_mode.go` (default backend — separate, cautiously reviewed)

*Ships as its own commit/PR, independent of Phase 2, per Risk Control's default-backend
scrutiny requirement and `research/pitfalls.md` §5's confirmed blast-radius asymmetry. Lands
behind `STAPLER_SQUAD_TMUX_LIFECYCLE_V2`, default `false` (Epic 3.1, Story 3.1.2) — the
default-backend-specific risk-control mechanism requirements.md's Risk Control section
requires beyond "separate PR + own tests," since every currently-running session runs this
backend and would otherwise flip onto the new classification path simultaneously on the next
restart with no canary and no rollback lever short of a full binary revert (Adversarial Review
Blocker).*

### Epic 3.1: Consolidate the 3 independently-repeated `intentionalStop` checks

**Goal**: One classification call site instead of three copies of
`if !intentionalStop.Load() { onExitOnce.Do(...) }` (confirmed real, not hypothetical, by
`research/pitfalls.md` §5's direct reading) — landed as an opt-in path behind
`STAPLER_SQUAD_TMUX_LIFECYCLE_V2` (Story 3.1.2), per Risk Control's requirement for a
default-backend-specific mitigation distinct from `session/tymux`'s inherent opt-in safety
net (Adversarial Review Blocker).

#### Story 3.1.1: `classifyControlModeExit` replaces all 3 call sites (the `STAPLER_SQUAD_TMUX_LIFECYCLE_V2=true` path)
**As a** maintainer adding a 4th notification type later, **I want** one function deciding
"was this deliberate," **so that** I can't add a new unilateral-exit path that forgets to
check it (the exact structural gap that makes this the same *class* of bug as tymux's, per
`research/pitfalls.md` §5).
**Acceptance Criteria**:
- A deliberate `StopControlMode()` call suppresses the callback at all 3 sites identically.
  - *Given* `t.intentionalStop.Load() == true` (set by `StopControlMode`), *When*
    `t.classifyControlModeExit()` is called from any of the scanner-EOF fallback, `%exit`, or
    `%session-closed` handlers, *Then* it returns `lifecycle.ReasonDeliberateClose` at every
    site, and `reason.ShouldFireExitCallback() == false`.
- A unilateral exit (process killed/crashed, no `StopControlMode()`) fires the callback.
  - *Given* `t.intentionalStop.Load() == false` and the control-mode pipe closes without a
    preceding `%exit` line (scanner EOF), *When* `t.classifyControlModeExit()` is called,
    *Then* it returns `lifecycle.ReasonTransportDrop`, and
    `reason.ShouldFireExitCallback() == true`.

**Files**: `session/tmux/control_mode.go`

##### Task 3.1.1a: Add `classifyControlModeExit` method and `lifecycle` import (~3 min)
- Add `"github.com/tstapler/stapler-squad/session/lifecycle"` import; add
  `func (t *TmuxSession) classifyControlModeExit() lifecycle.Reason { if
  t.intentionalStop.Load() { return lifecycle.ReasonDeliberateClose }; return
  lifecycle.ReasonTransportDrop }` near the existing `intentionalStop` field declaration.
- Files: `session/tmux/control_mode.go`

##### Task 3.1.1b: Replace the scanner-EOF fallback check at `control_mode.go:491` (~3 min)
- **Only reached when `config.TmuxLifecycleV2Enabled()` is true** — see Story 3.1.2's Task
  3.1.2b for the flag branch this and Tasks 3.1.1c/d live inside; the `false` branch keeps the
  literal pre-migration `if !t.intentionalStop.Load() { t.onExitOnce.Do(...) }` unchanged.
- Replace `if !t.intentionalStop.Load() { t.onExitOnce.Do(...) }` with
  `t.onExitOnce.Do(func() { reason := t.classifyControlModeExit();
  lifecycle.EndGeneration(span, "tmux_control_mode", reason); if reason.ShouldFireExitCallback()
  && t.onExit != nil { t.onExit("control-mode-pipe-closed") } })`. Classification, the metric/
  gauge update, and the conditional callback all move **inside** the closure passed to
  `t.onExitOnce.Do` — not gated separately before it — so `sync.Once`'s single-execution
  guarantee covers all three, guaranteeing exactly one `session_lifecycle_ends_total`
  increment per generation regardless of how many of the 3 call sites are reached
  (Architecture Review Blocker: the prior draft called `lifecycle.RecordEnd` *before*
  `onExitOnce.Do`, once per call site that observed `ShouldFireExitCallback()==true`, which
  could double-count). `EndGeneration` (not the lighter `RecordEnd`) is used here specifically
  because it also decrements `session_lifecycle_active_generations` (Story 1.3.3) — this
  generation's `span` was opened by `StartGeneration` in Task 3.2.1a, so its slot in the gauge
  must be released by the same function that ends its span; `RecordEnd` deliberately never
  touches the gauge (Task 1.3.3b), so using it here would leave every completed
  `tmux_control_mode` generation looking permanently "active." `span` is the value Task
  3.2.1a's `StartGeneration` call assigned at the top of `readControlModeOutput` — same
  function scope, captured by this closure.
- Files: `session/tmux/control_mode.go`

##### Task 3.1.1c: Replace the `%exit` handler check at `control_mode.go:656` (~3 min)
- **Only reached when `config.TmuxLifecycleV2Enabled()` is true** (see Task 3.1.1b).
- Same substitution as Task 3.1.1b, with `t.onExit("control-mode-%exit")`. Because
  classification, `EndGeneration`, and the callback all now live inside the one
  `t.onExitOnce.Do` closure, whichever of this site and Task 3.1.1b/d's sites reaches
  `t.onExitOnce.Do` first is the one whose closure actually runs — the other two calls to
  `t.onExitOnce.Do` block-then-return without re-executing anything, so
  `session_lifecycle_ends_total{subsystem="tmux_control_mode",...}` increments **exactly
  once** per generation no matter which site gets there first (supersedes the prior draft's
  "accepted, documented" double-count approximation — that approximation is no longer needed
  or produced).
- Files: `session/tmux/control_mode.go`

##### Task 3.1.1d: Replace the `%session-closed` handler check at `control_mode.go:668` (~3 min)
- **Only reached when `config.TmuxLifecycleV2Enabled()` is true** (see Task 3.1.1b).
- Same substitution as Task 3.1.1b/c, with `t.onExit("session-closed")`.
- Files: `session/tmux/control_mode.go`

#### Story 3.1.2: Gate the consolidated path behind `STAPLER_SQUAD_TMUX_LIFECYCLE_V2`
**As an** operator of the default, systemd-deployed backend, **I want** the new consolidated
classification (and the observability that ships with it) to be opt-in for one release before
it becomes every session's only code path, **so that** a bad migration doesn't flip on the
next restart with zero canary and zero rollback lever short of a full binary revert
(Adversarial Review Blocker: requirements.md's Risk Control explicitly requires "a build tag,
gradual rollout, or extra bake time in review" for this specific, default-backend migration,
distinct from `session/tymux`'s inherent opt-in-via-`config.TymuxSessionOverrides` safety net
— "own PR, own regression tests" alone does not satisfy that requirement).
**Acceptance Criteria**:
- Default (`STAPLER_SQUAD_TMUX_LIFECYCLE_V2` unset or `false`): `control_mode.go`'s 3 call
  sites run byte-for-byte the same code shipped today — independent
  `if !t.intentionalStop.Load() { t.onExitOnce.Do(func() { if t.onExit != nil { t.onExit(...) } }) }`
  checks, no `lifecycle` package call of any kind (no span, no counter, no gauge).
  - *Given* `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` is unset, *When* any of the 3 exit paths is
    exercised, *Then* `onExit` fires (or doesn't) with exactly the call-count/argument/timing
    behavior of the pre-migration code — verified by running Epic 3.3's 3 tests with the flag
    unset and confirming they pass against the *unmigrated* code path (Task 3.1.2c).
- `STAPLER_SQUAD_TMUX_LIFECYCLE_V2=true`: the 3 call sites run the consolidated
  `classifyControlModeExit()` + `onExitOnce`-gated path from Tasks 3.1.1b/c/d, with full
  `session/lifecycle` observability (span, counter, gauge).
  - *Given* `STAPLER_SQUAD_TMUX_LIFECYCLE_V2=true`, *When* the same 3 exit paths are
    exercised, *Then* `onExit`'s externally-observed call-count/argument/timing behavior is
    identical to the `false` case (Task 3.1.2c proves this parity), and
    `session_lifecycle_ends_total{subsystem="tmux_control_mode",...}` /
    `session_lifecycle_active_generations{subsystem="tmux_control_mode"}` are populated
    exactly as Epic 3.2/Story 1.3.3 describe.

**Files**: `config/config.go` (or wherever `STAPLER_SQUAD_USE_CONTROL_MODE` is currently read —
mirror that exact file/pattern), `session/tmux/control_mode.go`

##### Task 3.1.2a: Add `config.TmuxLifecycleV2Enabled() bool` (~4 min)
- Read `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` via `os.Getenv`, default `false` on empty/unset —
  mirror `STAPLER_SQUAD_USE_CONTROL_MODE`'s existing read-and-parse shape exactly (same
  package, same truthy-string parsing convention) so this isn't a second, subtly-different
  boolean-env-var idiom in the codebase.
- Files: wherever `STAPLER_SQUAD_USE_CONTROL_MODE` is read today

##### Task 3.1.2b: Branch each of the 3 call sites on the flag (~5 min)
- At each of `control_mode.go:491/656/668`, wrap Task 3.1.1b/c/d's new code in
  `if config.TmuxLifecycleV2Enabled() { <new consolidated code> } else { <literal
  pre-migration code, unchanged> }`. The `else` branch is a verbatim copy of what ships today
  — no `lifecycle` import, no `classifyControlModeExit` call — specifically so the default
  path for this release is provably identical to production, not merely "should behave the
  same after refactor."
- Files: `session/tmux/control_mode.go`

##### Task 3.1.2c: Run Epic 3.3's 3 tests under both flag values (~5 min)
- Parametrize (or duplicate via `t.Setenv("STAPLER_SQUAD_TMUX_LIFECYCLE_V2", ...)` per the
  repo's existing env-var test convention) each of Tasks 3.3.1a/b/c across `""` (default) and
  `"true"`, asserting identical `onExit` call-count/argument outcomes in both — this is the
  concrete proof backing Story 3.1.2's parity acceptance criteria, not an assumption.
- Files: `session/tmux/control_mode_test.go`

### Epic 3.2: Observability wiring

**Goal (revised)**: One generation span, opened only when `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` is
`true` (Story 3.1.2) — matching the "new mechanism, including its observability, is what's
opt-in" framing of the Risk Control fix — ended exactly once by the same `onExitOnce`-gated
closure that Tasks 3.1.1b/c/d already own, not by a second, independent defer. `control_mode.go`
has no observability of any kind today, so having it appear together with the new path (rather
than partially, under the old path too) is a deliberate, low-risk scoping choice, not an
oversight.

#### Story 3.2.1: One generation span per `readControlModeOutput` invocation, ended by Epic 3.1's single call site
**As an** operator, **I want** `session_lifecycle_ends_total{subsystem="tmux_control_mode",...}`
for the default backend too, **so that** the same trace-based diagnosis capability tymux gets
also covers the higher-traffic backend.
**Acceptance Criteria**:
- Exactly one span per `readControlModeOutput` goroutine invocation (when
  `STAPLER_SQUAD_TMUX_LIFECYCLE_V2=true`), ended exactly once by whichever of Epic 3.1's 3
  `onExitOnce`-guarded call sites reaches it first — not by an independent second
  end-of-function defer, which would double the counter increment `EndGeneration` performs
  (Architecture Review Blocker's exact failure mode, reintroduced at the epic boundary instead
  of within Epic 3.1 alone, if a defer here also called `EndGeneration`).
  - *Given* `readControlModeOutput` is spawned by `StartControlMode` with the flag enabled,
    *When* its scanner loop ends (for any reason — EOF, `%exit`, or a subsequent unrelated
    shutdown), *Then* `session_lifecycle_ends_total{subsystem="tmux_control_mode",...}`
    increments exactly once for that generation, labeled with whatever `classifyControlModeExit()`
    returned at whichever of Tasks 3.1.1b/c/d's call sites first reached `t.onExitOnce.Do`.
- A generation whose goroutine never reaches any of the 3 call sites at all (wedged/abandoned,
  the incident #3 shape) leaves its span un-ended and its slot in
  `session_lifecycle_active_generations{subsystem="tmux_control_mode"}` still counted as
  active — this is Story 1.3.3's intended signal, not a gap to patch here.

**Files**: `session/tmux/control_mode.go`

##### Task 3.2.1a: Start (but do not independently end) the generation span (~4 min)
- At the top of `readControlModeOutput` (`control_mode.go:386`) — a separate flag check from
  Task 3.1.2b's 3 per-call-site branches, since this is the function's single entry point, not
  one of the 3 exit call sites — add:
  `var span trace.Span; if config.TmuxLifecycleV2Enabled() { _, span =
  lifecycle.StartGeneration(context.Background(), "tmux_control_mode") }`
  (`context.Background()` since this goroutine is spawned without a caller-supplied `ctx`
  today, matching `ReconnectLoop`'s same documented scope cut in Phase 2). When the flag is
  `false`, `span` stays the zero value and is never read — Task 3.1.2b's `false` branch at each
  of the 3 call sites is the verbatim pre-migration code, which doesn't reference `span` at
  all. Deliberately **no**
  paired `defer ... EndGeneration(...)` here — `span` is captured by the closures Tasks
  3.1.1b/c/d pass to `t.onExitOnce.Do`, and one of those (guaranteed by `sync.Once` to run at
  most once) is the sole place `EndGeneration` is called for this generation. A second,
  independent `EndGeneration` call here would double-count the very metric Blocker A's fix
  exists to make single-count.
- Files: `session/tmux/control_mode.go`

### Epic 3.3: From-scratch regression tests

**Goal**: This decision point has no incident-driven test coverage today
(`research/pitfalls.md` §5's confirmed gap) — write tests proving the new mechanism correctly
classifies deliberate-stop vs. unilateral-exit for `control_mode.go` specifically, not by
assuming the tymux tests generalize.

#### Story 3.3.1: Deliberate-stop vs. unilateral-exit, proven for this file
**As a** reviewer of the default-backend migration, **I want** tests specific to
`control_mode.go`'s own signal set (`intentionalStop`, not `closing`/`exited`/`ctx.Err()`),
**so that** this migration's correctness doesn't ride on tymux's unrelated test suite.
**Acceptance Criteria**:
- `StopControlMode()` followed by pipe closure never fires `onExit`.
  - *Given* a `TmuxSession` with control mode started, *When* `StopControlMode()` is called
    (setting `intentionalStop`) and the underlying process/pipe subsequently closes, *Then*
    the registered `onExit` callback is never invoked.
- A pipe closing without `StopControlMode()` (scanner EOF, no `%exit` seen) fires `onExit`
  exactly once with reason `"control-mode-pipe-closed"`.
  - *Given* a `TmuxSession` with control mode started and `intentionalStop` never set, *When*
    the control-mode process is killed out-of-band (stdout pipe closes, scanner hits EOF
    without a prior `%exit` line), *Then* `onExit("control-mode-pipe-closed")` is invoked
    exactly once.
- A `%exit` notification without a prior `StopControlMode()` fires `onExit` exactly once, even
  though both the `%exit` handler and the subsequent scanner-EOF fallback independently check
  `classifyControlModeExit()` (guards against the 3-call-site drift bug this migration fixes).
  - *Given* the same unstopped `TmuxSession`, *When* a `%exit` line arrives followed
    immediately by the pipe closing (scanner EOF), *Then* `onExit` is invoked exactly once
    (verified via `sync.Once`'s existing guarantee plus this test asserting a call count of 1,
    not assuming it).

**Files**: `session/tmux/control_mode_test.go`

##### Task 3.3.1a: `TestControlMode_StopControlMode_IntentionalStop_DoesNotFireOnExit` (~5 min)
- Construct a `TmuxSession` with a fake/pipe-backed control-mode process (matching this test
  file's existing process-mocking convention), call `StopControlMode()`, close the pipe,
  assert `onExit` call count is 0.
- Files: `session/tmux/control_mode_test.go`

##### Task 3.3.1b: `TestControlMode_ScannerEOF_UnilateralExit_FiresOnExit` (~5 min)
- Close the pipe without calling `StopControlMode()` or sending `%exit`; assert `onExit` is
  called exactly once with `"control-mode-pipe-closed"`.
- Files: `session/tmux/control_mode_test.go`

##### Task 3.3.1c: `TestControlMode_PercentExit_UnilateralExit_FiresOnExitExactlyOnce` (~5 min)
- Send a `%exit` line, then close the pipe (scanner EOF); assert `onExit` call count is
  exactly 1 (not 2), with `"control-mode-%exit"` (the first classifier to run wins, matching
  `onExitOnce`'s existing fire-once contract — this test protects that contract against a
  future edit that duplicates classification logic incorrectly).
- Files: `session/tmux/control_mode_test.go`

### Epic 3.4: Full-suite verification, own PR

##### Task 3.4.1a: `go test ./session/tmux/... -race -v` (~2 min)
- Full existing `session/tmux` suite plus the 3 new tests above, green under `-race`. Structure
  this phase's commits as a separate PR from Phase 2's, per Risk Control (a bad migration in
  the default backend must be revertable without reverting tymux's).
- Files: none (verification only)

##### Task 3.4.1b: Re-run the full suite with `STAPLER_SQUAD_TMUX_LIFECYCLE_V2=true` (~2 min)
- `STAPLER_SQUAD_TMUX_LIFECYCLE_V2=true go test ./session/tmux/... -race -v` — confirms the
  opt-in path is fully green too, not just the default-`false` path Task 3.4.1a exercises.
  Both runs' output attached to the PR (Risk Control: the flag must be provably wired through
  cleanly on both sides before it ships, not just structurally present).
- Files: none (verification only)

---

## Phase 4: Migrate `session/external_streamer.go`'s `readLoop`

*Scope note (resolves a requirements.md cross-reference otherwise left ambiguous):
`readLoop`'s problem shape is "is this one error a benign poll-timeout," not "classify which
of N mutually exclusive lifecycle-ending causes fired" — it never had a `closing`/`exited`/
`ctx.Err()`-style multi-clause boolean to replace, only a fragile string-matching fallback
inside a single error check. So Phase 4 borrows exactly one helper from the shared package,
`lifecycle.IsBenignTimeout` (Epic 1.5) — it does **not** adopt the `lifecycle.Reason` enum, and
there is no `StartGeneration`/`EndGeneration` span, no `session_lifecycle_ends_total` label,
and no `session_lifecycle_active_generations` label for this subsystem. Requirements.md's
Scope → In Scope item 3 ("each migration ... gets observability (span/metric) via the
established pattern") describes the contract for a `Reason`-based migration; Phase 4 was never
that shape of migration, so it is explicitly exempt from that observability requirement.
Project-wide, this plan migrates **2 subsystems** to `Reason` (`session/tymux`,
`session/tmux`, Phases 2-3) and lands **1 adjacent fix** here in Phase 4 — see the Dependency
Visualization section above.*

### Epic 4.1: Replace the ad hoc/string-matching timeout classification

**Goal**: Remove the fragile `strings.Contains(err.Error(), "i/o timeout")` fallback
(`research/features.md` §4's explicit "worse than a boolean OR" finding) in favor of
`lifecycle.IsBenignTimeout`.

#### Story 4.1.1: `readLoop` uses the canonical classifier
**As a** maintainer, **I want** one typed timeout check instead of 4 typed checks plus a
string-matching fallback, **so that** a future Go stdlib or wrapped-library error-text change
can't silently break "is this benign" classification again.
**Acceptance Criteria**:
- A wrapped `os.ErrDeadlineExceeded` (the normal 1-second poll timeout this loop sets every
  iteration) is classified benign and the loop continues without marking the connection
  disconnected.
  - *Given* `conn.SetReadDeadline` has fired and `mux.DecodeMessage` returns
    `fmt.Errorf("failed to read message header: %w", os.ErrDeadlineExceeded)`, *When* the
    error branch runs, *Then* `lifecycle.IsBenignTimeout(err) == true`, the loop `continue`s,
    and `s.connected`/`s.conn` are left untouched.
- A genuine disconnect (not any of the 3 typed benign shapes) still marks the connection dead
  and triggers reconnect on the next iteration.
  - *Given* `mux.DecodeMessage` returns a plain `errors.New("connection reset by peer")`,
    *When* the error branch runs, *Then* `lifecycle.IsBenignTimeout(err) == false`,
    `s.conn` is closed and set to `nil`, and `s.connected` becomes `false`.

**Files**: `session/external_streamer.go`

##### Task 4.1.1a: Add `lifecycle` import and replace the classification chain (~5 min)
- Replace `external_streamer.go:413-431`'s 4-branch typed-check-plus-string-match chain with:
  `if lifecycle.IsBenignTimeout(err) { continue }` (single check, placed where the first
  `errors.As(err, &netErr)` check currently sits); remove the now-dead `netErr`/`errStr`
  locals and the `strings.Contains` branch entirely (Pattern Decisions: deliberately removed,
  not preserved alongside the new classifier).
- Files: `session/external_streamer.go`

### Epic 4.2: Regression tests for the near-miss

**Goal**: Prove the classifier handles the exact wrapped-error shape the removed
string-matching branch used to exist for, per Success Metrics' "concretely demonstrated
near-miss from the audit" requirement — this file's own comment ("fallback for wrapped
errors") is the near-miss evidence.

#### Story 4.2.1: `IsBenignTimeout` covers what the string-match used to catch, and nothing more
**As a** reviewer, **I want** a test proving the removed string-matching fallback's job is
still done by the typed classifier, **so that** its removal (Epic 4.1) is a like-for-like
replacement, not a silent behavior regression.
**Acceptance Criteria**:
- (Covered by Task 1.5.1b's unit tests on `IsBenignTimeout` itself — Phase 4 adds the
  integration-level proof that `readLoop` actually calls it and behaves correctly end to end.)
  - *Given* a fake `net.Conn` whose `Read` returns an error satisfying `net.Error` with
    `Timeout() == true`, *When* `readLoop` processes one iteration, *Then* it neither closes
    the connection nor logs a disconnect warning, and loops again.
  - *Given* a fake `net.Conn` whose `Read` returns `io.EOF`, *When* `readLoop` processes one
    iteration, *Then* it does mark the connection disconnected and the next iteration calls
    `reconnect()`.

**Files**: `session/external_streamer_test.go`

##### Task 4.2.1a: `TestExternalStreamer_ReadLoop_WrappedTimeoutError_KeepsPollingWithoutReconnect` (~5 min)
- Using this test file's existing fake-connection/mock-socket convention (mirrored from
  `startedSessionWithStream`'s style in `session/tymux`), simulate a wrapped-deadline-exceeded
  read error and assert the connection is not torn down.
- Files: `session/external_streamer_test.go`

##### Task 4.2.1b: `TestExternalStreamer_ReadLoop_RealDisconnectError_TriggersReconnect` (~5 min)
- Negative case: a genuine `io.EOF`/reset error marks `s.connected = false` and the loop's
  next iteration invokes `reconnect()`.
- Files: `session/external_streamer_test.go`

### Epic 4.3: Verification

##### Task 4.3.1a: `go test ./session/... -run ExternalStreamer -race -v` (~2 min)
- New and existing `ExternalStreamer` tests green under `-race`.
- Files: none (verification only)

---

## Phase 5: Scope documentation, ADR, and follow-on tracking

*Closes the Success Metrics requirement that any subsystem found during the audit and not
migrated has "an explicit, reviewed reason" — not silently skipped.*

### Epic 5.1: Document confirmed out-of-scope findings in place

#### Story 5.1.1: `session/actor.go`'s command-execution-SLA gap is named, not silently dropped
**As a** future maintainer reading `runActor` after this project ships, **I want** a doc
comment explaining why `runActor` was *not* migrated to `lifecycle.Reason` despite being named
a starting candidate in `requirements.md`, **so that** I don't waste time re-auditing a
question this project already answered.
**Acceptance Criteria**:
- The doc comment names the actual gap (command-execution SLA/preemption, not classification)
  and why it's a different mechanism, not a missing `Reason` case.
  - *Given* `session/actor.go`'s `runActor` function, *When* a future maintainer reads its doc
    comment, *Then* it states that `runActor`'s own `select` is single-signal and correct, that
    the real risk (a command closure blocking the actor indefinitely, per incident #2/#3) is a
    liveness/preemption gap in the actor's execution contract — not this project's
    classification problem — and links to the follow-on tracking item from Epic 5.3.

**Files**: `session/actor.go`

##### Task 5.1.1a: Add the out-of-scope doc-comment note above `runActor` (~4 min)
- Short addition (3-5 lines, per the repo's comment-proportionality convention) to
  `runActor`'s existing doc comment (`actor.go:103-120`), not a new file.
- Files: `session/actor.go`

#### Story 5.1.2: `session/tmux/server_registry.go`'s `reconnectLoop` is confirmed clean
**As a** future maintainer, **I want** a one-line note confirming this loop was audited and
found not to need migration, **so that** its different-looking-but-correct shape (unconditional
reconnect, no deliberate-vs-transient distinction) doesn't get "fixed" into an unnecessary
`lifecycle.Reason` dependency later.
**Acceptance Criteria**:
- The note is present and accurate.
  - *Given* `TmuxServerRegistry.reconnectLoop` (`server_registry.go:416-509`), *When* a future
    maintainer reads its doc comment, *Then* it states this loop was audited
    (session-lifecycle-state-machine project, 2026-09) and found to need no migration — its
    only stop-vs-continue signal is `ctx.Done()`, a single unambiguous cause with nothing to
    misclassify, matching `server_registry.go` cited as the "correctly not needing the
    pattern" reference example.

**Files**: `session/tmux/server_registry.go`

##### Task 5.1.2a: Add the confirmed-clean doc-comment note above `reconnectLoop` (~3 min)
- One-to-two-line addition, no functional change.
- Files: `session/tmux/server_registry.go`

### Epic 5.2: ADR

##### Task 5.2.1a: Write `ADR-001-bespoke-lifecycle-reason-over-fsm-library.md` (~5 min)
- Per Step 5 of this plan's governing instructions and `requirements.md`'s explicit call for
  one — see the ADR file itself for full content.
- Files: `project_plans/session-lifecycle-state-machine/decisions/ADR-001-bespoke-lifecycle-reason-over-fsm-library.md`

### Epic 5.3: Follow-on tracking (does not block this project's completion)

##### Task 5.3.1a: File a tracked follow-on for `session/actor.go`'s command-execution-SLA gap (~3 min)
- A GitHub issue (or, if issue-filing is unavailable in this environment, a dated entry under
  this project's `decisions/` directory noting the gap and linking back to
  `research/features.md` §3/`research/pitfalls.md` §3) — explicitly named, not left as an
  implicit TODO, per `research/features.md`'s own framing ("worth flagging as an adjacent
  finding for Phase 3, even though it isn't the same... shape").
- Files: none in this repo's tracked plan artifacts beyond the issue reference recorded in
  `ADR-001`'s "Consequences" section (Epic 5.2).

##### Task 5.3.2a: File a tracked follow-on to remove `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` after a dashboarded, staged bake period (~3 min)
- A GitHub issue (or dated `decisions/` entry, same fallback as Task 5.3.1a) recording the
  flag's purpose (Epic 3.1, Story 3.1.2 — default-backend risk control for this migration) and
  a concrete, monitored removal gate — not the vague "N weeks, no incident report" criterion
  this task originally specified (pre-mortem P1 #2: undefined, unowned, untied to any
  dashboard, so the flip would happen on schedule-optimism rather than evidence). The gate has
  three parts, all required before the `false` branch is deleted:
  - (a) **Dashboarded**: `session_lifecycle_ends_total{subsystem="tmux_control_mode"}` and
    `session_lifecycle_active_generations{subsystem="tmux_control_mode"}` (Epic 3.2, Story
    1.3.3) are on a dashboard panel before the flag's default flips to `true` — emitted-but-
    unwatched metrics do not satisfy this.
  - (b) **Staged, not global**: the flag is enabled on a staged subset of hosts/sessions first
    — never one global flip of every currently-running session at once — and that subset's
    dashboarded metrics are compared against the still-`false` remainder before wider rollout.
  - (c) **Bake-then-delete**: the `false` branch (Task 3.1.2b's literal pre-migration code
    path) is deleted only after that staged, dashboarded bake period shows no new-shape
    `tmux_control_mode` incidents. The bake period's *length* remains an operational call for
    whoever closes this issue (not predetermined here); what this task fixes is the *gate*
    itself — dashboarded and staged, not a calendar date alone.
  Link back to this plan's Risk Control section so the reason the flag exists isn't lost once
  it's routine. Not removing it is itself a form of the exact "ad hoc flag litter" this project
  exists to close out, so this follow-on is not optional busywork.
- Files: none in this repo's tracked plan artifacts beyond the issue reference recorded in
  `ADR-001`'s "Consequences" section (Epic 5.2).

### Epic 5.4: Full-repo verification gate

##### Task 5.4.1a: `make ci` (~10 min, background-runnable)
- The full existing CI pipeline (build, test, lint) green with every phase's changes present,
  confirming no cross-package regression beyond the touched files' own test suites: the 2
  subsystems migrated to `lifecycle.Reason` (`session/tymux`, `session/tmux`) plus
  `session/external_streamer.go`'s adjacent `IsBenignTimeout`-swap fix — 3 files touched, not
  3 `Reason` migrations.
- Files: none (verification only)

##### Task 5.4.1b: `make ready` (~10-15 min, background-runnable)
- Includes the `dupl`/`jscpd` duplication gates (new-code-only for Go) — verify the new
  `session/lifecycle` package and the 3 touched files' diffs (2 `Reason` migrations +
  `session/external_streamer.go`'s 1 adjacent fix) introduce no flagged duplication.
- Files: none (verification only)

