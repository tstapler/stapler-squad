# Architecture: Instance Actor + Atomic Snapshot Migration

Research/design only. No application files were modified to produce this document.

Grounded in: `session/instance.go` (Instance struct, `Start`/`start`), `session/instance_state.go`
(`transitionTo`, Is*/Get* accessors), `session/state_machine.go` (`TransitionDef`, guard/after
hooks), `session/instance_workspace.go` (`SwitchWorkspace`), `session/instance_hibernate.go`
(`hibernateProcess`, `resumeFromHibernation`), `session/instance_tmux.go` (RLock-across-I/O sites),
`session/instance_controller.go` (`StopController`), `session/instance_serialization.go`
(`ToInstanceData`), `server/adapters/instance_adapter.go` (`InstanceToProto`).

---

## 1. Concrete Go Types

### 1.1 Command type: closure-over-state, not discriminated struct-per-command

**Recommendation: `command{name string; fn func(s *instanceState)}` — a closure capturing its
own arguments and (if synchronous) response channel — not a `Command` interface with one
struct per command type.**

```go
// instanceState is the actor's private, single-goroutine-owned working set —
// effectively *Instance with stateMutex deleted.
type instanceState struct{ inst *Instance }

// command is the mailbox element. name is for logging/metrics only.
type command struct {
    name string
    fn   func(s *instanceState)
}
```

Rationale: with ~75 critical sections to migrate, a struct-per-command union requires a new
type *and* a switch-case per call site, living far from the code it replaces — effectively a
third registry to keep in sync (this repo already maintains two: OmnibarAction union and the
session-creation-mode 7-touchpoints list). A closure converts an existing
`stateMutex.Lock(); ...; Unlock()` body into `send(func(s){ ... })` almost mechanically — the
"case" *is* the call site, nothing else to register. The cost (no `cmd.Type()` for tracing) is
recovered by the `name` field. Reach for a discriminated union only if mailbox introspection
later proves necessary — not needed for this migration's scope.

### 1.2 `InstanceSnapshot`: full mutable-field mirror, not a narrow "what's read today" cut

**Recommendation: mirror essentially the whole mutable-field set of `Instance`, not just the
fields `ToInstanceData`/`InstanceToProto` happen to read today.**

Rationale: those two functions are the *unguarded* read paths the requirements flag as the
worst false-confidence case — proof that "read what's read today" is an unreliable, silently-drifting
boundary (new fields get added and read by new pollers without anyone updating an allowlist,
which is how the 75-site catalog happened in the first place). A full mirror costs one cheap
struct copy per command (microseconds, dwarfed by the tmux/git I/O the actor already does) and
needs only one function (`buildSnapshot`) kept in sync with the struct — the same function
that already has to change whenever a field is added to `Instance`.

```go
type InstanceSnapshot struct {
    ID, UUID, Title, Path, WorkingDir, Branch string
    CreatedAt, UpdatedAt                      time.Time
    Status                                    Status
    Program                                   string
    Height, Width                             int
    AutoYes, IsExpanded                       bool
    Prompt, InitialPrompt, Category           string
    SessionType                                SessionType
    TmuxPrefix, TmuxServerSocket               string
    Tags                                       []string // defensive copy, see note

    AutonomousMode                     bool
    AutonomousTurn, AutonomousMaxTurns int32
    AutonomousOutcome                  string

    // GitHub PR/URL integration + poller-populated PR status fields
    GitHubPRNumber                              int
    GitHubPRURL, GitHubOwner, GitHubRepo         string
    GitHubSourceRef, ClonedRepoPath, MainRepoPath string
    IsWorktree, GitHubIsFork, GitHubPRIsDraft     bool
    GitHubPRState, GitHubPRPriority               string
    GitHubApprovedCount, GitHubChangesReqCount    int
    GitHubCheckConclusion                         string
    GitHubPRStatusTerminal                        bool
    LastPRStatusCheck                             time.Time

    Checkpoints                    CheckpointList // copied, see note
    ActiveCheckpoint, ForkedFromID string
    OneShot, Hidden                bool
    ProjectID, HistoryFilePath, MCPServerURL string
    AppendSystemPrompt, AllowedTools, PermissionMode string
    LaunchCommand                  string
    RateLimitAutoResume            *bool // copy of pointee, see note
    PauseReason, WorkflowID        string
    EnvVars                        map[string]string // copied, see note
    CLIFlags                       string
    ArchivedAt                     *time.Time // copy of pointee

    ReviewState // embedded value type, copies by value

    InstanceType     InstanceType
    IsManaged        bool
    ExternalMetadata *ExternalInstanceMetadata
    Permissions      InstancePermissions
    Started          bool // renamed from unexported `started`
    Artifacts        *artifacts.SessionArtifactsBlob
}
```

**Excluded from the snapshot:** manager/dependency objects (`gitManager`, `vncManager`,
`cdpManager`, `processManager`, `controllerManager`, `tagManager`, `shellRepo`,
`historyDetector`) — behavior, not data; accessors needing them stay as mailbox round-trips or
call independently-synchronized sub-objects. Callback registrations (`lifecycleListeners`,
`onRateLimitDetected`, `onStatusChange`, etc.) stay actor-private, mutated only via commands.
`restartCount`/`recentRestartTimes` (internal to `trackRestartRate`, not read externally) stay
actor-private. `sessionGoal Locked[*SessionGoalData]` already has its own internal sync.

**Copy-semantics subtlety:** Go's default struct copy only copies slice/map *headers*. A
command handler that mutates `Tags`/`EnvVars`/`Checkpoints` in place after a snapshot already
aliases the backing array reintroduces exactly the race this migration removes. Rule:
handlers must replace slices/maps wholesale (`append` into a fresh backing array; build a new
map) rather than mutate in place; `buildSnapshot()` should still defensively copy as
insurance. Pointer fields (`ArchivedAt`, `RateLimitAutoResume`) must copy the *pointee* into a
fresh pointer, not the pointer value, or an old snapshot and the live actor alias the same
`time.Time`/`bool`. `ExternalMetadata`/`Artifacts` are acceptable as shallow pointer copies
only if every writer replaces the whole pointer rather than mutating through it — verify this
holds for `ArtifactExtractor` during migration.

### 1.3 The actor run loop

```go
type Instance struct {
    // ... unchanged identity/config fields ...
    mailbox  chan command
    snapshot atomic.Pointer[InstanceSnapshot]
    // stateMutex deadlock.RWMutex  <- REMOVED (R2.6)
}

func (i *Instance) runActor() {
    s := &instanceState{inst: i}
    for cmd := range i.mailbox {
        cmd.fn(s)
        i.snapshot.Store(buildSnapshot(s.inst)) // one snapshot per command (R2.3, R2.5)
    }
    // mailbox closed -> goroutine exits; snapshot pointer keeps its last value.
}
```

---

## 2. Synchronous request/response

```go
// send: fire-and-forget (MarkViewed, SetLastMeaningfulOutput, ...).
func (i *Instance) send(name string, fn func(s *instanceState)) {
    i.mailbox <- command{name: name, fn: fn}
}

// sendSync: blocks until the actor has processed the command, returns its result.
func sendSync[T any](i *Instance, name string, fn func(s *instanceState) T) T {
    reply := make(chan T, 1) // buffered: actor never blocks on an abandoned reader
    i.mailbox <- command{name: name, fn: func(s *instanceState) { reply <- fn(s) }}
    return <-reply
}

func sendSyncErr(i *Instance, name string, fn func(s *instanceState) error) error {
    return sendSync(i, name, fn)
}
```

`Pause() error` (today: `instance.go:972`, body under `stateMutex.Lock()`):

```go
func (i *Instance) Pause() error {
    return sendSyncErr(i, "Pause", func(s *instanceState) error {
        inst := s.inst
        if !inst.started { return fmt.Errorf("cannot pause instance that has not been started") }
        if inst.Status == Paused { return fmt.Errorf("instance is already paused") }
        inst.stopControllerLocked() // internal twin, NOT i.StopController() — see §3
        // ... git operations inline, same logic as today ...
        return inst.transitionTo(context.Background(), Paused)
    })
}
```

`GetEffectiveStatus() Status` (today: `instance_state.go:155`, mixed lock/no-lock branches) —
this is a pure read of already-published state plus a call into an independently-synchronized
manager, so it does **not** need a mailbox round-trip at all:

```go
func (i *Instance) GetEffectiveStatus() Status {
    mgr := i.GetStatusManager()
    if mgr == nil {
        return i.snapshot.Load().Status
    }
    statusInfo := mgr.GetStatus(i)
    if !statusInfo.IsControllerActive || statusInfo.ClaudeStatus == 0 {
        return i.snapshot.Load().Status
    }
    return StatusFromDetected(statusInfo.ClaudeStatus)
}
```

General rule: every former `RLock`-guarded one-liner getter (`IsActive`, `IsPaused`, `GetStatus`,
...) becomes a non-blocking `i.snapshot.Load().Field` read. Only methods that are
TOCTOU-sensitive (`Pause`, `Approve`, `Deny`, `SwitchWorkspace`) or that mutate and need the
outcome go through `sendSync`. Misclassifying a pure read as a mailbox round-trip adds needless
latency/contention on the single-consumer channel for zero correctness benefit.

---

## 3. Compound state-machine operations: safer by construction, with one new subtlety

`transitionTo`'s guard/after-hook pattern (`state_machine.go`) runs business logic while
holding `stateMutex.Lock()` today; under the actor model it runs inside the single actor
goroutine processing one command — **strictly safer and simpler**: there is no lock object to
mis-scope or forget, and every "protected by stateMutex" comment in the struct becomes
unconditionally true by construction.

**The hibernate/resume `go func()` workaround** (`state_machine.go:44-53`) exists because the
`After` hook runs with `stateMutex` held, and `hibernateProcess`/`resumeFromHibernation` need
to call methods (`i.Start(false)`, `i.stateMutex.Lock()` in the rollback path) that would
re-acquire the same non-reentrant lock on the same goroutine. **The actor model removes this
specific hazard** — there's no lock to re-enter, so this logic can be inlined directly into the
`After` closure.

**But a narrower version of the same bug class reappears**: if a command handler calls a
*public, mailbox-routed* method on its own instance (e.g. the migrated hibernate logic calling
the public `i.Start(false)`, which internally does `sendSyncErr`), the actor — being
single-threaded — cannot dequeue the `Start` command it just sent to itself until the current
closure returns. `sendSyncErr` blocks forever waiting for a reply only the same goroutine could
produce: a self-deadlock, same shape as the mutex case, manifesting as a blocked channel op
instead of a blocked mutex.

**Rule:** command closures must call internal, lock-free twins that operate directly on
`s.inst` (e.g. `startLocked(s, firstTimeSetup)`, `stopControllerLocked(s)`) — never the public
`sendSync`-wrapped API — for anything the closure needs the instance to do to itself. Every
public method that's also invoked internally during another command needs an internal
`xxxLocked(s *instanceState, ...)` twin holding the real logic, with the public method reduced
to `sendSync(i, "Xxx", func(s) T { return xxxLocked(s, ...) })` — exactly the shape used for
`Pause()` in §2.

I/O-heavy hibernate work (checkpoint write, tmux kill) can still run inline in the `After`
closure for correctness, but should be dispatched to a detached goroutine if it risks
head-of-line-blocking other queued commands (see §5) — that goroutine must re-enter only via a
normal external `send(...)` once it completes, never by calling back into the actor directly.

---

## 4. `SwitchWorkspace` → `Start()` reentrancy: structurally impossible, with one caveat

The confirmed deadlock (`instance_workspace.go:85-219`): `SwitchWorkspace` holds
`stateMutex.Lock()` for its whole body and, while holding it, calls `i.Start(false)` three
times (lines 148, 197, 206) — which tries to acquire the same lock.

Under the actor model this class of bug is **structurally impossible by default**:
`SwitchWorkspace` becomes one command closure; calling `startLocked(s, false)` (the internal
twin) inside it is an ordinary function call on the same goroutine — no lock, no channel send,
no deadlock. The §3 caveat applies identically: this only holds if the closure calls
`startLocked` rather than the public `i.Start(false)` (a self-`sendSync`, which hangs exactly
like the mutex bug does today). **The migration must explicitly rewrite the 3 call sites at
`instance_workspace.go:148,197,206` from `i.Start(false)` to `startLocked(s, false)` as part of
converting `SwitchWorkspace` — this is a deliberate, auditable step, not automatic.**

Mitigation to make the audit enforceable rather than a one-time manual check: keep internal
helpers as functions taking `*instanceState` (not methods on `*Instance`), so an `ast-grep`
lint rule matching `i.<PublicMethod>(...)` inside any function with an `s *instanceState`
parameter can catch a closure calling the public, mailbox-routed API on itself. Add this check
during Phase 6 (architecture review) of the migration — this exact invariant is what produced
the original Item 1 bug.

Item 1 (the immediate mutex-based unlock-before-`Start` fix) ships independently per the
requirements and is not superseded by this design. Its regression test (R1.3, timeout-bounded)
should be **kept and re-targeted** at the actor version post-migration — a hung `SwitchWorkspace`
call is exactly the symptom a self-`sendSync` mistake would produce.

---

## 5. Mailbox sizing / backpressure

**Recommendation: small buffered channel (16-32), blocking `send`/`sendSync` for ordinary
callers (RPC handlers), a context-bounded `sendCtx` for background pollers sweeping many
instances.**

```go
// sendCtx aborts if ctx expires before the mailbox accepts the command. Used
// by pollers iterating many instances so one stuck actor can't stall the sweep.
func (i *Instance) sendCtx(ctx context.Context, name string, fn func(s *instanceState)) error {
    select {
    case i.mailbox <- command{name: name, fn: fn}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

- **Buffered, not unbuffered:** absorbs ordinary bursts (concurrent RPCs touching the same
  instance) without serializing at the handoff; a `command` is a small struct + closure
  pointer, not a memory concern at this size.
- **RPC handlers block.** They already run on a per-request goroutine with no tighter deadline
  than the request's own context; blocking until the mailbox has room is the desired
  backpressure signal — a saturated actor should slow callers down, not accept unbounded queued
  work.
- **Pollers need a bounded escape hatch**, because the named anti-pattern (`instance_tmux.go`'s
  10 `RLock`-across-tmux-I/O sites, R2.8) means the actor will occasionally be slow for the
  duration of one command. `CapacityMonitor`/`ReviewQueuePoller`/`PRStatusPoller` should wrap
  each per-instance command in a short `context.WithTimeout` (e.g. 2s) when sweeping all
  sessions, log-and-skip on `DeadlineExceeded`, and move on — turning "one wedged tmux call
  hangs the whole poll cycle" into "one instance's data is briefly stale," which preserves
  R2.9 (UI reads come from the last-published snapshot regardless).
- **R2.8 still needs its own fix**: read the snapshot for the precondition *before* sending,
  perform tmux I/O outside the actor when it's a pure read; only route through a command when
  it must mutate state, accepting that this one command will be slow and leaning on `sendCtx`
  at poller call sites as the mitigation.
- **Not unbounded**: an unbounded mailbox would hide a wedged actor indefinitely (sends appear
  to "succeed" while effects never land) instead of surfacing backpressure. **Diagnostics:** log
  at Warn when a send blocks >1s waiting for mailbox room, including `command.name` — the signal
  that would have shortened the original "UI not loading" investigation to a log line.

---

## 6. Shutdown: no goroutine leak

Actor lifetime is tied to the `Instance`'s **removal from Storage**, not to status transitions
— a paused/hibernated `Instance` remains a live, addressable value (it can still receive
`Resume()`), so its actor must keep running.

```go
// Close stops the actor goroutine. Call exactly once, when Storage permanently
// removes the Instance (delete) — never on pause/hibernate.
func (i *Instance) Close() {
    close(i.mailbox) // runActor's `for cmd := range i.mailbox` exits after draining
}
```

- Storage must remove the instance from its live map/list (so no other goroutine — including
  pollers — can newly enqueue to it) **before** calling `Close()`. Go's `for range` on a closed
  channel drains any already-buffered commands before exiting, so in-flight work isn't dropped;
  only sends *after* close would panic, which the removal-then-close ordering prevents.
- `snapshot.Load()` keeps working after `Close()` — `atomic.Pointer` doesn't care whether the
  actor goroutine is alive, so an in-flight read (e.g. `ListSessions` racing a delete) sees the
  last valid state instead of a nil-pointer panic, matching today's de-facto behavior.
- Pause/Hibernate are ordinary commands; the actor goroutine keeps running, parked on an empty
  channel receive between them. That costs only a few KB of stack — negligible at the scale of
  concurrent sessions this app runs (tens, not millions); no park-and-respawn scheme needed.
- Process-wide shutdown needs no special handling — actor goroutines die with the process. A
  graceful `Storage.CloseAll(ctx)` (drain checkpoints before exit) is out of scope here (R2.9
  is about UI-visible behavior, not process shutdown).

---

## 7. Migration sequencing

**Recommendation: incremental, multi-PR migration that keeps `stateMutex` and the new
actor/snapshot running side by side during a transition period, removing the mutex only in the
final PR. Reject big-bang single-PR conversion.**

This is a live, daily-used service — the investigation itself started from "UI not loading."
A single PR touching 12+ files and ~75 critical sections risks a missed call site reintroducing
a silent race that `-race` may or may not catch depending on scheduling luck — the same failure
mode that produced the original unguarded-access catalog. It would also block Item 1 and any
unrelated `Instance` work for the duration (R1.4 already requires Item 1 ship independently).

1. **PR 0 (already separate): Item 1** — `SwitchWorkspace` unlock-before-`Start` fix, ships
   immediately under the existing mutex; establishes the regression test re-targeted in step 6.
2. **PR 1 — additive only:** add `InstanceSnapshot`/`atomic.Pointer`/`buildSnapshot()`; every
   existing mutator publishes a snapshot at the end of its still-mutex-held critical section.
   No reader migrated yet — pure addition, cannot regress anything.
3. **PR 2 — migrate the unguarded readers** (`InstanceToProto`, `ToInstanceData`,
   `CapacityMonitor`, `ReviewQueuePoller`/`PRStatusPoller` read paths, websocket handler) to
   `Load()`. Highest value, lowest risk: these take **no lock today**, so switching to `Load()`
   is a strict improvement independent of the write-side work. Re-run the pprof capture here —
   this alone should resolve the reader-pileup-behind-`StopController()` symptom.
4. **PR 3 — prove the actor plumbing** on one low-traffic write path with no guard/after hooks
   (`MarkViewed`/`MarkAcknowledged`/`SetLastMeaningfulOutput`), including `Close()`/shutdown.
   Validates `send`/`sendSync`/`instanceState` in production before betting the state machine on it.
5. **PR 4 — migrate the state-machine core together**: `transitionTo`, `Pause`, `Approve`,
   `Deny`, `StopController`, hibernate/resume (§3), `SwitchWorkspace` (§4). These call each
   other, so the internal/public (`xxxLocked`) split must land consistently across the whole
   cluster in one pass or the self-deadlock hazard reappears at the migrated/unmigrated
   boundary. Highest-risk PR — re-run the Item 1 regression test against the actor path, plus a
   manual pause/resume/hibernate/resume/program-switch pass (R2.9).
6. **PR 5(-7) — migrate `server/services/session_service.go`'s ~30+ call sites** and background
   goroutines (`AutonomousDriver` callbacks, `PRStatusPoller`'s `UpdatePRStatus`,
   `ReviewQueuePoller` writes), split by cluster (creation path / pause-resume-rename RPCs /
   PR-status+review-queue writers) so each is independently testable and revertable.
7. **PR 8 — migrate `instance_tmux.go`'s 10 RLock-across-I/O sites (R2.8)**, after the
   state-machine core (PR 4) is stable, since these depend on `Status`-check semantics PR 4
   changes the synchronization story for.
8. **PR 9 (final) — delete `stateMutex` (R2.6).** By now everything should route through the
   actor or `Load()`; this PR is mostly deletion, and the compiler finds anything missed — any
   remaining `i.stateMutex.X` fails to build once the field is gone, a forcing function more
   reliable than a manual 75-site audit. Run `go test -race ./session/... ./server/services/...`
   immediately before (mutex present, confirming PRs 1-8 introduced no regressions) and after
   (mutex gone, confirming removal itself is clean).

**Why this order:** snapshot-first (1-2) is independently shippable value and validates the
profiling methodology before the riskier write-side bet; proof-of-structure (3) precedes the
state-machine core (4) so the actor plumbing isn't being debugged at the same time as the
hardest reentrancy rule; `instance_tmux.go` (8) comes after the state machine because its
correctness depends on synchronization semantics PR 4 establishes; mutex removal strictly last
(9) turns "did we get every site" into a compile error instead of a manual audit.
