# A Terminal One-Shot Session Can Be Revived and Respawned Forever

A one-shot backlog session (`backlog:triage` or `backlog:review` tag — a single non-interactive `claude -p --resume ... --output-format json` invocation) that reaches `Stopped` status can get silently flipped back to `Active` and relaunched, over and over, roughly every 60 seconds, forever — even though the session has nothing left to do and its backlog item may already be archived.

## Evidence (VERIFIED, 2026-10-09)

Backlog item `ce71ad1a-a6a5-485f-8245-c5a502754a8b` (status `archived`) had a `backlog:review`-tagged session (`review:ce71ad1a`) that respawned 230+ times across several hours, visible in `~/.stapler-squad/workspaces/<hash>/logs/*.log.gz` as a repeating cycle:

```
tmux new-session command succeeded → tmux session created successfully (re-launches `claude --resume <uuid> -p --output-format json ...`)
found existing tmux session, will reattach to preserve history
attach-session process exited (exitErr: "exit status 1")
reconcileSessions: stopped session's pane exists but wrapped program has exited, leaving Stopped
successfully killed tmux session
# ...~30s later, repeat from the top
```

A `sessions.db` query confirmed `archived_at IS NULL` on the session row despite its backlog item being archived, and `item_sessions` had no link row for it at all — this session predated the `item_sessions` linkage that the periodic `reconcileTerminalItemSessions` safety-net sweep (`session/backlog_lifecycle_archive.go`) relies on to retire terminal items' sessions, so that sweep never saw it.

## Root cause

`session/instance_serialization.go`'s `fromInstanceData` has a Stopped-branch probe: if a session is stored as `Stopped` but its tmux pane still looks alive (`IsAlive() && !paneExited` — remain-on-exit keeps a dead pane around as a placeholder, so this alone doesn't mean real work is happening), it flips the session back to `Active` and calls `Start(false)`. This probe already skipped archived sessions (`ArchivedAt != nil`), but had no equivalent skip for one-shot sessions.

`session/session_driver.go`'s `handleStoppedStatus` already treats a one-shot session reaching `Stopped` as terminal — `isOneShot(inst)` (true for `backlog:triage`/`backlog:review` tags) short-circuits straight to "driver exits cleanly, BacklogLifecycleListener handles this," with no retry. The serialization-layer probe directly contradicted that decision: every `LoadInstances()` call (the 15s health-check tick, most MCP tools, many RPC handlers) re-evaluated the dead-looking-but-technically-alive pane and revived it, re-running the same one-shot command, which exits almost immediately and gets killed again on the next tick — repeat indefinitely.

## Fix

`fromInstanceData`'s skip condition now reads `instance.ArchivedAt != nil || isOneShot(instance)` — the same tag-based check the driver already uses, so the fix holds even for sessions (like this one) with no `item_sessions` link row and a nil `ArchivedAt`. See `session/instance_serialization.go` and the regression test `TestFromInstanceData_should_NotReviveStoppedToActive_When_OneShotSessionHasLiveTmuxPane` (`session/instance_cold_restore_test.go`), which starts a real tmux session (not a mock) to exercise the actual `IsAlive()`/`PaneExitStatus()` probe.

## What to check if this recurs

```bash
# Find sessions respawning on a ~60s cadence:
grep "tmux new-session command succeeded" ~/.stapler-squad/workspaces/*/logs/staplersquad.log | sort | uniq -c -f1 | sort -rn | head

# Confirm a suspect session's archival/tag state:
sqlite3 -readonly ~/.stapler-squad/workspaces/<hash>/sessions.db \
  "SELECT title, status, one_shot, archived_at FROM sessions WHERE title='<session-title>';"
```

If `one_shot=1` and `archived_at` is `NULL` while the backlog item is already `done`/`archived`, this is the same class of bug reappearing somewhere else in the revival/reconcile paths — check every `ArchivedAt`-based skip (`session/health.go`, `session/instance_serialization.go`, `session/review_queue_poller.go`) for a missing `isOneShot` (or equivalent terminal-status) companion check.
