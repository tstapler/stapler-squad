# Pitfall Research: `Registry.Register` rollback on `storage.AddInstance` failure

**Scope**: adversarial-review.md (fifth pass) finding 2 — `Register()` succeeds, then
`storage.AddInstance` fails, leaving a live, registered `LiveInstance` with no storage row.
Answers the four questions posed; research only, no plan/code changes made.

---

## 1. Confirmed: two-phase construct/confirm-or-abort is correct; reordering back is not an option

Reordering `Register()` back to *after* `storage.AddInstance` reopens the original
sweep race the ordering fix (Task 2.5.7h, `plan.md` ~994-1004) closes: any of
`health.go`/`hibernation_sweeper.go`/`daemon.go`, all converted to `Registry.AcquireAll()`
in this same epic, could observe a persisted-but-not-yet-registered row and construct a
second, independent `LiveInstance` for the same tmux session — the exact "two actors race
one tmux target" defect `requirements.md` R2.10's background describes. Reordering is a
regression, not a fix. The two failure modes are genuinely symmetric and both must be
closed at once:

| Order | Failure | Consequence |
|---|---|---|
| `AddInstance` → `Register` | window between persist and register | sweep finds persisted-but-unregistered row, double-constructs |
| `Register` → `AddInstance`, `AddInstance` fails, no rollback | `Register` succeeded, `AddInstance` failed | phantom registered entry, no storage row |

The correct shape is **construct → confirm-or-abort**, i.e. `Register()` optimistically
inserts (closing the sweep race), and the caller (`CreateSession`) must explicitly undo
that insertion if the subsequent step fails — precisely a DB transaction's
commit/rollback, applied to an in-memory map instead of a DB. This is not a novel
mechanism for this codebase:

- `session/instance.go:756-767` (`start`'s internal helper) already applies exactly this
  shape for a different resource: `setupFirstTimeWorktree` succeeds, then a later step in
  the same function fails, and a deferred block tears down what was already brought up
  (`i.Kill()`) and neutralizes the caller's handle (`*cleanup = func() error { return nil }`)
  so a double-cleanup can't happen. Construct-then-abort-on-later-failure is standing
  practice for this exact function.
- `server/services/session_service.go:2500-2503` (`RenameSession`) is the closer analog:
  `instance.Rename(newTitle)` mutates in-memory state first, `storage.SaveInstances`
  fails second, and the fix is a one-line revert of the in-memory mutation
  (`instance.Title = oldTitle`) before returning the error. `Register`/`AddInstance` is
  the same shape one level up: "in-memory mutation succeeded, persistence failed, revert
  the in-memory mutation" — only the in-memory mutation here is a registry map insertion
  (plus a spawned actor) instead of a struct field write.

So the fix is exactly the pattern the adversarial review's own §2 write-up sketches:
`CreateSession` calls `Register(instance)` → attempts `storage.AddInstance(instance)` →
on failure, tears the registration down before returning the error. No new abstraction is
required; this is `RenameSession`'s rollback-on-save-failure idiom applied to `Registry`.

---

## 2. `ForceRelease` is the right primitive — but the released closure (`makeRelease`) is not

`ForceRelease` (`plan.md` ~620-631, Story 2.5.3) already exists for exactly this shape of
problem: "tear down this registry entry and its actor immediately, unconditionally,
regardless of who else might reference it" — that is its documented contract for
`DeleteSession`'s force-invalidate (R2.18, Story 2.5.9). Reusing it for `Register`'s abort
path means the fix is "on `storage.AddInstance` failure, call
`registry.ForceRelease(instance.GetStableID())`" — no new API surface.

**But there is a sharp, non-obvious reason it must be `ForceRelease` and *not* the
`release()` closure `Register` itself returns** (`plan.md` line 583:
`return r.makeRelease(sessionID), nil`):

`makeRelease`'s underlying `release()` (`plan.md` ~605-618) only tears down the entry when
**refcount reaches zero** — it decrements and checks. `Register` inserts at refcount 1
(`plan.md` line 581: `&registryEntry{instance: instance, refcount: 1}`). In the *overwhelming
majority* of cases, calling the returned `release()` on an `AddInstance` failure correctly
decrements 1→0 and tears down. But `Registry.Acquire` (`plan.md` ~531-559) checks
`r.entries[sessionID]` **first, before any storage lookup** — so if any other goroutine calls
`Acquire(sameSessionID)` in the narrow window between `Register()` succeeding and
`CreateSession`'s failure-path cleanup running (e.g. a client retry using a
client-supplied/predictable ID, or a second RPC racing on the same ID due to a caller bug
elsewhere), that `Acquire` will find the phantom entry in the map, bump its refcount to 2,
and hand back a live `*LiveInstance` for a session with no storage row — before
`CreateSession`'s cleanup path runs at all.

If `CreateSession`'s cleanup then calls the plain `release()` closure, it decrements 2→1
and **does not tear down** — the exact phantom-entry bug this whole fix exists to close,
now recurring *inside the fix itself*, just gated behind a narrower, refcount-based race
instead of the original ordering race. `ForceRelease` sidesteps this entirely: it deletes
the map entry and stops the actor **unconditionally**, regardless of current refcount. Any
holder that raced in ahead of the cleanup (the concurrent `Acquire` above) is left with a
`*LiveInstance` whose actor is now stopped — its next `send`/`sendSync` gets the same typed
`ErrInstanceStopped` any other force-released holder gets (Story 2.5.9c's contract). That is
exactly the correct outcome for "this registration was aborted, no one should have gotten a
handle to it, but if someone raced in, they get a clean typed error, not silent corruption."

**Conclusion**: reuse `ForceRelease`, called directly with the session ID — do not use the
`release()`/`func()` that `Register`'s own return value provides for this purpose. This
is a one-line but easy-to-get-subtly-wrong distinction worth calling out explicitly in
Task 2.5.7h's text, since "just call the returned release()" is the more obvious-looking
(and the adversarial review's own phrasing literally offers both: *"call the `release()`
(or `ForceRelease`)"*) but occasionally-wrong choice.

### Does `ForceRelease`'s existing semantics/naming fit an "abort a never-confirmed
registration" case, or does clarity call for a distinctly-named method?

`ForceRelease`'s current doc comment (`plan.md` ~620-623) is written entirely in terms of
"tear down a live, fully-in-service session regardless of other holders" (`DeleteSession`'s
use case). Using it verbatim for "this registration was never confirmed and must be undone"
is implementation-correct (identical map-delete + `stopActor()` body) but reads oddly at the
call site — a reviewer seeing `registry.ForceRelease(id)` in `CreateSession`'s error path
may reasonably ask "wait, are we deleting a session here?" The two call sites have different
*intent* even though they need the same *mechanism*:

- `DeleteSession`: a real, previously-confirmed, possibly-multiply-held session is being
  destroyed on purpose, right now, by user action.
  `Register`'s abort path: a registration that was never confirmed (no storage row ever
  existed) is being undone because its *own* confirmation step failed — there is no
  "session" from any other caller's perspective; nothing was ever supposed to be visible.

Given this plan already tolerates one asymmetry note for the same reason (`onConstruct=nil`
for daemon's `Registry`, documented as "deliberate, process-boundary-driven asymmetry, not
an oversight" — `plan.md` ~1029-1032), the same treatment fits here. Two acceptable options,
in order of preference:

1. **(Recommended, smaller diff)** Keep `ForceRelease` as the sole mechanism, call it
   directly from `CreateSession`'s failure path, and add one sentence to `ForceRelease`'s
   doc comment noting the second caller and why the same unconditional-teardown semantics
   apply: *"Also used by `CreateSession` to abort a `Register()`'d entry when the
   immediately-following `storage.AddInstance` fails — the entry was never confirmed, so an
   unconditional teardown (not a refcount-gated `release()`) is required to correctly
   handle a concurrent `Acquire` racing in before the abort runs (see Task 2.5.7h)."* This
   avoids introducing a second method with byte-identical bodies purely for naming clarity,
   consistent with this plan's existing preference for reusing narrow mechanisms
   (`InstanceAcquirer`/`RegistryInspector` interfaces, `WithInstance` sugar) over new API
   surface.
2. **(Acceptable, if reviewers prefer call-site clarity over method-count minimalism)** Add
   a trivial one-line wrapper, e.g. `func (r *Registry) AbortRegistration(sessionID string) {
   r.ForceRelease(sessionID) }`, purely for self-documenting call sites — no behavioral
   difference, just a name. Only worth it if `code-review`/team convention weighs call-site
   readability over minimizing `Registry`'s public surface; not required for correctness.

Either way, **no new teardown mechanism is needed** — the existing `ForceRelease` body is
sufficient and already tested (Story 2.5.9c's regression test already proves "other holder's
next command gets a typed error, not a hang" for the identical mechanism).

---

## 3. Synchronous rollback is required; no async-cleanup shortcut exists

The rollback must complete, and be observably complete (map entry gone, actor's `stopActor()`
returned), **before** `CreateSession` returns its error to the RPC caller. `ForceRelease`'s
own body (`plan.md` ~624-631) is already synchronous in exactly this sense: it deletes the
map entry under `r.mu` and then calls `e.instance.stopActor()` — and per Story 2.5.3's own
acceptance criteria (`plan.md` ~664-668), `stopActor()` **must block until the actor's run
loop has actually exited**, not merely call `cancel()` and return (this is the same
ADR-029/finding-3 reconciliation the release-path already depends on). So calling
`ForceRelease` synchronously inline in `CreateSession`'s error path already gets a
fully-synchronous guarantee for free — no extra work needed to make it block.

An async cleanup (e.g. `go registry.ForceRelease(id)` before returning the error) would
reopen a **narrower version of the exact bug being fixed**: between `CreateSession`'s error
return and the detached goroutine's `ForceRelease` actually running, the phantom entry is
still live in `r.entries`, so any `Acquire(sameID)` landing in that window still gets a
handle to an unpersisted, about-to-be-torn-down session — the same class of "consistency
violation" finding 2 exists to close, just with a smaller time window instead of an
unbounded one. A smaller window is not a fix; the whole point (per the adversarial review's
own framing — "a later `Acquire()`... would find and return this phantom... handle") is
that no window should exist at all once `CreateSession`'s error is observable by any other
goroutine. There is no legitimate async-cleanup variant here: the map mutation and the
error return must be strictly ordered from every other goroutine's perspective, which means
synchronous, inline, same-goroutine cleanup before `return`.

---

## 4. Recommended code shape

### `CreateSession` (`server/services/session_service.go`, replacing the block at ~1249-1259)

```go
// Create instance using NewInstance constructor
instance, err := session.NewInstance(instanceOpts)
if err != nil {
    return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("failed to create instance: %w", err))
}

// Register before persisting (adversarial-review.md finding 5 / Task 2.5.7h): closes the
// sweep race where a converted AcquireAll() caller could observe a persisted-but-
// unregistered row. release/registerErr handling below closes the adjacent race this
// ordering opens (finding 2): Register succeeding but AddInstance subsequently failing.
if _, err := s.registry.Register(instance); err != nil {
    // Should not happen given fresh UUIDs, but handle per R2.18a's erroring semantics
    // (Task 2.5.7h's collision defense-in-depth) rather than leaving it silent.
    instance.stopActor()
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to register instance: %w", err))
}

// Save the instance to storage with Creating status immediately so the client
// can receive the session and show a spinner while initialization proceeds.
if err := s.storage.AddInstance(instance); err != nil {
    // Register() succeeded but persistence failed: abort the registration synchronously,
    // inline, before returning — NOT the release() Register returned (that closure only
    // tears down at refcount 0; a concurrent Acquire racing in ahead of this cleanup could
    // have already bumped the refcount to 2, in which case release() would silently no-op
    // and leave the phantom, unpersisted entry live). ForceRelease deletes unconditionally
    // and blocks until stopActor() has fully returned, so no Acquire after this call can
    // ever observe the aborted registration.
    s.registry.ForceRelease(instance.GetStableID())
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
}
```

Notes on this shape:
- `Register`'s own returned `release()`/first `func()` return value is **discarded** (`_`)
  on the collision-error path, and simply never invoked on the `AddInstance`-failure path
  either — `ForceRelease` is called directly with the session ID both times cleanup is
  needed, exactly because it's the unconditional primitive. (On the *success* path,
  `Register`'s returned `release()` presumably still needs to be retained/threaded
  somewhere for the session's later natural teardown — that's an existing open question for
  Task 2.5.7h's broader integration, not part of this rollback fix specifically.)
- `instance.stopActor()` on the `ErrSessionAlreadyRegistered` branch (already specified by
  Task 2.5.7h, `plan.md` ~1009-1011) is unchanged by this finding — that instance was never
  registered at all, so there's no map entry to abort; only the actor needs stopping.

### `Registry.ForceRelease` — doc comment addition only, no body change

```go
// ForceRelease tears down sessionID's actor/map-entry immediately, regardless of refcount
// (R2.18 — DeleteSession's force-invalidate, Story 2.5.9). Other holders' *LiveInstance
// pointers stay valid Go values; their next send()/sendSync() must return a typed error
// (Story 2.5.9), never hang.
//
// Also used by CreateSession (Task 2.5.7h) to abort a Register()'d entry when the
// immediately-following storage.AddInstance fails: the registration was never confirmed,
// so an unconditional teardown is required rather than the refcount-gated release()
// Register itself returns — a concurrent Acquire racing in between Register succeeding
// and this abort running would otherwise leave a phantom, unpersisted entry alive at
// refcount 1 after a plain release() only decremented it from 2.
func (r *Registry) ForceRelease(sessionID string) { ... } // body unchanged
```

### Unit test additions (Task 2.5.7h's existing "unit test" bullet, extended)

1. **Rollback-on-AddInstance-failure**: fake `Storage.AddInstance` to return an error in a
   simulated `CreateSession` flow; assert (a) `registry.Count()` is unchanged from before
   the call (or a subsequent `Acquire` for the same ID returns `ErrSessionNotFound`, not the
   phantom instance), and (b) the instance's actor has fully stopped (e.g. via a
   call-counting/goroutine-count fake), not just "map entry gone."
2. **Concurrent-Acquire-during-abort race** (directly targets the §2 distinction above): use
   a test-only hook to pause `CreateSession`'s error path after `Register()` succeeds but
   before `ForceRelease` runs; from a second goroutine, call `registry.Acquire(sameID)`
   (bumping refcount to 2); let the paused `CreateSession` path proceed and call
   `ForceRelease`; assert the entry is gone (`registry.Count()` back to baseline) and the
   second goroutine's held `*LiveInstance`'s next `send`/`sendSync` returns
   `ErrInstanceStopped` — proving `ForceRelease`, not `release()`, is what makes this
   interleaving safe. This test would fail (entry stays alive at refcount 1) if the fix used
   `Register`'s returned `release()` instead of `ForceRelease`, making it the regression test
   that specifically catches the wrong-but-plausible implementation choice.

---

## Summary of recommendation for Task 2.5.7h's text

Add one clause: *"If `storage.AddInstance` fails after `Register()` succeeded,
`CreateSession` must call `registry.ForceRelease(instance.GetStableID())` — not the
`release()` closure `Register` returned — before returning its error, so the registration is
undone unconditionally regardless of any concurrent `Acquire` that may have raced in ahead of
the cleanup. Extend `ForceRelease`'s doc comment to note this second caller. Add a unit test
forcing this failure path (registry entry does not survive it) and a second test proving the
`ForceRelease`-vs-`release()` distinction matters under a concurrent `Acquire`."* No new
`Registry` method, no redesign — this is a narrow, mechanical addition matching the
"two new blockers are narrow, mechanical additions to already-mostly-correct task text"
framing the adversarial review itself already uses for finding 4's fix.
