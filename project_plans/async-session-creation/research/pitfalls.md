# Research: Pitfalls of create-then-resolve-async RPC restructuring

Scope: known failure modes for converting a synchronous ConnectRPC handler
(`CreateSession`, `server/services/session_service.go:1799`) into a
create-placeholder-then-finish-in-background pattern, for a single-process,
single-user-per-instance Go server. Grounded in this repo's existing
precedent (`trackCleanup`/`deleteCleanupWG`, `server/services/session_service.go:249-336`,
already used for the `Start(true)` async goroutine at
`session_service.go:2390-2413` and `DeleteSession`'s cleanup at
`session_service.go:3193+`) plus general Go/distributed-systems pitfalls for
this class of restructuring.

## 1. Goroutine leaks

**What goes wrong:** A bare `go func() { ... }()` spawned per `CreateSession`
call has no owner. Nothing tracks it, nothing awaits it, and nothing kills it
if the server needs to shut down or restart. Under repeated
create/fail/retry cycles (a flaky GHE VPN connection, per the requirements'
own trigger case) this accumulates goroutines that hold references to
`*session.Instance`, tmux handles, and clone subprocesses — a slow leak that
only shows up after hours of dev-session churn, exactly the kind of bug that
doesn't reproduce in a quick manual test.

**This repo's existing answer:** `trackCleanup` (`session_service.go:320-336`)
wraps the goroutine in `deleteCleanupWG.Add(1)`/`Done()` so `Shutdown()`
(`session_service.go:1093`) can `Wait()` for in-flight background work before
the process exits, with a documented mutex-guarded escape hatch
(`deleteCleanupClosed`) so a goroutine spawned *during* shutdown doesn't
deadlock or panic against a `WaitGroup` in violation of its "Add before Wait"
contract. **The new background-resolution goroutine must go through the same
mechanism** (or an equivalent one), not a bare `go func()` — the codebase
already learned this lesson once (see the comment at `session_service.go:2390-2396`
explicitly citing a test flake, `TestCreateSession_should_...BothPresent`,
caused by a goroutine outliving its spawning test).

**Design against:**
- Route every background-resolution goroutine through `trackCleanup` (or a
  purpose-built sibling `WaitGroup` if resolution goroutines need distinct
  shutdown semantics from delete-cleanup goroutines — e.g. a bounded
  "cancel outstanding, don't wait forever" shutdown policy, since a
  wedged GHE clone could otherwise block process shutdown indefinitely).
- Verify no goroutine pile-up under repeated create/fail/retry with `go test
  -race` plus `goleak` (the `golang-testing` skill's goroutine-leak-detection
  guidance) around a test that hammers `CreateSession` with a failing
  resolution path many times in a loop.
- Every exit path out of the goroutine (success, resolution error, ctx
  cancellation, panic) must reach the `Done()`/cleanup — a `recover()` at the
  top of the goroutine body is required, not optional: an unrecovered panic
  in a detached goroutine crashes the whole process (no request boundary to
  contain it), unlike a panic inside an RPC handler which ConnectRPC's
  interceptor chain can catch per-request.

## 2. Context lifetime mistakes

**What goes wrong (the single most common bug in this pattern):** The
request context `ctx` passed into `CreateSession` is scoped to the RPC's
lifetime — ConnectRPC cancels it when the client disconnects, and this repo
additionally wraps it in `context.WithTimeout(ctx, createSessionTimeout)`
(`session_service.go:1803`). Today that's *correct*: `ctx` threads down into
`ResolveGitHubInputCtxWithHosts` specifically so an RPC timeout or client
disconnect cancels the underlying `git clone` subprocess
(`session_service.go:1915-1921`, explicitly commented as intentional). The
classic mistake when splitting this into "return early, finish in
background" is **forgetting to swap that same `ctx` for a new,
independently-scoped one before spawning the goroutine** — the moment the
handler returns, `defer cancel()` fires, the request context is canceled,
and the "background" resolution dies immediately (or a few instants later)
with a `context.Canceled` error that looks like a resolution failure but is
actually just... the RPC returning as designed. This is easy to miss in
testing because a fast RPC framework, run locally against a fast clone,
returns and cancels the context on almost the same tick that the goroutine
starts — timing-dependent, so it may pass locally and fail only when the
clone is genuinely slow (i.e., exactly the case this project exists to fix).

**Design against:**
- The background goroutine must capture a **new** root context —
  `context.WithTimeout(context.Background(), someBackgroundTimeout)` — never
  a derivative of the inbound `ctx`. The requirements doc already names this
  correctly (Rabbit Holes section, `context.WithTimeout(context.Background(),
  ...)`); make sure implementation doesn't accidentally close over `ctx` from
  the enclosing function by copy-paste (Go closures capture variables, not
  values — a goroutine literal referencing the outer `ctx` will silently pick
  up whatever `ctx` was, including one that gets canceled the instant the
  handler returns).
- Give the background context its *own* timeout, independent from
  `createSessionTimeout` (that constant is now purely an RPC-level budget for
  the fast synchronous validation phase). Needs to be generous enough for a
  slow-but-working GHE clone, but bounded — otherwise a wedged clone
  (network partition mid-transfer, credential prompt hanging forever) leaks
  the goroutine until the process restarts. This bound is exactly what
  feeds stale-creation detection (§6) — the two need to agree, or a
  legitimately-still-running goroutine can get its instance flipped to
  Failed by the stale detector while the goroutine is still trying to write
  to it (see §3).
- Cancel-in-progress (a user deleting a `Creating` session) needs an explicit
  per-instance `context.CancelFunc` stored somewhere reachable from
  `DeleteSession`/cancel handling — not `context.Background()`'s
  no-op cancel. Store it on the `*session.Instance` (or a side map keyed by
  instance ID) at goroutine-spawn time, and call it from the cancel path.
  Forgetting this means "cancel" only removes the storage row/UI card while
  the goroutine keeps cloning/writing to a deleted instance in the
  background — exactly the "orphaned worktree" failure mode called out in
  the requirements.
- Double-check `safeexec.CommandContext` call sites inside the resolution
  chain (`ResolveGitHubInputCtxWithHosts` and anything it calls) — they need
  to receive the *new* background context, not the old request `ctx`, or the
  subprocess-cancellation property this repo relies on (comment at
  `session_service.go:1915-1921`) silently breaks for the async path only,
  while still looking right for the sync fast-path tests.

## 3. Races between background status updates and concurrent cancel/retry

**What goes wrong:** Once creation is async, at least three actors can touch
the same `*session.Instance` concurrently: (a) the background resolution
goroutine calling `SetCreationProgress`/`ForceStatus`, (b) a user-triggered
cancel/delete RPC, and (c) a user-triggered retry RPC. Classic races:
- **Cancel-just-as-success races**: goroutine finishes resolution and is
  about to call `ForceStatus(session.Active)` at the exact moment
  `DeleteSession` marks the instance deleted and kills its tmux
  session/worktree. If the ordering isn't serialized, you get a session that
  briefly reports `Running` after being "deleted," or a worktree that gets
  torn down by delete-cleanup while the goroutine's `Start(true)` is still
  mid-write to it (partial worktree deleted out from under a live git
  operation → a corrupted or half-deleted `.git` state, not just a clean
  no-op).
- **Retry-just-as-late-failure races**: user hits retry on a Failed card; a
  *stale* background goroutine from the original attempt (which the
  requirements assume dies via context cancellation, but see §2 — if that
  wiring is subtly wrong, it doesn't) wakes up late and writes
  `SetCreationProgress`/`ForceStatus` over the retry's fresh in-progress
  state, silently reverting a successful retry back to Failed, or worse,
  double-starting tmux/worktree setup for the same instance ID from two
  concurrent goroutines.
- **Lost updates from unsynchronized field writes**: `SetCreationProgress`
  and `ForceStatus` need to be safe to call concurrently with whatever reads
  the instance for `WatchSessions`/`GetSession` RPCs — check whether
  `session.Instance`'s status/progress fields are already mutex-guarded (this
  repo's concurrency conventions in `.claude/docs/concurrency-patterns.md`
  are the place to check) or whether this new call pattern is the first one
  to actually exercise concurrent writers against those fields.

**Design against:**
- Every mutating action on a given instance ID (background progress update,
  cancel, retry) must go through a **single serialization point per
  instance** — e.g. an instance-scoped mutex, or a generation/epoch counter
  incremented on cancel/retry so a stale goroutine's writes can be detected
  and dropped ("if my captured epoch != instance's current epoch, no-op and
  return" — a standard fencing-token pattern for exactly this "a canceled
  worker wakes up late" race).
- Cancel must be **synchronous with respect to the goroutine it's canceling**
  in the sense that: calling `cancel()` on the background context is not
  sufficient by itself — the cancel path must also wait for (or fence
  against) that goroutine's in-flight write before it proceeds to tear down
  worktree/tmux state, or use the same instance-generation fencing so a
  late write from the canceled goroutine is provably a no-op rather than
  racing the teardown.
- Retry must not spawn a second background goroutine while the first one
  (from the original failed attempt) might still be alive — verify actual
  termination (via the epoch/fencing token, or by awaiting the tracked
  `WaitGroup` entry for that specific instance) before starting the retry's
  goroutine, not just optimistically assuming context cancellation already
  won the race.

## 4. Double-published events

**What goes wrong:** The plan already needs at least
`SessionCreatedEvent` (at instance-creation time) and one or more
`SessionUpdatedEvent`s (per progress phase, and at terminal
success/failure) — mirroring the existing pattern at
`session_service.go:2380-2411` (`NewSessionCreatedEvent` once, then
`NewSessionUpdatedEvent` for progress and for the terminal state change).
Two ways this goes wrong under the *new* retry/cancel/stale-detection
surface that didn't exist before:
- **Retry re-publishing `SessionCreatedEvent`**: if retry's code path
  reuses the "create" logic verbatim instead of a dedicated "resume
  resolution on existing instance" path, subscribers (the web-app's session
  list) see a second `Created` event for the same session ID — at best a
  harmless duplicate row that briefly flickers, at worst (if any subscriber
  keys off "first Created event wins" or does `assert not already present`)
  a real bug. The requirements' own Open Questions section flags this
  ambiguity explicitly and it should be resolved (bias toward: retry only
  ever emits `Updated` events, never a second `Created`).
- **Stale-detector races with a slow-but-genuine success**: the background
  ticker (§6) and the resolution goroutine both have a code path that ends
  in "publish an Updated event with a terminal status." If both fire near
  the threshold boundary — goroutine finishes and calls
  `ForceStatus(Active)` + publish at nearly the same instant the stale
  detector's ticker fires and flips the same instance to Failed — you can
  get two contradictory terminal events in quick succession (`Active` then
  `Failed`, or vice versa), which is confusing at best and could paper over
  a real success as a false stale-failure (see §6).
- **Event bus semantics**: check whether `s.eventBus.Publish` is at-least-once
  or whether any subscriber does deduplication by event ID/sequence number —
  if not, every one of the above races becomes a UI-visible glitch, not just
  an internal inconsistency.

**Design against:**
- One code path, one place, that's allowed to publish a *terminal* status
  event (Active or Failed) for a given creation attempt — gate it on the
  same fencing/epoch mechanism from §3 so only the "winning" writer (the one
  whose epoch matches current) actually publishes.
- Explicitly resolve the retry-event question the requirements leave open:
  document that retry re-uses the existing instance ID and never
  re-publishes `SessionCreatedEvent`, only `SessionUpdatedEvent`s for the
  status transition Failed → Creating → (Active|Failed).
- Add a test that asserts exactly one terminal event is published per
  creation attempt even when cancel/retry/stale-timeout race with a
  successful completion (this is the kind of thing that's easy to get right
  by inspection and wrong under `-race -count=50`).

## 5. Partial-failure cleanup: orphaned worktrees/tmux sessions/clones

**What goes wrong:** This is the risk the requirements call out most
explicitly (Rabbit Holes: "Retry-in-place vs. re-run-from-scratch"), and it's
a well-known failure mode for this exact pattern: resolution can fail *after*
partial side effects have already happened — a clone directory partially
populated, a git worktree created but `tmux new-session` not yet run, or a
tmux session started but the Claude Code process inside it not yet
launched. Naively "retry = re-run creation logic from scratch" then either:
- fails outright the second time because the clone directory/worktree
  already exists (git refuses to clone into a non-empty directory, `tmux
  new-session` on an existing session name errors), surfacing as a confusing
  secondary error that obscures the original failure; or
- silently creates a **second** worktree/tmux session alongside the orphaned
  first one, doubling disk usage and leaving a zombie tmux session running
  indefinitely (tmux sessions don't self-terminate just because nobody's
  attached, and this repo's own docs — `.claude/docs/tmux-keep-server-on-restart.md`
  — already establish that tmux server lifecycle is a known sharp edge here).
- Cancel has the identical failure mode: a user cancels mid-clone, and if
  cancellation only marks the instance deleted without also killing the
  clone subprocess/tmux session/removing the worktree, the abandoned
  resources sit on disk/in the tmux server indefinitely, invisible to the UI
  once the card disappears (exactly "no way to notice a creation that
  silently got orphaned," which the requirements name as a *current* gap
  this project should close, not reintroduce in a new shape).

**Design against:**
- Retry and cancel must both route through the **same idempotent cleanup
  primitive** — the requirements point at `DeleteSession`'s existing cleanup
  path (`session_service.go:3193+`, tracked via `trackCleanup` and bounded by
  `deleteSessionCleanupTimeout`) as the closest precedent; reuse or
  generalize it rather than writing a second, divergent cleanup routine.
  "Cleanup" must be safe to call on a resource that was never fully created
  (clone half-done, worktree absent, tmux session absent) — every step
  (remove clone dir, remove worktree, kill tmux session) needs to be a no-op
  on "already absent," not an error.
- Retry's actual sequence should be: idempotent-cleanup(instance) →
  reset progress/status to Creating → re-run resolution from a clean slate.
  Never "resolution logic assumes a pristine environment and hopes cleanup
  already ran" — make the cleanup step an explicit, awaited precondition of
  retry, not a side effect some other path is trusted to have already done.
- Cancel must synchronously (or via awaited async cleanup with the UI
  reflecting a "Cancelling..." transitional state, not an instant
  disappearance) kill the clone subprocess (context cancellation, §2),
  remove the tmux session if one was started, and remove the worktree/clone
  directory — in that order (kill the process actively writing into the
  directory before deleting the directory, not after, to avoid a
  use-after-delete race on the subprocess's own file handles).
- Crash/restart cleanup (server killed mid-goroutine, no cancel or delete
  ever ran) is a *separate* mechanism from in-process cancel — it needs
  either a startup-time sweep that finds `Creating`-status instances left
  over from a previous process and treats them as immediately stale (see
  §6), or acceptance that genuinely orphaned worktree/tmux resources from a
  hard crash require a manual/periodic sweep outside the RPC's control (this
  is explicitly a local dev tool per the requirements' NFRs, so "orphaned
  worktree survives until the user notices disk usage" may be an acceptable
  residual risk *as long as it's surfaced*, which is what stale-detection's
  metric is for).

## 6. Staleness-detection false positives/negatives

**What goes wrong:** A single fixed threshold is the natural first design
and it's wrong in both directions simultaneously, because "how long is a
legitimate creation allowed to take" varies enormously by session type and
network conditions:
- **False positive (kills a legitimately slow but working creation):** a
  large monorepo clone over a congested VPN, or a cold GHE host needing
  auth handshake, can legitimately take well past a threshold tuned for the
  common case (plain directory sessions, which should be near-instant). If
  the stale detector flips it to Failed while the clone is still actively
  making progress, the user sees a spurious failure for exactly the
  slow-network case this whole project exists to make visible and
  *tolerable*, not intolerant of. This is worse than today's behavior in one
  specific way: today a slow clone eventually succeeds (up to
  `createSessionTimeout`=150s); a too-aggressive stale detector could kill
  it *before* the same clone would have succeeded under the old
  synchronous path.
- **False negative (doesn't catch real orphaning):** if the threshold is a
  single wall-clock timer measured from creation-start with no reset on
  progress, a slow-but-genuinely-making-progress creation and a truly-stuck
  one (process crashed, goroutine leaked and hung, network partition with no
  timeout on the underlying read) look identical until the threshold fires
  — which is fine if progress-based liveness is tracked; **not fine if the
  detector can't distinguish "no update in 5 minutes because it's stuck" from
  "no update because it just entered a genuinely slow single phase" and the
  threshold has to be set conservatively high to accommodate the second
  case**, meaning real orphans (server crashed, goroutine gone entirely) sit
  invisible for that same long threshold — reintroducing exactly the
  "orphaned sessions sit invisible for a long time" failure the requirements
  ask to fix.
- **Server-restart edge case:** an instance left in `Creating` status
  persisted to storage from before a crash has *no* live goroutine at all
  behind it after restart — a plain "time since last update" check handles
  this fine (nothing will ever update it again, so it goes stale on the
  normal schedule), but if the design instead keys staleness off "is there a
  known-live goroutine for this ID," a restarted process has no such
  bookkeeping at all and needs the pure timestamp-based check as the
  fallback specifically for this case — don't build a liveness check that
  only works for the in-process case and silently never fires post-restart.
- **Clock/monotonic pitfalls:** if the "last updated" timestamp is
  wall-clock (`time.Now()`) rather than compared via monotonic reads, a
  system clock adjustment (NTP correction, sleep/wake on a laptop — this is
  a local dev tool that runs on laptops that sleep) can make an instance
  spuriously appear stale (clock jumped forward) or never appear stale
  (clock jumped backward). Go's `time.Time` retains a monotonic reading when
  obtained via `time.Now()` and comparisons via `Sub`/`After`/`Before` use it
  automatically *as long as neither value was serialized through
  `MarshalJSON`/parsed back* — but this instance's `CreatedAt`/`UpdatedAt`
  almost certainly round-trips through `session/ent`/JSON storage, which
  strips the monotonic reading, so the stale-check comparison ends up purely
  wall-clock. Laptop sleep/wake is a real, common case here, not a
  theoretical one.

**Design against:**
- Reset (or extend) the staleness timer on every genuine phase-progress
  update, not just at creation start — the check should be "how long since
  the last observed progress," not "how long since the instance was
  created." This directly separates "single long phase, still working" from
  "no update because nothing is running any more."
- Get a real number before picking a default threshold — the requirements'
  own Open Questions ask for this ("Needs a conservative default plus a
  config override, informed by Phase 2 research into typical
  clone/worktree/tmux-startup timings in this repo"). Do not ship a guessed
  constant; measure actual GHE clone times (including a cold/VPN-degraded
  case) before setting the default, and make it configurable per the
  requirements.
- Explicitly design for the sleep/wake case: consider whether "time
  since last update" alone is safe on a laptop that sleeps for 8 hours
  mid-clone (it will look stale on wake, which — arguably correctly — should
  fail it, since the clone subprocess is almost certainly dead after the
  network dropped during sleep; but confirm this is the *intended* behavior
  rather than an accidental one).
- Make the stale-to-Failed transition go through the same single-writer
  fencing mechanism as §3/§4 so it can't race a genuine late success.

## Summary of what commonly goes wrong (cross-cutting)

Across the five categories above, the recurring root causes are the same
handful of design gaps, worth stating explicitly as things to design against
up front rather than discover in code review:

1. **Reusing the request context for background work** (§2) — the single
   most common bug in this exact refactor shape, because it "just works" in
   fast local testing and only fails under real network latency.
2. **No single-writer/fencing discipline per instance** (§3, §4, §6) — once
   there are 3+ independent triggers (background goroutine, cancel, retry,
   stale-timeout) that can all reach "flip this instance's status," races
   between them are not an edge case, they're the default behavior unless
   explicitly serialized.
3. **Treating retry as "run create again"** instead of "idempotent
   cleanup, then resume" (§5) — this is what turns a failed clone into two
   orphaned worktrees.
4. **Guessing the staleness threshold** instead of measuring real timings
   first (§6) — a wrong-direction guess actively makes user experience worse
   than today's synchronous timeout in the false-positive case.
5. **Not reusing this repo's own `trackCleanup`/`WaitGroup` precedent**
   (§1) — the codebase already paid down this exact goroutine-lifecycle
   lesson once (see the explicit test-flake comment at
   `session_service.go:2390-2396`); the new background-resolution goroutine
   should extend that mechanism, not reinvent a parallel one.
