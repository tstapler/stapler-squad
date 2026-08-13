# Pitfalls Research — Risks, Gotchas, and Test Coverage

## 1. Test Coverage for server/dependencies.go Wiring

### Current tests in server/dependencies_test.go (827 bytes)

Only 3 tests exist, all testing nil-guard panics at the struct level:

```go
TestBuildServiceDeps_RejectsNilCore         // nil *CoreDeps → error
TestBuildServiceDeps_RejectsNilCoreFields   // CoreDeps{} with all zero fields → error
TestBuildRuntimeDeps_RejectsNilService      // nil *ServiceDeps → error
```

**What is NOT tested:**
- Removing a specific `Set*` call and verifying the app reports a descriptive error (not a nil panic)
- That `warren.Wire.Validate()` fires at all (the function is not yet called)
- That Phase 1 → Phase 2 → Phase 3 ordering is enforced at the type level (currently enforced by parameter types, not tested)
- `BuildCoreDepsWithOptions` with an injected `*ent.Client` (test path)
- That `SetScrollbackManager` being skipped produces an error vs. silently leaving a nil `scrollbackSequencer`

### Integration tests for wiring

`server/notification_wiring_test.go` (1.8K) tests notification wiring but is unrelated to the DI setter chain.

There are no integration-level tests that construct a complete `ServerDependencies` in a test environment and verify all fields are non-nil. The only integration coverage is implicit: the app would panic on nil access during normal operation.

**Gap severity: HIGH.** A typo deleting a `SetStatusManager` call would not be caught until runtime.

---

## 2. Double-Set Bugs — Are Any Setters Called More Than Once?

Analysis of all setter calls in `server/dependencies.go`:

| Setter | Call Count | Risk |
|---|---|---|
| `inst.SetReviewQueue` | 2× — once in Phase 3 loop (line 450) for loaded instances, once in ExternalDiscovery `OnSessionAdded` callback (line 562) | INTENTIONAL — second call is for newly-discovered sessions, not the same instance |
| `inst.SetStatusManager` | 2× — same pattern as above (lines 451, 563) | INTENTIONAL — same reason |
| `sessionService.SetStatusManager` | 1× | OK |
| `sessionService.SetReviewQueuePoller` | 1× | OK |
| `sessionService.SetReactiveQueueManager` | 1× | OK |
| `sessionService.SetHistoryLinker` | 1× | OK |
| `sessionService.SetScrollbackManager` | 1× | OK |
| `sessionService.SetExternalDiscovery` | 1× | OK |
| `sessionService.SetErrorRegistry` | 1× | OK |
| `reviewQueuePoller.SetApprovalProvider` | 1× | OK |
| `reviewQueuePoller.SetInstances` | 1× | OK |
| `svc.PRStatusPoller.SetInstances` | 1× | OK |
| `svc.PRStatusPoller.SetOnUpdated` | 1× | OK |
| `historyLinker.SetInstances` | 1× | OK |

**Conclusion**: No double-set bugs exist for the single-valued setters. The per-instance `SetReviewQueue`/`SetStatusManager` calls are correctly called for each new instance (not re-set on the same instance twice). Warren's `Set` would catch nil values but would NOT catch legitimate double calls on the same component — this is not a risk here.

---

## 3. Go Init-Order Risk for instance.go File Splitting

### Package-level `init()` functions in session package

The `session/` package (non-generated, non-subpackage) has **no `init()` functions** in any `instance*.go` file. Confirmed:
- `session/instance.go` — no `init()`
- `session/instance_status.go` — no `init()`
- `session/instance_workspace.go` — no `init()`

The only `init()` in `session/` is in sub-packages (`session/detection/ratelimit/detector.go:433`, `session/ent/` generated code). These are in different packages and not affected by file splits in `session/`.

### Package-level variables in instance.go

`instance.go` declares:
- `Status` constants (iota) — no file dependency
- `LifecycleEvent` constants (iota) — no file dependency
- `SessionType` constants — no file dependency

None of these reference variables from other files in initialization order-sensitive ways. The split is safe.

### Method ordering risk

Go methods are not ordered by file — the compiler resolves them across all files in the package. Moving `func (i *Instance) Foo()` from `instance.go` to `instance_tmux.go` has zero runtime effect as long as both files declare `package session`.

**Conclusion: Zero init-order risk from the file split.**

---

## 4. Test Files That Access Internal Symbols — Breaking Risk

### Private field accesses in test files (same-package tests)

Since all test files are in `package session` (not `package session_test`), they have access to all private fields. Moving code between files within `package session` does NOT break this — field access depends on package membership, not file location.

**Private field accesses found in test files:**

| Test File | Private Access | Risk of Split Breaking It |
|---|---|---|
| `instance_test.go:86-87` | `instance.gitManager.worktree` | NONE — if `gitManager` stays on Instance struct, access is unchanged regardless of which file defines the method |
| `comprehensive_session_creation_test.go:226` | `instance.tmuxManager.session = mockTmuxSession` | NONE — same reason |
| `instance_concurrency_test.go:31,33` | `inst.stateMutex.Lock/Unlock` | NONE — mutex stays on Instance struct |
| `instance_fork_test.go:86` | `fork.gitManager.HasWorktree()` | NONE — gitManager is an unexported field on Instance struct |
| `instance_rename_test.go:112` | `started: tt.started` — struct literal | NONE — `started` field stays on Instance struct |
| `review_queue_poller_test.go:18,301` | `inst.started = true` | NONE — `started` field stays on Instance struct |
| `storage_test.go:55,82,116,153,175,184,344` | `inst.started = true` | NONE — field stays on Instance struct |
| `session_restart_test.go:71` | `instance.tmuxManager.session != nil` | NONE |
| `wire_callbacks_concurrency_test.go:20` | `i.stateMutex.Lock()` | NONE |

**Critical insight**: All private field accesses are to **struct fields on `Instance`** — not to helper functions or package-level variables that would move with a file split. The struct definition stays in `instance.go` per the plan. Moving methods to other files does not affect field visibility.

### Conclusion: Zero breaking risk to private field access from the planned split.

---

## 5. instance_test.go and integration_test.go Dependency Depth

### instance_test.go (7.5K)

In `package session`. Uses:
- `InstanceOptions` struct — stays in `instance.go`
- `NewInstance()` constructor — stays in `instance.go`
- `SessionTypeNewWorktree`, `SessionTypeExistingWorktree` constants — may move to a constants file, but can stay in `instance.go`
- `FromInstanceData()` / `InstanceData` — targeted for `instance_serialization.go`; test still works as long as function is in `package session`

### integration_test.go (29K, 903 lines)

In `package session`. Uses `TestMain` to manage tmux server lifecycle. Uses:
- `NewInstance()`, `Instance` struct fields (Title, Status, Branch, Path, etc.)
- Session lifecycle calls: `Start()`, `Stop()`, `Pause()`, `Resume()`
- `GetReviewQueue()`, `SetReviewQueue()`

None of these are affected by moving methods to separate files — they remain in `package session`.

---

## 6. Additional Risks

### Risk: Warren.Set with interface types

`warren.Set[T comparable]` uses `value == zero` to detect nil. Interface types in Go support `==` comparison, so this works for `*session.ReviewQueue` (pointer), `session.InstanceStore` (interface), etc. **However**, if a setter accepts a non-comparable type (a struct with slice fields), `warren.Set` cannot be used — `warren.SetAlways` must be used instead, which skips nil detection.

For the specific setters to wrap:
- `SetStatusManager(*InstanceStatusManager)` — pointer, works with `Set`
- `SetReviewQueuePoller(*ReviewQueuePoller)` — pointer, works with `Set`
- `SetReactiveQueueManager(ReactiveQueueManager)` — interface, works with `Set`
- `SetHistoryLinker(*HistoryLinker)` — pointer, works with `Set`
- `SetScrollbackManager(scrollbackSequencer)` — interface, works with `Set`
- `SetExternalDiscovery(*ExternalSessionDiscovery)` — pointer, works with `Set`
- `SetApprovalProvider(ApprovalMetadataProvider)` — interface, works with `Set`

All relevant setters accept comparable types. **No risk.**

### Risk: Phase 3 background goroutine — setter calls inside goroutine

Lines 562–563 (`instance.SetReviewQueue` / `instance.SetStatusManager`) are inside a goroutine. Warren's `Wire` cannot track these because they happen asynchronously after `BuildRuntimeDeps` returns. These per-instance wires should remain untracked, or be handled via a separate validation that checks all instances after the goroutine completes. **Recommendation**: Do not wrap goroutine-internal setters in Warren; focus Warren coverage on the synchronous top-level wires only.

### Risk: UnfinishedWorkService may be nil

`unfinishedWorkSvc` is legitimately nil when `config.GetConfigDir()` fails. The `RuntimeDeps` struct documents this. Warren's `Set` would flag this as missing even though nil is acceptable. **Fix**: use `warren.SetAlways` or skip Warren tracking for optional deps. Alternatively, use `warren.Wire.Require` only for mandatory setters.
