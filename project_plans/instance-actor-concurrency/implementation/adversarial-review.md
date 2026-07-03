# Adversarial Review: Instance Actor + Atomic Snapshot Migration Plan (Sixth Pass)

**Reviewer**: independent re-verification pass over the fifth pass's two blockers (select-race,
Register-rollback) plus `type-driven-audit.md` findings A-E, checked directly against the
rewritten `implementation/plan.md` (8 epics / 32 stories / 114 tasks), `requirements.md`,
`research/pitfalls-select-race.md`, `research/pitfalls-register-rollback.md`, and
`daemon/daemon.go` / `session/mux/multiplexer.go` / `session/tmux/server_registry.go` /
`session/hibernation_sweeper.go` source directly (not assumed from the research docs' claims).
**Date**: 2026-07-01
**Verdict**: **CLEAN** (one cosmetic, non-blocking documentation nit noted — no code or task-text
defect)

## Why this pass believes "clean" where the prior five did not

Every one of the prior five passes found a real, code-verified defect and this plan's patches
have a track record of actually closing what they claim to close (verified again below, not
re-trusted). This pass is the first where independent re-derivation of each fix — read the
research doc's argument, then read the plan's task text, then read the actual source file the
plan cites as precedent or as the site being changed — produced no daylight between them. The
concrete things that changed my confidence relative to just reading the summary claims:

- The select-race fix's cited precedent (`session/mux/multiplexer.go:493-497`) was fetched and
  read directly: it is exactly `select { case <-m.ctx.Done(): return; default: }` at the top of a
  read loop, immediately before a blocking `SetReadDeadline`+`DecodeMessage` call — the identical
  shape Task 3.1a specifies, off by one line number (492-496 in the file today vs. 493-497 cited),
  not a fabricated or mismatched precedent. Two more cited precedents
  (`session/tmux/server_registry.go:307-311`, `session/hibernation_sweeper.go:270-274`) were also
  fetched and match the same shape exactly.
- Task 2.5.7h's rollback code block was read character-by-character against
  `pitfalls-register-rollback.md`'s recommendation: it calls `s.registry.ForceRelease(...)`, not
  the `release()` `Register` returns, on both the `AddInstance`-failure path and (implicitly, by
  never invoking it) the success path; the ordering (`Register` → `stopActor()`-on-collision →
  `AddInstance` → `ForceRelease()`-on-failure, all synchronous, inline, before any `return`) has
  no gap for another goroutine to observe an intermediate state.
- `daemon/daemon.go` was read directly to confirm Task 2.5.7i's cited line numbers
  (`session.NewStorageWithRepository` at line 32, `storage.LoadInstances()` at lines 37 and 292,
  `instance.AutoYes = true` at lines 43 and 312) are accurate against the file as it exists today,
  not stale from an earlier pass.
- The one genuine gap found (below) is a documentation leftover, not a mechanism gap — it does not
  change what an implementer would actually build, because the correct, resolved signature appears
  unambiguously three other places in the same document (Story 2.5.3, Task 2.5.7h, Open Decisions
  #7), all of which an implementer reaches before or alongside the stale text.

---

## 1. Select-race fix (fifth pass finding 1): verified closed, for all four send variants

**Claim under test**: does Task 3.1a's patched text specify the non-blocking priority pre-check
*before* the blocking select, correctly, for `send`, `sendSync`, `sendSyncErr`, and `sendCtx` (not
just one), and does the cited precedent actually match?

**Verified directly** (`plan.md` ~1356-1421, ~1517-1563):

- `send`: pre-check `select { case <-i.ctx.Done(): return; default: }` immediately before the
  existing blocking `select { case i.mailbox <- cmd: ; case <-i.ctx.Done(): return }`. Present.
- `sendSync[T any]`: same pre-check, returning `(zero, ErrInstanceStopped)` on the canceled branch,
  before the blocking select that also returns `(zero, ErrInstanceStopped)` on `ctx.Done()`.
  Present.
- `sendSyncErr`: sugar over `sendSync[error]` — "inherits the priority pre-check via its
  delegation to `sendSync`; no separate fix needed." Correct: it never touches `i.ctx` or the
  mailbox directly, so there is nothing for it to duplicate.
- `sendCtx`: explicitly called out as needing "the same non-blocking priority pre-check as
  `send`/`sendSync` above (checking both `ctx` and `i.ctx`)" — the three-case shape (mailbox send,
  actor's own `ctx.Done()`, caller's `ctx.Done()`) is specified, with the pre-check checking both
  contexts. Present.

All four variants are covered — three explicitly, one (`sendSyncErr`) correctly by delegation
rather than needing its own copy of the fix.

**Precedent check** (fetched `session/mux/multiplexer.go` directly, not trusted from the research
doc's quote):

```go
// session/mux/multiplexer.go:492-496 (as it exists today)
for {
    select {
    case <-m.ctx.Done():
        return
    default:
    }
    // Set read deadline to allow checking context
    _ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
    msg, err := DecodeMessage(conn)
    ...
```

This is exactly the non-blocking cancellation pre-check shape Task 3.1a specifies (guard a loop's
top against a canceled context before further, potentially-blocking work) — a real, accurate
precedent, not a superficial match. Two further cited precedents
(`session/tmux/server_registry.go:307-311`'s control-mode retry loop,
`session/hibernation_sweeper.go:270-274`'s per-instance warm loop) were also fetched and match the
identical `select { case <-ctx.Done(): return; default: }` shape.

One nuance worth being explicit about, since it's the crux of why the fix is correct: the
precedent's own use case is "don't do more work once canceled" (a loop-top guard), not "don't pick
the wrong branch of a race between a send and a cancellation" — but the *mechanism* (non-blocking
pre-check catches a long-since-canceled context deterministically, because a closed `Done()`
channel never un-closes) is identical regardless of what comes after the guard. The plan's own
text makes this connection explicitly rather than asserting it by analogy alone, and
`pitfalls-select-race.md` §1 independently derives the same conclusion from the Go spec's
"uniform pseudo-random selection among ready cases" language. Sound.

The regression-test requirement (call `send`/`sendSync` in a **loop of ≥100 iterations** after
`stopActor()` has already returned, asserting the typed error every single time — not a
single-call assertion, since the un-fixed bug is probabilistic) is present in Task 3.1a's own text,
matching the research doc's recommendation exactly.

**Verdict: closed correctly, no gaps found.**

---

## 2. Register-rollback fix (fifth pass finding 2): verified `ForceRelease`, verified ordering

**Claim under test**: does Task 2.5.7h now correctly call `ForceRelease` (not `release()`)
synchronously on `storage.AddInstance` failure, with unambiguous ordering?

**Verified directly** (`plan.md` ~1076-1137):

```go
if _, err := s.registry.Register(instance); err != nil {
    instance.stopActor()
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to register instance: %w", err))
}
if err := s.storage.AddInstance(instance); err != nil {
    s.registry.ForceRelease(instance.GetStableID())
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
}
```

- Calls `ForceRelease(instance.GetStableID())`, explicitly and repeatedly (the task text says so
  three separate times, once in prose, once in the code comment, once in the "must be
  `ForceRelease`, explicitly NOT `release()`" callout) — not the `release()` closure `Register`
  returned.
- The reasoning for why `release()` would be wrong is present and correct: `Register` inserts at
  refcount 1; a concurrent `Acquire(sameID)` racing in between `Register()` succeeding and the
  cleanup running would bump refcount to 2, and a plain `release()` would only decrement to 1,
  leaving the phantom entry alive — `ForceRelease` deletes unconditionally regardless of refcount,
  correctly handling that race.
- Ordering is unambiguous: the cleanup call is inline, in the same goroutine, before the
  `return` statement — no `go registry.ForceRelease(...)` shortcut, and the task text explicitly
  rules that out ("An async `go registry.ForceRelease(id)` before returning would reopen a
  narrower version of the same bug").
- Two additional unit tests are specified matching `pitfalls-register-rollback.md`'s
  recommendation exactly: (a) forced `AddInstance` failure → registry entry does not survive, actor
  fully stopped; (b) a concurrent-`Acquire`-during-abort race test that would fail if the fix used
  `release()` instead of `ForceRelease` — the specific regression test that catches the
  wrong-but-plausible implementation choice, not just "some test passes."
- `ForceRelease`'s doc comment (Story 2.5.3, `plan.md` ~648-658) is updated to document this second
  caller, matching the research doc's recommended addition verbatim in substance.

**Verdict: closed correctly, ordering unambiguous, no gaps found.**

---

## 3. `ReleaseFunc`/`ForceReleaseFunc` propagation (`type-driven-audit.md` finding B)

**Claim under test**: were the new types threaded through every place that previously returned a
bare `func()`, or is there a spot where a bare `func()` still lingers?

**Verified by direct grep of every `func (r *Registry)` signature and every consumer story**:

| Site | Signature | Status |
|---|---|---|
| `Registry.Acquire` | `(*LiveInstance, ReleaseFunc, error)` | Converted |
| `Registry.Register` | `(ReleaseFunc, error)` | Converted |
| `Registry.makeRelease` | returns `ReleaseFunc` | Converted |
| `Registry.AcquireAll` | `([]*LiveInstance, ReleaseFunc, error)` | Converted (the internal `func() { for _, release := range releases { release() } }` literal returned here is an unnamed `func()` value being *returned as* `ReleaseFunc` — Go's assignability rule allows this without an explicit conversion, since a literal with an unnamed type is assignable to any named type sharing its underlying type; this is not a type error, and not a place where `ReleaseFunc` and `ForceReleaseFunc` could be confused) |
| `Registry.ForceRelease` | `func (r *Registry) ForceRelease(sessionID string)` — no closure returned, called directly | Deliberately unchanged, per finding B's own recommendation ("keeps its own distinct signature... rather than being retrofitted to return a `ForceReleaseFunc`") |
| `InstanceAcquirer` interface | `Acquire(sessionID string) (*LiveInstance, ReleaseFunc, error)` | Converted |
| `ReviewQueuePoller.releases` (Story 2.5.8) | `map[string]session.ReleaseFunc` | Converted |
| `daemon.go`'s `detectAndAddNewSessions` (Task 2.5.7f) | `releases *[]session.ReleaseFunc` | Converted |
| `daemon.go`'s `RunDaemon` seed load (Task 2.5.7i) | `var releases []session.ReleaseFunc` | Converted |
| `BuildRuntimeDeps` Step 5 (Task 2.5.5b) | `releases` slice typed by `Acquire`'s already-`ReleaseFunc` return | Converted (inferred, not restated, but correct) |

No consumer site retains a bare `func()` for release semantics. `ForceReleaseFunc` itself is
defined but — by design, confirmed in its own doc comment — never actually used as a return type
anywhere in the plan, since `ForceRelease` has exactly two callers, both calling it directly with a
session ID rather than through a stored closure. This means there is no place in the plan where a
`ForceReleaseFunc` value is assigned to a `ReleaseFunc`-typed variable or vice versa — the type
exists purely as a forward-guard for a wrapper that doesn't exist yet, exactly as its own doc
comment states, and I found no place that would fail to compile or that silently coerces one into
the other.

**One genuine, but cosmetic, gap found**: `plan.md` line 476 (Story 2.5.2's "Open question" note,
written before Story 2.5.3 introduces `ReleaseFunc`) still reads:

> "...does creation call `NewInstance` directly and hand the result to a new
> `Registry.Register(id string, live *LiveInstance) (func(), error)` (mirroring `Acquire`'s dedup
> check but skipping the storage lookup)? ... **Open question, not resolved by research**"

This is stale on two counts: (1) it uses the old bare `func()` return type instead of `ReleaseFunc`,
and (2) it frames the question as unresolved, when the same document resolves it authoritatively
three other places — Story 2.5.3's actual `Register` signature (`plan.md` line 602,
`(ReleaseFunc, error)`), Task 2.5.7h's implementation (line 1049), and the Open Decisions table's
entry #7 (line 2406, explicitly marked "**Resolved**"). An implementer reading the document in
order reaches the correct, typed signature at Story 2.5.3 (a few hundred lines after Story 2.5.2)
and again at Task 2.5.7h, and the Open Decisions table is the canonical place this kind of
cross-cutting resolution is supposed to be recorded — so this is very unlikely to mislead an
implementer in practice, but it is a real, findable inconsistency in the document's own internal
cross-references. **Recommended cleanup** (does not block implementation): update line 476 to
either read `(ReleaseFunc, error)` and note "resolved — see Open Decisions #7 and Task 2.5.7h,"
or delete the now-answered open question entirely and replace it with a forward pointer.

**Verdict: propagation is correct everywhere it matters for compilation and for avoiding the
`release()`-vs-`ForceRelease()` confusion finding B targets. One stale cross-reference in Story
2.5.2's prose, cosmetic only — flagged for cleanup, not blocking.**

---

## 4. Field-unexporting (`type-driven-audit.md` finding A, Task 7.1e)

**Claim under test**: is Task 7.1e concrete enough to execute, and does Story 5.4's updated note
create ambiguity about whether the CI guard is still required?

**Verified** (`plan.md` ~2323-2345): Task 7.1e names the mechanism concretely — lowercase every
mutable field (explicitly lists `Status`→`status`, `Title`→`title`, `AutoYes`→`autoYes`,
`GitHubPRURL`→`gitHubPRURL`, `GitHubPRNumber`→`gitHubPRNumber`, and directs enumerating the rest of
the ~90-field set via `go build ./...`'s resulting compile errors — the same "compiler enumerates
the list" approach Task 7.1b already establishes for `stateMutex` removal, so this isn't a new
technique, just the same one applied to field visibility). It explicitly keeps `ID`/`UUID`/
`CreatedAt` exported per R2.1, explicitly routes every write through the already-existing,
already-package-scoped `instanceState` wrapper (no new mechanism — `instanceState{inst *Instance}`
already exists from Story 3.1), and explicitly calls out that `daemon/daemon.go`'s `AutoYes` writes
become compile errors here, closing the fifth pass's non-blocking finding at the same time.
This is concrete and directly executable, not hand-wavy.

Story 5.4's updated note (`plan.md` ~2149-2158) is unambiguous, not ambiguous: it states the guard
is "superseded, but not yet replaced" by Task 7.1e, explains why it's still load-bearing in the
interim ("Epic 5... merges before Epic 7 does, so for the transitional period between the two,
this grep-based check is the only thing enforcing the invariant at all"), and gives an explicit
retirement condition ("only consider retiring it once Story 7.1e has landed and a full `go build
./...` pass confirms no direct-write call site survives without it"). No ambiguity about whether
the guard is currently required (yes, until Epic 7 lands) or about what triggers its retirement.

**Verdict: concrete and executable, no ambiguity in the CI-guard note.**

---

## 5. Finding C's tightened lint pattern (Story 4.5 Pattern 2, closure-escaping-via-callback)

**Claim under test**: is the tightened ast-grep pattern specific enough to implement, or hand-wavy?

**Verified** (`plan.md` ~1939-1957, ~1974-1980): the pattern is decomposed into concrete,
structurally-matchable sub-conditions, not a vague "catch closures that escape":

1. A closure literal, lexically nested inside a function whose signature takes `*instanceState`.
2. That closure's body references the outer `*instanceState` parameter (by name, whatever the
   Task 4.3c rename lands on, e.g. `actorState`).
3. That closure literal is passed as an argument to a call matching
   `Register\w*Callback\(|SetOn\w*Callback\(`.

All three are ast-grep-expressible: (1)/(3) are a structural match on "closure literal as call
argument, where the call's callee matches a name pattern" (ast-grep supports regex constraints on
identifiers), and (2) is a "does this subtree reference this identifier" check, which ast-grep's
metavariable-constraint matching handles directly (the same category of check the base Pattern 1
already relies on for "does this function body call `i.<Public>()`"). This is a harder rule to write
than Pattern 1, but it is not asking for anything ast-grep structurally cannot express — no
semantic/dataflow analysis (e.g. "which goroutine will execute this") is required, only syntax
tree shape, which is exactly the class of check ast-grep is built for.

The plan is also explicit that a capability-token type was considered and correctly rejected for
this hazard (the mistake occurs within the same package/file/function, so an unexported marker
type proves nothing about actual goroutine identity — any code with lexical access to the token type
can construct one) — this is the right call, not a cop-out, since the failure mode here is a
control-flow/reviewer-attention gap, not a data-provenance gap a type could close.

**Verdict: concretely specified, implementable with ast-grep as described, not hand-wavy.**

---

## 6. New problems introduced by this patch: none found

Checked specifically for:

- **A `ForceReleaseFunc` passed where a `ReleaseFunc` is expected, or vice versa, that wouldn't
  compile**: none found. `ForceReleaseFunc` is never used as a return type or parameter type
  anywhere in the plan — its only role is a documented, currently-theoretical guard for a future
  wrapper that doesn't exist yet. There is no site in the plan where the two named types are
  brought into contact with each other, so there is no incompatible-assignment scenario to trip
  over. (Verified this isn't merely "not found because I didn't look hard enough" — grepped every
  `ForceReleaseFunc`/`ForceRelease(` occurrence in the document; all `ForceRelease(` call sites take
  a bare `sessionID string` argument and return nothing, consistent with its unchanged signature.)
- **Task 7.1e's field-unexport conflicting with anything else scheduled in Epic 7 or elsewhere**:
  none found. Epic 4's `xxxLocked` internal twins operate on `*instanceState`/`s.inst.field`
  within the `session` package, so lowercasing fields doesn't break them — they already have
  in-package access regardless of export status. Epic 6 converts `instance_tmux.go`'s direct field
  reads to `Snapshot()` calls and its writes to `sendCtx`-routed commands *before* Epic 7 runs, so
  by the time fields are unexported, that file no longer touches raw fields directly. Story 5.4's
  CI guard and Task 7.1e are explicitly cross-referenced in both directions (5.4 notes it will be
  superseded; 7.1e notes it closes 5.4's daemon.go gap) rather than silently duplicating or
  contradicting each other.
- **Any other named-type conflation left over from finding B's fix**: none found beyond the
  cosmetic line-476 staleness already noted in §3.

---

## 7. Consistency checks

- **Task Summary arithmetic**: recomputed independently from the epic/story/task headers, not
  copied from the document's own summary row. Stories: Epic 1 (1.1, 1.2, 1.3 = 3) + Epic 2 (2.1-2.4
  = 4) + Epic 2.5 (2.5.1-2.5.10 = 10) + Epic 3 (3.1, 3.2 = 2) + Epic 4 (4.1-4.6 = 6) + Epic 5
  (5.1-5.4 = 4) + Epic 6 (6.1 = 1) + Epic 7 (7.1, 7.2 = 2) = **32**, matches. Tasks, counted per
  story's lettered sub-tasks: Epic 1 = 9 (3+4+2), Epic 2 = 8 (3+3+1+1), Epic 2.5 = 47
  (3+2+3+3+4+10+9+4+3+6), Epic 3 = 10 (5+5), Epic 4 = 16 (5+3+3+2+2+1), Epic 5 = 12 (3+3+4+2),
  Epic 6 = 4, Epic 7 = 8 (5+3, up from the fifth pass's 7 — Task 7.1e is the new addition). Total
  = 9+8+47+10+16+12+4+8 = **114**, matches the document's own stated "8 Epics, 32 Stories, 114
  Tasks" exactly.
- **`R2.x` traceability**: R2.1-R2.9, R2.11-R2.18, R2.18a all remain traceable to specific stories
  either by explicit citation or substance, consistent with the standard the fourth and fifth
  passes already established. R2.14's idle-timeout sub-clause remains uncovered by any task (noted
  by the fifth pass as a minor, non-blocking gap given ForceRelease/DeleteSession's force-invalidate
  already closes the higher-value case an idle-timeout would otherwise paper over) — still true,
  still non-blocking, not newly introduced by this pass.
- **No dangling references to the old bare-`func()` release signature**: one found (§3 above,
  `plan.md` line 476) — cosmetic, isolated (confirmed via grep that every other `Registry.Register(`
  mention in the document, lines 981/1049/2406, correctly uses `(ReleaseFunc, error)`), and does
  not affect what an implementer would actually build, since the authoritative signature is
  unambiguous everywhere else including the Open Decisions table's explicit resolution.

---

## Summary

All five `type-driven-audit.md` findings (A-E) and both fifth-pass blockers are correctly and
concretely resolved in the current `plan.md`:

| # | Item | Status |
|---|---|---|
| Fifth-pass finding 1 | select-race steady-state hang | **Closed** — priority pre-check present for `send`/`sendSync`/`sendSyncErr`(via delegation)/`sendCtx`; precedent verified against actual source |
| Fifth-pass finding 2 | Register/AddInstance rollback | **Closed** — `ForceRelease` (not `release()`) called synchronously, inline, ordering unambiguous, both recommended regression tests present |
| Audit finding A | Fields stay exported forever | **Closed** — Task 7.1e concrete, routes through existing `instanceState`, Story 5.4's redundancy note unambiguous |
| Audit finding B | `release()`/`ForceRelease()` conflation | **Closed** — `ReleaseFunc`/`ForceReleaseFunc` threaded through every consumer; one cosmetic stale cross-reference (line 476) noted for cleanup |
| Audit finding C | `xxxLocked` lint gap | **Closed** — tightened Pattern 2 concretely specified, implementable with ast-grep, capability-type rejection correctly reasoned |
| Audit finding D | `sendSync`'s ignorable error | Not a plan defect (language property); correctly deferred to finding 1's fix |
| Audit finding E | `WithInstance` interface reachability | Self-resolving via compile error; one-line clarification present in Task 2.5.5c |

No new blocking findings. The one item worth fixing before or during implementation is purely
cosmetic: update `plan.md` line 476's stale "open question" framing and bare-`func()` signature to
match the resolution already recorded at Story 2.5.3 / Task 2.5.7h / Open Decisions #7. This does
not gate Epic 2.5/Epic 3's implementation start.

**This plan is ready to implement.**
