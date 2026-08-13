# ADR-001: Guard Pre-Launch DetectByPath Recovery Against Resurrecting an Explicitly-Cleared Conversation

**Date**: 2026-08-10
**Status**: Accepted
**Context**: cold-restart-uuid-recovery — moving `HistoryFileDetector.DetectByPath()` recovery ahead of `initTmuxSession()` in `session/instance.go`'s `startLocked()`/`start()`

---

## Context

The core fix in this plan hoists `i.tryExtractConversationUUID()` — which falls back to
`DetectByPath()`'s "newest JSONL in the directory wins" heuristic when no live process is
available — to run *before* `initTmuxSession()` decides whether to embed `--resume <uuid>`
in the launch command, instead of only after the process has already (re)started.

The six SDD research files for this project (`project_plans/cold-restart-uuid-recovery/research/`)
correctly flagged that a *naive* version of this reorder — "always attempt recovery whenever
the in-memory UUID is empty" — risks reintroducing the class of bug `session-resume-uuid-fix`
already fixed once, and named three variants (pitfalls.md §1): two sessions racing on the same
directory, a stale/recycled worktree path, and `Instance.ClearConversationState()` being
defeated. The first two are pre-existing, best-effort-accepted risks (see Pattern Decisions in
plan.md) not solved by this ADR. This ADR addresses the third — `ClearConversationState()` —
because a closer read of the code (beyond what the research files traced) shows it is not a
rare hypothetical but a **deterministic, already-wired trigger path** in this exact codebase.

## The concrete trigger path

1. `session/instance_claude.go:78-96`, `recoverFromStaleResume()`: when a `claude --resume
   <uuid>` process exits with "No conversation found with session ID" (detected by
   `isStaleResumeExit`, wired from `session/instance_controller.go:71-72`'s PTY-EOF callback),
   this function calls `i.ClearConversationState()` (clearing `ConversationUUID` and
   `HistoryFilePath`) and then `i.Start(false)`.
2. That specific `Start(false)` call does **not** take the cold-restore branch. `tmux`'s
   `remain-on-exit` option (`session/tmux/tmux.go:460-466,872-876`) keeps the tmux session
   object alive as a "Pane is dead" placeholder after the wrapped `claude` process exits —
   confirmed via `TmuxSession.DoesSessionExist()` (`session/tmux/tmux.go:1846`), which checks
   session existence, not pane liveness. So `i.pm().IsAlive()` is still `true`, and `Start(false)`
   takes the *hot*-restore branch (`session/instance.go:911-923`/`:1117-1138`), which only calls
   `RestoreWithWorkDir` — it does not relaunch the program.
3. `session/health.go:203-247`'s `CheckAllSessions()` is the code that actually detects and
   recovers this state (`instance.PaneProcessDead()`), and its own comment
   (`session/health.go:219-225`) spells out exactly why: *"Start(false) treats an existing
   (even dead-paned) tmux session as already running and just reattaches to it... Killing it
   first forces Start(false) down the cold-restore path that actually recreates the session and
   relaunches the program."* It calls `instance.KillSession()` then `instance.Start(false)`.
4. **This second `Start(false)` call (from health.go) does take the cold-restore branch** —
   `KillSession()` tears the tmux session down for real, so `i.pm().IsAlive()` is `false`. At
   this point: the in-memory `ConversationUUID` is still `""` (cleared in step 1, never
   re-populated because step 2 never actually relaunched `claude`), and the *old, stale, just-
   rejected* JSONL is still sitting on disk as the newest file in
   `~/.claude/projects/<encoded-path>/` (nothing has written there since).

Without a guard, this plan's new pre-launch `DetectByPath` call, running inside that second
`Start(false)`, would find that same stale JSONL — it's the newest (only) candidate — and embed
`--resume <same-stale-uuid>` into the relaunch command. Claude would reject it again with the
identical "No conversation found" error, re-triggering `recoverFromStaleResume`, which clears and
restarts again, which health.go recovers again... a self-sustaining failure loop, entirely
self-inflicted by this fix's own new call, with no external trigger required beyond the original
stale-resume event.

## Why this is not the same as the shared-directory carve-out

`session-resume-uuid-fix`'s accepted carve-out (a different, unlinked session's newer JSONL in a
shared directory) requires a coincidence: two sessions, same directory, timing overlap, no
guarantee it ever happens in practice. The `ClearConversationState` case requires none of that —
`recoverFromStaleResume` and `health.go`'s dead-pane recovery are both existing, always-on
mechanisms in this codebase, and the trigger (a stale `--resume` UUID) is exactly the scenario
`ClearConversationState` was written to prevent from recurring. Treating this as "same shape,
also accept" would mean this fix ships a change that measurably worsens a real recovery path.

## Decision

Add a single unexported field, `conversationClearedAt time.Time`, to `Instance`
(`session/instance.go`), set to `time.Now()` inside `ClearConversationState()`'s existing
`claudeSessionMu` critical section (`session/instance_claude.go`). `HistoryFileInfo` gains a
`ModTime time.Time` field, populated by `DetectByPath` (already computes mtime internally for
sorting; `Detect()`'s PID-based fast path leaves it zero since it needs no mtime gating — it's
process ground truth). `tryExtractConversationUUID()`'s `DetectByPath` fallback branch discards
any candidate whose `ModTime` does not postdate `conversationClearedAt`, logging at DEBUG so the
"predates an explicit clear" case is diagnosable and distinct from "no JSONL ever existed."

A JSONL written *after* the clear (a genuinely new conversation, later interrupted again) is
still recovered normally — the guard is a one-sided freshness check, not a permanent recovery
disable.

## Consequences

- Closes the concrete `recoverFromStaleResume` + `health.go` dead-pane-recovery loop described
  above, and generically protects any other current or future caller of `Start(false)` from the
  same class of problem.
- `conversationClearedAt` is **in-memory only, not persisted**. If the `stapler-squad` process
  itself restarts between `ClearConversationState()` and the session's next cold restart (before
  a new post-clear conversation is ever established), the guard is lost and the pre-launch
  recovery could resurrect the stale conversation once. This is judged acceptable: the concrete,
  evidenced regression this ADR closes is a same-process, same-boot loop; the cross-process-
  restart variant requires a narrower coincidence (process restart landing in that specific
  window) and degrades to the same already-accepted "best effort" character as the shared-
  directory carve-out, rather than a guaranteed loop. Persisting the field would require
  `session/ent` schema changes (`--feature sql/upsert` regenerate) — out of proportion for this
  fix; tracked as a named follow-up in plan.md's Risk Control section, not a blocker.
- `session/history_linker.go`'s `correlateSession()` has its own, independent `DetectByPath` call
  that is not covered by this guard (it doesn't influence the `--resume` launch decision, so it's
  outside this ADR's scope) — also tracked as a named follow-up.

## Alternatives Considered

- **Do nothing / fold into the same accepted carve-out as the shared-directory case**: Rejected
  — the trigger path is deterministic and already wired in this codebase (see above), not a rare
  coincidence; shipping this fix without the guard would be a net regression for stale-resume
  recovery, not a neutral pre-existing limitation.
- **Persist the guard via a new `session/ent` schema field**: Rejected — the concrete regression
  this ADR closes is same-process; a schema migration is disproportionate scope for that, and the
  residual cross-process-restart gap is a strictly narrower, already-accepted-shaped risk.
- **Scope pre-launch recovery to worktree-mode sessions only** (on the theory that worktree paths
  are less likely to be shared/reused): Rejected — does not address this specific risk at all,
  since `recoverFromStaleResume` applies equally to directory-mode sessions, and would silently
  narrow AC1 (the actual bug being fixed) for the majority-case directory-mode sessions.
- **Delete the stale JSONL file instead of gating on it**: Rejected — deleting Claude's own
  conversation history file as a side effect of an unrelated Go-side bug fix is a more invasive,
  higher-blast-radius change than a timestamp comparison, and Claude's on-disk JSONL format/
  lifecycle is explicitly out of this repo's control (build-vs-buy.md).
