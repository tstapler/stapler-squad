# Backlog Service Refactor — Features Research

*Researched: 2026-07-09*

---

## 1. Existing Service-Split Patterns in `server/services/`

The only prior service-split in this codebase is **`session_service_shells.go`** — a file
that holds the five shell-related RPC handlers (`SpawnShell`, `StopShell`, `RestartShell`,
`ListShells`, `DeleteShell`) extracted from the 4,075-line `session_service.go`.

Pattern used:
- Same `package services` — no sub-package, just a file split
- Receiver type stays on the single `SessionService` struct (unchanged)
- File-top comment names the file's RPC group (`// session_service_shells.go — ConnectRPC
  handlers for the custom-shell RPCs`)
- Private helpers that only those handlers use (`goShellStatusToProto`, `shellToProto`) live
  in the same file
- No separate constructor, no new types — just a file boundary

**Implication for P1 (split `backlog_service.go`):** Follow the same pattern. Three files in
`server/services/`, all `package services`, all methods on `*BacklogService`:
- `backlog_service.go` — struct, constructor, setters, private helpers, proto conversion fns
- `backlog_service_lifecycle.go` — `SpawnSessionFromItem`, `AutoReopenAfterFailedReview`,
  `AutoReopenForPRFix`, `AttachSessionToItem`, `TriggerReReview`, `commitAndPushItemWorktrees`,
  `cleanupItemWorktrees`
- `backlog_service_triage.go` — `TriggerTriage`, `CancelTriage`, `ApprovePlan`,
  `SuggestNextItem`, `OverrideVerdict`
- Query RPCs (`GetBacklogItem`, `ListBacklogItems`, `GetBacklogItemDiff`,
  `GetBacklogItemCost`, `GetSessionBacklogIndex`) can stay in `backlog_service.go` or go in
  a `backlog_service_query.go` file

The `session/` package already has a fine-grained split by concern:
`backlog.go`, `backlog_lifecycle.go`, `backlog_review.go`, `backlog_triage.go`,
`backlog_sync.go`, `backlog_context.go`, `backlog_commands.go`, `backlog_crypto.go`,
`backlog_plugin.go`. This is the established model to follow.

---

## 2. Exported Symbol Audit: `backlog_service.go` and Blast Radius

`backlog_service.go` (2,570 lines) exposes the following exported symbols:

**Types / Interfaces (defined, then used elsewhere):**
| Symbol | Type | Callers outside file |
|--------|------|----------------------|
| `BacklogService` | struct | `server/dependencies.go`, `server/server.go` (wiring) |
| `SessionCreator` | interface | `server/services/session_service.go` (impl: `SpawnReviewSession`) |
| `AutonomousDriverStarter` | interface | `server/services/autonomous_orchestration_service.go` |
| `SessionStopper` | interface | `server/services/session_service.go` |
| `NewBacklogService` | func | `server/dependencies.go` |

**RPC methods (all on `*BacklogService`, called via ConnectRPC routing only — zero direct
Go callers outside `server/server.go` registration):**
All 25+ `*BacklogService` methods are registered as ConnectRPC handlers. There are no
direct Go call sites from other service files except the two auto-reopen methods:
- `AutoReopenAfterFailedReview` and `AutoReopenForPRFix` are called from
  `session/backlog_lifecycle.go` via the `AutoReopenSpawner` and `PRFixSpawner` interfaces
  (wired in `server/dependencies.go`)

**Conclusion:** Split blast radius is minimal. Moving methods between files within the same
package requires no import changes anywhere. The only external Go callers are:
- `server/dependencies.go` — wiring (only touches constructor and setters)
- `server/server.go` — ConnectRPC registration (only references the struct name)

---

## 3. P2: `mergeAcCriteria` — What to Extract

There is **no existing function named `mergeAcCriteria`**. The merge logic is inlined inside
`submitTriageResult` in `server/mcp/tools_backlog.go` (lines ~533–584). It:

1. Loads the existing item from storage
2. Builds a `map[int]AcCriterion` from the existing criteria
3. Applies incoming criteria (add or update by index)
4. Rebuilds an ordered `[]AcCriterion` and calls `session.SerializeAcCriteria`

This logic is ~50 lines and is used in **exactly one place** (the MCP tool handler). The
natural extraction is a function in the same file or in `session/backlog.go`:

```go
// MergeAcCriteria merges incomingCriteria into existing, updating matching indices
// and adding new ones. It never removes criteria not mentioned in incoming.
// Returns the serialized JSON result ready for BacklogItemUpdate.AcceptanceCriteria.
func MergeAcCriteria(existing []AcCriterion, incoming []AcCriterion) (AcCriteriaJSON, error)
```

Moving it to `session/backlog.go` is preferred because:
- It operates on `AcCriterion` / `AcCriteriaJSON` types that already live there
- The MCP handler would become a thin parser → call → serialize wrapper
- `server/services/backlog_service.go` has a similar `acCriteriaToJSON` helper (line 575)
  that also belongs there

**No callers outside the single MCP handler** currently — so the extraction is
low-risk, but creates a reuse point for future AC-editing surfaces.

---

## 4. P3: `ReviewGateRunner` / `spawnReviewGate` — Field Dependencies

`spawnReviewGate` is a method on `*BacklogLifecycleListener`. If extracted into a
standalone `ReviewGateRunner` type, these fields from `BacklogLifecycleListener` become
required constructor args:

| Field | Type | Role |
|-------|------|------|
| `storage` | `*Storage` | DB operations (worktree lookup, ItemSession CRUD, verdict writes, AC updates) |
| `headlessPool` (via `getHeadlessPool()`) | `*headless.Pool` | LLM-driven headless review path |
| `sessionCreator` | `ReviewGateSpawner` | Legacy tmux-session review path |
| `autoReopener` (via `getAutoReopener()`) | `AutoReopenSpawner` | Auto-reopen after FAIL/PARTIAL |
| `shutdownCtx` | `context.Context` | Lifetime context for the blocking review call |
| `pushAndCreatePR` (a method on the listener) | — | Called on PASS verdict |

`pushAndCreatePR` is tightly coupled to `BacklogLifecycleListener` (it does git push +
GitHub PR creation). A clean extraction requires either:
- Passing a callback `func(ctx, item, is)` as a constructor arg, or
- Keeping `ReviewGateRunner` as an embedded helper on the listener, not a standalone type

`ReviewGateSpawner` is already marked `Deprecated` in the code comment at line 20. The
headless path (`headless.Pool`) is the active code path. An extraction should fold the
legacy `sessionCreator` path into the constructor (nullable/optional) while the headless
path is the primary one.

**Import constraint:** `spawnReviewGate` uses `ent.BacklogItem` and `ent.ItemSession`
directly as parameters (line 379). Any extracted type in `session/` would still need
to import `session/ent`, so no import cycle savings from the extraction.

---

## 5. P4: `BacklogItemSummary` — List Callers and Field Usage

**`ListBacklogItems` (the only list call path):**
- `ent_repository_backlog.go:140` — queries with `.WithSource().All(ctx)` only. No eager
  load of `ItemSessions` or `StatusEvents` (those are only in `GetBacklogItem`).
- `backlogItemToData` in `ent_repository_backlog.go:22` maps all scalar fields; `ItemSessions`
  and `StatusEvents` fields in `BacklogItemData` stay **nil** for list results.

**`backlogItemToProto` (the serialization path):**
- Lines 509–534: the item session and status event blocks are guarded by `len(...) > 0`
  checks — so for list results they are always skipped.
- All other fields ARE used: `ID`, `Title`, `Description`, `Priority`, `Status`, `RepoPath`,
  `SkipReviewGate`, `SkipPlanning`, `PlanApproved`, `PlanArtifactsPath`, `Notes`,
  `ExternalID`, `SourceID`, `PrURL`, `PrNumber`, `CreatedAt`, `UpdatedAt`, `ArchivedAt`,
  `PlanApprovedAt`, `AcceptanceCriteria` (parsed for the proto).

**Fields NOT needed by list views (only populated in `GetBacklogItem`):**
- `ItemSessions []*ent.ItemSession`
- `StatusEvents []*ent.BacklogStatusEvent`

**Current `ent.Select()` usage:** None for backlog items — there is no existing column-
projection pattern in `ent_repository_backlog.go`. The entire row is always fetched.

**Implication for P4:** A `BacklogItemSummary` type would strip exactly two fields from
`BacklogItemData`: `ItemSessions` and `StatusEvents`. These are the only fields that carry
`ent.*` types, so removing them from the summary would also make `BacklogItemSummary`
free of the `session/ent` import — enabling it to live in a `session/domain` sub-package
cleanly (see P5 below).

However, the current `backlogItemToProto` function accepts `*BacklogItemData` and handles
the nil-check already. The ergonomic gain from a separate type depends on whether the
service layer needs to enforce at compile-time that list paths don't accidentally load
relations. The current behavior (nil-check at runtime) works correctly today.

**The only current non-trivial list callers:**
- `ListBacklogItems` RPC → `backlogItemToProto` (22 fields, nil relation check)
- `SuggestNextItem` (line 1925) → calls `ListBacklogItems` internally, only uses top item
- `TriggerTriage` (lines 1695, 1902) → calls `ListItemSessions` to check existing sessions,
  not `ListBacklogItems`

---

## 6. P5: `session/domain` Sub-Package — Import Cycle Analysis

**Files in `session/` that are candidates for `session/domain`** (pure domain types,
no infrastructure imports):

| File | Imports ent? | Imports headless? | Imports git? | Verdict |
|------|-------------|-------------------|--------------|---------|
| `session/backlog.go` | No | No | No | Safe to move |
| `session/types.go` | No | No | No | Safe to move |
| `session/repository.go` | **Yes** (`ent.ItemSession`, `ent.BacklogStatusEvent`, `ent.Shell`) | No | No | Blocked by ent in field types |
| `session/interfaces.go` | No | No | Yes (`session/git`) | Blocked by git import |

**The blocker:** `BacklogItemData` in `repository.go` holds:
```go
ItemSessions []*ent.ItemSession
StatusEvents []*ent.BacklogStatusEvent
```
And `ShellRepository` interface returns `*ent.Shell`. These types reference `session/ent`,
so `repository.go` (and by extension all callers of `BacklogItemData`) cannot move to a
sub-package without creating an import cycle: `session/domain` → `session/ent` is fine,
but `session` (which wraps `session/ent`) would need to import `session/domain`, creating
`session` → `session/domain` → `session/ent` → (potentially back to `session`).

**What CAN be moved to `session/domain` without import cycles:**

1. `backlog.go` types: `BacklogStatus`, `AcStatus`, `AcCriteriaJSON`, `AcCriterion`,
   `ReviewOutcome`, `CriterionVerdict`, `AggregateOutcome`, `ParseAcCriteria`,
   `SerializeAcCriteria`, and all related constants. These have no ent dependencies.

2. `types.go` types: `InstanceType`, `ExternalInstanceMetadata`, `InstancePermissions`,
   `DiscoveryMode`, `PTYDiscoveryConfig`, `ErrInvalidTransition`, `Workspace`, etc.
   No ent or headless dependencies.

3. `BacklogItemSummary` (proposed P4 type): If the `ItemSessions` and `StatusEvents`
   fields are stripped, the resulting struct has no ent dependencies and could live in
   `session/domain`.

**Import cycle risk for current importers:** 15+ packages import `session/` today. If
`session/domain` is created as a sub-package:
- All 15 packages that currently import `session` would need to also import `session/domain`
  for the moved types (or `session` re-exports them via type aliases)
- Type aliases (`type BacklogStatus = domain.BacklogStatus`) in `session/` would preserve
  backward compatibility for all existing callers with no code changes outside `session/`
- The re-export alias pattern is idiomatic and low-risk

**Packages that import `session/` for domain types only (no Storage/Instance):**
- `pkg/events` — uses `session.Instance`, `session.Status` (instance lifecycle types)
- `server/adapters` — converts `session.Instance` and `session.ReviewQueue` to proto

**Packages that import `session/` for infrastructure (Storage, Instance lifecycle):**
- `server/services/*` — all service files use `*session.Storage` and `*session.Instance`
- `server/mcp/*` — uses `session.Storage`, `session.Instance`, `session.InstanceStore`
- `daemon/`, `server/server.go`, `server/dependencies.go` — wiring

---

## 7. P6: Storage Interface Boundary Audit

**Current leakage of `ent.*` types through `session.Storage`'s public API:**

| Method | Returns `ent.*` | Called from |
|--------|----------------|-------------|
| `GetItemSession` | `*ent.ItemSession` | `backlog_service.go` (OverrideVerdict) |
| `GetItemSessionBySessionUUID` | `*ent.ItemSession` | `backlog_lifecycle.go` |
| `GetItemSessionBySessionAndItem` | `*ent.ItemSession` | `mcp/tools_backlog.go` |
| `ListItemSessions` | `[]*ent.ItemSession` | `backlog_service.go` (12 call sites) |
| `CreateItemSession` | `*ent.ItemSession` | `backlog_service.go`, `backlog_lifecycle.go` |
| `CreateItemSessionWithVerdict` | `*ent.ItemSession, *ent.ReviewVerdict` | `backlog_lifecycle.go` |
| `SaveReviewVerdict` | `*ent.ReviewVerdict` | `backlog_service.go` (OverrideVerdict) |
| `ListSourceSyncEvents` | `[]*ent.SourceSyncEvent` | `backlog_service.go` (GetSyncHistory) |

**`BacklogItemData` in `repository.go` also leaks ent:**
- `ItemSessions []*ent.ItemSession` — populated by `GetBacklogItem` only
- `StatusEvents []*ent.BacklogStatusEvent` — populated by `GetBacklogItem` only

**`backlog_service.go` imports `session/ent` directly** (line 21) to:
- Use `ent.IsNotFound(err)` at 12+ call sites
- Construct inline `*ent.BacklogItem` structs (lines 1281, 1607) for the `BacklogContext`
  builder (which needs the ent type for `BuildBacklogContext`)
- Call `triageShortTitle(sessions []*ent.ItemSession, ...)` (line 338)
- Map `ent.ItemSession` to proto via `itemSessionToProto`

**The core tension:** `session.Storage` bypasses the `Repository` interface for all
ItemSession methods — these are direct `EntRepository` delegation via type assertions
(`er, ok := s.repo.(*EntRepository)`). The `Repository` interface in `repository.go`
does NOT include any `ItemSession` CRUD methods — meaning those are intentionally
(or accidentally) outside the abstraction boundary.

**Recommendation for P6:** The `Repository` interface is missing 8 ItemSession + verdict
methods that are currently only accessible via concrete `*Storage`. Before splitting the
service file, introduce `ItemSessionData` as a domain struct (mirroring `BacklogItemData`)
to replace `*ent.ItemSession` return types — then add the methods to `Repository`. This
fixes the abstraction without changing behavior.

---

## Summary: Key Cross-Cutting Constraints

1. **`backlog_service.go` directly imports `session/ent`** for error detection
   (`ent.IsNotFound`), inline struct construction for `BuildBacklogContext`, and
   `triageShortTitle`. Any file split keeps this dependency unless the `ent.IsNotFound`
   calls are replaced with `errors.Is(err, session.ErrNotFound)` — which the code already
   does inconsistently (both patterns appear at line 677).

2. **`BacklogItemData.ItemSessions` carries `*ent.ItemSession`**, meaning the boundary
   between domain and infrastructure is already blurred in the most-used data type. P4
   and P6 are coupled: both need an `ItemSessionData` domain struct to untangle cleanly.

3. **`spawnReviewGate` calls `pushAndCreatePR`** (another method on the listener), so
   extracting `ReviewGateRunner` as a standalone type requires either: (a) accepting
   a callback/interface for post-PASS behavior, or (b) keeping the runner as an embedded
   helper on `BacklogLifecycleListener` rather than a fully independent type.
