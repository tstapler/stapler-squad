# Implementation Plan: unified-vcs-widget

**Feature**: One shared `VcsWidget` component (full/compact density) unifies VcsPanel, Backlog item detail's Version Control section, and Unfinished item detail; backend durably snapshots GitHub PR/CI/review state and per-file diff stats at ship time so "done" items keep full richness after worktree cleanup.
**Date**: 2026-07-18
**Status**: Ready for implementation
**ADRs**: [docs/adr/ADR-024-no-new-diff-rendering-library.md](../../../docs/adr/ADR-024-no-new-diff-rendering-library.md) (no new diff-rendering library), [docs/adr/ADR-025-ship-snapshot-fields-on-backlog-item.md](../../../docs/adr/ADR-025-ship-snapshot-fields-on-backlog-item.md) (ship-snapshot fields on `BacklogItem`, not a new entity), [docs/adr/ADR-026-mergeability-state-synthesis.md](../../../docs/adr/ADR-026-mergeability-state-synthesis.md) (client-side `MergeabilityState` synthesis + closed/merged bug fix). Project-local copies remain at `project_plans/unified-vcs-widget/decisions/ADR-00{1,2,3}-*.md` for planning-history reference.

---

## Step 0.5 — Alternatives Considered (Creative Pass)

Three high-level shapes were evaluated for how the three surfaces get unified:

| # | Approach | Strength | Weakness |
|---|---|---|---|
| A | **Single `VcsWidget` component + 3 plain adapter functions** (`fromSessionVcs`, `fromShipStatus`, `fromUnfinishedWorktree`) normalizing into one `VcsWidgetData` shape, composed from small stateless sub-components | Matches the codebase's existing `GitHubBadge`-style `compact` convention exactly (features research §1); adapters are pure functions, trivially unit-testable in isolation from any component | A single component absorbing all three data regimes' rendering paths risks becoming the "God Component" pitfalls research warns about (§1) unless deliberately decomposed into sub-components up front |
| B | **Three thin wrapper components** (`SessionVcsWidget`, `BacklogVcsWidget`, `UnfinishedVcsWidget`), each composing shared sub-widgets (`FileList`, `CommitList`, `MergeabilityPill`) directly, with no single top-level `VcsWidget` | Naturally sidesteps the God Component risk — no file ever owns all three data-fetch/rendering concerns at once | Directly contradicts the requirement's explicit ask for "one shared component reused across all three surfaces" (Alternatives Considered in requirements.md); any cross-cutting UI change (e.g. the mergeability pill's color scheme) must be made in three places instead of one |
| C | **Backend-driven single normalized RPC** (`GetVcsWidgetData`) that server-side-normalizes all three data sources into one shape, replacing `useSessionVcs`/`useBacklogItemShipStatus`/`useUnfinishedWork` with one hook | Eliminates the three-different-fetch-shape problem entirely — frontend gets one trivially composable shape from one call | Live session polling (`SessionVcsContext`, 60s fallback poll paused on `document.hidden`) and Unfinished work's server-streaming RPC (`WatchUnfinishedWork`) cannot be replaced by a single request/response RPC without abandoning two already-working, differently-timed data planes — this is an unbounded backend rearchitecture that stack research explicitly argues against ("normalize at the hook layer, not force one fetch strategy") |

**Chosen: Approach A**, with the God Component risk from Step 4 of pitfalls research mitigated explicitly by decomposing `VcsWidget` into `MergeabilityPill`, `VcsWidgetHeader`, `VcsWidgetFileList`, `VcsWidgetCommitList`, `VcsWidgetGithubRow` sub-components from the first implementation pass (Phase 2 below), not as a later refactor.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `VcsWidget` | The single shared React component (`web-app/src/components/shared/VcsWidget.tsx`) rendering unified VCS/PR/CI/diff state for a session, backlog item, or unfinished worktree. | Replaces `VcsPanel`'s bespoke rendering, composes `VcsStatusDisplay`'s successor. |
| `VcsWidgetMode` | The `"full" \| "compact"` density prop on `VcsWidget`, mirroring `GitHubBadge`'s `compact?: boolean` convention. | `full` = VcsPanel-equivalent; `compact` = UnfinishedItemDetail-equivalent. |
| `VcsWidgetData` | Discriminated union `{ kind: "live"; ... } \| { kind: "historical"; snapshotAt: Date \| null; snapshotCaptureFailed?: boolean; ... }` in `web-app/src/lib/vcs/types.ts` — the one normalized shape all three data sources adapt into. | No optional-field explosion beyond what's genuinely conditional per source; per-source aggregate stats are grouped under `aggregateStats?` rather than 3 separate top-level optionals. The `kind` discriminant makes an `isLive`/`snapshotAt` mismatch a compile error, not a convention (architecture-review Concern). |
| `aggregateStats` | Optional nested object `{ filesChanged: number; additions: number; deletions: number }` on `VcsWidgetData`, replacing 3 separate top-level optional fields (`aggregateFilesChanged?`/`aggregateAdditions?`/`aggregateDeletions?`). | Populated only by `fromUnfinishedWorktree` (and any future source with aggregate-only stats, no per-file breakdown) — grouped per the "no field explosion" design goal (architecture-review Concern). |
| `FileChangeSummary` | Normalized per-file change record (`path`, `status`, `additions`, `deletions`, `section`) unifying `FileChange` proto and `UnfinishedWorktree`'s aggregate fields. | `section` is `"conflict" \| "staged" \| "unstaged" \| "untracked"`; compact mode ignores it. |
| `CommitSummary` | Normalized commit record (`sha`, `summary`, `authorName?`, `authoredAt?`), the target shape for both `ShippedCommit` (proto) and `UnfinishedWorktree.aheadCommitMessages` (plain strings, upgraded on adaptation). | |
| `GithubSummary` | Normalized GitHub PR/CI/review state (`owner`, `repo`, `prUrl`, `prNumber`, `prState`, `isDraft`, `checkConclusion`, `approvedCount`, `changesReqCount`) or `null` when no PR ever existed. | `prState: "open" \| "closed" \| "merged"` and `checkConclusion: "success" \| "failure" \| "pending" \| ""` are closed literal unions (not raw `string`), mirroring the exact set `github/priority.go` and `Session.githubCheckConclusion` produce today (architecture-review Concern). `checkConclusion` never carries a capture-failure sentinel — see `ShippedSnapshotCaptureFailed`. |
| `MergeabilityState` | The synthesized single-token union type (`"draft" \| "conflicted" \| "changes_requested" \| "ci_failing" \| "ci_pending" \| "ready_to_merge" \| "shipped" \| "closed_unshipped" \| "snapshot_unavailable" \| "no_pr"`) combining CI + review + conflict + shipped + capture-failure signals into one glanceable pill. | See ADR-003 (ADR-026 in `docs/adr/`). `"snapshot_unavailable"` is the explicit branch for a durable-snapshot capture failure (Story 3.3.1), checked before `ci_pending`/`no_pr` so a capture failure never silently reads as "CI still running" or "no PR ever existed" (architecture-review BLOCKER fix). New concept — none of the 3 existing surfaces has this today. |
| `deriveMergeabilityState` | Pure function `(data: VcsWidgetData) => MergeabilityState` in `web-app/src/lib/vcs/mergeability.ts`. | Shipped status always wins over a stale "merged"/"closed" GitHub state — the ADR-003 bug fix. |
| `fromSessionVcs` | Adapter function `(status: VCSStatus, session?: Session) => VcsWidgetData` normalizing the live session VCS path. | Plain function, no class/interface (interface-pollution-checklist smell #1). |
| `fromShipStatus` | Adapter function `(status: BacklogItemShipStatus) => VcsWidgetData` normalizing the durable/historical backlog path. | |
| `fromUnfinishedWorktree` | Adapter function `(wt: UnfinishedWorktree) => VcsWidgetData` normalizing the Unfinished-work compact path. | |
| `kind` | Discriminant on `VcsWidgetData`: `"live"` when data can change without a page refresh (live worktree/session, no `snapshotAt` field at all); `"historical"` for a durable snapshot (carries `snapshotAt` and optionally `snapshotCaptureFailed`). | Replaces the former `isLive: boolean` + always-present `snapshotAt` flat shape — the discriminated union makes an illegal `kind: "live"` + non-null `snapshotAt` combination unrepresentable, not just convention-enforced (architecture-review Concern). Drives "as of" copy and suppresses the refresh button when `kind === "historical"`. |
| `snapshotAt` | `Date \| null`, present only on the `kind: "historical"` branch of `VcsWidgetData`; set when a captured-at timestamp exists. | Presence of a non-null value *is* the "has a snapshot" check — no separate boolean (type-driven design: avoid the redundant-flag smell). Absent entirely (not just `null`) on the `"live"` branch — a structural guarantee, not a runtime convention. |
| `MergeabilityPill`, `VcsWidgetHeader`, `VcsWidgetFileList`, `VcsWidgetCommitList`, `VcsWidgetGithubRow` | The 5 stateless sub-components `VcsWidget` composes, each owning one section's rendering logic. | Decomposition that prevents the God Component / ternary-sprawl failure mode (pitfalls research §1). |
| `ShippedFileStat` | New proto message (`proto/session/v1/backlog.proto`) — `path`, `status` (`FileStatus` enum, reused from `types.proto`), `additions`, `deletions` — the durable per-file diff-stat snapshot record. | Mirrors `FileChange`'s field shape so the proto↔ent mapping stays mechanical. |
| `ShippedCheckConclusion`, `ShippedApprovedCount`, `ShippedChangesReqCount`, `ShippedSnapshotAt`, `ShippedFileStats`, `ShippedSnapshotCaptureFailed` | The 6 new optional fields on the `BacklogItem` ent schema (`session/ent/schema/backlog_item.go`) holding the durable GitHub/diff-stat snapshot. | See ADR-002 (ADR-025 in `docs/adr/`). `ShippedFileStats` is a JSON-blob string column, not a child table. `ShippedCheckConclusion` holds only genuine GitHub CI-conclusion values (or is left unset) — it is never repurposed as a failure sentinel; `ShippedSnapshotCaptureFailed` is the dedicated boolean for that. |
| `FileStatsBetween` | New Go helper `func FileStatsBetween(repoPath, baseSHA, headSHA string) ([]FileStat, error)` in `session/git/ops.go`, built on go-git's `object.Patch().Stats()`. | Per `.claude/rules/prefer-go-git-over-subshells.md`; the new "no worktree needed" per-file diff-stat computation. Rename/binary-file behavior is verified against real repo data by Epic 3.0's spike (Story 3.0.1) before this helper's own schema/proto work begins (adversarial-review Concern). |
| `CaptureShipSnapshot` | New free Go function (not a `BacklogLifecycleListener` method — architecture-review Concern) called synchronously from `ReconcilePRPending` immediately before the `pr_pending → done` transition. Takes the already-fetched `*git.PRStatus` as a parameter (no GitHub call of its own), independently calls `FileStatsBetween`, and writes whichever of the 6 new `BacklogItem` fields it successfully captured via one `UpdateBacklogItem` call — a failure in either the GitHub-data group or the file-stats group never discards a success in the other (adversarial-review Concern). | Synchronous with the done-transition to close the cleanup-before-snapshot race (pitfalls research §2). |
| `PRStatus` | Existing Go struct (`session/git/worktree_git.go:330`) — `CIFailing`, `HasBlockingReviews`, `HasConflicts`, `IsClosed`, `IsDraft`, `ApprovedCount`, `ChangesRequestedCount`, `Mergeable`. | Already fetched by `ReconcilePRPending` at the merge-detection point — the source `CaptureShipSnapshot` reads from, no new GitHub client call shape needed. |
| `ShippedSnapshotCaptureFailed` | New `bool` field (ent: `field.Bool("shipped_snapshot_capture_failed").Optional().Default(false)`; proto: `bool snapshot_capture_failed`; TS: `VcsWidgetData.snapshotCaptureFailed?: boolean`, valid only on the `"historical"` branch) set `true` when `CaptureShipSnapshot`'s GitHub-data group or file-stats group fails to capture, so the UI can say "couldn't fetch PR status" instead of rendering an indistinguishable-from-never-had-a-PR empty state — without overloading `ShippedCheckConclusion` (a real CI-conclusion field) with a non-CI sentinel value. | Fails-closed precedent from `docs/tasks/backlog-feature-improvement.md`'s "Merged-Before-Done Gate." Drives `MergeabilityState`'s `"snapshot_unavailable"` branch (Task 1.1.5c) and `VcsWidgetGithubRow`'s failure copy (Story 4.2.1), replacing the earlier `checkConclusion === "failed"` string-match design (architecture-review BLOCKER fix). |
| `data-testid="vcs-widget-loaded"` | The stable test hook asserting `VcsWidget` has finished loading (used instead of `waitForTimeout`), per `.claude/rules/e2e-test-conventions.md`. | |
| `VcsWidgetPage` | New Playwright page-helper class (`tests/e2e/pages/VcsWidgetPage.ts`) encapsulating widget-locator/assertion logic shared across the new e2e specs. | Per `.claude/rules/e2e-test-conventions.md` convention #4. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall unification shape | Single component + adapter functions (Approach A) | Step 0.5 creative pass | (B) three thin wrapper components; (C) backend-driven single normalized RPC | (B) contradicts requirements' explicit "one shared component" ask; (C) is an unbounded backend rearchitecture of two already-working, differently-timed data planes |
| Frontend data normalization | Adapter (GoF), as 3 plain functions | GoF | `VcsDataSource` interface with 3 class implementations (Strategy) | Only one implementation per source will ever exist — interface-pollution-checklist smell #1; a plain function is simpler and directly unit-testable with no mock harness |
| `VcsWidget` internal structure | Composition of 5 stateless sub-components, each with single-section responsibility | GoF Composite (informal) / house convention | One monolithic component with `mode === 'full' ? ... : ...` ternary sprawl | Pitfalls research §1 explicitly flags God Component and ternary-sprawl past ~3 conditional render paths for data this dense |
| `MergeabilityState` | Closed discriminated union (sum type) | Type-driven design | Separate booleans (`isMergeable`, `hasCIFailure`, `hasBlockingReview`) combined ad hoc per render site | Booleans permit contradictory/illegal combinations and re-derive logic at every call site — exactly the shape of bug `github/priority.go`'s closed/merged conflation already exhibits; a closed union + exhaustive switch makes the bug unrepresentable |
| `VcsWidgetData` live/historical representation | Discriminated union on `kind: "live" \| "historical"` (sum type) | Type-driven design | Flat interface with `isLive: boolean` + always-present nullable `snapshotAt` | The flat shape makes `{isLive: true, snapshotAt: <non-null>}` structurally representable even though no adapter should ever produce it (architecture-review Concern); a discriminated union makes that combination a compile error instead of a convention |
| `GithubSummary.prState` / `checkConclusion` | Closed literal unions (`"open" \| "closed" \| "merged"`, `"success" \| "failure" \| "pending" \| ""`) | Type-driven design | Raw `string` fields | ADR-003's whole premise is fixing a raw-string PR-state bug (`github/priority.go`'s closed/merged conflation) — leaving `GithubSummary` untyped reintroduces the exact class of bug the ADR fixes elsewhere (architecture-review Concern) |
| Snapshot-capture-failure signal | Dedicated `ShippedSnapshotCaptureFailed bool` field + `MergeabilityState` member `"snapshot_unavailable"` | Type-driven design | Repurpose `ShippedCheckConclusion`/`checkConclusion` with a `"failed"` sentinel string | A CI-conclusion field is not the place to smuggle a capture-failure flag — `"failed"` is not a real GitHub conclusion, has no branch in the closed `MergeabilityState` union, and silently misclassifies into `ci_pending` (architecture-review BLOCKER) |
| `VcsWidgetData` aggregate stats | One grouped `aggregateStats?: { filesChanged, additions, deletions }` object | Type-driven design (data clump → Value Object) | 3 separate top-level optionals (`aggregateFilesChanged?`/`aggregateAdditions?`/`aggregateDeletions?`) | 3 fields that are always populated or omitted together are a data clump; grouping keeps the "no field explosion" design goal honest as the shape grows (architecture-review Concern) |
| `CaptureShipSnapshot` receiver shape | Free function taking `*Storage` explicitly | `.claude/rules/interface-pollution-checklist.md` | Unexported method on `BacklogLifecycleListener` | The function needs no state from that type beyond `*Storage`, which is passed as a parameter — a method only earns its receiver when it genuinely needs the type's other state (architecture-review Concern) |
| `GetBacklogItemShipStatus` RPC handler | Transaction Script | PoEAA | Domain Model (a `BacklogItemShipStatus` aggregate object with behavior) | The handler is a straightforward read-plus-optional-recompute with no business rule reused elsewhere; matches the file's existing style (`server/services/backlog_service_ship_status.go` is already Transaction Script) — a Domain Model layer would be ceremony |
| Snapshot write path (`CaptureShipSnapshot`) | Transaction Script, called synchronously inside the existing `ReconcilePRPending` reconciler loop | PoEAA | Separate async worker/job queue for snapshot capture | Pitfalls research §2 identifies the cleanup-before-snapshot race explicitly; synchronous capture inside the already-scheduled reconciler tick (mirroring the existing `isCodeShippedToMain` gate pattern) eliminates the race with no new job infrastructure |
| Durable snapshot storage shape | 6 new fields directly on `BacklogItem` ent entity | Repository (existing `session.Storage`) | New `GitHubSnapshot` ent entity with a required unique edge to `BacklogItem` (mirroring `DiffStats`→`Session`) | See ADR-002 (ADR-025 in `docs/adr/`) — the relationship is 1:1 and write-once, unlike `DiffStats`' 1:many-in-principle shape; a new entity/edge doesn't earn its place (interface-pollution-checklist smell #6) |
| Per-file diff-stat computation | Single free function `FileStatsBetween` wrapping go-git's typed `Patch().Stats()` API | `.claude/rules/prefer-go-git-over-subshells.md` | Shell out to `git diff --numstat` and hand-parse tab-separated output | go-git's typed API removes an entire class of text-parsing bugs (binary files, renames, no-newline markers) a third hand-rolled parser would reintroduce |
| Diff viewing (`onViewDiff`) | Callback/Observer delegating to existing modals | House convention | Adopt `react-diff-view`, build inline expandable diff inside `VcsWidget` | See ADR-001 — two working diff modals already cover this; a third inline renderer duplicates functionality with no clear win |
| Snapshot validity representation | `snapshotAt: Date \| null` alone (no companion `hasSnapshot: boolean`) | Type-driven design (nullable-as-witness) | Separate `hasSnapshot: boolean` flag alongside `snapshotAt` | A boolean that must always agree with `snapshotAt !== null` is a data clump / primitive-obsession smell — the nullable field is already the presence check |

---

## Migration Plan

- **Migration file**: `session/ent/schema/backlog_item.go` (schema edit) → regenerated output under `session/ent/` (client, mutation, `backlogitem/` predicates). No separate SQL migration file — this ent setup applies schema changes via `ent`'s auto-migration at startup (consistent with all prior `BacklogItem` field additions in this codebase).
- **Reversibility**: All 6 new fields are `Optional()` (nil/zero-value-safe) — a rollback (revert the schema commit + regenerate) is safe with no data-loss risk, since no existing code path reads these fields yet and no NOT NULL constraint is introduced.
- **Zero-downtime strategy**: Additive-only schema change (new nullable columns), no column renames/drops/type changes on existing fields — ent's auto-migration adds columns without locking reads on existing columns. Existing "done" items (shipped before this feature) simply have all 6 fields at their zero-value/nil until (never automatically, per requirements' scope) backfilled — `GetBacklogItemShipStatus` must treat `ShippedSnapshotAt == nil` as "no snapshot captured" (UX research §4's "No history captured for this item" copy), not an error.
- **Rollback procedure**: `git revert` the schema-edit commit, re-run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`, `go build ./...`, redeploy. Column drop is not required for a safe rollback (unused nullable columns left behind are harmless) but may be done in a follow-up cleanup migration if desired.

## Observability Plan

- **Logs**: `CaptureShipSnapshot` logs failures through the existing `log.ErrorLog`/`log.WarningLog` pattern already used elsewhere in `ReconcilePRPending` (e.g. `session/backlog_lifecycle.go:95`'s `"[BacklogLifecycle] ReconcilePRPending done transition item=%s: %v"` style) — a snapshot-fetch failure logs `"[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d: %v"` at Warning (non-fatal — the `done` transition still proceeds; see Risk Control) so silent data loss is visible in `~/.stapler-squad/logs/stapler-squad.log`, matching the Observability Requirements in requirements.md.
- **Metrics**: None new — this project does not introduce a metrics backend; standard request logging is sufficient per requirements.md's explicit statement.
- **Alerts**: None new — no oncall alert condition per requirements.md.

## Risk Control

- **Feature flag**: None — per requirements.md, this is additive UI enrichment plus backend data persistence, not a session/worktree lifecycle behavior change. The shared component already degrades gracefully when historical richness is absent (`snapshotAt === null` → "no history captured" copy), so there is no unsafe intermediate state to gate behind a flag.
- **Rollback procedure**: Standard `git revert` per Phase (frontend `VcsWidget` wiring and backend snapshot capture are independently revertible — see Dependency Visualization). If `CaptureShipSnapshot`'s GitHub fetch call proves risky in practice (e.g. rate-limit pressure), it can be disabled independently by short-circuiting `CaptureShipSnapshot` to a no-op while leaving the `done` transition itself untouched — the transition must never block on snapshot-capture success (see "fails-closed but non-blocking" note in Phase 3 below).
- **Staged rollout**: Not required — no user segmentation exists in this deployment model (single-operator/small-team tool, no gradual rollout infrastructure). Ship Phase 1+2 (frontend) and Phase 3+4 (backend) as independently mergeable PRs so a problem in one does not block the other (see Dependency Visualization).

**Adjacent known issues (triad-review Engineering lens — not this project's to fix, but Phase 1/3 implementers should be aware their new code sits next to these):**
- `docs/bugs/open/BUG-020-vcs-status-diff-mutex-contention.md` — `GetVCSStatus`/`GetSessionDiff` run git subprocess calls under a lock, serializing concurrent callers. `fromSessionVcs` (Task 1.1.2b) consumes `GetVCSStatus`'s output as-is and does not change this call path, but a Phase 1 implementer should not assume `VcsWidget`'s live-mode refresh is cheap/concurrent-safe — it inherits this existing contention.
- `docs/bugs/open/BUG-023-pr-status-poller-mutex-churn.md` — `session/pr_status_poller.go`'s existing GitHub polling has known mutex-churn issues. `CaptureShipSnapshot` (Task 3.3.1a) reuses `GetPRStatus`, not the poller directly, so it isn't blocked by this bug, but if a future fix to BUG-023 changes `GetPRStatus`'s signature or locking behavior, Story 3.3.1 will need a compatibility check.

## Unresolved Questions

None — all three Open Questions from requirements.md are resolved by this plan:
1. "Where should durable snapshots be captured?" → `ReconcilePRPending`, synchronously before the `done` transition (Phase 3, ADR referenced in Pattern Decisions).
2. "Does `UnfinishedItemDetail` become a compact mode of the shared component?" → Yes, `VcsWidgetMode = "compact"` (Phase 2, Epic 2.2, Story 2.2.3).
3. "Files-tab navigation vs. inline expandable diff?" → `onNavigateToFile` optional callback (session context only) + `onViewDiff` delegating to existing modals; no inline diff (ADR-001).

The two "unstated needs" surfaces flagged by features research (`BacklogItemCard.tsx`, `SessionCard.tsx`/`SessionRow.tsx` compact VCS signal) are explicitly deferred — see Phase 6, Epic 6.3 note.

---

## Dependency Visualization

```
                         ┌─────────────────────────────────────────┐
                         │  Phase 1: Domain types + adapters        │
                         │  (frontend-only; builds against EXISTING │
                         │   live proto shapes — VCSStatus,          │
                         │   BacklogItemShipStatus as they exist    │
                         │   TODAY, before any backend change)      │
                         └───────────────┬───────────────────────────┘
                                         │
                         ┌───────────────▼───────────────────────────┐
                         │  Phase 2: VcsWidget component + wiring    │
                         │  into VcsPanel / BacklogItemDetail /      │
                         │  UnfinishedItemDetail (frontend-only)     │
                         └───────────────┬───────────────────────────┘
                                         │
              ┌──────────────────────────┴──────────────────────────┐
              │                                                      │
   (no blocking dependency — Phase 3 develops in parallel)           │
              │                                                      │
┌─────────────▼─────────────────────┐                                │
│ Phase 3: Backend durable snapshot │                                │
│ persistence (Go/ent/proto — fully │                                │
│ independent of Phase 1/2; can     │                                │
│ start on day 1 in parallel)       │                                │
│  3.0 SPIKE: verify go-git         │                                │
│      Patch().Stats() behavior     │                                │
│      (rename + binary) FIRST      │                                │
│  3.1 proto + ent schema           │                                │
│  3.2 FileStatsBetween (go-git)    │                                │
│  3.3 CaptureShipSnapshot in       │                                │
│      ReconcilePRPending           │                                │
│  3.4 RPC read-path population     │                                │
└─────────────┬──────────────────────┘                                │
              │                                                       │
              └──────────────────────────┬────────────────────────────┘
                                         │
                         ┌───────────────▼───────────────────────────┐
                         │  Phase 4: Frontend renders snapshot data  │
                         │  (extends fromShipStatus adapter + adds   │
                         │  "as of" UI — DEPENDS on Phase 3's proto  │
                         │  regen AND Phase 2's VcsWidget existing)  │
                         └───────────────┬───────────────────────────┘
                                         │
                         ┌───────────────▼───────────────────────────┐
                         │  Phase 5: Feature registry + e2e tests    │
                         │  (depends on Phase 2 minimum; Phase 4     │
                         │   adds durable-snapshot test coverage)    │
                         └─────────────────────────────────────────────┘
```

**Key ordering decision** (per Step 4 instructions): Phase 1+2 (frontend `VcsWidget`, built against the existing live-data proto shapes) and Phase 3 (backend snapshot persistence) have **no blocking dependency on each other** and should be developed as two parallel workstreams / separate PRs. Phase 3 does not require `VcsWidget` to exist — it's pure Go/ent/proto work. Phase 2 does not require Phase 3's new fields — `VcsWidget` renders correctly today off `VCSStatus`/`BacklogItemShipStatus` exactly as they exist now, with `snapshotAt: null` for any item lacking a snapshot (which is *all* items until Phase 3+4 ship). Phase 4 is the only phase with a hard dependency on Phase 3 (needs the regenerated proto fields) — it is a small, additive extension of Phase 1's `fromShipStatus` adapter, not a rewrite. Within Phase 3 itself, Epic 3.0's go-git spike (Story 3.0.1) runs before Epic 3.1's proto/schema work begins, so a `Patch().Stats()` limitation surfaces before schema work is sunk on top of an unverified assumption (adversarial-review Concern).

---

## Phase 1: Domain Types and Adapters (Frontend, No Backend Dependency)

### Epic 1.1: `VcsWidgetData` shape and adapter functions
**Goal**: Establish the one normalized data shape and the three pure functions that produce it, fully unit-testable in isolation before any component exists.

#### Story 1.1.1: Define `VcsWidgetData` and related types
**As a** frontend developer, **I want** one normalized TypeScript shape for VCS/PR/CI/diff data, **so that** `VcsWidget` takes a single typed prop instead of a per-source field explosion.
**Acceptance Criteria**:
- `VcsWidgetData` is a discriminated union on `kind: "live" | "historical"` (not a flat interface), plus `FileChangeSummary`, `CommitSummary`, `GithubSummary` interfaces exist in `web-app/src/lib/vcs/types.ts`, matching the shape in `research/architecture.md` §4 as amended by architecture-review Concern #1 (discriminated union) and Concern #4 (grouped aggregate stats).
  - *Given* the file `web-app/src/lib/vcs/types.ts`, *When* TypeScript compiles the project (`cd web-app && npx tsc --noEmit`), *Then* a value `{ kind: "live", branch: "feat/foo", isClean: false, fileChanges: [], aheadOfMain: 2, behindMain: 0, branchExists: true, commits: [], github: null, shipped: false }` type-checks with no type errors, AND a value with `kind: "live"` plus a `snapshotAt` property is a **type error** (the `"live"` branch has no `snapshotAt` field at all).
  - *Given* a value `{ kind: "historical", ..., snapshotAt: null, snapshotCaptureFailed: true }`, *When* TypeScript compiles, *Then* it type-checks with no errors (the `"historical"` branch carries `snapshotAt` and optional `snapshotCaptureFailed`).
- `GithubSummary.prState` and `GithubSummary.checkConclusion` are closed literal unions, not raw `string`.
  - *Given* `const g: GithubSummary = { ...valid, prState: "abandoned" }` (not one of `"open" | "closed" | "merged"`), *When* `npx tsc --noEmit` runs, *Then* it reports a type error.
- Aggregate stats are one grouped optional object, not 3 separate top-level optionals.
  - *Given* the `VcsWidgetData` common-fields interface, *When* inspected, *Then* it exposes `aggregateStats?: { filesChanged: number; additions: number; deletions: number }` and has no top-level `aggregateFilesChanged`/`aggregateAdditions`/`aggregateDeletions` fields.
**Files**: `web-app/src/lib/vcs/types.ts` (new)

##### Task 1.1.1a: Create `web-app/src/lib/vcs/types.ts` with `VcsWidgetData` discriminated union + `FileChangeSummary` (~5 min)
- Define a `VcsWidgetDataCommon` interface with the source-agnostic fields (`branch`, `isClean`, `fileChanges`, `aheadOfMain`, `behindMain`, `branchExists`, `commits`, `github`, `shipped`, `loadError?`, `aggregateStats?`), then `export type VcsWidgetData = (VcsWidgetDataCommon & { kind: "live" }) | (VcsWidgetDataCommon & { kind: "historical"; snapshotAt: Date | null; snapshotCaptureFailed?: boolean });`. Add `FileChangeSummary` (path, oldPath?, status, additions, deletions, section) in the same file.
- Files: `web-app/src/lib/vcs/types.ts`

##### Task 1.1.1b: Add `CommitSummary` + `GithubSummary` interfaces with closed literal unions (~4 min)
- Append `CommitSummary` (sha, summary, authorName?, authoredAt?) and `GithubSummary` (owner, repo, prUrl, prNumber, `prState: "open" | "closed" | "merged"`, isDraft, `checkConclusion: "success" | "failure" | "pending" | ""`, approvedCount, changesReqCount) to the same file. Export `PrState`/`CheckConclusion` as named type aliases so `deriveMergeabilityState` and tests can reference the exact legal set. `checkConclusion` never carries a capture-failure sentinel — see `snapshotCaptureFailed` (Task 1.1.5a/e).
- Files: `web-app/src/lib/vcs/types.ts`

##### Task 1.1.1c: Add `VcsWidgetMode` type export (~2 min)
- Add `export type VcsWidgetMode = "full" | "compact";` to the same file for reuse by both the adapters and the component.
- Files: `web-app/src/lib/vcs/types.ts`

##### Task 1.1.1d: Add `toPrState`/`toCheckConclusion` parse-boundary helpers (~4 min)
- `Session.githubPrState`/`githubCheckConclusion` (and `UnfinishedWorktree.githubPrState`) are plain unconstrained `string` fields on the wire (`proto/session/v1/types.proto`) — narrowing to `PrState`/`CheckConclusion` cannot be a direct field read or an unchecked `as` cast (architecture-review re-review Concern: raw proto strings have no parse step). In `web-app/src/lib/vcs/adapters.ts`, add `function toPrState(raw: string): PrState { return raw === "open" || raw === "closed" || raw === "merged" ? raw : "open"; }` (unrecognized/empty defaults to `"open"`, the safest "still needs attention" reading) and `function toCheckConclusion(raw: string): CheckConclusion { return raw === "success" || raw === "failure" || raw === "pending" ? raw : ""; }`. These are the *only* places a raw GitHub string may be assigned to `PrState`/`CheckConclusion` — every adapter must call through them, never assign the raw string directly.
- Files: `web-app/src/lib/vcs/adapters.ts`

#### Story 1.1.2: `fromSessionVcs` adapter (live session path)
**As a** frontend developer, **I want** a pure function converting `VCSStatus` + `Session` into `VcsWidgetData`, **so that** `VcsPanel`'s live data renders through the same widget as the other two surfaces.
**Acceptance Criteria**:
- `fromSessionVcs(status, session)` maps `VCSStatus.stagedFiles`/`unstagedFiles`/`untrackedFiles`/`conflictFiles` into one flat `fileChanges: FileChangeSummary[]` array with correct `section` tags, and reads `session.github*` fields into `GithubSummary` (or `null` if `session.githubOwner` is empty).
  - *Given* a `VCSStatus` with `branch: "feat/vcs-widget"`, `isClean: false`, `conflictFiles: [{path: "src/foo.ts", status: FILE_STATUS_CONFLICT, additions: 3, deletions: 1}]`, and a `Session` with `githubOwner: "tstapler"`, `githubRepo: "stapler-squad"`, `githubPrNumber: 42`, `githubCheckConclusion: "success"`, `githubApprovedCount: 1`, `githubChangesReqCount: 0`, *When* `fromSessionVcs(status, session)` is called, *Then* the result has `fileChanges: [{path: "src/foo.ts", status: "conflict", additions: 3, deletions: 1, section: "conflict"}]`, `github: {owner: "tstapler", repo: "stapler-squad", prNumber: 42, prState: "open", checkConclusion: "success", approvedCount: 1, changesReqCount: 0, ...}`, `kind: "live"` (no `snapshotAt` field present at all on the returned object).
- `session` is optional; when omitted, `github` is always `null`.
  - *Given* `fromSessionVcs(status)` called with no second argument, *When* the function runs, *Then* the returned `VcsWidgetData.github` is `null`.
**Files**: `web-app/src/lib/vcs/adapters.ts` (new), `web-app/src/lib/vcs/adapters.test.ts` (new)

##### Task 1.1.2a: Implement `fromSessionVcs` file-change flattening (~5 min)
- In new file `web-app/src/lib/vcs/adapters.ts`, write a private helper `flattenFileChanges(status: VCSStatus): FileChangeSummary[]` mapping the 4 `FileChange[]` arrays to one array with `section` set per source array, and a `mapFileStatus(s: FileStatus): FileChangeSummary["status"]` switch over the `FileStatus` enum.
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 1.1.2b: Implement `fromSessionVcs(status, session?)` (~5 min)
- Compose `flattenFileChanges` with branch/ahead/behind/clean fields off `status`, and `github: session?.githubOwner ? { ..., prState: toPrState(session.githubPrState), checkConclusion: toCheckConclusion(session.githubCheckConclusion) } : null` off `session` — always through the Task 1.1.1d parse helpers, never a direct field assignment. Return `{ kind: "live", ..., shipped: false }` — the `"live"` branch has no `snapshotAt`/`snapshotCaptureFailed` fields to set. `commits: []` (VCSStatus has no commit list — full mode's commit list stays empty for the live path, matching today's `VcsPanel` behavior which has no commit list either).
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 1.1.2c: Unit tests for `fromSessionVcs` (~5 min)
- Test: conflict file maps to `section: "conflict"`; no-GitHub session → `github: null`; `kind` is always `"live"` and the returned object has no `snapshotAt` property. Naming: `fromSessionVcs_should_<effect>_When_<condition>`.
- Files: `web-app/src/lib/vcs/adapters.test.ts`

#### Story 1.1.3: `fromShipStatus` adapter (historical backlog path)
**As a** frontend developer, **I want** a pure function converting `BacklogItemShipStatus` into `VcsWidgetData`, **so that** the Backlog item detail's durable ship-status renders through the same widget.
**Acceptance Criteria**:
- `fromShipStatus(status)` maps `status.commits` (`ShippedCommit[]`) into `CommitSummary[]`, sets `github: null` (Phase 1 — no snapshot fields exist on the proto yet), `kind: "historical"`, `snapshotAt: status.lastCommitAt` converted to a `Date` (or `null`).
  - *Given* a `BacklogItemShipStatus` with `shipped: true`, `shippedVia: "pr"`, `branchExists: false`, `commits: [{sha: "a1b2c3d", summary: "fix: widget bug", authorName: "Tyler Stapler", authoredAt: <timestamp for 2026-07-15>}]`, *When* `fromShipStatus(status)` is called, *Then* the result has `kind: "historical"`, `branchExists: false`, `commits: [{sha: "a1b2c3d", summary: "fix: widget bug", authorName: "Tyler Stapler", authoredAt: <Date for 2026-07-15>}]`.
- `status.error` (non-empty) maps to `fileChanges: []`, `commits: []`, and the error string is NOT silently dropped — it is threaded through as a new optional `VcsWidgetData.loadError?: string` field so `VcsWidget` can render UX research's "No history captured" copy instead of a blank widget.
  - *Given* a `BacklogItemShipStatus` with `error: "no work session ever committed code for this item"`, *When* `fromShipStatus(status)` is called, *Then* the result has `loadError: "no work session ever committed code for this item"`.
**Files**: `web-app/src/lib/vcs/adapters.ts`, `web-app/src/lib/vcs/adapters.test.ts`, `web-app/src/lib/vcs/types.ts` (add `loadError?` field)

##### Task 1.1.3a: Add `loadError?: string` to `VcsWidgetDataCommon` (~2 min)
- Add the optional field with a doc comment explaining its purpose (benign "no history" vs. hard error distinction, per `research/ux.md` §4).
- Files: `web-app/src/lib/vcs/types.ts`

##### Task 1.1.3b: Implement `fromShipStatus(status)` (~5 min)
- Map `commits`, `branchName → branch`, `branchExists`, `aheadOfMain → aheadOfMain`, `behindMain → behindMain`, `error → loadError`. `isClean` is not knowable historically — default to `true` (no working-directory concept once the worktree is gone). `github: null` for now (Phase 4 extends this). Return `{ kind: "historical", snapshotAt: status.lastCommitAt ? new Date(status.lastCommitAt) : null, ... }` (`snapshotCaptureFailed` left `undefined` — Phase 1 has no snapshot-capture concept yet).
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 1.1.3c: Unit tests for `fromShipStatus` (~5 min)
- Test: commit list maps correctly; `error` maps to `loadError`; `branchExists: false` passes through unchanged (the "deleted — already merged" case from `ShipStatusDisplay.tsx:66-75` must still be derivable downstream); `kind` is always `"historical"`.
- Files: `web-app/src/lib/vcs/adapters.test.ts`

#### Story 1.1.4: `fromUnfinishedWorktree` adapter (compact path)
**As a** frontend developer, **I want** a pure function converting `UnfinishedWorktree` into `VcsWidgetData`, **so that** the Unfinished item detail's compact card can render through `VcsWidget` in compact mode.
**Acceptance Criteria**:
- `fromUnfinishedWorktree(wt)` maps `wt.aheadCommitMessages` (`string[]`) into `CommitSummary[]` with only `summary` populated (`sha: ""`, no author/date — the proto doesn't carry them), and maps `wt.changedFiles`/`linesAdded`/`linesRemoved` into a grouped `aggregateStats: { filesChanged, additions, deletions }` object (per compact-mode-never-lists-files, features research edge case #6, `fileChanges` stays `[]`).
  - *Given* an `UnfinishedWorktree` with `changedFiles: 5`, `linesAdded: 42`, `linesRemoved: 8`, `aheadCommitMessages: ["fix: typo", "feat: add widget"]`, `githubPrNumber: 7`, `githubPrUrl: "https://github.com/tstapler/stapler-squad/pull/7"`, `githubPrState: "open"`, *When* `fromUnfinishedWorktree(wt)` is called, *Then* the result has `aggregateStats: { filesChanged: 5, additions: 42, deletions: 8 }`, `commits: [{sha: "", summary: "fix: typo"}, {sha: "", summary: "feat: add widget"}]`, `fileChanges: []`, `github: {owner: "tstapler", repo: "stapler-squad", prNumber: 7, prState: "open", checkConclusion: "", approvedCount: 0, changesReqCount: 0, ...}`, `kind: "live"`.
**Files**: `web-app/src/lib/vcs/adapters.ts`, `web-app/src/lib/vcs/adapters.test.ts`, `web-app/src/lib/vcs/types.ts` (add grouped `aggregateStats?` field)

##### Task 1.1.4a: Add `aggregateStats?: { filesChanged: number; additions: number; deletions: number }` to `VcsWidgetDataCommon` (~3 min)
- Add one grouped optional object field (not 3 separate top-level optionals — architecture-review Concern #4) with a doc comment: "populated only by `fromUnfinishedWorktree` and any other source that has aggregate-only stats without a per-file breakdown."
- Files: `web-app/src/lib/vcs/types.ts`

##### Task 1.1.4b: Implement `fromUnfinishedWorktree(wt)` — commits + aggregates (~5 min)
- Map `aheadCommitMessages` → `commits` (sha `""`), aggregate fields into `aggregateStats`, `branch: wt.branch`, `isClean: !wt.hasUncommitted`, `aheadOfMain: wt.commitsAhead`, `behindMain: wt.commitsBehind`. Return `{ kind: "live", ... }` (no `snapshotAt` field — it's a live scan-backed stream).
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 1.1.4c: Implement GitHub field mapping for `fromUnfinishedWorktree` (~4 min)
- Parse `wt.repoPath`-derived owner/repo is not available on `UnfinishedWorktree` directly — extract `owner`/`repo` from `wt.githubPrUrl` via URL parsing (`https://github.com/{owner}/{repo}/pull/{n}`), falling back to `""`/`""` if unparseable. `prState: toPrState(wt.githubPrState)` (through the Task 1.1.1d helper, never a direct assignment). `checkConclusion`/`approvedCount`/`changesReqCount` default to `""`/`0`/`0` (not present on `UnfinishedWorktree` per stack research §2's field gap) — `""` is already a legal `CheckConclusion` value, no helper call needed for that default.
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 1.1.4d: Unit tests for `fromUnfinishedWorktree` (~5 min)
- Test: `aggregateStats` populates correctly as one grouped object; `fileChanges` is always `[]`; GitHub URL parses owner/repo; no-PR worktree → `github: null`; `kind` is always `"live"`.
- Files: `web-app/src/lib/vcs/adapters.test.ts`

#### Story 1.1.5: `MergeabilityState` type and `deriveMergeabilityState`
**As a** developer viewing any VCS surface, **I want** one synthesized mergeability signal instead of mentally combining 3-4 fields, **so that** I can assess "is this ready" at a glance (UX research §2, ADR-003).
**Acceptance Criteria**:
- `deriveMergeabilityState(data)` returns `"shipped"` whenever `data` represents a git-history-confirmed shipped item, even if `data.github?.prState === "closed"` — this is the ADR-003 bug fix (durable ship status wins over stale GitHub state).
  - *Given* a `VcsWidgetData` with `shipped: true` (an explicit `shipped: boolean` field on `VcsWidgetDataCommon`, sourced from `BacklogItemShipStatus.shipped` in `fromShipStatus`) and `github: {prState: "closed", ...}`, *When* `deriveMergeabilityState(data)` is called, *Then* the result is `"shipped"`, not `"closed_unshipped"`.
- `deriveMergeabilityState(data)` returns `"closed_unshipped"` when the PR is closed, not shipped, per ADR-003's named bug-fix case.
  - *Given* a `VcsWidgetData` with `shipped: false` and `github: {prState: "closed", isDraft: false, ...}`, *When* `deriveMergeabilityState(data)` is called, *Then* the result is `"closed_unshipped"`.
- `deriveMergeabilityState(data)` returns `"snapshot_unavailable"` when `kind === "historical"` and `snapshotCaptureFailed === true`, checked **before** the `no_pr`/`ci_pending` branches — this is the architecture-review BLOCKER fix (a capture failure must never silently read as "CI still running" or "no PR ever existed").
  - *Given* a `VcsWidgetData` with `kind: "historical"`, `shipped: false`, `snapshotCaptureFailed: true`, `github: null`, *When* `deriveMergeabilityState(data)` is called, *Then* the result is `"snapshot_unavailable"`, not `"no_pr"`.
  - *Given* a `VcsWidgetData` with `kind: "historical"`, `shipped: false`, `snapshotCaptureFailed: true`, `github: {checkConclusion: "", ...}` (partial capture — PR data present, file-stats capture failed; see Story 3.3.1's independent-failure acceptance criterion), *When* `deriveMergeabilityState(data)` is called, *Then* the result is `"snapshot_unavailable"`, not `"ci_pending"`.
- All 10 states are reachable and the function never throws for any well-formed `VcsWidgetData`.
  - *Given* a `VcsWidgetData` with `github: null`, `shipped: false`, and (if `kind === "historical"`) `snapshotCaptureFailed` falsy, *When* `deriveMergeabilityState(data)` is called, *Then* the result is `"no_pr"`.
**Files**: `web-app/src/lib/vcs/mergeability.ts` (new), `web-app/src/lib/vcs/mergeability.test.ts` (new), `web-app/src/lib/vcs/types.ts` (add `shipped: boolean` field)

##### Task 1.1.5a: Add `shipped: boolean` field to `VcsWidgetDataCommon` (~2 min)
- Add the field (default `false` for `fromSessionVcs`/`fromUnfinishedWorktree` since liveness implies not-yet-shipped-in-the-durable sense; `fromShipStatus` sets it from `status.shipped`).
- Files: `web-app/src/lib/vcs/types.ts`, `web-app/src/lib/vcs/adapters.ts` (wire the field into all 3 adapters)

##### Task 1.1.5b: Define `MergeabilityState` union type with 10 members (~2 min)
- Create `web-app/src/lib/vcs/mergeability.ts` with the 10-member union type from the Domain Glossary, including `"snapshot_unavailable"`.
- Files: `web-app/src/lib/vcs/mergeability.ts`

##### Task 1.1.5c: Implement `deriveMergeabilityState` precedence logic (~5 min)
- Implement the top-down precedence: `shipped` → `snapshot_unavailable` (`data.kind === "historical" && data.snapshotCaptureFailed === true`) → `no_pr` (no `github`) → `draft` → `conflicted` (`data.fileChanges.some(f => f.section === "conflict")`) → `changes_requested` (`github.changesReqCount > 0`) → `ci_failing` (`github.checkConclusion` in `["failure","action_required"]`) → `closed_unshipped` (`github.prState === "closed"`) → `ci_pending` (`checkConclusion` in `["pending",""]` and `checkStatus` unresolved) → `ready_to_merge` (fallback). `snapshot_unavailable` is checked early (right after `shipped`, before every GitHub-signal branch) so a capture failure can never be misread as any other state, per the architecture-review BLOCKER remediation.
- Files: `web-app/src/lib/vcs/mergeability.ts`

##### Task 1.1.5d: Unit tests for `deriveMergeabilityState` (~6 min)
- One test per state (10 tests) plus the ADR-003 precedence test (`shipped` wins over `closed`) plus the capture-failure precedence test (`snapshot_unavailable` wins over both `no_pr` and `ci_pending`). Naming: `deriveMergeabilityState_should_Return<State>_When_<condition>`.
- Files: `web-app/src/lib/vcs/mergeability.test.ts`

---

## Phase 2: `VcsWidget` Component (Frontend, Wires Into Existing Call Sites)

### Epic 2.1: Sub-components
**Goal**: Build the 5 decomposed sub-components so `VcsWidget` itself stays a thin composition layer, avoiding the God Component failure mode.

#### Story 2.1.1: `MergeabilityPill` sub-component
**As a** user, **I want** one colored, labeled pill summarizing mergeability, **so that** I don't have to read 4 separate fields to know if work is ready.
**Acceptance Criteria**:
- Renders one of 10 labeled pills (e.g. `"shipped"` → "✓ Shipped", `"closed_unshipped"` → "✦ Closed — not merged", `"ci_failing"` → "✗ CI failing", `"snapshot_unavailable"` → "⚠ Status unavailable") using `lucide-react` icons (not emoji) and theme-token colors (`vars.color.success`/`vars.color.errorText`/`vars.color.warning`/etc.), never a hardcoded hex.
  - *Given* `deriveMergeabilityState` returns `"ci_failing"`, *When* `<MergeabilityPill state="ci_failing" />` renders, *Then* the DOM contains text "CI failing" and a `lucide-react` `XCircle` icon with `aria-hidden="true"` (decorative — the adjacent text already carries the meaning, per UX research §3 finding #4), and the pill's background color resolves to `vars.color.errorBg` (verifiable via computed style in a jsdom test or via `styles.css.ts` recipe class name).
- Component is exhaustive over `MergeabilityState` — a `switch` with no `default` case (TypeScript compile error if a new state is ever added without a render case).
  - *Given* the TypeScript compiler, *When* `cd web-app && npx tsc --noEmit` runs, *Then* no "not all code paths return a value" error is reported for `MergeabilityPill`'s render switch (confirming exhaustiveness).
**Files**: `web-app/src/components/shared/vcs-widget/MergeabilityPill.tsx` (new), `MergeabilityPill.css.ts` (new), `MergeabilityPill.test.tsx` (new)

##### Task 2.1.1a: Create `MergeabilityPill.css.ts` recipe (~6 min)
- `recipe()` with `variants: { state: { shipped: {...}, draft: {...}, conflicted: {...}, changes_requested: {...}, ci_failing: {...}, ci_pending: {...}, ready_to_merge: {...}, closed_unshipped: {...}, snapshot_unavailable: {...}, no_pr: {...} } }` (10 variants — includes the `snapshot_unavailable` state added to fix the architecture-review BLOCKER), each mapping to `vars.color.*` tokens (add any missing tokens to `web-app/src/app/globals.css` first per `.claude/rules/css-architecture.md`; `snapshot_unavailable` can reuse `vars.color.warning`/`vars.color.warningBg`, a distinct-but-not-alarming tone from `ci_failing`'s error tone).
- Files: `web-app/src/components/shared/vcs-widget/MergeabilityPill.css.ts`, `web-app/src/app/globals.css` (add tokens if missing)

##### Task 2.1.1b: Implement `MergeabilityPill.tsx` exhaustive switch over all 10 states (~5 min)
- Exhaustive `switch (state)` returning `{icon: LucideIcon, label: string}` per state (including `snapshot_unavailable` → `{icon: AlertTriangle, label: "Status unavailable"}`), rendered as `<span className={pill({state})}><Icon aria-hidden="true" size={14} />{label}</span>`.
- Files: `web-app/src/components/shared/vcs-widget/MergeabilityPill.tsx`

##### Task 2.1.1c: Unit tests for `MergeabilityPill` (~6 min)
- Render each of the 10 states (including `snapshot_unavailable`), assert the expected label text is present. Naming: `MergeabilityPill_should_Render<Label>_When_State<Is>`.
- Files: `web-app/src/components/shared/vcs-widget/MergeabilityPill.test.tsx`

#### Story 2.1.2: `VcsWidgetHeader` sub-component
**As a** user, **I want** branch/clean-dirty/ahead-behind and worktree path in one header row, **so that** "what state is the code in" is answered without scrolling.
**Acceptance Criteria**:
- Renders branch name with a `lucide-react` `GitBranch` icon (`aria-hidden="true"`), clean/dirty text (not color-only), ahead/behind counts when nonzero, and — full mode only — worktree path with copy-to-clipboard and "Browse files" buttons ported from `BacklogItemDetail.tsx:1252-1266`.
  - *Given* `VcsWidgetData` with `branch: "feat/vcs-widget"`, `isClean: false`, `aheadOfMain: 3`, `behindMain: 1`, *When* `<VcsWidgetHeader data={data} mode="full" worktreePath="/home/tstapler/.stapler-squad/worktrees/feat-vcs-widget" />` renders, *Then* the DOM contains "feat/vcs-widget", "Uncommitted changes", "↑3 ahead", "↓1 behind", and a button with `aria-label="Copy worktree path"` (not `title`-only, per UX research §3 finding #3).
- Copy-to-clipboard button uses `aria-label`, not `title`-only.
  - *Given* the rendered header, *When* a screen reader computes the accessible name of the copy button, *Then* it reads "Copy worktree path" (from `aria-label`), not the 📋 emoji glyph's Unicode name.
- When `activeSessionCount` prop is provided and `> 1`, renders a small "N active sessions" indicator (full mode only) surfacing the multi-concurrent-session ambiguity from `BacklogItemDetail.tsx:186`'s `.reverse().find()` "most recent work session" heuristic — the heuristic itself is unchanged (out of scope per requirements), but the ambiguity becomes visible instead of silently hidden (adversarial-review Concern).
  - *Given* `<VcsWidgetHeader data={data} mode="full" activeSessionCount={3} worktreePath="..." />`, *When* it renders, *Then* the DOM contains text "3 active sessions" (or equivalent) near the worktree-path row.
  - *Given* `activeSessionCount` is `1` or omitted, *When* the header renders, *Then* no such indicator appears.
**Files**: `web-app/src/components/shared/vcs-widget/VcsWidgetHeader.tsx` (new), `VcsWidgetHeader.css.ts` (new), `VcsWidgetHeader.test.tsx` (new)

##### Task 2.1.2a: Implement branch + clean/dirty row (~5 min)
- Port `VcsStatusDisplay.tsx:32-46`'s branch/clean-dirty rendering, swap the `⎇` emoji for `lucide-react`'s `GitBranch` icon with `aria-hidden="true"`.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetHeader.tsx`

##### Task 2.1.2b: Implement ahead/behind row (~3 min)
- Port `VcsStatusDisplay.tsx:74-84`'s ahead/behind rendering unchanged (already text+icon, no color-only issue).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetHeader.tsx`

##### Task 2.1.2c: Implement worktree path row (full mode only), with `aria-label` fix (~5 min)
- Port `BacklogItemDetail.tsx:1252-1266`'s copy/browse buttons, replacing `title="Copy worktree path"` with `aria-label="Copy worktree path"` (keep `title` too for desktop tooltip) and `title="Browse files..."` with `aria-label="Browse files in this worktree"`. Only rendered when `mode === "full"` and `worktreePath` prop is non-empty.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetHeader.tsx`

##### Task 2.1.2d: `VcsWidgetHeader.css.ts` (~3 min)
- `recipe()` with a `mode` variant axis for full/compact spacing (compact: smaller font-size/padding, per `.claude/rules/css-architecture.md`).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetHeader.css.ts`

##### Task 2.1.2e: Unit tests for `VcsWidgetHeader` (~6 min)
- Test: dirty/clean text renders correctly; ahead/behind hidden when both zero; worktree path row hidden in compact mode; copy button has `aria-label`; "N active sessions" indicator renders when `activeSessionCount > 1` and is absent when `1` or omitted.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetHeader.test.tsx`

##### Task 2.1.2f: Implement "N active sessions" indicator for the multi-concurrent-session case (~4 min)
- Add an optional `activeSessionCount?: number` prop to `VcsWidgetHeader`; when `mode === "full" && activeSessionCount && activeSessionCount > 1`, render a small badge/text near the worktree-path row (e.g. "3 active sessions" with a `lucide-react` `Users` icon, `aria-hidden="true"`) — this is a minimal visibility fix for the adversarial-review Concern about `BacklogItemDetail.tsx:186`'s silent "last work session wins" selection; it does not change which session's data the header actually displays (still the most-recent-work-session heuristic per requirements' scope).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetHeader.tsx`

#### Story 2.1.3: `VcsWidgetFileList` sub-component (full mode only)
**As a** user reviewing changes, **I want** categorized, clickable, keyboard-accessible file lists with per-file diff stats, **so that** I can navigate to any changed file without a mouse.
**Acceptance Criteria**:
- File rows render as real `<button>` elements (never a clickable `<span>`), fixing `VcsPanel.tsx:84-90`'s WCAG 2.1.1/4.1.2 violation.
  - *Given* `fileChanges: [{path: "src/foo.ts", status: "modified", additions: 5, deletions: 2, section: "unstaged"}]` and `onNavigateToFile` provided, *When* `<VcsWidgetFileList fileChanges={fileChanges} onNavigateToFile={fn} />` renders, *Then* the DOM contains a `<button>` element (not `<span onClick>`) with accessible text including "src/foo.ts", reachable via `Tab` and activatable via `Enter`/`Space`.
- Conflicts render first, always fully shown even when other categories are capped.
  - *Given* `fileChanges` containing 60 unstaged entries and 2 conflict entries, *When* the list renders, *Then* both conflict rows are present in the DOM, the unstaged section shows the first 20 plus a "Show all 60 files" button (per UX research §4's 50+-file edge case), and the conflict section heading appears before the unstaged section heading in DOM order.
- When `onNavigateToFile` is omitted, file paths render as plain (non-interactive) text, not a dead button.
  - *Given* `<VcsWidgetFileList fileChanges={fileChanges} />` with no `onNavigateToFile` prop, *When* it renders, *Then* the file path renders inside a `<span>`, not a `<button>`.
**Files**: `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx` (new), `VcsWidgetFileList.css.ts` (new), `VcsWidgetFileList.test.tsx` (new)

##### Task 2.1.3a: Port `FILE_STATUS_META` glyph lookup table (~4 min)
- Port `VcsPanel.tsx:26-39`'s `FILE_STATUS_META` unchanged (already correct: `aria-label={meta.label}` present) but swap emoji icons for `lucide-react` equivalents (`FilePlus`, `FileMinus`, `FileEdit`, `AlertTriangle` for conflict, etc.).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx`

##### Task 2.1.3b: Implement categorized section rendering with conflicts-first order (~5 min)
- Group `fileChanges` by `section`, render in order `conflict, staged, unstaged, untracked`, each as its own labeled `<ul>` (hidden entirely when empty, per GitLab's "hide sections with nothing to show" precedent).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx`

##### Task 2.1.3c: Implement real `<button>` file rows with keyboard support (~5 min)
- `onNavigateToFile ? <button onClick={() => onNavigateToFile(f.path)}>...</button> : <span>...</span>` — native `<button>` gets keyboard support for free (no manual `onKeyDown`/`tabIndex` needed, unlike the `<span>` it replaces).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx`

##### Task 2.1.3d: Implement 50+-file cap with "Show all N files" expand (~5 min)
- Per-section: if `section !== "conflict"` and `items.length > 20`, render first 20 + a `<button>` toggling a `showAll` boolean state to render the rest. Conflicts section never caps.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx`

##### Task 2.1.3e: `VcsWidgetFileList.css.ts` with `overflow-x: auto` scroll containment (~3 min)
- `recipe()` for row/section styling; long paths get `text-overflow: ellipsis` inside a container with `overflow-x: auto` (per pitfalls research §5 mobile overflow guidance), not silent clipping.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.css.ts`

##### Task 2.1.3f: Unit tests for `VcsWidgetFileList` (~5 min)
- Tests: real `<button>` not `<span>`; conflicts always shown past the cap; keyboard `Enter` triggers `onNavigateToFile`; empty section renders nothing.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetFileList.test.tsx`

#### Story 2.1.4: `VcsWidgetCommitList` sub-component
**As a** user, **I want** a truncated, tap-expandable commit list, **so that** long commit messages don't overflow or rely on hover-only tooltips (broken on mobile).
**Acceptance Criteria**:
- Long commit summaries truncate with `text-overflow: ellipsis` and expand on click/tap (not hover-only), fixing `ShipStatusDisplay.tsx:88`'s untruncated-summary gap and `UnfinishedItemDetail.tsx:118`'s hover-only-tooltip gap (WCAG 1.4.13, UX research §3 finding #5).
  - *Given* a `CommitSummary` with `summary: "feat: add a very long commit message that definitely exceeds one line of available width in the commit list row"`, *When* the row renders at a 320px-wide compact-mode container and the user taps the row, *Then* the row's expanded state shows the full untruncated summary text (verifiable via a `data-testid="commit-row-expanded"` element appearing after the click).
- Commit list caps display count in compact mode (≤5, matching `UnfinishedWorktree.aheadCommitMessages`' existing cap) but shows all in full mode with a "Show all N commits" affordance past a threshold (e.g. 20).
  - *Given* `commits` with 30 entries and `mode="full"`, *When* `<VcsWidgetCommitList commits={commits} mode="full" />` renders, *Then* the DOM shows the first 20 rows plus a "Show all 30 commits" button.
**Files**: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx` (new), `VcsWidgetCommitList.css.ts` (new), `VcsWidgetCommitList.test.tsx` (new)

##### Task 2.1.4a: Implement commit row with click-to-expand (~5 min)
- Each commit row is a `<li>` containing a `<button>` (not the whole `<li>`) toggling a per-row `expanded` boolean; collapsed state truncates via CSS, expanded state renders the full text in a `data-testid="commit-row-expanded"` element.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx`

##### Task 2.1.4b: Implement mode-based cap (compact ≤5, full ≤20 + expand) (~4 min)
- Slice `commits` per `mode`; full mode adds a "Show all N commits" expand button past 20, mirroring `VcsWidgetFileList`'s Task 2.1.3d pattern.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx`

##### Task 2.1.4c: `VcsWidgetCommitList.css.ts` (~3 min)
- `recipe()` with `mode` variant; ellipsis truncation on the collapsed row, full-width wrap on expanded.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.css.ts`

##### Task 2.1.4d: Unit tests for `VcsWidgetCommitList` (~4 min)
- Tests: tap expands truncated text; compact mode caps at 5; full mode caps at 20 with expand button.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.test.tsx`

#### Story 2.1.5: `VcsWidgetGithubRow` sub-component
**As a** user, **I want** PR link, draft flag, and review/CI counts in one row, **so that** GitHub state is visible without leaving the widget.
**Acceptance Criteria**:
- Renders nothing (`return null`) when `data.github === null` AND `data` is not a capture-failed historical snapshot — no empty-state placeholder, per GitLab's conditional-widget-row precedent and `VcsPanel.tsx:110-111`'s existing `hasGitHub` guard. The one exception: when `data.kind === "historical" && data.snapshotCaptureFailed === true`, the row renders the failure copy from Story 4.2.1 even if `data.github === null` — an honest "couldn't capture" message is not the same as "no empty-state placeholder for a normal no-PR case."
  - *Given* `VcsWidgetData` with `kind: "live"`, `github: null`, *When* `<VcsWidgetGithubRow data={data} />` renders, *Then* the component returns `null` and nothing GitHub-related appears in the DOM.
  - *Given* `VcsWidgetData` with `kind: "historical"`, `github: null`, `snapshotCaptureFailed: true`, *When* `<VcsWidgetGithubRow data={data} />` renders, *Then* the component does NOT return `null` — it renders Story 4.2.1's failure copy.
- Approved/changes-requested counts have `aria-label`, fixing `VcsPanel.tsx:150-155`'s unlabeled `✓ {count}`/`✗ {count}` glyphs.
  - *Given* `github: {approvedCount: 2, changesReqCount: 1, ...}`, *When* the row renders, *Then* the DOM contains an element with `aria-label="2 approved"` and one with `aria-label="1 changes requested"`.
**Files**: `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.tsx` (new), `VcsWidgetGithubRow.css.ts` (new), `VcsWidgetGithubRow.test.tsx` (new)

##### Task 2.1.5a: Implement PR link + draft flag row (~4 min)
- Port `VcsPanel.tsx`'s GitHub-section PR-link rendering, using `lucide-react`'s `GitPullRequest`/`GitPullRequestDraft` icons instead of `⑂`/`⎇`.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.tsx`

##### Task 2.1.5b: Implement approved/changes-requested counts with `aria-label` (~4 min)
- `<span aria-label={\`${count} approved\`}><Check aria-hidden="true"/>{count}</span>` and the changes-requested equivalent with `X`/`aria-label={\`${count} changes requested\`}`.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.tsx`

##### Task 2.1.5c: `VcsWidgetGithubRow.css.ts` with theme-token CI colors (~4 min)
- `recipe()` mapping `checkConclusion` to `vars.color.success`/`vars.color.errorText`/`vars.color.warning` — fixes `VcsPanel.tsx:113-116`'s hardcoded hex (`#7ee787`/`#f97583`). Add `vars.color.ciSuccess`/`vars.color.ciFailure` tokens to `globals.css` if not already covered by existing `success`/`errorText` tokens.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.css.ts`, `web-app/src/app/globals.css` (if new tokens needed)

##### Task 2.1.5d: Unit tests for `VcsWidgetGithubRow` (~4 min)
- Tests: `github: null` → renders nothing; counts have correct `aria-label`; draft flag renders the draft icon.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.test.tsx`

### Epic 2.2: `VcsWidget` composition and accessibility shell

#### Story 2.2.1: `VcsWidget` top-level component
**As a** frontend developer, **I want** the top-level `VcsWidget` composing all 5 sub-components behind a `mode` prop, **so that** call sites render one component instead of hand-assembling sections.
**Acceptance Criteria**:
- `<VcsWidget data={data} mode="full" onNavigateToFile={fn} onViewDiff={fn} onRefresh={fn} />` renders `MergeabilityPill` + `VcsWidgetHeader` + `VcsWidgetGithubRow` + `VcsWidgetFileList` + `VcsWidgetCommitList` in that order (mergeability-pill-first, per UX research §2's "above the fold" priority order).
  - *Given* a fully-populated `VcsWidgetData` and `mode="full"`, *When* `<VcsWidget data={data} mode="full" />` renders, *Then* querying the container's children in DOM order yields `MergeabilityPill` output first, followed by header, GitHub row, file list, commit list.
- `mode="compact"` omits `VcsWidgetFileList` entirely (never renders per-file rows, per features research edge case #6) and shows only the aggregate stat line + `VcsWidgetCommitList` capped at 5.
  - *Given* `data.aggregateStats: { filesChanged: 5, additions: 42, deletions: 8 }` and `mode="compact"`, *When* `<VcsWidget data={data} mode="compact" />` renders, *Then* the DOM contains "5 files changed", "+42", "-8" but contains no per-file `<button>`/`<span>` rows for individual file paths.
- `onRefresh` button (icon-only) renders with `aria-label="Refresh VCS status"`, never `title`-only — fixes `VcsPanel.tsx:214-216`.
  - *Given* `onRefresh` provided and `data.kind === "live"`, *When* the widget renders, *Then* a button exists with `aria-label="Refresh VCS status"`.
- `onRefresh` is omitted entirely when `data.kind === "historical"` — no refresh button on a snapshot.
  - *Given* `data.kind: "historical"`, `onRefresh` provided as a prop, *When* the widget renders, *Then* no refresh button appears in the DOM (the widget itself suppresses it based on `kind`, independent of whether the caller passed the callback).
- All interactive elements inside `VcsWidget` (file-row buttons, commit-row expand buttons, refresh button, copy/browse buttons, "Show all N" expand buttons) meet a 44×44px minimum touch target on mobile viewports (≤480px width), per pitfalls research §5's explicit "shrunk compact-mode hit area" regression warning.
  - *Given* `<VcsWidget data={data} mode="compact" />` rendered at a 375px-wide viewport, *When* the computed style of any `<button>` inside the widget is inspected, *Then* its `min-height` and `min-width` (padding-inclusive) are each ≥44px, even where visible icon/text content is smaller.
**Files**: `web-app/src/components/shared/VcsWidget.tsx` (new), `VcsWidget.css.ts` (new), `VcsWidget.test.tsx` (new)

##### Task 2.2.1a: Implement `VcsWidget` full-mode composition (~5 min)
- Compose `MergeabilityPill(deriveMergeabilityState(data))`, `VcsWidgetHeader`, `VcsWidgetGithubRow`, `VcsWidgetFileList`, `VcsWidgetCommitList` in that DOM order, passing `data`/`mode`/`onNavigateToFile`/`onViewDiff` through. Accept an optional `activeSessionCount?: number` prop on `VcsWidget` itself and forward it to `VcsWidgetHeader` (Task 2.1.2f).
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 2.2.1b: Implement `VcsWidget` compact-mode composition (~4 min)
- Compact mode: `MergeabilityPill`, `VcsWidgetHeader` (compact sub-mode, no worktree-path row), aggregate stat line (new small inline block, not a sub-component — too small to earn one per interface-pollution-checklist smell #6), `VcsWidgetCommitList` capped at 5. No `VcsWidgetFileList`, no `VcsWidgetGithubRow` (PR badge already lives in `GitHubBadge` at compact call sites per architecture research — avoid duplicating it; compact mode assumes the caller renders `GitHubBadge` alongside if desired).
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 2.2.1c: Implement `onRefresh` button gated on `kind === "live"`, with `aria-label` (~3 min)
- `{data.kind === "live" && onRefresh && <button aria-label="Refresh VCS status" onClick={onRefresh}><RefreshCw aria-hidden="true" /></button>}`.
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 2.2.1d: Implement "as of" snapshot timestamp copy (~4 min)
- `{data.kind === "historical" && data.snapshotAt && <span data-testid="vcs-widget-snapshot-timestamp">As of {formatRelative(data.snapshotAt)}</span>}` — addresses UX research §2/§5's "last synced" trust-building recommendation. `data.kind === "historical" && !data.snapshotAt && data.loadError` → renders the `loadError` string in neutral (not red-error) styling per UX research §4. `data.kind === "historical" && data.snapshotCaptureFailed` is handled separately by `VcsWidgetGithubRow`'s failure copy (Story 4.2.1) — not duplicated here.
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 2.2.1e: Add `aria-live="polite"` wrapper around the async-updating region (~3 min)
- Wrap `MergeabilityPill` + `VcsWidgetGithubRow` (the parts that change on poll/refresh) in a `<div role="status" aria-live="polite">` — fixes the "no live region" gap flagged for all 4 legacy files (UX research §3 finding #6).
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 2.2.1f: `VcsWidget.css.ts` top-level recipe with mobile stacking order (~5 min)
- `recipe()` with `mode` variant; mobile breakpoint (`@media` inside vanilla-extract) reorders sections so `MergeabilityPill` stays above the fold before commit/file lists, matching `UnfinishedItemDetail`'s existing compact-card hierarchy (pitfalls research §5).
- Files: `web-app/src/components/shared/VcsWidget.css.ts`

##### Task 2.2.1g: Add `data-testid="vcs-widget-loaded"` (~2 min)
- Add to the widget's root element once `data` is non-null (the caller passes `data` only after its own loading state resolves — see Epic 2.2 wiring stories) — the stable e2e hook per `.claude/rules/e2e-test-conventions.md`.
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 2.2.1h: Unit tests for `VcsWidget` composition (~5 min)
- Tests: full mode renders all 5 sections in order; compact mode omits file list/github row; refresh button gated on `kind === "live"`; `aria-live` wrapper present.
- Files: `web-app/src/components/shared/VcsWidget.test.tsx`

##### Task 2.2.1i: Audit and enforce 44×44px minimum touch targets on mobile (~5 min)
- Add a shared `minTouchTarget` style (`min-height: 44px; min-width: 44px;`) to `web-app/src/styles/theme.css.ts` or a small shared recipe mixin, and apply it to every `<button>` across `VcsWidgetHeader.css.ts` (copy/browse buttons, Task 2.1.2d), `VcsWidgetFileList.css.ts` (file-row buttons, "Show all N files", Task 2.1.3e), `VcsWidgetCommitList.css.ts` (commit-row expand, "Show all N commits", Task 2.1.4c), and `VcsWidget.css.ts`'s own refresh button, scoped to the compact/mobile breakpoint at minimum (full/desktop may already exceed 44px via existing padding) — adversarial-review Concern.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetHeader.css.ts`, `VcsWidgetFileList.css.ts`, `VcsWidgetCommitList.css.ts`, `web-app/src/components/shared/VcsWidget.css.ts`

#### Story 2.2.2: Wire `VcsWidget` into Session detail (replaces `VcsPanel`'s bespoke rendering)
**As a** user viewing a session's VCS tab, **I want** the same rich widget as everywhere else, **so that** the session view stops being the only place with full richness.
**Acceptance Criteria**:
- `VcsPanel.tsx` calls `fromSessionVcs(vcsStatus, session)` and renders `<VcsWidget data={...} mode="full" onNavigateToFile={onNavigateToFile} onRefresh={refresh} />` instead of its current hand-rolled JSX.
  - *Given* a live session with `githubPrNumber: 42` and a dirty worktree, *When* the Session detail VCS tab renders, *Then* the DOM contains the same information `VcsPanel` renders today (branch, PR badge, file lists, CI status) but sourced through `VcsWidget`'s `data-testid="vcs-widget-loaded"` element rather than `VcsPanel`'s legacy markup.
- No regression: every field `VcsPanel` renders today (per requirements.md's Success Metrics "no loss of any feature") is reachable in the new rendering.
  - *Given* the pre-change `VcsPanel.tsx` and post-change version, *When* both are diffed against the requirements' In Scope feature list (GitHub PR/CI badges, branch/clean-dirty/ahead-behind, categorized clickable file lists, worktree path), *Then* every item has a corresponding rendering path in the new `VcsWidget`-based `VcsPanel.tsx`.
**Files**: `web-app/src/components/sessions/VcsPanel.tsx` (rewrite), `web-app/src/components/sessions/VcsPanel.css.ts` (delete or trim to only what's still needed), `web-app/src/components/sessions/VcsPanel.test.tsx` (new or updated)

##### Task 2.2.2a: Replace `VcsPanel.tsx`'s JSX body with `VcsWidget` (~5 min)
- Keep `VcsPanel`'s existing `useSessionVcsContext()` data-fetching (`useSessionVcs.ts`) unchanged; replace only the render body with `fromSessionVcs(...)` + `<VcsWidget mode="full" .../>`.
- Files: `web-app/src/components/sessions/VcsPanel.tsx`

##### Task 2.2.2b: Remove now-dead `FILE_STATUS_META`/`ciColor`/inline styles from `VcsPanel.tsx` (~3 min)
- Delete the hand-rolled rendering helpers now superseded by the sub-components; keep only data-fetching/loading/error-state logic specific to the live path.
- Files: `web-app/src/components/sessions/VcsPanel.tsx`, `web-app/src/components/sessions/VcsPanel.css.ts`

##### Task 2.2.2c: Update/replace `VcsPanel` tests to assert through `VcsWidget`'s `data-testid` (~4 min)
- Update existing Jest tests (if any) to query `data-testid="vcs-widget-loaded"` and its children rather than removed `VcsPanel`-local class names.
- Files: `web-app/src/components/sessions/VcsPanel.test.tsx`

#### Story 2.2.3: Wire `VcsWidget` into Backlog item detail (replaces `VcsStatusDisplay`/`ShipStatusDisplay` fallback)
**As a** user viewing a backlog item, **I want** the same widget Session detail uses, **so that** the Backlog view stops being the poorer of the two.
**Acceptance Criteria**:
- `BacklogItemDetail.tsx`'s Version Control section (~line 1248) calls `fromSessionVcs(vcsStatus)` when `vcsStatus` is present, else `fromShipStatus(shipStatus)`, and renders one `<VcsWidget mode="full" onViewDiff={() => setShowChangesModal(true)} />` — no `onNavigateToFile` (Backlog detail has no Files tab, per ADR-implicit decision in architecture research §4).
  - *Given* `vcsStatus === null` (worktree cleaned up) and `shipStatus` present with `shipped: true`, `shippedVia: "pr"`, *When* the Version Control section renders, *Then* the DOM shows `VcsWidget`'s `MergeabilityPill` in `"shipped"` state and a "View Diff" affordance that opens `ReviewChangesModal` on click.
- The existing fallback-by-data-presence pattern (`vcsStatus ?? shipStatus`, not a mode prop) is preserved exactly, per features research §1's explicit recommendation to carry this pattern into the new widget's data-source selection.
  - *Given* both `vcsStatus` and `shipStatus` resolve non-null (a live worktree exists for an item not yet fully done), *When* the section renders, *Then* `fromSessionVcs(vcsStatus)` (the live path) is used, not `fromShipStatus`.
- No legacy field/interaction is deleted without first being traced to a specific `VcsWidget` sub-component (Task 2.2.3d's parity checklist), not just a grep for remaining imports (adversarial-review Concern).
  - *Given* Task 2.2.3d's parity checklist, *When* it is reviewed before Task 2.2.3e's deletion runs, *Then* every row in the checklist names a concrete `VcsWidget`/sub-component destination — none are blank or "N/A".
- When `linkedSessions` contains more than one session with an active work role, `VcsWidget` (via `VcsWidgetHeader`'s `activeSessionCount` prop, Task 2.1.2f) shows the "N active sessions" indicator rather than silently picking one via `.reverse().find()` with no visible sign a second session exists (adversarial-review Concern; the underlying "most recent work session" selection heuristic itself is unchanged, per requirements' scope).
  - *Given* `linkedSessions` with 2 sessions both having `role === "work"` and `status === "active"`, *When* the Version Control section renders, *Then* `activeSessionCount={2}` is passed to `VcsWidget`, and the header shows "2 active sessions".
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx` (edit ~lines 1244-1279)

##### Task 2.2.3a: Replace `VcsStatusDisplay`/`ShipStatusDisplay` JSX with `VcsWidget` (~5 min)
- Edit the block at `BacklogItemDetail.tsx:1248-1277`: `const widgetData = vcsStatus ? fromSessionVcs(vcsStatus) : shipStatus ? fromShipStatus(shipStatus) : null;` then `{widgetData && <VcsWidget data={widgetData} mode="full" onViewDiff={() => setShowChangesModal(true)} activeSessionCount={activeWorkSessionCount} />}`.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 2.2.3b: Keep worktree-path copy/browse row wired via `VcsWidgetHeader`'s prop, not duplicated inline (~3 min)
- Pass `worktreePath={latestWorkSession.worktreePath}` into `VcsWidget` (threaded to `VcsWidgetHeader`) instead of `BacklogItemDetail.tsx`'s separate inline copy/browse JSX block (Task 2.1.2c already implements this row inside the sub-component) — remove the now-duplicate inline block.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 2.2.3c: Remove now-unused `VcsStatusDisplay`/`ShipStatusDisplay` imports (~2 min)
- Delete the imports; leave the component files themselves in place for Story 2.2.3's own reference/rollback safety but confirm no other call site still imports them (`grep -rn "VcsStatusDisplay\|ShipStatusDisplay" web-app/src`) — if truly unused elsewhere, delete the files in a follow-up cleanup task.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 2.2.3d: Build and confirm the feature-parity checklist gate before deletion (~6 min)
- Before any deletion, enumerate every field/interaction currently rendered by `VcsStatusDisplay.tsx`, `ShipStatusDisplay.tsx`, and `VcsPanel.tsx` (per requirements.md's Success Metrics "no loss of any feature") in a short markdown table (in the PR description or a scratch file, not committed to the repo) with columns `Legacy field/interaction | Legacy source file | VcsWidget sub-component it now lands in`. Every row must resolve to a specific `VcsWidget`/sub-component location — no row may say "N/A" or be left blank. This is a stronger gate than Task 2.2.3c's import-grep — it verifies parity by content, not just by absence of a stale import (adversarial-review Concern). Do not proceed to Task 2.2.3e until every row resolves.
- Files: none (verification checklist only, not a code change)

##### Task 2.2.3e: Delete `VcsStatusDisplay.tsx`/`ShipStatusDisplay.tsx` and their `.css.ts`/`.test.tsx` if confirmed unused (~4 min)
- Run the grep from Task 2.2.3c and confirm Task 2.2.3d's parity checklist is complete; if zero remaining references and all rows resolved, delete `web-app/src/components/shared/VcsStatusDisplay.tsx`, `.css.ts`, and `web-app/src/components/backlog/ShipStatusDisplay.tsx`, `.css.ts`, `.test.tsx`.
- Files: `web-app/src/components/shared/VcsStatusDisplay.tsx` (+`.css.ts`), `web-app/src/components/backlog/ShipStatusDisplay.tsx` (+`.css.ts`,`.test.tsx`) — delete

##### Task 2.2.3f: Compute and thread `activeSessionCount` from `linkedSessions` (~3 min)
- Compute `const activeWorkSessionCount = linkedSessions.filter(s => s.role === "work" && s.status === "active").length` alongside the existing `latestWorkSession` selection (`.reverse().find(...)`), and pass it as `activeSessionCount` to `VcsWidget` (Task 2.2.3a). This does not change which session's worktree path/data is shown — only makes the multi-session ambiguity visible (adversarial-review Concern).
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

#### Story 2.2.4: Wire `VcsWidget` (compact mode) into Unfinished item detail
**As a** user scanning the Unfinished-work dashboard, **I want** the same compact widget density used elsewhere, **so that** compact-mode logic isn't duplicated a fourth time.
**Acceptance Criteria**:
- `UnfinishedItemDetail.tsx`'s diff-stats + commit-message rendering (lines ~88-119) is replaced by `<VcsWidget data={fromUnfinishedWorktree(worktree)} mode="compact" />`, with the action-button row (Reattach/Open Session, Commit & Push, View Diff, Summarize) staying as surrounding chrome outside `VcsWidget` (per architecture research §4 — `VcsWidget` owns only read-only display).
  - *Given* an `UnfinishedWorktree` with `changedFiles: 5, linesAdded: 42, linesRemoved: 8, aheadCommitMessages: ["fix: typo"]`, *When* `UnfinishedItemDetail` renders, *Then* the DOM shows the same "5 files changed +42 -8" and commit-message content as before, now sourced through `VcsWidget mode="compact"`, and the action buttons (`Reattach Session`, `View Diff`, `Summarize`) still render unchanged below it.
**Files**: `web-app/src/components/unfinished/UnfinishedItemDetail.tsx` (edit)

##### Task 2.2.4a: Replace diff-stats/commit-list JSX with `<VcsWidget mode="compact">` (~5 min)
- Remove the `statsRow`/`commitList` JSX block (lines ~88-119); insert `<VcsWidget data={fromUnfinishedWorktree(worktree)} mode="compact" />` in its place, keeping `noChanges` early-return logic (`VcsWidget` itself doesn't need a "no changes" special case — `aggregateStats.filesChanged === 0` naturally renders nothing via the compact aggregate-line's own zero-guard, added in Task 2.2.1b).
- Files: `web-app/src/components/unfinished/UnfinishedItemDetail.tsx`

##### Task 2.2.4b: Verify action-button row (Reattach/Commit/Diff/Summarize) is unaffected (~3 min)
- Confirm `handleOpenSession`, `showCommitModal`, `showDiffModal`, `handleSummarize` logic is untouched — this story only replaces the read-only display block.
- Files: `web-app/src/components/unfinished/UnfinishedItemDetail.tsx`

##### Task 2.2.4c: Update `UnfinishedItemDetail`'s existing tests (if any) for the new markup (~3 min)
- Adjust any Jest/RTL assertions that queried the removed `styles.statsRow`/`styles.commitList` class names to instead query `VcsWidget`'s `data-testid`.
- Files: `web-app/src/components/unfinished/UnfinishedItemDetail.test.tsx` (if exists — create if not, per project convention of co-located tests)

##### Task 2.2.4d: Verify PR/CI visibility isn't lost in compact mode on this surface (~4 min, pre-mortem P2 fix)
- `VcsWidget mode="compact"` intentionally omits `VcsWidgetGithubRow` (Task 2.2.1b), on the assumption that a `GitHubBadge` is already rendered separately wherever compact mode is used. Grep `UnfinishedItemDetail.tsx` for an existing `<GitHubBadge` usage. If present, this task is a no-op — confirm it stays untouched by Task 2.2.4a's edit. If **absent**, this is a real feature-loss gap against `fromUnfinishedWorktree`'s own computed `github` data (Task 1.1.4c) never landing anywhere — add `<GitHubBadge compact pr={...} />` (or equivalent) alongside `<VcsWidget mode="compact">` in this story's JSX, sourced from the same `worktree.githubPr*` fields `fromUnfinishedWorktree` already reads. Record the outcome (present vs. added) in this task's completion notes — it's also required input to Task 2.2.3d's parity checklist, which this story's own scope was previously excluded from per pre-mortem P2 finding #3.
- Files: `web-app/src/components/unfinished/UnfinishedItemDetail.tsx`

---

## Phase 3: Backend Durable Snapshot Persistence (Go/ent/proto — Independent of Phase 1/2)

### Epic 3.0: Spike — Verify `go-git`'s `Patch().Stats()` Behavior (Before Any Schema Work)

#### Story 3.0.1: Spike `object.Patch().Stats()` against a real rename, a real binary-file commit, AND the actual production SHA-resolution shape
**As a** backend developer, **I want** to verify go-git's `Patch().Stats()` API against real rename and binary-file data, AND verify that `FileStatsBetween` can actually resolve its SHA pair under this project's real ship workflow (squash-merge + delete-branch, worktree already removed by the time the reconciler runs) in this repo before any schema/proto work is built on top of the assumption, **so that** a go-git limitation *or* a SHA-resolution gap surfaces before Phase 3.1's schema work is sunk (adversarial-review Concern: `research/build-vs-buy.md` §5 recommends this API based on documented surface, not verified behavior; pre-mortem P1: the spike as originally scoped only exercised static existing history, never the actual "worktree/branch already deleted" shape `CaptureShipSnapshot` will hit in production).
**Acceptance Criteria**:
- A throwaway spike script (not committed, or committed as a `_spike_test.go` deleted at the end of this story) runs `object.Patch().Stats()` against (a) a real commit pair in this repo's own history containing a file rename with no content change, and (b) a real commit pair containing a binary file change (e.g. an image), and the actual observed `FileStat` output for each is recorded in this task's completion notes.
  - *Given* a commit pair `git log --diff-filter=R` identifies as a pure rename in this repo, *When* the spike calls `object.Patch().Stats()` on that pair, *Then* the output is recorded: does it report one entry for the renamed file (expected) or a delete+add pair (fallback-needed)?
  - *Given* a commit pair touching a binary file, *When* the spike runs, *Then* the output is recorded: does `Patch().Stats()` error, report `0/0`, or omit the entry entirely?
- **Production-shape verification (pre-mortem P1 fix):** the spike additionally picks a real, already-merged-and-branch-deleted PR from this repo's own GitHub history (per `.claude/rules/gh-pr-merge-repo-flag.md`'s standard `--squash --delete-branch` convention — pick any closed PR from `gh pr list --state merged --limit 5`), and calls `FileStatsBetween` using exactly the SHA pair `CaptureShipSnapshot` will actually have available at call time: the squash-merge commit's SHA on `main` as `headSHA` and its immediate parent on `main` as `baseSHA` (NOT the original pre-squash feature-branch commits, which become unreachable once the branch ref is deleted) — resolved against the *canonical repo path* (`item.RepoPath` as `CaptureShipSnapshot` will see it — verify whether this is the now-deleted worktree path or the shared canonical clone path that survives worktree removal, since git worktrees share one object database with the main checkout).
  - *Given* a real merged PR's squash-commit SHA and its parent SHA on `main`, resolved via the canonical repo path (not a worktree path that may have been removed), *When* the spike calls `git.PlainOpenWithOptions` + `.CommitObject()` on both SHAs, *Then* the result is recorded: do both SHAs resolve successfully with no `object not found` error, confirming worktree/branch deletion does not evict the commit objects needed for the diff?
  - *Given* the same commit pair, *When* `.Patch().Stats()` runs, *Then* the aggregate `+X/-Y` matches what `gh pr diff <PR#> --stat` reports for the same PR (cross-check against the real PR, not just internal consistency).
- If either the rename/binary case OR the production-shape case does NOT behave as Story 3.2.1/3.3.1 currently assume (single rename entry; binary reported as 0/0 with no error; SHAs always resolve against `item.RepoPath`), the relevant task's acceptance criteria are updated in this same story to specify the actual observed behavior and any required fallback (e.g. "resolve against the shared canonical repo path, not `item.RepoPath` if that has been repointed/removed" or "fall back to `gh pr diff --stat` parsing if local SHA resolution fails post-cleanup") — Story 3.2.1/3.3.1 must not proceed against an unverified assumption.
**Files**: none committed (spike is throwaway; only this story's own acceptance-criteria text is the durable artifact, updating Story 3.2.1/Task 3.2.1a/Story 3.3.1 if an assumption doesn't hold)

##### Task 3.0.1a: Write and run the throwaway spike against a real rename commit pair (~5 min)
- Use `git log --follow --diff-filter=R -- '*'` (or similar) to find a real rename in this repo's history; open the repo via `git.PlainOpenWithOptions`, resolve the two commit SHAs, call `.Patch().Stats()`, print the result.
- Files: none (throwaway; run via `go run` from a scratch `main.go` or `go test -run` scaffold, not committed)

##### Task 3.0.1b: Write and run the throwaway spike against a real binary-file commit pair (~4 min)
- Find a commit that added/modified a binary file (e.g. an image under `web-app/public/` or similar) in this repo's history; repeat the `.Patch().Stats()` call; print the result.
- Files: none (throwaway)

##### Task 3.0.1d: Write and run the throwaway spike against a real merged-and-branch-deleted PR's squash-commit pair (~6 min)
- Pick a real merged PR via `gh pr list --state merged --limit 5 --repo <this repo>`, get its squash-commit SHA (`gh pr view <N> --json mergeCommit`) and that commit's parent SHA on `main`. Resolve both against the canonical repo path this Go process would actually use (not a worktree subdirectory), call `.Patch().Stats()`, and cross-check the aggregate against `gh pr diff <N> --stat`. This is the exact scenario the pre-mortem flagged as untested: SHA resolution *after* the worktree and branch that produced the commits are already gone.
- Files: none (throwaway)

##### Task 3.0.1c: Record observed behavior and update Story 3.2.1/Task 3.2.1a/Story 3.3.1 if needed (~4 min)
- Write the 3 observed outcomes (rename, binary, production-shape SHA resolution) into this task's notes (PR description or commit message is sufficient — no new doc file required). If observed behavior differs from Story 3.2.1's or Story 3.3.1's current assumptions — especially if `item.RepoPath` does NOT reliably resolve post-cleanup — edit the relevant task's text in this plan before Epic 3.1/3.3 proceeds.
- Files: `project_plans/unified-vcs-widget/implementation/plan.md` (Task 3.2.1a and/or Story 3.3.1, only if the spike reveals a discrepancy)

### Epic 3.1: Proto and ent schema extensions

#### Story 3.1.1: Extend `BacklogItemShipStatus` proto with snapshot fields
**As a** backend developer, **I want** new proto fields for the durable GitHub/diff-stat snapshot, **so that** the frontend can eventually render them via `make proto-gen`'s regenerated TypeScript bindings.
**Acceptance Criteria**:
- `proto/session/v1/backlog.proto`'s `BacklogItemShipStatus` message gains `string shipped_check_conclusion = 13;`, `int32 shipped_approved_count = 14;`, `int32 shipped_changes_req_count = 15;`, `repeated ShippedFileStat file_stats = 16;`, `google.protobuf.Timestamp snapshot_at = 17;`, `bool snapshot_capture_failed = 18;`, and a new `message ShippedFileStat { string path = 1; session.v1.FileStatus status = 2; int32 additions = 3; int32 deletions = 4; }` (importing `types.proto` for the `FileStatus` enum). `shipped_check_conclusion` holds only genuine GitHub CI-conclusion values (or is unset) — `snapshot_capture_failed` is the dedicated signal for a capture failure, per the architecture-review BLOCKER fix (finding: `ShippedCheckConclusion` must not be overloaded with a `"failed"` sentinel).
  - *Given* the edited `backlog.proto`, *When* `make proto-gen` runs, *Then* it completes without error and `session/gen/session/v1/backlog_pb.go` contains a `ShippedFileStat` struct with `Path`, `Status`, `Additions`, `Deletions` fields, and `BacklogItemShipStatus` gains `ShippedCheckConclusion`, `ShippedApprovedCount`, `ShippedChangesReqCount`, `FileStats`, `SnapshotAt`, `SnapshotCaptureFailed` fields.
**Files**: `proto/session/v1/backlog.proto`

##### Task 3.1.1a: Add `import "session/v1/types.proto";` to `backlog.proto` (~2 min)
- Add the import line near the existing `google/protobuf/timestamp.proto` import (line 5).
- Files: `proto/session/v1/backlog.proto`

##### Task 3.1.1b: Add `ShippedFileStat` message (~3 min)
- Add the new message definition (path/status/additions/deletions) directly below `ShippedCommit` (line ~241).
- Files: `proto/session/v1/backlog.proto`

##### Task 3.1.1c: Add 6 new fields to `BacklogItemShipStatus` (~4 min)
- Add fields 13-18 as specified (including `bool snapshot_capture_failed = 18;`), each with a doc comment explaining it's populated only when a durable snapshot exists (nil/zero-value otherwise). Doc-comment `shipped_check_conclusion` explicitly: "genuine GitHub CI-conclusion values only — never a capture-failure sentinel; see `snapshot_capture_failed`."
- Files: `proto/session/v1/backlog.proto`

##### Task 3.1.1d: Run `make proto-gen` and commit regenerated files (~3 min)
- Run `make proto-gen`; verify `session/gen/session/v1/backlog_pb.go` and `web-app/src/gen/session/v1/backlog_pb.ts` both regenerate with the new fields; stage both alongside the proto edit (per CLAUDE.md's proto workflow, `web-app/src/gen` is tracked despite `.gitignore`).
- Files: `session/gen/session/v1/backlog_pb.go`, `web-app/src/gen/session/v1/backlog_pb.ts`

#### Story 3.1.2: Add 6 new `BacklogItem` ent fields
**As a** backend developer, **I want** the durable snapshot fields on the ent schema, **so that** `CaptureShipSnapshot` (Story 3.3.1) has somewhere to write.
**Acceptance Criteria**:
- `session/ent/schema/backlog_item.go` gains the 6 fields specified in ADR-002 (ADR-025 in `docs/adr/`) as amended by the architecture-review BLOCKER fix — 5 original fields plus `field.Bool("shipped_snapshot_capture_failed").Optional().Default(false)` as a field distinct from `shipped_check_conclusion` — and `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` completes without error.
  - *Given* the edited schema file, *When* `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` runs followed by `go build ./...`, *Then* both complete with exit code 0 and `session/ent/backlogitem/backlogitem.go` contains `FieldShippedCheckConclusion`, `FieldShippedApprovedCount`, `FieldShippedChangesReqCount`, `FieldShippedSnapshotAt`, `FieldShippedFileStats`, `FieldShippedSnapshotCaptureFailed` constants.
**Files**: `session/ent/schema/backlog_item.go`, `session/ent/` (regenerated, many files)

##### Task 3.1.2a: Add the 6 `field.*` declarations to `BacklogItem.Fields()` (~5 min)
- Insert after the existing `pr_number` field (line ~74): the 5 fields from ADR-002 plus `field.Bool("shipped_snapshot_capture_failed").Optional().Default(false).Comment("true when CaptureShipSnapshot's GitHub fetch or file-stats computation failed — distinct from shipped_check_conclusion, which holds only genuine CI-conclusion values")`.
- Files: `session/ent/schema/backlog_item.go`

##### Task 3.1.2b: Run ent codegen with `--feature sql/upsert` and `go build ./...` (~3 min)
- Run the exact command from `session/ent/generate.go`; verify build succeeds; stage all regenerated `session/ent/` files in the same commit as the schema edit (per `.claude/rules/ent-schema-generation.md`'s "always commit together" requirement).
- Files: `session/ent/` (regenerated)

#### Story 3.1.3: Extend `BacklogItemUpdate`/`BacklogItemData` Go structs
**As a** backend developer, **I want** the new fields threaded through the existing partial-update and read-model structs, **so that** `CaptureShipSnapshot` and `GetBacklogItemShipStatus` can read/write them without a raw ent client call.
**Acceptance Criteria**:
- `BacklogItemUpdate` (`session/repository.go:439`) gains `ShippedCheckConclusion *string`, `ShippedApprovedCount *int`, `ShippedChangesReqCount *int`, `ShippedSnapshotAt *time.Time`, `ShippedFileStats *string` (JSON-encoded), `ShippedSnapshotCaptureFailed *bool`, following the existing pointer-for-partial-update convention.
  - *Given* a `BacklogItemUpdate{ShippedCheckConclusion: ptr("success"), ShippedApprovedCount: ptr(2)}` passed to `storage.UpdateBacklogItem(ctx, itemID, update, nil)`, *When* the call completes, *Then* a subsequent `storage.GetBacklogItem(ctx, itemID)` returns a `BacklogItemData` with `ShippedCheckConclusion: "success"`, `ShippedApprovedCount: 2`, and all other fields unchanged from before the update (partial-update semantics preserved).
  - *Given* a `BacklogItemUpdate{ShippedSnapshotCaptureFailed: ptr(true)}` (no `ShippedCheckConclusion` set) passed to `storage.UpdateBacklogItem`, *When* the call completes, *Then* `BacklogItemData.ShippedSnapshotCaptureFailed == true` and `ShippedCheckConclusion` remains its prior/zero value — confirming the two fields are written and read independently.
- `BacklogItemData` (`session/repository.go:342`) gains the corresponding non-pointer read fields.
**Files**: `session/repository.go`, wherever `BacklogItemUpdate`→ent mapping happens (likely `session/ent_repository_backlog.go` or `session/storage.go`)

##### Task 3.1.3a: Add 6 pointer fields to `BacklogItemUpdate` (~4 min)
- Includes `ShippedSnapshotCaptureFailed *bool`.
- Files: `session/repository.go`

##### Task 3.1.3b: Add 6 non-pointer fields to `BacklogItemData` (~4 min)
- Includes `ShippedSnapshotCaptureFailed bool`.
- Files: `session/repository.go`

##### Task 3.1.3c: Wire the new `BacklogItemUpdate` fields into the ent update-builder call (~5 min)
- Find the function building `client.BacklogItem.UpdateOneID(...).Set...()` calls from a `BacklogItemUpdate` (likely in `session/ent_repository_backlog.go`, adjacent to the existing `PrURL`/`PrNumber` mapping) and add the 6 new conditional `.Set*()` calls following the existing pointer-nil-check pattern.
- Files: `session/ent_repository_backlog.go` (or the actual file found via `grep -n "func.*UpdateBacklogItem" session/*.go`)

##### Task 3.1.3d: Wire the new ent fields into the `BacklogItemData` read-mapping function (~4 min)
- Find the ent-entity-to-`BacklogItemData` mapper (likely in the same file as 3.1.3c) and add the 6 new field reads.
- Files: `session/ent_repository_backlog.go`

### Epic 3.2: Per-file diff-stat computation

#### Story 3.2.1: `git.FileStatsBetween` helper
**As a** backend developer, **I want** a go-git-based per-file diff-stat function between two SHAs, **so that** `CaptureShipSnapshot` can compute `ShippedFileStats` without a worktree. (Epic 3.0's spike has already verified rename/binary-file handling against real repo data before this story's implementation begins — if the spike found a discrepancy, Task 3.2.1a's acceptance criteria below were already updated per Task 3.0.1c.)
**Acceptance Criteria**:
- `FileStatsBetween(repoPath, baseSHA, headSHA)` returns `[]FileStat{Path, Status, Additions, Deletions}` using go-git's `object.Patch().Stats()`, with no `safeexec.CommandContext` shell-out.
  - *Given* a test repo at `repoPath` with `baseSHA` = the repo's initial commit and `headSHA` = a later commit that added 5 lines to `src/foo.go` and deleted 2 lines from `src/bar.go`, *When* `FileStatsBetween(repoPath, baseSHA, headSHA)` is called, *Then* it returns `[]FileStat{{Path: "src/foo.go", Additions: 5, Deletions: 0}, {Path: "src/bar.go", Additions: 0, Deletions: 2}}` (order per go-git's patch stat order) with no error.
- Renamed files report a single stat entry with the new path, not a delete+add pair.
  - *Given* a commit range where `src/old.go` was renamed to `src/new.go` with no content change, *When* `FileStatsBetween` is called, *Then* the result contains one entry for `src/new.go` (verifying go-git's native rename detection, the exact class of bug a hand-parsed `git diff --numstat` would risk mishandling).
**Files**: `session/git/ops.go`, `session/git/ops_test.go`

##### Task 3.2.1a: Implement `FileStatsBetween` using `object.Patch().Stats()` (~5 min)
- Open repo via `git.PlainOpenWithOptions` (per `.claude/rules/prefer-go-git-over-subshells.md`'s established pattern, mirroring `IsCommitOnMain`'s structure at `session/git/ops.go:46`), resolve both SHAs to `*object.Commit`, call `baseCommit.Patch(headCommit)` then `.Stats()`, map `object.FileStat{Name, Addition, Deletion}` to the new `FileStat{Path, Additions, Deletions}` type (add a `Status` field derived from whether the file exists in base/head trees — added/deleted/renamed/modified).
- Files: `session/git/ops.go`

##### Task 3.2.1b: Define `FileStat` struct if not already present (~2 min)
- `type FileStat struct { Path string; Status string; Additions int; Deletions int }` near the existing `ShippedCommit`/`BranchStatus` struct definitions in `session/git/ops.go`.
- Files: `session/git/ops.go`

##### Task 3.2.1c: Unit tests for `FileStatsBetween` (~5 min)
- Tests mirroring `ops_test.go`'s existing style (`TestFileStatsBetween_should_ReturnPerFileCounts_When_...`): basic add/delete, rename detection, binary file handling (go-git reports 0/0 for binary — assert this doesn't error), empty range (`baseSHA == headSHA` → empty slice).
- Files: `session/git/ops_test.go`

### Epic 3.3: Snapshot capture at ship time

#### Story 3.3.1: `CaptureShipSnapshot` + wiring into `ReconcilePRPending`
**As a** backend developer, **I want** the merge-detection point in `ReconcilePRPending` to capture and persist the GitHub/diff-stat snapshot before transitioning to `done`, **so that** the data survives worktree cleanup (the core requirement).
**Acceptance Criteria**:
- `CaptureShipSnapshot` is a free function `func CaptureShipSnapshot(ctx context.Context, storage *Storage, item *BacklogItemData, prStatus *git.PRStatus, lastWork *SessionData, wt *WorktreeData) error` in `session/backlog_lifecycle.go` — **not** a method on `BacklogLifecycleListener` (architecture-review Concern: the function needs no state from that type's receiver beyond `*Storage`, which is passed explicitly; per `.claude/rules/interface-pollution-checklist.md`, a method only earns its receiver when it genuinely needs that type's other state). It is called synchronously, immediately before the `TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, ...)` call at `session/backlog_lifecycle.go:94` (inside the `if merged {` block), never as a background goroutine.
  - *Given* `ReconcilePRPending` detects `merged == true` for an item with `PrNumber: 42`, *When* the merged branch executes, *Then* `CaptureShipSnapshot` runs and returns before `TransitionBacklogItemStatus` is called on the same goroutine (verifiable by a test double `prPendingChecker` that records call order into a slice, asserting `["CaptureShipSnapshot", "TransitionBacklogItemStatus"]` ordering — or more directly, a test asserting `BacklogItem.ShippedSnapshotAt` is non-nil immediately after `ReconcilePRPending` returns, without any `time.Sleep`/polling in the test).
- `CaptureShipSnapshot` maps the **already-fetched** `prStatus` (passed in by `ReconcilePRPending`, which obtained it at the merge-detection point — `CaptureShipSnapshot` makes no GitHub call of its own) into the 3 GitHub snapshot fields, and independently calls `git.FileStatsBetween(item.RepoPath, wt.BaseCommitSHA, lastWork.LastCommitSha)` for the file-stats fields, writing whatever succeeds via one `UpdateBacklogItem` call.
  - *Given* `prStatus = &git.PRStatus{ApprovedCount: 2, ChangesRequestedCount: 0}` and `FileStatsBetween` returns 3 file stats, *When* `CaptureShipSnapshot` runs, *Then* `storage.UpdateBacklogItem` is called with `ShippedApprovedCount: ptr(2)`, `ShippedChangesReqCount: ptr(0)`, `ShippedFileStats: ptr(<JSON-encoded 3-entry array>)`, `ShippedSnapshotAt: ptr(<current time>)`.
- **The two data groups — (A) GitHub PR/CI/review state from `prStatus`, and (B) per-file diff stats from `FileStatsBetween` — succeed or fail independently; a failure in one must never discard a success in the other.** Whichever group succeeds is written to its corresponding fields; whichever group fails leaves its fields unset/zero and sets `ShippedSnapshotCaptureFailed: ptr(true)` (never `ShippedCheckConclusion: ptr("failed")` — that field is reserved for genuine CI-conclusion values, per the architecture-review BLOCKER fix). `ShippedSnapshotAt` is still set to the current time whenever at least one group succeeds, so a partially-captured snapshot is still distinguishable from "never captured at all."
  - *Given* `prStatus` is available and maps successfully but `git.FileStatsBetween` returns an error (e.g. `baseSHA` has been pruned), *When* `CaptureShipSnapshot` runs, *Then* it logs a Warning-level message for the file-stats failure, writes `ShippedApprovedCount`/`ShippedChangesReqCount`/`ShippedCheckConclusion` from the successfully-mapped `prStatus` data, leaves `ShippedFileStats` unset, and sets `ShippedSnapshotCaptureFailed: ptr(true)` — the successfully-captured GitHub review/CI data is not discarded because file-stats failed.
  - *Given* `prStatus == nil` (the caller's own `GetPRStatus` call failed before invoking `CaptureShipSnapshot`) but `FileStatsBetween` succeeds, *When* `CaptureShipSnapshot` runs, *Then* it writes the successfully-computed `ShippedFileStats`, leaves `ShippedCheckConclusion`/`ShippedApprovedCount`/`ShippedChangesReqCount` unset, and sets `ShippedSnapshotCaptureFailed: ptr(true)`.
- On a failure in either group, `CaptureShipSnapshot` does **not** block the `done` transition — the transition proceeds regardless (fails-closed on data completeness, not on the workflow itself, since blocking `done` on a GitHub API hiccup or a pruned SHA would leave a genuinely-merged item stuck in `pr_pending` forever).
  - *Given* both groups fail, *When* `CaptureShipSnapshot` runs, *Then* `ReconcilePRPending` still calls `TransitionBacklogItemStatus(..., BacklogStatusDone, ...)` afterward — the item still reaches `done`, with `ShippedSnapshotCaptureFailed: true` and `ShippedSnapshotAt` set (so the frontend can distinguish "we tried and both groups failed" from "capture code never ran," per `MergeabilityState`'s `"snapshot_unavailable"` branch, Task 1.1.5c).
**Files**: `session/backlog_lifecycle.go` (new function + call site edit)

##### Task 3.3.1a: Implement `CaptureShipSnapshot(ctx, storage, item, prStatus, lastWork, wt) error` as a free function, with PR-status mapping (~5 min)
- New free function in `session/backlog_lifecycle.go` (explicitly **not** a method on `BacklogLifecycleListener` — see the architecture-review Concern in this story's acceptance criteria) that maps `*git.PRStatus` fields into the 3 GitHub snapshot fields when `prStatus != nil` (`ApprovedCount`→`ShippedApprovedCount`, `ChangesRequestedCount`→`ShippedChangesReqCount`; `CheckConclusion` is not on `git.PRStatus` today per `worktree_git.go:330-345`'s field list — derive `ShippedCheckConclusion` from `CIFailing` as `"failure"`/`"success"` since `PRStatus` doesn't expose the raw conclusion string; note this as a minor fidelity gap vs. the richer `Session.githubCheckConclusion` string, acceptable since `CIFailing` is the actionable signal `MergeabilityState` needs anyway). When `prStatus == nil`, skip this group entirely (leave the 3 fields unset) and track that group A failed. `CaptureShipSnapshot` itself makes **no new GitHub HTTP call** — `prStatus` is already fetched by the existing `GetPRStatus` call in `ReconcilePRPending`'s call site, which already goes through `github.DefaultRateLimiter` (adversarial-review Concern: rate-limiter reuse) — so no new rate-limit exposure is introduced by this story.
- Files: `session/backlog_lifecycle.go`

##### Task 3.3.1b: Implement file-stats capture + JSON encoding, independent of the PR-status group (~4 min)
- Call `git.FileStatsBetween(item.RepoPath, wt.BaseCommitSHA, lastWork.LastCommitSha)` in its own error-handling branch (not gated on group A's outcome), `json.Marshal` the result into the `ShippedFileStats` string field on success; on error, leave `ShippedFileStats` unset and track that group B failed.
- Files: `session/backlog_lifecycle.go`

##### Task 3.3.1c: Implement independent per-group failure tracking → single `ShippedSnapshotCaptureFailed` signal + non-blocking Warning log (~5 min)
- Track group A and group B success/failure independently (e.g. two local `bool` variables); log a Warning via `log.WarningLog.Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=%s: %v", ...)` per failed group (not blocking); after both groups have been attempted, set `ShippedSnapshotCaptureFailed: ptr(true)` on the `BacklogItemUpdate` if **either** group failed, and always set `ShippedSnapshotAt: ptr(now)` if **at least one** group succeeded (so a snapshot with partial data is still marked as "captured, some fields missing" rather than "never captured"). Never write `ShippedCheckConclusion: ptr("failed")` — that field only ever holds real CI-conclusion values or stays unset. The function always returns `nil` to the caller (`CaptureShipSnapshot` never blocks the `done` transition regardless of how many groups failed).
- Files: `session/backlog_lifecycle.go`

##### Task 3.3.1d: Wire `CaptureShipSnapshot` call into `ReconcilePRPending`'s merged branch (~4 min)
- Insert the call inside `if merged {` (line ~92), before the `TransitionBacklogItemStatus` call at line 94 — pass the already-fetched `prStatus` (from the merge-detection `GetPRStatus`/`IsPRMerged` call already in scope), `wt` (worktree data, via `s.storage.GetWorktreeDataBySessionUUID`, same call `backlog_service_ship_status.go:88` already makes), and `lastWork` (via `ListItemSessions`, same pattern as `backlog_service_ship_status.go:44-58`) fetched first.
- Files: `session/backlog_lifecycle.go`

##### Task 3.3.1e: Tests for `CaptureShipSnapshot` ordering and independent-group-failure paths (~6 min)
- Tests: successful capture of both groups writes all 6 fields (`ShippedSnapshotCaptureFailed` unset/false) before `done` transition; `FileStatsBetween`-only failure still writes the successfully-captured GitHub fields and sets `ShippedSnapshotCaptureFailed: true`; `prStatus == nil`-only failure still writes the successfully-captured file-stats and sets `ShippedSnapshotCaptureFailed: true`; both-groups failure still allows the `done` transition with `ShippedSnapshotCaptureFailed: true` and no `ShippedCheckConclusion` sentinel string anywhere. Uses the existing `prPendingChecker` test-double seam (`SetPRPendingCheckerFactory`) already established in `backlog_lifecycle_test.go`-style tests.
- Files: `session/backlog_lifecycle_test.go`

##### Task 3.3.1f: Double-checked-locking correctness audit (no cache introduced — confirm) (~2 min)
- Confirm `CaptureShipSnapshot` introduces no in-process memoization/cache of the snapshot (it's a direct write-through per invocation) — explicitly note in a code comment that if a future caching layer is added on top, it must return the locally-computed value per `.claude/rules/go-double-checked-locking.md`, not re-read a cache slot.
- Files: `session/backlog_lifecycle.go` (comment only)

### Epic 3.4: RPC read-path population

#### Story 3.4.1: Populate `BacklogItemShipStatus`'s new fields from durable `BacklogItem` columns
**As a** frontend consumer, **I want** `GetBacklogItemShipStatus` to return the durable snapshot fields when present, **so that** `fromShipStatus` (Phase 4) has real data to adapt.
**Acceptance Criteria**:
- When `item.ShippedSnapshotAt != nil`, the response's `ShippedCheckConclusion`/`ShippedApprovedCount`/`ShippedChangesReqCount`/`FileStats`/`SnapshotAt`/`SnapshotCaptureFailed` are populated from the durable `BacklogItem` columns, with no live GitHub/git call for those specific fields (the whole point of the snapshot).
  - *Given* a `BacklogItem` with `ShippedSnapshotAt: <2026-07-17T10:00:00Z>`, `ShippedApprovedCount: 2`, `ShippedFileStats: <JSON for 3 files>`, `ShippedSnapshotCaptureFailed: false`, *When* `GetBacklogItemShipStatus` is called for that item, *Then* the response's `Status.ShippedApprovedCount == 2`, `Status.SnapshotAt` unmarshal to `2026-07-17T10:00:00Z`, `Status.FileStats` contains 3 `ShippedFileStat` entries matching the decoded JSON, and `Status.SnapshotCaptureFailed == false`.
- When `item.ShippedSnapshotAt == nil` (pre-feature item, or capture failed with no fields ever written), the response's snapshot fields are left at zero-value/empty — `fromShipStatus`'s `snapshotAt: null` mapping (Phase 4) then correctly triggers the "no history captured" copy.
  - *Given* a `BacklogItem` with `ShippedSnapshotAt: nil`, *When* `GetBacklogItemShipStatus` is called, *Then* `Status.SnapshotAt` is unset (proto zero-value) and `Status.FileStats` is empty.
- On `ShippedFileStats` JSON-unmarshal failure (corrupt/truncated data, or a schema-drift mismatch across a deploy that changed `ShippedFileStat`'s shape), the RPC does **not** fail — it logs the error with the backlog item ID and treats file-stats as absent, degrading gracefully rather than failing the whole `GetBacklogItemShipStatus` call. This is the architecture-review Concern fix (Story 3.4.1a previously had no defined unmarshal-failure behavior).
  - *Given* a `BacklogItem` with `ShippedSnapshotAt` non-nil but `ShippedFileStats: "{not valid json"` (corrupt), *When* `GetBacklogItemShipStatus` is called, *Then* the RPC still returns successfully (no error), a Warning-level log line is emitted containing the backlog item's ID and the unmarshal error, and `Status.FileStats` is empty — all other populated fields (`ShippedApprovedCount`, `SnapshotAt`, etc.) are still returned normally, per this repo's "don't just check errors, handle them gracefully" convention.
**Files**: `server/services/backlog_service_ship_status.go`

##### Task 3.4.1a: Add JSON-decode of `ShippedFileStats` into `[]*sessionv1.ShippedFileStat`, with graceful degradation on unmarshal failure (~5 min)
- After the existing commit-list-building block (line ~121), add: if `item.ShippedSnapshotAt != nil`, `json.Unmarshal([]byte(item.ShippedFileStats), &decoded)`; on success, map to proto `ShippedFileStat` messages, appended to `status.FileStats`. On error, log `log.WarningLog.Printf("[BacklogService] GetBacklogItemShipStatus item=%s: failed to decode ShippedFileStats: %v", item.ID, err)` and leave `status.FileStats` empty — do **not** return the error from the RPC handler (the rest of the response is still valid and useful).
- Files: `server/services/backlog_service_ship_status.go`

##### Task 3.4.1b: Populate scalar snapshot fields + `SnapshotAt` timestamp + `SnapshotCaptureFailed` (~4 min)
- `status.ShippedCheckConclusion = item.ShippedCheckConclusion`, `status.ShippedApprovedCount = int32(item.ShippedApprovedCount)`, etc.; `status.SnapshotAt = timestamppb.New(*item.ShippedSnapshotAt)` guarded by nil-check; `status.SnapshotCaptureFailed = item.ShippedSnapshotCaptureFailed`.
- Files: `server/services/backlog_service_ship_status.go`

##### Task 3.4.1c: Tests for the populated/unpopulated/corrupt-JSON snapshot cases (~5 min)
- Table test in `server/services/backlog_service_ship_status_test.go` (or new file) covering: fully populated snapshot; no snapshot (`ShippedSnapshotAt: nil`); corrupt `ShippedFileStats` JSON (asserts no RPC error, empty `FileStats`, other fields still populated, and a Warning log call via a test logger seam); `ShippedSnapshotCaptureFailed: true` passes through to `Status.SnapshotCaptureFailed`.
- Files: `server/services/backlog_service_ship_status_test.go`

---

## Phase 4: Frontend Renders Durable Snapshot Data (Depends on Phase 3)

### Epic 4.1: Extend `fromShipStatus` adapter with snapshot fields

#### Story 4.1.1: `fromShipStatus` reads the 6 new proto fields
**As a** user viewing a done backlog item, **I want** the GitHub PR/CI/review badges and per-file diff stats to render even though the worktree is gone, **so that** the requirement's core success metric is met.
**Acceptance Criteria**:
- `fromShipStatus(status)` now maps `status.shippedCheckConclusion`/`shippedApprovedCount`/`shippedChangesReqCount` into `GithubSummary` (non-null whenever `status.prUrl` is non-empty, even with all-zero review counts), `status.fileStats` into `FileChangeSummary[]`, `status.snapshotAt` into `VcsWidgetData.snapshotAt` (`kind` stays `"historical"`), and `status.snapshotCaptureFailed` into `VcsWidgetData.snapshotCaptureFailed`.
  - *Given* a `BacklogItemShipStatus` with `prUrl: "https://github.com/tstapler/stapler-squad/pull/42"`, `shippedCheckConclusion: "success"`, `shippedApprovedCount: 2`, `shippedChangesReqCount: 0`, `fileStats: [{path: "src/foo.ts", status: FILE_STATUS_MODIFIED, additions: 5, deletions: 2}]`, `snapshotAt: <2026-07-17T10:00:00Z>`, `snapshotCaptureFailed: false`, *When* `fromShipStatus(status)` is called, *Then* the result has `github: {checkConclusion: "success", approvedCount: 2, changesReqCount: 0, prUrl: "https://github.com/tstapler/stapler-squad/pull/42", ...}`, `fileChanges: [{path: "src/foo.ts", status: "modified", additions: 5, deletions: 2, section: "unstaged"}]`, `snapshotAt: <Date for 2026-07-17T10:00:00Z>`, `snapshotCaptureFailed: false`.
- When `status.snapshotCaptureFailed === true`, `fromShipStatus` sets `VcsWidgetData.snapshotCaptureFailed: true` and does **not** invent a fake `checkConclusion` value — whichever of `github`/`fileChanges` the backend actually captured (per Story 3.3.1's independent-group-failure semantics) still populates normally; only the failed group is genuinely absent/zero.
  - *Given* a `BacklogItemShipStatus` with `snapshotCaptureFailed: true`, `shippedApprovedCount: 2`, `shippedChangesReqCount: 0`, `shippedCheckConclusion: "success"` (group A succeeded), and `fileStats: []` (group B — file stats — failed), *When* `fromShipStatus(status)` is called, *Then* the result has `snapshotCaptureFailed: true`, `github: {approvedCount: 2, changesReqCount: 0, checkConclusion: "success", ...}` (still populated — group A's real success is not discarded), and `fileChanges: []`.
- When `status.snapshotAt` is unset (pre-feature item), `fromShipStatus` behaves exactly as it did in Phase 1 (`github: null`, `fileChanges: []`, `snapshotAt: null`, `snapshotCaptureFailed` left `undefined`) — no regression for items without a captured snapshot.
  - *Given* a `BacklogItemShipStatus` with all 6 new fields at zero-value, *When* `fromShipStatus(status)` is called, *Then* `github: null` and `snapshotAt: null`, identical to Phase 1's behavior.
**Files**: `web-app/src/lib/vcs/adapters.ts`, `web-app/src/lib/vcs/adapters.test.ts`

##### Task 4.1.1a: Add snapshot-field mapping to `fromShipStatus`, including `snapshotCaptureFailed` (~5 min)
- Guard on `status.snapshotAt` presence (proto `Timestamp` truthiness check) before populating `github`/`fileChanges`/`snapshotAt` — falls through to Phase 1's existing empty-state behavior otherwise. Independently map `status.snapshotCaptureFailed` (boolean, present regardless of whether `github`/`fileChanges` individually populated) into `VcsWidgetData.snapshotCaptureFailed`.
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 4.1.1b: Map `ShippedFileStat.status` (proto `FileStatus` enum) to `FileChangeSummary["status"]` + assign `section` (~3 min)
- Reuse the `mapFileStatus` helper from Task 1.1.2a; since `ShippedFileStat` has no staged/unstaged/conflict distinction (it's a flat historical list), default `section: "unstaged"` for all entries (documented as a known simplification — historical snapshots don't distinguish staged vs. unstaged, only "changed").
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 4.1.1c: Unit tests for the new snapshot-field mapping, including the capture-failure and partial-failure cases (~6 min)
- Tests: populated snapshot maps correctly; zero-value snapshot preserves Phase 1 behavior; `snapshotCaptureFailed: true` with `github` fully populated (group A succeeded, group B failed) maps to a non-null `github` and empty `fileChanges` with `snapshotCaptureFailed: true` — asserting `deriveMergeabilityState` returns `"snapshot_unavailable"` for this case (not `"ci_pending"` and not a crash), confirming Task 1.1.5c's precedence fix.
- Files: `web-app/src/lib/vcs/adapters.test.ts`

### Epic 4.2: "As of" / staleness UI polish

#### Story 4.2.1: Snapshot-fetch-failure copy
**As a** user viewing a done item whose snapshot capture failed, **I want** an honest "couldn't fetch PR status" message instead of a misleading blank/empty GitHub section, **so that** I don't mistake a capture failure for "no PR was ever opened."
**Acceptance Criteria**:
- When `data.kind === "historical" && data.snapshotCaptureFailed === true` (the dedicated boolean from Story 3.3.1/Task 1.1.5a — never a string-matched `checkConclusion` sentinel), `VcsWidgetGithubRow` renders a distinct message instead of (or alongside, if `github` is partially populated) the normal CI-conclusion badge: "Couldn't capture PR status at ship time" when `github` is `null` (both groups failed), or "Couldn't fully capture PR status at ship time" when `github` is non-null (group A succeeded, group B didn't, or vice versa — some data is real).
  - *Given* `VcsWidgetData` with `kind: "historical"`, `snapshotCaptureFailed: true`, `github: null`, *When* `<VcsWidgetGithubRow data={data} />` renders, *Then* the DOM contains the text "Couldn't capture PR status at ship time" and no CI badge.
  - *Given* `VcsWidgetData` with `kind: "historical"`, `snapshotCaptureFailed: true`, `github: {checkConclusion: "success", approvedCount: 2, ..., prUrl: "https://github.com/.../pull/42"}` (group A succeeded, group B — file stats — failed), *When* `<VcsWidgetGithubRow data={data} />` renders, *Then* the DOM contains the real PR link and "success" CI badge (group A's captured data is not hidden) **plus** the text "Couldn't fully capture PR status at ship time" as a supplementary note — the partial failure is visible without discarding the data that was actually captured.
**Files**: `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.tsx`, `VcsWidgetGithubRow.test.tsx`

##### Task 4.2.1a: Add the `data.kind === "historical" && data.snapshotCaptureFailed` branch to `VcsWidgetGithubRow` (~5 min)
- Accept `data: VcsWidgetData` (not just the `github` slice) so the component can read both `snapshotCaptureFailed` and `github` together; render the supplementary/replacement message per the two cases in this story's acceptance criteria.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.tsx`

##### Task 4.2.1b: Unit tests for both the full-failure and partial-failure branches (~4 min)
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.test.tsx`

---

## Phase 5: Feature Registry and E2E Tests

### Epic 5.1: Feature registry updates

#### Story 5.1.1: Register new/changed features per `.claude/rules/feature-registry.md`
**As a** maintainer, **I want** the per-feature registry files updated, **so that** `make registry-generate` shows no new coverage gaps.
**Acceptance Criteria**:
- `docs/registry/features/frontend/vcs-widget.json` exists with `id: "vcs-widget"`, `filePath: "web-app/src/components/shared/VcsWidget.tsx"`, `tested: true` (once Epic 5.2's e2e test lands), `testIds` populated.
  - *Given* `make registry-generate` has run after Epic 5.2 lands, *When* `docs/registry/coverage-gaps.json` is inspected, *Then* `vcs-widget` does not appear as an untested feature.
- `docs/registry/features/backend/backlog/get-item-ship-status.json` (existing file) has its `lastModified` updated and `testIds` extended with the new snapshot-population test names from Story 3.4.1c.
**Files**: `docs/registry/features/frontend/vcs-widget.json` (new), `docs/registry/features/backend/backlog/get-item-ship-status.json` (edit)

##### Task 5.1.1a: Create `docs/registry/features/frontend/vcs-widget.json` (~3 min)
- Files: `docs/registry/features/frontend/vcs-widget.json`

##### Task 5.1.1b: Update `docs/registry/features/backend/backlog/get-item-ship-status.json`'s `testIds`/`lastModified` (~2 min)
- Files: `docs/registry/features/backend/backlog/get-item-ship-status.json`

##### Task 5.1.1c: Run `make registry-generate` and verify no net-new coverage gap (~3 min)
- Files: (generated) `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json`

### Epic 5.2: Playwright e2e coverage

#### Story 5.2.1: `VcsWidgetPage` helper + durable-snapshot e2e spec
**As a** QA/reviewer, **I want** e2e coverage of the unified widget's durable-snapshot (done-item) path, **so that** the DB-backed, deterministic half of this feature has automated regression coverage (per pitfalls research §6, the live-GitHub-API half is explicitly lower-priority for e2e determinism reasons).
**Acceptance Criteria**:
- `tests/e2e/pages/VcsWidgetPage.ts` exposes locators/assertions for `data-testid="vcs-widget-loaded"`, the mergeability pill, file-list rows, commit-list rows — no CSS class selectors, per `.claude/rules/e2e-test-conventions.md`.
  - *Given* the test server running with a seeded `done` backlog item that has a captured snapshot (`ShippedSnapshotAt` non-nil, via a test fixture/seed), *When* `tests/e2e/vcs-widget.spec.ts` navigates to that item's detail page and calls `VcsWidgetPage.waitForLoaded()`, *Then* the test asserts on `page.getByTestId("vcs-widget-loaded")` becoming visible — never `page.waitForTimeout(...)`.
- The new spec starts with `// @feature vcs-widget, backlog:get-item-ship-status`.
**Files**: `tests/e2e/pages/VcsWidgetPage.ts` (new), `tests/e2e/vcs-widget.spec.ts` (new)

##### Task 5.2.1a: Implement `VcsWidgetPage` locator helper class (~5 min)
- Methods: `waitForLoaded()`, `getMergeabilityPillText()`, `getFileRow(path)`, `clickViewDiff()` — each using `getByTestId`/`getByRole`, mirroring `tests/e2e/pages/BacklogPage.ts`'s existing structural style.
- Files: `tests/e2e/pages/VcsWidgetPage.ts`

##### Task 5.2.1b: Write the durable-snapshot spec test (~5 min)
- One test: navigate to a seeded done item with a captured snapshot, assert `VcsWidget` renders the mergeability pill, file list (from `ShippedFileStats`), commit list, and the "as of" timestamp — using `expect(locator).toBeVisible()`/`toHaveText()`, never a fixed sleep.
- Files: `tests/e2e/vcs-widget.spec.ts`

##### Task 5.2.1c: Write the "no snapshot captured" spec test (~4 min)
- One test: navigate to a seeded done item with `ShippedSnapshotAt` unset (pre-feature item), assert the "No history captured for this item" copy renders instead of a blank/error widget.
- Files: `tests/e2e/vcs-widget.spec.ts`

##### Task 5.2.1d: Write the compact-mode (Unfinished item detail) spec test (~4 min)
- One test: navigate to the Unfinished-work dashboard, expand an item's detail card, assert `VcsWidget mode="compact"` renders the aggregate stat line and commit list but no per-file rows.
- Files: `tests/e2e/vcs-widget.spec.ts`

### Epic 5.3: Deferred/flagged scope (not built in this project)

**Note, not a story**: Per features research §4, `BacklogItemCard.tsx` (board card) and `SessionCard.tsx`/`SessionRow.tsx` (Sessions list) are candidate surfaces for a compact `VcsWidget` integration but were **not** in requirements.md's In Scope list. Per the requirements' own flag ("call out explicitly rather than silently added"), this plan does **not** include tasks for those two surfaces — they are noted here as a natural, low-risk follow-up (the compact mode and `GitHubBadge` integration point already exist after Phase 2/4) for a future backlog item, not silently dropped or silently added.
