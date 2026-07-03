# Type-Driven Design Audit: `instance-actor-concurrency`

Research only — no `plan.md`/`requirements.md`/ADR/application-code changes made. Verified against
`requirements.md`, `research/architecture.md`, `research/architecture-registry.md`,
`research/pitfalls.md`, `research/pitfalls-registry.md`, `research/pitfalls-register-rollback.md`,
`implementation/plan.md` (full, 2081 lines), `implementation/adversarial-review.md` (fifth pass),
and ADRs 027/028/029/031. Test applied: does the *compiler* make the mistake impossible, or does a
comment/lint/convention merely make it discouraged? Ordered by severity, not prompt order.

---

## Finding A (was prompt item 2, promoted — biggest gap): `LiveInstance` fields stay exported after the migration completes

**Verdict: confirmed real, highest severity** — the exact defect class the migration exists to
close reappears in the design's own end state.

**Evidence.** Story 2.5.2 (`plan.md` ~450-485) specifies the `Instance`→`LiveInstance` rename as
mechanical, "no method-body changes," and gates only the *constructor* surface — never field
visibility. Epic 7 Story 7.1 (~1952-2002), titled "Final `stateMutex` Deletion," removes the mutex
and fixes resulting compile errors; nothing unexports a field. Story 5.3 routes
`liveInst.AutonomousTurn`/etc. through `send`/`sendSync`, but every other field across every epic
stays a directly-addressable exported identifier (`Status`, `GitHubPRURL`, `Title`, `AutoYes`, …) —
`architecture-registry.md` §5.3's own `updateSession` sketch writes `live.Title = newTitle` with a
comment admitting this is a raw field write "until architecture.md's actor migration lands," and no
later story ever converts that field to unexported+accessor.

**Story 5.4 is the tell.** Its entire purpose (`plan.md` ~1874-1898) is a grep/ast-grep CI guard
scanning `server/services/`, `autonomous_driver.go`, `pr_status_poller.go`, `review_queue_poller.go`
for `\w+\.\w+\s*=\s*` against known `Instance`-typed variables, failing CI on a new hit. This guard
is only *necessary* because fields are still exported and directly assignable from outside the
package — unexported, the same regression is a `go build` failure, not a pattern match kept in sync
by hand. `adversarial-review.md` §4 independently confirms the guard's own file list is already
incomplete (`daemon/daemon.go`'s `AutoYes` write is out of scope) — a second-order symptom of the
first-order problem.

`requirements.md` (lines 24-30) explicitly considered "rung 4" — unexport every field, force call
sites through locked accessors — and rejected it *only* because it wouldn't fix reader-contention
performance, stating outright: *"encapsulation, not the specific primitive, is what prevents the
bypassing seen today."* The plan adopts the actor for the performance win but never separately
re-adopts rung 4's encapsulation half. The two aren't mutually exclusive — nothing about a
mailbox-routed actor requires capitalized fields.

**Why it matters.** Once Epic 7 ships, "the actor is the sole mutator" is a comment claim, not a
compiler-enforced one. A future contributor can write `liveInst.Status = session.Paused` from any
new call site, or add a poller nobody adds to Story 5.4's grep-list, and it compiles, runs, and
silently races the actor's next `snapshot.Store()` — the exact clobbering `pitfalls.md` §4
describes today. Task 2.5.7i's own `daemon.go:AutoYes` direct write is a live instance of this
already sitting in the plan's text, caught only because a reviewer noticed it.

**Recommended fix.** Unexport the mutable state fields; keep `Snapshot()` for reads; route writes
through `instanceState`, which is already unexported, package-scoped, and already the only thing
`xxxLocked` functions operate on — no new mechanism required.

```go
// Before — compiles today and after this plan's Epic 7:
type LiveInstance struct {
    Status, Title, AutoYes, GitHubPRURL, GitHubPRNumber ... // ~90 exported fields
    mailbox chan command; ctx context.Context; cancel context.CancelFunc
    done chan struct{}; snapshot atomic.Pointer[InstanceSnapshot]
}
liveInst.AutoYes = true // compiles from any package, any time

// After:
type LiveInstance struct {
    status, title, autoYes, gitHubPRURL, gitHubPRNumber ... // same fields, unexported
    mailbox chan command; ctx context.Context; cancel context.CancelFunc
    done chan struct{}; snapshot atomic.Pointer[InstanceSnapshot]
}
func (i *LiveInstance) Snapshot() *InstanceSnapshot { return i.snapshot.Load() } // reads, unchanged

liveInst.AutoYes = true // compile error: unexported
liveInst.send("SetAutoYes", func(s *instanceState) { s.inst.autoYes = true }) // only path that compiles
```

Story 5.4's CI guard becomes unnecessary and deletable — the compiler subsumes it, with no file
list to keep in sync. Natural slot: Epic 7 Story 7.1, right after `stateMutex` deletion, once every
writer is already actor-routed (Epics 4/5 guarantee this) so the compiler enumerates every
remaining direct-write site the same way it enumerates dangling `stateMutex.Lock()` calls. Doing it
at the end of Epic 5 is also viable and arguably better — it removes the need for Story 5.4's lint
guard for the epics that follow it.

---

## Finding B (prompt item 1): `release()` vs `ForceRelease()` — confirmed, narrower than a raw signature collision

**Verdict: confirmed real**, but the mechanism differs from "both are `func()` and get swapped as
parameters." Per Story 2.5.3, `Acquire`/`Register` return a bound closure `release func()`, while
`ForceRelease(sessionID string)` is called directly on the receiver — different arity prevents an
accidental swap at most call sites.

The real risk is exactly what `pitfalls-register-rollback.md` §2 documents by hand: at a decision
point where both are valid Go and only one is correct — `CreateSession`'s `storage.AddInstance`
failure path holds a `release()` closure (from `Register`) *and* can call
`ForceRelease(instance.GetStableID())` — nothing about either API's type signals which is right; you
need the doc-comment's refcount-race explanation to know `release()` is silently wrong there. The
rollback doc's own words: *"the more obvious-looking (and occasionally wrong) choice"* is
`release()`; its own fix (§2, Conclusion) is a doc-comment addition, explicitly rejecting a
distinctly-named method as "not required for correctness" — a discipline fix, not a type fix.

The risk compounds once either closure is stored generically: `ReviewQueuePoller.releases
map[string]func()` and `daemon.go`'s `releases *[]func()` are both bare `func()`. Nothing stops a
future call site from putting a `func() { registry.ForceRelease(id) }` wrapper into one of these
collections, whose iteration code assumes every entry is a refcount-safe `release()` — silently
force-evicting a session out from under another holder, the exact hazard `Registry` exists to
prevent, one layer up.

**Recommended fix — two named func types, not a reason-argument:**

```go
type ReleaseFunc func()      // refcount-gated: safe from any holder, any # of calls, idempotent.
type ForceReleaseFunc func() // unconditional: evicts every holder. Never store where ReleaseFunc expected.

func (r *Registry) Acquire(sessionID string) (*LiveInstance, ReleaseFunc, error)  // was func()
func (r *Registry) Register(instance *LiveInstance) (ReleaseFunc, error)          // was func()

// Any future wrapper of ForceRelease must produce a ForceReleaseFunc explicitly:
func abortRegistration(r *Registry, id string) ForceReleaseFunc { return func() { r.ForceRelease(id) } }
```

```go
// Before: indistinguishable once assigned to a local.
var teardown func()
if forced { teardown = func() { registry.ForceRelease(id) } } else { teardown = release }

// After: unifying them without an explicit conversion is a compile error.
var teardown ReleaseFunc = release                          // ok
var teardown ReleaseFunc = abortRegistration(registry, id)  // compile error: type mismatch
```

**Newtype alone vs. reason-argument:** the newtype alone suffices. `ForceRelease` has exactly two
callers in the whole plan (`DeleteSession`'s force-invalidate, `CreateSession`'s abort-on-failure),
both already well-documented at the call site — a mandatory `ForceRelease(reason
ForceReleaseReason)` would mostly duplicate the existing doc-comment for two callers, which is the
over-engineering the skill warns against. Apply `ForceReleaseFunc` if/when a second genuine wrapper
of `ForceRelease` appears; not justified today.

---

## Finding C (prompt item 3): `xxxLocked` split relies on a lint rule the plan's own text says won't catch a specific mistake

**Verdict: confirmed real**, and unusually self-aware — Task 4.3c (`plan.md` ~1670-1677) states
outright: *"Story 4.5's ast-grep lint rule does **not** catch this specific mistake (... a
lexically-nested-but-different-goroutine closure calling a `Locked` twin) — call this out
explicitly in code review."* That's a discipline mitigation for a hazard the author already knows
the lint misses.

**The hazard:** `start()` becomes `startLocked(s *instanceState, ...)` (Task 4.2a). A closure
defined lexically inside it (`SetOnExitCallback`, fired by the tmux-reader goroutine, not the
actor) has `s` in lexical scope after conversion — an implementer can mechanically write
`transitionToLocked(s, ...)` directly because it visually looks like it belongs there. It compiles.
It's wrong: that closure runs on the tmux-reader goroutine, at callback-fire time, not the actor
goroutine at command-process time — calling the twin directly reintroduces unsynchronized direct
mutation via a closure capture instead of a public method call. Story 4.5's rule matches "a
`*instanceState`-taking function whose body calls `i.<Public>()`" — it can't detect that a *nested
closure* closing over `s` runs on a different goroutine than the one that's "supposed to" own `s`.

**Is a capability-type fix practical?** Investigated as asked:

```go
type actorToken struct{} // unexported; only constructed inside runActor's own frame
func transitionToLocked(s *instanceState, tok actorToken, ctx context.Context, to Status) error { ... }
```

This would close the gap if the hazard were "code outside `session` calling a `Locked` twin" — it
isn't. The hazard is a closure in the *same package, same file, same function being converted*:
lexical scope already gives it access to any unexported identifier, including `actorToken{}` itself
— an empty struct literal has no invariant to gate construction on (unlike a smart-constructor
`Email`), so any code in `session/` could write `transitionToLocked(s, actorToken{}, ctx, to)` and
the token buys nothing. This is a mistake occurring *within* the trust boundary a capability type
protects, so no in-package marker type can distinguish "the actor goroutine, right now" from "other
code with textual access to the same unexported struct" — that's goroutine identity, which Go's
type system can't express at compile time without runtime tagging (itself only a debug-assertion
idiom, not a proof).

**Recommendation: keep the lint rule, tighten it; don't build a capability type** — this is exactly
the skill's own anti-pattern (forcing a structural-type fix onto a control-flow problem, not a
data-provenance one). Two lighter fixes instead:

1. **Tighten Story 4.5's rule** to additionally flag any closure literal lexically nested inside a
   `*instanceState`-taking function that both references the outer `s` *and* is passed to something
   shaped like callback registration (`SetOnExitCallback`, `RegisterCompletionCallback`,
   `RegisterTurnCallback`) — narrowing from "any `i.Public()` call" to "any closure escaping via
   callback registration that still touches `s`." Pattern-matchable structurally with ast-grep,
   no runtime capability needed.
2. **Move the closure out of `Locked`'s lexical scope.** Task 4.3c's fix (route through
   `send`/`sendCtx`) is already correct; the residual risk is only that a future edit re-introduces
   the mistake because `s` is visually reachable. Rename `startLocked`'s parameter to something like
   `actorState` to signal "actor-only," and define the exit-callback as a named, top-level function
   taking `i *LiveInstance` (not an inline closure) that calls `i.send(...)`. Removes the visual
   temptation at zero type-system cost — the smallest fix here is naming/structure plus a sharper
   lint pattern, not a new type.

---

## Finding D (prompt item 4): `sendSync`'s typed error on a stopped actor

**Verdict: mostly fine as specified — ordinary idiomatic Go, not a fresh gap.**
`sendSync[T any](...) (T, error)` returns sentinel `ErrInstanceStopped`; `sendSyncErr` flattens the
outer actor-stopped error and the command's inner error into one value. The caller must still check
`err != nil` — Go has no way to make an error "unignorable" the way a checked exception or a
never-unwrap-without-checking `Result` would — but that's a language property, not a gap specific
to this plan. Adding a `Result[T]`-style wrapper here for one pattern, when every other
error-returning function in the codebase has the same "technically ignorable" property, would be
inconsistent, over-engineered ceremony for two callers.

**However**, `adversarial-review.md` (fifth pass, finding 1) found a real, *adjacent* bug worth
flagging: `select { case i.mailbox <- cmd: ; case <-i.ctx.Done(): return err }` doesn't
deterministically prefer the already-canceled branch once both are simultaneously ready (Go's
`select` picks uniformly at random among ready cases) — a caller sending to an already-long-dead
actor has a real, bounded (~1-in-32, per ADR-027's mailbox capacity) chance of hanging forever per
call, not a guaranteed typed error. Out of this audit's scope (control-flow bug, not a missing
type), but it changes the honest answer to "is `ErrInstanceStopped` reliable" from yes to "reliable
once the fifth-pass blocker's fix lands, not yet as currently specified." Defer to
`adversarial-review.md`'s existing fix recommendation rather than re-litigating it here.

---

## Finding E (additional, minor): `WithInstance` unreachable through the `InstanceAcquirer` interface Task 2.5.5c prescribes — self-resolving via compile error

Story 2.5.4 defines `WithInstance` as a concrete method on `*Registry`, not part of the
`InstanceAcquirer` interface (one method: `Acquire`). Task 2.5.5c's "narrowest interface" rule says
most consumers should be typed against `InstanceAcquirer` — but Story 2.5.7's `tools_lifecycle.go`
conversion (Task 2.5.7c) requires calling `.WithInstance(...)`, only possible if that field's static
type is `*Registry` or a wider interface. Followed literally, Task 2.5.5c's guidance would make
Task 2.5.7c's code fail to compile. Flagged for awareness only, not as a primary finding: unlike A-C
this is self-correcting — the compiler forces resolution the moment someone implements it. Worth a
one-line clarification in Task 2.5.5c naming `tools_lifecycle.go` (and any other
`WithInstance`-calling consumer) as taking `*Registry` or a `WithInstance`-inclusive interface, so
the implementer doesn't discover it via a failed build.

---

## Summary

| # | Finding | Real? | Fix size |
|---|---|---|---|
| A | `LiveInstance` fields stay exported post-migration; Story 5.4's CI grep is the only thing between "actor is sole mutator" and a silent direct write | Yes — confirmed, highest severity | Unexport state fields; route writes through the existing `instanceState`/`xxxLocked` mechanism. No new abstraction. |
| B | `release()`/`ForceRelease()` conflatable once stored as bare `func()`; rollback doc flags "the obvious-but-wrong choice," mitigates only with a comment | Yes — confirmed, narrower than a raw signature swap | Two named func types (`ReleaseFunc`/`ForceReleaseFunc`). No reason-argument — two callers don't justify it. |
| C | `xxxLocked` lint rule provably misses the lexically-nested-closure hazard (plan's own words) | Yes — confirmed, plan flags it but only mitigates via code review | Tighten the ast-grep pattern for closures escaping via callback registration; rename the actor-only parameter; extract to a named top-level function. Capability type investigated and rejected — hazard is same-package lexical scoping, not an external-caller problem a marker type can gate. |
| D | `sendSync`'s typed error is ordinary, checkable-but-ignorable Go error handling | Not a gap specific to this plan — flagged the adjacent real select-fairness bug from the fifth adversarial pass instead (control-flow bug, not a type gap) | N/A — defer to `adversarial-review.md`'s existing fix |
| E | `WithInstance` unreachable through the `InstanceAcquirer` interface Task 2.5.5c prescribes for "most consumers" | Real inconsistency, self-resolving via compile error | One-line clarification in Task 2.5.5c, not a design change |
