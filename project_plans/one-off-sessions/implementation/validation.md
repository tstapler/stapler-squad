# Validation Plan: One-Off Sessions

**Feature**: One-Off Sessions  
**Date**: 2026-04-24  
**Requirements source**: `project_plans/one-off-sessions/requirements.md`  
**Phase**: MDD Phase 4 — Validation (before any code is written)

---

## Requirements Coverage Matrix

| Req | Description | UT | IT | FE | Manual |
|---|---|---|---|---|---|
| US-1 | One-off session creates dir and starts via web UI | — | IT-1, IT-2 | FE-1, FE-2, FE-3 | M-1, M-2, M-3, M-4 |
| US-2 | `one_off_base_dir` config field with default `~/oneoff` | UT-5, UT-6, UT-7 | IT-1 | — | M-5 |
| US-3 | Directory name format, uniqueness, collision retry | UT-1, UT-2, UT-3, UT-4, UT-5 | IT-1 | — | M-6 |
| US-4 | Directory persists after session destroy | — | IT-3 | — | M-7 |
| NFR | Shell-safe names, embedded word lists, O(1) generation | UT-2, UT-3 | — | — | — |

**Summary**: 7 unit tests · 3 integration tests · 3 frontend tests · 7 manual steps  
**Requirements coverage**: 4/4 user stories, all NFRs

---

## Unit Tests (Go) — `session/namegen`

New package: `session/namegen/namegen.go`  
Test file: `session/namegen/namegen_test.go`

---

### UT-1 — Generate: correct format

**Covers**: US-3 AC1 — "Directory name format: `YYYYMMDD-<adjective>-<noun>-<number>`"

**Test name**: `TestGenerate_Format`

**Description**: Call `Generate()` once and assert the output matches the required format regex. Verify the date portion matches today's date in local time.

**Inputs**: None (calls `Generate()` with current time)

**Expected output**:
- Result matches `^\d{8}-[a-z]+-[a-z]+-\d{2}$`
- Date prefix == `time.Now().Format("20060102")`
- Total length ≤ 32 characters

**Implementation sketch**:
```go
func TestGenerate_Format(t *testing.T) {
    name := namegen.Generate()
    re := regexp.MustCompile(`^\d{8}-[a-z]+-[a-z]+-\d{2}$`)
    assert.Regexp(t, re, name)
    assert.LessOrEqual(t, len(name), 32, "name must be ≤ 32 chars, got %d: %s", len(name), name)
    datePrefix := time.Now().Format("20060102")
    assert.True(t, strings.HasPrefix(name, datePrefix), "name must start with today's date %s, got %s", datePrefix, name)
}
```

---

### UT-2 — Generate: shell-safe characters only

**Covers**: US-3 NFR — "Generated directory name is URL-safe and shell-safe (lowercase letters, digits, hyphens only)"

**Test name**: `TestGenerate_ShellSafe`

**Description**: Call `Generate()` 1,000 times and assert every result contains only `[a-z0-9-]`. This also validates the embedded word lists produce no special characters.

**Inputs**: 1,000 calls to `Generate()`

**Expected output**: Every result matches `^[a-z0-9-]+$`; no result is empty

**Implementation sketch**:
```go
func TestGenerate_ShellSafe(t *testing.T) {
    re := regexp.MustCompile(`^[a-z0-9-]+$`)
    for i := 0; i < 1000; i++ {
        name := namegen.Generate()
        assert.Regexp(t, re, name, "iteration %d: name %q contains unsafe chars", i, name)
        assert.NotEmpty(t, name)
    }
}
```

---

### UT-3 — Generate: uniqueness across rapid calls

**Covers**: US-3 AC3, AC4 — adjective/noun from embedded word lists, number 00–99

**Test name**: `TestGenerate_Uniqueness`

**Description**: Call `Generate()` 500 times in a tight loop on the same date and verify that at least 90% of results are distinct. (They will not all be unique due to random collisions in a 640,000-space, but the rate of collision must be low.) Also confirms numbers 00–99 are included in the output space.

**Inputs**: 500 calls to `Generate()`

**Expected output**:
- Distinct count ≥ 450 (90% unique)
- All numbers in results are in `[00, 99]` range (parsed from suffix)

**Implementation sketch**:
```go
func TestGenerate_Uniqueness(t *testing.T) {
    seen := make(map[string]bool, 500)
    re := regexp.MustCompile(`-(\d{2})$`)
    for i := 0; i < 500; i++ {
        name := namegen.Generate()
        seen[name] = true
        m := re.FindStringSubmatch(name)
        require.Len(t, m, 2, "should have 2-digit suffix")
        n, _ := strconv.Atoi(m[1])
        assert.GreaterOrEqual(t, n, 0)
        assert.LessOrEqual(t, n, 99)
    }
    assert.GreaterOrEqual(t, len(seen), 450)
}
```

---

### UT-4 — Generate: collision retry returns unique name

**Covers**: US-3 AC5 — "If the generated path already exists, regenerate until a unique name is found (max 10 attempts)"

**Test name**: `TestGenerateUnique_RetryOnCollision`

**Description**: Create a temporary directory. Pre-create several generated names as directories. Verify that `GenerateUnique(baseDir, 10)` still returns a path that does not exist on disk by exhausting the pre-created ones. Uses a controlled random seed (via monkey-patching `time.Now` or by pre-creating many dirs) to force collisions.

**Approach**: Since seeding `math/rand` directly is possible in tests (Go 1.20+ global source), use a wrapper function that accepts a `generateFn func() string` parameter, passing in a stub that returns colliding names for the first N calls, then a unique name. The public `GenerateUnique` should accept this as a functional option or the test can use an internal test helper.

**Inputs**:
- `baseDir`: `t.TempDir()`
- A `generateFn` stub that returns `"20260424-brave-falcon-01"` for the first 3 calls, then `"20260424-calm-otter-42"` on call 4
- Pre-create `baseDir/20260424-brave-falcon-01`

**Expected output**: `GenerateUnique` returns `filepath.Join(baseDir, "20260424-calm-otter-42")`, nil after 4 internal calls; returned path does not exist on disk (because `GenerateUnique` uses `os.Mkdir` not `os.MkdirAll` for the leaf)

**Implementation sketch**:
```go
func TestGenerateUnique_RetryOnCollision(t *testing.T) {
    baseDir := t.TempDir()
    // Pre-create the "collision" directory
    require.NoError(t, os.Mkdir(filepath.Join(baseDir, "20260424-brave-falcon-01"), 0755))

    callCount := 0
    stubFn := func() string {
        callCount++
        if callCount <= 3 {
            return "20260424-brave-falcon-01" // always collides
        }
        return "20260424-calm-otter-42"
    }

    path, err := namegen.GenerateUniqueWithFn(baseDir, 10, stubFn)
    require.NoError(t, err)
    assert.Equal(t, filepath.Join(baseDir, "20260424-calm-otter-42"), path)
    // The directory must have been created
    _, statErr := os.Stat(path)
    assert.NoError(t, statErr, "GenerateUnique must create the directory")
    assert.Equal(t, 4, callCount)
}
```

---

### UT-5 — Generate: error after max attempts exhausted

**Covers**: US-3 AC5 — "fail with an error after [10 attempts]"

**Test name**: `TestGenerateUnique_ErrorAfterMaxAttempts`

**Description**: Supply a `generateFn` stub that always returns the same name. Pre-create that directory. Assert `GenerateUnique` returns a non-nil error after exactly `maxAttempts` calls.

**Inputs**:
- `baseDir`: `t.TempDir()`
- Pre-created: `baseDir/20260424-stuck-owl-00`
- `generateFn` always returns `"20260424-stuck-owl-00"`
- `maxAttempts = 10`

**Expected output**: `GenerateUnique` returns `("", error)` where `error` message contains "failed to generate unique" or similar; stub was called exactly 10 times

**Implementation sketch**:
```go
func TestGenerateUnique_ErrorAfterMaxAttempts(t *testing.T) {
    baseDir := t.TempDir()
    require.NoError(t, os.Mkdir(filepath.Join(baseDir, "20260424-stuck-owl-00"), 0755))

    callCount := 0
    stubFn := func() string {
        callCount++
        return "20260424-stuck-owl-00"
    }

    path, err := namegen.GenerateUniqueWithFn(baseDir, 10, stubFn)
    assert.Error(t, err)
    assert.Empty(t, path)
    assert.Equal(t, 10, callCount)
    assert.Contains(t, err.Error(), "failed to generate unique")
}
```

---

### UT-6 — Word list size assertions

**Covers**: US-3 AC3 — "Adjective and noun are drawn from a fixed curated word list (≥ 50 adjectives, ≥ 50 nouns)"

**Test name**: `TestWordLists_MinimumSize`

**Description**: Export `Adjectives` and `Nouns` slices (or expose via `WordListLengths()`) and assert both have at least 50 entries. Assert no entry contains characters outside `[a-z]`.

**Inputs**: The compiled binary's embedded word lists

**Expected output**:
- `len(adjectives) >= 50`
- `len(nouns) >= 50`
- Every entry matches `^[a-z]+$`

**Implementation sketch**:
```go
func TestWordLists_MinimumSize(t *testing.T) {
    adjs, nouns := namegen.ExportedWordLists()
    assert.GreaterOrEqual(t, len(adjs), 50, "need ≥ 50 adjectives")
    assert.GreaterOrEqual(t, len(nouns), 50, "need ≥ 50 nouns")

    re := regexp.MustCompile(`^[a-z]+$`)
    for _, w := range adjs {
        assert.Regexp(t, re, w, "adjective %q must be lowercase letters only", w)
    }
    for _, w := range nouns {
        assert.Regexp(t, re, w, "noun %q must be lowercase letters only", w)
    }
}
```

---

## Unit Tests (Go) — `config` package

Test file: `config/config_test.go` (add new test functions to existing file)

---

### UT-7 — `Config.OneOffBaseDir`: empty string returns `~/oneoff` expanded

**Covers**: US-2 AC3, AC4 — "Default value: `~/oneoff`", "If empty or not set, use the default"

**Test name**: `TestOneOffBaseDirOrDefault_Empty`

**Description**: Call `OneOffBaseDirOrDefault()` (or equivalent helper) on a `Config` with `OneOffBaseDir == ""`. Verify the result equals `filepath.Join(os.UserHomeDir(), "oneoff")`.

**Inputs**: `&Config{OneOffBaseDir: ""}`

**Expected output**: `filepath.Join(homeDir, "oneoff")`, nil

**Implementation sketch**:
```go
func TestOneOffBaseDirOrDefault_Empty(t *testing.T) {
    cfg := &config.Config{}
    home, err := os.UserHomeDir()
    require.NoError(t, err)

    result, err := cfg.OneOffBaseDirOrDefault()
    require.NoError(t, err)
    assert.Equal(t, filepath.Join(home, "oneoff"), result)
}
```

---

### UT-8 — `Config.OneOffBaseDir`: tilde expansion

**Covers**: US-2 — "Default value: `~/oneoff` (expanded at runtime)"

**Test name**: `TestOneOffBaseDirOrDefault_TildeExpansion`

**Description**: Set `OneOffBaseDir = "~/my-oneoffs"`. Assert the returned path equals `filepath.Join(homeDir, "my-oneoffs")` — not a literal `~`.

**Inputs**: `&Config{OneOffBaseDir: "~/my-oneoffs"}`

**Expected output**: `filepath.Join(homeDir, "my-oneoffs")`, nil — no literal `~` in result

**Implementation sketch**:
```go
func TestOneOffBaseDirOrDefault_TildeExpansion(t *testing.T) {
    cfg := &config.Config{OneOffBaseDir: "~/my-oneoffs"}
    home, err := os.UserHomeDir()
    require.NoError(t, err)

    result, err := cfg.OneOffBaseDirOrDefault()
    require.NoError(t, err)
    assert.Equal(t, filepath.Join(home, "my-oneoffs"), result)
    assert.False(t, strings.HasPrefix(result, "~"), "result must not contain literal tilde")
}
```

---

### UT-9 — `Config.OneOffBaseDir`: custom absolute path passes through

**Covers**: US-2 — user can set any base directory

**Test name**: `TestOneOffBaseDirOrDefault_CustomAbsolutePath`

**Description**: Set `OneOffBaseDir` to an absolute path without a tilde. Assert the returned path equals the input exactly (no modification).

**Inputs**: `&Config{OneOffBaseDir: "/tmp/my-custom-oneoffs"}`

**Expected output**: `"/tmp/my-custom-oneoffs"`, nil

**Implementation sketch**:
```go
func TestOneOffBaseDirOrDefault_CustomAbsolutePath(t *testing.T) {
    cfg := &config.Config{OneOffBaseDir: "/tmp/my-custom-oneoffs"}

    result, err := cfg.OneOffBaseDirOrDefault()
    require.NoError(t, err)
    assert.Equal(t, "/tmp/my-custom-oneoffs", result)
}
```

---

### UT-10 — `Config.OneOffBaseDir`: field round-trips through JSON

**Covers**: US-2 AC1 — "`one_off_base_dir` field in `config.Config` (JSON key: `"one_off_base_dir"`)"

**Test name**: `TestOneOffBaseDir_JSONRoundTrip`

**Description**: Marshal a `Config` with `OneOffBaseDir = "~/oneoff"` to JSON and unmarshal it. Assert the field survives and the JSON key is `"one_off_base_dir"`. Also verify that marshaling a `Config` with `OneOffBaseDir = ""` omits the key from JSON (due to `omitempty`).

**Inputs**: `&Config{OneOffBaseDir: "~/oneoff"}`; `&Config{OneOffBaseDir: ""}`

**Expected output**:
- Loaded config has `OneOffBaseDir == "~/oneoff"`
- JSON bytes contain `"one_off_base_dir"` key for non-empty value
- JSON bytes do NOT contain `"one_off_base_dir"` key when value is empty

**Implementation sketch**:
```go
func TestOneOffBaseDir_JSONRoundTrip(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "config.json")

    cfg := &config.Config{OneOffBaseDir: "~/oneoff"}
    require.NoError(t, saveConfig(cfg, path))

    loaded, err := LoadConfigFromPath(path)
    require.NoError(t, err)
    assert.Equal(t, "~/oneoff", loaded.OneOffBaseDir)

    // JSON contains the key
    raw, err := os.ReadFile(path)
    require.NoError(t, err)
    assert.Contains(t, string(raw), `"one_off_base_dir"`)

    // omitempty: empty value omits key
    emptyCfg := &config.Config{}
    emptyPath := filepath.Join(dir, "empty-config.json")
    require.NoError(t, saveConfig(emptyCfg, emptyPath))
    emptyRaw, err := os.ReadFile(emptyPath)
    require.NoError(t, err)
    assert.NotContains(t, string(emptyRaw), `"one_off_base_dir"`)
}
```

---

## Integration Tests (Go) — `server/services`

Test file: `server/services/session_service_oneoff_test.go` (new file in existing package)

These tests exercise the full `CreateSession` RPC path against a real (but test-isolated) `SessionService`. They use `t.TempDir()` for all filesystem operations and set `STAPLER_SQUAD_INSTANCE` to an isolated value to prevent state bleed.

---

### IT-1 — `CreateSession` with `one_off=true`: directory created, session starts, path in response

**Covers**: US-1 AC3, AC4, AC5; US-2 AC2; US-3 all ACs

**Test name**: `TestCreateSession_OneOff_HappyPath`

**Description**: Call `CreateSession` with `one_off=true`, `title="Exploration"`, and no `path` field. Assert:
1. No error returned.
2. Response `session.path` is inside the resolved `one_off_base_dir`.
3. The path on disk exists and is a directory.
4. `session.session_type` is `SessionTypeDirectory`.
5. `session.title` is `"Exploration"` (not the directory name).
6. The directory name under `one_off_base_dir` matches `^\d{8}-[a-z]+-[a-z]+-\d{2}$`.

**Setup**:
- Create a temp dir for `one_off_base_dir` via `t.TempDir()`
- Construct `SessionService` with a test-isolated storage and config pointing at temp dir
- Use a `Program` of `"bash -c 'sleep 1'"` to avoid needing real claude

**Inputs**:
```
CreateSessionRequest{
    Title:  "Exploration",
    OneOff: true,
    // Path intentionally omitted
}
```
Config: `OneOffBaseDir = tempDir`

**Expected output**:
- `err == nil`
- `resp.Session.Path` has `tempDir` as prefix
- `os.Stat(resp.Session.Path)` succeeds (directory exists)
- `resp.Session.Title == "Exploration"`
- `resp.Session.SessionType == SESSION_TYPE_DIRECTORY`
- Basename of `resp.Session.Path` matches name regex

**Key assertion**:
```go
assert.True(t, strings.HasPrefix(resp.Msg.Session.Path, tempDir))
info, err := os.Stat(resp.Msg.Session.Path)
require.NoError(t, err)
assert.True(t, info.IsDir())
re := regexp.MustCompile(`^\d{8}-[a-z]+-[a-z]+-\d{2}$`)
assert.Regexp(t, re, filepath.Base(resp.Msg.Session.Path))
assert.Equal(t, "Exploration", resp.Msg.Session.Title)
```

---

### IT-2 — `CreateSession` with `one_off=true` and non-creatable base dir: returns error

**Covers**: US-2 backend validation NFR — "reject creation if `one_off_base_dir` resolves to a non-existent and non-creatable path"

**Test name**: `TestCreateSession_OneOff_BadBaseDir_ReturnsError`

**Description**: Point `one_off_base_dir` at a path the process cannot create (e.g., `/root/no-permission` on macOS, or a path under a read-only mount). Simpler approach: use a path whose parent is a regular file (not a directory), making `os.MkdirAll` fail with `ENOTDIR`.

**Setup**:
- Create `t.TempDir()/blocker` as a regular file
- Set `OneOffBaseDir = t.TempDir() + "/blocker/oneoff"` — parent is a file, cannot be a directory

**Inputs**:
```
CreateSessionRequest{
    Title:  "Blocked",
    OneOff: true,
}
```

**Expected output**:
- `err != nil`
- Connect error code is `CodeInvalidArgument` or `CodeInternal`
- Error message contains "cannot create" or "one_off_base_dir"
- No session created in storage

---

### IT-3 — Directory persists after session destroy

**Covers**: US-4 — "Destroying a one-off session does NOT delete the generated directory"

**Test name**: `TestCreateSession_OneOff_DirectoryPersistsAfterDestroy`

**Description**: Create a one-off session (IT-1 happy path). Call `DestroySession` (or `instance.Destroy()`). Assert the directory created during `CreateSession` still exists on disk.

**Inputs**: Session created via `CreateSession` with `one_off=true`; then `DestroySession{id: session.id}`

**Expected output**:
- `DestroySession` returns no error
- `os.Stat(sessionPath)` still succeeds after destroy — directory remains

**Key assertion**:
```go
sessionPath := resp.Msg.Session.Path
_, err = svc.DestroySession(ctx, connect.NewRequest(&sessionv1.DestroySessionRequest{Id: resp.Msg.Session.Id}))
require.NoError(t, err)
_, statErr := os.Stat(sessionPath)
assert.NoError(t, statErr, "one-off directory must survive session destroy")
```

---

## Frontend Tests (Jest / React Testing Library)

Test file: `web-app/src/components/CreateSessionDialog/__tests__/CreateSessionDialog.oneoff.test.tsx`  
(or alongside the existing `CreateSessionDialog` / new session form component)

These tests use RTL with `@testing-library/user-event` and mock the ConnectRPC client.

---

### FE-1 — One-off option visible in creation form

**Covers**: US-1 AC1 — "A 'One-off session' option is visible and selectable in the session creation UI"

**Test name**: `renders one-off session option in creation form`

**Description**: Render the `CreateSessionDialog` (or new session form). Assert a radio button, checkbox, or toggle labeled "One-off session" (or similar) is present in the DOM and is not disabled.

**Inputs**: Default-rendered component with no props override

**Expected output**:
- `getByRole('radio', { name: /one.off session/i })` or `getByLabelText(/one.off/i)` is in document
- Element is not disabled

**Implementation sketch**:
```tsx
it('renders one-off session option', () => {
    render(<CreateSessionDialog />);
    const option = screen.getByRole('radio', { name: /one.off session/i });
    expect(option).toBeInTheDocument();
    expect(option).not.toBeDisabled();
});
```

---

### FE-2 — When one-off selected: path input hidden, title input present

**Covers**: US-1 AC2 — "When selected, the directory field is hidden; only a session title field is shown"

**Test name**: `hides path input and shows title input when one-off selected`

**Description**: Render the form. Click/select the "One-off session" option. Assert the path/directory input is removed from the DOM (or has `display: none` / `aria-hidden`). Assert the title input remains present and is focused or at least enabled.

**Inputs**: User clicks one-off radio/toggle

**Expected output**:
- Path input: `queryByLabelText(/directory|path/i)` returns null (or `not.toBeVisible()`)
- Title input: `getByLabelText(/title|session name/i)` is in document and enabled

**Implementation sketch**:
```tsx
it('hides path input when one-off selected', async () => {
    render(<CreateSessionDialog />);
    const oneOffOption = screen.getByRole('radio', { name: /one.off session/i });
    await userEvent.click(oneOffOption);

    expect(screen.queryByLabelText(/directory|path/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/title|session name/i)).toBeEnabled();
});
```

---

### FE-3 — Submit sends `one_off: true` with empty path

**Covers**: US-1 AC3 — "On submit, a new directory is created"; backend receives `one_off: true`

**Test name**: `submits with one_off: true and no path when one-off selected`

**Description**: Render the form with a mocked `createSession` RPC function. Select the one-off option. Type a title. Click submit. Assert the mock was called with `{ one_off: true, title: "My Exploration", path: "" }` (or path omitted entirely) — NOT with a user-supplied path.

**Inputs**:
- User selects "One-off session"
- User types `"My Exploration"` into the title field
- User clicks Submit

**Expected output**:
- Mock `createSession` called exactly once
- Called with `req.one_off === true`
- Called with `req.title === "My Exploration"`
- Called with `req.path === ""` or `req.path` is absent/undefined

**Implementation sketch**:
```tsx
it('submits one_off: true with no path', async () => {
    const mockCreate = jest.fn().mockResolvedValue({ session: { id: 'abc', title: 'My Exploration' } });
    render(<CreateSessionDialog onCreateSession={mockCreate} />);

    await userEvent.click(screen.getByRole('radio', { name: /one.off session/i }));
    await userEvent.type(screen.getByLabelText(/title|session name/i), 'My Exploration');
    await userEvent.click(screen.getByRole('button', { name: /create|start/i }));

    expect(mockCreate).toHaveBeenCalledTimes(1);
    expect(mockCreate).toHaveBeenCalledWith(
        expect.objectContaining({ one_off: true, title: 'My Exploration' })
    );
    // path must not be a user-supplied value
    const callArg = mockCreate.mock.calls[0][0];
    expect(callArg.path ?? '').toBe('');
});
```

---

## Manual Verification Checklist

These steps verify acceptance criteria that require a running application instance. Run after implementation, before PR merge.

---

### M-1 — One-off option appears in new-session dialog

**Covers**: US-1 AC1

**Steps**:
1. Start `./stapler-squad` (`make restart-web`)
2. Open http://localhost:8543
3. Click "New Session" or press the new-session keyboard shortcut
4. **Verify**: A "One-off session" option (radio button, toggle, or checkbox) is visible in the dialog/form

**Pass**: Option is visible and not greyed out  
**Fail**: Option is absent or disabled

---

### M-2 — Path field hidden when one-off selected

**Covers**: US-1 AC2

**Steps**:
1. Open the new-session dialog (see M-1)
2. Select the "One-off session" option
3. **Verify**: The directory/path input field disappears (hidden or removed from DOM)
4. **Verify**: A title input field is present and accepting text

**Pass**: Directory field gone, title field visible and editable  
**Fail**: Directory field still visible, or title field absent

---

### M-3 — One-off session creates directory with correct name

**Covers**: US-1 AC3, AC5; US-3 all ACs

**Steps**:
1. Select "One-off session"
2. Enter title `"Quick Experiment"`
3. Click Create
4. In a terminal: `ls ~/oneoff/` (or custom `one_off_base_dir`)
5. **Verify**: A new directory exists matching pattern `YYYYMMDD-<adjective>-<noun>-<NN>` where date is today
6. **Verify**: Directory name contains only lowercase letters, digits, and hyphens

**Pass**: Directory present, name matches pattern, shell-safe characters only  
**Fail**: Directory missing, name format wrong, or non-safe characters present

---

### M-4 — Session title is user-supplied, not directory name

**Covers**: US-1 AC5 — "The session title is whatever the user typed (not the directory name)"

**Steps**:
1. Create a one-off session with title `"Quick Experiment"` (see M-3)
2. Find the new session in the session list
3. **Verify**: Session card/list item shows `"Quick Experiment"` as the title
4. **Verify**: Session detail view shows the generated directory path (e.g., `~/oneoff/20260424-brave-falcon-07`) — not the title used as path

**Pass**: Title is `"Quick Experiment"`; path is the generated directory  
**Fail**: Title is the directory name, or path is empty/wrong

---

### M-5 — `one_off_base_dir` config field is respected

**Covers**: US-2 all ACs

**Steps**:
1. Edit `~/.stapler-squad/config.json` and add `"one_off_base_dir": "~/my-sandbox"` (create the file if needed via settings UI or directly)
2. Restart stapler-squad
3. Create a one-off session (M-3 steps)
4. **Verify**: New directory is created under `~/my-sandbox/`, not `~/oneoff/`
5. **Verify**: `~/my-sandbox/` was auto-created if it didn't exist
6. Remove the `one_off_base_dir` key from config and restart
7. Create another one-off session
8. **Verify**: Directory is now created under the default `~/oneoff/`

**Pass**: Config field controls base dir; default works when field absent  
**Fail**: Directory ignores config, always uses `~/oneoff`, or fails when dir doesn't exist

---

### M-6 — Directory path visible in session detail

**Covers**: US-1 AC6 — "The generated directory path is visible in the session detail view"

**Steps**:
1. Create a one-off session
2. Click on the session to open detail/info view
3. **Verify**: The generated path (e.g., `~/oneoff/20260424-witty-penguin-42`) is displayed somewhere in the session detail panel

**Pass**: Full generated path shown in UI  
**Fail**: Path is absent, truncated incorrectly, or shows only the title

---

### M-7 — Directory persists after session destroy

**Covers**: US-4 all ACs

**Steps**:
1. Create a one-off session; note the generated directory path
2. In terminal: `ls <generated-path>` — confirm directory exists
3. In the web UI, click Destroy (or Stop + Destroy) on the session
4. Confirm the destroy action
5. In terminal: `ls <generated-path>` again
6. **Verify**: The directory still exists and is not deleted
7. **Verify**: Any files created by the AI inside the directory are still present

**Pass**: Directory and contents survive session destroy  
**Fail**: Directory deleted on destroy, or contents removed

---

## Test Count Summary

| Category | Count | Requirements |
|---|---|---|
| Unit tests (Go — `namegen`) | 6 (UT-1 through UT-6) | US-3, NFR |
| Unit tests (Go — `config`) | 4 (UT-7 through UT-10) | US-2 |
| Integration tests (Go) | 3 (IT-1 through IT-3) | US-1, US-2, US-4 |
| Frontend tests (Jest/RTL) | 3 (FE-1 through FE-3) | US-1 |
| Manual verification steps | 7 (M-1 through M-7) | US-1, US-2, US-3, US-4 |
| **Total automated** | **13** | |
| **Total including manual** | **20** | |

**Requirements coverage**: 4/4 user stories (100%) · All NFRs covered by UT-2, UT-3, UT-6

---

## Risk Notes (from pitfalls.md)

These are not additional test cases — they are implementation constraints the tests implicitly verify:

1. **Race safety**: IT-1 uses `os.Mkdir` (not `os.MkdirAll`) for the leaf directory. The test verifies the created path exists but was not pre-existing, implying the atomic mkdir behavior is correct.

2. **Directory before `NewInstance`**: IT-1 confirms the generated directory exists when the session starts — validating that creation happens before `session.NewInstance` is called (not after).

3. **Title length**: UT-1 asserts `len(name) <= 32`. The 32-char constraint applies to the _directory name_, not the user title; no test enforces a 32-char limit on the title in `CreateSession` because that validation pre-existed and is not part of this feature.

4. **Config not auto-saved**: No test asserts that `SaveConfig` is called when a one-off session is created with an empty `one_off_base_dir`. The lazy default approach (no auto-save) must be verified by UT-7 returning the expanded default without mutating the config object.
