# Architecture Research — Session Hibernation

## Session lifecycle
- Instances start at `Loading` (or `Ready`), then transition through the state machine defined in `session/state_machine.go`.
- `Stop()` / `Kill()` in `session/instance.go` send SIGTERM via `tmux kill-session` (see `instance_tmux.go`).
- `Pause()` removes the worktree and transitions to `Paused`; `Resume()` recreates the worktree and transitions back to `Running`.
- Hibernation should mirror Pause's structure but kill the AI process rather than the worktree.

## No existing idle sweeper
- `SessionHealthChecker` (`session/health.go`) provides the sweeper pattern (ticker loop, batch processing over all instances) but is not currently wired into server startup — only tested.
- Sessions track `LastTerminalUpdate` and `LastMeaningfulOutput` timestamps in `ReviewState`.
- Idle sweeper: follow the health checker ticker pattern, filter sessions where `time.Since(lastActivity) > idleTimeout && status == Active`, call `Hibernate()` on each.

## Scrollback is already persisted
- `ScrollbackManager` (`session/scrollback/manager.go`) auto-captures all terminal output to disk every 5 seconds (compressed with zstd by default).
- Stored at `~/.stapler-squad/sessions/<sessionID>/scrollback.json[.zstd]`.
- On hibernate: reference or copy the existing scrollback file — no re-capture needed.

## Checkpoint storage
- New dir: `~/.stapler-squad/checkpoints/<session-uuid>/`
  - `checkpoint.json` — subset of `InstanceData` (title, path, branch, status, timestamps)
  - `scrollback_ref.txt` — path to existing scrollback file, or copy it here
- `instance_serialization.go` already has `ToInstanceData()` — reuse for checkpoint.

## Integration point for sweeper
- Wire in `server/dependencies.go` after SessionService/Storage are created.
- Launch background goroutine: `go sweeper.ScheduledSweep(interval, ctx.Done())`.
- Same ticker pattern as `SessionHealthChecker` (~line 209).

## Process termination
- `KillSession()` in `instance_tmux.go:79` sends SIGTERM via `tmux kill-session`.
- For hibernation: call `KillSession()`, then spawn 10-second SIGKILL fallback timer.
- Write checkpoint to disk in parallel while waiting for graceful exit.
