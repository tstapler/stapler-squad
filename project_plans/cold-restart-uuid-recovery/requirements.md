# Requirements: Cold-Restart UUID Recovery Before Fresh-Start Decision

**Source**: Backlog item `f5c5a35e-b5f1-491d-81de-66e94028a085`, re-filed from
`TylerStaplerAtFanatics/stapler-squad#194` (deprecated repo). Re-verified against
this worktree's checkout (v1.41.0-equivalent, commit `be2c062` lineage) on 2026-08-10.

## Problem Statement

A session whose tmux pane died can be revived (`Start(firstTimeSetup=false)`) while
its in-memory `claudeSession.ConversationUUID` is empty — even though a valid Claude
conversation JSONL for that project path exists on disk. When this happens, the
session comes back as a **brand-new Claude with no conversation history** instead of
resuming, and the working conversation is silently discarded. The only signal is a
single `log.Warn` line.

## Root Cause (confirmed by code inspection, not yet fixed)

`session/instance.go`, `startLocked()`:

1. `i.initTmuxSession()` (line ~846) runs first. It reads `i.claudeSession.ConversationUUID`
   directly and bakes it into the launch command via `buildLaunchCommand()` —
   this is what actually decides whether `--resume <uuid>` is embedded in the
   `claude` invocation. See `session/instance_tmux.go:249-259`.
2. Only *after* the tmux session object is constructed does the `!firstTimeSetup`
   branch (line ~868, mirrored at ~1061) check `i.HasClaudeSession()` — but this
   check is now purely cosmetic (chooses which log line to print). The launch
   command was already fixed at step 1.
3. The one mechanism that *can* recover a UUID without a live process —
   `HistoryFileDetector.DetectByPath()` (`session/history_detector.go:131`), which
   scans `~/.claude/projects/<encoded-path>/` for the newest conversation JSONL and
   requires no live PID — is only invoked via `i.tryExtractConversationUUID()`
   **after** `Start()` has already launched the (already-decided) fresh process
   (`session/instance.go:910`, mirrored at `:1116`).

So the recovery infrastructure already exists in the codebase and already runs on
every cold restart — it just runs one step too late to influence the decision it
would need to influence. By the time it runs, the `claude` process is already up
without `--resume`.

## How a live session ends up with no captured in-memory UUID

The UUID is captured into `i.claudeSession` after a session starts (via
`tryExtractConversationUUID` / `HistoryLinker`). When a session is **restarted
repeatedly in a short window** (e.g. inactivity-timeout restarts, auto-hibernation),
a revive can fire before the UUID is (re)captured into memory, even though the JSONL
file for the *previous* run is already sitting on disk. Captured example from a live
v1.40.0 session (2026-07-27):

```
15:23  restarting session after failure  reason="inactivity timeout"
15:41  restarting session after failure  reason="inactivity timeout"
16:04  auto-hibernating idle session → tmux session killed
17:13  cold start: tmux dead, no conversation UUID, starting fresh   ← context lost
18:18  cold restoring with --resume  uuid=222e0eaa-...               ← later revive resumed fine
```

The same session cold-started fresh at 17:13 but resumed correctly at 18:18 — the
in-memory UUID capture is racy/inconsistent across restarts, not permanently absent,
which is what makes it recoverable via `DetectByPath` in principle.

## Existing related work (do not duplicate)

- `project_plans/session-resume-uuid-fix/` — fixed a *different* bug: `DetectByPath`
  overwriting a paused session's **correct** UUID with a newer, wrong one from a
  different session sharing the same directory. That fix protects the stored UUID
  from unwanted overwrites; it does not address firing `DetectByPath` too late to
  matter on cold restart.
- `project_plans/session-resumption-hardening/` — wired `HistoryLinker` into startup,
  added `session/instance_cold_restore_test.go`. That test file's
  `TestColdRestore_WithoutUUID` (`session/instance_cold_restore_test.go:99-142`)
  exercises exactly the "no UUID, tmux dead" cold-start path but only asserts the
  instance reaches `Running` — it does **not** assert whether a same-path JSONL
  history file would have been (or should have been) picked up. This is the coverage
  gap this item closes.

## Acceptance Criteria

1. On revive (`Start(firstTimeSetup=false)`) with a dead tmux pane and an empty
   in-memory `ConversationUUID`, if `HistoryFileDetector.DetectByPath()` finds a
   conversation JSONL for the session's effective root dir, that UUID MUST be used
   to build the `--resume` launch command — i.e. the recovery attempt must run
   *before* `i.initTmuxSession()` / `buildLaunchCommand()`, not after.
2. If recovery in (1) fails (no JSONL found, or lookup errors), the session MUST
   still start fresh (current fallback behavior preserved) — this is not a hard
   failure path.
3. When recovery in (1) fails but there is other evidence a conversation existed
   (e.g. a previously-known-but-now-stale `HistoryFilePath` was cleared, or the
   session was known to have run Claude before), the WARN log must remain
   observable, and a user-visible signal (session state/UI) should distinguish
   "started fresh because there was never a conversation" from "started fresh
   despite an apparent prior conversation" — at minimum, the log line must
   include enough context (path searched, whether any JSONL existed) to diagnose
   after the fact. Full UI surfacing is a stretch goal, not a hard requirement,
   scope this during planning.
4. A regression test must cover: tmux dead, in-memory UUID empty, a same-path
   conversation JSONL present on disk → revived session launches with
   `--resume <uuid-from-jsonl>`, not fresh. This closes the gap left by
   `TestColdRestore_WithoutUUID`.
5. No change to the behavior already fixed by `session-resume-uuid-fix` — a
   *paused* session with a correct stored UUID must still not have that UUID
   overwritten by a newer JSONL from a different session in the same directory.
6. `make quick-check` (build + test + lint) stays green.

## Out of Scope

- The related `driverInactivityTimeout` hardcoded-10-minute restart-churn issue
  (`session/session_driver.go`) that opens the window for UUID loss in the first
  place — worth a separate follow-up item, not fixed here.
- Full UI "conversation could not be resumed" banner — tracked as a stretch/nice-to-have
  suggestion, not a hard acceptance criterion (see AC3).
- Persisting the UUID more eagerly / earlier in the session lifecycle beyond what's
  needed to make `DetectByPath` run before the launch-command decision.
