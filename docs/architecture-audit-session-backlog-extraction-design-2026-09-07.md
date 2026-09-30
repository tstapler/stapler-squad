# Extraction Design: `session/backlog_lifecycle.go` → `session/backlog` (2026-09-07)

Targeted architecture review (`quality:architecture-review --target=file:session/backlog_lifecycle.go`,
principles=srp,dip,ddd,coupling) following on from
`docs/architecture-audit-session-package-split-2026-09-07.md`, which ranked this file the
single hottest in the `session` package (hotspot score 25,004; 2,185 lines; 94 revisions in
the last 1,500 commits). Read-only; no code changed.

## What the file actually looks like (not assumed — read/grepped directly)

`BacklogLifecycleListener` is spread across 8 files under one type, already split by
sub-concern — the file-split precedent this design extends:

| File | Lines | Methods | Concern |
|---|---|---|---|
| `backlog_lifecycle.go` | 2,185 | 71 | ~48 are boilerplate get/set accessor pairs for the 13 injected interfaces (`SetPRFixSpawner`/`getPRFixSpawner`, etc.); the remaining ~23 are the real orchestration surface: `onSessionStarted/Exited`, `ReconcileStuck`, escalation, `WireToInstance` |
| `backlog_lifecycle_pr.go` | 1,958 | 22 | PR creation/tracking/drift/push-failure remediation |
| `backlog_lifecycle_review.go` | 786 | 8 | Review-gate spawn, verdict processing, abandoned-review detection |
| `backlog_lifecycle_triage.go` | 398 | 3 | Orphaned-triage reconciliation |
| `backlog_lifecycle_stale.go` | 321 | 4 | Stale work-session / rework-block detection |
| `backlog_lifecycle_gates.go` | 152 | 2 | Custom transition-gate checks |
| `backlog_lifecycle_archive.go` | 128 | 2 | Terminal-item session archival |
| **Total** | **5,928** | **112** listener methods (verified: `grep -c '^func (l \*BacklogLifecycleListener)'`, matches the parent audit's 112 figure) | |

Note: the ~48 getter/setter pairs are exactly the boilerplate §4's sub-object split eliminates —
once `prHandler`/`reviewHandler`/etc. hold their own dependencies as plain fields, most of
`Set*`/`get*` collapses into direct field access or a constructor argument, not a method at all.

Good news the July/September hotspot audits didn't have visibility into: **DIP is already
mostly followed for collaborators.** 13 interfaces already exist as injection seams —
`Notifier`, `QueueDequeuer`, `ReviewGateSpawner`, `AutoReopenSpawner`, `PRFixSpawner`,
`StaleWorkRemediator`, `ReworkBlockStaleResolver`, `ReviewRespawner`, `TriageRespawner`,
`SessionArchiver`, `OneShotShipRunner`, `prPendingChecker`, `prCreator` (all defined in the
consumer files, per DIP). The struct's 22 fields are almost entirely these interfaces plus
their `sync.RWMutex` guards (concurrent Set/get, documented per-field) — this is not an
undisciplined God Object, it's a **God Object that already has clean edges to everything
except its two remaining concrete dependencies**: persistence (`*Storage`/`*EntRepository`)
and `*Instance`.

## 1. What moves where

**Stays in `session/domain`** (already there, zero internal imports, confirmed —
`domain/backlog.go:20-569`): `BacklogStatus`, `StuckReason`, `AcStatus`, `BacklogCategory`,
`AcCriteriaJSON`/`AcCriterion` + `Parse`/`Serialize`, `ReviewOutcome`, `CriterionVerdict` +
`AggregateOutcome`, `BacklogItemTransitionInput`, `CanTransitionBacklog`/`ValidTransitions`/
`TransitionGuard`. Nothing changes here — this package is the finished half of the migration.

**Moves to `session/backlog`** (new package — orchestration + DTOs):
- All 7 `backlog_lifecycle_*.go` files, minus the two `*Instance`-touching seams (see §3).
- `backlog_remediation.go` (401 lines) — `remediationDecision`, `evaluateRemediation`,
  `nextRemediationAt*`, and the `(s *Storage)` remediation methods (`RemediationDue`,
  `RemediationBlocked`, `Record/Reset/BulkResetStuckRemediation`,
  `RecordManualRemediationAttempt`) — pure backlog-stuck-state logic, no `*Instance`/tmux
  dependency (confirmed by grep — no `session/tmux` or `*Instance` import in this file).
- `backlog_review.go` (866 lines) — prompt builders (`BuildReviewPrompt`,
  `BuildHeadlessReviewPrompt`), verdict parsing (`ParseHeadlessVerdictResult`,
  `DegradeIfUnverified`), git-diff helpers (`GetGitDiff*`, `IsWorktreeDirty`,
  `RecoverBaseCommitSHA`). Only imports `session/git`, not `*Instance` — safe to move whole.
- `backlog_commands.go` (404 lines) — slash-command/context-file scaffolding
  (`WriteSlashCommands`, `WriteBacklogContextFile`, `selfHealWorktreeScaffolding`). Same
  profile: `session/git` import only, no `*Instance`.
- `backlog_sync.go` (596 lines) — `SyncLoop`/`NewSyncLoop`, the plugin-sync engine
  (`SyncOne` is the #5-hottest function in the package, cognitive 89). Takes `*Storage` and
  `*PluginRegistry` — both become the new repository interface (§2) and move with it.
- The 11 `Backlog*`/`ItemSession*` DTOs currently in `repository.go:57-588`
  (`BacklogItemDependencyEdge`, `BacklogItemData`, `BacklogItemSummary`, `BacklogItemFilter`,
  `BacklogItemUpdate`, `BacklogItemPrecondition`, `BacklogStatusEventData`,
  `ItemSessionSummary`, `ItemSessionBacklogEntry`). `ApprovalRuleData` and `AnalyticsData` in
  the same file are **not** backlog-specific (used by the approval-policy and analytics
  subsystems too — confirmed by name, not yet by call-site audit) — leave them in `session`
  for now; don't fold them into `session/backlog` speculatively.
- `backlog.go` (244 lines) minus `IsBacklogOriginatedSession` (§3 — stays, touches
  `*Instance`) — the rest are pure type aliases to `domain.*` plus `MergeAcCriteria`,
  `IsValidBacklogCategory`, `HasActiveJulesSession`.

**Becomes an adapter package** (`session/backlog/entadapter` or similar): the ~90
backlog-related methods currently on `*EntRepository`, split across `ent_repository_backlog.go`
(51 methods: `CreateBacklogItem`, `ListBacklogItems`, `UpdateBacklogItem`,
`TransitionBacklogItemStatus`, `MarkStuck`/`ResolveStuck`, item-source sync methods, etc.) and
`storage_backlog.go` (also all on `*EntRepository` despite the filename — 40 methods:
`CreateItemSession`, `SaveReviewVerdict`, `FindStuckReviewItems`, `FindPRPendingItems`, etc.).
**`storage.go:259-261` confirms `Storage` is a bare `{repo *EntRepository}` wrapper** — it adds
no logic of its own for backlog paths; `BacklogLifecycleListener` code reaches through it via
`l.storage.repo` (e.g. `backlog_lifecycle.go:956`) specifically *because* there's no exported
accessor, which is itself evidence the `Storage` indirection isn't earning its keep on this
path. The extraction should let `session/backlog`'s handlers depend on the adapter directly
(via the port interfaces in §2), not reach through `Storage` at all.

**Stays in `session`** (genuinely `*Instance`/tmux-coupled, per §3):
`instanceBacklogListener`/`WireToInstance` (backlog_lifecycle.go:708-738),
`IsBacklogOriginatedSession` (backlog.go:107), the concrete `ReviewGateSpawner` production
implementation (wherever `SpawnReviewSession` is actually implemented — outside this file,
likely `server/services` or a `session` glue file; not read in this pass, flag for the
implementer to locate before PR4).

## 2. Repository interface design

Given ~90 methods, one flat interface violates ISP and just relocates the God Object
problem into an interface. Instead, **each sub-handler defines its own narrow repository
interface in its own file**, structural typing satisfies all of them from one concrete
adapter — this mirrors the pattern the file split already established for spawners.

Representative example (not exhaustive — the implementer enumerates the full call-site list
per handler when doing the move):

```go
// session/backlog/pr.go
type prRepository interface {
    FindPRPendingItems(ctx context.Context) ([]*ent.BacklogItem, error)
    FindDriftedPRItems(ctx context.Context) ([]*ent.BacklogItem, error)
    TransitionBacklogItemStatusWithPRFields(ctx context.Context, id string, toStatus domain.BacklogStatus,
        prURL string, prNumber int, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error)
    BackfillMissingPRNumbers(ctx context.Context) (int, error)
    // ... the remaining ~15 PR-specific methods reconcilePRPendingItem/ReconcilePRPending/etc. call
}

// session/backlog/review.go
type reviewRepository interface {
    FindStuckReviewItems(ctx context.Context) ([]*ent.BacklogItem, error)
    FindReviewItemsWithUnprocessedVerdict(ctx context.Context) ([]*ent.BacklogItem, error)
    SaveReviewVerdict(ctx context.Context, itemSessionID string, verdict ReviewVerdictData) error
    // ...
}

// session/backlog/repository.go — the composed port the constructor takes,
// satisfied entirely by entadapter.Repository (one struct, one *ent.Client)
type Repository interface {
    prRepository
    reviewRepository
    staleWorkRepository
    triageRepository
    gateRepository
    archiveRepository
    CoreRepository // onSessionStarted/Exited, BackfillStuckStates' direct calls
}
```

`session/backlog/entadapter.Repository` (concrete, wraps `*ent.Client` — not `*EntRepository`,
since `EntRepository` itself is a 143-method God Object per the parent audit and re-wrapping
it would just move the coupling one hop) implements `Repository` by lifting the existing
method bodies from `ent_repository_backlog.go`/`storage_backlog.go` verbatim (mechanical move,
not a rewrite — this PR sequence is about boundaries, not behavior change). `session`
package's remaining `*EntRepository` keeps the non-backlog methods (Instance CRUD,
ApprovalRule, Analytics, ItemSource-adjacent-but-shared bits if any).

## 3. What crosses the boundary, and how the import cycle is avoided

`session` will need to import `session/backlog` (to construct `BacklogLifecycleListener` in
its wiring code) — so `session/backlog` **must never import `session`**, or it's a cycle.
Two concrete places in the current code would break this if moved naively:

1. **`ReviewGateSpawner.SpawnReviewSession(ctx, item, itemSessionID, prompt) (*Instance, error)`**
   (`backlog_lifecycle_review.go:18`). Fix: change the interface's return type to a narrow
   local type defined in `session/backlog` itself:
   ```go
   // session/backlog: defined here, not in session
   type SpawnedSession interface { UUID() string }
   type ReviewGateSpawner interface {
       SpawnReviewSession(ctx context.Context, item *BacklogItemData, itemSessionID, prompt string) (SpawnedSession, error)
   }
   ```
   `*session.Instance` already has (or gets) a trivial `UUID() string` method satisfying this
   structurally — `session` package's concrete spawner implementation returns `*Instance`
   unchanged, Go's structural typing does the rest. No behavior change, no cast needed at the
   call site since `BacklogLifecycleListener` only ever used the returned value's identity,
   never other `*Instance` methods (confirmed: `spawnReviewGate` at
   `backlog_lifecycle_review.go:367` doesn't touch the returned Instance).

2. **`WireToInstance(inst *Instance)` / `instanceBacklogListener`** (`backlog_lifecycle.go:708-738`).
   This concretely needs `*Instance` to call `inst.RegisterLifecycleListener(...)` and read
   `inst.UUID`. It **stays in `session`**, not `session/backlog`, as a thin adapter file (e.g.
   `session/backlog_wiring.go`, ~35 lines) that imports `session/backlog` and does:
   ```go
   func WireBacklogListener(l *backlog.Listener, inst *Instance) {
       inst.RegisterLifecycleListener(&instanceBacklogListener{parent: l, instanceUUID: inst.UUID})
   }
   ```
   `instanceBacklogListener.OnLifecycleEvent` calls back into `l.parent`'s exported
   `OnSessionStarted(sessionUUID string)`/`OnSessionExited(sessionUUID string)` methods (renamed
   from today's unexported `onSessionStarted`/`onSessionExited` since they cross a package
   boundary now) — `session/backlog` never sees `*Instance`, only string UUIDs, which is all
   it ever actually used (confirmed: `onSessionStarted`/`onSessionExited` bodies at
   `backlog_lifecycle.go:741,757` take `sessionUUID string`, not an Instance reference).

No other `*Instance` coupling exists in the backlog cluster — confirmed by grep across every
`backlog_lifecycle*.go`/`backlog_remediation.go`/`backlog_sync.go`/`backlog_review.go`/
`backlog_commands.go`: the only two hits are the ones above and `backlog.go:107`'s
`IsBacklogOriginatedSession`, which stays in `session` as a method on `*Instance` (it's an
`Instance` query, not backlog orchestration, despite living in a `backlog.go`-named file today).

## 4. God Object triage: composed sub-objects, not one flat struct

The file split already IS the natural grouping — turning it into composed sub-objects (each
holding only the dependencies it needs) is mechanical, not a redesign:

| Sub-object | Absorbs | Dependencies it alone needs |
|---|---|---|
| `prHandler` | `backlog_lifecycle_pr.go`'s 22 methods | `PRFixSpawner`, `OneShotShipRunner`, `prPendingCheckerFactory`, `prCreatorFactory`, `orphanedPRFinder`, `prByNumberFinder`, `branchReconciler` — **7 of the struct's 22 fields, currently all sitting on the flat top-level type though nothing outside `backlog_lifecycle_pr.go` reads them** |
| `reviewHandler` | `backlog_lifecycle_review.go`'s 8 methods | `sessionCreator` (`ReviewGateSpawner`), `headlessPool`, `autoReopener`, `reviewRespawner`, `runner` (`*ReviewGateRunner`), `reviewSem` |
| `staleWorkHandler` | `backlog_lifecycle_stale.go`'s 4 methods | `staleWorkRemediator`, `reworkBlockStaleResolver` |
| `triageHandler` | `backlog_lifecycle_triage.go`'s 3 methods | `triageRespawner` |
| `gateChecker` | `backlog_lifecycle_gates.go`'s 2 methods | `gateSatisfactionRepo`, `workflowEngine` |
| `archiveHandler` | `backlog_lifecycle_archive.go`'s 2 methods | `sessionArchiver` |
| `Listener` (top-level, ~15 methods remain) | `backlog_lifecycle.go`'s core: `onSessionStarted/Exited`, `ReconcileStuck` (dispatches to every sub-object's `reconcile*`), `BackfillStuckStates`, `WireToInstance`-facing exported methods | `notifier`, `dequeuer`, `chainReconciler`, `pipelineEngine`, `livenessEngine`, the 6 sub-objects above |

This directly resolves the ISP violation the flat struct currently has: today, wiring code
that only needs to set a `PRFixSpawner` still depends on the full `BacklogLifecycleListener`
type with all 22 fields; after this, `prHandler` is independently constructible and testable
without the other 6 concerns.

## 5. Incremental PR sequence

Six PRs, each independently shippable and behavior-preserving (mechanical moves + narrowing
casts, no logic changes) — order chosen so nothing downstream depends on something not yet
moved:

1. **DTOs + domain aliases.** Move the 9 `Backlog*`/`ItemSession*` DTOs from `repository.go`
   into `session/backlog/types.go`. Leave `type BacklogItemData = backlog.BacklogItemData`
   etc. as aliases in `session` so every existing call site in `session`/`server` keeps
   compiling unchanged. Zero behavior change, pure mechanical move + alias.
2. **Pure/leaf files.** Move `backlog_review.go`, `backlog_commands.go`,
   `backlog_remediation.go` into `session/backlog` verbatim (confirmed zero `*Instance`
   coupling in all three). These have no repository-interface work yet since
   `backlog_remediation.go`'s `(s *Storage)` methods move as free functions or a small
   `remediation` sub-type taking the new `Repository` port (see PR3).
3. **Define the repository ports + adapter.** Create `session/backlog.Repository` (composed
   from the per-handler interfaces in §2) and `session/backlog/entadapter.Repository`
   (concrete, wraps `*ent.Client`, lifts method bodies from `ent_repository_backlog.go` +
   `storage_backlog.go`). Wire `backlog_sync.go`'s `SyncLoop` against the new interface (it's
   already a clean unit — `*Storage`/`*PluginRegistry` become `Repository`/`*PluginRegistry`).
   `session`'s `*EntRepository` keeps its non-backlog methods; the backlog methods move
   wholesale into `entadapter`.
4. **Sub-object split, non-PR handlers.** Introduce the `gateChecker`/`staleWorkHandler`/
   `triageHandler`/`archiveHandler`/`reviewHandler` sub-objects inside `session/backlog`
   (§4), move their source files, resolve the `SpawnReviewSession` cycle via the
   `SpawnedSession` interface (§3.1). `BacklogLifecycleListener`'s top-level struct in
   `session` becomes a thin re-export or is renamed/moved to `backlog.Listener` — pick
   whichever keeps `server/dependencies.go`'s construction call site smallest to change.
5. **PR handler.** Move `backlog_lifecycle_pr.go` (the largest single file, 1,958 lines) into
   `session/backlog` as `prHandler`, plus the remaining core-orchestration methods from
   `backlog_lifecycle.go`. Move `WireToInstance`/`instanceBacklogListener` to the new
   `session/backlog_wiring.go` shim (§3.2) — this is the PR where `session/backlog_lifecycle*.go`
   is fully deleted from `session`.
6. **Lock the boundary.** Per `golang-depguard-architecture`'s incremental-adoption pattern:
   baseline `golangci-lint run --enable-only depguard ./session/...` (should be near-zero
   violations by now since the move is complete), add a `session-backlog-no-ent-direct`
   deny-list rule (blocks `session/backlog/**` from importing `entgo.io/ent` or
   `session/ent` directly — only `entadapter` may), and a `session-backlog-no-session-cycle`
   rule (`session/backlog/**` may not import `.../session` itself) so the cycle broken in
   PR4/5 can't silently regress. Promote both to blocking once `golangci-lint` reports zero
   violations under the new rules. Re-run the parent hotspot audit's temporal-coupling pass
   afterward to confirm `backlog_lifecycle.go`'s score actually drops rather than shifting
   unchanged onto whatever absorbed it.

## Next steps

- Locate the concrete `ReviewGateSpawner` implementation (not in this file — likely
  `server/services`) before starting PR4, to confirm the `SpawnedSession` interface change
  doesn't ripple further than expected.
- Audit `ApprovalRuleData`/`AnalyticsData` call sites before deciding whether they ever
  belong in `session/backlog` — flagged as "probably not" here but not verified by call-site
  read in this pass.
- `Instance` itself remains a separate follow-up per the parent audit — this design doesn't
  touch it beyond the two narrow seams in §3.
