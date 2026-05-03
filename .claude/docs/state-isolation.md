# State File Isolation and Multi-Instance Support

Hierarchical state file isolation prevents conflicts between tests, benchmarks, and multiple production instances.

## Isolation Hierarchy (highest to lowest priority)

| Priority | Mechanism | State Location | Activation |
|---|---|---|---|
| 1 | Explicit Instance ID | `~/.stapler-squad/instances/{INSTANCE_ID}/` | `STAPLER_SQUAD_INSTANCE=name` |
| 2 | Test Mode Auto-Detection | `~/.stapler-squad/test/test-{PID}/` | Automatic when running `go test` |
| 3 | Workspace-Based (default) | `~/.stapler-squad/workspaces/{WORKSPACE_HASH}/` | Default (SHA256 of cwd) |
| 4 | Global Shared State | `~/.stapler-squad/` | `STAPLER_SQUAD_WORKSPACE_MODE=false` |

## Common Usage Patterns

```bash
# Default: per-directory workspace isolation
./stapler-squad

# Named instance (useful for project-specific state)
STAPLER_SQUAD_INSTANCE=work ./stapler-squad
STAPLER_SQUAD_INSTANCE=personal ./stapler-squad

# Shared global state (legacy behavior)
STAPLER_SQUAD_INSTANCE=shared ./stapler-squad
STAPLER_SQUAD_WORKSPACE_MODE=false ./stapler-squad

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

- Existing `~/.stapler-squad/` state is preserved; workspace isolation is now the default.
- To restore old shared behavior: `STAPLER_SQUAD_INSTANCE=shared` or `STAPLER_SQUAD_WORKSPACE_MODE=false`.
- Tests auto-detect isolation — no code changes needed.

## Troubleshooting

| Issue | Fix |
|---|---|
| "Can't find sessions after restart" | Different directory active; use `STAPLER_SQUAD_WORKSPACE_MODE=false` for directory-independent state |
| "Tests modifying production state" | Shouldn't happen; verify `go test` is used (binary names must contain `.test`) |
| "Multiple instances conflicting" | Each workspace isolated by default; use explicit `STAPLER_SQUAD_INSTANCE` |
| "Want shared state across directories" | `STAPLER_SQUAD_INSTANCE=shared` or `STAPLER_SQUAD_WORKSPACE_MODE=false` |
