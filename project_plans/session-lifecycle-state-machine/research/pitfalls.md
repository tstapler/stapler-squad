# Pitfalls: goroutine-lifecycle state machine in Go

Research for Phase 2 (Agent 4). Sources: this repo's git history, current
`session/tymux` and `session/tmux` source, `qmuntal/stateless`'s GitHub
issue/PR history (fetched via `gh api`/`gh issue view`/`gh pr view`), and its
README (fetched via `read_website`).

## 1. What this codebase's own history already proves about the failure mode

The bug is never "the enum is missing a case" in the abstract — it's **a
decision point built to be extended by accretion, extended three times, each
extension leaving the mechanism *more* boolean-shaped, not less**:

| Commit / fix | Signal added | Type |
|---|---|---|
| `477eacd6a` (2026-08-23) | `s.exited` | field read via `s.mu.RLock()` |
| this session, fix #1 | `ctx.Err() != nil` | inline call, no field at all |
| this session, fix #2 | `maxTeardownWait` (5s) bound on `teardownStandingStream`'s `<-done` wait | a *timeout*, not a classification — doesn't even live in the `if` |

Current condition in [`session/tymux/stream.go:212`](session/tymux/stream.go#L212)
(uncommitted as of this research):

```go
if s.closing.Load() || exited || ctx.Err() != nil {
    close(done)
    return
}
```

Three independent signals, three different storage/sync strategies
(`atomic.Bool`, mutex-guarded `bool`, a `context.Context` method call), OR'd
together at one `if`. Nothing forces a fourth legitimate "this generation is
ending deliberately" cause (there will be one — the pre-mortem should assume
it) to be added to this line. The comment above it is now 24 lines long,
explaining three different provenances by hand — **the documentation is
compensating for the type system's silence, which is exactly backwards.**
The recurring mistake is not "forgot a case," it's **using an untyped boolean
OR as the closed-set membership test**. A new mechanism that produces an
`if isTerminal(reason) || isOtherTerminal(reason) ...` shaped call site
anywhere has relocated the bug, not eliminated it — the fix must make
"handle every current member of the terminal set" a single exhaustive
`switch`/lookup keyed on one classified value, not a boolean accumulator.

Fix #2's mechanism (`maxTeardownWait`, [`stream.go:140`](session/tymux/stream.go#L140))
is itself instructive: it's a **reliability patch bolted next to** the
classification bug, not integrated with it. The new design should not
reproduce this shape — "what timeout applies while waiting for a
generation to report its terminal reason" is a property of the state
machine (a state can have a bounded max dwell time), not a freestanding
`time.After` race bolted onto a `<-done` wait outside it.

## 2. Idiomatic-Go closed-enum pitfalls

**Zero-value trap.** An `iota`-based enum's zero value is whatever the first
declared constant is, by declaration order alone — nothing marks it
"special." For *this* problem shape the danger is concrete: if the chosen
type is something like

```go
type StopReason int
const (
    StopReasonUnknown StopReason = iota // = 0
    StopReasonDeliberateClose
    StopReasonTransportDrop
    ...
)
```

then any code path that constructs the value without an explicit reason
(a forgotten assignment, a struct literal that never sets the field, a new
call site added under time pressure) silently gets `StopReasonUnknown == 0`.
Whether that's *safe* depends entirely on what the decision point does with
`Unknown` — and the Success Metrics in `requirements.md` require the answer
to be "stop," never "loop/reconnect forever." Concretely: **the enum's zero
value must either be a reserved "stop for an unclassified reason, log it
loudly" sentinel, or the decision switch's `default:` case must fail safe
regardless of which zero-valued variant reaches it** — do not rely on
authors always remembering to set the field. The now-superseded `session/
tymux` booleans actually got the *direction* right by accident (`closing`/
`exited` default `false`, i.e. "still running" — the bug was that the
*absence* of a signal fell through to "reconnect," which is what the new
design must invert: absence of a recognized terminal signal should fall
through to "stop, with a loud unknown-reason log/metric," not "continue as
if nothing happened").

**Exhaustiveness is a linter opt-in, not a language guarantee.** Go's
`switch` has no compiler-enforced exhaustiveness; `golangci-lint`'s
`exhaustive` linter is what fills that gap, and this repo already runs it —
but currently carved **out** of every package except `session/detection/`,
via a name-by-name regex-negation chain in
[`.golangci.yml:181-212`](/.golangci.yml#L181-L212) (`^session/[^d]`,
`^session/d[^e]`, `^session/de[^t]`, ...). Two concrete risks in reusing
that mechanism for the new type(s):
- It's easy to get the regex chain subtly wrong in either direction — too
  broad silently re-enables `exhaustive` somewhere unintended (build
  breaks on unrelated pre-existing switches using `iota` + `default:`,
  which the comment there says is the deliberate repo-wide convention), too
  narrow silently leaves the new type's switches unchecked, which is a
  *false sense of safety*: "we adopted an exhaustive-checked enum" is not
  true unless the linter is actually wired for that specific package/type.
- `exhaustive` by default treats a `default:` clause as satisfying
  exhaustiveness (`default-signifies-exhaustive: false` is explicitly set
  in this repo's config to turn that permissiveness *off* — see
  `.golangci.yml:23`). That's the correct setting for this project's goal,
  but it means every migrated switch **must not** lean on a catch-all
  `default:` to paper over an unhandled case, or the linter's guarantee is
  worthless even when wired on. This is a plan-phase decision to get right
  per subsystem, not assumed.

**Untyped-int leakage across package boundaries.** If the shared building
block's reason/state type is a plain `int`-based type (not a distinct named
type with unexported validation), nothing stops a caller in another package
from constructing an invalid value via a raw integer literal or an
out-of-range conversion, defeating the "closed" property entirely. The
constraints section's own suggested idioms (unexported-method-gated closed
type, required constructor parameter) exist specifically to close this
hole — a `type Reason int` with exported `iota` constants and no
constructor gate is not actually closed against the rest of the module,
let alone another package.

## 3. Concurrency-specific pitfalls (this repo's own documented class)

`.claude/rules/instance-lock-free-reads.md` already names this exact bug
class for a different field set: `*Instance` fields written under
`i.mu.Lock()` by actor setters and republished to an atomically-stored
`InstanceSnapshot`; **reading the raw field directly, without `Snapshot()`
or an `RLock()`-guarded accessor, races with those writes** — caught by
`go test -race` in `TestCreateSession_GitHubURLResolution_NotBoundByRequestContext`.

The exact same shape already exists in the code this project is replacing:
`session/tymux/stream.go`'s `s.exited` is read via `s.mu.RLock()` in the
reader goroutine ([`stream.go:195-197`](session/tymux/stream.go#L195-L197))
while `handleAttachEvent` writes it under `s.mu.Lock()`
([`stream.go:252-254`](session/tymux/stream.go#L252-L254)) from the same
goroutine, but nothing about the *type* enforces that every future field
composing the terminal-reason decision gets the same lock discipline —
today it's tracked by convention and comment, exactly the gap the
`instance-lock-free-reads.md` rule closes for `Instance` fields.

**Design requirement this implies:** whatever the new mechanism exposes
publicly (a `Snapshot()`-shaped accessor returning the current
state/reason, or an atomically-published value) must be the *only*
sanctioned way to read "why is this generation ending" from a goroutine
other than the one driving the transition — mirroring the
`Snapshot()`/`atomic.Pointer[InstanceSnapshot]` pattern already proven in
`session/actor.go`'s `runActor` (stores a fresh snapshot after every
command) rather than reinventing an ad hoc mutex-guarded struct field per
subsystem the way `session/tymux` and `session/tmux` currently each do
independently. If the chosen mechanism is a library (`qmuntal/stateless`),
its own concurrency contract (see §4) must be verified to satisfy this —
"the library is thread-safe" is not the same claim as "reading current
state from a second goroutine while the first goroutine is mid-transition
is race-free and wait-free," and the codebase needs the latter.

**Actor-goroutine wedging is a distinct problem shape, not the same bug.**
Confirmed by reading [`session/actor.go`](session/actor.go) in full: the
actor's *own* shutdown condition (`runActor`'s `select { case
<-li.ctx.Done(): return; case cmd := <-li.mailbox: ... }`,
[`actor.go:121-135`](session/actor.go#L121-L135)) is already a single,
unambiguous, two-branch `select` — it is *not* an instance of the
scattered-boolean smell, confirming the Feasibility Risk in
`requirements.md` ("the audit may find `session/actor.go`'s concern is
about shutdown *ordering*, not stop-vs-reconnect classification"). The real
risk incident #2 exposed is different: a **command closure running inside
the actor can block indefinitely on something outside the actor's own
control** (here, `teardownStandingStream`'s unbounded `<-done` wait on a
goroutine wedged inside `Receive()`), which stalls the mailbox drain loop
for every subsequent command on that `Instance` — a liveness/deadlock risk
of the actor's *execution model*, not a missing state in an enum. Forcing
this into the same state-machine type as the tymux/tmux reader-goroutine
decision would be the "conflating problem shapes" rabbit hole the
requirements doc names; if it's addressed at all here, it should be a
different mechanism (e.g. a required per-command deadline/context
enforced by the actor itself), not a state added to the terminal-reason
enum.

## 4. `qmuntal/stateless`-specific pitfalls (from its own issue/PR history)

Checked via `gh api search/issues`, `gh pr view`, `gh issue view` against
`qmuntal/stateless` (BSD-2-Clause, 1383 stars, last push 2026-02-10, 16 open
issues as of this research — active but not fast-moving).

- **The README markets "Thread-safe" as a headline bullet, but the
  project's own history shows that guarantee was earned late and narrowly,
  not designed in from the start:**
  - [Issue #61](https://github.com/qmuntal/stateless/issues/61) —
    `String()` (a read method, not even a transition) panicked under
    concurrent calls because the internal `stateConfig` map was lazily
    populated on read with no locking (`sm.stateConfig[state] = sr` — "concurrent
    panic point," maintainer's own words in the fix commit). Fixed, but it
    shows introspection/read paths were not part of the original
    thread-safety design.
  - [PR #13](https://github.com/qmuntal/stateless/pull/13),
    [PR #66](https://github.com/qmuntal/stateless/pull/66), and
    [PR #68](https://github.com/qmuntal/stateless/pull/68) — **three
    separate rounds** of fixing race conditions in the library's own
    internal queued-`Fire()` implementation; PR #68's description says it
    "reimplements how triggers are fired in queued mode to make it more
    robust to race conditions" and explicitly supersedes #66. A library
    needing three iterations to get its *own* concurrent-Fire path
    race-free is a signal to verify current behavior with a real `-race`
    spike against this project's actual usage pattern, not to trust the
    README bullet at face value.
- **`FiringMode` (`FiringQueued` vs `FiringImmediate`) has surprising
  semantics for exactly this project's use case** — a background goroutine
  firing a trigger asynchronously in reaction to an I/O error, potentially
  while another action is still in flight.
  [Issue #6](https://github.com/qmuntal/stateless/issues/6): with the
  default `FiringQueued` mode, if `OnEntry()` is slow (their repro: a 5s
  sleep), a `Fire()` call from *another goroutine* during that window is
  silently queued rather than either applying immediately or erroring —
  "no error happened" was the surprising part reported. For a reader
  goroutine that needs an authoritative, timely answer to "should I stop or
  reconnect right now," the default mode is the wrong choice without a
  deliberate, tested decision to use `FiringImmediate` (which has its own
  reentrancy hazards — Fire-from-within-an-action). This is precisely the
  "does it fit an async, event-driven transition, or a synchronous
  explicitly-triggered model" question `requirements.md`'s Rabbit Holes
  section already flags — the issue history confirms the concern is real,
  not hypothetical, and resolvable only by an explicit `FiringMode` choice
  plus a spike, not by adopting defaults.
- [Issue #36](https://github.com/qmuntal/stateless/issues/36) —
  `GetTransition` can panic (introspection API, same family of gap as #61).
  Anything the new design exposes for observability (span attributes,
  "what state was I in") should be verified against the exact library
  version pinned, not assumed safe because "thread-safe" is in the README.
- [Issue #79](https://github.com/qmuntal/stateless/issues/79) (open,
  unresolved as of this research) — asks for the ability to reuse/reset a
  state machine instance rather than constructing a fresh one per use.
  Relevant because this project's shape is "one state machine per
  goroutine generation" (a new `readAttachLoop` invocation per reconnect) —
  if the library expects one long-lived `StateMachine` value rather than
  cheap per-generation construction, that mismatch needs to be resolved
  explicitly (construct fresh per generation vs. reset-and-reuse), not
  discovered mid-migration.
- **No documented cancellation/context-propagation contract for `Fire`
  beyond passing a `context.Context` argument through to actions** — the
  library's `context.Context` parameter is for user callback plumbing
  (`OnEntry(func(ctx context.Context, args ...any) error {...})`), not a
  cancellation signal for `Fire` itself. If the new mechanism needs "abandon
  this transition if the caller/generation is being torn down" semantics
  (which incident #2 and fix #2's bounded wait suggest it will), that has
  to be built on top, not assumed to come from the library passing ctx
  through.

**Bottom line for the spike the requirements doc calls for:** validate
`FiringImmediate` (or a considered `FiringQueued` choice with an explicit
test for the issue #6 scenario) against the actual `readAttachLoop` shape
under `-race`, with a concurrent `String()`/introspection call from an
observability path running at the same time as a live `Fire()`, before
committing the whole migration to the library.

## 5. Migrating two production subsystems: partial-rollout and drift risk

**`session/tmux/control_mode.go` already has the same smell, confirmed by
reading it, not just suspected in the requirements doc.** Independent
signals checked ad hoc at *three separate call sites* to decide whether to
fire `onExit`:

| Site | Guard | Line |
|---|---|---|
| Scanner-EOF fallback (pipe closed, no `%exit` seen) | `!t.intentionalStop.Load()` | [`control_mode.go:491`](session/tmux/control_mode.go#L491) |
| `%exit` notification handler | `!t.intentionalStop.Load()` | [`control_mode.go:656`](session/tmux/control_mode.go#L656) |
| `%session-closed` notification handler | `!t.intentionalStop.Load()` | [`control_mode.go:668`](session/tmux/control_mode.go#L668) |

plus a second, separately-guarded flag `controlModeExited` (under
`controlModeSubMu`, [`control_mode.go:635`](session/tmux/control_mode.go#L635))
tracking a related-but-distinct fact (whether the drain has already run) —
two boolean flags, three call sites, guarding two overlapping but not
identical questions ("was this an intentional stop" vs. "have we already
drained"). This is the *same class* as `session/tymux`'s bug: a fourth
notification type or a new intentional-stop path added later has no
structural nudge to update all three checks. **This confirms the audit
finding from `requirements.md` rather than merely assuming it** — it is a
real second migration target, not a hypothetical.

**Blast-radius asymmetry is the dominant risk, not migration mechanics.**
`session/tmux` is the default, highest-traffic backend; `session/tymux` is
opt-in via `config.TymuxSessionOverrides`. Concrete failure modes specific
to running both migrations under one shared mechanism:
- **Behavior drift between backends disguised as "using the same
  abstraction."** If the shared type's terminal-reason vocabulary is
  designed against `session/tymux`'s three known signals
  (`closing`/`exited`/`ctx.Err()`) and then mapped onto `session/tmux`'s
  different signal set (`intentionalStop`/`controlModeExited`/scanner-EOF)
  by approximation rather than exact correspondence, a subtle mismatch
  (e.g. `session/tmux`'s scanner-EOF fallback has no equivalent to
  `session/tymux`'s clean-`exited`-then-stream-closes sequencing) could
  silently change which reason code the default backend reports for a
  case that used to be handled correctly by its own bespoke logic — a
  regression that passes review because "it compiles and the enum is
  exhaustive" while being behaviorally wrong for one specific transition.
  Each subsystem's full existing signal set must be enumerated and mapped
  1:1 before migration, not inferred from the other backend's shape.
- **A shared bug in the mechanism itself now hits both backends at once**
  instead of being contained to the one subsystem that has always carried
  the risk. This is exactly why `requirements.md`'s Risk Control section
  requires separable commits/PRs per subsystem — a bad migration in
  `session/tmux` must be revertable without also reverting the
  `session/tymux` migration (and vice versa), which means the shared
  building block's own API must be stable/versioned enough that reverting
  one consumer doesn't strand the other mid-migration on a
  since-changed shared type.
- **Regression coverage asymmetry.** `session/tymux` has 46 existing
  tests plus this session's own two new regression tests for incidents #1
  and #2 — a reasonably well-instrumented target. `session/tmux`'s
  `control_mode.go` path has no equivalent history of production incidents
  driving test coverage of *this specific* decision point (the three
  `onExit`-guard call sites) — the audit finding it has the smell doesn't
  mean it has the same test scaffolding to validate a migration against.
  Per Risk Control, the `session/tmux` migration needs its own
  from-scratch regression tests for whatever concrete near-miss justifies
  including it, not a rider on `session/tymux`'s existing suite.

## 6. Summary of what the design must do to actually close the class

1. **One classified value, not an accumulator.** The decision point must
   consume a single already-classified reason value (produced once, at the
   point the generation actually ends) via an exhaustive `switch`, never a
   boolean expression combining multiple independently-set signals at the
   read site — that's the literal shape of all three incidents.
2. **Zero value must be the safe outcome, or the switch's fallback must be
   safe regardless of zero value.** Verified by a test that adds a new,
   unhandled reason and asserts the goroutine stops (per Success Metrics) —
   not merely by code review of the enum's declaration order.
3. **Exhaustiveness must be actually wired, not just structurally
   possible.** Confirm the `.golangci.yml` carve-out is correctly scoped to
   the new type's package(s) with a test/CI check that intentionally
   fails if a case is added to the type but not the switch — don't accept
   "we used an exhaustive-shaped idiom" as equivalent to "the linter
   enforces it here."
4. **Cross-goroutine reads go through one sanctioned accessor**
   (`Snapshot()`-shaped or equivalent), mirroring
   `.claude/rules/instance-lock-free-reads.md` and `session/actor.go`'s
   existing atomic-snapshot pattern — never a raw field plus a
   caller-remembered lock.
5. **If `qmuntal/stateless` is adopted, its `FiringMode` choice and
   concurrent-introspection behavior must be spike-tested against the real
   `readAttachLoop` shape under `-race`** before the whole migration
   depends on it — its own issue history shows both "queued Fire from
   another goroutine has surprising silent-delay semantics" (#6) and
   "introspection methods have had their own separate concurrency bugs"
   (#61, #36) as real, not hypothetical, risks.
6. **`session/tmux`'s migration is scoped, tested, and reverted
   independently of `session/tymux`'s** — separate commits/PRs, its own
   from-scratch regression tests for the three-call-site `onExit`-guard
   smell confirmed above, and explicit sign-off given its default-backend
   blast radius, per Risk Control.
