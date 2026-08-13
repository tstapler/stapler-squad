# Architecture Research: New Project Creation

## All 7 Session Creation Registry Touchpoints

### 1. Proto Enum — `proto/session/v1/types.proto`
Add `SESSION_TYPE_NEW_PROJECT = 4` to the `SessionType` enum. Use a distinct enum value (not a flag) because New Project has a fundamentally different lifecycle: dir creation → git init → initial commit → optional worktree — and this type is stored on the session record for restoration behavior.

### 2. Proto Request Message — `proto/session/v1/session.proto`
Add `bool create_if_missing = 18` to `CreateSessionRequest`. This decouples the Directory mode confirmation flow from New Project (Directory can optionally create; New Project always creates).

### 3. Go Backend Handler — `server/services/session_service.go`

**3a. Path validation guard** (~line 542):
```go
if !req.Msg.OneOff &&
   req.Msg.SessionType != sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT &&
   req.Msg.Path == "" {
    return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("path is required"))
}
```

**3b. Session type resolution** (`resolveSessionType`, ~lines 705-730):
```go
case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT:
    st = session.SessionTypeNewProject
```

**3c. Mode-specific logic** (after line ~615):
```go
if req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT {
    // path is already the full resolved path (parentDir/projectName) sent by frontend
    sessionType = session.SessionTypeNewProject
}
```

For R2 (Directory confirmation), add to the Directory path:
```go
if sessionType == session.SessionTypeDirectory && !req.Msg.CreateIfMissing {
    if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
        return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("path does not exist: %s", resolvedPath))
    }
}
```

### 4. Go Session Type Constants — `session/instance.go`
```go
SessionTypeNewProject SessionType = "new_project"
```
Update `IsValid()` if present.

### 5. Frontend Type Union — `web-app/src/components/sessions/Omnibar.tsx`
Add `"new_project"` to `sessionType` in `OmnibarFormState`. Add `parentDir`, `projectName`, `newProjectSessionType` fields. Update `canSubmit` and `handleSubmit`.

### 6. Frontend Radio Group — `web-app/src/components/sessions/OmnibarCreationPanel.tsx`
Add to `SESSION_TYPES`: `{ value: "new_project", label: "New Project" }`. Add conditional UI block (parent dir, project name, path preview, "open as" radio, conditional branch field).

### 7. Frontend Context + Hook
- **`OmnibarContext.tsx`**: Add `new_project: SessionType.NEW_PROJECT` to `sessionTypeMap`. Thread `parentDir`, `projectName`, `createIfMissing` through.
- **`useSessionService.ts`**: Thread `createIfMissing: request.createIfMissing ?? false` to RPC body.

---

## Complete Data Flow

```
User selects "New Project" radio
  → fills: parentDir="~/Projects", projectName="my-app", openAs="new_worktree"
  → Omnibar.handleSubmit:
      path = "~/Projects/my-app"
      sessionType = "new_project"
  → OmnibarContext.createSession:
      sessionType: SessionType.NEW_PROJECT  (proto enum 4)
      path: "~/Projects/my-app"
  → useSessionService → CreateSessionRequest {
      title: "my-app",
      path: "~/Projects/my-app",
      session_type: SESSION_TYPE_NEW_PROJECT
    }
  → session_service.go CreateSession:
      resolveSessionType → SessionTypeNewProject
      instance.Start(true) → setupFirstTimeWorktree()
        case SessionTypeNewProject:
          git.InitializeProjectDirectory("~/Projects/my-app")
            os.MkdirAll → git.PlainInit → createInitialCommit
          gitManager.SetWorktree(nil)
      → launch tmux in /home/user/Projects/my-app
```

---

## Settings UI Location

A settings page already exists at `web-app/src/app/settings/defaults/page.tsx`. Add a "New Project Base Directory" text input that saves to `new_project_base_dir` via the existing `UpdateGlobalDefaults` RPC pattern.

The OmnibarCreationPanel should load this value on mount via `useEffect` when `sessionType === "new_project"`.

---

## Integration Risks

| Risk | Mitigation |
|------|-----------|
| Partial failure (dir created, git init fails — orphaned dir) | Rollback: on error in `InitializeProjectDirectory`, attempt `os.RemoveAll` of the created dir; log if cleanup also fails |
| Race: two sessions same name created simultaneously | `os.MkdirAll` is safe; git.PlainInit returns error if `.git` exists — catch and return `CodeAlreadyExists` |
| `~` expansion mismatch | Frontend sends raw `~/Projects`; backend expands via `os.UserHomeDir()` — consistent |
| Directory confirmation creates dir then session creation fails | Same rollback pattern applies when `create_if_missing=true` |
| Proto regeneration skipped | `make generate-proto` required; CI proto-check step will catch mismatch |

---

## Feature Registry Entries Needed

**Backend** — update `docs/registry/features/backend/session/create.json`:
- Add `SESSION_TYPE_NEW_PROJECT` to supported modes
- Add `create_if_missing` field
- Set `tested: true` + add test IDs once covered

**Frontend** — new file `docs/registry/features/frontend/session-creation/new-project.json`:
```json
{
  "id": "new-project-creation",
  "type": "frontend",
  "component": "OmnibarCreationPanel",
  "file": "web-app/src/components/sessions/OmnibarCreationPanel.tsx",
  "tested": false,
  "testIds": []
}
```
