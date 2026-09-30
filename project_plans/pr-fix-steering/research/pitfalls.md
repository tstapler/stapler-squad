# Research: Pitfalls and Risks — Automated Steer Injection

Research question: what commonly goes wrong with automated PTY/controller injection into a
live session, and what's specific to this codebase's chosen approach (extending
`AutoReopenForPRFix` to call a new `SessionSteerer`, reusing `UpdateSession`'s
`SteerMessage` branch)?

## 1. Prior documented incidents this feature must not repeat

- **Double-transition churn (the exact scenario this feature extends).**
  `AutoReopenForPRFix`'s own doc comment
  (`server/services/backlog_service_triage.go:2037-2046`) and
  `docs/tasks/backlog-feature-improvement.md:456` describe a live incident: the 60s
  `ReconcilePRPending` tick called `AutoReopenForPRFix` unconditionally, transitioned
  `pr_pending`→`in_progress`, discovered the spawn was blocked by an active session, and
  rolled back to `pr_pending` — producing 2 `BacklogStatusEvent` rows every tick,
  indefinitely, including while a **genuinely still-active 4-hour session** was already
  correctly detected as alive by `IsSessionLive`. Fix required two commits: tombstoning
  confirmed-dead sessions (`af426f27`) *and* checking `findActiveWorkSession` **before**
  any status transition, regardless of alive/dead (`f8f788ab`).
  **Design implication**: the new steer branch must follow the same shape — check
  dedup/liveness before doing anything with side effects (steering, notifying), and must
  **not** perform any `TransitionBacklogItemStatus` call in the active-session branch (it
  already doesn't — `AutoReopenForPRFix` returns immediately after
  `notifyRespawnBlockedByActiveSession` today, before the transition call). Adding a steer
  call to that early-return branch is safe from re-introducing the churn *only if* it stays
  a no-status-change side effect, and the reason-signature dedup itself must be as
  deterministic as `f8f788ab`'s fix was (checked once per tick, not re-derived from partial
  state).

- **PR #157 pattern (branch drift silently escalating to a hard conflict).** Cited at
  `backlog_service_triage.go:2085-2095`: a PR branch drifted from `main` with nobody
  proactively resyncing it until it hit a hard, harder-to-diagnose conflict. The comment
  documents this is *why* `syncPRBranchWithMain` runs before spawning a fresh fix session.
  **Design implication**: the steer path bypasses `syncPRBranchWithMain` entirely — that
  call only happens in the no-active-session branch, right before
  `TransitionBacklogItemStatus`. If a `HasConflicts` steer fires while a session is active,
  it is telling the agent about a conflict without the proactive main-merge that the other
  path gets for free. This isn't necessarily wrong (the active session can run its own
  merge), but the steer content should say "your branch may need a merge with main," not
  assume the state is already resynced — the two paths diverge here and the requirements'
  `fixContext` reuse doesn't account for it.

- **MCP-driven session steering already fails silently when the controller hasn't wired up
  yet** (`docs/tasks/backlog-feature-improvement.md:480`, "Found live during Phase 5"):
  `steer_session`/`run_command`/`write_to_session` all fail with *"cannot send keys to
  instance that has not been started or is paused"* until a human opens the session in the
  web UI, because MCP-created sessions log `"skipping controller startup, will be started
  after wiring"` and nothing ever finishes that wiring without a UI open event. This is
  **not fixed** upstream. Any backlog work session spawned the same way (autonomous,
  MCP/API-created, never opened in the UI by a human) is a plausible target for this
  feature's steer call and will hit the identical failure. The reconciler is exactly the
  kind of caller that would try to steer a session nobody has opened yet.

## 2. Timeout/failure semantics already handled vs. not

Reused code path: `session_service.go:2929-2972` (`UpdateSession`'s `SteerMessage`
handling), which the requirements say the new `SessionSteerer` must delegate to.

| Session state | Autonomous branch (`SendCommandImmediate`, lines ~2934-2945) | Interactive branch (`SendKeys`, lines ~2946-2969) |
|---|---|---|
| **Dead / never started / paused** | `controller.GetController()` may be `nil` → **entire block is a silent no-op**: no log, no error, no notification. If controller exists but `!cc.started.Load()`, `SendCommandImmediate` returns `"controller not started"` — but the caller only `log.Warn`s it, **does not return an error**, and skips `notifySteerSent`. | `instance.SendKeys` returns `"cannot send keys to instance that has not been started or is paused"` (`session/instance_tmux.go:1023`) inside a goroutine race against a 5s timeout; this error **is** propagated to the RPC caller as `CodeFailedPrecondition`. |
| **Mid-generation / busy** | `SendCommandImmediate` bypasses the queue (comment: "bypasses queue... High priority") — it does not check whether the agent is mid-turn; it is fire-and-forget into the controller's immediate-execution path. No busy detection exists in this call. | `SendKeys` writes bytes straight to the PTY regardless of what's on screen — same lack of busy-awareness. This is exactly the Rabbit Hole the requirements call out ("interrupting in-progress work") and the requirements *assume* it's already solved because humans use this path today — it is not solved, it is simply tolerated because a human decides when to send. |
| **Timeout (5s)** | No timeout at all on `SendCommandImmediate` — it's a synchronous in-process call, not a PTY write, so there's nothing to time out; but this also means a genuinely wedged `ExecuteImmediate` (e.g. lock contention inside the controller) would block the RPC handler goroutine indefinitely rather than degrading. | Both `session_service.go`'s interactive branch and `tools_terminal.go:686-699`'s `steerSession` spin `instance.SendKeys` in a goroutine racing a `context.WithTimeout(..., 5*time.Second)` `select`. **On timeout, the goroutine is not canceled** — `errCh` is buffered (`chan error, 1`) precisely so the abandoned goroutine can still write to it without blocking forever, but the underlying `SendKeys` call itself keeps running in the background against a possibly-wedged session. This is a known, accepted leak-on-timeout pattern in this codebase (used identically in two places), not a bug specific to this feature — but a **third** call site (the new `SessionSteerer`) reusing the same shape multiplies the number of orphaned goroutines a single wedged session can accumulate if the reconciler retries every ~60s tick without an existing steer succeeding to clear the failure reason. |

**Concrete asymmetry to flag for planning**: the autonomous branch's error handling is
strictly weaker than the interactive branch's. If `SessionSteerer`'s implementation reuses
`UpdateSession`'s logic verbatim (as the requirements direct), and most backlog work
sessions run autonomous-mode Claude Code (the common case this feature targets), **the new
caller has no reliable signal of steer failure for the majority of its targets** — a `nil`
controller or `"controller not started"` result never reaches `AutoReopenForPRFix` as an
error. Since the requirements explicitly demand "every steer attempt (success or failure)
is visible... not just a log line," the planning phase needs to either (a) have
`SessionSteerer`'s new method return an explicit `error`/success bool for *both* branches
(fixing the existing autonomous-branch swallow as part of this work), or (b) accept the gap
and only guarantee visibility for the interactive branch — but that should be a stated,
reviewed decision, not an incidental inheritance from code this project was told not to
redesign ("Redesigning `steer_session`... is out of scope").

## 3. Race: `tombstoneOrphanWorkSessions` vs. a concurrent steer landing on the same session

`AutoReopenForPRFix` (`backlog_service_triage.go:2047`) always calls
`s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)` before checking
`findActiveWorkSession`. That tombstone sweep ends any work session whose
`sessionStopper.IsSessionLive` reports `false`, and then prunes its worktree
(`cleanupItemWorktrees`). Today, if the sweep tombstones a session, the very next line's
`findActiveWorkSession` correctly sees no active session and falls through to the
spawn-a-fresh-session path — no conflict, because "tombstoned" and "steer target" are
mutually exclusive branches of the *same* call.

The risk this feature introduces is **cross-tick**, not intra-call:
`IsSessionLive` is a point-in-time in-memory check (`session/services` tracks live
sessions in a poller). Between one reconcile tick's `tombstoneOrphanWorkSessions` finding a
session live (skip) and a **steer call actually reaching `SendKeys`/`SendCommandImmediate`
moments later**, nothing prevents:
- a different code path (a human killing the session, `StopSessionByUUID` from an
  unrelated flow, the tmux server itself dying) from tearing the session down
  concurrently, so the steer's `SendKeys` lands on a session that is mid-teardown — the
  observed failure mode is whatever `instance_tmux.go:1023`'s guard returns
  ("not started or is paused"), which is at least a clean error, not a corrupt write; but
- the *next* reconcile tick's own `tombstoneOrphanWorkSessions` call, if `ReconcilePRPending`
  ticks were ever allowed to overlap (see below), racing the current tick's steer attempt on
  the same `SessionUUID`.

There's no unit test named for this cross-tick interleaving today (only
`TestAutoReopenForPRFix_ActiveWorkSession_*`), so it's unverified whether the existing
tombstone-then-check sequence is safe against a session that dies **between** the liveness
check and the steer write within a single call — worth an explicit test:
steer-after-tombstone-flip, i.e. `IsSessionLive` returns true at check time, then the
underlying `SendKeys`/`SendCommandImmediate` call itself fails because the session
died in the intervening milliseconds.

## 4. Concurrency: no per-item serialization guards this new call path

`backlog_service.go`'s struct comments document two existing self-cleaning in-flight sets
purpose-built for exactly this class of bug:
- `spawnInFlight` (`backlog_service.go:139-165`) — guards `SpawnSessionFromItem`'s
  read-check-write sequence after a **confirmed live incident** (2026-07-19, item
  `d3227302` ended up with two literal overlapping "work" role `ItemSession`s because two
  concurrent spawn calls both read "no active work session" before either had inserted its
  row).
- `dequeueMu` (`backlog_service.go:131-137`) — serializes `DequeueNextQueuedItems`'s whole
  body because two independent unsynchronized callers (`onSessionExited`'s goroutine and
  the periodic sweep) could each compute `freeSlots` from a stale snapshot and jointly
  overshoot the WIP cap (PR #199 review F2).

**Neither guard covers a steer call**, and the requirements don't specify a new one. Given
`ReconcilePRPending` polls every ~60s, the realistic concurrency scenarios are:
- A slow tick (headless LLM triage, gh API latency, a large `syncPRBranchWithMain`) still
  running past 60s while the next tick starts — if `ReconcilePRPending` itself has no
  overlap guard (not confirmed in this pass; worth checking during planning whether it's
  called from a single-goroutine ticker or can genuinely overlap), two ticks could both
  read the same stale reason-signature state and both fire a steer for the same
  newly-changed reason before either records the "already delivered" state — a duplicate
  steer, not an unbounded spam loop (bounded by "how many overlapping ticks can pile up"),
  but still a real spam-of-one risk the requirements' dedup section doesn't explicitly
  guard against with a `LoadOrStore`-style primitive.
- A manual/UI-triggered steer (human uses the steer box) landing in the same PTY moments
  before or after the reconciler's automated steer — **not new** (the codebase already
  tolerates concurrent human writes today via the identical `SendKeys` call), but this
  feature is the first *automated, unattended, periodic* caller of it, which changes the
  probability distribution from "rare, human-initiated" to "routine, every ~60s while a
  condition persists."

**Recommendation for planning**: reuse the `spawnInFlight`/`triageInFlight` self-cleaning
`sync.Map` idiom (LoadOrStore-on-entry, Delete-via-defer-on-exit) for a new
`steerInFlight`, keyed by item ID (or by session UUID, whichever the dedup state is keyed
on) — cheap insurance against the exact double-write failure mode `spawnInFlight` was
built to close, and it's already the established idiom in this file rather than a novel
pattern reviewers would need to re-justify.

## 5. Worst case if dedup/idempotency is wrong

Per the requirements' own Risk Control section, a dedup bug is explicitly bounded: "at
worst, send one redundant steer message per reason change per item — not an unbounded spam
loop — because the dedup check gates every call." That bound holds **only if** the dedup
check itself is correctly gated per call (see §4) and per reason-signature transition, not
per tick. If the dedup key is computed from mutable state that can itself flip-flop (see
§6 below, `mergeable` staleness), the "one redundant message per reason change" bound can
be violated by the *reason* oscillating due to upstream API staleness rather than any bug
in this feature's own logic — worth stating explicitly in the plan that the bound assumes
a stable ground-truth signal, which `HasConflicts` specifically is not guaranteed to be.

PTY corruption from literally overlapping writes is not a concern for the `SendKeys` path
specifically — tmux's `send-keys` is a single call per invocation and this codebase's own
existing concurrent-write handling (multiple humans/tools writing to the same pane today)
hasn't surfaced pane corruption as a bug in this repo's history; the realistic "spam" harm
here is agent context/attention pollution (an autonomous agent seeing duplicate identical
instructions competing with its own reasoning), not corrupted terminal state.

## 6. `gh`'s `mergeable` staleness (cli/cli#9583) and steer-content freshness

`session/git/worktree_git.go:505-508`'s `PRStatus.HasConflicts` doc comment: `gh`'s
`mergeable` field "has been observed returning stale data (cli/cli#9583)," which is why
`HasConflicts` already checks **both** `mergeStateStatus == "DIRTY"` *and*
`mergeable == "CONFLICTING"` rather than trusting `mergeable` alone (Task 1.1.1d).

This partially mitigates but does not eliminate the risk for this feature specifically:
- Both signals are still sourced from the same underlying GitHub-computed merge state,
  which can lag actual branch content after a very recent push (the staleness window
  `cli/cli#9583` describes) — a dual-check doesn't help if *both* fields are stale at the
  same GitHub-side computation cycle, which is the exact failure mode of a mergeability
  cache rather than of `gh`'s field selection.
- **New risk this feature introduces that the existing dual-check use case doesn't have**:
  `GetPRStatus` today is read once per `ReconcilePRPending` tick to *decide whether to act*
  (spawn or skip); a stale `true`→`false`→`true` flap only matters if it changes the
  spawn/no-spawn decision, which is comparatively rare (conflicts don't typically resolve
  and reappear within a 60s window). The new steer path reads `GetPRStatus` on **every**
  tick to compute the reason-signature for dedup — a session actively pushing commits to
  resolve a conflict is exactly the scenario most likely to produce a stale "still
  CONFLICTING" read moments after the real conflict was resolved, because the fix session's
  own push is what would flip the signal, and it just happened. A stale read here means the
  agent gets steered with "you still have a conflict" the same tick it just cleared one —
  a confusing, contradictory message to inject into a session that just did the right
  thing, right after doing it.
- **Recommendation for planning**: don't derive "reason changed → steer immediately" purely
  from a single fresh `GetPRStatus` call at reason-signature-change time. Consider either
  (a) requiring the same new reason to be observed on two consecutive ticks before steering
  (a simple debounce, cheap given the 60s cadence), or (b) explicitly excluding
  `HasConflicts` transitions from *triggering* a steer within some short grace window after
  the active session's last observed push, to avoid steering an agent about a conflict it
  may have just fixed. This should be a stated design decision in the plan, not silently
  inherited from the existing dual-check's mitigation, which was designed for a different
  read pattern (single decision point, not continuous dedup key).

## Summary of what to explicitly design against

1. Any status-transition side effect must stay out of the active-session branch (repeat of
   the double-transition-churn incident) — the steer call itself must be side-effect-free
   with respect to backlog status.
2. The autonomous-branch steer path (`SendCommandImmediate`) currently swallows both a
   `nil` controller and a `"controller not started"` error as silent no-ops — this directly
   conflicts with the requirement that every attempt (success or failure) be visible, and
   must be fixed or explicitly scoped around, not inherited by assumption.
3. MCP-created/never-UI-opened sessions have a known, unfixed wiring gap
   (`docs/tasks/backlog-feature-improvement.md:480`) that will make steer attempts against
   them fail with "not started or is paused" — the reconciler will hit this class of target
   routinely, unlike the human-driven steer box.
4. No in-flight guard exists yet for steer calls; reuse the `spawnInFlight`/
   `triageInFlight` self-cleaning `sync.Map` idiom (keyed appropriately) rather than assume
   the ~60s tick cadence alone prevents overlap.
5. `SendKeys`'s goroutine-leak-on-timeout pattern (already accepted at two call sites) gets
   a third caller here; a wedged session steered on every tick can accumulate orphaned
   goroutines faster than the two existing call sites (human-paced) ever would.
6. `HasConflicts`'s upstream staleness (cli/cli#9583) is a live risk specifically for a
   continuously-polled dedup key, not just for the single-decision-point use case it was
   originally mitigated for — plan for a debounce or grace window rather than trusting a
   single fresh read at reason-change time.
