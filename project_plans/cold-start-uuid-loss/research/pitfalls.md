# Pitfalls Research: cold-start-uuid-loss

## 1. Wrong-conversation resumption is worse than fresh-start — design against it explicitly

`DetectByPath` (`session/history_detector.go:137-199`) has **no attribution check** beyond
"newest `.jsonl` in this project's encoded directory." It does not correlate the file against
this session's previously-known UUID, git branch/worktree identity, or any session-scoped
marker — it just does `sort.Slice` by `modTime` and returns `candidates[0]`
(`session/history_detector.go:190-193`).

Today this runs *after* the fresh-start decision, purely as best-effort enrichment
(`session/instance.go:921`, `:1127`) — a wrong pick there just mislabels metadata post-hoc. The
requirements doc's own success metric ("attempt recovery before deciding to start fresh") turns
this into a **load-bearing gate for which `--resume <uuid>` gets passed to the live `claude`
process**. A wrong pick is no longer cosmetic — it silently attaches the session to a
*different* conversation's history:

- **Worktree path reuse**: a worktree directory is torn down and a new session created at the
  same path (or same `ClaudeProjectDirName` encoding) before Claude's own project directory is
  garbage-collected. `DetectByPath` will happily return the newest JSONL, which now belongs to
  an unrelated conversation. The user gets what looks like a successful resume, but it's someone
  else's (or their own prior, now-irrelevant) context — actively misleading rather than merely
  losing history.
- **Concurrent sessions sharing a path** (e.g. two `SessionTypeDirectory` instances pointed at
  the same folder, or a session and a manually-run `claude` in the same dir) — `modTime` order
  says nothing about *which* session's conversation is "the" one to resume.
- Losing a conversation (fresh start) is a **data-loss** failure the user can notice ("agent
  forgot everything") and recover from by re-explaining context. Resuming the *wrong* one is a
  **data-corruption/confusion** failure that may not be noticed at all — the agent has full,
  plausible-looking context, just for the wrong task/branch. Requirements.md's Rabbit Holes
  section already flags this ambiguity; treat it as the primary risk to design against, not a
  secondary detail. **Recommendation for Phase 3 planning**: gate promotion of a path-recovered
  UUID to "used for `--resume`" on some confidence signal beyond recency — e.g. only trust it
  when there is exactly one candidate file, or when the instance has a previously-recorded UUID
  to cross-check the file's own session-start metadata against (JSONL first lines typically
  encode the working directory / cwd) — rather than blindly taking `candidates[0]`.

## 2. TOCTOU between "found UUID via path scan" and "actually start --resume with it"

`startLocked`'s cold-restore branch (`session/instance.go:878-921`) builds `startPath`, decides
resume-vs-fresh, *then* calls `i.pm().Start(startPath)` — and per the file-level doc comment on
`initTmuxSession`/`ClaudeCommandBuilder`, the `--resume` flag is baked into the tmux program
command string *before* the process launches. If the plan moves `DetectByPath` in front of the
decision, there is a window between "scan disk, pick UUID" and "process actually launches with
`--resume <uuid>`" during which:

- A concurrent `HistoryWatcher`/`HistoryLinker` goroutine (`session/history_linker.go:316`, via
  `inst.SetHistoryInfo`) could rewrite `i.claudeSession.ConversationUUID` out from under the
  just-computed value if it's watching the same project directory and picks up a newer file
  written in that window (e.g. another instance's process is still flushing to the same
  `.claude/projects/<dir>/` because of the same path-reuse scenario in Finding 1).
  `SetHistoryInfo` takes `claudeSessionMu.Lock()` (session/instance_claude.go:465) — a
  **different** lock discipline than the actor-command code that would be doing the early scan
  (see Finding 3) — so this isn't just a logical race, it's a genuine unsynchronized write race
  on the same field from two different locking protocols.
- The Claude CLI itself, once launched with `--resume <uuid>`, can independently decide the UUID
  is stale/invalid (see `isStaleResumeExit`/`recoverFromStaleResume`,
  `session/instance_claude.go:17-96`) — i.e. the codebase already has a *second*,
  process-launch-time verification step for exactly this class of problem (a UUID that looked
  valid at decision-time turns out not to be). Any new pre-decision path scan should compose
  with this existing safety net, not duplicate or bypass it — e.g. don't remove
  `recoverFromStaleResume` on the assumption the new upfront scan makes it redundant; the two
  race windows (disk-scan-time vs process-launch-time) are different.

## 3. Actor/mailbox concurrency: blocking filesystem scan inside an actor command

`session/actor.go` documents `runActor` as **single-goroutine confinement**: `startLocked` and
other "Locked" twins mutate `Instance` fields directly, relying on the fact that only the actor
goroutine executes commands from `li.mailbox` one at a time (`session/actor.go:121-135`). This
buys freedom from `i.mu` for actor-internal state, but it also means:

- **A blocking `os.ReadDir` + per-entry `os.Stat`/`Info()` call (what `DetectByPath` does)
  blocks the entire actor mailbox for that Instance** while it runs. Every other queued command
  for that session — status queries, `sendSync`/`sendSyncErr` callers, lifecycle transitions —
  stalls until the scan returns. Today this cost already exists (`tryExtractConversationUUID`
  is called at `session/instance.go:921`/`:1127`, also on the actor goroutine, also blocking),
  so per the requirements doc's own NFR this isn't a *new* cost — but moving the call earlier
  changes it from "runs once, after the process is already up and doing useful work" to "runs
  before the process launches, directly extending the actor-blocked window that gates
  `Start()`'s return to `sendSync`/`sendSyncErr` callers." Any caller doing `i.Start(false)`
  synchronously (e.g. `recoverFromStaleResume`, `session_driver.go`'s restart path at line 541)
  now waits longer for the same actor-serialization reason.
- **`tryExtractConversationUUID`'s own doc comment is a trap for a "call it earlier" refactor**:
  "IMPORTANT: This method assumes stateMutex is already held by the caller. It must NOT be
  called without the lock" (`session/instance_claude.go:302-304`). But `startLocked` calls it
  today with **no explicit `claudeSessionMu` lock at all** — it relies on actor confinement
  instead, and the comment is stale/misleading relative to how the actor-safe caller actually
  works. A researcher or implementer reading only that comment could "fix" the reordering by
  adding a `claudeSessionMu.Lock()` around the earlier call site — which would be wrong twice
  over: (a) unnecessary inside actor confinement, since no other actor-goroutine is competing,
  and (b) insufficient against the *real* competing writers, which are the non-actor direct
  mutators (`SetClaudeConversationUUID`, `SetHistoryInfo`, `ClearConversationState`) that
  already use `claudeSessionMu` from arbitrary goroutines. Taking `claudeSessionMu` inside the
  actor command would at least close that specific race, but doing so requires care around the
  `i.mu`-nested-inside-`claudeSessionMu` order documented in `ClearConversationState`'s comment
  (`session/instance_claude.go:280-295`) and `SetHistoryInfo`'s (`:465-499`) — **that is the
  only lock order used anywhere for these two locks; a reordering fix must not invert it.**
- **Pre-existing, unrelated-to-this-fix data race, worth flagging even if out of scope to fix
  here**: `startLocked`'s current direct read `i.claudeSession.ConversationUUID` at
  `session/instance.go:882` (log line) and `tryExtractConversationUUID`'s direct writes to
  `i.claudeSession.ConversationUUID`/`i.HistoryFilePath` (`session/instance_claude.go:357-361`)
  are **unsynchronized** against `SetHistoryInfo`/`SetClaudeConversationUUID`, which mutate the
  identical fields under `claudeSessionMu` from non-actor goroutines (e.g. `HistoryLinker`,
  `RunWithResume`). This is a real `go test -race`-detectable hazard today, not a hypothetical —
  the actor-confinement contract and the `claudeSessionMu`-protected contract both claim
  ownership of the same field via two different, non-composing protocols. Moving *more* logic
  (the path scan and its UUID assignment) earlier into the hot, more-frequently-hit branch of
  `startLocked` **increases the window of exposure** for this race rather than introducing a new
  one. Phase 3 should decide explicitly whether closing this is in-scope (e.g. route
  `SetHistoryInfo`/history-linker writes through the actor mailbox instead of `claudeSessionMu`,
  or take `claudeSessionMu` inside the actor command too) or flag it as a named, deliberately
  deferred pre-existing issue — don't let the fix silently widen it without comment.

## 4. Double-checked-locking analogue: don't re-read the slot after computing the value

This repo's own rule (`.claude/rules/go-double-checked-locking.md`,
`.claude/docs/concurrency-patterns.md`) — "in a read → miss → compute → write → conditional
store pattern, always return the locally-computed value, not the cache slot" — has a direct
analogue here. If the fix's new pre-decision recovery step looks like:

```go
// WRONG shape (analogous to the documented anti-pattern)
i.tryExtractConversationUUID()  // may or may not set i.claudeSession.ConversationUUID
if i.HasClaudeSession() {       // re-reads i.claudeSession under claudeSessionMu.RLock()
    resume(i.claudeSession.ConversationUUID)
}
```

the second read of `i.HasClaudeSession()`/`i.claudeSession.ConversationUUID` after
`tryExtractConversationUUID()` returns is reading the "slot" again rather than trusting what the
just-completed detection actually found. Given Finding 3's race (a concurrent
`SetHistoryInfo`/`ClearConversationState` call between the write inside
`tryExtractConversationUUID` and the re-read), this reintroduces exactly the class of bug the
existing rule calls out: the value acted upon may not be the value this goroutine (actor
command) just computed. **Recommendation**: have the detection helper *return* the
`*HistoryFileInfo`/UUID it found (or a clear "not found" signal) rather than mutating shared
state and expecting the caller to re-read it; branch on the local return value directly, and
only write it into `i.claudeSession` once, at the point of use. `tryExtractConversationUUID`'s
current `void`-return, mutate-in-place shape actively encourages the wrong pattern — plan to
either give it a return value or make the caller trust its own already-in-hand result rather
than re-deriving via `HasClaudeSession()`.

## 5. Restart-churn interaction (`driverInactivityTimeout`) — this fix can mask, not close, the real gap

`session/session_driver.go`'s inactivity watchdog (`driverInactivityTimeout = 10 * time.Minute`,
line 46) restarts a session that appears stuck, via `inst.RecoverFromStopped()` +
`inst.Start(false)` (lines 538, 541) — a **live, in-process** restart, not a cold (tmux-dead)
restart. This is explicitly out of scope per requirements.md, but two interactions are worth
naming so Phase 3 doesn't accidentally treat this fix as having closed the whole problem class:

- The bug as described only triggers on the **cold-start branch** (`!i.pm().IsAlive()`,
  `session/instance.go:879`/`:1069`) — a live-restart cycle through the driver's inactivity path
  does **not** go through this branch at all (tmux is still alive), so the driver's own restarts
  are not the direct trigger of the observed bug. The requirements doc is correct to list it as
  "related but separately scoped," specifically as a contributor to the *frequency* of cold
  restarts happening in the first place (reboots, `tmux kill-server`, service restarts,
  hibernation) — the driver doesn't cause cold restarts, but a session that restarts often for
  any reason (churny inactivity behavior, flaky driver logic, repeated crash-loop via
  `recoverFromStaleResume`) has more opportunities to land in the racy window where the
  in-memory UUID hasn't been (re)captured yet before the *next* cold event.
- **Risk to call out explicitly**: if this fix's path-based recovery becomes reliable enough
  that fresh-starts "silently" stop looking like a problem (recovery papers over the missing
  UUID every time), it removes the visible symptom that would otherwise motivate someone to
  later investigate *why* the in-memory UUID keeps going missing between restarts in the first
  place (e.g. whether `ClearConversationState` is being called more aggressively than intended,
  or whether some restart path never re-captures at all). The durable "started fresh" marker
  required by the success metrics is exactly the right guard against this: it must fire
  distinctly from "resumed via recovered UUID" (per the Observability Requirements' three-way
  event split), so a maintainer looking at event history later can still see recovery-path usage
  trending up even though the user never sees a fresh-start. Don't let "recovery worked" and
  "recovery wasn't needed" collapse into the same signal.

## Summary of concrete design constraints for Phase 3

1. Do not treat "found a JSONL via `DetectByPath`" as sufficient on its own to trust it for
   `--resume` when it's now load-bearing rather than best-effort — add a confidence/ambiguity
   check (single-candidate, or cross-check against any previously-known UUID/path metadata)
   before promoting it to "safe to auto-resume."
2. Preserve the existing process-launch-time stale-resume safety net
   (`recoverFromStaleResume`/`isStaleResumeExit`) as a second line of defense — the new
   pre-decision scan and this existing post-launch check guard different race windows and both
   are needed.
3. Any new write into `i.claudeSession` from inside an actor command must reconcile with the
   `claudeSessionMu`-protected non-actor writers (`SetClaudeConversationUUID`, `SetHistoryInfo`,
   `ClearConversationState`) — decide explicitly whether to take `claudeSessionMu` inside the
   actor command (respecting the `i.mu`-nested-inside-`claudeSessionMu` order) or route the
   non-actor writers through the mailbox; don't leave the two protocols racing on the same
   fields, and don't widen the existing exposure window without at least naming it.
4. Prefer a detection function that returns its result for the caller to branch on immediately,
   rather than mutating shared state and having the caller re-read it — avoids the
   double-checked-locking-style bug this repo already has a written rule against.
5. Keep the three-way structured event (resumed via known UUID / resumed via recovered UUID /
   started fresh) genuinely distinct in the implementation, not collapsed — it's the
   observability signal that keeps a successful band-aid from hiding the deeper restart-churn
   question called out in the Baseline/Users sections of requirements.md.
