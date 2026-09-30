# Research: Architecture — async-session-creation

## 0. Adjacent research that does NOT apply here

`project_plans/omnibar-creation-stuck-modal/research/architecture.md` covers a **different,
already-fixed, purely-frontend bug**: `Omnibar.tsx`'s `isSubmitting` `useState` boolean never
got reset because the component is never unmounted (`OmnibarProvider` renders it unconditionally
and gates visibility internally) and the "reset on close" effect (`Omnibar.tsx:588-599`) forgot
to clear `isSubmitting`. That was shipped and merged via PR #441. It has nothing to do with
`CreateSession`'s server-side control flow, doesn't touch `session_service.go`, and doesn't
inform this restructure — it's a stale-boolean UI bug, not a blocking-RPC bug. Confirmed by
reading its file: no mention of `CreateSession`, the event bus, or instance status. **Do not
build on it; it is cited here only to record that it was checked and ruled out.**

## 1. Architectural pattern: which one actually fits

Three candidates were on the table per the prompt. Verdict, grounded in what's already built:

- **Event sourcing** — rejected. There is no event store/replay-from-events model anywhere in
  this codebase; `events.EventBus` is a pub/sub fan-out for *notifying* subscribers (WatchSessions
  streams), not a source of truth instances are rebuilt from. Introducing event sourcing here
  would be a second persistence paradigm alongside `session/ent` for no benefit this feature needs.
- **Saga / multi-phase orchestration** — partially right in spirit (this genuinely is a multi-phase
  process: resolve → clone → infer → worktree → tmux, each of which can fail independently and
  needs compensating cleanup), but the repo has no saga framework and doesn't need one. What it
  actually needs is a **linear phase pipeline inside one goroutine**, not a saga's defining
  feature (independently-schedulable steps with a coordinator that can resume from any step). A
  single ordered function that calls `SetCreationProgress` between phases, wrapped in one
  `recover()` and one cleanup path, is sufficient at this project's scale (single-user,
  single-process, no distributed coordination — explicitly out of scope per requirements.md).
- **Finite state machine for session lifecycle status** — this is the right frame, and it's
  **already half-built**, not something to invent. `session/instance_state.go`'s
  `transitionToLocked` (line 69) is a real FSM transition function running inside the actor
  (`session/actor.go`), and `ForceStatus` (line 367) is the documented **escape hatch** for
  transitions the FSM can't validate (exactly the async-creation-failure case — see its own doc
  comment: "Only call from error recovery paths where the normal transition would itself fail").
  The correct architecture is: extend the FSM's *vocabulary* (add `Failed`, teach it that
  `Failed → Creating` is a legal transition for retry) rather than bypass it further. Currently
  `STOPPED`'s doc comment says "terminal state, cannot transition further" — that comment becomes
  false the moment `Failed` (which retry legally exits) exists, so whatever the new status is
  named, it must be documented as explicitly non-terminal, unlike `Stopped`.

**Chosen shape: a single-writer phase pipeline running inside the existing actor/FSM
infrastructure, with an explicit epoch/fencing field for the 3 external triggers (background
goroutine, cancel, retry) that will race against it.** No saga framework, no event sourcing —
extend what's there.

## 2. The FSM/actor infrastructure already does more of the "hard part" than the sibling
   research assumed

`pitfalls.md` (§3) flagged "no CAS/guard on `ForceStatus`" as an open risk needing a targeted
look. That look is done now: **`ForceStatus` already serializes through the actor mailbox**
(`session/instance_state.go:349-376`, `session/actor.go:76-` `sendCtx`), not a bare mutex write —
its own doc comment explains exactly why: "ForceStatus is invoked from ad hoc goroutines outside
the actor (e.g. the async CreateSession goroutine in SessionService)... Funneling through sendCtx
serializes this write with the actor's command loop when the instance is actor-owned." Likewise
`SetCreationProgress` (`session/instance_actor_setters.go:68`) routes through `sendSyncErr`, the
same mailbox. So:

- **Solved by existing infrastructure**: two concurrent callers (e.g. the background resolution
  goroutine and a cancel handler) calling `ForceStatus`/`SetCreationProgress` on the *same
  instance* at the *same instant* will not corrupt shared state or data-race — the actor mailbox
  linearizes them.
- **NOT solved by existing infrastructure**: the actor mailbox linearizes *writes*, but has no
  concept of "this write is stale, ignore it." If cancel enqueues `ForceStatus(Cancelled)` and,
  microseconds later, a resolution goroutine that didn't yet observe the cancellation enqueues
  `ForceStatus(Active)`, the actor will faithfully execute both **in enqueue order** — the late
  `Active` write wins, silently reviving a cancelled session. This is exactly pitfalls.md §3's
  "retry-just-as-late-failure" and "cancel-just-as-success" races, and the actor does not fix it
  by itself. **An explicit generation/epoch field is still required**, checked by the *sender*
  before enqueueing a terminal-status command, not just relied upon as an ordering guarantee from
  the actor.

Recommended shape: add a `creationEpoch uint64` (or similar) to the instance, incremented on
every cancel and every retry-start. The background goroutine captures the epoch it was started
with; before sending any terminal `ForceStatus`/`SetCreationProgress` call, it checks (via a new
lightweight actor-routed read, e.g. `i.CreationEpoch()`) that its captured epoch still matches
current. A mismatch means "I've been superseded — no-op, do not publish a terminal event." This
also directly resolves pitfalls.md §4 (double-published terminal events): only the goroutine
holding the current epoch is allowed to publish a terminal `SessionUpdatedEvent`.

## 3. Data flow restructuring: what moves before/after the create+publish boundary

Current order (`session_service.go:1799-2430`, per stack.md/features.md):
1. Fast-fail validation (title required, path required, duplicate title, resume_id format) — L1806-1845
2. Fork dispatch (reads a file, writes a new conversation file) — L1847-1866
3. Config load, remote-target resolution, restart-source resolution — L1868-1889
4. **GitHub URL detection + clone/fetch** (`ResolveGitHubInputCtxWithHosts`, threads request `ctx`) — L1912-1938
5. (further down, not read in full but implied by stack.md) alias/default resolution, branch/session-type inference, one-off directory generation
6. Instance construction
7. `storage.SaveInstances` (implied — must happen before the uniqueness check in a second racing request can be trusted; needs confirming exact line in implementation phase)
8. `s.eventBus.Publish(NewSessionCreatedEvent(instance))`
9. `s.trackCleanup(func(){ ... instance.Start(true) ... })` — worktree/tmux only

**New order** (per requirements.md's explicit ask — instance created and event published before
*any* potentially-slow work, for every session type, not just GitHub-URL ones):

1. Fast-fail validation — **unchanged, stays synchronous** (Constraints: must preserve today's
   synchronous fast-fail behavior for these).
2. Fork dispatch — needs a decision: it does file I/O (read a conversation file, write a forked
   copy) but is not "potentially slow" in the same class as a GHE clone over VPN, and downstream
   code depends on `req.Msg.ResumeId` being set before instance construction. Recommend leaving
   fork dispatch synchronous (it's local-disk, not network) — it is not named as one of the
   move-to-background phases in the Scope section (which explicitly lists "GitHub URL clone/fetch,
   alias/default resolution, branch/session-type inference, worktree setup, tmux startup").
3. Config load, remote-target resolution, restart-source resolution — these are *cheap* (in-memory
   config, string parsing) and some (restart-source) are needed to decide fast-fail validation
   (Path-required check depends on `restartSourcePath`), so they stay synchronous, ahead of
   instance construction.
4. **Title-uniqueness check — stays synchronous** and must remain the *last* synchronous check
   before instance construction+save, so the shrinking-window property features.md §8 identifies
   holds: the window between "check title" and "persist the Creating instance" should be as small
   as possible, not have resolution work sandwiched inside it as today.
5. **Instance construction with status `Creating`, `storage.SaveInstances` (synchronous, so a
   racing second `CreateSession` call's uniqueness check sees it), then
   `eventBus.Publish(NewSessionCreatedEvent(instance))`** — this is the new "fast path ends here,
   RPC returns" boundary. The RPC response's `InstanceToProto` snapshot is taken here, before any
   background work starts (same discipline as today's L2380-ish snapshot-before-goroutine pattern,
   just moved earlier).
6. **Single background goroutine** (`s.trackCleanup`, not bare `go func()`), started with a fresh
   `context.WithoutCancel(ctx)` (or `context.Background()` per stack.md's recommendation, wrapped
   in `context.WithTimeout` — see §4 below), that runs the **merged pipeline**, publishing
   `SetCreationProgress` + `SessionUpdatedEvent{"creation_progress"}` between each phase:
   - a. GitHub URL detection + `ResolveGitHubInputCtxWithHosts` (now against the new background
     context, not the RPC's `ctx` — the RPC's `ctx`/timeout no longer reaches this call at all)
   - b. alias/default resolution, branch/session-type inference (whatever currently lives between
     L1938 and instance construction — needs the exact line range confirmed in Phase 3 detailed
     design/Phase 5 implementation, not fully visible in this survey's read window)
   - c. worktree/tmux startup (today's existing L2397-2413 tail, unchanged in shape)
   - d. terminal status: `ForceStatus(Active)` on success (gated by the epoch check from §2),
     or `ForceStatus(Failed)` with an error-reason string on any failure in a/b/c (also gated).

**Consequence for fast-path (plain directory) sessions**: they now *also* go through the
background goroutine (for consistency, per requirements' explicit "generalize to all session
types" choice), but since none of their phases involve real I/O beyond a fast local `os.Stat`,
the goroutine completes in the same low-single-digit milliseconds it always did — the RPC caller
sees no behavioral change, only an even-shorter RPC latency (since even the trivial local
resolution work no longer blocks the RPC).

## 4. Integration points

| System | Integration |
|---|---|
| `proto/session/v1/types.proto` `SessionStatus` enum (L358-392) | Add `SESSION_STATUS_FAILED = 11` (confirmed no existing value fits — `CRASHED` is post-Running, `STOPPED`'s own doc comment says terminal/no-further-transition, which retry violates). Must update the enum's `allow_alias` block only if reusing a wire value — a fresh value doesn't need aliasing. |
| `session/instance_state.go` FSM | Add `Failed` to the `Status` type; teach `transitionToLocked`/whatever validates legal transitions that `Failed → Creating` (retry) is legal, and that `Creating → Failed` is legal via the `ForceStatus` escape hatch (documented pattern, not a new one). Update `STOPPED`'s doc comment cross-reference if it's used as a "terminal states" enumeration elsewhere (grep for `Stopped` in switch statements that assume terminality). |
| `session/actor.go` mailbox (`sendCtx`/`sendSyncErr`) | Reused as-is for `ForceStatus`/`SetCreationProgress` calls from the background goroutine, cancel handler, retry handler, and stale-detector — no new mailbox needed. New: an epoch-check read/write pair, routed the same way. |
| `events.EventBus` / `NewSessionCreatedEvent` / `NewSessionUpdatedEvent` | `SessionCreatedEvent` published exactly once, at step 5 above (moved earlier, not duplicated). Retry publishes only `SessionUpdatedEvent`s (`["status", "creation_progress"]`) reusing the existing instance ID — never a second `Created` (resolves requirements.md's open question, consistent with pitfalls.md §4's recommendation). |
| `WatchSessions` (`session_service.go:3317`) | No RPC-shape change needed — it already streams both event types and already applies `StatusFilter` via `adapters.StatusToProto(session.Status(...))`, which needs its switch/mapping extended for the new `Failed` status (else a `Failed`-status instance either mismatches every filter or wrongly matches `UNSPECIFIED`). Also: `WatchSessions`'s initial-snapshot path reads `inst.GetStatus()` — the lock-free published snapshot, not a raw field — so any new status is automatically race-safe there for free. |
| `storage.ListInstanceData` / `storage.UpdateInstance` / `storage.SaveInstances` (`session/storage.go:277,399,546`) | `SaveInstances` happens synchronously at instance-creation time (step 5); the background goroutine's terminal write persists via the existing `SaveInstances`/`UpdateInstance` pattern the L2397-2413 precedent already uses on its failure path. Retry updates the *same* row in place — no new storage method needed, `UpdateInstance` (or `SaveInstances` on the single instance) already supports update-in-place. |
| `adapters.InstanceToProto` / `adapters.StatusToProto` | Both need a `Failed` branch — these are the two "parallel switch statements" features.md flags (§3) as needing exhaustiveness checking, mirrored on the frontend by `SessionCard.tsx`'s `getStatusColor`/`getStatusText`. |
| `s.trackCleanup` / `deleteCleanupWG` (`session_service.go:249-336`) | Reused for the new merged background goroutine exactly as today's L2397 tail uses it — this is the existing leak-prevention/shutdown-blocking mechanism (stack.md/pitfalls.md §1), not something to reinvent. |
| `DeleteSession` (`session_service.go:3193`) | Extended (or a new sibling `CancelSessionCreation` RPC/branch) to be the cancel-in-progress entry point — must (a) bump the epoch, (b) call the stored per-instance `context.CancelFunc` to interrupt the background goroutine's subprocess/clone, (c) route cleanup through the same idempotent cleanup primitive `DeleteSession` already uses (safe to call on partially-created state). |
| New `CancelFunc` storage | A `context.CancelFunc` needs a reachable home — either a field on `*session.Instance` (guarded the same way other actor-adjacent fields are) or a `map[instanceID]context.CancelFunc` on `SessionService`, cleared on goroutine exit to avoid a leak. Given the actor already centralizes per-instance state, a new instance field (set once at goroutine spawn, read once at cancel time) is more consistent with the existing architecture than a side map on the service. |
| New retry entry point | A new RPC method (e.g. `RetrySessionCreation`) or a branch on an existing mutation RPC — bumps the epoch, resets status to `Creating` + a fresh `creation_progress`, re-spawns the background goroutine (idempotent-cleanup-first per pitfalls.md §5), reuses the existing instance ID/storage row. |
| `config.StaleSessionConfig` (`config/types.go:126-152`) | Closest existing shape for the new stale-Creating threshold — mirror its `ThresholdMinutes`/`ThresholdMinutesOrDefault()`/`NotifyEnabled *bool` pattern rather than a package-level `const` (per requirements' explicit ask for a configurable threshold with a conservative default). |
| `StaleSessionNotifier` (`server/services/stale_session_notifier.go`, 155 lines) | Structural template for the new `StaleCreationSweeper`: ticker-driven, edge-triggered dedup map, tolerant of nil `eventBus`, reads only in-memory/persisted instance state (no extra I/O). New sweeper scans for `Creating`-status instances whose **last-progress-update timestamp** (not creation-start timestamp — see pitfalls.md §6) exceeds the configured threshold, and drives them through the same epoch-gated `ForceStatus(Failed)` path — critically, checking/bumping the epoch itself so a stale-flip and a genuine late success can't both "win." |
| OpenTelemetry (`telemetry/telemetry.go`) | New linked-root span (`trace.WithNewRoot()` + `trace.WithLinks(trace.LinkFromContext(ctx))`) per stack.md's already-detailed recommendation — not re-derived here, see stack.md §(b). |

## 5. Data-flow / consistency requirements — concrete rules

1. **Exactly one `SessionCreatedEvent` per session, ever.** Published synchronously at instance
   creation (new step 5), never again — not on retry, not on any background-phase transition.
2. **Exactly one terminal status write wins per creation attempt**, enforced by the epoch check in
   §2 — the background goroutine, the cancel handler, and the stale-detector must all check the
   epoch immediately before their terminal `ForceStatus` call, not just at the top of their
   function (a check-then-act gap between "read epoch" and "call ForceStatus" reopens the race —
   the actor mailbob doesn't close this gap by itself since the epoch check and the ForceStatus
   call are two separate mailbox round-trips unless combined into one enqueued command). Recommend
   a single new actor-routed helper, e.g. `TryForceStatusIfEpoch(epoch uint64, s Status) bool`,
   that reads-and-writes atomically *inside one enqueued command* rather than composing two
   separate `sendCtx` calls from the caller.
3. **`storage.SaveInstances` for the Creating instance must complete before the RPC returns**,
   preserving the title-uniqueness race property features.md §8 identifies (a second rapid
   `CreateSession` call's `ListInstanceData()` scan must see the first request's row).
4. **Cancel must order: stop background work → kill any live subprocess/tmux → remove
   worktree/clone directory → mark Cancelled/deleted**, mirroring pitfalls.md §5's ordering
   rationale (never delete a directory a subprocess still has open file handles into).
5. **Retry must run idempotent-cleanup before re-resolution**, never assume a prior cleanup
   already ran — same idempotent primitive as cancel and `DeleteSession`.
6. **Stale-detection timestamp must be `time.Time`-based on the last *persisted* progress update**,
   not in-process elapsed time or a monotonic-only reading (session/ent round-trips through JSON,
   stripping the monotonic component per pitfalls.md §6) — so it fires correctly for both an
   in-process hang and a `Creating` row orphaned by a server restart.

## 6. EventStorming: Event-Command-Policy table

Actors: **User** (via omnibar/session-card UI), **Omnibar/UI** (frontend), **CreateSession RPC
handler** (synchronous prefix), **Background Resolution Goroutine** (the merged async pipeline),
**Stale-Creation Sweeper** (new ticker), **GitHub API/GHE host** (external system).

| # | Actor | Command | Domain Event | Policy (reaction) |
|---|---|---|---|---|
| 1 | User | SubmitCreateSessionForm | — | Omnibar calls `CreateSession` RPC |
| 2 | CreateSession RPC handler | ValidateFastFailFields | `SessionCreationRejected` (sync RPC error: bad title/path/alias/resume_id/fork-source) | Omnibar keeps dialog open, shows inline error (unchanged today) |
| 3 | CreateSession RPC handler | CheckTitleUniqueness | `SessionCreationRejected` (AlreadyExists) — *or* proceeds | as above on rejection |
| 4 | CreateSession RPC handler | CreateInstanceAndPublish | **`SessionCreated`** (status=Creating, epoch=0) | (a) RPC returns fast to Omnibar; (b) `s.trackCleanup` spawns Background Resolution Goroutine; (c) WatchSessions subscribers (session list) render a new Creating card |
| 5 | Omnibar/UI | (none — passive) | — | On receiving the fast RPC response, closes the dialog immediately (no longer awaits full completion) |
| 6 | Background Resolution Goroutine | ResolveGitHubURL (if applicable) | `SessionCreationProgressed` (msg="Resolving GitHub URL...") | SessionCard renders updated progress text |
| 7 | GitHub API/GHE host | (external) RespondToCloneRequest | — | triggers goroutine to continue or fail |
| 8 | Background Resolution Goroutine | CloneOrFetchRepository | `SessionCreationProgressed` (msg="Cloning repository...") / on error: **`SessionCreationFailed`** (reason=GitHubResolutionError, gated by epoch check) | on success, continue to next phase; on failure, publish terminal Failed event + persist |
| 9 | Background Resolution Goroutine | ResolveAliasAndDefaults / InferBranchAndSessionType | `SessionCreationProgressed` | SessionCard renders updated progress text |
| 10 | Background Resolution Goroutine | StartWorktreeAndTmux | `SessionCreationProgressed` (msg="Setting up worktree...", "Starting session...") / on error: **`SessionCreationFailed`** (reason=StartupError, gated by epoch check) | as above |
| 11 | Background Resolution Goroutine | CompleteCreation | **`SessionCreationSucceeded`** (ForceStatus→Active, gated by epoch check) | SessionCard shows Running; toast (if any) clears |
| 12 | User | CancelInProgressCreation | **`SessionCreationCancelled`** (bumps epoch, cancels background ctx, gated ordering per §5.4) | Background goroutine's next epoch check no-ops; cleanup runs (kill subprocess, remove worktree, remove tmux); card removed/hidden |
| 13 | User | RetryFailedCreation | **`SessionCreationRetried`** (bumps epoch, resets status Failed→Creating, re-spawns Background Resolution Goroutine after idempotent cleanup) | SessionCard shows Creating again with fresh progress; no new `SessionCreated` event |
| 14 | Stale-Creation Sweeper (ticker) | DetectStaleCreation | **`SessionCreationTimedOut`** (ForceStatus→Failed, reason=Stale, gated by epoch check) | SessionCard shows Failed with "stuck/orphaned" reason distinct from a resolution error; metric emitted |
| 15 | Background Resolution Goroutine or Stale Sweeper (server-restart case) | (none — process died) | *(no event — this is the absence case)* | Next process's Stale-Creation Sweeper run detects the orphaned `Creating` row via its persisted last-update timestamp (§5.6) and fires #14 |
| 16 | Any of #8/#11/#14 (whichever wins the epoch race) | PublishTerminalStatusEvent | **`SessionCreationFailed`** / **`SessionCreationSucceeded`** / **`SessionCreationTimedOut`** — exactly one, per §5.2 | UI toast fires at the moment of terminal failure (per requirements' success metric); persistent Failed card state remains after toast dismisses |

Notes on the table:
- Rows 8/10/14/16 all funnel through the same single **"terminal status write" policy boundary**
  (§5.2's `TryForceStatusIfEpoch`) — that's the one place double-publish (pitfalls.md §4) is
  prevented, not by convention across call sites but by one shared, atomic, epoch-gated primitive.
- `SessionCreationProgressed` is not a new named domain concept — it's the existing
  `SessionUpdatedEvent{"creation_progress"}` mechanism, listed distinctly here only because
  EventStorming grammar calls for naming each meaningfully-different domain occurrence, not
  because a new wire event type is needed (confirms features.md §2's read).
- `SessionCreationRejected` (rows 2-3) is not a domain event in the eventual-consistency sense —
  it's a synchronous RPC error, listed here only to show where the fast-fail boundary sits before
  any of the async machinery below it engages.

## 7. Failure modes specific to this being a live-critical, all-session-types RPC

The prompt asks to separately evaluate: "a bug here could leave ALL users unable to create ANY
session, not just GitHub-URL ones, since every session type now flows through the new
create-then-resolve-async path." This is a materially different risk profile than "the GitHub-URL
path might have a bug" — every non-GitHub session (plain directory, one-off, restart, fork, alias,
autonomous, remote) now depends on code that didn't previously touch it.

- **A single shared bug in the merged pipeline's *early* phases breaks every session type, not
  just GitHub ones.** Today, a bug in `ResolveGitHubInputCtxWithHosts` only breaks GitHub-URL
  sessions — a directory session never calls it. After the merge, *all* session types pass through
  the same goroutine function; if a refactoring mistake makes phase-sequencing or the epoch-check
  itself buggy (e.g. an off-by-one that treats every instance as already-superseded, or a nil
  pointer in shared setup code that runs before the GitHub-specific branch), **every session
  creation fails**, including trivial directory sessions that have no reason to. This is the
  single highest-severity regression class this restructure introduces, and it's structural (a
  consequence of the requirements' explicit choice to generalize, not scope to GitHub-only) —
  mitigate by keeping the shared pre-branch code in the merged goroutine as minimal as possible
  (just the phase-dispatch skeleton + epoch/progress plumbing), with each session type's actual
  logic in its own already-tested function, so a bug in the GitHub branch's *body* can't reach a
  directory session's code path — only a bug in the *shared skeleton* can, and that skeleton
  should be small enough to review and test exhaustively.
- **The synchronous prefix becomes the new single point of total failure.** Since fast-fail
  validation, title-uniqueness check, and instance construction+save+publish (new step 5) are now
  the *only* synchronous code every session type runs through, any regression there (e.g. a
  mis-ordered check, a panic in instance construction) blocks 100% of session creation
  immediately and synchronously — this was already true today for the equivalent prefix, but the
  prefix's *content* changes in this restructure (instance construction moves earlier, some
  resolution logic that used to gate construction is removed from this path), so it needs the same
  scrutiny as new code, not "it was already there, skip re-review."
- **A goroutine-spawn failure (not a goroutine-body failure) silently drops every session into
  permanent Creating.** If `s.trackCleanup` itself panics or fails to enqueue (e.g. because
  `deleteCleanupClosed` is true — a shutdown race) *after* the instance is already created and
  published (step 5), the instance is now stuck in `Creating` forever with no goroutine ever
  running to move it forward, for every session type simultaneously if this happens under load
  (e.g. during a rolling restart with in-flight creations). The stale-detector (§6 row 14/15) is
  the intended backstop for this, but it must be verified to catch *this specific* case (spawn
  failure with zero progress messages ever set) not just the "goroutine started but got stuck
  mid-clone" case — a stale-check keyed on "time since last progress update" also covers "zero
  progress updates ever," using the instance's `Creating`-status onset time as the initial
  baseline, but this must be an explicit test case in Phase 6, not assumed.
- **Regression surface is every one of the 7 session-creation touchpoints simultaneously**, per
  `.claude/docs/session-creation-registry.md` and requirements.md's Feasibility Risks — directory,
  one-off, restart, fork, alias, autonomous, remote. Since there's no feature flag (explicit
  Risk Control choice), there is no way to roll out to a subset of session types or roll back
  partially; the only rollback is a full `git revert`. This raises the bar on Phase 4
  (pre-mortem)/Phase 6 (verification) to explicitly exercise **all seven** touchpoints against the
  new code path before merge, not sample a subset and extrapolate — a bug isolated to, say, the
  `restart_from_session_id` touchpoint would otherwise ship silently since it's the least
  frequently manually tested of the seven.
- **The epoch/fencing mechanism itself becomes new shared infrastructure every session type
  depends on.** If `TryForceStatusIfEpoch` (§5.2) has a bug that makes it *always* report "no
  longer current" (an inverted comparison, e.g.), every successful creation's terminal
  `Active`-status write would be silently dropped — every session would appear to spin in
  `Creating` forever even though its worktree/tmux actually started fine underneath. This is a
  correctness-of-new-shared-primitive risk, not a resolution-logic risk, and it means the epoch
  mechanism needs its own focused unit tests (success path writes through; superseded-epoch path
  correctly no-ops; back-to-back cancel+retry doesn't leave the epoch permanently stuck) before
  it's trusted as the single serialization point for every session type's terminal status.
- **Observability blind spot during exactly the failure this project is trying to prevent.** If
  the new tracing span/metric wiring itself has a bug (e.g. throws on a nil meter before OTel is
  initialized, or the span helper panics when `ctx` has no parent span to link from), and that
  code sits in the shared pre-branch skeleton rather than being defensively no-op-safe (per
  stack.md's note that `telemetry.GetMeter()`/`GetTracer()` are already designed to be safe when
  OTel is disabled — reuse that guarantee, don't bypass it with a hand-rolled span/metric call
  that assumes OTel is always configured), a purely-observability change could itself become the
  outage vector for 100% of session creation — worth an explicit test that creation still succeeds
  with OTel fully disabled/unconfigured.
