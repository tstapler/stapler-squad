# Backend Architecture Improvements

**Source review score**: 7.5 / 10
**Review date**: 2026-04-20
**Updated**: 2026-04-22 (Stories 6-7 added; ADRs 004-008 created; Story 8 added after tmux-leakage audit)
**Module**: `github.com/tstapler/stapler-squad`

---

## Epic Overview

### User Value

Engineers maintaining and extending stapler-squad gain:

- **Safer refactoring**: Narrow interfaces and compile-time assertions make breaking changes visible at build time rather than at runtime or in production logs.
- **Honest test coverage**: Tests that inject fake `InstanceStore` values currently silently bypass rules and analytics code paths due to the type assertion in `NewSessionService`. After Story 1, fakes exercise the same code paths as production.
- **Fewer runtime bugs**: The Title-keyed maps in `ReviewQueuePoller` (Story 6) are a live, silent cache-corruption bug. Fixing UUID keying eliminates an entire class of hard-to-reproduce issues.
- **Faster unit tests**: `ReviewQueuePoller` currently requires a real SQLite database for any test. After Stories 6-7, it accepts interface fakes and runs without a database.
- **Readable, maintainable code**: `BuildRuntimeDeps` (190 lines), `StreamTerminal` (294 lines), and `CreateSession` (165 lines) are replaced by clearly named, single-purpose functions that a new contributor can understand without reading the full file.

### Success Metrics

| Metric | Current state | Target after all stories |
|--------|--------------|--------------------------|
| Type assertions on `*session.Storage` outside `session/` | 2 (`session_service.go:103`, `GetStorage():232`) | 0 |
| Methods on `Repository` interface | 21 | 4 sub-interfaces of 3-5 methods each |
| Lines in `BuildRuntimeDeps` | ~190 | <30 (orchestrator only) |
| Lines in `ReviewQueuePoller` | to be measured | Reduced by `TerminalContentCache` + `TmuxReconciler` extraction |
| ReviewQueuePoller tests requiring SQLite | All | 0 (interface fakes only) |
| `ReviewQueuePoller` map key type | `Instance.Title` (mutable) | `Instance.UUID` (stable) |
| Independent `LastActivity` timestamps | 4 (ClaudeController, poller, ReviewState, ReviewItem) | 1 authoritative source |

### Scope

**In scope**:
- Stories 1-7 as described in this document
- All files listed in each story's "Files in scope" section
- New files: `session/errors.go`, `session/terminal_content_cache.go`, `session/tmux_reconciler.go`, `session/storage_interfaces.go` (if needed)

**Out of scope** (explicitly excluded per 2026-04-22 review):
- `session/vc/` abstraction (git / jj providers) — stable, no issues identified
- `executor/` circuit-breaker wrappers — must not be inlined or removed
- Proto schema changes — no breaking API changes to gRPC/ConnectRPC endpoints
- TUI (`app/`) — out of scope for backend architecture review
- Database migration tooling (`session/ent/migrate/`) — ent-generated, do not hand-edit
- `PRStatusPoller` internal logic — only the shape change in Story 4 (field grouping) touches it

### Constraints

- Go 1.21+ only; no generics-based solutions that require a newer toolchain
- Zero behaviour changes — all refactors are structural; observable behaviour (API responses, terminal streaming, queue membership decisions) must not change
- No breaking changes to public gRPC/ConnectRPC API (`proto/session/v1/`)
- All existing tests must pass after each task — no accumulating broken state across tasks
- Ent-generated files (`session/ent/`) must not be hand-edited; regenerate with `make generate-proto` if schema changes are needed
- Linting must pass after each story (`make lint`); the build pipeline enforces this

---

## Architecture Decisions

The following ADRs capture the key design choices embedded in this plan. Read the relevant ADR before implementing the corresponding story.

- [ADR-004](../../project_plans/stapler-squad/decisions/ADR-004-interface-at-boundary.md): Narrow interfaces defined in domain package (`session/`) at the boundary, not in the consuming layer
- [ADR-005](../../project_plans/stapler-squad/decisions/ADR-005-uuid-as-session-identity.md): UUID is the canonical session key; Title is display metadata and must not be used as a map key
- [ADR-006](../../project_plans/stapler-squad/decisions/ADR-006-composed-constructor-interface.md): `SessionServiceStore` composed interface pattern — single injection point for `NewSessionService`
- [ADR-007](../../project_plans/stapler-squad/decisions/ADR-007-terminal-content-cache-placement.md): `TerminalContentCache` belongs in `session/` layer, not in polling infrastructure
- [ADR-008](../../project_plans/stapler-squad/decisions/ADR-008-lastactivity-single-publisher.md): `ClaudeController` is the authoritative `LastActivity` publisher; all other components derive from it
- [ADR-009](../../project_plans/stapler-squad/decisions/ADR-009-terminal-session-interface.md): `TerminalSession` interface — transport-agnostic terminal abstraction; tmux concepts must not appear in `server/services/`

---

## Implementation Order

Stories are ordered by the risk they carry if deferred. P0 must be done first; P2 stories are safe to batch or defer.

| # | Story | Priority | Risk if deferred | Recommended sprint |
|---|-------|----------|------------------|--------------------|
| 6.1 | UUID keying in `ReviewQueuePoller` | P0 | Silent cache corruption on session rename or title collision | Sprint 1 |
| 6.2 | `InstanceReader` interface; inject into poller | P1 | Poller untestable without SQLite; couples server to domain impl | Sprint 1 |
| 6.3 | Domain error types (`ErrSessionNotFound`) | P1 | String-matched errors; fragile caller logic | Sprint 1 |
| 1 | Fix `InstanceStore` abstraction leak | P1 | Fake stores silently bypass rules and analytics in tests | Sprint 1 |
| 7 | Decompose `ReviewQueuePoller` | P1 | Poller grows more complex with each new feature; panic-prone | Sprint 2 |
| 2 | Refactor `BuildRuntimeDeps` into `RuntimeWirer` | P2 | Ordering bugs compound as more steps are added | Sprint 2 |
| 3 | Split `Repository` interface | P2 | Every mock stubs 21 methods; dual-model confusion | Sprint 2 |
| 5 | Split mega-functions (`StreamTerminal`, `CreateSession`) | P2 | Large functions resist unit testing | Sprint 2 |
| 8 | Transport-agnostic `TerminalSession` interface | P2 | Server layer hardwired to tmux; can never swap backend; lifecycle bugs like the session-restore race (Apr 2026) compound | Sprint 2 |
| 4 | Extract `GitHubPRStatus` value object | P2 | JSON schema migration risk; do last, with careful review | Sprint 3 |

---

## Overview

This plan organises the architecture findings from the recent backend review into five independent stories that can be worked sequentially or in parallel by different developers. Each story targets a specific structural problem, and its tasks are sized to touch at most 3-5 files and fit within a single focused work session.

Stories are ordered by the risk reduction they deliver:

| # | Story | Priority | Risk if deferred |
|---|-------|----------|-----------------|
| 1 | Fix `InstanceStore` abstraction leak | P1 | Silent loss of search/rules/analytics in tests; casting couples concrete types |
| 2 | Refactor `BuildRuntimeDeps` | P1 | Ordering bugs compound as more steps are added |
| 3 | Split `Repository` interface | P2 | Every mock must stub 21 methods; dual API causes confusion |
| 8 | Transport-agnostic `TerminalSession` interface | P2 | 42 tmux leakage points in server layer; lifecycle bugs caused by caller managing what instance should own; backend swap impossible |
| 4 | Extract `GitHubPRStatus` value object | P2 | 15 PR fields scattered across Instance lifecycle |
| 5 | Split mega-functions | P2 | `StreamTerminal` (294 lines) and `CreateSession` (165 lines) resist unit testing |

---

## Strengths to Preserve

The following patterns must not be disturbed during refactoring:

- **Three-phase initialisation** (`BuildCoreDeps → BuildServiceDeps → BuildRuntimeDeps`) in `server/dependencies.go`. The phase boundary comments and `nil`-guard at `BuildRuntimeDeps:368` enforce ordering — preserve them.
- **`InstanceContext` interface** in `session/claude_controller.go:16` — the pattern of a narrow interface breaking a bidirectional dependency is exactly what Story 1 replicates.
- **Compile-time interface assertions** (`var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)`) — add parallel assertions for every new interface introduced.
- **`executor/` circuit-breaker wrappers** — do not inline or remove.
- **Graceful degradation on `concStorage == nil`** — the nil-guard strategy at `session_service.go:103-106` is intentional; Stories 1-3 formalise it rather than remove it.
- **`session/vc/` abstraction** for git / jj providers — out of scope; do not touch.

---

## Story 1 — Fix `InstanceStore` Abstraction Leak

**Addresses**: P1 finding #1, P3 finding #8  
**Files in scope**: `server/services/session_service.go`, `server/services/rules_service.go`, `server/services/review_queue_service.go`, `session/storage.go`

### Problem

`NewSessionService` accepts a `session.InstanceStore` interface but immediately performs a concrete type assertion (`storage.(*session.Storage)`) at line 103-105 to obtain `concStorage`. Three sub-services (`ReviewQueueService`, `RulesService`, `AnalyticsStore`) were then built against `*session.Storage` rather than against narrow interfaces. Any test that injects a fake `InstanceStore` silently loses search, rules, and analytics.

`GetStorage()` at line 232-237 repeats the same type assertion and returns the concrete type to callers in `server/`, further coupling the server layer to the storage implementation.

The `concStorage` variable name also carries no semantic signal about why the assertion is necessary.

### Acceptance Criteria

- `NewSessionService` contains zero type assertions on its `storage` parameter.
- `GetStorage()` is removed or returns `session.InstanceStore`.
- `RulesStore`, `AnalyticsStore`, and `ReviewQueueService` accept narrow interfaces instead of `*session.Storage`.
- A fake `InstanceStore` used in tests exercises rules and analytics code paths (no silent no-op).
- Compile-time interface assertions added for each new interface.
- All existing tests pass without modification.

### INVEST

- **Independent**: No other story depends on this completing first (Story 3 may be done concurrently).
- **Negotiable**: The exact interface names and method sets can be adjusted; the requirement is removal of the type assertion.
- **Valuable**: Fake stores in tests will actually exercise sub-service code paths.
- **Estimable**: 3 narrow interfaces × ~3 methods each = well-scoped.
- **Small**: 4 files, incremental — each task can compile and pass tests independently.
- **Testable**: Existing unit tests will fail if assertion is replaced incorrectly.

---

#### Task 1.1 — Define `RulesStorage` and `AnalyticsStorage` interfaces

**Files**: `session/storage.go` (or new `session/storage_interfaces.go`)  
**Effort**: ~2 hours

**Steps**:

1. Open `server/services/rules_service.go` and note every method called on the `*session.Storage` argument passed to `NewRulesStore` and `NewAnalyticsStore`. As of the review, these are repository-level read/write operations for approval rules and analytics records that delegate to `Repository`.

2. In `session/storage.go` (near the existing `InstanceStore` definition), declare two interfaces:

   ```go
   // RulesStorage is the narrow interface required by RulesStore.
   // *Storage satisfies this interface.
   type RulesStorage interface {
       // methods derived in step 1
   }

   // AnalyticsStorage is the narrow interface required by AnalyticsStore.
   // *Storage satisfies this interface.
   type AnalyticsStorage interface {
       // methods derived in step 1
   }
   ```

3. Add compile-time assertions in the same file:

   ```go
   var _ RulesStorage    = (*Storage)(nil)
   var _ AnalyticsStorage = (*Storage)(nil)
   ```

4. Run `go build ./...` — must compile clean.

**Known risk**: If `*Storage` delegates directly to `Repository` for these methods, the interface will need to mirror `Repository` method signatures exactly. Verify with `grep -n "storage\." server/services/rules_service.go` before defining signatures.

---

#### Task 1.2 — Define `ReviewQueueStorage` interface; migrate `ReviewQueueService`

**Files**: `session/storage.go`, `server/services/review_queue_service.go`  
**Effort**: ~2 hours  
**Depends on**: Task 1.1 complete (for consistency of placement)

**Steps**:

1. Identify methods on `*session.Storage` consumed by `ReviewQueueService`. The service receives `concStorage` at `session_service.go:134`.

2. Add `ReviewQueueStorage` interface in `session/storage.go` following the same pattern as Task 1.1.

3. Change `ReviewQueueService`'s storage field type from `*session.Storage` to `session.ReviewQueueStorage`. Update the constructor signature.

4. In `NewSessionService` (`session_service.go:97`), change the `concStorage` variable to type `session.ReviewQueueStorage`. This is still obtained from the type assertion at this point — Task 1.3 removes the assertion entirely.

5. Add compile-time assertion: `var _ ReviewQueueStorage = (*Storage)(nil)`.

6. Run `go test ./server/services/...` — must pass.

---

#### Task 1.3 — Remove the type assertion from `NewSessionService`; remove `GetStorage()`

**Files**: `server/services/session_service.go`  
**Effort**: ~3 hours  
**Depends on**: Tasks 1.1 and 1.2

**Steps**:

1. Change `NewSessionService` signature to accept three storage parameters — or extend `InstanceStore` into a composed interface. The recommended approach is a composed constructor interface:

   ```go
   // SessionServiceStore is the storage interface required by SessionService.
   // Production code passes *session.Storage. Tests pass fakes.
   type SessionServiceStore interface {
       session.InstanceStore
       session.RulesStorage
       session.AnalyticsStorage
       session.ReviewQueueStorage
   }
   ```

   Place this type in `server/services/session_service.go` (it is a service-layer concern).

2. Change `NewSessionService(storage session.InstanceStore, ...)` to `NewSessionService(storage SessionServiceStore, ...)`.

3. Delete the type assertion block at lines 103-106 (the `concStorage` variable). Pass `storage` directly to `NewRulesStore`, `NewAnalyticsStore`, and `NewReviewQueueService` — they now accept their respective narrow interfaces, which `SessionServiceStore` satisfies.

4. Delete `GetStorage()` at line 232-237. Grep for all callers of `GetStorage()` in `server/` and update them to use `GetInstanceStore()` or pass the store through a constructor argument.

5. Update all test files that call `NewSessionService` — they now must pass a value satisfying `SessionServiceStore`. If existing fakes only implement `InstanceStore`, they will need stub implementations of the three narrow interfaces (all methods can return zero values / `nil, nil`).

6. Run `go build ./...` and `go test ./...`.

**Known risk**: `GetStorage()` may be called from `server/server.go` or from `server/dependencies.go` to wire sub-systems. Audit before deleting:

```
grep -rn "GetStorage()" server/
```

Each caller that truly needs `*session.Storage` should receive it as a constructor argument instead of reaching through `SessionService`.

---

## Story 2 — Refactor `BuildRuntimeDeps` into `RuntimeWirer`

**Addresses**: P1 finding #2  
**Files in scope**: `server/dependencies.go`

### Problem

`BuildRuntimeDeps` is a 190-line procedural function (`dependencies.go:366-556`) with twelve numbered comments acting as makeshift sub-routine headers. Infrastructure wiring (filesystem paths, goroutine starts) is interleaved with domain wiring (injecting `ReviewQueue` and `StatusManager` into instances). Adding a new wirable component requires reading the entire function to find the right insertion point, and an ordering mistake silently produces a nil dependency.

### Acceptance Criteria

- `BuildRuntimeDeps` is reduced to a thin orchestrator that calls discrete methods on a `RuntimeWirer` struct.
- Each method is independently testable (can be called with a partially wired `RuntimeWirer`).
- The three-phase boundary (`BuildCoreDeps → BuildServiceDeps → BuildRuntimeDeps`) is preserved.
- No behaviour changes — all goroutine start logic, callbacks, and wiring order are preserved.
- All existing tests pass.

### INVEST

- **Independent**: Does not depend on any other story.
- **Negotiable**: The struct name and method granularity are adjustable.
- **Valuable**: Reduces the risk of ordering bugs and makes each wiring step independently testable.
- **Estimable**: Mechanical extraction; no logic changes.
- **Small**: Single file, no interface changes.
- **Testable**: Each extracted method can be unit-tested by constructing a `RuntimeWirer` with mock dependencies.

---

#### Task 2.1 — Extract instance-wiring into `wireInstances()`

**Files**: `server/dependencies.go`  
**Effort**: ~2 hours

**Steps**:

1. Create a `RuntimeWirer` struct at the top of the `BuildRuntimeDeps` section:

   ```go
   type RuntimeWirer struct {
       svc *ServiceDeps
   }

   func newRuntimeWirer(svc *ServiceDeps) *RuntimeWirer {
       return &RuntimeWirer{svc: svc}
   }
   ```

2. Extract Steps 5 and 5-continued (lines 379-422) into a method:

   ```go
   func (w *RuntimeWirer) wireInstances() ([]*session.Instance, error) {
       // load from storage, set ReviewQueue, set StatusManager, set poller instances
   }
   ```

3. The method returns the loaded `[]*session.Instance` slice that subsequent methods need.

4. In `BuildRuntimeDeps`, replace the extracted block with `instances, err := wirer.wireInstances()`.

5. `go build ./...` must pass after each extraction step — do not accumulate all extractions before compiling.

---

#### Task 2.2 — Extract goroutine startup into `startControllers()`

**Files**: `server/dependencies.go`  
**Effort**: ~2 hours  
**Depends on**: Task 2.1

**Steps**:

1. Extract the `go func()` block at lines 398-445 (Steps 6, 6.5, 7, 7.5) into:

   ```go
   func (w *RuntimeWirer) startControllers(instances []*session.Instance) {
       go func() {
           // staggered tmux start, persist migration, start controllers, startup scan
       }()
   }
   ```

2. The goroutine is launched inside the method — the method itself returns immediately, preserving the existing non-blocking semantics.

3. Replace the original goroutine block in `BuildRuntimeDeps` with `wirer.startControllers(instances)`.

4. `go build ./...` must pass.

---

#### Task 2.3 — Extract external-discovery wiring into `startExternalDiscovery()`

**Files**: `server/dependencies.go`  
**Effort**: ~2 hours  
**Depends on**: Task 2.2

**Steps**:

1. Extract Steps 11 and 12 (lines 471-543) into:

   ```go
   func (w *RuntimeWirer) startExternalDiscovery(instances []*session.Instance) *session.ExternalSessionDiscovery {
       // OnSessionAdded / OnSessionRemoved callbacks
       // ExternalApprovalMonitor
       // SetExternalDiscovery on SessionService
       return externalDiscovery
   }
   ```

2. Extract Steps 8-10 (ReactiveQueueManager, HistoryLinker, ScrollbackManager, TmuxStreamerManager) into:

   ```go
   func (w *RuntimeWirer) startManagers(instances []*session.Instance) managerBundle {
       // Steps 8, 8.5, 9, 10
   }
   ```

   where `managerBundle` is a small unexported struct holding the four manager references.

3. Reduce `BuildRuntimeDeps` to:

   ```go
   func BuildRuntimeDeps(svc *ServiceDeps) (*RuntimeDeps, error) {
       if svc == nil {
           return nil, fmt.Errorf("BuildRuntimeDeps: ServiceDeps is nil (Phase 2 not completed)")
       }
       wirer := newRuntimeWirer(svc)
       instances, err := wirer.wireInstances()
       if err != nil { return nil, err }
       wirer.startControllers(instances)
       mgrs := wirer.startManagers(instances)
       extDisc := wirer.startExternalDiscovery(instances)
       return &RuntimeDeps{ ... }, nil
   }
   ```

4. Run `go test ./...` — full suite must pass.

---

## Story 3 — Split the `Repository` Interface

**Addresses**: P2 finding #3, P3 finding #7  
**Files in scope**: `session/repository.go`, `session/ent_repository.go`, `session/storage.go`

### Problem

`Repository` (defined in `session/repository.go:11-90`) has 21 methods spanning three distinct concerns:

- Session CRUD via `InstanceData` (8 methods: Create, Update, Delete, Get, GetWithOptions, List, ListWithOptions, ListByStatus, ListByStatusWithOptions, ListByTag, ListByTagWithOptions, UpdateTimestamps, Close — 13 including `WithOptions` variants)
- Session CRUD via the newer `Session` domain model (4 methods: GetSession, ListSessions, CreateSession, UpdateSession)
- Rules management (3 methods: AllRules, UpsertRule, DeleteRule)
- Analytics (2 methods: RecordAnalytics, ListAnalytics)

Any mock must stub all 21 methods, and the in-flight dual-model migration (finding #7) adds confusion about which API to use for new code.

### Acceptance Criteria

- `Repository` is split into at least `SessionRepository` (InstanceData CRUD), `SessionDomainRepository` (Session domain model CRUD), `RulesRepository` (rules management), and `AnalyticsRepository` (analytics).
- An `EntRepository` composed interface re-assembles all four for production use (the SQLite implementation satisfies all four).
- The `Storage` wrapper is updated to delegate through the correct sub-interface.
- A deprecation comment marks all `InstanceData`-based methods on `SessionRepository`, with a migration deadline comment.
- All existing callers compile without change (they continue to accept the composed interface).

### INVEST

- **Independent**: Can be done concurrently with Story 1.
- **Negotiable**: Interface naming is flexible; the constraint is separation of concerns.
- **Valuable**: Mocks for rules testing no longer need session CRUD stubs.
- **Estimable**: Interface extraction only; `EntRepository` struct already implements all methods.
- **Small**: 3 files touched, no logic changes.
- **Testable**: Adding a mock that implements only `RulesRepository` and compiles proves the split.

---

#### Task 3.1 — Define narrow sub-interfaces and composed `EntRepository` interface

**Files**: `session/repository.go`  
**Effort**: ~2 hours

**Steps**:

1. Below the current `Repository` interface, define:

   ```go
   // SessionRepository handles InstanceData persistence.
   // Deprecated: prefer SessionDomainRepository for new code. See migration note at top of file.
   type SessionRepository interface {
       Create(ctx context.Context, data InstanceData) error
       Update(ctx context.Context, data InstanceData) error
       Delete(ctx context.Context, title string) error
       Get(ctx context.Context, title string) (*InstanceData, error)
       GetWithOptions(ctx context.Context, title string, options LoadOptions) (*InstanceData, error)
       List(ctx context.Context) ([]InstanceData, error)
       ListWithOptions(ctx context.Context, options LoadOptions) ([]InstanceData, error)
       ListByStatus(ctx context.Context, status Status) ([]InstanceData, error)
       ListByStatusWithOptions(ctx context.Context, status Status, options LoadOptions) ([]InstanceData, error)
       ListByTag(ctx context.Context, tag string) ([]InstanceData, error)
       ListByTagWithOptions(ctx context.Context, tag string, options LoadOptions) ([]InstanceData, error)
       UpdateTimestamps(ctx context.Context, title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string) error
       Close() error
   }

   // SessionDomainRepository handles the newer Session domain model persistence.
   type SessionDomainRepository interface {
       GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error)
       ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error)
       CreateSession(ctx context.Context, session *Session) error
       UpdateSession(ctx context.Context, session *Session) error
   }

   // RulesRepository manages auto-approval rules.
   type RulesRepository interface {
       AllRules(ctx context.Context) ([]ApprovalRuleData, error)
       UpsertRule(ctx context.Context, rule ApprovalRuleData) error
       DeleteRule(ctx context.Context, id string) error
   }

   // AnalyticsRepository records and retrieves classification decisions.
   type AnalyticsRepository interface {
       RecordAnalytics(ctx context.Context, data AnalyticsData) error
       ListAnalytics(ctx context.Context, limit int) ([]AnalyticsData, error)
   }

   // FullRepository composes all four repository concerns and is the type
   // expected by Storage and EntRepository. Prefer injecting a narrow
   // sub-interface at each call site.
   type FullRepository interface {
       SessionRepository
       SessionDomainRepository
       RulesRepository
       AnalyticsRepository
   }
   ```

2. Add a file-level migration comment:

   ```go
   // Migration note (added 2026-04-20): new code should use SessionDomainRepository
   // methods (GetSession, ListSessions, CreateSession, UpdateSession). The InstanceData-
   // based methods on SessionRepository are deprecated and will be removed once all
   // callers have been migrated. Target: before the next major release.
   ```

3. **Do not change the existing `Repository` interface yet** — leave it as an alias or keep it unchanged so callers compile. The alias approach:

   ```go
   // Repository is kept for backward compatibility. Use FullRepository for new code.
   type Repository = FullRepository
   ```

   This is a non-breaking change; `= FullRepository` is a type alias.

4. `go build ./...` must pass.

---

#### Task 3.2 — Add compile-time assertions in `session/ent_repository.go`

**Files**: `session/ent_repository.go`  
**Effort**: ~1 hour  
**Depends on**: Task 3.1

**Steps**:

1. Near the top of `session/ent_repository.go`, add assertions:

   ```go
   var _ SessionRepository       = (*EntRepository)(nil)
   var _ SessionDomainRepository  = (*EntRepository)(nil)
   var _ RulesRepository          = (*EntRepository)(nil)
   var _ AnalyticsRepository      = (*EntRepository)(nil)
   var _ FullRepository           = (*EntRepository)(nil)
   ```

2. If any assertion fails, the corresponding method is missing from `EntRepository` — add a stub that returns `errors.New("not implemented")` and open a follow-up task.

3. `go build ./...` must pass.

---

#### Task 3.3 — Update `Storage` wrapper to use `FullRepository`; document dual-model timeline

**Files**: `session/storage.go`  
**Effort**: ~2 hours  
**Depends on**: Tasks 3.1, 3.2

**Steps**:

1. Find the field in `Storage` that holds the repository. Change its declared type from `Repository` (or the concrete `*EntRepository`) to `FullRepository`. This should be a no-op if `Repository` is now a type alias for `FullRepository`.

2. Add a comment block above the `InstanceData`-based method group in `Storage` indicating they are deprecated and delegate to `SessionRepository`.

3. Run `go test ./session/...` — all tests must pass.

---

## Story 4 — Extract `GitHubPRStatus` Value Object

**Addresses**: P2 finding #4  
**Files in scope**: `session/instance.go`, `session/storage.go` (InstanceData), `session/ent_repository.go`

### Problem

`Instance` carries 15 PR status fields at lines 159-176 (`GitHubPRState`, `GitHubPRIsDraft`, `GitHubPRPriority`, `GitHubApprovedCount`, `GitHubChangesReqCount`, `GitHubCheckConclusion`, `GitHubPRStatusTerminal`, `LastPRStatusCheck`, and their `InstanceData` counterparts at `storage.go:50-57`). These fields have a different lifecycle from session lifecycle — they are polled externally by `PRStatusPoller` and should not be mutated by session management code.

The same fields are duplicated in `InstanceData` (the persistence DTO), meaning any change to the PR status shape requires edits in at least three places.

### Acceptance Criteria

- A `GitHubPRStatus` struct (value object) encapsulates all seven PR status fields.
- `Instance` embeds or holds a single `GitHubPRStatus` field.
- `InstanceData` holds a single `GitHubPRStatus` field (for JSON persistence).
- `PRStatusPoller` reads and writes through typed accessors rather than scattered field assignments.
- No behaviour changes — polling, event publishing, and proto mapping continue to work.
- `go test ./...` passes.

### INVEST

- **Independent**: Can be done in parallel with Stories 1, 2, and 3.
- **Negotiable**: Embedding vs. named field is flexible; the requirement is a single type.
- **Valuable**: PR status shape changes now require one edit, not three.
- **Estimable**: Mechanical struct extraction.
- **Small**: 3 files, no logic changes.
- **Testable**: After extraction, `PRStatusPoller` assignment code becomes a single struct literal — misnamed fields cause compile errors.

---

#### Task 4.1 — Define `GitHubPRStatus` struct; update `Instance` and `InstanceData`

**Files**: `session/instance.go`, `session/storage.go`  
**Effort**: ~3 hours

**Steps**:

1. Define the value object in `session/instance.go` above the `Instance` struct (or in a new `session/github.go`):

   ```go
   // GitHubPRStatus holds the polled state of a GitHub pull request associated with this session.
   // Fields are populated and updated exclusively by PRStatusPoller.
   type GitHubPRStatus struct {
       State          string    `json:"github_pr_state,omitempty"`
       IsDraft        bool      `json:"github_pr_is_draft,omitempty"`
       Priority       string    `json:"github_pr_priority,omitempty"`
       ApprovedCount  int       `json:"github_approved_count,omitempty"`
       ChangesReqCount int      `json:"github_changes_req_count,omitempty"`
       CheckConclusion string   `json:"github_check_conclusion,omitempty"`
       StatusTerminal bool      `json:"github_pr_status_terminal,omitempty"`
       LastChecked    time.Time `json:"last_pr_status_check,omitempty"`
   }
   ```

2. In `Instance`, replace the seven scattered fields (lines 161-175) with:

   ```go
   // PRStatus holds polled GitHub PR state. Updated by PRStatusPoller.
   PRStatus GitHubPRStatus
   ```

3. In `InstanceData` in `session/storage.go`, replace the seven fields (lines 50-57) with:

   ```go
   PRStatus GitHubPRStatus `json:"pr_status,omitempty"`
   ```

   The JSON key changes from individual field keys to a nested `"pr_status"` object. This is a **backwards-incompatible JSON schema change**. See the migration note in the Known Issues section below.

4. Update `Instance.ToInstanceData()` and `Instance.FromInstanceData()` (or their equivalents) to read/write `PRStatus` as a struct.

5. `go build ./...` must pass after this step.

---

#### Task 4.2 — Update `PRStatusPoller` and all scattered field assignments

**Files**: `session/instance.go` (or wherever `PRStatusPoller` is implemented), callers in `server/`  
**Effort**: ~2 hours  
**Depends on**: Task 4.1

**Steps**:

1. Search for every assignment to the old fields:

   ```
   grep -rn "GitHubPRState\|GitHubPRIsDraft\|GitHubPRPriority\|GitHubApprovedCount\|GitHubChangesReqCount\|GitHubCheckConclusion\|GitHubPRStatusTerminal\|LastPRStatusCheck" session/ server/
   ```

2. For each assignment site in `PRStatusPoller`, replace individual field assignments with a single struct assignment:

   ```go
   inst.PRStatus = session.GitHubPRStatus{
       State:           apiResp.State,
       IsDraft:         apiResp.Draft,
       Priority:        derivedPriority,
       ApprovedCount:   approvedCount,
       ChangesReqCount: changesReqCount,
       CheckConclusion: checkConclusion,
       StatusTerminal:  isTerminal,
       LastChecked:     time.Now(),
   }
   ```

3. Update proto adapter (`adapters/` package) to read from `inst.PRStatus.State` etc.

4. Update event publishing in `BuildRuntimeDeps` at `dependencies.go:393`:

   ```go
   eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"pr_status"}))
   ```

5. Run `go test ./...` — must pass.

---

## Story 5 — Split Mega-Functions in `SessionService`

**Addresses**: P2 findings #5 and #6  
**Files in scope**: `server/services/session_service.go`

### Problem

Two functions dominate `session_service.go` and resist unit testing:

- `StreamTerminal` (lines 947-1238, ~294 lines): mixes three distinct responsibilities — instance lookup and validation (lines 947-999), scrollback replay, and live PTY streaming (lines 1000-1238, two goroutines plus flow-control state).
- `CreateSession` (lines 501-663, ~165 lines): mixes request validation, GitHub URL resolution, config-default resolution, instance construction, tmux start, storage persistence, and poller update.

Neither function can be tested without standing up a full `SessionService` and a running tmux session.

### Acceptance Criteria

- `StreamTerminal` delegates to at least two extracted functions: one for instance lookup/validation and one for the streaming loop.
- `CreateSession` delegates to at least three extracted functions: request validation, instance construction, and post-start wiring.
- Each extracted function can be called from a unit test without a live tmux session (instance lookup uses an interface; construction uses value types).
- No behaviour changes.
- `go test ./...` passes.

### INVEST

- **Independent**: Purely internal to `session_service.go`; no interface changes.
- **Negotiable**: The exact function boundaries are flexible within the above groupings.
- **Valuable**: Each extracted function is independently unit-testable.
- **Estimable**: Mechanical extraction; main risk is identifying all shared variables.
- **Small**: Single file.
- **Testable**: A unit test calling `resolveStreamInstance(...)` with a fake store proves the extraction.

---

#### Task 5.1 — Extract `resolveStreamInstance` from `StreamTerminal`

**Files**: `server/services/session_service.go`  
**Effort**: ~2 hours

**Steps**:

1. Extract lines 965-999 (instance lookup, poller fallback, started/paused validation) into:

   ```go
   // resolveStreamInstance finds the live instance for a terminal stream request.
   // It prefers the ReviewQueuePoller's in-memory reference to avoid timestamp desync.
   func (s *SessionService) resolveStreamInstance(sessionID string) (*session.Instance, error) {
       // poller lookup → storage fallback → nil check → started check → paused check
   }
   ```

2. In `StreamTerminal`, replace the extracted block with:

   ```go
   instance, err := s.resolveStreamInstance(initialMsg.SessionId)
   if err != nil {
       return err
   }
   ```

3. `go build ./...` must pass.

**Known risk**: The fallback path at line 974 calls `s.loadInstancesWithWiring()`, which has side effects. The extracted function must preserve this exact fallback path and its warning log.

---

#### Task 5.2 — Extract `runTerminalStream` from `StreamTerminal`

**Files**: `server/services/session_service.go`  
**Effort**: ~3 hours  
**Depends on**: Task 5.1

**Steps**:

1. Extract lines 1001-1237 (PTY acquisition, goroutine launch, flow control, `select` wait) into:

   ```go
   func (s *SessionService) runTerminalStream(
       ctx context.Context,
       stream *connect.BidiStream[sessionv1.TerminalData, sessionv1.TerminalData],
       instance *session.Instance,
       initialMsg *sessionv1.TerminalData,
   ) error {
       // PTY get, terminalState, goroutines, select
   }
   ```

2. `StreamTerminal` becomes:

   ```go
   func (s *SessionService) StreamTerminal(...) error {
       initialMsg, err := stream.Receive()
       // nil checks and session_id validation (lines 953-963)
       instance, err := s.resolveStreamInstance(initialMsg.SessionId)
       if err != nil { return err }
       return s.runTerminalStream(ctx, stream, instance, initialMsg)
   }
   ```

3. `go build ./...` and `go test ./...` must pass.

---

#### Task 5.3 — Extract `validateCreateRequest` from `CreateSession`

**Files**: `server/services/session_service.go`  
**Effort**: ~1 hour

**Steps**:

1. Extract lines 506-522 (required field checks and duplicate title detection) into:

   ```go
   func validateCreateRequest(req *sessionv1.CreateSessionRequest, existing []*session.Instance) error {
       if req.Title == "" { return connect.NewError(...) }
       if req.Path == ""  { return connect.NewError(...) }
       for _, inst := range existing {
           if inst.Title == req.Title { return connect.NewError(...) }
       }
       return nil
   }
   ```

   Making this a package-level function (not a method) allows calling it in tests without constructing a `SessionService`.

2. In `CreateSession`, load instances first, then call `validateCreateRequest(req.Msg, instances)`.

3. `go build ./...` must pass.

---

#### Task 5.4 — Extract `buildInstanceFromRequest` from `CreateSession`

**Files**: `server/services/session_service.go`  
**Effort**: ~2 hours  
**Depends on**: Task 5.3

**Steps**:

1. Extract lines 524-625 (GitHub URL resolution, config defaults, session type inference, `InstanceOptions` construction) into:

   ```go
   func buildInstanceFromRequest(req *sessionv1.CreateSessionRequest, mcpServerURL string) (session.InstanceOptions, error) {
       // GitHub URL resolution
       // config.ResolveDefaults
       // session type inference
       // build and return InstanceOptions
   }
   ```

2. This function has no storage or service dependencies and can be fully unit-tested.

3. In `CreateSession`, replace the extracted block with:

   ```go
   opts, err := buildInstanceFromRequest(req.Msg, s.mcpServerURL)
   if err != nil { return nil, err }
   instance, err := session.NewInstance(opts)
   ```

4. `go build ./...` and `go test ./...` must pass.

---

#### Task 5.5 — Extract `startAndWireInstance` from `CreateSession`

**Files**: `server/services/session_service.go`  
**Effort**: ~2 hours  
**Depends on**: Task 5.4

**Steps**:

1. Extract lines 626-662 (instance start, hook config injection, storage save with rollback, poller update, event publish) into:

   ```go
   func (s *SessionService) startAndWireInstance(instance *session.Instance, allInstances []*session.Instance) ([]*session.Instance, error) {
       // inst.Start(true)
       // InjectHookConfig (non-fatal)
       // append + SaveInstances (with Destroy rollback)
       // reviewQueuePoller.SetInstances
       // eventBus.Publish
       return updatedInstances, nil
   }
   ```

2. `CreateSession` becomes a readable three-step pipeline:

   ```go
   instances, err := s.storage.LoadInstances()
   if err := validateCreateRequest(req.Msg, instances); err != nil { return nil, err }
   opts, err := buildInstanceFromRequest(req.Msg, s.mcpServerURL)
   inst, err := session.NewInstance(opts)
   updated, err := s.startAndWireInstance(inst, instances)
   return connect.NewResponse(&sessionv1.CreateSessionResponse{
       Session: adapters.InstanceToProto(inst),
   }), nil
   ```

3. Run `go build ./...` and `go test ./...`.

---

## Story 6 — Fix `ReviewQueuePoller` Identity and Interface Bugs

**Addresses**: P0 findings #1 and #2 (from 2026-04-22 review); P2 finding (error types)
**Files in scope**: `session/review_queue_poller.go`, `session/storage.go`, `session/storage_interfaces.go` (new), `server/dependencies.go`

### Problem

`ReviewQueuePoller` keys three internal maps (`lastSeenActivity`, `cachedContent`, `FindInstance`) by `Instance.Title` — a user-editable string. If a session is renamed, the poller's cache silently de-syncs. If two sessions share a title at different points in time, entries collide. The correct key is `Instance.UUID`, which was added to the persistence layer in a recent fix but never propagated to the poller layer.

In addition, `ReviewQueuePoller`'s constructor accepts `*session.Storage` directly. Any test that exercises poller logic must stand up a real SQLite database, making this one of the least testable components in the codebase.

A related problem: `fmt.Errorf("session not found: %s", id)` is used throughout `Storage`, `ReviewQueuePoller`, and `SessionService`. Callers cannot distinguish a "not found" condition from other errors without string matching — a fragile anti-pattern.

### Acceptance Criteria

- All `ReviewQueuePoller` internal maps use `Instance.UUID` (a `string`) as keys.
- `FindInstance(title string)` is renamed to `FindInstanceByUUID(uuid string)` and all callers updated.
- `ApprovalMetadataProvider` interface (or equivalent) passes UUID, not Title, at all call sites.
- `ReviewQueuePoller` constructor accepts `InstanceReader` (narrow interface) instead of `*session.Storage`.
- `session.ErrSessionNotFound`, `session.ErrSessionAlreadyExists` defined as typed errors; used in `Storage` and `SessionService`.
- All existing tests pass.

### INVEST

- **Independent**: UUID keying (Task 6.1) does not depend on interface extraction (Task 6.2) and both can proceed in parallel.
- **Negotiable**: The exact interface name and method set of `InstanceReader` can be adjusted.
- **Valuable**: Eliminates a class of subtle cache-corruption bugs; makes poller unit-testable.
- **Estimable**: Mechanical rekeying + interface definition; no algorithm changes.
- **Small**: 3 files, no logic changes.
- **Testable**: A unit test constructing a `ReviewQueuePoller` with a fake `InstanceReader` proves the extraction.

---

#### Task 6.1 — Migrate `ReviewQueuePoller` maps from Title to UUID keys

**Files**: `session/review_queue_poller.go`
**Effort**: ~2 hours

**Steps**:

1. Audit every map and lookup in `review_queue_poller.go` that currently uses a session title as a key:

   ```
   grep -n "Title\|lastSeenActivity\|cachedContent\|FindInstance" session/review_queue_poller.go
   ```

2. Change every `map[string]...` that is keyed by Title to be keyed by UUID. The field comments must state the key type:

   ```go
   // lastSeenActivity maps Instance.UUID → last observed output signature.
   lastSeenActivity map[string]string

   // cachedContent maps Instance.UUID → most recently captured terminal output.
   cachedContent map[string]string
   ```

3. Rename `FindInstance(title string) *session.Instance` to `FindInstanceByUUID(uuid string) *session.Instance`. Update all callers in `server/services/session_service.go` and `server/dependencies.go`.

4. In `SetInstances`, when building the new internal map, key by `inst.UUID` (assert UUID is non-empty; log a warning and skip if empty to avoid replacing a keyed entry with blank).

5. `go build ./...` must pass. Run `go test ./session/... ./server/...`.

**Known risk**: Any caller that currently passes `instance.Title` to `FindInstance` must be updated to pass `instance.UUID`. Audit all call sites before renaming.

---

#### Task 6.2 — Define `InstanceReader` interface; inject into `ReviewQueuePoller`

**Files**: `session/storage.go` (or `session/storage_interfaces.go`), `session/review_queue_poller.go`, `server/dependencies.go`
**Effort**: ~2 hours
**Depends on**: Task 6.1

**Steps**:

1. Identify every method called on the `*session.Storage` parameter inside `ReviewQueuePoller`. As of the review, this is limited to listing/loading instances for reconciliation.

2. Define a narrow interface in `session/storage.go`:

   ```go
   // InstanceReader is the read-only storage interface required by ReviewQueuePoller.
   // *Storage satisfies this interface.
   type InstanceReader interface {
       ListInstances(ctx context.Context) ([]*Instance, error)
       // add any other read-only methods actually called by the poller
   }

   var _ InstanceReader = (*Storage)(nil)
   ```

3. Change the `ReviewQueuePoller` constructor to accept `InstanceReader` instead of `*session.Storage`:

   ```go
   func NewReviewQueuePoller(reader session.InstanceReader, ...) *ReviewQueuePoller
   ```

4. Update `server/dependencies.go` to pass the `*session.Storage` value (which satisfies `InstanceReader`) to `NewReviewQueuePoller`.

5. `go build ./...` and `go test ./...` must pass.

---

#### Task 6.3 — Define domain error types; replace string-matched errors

**Files**: `session/errors.go` (new), `session/storage.go`, `server/services/session_service.go`
**Effort**: ~2 hours
**Independent of**: Tasks 6.1 and 6.2

**Steps**:

1. Create `session/errors.go`:

   ```go
   package session

   import "fmt"

   // ErrSessionNotFound is returned when a session cannot be found by the given identifier.
   type ErrSessionNotFound struct {
       ID string
   }

   func (e ErrSessionNotFound) Error() string {
       return fmt.Sprintf("session not found: %s", e.ID)
   }

   // ErrSessionAlreadyExists is returned when a session with the given title already exists.
   type ErrSessionAlreadyExists struct {
       Title string
   }

   func (e ErrSessionAlreadyExists) Error() string {
       return fmt.Sprintf("session already exists: %s", e.Title)
   }
   ```

2. Replace all `fmt.Errorf("session not found: ...")` occurrences in `session/storage.go` with `ErrSessionNotFound{ID: title}` (or UUID where applicable).

3. In `server/services/session_service.go`, update callers that currently do string comparison (`strings.Contains(err.Error(), "session not found")`) to use `errors.As(err, &session.ErrSessionNotFound{})`.

4. `go build ./...` and `go test ./...` must pass.

---

## Story 7 — Decompose `ReviewQueuePoller` into Single-Responsibility Components

**Addresses**: P1 finding #5 (from 2026-04-22 review); P2 finding (LastActivity authority)
**Files in scope**: `session/review_queue_poller.go`, `session/terminal_content_cache.go` (new), `session/tmux_reconciler.go` (new)

### Problem

`ReviewQueuePoller` owns five distinct responsibilities:

1. Monitoring instance status and updating the `ReviewQueue`
2. Caching terminal content (tmux capture-pane output)
3. Spawning tmux subprocesses to capture terminal state
4. Reconciling tmux reality against in-memory instance state (zombie/stale detection)
5. Managing exponential backoff on errors

The file is complex enough to require panic recovery. Testing any single behaviour requires understanding all five. The terminal content cache conceptually belongs closer to the PTY/scrollback layer; the tmux reconciler is an infrastructure concern that has no business being interleaved with queue management.

A related problem: `lastSeenActivity` in `ReviewQueuePoller`, `lastActivity` in `ClaudeController.idleDetector`, `LastMeaningfulOutput` in `Instance.ReviewState`, and `LastActivity` in `ReviewQueue.ReviewItem` represent the same semantic concept but are updated independently, creating temporal inconsistency in queue membership decisions.

### Acceptance Criteria

- `TerminalContentCache` is a distinct type that owns tmux capture-pane invocations and the content cache map.
- `TmuxReconciler` is a distinct type that owns zombie/stale instance detection and the reconciliation loop.
- `ReviewQueuePoller` delegates to both and is reduced to: read status → update queue → backoff.
- `ClaudeController` is documented as the authoritative source for `LastActivity`; all other stores query or subscribe rather than maintain their own clock.
- Panic recovery is preserved in all background goroutines.
- All existing tests pass.

### INVEST

- **Independent**: Can be done after Story 6 Task 6.1 (UUID keying) to avoid migrating the same maps twice.
- **Negotiable**: The exact type names and method boundaries are flexible.
- **Valuable**: Each extracted type is independently testable; `ReviewQueuePoller` scope is reduced to its core purpose.
- **Estimable**: Mechanical extraction; no algorithm changes.
- **Small**: 2 new files + reduced `review_queue_poller.go`; no interface changes at package boundaries.
- **Testable**: A test constructing `TerminalContentCache` with a fake tmux executor proves the extraction.

---

#### Task 7.1 — Extract `TerminalContentCache`

**Files**: `session/review_queue_poller.go`, `session/terminal_content_cache.go` (new)
**Effort**: ~3 hours
**Depends on**: Story 6 Task 6.1 (so cache is keyed by UUID from the start)

**Steps**:

1. Identify all fields and methods in `ReviewQueuePoller` related to terminal content:
   - `cachedContent map[string]string` (the cache itself)
   - The `tmux capture-pane` subprocess invocation logic
   - The cache update and invalidation paths

2. Create `session/terminal_content_cache.go`:

   ```go
   // TerminalContentCache captures and caches terminal output from tmux sessions.
   // It is keyed by Instance.UUID.
   type TerminalContentCache struct {
       mu      sync.Mutex
       content map[string]string // UUID → last captured output
   }

   func NewTerminalContentCache() *TerminalContentCache

   // Capture runs tmux capture-pane for the given instance and updates the cache.
   func (c *TerminalContentCache) Capture(inst *Instance) (string, error)

   // Get returns the most recently captured content for the given UUID, or "" if not cached.
   func (c *TerminalContentCache) Get(uuid string) string
   ```

3. In `ReviewQueuePoller`, replace the inline cache fields and tmux invocations with:

   ```go
   contentCache *TerminalContentCache
   ```

   Delegate all capture calls to `c.contentCache.Capture(inst)`.

4. `go build ./...` must pass after each extraction step.

---

#### Task 7.2 — Extract `TmuxReconciler`

**Files**: `session/review_queue_poller.go`, `session/tmux_reconciler.go` (new)
**Effort**: ~3 hours
**Depends on**: Task 7.1

**Steps**:

1. Identify all fields and methods in `ReviewQueuePoller` related to tmux state reconciliation:
   - Zombie session detection logic
   - Stale instance cleanup
   - The reconciliation loop that compares in-memory state against `tmux ls` output

2. Create `session/tmux_reconciler.go`:

   ```go
   // TmuxReconciler detects and handles stale or zombie tmux sessions.
   type TmuxReconciler struct {
       reader InstanceReader
   }

   func NewTmuxReconciler(reader InstanceReader) *TmuxReconciler

   // Reconcile compares in-memory instances against live tmux sessions and
   // marks stale instances as stopped.
   func (r *TmuxReconciler) Reconcile(ctx context.Context, instances []*Instance) error
   ```

3. In `ReviewQueuePoller`, replace the inline reconciliation logic with:

   ```go
   reconciler *TmuxReconciler
   ```

   Call `p.reconciler.Reconcile(ctx, instances)` at the appropriate point in the poll loop.

4. Preserve panic recovery: any goroutine spawned inside `TmuxReconciler` must wrap its body in a `defer func() { recover() }()` matching the existing pattern.

5. `go build ./...` and `go test ./...` must pass.

---

#### Task 7.3 — Document `ClaudeController` as authoritative `LastActivity` publisher

**Files**: `session/claude_controller.go`, `session/review_queue_poller.go`, `session/instance.go`
**Effort**: ~1 hour
**Depends on**: Task 7.2

**Steps**:

1. Add a doc comment to `ClaudeController`'s `lastActivity` field (or `idleDetector`):

   ```go
   // lastActivity is the authoritative record of when Claude last produced output.
   // All other activity timestamps (ReviewState.LastMeaningfulOutput, ReviewQueue.ReviewItem.LastActivity)
   // must be derived from this value — never maintained independently.
   ```

2. In `ReviewQueuePoller`, remove the `lastSeenActivity` map (after UUID keying from Story 6 is done) and replace it with a call to `inst.ClaudeController().LastActivity()` (or the equivalent accessor). If the accessor doesn't exist, add it.

3. Add a comment in `Instance.ReviewState` above `LastMeaningfulOutput`:

   ```go
   // LastMeaningfulOutput is set by ReviewQueuePoller from ClaudeController.LastActivity().
   // Do not update this field from any other code path.
   ```

4. `go build ./...` must pass. No test changes expected (behaviour is preserved).

---

## Story 8 — Transport-Agnostic `TerminalSession` Interface

**Addresses**: Audit finding (2026-04-22) — 42 tmux leakage points across `server/services/`
**Files in scope**: `server/services/session_streamer.go` (rename), `server/services/connectrpc_websocket.go`, `session/instance.go`, `server/services/session_service.go`, `server/services/debug_snapshot.go`

### Problem

The server layer knows far too much about tmux. An audit of `connectrpc_websocket.go` found 21 direct tmux call sites; the `SessionStreamer` interface itself names a tmux protocol feature (`StartControlMode`, `SubscribeControlModeUpdates`). Concrete problems:

1. **Lifecycle bugs caused by misplaced logic.** The Apr 2026 session-restore bug (`DoesSessionExist` → `RestoreWithWorkDir`) lived in the WebSocket handler because there was no interface method that meant "make this session ready to stream". The handler had to reach through `GetTmuxSession()` and call tmux operations directly, bypassing any abstraction.

2. **Backend is unswappable.** There is no seam where tmux could be replaced by docker exec, SSH, or a direct PTY without rewriting the server layer.

3. **Mocks expose tmux concepts.** Any test double for `SessionStreamer` must implement `StartControlMode`, making test vocabulary leak implementation vocabulary.

4. **Two resize methods doing the same thing.** `ResizePTY` and `SetWindowSize` are both called from the handler, both delegating to tmux. A domain-neutral `ResizeTerminal` replaces both.

### Acceptance Criteria

- `SessionStreamer` renamed to `TerminalSession` with domain-neutral method names.
- `Instance` exposes `EnsureConnectable`, `Snapshot`, `ResizeTerminal`, `Subscribe`, `Unsubscribe` (domain-neutral wrappers).
- `connectrpc_websocket.go` contains zero calls to `GetTmuxSession()`, `ResizePTY()`, `CapturePaneContentRaw()`, `SetWindowSize()`, `StartControlMode()`, or `StopControlMode()`.
- `session_service.go` contains zero calls to `ResizePTY()`.
- `debug_snapshot.go` contains zero calls to `CapturePaneContentRaw()`.
- No behaviour changes. `go test ./...` passes.

### INVEST

- **Independent**: Interface renaming + thin wrappers on `Instance`; does not conflict with Stories 1-7 at compile time. (Task 8.3 shares `connectrpc_websocket.go` with Story 5 Task 5.2 — coordinate merges.)
- **Negotiable**: Method signatures on `TerminalSession` are flexible; the constraint is zero tmux vocabulary in the interface or in its callers.
- **Valuable**: Unlocks backend swap; eliminates an entire class of lifecycle bugs where callers manage what the instance should own.
- **Estimable**: Mostly mechanical rename + wrapper delegation; main effort is identifying all call sites (audit above enumerates them).
- **Small**: 5 files, ~120 lines of changes, mostly deletions.
- **Testable**: `grep -rn "StartControlMode\|StopControlMode\|SubscribeControlModeUpdates\|ResizePTY\|CapturePaneContentRaw\|SetWindowSize\|GetTmuxSession" server/services/` returns zero results.

---

#### Task 8.1 — Define `TerminalSession` interface; rename `session_streamer.go`

**Files**: `server/services/session_streamer.go` (rename to `server/services/terminal_session.go`)
**Effort**: ~1 hour

**Steps**:

1. Rename `server/services/session_streamer.go` → `server/services/terminal_session.go`.

2. Replace the `SessionStreamer` interface with `TerminalSession`:

   ```go
   // TerminalSession is the transport-agnostic interface the WebSocket streaming
   // handler requires from a session. Callers know nothing about tmux, control mode,
   // PTYs, or any other terminal multiplexer.
   //
   // *session.Instance satisfies this interface via the wrapper methods added in Story 8.
   //
   // Read ADR-009 before modifying this interface.
   type TerminalSession interface {
       // EnsureConnectable prepares the session for streaming.
       // Implementations handle existence checks and backend restoration internally.
       // Returns an error if the session cannot be made connectable.
       EnsureConnectable(ctx context.Context) error

       // Snapshot returns the current terminal content resized to (cols × rows).
       // The returned bytes are raw ANSI terminal data suitable for replay in xterm.js.
       Snapshot(ctx context.Context, cols, rows int) ([]byte, error)

       // ResizeTerminal adjusts the terminal to the given dimensions.
       ResizeTerminal(cols, rows int) error

       // Subscribe registers a consumer for live output updates.
       // Returns a stable subscriber ID and a receive-only channel of raw output bytes.
       // The channel is closed when the session ends.
       Subscribe() (id string, updates <-chan []byte)

       // Unsubscribe removes the consumer registered under id.
       Unsubscribe(id string)
   }
   ```

3. `go build ./...` will fail until Task 8.2 adds the methods to `Instance`. Commit interface only; proceed to Task 8.2 immediately.

**Note**: `context.Context` on `EnsureConnectable` and `Snapshot` is for future cancellation support. Current implementations may ignore it.

---

#### Task 8.2 — Add domain-neutral wrapper methods to `session.Instance`

**Files**: `session/instance.go`
**Effort**: ~2 hours
**Depends on**: Task 8.1

**Steps**:

1. Add `EnsureConnectable(ctx context.Context) error`. This method internalises the lifecycle logic that previously lived in `streamViaControlMode`:

   ```go
   // EnsureConnectable prepares the session for terminal streaming.
   // It checks whether the backing tmux session exists and restores it if not,
   // then starts the streaming mechanism. Satisfies TerminalSession.
   func (i *Instance) EnsureConnectable(_ context.Context) error {
       ts := i.GetTmuxSession()
       if ts != nil && !ts.DoesSessionExist() {
           workDir := i.GetWorkingDirectory()
           if err := ts.RestoreWithWorkDir(workDir); err != nil {
               return fmt.Errorf("session restore failed: %w", err)
           }
       }
       return i.StartControlMode()
   }
   ```

2. Add `Snapshot(ctx context.Context, cols, rows int) ([]byte, error)`. This replaces the inline capture-pane + resize logic in the handler:

   ```go
   // Snapshot returns the current terminal content at the requested dimensions.
   // Satisfies TerminalSession.
   func (i *Instance) Snapshot(_ context.Context, cols, rows int) ([]byte, error) {
       if err := i.ResizePTY(cols, rows); err != nil {
           // Non-fatal: capture may still succeed at existing dimensions.
           log.WarningLog.Printf("[Snapshot] resize to %dx%d failed: %v", cols, rows, err)
       }
       raw, err := i.CapturePaneContentRaw()
       if err != nil {
           return nil, err
       }
       return []byte(raw), nil
   }
   ```

3. Add `ResizeTerminal(cols, rows int) error`. Replaces both `ResizePTY` and `SetWindowSize` at the call sites:

   ```go
   // ResizeTerminal adjusts the terminal to the given dimensions.
   // Satisfies TerminalSession.
   func (i *Instance) ResizeTerminal(cols, rows int) error {
       return i.ResizePTY(cols, rows)
   }
   ```

   `SetWindowSize` is already a delegating wrapper; `ResizePTY` is the canonical call. After call sites are updated in Task 8.3, both old methods may be unexported or removed if nothing outside `session/` calls them.

4. Add `Subscribe() (string, <-chan []byte)` and `Unsubscribe(id string)` as thin wrappers:

   ```go
   func (i *Instance) Subscribe() (string, <-chan []byte) {
       return i.SubscribeControlModeUpdates()
   }

   func (i *Instance) Unsubscribe(id string) {
       i.UnsubscribeControlModeUpdates(id)
   }
   ```

5. Add compile-time assertion in `session/instance.go`:

   ```go
   // Ensure Instance satisfies the server-layer TerminalSession interface.
   // Import cycle prevention: assert against the interface copy in session/ if needed,
   // or use a build tag. Preferred: assert in server/services/ test file.
   ```

   Add the assertion in a new `server/services/terminal_session_test.go`:

   ```go
   var _ TerminalSession = (*session.Instance)(nil)
   ```

6. `go build ./...` must pass.

---

#### Task 8.3 — Update `connectrpc_websocket.go` to use `TerminalSession`

**Files**: `server/services/connectrpc_websocket.go`
**Effort**: ~3 hours
**Depends on**: Task 8.2
**Coordinate with**: Story 5 Task 5.2 (both touch `connectrpc_websocket.go`)

**Steps**:

1. Replace `var streamer SessionStreamer = instance` with `var terminal TerminalSession = instance` throughout `streamViaControlMode`.

2. Replace `streamer.StartControlMode()` and the surrounding `GetTmuxSession()` / `DoesSessionExist()` / `RestoreWithWorkDir()` block with a single call:

   ```go
   if err := terminal.EnsureConnectable(stream.Context()); err != nil {
       return fmt.Errorf("session not connectable: %w", err)
   }
   defer func() {
       // StopStreaming is still needed for cleanup; add as TerminalSession method in Task 8.4
       if err := instance.StopControlMode(); err != nil {
           log.WarningLog.Printf("[streamViaControlMode] stop: %v", err)
       }
   }()
   ```

   Note: `StopControlMode` can be promoted to `TerminalSession` as `StopStreaming()` in a follow-up or handled via `context.Context` cancellation. For this task, a direct call to `instance.StopControlMode()` is acceptable as a transitional measure — it is the only remaining tmux call in scope after this task.

3. Replace all `instance.ResizePTY(...)` calls with `terminal.ResizeTerminal(...)`.

4. Replace `instance.SetWindowSize(...)` calls with `terminal.ResizeTerminal(...)`.

5. Replace `instance.CapturePaneContentRaw()` calls with `terminal.Snapshot(stream.Context(), targetCols, targetRows)`. Adjust the surrounding resize-nudge logic:

   ```go
   // The ±1 nudge is now encapsulated in Snapshot. Remove the inline nudge calls.
   initialContent, err := terminal.Snapshot(stream.Context(), targetCols, targetRows)
   if err != nil {
       log.InfoLog.Printf("[streamViaControlMode] snapshot failed, proceeding empty: %v", err)
       initialContent = nil
   }
   ```

   **Known risk**: The current capture-pane path includes quiescence detection (waitForQuiescence) before capturing. `Snapshot` must preserve this ordering. Either pass quiescence state as an argument or keep the quiescence wait in the handler and call `terminal.ResizeTerminal` + `terminal.Snapshot` separately. Do not silently drop quiescence detection.

6. Replace `streamer.SubscribeControlModeUpdates()` with `terminal.Subscribe()`.

7. Replace `streamer.UnsubscribeControlModeUpdates(id)` with `terminal.Unsubscribe(id)`.

8. Remove the `GetTmuxSession()` call entirely (now inside `EnsureConnectable`).

9. Remove the `instance.TmuxPrefix` field access if it is only used to construct `tmuxSessionName` for logging. The log message is acceptable as a one-time lookup; if it is used elsewhere in the function, keep it as a local variable.

10. `go build ./...` and `go test ./server/services/...` must pass.

---

#### Task 8.4 — Update remaining callers; deprecate old method names

**Files**: `server/services/session_service.go`, `server/services/debug_snapshot.go`
**Effort**: ~1 hour
**Depends on**: Task 8.2

**Steps**:

1. In `session_service.go` (line 1172): replace `instance.ResizePTY(cols, rows)` with `instance.ResizeTerminal(cols, rows)`.

2. In `debug_snapshot.go` (line 189): replace `inst.CapturePaneContentRaw()` with `inst.Snapshot(ctx, 0, 0)` (zero dimensions = current size; implementation may need a zero-size guard).

3. Add deprecation comments on the old `Instance` methods that are now wrapped:

   ```go
   // Deprecated: use ResizeTerminal instead. Will be unexported in a future release.
   func (i *Instance) ResizePTY(cols, rows int) error { ... }

   // Deprecated: use Snapshot instead. Will be unexported in a future release.
   func (i *Instance) CapturePaneContentRaw() (string, error) { ... }

   // Deprecated: use ResizeTerminal instead. Will be unexported in a future release.
   func (i *Instance) SetWindowSize(cols, rows int) error { ... }
   ```

   Do not remove them yet — they may be called from `session/` tests or from `TmuxManager`. Mark for removal after all callers are confirmed updated.

4. Verify the zero-tmux-call invariant:

   ```bash
   grep -rn "StartControlMode\|StopControlMode\|SubscribeControlModeUpdates\|ResizePTY\|CapturePaneContentRaw\|SetWindowSize\|GetTmuxSession" server/services/
   ```

   This must return zero results (excluding the `terminal_session.go` interface file and any deprecation comments).

5. `go build ./...` and `go test ./...` must pass.

---

## Known Issues

### Potential Bugs Identified During Planning

#### JSON Schema Break in `GitHubPRStatus` extraction (Story 4, Task 4.1) — SEVERITY: High

**Description**: Changing `InstanceData` from individual flat fields (`github_pr_state`, `github_pr_is_draft`, etc.) to a nested struct (`"pr_status": {...}`) breaks the JSON serialization format. Any persisted `sessions.json` or SQLite row written before this change will deserialize `PRStatus` as a zero-value struct, silently clearing all PR status data on the next load.

**Mitigation**:
- Add a `MigrateFromLegacyPRFields` step in `Storage.LoadInstances()` that detects the old flat fields via `json.RawMessage` and back-fills `PRStatus`.
- Write a migration test that loads a fixture file using the old schema and verifies `PRStatus` is populated correctly after migration.
- Alternatively, keep the flat fields on `InstanceData` (JSON DTO) and only introduce `GitHubPRStatus` as an in-memory type on `Instance`, mapping between them in `ToInstanceData` / `FromInstanceData`. This avoids the schema migration entirely.

**Files affected**: `session/storage.go`, `session/ent_repository.go`, any fixture JSON files in `testdata/`

---

#### `SessionServiceStore` composed interface excludes future store additions (Story 1, Task 1.3) — SEVERITY: Medium

**Description**: Introducing `SessionServiceStore` as a composed interface in `session_service.go` means any new storage concern added later must be added to the interface, recompiled, and all fakes updated. If a developer adds a new sub-service that takes `*session.Storage` directly (the current pattern), they bypass the interface and the type assertion returns.

**Mitigation**:
- Add a `// ADD NEW STORAGE INTERFACES HERE` comment inside `SessionServiceStore`.
- Document in a code review checklist: "Does the new sub-service accept a narrow interface or `*session.Storage`?"
- Add a linter rule (via `golangci-lint` custom check or `go vet` plugin) that flags direct use of `*session.Storage` outside of `session/` package.

**Files affected**: `server/services/session_service.go`, any future sub-service files

---

#### `RuntimeWirer` method ordering not enforced at compile time (Story 2) — SEVERITY: Medium

**Description**: Extracting `BuildRuntimeDeps` into discrete `RuntimeWirer` methods does not prevent a caller from calling `startControllers()` before `wireInstances()`. The instances slice is passed as an argument, so a nil slice would cause a panic rather than a compile error.

**Mitigation**:
- Name the return value of `wireInstances()` clearly (`instances`) and document the required call order at the top of `BuildRuntimeDeps`.
- Add a nil check at the start of `startControllers(instances []*session.Instance)`: `if instances == nil { return }`.
- Consider using an `Option` pattern or a builder that enforces ordering via internal state, but only if the function grows further — for the current 4-step sequence, argument passing with nil guards is sufficient.

**Files affected**: `server/dependencies.go`

---

#### Dual `InstanceData` / `Session` model creates ambiguity for new contributors (Story 3, Task 3.3) — SEVERITY: Low

**Description**: Marking `InstanceData` methods as deprecated without a concrete migration path leaves new contributors uncertain which API to use. If migration is never completed, the deprecation comment becomes misleading noise.

**Mitigation**:
- Add a tracked issue (or TODO comment with date) referencing the migration target.
- In the next feature that requires a new persistence query, require use of `SessionDomainRepository` methods.
- Set a migration deadline in the deprecation comment (e.g., "Target removal: next major version bump").

**Files affected**: `session/repository.go`, `session/ent_repository.go`

---

#### `resolveStreamInstance` fallback path side effects (Story 5, Task 5.1) — SEVERITY: Low

**Description**: The fallback in `StreamTerminal` at line 974 calls `s.loadInstancesWithWiring()`, which initialises controller wiring as a side effect. If `resolveStreamInstance` is called from a unit test, this side effect may panic or produce unexpected state.

**Mitigation**:
- In the extracted function, accept an `instanceLoader` function argument (or a small interface) rather than calling `s.loadInstancesWithWiring()` directly. This makes the dependency injectable and testable.
- In production, `SessionService.StreamTerminal` passes `s.loadInstancesWithWiring`; in tests, a fake loader is passed.

**Files affected**: `server/services/session_service.go`

---

## Dependency Map

```
Story 1
  Task 1.1 ──► Task 1.2 ──► Task 1.3
                                │
                     (Story 3 can proceed in parallel)

Story 2
  Task 2.1 ──► Task 2.2 ──► Task 2.3

Story 3
  Task 3.1 ──► Task 3.2 ──► Task 3.3

Story 4
  Task 4.1 ──► Task 4.2

Story 5
  Task 5.3 ──► Task 5.4 ──► Task 5.5   (CreateSession path, independent of stream path)
  Task 5.1 ──► Task 5.2                 (StreamTerminal path, independent of create path)

Story 8
  Task 8.1 ──► Task 8.2 ──► Task 8.3 ──► Task 8.4
                                │
                     (Task 8.3 coordinates with Story 5 Task 5.2 — same file)
```

Stories 1, 2, 3, 4, 5, and 8 are mutually independent and can be worked on separate branches simultaneously. Within each story, tasks follow the order listed above.

**Story 8 + Story 5 coordination**: Tasks 8.3 and 5.2 both modify `connectrpc_websocket.go`. If worked in parallel, merge Story 5 first (pure extraction, no interface changes) then apply Story 8 (interface rename, call-site updates). If worked sequentially, Story 8 after Story 5 is cleanest.

---

## Implementation Order Recommendation

For a single developer, the recommended sequence is:

1. **Story 5 first** (Tasks 5.1-5.5) — purely mechanical extraction within one file, zero interface changes, builds confidence and familiarity with the service layer.
2. **Story 8** (Tasks 8.1-8.4) — immediately after Story 5; cleans up the interface while Story 5's extraction points are fresh. Eliminates the class of lifecycle bugs (session-restore race) that Story 5 alone cannot prevent.
3. **Story 1** (Tasks 1.1-1.3) — highest risk reduction; removes the concrete type assertion that undermines test fidelity.
4. **Story 2** (Tasks 2.1-2.3) — mechanical extraction in `dependencies.go`; reduces future ordering bugs.
5. **Story 3** (Tasks 3.1-3.3) — interface split; can be done safely after Stories 1 and 2 have settled.
6. **Story 4 last** (Tasks 4.1-4.2) — requires the JSON migration decision to be made carefully; do not merge until the migration strategy (flat fields vs. nested struct) is agreed.

For a team of two developers, Stories 5+8 and Stories 1+2 can be worked in parallel. Story 3 follows Stories 1+2. Story 4 should be reviewed by both before merging due to the schema impact.

---

## Integration Checkpoints

These are objectively verifiable gates between story groups. Do not move to the next group until the checkpoint passes.

### Checkpoint 1 — After Story 6 (Tasks 6.1, 6.2, 6.3)

All three tasks can be verified independently; together they constitute the P0/P1 bug-fix group.

**6.1 — UUID keying**
- `grep -n "Title" session/review_queue_poller.go` returns zero hits on map key assignments (`lastSeenActivity[`, `cachedContent[`).
- `FindInstance` is absent from the codebase: `grep -rn "FindInstance[^B]" server/ session/` returns no results.
- `FindInstanceByUUID` exists: `grep -rn "FindInstanceByUUID" server/ session/` returns at least two call sites.
- `go build ./...` passes.

**6.2 — InstanceReader interface**
- `grep -n "storage \*Storage" session/review_queue_poller.go` returns zero hits.
- `session.InstanceReader` exists: `grep -n "InstanceReader" session/storage.go` returns the interface definition and the compile-time assertion.
- A test in `session/` or `server/services/` constructs `ReviewQueuePoller` with a fake `InstanceReader` (not `*session.Storage`) and the test file compiles.
- `go test ./session/... ./server/...` passes.

**6.3 — Domain error types**
- `session/errors.go` exists and defines `ErrSessionNotFound` and `ErrSessionAlreadyExists`.
- `grep -rn '"session not found' session/ server/` returns zero hits (all replaced with typed errors).
- `grep -rn 'strings.Contains(err.Error()' server/services/session_service.go` returns zero hits for "session not found" pattern.
- `go test ./...` passes.

**Story 1 — InstanceStore abstraction leak**
- `grep -n "\.(\*session\.Storage)" server/services/session_service.go` returns zero hits.
- `GetStorage()` is absent: `grep -rn "GetStorage()" server/` returns zero hits.
- `SessionServiceStore` exists in `server/services/session_service.go`.
- A test in `server/services/` passes a fake `SessionServiceStore` (not `*session.Storage`) and exercises `RulesStore` or `AnalyticsStore` code paths without panicking or no-oping.
- `go test ./server/services/...` passes.

---

### Checkpoint 2 — After Story 7 (Tasks 7.1, 7.2, 7.3)

**7.1 — TerminalContentCache extracted**
- `session/terminal_content_cache.go` exists.
- `grep -n "cachedContent" session/review_queue_poller.go` returns zero hits (cache fields moved to `TerminalContentCache`).
- A test in `session/` constructs `TerminalContentCache` with a fake tmux executor and calls `Capture` — it compiles and runs without a live tmux process.
- `go build ./...` passes.

**7.2 — TmuxReconciler extracted**
- `session/tmux_reconciler.go` exists.
- `go test ./session/...` passes with no panics in reconciliation paths.
- Any goroutine spawned inside `TmuxReconciler` contains a `defer func() { recover() }()` block.

**7.3 — ClaudeController authority documented**
- `session/claude_controller.go` contains the doc comment marking `lastActivity` as the authoritative source.
- `grep -n "lastSeenActivity" session/review_queue_poller.go` returns zero hits (map removed; replaced by `inst.ClaudeController().LastActivity()` call).
- `session/instance.go` contains the doc comment on `LastMeaningfulOutput` restricting writers.
- `go build ./...` passes.

---

### Checkpoint 3 — After Stories 2, 3, 5

**Story 2 — RuntimeWirer**
- `grep -c "^func " server/dependencies.go` returns a count reflecting that `BuildRuntimeDeps` itself is under 30 lines.
- `RuntimeWirer` struct and at least three methods exist in `server/dependencies.go`.
- `go test ./server/...` passes.

**Story 3 — Repository split**
- `grep -n "^type Repository " session/repository.go` returns the type alias (`= FullRepository`).
- `grep -n "SessionRepository\|SessionDomainRepository\|RulesRepository\|AnalyticsRepository\|FullRepository" session/repository.go` returns all five interface definitions.
- `grep -n "var _" session/ent_repository.go` returns five compile-time assertions.
- `go build ./...` passes.

**Story 5 — Mega-function split**
- `resolveStreamInstance` exists as a method on `*SessionService`.
- `runTerminalStream` exists as a method on `*SessionService`.
- `validateCreateRequest` exists as a package-level function.
- `buildInstanceFromRequest` exists as a package-level function.
- `startAndWireInstance` exists as a method on `*SessionService`.
- `go test ./server/services/...` passes; at least one new unit test calls `validateCreateRequest` or `buildInstanceFromRequest` without constructing a `SessionService`.

---

### Checkpoint 5 — After Story 8

**8.1 — Interface defined**
- `server/services/terminal_session.go` exists with `TerminalSession` interface declaring `EnsureConnectable`, `Snapshot`, `ResizeTerminal`, `Subscribe`, `Unsubscribe`.
- `server/services/session_streamer.go` no longer exists (renamed).
- `go build ./...` passes.

**8.2 — Instance satisfies interface**
- `grep -n "var _ TerminalSession" server/services/terminal_session_test.go` returns the compile-time assertion.
- `go build ./...` passes.

**8.3 — Server layer clean**
- `grep -rn "StartControlMode\|StopControlMode\|SubscribeControlModeUpdates\|ResizePTY\|CapturePaneContentRaw\|SetWindowSize\|GetTmuxSession" server/services/` returns zero hits (excluding the interface definition file and any deprecation-comment lines).
- `go test ./server/services/...` passes.

**8.4 — Old callers updated**
- `grep -rn "ResizePTY\|CapturePaneContentRaw" server/` returns zero hits.
- Deprecated comments exist on the old `Instance` methods.
- `go test ./...` passes.

---

### Checkpoint 4 — After Story 4 (optional; defer if schema migration is not ready)

- `GitHubPRStatus` struct exists in `session/instance.go` or `session/github.go`.
- `InstanceData.PRStatus` is a single field of type `GitHubPRStatus`.
- If nested JSON serialisation is chosen: a migration test in `session/` loads a fixture file with the old flat field schema and verifies `PRStatus` is populated correctly.
- If flat fields are preserved on `InstanceData` (in-memory only change): `grep -n "GitHubPRState\|GitHubPRIsDraft" session/instance.go` returns zero hits (fields removed from `Instance`, not from `InstanceData`).
- `go test ./session/...` passes.

---

## Context Preparation Guide

Read these files before starting each story. The list is minimal — only files whose design decisions directly constrain the implementation.

### Before Story 6 (Tasks 6.1, 6.2, 6.3) — ReviewQueuePoller bugs

| File | What to look for |
|------|-----------------|
| `session/review_queue_poller.go` | All map field declarations (lines 66-68); `NewReviewQueuePoller` constructor (line 83); `SetInstances` and `RemoveInstance` methods; all call sites of `lastSeenActivity` and `cachedContent` |
| `session/storage.go` | `InstanceStore` interface definition (line 144); `InstanceData.UUID` field (line 13); existing `var _ InstanceStore = (*Storage)(nil)` pattern |
| `session/instance.go` | `Instance.UUID` field; confirm UUID is set on construction |
| `server/services/session_service.go` | Calls to `FindInstance` (the method being renamed); calls to `NewReviewQueuePoller` or `NewReviewQueuePollerWithConfig` |
| `server/dependencies.go` | Call to `NewReviewQueuePoller` — this is the injection point that must be updated after Task 6.2 |

Key concept: `ApprovalMetadataProvider.GetApprovalMetadataBySession(sessionID string)` already passes a session ID (likely UUID). Confirm whether `sessionID` is UUID or Title before Task 6.1 to determine if `ApprovalMetadataProvider` callers also need updating.

---

### Before Story 1 — InstanceStore abstraction leak

| File | What to look for |
|------|-----------------|
| `server/services/session_service.go` | The type assertion at lines 103-105; `concStorage` usages (lines 134, 137, 140); `GetStorage()` at lines 232-237 and all its callers |
| `server/services/rules_service.go` | Constructor signature — what type does it accept? What methods does it call on that argument? |
| `server/services/review_queue_service.go` | Same as rules_service.go |
| `session/storage.go` | `AllRules`, `UpsertRule`, `DeleteRule`, `RecordAnalytics`, `ListAnalytics` methods — these define the method sets for `RulesStorage` and `AnalyticsStorage` |
| `server/dependencies.go` | Any call to `GetStorage()` that must be updated after it is removed |

Key concept (ADR-006): The composed `SessionServiceStore` interface is defined in `server/services/session_service.go`, not in `session/`. Read ADR-006 before Task 1.3.

---

### Before Story 7 — ReviewQueuePoller decomposition

| File | What to look for |
|------|-----------------|
| `session/review_queue_poller.go` | The full file — understand all five responsibilities before extracting any; identify which goroutines exist and where panic recovery is applied |
| `session/claude_controller.go` | `InstanceContext` interface (line 16); `idleDetector` and `LastActivity()` accessor; this is the authority source per ADR-008 |
| `session/instance.go` | `Instance.ReviewState` struct and `LastMeaningfulOutput` field |

Dependency: Story 7 must follow Story 6 Task 6.1 (UUID keying) so extracted types are keyed by UUID from the start. Do not begin Story 7 until Story 6 Task 6.1 is merged.

---

### Before Story 2 — RuntimeWirer

| File | What to look for |
|------|-----------------|
| `server/dependencies.go` | The full `BuildRuntimeDeps` function (lines 366-556); the twelve numbered-comment sections; the nil guard at line 368; the `ServiceDeps` struct |

No interface changes. This is a purely mechanical extraction. The key risk is preserving the non-blocking goroutine semantics in `startControllers` — read the existing goroutine block carefully before extracting.

---

### Before Story 3 — Repository interface split

| File | What to look for |
|------|-----------------|
| `session/repository.go` | The full `Repository` interface (lines 11-90); all 21 method signatures |
| `session/ent_repository.go` | Confirm `EntRepository` implements all 21 methods; look for any methods that return `errors.New("not implemented")` |
| `session/storage.go` | How `Storage` holds and delegates to `Repository`; the `Session`-first convenience methods (lines 343-367) — these map to `SessionDomainRepository` |

The `Repository = FullRepository` type alias approach (Task 3.1 step 3) is a non-breaking change. Confirm the alias compiles before proceeding to Task 3.2.

---

### Before Story 5 — Mega-function split

| File | What to look for |
|------|-----------------|
| `server/services/session_service.go` | `StreamTerminal` full body (lines 947-1238); `CreateSession` full body (lines 501-663); the `loadInstancesWithWiring()` side-effect call at line 974 (see Known Issues section) |

No interface changes. The key risk in `StreamTerminal` is the `loadInstancesWithWiring()` fallback side effect (Task 5.1 Known Risk). Read it before extracting `resolveStreamInstance`.

---

### Before Story 8 — Transport-agnostic TerminalSession interface

| File | What to look for |
|------|-----------------|
| `server/services/session_streamer.go` | The four existing `SessionStreamer` methods — these are what Task 8.1 replaces |
| `server/services/connectrpc_websocket.go` | The full `streamViaControlMode` and `streamViaTmuxCapturePane` bodies; identify every call site in the audit table in Story 8 before making changes |
| `session/instance.go` | The existing `StartControlMode`, `ResizePTY`, `CapturePaneContentRaw`, `SetWindowSize`, `GetTmuxSession` methods — these are what Task 8.2 wraps |
| `server/services/session_service.go` | Line 1172: the `ResizePTY` call being moved to `ResizeTerminal` |
| `server/services/debug_snapshot.go` | Line 189: the `CapturePaneContentRaw` call being moved to `Snapshot` |

Read ADR-009 before beginning. The critical constraint: `EnsureConnectable` must internalise **all** of the tmux lifecycle logic (existence check + restore + start) so callers are reduced to a single method call.

**Coordination check**: Confirm Story 5 Task 5.2 is merged before starting Task 8.3. Both tasks modify `connectrpc_websocket.go`.

---

### Before Story 4 — GitHubPRStatus value object

| File | What to look for |
|------|-----------------|
| `session/storage.go` | PR status fields in `InstanceData` (lines 49-57); `UpdateInstancePRStatus` method (line 320) — the current flat-field update API |
| `session/instance.go` | PR status fields in `Instance` (lines 159-176) |
| `session/ent_repository.go` | Whether PR fields are stored as individual columns or as a JSON blob — this determines the migration strategy |

**Stop and decide before Task 4.1**: Choose between (a) nested JSON struct (`pr_status: {...}`) with a migration step, or (b) in-memory-only `GitHubPRStatus` with flat fields preserved on `InstanceData`. Document the decision in a comment at the top of `session/storage.go`. The Known Issues section of this plan describes both options.

---

## Epic Success Criteria

The epic is complete when all of the following are objectively true. Each criterion is verifiable by running the listed command or inspecting the listed location.

### Structural

1. **Zero type assertions on `*session.Storage` outside `session/`**
   `grep -rn "\.(\*session\.Storage)" server/` returns zero results.

2. **`SessionServiceStore` composed interface exists**
   `grep -n "SessionServiceStore" server/services/session_service.go` returns the interface definition.

3. **`GetStorage()` removed**
   `grep -rn "GetStorage()" .` returns zero results.

4. **`ReviewQueuePoller` maps keyed by UUID**
   `grep -n "lastSeenActivity\|cachedContent" session/review_queue_poller.go` either returns zero results (if maps are fully removed per Story 7) or returns only UUID-keyed field declarations with the comment `// Instance.UUID →`.

5. **`FindInstanceByUUID` replaces `FindInstance`**
   `grep -rn "FindInstance[^B]" .` returns zero results.

6. **Domain error types defined and used**
   `session/errors.go` exists. `grep -rn '"session not found' session/ server/` returns zero results.

7. **`TerminalContentCache` extracted**
   `session/terminal_content_cache.go` exists with `NewTerminalContentCache`, `Capture`, and `Get` exported.

8. **`TmuxReconciler` extracted**
   `session/tmux_reconciler.go` exists with `NewTmuxReconciler` and `Reconcile` exported.

9. **`Repository` split into four sub-interfaces**
   `grep -n "^type.*Repository interface" session/repository.go` returns at least five lines (`SessionRepository`, `SessionDomainRepository`, `RulesRepository`, `AnalyticsRepository`, `FullRepository`).

10. **`BuildRuntimeDeps` reduced to orchestrator**
    The body of `BuildRuntimeDeps` in `server/dependencies.go` contains no inline logic — only calls to `RuntimeWirer` methods and a final `return`.

10. **`TerminalSession` interface replaces `SessionStreamer`**
    `grep -n "SessionStreamer" server/services/` returns zero results.
    `grep -n "TerminalSession" server/services/terminal_session.go` returns the interface definition.

11. **Server layer contains zero tmux vocabulary**
    `grep -rn "StartControlMode\|StopControlMode\|SubscribeControlModeUpdates\|ResizePTY\|CapturePaneContentRaw\|SetWindowSize\|GetTmuxSession" server/services/` returns zero results (excluding the interface file and deprecation comments).

12. **Compile-time assertion exists**
    `grep -n "var _ TerminalSession" server/services/terminal_session_test.go` returns the assertion line.

### Behavioural (no regressions)

13. **Full test suite passes**
    `go test ./...` exits 0.

14. **Linting passes**
    `make lint` exits 0.

15. **Build passes**
    `go build ./...` exits 0.

### Test coverage (new coverage introduced)

16. **ReviewQueuePoller unit-testable without SQLite**
    At least one test in `session/` or `server/services/` constructs `ReviewQueuePoller` (or its successor after Story 7) using only interface fakes — no `*session.Storage`, no `*ent.Client`.

17. **`validateCreateRequest` has a unit test**
    At least one test calls `validateCreateRequest(req, instances)` directly (package-level function, no `SessionService` construction needed).

18. **`TerminalContentCache` has a unit test**
    At least one test in `session/` exercises `TerminalContentCache.Capture` and `Get` with a fake tmux executor.

19. **`TerminalSession` compile-time assertion exists**
    `server/services/terminal_session_test.go` contains `var _ TerminalSession = (*session.Instance)(nil)` and the file compiles.
