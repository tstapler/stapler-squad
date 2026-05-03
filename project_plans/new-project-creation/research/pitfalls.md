# Implementation Pitfalls & Testing Strategy: New Project Creation Feature

## Executive Summary

This document covers the testing landscape and implementation pitfalls for adding a new `SESSION_TYPE_NEW_PROJECT` session type to stapler-squad. The feature creates a directory + initializes git + makes an initial commit automatically, replacing the current manual workflow. It includes a backend new-project-type and a frontend confirmation dialog for Directory mode when paths don't exist.

**Key Findings:**
- Existing git init logic is well-tested and reusable
- Proto regeneration is required for new enum value
- One-off priority override interacts with new type — must be checked carefully
- E2E tests run on ports 8543–8544 (configurable via `TEST_SERVER_URL`)
- Next available T-UNIT-TS ID is T-UNIT-TS-012 (T-UNIT-TS-008 through 011 are taken)
- No ent schema changes needed — session type is a string field

---

## Part 1: Existing Tests Covering Session Creation

### Go (Backend) Tests

#### `session/git/util_test.go`
- **Scope**: Tests `sanitizeBranchName()` utility only
- **Coverage**: 9 tests covering edge cases (spaces, special chars, leading/trailing dashes)
- **Limitation**: Does NOT test `findGitRepoRoot()` or `createInitialCommit()` — these are critical for new_project type and are currently untested at the unit level

#### `session/git/worktree_creation_test.go`
- **Scope**: Tests git worktree setup, diff stats, and storage round-trips
- **Key tests**:
  - `TestNewWorktreeSetup_SetsBaseCommitSHA` — verifies HEAD SHA is captured
  - `TestNewWorktreeSetup_WorktreePathExists` — verifies directory creation
  - `TestDiff_EmptyWorktree_ReturnsZeroStats` — baseline diff behavior
  - `TestDiff_WithChanges_ReturnsNonZeroStats` — diff detection
  - `TestDiff_DeletedFile_ReturnsRemovedLines` — diff edge case
  - `TestNewGitWorktreeFromStorage_RoundTrip` — serialization (partial read)
- **Pattern**: Setup test repo with initial commit, use `NewGitWorktree()`, call Setup()/Cleanup(), verify state
- **Limitation**: Tests `SessionTypeNewWorktree` path only; no `SessionTypeNewProject` or directory auto-creation scenarios

#### `server/services/session_service_create_test.go`
- **Scope**: Tests `CreateSession` RPC and `resolveSessionType()` routing logic
- **Key tests**:
  - `TestResolveSessionType_*` (9 tests) — exhaustive enum routing
    - Explicit types: `DIRECTORY`, `NEW_WORKTREE`, `EXISTING_WORKTREE`
    - Unspecified inference: branch → `NEW_WORKTREE`, existing_worktree field → `EXISTING_WORKTREE`, neither → `DIRECTORY`
    - **Critical**: `TestResolveSessionType_OneOffOverridesExplicitNewWorktree` — proves `one_off` flag wins (line 726 priority)
  - `TestCreateSession_EmptyTitle_ReturnsInvalidArgument` — validation
  - `TestCreateSession_EmptyPath_NonOneOff_ReturnsInvalidArgument` — path required except for one-off
  - `TestCreateSession_EmptyPath_OneOff_PassesPathValidation` — one-off exemption
  - `TestCreateSession_DuplicateTitle_ReturnsAlreadyExists` — title uniqueness
  - `TestCreateSession_OneOff_CreatesDirectoryInBaseDir` — one-off directory generation (uses `namegen.Generate()`)
  - `TestCreateSession_OneOff_TwoCallsCreateTwoDistinctDirectories` — concurrency safety
  - `TestCreateSession_OneOff_BadBaseDir_ReturnsInternalError` — error handling
- **Pattern**: Mock storage, inject event bus, call `CreateSession()`, verify connect error codes and side effects
- **Limitation**: No tests for non-existent path creation in Directory mode (the feature we're adding)

#### `server/services/oneshot_test.go`
- **Scope**: One-shot session tests (one-off variant)
- **Not directly relevant** to new_project but demonstrates staging patterns

### TypeScript (Frontend) Tests

#### `web-app/src/lib/omnibar/dispatch.test.ts`
- **Scope**: Routes `OmnibarAction` discriminated union → handler functions
- **Key tests**:
  - `describe("create_session")` — verifies payload marshalling
    - Checks `sessionType` field is passed correctly
    - Verifies `program` defaults to empty string if omitted
  - `describe("create_session (one-off)")` — maps `sessionType: "one_off"` → `oneOff: true, sessionType: undefined`
  - All other action types (navigate, clone, pause, resume, delete)
- **Pattern**: Mock `ActionDeps`, call `dispatchOmnibarAction()`, verify mock was called with correct args
- **Limitation**: Only checks marshalling; doesn't test UI state or detection logic

#### `web-app/src/lib/omnibar/detector.test.ts`
- **Scope**: Input detection → `InputType` and routing
- **Test IDs**:
  - `T-UNIT-TS-008` — bare word → `SessionSearch`
  - `T-UNIT-TS-009` — empty string → `Unknown` (not `SessionSearch`)
  - `T-UNIT-TS-010` — path input → `LocalPath` (not displaced by `SessionSearch`)
  - `T-UNIT-TS-011` — GitHub shorthand → `GitHubShorthand` (not displaced)
  - `T-PITFALL-001` — bare text does not fall through to Unknown
  - `T-PITFALL-002` — hyphenated bare text → `SessionSearch`
- **Key descriptor**: `NewSessionDetector` tests — `new/` prefix routing
- **Pattern**: Create registry, call `registry.detect(input)`, verify `result.type` and `result.parsedValue`
- **Limitation**: No tests for new session type variants (directory, new_worktree, new_project modes)

#### `tests/e2e/session-create-directory.spec.ts`
- **Scope**: E2E validation of Directory session creation UI
- **Tests**:
  - `directory type is selectable` — radio button UI
  - `hides branch controls when directory is selected` — conditional rendering
  - `shows working directory field for directory mode` — field visibility
  - `submit is disabled without a path` — validation state
  - `sends directory session type in payload` — RPC payload check (confirms `sessionType` ≠ NEW_WORKTREE/EXISTING_WORKTREE)
- **Port**: `http://localhost:8543` (configurable via `TEST_SERVER_URL`)
- **Pattern**: Open page, interact with UI, wait for state, intercept fetch/XHR, check payload
- **Limitation**: Tests existing Directory mode; does NOT test path-doesn't-exist scenario or confirmation dialog

#### `tests/e2e/session-create-new-worktree.spec.ts` & `session-create-existing-worktree.spec.ts`
- Similar patterns to directory test
- Focus on worktree-specific branching and repository selection

#### `tests/e2e/one-off-session.spec.ts`
- **Scope**: One-off session UI and creation
- **Tests**:
  - `shows one-off option in creation panel`
  - `hides path input when one-off is selected`
  - `creation panel stays visible while typing session name` (regression test)
  - `creates session with one_off flag` — RPC payload validation
- **Port**: `http://localhost:8544`
- **Pattern**: Similar to directory test

---

## Part 2: Tests That Must Be Updated

### 1. `server/services/session_service_create_test.go` — resolveSessionType Tests

**Change**: Add new test case(s) for `SESSION_TYPE_NEW_PROJECT`:

```go
func TestResolveSessionType_ExplicitNewProject(t *testing.T) {
    msg := &sessionv1.CreateSessionRequest{
        SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT,
    }
    assert.Equal(t, session.SessionTypeNewProject, resolveSessionType(msg, ""))
}

func TestResolveSessionType_OneOffOverridesExplicitNewProject(t *testing.T) {
    msg := &sessionv1.CreateSessionRequest{
        SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT,
        OneOff:      true,
    }
    // one_off always wins, must resolve to SessionTypeDirectory
    assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}
```

**Rationale**: Ensure `new_project` routes correctly and one-off priority is maintained.

### 2. `server/services/session_service_create_test.go` — Path Validation Tests

**Change**: Add test for relaxed path validation in new_project mode:

```go
func TestCreateSession_NewProject_EmptyPath_PassesValidation(t *testing.T) {
    // new_project should NOT require path to exist (will be created)
    storage := createTestStorage(t)
    svc := newCreateTestService(t, storage)
    
    nonExistentDir := filepath.Join(t.TempDir(), "does-not-exist")
    
    _, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
        Title:       "new-proj",
        Path:        nonExistentDir,
        SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT,
    }))
    
    if err != nil {
        assertNotConnectCode(t, err, connect.CodeInvalidArgument, 
            "new_project with non-existent path must not fail validation")
    } else {
        destroyCreatedSession(t, svc, err.Msg.Session.Id)
    }
}
```

**Rationale**: Validates that non-existent paths are allowed for `new_project` type.

### 3. `web-app/src/lib/omnibar/detector.test.ts` — Add Test ID T-UNIT-TS-012

**Change**: Add test for `new_project/` prefix detection (if you add a detector for it):

```typescript
// T-UNIT-TS-012
it("new_project/ prefix resolves to NewProject (if added)", () => {
    const result = registry.detect("new_project/my-app");
    expect(result.type).toBe(InputType.NewProject);
    expect(result.parsedValue).toBe("my-app");
});
```

**Note**: This assumes you add a `NewProjectDetector` to complement `NewSessionDetector`. If not, skip this.

### 4. `tests/e2e/session-create-directory.spec.ts` — Add Confirmation Dialog Test

**Change**: Add test for the path-doesn't-exist scenario:

```typescript
test('shows confirmation dialog when path does not exist', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();
    
    const nonExistentPath = '/tmp/this-path-should-not-exist-' + Date.now();
    await page.locator('input[aria-label="Session source input"]').fill(nonExistentPath);
    
    // Should show a confirmation/warning
    await expect(page.getByText(/create.*directory|path does not exist|confirm/i))
        .toBeVisible({ timeout: 3000 });
});

test('creates directory after user confirms path creation', async ({ page }) => {
    await openInCreationMode(page);
    await page.getByRole('radio', { name: 'Directory' }).click();
    
    const nonExistentPath = '/tmp/new-dir-' + Date.now();
    await page.locator('input[aria-label="Session source input"]').fill(nonExistentPath);
    
    // Accept the confirmation (e.g., button or checkbox)
    await page.getByRole('button', { name: /create.*confirm|yes/i }).click();
    
    // Verify the request includes create_if_missing: true
    const requestPromise = page.waitForRequest(req => req.url().includes('CreateSession'));
    // ... (complete session creation)
    const request = await requestPromise;
    const body = request.postDataJSON();
    expect(body.createIfMissing ?? false).toBe(true);
});
```

**Rationale**: Tests the new confirmation dialog feature.

### 5. `web-app/src/lib/omnibar/actions/dispatch.test.ts` — Add new_project Tests

**Change**: If `new_project` is handled as a distinct `sessionType` (not via a new action), add test:

```typescript
describe("create_session (new_project)", () => {
    it("dispatchOmnibarAction_should_sendNewProjectSessionType_When_sessionTypeIsNewProject", () => {
        const deps = makeDeps();
        const action: OmnibarAction = {
            type: "create_session",
            path: "/home/user/MyNewProject",
            sessionType: "new_project",
            title: "My New Project",
            program: "claude",
        };
        dispatchOmnibarAction(action, deps);
        expect(deps.createSession).toHaveBeenCalledWith(
            expect.objectContaining({ sessionType: "new_project" })
        );
        expect(deps.close).toHaveBeenCalled();
    });
});
```

**Rationale**: Ensures dispatch routing for new_project type.

---

## Part 3: New Tests That Must Be Added

### Go Unit Tests

#### 3.1: `session/git/util_test.go` — createInitialCommit

**New tests**:

```go
func TestCreateInitialCommit_Success(t *testing.T) {
    // Test that createInitialCommit() creates a .gitignore file and commits
    repoDir := t.TempDir()
    repo, err := git.PlainInit(repoDir, false)
    require.NoError(t, err)
    
    err = createInitialCommit(repo, repoDir)
    require.NoError(t, err)
    
    // Verify .gitignore exists
    gitignorePath := filepath.Join(repoDir, ".gitignore")
    assert.FileExists(t, gitignorePath)
    
    // Verify it contains expected content
    content, _ := os.ReadFile(gitignorePath)
    assert.Contains(t, string(content), "gitignore")
    
    // Verify HEAD commit exists
    ref, _ := repo.Head()
    assert.NotNil(t, ref)
}

func TestCreateInitialCommit_AlreadyCommitted_Idempotent(t *testing.T) {
    // If repository already has a commit, createInitialCommit should still work
    // (e.g., when called on existing repo for new_project that was already initialized)
    repoDir := t.TempDir()
    repo, _ := git.PlainInit(repoDir, false)
    
    // Create one commit manually
    wt, _ := repo.Worktree()
    f, _ := os.Create(filepath.Join(repoDir, "README.md"))
    f.WriteString("# Test")
    f.Close()
    wt.Add("README.md")
    wt.Commit("First", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}})
    
    // Try to create initial commit again (should not fail)
    err := createInitialCommit(repo, repoDir)
    // Behavior: either skip or overwrite — verify no error in either case
    require.NoError(t, err)
}
```

**Rationale**: `createInitialCommit()` is currently untested and critical for new_project type.

#### 3.2: `session/git/util_test.go` — findGitRepoRoot

**New tests**:

```go
func TestFindGitRepoRoot_PathNotExist_CreatesAndInits(t *testing.T) {
    // Test that findGitRepoRoot creates missing directory, inits repo, commits
    parentDir := t.TempDir()
    nonExistentPath := filepath.Join(parentDir, "new-repo")
    
    // Path must not exist initially
    _, err := os.Stat(nonExistentPath)
    require.True(t, os.IsNotExist(err))
    
    // Call findGitRepoRoot
    result, err := findGitRepoRoot(nonExistentPath)
    require.NoError(t, err)
    assert.Equal(t, nonExistentPath, result)
    
    // Verify directory was created
    assert.DirExists(t, nonExistentPath)
    
    // Verify it's a git repo with a commit
    repo, err := git.PlainOpen(nonExistentPath)
    require.NoError(t, err)
    ref, _ := repo.Head()
    assert.NotNil(t, ref, "HEAD commit must exist")
}

func TestFindGitRepoRoot_PathExistsNoGit_Inits(t *testing.T) {
    // Directory exists but no git — should init and commit
    dirPath := t.TempDir()
    
    result, err := findGitRepoRoot(dirPath)
    require.NoError(t, err)
    assert.Equal(t, dirPath, result)
    
    repo, _ := git.PlainOpen(dirPath)
    ref, _ := repo.Head()
    assert.NotNil(t, ref)
}

func TestFindGitRepoRoot_PathExistsWithGit_ReturnsNoInit(t *testing.T) {
    // Directory is already a git repo — should return root, no extra commit
    repoDir := t.TempDir()
    git.PlainInit(repoDir, false)
    
    result, err := findGitRepoRoot(repoDir)
    require.NoError(t, err)
    assert.Equal(t, repoDir, result)
}

func TestFindGitRepoRoot_PathExistsParentGit_ReturnsParentRoot(t *testing.T) {
    // Path is inside a git repo — should return the repo root
    repoDir := t.TempDir()
    git.PlainInit(repoDir, false)
    
    subDir := filepath.Join(repoDir, "subdir")
    os.Mkdir(subDir, 0755)
    
    result, err := findGitRepoRoot(subDir)
    require.NoError(t, err)
    assert.Equal(t, repoDir, result)
}

func TestFindGitRepoRoot_PermissionDenied_ReturnsError(t *testing.T) {
    // Test error handling: path parent not writable
    if os.Geteuid() == 0 {
        t.Skip("cannot test permission denied as root")
    }
    
    readOnlyDir := t.TempDir()
    os.Chmod(readOnlyDir, 0444)
    defer os.Chmod(readOnlyDir, 0755)
    
    nonExistentChild := filepath.Join(readOnlyDir, "child")
    _, err := findGitRepoRoot(nonExistentChild)
    require.Error(t, err)
}
```

**Rationale**: `findGitRepoRoot()` is core to new_project initialization and currently untested.

#### 3.3: `server/services/session_service_create_test.go` — New Project Creation

**New tests**:

```go
func TestCreateSession_NewProject_CreatesDirectoryAndInits(t *testing.T) {
    // Test that new_project type creates directory and initializes git
    storage := createTestStorage(t)
    svc := newCreateTestService(t, storage)
    
    parentDir := t.TempDir()
    newProjectPath := filepath.Join(parentDir, "my-new-project")
    
    // Path must not exist
    _, err := os.Stat(newProjectPath)
    require.True(t, os.IsNotExist(err))
    
    resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
        Title:       "new-proj-test",
        Path:        newProjectPath,
        SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT,
    }))
    
    if err == nil {
        defer destroyCreatedSession(t, svc, resp.Msg.Session.Id)
        
        // Verify directory was created
        assert.DirExists(t, newProjectPath)
        
        // Verify it's a git repo
        repo, gitErr := git.PlainOpen(newProjectPath)
        require.NoError(t, gitErr)
        ref, _ := repo.Head()
        assert.NotNil(t, ref, "should have initial commit")
    } else {
        // If tmux unavailable, error should NOT be CodeInvalidArgument (path validation)
        assertNotConnectCode(t, err, connect.CodeInvalidArgument)
    }
}

func TestCreateSession_NewProject_ExistingGitRepo_NoError(t *testing.T) {
    // If path is already a git repo, new_project should succeed (idempotent)
    storage := createTestStorage(t)
    svc := newCreateTestService(t, storage)
    
    repoDir := t.TempDir()
    git.PlainInit(repoDir, false)
    
    resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
        Title:       "existing-repo-proj",
        Path:        repoDir,
        SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT,
    }))
    
    if err == nil {
        defer destroyCreatedSession(t, svc, resp.Msg.Session.Id)
        assert.DirExists(t, repoDir)
    } else {
        assertNotConnectCode(t, err, connect.CodeInvalidArgument)
    }
}

func TestCreateSession_NewProject_PartialFailure_Cleanup(t *testing.T) {
    // PITFALL: If directory is created but git init fails, should we rm -rf or leave?
    // Current behavior: leave as-is (let user clean up manually or retry with existing repo)
    // This test documents the behavior for regression detection
    
    // This is implementation-dependent; add based on actual behavior
    t.Skip("implementation-dependent cleanup strategy")
}
```

**Rationale**: Validates new_project creation flow and edge cases.

#### 3.4: `server/services/session_service_create_test.go` — Directory Mode Confirmation

**New tests**:

```go
func TestCreateSession_Directory_PathNotExist_RequiresConfirmation(t *testing.T) {
    // Test that Directory mode + non-existent path returns specific error
    // when create_if_missing is not set (or false)
    storage := createTestStorage(t)
    svc := newCreateTestService(t, storage)
    
    nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist")
    
    _, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
        Title:       "needs-confirm",
        Path:        nonExistentPath,
        SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
        // create_if_missing: false (default)
    }))
    
    require.Error(t, err)
    assertConnectCode(t, err, connect.CodeNotFound)
    assert.Contains(t, err.Error(), "path does not exist")
}

func TestCreateSession_Directory_PathNotExist_CreatedIfConfirmed(t *testing.T) {
    // Test that Directory mode with create_if_missing=true creates the path
    storage := createTestStorage(t)
    svc := newCreateTestService(t, storage)
    
    nonExistentPath := filepath.Join(t.TempDir(), "confirmed-creation")
    
    resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
        Title:           "confirmed-dir",
        Path:            nonExistentPath,
        SessionType:     sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
        CreateIfMissing: true,
    }))
    
    if err == nil {
        defer destroyCreatedSession(t, svc, resp.Msg.Session.Id)
        assert.DirExists(t, nonExistentPath)
    } else {
        assertNotConnectCode(t, err, connect.CodeNotFound)
    }
}
```

**Rationale**: Tests the confirmation dialog backend (frontend will show dialog on CodeNotFound).

### TypeScript Unit Tests

#### 3.5: `web-app/src/lib/omnibar/detector.test.ts` — T-UNIT-TS-012 (or higher)

**New test**:

```typescript
// T-UNIT-TS-012 (or next available)
it("new_project/ prefix resolves to NewProject detection type", () => {
    const result = registry.detect("new_project/");
    expect(result.type).toBe(InputType.NewProject);
    expect(result.parsedValue).toBe("");
});

// T-UNIT-TS-013
it("new_project/my-app resolves to NewProject with parsed name", () => {
    const result = registry.detect("new_project/my-app");
    expect(result.type).toBe(InputType.NewProject);
    expect(result.parsedValue).toBe("my-app");
});
```

**Note**: Requires adding `NewProjectDetector` to the detector registry and `InputType.NewProject` enum variant.

### E2E Tests

#### 3.6: `tests/e2e/session-create-new-project.spec.ts` — New File

**New test file**:

```typescript
// @feature session:create-new-project
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8543';

test.describe('new project session creation', () => {
  test('new_project type is selectable', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    await page.keyboard.press('Control+Shift+K');
    
    await expect(page.getByRole('radio', { name: /new project/i })).toBeVisible({ timeout: 5000 });
    await page.getByRole('radio', { name: /new project/i }).click();
    await expect(page.getByRole('radio', { name: /new project/i })).toHaveAttribute('aria-checked', 'true');
  });

  test('shows project name input when new_project is selected', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    await page.keyboard.press('Control+Shift+K');
    
    await page.getByRole('radio', { name: /new project/i }).click();
    
    // Should show session name field
    await expect(page.getByLabel('Session Name')).toBeVisible();
    // Should show project name or path field
    await expect(page.getByPlaceholder(/project name|path/i)).toBeVisible();
  });

  test('sends new_project session type in payload', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    await page.keyboard.press('Control+Shift+K');
    
    await page.getByRole('radio', { name: /new project/i }).click();
    await page.getByLabel('Session Name').fill('my-new-project');
    await page.locator('input[aria-label="Session source input"]').fill('~/projects/my-new-project');
    
    const requestPromise = page.waitForRequest(req => req.url().includes('CreateSession'));
    await page.getByRole('button', { name: /create|start/i }).click();
    
    const request = await requestPromise;
    const body = request.postDataJSON();
    expect(body.sessionType).toBe(4); // SESSION_TYPE_NEW_PROJECT enum value
    expect(body.path).toBeTruthy();
  });
});
```

**Rationale**: E2E validation of new_project UI and RPC marshalling.

#### 3.7: `tests/e2e/session-create-directory.spec.ts` — Add Confirmation Tests

See Part 2, section 4 above for specific test additions.

---

## Part 4: Implementation Pitfalls to Avoid

### Pitfall 1: Cleanup on Partial Failure

**Issue**: If directory is created but git init fails (e.g., permission denied mid-init), should we rm -rf or leave the orphaned directory?

**Current Behavior** (in `findGitRepoRoot()`):
- `os.MkdirAll(path, 0755)` succeeds
- `git.PlainInit(path, false)` fails
- Orphaned empty directory remains

**Recommendation**:
- **Option A** (Recommended): Wrap in a transaction-like pattern:
  ```go
  tempDir := path + ".tmp"
  if err := os.MkdirAll(tempDir, 0755); err != nil { return "", ... }
  if err := git.PlainInit(tempDir, false); err != nil {
      os.RemoveAll(tempDir) // cleanup
      return "", ...
  }
  if err := os.Rename(tempDir, path); err != nil {
      os.RemoveAll(tempDir)
      return "", ...
  }
  ```
- **Option B**: Document "best-effort" semantics — don't rm -rf on error (let user clean up or retry)
- **Current Impact**: Low if git init rarely fails; high if hitting permission issues in CI

**Test**: `TestCreateSession_NewProject_PartialFailure_Cleanup` (add once strategy is decided)

### Pitfall 2: The `--feature sql/upsert` Ent Requirement

**Issue**: Does new_project touch the ent schema?

**Answer**: **NO**. Session type is stored as a string field in the existing schema:
```go
// session/ent/schema/instance.go
Type   string `json:"type"` // SessionTypeDirectory, SessionTypeNewWorktree, etc.
```

Adding `SessionTypeNewProject` does **NOT** require ent schema changes or `--feature sql/upsert`.

**Impact**: Zero — commit new_project without special ent handling.

### Pitfall 3: Proto Regeneration Gotchas

**Issue**: After adding `SESSION_TYPE_NEW_PROJECT = 4` to `proto/session/v1/types.proto`, must regenerate Go and TypeScript bindings.

**Steps**:
1. Edit `proto/session/v1/types.proto` — add enum value 4
2. Run `make generate-proto` (or equivalent)
3. Verify generated files in:
   - `gen/proto/go/session/v1/types.pb.go` (Go)
   - `gen/proto/connect-es/session/v1/types_pb.ts` (TypeScript)
4. Commit both — mismatches cause RPC failures

**Common Mistake**: Forgetting to regenerate → Proto mismatch → runtime errors like:
- "unknown enum value 4"
- TypeScript doesn't recognize `SessionType_SESSION_TYPE_NEW_PROJECT`

**Test**: Inspect generated files in git diff before committing.

### Pitfall 4: The `one_off` Override at Line 726

**Issue**: In `resolveSessionType()`, `one_off` always overrides to `SessionTypeDirectory`:
```go
if msg.OneOff {
    st = session.SessionTypeDirectory
}
```

If `new_project` ever needs to coexist with one-off mode (unlikely), this will silently break.

**Current Risk**: Low — one-off and new_project are mutually exclusive concepts.

**Safeguard**: Add assertion test:
```go
func TestResolveSessionType_OneOffOverridesNewProject(t *testing.T) {
    msg := &sessionv1.CreateSessionRequest{
        SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT,
        OneOff:      true, // Conflicting flags
    }
    // one_off always wins
    assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}
```

**Implication**: Cannot have `oneOff: true` + `sessionType: new_project` simultaneously — one-off takes priority.

### Pitfall 5: Proto Field Numbers and Backwards Compatibility

**Issue**: Adding `bool create_if_missing = 18` to `CreateSessionRequest` uses field 18 (already taken by `project_id`).

**Solution**: Use next available field number. Check current high field:
```bash
grep "= [0-9]*;" proto/session/v1/session.proto | tail -5
```

**Current state** (from stack.md): Field 17 is `project_id`, so next is **18**.

**But**: If adding field conflicts, use the next available (e.g., 19, 20).

**Test**: Proto file compiles without conflicts → `make generate-proto` succeeds.

### Pitfall 6: Directory Validation Exemption List

**Issue**: In `CreateSession()`, path validation checks:
```go
if !req.Msg.OneOff && req.Msg.Path == "" {
    return nil, connect.NewError(...)
}
```

For `new_project`, path may not exist initially — must also exempt:
```go
if !req.Msg.OneOff && 
   req.Msg.SessionType != sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT && 
   req.Msg.Path == "" {
    return nil, connect.NewError(...)
}
```

**Failure Mode**: Non-existent paths rejected during validation even for `new_project`.

**Test**: `TestCreateSession_NewProject_EmptyPath_PassesValidation` (if empty path is intentional for new_project)

### Pitfall 7: Working Directory Resolution for New Projects

**Issue**: In `session/instance.go`, `resolveStartPath()` validates that `WorkingDir` exists. For new projects, `WorkingDir` should be empty (start at project root).

**Current Code** (line ~1186):
```go
if i.WorkingDir != "" {
    // validate it exists
}
```

**For new_project**: Set `WorkingDir = ""` in the proto before returning, or skip validation if `SessionTypeNewProject`.

**Test**: Verify new project session starts at repo root (no working_dir).

### Pitfall 8: Config Loading and Backwards Compatibility

**Issue**: Adding `NewProjectBaseDir` to config must not break existing deployments.

**Safe Pattern** (from `config.go`):
```go
type Config struct {
    OneOffBaseDir     string `json:"one_off_base_dir,omitempty"`
    NewProjectBaseDir string `json:"new_project_base_dir,omitempty"` // NEW
}

func (c *Config) NewProjectBaseDirOrDefault() (string, error) {
    dir := c.NewProjectBaseDir
    if dir == "" { dir = "~/Projects" }
    // ... expand ~ ...
    return dir, nil
}
```

**Why Safe**: Zero-value defaults work; old configs load without change.

**Test**: Load old config file without `new_project_base_dir` → default to `~/Projects`.

---

## Part 5: Test Naming Convention & Next Available IDs

### T-UNIT-TS IDs

**Current usage** (from `detector.test.ts`):
- T-UNIT-TS-008 through T-UNIT-TS-011 (in detector.test.ts)
- T-PITFALL-001, T-PITFALL-002 (pitfall guards, in detector.test.ts)

**Next available**: **T-UNIT-TS-012**

**Format**: `T-{CATEGORY}-{LANGUAGE}-{NUMBER}`
- Category: UNIT, E2E, PITFALL
- Language: TS (TypeScript), GO (Go unit), INTEGRATION (cross-layer)
- Number: Sequential

**Suggested new IDs**:
- T-UNIT-TS-012: NewProject detector
- T-UNIT-TS-013: NewProject dispatch action
- T-UNIT-GO-001: createInitialCommit()
- T-UNIT-GO-002: findGitRepoRoot()
- T-UNIT-GO-003: new_project session creation
- T-E2E-001: new_project UI creation
- T-PITFALL-003: Partial failure cleanup

---

## Part 6: E2E Test Environment

### Port Configuration

E2E tests use **multiple ports** to avoid conflicts:

| Test File | Port | Purpose |
|-----------|------|---------|
| accessibility.spec.ts | 8543 (TEST_SERVER_URL) | Accessibility testing |
| demo.spec.ts | 8543 | Demo/general |
| session-create-directory.spec.ts | 8543 | Directory session creation |
| one-off-session.spec.ts | 8544 | One-off session creation |
| nav-navigation.spec.ts | 8544 | Navigation tests |
| session-create-new-worktree.spec.ts | (check file) | Worktree creation |
| session-create-existing-worktree.spec.ts | (check file) | Existing worktree |

### Environment Variable

```bash
export TEST_SERVER_URL=http://localhost:8543
```

Overrides hardcoded defaults in test files.

### New Project E2E Test

Recommend adding new test on **port 8543** (main UI port) in file `tests/e2e/session-create-new-project.spec.ts`.

---

## Part 7: Summary Table of Changes

| Artifact | Change | Priority | Tested By |
|----------|--------|----------|-----------|
| `proto/session/v1/types.proto` | Add SESSION_TYPE_NEW_PROJECT = 4 | HIGH | Compilation |
| `session/instance.go` | Add SessionTypeNewProject constant | HIGH | T-UNIT-GO-003 |
| `server/services/session_service.go` | Add new_project case to resolveSessionType() | HIGH | TestResolveSessionType_ExplicitNewProject |
| `server/services/session_service.go` | Exempt new_project from path validation | HIGH | TestCreateSession_NewProject_* |
| `session/git/util.go` | (Existing createInitialCommit, no change) | — | T-UNIT-GO-001 |
| `session/git/util.go` | Add InitializeProjectDirectory() or reuse findGitRepoRoot() | HIGH | TestFindGitRepoRoot_* |
| `session/instance.go` | setupFirstTimeWorktree() — add case for new_project | HIGH | T-UNIT-GO-003 |
| `config/config.go` | Add NewProjectBaseDir + method | MEDIUM | Integration test |
| `proto/session/v1/session.proto` | Add `bool create_if_missing = 18` (optional, for confirmation) | MEDIUM | TestCreateSession_Directory_PathNotExist_* |
| `server/services/session_service.go` | Handle create_if_missing in Directory mode | MEDIUM | TestCreateSession_Directory_PathNotExist_* |
| `web-app/src/lib/omnibar/types.ts` | (No change if sessionType string is reused) | — | — |
| `web-app/src/lib/omnibar/detector.ts` | Add NewProjectDetector (if new_project/ prefix) | MEDIUM | T-UNIT-TS-012 |
| `web-app/src/lib/omnibar/actions/dispatch.ts` | Handle sessionType: "new_project" | MEDIUM | T-UNIT-TS-013 |
| `tests/e2e/session-create-directory.spec.ts` | Add confirmation dialog tests | MEDIUM | E2E confirmation tests |
| `tests/e2e/session-create-new-project.spec.ts` | New file: new_project E2E tests | HIGH | T-E2E-001 |

---

## Part 8: Validation Checklist Before Merge

- [ ] Proto regenerated: `make generate-proto`
- [ ] Generated files committed (Go + TypeScript)
- [ ] All resolveSessionType tests pass (including new_project + one_off interaction)
- [ ] All directory validation tests pass (path exists / doesn't exist, create_if_missing flag)
- [ ] All git init tests pass (createInitialCommit, findGitRepoRoot)
- [ ] E2E tests pass on configured port (TEST_SERVER_URL)
- [ ] No ent schema changes (verify with `git diff` on schema files)
- [ ] Config loads old files without new_project_base_dir (backwards compat)
- [ ] One-off priority override preserved (test included)
- [ ] Directory cleanup on partial failure documented (test or comment)
- [ ] T-UNIT-TS and T-UNIT-GO test IDs assigned and in registry

---

## References

- `server/services/session_service_create_test.go` (existing CreateSession tests)
- `session/git/util.go` (git init logic)
- `session/namegen/namegen.go` (one-off directory generation pattern)
- `project_plans/new-project-creation/research/stack.md` (feature design)
- Feature registry: `.claude/rules/feature-testing-registry.md`
