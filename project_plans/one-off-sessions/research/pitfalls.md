# Research: Pitfalls — Failure Modes and Edge Cases

## Summary

Four key failure modes were researched: name collision races, path non-existence behavior, title length constraints, and config persistence. Each has a clear mitigation path.

---

## 1. Directory Creation Race — Simultaneous One-Off Sessions

### The Problem

If two `CreateSession` requests with `one_off=true` arrive simultaneously:
1. Both generate the same name (same date, same random adjective/noun/number).
2. Both call `os.Stat(path)` — neither exists yet.
3. Both call `os.MkdirAll(path)` — only one wins.
4. The loser gets an error.

### Probability

With 80 adjectives × 80 nouns × 100 numbers = 640,000 combinations per date, plus the race window being only a few microseconds, simultaneous collisions in practice are nearly impossible. However, correctness demands we handle it.

### Mitigation: Use `os.Mkdir` (not `MkdirAll`) for the leaf directory

`os.Mkdir` (not `os.MkdirAll`) is atomic on most Unix filesystems: it calls the `mkdir(2)` syscall which is guaranteed to fail if the directory already exists. This turns the race into a deterministic error:

```go
// Create base dir with MkdirAll (idempotent)
if err := os.MkdirAll(baseDir, 0755); err != nil {
    return "", fmt.Errorf("cannot create one_off_base_dir: %w", err)
}

// Attempt to create the leaf with Mkdir (atomic, fails if exists)
for i := 0; i < maxAttempts; i++ {
    name := namegen.Generate()
    fullPath := filepath.Join(baseDir, name)
    if err := os.Mkdir(fullPath, 0755); err == nil {
        return fullPath, nil // success — this process owns the directory
    }
    // err != nil: either collision or permissions error; try again
}
return "", fmt.Errorf("failed to generate unique one-off directory after %d attempts", maxAttempts)
```

`os.Mkdir` fails with `EEXIST` on collision, `ENOTDIR` if base is a file, `EACCES` for permissions. These should be wrapped and returned as appropriate errors.

**Note**: This approach is safe even without file locking because `mkdir(2)` is atomic at the OS level.

---

## 2. What Happens in `NewInstance` / `Start` When Path Doesn't Exist

### Current behavior (directory session, no mkdir)

In `NewInstance` (`session/instance.go:601`):
- Expands `~` and converts to absolute path via `filepath.Abs`.
- Does NOT call `os.Stat` — no existence check.
- Returns `(*Instance, error)` successfully even if path doesn't exist.

In `Start(true)` → `setupFirstTimeWorktree()` (line 1088):
- For `SessionTypeDirectory`: sets `gitManager` worktree to nil and returns nil (no error).

In `start()` (line 979):
```go
startPath := i.resolveStartPath(basePath)  // basePath = i.Path
```

In `resolveStartPath` (line 1096–1122):
```go
if _, err := os.Stat(startPath); os.IsNotExist(err) {
    log.WarningLog.Printf("Working directory '%s' doesn't exist, using '%s' instead", startPath, basePath)
    return basePath
}
```

If `i.Path` itself doesn't exist, `resolveStartPath` returns `basePath` (which is also `i.Path`), and then `tmuxManager.Start(startPath)` is called with the non-existent path.

**Tmux behavior**: `tmux new-session -d -c /nonexistent/path` will fail on Linux (returns exit code 1) or silently start in `$HOME` on some macOS versions. This could produce a session that appears to start but is in the wrong directory.

**Conclusion**: The one-off directory **must be created before** `session.NewInstance` / `instance.Start` is called. The safest implementation creates the directory in `CreateSession`, immediately after validating `one_off_base_dir`, before building `instanceOpts`.

---

## 3. Title Validation Constraints

### `MaxTitleLength = 32`, `MinTitleLength = 1`

**File**: `session/types.go:255–259`

```go
const (
    MinTitleLength = 1
    MaxTitleLength = 32
)
```

The title length is checked in `RenameSession` (line 1519 in `instance.go`):
```go
if len(newTitle) < MinTitleLength || len(newTitle) > MaxTitleLength {
```

For one-off sessions, `InstanceOptions.Title` = user-supplied session title. `InstanceOptions.Path` = generated directory path.

**These are separate**: the directory name (e.g. `20260424-brave-falcon-07`) is NOT the title. The user provides the title separately. So `MaxTitleLength=32` applies to the user-supplied title, not the generated directory name.

**Validation needed in frontend**: The session name input should enforce `maxLength=32`. The Omnibar input (`sessionName`) already allows arbitrary text — the backend returns `AlreadyExists` or other errors if validation fails, but there's no explicit title-length validation in `CreateSession` (only duplicate check). Title validation happens in `RenameSession`. Add a note: if the user submits a title > 32 chars via the create flow, the backend won't reject it at creation time (only `RenameSession` enforces it). This is a pre-existing gap, not introduced by one-off.

**Generated directory name length**: As calculated in `architecture.md`, max is 30 chars (under 32). The directory name being stored as `i.Path` is fine — `Path` has no length limit.

---

## 4. Config Persistence — `one_off_base_dir` Field

### How Config is Saved

`config/config.go:499`:
```go
func SaveConfig(config *Config) error {
    return saveConfig(config)
}
```

`saveConfig` (line 466–495):
1. Gets config path from `GetConfigDir()`.
2. Calls `json.MarshalIndent(config, "", "  ")`.
3. Writes to a `.tmp` file.
4. Renames to the target atomically.

**Adding `OneOffBaseDir string \`json:"one_off_base_dir,omitempty"\``** to the `Config` struct:
- Existing config files without this field will load with `OneOffBaseDir == ""`.
- `omitempty` means the field is omitted when empty — existing users' configs won't be modified.
- When the user first creates a one-off session and the backend uses the default `~/oneoff`, the config is NOT automatically saved back — only if something calls `SaveConfig`.

**Recommendation**: Do NOT auto-save the default. Apply the default lazily:
```go
func resolveOneOffBaseDir(cfg *config.Config) (string, error) {
    dir := cfg.OneOffBaseDir
    if dir == "" {
        dir = "~/oneoff"
    }
    // Expand tilde
    if strings.HasPrefix(dir, "~/") {
        home, err := os.UserHomeDir()
        if err != nil {
            return "", fmt.Errorf("cannot expand home dir: %w", err)
        }
        dir = filepath.Join(home, dir[2:])
    }
    return dir, nil
}
```

This is consistent with how other defaults in `DefaultConfig()` work.

### Potential Pitfall: Config Race

`SaveConfig` is not goroutine-safe (no mutex). But for the `one_off_base_dir` field, we're only reading it in `CreateSession` — not writing it back. So no race.

If a future settings UI allows editing `one_off_base_dir`, it should use the same write-through pattern used by other config endpoints (load, modify, SaveConfig) with file-level atomicity provided by the temp-file rename pattern already in place.

---

## 5. Additional Pitfalls

### Base Dir Creation Fails

If `one_off_base_dir` resolves to a path with missing parent dirs (e.g. `~/projects/experiments/oneoff` but `~/projects/experiments/` doesn't exist), `os.MkdirAll` will create the full path. If permissions deny creation, return a `connect.CodeInvalidArgument` error with a message like "cannot create one_off_base_dir '…': permission denied".

### `one_off_base_dir` is a File (Not a Dir)

If the resolved path exists but is a regular file, `os.MkdirAll` returns `ENOTDIR`. Handle: check `os.Stat(baseDir)` after `MkdirAll` — if it's not a directory, return an explicit error.

### User's `one_off_base_dir` Config Points Outside Home Dir

The existing path validation in `ListBranches` requires paths to be within home dir. No such check exists in `CreateSession` for the path field. For one-off, we should add a security check: `one_off_base_dir` must resolve within `os.UserHomeDir()` (same pattern as line 1913–1918 in session_service.go).

### Session Title Collision on Rapid Creation

`CreateSession` checks for duplicate titles (line 513–524) using an in-memory scan of `storage.ListInstanceData()`. If two requests arrive with the same title simultaneously, both could pass the duplicate check before either saves. This is a pre-existing race in the app (not introduced by one-off), but worth noting. For one-off sessions, the title is user-provided so the probability is low.

### Tmux Session Name Collision

Tmux session names are derived from the session `Title` with a prefix (`staplersquad_{title}`). If two sessions have the same sanitized title, the second tmux session creation will fail. The pre-existing duplicate title check should prevent this, but the sanitization (special chars → underscores) could cause two different titles to map to the same tmux name. This is pre-existing; not introduced by one-off.
