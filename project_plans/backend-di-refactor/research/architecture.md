# Architecture Research — Dependency Graph and Interface Extraction

## 1. Dependency Graph

### Phase 1: BuildCoreDeps — Produces CoreDeps

**Inputs:** config (implicit, via `services.NewSessionServiceFromConfig()`)

**Constructs:**
- `*services.SessionService` — top-level orchestrator; owns storage, event bus, review queue
- `*session.Storage` — extracted from SessionService via `sessionService.GetStorage()`
- `*events.EventBus` — extracted from SessionService via `sessionService.GetEventBus()`
- `*session.ReviewQueue` — extracted from SessionService via `sessionService.GetReviewQueueInstance()`
- `*services.ApprovalStore` — extracted from SessionService via `sessionService.GetApprovalStore()`
- `*services.ErrorRegistry` — constructed with `storage.GetEntClient()`, then wired into SessionService via `SetErrorRegistry`

**CoreDeps output fields:**
```
CoreDeps {
  SessionService *services.SessionService
  Storage        *session.Storage
  EventBus       *events.EventBus
  ReviewQueue    *session.ReviewQueue
  ApprovalStore  *services.ApprovalStore
  ErrorRegistry  *services.ErrorRegistry
}
```

### Phase 2: BuildServiceDeps — Consumes CoreDeps, Produces ServiceDeps

**Inputs:** `*CoreDeps`

**Constructs:**
- `*session.InstanceStatusManager` — standalone, no deps
- `*session.ReviewQueuePoller` — takes ReviewQueue, StatusManager, Storage; gets `SetApprovalProvider(ApprovalStore)`
- `*session.PRStatusPoller` — takes Storage

**Wires into CoreDeps:**
- `core.SessionService.SetStatusManager(statusManager)`
- `core.SessionService.SetReviewQueuePoller(reviewQueuePoller)`

**ServiceDeps output** embeds `*CoreDeps` plus:
```
ServiceDeps {
  *CoreDeps
  StatusManager     *session.InstanceStatusManager
  ReviewQueuePoller *session.ReviewQueuePoller
  PRStatusPoller    *session.PRStatusPoller
}
```

### Phase 3: BuildRuntimeDeps — Consumes ServiceDeps, Produces RuntimeDeps

**Requires:** `tmux.TmuxServerReady` token (enforces ordering)

**Constructs (synchronous):**
- `[]*session.Instance` — loaded from `storage.LoadInstances()`; each gets `SetReviewQueue` + `SetStatusManager`
- `*ReactiveQueueManager` — takes ReviewQueue, ReviewQueuePoller, EventBus, StatusManager, Storage
- `*session.HistoryLinker` — takes instances
- `*scrollback.ScrollbackManager` — standalone
- `*session.ExternalTmuxStreamerManager` — standalone
- `*session.ExternalSessionDiscovery` — standalone, gets OnSessionAdded/OnSessionRemoved callbacks
- `*session.ExternalApprovalMonitor` — standalone, gets OnApproval callback
- `*unfinished.Scanner`, `*unfinished.StateStore`, `*services.UnfinishedWorkService` — optional (nil if config missing)

**Wires into SessionService:**
- `SetReactiveQueueManager(reactiveQueueMgr)`
- `SetHistoryLinker(historyLinker)`
- `SetScrollbackManager(scrollbackManager)`
- `SetExternalDiscovery(externalDiscovery)`

**Heavy work deferred to goroutine:** instance tmux start, controller start, startup scan, approval sync

---

## 2. Methods on *session.Instance — Who Calls What

### Methods called by server/services/ (cross-boundary calls)

From `server/services/session_service.go` and handlers:

**Read-only access (field reads or getter methods):**
- `inst.Title` — direct field read
- `inst.Status` — direct field read
- `inst.Branch` — direct field read
- `inst.Path` — direct field read
- `inst.WorkingDir` — direct field read
- `inst.Program` — direct field read
- `inst.Tags` — direct field read
- `inst.Category` — direct field read
- `inst.GetDiffStats()` — read-only computed
- `inst.GetTitle()` — getter
- `inst.GetStableID()` — getter
- `inst.GetTags()` — getter (returns copy)
- `inst.Started()` — boolean
- `inst.Paused()` — boolean
- `inst.IsPRSession()` — boolean
- `inst.GetGitHubRepoFullName()` — string
- `inst.GetPRDisplayInfo()` — string
- `inst.LastMeaningfulOutput` — direct field read (time.Time)
- `inst.GetCreatedAt()` — getter

**Lifecycle methods:**
- `inst.Start(bool)` — called from session_service.go CreateSession
- `inst.Stop()` / `inst.Pause()` / `inst.Resume()` / `inst.Destroy()` / `inst.Kill()`
- `inst.StartController()` — starts Claude controller

**State mutation:**
- `inst.SetTitle(title)` — rename
- `inst.SetTags(tags)` — tag update
- `inst.AddTag(tag)` / `inst.RemoveTag(tag)` — tag mutation
- `inst.SetReviewQueue(q)` — wiring
- `inst.SetStatusManager(m)` — wiring
- `inst.SetRateLimitCallbacks(...)` — callback wiring

**Approval/review:**
- `inst.MarkNeedsApproval()` / `inst.Approve()` / `inst.Deny()`
- `inst.NeedsReview()` — boolean
- `inst.GetReviewItem()` — returns *ReviewItem

**Terminal/content:**
- `inst.Preview()` — string
- `inst.CaptureCurrentState()` — terminal capture
- `inst.GetExitContent()` — []byte

### Methods called only within session/ package (internal)

- `inst.start(...)` — private, delegates to all sub-managers
- `inst.setStatus(...)` / `inst.transitionTo(...)` — private state machine
- `inst.buildLaunchCommand(...)` — private tmux command builder
- `inst.setupFirstTimeWorktree()` — private worktree setup
- `inst.resolveStartPath(...)` — private path resolution
- `inst.ensureTagManager()` — private init
- `inst.wireRateLimitCallbacks(...)` — private
- `inst.fireLifecycleEvent(...)` — private

---

## 3. Narrowest Interface session_service.go Needs from *session.Instance

Based on analysis of what `server/services/session_service.go` actually calls on `*session.Instance`:

```go
// InstanceReader is the read-only projection of *session.Instance that
// most listing/filtering operations in session_service need.
type InstanceReader interface {
    // Identification
    GetTitle() string
    GetStableID() string
    MatchesID(id string) bool

    // Status
    Started() bool
    Paused() bool
    GetStatus() int            // returns int for SessionAccessor compat

    // Session metadata
    GetCreatedAt() time.Time
    GetTags() []string

    // GitHub / PR
    IsPRSession() bool
    GetGitHubRepoFullName() string
    GetPRDisplayInfo() string

    // Review queue integration
    NeedsReview() bool
    GetReviewItem() (*ReviewItem, bool)

    // Terminal
    Preview() (string, error)
}
```

However, `session_service.go` also performs lifecycle operations (`Start`, `Stop`, `Pause`, `Resume`, `Destroy`) and wiring (`SetReviewQueue`, `SetStatusManager`, etc.), so a single interface cannot cover all usage. The practical approach is:

1. Extract `InstanceReader` for listing/display operations.
2. Keep `*session.Instance` for lifecycle management functions.
3. Gradually migrate read-only service operations to `InstanceReader`.

**session_service.go also does type assertions**: Line 122-123 and 279 perform `storage.(*session.Storage)` to access `concStorage`. This is a known abstraction leak (documented as Architecture Story 1).

---

## 4. Narrowest Interface a ReviewQueue Consumer Needs

From analysis of how `*session.ReviewQueue` is consumed in `server/services` and `server/`:

### Write-only consumers (only call Add)
- `server/dependencies.go:addStartupItem()` — calls `queue.Add(item)`
- `server/dependencies.go:syncOrphanedApprovalsToQueue()` — calls `queue.Add(item)`
- ExternalApprovalMonitor callback — calls `reviewQueue.Add(item)`
- ExternalDiscovery OnSessionAdded callback — calls `reviewQueue.Remove(instance.Title)` on removal

```go
// ReviewQueueWriter is for components that only enqueue items.
type ReviewQueueWriter interface {
    Add(item *ReviewItem)
    Remove(sessionID string)
}
```

### Read-write consumers (need full API)
- `server/review_queue_manager.go` — needs List, Has, Get, Add, Remove, Clear
- `server/services/review_queue_service.go` — needs List, Get, Has, Remove for API responses

### Current ReviewQueue public methods (from queue usage patterns):
- `Add(item *ReviewItem)`
- `Get(sessionID string) (*ReviewItem, bool)`
- `Has(sessionID string) bool`
- `Remove(sessionID string)`
- `List() []*ReviewItem` (or similar)

**Recommendation**: Extract `ReviewQueueWriter` (Add + Remove) as the narrowest interface. Services like `addStartupItem` and the ExternalApprovalMonitor only need this surface. The `ReviewQueueManager` and `ReviewQueueService` need the full concrete type or a wider `ReviewQueueReadWriter` interface.

---

## 5. Summary: Coupling Severity

| Coupling Point | Severity | Fix |
|---|---|---|
| 8+ service constructors take `*session.Storage` | High | Accept `session.InstanceStore` or narrower per-service interface |
| `NewSessionService` type-asserts `session.InstanceStore` to `*session.Storage` | High | Architecture Story 1 (documented separately) |
| `*session.ReviewQueue` used concrete in server layer | Medium | Extract `ReviewQueueWriter` for write-only consumers |
| 16 setter calls in BuildRuntimeDeps with no validation | High | Wrap in `warren.Wire` + `Validate()` |
| `BuildServiceDeps` has 3 setter calls with no Warren tracking | High | Same fix |
