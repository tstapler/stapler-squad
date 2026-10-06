# Architecture Audit: Splitting the `session` Package (2026-09-07)

Hotspot analysis (`code-hotspot-analysis` skill: static coupling + git-history temporal
coupling → complexity × churn) scoped specifically to the top-level `session` package,
to prioritize an eventual package split. Read-only; no code changed.

**Builds on, does not repeat**: `docs/architecture-audit-2026-07-01.md` (repo-wide
static pass — already confirmed the `Instance` God Object and the `session/` sprawl,
but explicitly deferred both as "no action needed on sub-package structure") and
`docs/architecture-audit-temporal-coupling-2026-07-01.md` (repo-wide churn pass,
focused mainly on `server/services/session_service.go`). This pass goes one level
deeper: given the decision to split the flat top-level `session` package, *which*
files go *where*.

## Scope

- Top-level `session` package only: 200 non-test `.go` files, **206,532 lines**
  (+198 test files, 74,818 lines). The ~30 existing subpackages (`session/tmux`,
  `session/git`, `session/detection`, `session/queue`, `session/domain`, etc.) are
  already reasonably decomposed leaves — out of scope, confirmed below.
- Tools: `mcp__kibitzer__architecture_assessment` (759 findings across 397 files),
  `gocyclo`/`gocognit` (real complexity numbers, not just line-count heuristics),
  `ast-grep` (struct field counts), `goda` (package-level fan-in/reach), and a
  self-contained temporal-coupling script (code-maat's algorithm) over the last
  1,500 commits (~2026-07-02 to today) scoped to `session/`.

## 1. Static coupling — clean external boundary, huge internal surface

```
goda list "incoming(github.com/tstapler/stapler-squad/..., .../session)"  → 19 dependents
goda list "reach(.../session/..., .../server/...)"                        → empty (no violation)
goda list "reach(.../session/..., .../daemon/...)"                        → empty (no violation)
```

Good news: `session` does not reach into `server` or `daemon` — the *external*
layering (CLAUDE.md's Web Server → Session Mgmt → Config) is intact, unchanged since
the July audit (still exactly 20 packages including itself). This means a split is
safe to do incrementally — no upstream layering rule is currently being violated that
a split would need to fix first, only internal cohesion.

`session/*.go` itself already imports 25+ of its own subpackages (`git`, `tmux`,
`detection`, `queue`, `lifecycle`, `domain`, …) — the package-by-feature pattern this
split should extend is already the codebase's own established convention, not a new
style being introduced.

### God Objects (method + struct-body size, ast-grep + grep)

| Type | Methods | Struct body | Primary file |
|---|---|---|---|
| `Instance` | **306** (283 in `instance*.go` alone) | 520 lines | `session/instance.go` |
| `EntRepository` | **143** | — | `session/ent_repository.go` |
| `Storage` | **126** | 146-line `InstanceData` sibling | `session/storage.go` |
| `BacklogLifecycleListener` | **112** | 177 lines | `session/backlog_lifecycle.go` |

(July's audit counted `Instance` at 95 *fields* by a different method; method count
— the more relevant number for "how much does one type do" — wasn't measured then.)

## 2. Complexity — real gocyclo/gocognit, not line-count proxies

`session`-scoped cyclomatic average is 3.71 (`gocyclo -avg session/*.go`, non-test;
whole-repo average is 3.28), so every function below is a genuine outlier, not a
style artifact:

| Function | Cyclomatic | Cognitive | File:line |
|---|---|---|---|
| `(*EntRepository).Update` | 70 | **126** | `ent_repository.go:459` |
| `(*EntRepository).UpdateBacklogItem` | 53 | 55 | `ent_repository_backlog.go:876` |
| `(*EntRepository).Create` | 52 | 73 | `ent_repository.go:232` |
| `(*ReviewQueuePoller).checkSession` | 49 | **100** | `review_queue_poller.go:768` |
| `fromInstanceData` | 47 | 88 | `instance_serialization.go:224` |
| `(*BacklogLifecycleListener).reconcilePRPendingItem` | 47 | 77 | `backlog_lifecycle_pr.go:1287` |
| `startLocked` | 40 | 93 | `instance.go:1257` |
| `(*SyncLoop).SyncOne` | 40 | 89 | `backlog_sync.go:291` |
| `(*Instance).start` | 38 | 89 | `instance.go:1503` |
| `(*ReviewQueuePoller).reconcileSessions` | 26 | 92 | `review_queue_poller.go:450` |

kibitzer's line-count heuristic agrees at the file level (longest bodies:
`instance_serialization.go:224` at 372 lines, `ent_repository.go:459` at 324, and 9
files over 150 lines in one function), and separately flags `instance_checkpoint.go:42`
(`createCheckpointLocked`) at **9 levels of nesting** — the deepest in the package.

## 3. Temporal coupling — two dominant, already-partially-separated clusters

1,500-commit window, `session/` scoped, no bot noise (100% human-authored in-window).

### Top revision-count files (churn)

| Revisions | File |
|---|---|
| 94 | `backlog_lifecycle.go` |
| 58 | `storage.go` |
| 51 | `ent_repository_backlog.go` |
| 43 | `instance.go` |
| 43 | `repository.go` |
| 39 | `tmux/tmux.go` |
| 35 | `storage_backlog.go` |
| 32 | `ent_repository.go` |
| 30 | `instance_tmux.go` |
| 24 | `instance_worktree.go`, `backlog_commands.go` |

### Co-change clusters (ratio = shared ÷ min(revisions))

- **Backlog cluster** (dominant): `backlog_lifecycle.go` ↔ `backlog_lifecycle_test.go`
  (40 shared, ratio 0.83), `ent_repository_backlog.go` ↔ `repository.go` (32, 0.74),
  `ent_repository_backlog.go` ↔ `storage.go` (27, 0.53), `backlog_lifecycle.go` ↔
  `storage_backlog.go` (19, 0.54). Also pulls in `domain/backlog.go` (23 revisions,
  0 internal imports — already a clean leaf) and `ent/schema/backlog_item.go`.
- **Instance/actor cluster**: `instance.go`, `instance_tmux.go`, `instance_worktree.go`,
  `instance_serialization.go`, `instance_claude.go`, `instance_checkpoint.go`,
  `instance_actor_setters.go` — confirms, doesn't extend, July's finding that the
  `instance*.go` file split spread the God Object's churn across files without
  decoupling the type itself (`instance-lock-free-reads.md`'s Snapshot() pattern is
  the one place this coupling is already being actively managed).
- **Review cluster** (smaller, semi-independent): `review_gate.go`,
  `review_queue_poller.go`, `review_queue_determiner.go`, `review_state.go`.
- Generated-code pairs (`ent/migrate/schema.go` ↔ `ent/mutation.go` ↔ `ent/runtime.go`,
  ratio 0.91) are expected entgo regeneration noise, kept only as confirmation.

### A half-finished migration already points at the answer

`session/domain/backlog.go` (649 lines, **zero internal imports** — a clean leaf) is
already imported by 12 files in the flat package (`backlog_lifecycle*.go`,
`storage*.go`, `ent_repository_backlog.go`, …). Someone already started extracting
backlog domain types out of `session` into `session/domain`, then stopped: the domain
types moved, but the 8,000+ lines of orchestration logic that operates on them
(`backlog_lifecycle.go` alone is 2,185 lines and the single hottest file in the
package) never did. This is the strongest signal in the whole analysis for *where*
the first package boundary should go — it's not a new design, it's finishing one
already in flight.

## 4. Cross-referenced hotspot ranking (revisions × cognitive-complexity sum)

| Rank | Score | File | Cluster |
|---|---|---|---|
| 1 | 25,004 | `backlog_lifecycle.go` | Backlog |
| 2 | 21,981 | `ent_repository_backlog.go` | Backlog (persistence) |
| 3 | 16,469 | `instance.go` | Instance |
| 4 | 11,840 | `ent_repository.go` | Instance (persistence) |
| 5 | 7,714 | `storage.go` | Instance (persistence) |
| 6 | 5,460 | `storage_backlog.go` | Backlog (persistence) |
| 7 | 4,050 | `instance_tmux.go` | Instance |
| 8 | 3,234 | `backlog_lifecycle_pr.go` | Backlog |
| 9 | 3,196 | `session_driver.go` | Instance (orchestration) |
| 10 | 3,179 | `review_queue_poller.go` | Review |
| 11 | 3,080 | `backlog_review.go` | Backlog |
| 12 | 2,904 | `instance_worktree.go` | Instance |
| 13 | 1,824 | `review_gate.go` | Review |
| 14 | 1,656 | `instance_serialization.go` | Instance |
| 15 | 1,056 | `backlog_commands.go` | Backlog |

Backlog-cluster files sum to **~60%** of top-15 hotspot score (and 28,999 raw lines,
vs. Instance-cluster's 17,796) despite having a head start on extraction via
`session/domain`. `repository.go` (43 revisions, complexity sum 2) is a **coupling
hub, not a complexity hotspot** — it's the shared `BacklogItemData`/`InstanceData`
type-definition file every persistence path imports, exactly the "shared types.go
imported by every layer" anti-pattern `code-golang-architecture` calls out.

## Recommended split (package-by-feature, per `code-golang-architecture`)

Two bounded contexts already exist in practice (confirmed by both clusters above);
Go's package-as-boundary model + the codebase's own existing subpackage convention
both point at vertical slices, not a layer split:

1. **`session/backlog`** — highest-value target. Absorbs `backlog_lifecycle*.go`,
   `backlog_review.go`, `backlog_commands.go`, `backlog_sync.go`, `backlog.go`,
   `storage_backlog.go`, `ent_repository_backlog.go`, and the `Backlog*` types
   currently in `repository.go`. Finishes the migration `session/domain/backlog.go`
   already started — domain types stay in `session/domain`, orchestration moves to
   `session/backlog`, persistence adapter (ent-backed) implements a repository
   interface defined by the consumer, per `golang-depguard-architecture`'s
   `domain-is-pure` pattern.
2. **`session/review`** — smaller, cleaner cut: `review_gate.go`,
   `review_queue_poller.go`, `review_queue_determiner.go`, `review_state.go`. Low
   risk, good first PR to prove the split pattern before tackling Instance.
3. **`Instance` itself is the hard problem, not a file move.** July's audit already
   found file-splitting `instance*.go` didn't decouple the actor-model type — 306
   methods on one struct behind one mutex/snapshot. Extracting a `session/instance`
   package only relocates the God Object; it doesn't shrink it. This needs
   `golang-structs-interfaces`' composed-sub-object treatment first (separate
   `TmuxSession`/`WorktreeState`/`ApprovalState`/`CheckpointState` types the actor
   composes) — treat as its own follow-up analysis, not part of this split.
4. **Persistence** (`ent_repository.go`, `storage.go`) splits along the same seam as
   #1/#3 once their domain packages exist — each becomes the adapter implementing
   that domain's repository port, not a separate `session/persistence` package (that
   would just recreate a cross-cutting `types.go` problem one level up).

## Next steps

- Hand `session/backlog_lifecycle.go` and `session/instance.go` specifically to
  `architecture-review` (`--target=file:...`, not a full-codebase sweep) for the
  principle-level extraction design.
- Once `session/backlog` exists, lock the boundary with `golangci-lint`'s `depguard`
  (`golang-depguard-architecture` Steps 0–2: `domain-is-pure` allow-list on
  `session/domain/**`, deny-list keeping `session/backlog` off `session/ent` directly
  — route through a repository interface instead) so the next contributor can't
  silently recreate the coupling this audit found.
- Re-run this same hotspot pass after the `backlog` split lands — the goal is the
  ranking measurably drops for backlog files and doesn't just shift the same score
  onto whatever absorbs them.
