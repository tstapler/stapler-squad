# Architecture Research: Backlog Service Refactor

## 1. Current Layering in `server/services/`

The package uses a **flat-file-split pattern**. Each concern gets its own file but everything stays in `package services`. No sub-packages exist under `server/services/`.

Key evidence:
- `session_service.go` (4,075 lines) has a companion `session_service_shells.go` that holds shell-related RPCs — same struct, same package, different file.
- Well-structured services are 150–400 lines with a single responsibility: `approval_service.go` (148 lines), `review_queue_service.go` (365 lines).
- `BacklogService` (2,570 lines) is the outlier.

**Template services with clean boundaries:**
- `ReviewQueueService` — wired via setters (`SetReactiveQueueManager`, `SetReviewQueuePoller`, `SetApprovalStore`); dependencies injected post-construction. Same pattern `BacklogService` follows with `SetHeadlessPool`, `SetSessionStopper`, etc.
- `ApprovalService` — uses a narrow `*ApprovalStore` concrete type, not an interface. Demonstrates that speculative interfaces are avoided in this codebase.

## 2. P1 Split: File Split vs Package Split

**Recommendation: file split within `package services`.**

The case for a package split is weak:
- `BacklogService` is a single struct registered with the ConnectRPC mux in `server/server.go`. Splitting into sub-packages would require either splitting the struct (API change) or embedding sub-types (complexity).
- No other service in `server/services/` uses a sub-package. Consistency matters here.
- All test helpers (`createTestStorage`, mock types) are in `package services_test` and would need to be split or duplicated.

**Proposed file split (natural semantic boundaries already visible from line numbers):**

| File | Methods | Approx lines |
|---|---|---|
| `backlog_service.go` | struct, constructors, setters, helpers (slugify, triageShortTitle, acCriteriaToJSON, proto converters, buildCostLookup, resolveRepoPathInput, encryptAndMergeToken) | ~550 |
| `backlog_service_query.go` | GetBacklogItem (667), ListBacklogItems (726), ListItemSources (1094), SuggestNextItem (1925), GetSyncHistory (2273), SearchGitHubRepos (2304), ListGitHubIssues (2327), GetBacklogItemDiff (2361), GetBacklogItemCost (2432), GetSessionBacklogIndex (2498) | ~700 |
| `backlog_service_lifecycle.go` | CreateBacklogItem (598), UpdateBacklogItem (769), ArchiveBacklogItem (845), DeleteBacklogItem (880), TransitionBacklogItemStatus (902), ApprovePlan (1013), CreateItemSource (1059), UpdateItemSource (1119), DeleteItemSource (1158), OverrideVerdict (1955) | ~700 |
| `backlog_service_triage.go` | SpawnSessionFromItem (1180), AutoReopenAfterFailedReview (1410), AutoReopenForPRFix (1470), AttachSessionToItem (1549), TriggerTriage (1662), CancelTriage (1894), TriggerReReview (2041), TriggerSync (2230), ImportGitHubIssue (2300) | ~900 |

The test file `backlog_github_rpc_test.go` already hints at a further `backlog_service_github.go` split for SearchGitHubRepos, ListGitHubIssues, ImportGitHubIssue if desired later.

**No package rename, no import changes** — all files remain `package services`.

## 3. P3: ReviewGateRunner Type Shape

`spawnReviewGate` in `session/backlog_lifecycle.go` (line 379) currently lives as a method on `BacklogLifecycleListener`. It has 13 revisions and cognitive complexity 35.

**Fields it reads from its caller's struct:**
- `l.storage` — for GetWorktreeDataBySessionUUID, CreateItemSessionWithVerdict, UpdateItemSessionEnded, UpdateBacklogItem, TransitionBacklogItemStatus
- `l.shutdownCtx` — parent context for the review call
- `l.getHeadlessPool()` — for headless review path
- `l.sessionCreator` — legacy ReviewGateSpawner path (deprecated per the code comment)

**Fields read from the ent types it receives:**

From `*ent.BacklogItem`: ID, RepoPath, AcceptanceCriteria, SkipReviewGate, Status, Title, PrURL, PrNumber, UpdatedAt, Edges (BacklogItem for item-session join)

From `*ent.ItemSession`: ID, SessionUUID, SessionRole, AcSnapshot, LastCommitSha

**Proposed type:**

```go
// session/review_gate.go
type ReviewGateRunner struct {
    storage     *Storage
    pool        *headless.Pool
    shutdownCtx context.Context
}

func NewReviewGateRunner(storage *Storage, pool *headless.Pool, shutdownCtx context.Context) *ReviewGateRunner {
    return &ReviewGateRunner{storage: storage, pool: pool, shutdownCtx: shutdownCtx}
}

func (r *ReviewGateRunner) Run(item *ent.BacklogItem, is *ent.ItemSession) {
    // body of current spawnReviewGate
}
```

**Constructor args, not option struct.** Three fields is below the threshold where option structs add value. Option structs are justified when 5+ fields exist, fields have useful zero-values, or callers frequently want to omit fields — none of these apply here.

**ent coupling in signature:** `Run` takes `*ent.BacklogItem` and `*ent.ItemSession` directly. This is intentional until P6 domain DTOs for ItemSession land. Introducing a narrow DTO now just for P3 would be premature — wait for P6.

**Storage interaction:** `ReviewGateRunner.Run` calls `storage.GetWorktreeDataBySessionUUID`, `storage.CreateItemSessionWithVerdict`, `storage.UpdateItemSessionEnded`, `storage.UpdateBacklogItem`, and `storage.TransitionBacklogItemStatus`. These are all currently on `*session.Storage` (concrete type). No interface is needed here per the codebase's anti-speculative-interface rule (see `.claude/rules/interface-pollution-checklist.md`).

**Integration with lifecycle listener:** `BacklogLifecycleListener` would hold a `*ReviewGateRunner` field and its `spawnReviewGate` becomes a one-liner delegating to `runner.Run(item, is)`. The semaphore (`reviewSem`) stays on `BacklogLifecycleListener` since it governs fan-out, not the single run.

## 4. P5: session/domain Sub-Package

**Finding: no existing `domain` sub-package; the session root IS the domain package.**

The session package already has 22+ sub-packages, all purpose-specific (artifacts, cdp, detection, ent, git, headless, hibernation, memory, namegen, tokens, etc.). None is a generic "domain" package.

Pure domain types — `AcCriterion`, `BacklogStatus`, `ReviewOutcome`, `CriterionVerdict`, `AggregateOutcome` — already live in `session/backlog.go`. Domain logic — `CanTransitionBacklog`, `TransitionGuard`, `ParseAcCriteria`, `SerializeAcCriteria` — lives in the same file.

**Two options:**

Option A: Keep pure domain types in `session` root (recommended).
- Avoids breaking the 65+ importers of `session`.
- `session` is already the domain package by convention in this codebase.
- The `session/artifacts` sub-package is the closest precedent: it contains pure types (`SessionArtifactsBlob`, `CommandArtifact`) with zero external deps and a clean package name. This is the right pattern for genuinely separable sub-concerns.

Option B: Extract to `session/domain`.
- Makes pure domain types importable without pulling in ent or headless.
- Creates an awkward import path for callers who already import `session`: they'd need both `session` and `session/domain`.
- Introduces a package rename for all callers of `AcCriterion`, `BacklogStatus`, etc.
- Justified only if the session package develops import cycles that require it, which it doesn't currently.

**Verdict for P5:** If the goal is to isolate pure types with no ent/headless dependencies, the right action is to ensure `session/backlog.go` has no ent/headless imports at the top (it currently doesn't — ent imports are in `backlog_review.go` and `backlog_lifecycle.go`). A `session/domain` package is not warranted given current import structure.

## 5. P6: Storage Interface — ent Type Leakage

**Current leaks through `*session.Storage` methods (in `session/storage.go` and `session/storage_backlog.go`):**

| Method | Leaked ent type |
|---|---|
| `GetItemSession` | `*ent.ItemSession` |
| `CreateItemSession` | `*ent.ItemSession` |
| `GetItemSessionBySessionUUID` | `*ent.ItemSession` |
| `GetItemSessionBySessionAndItem` | `*ent.ItemSession` |
| `ListItemSessions` | `[]*ent.ItemSession` |
| `CreateItemSessionWithVerdict` | `(*ent.ItemSession, *ent.ReviewVerdict)` |
| `ListSourceSyncEvents` | `[]*ent.SourceSyncEvent` |
| `BacklogItemData.ItemSessions` | `[]*ent.ItemSession` (field in domain DTO) |
| `BacklogItemData.StatusEvents` | `[]*ent.BacklogStatusEvent` (field in domain DTO) |

**Root cause:** `ItemSession`, `ReviewVerdict`, `SourceSyncEvent`, and `BacklogStatusEvent` have no domain DTO equivalents yet — unlike `BacklogItemData` which was already lifted. The `storage_backlog.go` methods that return these types are implemented on `*EntRepository` directly (not via the `Repository` interface), so they bypass the abstraction layer.

**Correct fix pattern (P6):**

1. Introduce domain DTOs alongside existing ones in `session/repository.go`:
   ```go
   type ItemSessionSummary struct { /* fields from *ent.ItemSession that callers actually use */ }
   type ReviewVerdictData struct { /* existing — already defined */ }
   type SourceSyncEventData struct { /* fields from *ent.SourceSyncEvent */ }
   type BacklogStatusEventData struct { /* fields from *ent.BacklogStatusEvent */ }
   ```

2. Add conversion functions to `session/ent_repository_backlog.go` (same pattern as `backlogItemToData`):
   ```go
   func itemSessionToSummary(is *ent.ItemSession) ItemSessionSummary { ... }
   ```

3. Update `storage_backlog.go` methods to return domain types. Update `BacklogItemData.ItemSessions` field type.

4. **Error handling:** Both `backlog_lifecycle.go` and `backlog_service.go` currently call `ent.IsNotFound(err)` directly, bypassing the `session.ErrNotFound` sentinel. The `ent_repository_backlog.go` already wraps some errors as `ErrNotFound` (lines 119, 131, 192, 199, 266, 277). The pattern to enforce: `ent_repository_backlog.go` wraps all ent errors; callers above Storage use only `errors.Is(err, session.ErrNotFound)`. The dual-check `ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound)` in lifecycle (lines 677, 830, 860, 913, 1145, 1167, 1191, 1568) is the symptom; fix is consistent wrapping in the repository.

**Add to `Repository` interface:** The `ItemSession` CRUD methods (`CreateItemSession`, `GetItemSession`, `ListItemSessions`, etc.) are NOT in the `Repository` interface — they live only on `*EntRepository` and are called directly. These should be added to the `Repository` interface (with domain return types) to enable test doubles that don't need a real SQLite DB.

## 6. Dependency Graph (backlog layer)

```
session/ent/                          (generated ORM types)
    ^
    |
session/ent_repository_backlog.go     (DB implementation; converts ent → domain DTOs)
    ^
    |
session/storage_backlog.go            (delegating Storage methods; some leak *ent.ItemSession)
session/backlog_lifecycle.go          (lifecycle events; imports ent directly — smell)
    ^
    |
server/services/backlog_service.go    (ConnectRPC handlers; imports session/ent for ent.ItemSession)
    ^
    |
server/mcp/tools_backlog.go           (MCP tool handlers; imports session, calls BacklogService)
```

**Temporal coupling (from prior hotspot analysis):**
- `ent_repository_backlog` ↔ `backlog_service`: 0.78 (change together 78% of the time)
- `backlog_lifecycle` ↔ `backlog_service`: 0.71
- These ratios suggest the ent leakage is the root cause — every schema change ripples up through both layers simultaneously.

## 7. Integration Tests

**Layer coverage:**

| File | Package | What it tests |
|---|---|---|
| `session/backlog_integration_test.go` | `session` | Storage-layer lifecycle: IT-001 to IT-010. Uses `createTestStorage` with real SQLite. |
| `server/services/backlog_service_test.go` | `services` | BacklogService RPCs with real SQLite storage (via `createTestStorage` in `session_service_test.go`). ~30 unit tests. |
| `server/services/backlog_triage_harness_test.go` | `services` | Full triage workflow end-to-end with fake LLM pool (`fastTriagePool`). `TestTriageHarness`, `TestTriageHarness_RealClaude`. |
| `server/services/backlog_github_rpc_test.go` | `services` | GitHub-specific RPCs. |
| `server/services/backlog_service_encryption_test.go` | `services` | Token encryption in item source config. |

**No cross-layer integration tests** (session + services + mcp together). Each layer tests in isolation.

**`createTestStorage` pattern (important for refactor):**
- `server/services/session_service_test.go` line 19 defines `createTestStorage` returning `*session.Storage` backed by a temporary SQLite DB via `session.NewEntRepository(session.WithDatabasePath(...))`.
- This same helper is used by `backlog_service_test.go`, meaning the service-layer tests exercise the real EntRepository, not a mock.
- For the file split (P1), no test changes are needed — tests in `package services` see all files in the package.
- For ReviewGateRunner (P3), existing `TestBacklogIntegration_IT002` and `IT006` in `session/backlog_integration_test.go` already cover the review gate path; a dedicated `TestReviewGateRunner` in `session/review_gate_test.go` would complement them.

## 8. Key Architectural Smells to Address (by priority)

1. **ent leakage in BacklogItemData**: `BacklogItemData.ItemSessions []*ent.ItemSession` embeds an ORM model in a domain DTO. This is the highest-value fix in P6 — it makes the domain DTO truly domain-scoped.

2. **`backlog_lifecycle.go` calling `ent.IsNotFound` directly**: The session layer's backlog lifecycle imports `session/ent` for error checking instead of using `session.ErrNotFound`. Fix: ensure ent_repository_backlog wraps all ent errors consistently.

3. **BacklogService struct (P1)**: At 2,570 lines with 52 exported functions, it violates the single-responsibility principle. The file split keeps the type coherent while making individual areas navigable and testable in isolation.

4. **ItemSession CRUD outside Repository interface**: Storage methods for ItemSession are not in `Repository`, making them impossible to stub without a real SQLite DB. Adding them to the interface (with domain types) enables cheaper test doubles.
