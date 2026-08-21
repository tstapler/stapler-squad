# Features Audit — Setter Calls, Interfaces, Concrete Type Coupling

## 1. All Set* Calls in server/dependencies.go

### BuildCoreDeps (Phase 1) — 1 setter call

| Line | Call | Target Field | Nil Guard? |
|---|---|---|---|
| 338 | `sessionService.SetErrorRegistry(errorRegistry)` | `SessionService.errorRegistry` | None — `errorRegistry` is freshly constructed, always non-nil |

### BuildServiceDeps (Phase 2) — 3 setter calls

| Line | Call | Target Field | Nil Guard? |
|---|---|---|---|
| 378 | `reviewQueuePoller.SetApprovalProvider(core.ApprovalStore)` | poller's `approvalProvider` | None |
| 381 | `core.SessionService.SetStatusManager(statusManager)` | `SessionService.statusManager` | None |
| 382 | `core.SessionService.SetReviewQueuePoller(reviewQueuePoller)` | `SessionService.reviewQueuePoller` | None |

Note: `statusManager` and `reviewQueuePoller` are constructed on lines 374–382. They are always non-nil if construction succeeds, but there is no `warren.Wire` tracking any of these — if a setter call were deleted, no error would surface at startup.

### BuildRuntimeDeps (Phase 3) — 10 setter calls (at top-level, excluding goroutine)

| Line | Call | Target | Nil Guard? |
|---|---|---|---|
| 450 | `inst.SetReviewQueue(reviewQueue)` | per-instance wiring | None (in loop) |
| 451 | `inst.SetStatusManager(statusManager)` | per-instance wiring | None (in loop) |
| 453 | `reviewQueuePoller.SetInstances(instances)` | poller's instance list | None |
| 454 | `svc.PRStatusPoller.SetInstances(instances)` | PR poller's instance list | None |
| 455 | `svc.PRStatusPoller.SetOnUpdated(...)` | PR poller callback | None |
| 529 | `sessionService.SetReactiveQueueManager(reactiveQueueMgr)` | SessionService | None |
| 535 | `historyLinker.SetInstances(instances)` | history linker | None |
| 536 | `sessionService.SetHistoryLinker(historyLinker)` | SessionService | None |
| 548 | `sessionService.SetScrollbackManager(scrollbackManager)` | SessionService | None |
| 626 | `sessionService.SetExternalDiscovery(externalDiscovery)` | SessionService | None |

Additionally, inside the background goroutine (lines 562–563):
- `instance.SetReviewQueue(reviewQueue)` — per newly-discovered external session
- `instance.SetStatusManager(statusManager)` — per newly-discovered external session

**Total setter calls with no nil guard or Warren tracking**: 14 calls across all 3 phases.

### SessionService Set* Methods (for Warren wrapping reference)

From `server/services/session_service.go`:
```go
SetErrorRegistry(r *ErrorRegistry)
SetReactiveQueueManager(mgr ReactiveQueueManager)
SetMCPServerURL(url string)
SetHistoryLinker(hl *session.HistoryLinker)
SetReviewQueuePoller(poller *session.ReviewQueuePoller)
SetStatusManager(mgr *session.InstanceStatusManager)
SetExternalDiscovery(discovery *session.ExternalSessionDiscovery)
SetNotificationStore(store *notifications.NotificationStore)
SetConfigService(svc *ConfigService)
SetScrollbackManager(mgr scrollbackSequencer)
```

---

## 2. Existing Interfaces in session/ and server/services/

### session/ package interfaces

| Interface | File | Purpose |
|---|---|---|
| `LifecycleListener` | `instance.go:81` | Receives EventStarted/EventExited |
| `InstanceContext` | `claude_controller.go:20` | Minimal view of Instance for controller |
| `PTYReader` | `detection/idle.go:41` | PTY read abstraction |
| `BufferReader` | `detection/ratelimit/integration.go:10` | Rate limit buffer read |
| `SessionAccessor` | `detection/ratelimit/manager.go:19` | GetStatus(), GetStableID() for rate limit |
| `GitManager` | `git_worktree_manager.go:207` | Git worktree operations |
| `ProcessFileInspector` | `history_detector.go:22` | History file inspection |
| `ReviewQueueObserver` | `queue/queue.go:188` | Queue change notifications |
| `Repository` | `repository.go:11` | Persistence backend abstraction |
| `ApprovalMetadataProvider` | `review_queue_poller.go:50` | Approval lookup for poller |
| `ScrollbackStorage` | `scrollback/storage.go:18` | Scrollback persistence |
| `InstanceStore` | `storage.go:159` | Minimal storage interface for server layer |
| `PtyFactory` | `tmux/pty.go:10` | PTY creation |
| `SessionExistenceChecker` | `tmux/registry_port.go:7` | Does session exist check |
| `SessionLister` | `tmux/registry_port.go:14` | List sessions |
| `PaneExitSubscriber` | `tmux/registry_port.go:22` | Pane exit events |
| `TmuxStatePort` | `tmux/registry_port.go:27` | Combined tmux state |
| `TmuxManager` | `tmux_process_manager.go:356` | Tmux session management |
| `VCSProvider` | `vc/provider.go:13` | VCS abstraction |
| `VCS` | `vcs/vcs.go:126` | Git VCS operations |
| `DistributedLock` | `workspace/lock.go:13` | Distributed locking |
| `LockHandle` | `workspace/lock.go:29` | Lock handle |
| `CacheInvalidationNotifier` | `workspace/lock.go:50` | Cache invalidation |
| `Registry` | `workspace/registry.go:13` | Workspace registry |

### server/services/ package interfaces

| Interface | File | Purpose |
|---|---|---|
| `ReviewQueueChecker` | `approval_handler.go:40` | Check if session is in queue |
| `approvalNotificationStamper` | `approval_handler.go:47` | Notification timestamps |
| `autoApprovalLogger` | `approval_handler.go:53` | Auto-approval audit log |
| `notificationMetadataStore` | `approval_service.go:17` | Notification metadata storage |
| `instanceFinder` | `session_image_upload_handler.go:25` | Find live instance by ID |
| `ReactiveQueueManager` | `session_service.go:40` | Reactive queue operations |
| `scrollbackSequencer` | `session_service.go:109` | Scrollback sequence provider |
| `SessionStreamer` | `session_streamer.go:8` | Terminal streaming |
| `WorkspaceProvider` | `workspace_service.go:23` | Workspace info provider |

---

## 3. Services Taking Concrete Types vs Interfaces

### Services taking *session.Storage (concrete — should be InstanceStore or narrower)

| Service Constructor | Concrete Type | Notes |
|---|---|---|
| `NewAnalyticsStore(storage *session.Storage)` | `*session.Storage` | Also needs GetEntClient() — may need wider interface |
| `NewApprovalHandler(store *ApprovalStore, storage *session.Storage, ...)` | `*session.Storage` | Uses storage.FindLiveInstance indirectly |
| `NewGitHubService(storage *session.Storage)` | `*session.Storage` | Read-only access patterns |
| `NewProjectService(storage *session.Storage)` | `*session.Storage` | Read-only access patterns |
| `NewRulesStore(storage *session.Storage)` | `*session.Storage` | Likely uses narrow subset |
| `NewWorkspaceService(storage *session.Storage, ...)` | `*session.Storage` | |
| `NewUnfinishedWorkService(storage *session.Storage, ...)` | `*session.Storage` | |
| `NewReviewQueueService(storage *session.Storage, ...)` | `*session.Storage` | |

Also: `server/review_queue_manager.go:25` has `storage *session.Storage` field.

### Services already using session.InstanceStore (correct pattern)

| Component | File | Notes |
|---|---|---|
| `server/mcp/server.go` | `NewCore(store session.InstanceStore, ...)` | Correct |
| `server/mcp/tools_discovery.go` | `store session.InstanceStore` | Correct |
| `server/mcp/tools_lifecycle.go` | `store session.InstanceStore` | Correct |
| `server/mcp/tools_terminal.go` | `store session.InstanceStore` | Correct |
| `server/mcp/tools_vcs.go` | `store session.InstanceStore` | Correct |
| `server/services/session_image_upload_handler.go` | `storage session.InstanceStore` | Correct, has mock test |

### The InstanceStore Abstraction Leak (Known Issue)

`NewSessionService` accepts a `session.InstanceStore` but immediately performs a type assertion at line 103–105 (`storage.(*session.Storage)`) to access `concStorage`, which is then passed to `ReviewQueueService`, `RulesStore`, and `AnalyticsStore`. This means any fake `InstanceStore` injected in tests silently bypasses those sub-services. This is documented in `docs/tasks/backend-architecture-improvements.md` as Story 1 (P1).

### Services taking *session.ReviewQueue (concrete)

`*session.ReviewQueue` is used in several places:
- `server/dependencies.go:29` — `ReviewQueue *session.ReviewQueue` in `ServerDependencies`
- `server/review_queue_manager.go:21` — `queue *session.ReviewQueue`
- `server/services/review_queue_service.go:29` — `reviewQueue *session.ReviewQueue`
- `server/dependencies.go:110, 187, 214` — parameter types

The `ReviewQueue` type has only one exported constructor (`NewReviewQueue()`, line 50 in `review_queue.go`). Its public API (based on usage): `Add`, `Get`, `Has`, `Remove`. A narrow `ReviewQueueWriter` interface needs only `Add` for services that only enqueue items.

---

## 4. server/dependencies_test.go Current Coverage

File: `server/dependencies_test.go` (827 bytes, 3 tests only)

```go
func TestBuildServiceDeps_RejectsNilCore(t *testing.T)        // nil CoreDeps → error
func TestBuildServiceDeps_RejectsNilCoreFields(t *testing.T)  // CoreDeps{} (all nil) → error
func TestBuildRuntimeDeps_RejectsNilService(t *testing.T)     // nil ServiceDeps → error
```

**Gaps:**
- No test for missing individual setter (e.g., what happens if `SetStatusManager` is skipped)
- No test that Warren validation fires with a descriptive error
- No test for phase ordering enforcement (BuildRuntimeDeps before tmux ready)
- No test that all required setters on a happy-path wiring succeed
- No integration test that `BuildCoreDepsWithOptions` with an injected `EntClient` works end-to-end
