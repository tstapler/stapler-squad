# Implementation Plan: One-Off Sessions

**Feature**: One-Off Sessions  
**Date**: 2026-04-24  
**Status**: Ready for implementation  
**Requirements**: `../requirements.md`  
**Research**: `../research/`

---

## Summary

4 epics, 13 tasks. No blocked items. One risky item (noted in Epic 3).

---

## Architecture Decision

- One-off sessions reuse `SessionTypeDirectory` — no new session type.
- A new `bool one_off = 14` field is added to `CreateSessionRequest` proto. The backend generates the path when `one_off=true`; the `path` field in the request is ignored.
- Directory creation uses `os.Mkdir` (atomic on Unix) to handle simultaneous creation races.
- The generated path is stored in `instance.Path` and surfaces in the session detail view with no additional backend work.

---

## Epic 1: Backend — Name Generation Package

**Goal**: A standalone, testable package that generates unique, human-readable directory names and creates the leaf directory atomically.

---

### Task 1.1 — Create `session/namegen/namegen.go`

**File**: `session/namegen/namegen.go` (new file)

**Change description**:

Create a new Go package `namegen` at `session/namegen/namegen.go`.

The package must export exactly two functions:

```go
// Generate returns a name string in YYYYMMDD-adjective-noun-NN format.
// It does NOT create a directory — it only generates the name string.
func Generate() string

// GenerateAndCreate creates a unique subdirectory inside baseDir and returns
// the full absolute path. It uses os.Mkdir (atomic) for the leaf to handle
// concurrent creation races. It retries up to maxAttempts times.
// On success, the directory exists on disk and is owned by this call.
// Returns error if maxAttempts exhausted or baseDir cannot be written.
func GenerateAndCreate(baseDir string, maxAttempts int) (string, error)
```

Implementation details for `Generate()`:
- Use `time.Now().Format("20060102")` for the date prefix (local time).
- Select one adjective from `adjectives` and one noun from `nouns` using `math/rand.Intn(len(list))`.
- Number is `rand.Intn(100)` formatted as `%02d`.
- Return `fmt.Sprintf("%s-%s-%s-%02d", date, adj, noun, num)`.

Implementation details for `GenerateAndCreate(baseDir, maxAttempts)`:
- Call `os.MkdirAll(baseDir, 0755)` first. If it fails, return `fmt.Errorf("cannot create one_off_base_dir %q: %w", baseDir, err)`.
- After MkdirAll, call `os.Stat(baseDir)` to confirm it is a directory (guard against baseDir being a file). If not a directory, return `fmt.Errorf("one_off_base_dir %q exists but is not a directory", baseDir)`.
- Loop up to `maxAttempts` times:
  - Call `name := Generate()`
  - `fullPath := filepath.Join(baseDir, name)`
  - `err := os.Mkdir(fullPath, 0755)` — if `err == nil`, return `fullPath, nil` (success; this goroutine owns the directory).
  - If `err != nil`, continue to next attempt (handles both EEXIST and transient errors).
- If all attempts fail, return `"", fmt.Errorf("failed to generate unique one-off directory after %d attempts", maxAttempts)`.

Word lists (embed directly in the file — these are the authoritative lists, do not shrink them):

```go
var adjectives = []string{
    "agile", "amber", "ancient", "azure", "bold", "brave", "bright", "calm",
    "clever", "coastal", "cosmic", "crisp", "daring", "dawn", "deep", "distant",
    "eager", "early", "elegant", "emerald", "fierce", "fleet", "flowing", "frosty",
    "gentle", "gilded", "glowing", "golden", "grand", "green", "hidden", "hollow",
    "humble", "icy", "jade", "jolly", "keen", "kind", "lively", "lunar",
    "mellow", "misty", "mossy", "mystic", "noble", "northern", "ocean", "open",
    "patient", "peaceful", "polar", "proud", "quiet", "rapid", "rising", "rocky",
    "royal", "rustic", "serene", "sharp", "silver", "sleek", "solar", "solid",
    "steady", "stellar", "still", "sunny", "swift", "tidal", "twilight", "vast",
    "vibrant", "vivid", "warm", "western", "wild", "windy", "wise", "witty",
}  // 80 adjectives — max length: "twilight" (8 chars)

var nouns = []string{
    "albatross", "badger", "bear", "beaver", "bison", "buck", "buffalo", "cardinal",
    "cedar", "cliff", "cloud", "condor", "coral", "crane", "creek", "crow",
    "dune", "eagle", "elm", "falcon", "fern", "finch", "fjord", "fox",
    "glacier", "glen", "grove", "gull", "harbor", "hawk", "heath", "heron",
    "hill", "ibis", "jay", "juniper", "kelp", "kite", "lagoon", "lark",
    "loon", "lynx", "maple", "marsh", "meadow", "mesa", "mink", "moose",
    "moss", "oak", "osprey", "otter", "owl", "peak", "pine", "plover",
    "pond", "raven", "reef", "ridge", "robin", "rock", "salmon", "sedge",
    "shore", "sparrow", "spruce", "starling", "stone", "storm", "swallow", "swan",
    "swift", "teal", "tern", "thistle", "thrush", "tide", "trail", "vale",
}  // 80 nouns — max length: "albatross" (9 chars)
```

Name length validation: max generated name = 8 (date) + 1 + 8 (max adj "twilight") + 1 + 9 (max noun "albatross") + 1 + 2 = 30 chars. All names ≤ 32 chars, URL-safe, shell-safe (lowercase letters + hyphens + digits only).

Imports needed: `"fmt"`, `"math/rand"`, `"os"`, `"path/filepath"`, `"time"`.

**Acceptance test**:
- Build passes: `go build ./session/namegen/...`
- No new Go module dependencies (zero new imports outside stdlib + existing `go.mod`)

---

### Task 1.2 — Unit tests in `session/namegen/namegen_test.go`

**File**: `session/namegen/namegen_test.go` (new file)

**Change description**:

Write tests in package `namegen_test` (external test package). Use only stdlib `testing`, `regexp`, and `os`/`path/filepath`.

Test cases:

1. `TestGenerate_Format` — call `Generate()` 1000 times, assert each result matches `^\d{8}-[a-z]+-[a-z]+-\d{2}$` and has length ≤ 30.

2. `TestGenerate_ShellSafe` — call `Generate()` 100 times, assert each result matches `^[a-z0-9-]+$` (no uppercase, no special chars).

3. `TestGenerate_DatePrefix` — call `Generate()`, parse first 8 chars as a date (`time.Parse("20060102", ...)`), assert it succeeds (valid date).

4. `TestGenerate_NumberRange` — call `Generate()` 1000 times, parse the last two chars as an integer, assert 0 ≤ n ≤ 99.

5. `TestGenerateAndCreate_CreatesDir` — call `GenerateAndCreate(t.TempDir(), 10)`, assert returned path exists on disk (`os.Stat` succeeds), assert it is a directory.

6. `TestGenerateAndCreate_BaseDir_Created` — call `GenerateAndCreate(filepath.Join(t.TempDir(), "nested/subdir"), 10)`, assert no error (MkdirAll creates nested dirs), assert returned path is a directory.

7. `TestGenerateAndCreate_BaseDir_IsFile_Error` — create a regular file at tmpDir/notadir, call `GenerateAndCreate(tmpDir+"/notadir", 10)`, assert error is non-nil and message contains "not a directory".

8. `TestGenerateAndCreate_RetryOnCollision` — pre-create a directory whose name would be the first generated name, assert `GenerateAndCreate` still succeeds on retry (simulated by calling it twice and asserting both paths are distinct).

**Acceptance test**:
- `go test ./session/namegen/...` passes with all 8 tests green.

---

## Epic 2: Backend — Config Field

**Goal**: Expose `one_off_base_dir` as a configurable field with a sensible default and tilde expansion.

---

### Task 2.1 — Add `OneOffBaseDir` to `config.Config`

**File**: `config/config.go`

**Change description**:

1. In the `Config` struct (line 181), add the new field after `Notifications NotificationPrefs`:
   ```go
   // OneOffBaseDir is the base directory where one-off session directories are created.
   // Default: "~/oneoff". Tilde is expanded at runtime. Created automatically on first use.
   OneOffBaseDir string `json:"one_off_base_dir,omitempty"`
   ```

2. In the same file, add a new exported method on `*Config` (place after the `DefaultConfig` function block, around line 290):
   ```go
   // OneOffBaseDirOrDefault returns the resolved one-off base directory.
   // If OneOffBaseDir is empty, it returns "~/oneoff" with ~ expanded to the
   // current user's home directory. The directory is NOT created here — call
   // namegen.GenerateAndCreate to create it on first use.
   func (c *Config) OneOffBaseDirOrDefault() (string, error) {
       dir := c.OneOffBaseDir
       if dir == "" {
           dir = "~/oneoff"
       }
       if strings.HasPrefix(dir, "~/") {
           home, err := os.UserHomeDir()
           if err != nil {
               return "", fmt.Errorf("cannot expand home dir: %w", err)
           }
           dir = filepath.Join(home, dir[2:])
       } else if dir == "~" {
           home, err := os.UserHomeDir()
           if err != nil {
               return "", fmt.Errorf("cannot expand home dir: %w", err)
           }
           dir = home
       }
       return dir, nil
   }
   ```
   Required imports already present in `config.go`: `"fmt"`, `"os"`, `"path/filepath"`, `"strings"`. Verify each is present; add any missing.

**Acceptance test**:
- `go build ./config/...` passes.
- A temporary test (or manual check): `cfg := &Config{}; dir, err := cfg.OneOffBaseDirOrDefault()` returns a path ending in `/oneoff` with no error.
- An existing config JSON file without `one_off_base_dir` loads successfully (`OneOffBaseDir == ""`).

---

## Epic 3: Backend — Proto + RPC

**Goal**: Wire the `one_off` flag through the proto schema into the `CreateSession` handler, which generates the directory and overrides the path.

**Risk**: Proto field number 14 must be verified against the current proto file before use. The current highest field number in `CreateSessionRequest` is 13 (`session_type`). Confirm this before regenerating.

---

### Task 3.1 — Add `one_off` field to proto

**File**: `proto/session/v1/session.proto`

**Change description**:

In the `CreateSessionRequest` message (currently ends at field 13), add after `session_type`:

```proto
  // Optional: If true, ignore the path field and generate a fresh directory
  // under one_off_base_dir. The session title is still required.
  bool one_off = 14;
```

No other changes to the proto file.

**Acceptance test**:
- `buf lint` passes (or `protoc` compiles without error).

---

### Task 3.2 — Regenerate proto

**Command**: Run `make generate-proto` from the repository root.

This regenerates:
- Go bindings: `gen/session/v1/session.pb.go`, `gen/session/v1/sessionv1connect/session.connect.go`
- TypeScript bindings: `web-app/src/gen/session/v1/session_pb.ts` (and related files)

**Acceptance test**:
- `go build ./...` passes after regeneration.
- `web-app/src/gen/session/v1/session_pb.ts` contains `oneOff` (camelCase) in the `CreateSessionRequest` class definition.

---

### Task 3.3 — Handle `one_off=true` in `CreateSession` RPC handler

**File**: `server/services/session_service.go`

**Change description**:

Three surgical edits to `CreateSession` (lines 500–668):

**Edit A — Relax path validation for one-off requests.**

Change the path-required guard (line 509–511) from:
```go
if req.Msg.Path == "" {
    return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
}
```
to:
```go
if !req.Msg.OneOff && req.Msg.Path == "" {
    return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
}
```

**Edit B — Generate and create the one-off directory, then set `resolvedPath`.**

Insert the following block immediately after the GitHub URL resolution block (after line ~548, before the session defaults block). Place it as an early `if` branch before the `resolvedPath` variable is used further:

```go
// One-off session: generate a fresh directory and override resolvedPath.
if req.Msg.OneOff {
    cfg := config.LoadConfig()
    baseDir, err := cfg.OneOffBaseDirOrDefault()
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve one_off_base_dir: %w", err))
    }
    generatedPath, err := namegen.GenerateAndCreate(baseDir, 10)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create one-off directory: %w", err))
    }
    resolvedPath = generatedPath
    // Force session type to directory — one-off sessions never use worktrees.
    // Override the sessionType variable that will be computed below.
    // We achieve this by setting req.Msg.SessionType to DIRECTORY before the
    // sessionType determination block runs. Since req.Msg is a pointer to the
    // deserialized proto message (not the wire bytes), this mutation is safe
    // and local to this request handler.
    // Alternatively (cleaner): set a local flag and skip the inference block.
}
```

Then, in the sessionType determination block (line 570–592), add an additional guard so that if `req.Msg.OneOff`, `sessionType` is always `session.SessionTypeDirectory` regardless of other fields:

```go
// Force directory type for one-off sessions (overrides explicit session_type).
if req.Msg.OneOff {
    sessionType = session.SessionTypeDirectory
}
```

Place this immediately after the existing sessionType determination block closes.

**Edit C — Add import for `namegen` package.**

At the top of `server/services/session_service.go`, in the import block, add:
```go
"github.com/TylerStaplerAtFanatics/stapler-squad/session/namegen"
```
(Adjust the module path to match the value in `go.mod`'s `module` directive — verify it before editing.)

**Acceptance test**:
- `go build ./server/...` passes.
- Integration test (manual): Open the web UI, create a one-off session with title "test-oneoff". Verify:
  1. No error on creation.
  2. Session appears in the session list.
  3. Session detail view shows a `Path` value like `~/oneoff/20260424-brave-falcon-07`.
  4. That directory exists on disk.
  5. Claude starts in that directory (terminal shows `~/oneoff/20260424-...` as the prompt directory).

---

## Epic 4: Frontend — UI Changes

**Goal**: Add "One-off" as a selectable session type in the Omnibar creation panel. When selected, hide the path input, show only the session title field, and submit `one_off: true` to the backend.

---

### Task 4.1 — Extend `OmnibarFormState` and `OmnibarSessionData`

**File**: `web-app/src/components/sessions/Omnibar.tsx`

**Change description**:

Three changes in this file:

**Change A — Add `"one_off"` to the `sessionType` union in `OmnibarFormState`.**

```typescript
// Before:
sessionType: "directory" | "new_worktree" | "existing_worktree";

// After:
sessionType: "directory" | "new_worktree" | "existing_worktree" | "one_off";
```

**Change B — Add `oneOff?: boolean` to `OmnibarSessionData`.**

```typescript
export interface OmnibarSessionData {
  // ... existing fields ...
  oneOff?: boolean;  // add this field
}
```

**Change C — Update `canSubmit` to allow submission without path input when `sessionType === "one_off"`.**

The current `canSubmit` (line 551–566) requires `input.trim()` and a valid `detection`. Add an early-return path for one-off mode:

```typescript
const canSubmit = useMemo(() => {
  // One-off mode: only session name is required (no path needed).
  if (sessionType === "one_off") {
    return !!sessionName.trim();
  }
  // ... existing checks unchanged ...
  if (!input.trim()) return false;
  if (!sessionName.trim()) return false;
  if (!detection || detection.type === InputType.Unknown || detection.type === InputType.SessionSearch) return false;
  if (sessionType === "new_worktree") {
    if (!useTitleAsBranch && !branch.trim()) return false;
  } else if (sessionType === "existing_worktree") {
    if (!existingWorktree.trim()) return false;
  }
  return true;
}, [input, sessionName, detection, sessionType, branch, useTitleAsBranch, existingWorktree]);
```

**Change D — Update `handleSubmit` to set `oneOff: true` and skip path when `sessionType === "one_off"`.**

In `handleSubmit` (line 569–635), modify the `sessionData` construction:

```typescript
const sessionData: OmnibarSessionData = {
  title: sessionName.trim(),
  path: sessionType === "one_off" ? "" : (detection?.localPath || ""),
  branch: sessionType === "one_off" ? undefined : (finalBranch || undefined),
  program,
  category: category.trim() || undefined,
  autoYes,
  sessionType: sessionType === "one_off" ? "directory" : sessionType,
  existingWorktree: sessionType === "one_off" ? undefined : (existingWorktree.trim() || undefined),
  workingDir: sessionType === "one_off" ? undefined : (workingDir.trim() || undefined),
  oneOff: sessionType === "one_off" ? true : undefined,
};
```

Also, skip the `saveHistory` call for one-off sessions (there is no meaningful path to save):
```typescript
if (isPathInput && detection?.localPath && sessionType !== "one_off") {
  saveHistory(detection.localPath);
}
```

**Change E — Update `handleSubmit` dependency array** to include `sessionType` (it is already present, but verify `oneOff` handling doesn't add new deps).

**Acceptance test**:
- TypeScript compiles: `cd web-app && npx tsc --noEmit` passes.
- In the Omnibar with `sessionType === "one_off"` and `sessionName = "my session"`, `canSubmit` is `true` even with an empty `input` field.

---

### Task 4.2 — Add "One-off" radio button in `OmnibarCreationPanel`

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

**Change description**:

**Change A — Add `"one_off"` to `SESSION_TYPES` array.**

```typescript
const SESSION_TYPES = [
  { value: "new_worktree", label: "New Worktree" },
  { value: "directory",    label: "Directory" },
  { value: "existing_worktree", label: "Use Worktree" },
  { value: "one_off",      label: "One-off" },
] as const;
```

The `SessionTypeValue` type is derived from `as const` so it will automatically include `"one_off"` — no separate type change needed.

**Change B — Add hint text for `"one_off"` in the hint span.**

```typescript
<span className={hint}>
  {sessionType === "new_worktree" && "Creates an isolated git worktree for this session"}
  {sessionType === "existing_worktree" && "Uses an existing worktree at a specific path"}
  {sessionType === "directory" && "Works directly in the repository without worktree isolation"}
  {sessionType === "one_off" && "A fresh directory will be created automatically — no path needed"}
</span>
```

**Change C — Hide Working Directory field when `sessionType === "one_off"`.**

Wrap the Working Directory field (lines 225–239) in a conditional:

```tsx
{sessionType !== "one_off" && (
  <div className={field}>
    <label className={labelClass} htmlFor="omnibar-working-dir">
      Working Directory
    </label>
    <input ... />
    <span className={hint}>Optional: Start in a subdirectory (relative path)</span>
  </div>
)}
```

**Change D — Show informational banner when `sessionType === "one_off"`.**

Inside the `<div className={body}>` block, add after the Session Type radio group section and before branch controls:

```tsx
{sessionType === "one_off" && (
  <div className={hint} style={{ marginTop: 0 }}>
    Directory will be created at <code>~/oneoff/YYYYMMDD-word-word-NN</code>
  </div>
)}
```

(Use existing `hint` CSS class from `Omnibar.css`. The `<code>` element is fine inside a `<span>`/`<div>` with `hint` styling.)

**Change E — Update `OmnibarCreationPanelProps` prop type for `sessionType`** is not needed because `formState.sessionType` comes from `OmnibarFormState` which is already updated in Task 4.1. The panel destructures `sessionType` from `formState`, so type propagates automatically.

**Acceptance test**:
- `cd web-app && npx tsc --noEmit` passes.
- In the running UI (`make restart-web`), the Omnibar creation panel shows 4 radio buttons: "New Worktree", "Directory", "Use Worktree", "One-off".
- Selecting "One-off" hides the Working Directory field and shows the informational hint.
- The "Create Session" button is enabled as soon as a session name is typed (even with empty path input).

---

### Task 4.3 — Wire `oneOff` through `OmnibarContext` to the RPC call

**File**: `web-app/src/lib/contexts/OmnibarContext.tsx`

**Change description**:

In `handleCreateSession` (line 94–117), pass `oneOff` through to `createSession`. Also update `sessionTypeMap` to gracefully handle `"one_off"` (map it to `SessionType.DIRECTORY`):

**Change A — Update `sessionTypeMap`.**

```typescript
const sessionTypeMap: Record<string, SessionType> = {
  directory:          SessionType.DIRECTORY,
  new_worktree:       SessionType.NEW_WORKTREE,
  existing_worktree:  SessionType.EXISTING_WORKTREE,
  one_off:            SessionType.DIRECTORY,  // one-off is a directory session; type overridden server-side
};
```

**Change B — Pass `oneOff` to `createSession`.**

```typescript
const handleCreateSession = useCallback(
  async (data: OmnibarSessionData) => {
    const session = await createSession({
      title: data.title,
      path: data.path,
      branch: data.branch,
      program: data.program,
      category: data.category,
      prompt: data.prompt,
      autoYes: data.autoYes,
      workingDir: data.workingDir,
      existingWorktree: data.existingWorktree,
      sessionType: data.sessionType ? sessionTypeMap[data.sessionType] : undefined,
      oneOff: data.oneOff ?? false,   // new field
    });
    // ... rest unchanged ...
  },
  [createSession, router]
);
```

**File**: `web-app/src/lib/hooks/useSessionService.ts`

**Change description**:

In `createSession` (line 141–165), pass `oneOff` to the RPC call:

```typescript
const response = await clientRef.current.createSession({
  title: request.title ?? "",
  path: request.path ?? "",
  workingDir: request.workingDir,
  branch: request.branch,
  program: request.program,
  category: request.category,
  prompt: request.prompt,
  autoYes: request.autoYes,
  existingWorktree: request.existingWorktree,
  sessionType: request.sessionType,
  oneOff: request.oneOff ?? false,   // new field
});
```

**Acceptance test**:
- `cd web-app && npx tsc --noEmit` passes.
- Network inspection (browser DevTools) shows `oneOff: true` in the `CreateSession` request payload when creating a one-off session.

---

### Task 4.4 — Hide main path input when Omnibar is in one-off mode

**File**: `web-app/src/components/sessions/Omnibar.tsx`

**Context**: The main `<input>` in the Omnibar currently serves as the path input. When `sessionType === "one_off"`, this input is meaningless (and confusing), but it is also used by the detection debounce to drive mode state. We need to suppress path input behavior without breaking discovery mode.

**Change description**:

**Change A — Override the placeholder text when `sessionType === "one_off"`.**

In the main `<input>` element, update the `placeholder` prop:

```tsx
placeholder={
  sessionType === "one_off"
    ? "Session title is the only thing needed…"
    : isDiscoveryMode
    ? "Jump to session or search repos..."
    : "Enter path, GitHub URL, or owner/repo..."
}
```

**Change B — Visually indicate one-off mode in the type indicator.**

Before the main `<input>`, the `typeIndicator` span shows an icon from `typeInfo.icon`. Add a conditional override when `sessionType === "one_off"`:

```tsx
<span className={typeIndicator} aria-hidden="true">
  {sessionType === "one_off" ? "⚡" : typeInfo.icon}
</span>
```

**Change C — Suppress the path existence indicator for one-off mode.**

The `pathIndicator` currently renders when `isPathInput && !isDiscoveryMode && input.trim()`. Add `&& sessionType !== "one_off"`:

```tsx
{isPathInput && !isDiscoveryMode && input.trim() && sessionType !== "one_off" && (
  <span className={pathIndicator} ...>
    ...
  </span>
)}
```

**Change D — Suppress the path completion dropdown for one-off mode.**

```tsx
{!isDiscoveryMode && isDropdownVisible && sessionType !== "one_off" && (
  <PathCompletionDropdown ... />
)}
```

**Change E — Suppress the detection badge for one-off mode.**

The detection badge renders when `input.trim() && !isDiscoveryMode`. Add `&& sessionType !== "one_off"`:

```tsx
{input.trim() && !isDiscoveryMode && sessionType !== "one_off" && (
  <div className={detectionInfo}>
    ...
  </div>
)}
```

**Acceptance test**:
- `make restart-web` builds successfully.
- In the UI with "One-off" selected in the radio group:
  - The main input shows the override placeholder text.
  - No path-completion dropdown appears when typing in the main input.
  - No path existence indicator (✓/✗) appears.
  - No detection badge appears.
  - The session creation panel is still visible (it's shown based on `!isDiscoveryMode`).

---

## Build and Verification Sequence

Run these in order after all tasks are complete:

1. `go build ./session/namegen/...` — namegen package compiles
2. `go test ./session/namegen/...` — all 8 namegen tests pass
3. `go build ./config/...` — config package compiles
4. `make generate-proto` — regenerates Go + TS bindings from updated proto
5. `go build ./...` — entire Go codebase compiles
6. `make lint` — no lint errors (required for `make build` to pass)
7. `cd web-app && npx tsc --noEmit` — TypeScript typechecks
8. `cd web-app && npx jest --no-coverage` — frontend tests pass
9. `make restart-web` — UI builds and server restarts

---

## Key File Index

| File | Epic | Change |
|---|---|---|
| `session/namegen/namegen.go` | 1 | New file — name generation + atomic directory creation |
| `session/namegen/namegen_test.go` | 1 | New file — 8 unit tests |
| `config/config.go` | 2 | Add `OneOffBaseDir` field + `OneOffBaseDirOrDefault()` method |
| `proto/session/v1/session.proto` | 3 | Add `bool one_off = 14` to `CreateSessionRequest` |
| `server/services/session_service.go` | 3 | Relax path validation + generate dir + force directory type |
| `web-app/src/components/sessions/Omnibar.tsx` | 4 | Add `"one_off"` to sessionType union; update `canSubmit`, `handleSubmit`, UI |
| `web-app/src/components/sessions/OmnibarCreationPanel.tsx` | 4 | Add "One-off" radio; hide working-dir; add hint banner |
| `web-app/src/lib/contexts/OmnibarContext.tsx` | 4 | Update `sessionTypeMap`; pass `oneOff` to `createSession` |
| `web-app/src/lib/hooks/useSessionService.ts` | 4 | Pass `oneOff` in RPC call body |

---

## Risk Register

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Proto field 14 already taken | Low | Medium | Verify current max field number in `CreateSessionRequest` before editing proto. Stack research confirms 13 is the max. |
| `make generate-proto` tool not installed | Low | Medium | Run `make install-tools` first if `buf` is missing |
| `go.mod` module path mismatch in import | Low | Low | Check `module` directive in `go.mod` before adding `namegen` import to `session_service.go` |
| One-off mode breaks discovery: typing in main input while `one_off` selected could trigger path detection and flip mode | Medium | Low | Detection runs but mode state stays in `creation` because `handleSubmit` checks `sessionType === "one_off"` first. The dropdown and badge are suppressed by Task 4.4. Not a correctness issue — cosmetically OK. |

---

## Out of Scope (Not in This Plan)

Per requirements:
- CLI `stapler-squad create --one-off` flag
- Git initialization inside the one-off directory
- Auto-cleanup / TTL expiry of old directories
- Configurable name format
- Settings UI for `one_off_base_dir` (config must be edited manually in JSON)
