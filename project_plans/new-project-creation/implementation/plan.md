# Implementation Plan: New Project Creation

## Overview

Add a first-class "New Project" session creation mode (R1) that creates a directory, runs `git init`,
makes an initial commit, then opens a session. Also fix Directory mode to show a confirmation dialog
when the given path does not exist (R2), and expose the new project base directory in the settings UI (R3).

Total: **5 epics · 20 stories · 46 tasks**

---

## Implementation Order

Proto changes are required first because all other layers consume the generated bindings.

1. Epic 1 — Backend proto + config + session type constant (Stories 1.1–1.3)
2. Epic 1 — Git initialization helper (Story 1.4)
3. Epic 1 — CreateSession handler (Stories 1.5–1.6)
4. Epic 2 — Frontend (Stories 2.1–2.5); can begin in parallel after proto is regenerated
5. Epic 3 — Directory mode confirmation (Stories 3.1–3.2); Story 3.2 is partly covered by Epic 1
6. Epic 4 — Settings UI (Story 4.1); depends on config work in Epic 1 Story 1.2
7. Epic 5 — Tests (Stories 5.1–5.4); write alongside each epic, finalize at end

---

## Epic 1: Backend — New Project Session Type

### Story 1.1: Proto Changes

**Task 1.1.1** — Add `SESSION_TYPE_NEW_PROJECT = 4` to the `SessionType` enum in
`proto/session/v1/types.proto` after `SESSION_TYPE_EXISTING_WORKTREE = 3`.

```protobuf
// Create a directory, run git init, and start a session in the new repo.
SESSION_TYPE_NEW_PROJECT = 4;
```

**Task 1.1.2** — Add `bool create_if_missing = 18` to `CreateSessionRequest` in
`proto/session/v1/session.proto`. This field is used by Directory mode (R2) to confirm
path creation after the frontend shows the user a dialog; New Project always creates and
does not use this field.

```protobuf
// Optional: When session_type is DIRECTORY and the path does not exist,
// setting this to true will create the directory and initialize a git repo.
// The backend returns CodeNotFound when path is missing and this is false.
bool create_if_missing = 18;
```

Note: field numbers 1–17 are all occupied in `CreateSessionRequest` (confirmed by reading
the proto); 18 is the next available slot.

**Task 1.1.3** — Run `make generate-proto`. Verify the generated files are updated:
- `session/gen/session/v1/types.pb.go` — must contain `SessionType_SESSION_TYPE_NEW_PROJECT`
- `web-app/src/gen/session/v1/types_pb.ts` — must export `SessionType.NEW_PROJECT`

Commit the generated files together with the proto edits — CI's proto-check step will fail
if generated files diverge.

---

### Story 1.2: Config

**Task 1.2.1** — Add `NewProjectBaseDir` field to the `Config` struct in `config/config.go`
immediately after the `OneOffBaseDir` field (line ~243):

```go
// NewProjectBaseDir is the base directory where new project directories are created.
// Default: "~/Projects". Tilde is expanded at runtime. Created on first use.
NewProjectBaseDir string `json:"new_project_base_dir,omitempty"`
```

**Task 1.2.2** — Add `NewProjectBaseDirOrDefault()` method to `*Config`, modelled exactly
after the existing `OneOffBaseDirOrDefault()` pattern:

```go
// NewProjectBaseDirOrDefault returns the resolved new-project base directory.
// If NewProjectBaseDir is empty, it defaults to "~/Projects" with ~ expanded.
func (c *Config) NewProjectBaseDirOrDefault() (string, error) {
    dir := c.NewProjectBaseDir
    if dir == "" {
        dir = "~/Projects"
    }
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

**Task 1.2.3** — Verify backwards compatibility: `LoadConfigFromPath()` handles zero-value
fields through JSON unmarshalling; no migration code is needed. Document this in a comment
on the field.

---

### Story 1.3: Session Type Constant

**Task 1.3.1** — Add `SessionTypeNewProject` to the constants block in `session/instance.go`
after `SessionTypeExistingWorktree` (line ~589):

```go
// SessionTypeNewProject creates a new directory, initializes a git repo with an
// initial commit, and opens the session. The directory need not exist beforehand.
SessionTypeNewProject SessionType = "new_project"
```

**Task 1.3.2** — Update `IsValid()` (line ~593) to include the new constant:

```go
func (st SessionType) IsValid() bool {
    switch st {
    case SessionTypeDirectory, SessionTypeNewWorktree, SessionTypeExistingWorktree,
        SessionTypeNewProject:
        return true
    default:
        return false
    }
}
```

---

### Story 1.4: Git Initialization Helper

**Task 1.4.1** — Add `InitializeProjectDirectory(path string) error` to
`session/git/util.go`. This function is the single implementation point for all new-project
initialization; it is called from `setupFirstTimeWorktree()` and may also be called by the
Directory mode confirmation path.

Exact function signature and behavior contract:

```go
// InitializeProjectDirectory creates a directory and initializes it as a git repository.
// Behavior by pre-existing state:
//   - Path does not exist: creates with os.MkdirAll(path, 0755), runs git init, commits.
//   - Path exists, no .git: runs git init in place, commits.
//   - Path exists, already a git repo: no-op, returns nil.
//   - Path exists but is a regular file: returns an error.
//
// On partial failure (dir created, git init failed): attempts os.RemoveAll to roll back
// the newly created directory. Logs a warning if rollback also fails.
func InitializeProjectDirectory(path string) error {
    // 1. Check if already a git repo (open succeeds) → no-op
    if _, err := gogit.PlainOpen(path); err == nil {
        return nil
    }

    // 2. Check for file collision
    if info, err := os.Stat(path); err == nil && !info.IsDir() {
        return fmt.Errorf("path exists and is not a directory: %s", path)
    }

    // 3. Track whether we created the directory so we can roll back on failure
    dirCreated := false
    if _, err := os.Stat(path); os.IsNotExist(err) {
        if err := os.MkdirAll(path, 0755); err != nil {
            return fmt.Errorf("failed to create directory: %w", err)
        }
        dirCreated = true
    }

    // 4. git init
    repo, err := gogit.PlainInit(path, false)
    if err != nil {
        if dirCreated {
            if rmErr := os.RemoveAll(path); rmErr != nil {
                log.ErrorLog.Printf("InitializeProjectDirectory: rollback failed for %s: %v", path, rmErr)
            }
        }
        return fmt.Errorf("failed to init git repo: %w", err)
    }

    // 5. Initial commit (reuses the existing createInitialCommit helper)
    if err := createInitialCommit(repo, path); err != nil {
        if dirCreated {
            _ = os.RemoveAll(path)
        }
        return fmt.Errorf("failed to create initial commit: %w", err)
    }

    return nil
}
```

Rationale for rollback strategy: Option A from pitfalls.md (remove on failure) is
preferred over Option B (leave orphaned) because a partially-initialized directory
causes confusing behaviour if the user retries. The rollback only applies to directories
we created; pre-existing directories are left untouched.

---

### Story 1.5: CreateSession Handler

**Task 1.5.1** — Update the path validation guard in `server/services/session_service.go`
(around line 542) to exempt `SESSION_TYPE_NEW_PROJECT` alongside `one_off`:

```go
if !req.Msg.OneOff &&
    req.Msg.SessionType != sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT &&
    req.Msg.Path == "" {
    return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("path is required"))
}
```

**Task 1.5.2** — Add a case to `resolveSessionType()` (~lines 705–730):

```go
case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT:
    st = session.SessionTypeNewProject
```

This case must appear before the `one_off` override block (the one that forces
`SessionTypeDirectory`). The two modes are mutually exclusive in the UI; the override is
kept as a safeguard but the new case should not be masked.

**Task 1.5.3** — Add a Directory mode path-existence check for R2. After the `resolvedPath`
is computed (after the `one_off` block that generates a temp dir, around line ~615), add:

```go
if sessionType == session.SessionTypeDirectory {
    if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
        if !req.Msg.CreateIfMissing {
            return nil, connect.NewError(connect.CodeNotFound,
                fmt.Errorf("path does not exist: %s", resolvedPath))
        }
        // create_if_missing=true: fall through; setupFirstTimeWorktree handles creation
    }
}
```

The "create and git init" action for `create_if_missing=true` in Directory mode is handled
in Story 1.6 via `setupFirstTimeWorktree`.

---

### Story 1.6: setupFirstTimeWorktree()

**Task 1.6.1** — Add a case for `SessionTypeNewProject` in the `setupFirstTimeWorktree()`
function in `session/instance.go` (the switch on `i.SessionType`):

```go
case session.SessionTypeNewProject:
    if err := git.InitializeProjectDirectory(i.Path); err != nil {
        return fmt.Errorf("new_project initialization failed: %w", err)
    }
    i.gitManager.SetWorktree(nil)
```

**Task 1.6.2** — Add a case for `SessionTypeDirectory` + `CreateIfMissing` flag. Because
`InstanceOptions` does not currently carry `CreateIfMissing`, thread it through from the
session service to `setupFirstTimeWorktree()`. The simplest approach is to add a
`CreateIfMissing bool` field to `InstanceOptions`:

```go
// CreateIfMissing: when SessionTypeDirectory, create the directory and run git init
// if the path does not exist. Only set when the user has confirmed the action.
CreateIfMissing bool
```

Then in the `SessionTypeDirectory` case of `setupFirstTimeWorktree()`:

```go
case session.SessionTypeDirectory:
    if i.CreateIfMissing {
        if _, err := os.Stat(i.Path); os.IsNotExist(err) {
            if err := git.InitializeProjectDirectory(i.Path); err != nil {
                return fmt.Errorf("failed to create directory for session: %w", err)
            }
        }
    }
    i.gitManager.SetWorktree(nil)
```

Thread `CreateIfMissing` from `req.Msg.CreateIfMissing` through `CreateSession` into
`InstanceOptions`.

---

## Epic 2: Frontend — New Project Form

### Story 2.1: OmnibarFormState

**Task 2.1.1** — Add `"new_project"` to the `sessionType` union in `OmnibarFormState`
(`web-app/src/components/sessions/Omnibar.tsx`):

```typescript
sessionType: "new_worktree" | "directory" | "existing_worktree" | "one_off" | "new_project";
```

**Task 2.1.2** — Add three new fields to `OmnibarFormState`:

```typescript
parentDir: string;                              // base directory, populated from config
projectName: string;                            // folder name for the new repo
newProjectSessionType: "directory" | "new_worktree"; // how to open after init
```

**Task 2.1.3** — Update `INITIAL_FORM_STATE` to include the new fields:

```typescript
parentDir: "",          // populated by useEffect when new_project is selected
projectName: "",
newProjectSessionType: "new_worktree",  // default: open as New Worktree
```

**Task 2.1.4** — Add `createIfMissing?: boolean` to `OmnibarSessionData` for R2
Directory confirmation threading.

---

### Story 2.2: OmnibarCreationPanel

**Task 2.2.1** — Add `{ value: "new_project", label: "New Project" }` to `SESSION_TYPES`
in `web-app/src/components/sessions/OmnibarCreationPanel.tsx`. Insert as the second entry
(after `new_worktree`) so the most common creation modes appear first:

```typescript
const SESSION_TYPES = [
  { value: "new_worktree",      label: "New Worktree" },
  { value: "new_project",       label: "New Project" },  // NEW
  { value: "directory",         label: "Directory" },
  { value: "existing_worktree", label: "Use Worktree" },
  { value: "one_off",           label: "One-off" },
] as const;
```

`SessionTypeValue` is derived via `(typeof SESSION_TYPES)[number]["value"]` so it will
automatically include `"new_project"`.

**Task 2.2.2** — Add hint text for the new mode in the conditional hint block:

```typescript
// existing hints:
// new_worktree → "Create a new git worktree..."
// directory → "Open a directory directly..."
// existing_worktree → "Resume work in an existing worktree..."
// one_off → "Quick one-off session..."
// new_project → "Create a brand-new project directory, initialize git, and start a session."
```

**Task 2.2.3** — Add the conditional UI block for `new_project` mode. Place it after the
`one_off` banner block, following the existing rendering pattern. The block renders:

1. **Parent Directory** input (`id="omnibar-parent-dir"`, `data-testid="parent-dir-input"`),
   pre-populated from config, with hint "Where the new project folder will be created".
2. **Project Name** input (`id="omnibar-project-name"`, `data-testid="project-name-input"`),
   placeholder `"my-project"`, with inline validation (no `/`, `\`, null bytes, spaces).
   Show inline error text when validation fails; disable submit instead of showing a toast.
3. **Resolved path preview** — a read-only `<code>` element showing
   `{parentDir.trimEnd('/')}/{projectName}`, visible only when both fields are non-empty.
   `data-testid="path-preview"`.
4. **"Open as" radio group** — `"new_worktree"` (default) or `"directory"`, bound to
   `newProjectSessionType`. `data-testid="open-as-radio-group"`.
5. **Branch field** — the existing `branch` / `useTitleAsBranch` form state fields,
   conditionally rendered when `newProjectSessionType === "new_worktree"`. Reuse the same
   branch input that appears in the top-level `new_worktree` mode.

---

### Story 2.3: Omnibar Submit Logic

**Task 2.3.1** — Update `canSubmit` in `Omnibar.tsx` for the `new_project` case.
New project mode does not use the main `input` path detection pipeline:

```typescript
else if (sessionType === "new_project") {
  if (!parentDir.trim()) return false;
  if (!projectName.trim()) return false;
  if (!isValidProjectName(projectName)) return false;
  if (newProjectSessionType === "new_worktree" && !useTitleAsBranch && !branch.trim()) {
    return false;
  }
  return true;
}
```

Add `isValidProjectName(name: string): boolean` as a pure utility function:

```typescript
function isValidProjectName(name: string): boolean {
  if (!name.trim()) return false;
  // Reject path separators, null bytes, leading/trailing dots or spaces
  return !/[/\\<>:"|?*\x00]/.test(name) && !/^\.|\.$|^ | $/.test(name);
}
```

**Task 2.3.2** — Update `handleSubmit` to build `OmnibarSessionData` for `new_project` mode.
The path sent to the backend is the full resolved path; the backend will create the directory
and run git init:

```typescript
case "new_project": {
  const resolvedPath = `${parentDir.trim().replace(/\/$/, "")}/${projectName.trim()}`;
  return {
    path: resolvedPath,
    sessionType: newProjectSessionType,   // "directory" or "new_worktree"
    isNewProject: true,
    branch: newProjectSessionType === "new_worktree"
      ? (useTitleAsBranch ? title : branch.trim())
      : undefined,
  };
}
```

Note: `sessionType` in `OmnibarSessionData` is set to `newProjectSessionType` (the "open
as" choice), not `"new_project"`. The `isNewProject: true` flag tells the context layer to
use `SessionType.NEW_PROJECT` on the proto.

**Task 2.3.3** — Add `isNewProject?: boolean` to `OmnibarSessionData`.

---

### Story 2.4: Context + Hook Wiring

**Task 2.4.1** — Update `sessionTypeMap` in `OmnibarContext.tsx` to handle the `new_project`
flag. Because `isNewProject` determines the proto enum rather than the `sessionType` string,
add a conditional in `createSession`:

```typescript
const protoSessionType: SessionType = data.isNewProject
  ? SessionType.NEW_PROJECT
  : sessionTypeMap[data.sessionType ?? "directory"] ?? SessionType.DIRECTORY;
```

Thread `createIfMissing` through:
```typescript
createIfMissing: data.createIfMissing ?? false,
```

**Task 2.4.2** — Update `useSessionService.ts` to thread `createIfMissing` to the RPC body:

```typescript
createIfMissing: request.createIfMissing ?? false,
```

---

### Story 2.5: Load Default parentDir from Config

**Task 2.5.1** — Add a `useEffect` in `OmnibarCreationPanel.tsx` that fetches
`new_project_base_dir` from config when `new_project` mode is selected and `parentDir` is
empty. Use the existing config fetch hook (check for a `useConfig` hook or the settings
context — confirmed pattern exists for `SessionDefaults` loading):

```typescript
useEffect(() => {
  if (sessionType === "new_project" && !parentDir) {
    fetchConfigField("new_project_base_dir").then((val) => {
      if (val) setFormField("parentDir", val);
      // else: leave empty; hint text shows the default "~/Projects"
    });
  }
}, [sessionType]);
```

The backend already expands `~/Projects` to an absolute path when it evaluates
`NewProjectBaseDirOrDefault()`. The frontend sends the raw `~/Projects` string; the backend
must handle tilde expansion for the path it receives. Verify that `InitializeProjectDirectory`
receives an already-expanded path by ensuring the session service calls
`filepath.Abs` or uses the config method before passing to the function.

---

## Epic 3: Directory Mode Confirmation (R2)

### Story 3.1: Frontend Confirmation Dialog

**Task 3.1.1** — Add two state variables to `Omnibar.tsx`:

```typescript
const [showPathConfirmation, setShowPathConfirmation] = useState(false);
const [pendingSessionData, setPendingSessionData] = useState<OmnibarSessionData | null>(null);
```

**Task 3.1.2** — In `handleSubmit`, intercept Directory mode submissions. After building
`sessionData`, before calling `onCreateSession`, detect a non-existent path by calling the
submit RPC and handling `CodeNotFound`:

Strategy: attempt the create call; on `connect.CodeNotFound` response (path does not exist),
store the session data and show the confirmation dialog. This avoids an extra pre-flight
existence check and is consistent with the backend-driven design.

```typescript
// In the onCreateSession callback wrapper:
try {
  await onCreateSession(sessionData);
} catch (err) {
  if (isConnectNotFound(err) && sessionType === "directory") {
    setPendingSessionData(sessionData);
    setShowPathConfirmation(true);
    return;
  }
  throw err;
}
```

**Task 3.1.3** — Render the confirmation modal in the `Omnibar` JSX using the existing
`Modal` component from `web-app/src/components/ui/Modal.tsx` (Radix UI based, same pattern
as `ResumeSessionModal.tsx`):

```tsx
{showPathConfirmation && pendingSessionData && (
  <Modal open onOpenChange={() => setShowPathConfirmation(false)}>
    <ModalContent>
      <ModalTitle>Create directory?</ModalTitle>
      <ModalDescription>
        <code>{pendingSessionData.path}</code> does not exist.
        Create the directory and initialize it as a git repository?
      </ModalDescription>
      <ModalFooter>
        <ModalClose asChild>
          <button data-testid="path-confirm-cancel">Cancel</button>
        </ModalClose>
        <button
          data-testid="path-confirm-create"
          onClick={() => {
            setShowPathConfirmation(false);
            void onCreateSession({ ...pendingSessionData, createIfMissing: true });
            setPendingSessionData(null);
          }}
        >
          Create
        </button>
      </ModalFooter>
    </ModalContent>
  </Modal>
)}
```

---

### Story 3.2: Backend Path Existence Check

This story is already covered by Epic 1 Story 1.5 Task 1.5.3. The backend returns
`connect.CodeNotFound` when `SessionTypeDirectory` + path does not exist +
`create_if_missing=false`. The implementation in `setupFirstTimeWorktree()` (Story 1.6
Task 1.6.2) handles the actual directory creation when `create_if_missing=true`.

No additional backend tasks required.

---

## Epic 4: Settings UI (R3)

### Story 4.1: Settings Page

**Task 4.1.1** — Locate the existing settings/defaults page at
`web-app/src/app/settings/defaults/page.tsx`. Add a "New Project Base Directory" text
input field in the same section as (or adjacent to) the one-off base directory setting.

The component should:
- Label: "New Project Base Directory"
- Placeholder: `~/Projects`
- Helper text: "Where new project folders are created by default. Defaults to ~/Projects."
- `data-testid="new-project-base-dir-input"`

**Task 4.1.2** — Wire the field to the existing settings save mechanism. The settings page
uses the `UpdateGlobalDefaults` RPC or a direct config-write endpoint. Extend the existing
save handler to include `new_project_base_dir`:

```typescript
await saveConfigField("new_project_base_dir", newProjectBaseDir);
```

If the existing settings save path sends the full `Config` struct, add `newProjectBaseDir`
to the form state and include it in the payload. Follow the exact pattern used for
`one_off_base_dir` (the `OneOffBaseDir` field).

**Task 4.1.3** — Load the current value on mount using the same config-read hook used
elsewhere in the settings page.

---

## Epic 5: Tests

### Story 5.1: Go Unit Tests

**Task 5.1.1** — `session/git/util_test.go`: Add tests for `InitializeProjectDirectory`
(T-UNIT-GO-001 through T-UNIT-GO-004):

- `TestInitializeProjectDirectory_PathNotExist_CreatesAndInits` — path doesn't exist;
  verify dir created, git repo opened, HEAD commit exists.
- `TestInitializeProjectDirectory_PathExistsNoGit_InitsInPlace` — dir exists but no `.git`;
  verify git initialized without removing pre-existing files.
- `TestInitializeProjectDirectory_PathExistsWithGit_NoOp` — dir is already a repo; verify
  function returns nil and makes no changes.
- `TestInitializeProjectDirectory_PathIsFile_ReturnsError` — path is a regular file; verify
  error returned.
- `TestInitializeProjectDirectory_PartialFailure_RollsBackCreatedDir` — mock a git init
  failure (use a read-only parent if possible); verify the newly created dir is removed.

Note: `createInitialCommit` and `findGitRepoRoot` are implicitly tested by the above; add
explicit tests for `createInitialCommit` (T-UNIT-GO-005) only if they are exported or
callable from tests in the same package.

**Task 5.1.2** — `config/config_test.go` (or equivalent): Add two tests for
`NewProjectBaseDirOrDefault`:

- `TestNewProjectBaseDirOrDefault_Empty_ReturnsExpandedProjects` — empty config returns
  `~` expanded `Projects` path.
- `TestNewProjectBaseDirOrDefault_CustomDir_ReturnsExpanded` — custom tilde-prefixed dir
  is expanded correctly.

**Task 5.1.3** — Verify `SessionTypeNewProject.IsValid()` returns true in
`session/instance_test.go` (or the closest existing test file for that package).

---

### Story 5.2: Go Integration Tests

**Task 5.2.1** — `server/services/session_service_create_test.go`: Add routing tests
(following the existing `TestResolveSessionType_*` pattern):

- `TestResolveSessionType_ExplicitNewProject` — `SESSION_TYPE_NEW_PROJECT` → `SessionTypeNewProject`
- `TestResolveSessionType_OneOffOverridesNewProject` — `SESSION_TYPE_NEW_PROJECT + one_off=true` →
  `SessionTypeDirectory` (one_off always wins; documents the priority order)

**Task 5.2.2** — Add path-validation tests:

- `TestCreateSession_NewProject_NonExistentPath_PassesValidation` — new_project with a
  non-existent path must NOT return `CodeInvalidArgument`.
- `TestCreateSession_Directory_PathNotExist_NoFlag_ReturnsNotFound` — directory mode +
  non-existent path + `create_if_missing=false` must return `CodeNotFound`.
- `TestCreateSession_Directory_PathNotExist_WithFlag_Succeeds` — directory mode +
  non-existent path + `create_if_missing=true` must not return `CodeNotFound`.

**Task 5.2.3** — Add end-to-end handler test (may require tmux to be available; guard with
`t.Skip` if not):

- `TestCreateSession_NewProject_CreatesDirectoryAndGitRepo` — calls `CreateSession` with
  `SESSION_TYPE_NEW_PROJECT` and a temp path; verifies the directory exists and is a git
  repo with at least one commit after the call.
- `TestCreateSession_NewProject_ExistingGitRepo_IsIdempotent` — calls `CreateSession` on
  a path that is already a git repo; verifies no additional commits are created.

---

### Story 5.3: TypeScript Tests

**Task 5.3.1** — `web-app/src/lib/omnibar/actions/dispatch.test.ts`: Add a describe block
for `new_project` (T-UNIT-TS-012):

```typescript
describe("create_session (new_project)", () => {
  it("dispatchOmnibarAction_should_useIsNewProjectFlag_When_sessionTypeIsNewProject", () => {
    // T-UNIT-TS-012
    const deps = makeDeps();
    dispatchOmnibarAction(
      { type: "create_session", path: "/home/user/Projects/my-app",
        sessionType: "new_project", title: "my-app", program: "claude" },
      deps,
    );
    expect(deps.createSession).toHaveBeenCalledWith(
      expect.objectContaining({ isNewProject: true })
    );
    expect(deps.close).toHaveBeenCalled();
  });
});
```

**Task 5.3.2** — `web-app/src/components/sessions/OmnibarCreationPanel.test.tsx` (create
if not present, following the `one-off-session.spec.ts` test structure as a model): Add
tests for the new project form:

- `new_project option is visible in SESSION_TYPES radio group` — render panel, verify radio
  button with label "New Project" is present.
- `shows parentDir and projectName inputs when new_project is selected` — select the radio,
  verify both inputs are visible.
- `shows path preview when both fields are non-empty` — fill parentDir and projectName,
  verify preview text matches `parentDir/projectName`.
- `submit is disabled when projectName is empty` — verify `canSubmit` returns false.
- `shows open-as radio group when new_project selected` — verify the "Open as" sub-radio
  is visible with "New Worktree" and "Directory" options.
- `shows branch field when open-as is new_worktree` — verify branch input appears.
- `hides branch field when open-as is directory` — verify branch input does not appear.

---

### Story 5.4: E2E Tests

**Task 5.4.1** — Create `tests/e2e/session-create-new-project.spec.ts` with the
`// @feature session:create, session:create-new-project` annotation:

- `new_project type is selectable in creation panel` — open omnibar, click "New Project"
  radio, verify `aria-checked="true"`.
- `shows project name input when new_project selected` — verify `data-testid="project-name-input"`
  is visible.
- `shows parent dir input pre-populated` — verify `data-testid="parent-dir-input"` is visible
  and non-empty (loaded from config default).
- `shows resolved path preview` — fill both inputs, verify `data-testid="path-preview"`
  shows concatenated path.
- `submit button is disabled without project name` — leave projectName empty, verify
  submit is disabled.
- `sends SESSION_TYPE_NEW_PROJECT (enum 4) in RPC payload` — fill form, intercept
  `CreateSession` request, verify `sessionType === 4`.

**Task 5.4.2** — Add two tests to `tests/e2e/session-create-directory.spec.ts`:

- `shows confirmation dialog when directory path does not exist` — fill a non-existent path
  for directory mode, submit, verify confirmation dialog appears with
  `data-testid="path-confirm-create"` button.
- `retries with createIfMissing=true after confirmation` — confirm the dialog, intercept
  the retry request, verify `createIfMissing === true`.

**Task 5.4.3** — Update the feature registry:

- `docs/registry/features/backend/session/create.json`: add `SESSION_TYPE_NEW_PROJECT` to
  supported modes, add `create_if_missing` field note, update `lastModified`, add test IDs
  from Story 5.2.
- Create `docs/registry/features/frontend/session-creation/new-project.json`:

```json
{
  "id": "new-project-creation",
  "type": "frontend",
  "component": "OmnibarCreationPanel",
  "file": "web-app/src/components/sessions/OmnibarCreationPanel.tsx",
  "tested": true,
  "testIds": [
    "new project session creation > new_project type is selectable in creation panel",
    "new project session creation > shows project name input when new_project selected",
    "new project session creation > sends SESSION_TYPE_NEW_PROJECT (enum 4) in RPC payload"
  ]
}
```

---

## Technology Choices

### Proto enum (not a flag) for New Project

The research recommends and this plan uses `SESSION_TYPE_NEW_PROJECT = 4` as a distinct
enum value rather than a boolean flag (e.g., `bool is_new_project`). Rationale: New Project
has a fundamentally different initialization lifecycle (dir creation → git init → initial
commit → optional worktree) that warrants a separate type. The enum value is stored on the
session record and influences restoration behavior, so it must be queryable independently.
This is consistent with how `SESSION_TYPE_EXISTING_WORKTREE` was handled.

This plan differs slightly from the `one_off` precedent (which uses a bool flag). The
difference is intentional: one-off sessions reuse the Directory lifecycle entirely; new
project sessions have a distinct lifecycle step.

### `create_if_missing` field (field 18) for R2

The research considered two approaches for R2:
1. Return `CodeNotFound` when the path doesn't exist + retry with `create_if_missing=true`
2. A pre-flight existence check on the frontend before submitting

This plan uses approach 1 (backend-driven with retry), which keeps the frontend stateless
and avoids a separate existence-check RPC. The pitfalls doc (Pitfall 5) confirms field 18
is the next available slot (field 17 is `project_id`). This is verified by reading the
proto.

### Rollback strategy for partial init failure (Pitfall 1)

This plan uses Option A (rollback on failure) from pitfalls.md: if the directory was newly
created by `InitializeProjectDirectory` and a subsequent step fails, attempt `os.RemoveAll`
of that directory with a warning log if cleanup also fails. Pre-existing directories are
never removed. This prevents orphaned empty directories that confuse retries.

### `isNewProject` flag rather than mapping `new_project` through sessionTypeMap

The frontend uses `isNewProject: true` in `OmnibarSessionData` and the context layer
maps it to `SessionType.NEW_PROJECT`. This avoids adding `"new_project"` to `sessionTypeMap`
in a way that would be confused with the `newProjectSessionType` sub-field ("directory" or
"new_worktree"). The two-level design (outer type = new_project, inner type = open_as) is
unique to this mode and requires this indirection.

---

## Risk Register

| Risk | Source | Mitigation |
|---|---|---|
| Proto regeneration skipped | Pitfall 3 | CI proto-check step catches divergence; enforce `make generate-proto` in PR checklist |
| `one_off` override silently wins over `new_project` | Pitfall 4 | Add `TestResolveSessionType_OneOffOverridesNewProject`; document in code comment near the override |
| Tilde `~/Projects` not expanded before `InitializeProjectDirectory` | Pitfall 2 (frontend), stack.md | Session service must call `NewProjectBaseDirOrDefault()` or `filepath.Abs` before passing path to the function; add an assertion test |
| Orphaned directory on partial init failure | Pitfall 1 | Rollback strategy in `InitializeProjectDirectory`; test in Story 5.1 Task 5.1.1 |
| Project name with path separators creates unintended nested dirs | Pitfall 2 (frontend) | `isValidProjectName()` utility rejects `/`, `\`; inline error shown before submit |
| Field 18 conflict if another branch also adds to `CreateSessionRequest` | Pitfall 5 | Merge early; confirm field 18 is free at PR creation time |
| `SessionTypeDirectory` path check fires on paths that do not exist for legitimate reasons (e.g., paths inside a volume not yet mounted) | New | The check is limited to `os.IsNotExist(err)` — permission errors and other I/O errors still propagate as `CodeInternal`, not `CodeNotFound` |
| Ent schema accidentally changed | Pitfall 2 | Session type is stored as a string field; no schema change is needed; add a `git diff session/ent/schema/` check to the PR checklist |

---

## Acceptance Criteria Traceability

| AC | Requirement | Implementing Tasks |
|---|---|---|
| AC1 | "New Project" radio appears in creation panel | Story 2.2 Task 2.2.1 |
| AC2 | Valid submit creates dir, git init, initial commit, opens session | Story 1.4 Task 1.4.1; Story 1.6 Task 1.6.1; Story 2.3 Task 2.3.2 |
| AC3 | Path already a git repo → skip init, open session | Story 1.4 Task 1.4.1 (no-op branch in `InitializeProjectDirectory`) |
| AC4 | Path exists as non-git dir → warn user | Story 1.4 Task 1.4.1 (`git init` in place); the git init action constitutes the "offer to git init in place" behavior; the frontend does not need a separate warning because the backend proceeds successfully (R1.3 behavior) |
| AC5 | Directory mode shows confirmation dialog on non-existent path | Story 1.5 Task 1.5.3; Story 3.1 Tasks 3.1.1–3.1.3 |
| AC6 | `new_project_base_dir` readable/writable via settings UI | Story 1.2 Tasks 1.2.1–1.2.2; Story 4.1 Tasks 4.1.1–4.1.3 |
| AC7 | New code paths have Go unit tests; new mode has Playwright e2e | Story 5.1; Story 5.2; Story 5.4 Task 5.4.1 |

Note on AC4: the requirements say "warn the user; offer to `git init` in place or abort."
The backend `InitializeProjectDirectory` automatically initializes in place without an
explicit warning dialog, since an existing non-git directory is a valid target for a new
project. If a stricter warning is required, add a pre-check in the session service that
returns a new `CodeAlreadyExists` variant when the dir exists without `.git`, and add a
corresponding frontend dialog. This is considered a low-priority edge case and is deferred
unless requirements prioritize it.

---

## Session Creation Registry Checklist

Per `.claude/rules/session-creation-registry.md`, all 7 touchpoints must be updated:

- [x] `proto/session/v1/types.proto` — `SESSION_TYPE_NEW_PROJECT = 4` (Story 1.1 Task 1.1.1)
- [x] `proto/session/v1/session.proto` — `bool create_if_missing = 18` (Story 1.1 Task 1.1.2)
- [x] `make generate-proto` — (Story 1.1 Task 1.1.3)
- [x] `server/services/session_service.go` — path guard, switch case, mode logic (Story 1.5)
- [x] `session/instance.go` — `SessionTypeNewProject` constant + `IsValid()` (Story 1.3)
- [x] `web-app/src/components/sessions/Omnibar.tsx` — type union, canSubmit, handleSubmit (Story 2.1, 2.3)
- [x] `web-app/src/components/sessions/OmnibarCreationPanel.tsx` — SESSION_TYPES, hint, UI block (Story 2.2)
- [x] `web-app/src/lib/contexts/OmnibarContext.tsx` — isNewProject → SessionType.NEW_PROJECT (Story 2.4 Task 2.4.1)
- [x] `web-app/src/lib/hooks/useSessionService.ts` — `createIfMissing` RPC thread-through (Story 2.4 Task 2.4.2)
