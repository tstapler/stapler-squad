# Research: Stack — Go Backend End-to-End Session Creation

## Summary

Directory sessions flow from the RPC request through a service function into `NewInstance` + `Start`. Path validation is minimal (required but not existence-checked in CreateSession). Config is JSON, adding fields is straightforward.

---

## 1. End-to-End: Directory Session Creation

### RPC Entry Point

**File**: `server/services/session_service.go`, `func (s *SessionService) CreateSession(...)` (line 500)

Steps:
1. Validate `title` and `path` are non-empty (lines 506–511).
2. Check for duplicate titles by scanning `storage.ListInstanceData()` (lines 513–524).
3. Optionally resolve GitHub URLs to local paths (lines 526–548).
4. Resolve session defaults (global → directory → profile) unless `skip_defaults=true` (lines 550–568).
5. Determine `sessionType` from `req.Msg.SessionType` (explicit) or infer from fields (branch/existing_worktree) (lines 570–592).
6. Build `session.InstanceOptions{...}` (lines 594–608).
7. Call `session.NewInstance(instanceOpts)` (line 623).
8. Call `instance.Start(true)` — `true` = first-time setup (line 630).
9. Inject Claude hook config (line 638, non-fatal).
10. Call `s.storage.AddInstance(instance)` (line 646).
11. Notify review queue poller and publish `SessionCreatedEvent` (lines 655–663).
12. Return `CreateSessionResponse` (lines 665–667).

### `NewInstance` (session/instance.go, line 601)

- Expands `~` prefix in `opts.Path` using `os/user.Current()`.
- Converts to absolute path via `filepath.Abs`.
- Defaults `sessionType` to `SessionTypeDirectory` if empty.
- Validates `sessionType.IsValid()`.
- Constructs `*Instance` with all opts fields.
- Calls `uuid.New().String()` for the session UUID.
- Returns `(*Instance, error)` — does **not** create the directory; only validates the type.

### `Start(firstTimeSetup=true)` → `start()`  (instance.go, line 869)

- Calls `initTmuxSession()` — creates the tmux session object (not started yet).
- Calls `setupFirstTimeWorktree()` (line 902–906).

### `setupFirstTimeWorktree` (instance.go, line 1062)

For `SessionTypeDirectory` (the default case, line 1088):
```go
default: // SessionTypeDirectory and unknown types → no worktree
    log.InfoLog.Printf("Directory session for instance '%s' at '%s' (no git worktree)", i.Title, i.Path)
    i.gitManager.SetWorktree(nil)
    i.Branch = ""
```
No git operations at all. No `os.Mkdir` or path existence check.

Then `tmuxManager.Start(startPath)` is called — `startPath` is `i.resolveStartPath(i.Path)`, which checks `os.Stat(startPath)` and falls back to basePath if the resolved path doesn't exist.

**Critical finding**: For `SessionTypeDirectory`, tmux is started with `startPath = i.Path`. If `i.Path` does not exist, tmux will start but the shell `cd` will fail silently (tmux still starts in some fallback dir). There is **no pre-flight `os.Stat` check on `i.Path`** in `CreateSession` or `NewInstance`.

---

## 2. Path Validation

Current path validation in `CreateSession`:
- `path is required` (empty string check only, line 509–511).
- No existence check, no directory check.

`NewInstance` does:
- Tilde expansion.
- `filepath.Abs` conversion.
- Type validation.
- No `os.Stat`.

For one-off sessions, the generated directory must be **created before** `instance.Start(true)` is called, because `resolveStartPath` falls back to `basePath` if the path doesn't exist — but more importantly, we want Claude to actually start in the correct directory.

**Where to create the directory**: In `CreateSession`, after building `instanceOpts.Path` and before calling `session.NewInstance(instanceOpts)`, call `os.MkdirAll(resolvedPath, 0755)`.

---

## 3. Config Struct — Adding `one_off_base_dir`

**File**: `config/config.go`

The `Config` struct (line 181) is a flat Go struct serialized as JSON. Fields are added simply:

```go
// OneOffBaseDir is the base directory for one-off session directories.
// Default: "~/oneoff". Created automatically if it doesn't exist.
OneOffBaseDir string `json:"one_off_base_dir,omitempty"`
```

**Persistence**: `SaveConfig(config)` → `saveConfig(config)` writes atomically via temp-file rename to `<configDir>/config.json` (line 464–495).

**Loading**: `LoadConfigFromPath` reads and unmarshals JSON (line 505–536). New fields with `omitempty` return zero-value (`""`) when absent from existing config files — no migration needed. Apply the default in `DefaultConfig()` or lazily when the field is first used (the latter is simpler and avoids resaving existing configs).

**Recommended approach**: Apply the default lazily in the `CreateSession` handler or a helper function:
```go
func OneOffBaseDir(cfg *Config) string {
    if cfg.OneOffBaseDir == "" {
        return "~/oneoff"
    }
    return cfg.OneOffBaseDir
}
```

---

## 4. Key File Locations

| Component | File |
|---|---|
| Session type definitions | `session/instance.go:531–541` |
| `InstanceOptions` struct | `session/instance.go:554–599` |
| `NewInstance` constructor | `session/instance.go:601–704` |
| `setupFirstTimeWorktree` | `session/instance.go:1062–1094` |
| `resolveStartPath` | `session/instance.go:1096–1122` |
| `CreateSession` RPC handler | `server/services/session_service.go:500–668` |
| `Config` struct | `config/config.go:181–240` |
| `DefaultConfig` | `config/config.go:283–323` |
| `SaveConfig` / `saveConfig` | `config/config.go:464–500` |
| Title constants (`MaxTitleLength=32`) | `session/types.go:255–259` |
| Proto: `CreateSessionRequest` | `proto/session/v1/session.proto:276–317` |

---

## 5. Proto — `CreateSessionRequest`

The existing `CreateSessionRequest` has:
- `string title = 1` — required
- `string path = 2` — required (but we'll generate it server-side for one-off)
- `SessionType session_type = 13` — explicit type enum

For one-off sessions, the plan is to **reuse `CreateSessionRequest`** with a new boolean flag:
```proto
bool one_off = 14;  // If true, generate path from one_off_base_dir; path field ignored
```

Or simpler: send `session_type = SESSION_TYPE_DIRECTORY` and `path = ""` — the backend detects `path == ""` + some signal and generates the directory. However, a dedicated `bool one_off = 14` field is cleaner and avoids ambiguity with existing path-required validation.

**Important**: The existing validation `if req.Msg.Path == ""` at line 509–511 will need to be conditioned on `!req.Msg.OneOff`.
