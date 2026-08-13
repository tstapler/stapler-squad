# Configuration Reference

stapler-squad stores its configuration in `~/.stapler-squad/config.json`. The file is created automatically on first run with sensible defaults. You can edit it directly or through the **Settings → Config Files** tab in the UI.

## Top-Level Options

### `defaultProgram`

The agent program to use when creating a new session without explicitly selecting one.

```json
{ "defaultProgram": "claude" }
```

Common values: `"claude"`, `"aider"`, `"custom"`.

### `defaultPath`

The default working directory pre-filled in the session creation form.

```json
{ "defaultPath": "~/code" }
```

### `oneOffBaseDir`

The base directory for one-off sessions. stapler-squad creates auto-named subdirectories here for one-off sessions.

```json
{ "oneOffBaseDir": "~/.stapler-squad/one-off" }
```

If not set, defaults to `~/.stapler-squad/one-off/`.

## Instance Naming

stapler-squad supports running multiple isolated instances. Each instance has its own state (sessions, config, logs) under `~/.stapler-squad/<instance>/`.

To run a named instance:

```bash
STAPLER_SQUAD_INSTANCE=work ./stapler-squad
```

The default instance name is derived from the current git repository root (workspace-based isolation), so two projects get separate session lists automatically. To share state across all projects, use `STAPLER_SQUAD_INSTANCE=shared`.

## Workspace Mode

By default (`STAPLER_SQUAD_WORKSPACE_MODE=true`), stapler-squad scopes sessions to the current git repository. To disable this and use a single global session list:

```bash
STAPLER_SQUAD_WORKSPACE_MODE=false ./stapler-squad
```

## Server Configuration

The web server listens on `localhost:8543` by default. This is not configurable via `config.json` — use the `--port` flag if you need a different port.

## Logs and State Files

| Path | Contents |
|---|---|
| `~/.stapler-squad/logs/stapler-squad.log` | Main application log |
| `~/.stapler-squad/sessions.json` | Persisted session state |
| `~/.stapler-squad/worktrees/` | Git worktrees for isolated sessions |
| `~/.stapler-squad/config.json` | User configuration |

## Editing Configuration in the UI

The **Settings → Config Files** tab provides a Monaco editor for `CLAUDE.md` and `settings.json`. Changes are saved immediately. For `config.json` itself, edit the file directly and restart the server.
