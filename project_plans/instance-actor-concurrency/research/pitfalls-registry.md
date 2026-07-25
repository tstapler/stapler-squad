# Pitfalls — `Registry` + `LiveInstance` Refcounting (ADR-031)

Extends `research/pitfalls.md` with failure modes specific to R2.11-R2.17's
`Registry.Acquire`/`release()` design. Does not repeat that document's general
actor-model risks (self-deadlock, snapshot staleness, migration ordering, snapshot cost,
async test idioms) — those still apply unchanged to the actor each `LiveInstance` owns;
this file covers the new refcounted lifecycle layer in front of it. Citations to `main`
as of 2026-06-30.

---

## 1. Refcount leak: `Acquire()` without a matching `release()`

A forgotten `release()` (missing `defer`, early return before the `defer` registers, a
skipped panic path) means refcount never returns to zero, so per R2.14 the registry never
stops the actor or removes the map entry — `pitfalls.md` §1's goroutine leak, now
concentrated behind one API instead of scattered across the ~30 sites
`adversarial-review.md` §2/§3 catalogued (`workspace_service.go:76`,
`session_service.go:1626,1686`, `tools_lifecycle.go:332,385,459`,
`terminal_websocket.go:49`, `health.go:47,201`, `hibernation_sweeper.go:211`,
`daemon/daemon.go:292`, `loadInstancesWithWiring`'s ~10 callers).

**Centralization is ADR-031's whole point** ("removes ADR-030's remaining discipline
requirement entirely"), so the mitigation can't be "remember to release" — that's the
exact discipline failure (~123 unguarded `stateMutex` accesses, per `requirements.md`
background) this project is trying to stop depending on. A bespoke static analyzer
proving every `Acquire` has a reachable `release()` on every path (including panics) is
high effort for something `defer` already solves at the language level once the API stops
requiring callers to remember it.

**Recommended API: make the common case structurally unable to forget `release()`.**

```go
// Acquire — low-level primitive for callers holding a LiveInstance across
// multiple async steps (WebSocket stream, poller cache). release() is
// idempotent (§2) but not otherwise guarded against being forgotten.
func (r *Registry) Acquire(sessionID string) (*LiveInstance, func(), error)

// WithInstance — preferred entry point for synchronous, single-call-stack
// use (RPC handlers, one-shot poller lookups, MCP tools). release() is
// internal; there is no release() for the caller to forget.
func (r *Registry) WithInstance(ctx context.Context, sessionID string, fn func(*LiveInstance) error) error {
	inst, release, err := r.Acquire(sessionID)
	if err != nil {
		return err
	}
	defer release()
	return fn(inst)
}
```

Convert every single-lookup synchronous site (`findInstanceFast`, `HibernateSession`/
`ResumeHibernatedSession`, the three `tools_lifecycle.go` fallbacks, `health.go`'s sweep
body, `hibernation_sweeper.go`'s sweep, `daemon.go:292`'s per-title lookup) to
`WithInstance`. Reserve raw `Acquire`/`release()` for genuinely long-lived holders:
`terminal_websocket.go:49` (release on connection close), `ReviewQueuePoller`'s cache
(release on `RemoveInstance`), `AutonomousDriver`'s background goroutine (release on
driver stop — mirrors the existing symmetric `stopAndDeregisterDriver`,
`session_service.go:1754`).

**Defense in depth** for the remaining raw-`Acquire` sites: a CI grep check (same spirit
as `pitfalls.md` §4's unguarded-write check and this repo's `make registry-diff`) —
require a `release(` call in the same function body as any `\.Acquire\(`. Won't catch
every control-flow shape, but catches the common regression pre-merge. R2.14's
idle-timeout remains the second-order runtime safety net for whatever slips through.

---

## 2. Double-release: `release()` called twice for one `Acquire()`

Two calls to `release()` (duplicate `defer`, an explicit call on an error path plus the
deferred one, a `release` closure stored in a struct and invoked from two cleanup paths)
double-decrements the refcount. If another legitimate holder is still active, this can
drive the count to zero while that holder still believes its reference is good — the
registry stops the actor and removes the entry **out from under a live user**, the exact
premature-teardown hazard the design exists to prevent.

**`release()` must be idempotent — this project already has the pattern**, in
`session/instance_workspace.go:92-99` (Item 1's fix):

```go
unlocked := false
unlock := func() {
	if !unlocked {
		unlocked = true
		i.stateMutex.Unlock()
	}
}
defer unlock()
```

Apply the same shape (or `sync.Once`) to the registry's returned `release`:

```go
func (r *Registry) Acquire(sessionID string) (*LiveInstance, func(), error) {
	r.mu.Lock()
	e, ok := r.entries[sessionID]
	if !ok {
		inst, err := r.construct(sessionID)
		if err != nil {
			r.mu.Unlock()
			return nil, nil, err
		}
		e = &entry{instance: inst}
		r.entries[sessionID] = e
	}
	e.refcount++
	r.mu.Unlock()

	var once sync.Once
	release := func() { once.Do(func() { r.release(sessionID, e) }) }
	return e.instance, release, nil
}
```

`sync.Once` turns a double-call into a silent no-op instead of a double-decrement. Note
what it does *not* fix: two independently-obtained `release`s where one is invoked twice
under the mistaken belief it covers two acquisitions — that's really a §1/§3-shaped
accounting bug (an Acquire's worth of ownership "spent" twice), not a closure-idempotency
problem. A test-only wrapper asserting `acquireCount == releaseCount` at teardown (an
atomic counter in a test decorator) catches both shapes deterministically — see §5.

---

## 3. Acquire during teardown: race between the last `release()` and a new `Acquire()`

**The scenario the design must rule out:** refcount hits zero, registry begins teardown
(stop actor, remove map entry); concurrently a new caller calls `Acquire` for the same
session. Without synchronizing the *entire* decrement→zero-check→remove sequence, either
(a) the new `Acquire` returns a `*LiveInstance` whose actor is mid-`Stop()` (later sends
block forever or are dropped), or (b) it races the map `delete` and constructs a
**second** actor for the same session — literally the `daemon.go:292` duplicate-actor
hazard (`adversarial-review.md` §3b) ADR-031 exists to make impossible.

**Recommended (Design A): hold the registry mutex across decrement + zero-check + map
removal; run `Stop()` after releasing the lock, but block `release()` on its completion.**

```go
func (r *Registry) release(sessionID string, e *entry) {
	r.mu.Lock()
	e.refcount--
	if e.refcount > 0 {
		r.mu.Unlock()
		return
	}
	delete(r.entries, sessionID) // still locked: no concurrent Acquire can revive this entry
	r.mu.Unlock()

	// Runs outside the lock (no I/O under the lock — same discipline as
	// instance_checkpoint.go:33-36). Blocking here guarantees that once
	// release() returns, any new Acquire for this sessionID is guaranteed
	// to build a genuinely fresh, independent LiveInstance.
	e.instance.Stop()
}
```

Because decrement/zero-check/`delete` share one critical section with `Acquire`'s own
check-then-increment, mutual exclusion is automatic: a concurrent `Acquire` either runs
first (sees the entry, increments, `release()`'s zero-check then sees `refcount > 0` and
skips teardown) or `release()`'s section runs first (removes the entry; the concurrent
`Acquire` then finds nothing and builds an independent new one — correct, not a
duplicate).

**`Stop()` here must mean "cancel the actor's context, let `select` return" — fast, not
"clean up tmux/git"** (that's `Destroy()`, async, see §4). Blocking `release()` on a slow
`Stop()` would strain R2.13's "held for microseconds" budget for the lock's *scope* (note
`Stop()` itself runs outside the lock — only the map bookkeeping is inside it). Flag as a
decision to confirm during planning if `Stop()` later grows synchronous work.

**Alternative (Design B)**, if blocking `release()` is rejected: a `tearingDown`/`doneCh`
flag `Acquire` checks and retries against instead of relying on lock-held-across-Stop().
Recommend Design A as simpler (no extra fields, no retry loop); keep B on record only if
`Stop()` becomes slow enough that blocking every `release()` on it is measurable.

---

## 4. `DeleteSession` interaction — judgment call, needs confirmation

**Current behavior** (`session_service.go:1722-1817`, async-`Destroy` fix in `f316570b`
"make session destroy async so DeleteSession RPC returns immediately"):
`stopAndDeregisterDriver` → `removeFromAllPollers` → look up the live instance via
`FindLiveInstance`, fire a **detached goroutine** calling `Destroy()` (errors logged,
non-fatal, RPC does not wait) → cancel pending approvals → delete from storage
(synchronous — this is what the RPC waits on) → publish `SessionDeleted`. Driver/poller
deregistration is front-loaded before `Destroy()` so nothing calls back into a freed
instance; storage deletion completes before cleanup does, so the client sees success
without waiting on tmux/git teardown.

**Once this goes through `Registry`**, `DeleteSession` needs a `*LiveInstance` (via
`Acquire`) to call `Destroy()` on, and must decide what happens to the actor relative to
any *other* holder with an outstanding reference (e.g. `terminal_websocket.go`'s streaming
handler mid-connection, `ReviewQueuePoller`'s cache).

- **Option 1 — force teardown immediately**, regardless of refcount
  (`Registry.ForceRelease`). Matches "delete means gone now" and today's timing. But an
  in-flight RPC's `*LiveInstance` gets yanked mid-use — every command handler must
  tolerate "actor already stopped" instead of hanging, and it reintroduces the "is this
  handle still valid?" ambiguity R2.12's smart constructor was meant to remove.
- **Option 2 — deletion is just another `release()`**; teardown waits for the last
  holder. Storage deletion (client-visible success) stays immediate and synchronous, same
  as today; only the actor/map-entry lifecycle waits. Preserves "holding a pointer means
  the actor is alive" for everyone else. But a deleted session's tmux/git resources could
  stay alive as long as an unrelated long-poll client keeps a reference — a
  deleted-but-still-running session is a resource/security concern.

**Recommended hybrid, closest to current behavior**: run `Destroy()` immediately via a
short `Acquire`/use/`release()` inside `DeleteSession` — same fire-and-forget timing as
today, not gated on other holders — but leave the actor/map-entry lifecycle to ordinary
refcounting (Option 2's shape) so other holders' pointers stay structurally valid. Any
command another holder sends *after* `Destroy()` has run should fail gracefully (typed
error, not a hang) — same "encode the precondition in the command handler" principle
`pitfalls.md` §3 uses for TOCTOU-prone transitions.

**Flagged explicitly as a judgment call needing the requirements author's (Tyler's)
confirmation**: whether an in-flight RPC against a just-deleted session should be
forcibly invalidated (Option 1) or allowed to observe a "deleted, please disconnect"
state gracefully until it releases on its own (Option 2/hybrid) is a UX decision about
what "delete while someone else has an active RPC" should mean to a user — not resolvable
from the code alone.

**Aside, unrelated to Registry but noticed while reading this path**: `DeleteSession`
currently has three verbatim-duplicate `s.approvalStore.CancelSession(sessionUUID)`
blocks (`session_service.go:1783-1802`), apparently a copy-paste/merge artifact —
harmless (later calls are no-ops) but worth a one-line cleanup whenever this function is
touched for the Registry conversion.

---

## 5. Testing refcounting and the Acquire-during-teardown race deterministically

Extends `pitfalls.md` §6's patterns (done-channel+timeout, `require.Eventually`, explicit
sync channels instead of `time.Sleep`) to the registry's map/refcount state.

**Test 1 — refcount is shared, teardown only at zero:**

```go
func TestRegistry_RefcountSharedAcrossAcquires(t *testing.T) {
	r := NewRegistry(testDeps(t))

	inst1, release1, err := r.Acquire("session-a")
	require.NoError(t, err)
	inst2, release2, err := r.Acquire("session-a")
	require.NoError(t, err)
	require.Same(t, inst1, inst2) // one actor, not two

	release1()
	require.Never(t, func() bool {
		return r.entryCount("session-a") == 0 // second holder still active
	}, 100*time.Millisecond, 10*time.Millisecond)

	release2()
	require.Eventually(t, func() bool {
		return r.entryCount("session-a") == 0
	}, time.Second, time.Millisecond)
}
```

**Test 2 — Acquire-during-teardown, synchronized via an injected hook, not sleep:**

```go
func TestRegistry_AcquireDuringTeardown_NeverReusesADyingActor(t *testing.T) {
	stopStarted := make(chan struct{})
	stopProceed := make(chan struct{})
	r := NewRegistryForTest(testDeps(t), withStopHook(func(string) {
		close(stopStarted)
		<-stopProceed // hold Stop() open until the test allows it to finish
	}))

	inst1, release, err := r.Acquire("session-a")
	require.NoError(t, err)

	releaseDone := make(chan struct{})
	go func() { release(); close(releaseDone) }()
	<-stopStarted // teardown provably in flight

	acquireDone := make(chan struct{})
	var inst2 *LiveInstance
	go func() {
		var aerr error
		inst2, _, aerr = r.Acquire("session-a")
		require.NoError(t, aerr)
		close(acquireDone)
	}()

	select {
	case <-acquireDone:
		t.Fatal("Acquire returned while teardown was still in flight")
	case <-time.After(50 * time.Millisecond):
		// expected: still blocked/not yet observed removal
	}

	close(stopProceed)
	<-releaseDone

	select {
	case <-acquireDone:
	case <-time.After(time.Second):
		t.Fatal("Acquire never completed after teardown finished")
	}
	require.NotSame(t, inst1, inst2) // fresh, independent — no reuse, no duplication
}
```

`withStopHook` is a test-only injection point (same idiom as `pitfalls.md` §6's
flush/barrier helper) turning a timing-dependent race into a deterministic one. Add a
`runtime.NumGoroutine()` before/after check (per `pitfalls.md` §1) to catch a `Stop()`
that silently didn't run.

---

## 6. Does this obsolete the Epic 3 goleak plan? No — retarget it

`adversarial-review.md`'s Task 3.1d/3.1e/3.1f goleak test (`GitHubService.GetPRInfo`
before/after actor-spawn) targeted raw `FromInstanceData` construction and a raw
`Stop()`/`Destroy()` call. That review's own §2/§3 findings show this was already the
wrong level even before `Registry` existed: the real bugs were about *how many* raw
constructions happen per call (`LoadInstances()`'s N-1 sibling leak) and whether a
*second* live actor gets built for an already-tracked session (`daemon.go:292`) — both
call-site-discipline failures, which is exactly what R2.12's "no exported constructor
other than `Registry.Acquire`" removes as an option.

**Recommendation: retarget, don't discard.**

- Old target: construct N raw `Instance`s via `FromInstanceData` in a loop, assert
  `Stop()`/`Destroy()` on all of them returns goroutine count to baseline.
- New target: call `Registry.Acquire(sessionID)` N times for the **same** `sessionID`
  (modeling the ~10 callers sharing `loadInstancesWithWiring`), assert exactly **one**
  actor goroutine exists, `release()` all N times, assert goroutine count returns to
  baseline. Strictly stronger than the old test: N independently-constructed instances
  that all got cleaned up look identical, goroutine-count-wise, to one shared actor —
  only testing through `Acquire` can distinguish "no leak" from "no duplication."
- Keep a second scenario matching `adversarial-review.md` §2's own ask: seed the
  registry/storage with **≥2** sessions, `Acquire`/`release()` only session A repeatedly,
  assert session B's actor is never constructed — targets the sibling-leak bug at the
  `Registry` boundary instead of the old raw-storage boundary.
- `GitHubService.GetPRInfo` itself: post-`Registry`, its lookup must go through
  `Acquire`/`release()` too — per R2.16, *every* call site needing a `*LiveInstance`
  (even previously-Group-A read-mostly ones) goes through the registry. The test's shape
  (call `GetPRInfo`, assert goroutine count returns to baseline) stays valid; only what's
  being spawned/torn down underneath changes from "a bare `Instance` the call site must
  remember to `Stop()`" to "a registry-managed, refcounted entry the call site's
  `release()` decrements."

Net: the goleak work is still required, but its assertions must move from "did this one
construction clean up" to "did the registry maintain at-most-one-actor-per-session while
also cleaning up" — the latter is the property ADR-031 actually guarantees, and the
former cannot detect its violation.
