# Backend DI Refactor — Implementation Plan

**Branch:** `stapler-squad-backend-refactor`
**Goals in priority order:** G1 Warren Wire Coverage, G2 instance.go split, G3 interface extraction, G4 test coverage

---

## Technology Choices Validated

### Same-Package File Splitting (G2)
Same-package file splitting is the correct approach. Go resolves all methods across all files within a package — `func (i *Instance) Foo()` declared in `instance_tmux.go` is indistinguishable from one declared in `instance.go`. Sub-package extraction is prohibited by the requirements (no moving packages) and would cause circular import risk: `session/instance.go` accesses private fields like `i.tmuxManager`, `i.gitManager`, `i.stateMutex`, and `i.controllerManager` that cannot cross package boundaries. The two files already split out (`instance_status.go` at 221 lines and `instance_workspace.go` at 564 lines) prove the pattern is working and safe.

### Warren Wire as the DI Safety Layer (G1)
Warren Wire is the right tool. The project already owns `pkg/warren` and uses `warren.Wire` in `main.go` for App phase wiring. Adding `warren.NewWire` + `warren.Set` calls inside `BuildServiceDeps` and `BuildRuntimeDeps` is a minimal-changeset, non-breaking addition that gives the exact error-at-startup semantics required. A third-party DI framework (Wire, Fx, Dig) would require constructor injection refactors — out of scope per requirements. Rolling a manual `if dep == nil { return err }` chain is weaker because it reports the first missing dep only; `warren.Wire.Validate()` reports all missing at once.

### Interface Segregation at the server/session Boundary (G3)
The session/server package boundary is the correct scope for interface extraction. Interfaces defined in `session/` (alongside the concrete types they abstract) enable the server layer to take narrow interface parameters without creating a new import cycle. Extracting interfaces inside `server/services/` would require every caller to import from that package, violating the rule that `session/` must not depend on `server/`. The existing `session.InstanceStore` (in `session/storage.go:159`) validates this pattern and is already used correctly in `server/mcp/` and `server/services/session_image_upload_handler.go`.

---

## Epic 1 — Warren Wire Coverage (G1)

**Goal:** Every `Set*` call in `BuildCoreDeps`, `BuildServiceDeps`, and `BuildRuntimeDeps` is wrapped in a `warren.Wire` validator that returns an error at startup for any nil or missing dependency.

**Risk:** Low. Warren's `Set` only adds a thin wrapper around the existing setter call and a nil check. No behavior changes.

### Story 1.1 — Add import and wire `BuildCoreDeps` (Phase 1)

Phase 1 has a single setter call. It is currently unwrapped.

**Tasks:**

1.1.1 Add `"github.com/tstapler/stapler-squad/pkg/warren"` to the imports in `server/dependencies.go`.

1.1.2 In `BuildCoreDepsWithOptions`, after `services.NewErrorRegistry(...)` constructs `errorRegistry` and before the `return &CoreDeps{...}` statement, add:

```go
w := warren.NewWire("CoreDeps")
warren.Set(w, "ErrorRegistry", sessionService.SetErrorRegistry, errorRegistry)
if err := w.Validate(); err != nil {
    return nil, err
}
```

Note: `errorRegistry` is always non-nil when `services.NewErrorRegistry` succeeds (it returns a valid struct even with a nil ent client). This wire call serves as documentation that the setter is intentional and not accidental; it also catches future regressions if the setter is deleted.

### Story 1.2 — Wire `BuildServiceDeps` (Phase 2)

Phase 2 has 3 setter calls across two receivers. All three values are freshly constructed and non-nil if construction succeeds.

**Tasks:**

1.2.1 In `BuildServiceDeps`, after the three setter calls (`reviewQueuePoller.SetApprovalProvider`, `core.SessionService.SetStatusManager`, `core.SessionService.SetReviewQueuePoller`) and before `return &ServiceDeps{...}`, add:

```go
w := warren.NewWire("ServiceDeps")
warren.Set(w, "ApprovalProvider",     reviewQueuePoller.SetApprovalProvider,     core.ApprovalStore)
warren.Set(w, "StatusManager",        core.SessionService.SetStatusManager,       statusManager)
warren.Set(w, "ReviewQueuePoller",    core.SessionService.SetReviewQueuePoller,   reviewQueuePoller)
if err := w.Validate(); err != nil {
    return nil, err
}
```

Note: The setter calls above both register the call with Warren AND call the setter. Remove the standalone setter calls that precede them (lines 378, 381, 382 in the current file) to avoid double-calling.

1.2.2 Confirm `core.ApprovalStore` is of type `*services.ApprovalStore` (a pointer, comparable). `warren.Set` will flag nil correctly.

### Story 1.3 — Wire `BuildRuntimeDeps` (Phase 3, synchronous setters only)

Phase 3 has 10 synchronous setter calls at the top level and 2 goroutine-internal setter calls. Only the synchronous setters can be tracked by Warren.

**Important pitfall:** Lines 562–563 (inside the `go func()` goroutine) call `instance.SetReviewQueue(reviewQueue)` and `instance.SetStatusManager(statusManager)` for newly discovered external sessions. These are intentionally per-instance wires that happen asynchronously after `BuildRuntimeDeps` returns. Warren cannot track these — they must remain unwrapped. Adding them to a Wire object whose `Validate()` is called before the goroutine runs would incorrectly flag them as missing.

**Tasks:**

1.3.1 In `BuildRuntimeDeps`, add a Warren wire block after all synchronous setters are complete and before the `return &RuntimeDeps{...}` statement:

```go
w := warren.NewWire("RuntimeDeps.SessionService")
warren.Set(w, "ReactiveQueueManager",  sessionService.SetReactiveQueueManager, reactiveQueueMgr)
warren.Set(w, "HistoryLinker",         sessionService.SetHistoryLinker,        historyLinker)
warren.Set(w, "ScrollbackManager",     sessionService.SetScrollbackManager,    scrollbackManager)
warren.Set(w, "ExternalDiscovery",     sessionService.SetExternalDiscovery,    externalDiscovery)
if err := w.Validate(); err != nil {
    return nil, err
}
```

1.3.2 Add a second wire object for the non-SessionService setters:

```go
w2 := warren.NewWire("RuntimeDeps.Pollers")
warren.Set(w2, "ReviewQueuePoller.Instances",  reviewQueuePoller.SetInstances,          instances)
warren.Set(w2, "PRStatusPoller.Instances",     svc.PRStatusPoller.SetInstances,         instances)
warren.Set(w2, "PRStatusPoller.OnUpdated",     svc.PRStatusPoller.SetOnUpdated,         onUpdatedFn)
warren.Set(w2, "HistoryLinker.Instances",      historyLinker.SetInstances,              instances)
if err := w2.Validate(); err != nil {
    return nil, err
}
```

Note on `PRStatusPoller.SetOnUpdated`: the callback is an inline `func` literal — it will never be nil. Use `warren.SetAlways` instead of `warren.Set` for this entry, since function values are not comparable in Go:

```go
warren.SetAlways(w2, "PRStatusPoller.OnUpdated", svc.PRStatusPoller.SetOnUpdated, func(inst *session.Instance) {
    eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_priority", "github_pr_state"}))
})
```

1.3.3 Handle the optional `UnfinishedWorkService`. `unfinishedWorkSvc` is legitimately nil when `config.GetConfigDir()` fails (no config directory available). Do not add it to the mandatory Wire. Instead, document the optionality explicitly:

```go
// UnfinishedWorkService is optional — nil when config directory is unavailable.
// Do not add to Warren Wire; nil is a valid production value documented on RuntimeDeps.
```

1.3.4 Handle the per-instance loop setters (lines 450–451: `inst.SetReviewQueue` and `inst.SetStatusManager` in the `for _, inst := range instances` loop). These are called in a synchronous loop before the goroutine, so they could technically be tracked. However, Warren is designed for named scalar setters, not loop iterations over a slice. Leave these as-is; the nil guards on `reviewQueue` and `statusManager` at this point in the function are sufficient (both are non-nil after Phase 2 succeeds).

**Full setter inventory — what is wrapped vs left unwrapped:**

| Setter | Location | Warren treatment |
|---|---|---|
| `sessionService.SetErrorRegistry(errorRegistry)` | Phase 1, sync | `warren.Set` in Story 1.1 |
| `reviewQueuePoller.SetApprovalProvider(core.ApprovalStore)` | Phase 2, sync | `warren.Set` in Story 1.2 |
| `core.SessionService.SetStatusManager(statusManager)` | Phase 2, sync | `warren.Set` in Story 1.2 |
| `core.SessionService.SetReviewQueuePoller(reviewQueuePoller)` | Phase 2, sync | `warren.Set` in Story 1.2 |
| `inst.SetReviewQueue(reviewQueue)` | Phase 3, sync loop | Left unwrapped — loop iteration |
| `inst.SetStatusManager(statusManager)` | Phase 3, sync loop | Left unwrapped — loop iteration |
| `reviewQueuePoller.SetInstances(instances)` | Phase 3, sync | `warren.Set` in Story 1.3 |
| `svc.PRStatusPoller.SetInstances(instances)` | Phase 3, sync | `warren.Set` in Story 1.3 |
| `svc.PRStatusPoller.SetOnUpdated(func(...))` | Phase 3, sync | `warren.SetAlways` in Story 1.3 |
| `sessionService.SetReactiveQueueManager(reactiveQueueMgr)` | Phase 3, sync | `warren.Set` in Story 1.3 |
| `historyLinker.SetInstances(instances)` | Phase 3, sync | `warren.Set` in Story 1.3 |
| `sessionService.SetHistoryLinker(historyLinker)` | Phase 3, sync | `warren.Set` in Story 1.3 |
| `sessionService.SetScrollbackManager(scrollbackManager)` | Phase 3, sync | `warren.Set` in Story 1.3 |
| `sessionService.SetExternalDiscovery(externalDiscovery)` | Phase 3, sync | `warren.Set` in Story 1.3 |
| `instance.SetReviewQueue(reviewQueue)` | Phase 3, goroutine | Left unwrapped — async goroutine |
| `instance.SetStatusManager(statusManager)` | Phase 3, goroutine | Left unwrapped — async goroutine |
| `UnfinishedWorkService` wiring (none currently) | Phase 3, conditional | Left unwrapped — legitimately optional |

### Story 1.4 — Verify build and behavior

**Tasks:**

1.4.1 Run `make build` to confirm the warren import resolves and no type mismatches exist.

1.4.2 Run `go test ./server/... -run TestBuild` to confirm existing tests pass.

1.4.3 Manually verify the error message format by temporarily removing one `warren.Set` call and confirming the app exits with a message matching: `"warren: ServiceDeps wiring incomplete — unapplied setters: StatusManager (value is nil/zero — dependency may not have been constructed)"`.

---

## Epic 2 — Split `session/instance.go` (G2)

**Goal:** Break the 3168-line monolith into domain-focused files within `package session`. No API changes. All existing tests pass unchanged.

**Risk:** Moderate. The mechanical risk is low (Go resolves methods across files), but merge conflicts and misplaced functions are real risks. Mitigate with incremental per-domain PRs or at least per-story commits.

**Pre-condition:** `instance_status.go` (221 lines, already exists) and `instance_workspace.go` (564 lines, already exists) are excluded from the split work — they are already done. The task covers what remains in `instance.go` (3168 lines).

**Important:** `instance_status.go` currently contains `InstanceStatusManager` and related types — not `Instance` status methods like `setStatus`, `transitionTo`, `GetCategoryPath`. Those are still in `instance.go` and should move to a new `instance_state.go`. Do not confuse the two.

### Story 2.1 — Extract `instance_serialization.go`

Contains serialization/deserialization functions that have no dependency on the tmux, git, or controller sub-managers.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) ToInstanceData()` | 282 | Long: ~100 lines of field mapping |
| `FromInstanceData(data InstanceData) (*Instance, error)` | 381 | ~230 lines including path migration |
| `InstanceData` type definition | ~430 | The struct type itself |
| `GitWorktreeData` type definition | near `InstanceData` | Part of serialization schema |
| `DiffStatsData` type definition | near `InstanceData` | Part of serialization schema |

**Imports needed in `instance_serialization.go`:**
- `"os/user"` (FromInstanceData tilde expansion)
- `"strings"`, `"path/filepath"`, `"time"`, `"fmt"`
- `"github.com/tstapler/stapler-squad/log"`
- `"github.com/google/uuid"`

**Tasks:**

2.1.1 Create `session/instance_serialization.go` with `package session` header.

2.1.2 Move `InstanceData`, `GitWorktreeData`, `DiffStatsData` struct definitions into the new file.

2.1.3 Move `ToInstanceData()` and `FromInstanceData()` into the new file.

2.1.4 Remove the moved code from `instance.go`. Keep `SessionType`, `InstanceOptions`, `InstanceType`, `ExternalInstanceMetadata`, and `InstancePermissions` type definitions in `instance.go` — they are used by the constructor and lifecycle methods which stay there.

2.1.5 Run `go build ./session/...` and `go test ./session/...` to verify.

**Expected file size:** ~350 lines.

### Story 2.2 — Extract `instance_state.go` (status machine)

Contains the `Instance` status/state machine methods — distinct from `InstanceStatusManager` (which is already in `instance_status.go`).

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(s Status) String()` | 44 | Status constant String method |
| `(i *Instance) setStatus(status Status)` | 912 | Private setter |
| `(i *Instance) transitionTo(s Status) error` | 918 | State machine enforcement |
| `(i *Instance) GetCategoryPath() []string` | 928 | Status/category grouping |
| `(i *Instance) MarkViewed()` | 826 | Review state timestamp |
| `(i *Instance) MarkUserResponded() time.Time` | 834 | Review state timestamp |
| `(i *Instance) MarkAcknowledged()` | 842 | Review state timestamp |
| `(i *Instance) MarkNeedsApproval() error` | 850 | State transition |
| `(i *Instance) LastMeaningfulOutputTime() time.Time` | 857 | Review state read |
| `(i *Instance) SetLastMeaningfulOutput(t time.Time)` | 864 | Review state write |
| `(i *Instance) GetEffectiveStatus() Status` | 2647 | Effective status via controller |
| `(i *Instance) GetStatus() int` | 2661 | SessionAccessor interface |
| `(i *Instance) Approve() error` | 2975 | NeedsApproval → Running |
| `(i *Instance) Deny() error` | 2987 | NeedsApproval → Paused |
| `(i *Instance) Paused() bool` | 1533 | Status predicate |
| `(i *Instance) Started() bool` | 1471 | Status predicate |
| `(i *Instance) RecoverFromStopped()` | 1511 | Startup reconciliation |
| `StatusFromDetected(...)` | (locate in file) | Maps DetectedStatus → Status |

Also move the `Status` iota constants block (lines 24–41) to this file, or to `instance.go` — leave in `instance.go` since the struct definition uses it.

**Imports needed in `instance_state.go`:**
- `"fmt"`, `"time"`
- `"github.com/tstapler/stapler-squad/log"`
- `"github.com/tstapler/stapler-squad/session/detection"`
- `"github.com/linkdata/deadlock"` (stateMutex type is defined on Instance struct in instance.go, accessible)

**Tasks:**

2.2.1 Create `session/instance_state.go` with `package session` header.

2.2.2 Move the listed methods into the new file.

2.2.3 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~200 lines.

### Story 2.3 — Extract `instance_tmux.go`

Contains tmux session creation, terminal I/O delegation, and PTY access. Already has a clear boundary: all methods delegate to `i.tmuxManager`.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) GetTmuxSessionName() string` | 906 | Simple getter |
| `(i *Instance) initTmuxSession()` | 1156 | Creates tmux.TmuxSession |
| `(i *Instance) buildLaunchCommand(claudeSessionID string) string` | 1138 | Command builder |
| `(i *Instance) KillSession() error` | 1316 | Kills tmux only |
| `(i *Instance) KillSessionKeepWorktree() error` | 1336 | Alias |
| `(i *Instance) KillExternalSession() error` | 1341 | External mux session kill |
| `(i *Instance) TmuxSessionExists() bool` | 1505 | Existence check |
| `(i *Instance) TmuxAlive() bool` | 1537 | Alive check |
| `(i *Instance) trackRestartRate()` | 1475 | Restart storm detection |
| `(i *Instance) HasUpdated() (bool, bool)` | 1411 | Terminal update check |
| `(i *Instance) TapEnter()` | 1433 | AutoYes helper |
| `(i *Instance) Attach() (chan struct{}, error)` | 1443 | Attach to tmux |
| `(i *Instance) SetPreviewSize(width, height int) error` | 1450 | Detached size |
| `(i *Instance) GetPTYReader() (*os.File, error)` | 1872 | PTY access |
| `(i *Instance) WriteToPTY(data []byte) (int, error)` | 1884 | PTY write |
| `(i *Instance) ResizePTY(cols, rows int) error` | 1896 | PTY resize |
| `(i *Instance) CapturePaneContent() (string, error)` | 1912 | Capture visible content |
| `(i *Instance) CapturePaneContentRaw() (string, error)` | 1922 | Capture with ANSI codes |
| `(i *Instance) GetCurrentPaneContent(lines int) (string, error)` | 1937 | Viewport capture |
| `(i *Instance) GetPaneCursorPosition() (x, y int, err error)` | 1947 | Cursor position |
| `(i *Instance) GetPaneDimensions() (width, height int, err error)` | 1957 | Pane dimensions |
| `(i *Instance) GetScrollbackHistory(startLine, endLine string) (string, error)` | 1967 | Scrollback capture |
| `(i *Instance) GetTmuxSession() *tmux.TmuxSession` | 2075 | Session accessor |
| `(i *Instance) StartControlMode() error` | 2088 | Control mode stream |
| `(i *Instance) StopControlMode() error` | 2093 | Control mode stop |
| `(i *Instance) SubscribeControlModeUpdates() (string, <-chan []byte)` | 2098 | Subscribe |
| `(i *Instance) UnsubscribeControlModeUpdates(id string)` | 2104 | Unsubscribe |
| `(i *Instance) SetTmuxSession(session *tmux.TmuxSession)` | 2108 | Test helper |
| `(i *Instance) SetWindowSize(cols, rows int) error` | 2116 | Window resize |
| `(i *Instance) RefreshTmuxClient() error` | 2123 | Client refresh |
| `(i *Instance) SendKeys(keys string) error` | 2137 | Key send |
| `(i *Instance) SendInputViaControlMode(ctx context.Context, data []byte) error` | 2147 | Control mode input |
| `(i *Instance) SendPrompt(prompt string) error` | 2044 | Prompt send |
| `(i *Instance) GetPanePID() (int32, error)` | 2419 | PID retrieval |

**Imports needed in `instance_tmux.go`:**
- `"context"`, `"fmt"`, `"os"`, `"time"`
- `"github.com/tstapler/stapler-squad/log"`
- `"github.com/tstapler/stapler-squad/session/tmux"`

**Tasks:**

2.3.1 Create `session/instance_tmux.go` with `package session` header.

2.3.2 Move the listed methods into the new file.

2.3.3 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~380 lines.

### Story 2.4 — Extract `instance_worktree.go` additions

The file `instance_workspace.go` already exists (564 lines, covering `SwitchWorkspace` and related workspace switching). Story 2.4 moves the remaining git/worktree methods from `instance.go` into a new `instance_worktree.go` (or appends to `instance_workspace.go` if it stays under 400 lines).

Since `instance_workspace.go` is already 564 lines (over the 400-line target), create `instance_worktree.go` for the remaining git/worktree methods.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) setupFirstTimeWorktree() error` | 1184 | SessionType routing |
| `(i *Instance) resolveStartPath(basePath string) string` | 1233 | CWD resolution |
| `(i *Instance) GetEffectiveRootDir() string` | 1261 | Root dir accessor |
| `(i *Instance) Workspace() Workspace` | 1273 | Workspace value |
| `(i *Instance) CleanupWorktree() error` | 1326 | Worktree cleanup |
| `(i *Instance) GetGitWorktree() (*git.GitWorktree, error)` | 1458 | Worktree accessor |
| `(i *Instance) HasGitWorktree() bool` | 1466 | Worktree predicate |
| `(i *Instance) SetGitWorktree(worktree *git.GitWorktree)` | 2131 | Test helper |
| `(i *Instance) UpdateDiffStats() error` | 1981 | Git diff compute |
| `(i *Instance) GetDiffStats() *git.DiffStats` | 2037 | Diff stats accessor |
| `(i *Instance) GetWorkingDirectory() string` | 2314 | CWD accessor |
| `(i *Instance) DetectAndPopulateWorktreeInfo() error` | 3113 | Worktree detection |
| `(i *Instance) RepoName() (string, error)` | 812 | Repo name |

**Imports needed in `instance_worktree.go`:**
- `"fmt"`, `"os"`, `"path/filepath"`, `"strings"`
- `"github.com/tstapler/stapler-squad/log"`
- `"github.com/tstapler/stapler-squad/session/git"`

**Tasks:**

2.4.1 Create `session/instance_worktree.go` with `package session` header.

2.4.2 Move the listed methods into the new file.

2.4.3 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~350 lines.

### Story 2.5 — Extract `instance_approval.go`

Contains review queue integration and approval state methods.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) GetReviewQueue() *ReviewQueue` | 2604 | Queue accessor |
| `(i *Instance) SetReviewQueue(queue *ReviewQueue)` | 2610 | Queue setter |
| `(i *Instance) NeedsReview() bool` | 2614 | Queue membership |
| `(i *Instance) GetReviewItem() (*ReviewItem, bool)` | 2622 | Queue item |
| `(i *Instance) SetStatusManager(manager *InstanceStatusManager)` | 2634 | Manager setter |
| `(i *Instance) GetStatusManager() *InstanceStatusManager` | 2639 | Manager getter |
| `(i *Instance) UpdateTerminalTimestamps(content string, forceUpdate bool)` | 2928 | Coordinator method |
| `(i *Instance) GetTimeSinceLastMeaningfulOutput() time.Duration` | 2960 | ReviewState delegation |
| `(i *Instance) GetTimeSinceLastTerminalUpdate() time.Duration` | 2968 | ReviewState delegation |
| `(i *Instance) detectAndTrackPrompt(content string, statusInfo InstanceStatusInfo) bool` | 3164 | Private delegation |

**Imports needed in `instance_approval.go`:**
- `"time"`
- `"github.com/tstapler/stapler-squad/log"`

**Tasks:**

2.5.1 Create `session/instance_approval.go` with `package session` header.

2.5.2 Move the listed methods.

2.5.3 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~130 lines.

### Story 2.6 — Extract `instance_checkpoint.go`

Contains checkpoint creation, forking, and retrieval methods.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) CreateCheckpoint(label string, scrollbackSeq uint64) (*Checkpoint, error)` | 2480 | Checkpoint creation |
| `(i *Instance) ForkFromCheckpoint(checkpointID, newTitle string, configDir string) (*Instance, error)` | 2533 | Fork operation |
| `(i *Instance) GetCheckpoints() CheckpointList` | 2596 | Snapshot read |
| `Checkpoint` type definition | (locate) | If not already in separate file |
| `CheckpointList` type definition | (locate) | If not already in separate file |
| `newCheckpointID()` | (locate) | Helper |

**Note:** Check if `Checkpoint`, `CheckpointList`, and `newCheckpointID` are already defined in a separate file (e.g., `session/checkpoint.go`). Only move them if they are currently in `instance.go`.

**Imports needed in `instance_checkpoint.go`:**
- `"bufio"`, `"fmt"`, `"os"`, `"path/filepath"`, `"time"`
- `"github.com/tstapler/stapler-squad/log"`
- `"github.com/tstapler/stapler-squad/session/scrollback"`
- `"github.com/tstapler/stapler-squad/session/git"`

**Tasks:**

2.6.1 Check if `Checkpoint` type is already in a separate file: `ls session/checkpoint*.go`.

2.6.2 Create `session/instance_checkpoint.go` with `package session` header.

2.6.3 Move the listed methods (and types if needed) into the new file.

2.6.4 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~180 lines.

### Story 2.7 — Extract `instance_tags.go`

Tag management is already fully delegated to `TagManager`. The extraction is mechanical.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) ensureTagManager()` | 3004 | Private initializer |
| `(i *Instance) AddTag(tag string) error` | 3012 | Delegate to TagManager |
| `(i *Instance) RemoveTag(tag string)` | 3020 | Delegate to TagManager |
| `(i *Instance) HasTag(tag string) bool` | 3028 | Delegate to TagManager |
| `(i *Instance) GetTags() []string` | 3043 | Delegate to TagManager |
| `(i *Instance) SetTags(tags []string) error` | 3056 | Delegate to TagManager |

**Imports needed in `instance_tags.go`:**
- None beyond what `package session` already provides (TagManager is in `session/`)

**Tasks:**

2.7.1 Create `session/instance_tags.go` with `package session` header.

2.7.2 Move the listed methods.

2.7.3 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~70 lines.

### Story 2.8 — Extract `instance_terminal.go`

Contains terminal content preview methods and the GitHub metadata delegation methods. The GitHub methods are read-only views over Instance fields and are a natural pair with display/content methods.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) Preview() (string, error)` | 1381 | Terminal content read |
| `(i *Instance) PreviewFullHistory() (string, error)` | 2052 | Full history capture |
| `(i *Instance) CaptureCurrentState() error` | 2451 | Shutdown state capture |
| `(i *Instance) GetTitle() string` | 876 | Getter |
| `(i *Instance) GetStableID() string` | 883 | Getter |
| `(i *Instance) MatchesID(id string) bool` | 894 | ID matching |
| `(i *Instance) GetCreatedAt() time.Time` | 871 | Time getter |
| `(i *Instance) SetTitle(title string) error` | 1523 | Title setter |
| `(i *Instance) Rename(newTitle string) error` | 1704 | Rename operation |
| `(i *Instance) GitHub() GitHubMetadataView` | 3071 | GitHub metadata view |
| `(i *Instance) IsPRSession() bool` | 3084 | PR predicate |
| `(i *Instance) GetGitHubRepoFullName() string` | 3088 | Repo name |
| `(i *Instance) GetPRDisplayInfo() string` | 3092 | Display info |
| `(i *Instance) IsGitHubSession() bool` | 3096 | GitHub predicate |
| `(i *Instance) UpdatePRStatus(...)` | 3100 | PR status update |
| `(i *Instance) GetPermissions() InstancePermissions` | 2889 | Permissions |
| `(i *Instance) GetStatusIconForType() string` | 2901 | Status icon |
| `(i *Instance) combineErrors(errs []error) error` | 1366 | Private helper |

**Imports needed in `instance_terminal.go`:**
- `"fmt"`, `"time"`
- `"github.com/tstapler/stapler-squad/log"`

**Tasks:**

2.8.1 Create `session/instance_terminal.go` with `package session` header.

2.8.2 Move the listed methods.

2.8.3 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~280 lines.

### Story 2.9 — Extract `instance_claude.go`

Contains Claude session management: history file detection, UUID extraction, conversation re-attachment.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) handleClaudeSessionReattachment() error` | 2158 | Re-attach logic |
| `(i *Instance) createNewClaudeSession() error` | 2217 | New session creation |
| `(i *Instance) findAndAttachToProjectSession(sessionManager *ClaudeSessionManager) error` | 2261 | Project session finder |
| `(i *Instance) GetClaudeSession() *ClaudeSessionData` | 2323 | Session data accessor |
| `(i *Instance) SetClaudeSession(sessionData *ClaudeSessionData)` | 2328 | Session data setter |
| `(i *Instance) HasClaudeSession() bool` | 2332 | Predicate |
| `(i *Instance) ClearConversationState()` | 2340 | Clear UUID |
| `(i *Instance) tryExtractConversationUUID()` | 2349 | UUID extraction |
| `(i *Instance) GetConversationUUID() string` | 2410 | UUID getter |
| `(i *Instance) SetHistoryInfo(conversationUUID, historyFilePath string)` | 2427 | History info setter |

**Imports needed in `instance_claude.go`:**
- `"fmt"`, `"time"`
- `"github.com/tstapler/stapler-squad/log"`

**Tasks:**

2.9.1 Create `session/instance_claude.go` with `package session` header.

2.9.2 Move the listed methods.

2.9.3 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~250 lines.

### Story 2.10 — Extract `instance_controller.go`

Contains the `ClaudeController` lifecycle and `RateLimit` delegation methods.

**Functions to move from `instance.go`:**

| Function | Line (approx) | Notes |
|---|---|---|
| `(i *Instance) StartController() error` | 2665 | Controller lifecycle |
| `(i *Instance) StopController()` | 2764 | Controller lifecycle |
| `(i *Instance) GetController() *ClaudeController` | 2778 | Accessor |
| `(i *Instance) GetExitContent() []byte` | 2785 | Exit content |
| `(i *Instance) GetRateLimitState() int` | 2796 | Rate limit state |
| `(i *Instance) GetRateLimitResetTime() time.Time` | 2804 | Rate limit time |
| `(i *Instance) SetRateLimitEnabled(enabled bool)` | 2814 | Rate limit toggle |
| `(i *Instance) SetRateLimitCallbacks(onDetected, onRecovery func(...))` | 2830 | Callback wiring |
| `(i *Instance) wireRateLimitCallbacks(ctrl *ClaudeController)` | 2845 | Private wiring |
| `(i *Instance) IsRateLimitEnabled() bool` | 2882 | Predicate |
| `(i *Instance) RegisterLifecycleListener(l LifecycleListener)` | 2742 | Listener registration |
| `(i *Instance) fireLifecycleEvent(event LifecycleEvent, reason string)` | 2753 | Private fire |

**Imports needed in `instance_controller.go`:**
- `"context"`, `"fmt"`, `"time"`
- `"github.com/tstapler/stapler-squad/log"`
- `"github.com/tstapler/stapler-squad/session/detection/ratelimit"`

**Tasks:**

2.10.1 Create `session/instance_controller.go` with `package session` header.

2.10.2 Move the listed methods.

2.10.3 Run `go build ./session/...` and `go test ./session/...`.

**Expected file size:** ~220 lines.

### Story 2.11 — Clean up `instance.go` (residual)

After all extractions, `instance.go` should contain only:

- Package declaration and imports
- `Status`, `LifecycleEvent`, `LifecycleEvent constants` iota blocks (lines 24–83)
- `Instance` struct definition (lines 87–279)
- `InstanceOptions` struct definition
- `SessionType` constants and `IsValid()` method (~615–682)
- `InstanceType`, `ExternalInstanceMetadata`, `InstancePermissions` types
- `NewInstance(opts InstanceOptions) (*Instance, error)` constructor
- `NewInstanceWithCleanup(opts InstanceOptions) (*Instance, tmux.CleanupFunc, error)` constructor
- `Start(firstTimeSetup bool) error`
- `StartWithCleanup(firstTimeSetup bool) (tmux.CleanupFunc, error)`
- `start(...)` private method (the main startup logic, ~965–1137 lines)
- `Destroy() error`
- `Kill() error` (delegates to Destroy)
- `Pause() error`
- `Resume() error`
- `Restart(preserveOutput bool) error`

**Tasks:**

2.11.1 Review remaining `instance.go` line count. Target: under 600 lines.

2.11.2 Remove any duplicate imports created by the splits.

2.11.3 Run `make lint` to catch import cycle issues, unused imports, or duplicate declarations.

2.11.4 Run `go test ./session/...` for final verification.

---

## Epic 3 — Interface Extraction at the server/session Boundary (G3)

**Goal:** Extract narrow interfaces that allow server-layer services to depend on interfaces instead of concrete types where they only use a subset of the concrete type's API.

**Risk:** Moderate. Changing function signatures in `server/services/` may break call sites in `server/` that construct service structs. Verify each change compiles before moving to the next.

### Story 3.1 — Confirm `session.InstanceStore` is used consistently

`session.InstanceStore` is already defined in `session/storage.go:159`. A compile-time assertion (`var _ InstanceStore = (*Storage)(nil)`) exists. Several services still accept `*session.Storage` directly.

**Tasks:**

3.1.1 Audit every service constructor in `server/services/` that currently takes `*session.Storage`:
- `NewAnalyticsStore(storage *session.Storage)`
- `NewApprovalHandler(store *ApprovalStore, storage *session.Storage, ...)`
- `NewGitHubService(storage *session.Storage)`
- `NewProjectService(storage *session.Storage)`
- `NewRulesStore(storage *session.Storage)`
- `NewWorkspaceService(storage *session.Storage, ...)`
- `NewReviewQueueService(storage *session.Storage, ...)`
- `server/review_queue_manager.go`: `storage *session.Storage` field

3.1.2 For the first service that is purely read-only (recommend `NewGitHubService` — it performs read-only access patterns), change the parameter from `*session.Storage` to `session.InstanceStore`.

3.1.3 Verify the change compiles. Add a mock implementation of `session.InstanceStore` in `server/services/mocks_test.go` for use in tests.

3.1.4 Write one unit test for `GitHubService` that injects a fake `InstanceStore` instead of a real `*session.Storage`.

**Note on the type-assertion leak:** `NewSessionService` performs `storage.(*session.Storage)` at lines 122–123 to access `concStorage` for sub-services. This is a known architectural issue documented in `docs/tasks/backend-architecture-improvements.md` as Story 1 (P1). It is out of scope for this PR — do not attempt to fix it here.

### Story 3.2 — Define and use `session.ReviewQueueWriter`

**Tasks:**

3.2.1 Add the following interface to `session/review_queue.go` (or a new `session/interfaces.go` if that file does not yet exist):

```go
// ReviewQueueWriter is implemented by components that only need to add or remove
// items from the review queue, without needing to list or query it.
// Use *ReviewQueue directly when full read-write access is needed.
type ReviewQueueWriter interface {
    Add(item *ReviewItem)
    Remove(sessionID string)
}

// Compile-time assertion: *ReviewQueue must satisfy ReviewQueueWriter.
var _ ReviewQueueWriter = (*ReviewQueue)(nil)
```

3.2.2 Change the `addStartupItem` helper in `server/dependencies.go` to accept `ReviewQueueWriter` instead of `*session.ReviewQueue`:

```go
func addStartupItem(queue session.ReviewQueueWriter, inst *session.Instance, reason session.AttentionReason, priority session.Priority, context string) {
```

3.2.3 Change `syncOrphanedApprovalsToQueue` similarly:

```go
func syncOrphanedApprovalsToQueue(
    store *services.ApprovalStore,
    instances []*session.Instance,
    queue session.ReviewQueueWriter,
) {
```

3.2.4 Verify the call sites in `BuildRuntimeDeps` still compile — they pass `reviewQueue` (`*session.ReviewQueue`) which satisfies `ReviewQueueWriter`.

3.2.5 Write one test that uses a fake `ReviewQueueWriter` in `server/dependencies_test.go` or a new `server/review_queue_test.go` to verify `addStartupItem` calls `Add` with the correct fields.

### Story 3.3 — Define `session.InstanceReader`

**Tasks:**

3.3.1 Add the following interface to `session/interfaces.go` (create the file if it does not exist):

```go
// InstanceReader is the read-only projection of *Instance used by listing,
// filtering, and display operations in the server layer. Services that only
// need to display session data should accept InstanceReader rather than *Instance.
// Services that drive lifecycle operations (Start, Stop, etc.) continue to
// accept *Instance directly.
type InstanceReader interface {
    // Identification
    GetTitle() string
    GetStableID() string
    MatchesID(id string) bool

    // Status predicates
    Started() bool
    Paused() bool
    GetStatus() int // int for SessionAccessor compat

    // Metadata
    GetCreatedAt() time.Time
    GetTags() []string

    // GitHub / PR metadata
    IsPRSession() bool
    GetGitHubRepoFullName() string
    GetPRDisplayInfo() string

    // Review queue
    NeedsReview() bool
    GetReviewItem() (*ReviewItem, bool)

    // Terminal display
    Preview() (string, error)
}

// Compile-time assertion: *Instance must satisfy InstanceReader.
var _ InstanceReader = (*Instance)(nil)
```

3.3.2 Identify at least one method in `server/services/session_service.go` that iterates over instances and only calls read-only methods (e.g., list/filter operations). Change the local type of the instances slice from `[]*session.Instance` to `[]session.InstanceReader` for that method's internal loop variable.

3.3.3 Write one unit test using a struct implementing `InstanceReader` to confirm the interface is mockable without constructing a real `*Instance`.

---

## Epic 4 — Test Coverage for the Wiring Layer (G4)

**Goal:** Grow `server/dependencies_test.go` from 3 tests to at least 8 tests, covering Warren Wire validation behavior and phase ordering.

### Story 4.1 — Add Warren Wire validation tests

**Tasks:**

4.1.1 Add test: `TestBuildServiceDeps_WarrenValidatesStatusManager`
- Construct a valid `*CoreDeps` (use `BuildCoreDepsWithOptions` with an injected `*ent.Client` or a minimal real CoreDeps)
- Delete or skip the `SetStatusManager` call (this will require either a refactor for testability or dependency injection)
- Confirm `BuildServiceDeps` returns an error containing `"StatusManager"`

Note: Testing that a specific `Set` call is "deleted" requires either a testable seam (e.g., a `buildServiceDepsWithSetters` helper that accepts override setters) or a more indirect approach (confirm the Warren error message format on an explicitly nil value). The simpler approach for now: pass a `*CoreDeps` where `ApprovalStore` is nil and confirm the Warren error fires for `ApprovalProvider`.

4.1.2 Add test: `TestBuildServiceDeps_WarrenErrorMessageFormat`
- Construct a `*CoreDeps` where `ApprovalStore` is nil (zero value)
- Call `BuildServiceDeps`
- Confirm the error message contains `"warren:"` and `"ApprovalProvider"`

```go
func TestBuildServiceDeps_WarrenErrorMessageFormat(t *testing.T) {
    core := &CoreDeps{
        Storage:    &session.Storage{}, // minimal non-nil
        EventBus:   &events.EventBus{},
        ReviewQueue: session.NewReviewQueue(),
        // ApprovalStore: nil — intentionally missing
    }
    _, err := BuildServiceDeps(core)
    if err == nil {
        t.Fatal("expected error from Warren validation")
    }
    if !strings.Contains(err.Error(), "warren:") {
        t.Errorf("expected warren error prefix, got: %v", err)
    }
    if !strings.Contains(err.Error(), "ApprovalProvider") {
        t.Errorf("expected ApprovalProvider in error, got: %v", err)
    }
}
```

4.1.3 Add test: `TestBuildServiceDeps_HappyPath_NoWarrenError`
- Construct a fully-populated `*CoreDeps` with all non-nil fields
- Call `BuildServiceDeps`
- Confirm no error is returned

```go
func TestBuildServiceDeps_HappyPath_NoWarrenError(t *testing.T) {
    // Use BuildCoreDepsWithOptions to get a real CoreDeps
    // This requires an in-memory ent client — check for existing test helpers
    // or use a minimal fake if available
    t.Skip("requires test ent client — add when test helpers are available")
}
```

4.1.4 Add test: `TestBuildRuntimeDeps_WarrenValidatesScrollbackManager`
- Construct a minimal `*ServiceDeps` where construction succeeds but the scrollback manager would be nil (this is harder to trigger without config; use a more targeted approach)
- Alternatively: test `warren.Wire` directly in the `pkg/warren` package for this validation pattern

4.1.5 Add test: `TestBuildRuntimeDeps_RejectsNilServiceDepsFields`
- Pass a `&ServiceDeps{}` (all zero fields) to `BuildRuntimeDeps`
- Confirm the error reports the first missing required dependency

### Story 4.2 — Add phase ordering test

**Tasks:**

4.2.1 Add test: `TestBuildDependencies_PhaseOrdering`
- Confirm that calling `BuildRuntimeDeps` with a nil `ServiceDeps` returns an error mentioning "Phase 2 not completed"
- This test already exists as `TestBuildRuntimeDeps_RejectsNilService` — extend it to verify the error message content

```go
func TestBuildRuntimeDeps_ErrorMessageMentionsPhase(t *testing.T) {
    _, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, nil)
    if err == nil {
        t.Fatal("expected error")
    }
    if !strings.Contains(err.Error(), "Phase 2") {
        t.Errorf("expected Phase 2 mention, got: %v", err)
    }
}
```

4.2.2 Add test: `TestBuildServiceDeps_ErrorMessageMentionsPhase`
- Call `BuildServiceDeps(nil)` and verify the error message mentions "Phase 1"

### Story 4.3 — Add Warren Wire unit tests in `pkg/warren`

**Tasks:**

4.3.1 Confirm `pkg/warren/wire_test.go` covers the `Set` + nil value path. Add a test if missing:

```go
func TestSet_SkipsNilAndRecordsError(t *testing.T) {
    w := NewWire("test")
    var called bool
    Set(w, "Foo", func(v *int) { called = true }, nil)
    if called {
        t.Fatal("setter should not be called for nil value")
    }
    err := w.Validate()
    if err == nil {
        t.Fatal("expected validation error for nil setter")
    }
    if !strings.Contains(err.Error(), "Foo") {
        t.Errorf("expected Foo in error: %v", err)
    }
}
```

4.3.2 Add test: `TestSetAlways_CallsSetterUnconditionally`
- Verify `SetAlways` calls the setter even for zero bool (false)

4.3.3 Verify current coverage: `go test ./pkg/warren/... -cover`. Must stay at or above current level.

---

## Implementation Sequence

Execute in this order to minimize integration risk:

1. **Epic 4 (G4) first** — add tests before making changes. The 3 existing tests serve as a regression baseline. New tests for Epic 1 can be added as `t.Skip()` stubs, then un-skipped as Epic 1 progresses.
2. **Epic 1 (G1)** — Warren Wire wiring. Low risk, immediately validates the safety net.
3. **Epic 4 (G4) completion** — un-skip tests now that Warren is wired.
4. **Epic 2 (G2)** — file splitting. Do one story per commit. Run `go test ./session/...` after each story.
5. **Epic 3 (G3)** — interface extraction. Do after G2 since it references methods that may have moved.

Final validation after all epics: `make ci` (lint + build + test).

---

## Open Questions / Flags

1. **`InstanceOptions.ResumeId` field**: `ForkFromCheckpoint` sets `ResumeId` on `InstanceOptions` — confirm this field name exists and is not `ConversationUUID` or similar.

2. **`CheckpointList`, `Checkpoint`, `newCheckpointID`**: verify whether these types are already in a separate `session/checkpoint.go` before Story 2.6 moves them. Run `grep -l "type Checkpoint " session/*.go` to confirm.

3. **`instance_workspace.go` size**: at 564 lines it already exceeds the 400-line target. The requirements document says each sub-file should be under 400 lines. Consider splitting `instance_workspace.go` into `instance_workspace.go` (workspace switch operations) and `instance_vcs.go` (VCS delegation). This is not blocking for this PR.

4. **Fake `InstanceStore` for tests**: Story 4.1.3 notes that a test `ent.Client` or in-memory store may be needed. Check `server/dependencies_test.go` for existing test setup helpers before writing new ones.

---

## Summary

| Metric | Count |
|---|---|
| Epics | 4 |
| Stories | 18 |
| Tasks | ~65 |

**Epic 1 (G1):** 4 stories, ~10 tasks — Warren Wire on all synchronous setters in all 3 build phases.
**Epic 2 (G2):** 11 stories, ~33 tasks — Split 3168-line `instance.go` into 9 domain files; 2 already exist.
**Epic 3 (G3):** 3 stories, ~12 tasks — `ReviewQueueWriter` interface, `InstanceReader` interface, `InstanceStore` consistency audit.
**Epic 4 (G4):** 3 stories, ~10 tasks — 5+ new tests in `server/dependencies_test.go` covering Warren validation, phase ordering, and error message format.

**Flagged technology choices:**
- `warren.SetAlways` required for `PRStatusPoller.SetOnUpdated` (func literal — not comparable)
- Goroutine-internal setters (lines 562–563 in `BuildRuntimeDeps`) are explicitly excluded from Warren tracking; they must stay unwrapped
- `UnfinishedWorkService` is legitimately nil in some configs — excluded from mandatory Warren validation, documented on `RuntimeDeps` struct
- `session_service.go`'s type assertion `storage.(*session.Storage)` is a known architectural issue (Architecture Story 1, P1) — out of scope, not addressed in this PR
- `instance_workspace.go` already exceeds the 400-line target; flagged for a follow-on split but not blocking
