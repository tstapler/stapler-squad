# State File Isolation and Multi-Instance Support

Hierarchical state file isolation prevents conflicts between tests, benchmarks, and multiple production instances. Implemented by `config.GetConfigDirForDir` (`config/config.go`) and mirrored exactly by `log.GetConfigDirForDir` (`log/log.go`), so log files always land next to the DB/session state they describe — see "log.GetConfigDir parity" below.

## Isolation Hierarchy (highest to lowest priority)

| Priority | Mechanism | State Location | Activation |
|---|---|---|---|
| 1 | Test directory override | `$STAPLER_SQUAD_TEST_DIR` | `STAPLER_SQUAD_TEST_DIR=<dir>` (used by `--test-mode` harnesses) |
| 2 | Explicit Instance ID | `~/.stapler-squad/instances/{INSTANCE_ID}/` | `STAPLER_SQUAD_INSTANCE=name` (`=shared` explicitly opts back into Priority 6) |
| 3 | Test Mode Auto-Detection | `~/.stapler-squad/test/test-{PID}/` | Automatic whenever the running binary is a `go test`/benchmark binary |
| 4 | Preferred Workspace (explicit switch) | Path recorded in `~/.stapler-squad/preferred_workspace` | Set via the `SwitchDatabase` RPC / workspace switcher UI |
| 5 | Per-Directory Workspace Isolation | `~/.stapler-squad/workspaces/{WORKSPACE_HASH}/` (SHA256 of cwd, first 8 bytes hex) | **Opt-in**, `STAPLER_SQUAD_WORKSPACE_MODE=true` (exact string match — `"1"`/`"TRUE"` do not activate it) |
| 6 | Global Shared State (default) | `~/.stapler-squad/` | Default — no env var or preference file set |

**Workspace isolation is opt-in, not the default.** A per-cwd auto-isolated workspace being the default previously caused sessions to silently "disappear" when the binary was started from an unusual cwd (e.g. a worktree). Priority 5 only activates with an explicit `STAPLER_SQUAD_WORKSPACE_MODE=true`; switching between workspaces is otherwise meant to be an explicit user action via the `SwitchDatabase` RPC (Priority 4), not a side effect of cwd.

## log.GetConfigDir parity

`log.GetConfigDirForDir` mirrors `config.GetConfigDirForDir` priority-for-priority, including Priority 4 (preferred workspace file) and Priority 5 (opt-in workspace mode) — both packages call the same shared, `log`-free leaf package (`config/workspacepath`) for that logic, so log files land in the same directory as DB/session state under every activation mode. This wasn't always true: prior to this fix, `log.GetConfigDir` implemented only Priority 1-2, so an opted-in workspace-mode or preferred-workspace user's logs stayed in the shared `~/.stapler-squad/logs/` directory while their DB/session state moved elsewhere. If you were relying on `STAPLER_SQUAD_WORKSPACE_MODE=true` or a `SwitchDatabase`-set preference before this fix, your log location has moved to match — check the new location via `config.GetConfigDir()`'s / `log.GetConfigDir()`'s return value (both now agree), not the old shared path.

## Common Usage Patterns

```bash
# Default: shared global state
./stapler-squad

# Opt in to per-directory workspace isolation
STAPLER_SQUAD_WORKSPACE_MODE=true ./stapler-squad

# Named instance (useful for project-specific state)
STAPLER_SQUAD_INSTANCE=work ./stapler-squad
STAPLER_SQUAD_INSTANCE=personal ./stapler-squad

# Explicit shared global state (same as the default, useful to override an
# ambient STAPLER_SQUAD_WORKSPACE_MODE/preference file)
STAPLER_SQUAD_INSTANCE=shared ./stapler-squad

# Tests: isolated automatically — no config needed
go test ./...
```

## Instance Identification in Logs

```
[work] INFO: Session created
[pid-12345-1704132000] INFO: Session started
[work][DAEMON] INFO: Polling sessions
```

## Migration Notes

- Existing `~/.stapler-squad/` state is preserved; the global shared directory remains the default unless workspace mode is explicitly opted into.
- Tests auto-detect isolation — no code changes needed.

## Troubleshooting

| Issue | Fix |
|---|---|
| "Can't find sessions after restart" | A preferred-workspace file or `STAPLER_SQUAD_WORKSPACE_MODE=true` is pointing at a different directory than expected; unset `STAPLER_SQUAD_WORKSPACE_MODE` or use `STAPLER_SQUAD_INSTANCE=shared` for directory-independent state |
| "Tests modifying production state" | Shouldn't happen; verify `go test` is used (binary names must contain `.test`) |
| "Multiple instances conflicting" | Use explicit `STAPLER_SQUAD_INSTANCE` per instance |
| "Want workspace isolation per directory" | `STAPLER_SQUAD_WORKSPACE_MODE=true` (opt-in, not the default) |
| "Logs are in a different directory than sessions/DB" | Should not happen — `log.GetConfigDir()` and `config.GetConfigDir()` are guaranteed to agree (see "log.GetConfigDir parity" above); if they diverge, it's a bug |
