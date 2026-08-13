# Stack Research: New Project Creation (Backend)

## Existing Git Auto-Init Mechanism

**`session/git/util.go`** — `findGitRepoRoot()` (lines 78-101):
- Checks if directory exists; if not, `os.MkdirAll(path, 0755)`
- `git.PlainInit(path, false)` to init the repo
- Calls `createInitialCommit(repo, repoPath)` — creates a `.gitignore` file and commits with author "Stapler Squad <stapler-squad@localhost>"

This logic is triggered only for `SessionTypeNewWorktree` via `setupFirstTimeWorktree()` → `NewGitWorktreeWithBranchAndExecutor()`. It does NOT run for `SessionTypeDirectory`.

**Reusable functions:**
- `createInitialCommit(repo *git.Repository, repoPath string)` — already handles git add + commit
- `findGitRepoRoot(path string)` — creates dir + init + initial commit if path doesn't exist

---

## Proto Changes Required

### `proto/session/v1/types.proto`
Add enum value 4:
```protobuf
enum SessionType {
  SESSION_TYPE_UNSPECIFIED = 0;
  SESSION_TYPE_DIRECTORY = 1;
  SESSION_TYPE_NEW_WORKTREE = 2;
  SESSION_TYPE_EXISTING_WORKTREE = 3;
  SESSION_TYPE_NEW_PROJECT = 4;
}
```

### `proto/session/v1/session.proto`
Next available field number on `CreateSessionRequest` is **18** (field 17 is `project_id`). No new field needed if relying on the enum — the existing `session_type = 13` field carries `SESSION_TYPE_NEW_PROJECT`. The backend logic distinguishes behavior by session type.

---

## Config Changes

### Pattern to follow: `config/config.go`
`OneOffBaseDirOrDefault()` is the template:
```go
func (c *Config) OneOffBaseDirOrDefault() (string, error) {
    dir := c.OneOffBaseDir
    if dir == "" { dir = "~/oneoff" }
    if strings.HasPrefix(dir, "~/") {
        home, _ := os.UserHomeDir()
        dir = filepath.Join(home, dir[2:])
    }
    return dir, nil
}
```

**Add to Config struct** (near `OneOffBaseDir` field):
```go
NewProjectBaseDir string `json:"new_project_base_dir,omitempty"`
```

**Add method:**
```go
func (c *Config) NewProjectBaseDirOrDefault() (string, error) {
    dir := c.NewProjectBaseDir
    if dir == "" { dir = "~/Projects" }
    if strings.HasPrefix(dir, "~/") {
        home, err := os.UserHomeDir()
        if err != nil { return "", fmt.Errorf("cannot expand home dir: %w", err) }
        dir = filepath.Join(home, dir[2:])
    }
    return dir, nil
}
```

---

## Session Type Constants (`session/instance.go`)

Add after existing constants:
```go
SessionTypeNewProject SessionType = "new_project"
```

Update `IsValid()` if it exists to include the new value.

---

## `setupFirstTimeWorktree()` Changes

For `SessionTypeDirectory` (current): sets worktree to nil, no git ops.

Add new case for `SessionTypeNewProject`:
```go
case session.SessionTypeNewProject:
    if err := git.InitializeProjectDirectory(i.Path); err != nil {
        return fmt.Errorf("failed to initialize project: %w", err)
    }
    i.gitManager.SetWorktree(nil)
```

**New function in `session/git/util.go`:**
```go
// InitializeProjectDirectory creates a directory and initializes it as a git repo.
// If the directory already exists and is a git repo, it is a no-op.
// If the directory exists but is not a git repo, it initializes git in place.
func InitializeProjectDirectory(path string) error {
    if err := os.MkdirAll(path, 0755); err != nil {
        return fmt.Errorf("failed to create directory: %w", err)
    }
    // Check if already a git repo
    if _, err := git.PlainOpen(path); err == nil {
        return nil // already initialized
    }
    repo, err := git.PlainInit(path, false)
    if err != nil {
        return fmt.Errorf("failed to init git repo: %w", err)
    }
    return createInitialCommit(repo, path)
}
```

---

## Session Type Resolution (`server/services/session_service.go`)

In `resolveSessionType()` (lines 705-730), add case:
```go
case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT:
    st = session.SessionTypeNewProject
```

Also: path validation guard (line ~542) — for `new_project` mode, path is the *resolved* `parentDir/projectName`, so validation can be relaxed (path need not exist yet). Add `new_project` to the exemption list alongside `one_off`:
```go
if !req.Msg.OneOff && req.Msg.SessionType != sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT && req.Msg.Path == "" {
    return nil, connect.NewError(...)
}
```

---

## Directory Mode Confirmation (R2)

This is a **frontend concern** — the backend should accept a `confirmed_path_creation bool` field (or simply proceed when told to). However, the simplest implementation:

- Backend: for `SessionTypeDirectory` + non-existent path, return a structured error with a specific code/message that the frontend can detect and show a confirmation dialog.
- Or: add `bool create_if_missing = 18` to `CreateSessionRequest` — if false (default) and path doesn't exist, return error code `NOT_FOUND`; if true, create the dir.

**Recommended**: Add `bool create_if_missing = 18` to proto. Backend checks: if `SessionTypeDirectory` and path doesn't exist and `!create_if_missing`, return `connect.CodeNotFound` with message `"path does not exist"`. Frontend retries with `create_if_missing: true` after confirmation.

---

## Risks & Gotchas

1. **Initial commit author**: hardcoded "Stapler Squad" — acceptable for now.
2. **`resolveStartPath()`** (instance.go ~1186): validates `WorkingDir` exists. For new project, `WorkingDir` is empty, so this is fine — it falls through to the session path.
3. **One-off priority**: `one_off` always overrides to `SessionTypeDirectory` (line 726). Ensure `new_project` is checked before the `one_off` override, or ensure they can't be set simultaneously.
4. **Config migration**: `LoadConfigFromPath()` handles zero-value defaults — adding a new field is backwards-compatible.
5. **Proto regeneration**: After any proto change, `make generate-proto` is required. Committed generated files must match.
6. **ent schema**: No ent schema changes needed — session type is stored as a string field in the existing schema.
