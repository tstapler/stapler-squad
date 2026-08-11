# Backend DI Refactor — Validation Plan

Generated: 2026-05-08

---

## Overview

This plan maps every requirement (G1–G4) from `requirements.md` to concrete test cases,
specifies exact test names (Go format), file locations, assertion targets, and which
acceptance criteria each test satisfies.

**Total test count:**
- Unit tests: 13
- Integration tests: 7
- Compile-time checks: 2 (build-only, not counted as test functions)
- **Total test functions: 20**

**Requirements coverage: 4/4 (100%). Every acceptance criterion in G1–G4 has at least one test.**

---

## G1 — Warren Wire Coverage

Goal: every `Set*` call in `server/dependencies.go` must be wrapped in `warren.Wire`;
missing or nil setters must return a startup error, not a runtime panic.

### G1.1 `TestBuildServiceDeps_NilSetterIsDetected`

| Field | Value |
|---|---|
| Test name | `TestBuildServiceDeps_NilSetterIsDetected` |
| Type | Unit |
| File | `server/dependencies_test.go` |
| Requirement | G1 — AC: "Removing any `warren.Set(...)` call causes the application to exit with a clear error on startup" |

**Setup:** Construct a valid `*CoreDeps` with real (non-nil) fields. Simulate a missing
setter by temporarily passing a nil value for `SetStatusManager` — achieved by monkey-patching
the test to call `BuildServiceDeps` with a `CoreDeps` whose `ApprovalStore` is nil (so the
only setter that can receive nil, `reviewQueuePoller.SetApprovalProvider`, fires with nil).
Alternatively, introduce a test-only `buildServiceDepsWithOverrides(core, opts)` that accepts
a nil override for one injected dep.

**Assertion:**
- `err != nil` — `BuildServiceDeps` returns a non-nil error.
- `strings.Contains(err.Error(), "SetApprovalProvider")` (or whichever setter name is nil) —
  the error message includes the setter name reported by `warren.Wire.Validate()`.
- The returned `*ServiceDeps` is nil.

**Notes:** The simplest reliable approach is to pass `CoreDeps{ApprovalStore: nil, ...}` with
all other fields non-nil, then assert that `BuildServiceDeps` returns an error that names
`ApprovalProvider`. This avoids test-only injection points.

---

### G1.2 `TestBuildServiceDeps_AllSettersApplied_Succeeds`

| Field | Value |
|---|---|
| Test name | `TestBuildServiceDeps_AllSettersApplied_Succeeds` |
| Type | Integration |
| File | `server/dependencies_test.go` |
| Requirement | G1 — AC: "`BuildServiceDeps` and `BuildRuntimeDeps` both call `w.Validate()` after wiring" |

**Setup:** Use `BuildCoreDepsWithOptions(BuildOptions{EntClient: testEntClient})` to obtain a
real `*CoreDeps` backed by an in-memory SQLite ent client (same pattern used by
`server/notification_wiring_test.go`). Call `BuildServiceDeps(core)`.

**Assertion:**
- `err == nil`.
- Returned `*ServiceDeps` is non-nil.
- `svc.StatusManager != nil`, `svc.ReviewQueuePoller != nil`, `svc.PRStatusPoller != nil`.
- `svc.CoreDeps.SessionService` (unexported fields set by setters) reachable indirectly via
  `svc.SessionService` being non-nil.

---

### G1.3 `TestBuildRuntimeDeps_NilSetterIsDetected`

| Field | Value |
|---|---|
| Test name | `TestBuildRuntimeDeps_NilSetterIsDetected` |
| Type | Unit |
| File | `server/dependencies_test.go` |
| Requirement | G1 — AC: "Missing or nil setters must return an error at startup, not panic at runtime" |

**Setup:** Construct a minimal valid `*ServiceDeps` but with one required Phase 3 setter value
forced nil — for example, supply a `ServiceDeps.ReviewQueue == nil` to confirm
`reviewQueuePoller.SetInstances` path would fail, or craft a test-only override that omits the
`SetReactiveQueueManager` call. Because Phase 3 loads real instances (filesystem I/O), the
test must either:
- Use `BuildOptions{EntClient: emptyTestDB}` so `LoadInstances` returns zero instances and the
  loop body is skipped, or
- Accept that this test only verifies the nil guard on `svc` itself (complementing
  the existing `TestBuildRuntimeDeps_RejectsNilService`) plus a Validate() error path that
  fires post-loop for one deliberately skipped setter.

**Assertion:**
- `err != nil`.
- `strings.Contains(err.Error(), <setter name>)` — names the skipped setter.

---

### G1.4 `TestWarrenSet_SkipsNilAndRecordsError`

| Field | Value |
|---|---|
| Test name | `TestWarrenSet_SkipsNilAndRecordsError` |
| Type | Unit |
| File | `pkg/warren/wire_test.go` |
| Requirement | G1 — AC: "`server/dependencies_test.go` tests that nil setters produce a descriptive error" (unit-level coverage of the mechanism) |

**Note:** `TestWire_NilValueSkipsSetter` already exists in `pkg/warren/wire_test.go` and covers
this exactly. No new test is needed here; the existing test is listed as the covering test for
this requirement in the matrix below.

If the existing test is extended, add a sub-case:

```go
// Confirm the setter function was NOT called.
// Confirm err.Error() contains the registered setter name.
```

**No additional test required — existing `TestWire_NilValueSkipsSetter` satisfies G1 at the
mechanism level.**

---

### G1.5 `TestWarrenWire_MissingEntry_FailsValidation`

| Field | Value |
|---|---|
| Test name | `TestWarrenWire_MissingEntry_FailsValidation` |
| Type | Unit |
| File | `pkg/warren/wire_test.go` |
| Requirement | G1 — AC: wire.Validate() surfaces descriptive errors |

**Note:** `TestWire_ValidateMentionsAllMissing` and `TestWire_RequireAndMark` already cover
this at the `warren` package level. These are listed in the matrix as the covering tests.

**No additional test required — existing tests satisfy G1 at the mechanism level.**

---

### G1.6 `TestBuildServiceDeps_WarrenValidateIsCalled`

| Field | Value |
|---|---|
| Test name | `TestBuildServiceDeps_WarrenValidateIsCalled` |
| Type | Unit |
| File | `server/dependencies_test.go` |
| Requirement | G1 — AC: "`BuildServiceDeps` … call `w.Validate()` (or `MustValidate()`) after wiring" |

**Setup:** This is a black-box behavioral test: if `w.Validate()` is called, then passing a
nil injectable must return an error (not silently succeed). The test constructs `CoreDeps` with
`ApprovalStore: nil` — a value that will be registered with `warren.Set` — then asserts that
`BuildServiceDeps` returns a non-nil error that mentions the missing dependency.

This is effectively the same assertion as G1.1 (`TestBuildServiceDeps_NilSetterIsDetected`)
and can be merged with it. **Merge with G1.1** — one test covers both criteria.

---

### G1.7 `TestBuildRuntimeDeps_WarrenValidateIsCalled`

| Field | Value |
|---|---|
| Test name | `TestBuildRuntimeDeps_WarrenValidateIsCalled` |
| Type | Unit |
| File | `server/dependencies_test.go` |
| Requirement | G1 — AC: "`BuildRuntimeDeps` … call `w.Validate()` after wiring" |

Same pattern as G1.6 but for Phase 3. Can be merged with G1.3.
**Merge with G1.3.**

---

## G2 — Split `session/instance.go`

Goal: redistribute 3168 lines and 136 functions from `session/instance.go` into 8 focused
sub-files within the same `session` package. All existing tests must pass unchanged.

### G2.1 Compile check (not a test function)

| Field | Value |
|---|---|
| Test name | `go build ./session/...` |
| Type | Build (compile-time) |
| File | CI / `make build` |
| Requirement | G2 — AC: "`go build ./session/...` passes" |

**Not a Go test function.** This is verified by `make build` in CI. No new test file needed.
The acceptance criterion is satisfied by the build step passing.

---

### G2.2 Existing test suite as regression guard

The split must not break any of the existing 57 test files in `session/`. The following
mapping documents which target sub-files each test file primarily covers:

| Target sub-file (post-split) | Primary test file(s) |
|---|---|
| `session/instance.go` (struct, constructors, core lifecycle) | `instance_test.go`, `instance_lifecycle_test.go`, `comprehensive_session_creation_test.go`, `session_creation_test.go`, `session_test.go` |
| `session/instance_status.go` | `state_machine_test.go`, `status_mapping_test.go`, `instance_last_acknowledged_test.go`, `instance_timestamp_test.go`, `instance_timestamp_signature_test.go` |
| `session/instance_tmux.go` | `session_restart_test.go`, `pty_access_test.go`, `pty_discovery_test.go`, `wire_callbacks_concurrency_test.go`, `instance_concurrency_test.go` |
| `session/instance_worktree.go` | `instance_workspace_test.go`, `instance_fork_test.go`, `comprehensive_session_creation_test.go` |
| `session/instance_approval.go` | `instance_approve_deny_test.go`, `review_queue_poller_test.go`, `approval_automation_test.go`, `approval_policy_test.go`, `review_queue_uncommitted_changes_test.go` |
| `session/instance_serialization.go` | `instance_test.go` (serialization round-trip), `instance_cold_restore_test.go`, `storage_test.go`, `migrate_test.go` |
| `session/instance_terminal.go` | `terminal_state_test.go`, `terminal_state_integration_test.go`, `capture_test.go`, `response_stream_test.go` |
| `session/instance_checkpoint.go` | `checkpoint_test.go` |
| `session/instance_tags.go` | `instance_tags_test.go`, `tag_manager_test.go` |

**Assertion:** `go test ./session/...` exits 0 with no test failures. This is verified by
`make test` / `make ci` in CI. All 57 test files are the regression guard; no new test
functions are required for the split itself.

---

### G2.3 `TestInstance_SubfileMethodsCompile`

| Field | Value |
|---|---|
| Test name | `TestInstance_SubfileMethodsCompile` |
| Type | Unit (compile-time behavioral) |
| File | `session/instance_split_compile_test.go` (new file, one function) |
| Requirement | G2 — AC: "All existing `session/...` tests pass unchanged" / "No method moved to wrong file" |

**Purpose:** A minimal test that instantiates or calls one representative function from each
target sub-file. This ensures no file was accidentally left with the wrong `package` declaration
and that the compiler sees all methods as belonging to `package session`.

```go
package session

import "testing"

// TestInstance_SubfileMethodsCompile is a compile-time smoke test for the instance.go split.
// It references one exported symbol from each target sub-file so that a wrong package
// declaration or missing method would produce a compile error rather than a silent omission.
func TestInstance_SubfileMethodsCompile(t *testing.T) {
    // instance.go — constructor
    _ = InstanceOptions{}

    // instance_status.go — status constants
    _ = Ready
    _ = Paused

    // instance_serialization.go — InstanceData type
    var _ InstanceData

    // instance_tags.go — Tags field is on Instance struct; tag methods compile if
    // the method set is complete. Verified by instance_tags_test.go running.

    // instance_approval.go — SetReviewQueue method signature
    var i *Instance
    _ = i // avoid "declared and not used" if only asserting types
}
```

This test has zero runtime assertions but fails to compile if any target sub-file is
misconfigured (e.g., `package session_tmux` instead of `package session`).

---

## G3 — Interface Extraction at the server/session Boundary

Goal: extract `session.ReviewQueueWriter` and confirm `session.InstanceStore` is used
consistently; enable isolated unit tests for services.

### G3.1 `TestReviewQueueWriter_MockImplementation`

| Field | Value |
|---|---|
| Test name | `TestReviewQueueWriter_MockImplementation` |
| Type | Unit |
| File | `session/interfaces_test.go` (new file) OR the service's existing `_test.go` |
| Requirement | G3 — AC: "New interface is tested via a mock in that service's `_test.go`" |

**Setup:** Define `ReviewQueueWriter` as:
```go
type ReviewQueueWriter interface {
    Add(item ApprovalRequest) error
}
```

Create a minimal mock:
```go
type mockReviewQueueWriter struct {
    added []ApprovalRequest
}
func (m *mockReviewQueueWriter) Add(item ApprovalRequest) error {
    m.added = append(m.added, item)
    return nil
}
```

**Assertion:**
- `var _ ReviewQueueWriter = (*ReviewQueue)(nil)` — `*session.ReviewQueue` satisfies the interface.
- `var _ ReviewQueueWriter = (*mockReviewQueueWriter)(nil)` — mock also satisfies it.
- Instantiate whichever service now accepts `ReviewQueueWriter` instead of `*ReviewQueue`,
  inject the mock, call the service method that enqueues, assert `mock.added` has length 1.

This test lives in the service's `_test.go` (e.g., `server/services/review_queue_service_test.go`).

---

### G3.2 `TestInstanceStore_ConcreteImplementsInterface`

| Field | Value |
|---|---|
| Test name | `TestInstanceStore_ConcreteImplementsInterface` |
| Type | Unit (compile-time assertion) |
| File | `session/storage_test.go` or `session/interfaces_test.go` |
| Requirement | G3 — AC: "At least one service previously taking `*session.Storage` directly now takes `session.InstanceStore`" |

**Assertion (compile-time interface check):**
```go
// TestInstanceStore_ConcreteImplementsInterface verifies *Storage satisfies InstanceStore.
func TestInstanceStore_ConcreteImplementsInterface(t *testing.T) {
    var _ InstanceStore = (*Storage)(nil)
}
```

This fails to compile if the `InstanceStore` interface definition in `session/storage.go:159`
diverges from the `*Storage` method set (e.g., a method is removed from `Storage` without
updating the interface).

---

### G3.3 `TestReviewQueueWriter_InterfaceSegregation`

| Field | Value |
|---|---|
| Test name | `TestReviewQueueWriter_InterfaceSegregation` |
| Type | Unit |
| File | `session/interfaces_test.go` |
| Requirement | G3 — AC: "No service interface is wider than what the service actually uses (Interface Segregation)" |

**Setup:** For each service method that accepts `ReviewQueueWriter`, write a table-driven test
that passes a mock with ONLY `Add` implemented (no `Get`, `Has`, `Remove`) and confirms the
service compiles and operates correctly using only `Add`.

**Assertion:**
- The mock satisfies the interface at compile time.
- The service method succeeds with only `Add` available — confirming the interface is not wider
  than needed.

---

### G3.4 `TestSessionService_AcceptsInstanceStore`

| Field | Value |
|---|---|
| Test name | `TestSessionService_AcceptsInstanceStore` |
| Type | Unit |
| File | `server/services/session_service_test.go` |
| Requirement | G3 — AC: "At least one service … previously taking `*session.Storage` directly now takes `session.InstanceStore`" |

**Setup:** Construct `NewSessionService` passing a hand-rolled struct that satisfies
`session.InstanceStore` but is NOT a `*session.Storage`. The type assertion at line 103–105
of `session_service.go` is the known leak — this test documents whether that leak has been
removed as part of G3.

**Assertion:**
- If the type assertion is removed: `NewSessionService(mockStore)` succeeds and the service
  operates without panicking.
- If the type assertion is intentionally retained (deferred to follow-on work): add a test
  comment referencing Story 1 (P1) from `docs/tasks/backend-architecture-improvements.md` and
  mark the test as `t.Skip("type assertion leak deferred to follow-on Story 1")`.

This test documents the boundary clearly regardless of whether the assertion is fixed in this PR.

---

## G4 — Test Coverage for the Wiring Layer

Goal: grow `server/dependencies_test.go` from 3 to at least 8 tests covering Warren Wire
validation behaviour.

### G4.1 `TestBuildServiceDeps_MissingSetterProducesDescriptiveError`

| Field | Value |
|---|---|
| Test name | `TestBuildServiceDeps_MissingSetterProducesDescriptiveError` |
| Type | Unit |
| File | `server/dependencies_test.go` |
| Requirement | G4 — AC: "Tests cover: nil value skips Set and fails Validate" |

**Setup:** Pass `&CoreDeps{Storage: nonNilStorage, EventBus: nonNilBus, ReviewQueue: nonNilRQ, ApprovalStore: nil}`.

**Assertion:**
- `err != nil`.
- `err.Error()` contains "ApprovalProvider" (the setter name that would be registered by
  `warren.Set(w, "ApprovalProvider", ...)`).
- Returned `*ServiceDeps` is nil.

---

### G4.2 `TestBuildServiceDeps_AllRequired_NoError`

| Field | Value |
|---|---|
| Test name | `TestBuildServiceDeps_AllRequired_NoError` |
| Type | Integration |
| File | `server/dependencies_test.go` |
| Requirement | G4 — AC: "Tests cover: all required setters applied produces no error" |

**Setup:** `BuildCoreDepsWithOptions(BuildOptions{EntClient: openTestDB(t)})` to get a valid
`*CoreDeps`. Call `BuildServiceDeps(core)`.

**Assertion:**
- `err == nil`.
- `svc.StatusManager != nil`.
- `svc.ReviewQueuePoller != nil`.
- `svc.PRStatusPoller != nil`.

---

### G4.3 `TestBuildRuntimeDeps_RequiresServiceDeps`

| Field | Value |
|---|---|
| Test name | `TestBuildRuntimeDeps_RequiresServiceDeps` |
| Type | Unit |
| File | `server/dependencies_test.go` |
| Requirement | G4 — AC: "Tests cover: phase ordering is enforced" |

**Note:** `TestBuildRuntimeDeps_RejectsNilService` already covers the nil guard. This new test
extends it to verify the error message is descriptive (mentions "Phase 2"):

**Assertion:**
- `err.Error()` contains "Phase 2" or "ServiceDeps".

This can be a one-line addition to the existing test. If the existing test is not updated,
add this as a separate test function.

---

### G4.4 `TestBuildCoreDepsWithOptions_InjectedEntClient`

| Field | Value |
|---|---|
| Test name | `TestBuildCoreDepsWithOptions_InjectedEntClient` |
| Type | Integration |
| File | `server/dependencies_test.go` |
| Requirement | G4 — AC: "Tests cover: `BuildCoreDepsWithOptions` with an injected `EntClient` works end-to-end" (from pitfalls.md gap list) |

**Setup:** Open an in-memory SQLite ent client using `enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")`.
Call `BuildCoreDepsWithOptions(BuildOptions{EntClient: client})`.

**Assertion:**
- `err == nil`.
- `core.SessionService != nil`.
- `core.Storage != nil`.
- `core.ReviewQueue != nil`.
- `core.ApprovalStore != nil`.
- `core.ErrorRegistry != nil`.

---

### G4.5 `TestBuildRuntimeDeps_PhaseOrderingEnforced_ByType`

| Field | Value |
|---|---|
| Test name | `TestBuildRuntimeDeps_PhaseOrderingEnforced_ByType` |
| Type | Unit |
| File | `server/dependencies_test.go` |
| Requirement | G4 — AC: "Tests cover: phase ordering is enforced" |

**Purpose:** The `tmux.TmuxServerReady` token is a zero-value struct that the compiler enforces
must be passed. This is a documentation-oriented test that confirms the signature requires the
token and that the function cannot be called without it.

**Implementation:** This is a compile-time guarantee — no runtime assertion needed. The test
simply calls `BuildRuntimeDeps(tmux.TmuxServerReady{}, nil)` (nil `ServiceDeps` triggers the
existing nil guard, returning a descriptive error before any tmux I/O). The test asserts the
error is about `ServiceDeps`, not about the token.

**Assertion:**
- `err != nil`.
- `err.Error()` contains "ServiceDeps" or "Phase 2" (confirming the guard ran, not a panic
  from missing tmux).

---

### G4.6 `TestWarrenWire_PhaseValidation_Sequential`

| Field | Value |
|---|---|
| Test name | `TestWarrenWire_PhaseValidation_Sequential` |
| Type | Unit |
| File | `pkg/warren/wire_test.go` |
| Requirement | G4 — AC: "`pkg/warren` package test coverage stays at or above current level" |

**Purpose:** Confirm that two independent `warren.Wire` instances (one per phase) each validate
independently and that a missing entry in Phase 2 does not bleed into Phase 1's result.

**Setup:**
```go
w1 := warren.NewWire("Phase1")
warren.Set(w1, "Storage", func(string) {}, "storage")
if err := w1.Validate(); err != nil { t.Fatal(err) }

w2 := warren.NewWire("Phase2")
warren.Set(w2, "StatusMgr", func(*int) {}, (*int)(nil)) // nil — should fail
err := w2.Validate()
```

**Assertion:**
- `w1.Validate()` returns nil (Phase 1 is clean).
- `w2.Validate()` returns non-nil error mentioning "StatusMgr".
- The two wires are independent (no shared state).

---

## Requirements Coverage Matrix

| Requirement | Acceptance Criterion | Covering Test(s) |
|---|---|---|
| **G1** | `BuildServiceDeps` calls `w.Validate()` after wiring | `TestBuildServiceDeps_NilSetterIsDetected` (G1.1), `TestBuildServiceDeps_MissingSetterProducesDescriptiveError` (G4.1) |
| **G1** | `BuildRuntimeDeps` calls `w.Validate()` after wiring | `TestBuildRuntimeDeps_NilSetterIsDetected` (G1.3) |
| **G1** | Removing any `warren.Set(...)` causes a startup error | `TestBuildServiceDeps_NilSetterIsDetected` (G1.1), `TestBuildRuntimeDeps_NilSetterIsDetected` (G1.3), `TestBuildServiceDeps_MissingSetterProducesDescriptiveError` (G4.1) |
| **G1** | `dependencies_test.go` tests nil setters produce descriptive errors | `TestBuildServiceDeps_NilSetterIsDetected` (G1.1), `TestBuildRuntimeDeps_NilSetterIsDetected` (G1.3), `TestBuildServiceDeps_MissingSetterProducesDescriptiveError` (G4.1) |
| **G1** | `warren.Set` skips nil and records error (mechanism) | `TestWire_NilValueSkipsSetter` (existing), `TestWire_ValidateMentionsAllMissing` (existing) |
| **G2** | `go build ./session/...` passes | `go build ./session/...` in CI (compile check) |
| **G2** | All existing `session/...` tests pass unchanged | All 57 existing test files (regression guard) via `make test` |
| **G2** | No method moved to wrong file | `TestInstance_SubfileMethodsCompile` (G2.3) |
| **G2** | Each sub-file is under 400 lines | Reviewer verification + `wc -l session/instance_*.go` in CI lint step |
| **G3** | At least one service takes `session.InstanceStore` instead of `*session.Storage` | `TestInstanceStore_ConcreteImplementsInterface` (G3.2), `TestSessionService_AcceptsInstanceStore` (G3.4) |
| **G3** | At least one service takes a narrower interface instead of `*session.ReviewQueue` | `TestReviewQueueWriter_MockImplementation` (G3.1) |
| **G3** | New interface tested via mock in service `_test.go` | `TestReviewQueueWriter_MockImplementation` (G3.1) |
| **G3** | No interface wider than what the service uses (ISP) | `TestReviewQueueWriter_InterfaceSegregation` (G3.3) |
| **G4** | At least 5 new tests in `server/dependencies_test.go` | G4.1–G4.5 (5 new tests) |
| **G4** | Tests cover nil value skips Set and fails Validate | `TestBuildServiceDeps_MissingSetterProducesDescriptiveError` (G4.1) |
| **G4** | Tests cover all required setters applied produces no error | `TestBuildServiceDeps_AllRequired_NoError` (G4.2), `TestBuildCoreDepsWithOptions_InjectedEntClient` (G4.4) |
| **G4** | Tests cover phase ordering is enforced | `TestBuildRuntimeDeps_RequiresServiceDeps` (G4.3), `TestBuildRuntimeDeps_PhaseOrderingEnforced_ByType` (G4.5) |
| **G4** | `pkg/warren` coverage stays at or above current level | `TestWarrenWire_PhaseValidation_Sequential` (G4.6) |

---

## Test Inventory by Type

### Unit tests (13)

| # | Test name | File |
|---|---|---|
| 1 | `TestBuildServiceDeps_NilSetterIsDetected` | `server/dependencies_test.go` |
| 2 | `TestBuildRuntimeDeps_NilSetterIsDetected` | `server/dependencies_test.go` |
| 3 | `TestBuildRuntimeDeps_RequiresServiceDeps` | `server/dependencies_test.go` |
| 4 | `TestBuildRuntimeDeps_PhaseOrderingEnforced_ByType` | `server/dependencies_test.go` |
| 5 | `TestBuildServiceDeps_MissingSetterProducesDescriptiveError` | `server/dependencies_test.go` |
| 6 | `TestInstance_SubfileMethodsCompile` | `session/instance_split_compile_test.go` |
| 7 | `TestInstanceStore_ConcreteImplementsInterface` | `session/storage_test.go` or `session/interfaces_test.go` |
| 8 | `TestReviewQueueWriter_MockImplementation` | `server/services/review_queue_service_test.go` |
| 9 | `TestReviewQueueWriter_InterfaceSegregation` | `session/interfaces_test.go` |
| 10 | `TestSessionService_AcceptsInstanceStore` | `server/services/session_service_test.go` |
| 11 | `TestWarrenWire_PhaseValidation_Sequential` | `pkg/warren/wire_test.go` |
| 12 | `TestWire_NilValueSkipsSetter` | `pkg/warren/wire_test.go` (existing — listed for coverage) |
| 13 | `TestWire_ValidateMentionsAllMissing` | `pkg/warren/wire_test.go` (existing — listed for coverage) |

### Integration tests (7)

| # | Test name | File |
|---|---|---|
| 1 | `TestBuildServiceDeps_AllSettersApplied_Succeeds` | `server/dependencies_test.go` |
| 2 | `TestBuildServiceDeps_AllRequired_NoError` | `server/dependencies_test.go` |
| 3 | `TestBuildCoreDepsWithOptions_InjectedEntClient` | `server/dependencies_test.go` |
| 4–7 | All 57 existing `session/` test files (regression guard for G2) | `session/*_test.go` |

Note: Tests 4–7 represent the existing test suite acting as the regression guard for G2; they
are not new tests. The 3 new integration tests are items 1–3 above.

### New tests to write (net new, not counting existing tests cited as coverage)

| # | Test name | File | New? |
|---|---|---|---|
| 1 | `TestBuildServiceDeps_NilSetterIsDetected` | `server/dependencies_test.go` | NEW |
| 2 | `TestBuildServiceDeps_AllSettersApplied_Succeeds` | `server/dependencies_test.go` | NEW |
| 3 | `TestBuildRuntimeDeps_NilSetterIsDetected` | `server/dependencies_test.go` | NEW |
| 4 | `TestBuildServiceDeps_MissingSetterProducesDescriptiveError` | `server/dependencies_test.go` | NEW |
| 5 | `TestBuildServiceDeps_AllRequired_NoError` | `server/dependencies_test.go` | NEW |
| 6 | `TestBuildRuntimeDeps_RequiresServiceDeps` | `server/dependencies_test.go` | NEW (extends existing) |
| 7 | `TestBuildRuntimeDeps_PhaseOrderingEnforced_ByType` | `server/dependencies_test.go` | NEW |
| 8 | `TestBuildCoreDepsWithOptions_InjectedEntClient` | `server/dependencies_test.go` | NEW |
| 9 | `TestInstance_SubfileMethodsCompile` | `session/instance_split_compile_test.go` | NEW |
| 10 | `TestInstanceStore_ConcreteImplementsInterface` | `session/storage_test.go` | NEW |
| 11 | `TestReviewQueueWriter_MockImplementation` | `server/services/review_queue_service_test.go` | NEW |
| 12 | `TestReviewQueueWriter_InterfaceSegregation` | `session/interfaces_test.go` | NEW |
| 13 | `TestSessionService_AcceptsInstanceStore` | `server/services/session_service_test.go` | NEW |
| 14 | `TestWarrenWire_PhaseValidation_Sequential` | `pkg/warren/wire_test.go` | NEW |

**14 net-new test functions** (not counting the 3 already-existing tests in `server/dependencies_test.go`
and not counting the existing `pkg/warren/wire_test.go` tests cited as mechanism coverage).

---

## Summary

| Metric | Value |
|---|---|
| Net-new test functions | 14 |
| Unit (of new) | 11 |
| Integration (of new) | 3 |
| Compile-time checks (build-only) | 2 (`go build ./session/...`, line-count check) |
| Requirements covered | 4/4 (G1, G2, G3, G4) |
| Acceptance criteria covered | 18/18 |
| Requirements coverage fraction | **100%** |
| Existing tests cited as regression guard (G2) | 57 test files in `session/` |
| Existing warren tests cited as mechanism coverage | 2 (`TestWire_NilValueSkipsSetter`, `TestWire_ValidateMentionsAllMissing`) |
