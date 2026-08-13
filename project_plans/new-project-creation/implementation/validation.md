# Validation Plan: New Project Creation

## Requirements Traceability Matrix

| AC | Requirement Summary | Test(s) |
|---|---|---|
| AC1 | "New Project" radio option appears in creation panel | T-UNIT-TS-013 (OmnibarCreationPanel — radio visible), T-E2E-NP-001 |
| AC2 | Valid submit creates dir, git init, initial commit, opens session | T-UNIT-GO-001, T-UNIT-GO-002, T-UNIT-GO-003, TestCreateSession_NewProject_CreatesSession, T-E2E-NP-002 |
| AC3 | Path already a git repo → skip init, open session | T-UNIT-GO-004, TestCreateSession_NewProject_ExistingGitRepo_IsIdempotent, T-E2E-NP-003 |
| AC4 | Path exists as non-git dir → git init in place | T-UNIT-GO-005, TestCreateSession_NewProject_ExistingDirNoGit_InitsInPlace |
| AC5 | Directory mode shows confirmation dialog when path doesn't exist; proceeds only on confirm | TestCreateSession_DirectoryMissingPath_ReturnsNotFound, TestCreateSession_DirectoryMissingPath_CreateIfMissing, T-UNIT-TS-014, T-UNIT-TS-015, T-UNIT-TS-016, T-E2E-NP-004 |
| AC6 | `new_project_base_dir` readable/writable via settings UI | TestNewProjectBaseDirOrDefault_Empty, TestNewProjectBaseDirOrDefault_CustomDir, T-E2E-SETTINGS-001, T-E2E-SETTINGS-002 |
| AC7 | All new code paths have Go unit tests; new mode has Playwright e2e | T-UNIT-GO-001 through T-UNIT-GO-007, T-E2E-NP-001 through T-E2E-NP-004 |

---

## Test Suite

### Unit Tests — Go (`session/git/util_test.go`)

All tests use `t.TempDir()` for filesystem isolation. The package is `git_test` (external test package) unless `createInitialCommit` needs to be tested from within the `git` package.

---

#### T-UNIT-GO-001 — `TestInitializeProjectDirectory_NewPath`

| Field | Value |
|---|---|
| Function | `git.InitializeProjectDirectory(path)` |
| Input | `path` = `t.TempDir() + "/new-project"` (does not exist) |
| Expected output | `nil` error; `path` exists as a directory; `gogit.PlainOpen(path)` succeeds; `repo.Head()` returns a non-nil ref |
| AC covered | AC2, AC7 |

Verification steps:
1. Assert `os.Stat(path)` returns `nil` error and `IsDir() == true`.
2. Assert `gogit.PlainOpen(path)` returns no error.
3. Call `repo.Head()` and assert the returned reference is non-nil (initial commit exists).

---

#### T-UNIT-GO-002 — `TestInitializeProjectDirectory_ExistingGitRepo`

| Field | Value |
|---|---|
| Function | `git.InitializeProjectDirectory(path)` |
| Input | `path` = an existing directory already initialized with `gogit.PlainInit` + one commit |
| Expected output | `nil` error; commit count unchanged (no extra commits added) |
| AC covered | AC3, AC7 |

Verification steps:
1. Count commits before call via `repo.Log`.
2. Call `InitializeProjectDirectory`.
3. Count commits after call; assert count is unchanged.

---

#### T-UNIT-GO-003 — `TestInitializeProjectDirectory_ExistingDirNoGit`

| Field | Value |
|---|---|
| Function | `git.InitializeProjectDirectory(path)` |
| Input | `path` = an existing directory with a pre-existing file but no `.git` subdirectory |
| Expected output | `nil` error; `.git` directory now exists; pre-existing file is still present |
| AC covered | AC4, AC7 |

Verification steps:
1. Write a sentinel file `sentinel.txt` to `path` before calling the function.
2. Call `InitializeProjectDirectory`.
3. Assert `gogit.PlainOpen(path)` succeeds.
4. Assert `sentinel.txt` still exists (directory was not wiped).

---

#### T-UNIT-GO-004 — `TestInitializeProjectDirectory_PathIsFile`

| Field | Value |
|---|---|
| Function | `git.InitializeProjectDirectory(path)` |
| Input | `path` = path to a regular file created with `os.Create` |
| Expected output | Non-nil error containing "not a directory" (or equivalent); no git repo created |
| AC covered | AC2 error path, AC7 |

---

#### T-UNIT-GO-005 — `TestInitializeProjectDirectory_RollbackOnGitFailure`

| Field | Value |
|---|---|
| Function | `git.InitializeProjectDirectory(path)` |
| Input | `path` = new path inside a read-only parent directory (use `os.Chmod(parent, 0555)` + `defer os.Chmod(parent, 0755)`); skip if `os.Geteuid() == 0` |
| Expected output | Non-nil error; `path` does not exist (directory was rolled back) |
| AC covered | AC2 error path, AC7 |

Note: This test documents the Option A rollback strategy chosen in the plan. The rollback only applies to directories newly created by this function.

---

#### T-UNIT-GO-006 — `TestNewProjectBaseDirOrDefault_Empty_ReturnsExpandedProjects`

| File | `config/config_test.go` |
|---|---|
| Function | `(*Config).NewProjectBaseDirOrDefault()` |
| Input | `Config{}` (zero-value, `NewProjectBaseDir` is empty string) |
| Expected output | Resolved path equals `filepath.Join(os.UserHomeDir(), "Projects")`; no error |
| AC covered | AC6, AC7 |

---

#### T-UNIT-GO-007 — `TestNewProjectBaseDirOrDefault_CustomDir_ReturnsExpanded`

| File | `config/config_test.go` |
|---|---|
| Function | `(*Config).NewProjectBaseDirOrDefault()` |
| Input | `Config{NewProjectBaseDir: "~/my-code"}` |
| Expected output | Resolved path equals `filepath.Join(os.UserHomeDir(), "my-code")`; no error |
| AC covered | AC6, AC7 |

---

### Unit Tests — Go (`server/services/session_service_create_test.go`)

These tests follow the existing patterns in that file: construct a test service with a mock storage backend, invoke the handler via `connect.NewRequest`, assert on connect error codes and side effects.

---

#### `TestResolveSessionType_ExplicitNewProject`

| Field | Value |
|---|---|
| Function | `resolveSessionType(msg, "")` (internal helper, tested via exported unit if unexported) |
| Input | `CreateSessionRequest{SessionType: SessionType_SESSION_TYPE_NEW_PROJECT}` |
| Expected output | Returns `session.SessionTypeNewProject` |
| AC covered | AC2, AC7 |

---

#### `TestCreateSession_NewProject_CreatesSession`

| Field | Value |
|---|---|
| Function | `SessionService.CreateSession` handler |
| Input | `CreateSessionRequest{Title: "my-proj", Path: t.TempDir()+"/my-proj", SessionType: SESSION_TYPE_NEW_PROJECT}` |
| Expected output | No `connect.CodeInvalidArgument`; directory exists at path; `gogit.PlainOpen(path)` succeeds; response contains a session ID |
| AC covered | AC2, AC7 |

If tmux is unavailable (CI without tmux), the test may return a non-`CodeInvalidArgument` error from the tmux start step. Guard with: if `connect.CodeOf(err) == connect.CodeInvalidArgument`, fail the test; any other code is acceptable and means path validation passed correctly.

---

#### `TestCreateSession_DirectoryMissingPath_ReturnsNotFound`

| Field | Value |
|---|---|
| Function | `SessionService.CreateSession` handler |
| Input | `CreateSessionRequest{Title: "dir-missing", Path: t.TempDir()+"/does-not-exist", SessionType: SESSION_TYPE_DIRECTORY, CreateIfMissing: false}` |
| Expected output | `connect.CodeNotFound`; error message contains `"path does not exist"` |
| AC covered | AC5, AC7 |

---

#### `TestCreateSession_DirectoryMissingPath_CreateIfMissing`

| Field | Value |
|---|---|
| Function | `SessionService.CreateSession` handler |
| Input | Same as above but `CreateIfMissing: true` |
| Expected output | No `connect.CodeNotFound`; directory exists at path after the call returns |
| AC covered | AC5, AC7 |

---

#### `TestResolveSessionType_OneOffOverridesNewProject`

| Field | Value |
|---|---|
| Function | `resolveSessionType` |
| Input | `CreateSessionRequest{SessionType: SESSION_TYPE_NEW_PROJECT, OneOff: true}` |
| Expected output | Returns `session.SessionTypeDirectory` (one_off always wins) |
| AC covered | AC7 (regression guard for Pitfall 4) |

---

### Unit Tests — TypeScript (`web-app/src/lib/omnibar/actions/dispatch.test.ts`)

---

#### T-UNIT-TS-012 — `create_session (new_project)` dispatch

```
describe("create_session (new_project)")
  it("dispatchOmnibarAction_should_useIsNewProjectFlag_When_sessionTypeIsNewProject")
```

| Field | Value |
|---|---|
| Test ID | T-UNIT-TS-012 |
| Input | `OmnibarAction { type: "create_session", path: "/home/user/Projects/my-app", sessionType: "new_project", title: "my-app", program: "claude" }` |
| Expected output | `deps.createSession` called with `expect.objectContaining({ isNewProject: true })`; `deps.close` called |
| AC covered | AC2, AC7 |

Run with: `cd web-app && npx jest --no-coverage --testPathPatterns="dispatch.test"`

---

### Unit Tests — TypeScript (`web-app/src/components/sessions/OmnibarCreationPanel.test.tsx`)

Create this file if it does not exist, following the `one-off-session.spec.ts` structure as a model. Use React Testing Library (`render`, `screen`, `userEvent`).

---

#### T-UNIT-TS-013 — New Project radio visible; parentDir and projectName inputs shown

| Test ID | T-UNIT-TS-013 |
|---|---|
| Setup | Render `<OmnibarCreationPanel>` with default props |
| Steps | Verify "New Project" radio is present; click it |
| Assertions | `data-testid="parent-dir-input"` is visible; `data-testid="project-name-input"` is visible; detection badge is NOT rendered (new_project hides the path detection UI) |
| AC covered | AC1, AC2, AC7 |

---

#### T-UNIT-TS-014 — Path preview updates as user types

| Test ID | (no separate ID; covered by T-UNIT-TS-013 suite) |
|---|---|
| Setup | Select "New Project" radio; fill `parent-dir-input` with `/home/user/Projects`; fill `project-name-input` with `my-app` |
| Assertions | `data-testid="path-preview"` contains text `/home/user/Projects/my-app` |
| AC covered | AC2, AC7 |

---

#### T-UNIT-TS-015 — `canSubmit` false until both fields filled

| Setup | Select "New Project" radio |
|---|---|
| Steps | (a) Leave both fields empty; (b) fill only `parent-dir-input`; (c) fill only `project-name-input` |
| Assertions | Submit button is disabled in cases (a), (b), and (c) |
| AC covered | AC2, AC7 |

---

#### T-UNIT-TS-016 — "Open as" radio group visible; branch field conditional

| Setup | Select "New Project" radio |
|---|---|
| Assertions | `data-testid="open-as-radio-group"` is visible with "New Worktree" and "Directory" options; when "New Worktree" is selected the branch input is visible; when "Directory" is selected the branch input is NOT visible |
| AC covered | AC2, AC7 |

---

### Integration Tests — Confirmation Dialog (`Omnibar.test.tsx` or `directory-mode.test.tsx`)

These tests exercise the `Omnibar` component's error-intercept and dialog flow using a mocked `onCreateSession` that throws a ConnectRPC `CodeNotFound` error on the first call and resolves successfully on the second.

---

#### T-UNIT-TS-017 — Directory mode + non-existent path → confirmation modal appears

| Test ID | T-UNIT-TS-017 |
|---|---|
| Setup | Render `<Omnibar>` with `onCreateSession` mocked to reject with a ConnectRPC `CodeNotFound` error on first invocation |
| Steps | Select "Directory" radio; enter a non-existent path; click submit |
| Assertions | A modal with `data-testid="path-confirm-create"` and `data-testid="path-confirm-cancel"` is visible |
| AC covered | AC5, AC7 |

---

#### T-UNIT-TS-018 — Confirmation dialog → confirm → createSession called with `createIfMissing: true`

| Test ID | T-UNIT-TS-018 |
|---|---|
| Setup | Same setup as T-UNIT-TS-017; dialog is shown |
| Steps | Click `data-testid="path-confirm-create"` |
| Assertions | `onCreateSession` is called a second time with `expect.objectContaining({ createIfMissing: true })`; modal is closed |
| AC covered | AC5, AC7 |

---

#### T-UNIT-TS-019 — Confirmation dialog → cancel → createSession NOT called again

| Test ID | T-UNIT-TS-019 |
|---|---|
| Setup | Same setup as T-UNIT-TS-017; dialog is shown |
| Steps | Click `data-testid="path-confirm-cancel"` |
| Assertions | `onCreateSession` call count remains 1 (no retry); modal is closed |
| AC covered | AC5, AC7 |

---

### E2E Tests (`tests/e2e/session-create-new-project.spec.ts`)

File header annotation: `// @feature session:create, session:create-new-project`

All tests run against `process.env.TEST_SERVER_URL ?? "http://localhost:8543"`. All locators use `data-testid` attributes or ARIA roles — no CSS class selectors.

---

#### T-E2E-NP-001 — New Project radio button visible in creation panel

| Test ID | T-E2E-NP-001 |
|---|---|
| Preconditions | Server running on 8543 |
| Steps | Navigate to base URL; open creation panel (keyboard shortcut or "New Session" button) |
| Assertions | `page.getByRole('radio', { name: /new project/i })` is visible |
| `data-testid` needed | `open-as-radio-group` (not required for this test, but for completeness) |
| AC covered | AC1 |

---

#### T-E2E-NP-002 — Submit valid form → session list shows new session in a new git repo

| Test ID | T-E2E-NP-002 |
|---|---|
| Preconditions | Server running; temp base dir exists and is writable |
| Steps | Open creation panel; click "New Project" radio; fill `data-testid="parent-dir-input"` with a writable temp path; fill `data-testid="project-name-input"` with a unique name; fill session title; click submit |
| Assertions | Session card for the new title appears in the session list; verify the directory at `{parentDir}/{projectName}` exists on the filesystem (if accessible from test) or verify the session status becomes "Running" |
| `data-testid` needed | `parent-dir-input`, `project-name-input`, `path-preview`, session list card |
| AC covered | AC2 |

---

#### T-E2E-NP-003 — Path already exists as git repo → session created without error

| Test ID | T-E2E-NP-003 |
|---|---|
| Preconditions | A git-initialized directory exists at a known temp path |
| Steps | Open creation panel; select "New Project"; set `parent-dir-input` to the parent of the existing repo; set `project-name-input` to the existing repo's folder name; submit |
| Assertions | No error toast shown; session card appears in session list |
| `data-testid` needed | `parent-dir-input`, `project-name-input`, error toast (absence asserted) |
| AC covered | AC3 |

---

#### T-E2E-NP-004 — Directory mode + non-existent path → confirmation dialog → confirm → session created

| Test ID | T-E2E-NP-004 |
|---|---|
| Preconditions | Server running; a path that does not exist (`/tmp/nonexistent-e2e-` + timestamp) |
| Steps | Open creation panel; select "Directory" radio; enter non-existent path in path input; submit |
| Assertions | Modal with text matching `/create.*directory|path does not exist/i` appears AND `data-testid="path-confirm-create"` button is visible |
| Steps (continued) | Click `data-testid="path-confirm-create"` |
| Assertions (continued) | Modal closes; session card appears in session list |
| `data-testid` needed | `path-confirm-create`, `path-confirm-cancel` |
| AC covered | AC5 |

---

### Settings UI Tests (`tests/e2e/settings.spec.ts` or a dedicated file)

File header annotation: `// @feature session:create, settings:config`

---

#### T-E2E-SETTINGS-001 — New Project Base Directory field visible in settings

| Test ID | T-E2E-SETTINGS-001 |
|---|---|
| Preconditions | Server running |
| Steps | Navigate to settings page (e.g., `/settings/defaults`); locate the "New Project Base Directory" section |
| Assertions | `data-testid="new-project-base-dir-input"` is visible |
| AC covered | AC6 |

---

#### T-E2E-SETTINGS-002 — Value persists after save + reload

| Test ID | T-E2E-SETTINGS-002 |
|---|---|
| Preconditions | Server running |
| Steps | Navigate to settings; clear `data-testid="new-project-base-dir-input"`; type a custom path (e.g., `~/code`); click save; reload the page |
| Assertions | After reload, `data-testid="new-project-base-dir-input"` has value `~/code` |
| AC covered | AC6 |

---

## Test Infrastructure Notes

### Go tests

- All filesystem tests must use `t.TempDir()` for temporary directories. `t.TempDir()` is automatically cleaned up by the test runner and is safe for parallel tests.
- Tests that require tmux to be available for the session start step should either assert on connect error codes (not equality) or use `t.Skip("requires tmux")` where the underlying session launch is not the test subject.
- Permission-based tests (rollback on git failure) must skip when `os.Geteuid() == 0` to avoid false passes in CI containers running as root.
- The `InitializeProjectDirectory` function is in the `session/git` package. Tests that need to call `gogit.PlainOpen` should import `github.com/go-git/go-git/v5` directly.

### TypeScript unit tests

- Use `@testing-library/react` and `@testing-library/user-event` for component tests.
- Mock the config fetch hook (`useConfig` or equivalent) to return a predictable `new_project_base_dir` value (`~/Projects`) so tests are not dependent on actual config state.
- Mock `onCreateSession` in component tests to control success/failure behavior.
- Run with: `cd web-app && npx jest --no-coverage --testPathPatterns="OmnibarCreationPanel|dispatch.test|directory-mode"`

### E2E tests

- All tests run against port 8543 (default) or `process.env.TEST_SERVER_URL`.
- The `// @feature` annotation is required on line 1 of every new spec file (CI enforces this).
- Locators must use `data-testid` attributes or ARIA roles. No CSS class selectors or `nth-child`.
- No `waitForTimeout`. Use `await expect(locator).toBeVisible({ timeout: 5000 })` for async state.

**`data-testid` values required by the E2E tests:**

| `data-testid` | Component | Used in |
|---|---|---|
| `parent-dir-input` | `OmnibarCreationPanel` | T-E2E-NP-001, T-E2E-NP-002, T-E2E-NP-003 |
| `project-name-input` | `OmnibarCreationPanel` | T-E2E-NP-001, T-E2E-NP-002 |
| `path-preview` | `OmnibarCreationPanel` | T-E2E-NP-002, T-UNIT-TS-014 |
| `open-as-radio-group` | `OmnibarCreationPanel` | T-UNIT-TS-016 |
| `path-confirm-create` | `Omnibar` confirmation modal | T-E2E-NP-004, T-UNIT-TS-018 |
| `path-confirm-cancel` | `Omnibar` confirmation modal | T-UNIT-TS-019 |
| `new-project-base-dir-input` | Settings page | T-E2E-SETTINGS-001, T-E2E-SETTINGS-002 |

---

## Coverage Summary

| Test type | Count | ACs covered |
|---|---|---|
| Go unit (git helper) | 5 (T-UNIT-GO-001 to T-UNIT-GO-005) | AC2, AC3, AC4 |
| Go unit (config) | 2 (T-UNIT-GO-006 to T-UNIT-GO-007) | AC6 |
| Go service (handler + resolver) | 5 (TestResolveSessionType_ExplicitNewProject, TestCreateSession_NewProject_CreatesSession, TestCreateSession_DirectoryMissingPath_ReturnsNotFound, TestCreateSession_DirectoryMissingPath_CreateIfMissing, TestResolveSessionType_OneOffOverridesNewProject) | AC2, AC3, AC5 |
| TypeScript unit (dispatch) | 1 (T-UNIT-TS-012) | AC2 |
| TypeScript unit (OmnibarCreationPanel) | 4 (T-UNIT-TS-013 to T-UNIT-TS-016) | AC1, AC2 |
| TypeScript integration (confirmation dialog) | 3 (T-UNIT-TS-017 to T-UNIT-TS-019) | AC5 |
| E2E new-project | 4 (T-E2E-NP-001 to T-E2E-NP-004) | AC1, AC2, AC3, AC5 |
| E2E settings | 2 (T-E2E-SETTINGS-001 to T-E2E-SETTINGS-002) | AC6 |
| **Total** | **26** | **7/7 ACs covered** |
