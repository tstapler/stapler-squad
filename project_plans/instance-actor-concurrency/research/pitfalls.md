# Pitfalls — Instance Actor Model + Atomic Snapshot Migration

Failure modes specific to migrating `session.Instance` (a live, currently-running
service's session object) from `stateMutex deadlock.RWMutex` (`session/instance.go:332`)
to one-actor-goroutine-per-Instance + `atomic.Pointer[InstanceSnapshot]`. Citations are
to `main`.

Framing fact: `Instance.stateMutex` is not `sync.RWMutex` — it's
`github.com/linkdata/deadlock` (`go.mod:22`, `session/instance.go:16,332`), a runtime
self-deadlock detector that logs a stack trace on timeout instead of hanging silently.
The actor model gives up this safety net implicitly: a self-send-and-wait on a channel
mailbox just blocks forever with nothing watching it. Pitfall 2 is the consequence.

---

## 1. Goroutine leak: one goroutine per Instance, no automatic teardown

If each `Instance` spawns an owning goroutine (`for { select { case cmd := <-mailbox: ... } }`)
at construction and nothing ever cancels it, the goroutine runs forever even after every
other reference to the `Instance` is dropped. Go's GC does not collect an object a live
goroutine still closes over — this leaks the whole `Instance` (tmux handles, ~120 fields),
not just a goroutine. Finalizers (`runtime.SetFinalizer`) do not fix this: a goroutine
referencing the object keeps it reachable, so the finalizer entry is itself
unreachable-but-pinned and never runs. Finalizers are GC-cycle-dependent and are a
last-resort leak *detector*, not a shutdown mechanism — wrong tool here.

**This is a real risk, not theoretical**: instances are dropped during the life of the
running server, not just at process exit — `server/services/session_service.go:1827,1832`
call `s.reviewQueuePoller.RemoveInstance(id)` / `s.historyLinker.RemoveInstance(id)` as
part of session deletion. Whatever owns the actor goroutine must be torn down at that
same call site, or the goroutine (and the Instance it closes over) outlives the deletion.

**Existing precedent to copy**: every long-lived background goroutine in `session/`
already uses context-cancellation + an explicit `Stop()`, not GC:
- `session/pr_status_poller.go:148` (`context.WithCancel`), `:157-160` (`Stop() { p.cancel() }`)
- `session/review_queue_poller.go:218,236-239` — same shape
- `server/services/capacity_monitor.go:86` — `case <-ctx.Done():` in its poll loop

**Recommendation**: give `Instance` a `context`/`cancel` pair created with the mailbox,
and an `Instance.Stop()` called from the same code path that already calls
`RemoveInstance` on the pollers above. The actor's `select` needs a `case <-ctx.Done(): return`
arm. Don't close the mailbox channel itself — it's multi-producer, and closing from the
consumer side is the bug-prone direction; `ctx.Done()` is the authoritative stop signal.
Add a regression test: create N instances, delete them via the chosen teardown path,
assert `runtime.NumGoroutine()` returns to baseline within an `Eventually` window — this
is invisible without a test, since a leaked actor just looks like normal background load.

---

## 2. Self-deadlock in the actor model is the same bug class, not a new one

`stateMutex` is non-reentrant: a goroutine holding `Lock()` deadlocks if it calls another
method that locks again. A single-consumer mailbox has the identical shape: if handling
command A requires sending another command to *the same mailbox* and blocking for the
reply, it deadlocks — the actor is the only reader and is currently blocked sending, not
reading. "Reentrant lock" maps losslessly onto "reentrant self-send-and-wait." Swapping
primitives relocates the hazard rather than removing it, and makes it *less visible*:
`linkdata/deadlock` actively detects the mutex version; nothing watches a stuck channel
send the same way.

**The confirmed bug (Item 1) is the canonical example.** `SwitchWorkspace`
(`instance_workspace.go:75-219`) holds `stateMutex.Lock()` (line 85) for its whole body
and, while holding it, calls `i.Start(false)` synchronously three times (148, 197, 206);
`Start()` re-acquires the same lock at `instance.go:900`. In actor terms: the
`SwitchWorkspace` command handler must not dispatch a `Start` command through its own
mailbox and block on the reply — it must call `Start`'s internal, lock-free logic
directly, in-process, as part of the same handler.

**This codebase has already hit this exact failure twice more, both fixed manually —
strong evidence this must be a designed-in invariant, not an after-the-fact patch:**

1. *`StartController`/`GetController()` (fixed)*. `wire_callbacks_concurrency_test.go:8-35`
   is a standing regression test: `wireRateLimitCallbacks` used to call `GetController()`
   (`i.stateMutex.RLock()`, `instance_controller.go:151-152`) while the caller already
   held `Lock()`. Fix (current code, `instance_controller.go:84-88`): pass the controller
   in directly instead of fetching via the locking getter. The test pattern — run in a
   goroutine, signal over a `done` channel, `select` against `time.After(1s)`, `t.Fatal`
   on timeout — is exactly the template for R1.3's regression test and generalizes
   directly to mailbox self-deadlocks.
2. *`CreateCheckpoint`/`tryExtractConversationUUID` (avoided by call-order restructuring)*.
   `instance_checkpoint.go:33-36`: *"Perform I/O-heavy adapter import BEFORE acquiring the
   write lock... calling it while we already hold stateMutex.Lock() would be a recursive
   non-reentrant lock acquisition and causes a deadlock detected by linkdata/deadlock."*
   Same fix shape `SwitchWorkspace` should have used: extract first, lock second.
3. *A live doc/code contradiction proving the discipline already drifts even when called
   out explicitly.* `instance_claude.go:291-293` claims `tryExtractConversationUUID`
   "must NOT be called without the lock (e.g., from SwitchWorkspace which holds
   stateMutex)" — but `instance_workspace.go:76-83` calls it **before** `stateMutex.Lock()`
   at line 85, the opposite of the comment, and matching the *correct* pattern from point
   2. The comment is stale; nothing catches that drift today. Don't carry
   doc-comment-only contracts ("caller must hold X") into the actor design — encode the
   precondition in the command/handler structure so it can't silently rot.

**Compound (locked-method-calls-locked-method) sites needing explicit design attention:**

| Operation | Site | Calls into (currently re-locking) |
|---|---|---|
| `SwitchWorkspace` | `instance_workspace.go:75-219` | `i.Start(false)` ×3 (148,197,206), `i.KillSession()` ×2 (142,180), `switchRevision`/`switchWorktree` (189,191) |
| `StartController` | `instance_controller.go:19-105` | `controller.Start()` (80) launches goroutines calling back into `i.transitionTo` via the EOF callback (61-67) |
| `CreateCheckpoint` | `instance_checkpoint.go:28-115` | `adapter.Import()`→`GetClaudeConversationUUID` (RLock), already extracted before the lock — preserve this ordering |
| `transitionTo` After-hooks | `state_machine.go:44-54` | `go i.hibernateProcess(ctx)` / `go i.resumeFromHibernation(ctx)` spawned specifically so heavy work runs after the lock is released — must become *new commands sent to the mailbox*, not blocking calls |
| `reconcileSessions` | `review_queue_poller.go:437-467` | `inst.transitionTo(...)` under a lock acquired by the *poller* — must become one command sent from the poller, not poller-side locking |
| `PRStatusPoller.applyPRUpdate` | `pr_status_poller.go:368-406` | reads `oldPriority` (RLock), calls `UpdatePRStatus` (separate Lock), compares outside any lock — see Pitfall 3 |

**Design rule**: split every `Instance` method into (a) unexported, lock-free internal
logic functions taking already-resolved data, and (b) command handlers, run only inside
the actor goroutine, that call those internal functions directly and never send-and-block
on their own mailbox. Public methods become thin: build a command, send it, wait on a
response channel embedded in the command — fine for external callers, fatal only when
the actor blocks on its *own* mailbox.

---

## 3. Ordering/staleness: snapshot reads are TOCTOU by construction

`atomic.Pointer[InstanceSnapshot].Load()` gives a consistent point-in-time view, but by
the time a reader acts on it, the actor may have applied a different command. Anywhere
current code does "check field X, then (outside the same lock) decide to write Y," that
becomes unsafe once reads and writes are decoupled — worse than today, because today's
inconsistent locking at least sometimes serializes read+decide+write by accident;
snapshot reads are explicitly designed to never block, so they're never atomic with a
following write.

**Existing case: `PRStatusPoller.applyPRUpdate`** (`pr_status_poller.go:368-406`):
reads `oldPriority` under RLock (384-386), unlocks, calls `inst.UpdatePRStatus(...)`
which takes its own separate Lock (388, writing 8 fields — `instance_terminal.go:247-258`),
then compares `priority != oldPriority` **outside any lock** (397) to decide whether to
fire `onUpdated`. A race window exists today (narrow, since one poller goroutine owns
this per instance in practice). Post-migration, if rewritten naively as "read snapshot,
send command, diff against the pre-send snapshot," the same staleness becomes an
*architectural* pattern other authors copy.

**Fix required as part of R2.5**: the `UpdatePRStatus` command handler must itself
determine "did priority change" — comparing against its own prior state immediately
before publishing the new snapshot — and return/act on that, rather than the caller
diffing two independently-fetched values taken before and after a write whose timing it
doesn't control.

**Existing case: check-then-transition patterns**, currently atomic only because the
check and transition share one lock acquisition:
- `instance_controller.go:61-67`: `if i.Status == Active { i.transitionTo(ctx, Stopped) }`
- `review_queue_poller.go:442-449,457-463` (`reconcileSessions`): `if inst.Status == Active {...}` / `if inst.Status == Stopped {...}`, called by an *external poller goroutine*

If the actor exposes `GetStatus()` via snapshot and lets callers decide before sending a
transition command, every one of these becomes a TOCTOU bug (poller observes `Active`,
sends "transition to Stopped," but the real state already changed by the time the actor
processes it).

**The actual fix**: a command must encode its own precondition and no-op (or return a
typed failure) if the precondition doesn't hold *when the actor processes it* — never
"transition to Stopped unconditionally" sent by a caller that merely observed `Active` a
moment earlier. E.g. `TransitionCommand{From: Active, To: Stopped}`, checked by the
handler against current state before applying — mirroring what `transitionIndex` already
does (`instance_state.go:32-47`) and what `TransitionDef.Guard` already encodes
(`state_machine.go:29-31`). That existing guard+mutation-together structure is already
close to the right shape for a command handler; it just runs under a mutex today instead
of inside an actor loop iteration.

---

## 4. Migration-in-flight: can writers convert incrementally, file by file?

R2.2 requires that no goroutine mutate `Instance` fields directly after construction —
this must hold for **all** writers of a given `Instance` simultaneously, since there is
exactly one `Instance` type shared process-wide. A half-converted state where the actor
owns the mailbox/snapshot *and* `session_service.go` still does `inst.GitHubPRURL = prURL`
directly (confirmed today, no lock at all: `session_service.go:3436-3439`) is **strictly
worse than the current state**, not a safe intermediate step:
- Today that unguarded write races against other readers — bad, but no actor exists yet
  to silently discard it.
- Once the actor exists, that same write races against the actor's *next snapshot
  publish*, which rebuilds the snapshot from its own (stale, pre-direct-write) in-memory
  state and **clobbers** the direct write the instant any other command is processed —
  deterministic, guaranteed silent data loss, not a probabilistic race.

**This rules out incremental file-by-file conversion for the write path.** Converting
`session/instance*.go` to route through the actor while `session_service.go`'s ~28
direct write sites (plus `autonomous_driver.go`'s background-goroutine writes and
`pr_status_poller.go`'s writes, e.g. `instance_terminal.go:247-258`,
`pr_status_poller.go:301-303,328-330`) convert later guarantees the clobber above for
every field touched by both sides. Practical sequencing:
1. Land the actor/mailbox/snapshot machinery inert (not yet spawning the goroutine, not
   yet the load-bearing read path) until every known writer is converted.
2. Convert all writers (session_service.go's ~28 sites, autonomous_driver.go,
   pr_status_poller.go) to commands in the same PR/commit (or single feature flag) that
   flips the actor on — not per-file.
3. Only after that flip should `Instance` construction spawn the actor goroutine and the
   snapshot pointer become authoritative.

**Reads can migrate incrementally — exploit this asymmetry.** Read sites
(`InstanceToProto`, `ToInstanceData`, `CapacityMonitor.evaluate` reading `inst.Status`
directly today with zero locking at `capacity_monitor.go:145`, `ReviewQueuePoller`,
`ConnectRPCWebSocketHandler`, `PRStatusPoller`) can convert to `snapshot.Load()` one at a
time, before the write cutover, because a stale/torn read today is already the status quo
for several of these — switching to `Load()` only improves consistency (no torn reads)
and regresses nothing, since nothing yet depends on the snapshot being authoritative.
Writes-lost-to-clobbering is specifically a *write* hazard; a read that's one generation
behind is merely stale, not lossy.

Sequence: (a) build actor+snapshot inert, (b) migrate reads to `Load()` incrementally,
each its own PR — independently safe — (c) migrate every write site in one atomic cutover
that also flips the actor live, (d) remove `stateMutex` only after (c) lands and
`go test -race` confirms it. A CI grep check ("no `inst\.\w+\s*=` / `i\.\w+\s*=` outside
`session/instance*.go` remains in `server/services/`") would catch a forgotten call site
before merge — same spirit as this repo's existing registry-coverage CI checks.

---

## 5. Performance: is rebuilding the snapshot on every command a problem?

`Instance` (`instance.go:92-373`) has ~90+ declared fields plus the embedded
`ReviewState`'s 9 more (`review_state.go:35-81`) — mostly `string`/`bool`/`int`/`time.Time`
(24 bytes), a few slices/maps. A reasonable `InstanceSnapshot` (excluding `stateMutex`,
`pmMu`, and internal manager pointers like `gitManager`/`vncManager`/`cdpManager`/
`tagManager`, which stay on the live `Instance`) is plausibly 400-800 bytes of
header/scalar data per snapshot — slice/map/string backing arrays are not copied, only
their headers, *as long as the actor always assigns a new slice/map rather than mutating
one in place after publishing a snapshot that already references it* (e.g. mutating
`i.Tags[0] = "x"` post-publish would retroactively corrupt an already-published,
supposedly-immutable snapshot — a real implementation detail, not free by default).

**This is almost certainly fine for the workload.** Command frequency is bounded by
human/LLM-paced RPCs (pause/resume/rename/program-switch — at most a few/sec across the
whole fleet) and poller ticks (`PRStatusPoller`/`ReviewQueuePoller` on 2-8s/60s intervals;
even `MarkViewed()`/`MarkUserResponded()`, `instance_state.go:108-121`, fire on user
interaction, not in a hot loop). A ~500-800 byte allocation at, generously, tens of
commands/sec process-wide is noise against Go's young-gen GC throughput — don't
pre-optimize. This matches R2.7's own reasoning for rejecting a lock-free queue
("revisit only if profiling shows it's hot"); apply the same standard here — ship the
full-struct-copy snapshot, pull a field into an independent atomic only if `go tool
pprof` (already used operationally per `.claude/docs/profiling.md`) shows it.

**Precedent for the "pull it out" escape hatch, used exactly once today**:
`ReviewState.lastMeaningfulOutputNs` (`review_state.go:77-80,86-96`) is already a
lock-free `atomic.Int64` shadow of `LastMeaningfulOutput`, written under `stateMutex` but
read via `atomic.LoadInt64`. This is the right model for "if profiling shows a problem,
here's the one-off fix" — but it has its own cost (keeping two representations in sync
via `SyncAtomicTimestamps()`, `review_state.go:83-90` — exactly the
two-representations-of-one-fact bug surface this migration is trying to eliminate
elsewhere). Don't generalize it to all 120+ fields without a profiling reason.

---

## 6. Testing an asynchronous actor without sleep-based polling

**Pattern 1 — deadlock-by-timeout, for self-deadlock regressions (R1.3).**
`wire_callbacks_concurrency_test.go:12-35` (`TestWireRateLimitCallbacks_NoDeadlock`): run
the operation in a goroutine, signal over `done := make(chan bool)`, then
`select { case <-done: ; case <-time.After(1*time.Second): t.Fatal(...) }`. Fails fast
instead of hanging the suite — matters because `go test -race` doesn't catch deadlocks,
only data races. Generalizes directly to mailbox self-deadlocks (Pitfall 2).

**Pattern 2 — `require.Eventually`, for "command sent, wait for snapshot to update."**
Already this repo's idiom (`server/services/capacity_monitor_test.go:219-221`):
```go
require.Eventually(t, func() bool {
    return switcher.GetTarget("test-session-auto") == "agy"
}, 2*time.Second, 10*time.Millisecond, "expected switcher target ...")
```
Actor-model equivalent:
```go
inst.SendCommand(MarkViewedCommand{})
require.Eventually(t, func() bool {
    return !inst.Snapshot().LastViewed.IsZero()
}, time.Second, time.Millisecond)
```
Use a tight poll interval (1ms) — the actor applies commands essentially immediately, so
`Eventually` here is confirming ordering, not waiting out real latency.

**Pattern 3 — `sync.WaitGroup` convergence, valid only for today's synchronous-call model.**
`instance_concurrency_test.go:54-98` (`TestTransitionTo_ConcurrentPause/Approve/Mixed`)
launches N goroutines calling `transitionTo`/`Approve`/`Deny` directly, `wg.Wait()`s, then
asserts on `inst.Status` — works only because these are synchronous, lock-blocking calls
today. Post-migration, if these send mailbox commands, `wg.Wait()` only confirms "sent,"
not "applied," and the post-wait assertions become flaky. Rewrite using either (a) a
synchronous request-response command (a `chan error`/`chan *InstanceSnapshot` in the
command struct, blocking the *caller* — fine, only actor-internal self-sends are
hazardous per Pitfall 2), or (b) wrap in `require.Eventually` per pattern 2.

**A flush/barrier test helper is worth adding.** Because the mailbox is single-consumer
FIFO, `inst.testFlush()` — send a no-op sentinel command, block on its ack channel —
gives an exact "everything sent so far has been applied" barrier without per-test
polling. This is the actor-model equivalent of the explicit `Lock()`/`Unlock()` pairs
used as synchronization barriers today (`instance_concurrency_test.go:76-78`). Most
request-response commands get this for free; the helper matters for fire-and-forget
commands like `MarkViewed()` that return nothing today.

**Net strategy**: self-deadlock → done-channel+timeout (1); async visibility →
`Eventually` on `Snapshot()` or `testFlush()` (2); request-response commands → no new
pattern needed, the response channel is the sync point. Never reintroduce
`time.Sleep(N)`-based waits — this project's e2e conventions already ban
`waitForTimeout` in Playwright specs for the same flakiness reason (per `CLAUDE.md`);
extend that discipline to the new Go-level async tests.
