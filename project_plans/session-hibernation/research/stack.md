# Stack Research — Session Hibernation

## Memory monitoring
- `github.com/shirou/gopsutil/v3` is **already a direct dependency** (go.mod line 25), used in `session/procinfo/`.
- Use `mem.VirtualMemory().Percent` for threshold-based resource monitoring — zero additional dependencies, works on Linux and macOS.
- No need for `golang.org/x/sys/unix` syscalls or reading `/proc/meminfo` directly.

## Existing useful dependencies (go.mod)
- gopsutil v3 — memory/CPU
- stdlib `os/signal`, `syscall` — process signaling (SIGTERM/SIGKILL)
- `golang.org/x/sync` and `golang.org/x/time` — concurrency helpers
- `entgo.io/ent` — ORM for session status persistence

## Config integration
- Follow `config/config.go`'s `OneOffBaseDirOrDefault()` pattern (lines 388–434) for tilde expansion and lazy defaults.
- Add `HibernationConfig` struct with fields: `Enabled` (default true), `IdleTimeoutMinutes` (default 120), `MemoryThresholdPct` (default 85), `CheckpointDir` (default `~/.stapler-squad/checkpoints`), `RetentionDays`.
- Add `HibernationCheckpointDirOrDefault()` method following the same pattern.

## Status / ORM
- Session status stored as `int` field in ent (`session/ent/schema/session.go` line 34).
- Adding `Hibernated Status = 7` after `Stopped` in `session/instance.go` is straightforward.
- **Critical guards**: add `Hibernated` to the `Stopped` exclusion in `instance_serialization.go:329` and add early bailout in `health.go:130`.

## Scrollback
- `session/scrollback/` has `CircularBuffer.GetAll()` returning `[]ScrollbackEntry`.
- Terminal output is already persisted to `~/.stapler-squad/sessions/<sessionID>/scrollback.json[.zstd]` every 5 seconds by `ScrollbackManager`.
- On hibernation: copy or reference existing scrollback file rather than recapturing from tmux.
