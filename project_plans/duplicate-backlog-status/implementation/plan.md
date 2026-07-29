# Implementation Plan: duplicate-backlog-status

**Feature**: Add a first-class `duplicate` backlog status with a `duplicate_of_id` link, a `mark_duplicate` MCP tool, list exclusion, and a distinct UI badge/link across all status-aware surfaces.
**Date**: 2026-07-29
**Status**: Ready for implementation
**ADRs**: ADR-001-transition-options-parameter-threading, ADR-002-fold-chain-guard-into-existing-sentinel

**Repo VCS note**: this worktree (`stapler-squad`) is a plain git repository (confirmed via `git status` at session start), not `jj`/Jujutsu colocated mode used elsewhere in this user's ecosystem. Standard git branch/PR workflow applies throughout this plan — no jj-specific steps.

---

## Step 0.5 — Creative Pass: Alternatives Considered for the Data Model

Three distinct approaches were brainstormed for how a duplicate item points at its canonical counterpart, before committing to the research's recommendation:

**Approach A — Plain nullable string field (`duplicate_of_id`), app-layer validation only.**
Strength: matches the existing `archived_at`-style bare-optional-field convention exactly (`session/ent/schema/backlog_item.go`), zero new ent edge/migration complexity, and keeps `TransitionGuard` pure (no DB dependency), consistent with every other guard in the file.
Weakness: no DB-level referential integrity — a dangling `duplicate_of_id` is possible if application-layer checks are ever bypassed (mitigated by AC11's required graceful-degradation UI).

**Approach B — Ent self-referential edge (`edge.To("duplicate_of", BacklogItem.Type).Unique()`).**
Strength: DB-level referential integrity and declarative cascade-behavior options (e.g. `SetNull` on target deletion).
Weakness: no self-referential edge precedent exists anywhere in `session/ent/schema/*.go` today (confirmed by direct inspection of all schema files); introduces `WithDuplicateOf()` eager-load plumbing and edge-mutation API surface for a feature whose own Non-Goals section explicitly rules out reverse lookup and chain resolution — the edge's main value-adds are exactly the things this feature deliberately does not need.

**Approach C — Separate `DuplicateLink` join entity** (`id`, `duplicate_item_id`, `canonical_item_id`, `created_at`).
Strength: could naturally support reverse lookup (given canonical X, list duplicates) and a full link-history audit trail without overloading `BacklogStatusEvent`.
Weakness: introduces an entirely new entity, migration, repository CRUD surface, and proto messages for a feature whose acceptance criteria explicitly declare reverse lookup and chain resolution out of scope (Non-Goals) — premature generalization for a need that does not exist yet.

**Decision: Approach A.** It is the only option whose complexity matches the literal 13 ACs; B and C both buy capabilities (referential integrity, reverse lookup) that the Non-Goals section explicitly defers. Recorded in the Pattern Decisions table below with the same three-way comparison.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `BacklogStatus` | `string`-based enum type for a backlog item's lifecycle state (`idea`, `refining`, `ready`, `in_progress`, `review`, `done`, `archived`, and now `duplicate`) | Existing type in `session/backlog.go`; sum-type-in-a-string, accepted existing convention (see Pattern Decisions) |
| `BacklogStatusDuplicate` | New `BacklogStatus` constant `"duplicate"` — an item that has been identified as describing the same problem as another (canonical) item | `session/backlog.go` |
| `DuplicateOfID` | The Go struct field / ent column (`duplicate_of_id`) holding the UUID string of the canonical item a duplicate item points to | Plain optional string, not an ent edge — see Approach A above |
| `duplicate_of_id` | The proto field name (snake_case) and ent schema field name corresponding to `DuplicateOfID` | `proto/session/v1/backlog.proto` field 21 on `BacklogItem`, field 6 on `TransitionBacklogItemStatusRequest` |
| `Canonical item` | The item a duplicate's `DuplicateOfID` points to — the "kept" item that survives triage | Domain term only, no dedicated Go type |
| `TransitionOptions` | New Go struct carrying transition-scoped side-channel data (currently only `DuplicateOfID`) threaded as a 5th parameter through `Repository.TransitionBacklogItemStatus` / `Storage.TransitionBacklogItemStatus` / `EntRepository.TransitionBacklogItemStatus` | Parameter Object pattern — see Pattern Decisions |
| `BacklogItemTransitionInput` | Existing struct carrying all fields `TransitionGuard` needs, resolved by the caller before invoking the guard | Gains `ID`, `DuplicateOfID`, `DuplicateOfExists`, `DuplicateOfStatus` fields |
| `TransitionGuard` | Pure function `func(item BacklogItemTransitionInput, to BacklogStatus) error` validating business-rule gates for a transition (no DB access) | `session/backlog.go`; gains a new `case to == BacklogStatusDuplicate` branch |
| `CanTransitionBacklog` | Pure function checking structural transition validity against the `validTransitions` map | `session/backlog.go`; unchanged signature, new table entries |
| `ErrDuplicateOfRequired` | Sentinel error: `duplicate_of_id` was empty when transitioning to `duplicate` | New in `session/backlog.go` |
| `ErrDuplicateOfSelf` | Sentinel error: `duplicate_of_id == id` (self-reference) | New in `session/backlog.go` |
| `ErrDuplicateOfInvalidTarget` | Sentinel error: `duplicate_of_id` does not reference an existing item, OR references an item that is *itself* already `duplicate` status (single-hop chain-prevention folded into this sentinel — see ADR-002) | New in `session/backlog.go` |
| `DuplicateOfExists` | Bool field on `BacklogItemTransitionInput`, resolved by the caller via a prior `storage.GetBacklogItem(ctx, duplicateOfID)` lookup, mirroring how `OverallOutcome` is resolved via `GetMostRecentReviewVerdictForItem` today | Keeps `TransitionGuard` pure |
| `DuplicateOfStatus` | `BacklogStatus` field on `BacklogItemTransitionInput`, resolved from the same `GetBacklogItem(duplicateOfID)` lookup used for `DuplicateOfExists` — no extra query. Used only to check "target is not itself already duplicate" | New field, added specifically for the in-scope chain-prevention rule (Non-Goals section) |
| `mark_duplicate` | New MCP tool (`item_id`, `duplicate_of_id`, optional `note`) in `server/mcp/tools_backlog.go` that runs `CanTransitionBacklog` + `TransitionGuard` + the transition end-to-end for agent (triage/work) sessions | First MCP tool to mutate backlog status directly by calling the pure state-machine functions itself, rather than hardcoding one known-safe transition like `submitReviewVerdict` does |
| `BacklogItemPrecondition` | Existing struct (`ExpectedStatus`, `ExpectedUpdatedAt`) used for optimistic-concurrency checks on update/transition | `session/repository.go`; the atomic-write fix (FR4/AC7) moves its enforcement from an app-level pre-check into a `.Where()` clause on the same SQL statement as the write |
| `ErrPreconditionFailed` | Existing sentinel error returned when an optimistic-locking precondition fails | `session/repository.go`; now also returned when `.Where(backlogitem.StatusEQ(current.Status))` excludes a concurrently-modified row on `TransitionBacklogItemStatus` |
| `StatusEQ` | Generated ent predicate helper (`session/ent/backlogitem`) used in `.Where()` on the `UpdateOneID` builder to make the status-check-and-write atomic | Already used elsewhere for `StatusIn`/`StatusNotIn`; `StatusEQ` is the same family |
| `ExcludeTerminal` | Existing bool field on `BacklogItemFilter` gating the `StatusNotIn(done, archived)` exclusion in `ListBacklogItems` | Gains `duplicate` as a third excluded status |
| `STATUS_CLASS` / `STATUS_CSS` | Per-component `Record<string,string>` maps (one each in `BacklogItemBadge.tsx`, `BacklogItemDetail.tsx`, `page.tsx`) mapping a status string to a vanilla-extract class | No shared type — each of the 3 surfaces has its own independent map; this is the single highest-risk silent-failure point in the whole feature (see pitfalls research) |
| `vars.statusBadge.duplicateBg/Fg/Border` | New theme-contract token triplet (parallel to existing `approvalBg/Fg/Border` etc.) | `web-app/src/styles/theme-contract.css.ts`; populated with real hex values in all 6 `theme.css.ts` blocks |
| `KnownBacklogStatus` | Frontend string-literal union of known status values, with a `(string & {})` escape hatch via `BacklogItemStatus` | `web-app/src/lib/hooks/useBacklogService.ts`; gains `"duplicate"` |
| `duplicateOfId` | New optional field on the frontend `BacklogItem` interface, populated from the proto's `duplicate_of_id` | `web-app/src/lib/hooks/useBacklogService.ts` |
| `getActionSpec` | Existing `switch (item.status)` in `BacklogItemCard.tsx` mapping status → card action-button spec | Gains a `case "duplicate":` — otherwise falls through to `default` and renders the raw string `"duplicate"` as a button label |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `duplicate_of_id` storage | Plain optional string field on the aggregate (matches `archived_at`) | PoEAA (simple Active-Record-style field, not a Domain Model association) | (B) Ent self-referential edge; (C) separate `DuplicateLink` join entity | Both alternatives buy referential integrity / reverse-lookup capability the Non-Goals section explicitly defers; app-layer validation via `TransitionGuard` matches the file's existing all-guards-are-plain-fields philosophy |
| `BacklogStatus` | Sum-type-in-a-string with an exhaustive `validTransitions` map + `CanTransitionBacklog` gate function | type-driven-design (accepted, pre-existing pattern) | Proper Go sum type (sealed interface / `iota`-backed enum with a `String()` method) | This is a pre-existing convention across all 7 existing statuses, not a new violation introduced by this feature; ent stores it as a plain `string` column already. Converting the whole type is out of scope and would be a much larger, unrelated refactor |
| `TransitionOptions` | Parameter Object | GoF (creational-adjacent) / PoEAA | Adding `duplicateOfID string` as a bare 5th positional parameter on all 3 signatures | A struct leaves room for future transition-scoped data (e.g. a future `Reason string`) without another 3-signature + 6-call-site diff; also self-documents at call sites (`opts.DuplicateOfID` vs. an unnamed positional string) |
| `TransitionGuard` extension | Flat `switch` case addition (existing style) | PoEAA Transaction-Script-shaped guard function | Chain-of-Responsibility validator pipeline | The function handles 7 states today in ~40 lines; introducing a pluggable-validator abstraction for one more `case` is premature generalization with no second consumer in sight |
| `mark_duplicate` MCP handler validation | Direct call of the pure package-level `session.CanTransitionBacklog` + `session.TransitionGuard` functions from the handler | Simple Function (explicitly preferred over an unnecessary Adapter) | Thread `session.WorkflowEngine` into `backlogHandlers` as a constructor dependency (Dependency Inversion via interface) | `WorkflowEngine` exists specifically as RPC-layer DI/mocking machinery (`NewBacklogService` is its only construction site); `tools_backlog.go` already imports the `session` package directly and calls its pure helpers (e.g. `ParseAcCriteria`) without an interface layer — adding DI here for one tool is indirection with no test-isolation benefit, since the pure functions are already trivially unit-testable without a fake |
| Repository write-path signature (3 layers: `Repository` interface, `Storage`, `EntRepository`) | Extend the existing single `TransitionBacklogItemStatus` method with one additional parameter, update all 6 call sites to pass `nil`/a populated `*TransitionOptions` | PoEAA Repository (single write path preserved) | Fork a second near-duplicate method (e.g. `TransitionBacklogItemStatusWithDuplicate`) | A second method is exactly the kind of "illegal state of the system" type-driven-design warns about at the architecture level: two write paths for the same aggregate can silently drift in validation behavior between RPC and MCP callers. One method, one signature change, is the smaller and safer diff — see ADR-001 |
| Chain-prevention guard ("target must not itself be `duplicate`") | Folded into the existing `ErrDuplicateOfInvalidTarget` sentinel via a `DuplicateOfStatus` field, rather than adding a 4th sentinel | type-driven-design (illegal-state prevention via an additional guard clause, not a new type) | A dedicated 4th sentinel `ErrDuplicateOfChained` | AC6 explicitly enumerates "three new sentinel errors" as the acceptance bar; folding keeps that literal count exact while still implementing the in-scope Non-Goals chain rule. See ADR-002 for the full reasoning and caller-facing implications |
| `ListBacklogItems` exclusion | Extend existing `StatusNotIn(done, archived)` predicate with a third argument | PoEAA Repository query object | New dedicated filter flag (e.g. `filter.ExcludeDuplicates`) | `ExcludeTerminal` already exists and is the exact mechanism `archived`/`done` use; `duplicate` is semantically terminal-for-active-view purposes too — reusing the flag is the one-line change the requirements call for |
| Frontend status→class maps (`BacklogItemBadge`, `BacklogItemDetail`, `page.tsx`) | Leave as 3 independent maps (existing pattern), add `duplicate` to each; retype each as `Record<KnownBacklogStatus, string>` (see Epic 5.3) so a missing entry is a compile error | N/A — explicit non-pattern | Extract a single shared `STATUS_CLASS` map/component | Confirmed by direct inspection: `BacklogItemDetail` does not import `BacklogItemBadge` today and each has its own CSS Modules-adjacent (vanilla-extract) class exports; unifying them is a legitimate future refactor but is out of scope for a status addition — doing it here would silently expand this backlog item's blast radius beyond its 13 ACs |

**Note on the `mark_duplicate` MCP handler validation row above**: the "same validation, no drift" framing holds only because `DefaultWorkflowEngine` (`session/workflow_engine.go`) is currently a thin passthrough to the exact same package-level functions `mark_duplicate` calls directly (`session.CanTransitionBacklog`/`session.TransitionGuard`). If a future `WorkflowEngine` implementation ever added cross-cutting behavior (metrics, feature-gating, logging) to the guard invocation, the MCP path would not automatically inherit it — this is the same two-write-path drift risk ADR-001 argues against, just at the guard layer instead of the write layer. Accepted as low-risk today because `NewBacklogService` is the only construction site for a non-default engine (no such implementation exists yet); revisit if that ever changes.

---

## Migration Plan

- **Migration file**: none in the traditional SQL-migration sense — this project uses ent's schema-driven approach. The only "migration artifact" is the schema source edit at `session/ent/schema/backlog_item.go` (add `field.String("duplicate_of_id").Optional()`); ent's generated client (gitignored, regenerated via `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`) applies the new nullable column automatically on next schema-sync at server startup (ent's default `Schema.Create` auto-migration, already relied upon by every prior optional-field addition in this schema, e.g. `plan_artifacts_path`).
- **Reversibility**: fully reversible without a down-script — the new column is `Optional()` (nullable, no default required), so no backfill is needed and no existing row becomes invalid. Rolling back the code change leaves an unused nullable column behind (harmless; matches how this repo already handles additive schema changes with no down-migration tooling).
- **Zero-downtime strategy**: N/A at this project's scale (single-tenant, single-instance SQLite-backed dev tool per `build-vs-buy.md` framing) — no concurrent-write coordination is needed beyond the optimistic-concurrency fix already in scope (AC7).
- **Rollback procedure**: standard `git revert` of the PR. No data cleanup required since the column is additive/nullable and no other code path depends on its presence.

## Observability Plan

This is low-traffic internal tooling (backlog triage, not a customer-facing hot path) — keep observability proportionate, not exhaustive.

- **Logs**:
  - `mark_duplicate` MCP handler: one `log.InfoLog.Printf("[mcp:mark_duplicate] session=%s item=%s duplicate_of=%s", ...)` on success, matching the existing `[mcp:request_review]`/`[mcp:submit_triage_result]` convention in `server/mcp/tools_backlog.go`.
  - Best-effort note-append failure (FR5): `log.WarningLog.Printf("[mcp:mark_duplicate] note append failed for item %s: %v", itemID, err)` — non-fatal, matches the existing non-fatal audit-log-failure pattern in `EntRepository.TransitionBacklogItemStatus` (`_ = evErr`).
  - No new logging needed in `TransitionGuard`/`CanTransitionBacklog` — both are pure functions returning errors the caller already logs/surfaces.
- **Metrics**: none new. `TransitionBacklogItemStatus` is not currently instrumented with metrics for any other status, and duplicate-marking is a low-frequency, human/agent-judgment-driven action — adding a metric here would be the only status-transition metric in the codebase, an inconsistent precedent to set unilaterally.
- **Alerts**: no new alerts required. This is not an operationally critical path; a failed `mark_duplicate` call surfaces synchronously to the calling agent session via the MCP tool's error result.

## Risk Control

- **Feature flag**: not gated. The new status is purely additive (a new enum value + one nullable column); no existing transition, list query default, or UI surface changes behavior for items that are not `duplicate` status. Shipping directly on merge is consistent with how `archived_at` and other optional fields were added previously in this schema.
- **Rollback procedure**: standard revert via PR close + revert commit (plain git, not jj — see VCS note above). No feature-flag toggle to also flip.
- **Staged rollout**: full rollout on merge — single-tenant internal tool, no cohort/percentage rollout infrastructure exists or is warranted here.
- **Explicit out-of-scope carve-out** (not a risk to *this* change, but must not be silently conflated with it): `UpdateBacklogItem` (`session/ent_repository_backlog.go:183`) has the identical read-then-blind-write TOCTOU race that `TransitionBacklogItemStatus` has today. This plan fixes `TransitionBacklogItemStatus` only (AC7's literal scope). `UpdateBacklogItem`'s gap is **not** touched, **not** silently ignored — it is a known, pre-existing, accepted gap, flagged here as a candidate for a future separate backlog item. Do not "fix it while we're in the file."
- **Explicit out-of-scope carve-out — in-flight sessions on a mid-transition item**: FR1/AC2 allow `in_progress → duplicate` and `review → duplicate`, i.e. an item can be marked duplicate while a work or review `ItemSession` is actively running against it. This is an accepted, explicit scope limitation: `mark_duplicate`/the RPC transition do NOT stop or notify any active `ItemSession` (work/review) running against an item that gets marked duplicate mid-flight — unlike `TriggerTriage`'s existing precedent of explicitly ending and stopping superseded `ItemSession`s when re-triage supersedes them. A running session will only discover the status change the next time it calls `report_progress`/`request_review` (both of which will still succeed against a `duplicate`-status item today, since neither checks `item.Status` as a precondition). Flagged here as a candidate follow-up, not fixed in this plan — no code task is added for it.
- **Explicit out-of-scope carve-out — second-order TOCTOU on chain-prevention target status**: `DuplicateOfStatus` is resolved via a single `GetBacklogItem` read at guard-check time (RPC Task 2.2.4a / MCP Task 3.1.1b), and nothing locks or re-validates the *target* row's status inside the same write that commits the source item's transition — between that read and the write committing, a concurrent transition could mark the target itself `duplicate`, allowing a rare 2-hop chain to become briefly reachable despite ADR-002/the Non-Goals section's chain-prevention intent. This is a known, low-probability, accepted limitation, not fixed in this plan: a full fix would require locking/re-validating the target row at write time too (e.g. a second `.Where()` clause against the target's own row), disproportionate effort for this internal, low-traffic, human/agent-judgment-driven tool. No code task is added for it.

## Unresolved Questions

None. All ambiguities the requirements flagged for the planning subagent to resolve have been settled explicitly:
- Sentinel error count/semantics for the chain-prevention rule → resolved via ADR-002 (fold into `ErrDuplicateOfInvalidTarget`).
- `TransitionOptions` threading approach → resolved via ADR-001 (parameter object, single write path).
- Exact call-site count for the 3-layer signature change → resolved by direct inspection: **6 call sites** (5 in `server/services/backlog_service.go` at lines 661, 953, 1046, 1119, 1356; 1 in `server/mcp/tools_backlog.go` at line 375), plus the 3 signature declarations themselves (`session/repository.go:167`, `session/storage.go:528`, `session/ent_repository_backlog.go:274`) — see Story 2.2.2.
- Theme-token contrast ratios → computed and recorded inline per Story 5.1.2 (all 6 pairs pass WCAG AA 4.5:1; see that story for the actual ratios).

---

## Dependency Visualization

```
Phase 1: State Machine + Ent Schema
  Epic 1.1 (backlog.go)  ──────┐
  Epic 1.2 (ent schema)  ──────┤
                                ▼
Phase 2: Proto + Plumbing + Atomic Write
  Epic 2.1 (proto)  ───────────┤
  Epic 2.2 (TransitionOptions, atomic write, RPC wiring) ◄── depends on 1.1, 1.2, 2.1
                                │
                ┌───────────────┼───────────────┐
                ▼                               ▼
Phase 3: mark_duplicate MCP tool     Phase 4: List exclusion
  Epic 3.1 ◄── depends on 1.1, 2.2      Epic 4.1 ◄── depends on 1.1 only
                │                               │
                └───────────────┬───────────────┘
                                ▼
Phase 5: Frontend (independent of 3 & 4, depends only on 2.1's proto fields existing)
  Epic 5.1 (theme tokens) ──────┐
  Epic 5.2 (labels + types) ────┤
  Epic 5.3 (4 touchpoints) ◄────┴── depends on 5.1, 5.2
  Epic 5.4 (frontend tests) ◄──────── depends on 5.3
                                │
                                ▼
Phase 6: Tests (backend) ◄── depends on 1, 2, 3, 4
Phase 7: Docs + Backfill ◄── depends on 3 (mark_duplicate must exist to backfill with it)
```

Phases 3 and 4 can run in parallel once Phase 2 lands. Phase 5 (frontend) can start as soon as Phase 2's proto fields exist (it does not need the MCP tool or list-exclusion logic). Phase 7's backfill task is the only one that must strictly follow Phase 3.

---

## Phase 1: Backend State Machine + Ent Schema

### Epic 1.1: Backlog Status State Machine
**Goal**: `duplicate` is a structurally valid, guarded status with correct transition rules (AC1, AC2, AC3, AC6).

#### Story 1.1.1: Add `BacklogStatusDuplicate` and its transitions
**As a** triage agent, **I want** the state machine to recognize `duplicate` as a valid status with the correct fan-in/fan-out edges, **so that** `CanTransitionBacklog` and `TransitionGuard` enforce the right rules before any write happens.

**Acceptance Criteria**:
- AC1: `session/backlog.go` defines `BacklogStatusDuplicate BacklogStatus = "duplicate"`.
  - *Given* the `session` package is compiled, *When* code references `session.BacklogStatusDuplicate`, *Then* it resolves to the string value `"duplicate"`.
- AC2: `validTransitions` allows `idea|refining|ready|in_progress|review → duplicate` and `duplicate → idea`; `done → duplicate` is deliberately not added.
  - *Given* an item with `Status: session.BacklogStatusInProgress`, *When* `session.CanTransitionBacklog(session.BacklogStatusInProgress, session.BacklogStatusDuplicate)` is called, *Then* it returns `true`.
  - *Given* an item with `Status: session.BacklogStatusDone`, *When* `session.CanTransitionBacklog(session.BacklogStatusDone, session.BacklogStatusDuplicate)` is called, *Then* it returns `false`.
- AC3: `CanTransitionBacklog` returns the correct bool for all new/rejected edges, covered by unit tests (see Story 1.1.2).
- AC6 (guard portion only — see Story 1.2.2 for the field additions this depends on): `TransitionGuard` rejects a transition to `duplicate` when `duplicate_of_id` is empty, self-referencing, or references a nonexistent (or already-`duplicate`-status) item, and accepts a valid non-self, existing, non-duplicate-status target.
  - *Given* `BacklogItemTransitionInput{ID: "10128af0-e1eb-47bc-9016-3af8fde83b4d", DuplicateOfID: "", Status: session.BacklogStatusIdea}`, *When* `TransitionGuard(input, session.BacklogStatusDuplicate)` is called, *Then* it returns `session.ErrDuplicateOfRequired`.
  - *Given* `BacklogItemTransitionInput{ID: "10128af0-e1eb-47bc-9016-3af8fde83b4d", DuplicateOfID: "10128af0-e1eb-47bc-9016-3af8fde83b4d", Status: session.BacklogStatusIdea}` (self-reference), *When* `TransitionGuard(input, session.BacklogStatusDuplicate)` is called, *Then* it returns `session.ErrDuplicateOfSelf`.
  - *Given* `BacklogItemTransitionInput{ID: "67de6c7b-....", DuplicateOfID: "1dc7ff10-326c-4276-a70f-eb8869713593", DuplicateOfExists: false, Status: session.BacklogStatusIdea}`, *When* `TransitionGuard(input, session.BacklogStatusDuplicate)` is called, *Then* it returns `session.ErrDuplicateOfInvalidTarget`.
  - *Given* `BacklogItemTransitionInput{ID: "67de6c7b-....", DuplicateOfID: "1dc7ff10-326c-4276-a70f-eb8869713593", DuplicateOfExists: true, DuplicateOfStatus: session.BacklogStatusDuplicate, Status: session.BacklogStatusIdea}` (target is itself already a duplicate — chain-prevention), *When* `TransitionGuard(input, session.BacklogStatusDuplicate)` is called, *Then* it returns `session.ErrDuplicateOfInvalidTarget` (folded per ADR-002).
  - *Given* `BacklogItemTransitionInput{ID: "67de6c7b-....", DuplicateOfID: "10128af0-e1eb-47bc-9016-3af8fde83b4d", DuplicateOfExists: true, DuplicateOfStatus: session.BacklogStatusIdea, Status: session.BacklogStatusReady}`, *When* `TransitionGuard(input, session.BacklogStatusDuplicate)` is called, *Then* it returns `nil`.

**Files**: `session/backlog.go`

##### Task 1.1.1a: Add `BacklogStatusDuplicate` const (~2 min)
- Add `BacklogStatusDuplicate BacklogStatus = "duplicate"` to the const block at `session/backlog.go:11-19`.
- Files: `session/backlog.go`

##### Task 1.1.1b: Add `validTransitions` entries (~3 min)
- In `validTransitions` (`session/backlog.go:121-151`): add `BacklogStatusDuplicate: true` to the target maps for `BacklogStatusIdea`, `BacklogStatusRefining`, `BacklogStatusReady`, `BacklogStatusInProgress`, `BacklogStatusReview`.
- Add a new top-level entry `BacklogStatusDuplicate: {BacklogStatusIdea: true}` (reopen-only, mirrors `BacklogStatusArchived`'s `{BacklogStatusIdea: true}`).
- Do NOT add `BacklogStatusDuplicate: true` to `BacklogStatusDone`'s target map (explicitly excluded per AC2).
- Do NOT add anything to `BacklogStatusArchived`'s target map (archived → duplicate is not in the proposed set).
- Files: `session/backlog.go`

##### Task 1.1.1c: Add `ID`/`DuplicateOfID`/`DuplicateOfExists`/`DuplicateOfStatus` fields to `BacklogItemTransitionInput` (~2 min)
- Add to the struct at `session/backlog.go:171-179`:
  ```go
  ID                string       // the item's own id — needed for the self-reference check
  DuplicateOfID     string       // resolved from the transition request
  DuplicateOfExists bool         // resolved by caller via prior GetBacklogItem lookup
  DuplicateOfStatus BacklogStatus // resolved from the same lookup; used for chain-prevention
  ```
- Files: `session/backlog.go`

##### Task 1.1.1d: Add three sentinel errors (~2 min)
- Add to the sentinel error var block at `session/backlog.go:163-168`:
  ```go
  ErrDuplicateOfRequired = errors.New("duplicate_of_id is required when marking an item duplicate")
  ErrDuplicateOfSelf     = errors.New("duplicate_of_id cannot reference the item itself")
  ErrDuplicateOfInvalidTarget = errors.New("duplicate_of_id does not reference a valid (existing, non-duplicate) backlog item")
  ```
- Files: `session/backlog.go`

##### Task 1.1.1e: Add the guard case to `TransitionGuard` (~3 min)
- Add a new case to the `switch` in `TransitionGuard` (`session/backlog.go:188-224`), before the `default` case:
  ```go
  case to == BacklogStatusDuplicate:
      if item.DuplicateOfID == "" {
          return ErrDuplicateOfRequired
      }
      if item.DuplicateOfID == item.ID {
          return ErrDuplicateOfSelf
      }
      if !item.DuplicateOfExists || item.DuplicateOfStatus == BacklogStatusDuplicate {
          return ErrDuplicateOfInvalidTarget
      }
      return nil
  ```
- Files: `session/backlog.go`

#### Story 1.1.2: Unit tests for the state machine
**As a** developer, **I want** every new/rejected transition edge and every `TransitionGuard` branch covered, **so that** regressions in the state machine are caught before they reach the RPC/MCP layers.

**Acceptance Criteria**:
- AC3 (tests): every new `CanTransitionBacklog` edge (5 new allowed `→duplicate` edges, the `duplicate→idea` reopen edge) and every previously-invalid edge that must stay `false` (`done→duplicate`, `archived→duplicate`, `duplicate→` anything except `idea`) is asserted.
  - *Given* the table-driven test in `TestCanTransition_AllValidPaths`, *When* the test runs, *Then* it includes a row `{from: BacklogStatusReview, to: BacklogStatusDuplicate, want: true}`.
  - *Given* the table-driven test in `TestCanTransition_AllInvalidPaths`, *When* the test runs, *Then* it includes a row `{from: BacklogStatusDone, to: BacklogStatusDuplicate, want: false}` and `{from: BacklogStatusArchived, to: BacklogStatusDuplicate, want: false}`.
- AC6 (tests): `TransitionGuard`'s four new branches (empty, self-ref, nonexistent, chained-duplicate-target, valid) each have a dedicated test function following the `TestTransitionGuard_<From>To<To>_<Rule>` naming convention already used in this file.
- AC1 (tests): `BacklogStatusDuplicate` resolves to the exact string `"duplicate"`, asserted standalone (not just implicitly via every other test in this file that happens to reference it).

**Files**: `session/backlog_test.go`

##### Task 1.1.2a: Extend `TestCanTransition_AllValidPaths` / `TestCanTransition_AllInvalidPaths` (~4 min)
- Add the 6 new valid-edge rows (5 `→duplicate` + `duplicate→idea`) to `TestCanTransition_AllValidPaths` (`session/backlog_test.go:9-34`).
- Add the invalid-edge rows (`done→duplicate`, `archived→duplicate`) to `TestCanTransition_AllInvalidPaths` (`session/backlog_test.go:35-54`).
- Files: `session/backlog_test.go`

##### Task 1.1.2b: Add `TestTransitionGuard_AnyToDuplicate_RequiresDuplicateOfID` (~3 min)
- New test asserting `TransitionGuard(BacklogItemTransitionInput{ID: "a", DuplicateOfID: ""}, BacklogStatusDuplicate)` returns `ErrDuplicateOfRequired` via `errors.Is`.
- Files: `session/backlog_test.go`

##### Task 1.1.2c: Add `TestTransitionGuard_AnyToDuplicate_RejectsSelfReference` (~3 min)
- New test asserting `TransitionGuard(BacklogItemTransitionInput{ID: "a", DuplicateOfID: "a"}, BacklogStatusDuplicate)` returns `ErrDuplicateOfSelf`.
- Files: `session/backlog_test.go`

##### Task 1.1.2d: Add `TestTransitionGuard_AnyToDuplicate_RejectsNonexistentTarget` and `..._RejectsChainedDuplicateTarget` (~4 min)
- Two new tests: one with `DuplicateOfExists: false`, one with `DuplicateOfExists: true, DuplicateOfStatus: BacklogStatusDuplicate` — both assert `ErrDuplicateOfInvalidTarget`.
- Files: `session/backlog_test.go`

##### Task 1.1.2e: Add `TestTransitionGuard_AnyToDuplicate_AcceptsValidTarget` (~2 min)
- New test with `DuplicateOfID: "b", DuplicateOfExists: true, DuplicateOfStatus: BacklogStatusIdea` asserting `nil` error.
- Files: `session/backlog_test.go`

##### Task 1.1.2f: Add `TestBacklogStatusDuplicate_HasExpectedStringValue` (~1 min)
- New standalone test asserting `string(session.BacklogStatusDuplicate) == "duplicate"` — closes the validation-mapping gap where AC1 was previously exercised only implicitly by every other test in this file that happens to reference the constant, with no test asserting the literal string value on its own.
- Files: `session/backlog_test.go`

---

### Epic 1.2: Ent Schema — `duplicate_of_id` field
**Goal**: `BacklogItem` has a persisted, nullable `duplicate_of_id` column round-tripping through the domain model (AC4).

#### Story 1.2.1: Add and regenerate the schema field
**As a** developer, **I want** `duplicate_of_id` on the `BacklogItem` ent schema, **so that** it can be persisted and read like every other optional field on this entity.

**Acceptance Criteria**:
- AC4: `BacklogItem` ent schema gets an `Optional()` `duplicate_of_id` string field, regenerated via `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`; generated output stays gitignored/uncommitted, only `schema/backlog_item.go` is committed.
  - *Given* `session/ent/schema/backlog_item.go` has been edited and the ent generate command above has been run, *When* `go build ./...` runs, *Then* it compiles cleanly and `ent.BacklogItem` has a `DuplicateOfID string` field and `SetDuplicateOfID(string)` / `SetNillableDuplicateOfID(*string)` builder methods.
  - *Given* `git status` after running the generate command, *When* inspecting changed files, *Then* only `session/ent/schema/backlog_item.go` shows as a tracked-file diff (everything under `session/ent/*.go` excluding `schema/` remains gitignored/untracked).

**Files**: `session/ent/schema/backlog_item.go`, `session/repository.go`, `session/ent_repository_backlog.go`

##### Task 1.2.1a: Add the field to the ent schema (~2 min)
- In `session/ent/schema/backlog_item.go`'s `Fields()` (currently lines 20-70), add after `external_id` (line 55-56):
  ```go
  field.String("duplicate_of_id").
      Optional(),
  ```
- Files: `session/ent/schema/backlog_item.go`

##### Task 1.2.1b: Regenerate the ent client (~3 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` from the repo root (matches `session/ent/generate.go`'s `go generate` directive exactly).
- Run `go build ./...` to confirm the generated code compiles (it will fail loudly with "undefined" errors if the `--feature sql/upsert` flag was omitted — this is a compile-time gate, not a silent risk, per pitfalls research).
- Files: none committed by this task (generated `session/ent/*.go` stays gitignored)

##### Task 1.2.1c: Add `DuplicateOfID` to `BacklogItemData` and thread it through the converter (~3 min)
- Add `DuplicateOfID string` field to `BacklogItemData` struct (`session/repository.go:244-269`).
- Add `DuplicateOfID: item.DuplicateOfID,` to `backlogItemToData` (`session/ent_repository_backlog.go:18-47`).
- Files: `session/repository.go`, `session/ent_repository_backlog.go`
- **Test coverage note**: no dedicated standalone test is added for this converter field in this story. Task 2.2.5a's `GetBacklogItem` re-fetch (which goes through this exact `backlogItemToData` conversion) already exercises the round-trip end-to-end — set `DuplicateOfID` via the ent builder, read it back through this converter, assert it survives. A separate `TestBacklogItemData_DuplicateOfIDRoundTripsThroughConverter` would duplicate that coverage with no additional risk closed; judged genuinely redundant rather than a gap.

---

**Note on scope**: `CreateBacklogItem` deliberately does NOT thread `duplicate_of_id` through its builder chain. No AC or FR requires setting `duplicate_of_id` at creation time — the only supported workflow is create → (later) `mark_duplicate`/RPC transition, both of which go through `TransitionGuard`. Adding it to the create path would open an unguarded write path (a caller could set `duplicate_of_id` to a nonexistent id, a self-reference, or an already-`duplicate` target while `status` is `idea`, an illegal/inconsistent state none of this plan's guard logic ever checks at create time). If a future AC needs create-time duplicate marking, add the write path — and its guard coverage — then.

---

## Phase 2: Proto + Request Plumbing + Atomic Write

### Epic 2.1: Proto Messages
**Goal**: `duplicate_of_id` is exposed over the wire on both the read model and the transition request (AC5).

#### Story 2.1.1: Add proto fields and regenerate bindings
**As a** frontend/MCP caller, **I want** `duplicate_of_id` available on `BacklogItem` and settable on `TransitionBacklogItemStatusRequest`, **so that** the field round-trips end-to-end.

**Acceptance Criteria**:
- AC5: `proto/session/v1/backlog.proto`'s `BacklogItem` and `TransitionBacklogItemStatusRequest` carry a `duplicate_of_id` field, regenerated via `make proto-gen` (not `make generate-proto`); generated bindings stay gitignored/uncommitted.
  - *Given* `proto/session/v1/backlog.proto` has been edited and `make proto-gen` has run, *When* `web-app/src/gen/session/v1/backlog_pb.ts` is inspected, *Then* it exposes `duplicateOfId: string` on the generated `BacklogItem` TS type and on `TransitionBacklogItemStatusRequest`.
  - *Given* `git status` after `make proto-gen`, *When* inspecting changed files, *Then* only `proto/session/v1/backlog.proto` shows as a tracked diff (`gen/proto/go/...` and `web-app/src/gen/...` remain gitignored).

**Files**: `proto/session/v1/backlog.proto`

##### Task 2.1.1a: Add `duplicate_of_id` to `BacklogItem` (~1 min)
- Add `string duplicate_of_id = 21;` after `repeated BacklogStatusEvent status_events = 20;` in the `BacklogItem` message (`proto/session/v1/backlog.proto:84-105`).
- Files: `proto/session/v1/backlog.proto`

##### Task 2.1.1b: Add `duplicate_of_id` to `TransitionBacklogItemStatusRequest` (~1 min)
- Add `string duplicate_of_id = 6;` after `string override_reason = 5;` in `TransitionBacklogItemStatusRequest` (`proto/session/v1/backlog.proto:196-202`).
- Files: `proto/session/v1/backlog.proto`

##### Task 2.1.1c: Regenerate bindings (~2 min)
- Run `make proto-gen` from repo root.
- Run `go build ./...` and `cd web-app && npx tsc --noEmit` to confirm both generated outputs compile.
- Files: none committed by this task (generated dirs stay gitignored)

---

### Epic 2.2: `TransitionOptions` + Atomic Write
**Goal**: `duplicate_of_id` is threaded through all 3 layers of the write path and persisted atomically with the status change, closing the existing TOCTOU race (AC7).

#### Story 2.2.1: Define `TransitionOptions`
**As a** developer, **I want** a small parameter object for transition-scoped side data, **so that** the write path stays a single method instead of forking.

**Acceptance Criteria**:
- Supporting AC7: `TransitionOptions{DuplicateOfID string}` exists and is the vehicle for passing `duplicate_of_id` into the write layer.
  - *Given* `session.TransitionOptions{DuplicateOfID: "10128af0-e1eb-47bc-9016-3af8fde83b4d"}`, *When* passed as the 5th argument to `EntRepository.TransitionBacklogItemStatus`, *Then* the resulting row's `duplicate_of_id` column equals `"10128af0-e1eb-47bc-9016-3af8fde83b4d"`.

**Files**: `session/repository.go`

##### Task 2.2.1a: Add `TransitionOptions` struct (~1 min)
- Add near `BacklogItemPrecondition` (`session/repository.go:302-308`):
  ```go
  // TransitionOptions carries transition-scoped side-channel data that is not
  // part of the core (from, to, precondition) transition signature. Currently
  // only used to pass duplicate_of_id when transitioning to BacklogStatusDuplicate.
  type TransitionOptions struct {
      DuplicateOfID string
  }
  ```
- Files: `session/repository.go`

#### Story 2.2.2: Thread `opts *TransitionOptions` through all 3 layers and 6 call sites
**As a** developer, **I want** one write path (not two) for backlog status transitions, **so that** RPC and MCP entry points cannot drift in validation/persistence behavior.

**Acceptance Criteria**:
- Supporting AC7: signature change is mechanical and every existing call site compiles by passing `nil`.
  - *Given* the updated `Repository` interface signature `TransitionBacklogItemStatus(ctx, id, toStatus, precondition, opts *TransitionOptions)`, *When* `go build ./...` runs after updating all 6 call sites, *Then* it compiles with zero errors.

**Files**: `session/repository.go`, `session/storage.go`, `session/ent_repository_backlog.go`, `server/services/backlog_service.go`, `server/mcp/tools_backlog.go`

##### Task 2.2.2a: Update the 3 signature declarations (~3 min)
- `session/repository.go:167`: `TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, opts *TransitionOptions) (*BacklogItemData, error)`.
- `session/storage.go:528-529`: same signature; body becomes `return s.repo.TransitionBacklogItemStatus(ctx, id, toStatus, precondition, opts)`.
- `session/ent_repository_backlog.go:274`: same signature (body updated in Story 2.2.3).
- Files: `session/repository.go`, `session/storage.go`, `session/ent_repository_backlog.go`

##### Task 2.2.2b: Update the 5 call sites in `server/services/backlog_service.go` (~4 min)
- Line 661 (main `TransitionBacklogItemStatus` RPC handler): pass a real `opts` (built in Story 2.2.4), not `nil`.
- Line 953 (`SpawnSessionFromItem`), line 1046 (`AttachSessionToItem`), line 1119 (`TriggerTriage`), line 1356 (`OverrideVerdict`): all four transition to non-`duplicate` statuses — update each call to pass `nil` as the 5th argument.
- Files: `server/services/backlog_service.go`

##### Task 2.2.2c: Update the 1 call site in `server/mcp/tools_backlog.go` (~2 min)
- Line 375 (`submitReviewVerdict`, transitions `review→done`): pass `nil` as the 5th argument.
- Files: `server/mcp/tools_backlog.go`

#### Story 2.2.3: Atomic write with optimistic concurrency
**As a** system, **I want** the status/`duplicate_of_id` write and its concurrency check to happen in one SQL statement, **so that** two concurrent transitions cannot silently race.

**Acceptance Criteria**:
- AC7: `TransitionBacklogItemStatus` writes `status` and `duplicate_of_id` atomically in one update, guarded by an optimistic-concurrency `StatusEQ(current.Status)` precondition that returns `ErrPreconditionFailed` on a stale write.
  - *Given* a backlog item `10128af0-e1eb-47bc-9016-3af8fde83b4d` currently at status `idea`, *When* two goroutines concurrently call `EntRepository.TransitionBacklogItemStatus(ctx, id, BacklogStatusReady, nil, nil)` and `EntRepository.TransitionBacklogItemStatus(ctx, id, BacklogStatusDuplicate, nil, &TransitionOptions{DuplicateOfID: "1dc7ff10-326c-4276-a70f-eb8869713593"})` in either order, *Then* exactly one succeeds and the other returns an error wrapping `session.ErrPreconditionFailed`.
  - *Given* the same item, *When* `TransitionBacklogItemStatus(ctx, id, BacklogStatusDuplicate, nil, &TransitionOptions{DuplicateOfID: "1dc7ff10-326c-4276-a70f-eb8869713593"})` succeeds, *Then* a single re-`Get` of the item shows both `Status == "duplicate"` and `DuplicateOfID == "1dc7ff10-326c-4276-a70f-eb8869713593"` — i.e. one write, not two round trips.
  - *Given* a backlog item currently at status `duplicate` with a non-empty `duplicate_of_id`, *When* `TransitionBacklogItemStatus(ctx, id, BacklogStatusIdea, nil, nil)` (reopen) is called, *Then* the item's `duplicate_of_id` is cleared to empty on the same write, alongside `Status` becoming `"idea"` — this is a defensive data-hygiene fix that goes a little beyond AC2/AC7's literal wording (see Cross-Cutting Notes), not new scope: it prevents the illegal-in-spirit state `(Status: idea, DuplicateOfID: <non-empty>)` from being silently reachable after a reopen.
  - *Given* a backlog item currently at status `idea`, *When* `TransitionBacklogItemStatus(ctx, id, BacklogStatusReady, nil, &TransitionOptions{DuplicateOfID: "10128af0-e1eb-47bc-9016-3af8fde83b4d"})` is called (a non-`duplicate` target status with a populated `DuplicateOfID`), *Then* the write succeeds (status becomes `ready`) but `duplicate_of_id` is NOT persisted — a follow-up `Get` shows `DuplicateOfID == ""` — proving the `toStatus == BacklogStatusDuplicate` guard on `SetDuplicateOfID` (Task 2.2.3a) is the authoritative check regardless of what a caller passes in `opts`.

**Files**: `session/ent_repository_backlog.go`

##### Task 2.2.3a: Rewrite the update builder with `.Where()` + conditional `SetDuplicateOfID` (~4 min)
- Replace the unconditional update at `session/ent_repository_backlog.go:297-307` with:
  ```go
  builder := r.client.BacklogItem.UpdateOneID(parsedID).
      SetStatus(string(toStatus)).
      SetUserModifiedStatusAt(now).
      Where(backlogitem.StatusEQ(current.Status))

  if precondition != nil && precondition.ExpectedUpdatedAt != nil {
      builder = builder.Where(backlogitem.UpdatedAtEQ(*precondition.ExpectedUpdatedAt))
  }
  // toStatus == BacklogStatusDuplicate is the AUTHORITATIVE guard here, not a
  // convenience check: without it, a caller sending DuplicateOfId alongside a
  // non-duplicate TargetStatus (e.g. TargetStatus: "ready") would silently
  // persist duplicate_of_id on a non-duplicate item, completely bypassing every
  // TransitionGuard check (empty/self-ref/chain) that only fires for
  // to == BacklogStatusDuplicate. This check must live at the write layer
  // (not only at the RPC caller, see Task 2.2.4c) so it's correct regardless
  // of which caller builds opts.
  if opts != nil && opts.DuplicateOfID != "" && toStatus == BacklogStatusDuplicate {
      builder = builder.SetDuplicateOfID(opts.DuplicateOfID)
  }

  item, err := builder.Save(ctx)
  if err != nil {
      if ent.IsNotFound(err) {
          // Get() above already confirmed the row exists; NotFound here can only
          // mean the StatusEQ (or UpdatedAtEQ) predicate excluded a concurrently-
          // modified row — translate to the precondition-failure sentinel, not "not found".
          return nil, fmt.Errorf("%w: status changed concurrently for item %s", ErrPreconditionFailed, id)
      }
      return nil, fmt.Errorf("failed to transition backlog item %s status: %w", id, err)
  }
  ```
- Add the new `opts *TransitionOptions` parameter to the function signature (`session/ent_repository_backlog.go:274`).
- Keep the existing app-level precondition pre-check block (`session/ent_repository_backlog.go:288-295`) as a fast-fail belt-and-suspenders — not required, but harmless and cheaper to leave than to prove removing it is safe.
- Files: `session/ent_repository_backlog.go`

##### Task 2.2.3b: Confirm import of the `backlogitem` predicate package (~1 min)
- `session/ent_repository_backlog.go` already imports `github.com/tstapler/stapler-squad/session/ent/backlogitem` (line 11) — verify `backlogitem.StatusEQ` and `backlogitem.UpdatedAtEQ` exist in the regenerated client (they will, as generated predicate helpers for every field).
- Files: `session/ent_repository_backlog.go`

##### Task 2.2.3c: Clear stale `duplicate_of_id` on reopen (`duplicate → idea`) (~3 min)
- Without this, reopening a duplicate item (any transition where `toStatus != BacklogStatusDuplicate` and the item's current status IS `BacklogStatusDuplicate`) leaves the row's `duplicate_of_id` column pointing at the old canonical item even though the item is no longer a duplicate — a silently-reachable illegal-in-spirit combination `(Status: idea, DuplicateOfID: <non-empty>)`.
- Extend the same builder chain introduced in Task 2.2.3a (added before `.Save(ctx)`, so it lands atomically with the status write):
  ```go
  if toStatus != BacklogStatusDuplicate && current.Status == string(BacklogStatusDuplicate) {
      builder = builder.ClearDuplicateOfID()
  }
  ```
- **Implementer note**: this assumes the generated ent client exposes `ClearDuplicateOfID()` — the standard generated method name for clearing an `Optional()` string field (mirrors, e.g., `ClearExternalID()` for `external_id`). Verify against the actual regenerated client after Task 1.2.1b runs; if the generated code exposes a differently-named method for this purpose, use `SetDuplicateOfID("")` as the fallback instead.
- Scoped ONLY to the reopen case (`duplicate → idea`) — do NOT clear `duplicate_of_id` on every non-`duplicate` transition. Only an item whose *current* status is `duplicate` can have a meaningful value to clear; clearing unconditionally on every transition would be either a no-op (field already empty) or, worse, silently incorrect if this logic is ever copy-pasted somewhere `current.Status` isn't actually being checked.
- Files: `session/ent_repository_backlog.go`

#### Story 2.2.4: RPC handler wiring
**As a** frontend caller, **I want** the `TransitionBacklogItemStatus` RPC to resolve the duplicate-target lookup and pass it through the guard and the write, **so that** UI-driven duplicate marking (if ever added) and the proto round-trip both work correctly.

**Acceptance Criteria**:
- Supporting AC6 + AC7: the RPC handler resolves `DuplicateOfExists`/`DuplicateOfStatus` via a prior `storage.GetBacklogItem` lookup and builds `opts` only when `req.Msg.DuplicateOfId != ""`.
  - *Given* a `TransitionBacklogItemStatusRequest{ItemId: "67de6c7b-...", TargetStatus: "duplicate", DuplicateOfId: "10128af0-e1eb-47bc-9016-3af8fde83b4d"}` where `10128af0-...` exists with status `idea`, *When* `BacklogService.TransitionBacklogItemStatus` is called, *Then* the response's `Item.DuplicateOfId` equals `"10128af0-e1eb-47bc-9016-3af8fde83b4d"` and `Item.Status` equals `"duplicate"`.

**Files**: `server/services/backlog_service.go`

##### Task 2.2.4a: Resolve `DuplicateOfExists`/`DuplicateOfStatus` and extend `guardInput` (~5 min)
- In `TransitionBacklogItemStatus` (`server/services/backlog_service.go:597-672`), after the existing `overallOutcome` lookup (line 624-628), add:
  ```go
  var duplicateOfExists bool
  var duplicateOfStatus session.BacklogStatus
  if req.Msg.DuplicateOfId != "" {
      target, targetErr := s.storage.GetBacklogItem(ctx, req.Msg.DuplicateOfId)
      switch {
      case targetErr == nil:
          duplicateOfExists = true
          duplicateOfStatus = session.BacklogStatus(target.Status)
      case errors.Is(targetErr, session.ErrNotFound):
          // Genuinely missing: leave duplicateOfExists false so TransitionGuard
          // correctly rejects with ErrDuplicateOfInvalidTarget.
      default:
          // Any OTHER error (DB timeout, connection failure, etc.) is an infra
          // failure, not "target doesn't exist" — do not fold it into the guard
          // rejection path, which would surface as CodeFailedPrecondition and
          // mask a real outage as "your duplicate_of_id is bad." Matches the
          // first GetBacklogItem lookup's error handling ten lines above, and
          // mark_duplicate's own handling in Task 3.1.1b.
          return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("look up duplicate_of_id: %w", targetErr))
      }
  }
  ```
- Extend `guardInput` (line 631-639) with `ID: req.Msg.ItemId, DuplicateOfID: req.Msg.DuplicateOfId, DuplicateOfExists: duplicateOfExists, DuplicateOfStatus: duplicateOfStatus`.
- Files: `server/services/backlog_service.go`

##### Task 2.2.4b: Add `ErrDuplicateOfRequired`/`ErrDuplicateOfSelf`/`ErrDuplicateOfInvalidTarget` to the guard-error classification (~2 min)
- Extend the `errors.Is` chain at `server/services/backlog_service.go:641-644` to also check the three new sentinels, mapping them to `connect.CodeFailedPrecondition` alongside the existing ones (same treatment as `ErrACRequired` etc., since they are all "business rule blocked this transition" cases, not malformed-request cases).
- Files: `server/services/backlog_service.go`

##### Task 2.2.4c: Build `opts` and pass to `storage.TransitionBacklogItemStatus` (~2 min)
- Before line 661, add:
  ```go
  var opts *session.TransitionOptions
  if to == session.BacklogStatusDuplicate && req.Msg.DuplicateOfId != "" {
      opts = &session.TransitionOptions{DuplicateOfID: req.Msg.DuplicateOfId}
  }
  ```
  Note: `to == session.BacklogStatusDuplicate` is added here as a secondary, belt-and-suspenders check — the authoritative guard against writing `duplicate_of_id` on a non-`duplicate` transition lives at the write layer (Task 2.2.3a's `toStatus == BacklogStatusDuplicate` condition), which is correct regardless of what any caller (this RPC handler, `mark_duplicate`, or a future caller) passes in `opts`. Tightening the construction here too is cheap and keeps the RPC handler from even attempting to build a semantically-invalid `opts` value, but do not treat this line as the only thing preventing the bug — the write-layer check is non-negotiable.
- Update the call at line 661 to `s.storage.TransitionBacklogItemStatus(ctx, req.Msg.ItemId, to, precondition, opts)`.
- Files: `server/services/backlog_service.go`

##### Task 2.2.4d: Map `DuplicateOfId` in `backlogItemToProto` (~1 min)
- Add `DuplicateOfId: item.DuplicateOfID,` to the struct literal in `backlogItemToProto` (`server/services/backlog_service.go:251-268`).
- Files: `server/services/backlog_service.go`

#### Story 2.2.5: Backend tests for the write path
**As a** developer, **I want** the atomic write, optimistic-concurrency behavior, and request plumbing covered, **so that** the TOCTOU fix and signature change don't regress silently.

**Acceptance Criteria**:
- AC7 (tests): a stale-write test proves `ErrPreconditionFailed` is returned, and a happy-path test proves `status` + `duplicate_of_id` land in one update.
- AC12 (partial): `backlog_service_test.go` continues to pass with the new 5th parameter threaded through every existing test call site.
- AC2/AC7 (data-hygiene, supporting Task 2.2.3c): a reopen test proves `duplicate_of_id` is cleared on `duplicate → idea`.
- AC7 (write-layer guard, supporting Task 2.2.3a's `toStatus == BacklogStatusDuplicate` fix): a non-`duplicate` transition with a populated `duplicate_of_id` in the request does NOT persist it.
- AC8/AC6 (error classification, mirroring Task 3.1.3c at the RPC layer): the RPC handler's `duplicate_of_id` lookup returns `connect.CodeInternal` (not `connect.CodeFailedPrecondition`) on a non-not-found lookup error.
- AC2 (integration): a rejected `done → duplicate` transition surfaces at the RPC layer (not just as a pure-function `false` in `session/backlog_test.go`) — **note**: `validation.md` names this test `..._RejectsWithFailedPrecondition`, but direct inspection of the handler (`server/services/backlog_service.go:617-620`) shows a structurally-invalid transition (the `!s.engine.CanTransition(from, to)` branch, which is what rejects `done→duplicate`) returns `connect.CodeInvalidArgument`, not `connect.CodeFailedPrecondition` — `CodeFailedPrecondition` is reserved for `TransitionGuard`'s business-rule rejections (line 640-648) on a *structurally valid* edge. The task below uses the code-correct expectation and a corrected test name; `validation.md` should be read as having a naming/expectation bug on this one row, not as the source of truth for the asserted error code.

**Files**: `server/services/backlog_service_test.go`

##### Task 2.2.5a: Add `TestTransitionBacklogItemStatus_ToDuplicate_SetsStatusAndDuplicateOfIdAtomically` (~4 min)
- Create an item, transition it to `duplicate` with a valid `duplicate_of_id` via the RPC handler, assert the response and a follow-up `GetBacklogItem` both show `status: "duplicate"` and the correct `duplicate_of_id`.
- Files: `server/services/backlog_service_test.go`

##### Task 2.2.5b: Add `TestTransitionBacklogItemStatus_ConcurrentStatusChange_ReturnsPreconditionFailed` (~5 min)
- Simulate the race: transition an item once (changing its status), then attempt a second transition using a `precondition`/expected-status that no longer matches; assert the returned error wraps `session.ErrPreconditionFailed` and surfaces as `connect.CodeAborted` at the RPC layer (matching the existing handling at `server/services/backlog_service.go:663-665`).
- Files: `server/services/backlog_service_test.go`

##### Task 2.2.5c: Update existing test call sites for the new 5th parameter (~3 min)
- Grep `server/services/backlog_service_test.go` for direct `storage.TransitionBacklogItemStatus(` calls (if any exist outside the RPC handler path) and add `nil` as the 5th argument.
- Run `go build ./... && go test ./server/services/... ./session/...` to confirm everything compiles and passes.
- Files: `server/services/backlog_service_test.go`

##### Task 2.2.5d: Add `TestTransitionBacklogItemStatus_Reopen_ClearsDuplicateOfId` (~4 min)
- Create an item, transition it to `duplicate` with a valid `duplicate_of_id` (as in Task 2.2.5a), then transition it again to `idea` (reopen, per Task 2.2.3c). Re-`GetBacklogItem` and assert `status == "idea"` AND `duplicate_of_id == ""` — proves Task 2.2.3c's `ClearDuplicateOfID()`/`SetDuplicateOfID("")` call actually clears the stale link and does so atomically with the status write.
- Files: `server/services/backlog_service_test.go` (or `session/ent_repository_backlog_test.go` if a lower-level test is a better fit for this repository-layer behavior — either location satisfies the AC as long as it exercises `TransitionBacklogItemStatus` directly)

##### Task 2.2.5e: Add `TestTransitionBacklogItemStatus_NonDuplicateTargetWithDuplicateOfId_DoesNotPersistIt` (~4 min)
- Create an item at `idea`. Call the RPC handler's `TransitionBacklogItemStatus` with `TargetStatus: "ready"` and `DuplicateOfId` set to some other existing item's id. Assert the call succeeds (status becomes `ready`) but a follow-up `GetBacklogItem` shows `duplicate_of_id == ""` — proves both the write-layer guard (Task 2.2.3a's `toStatus == BacklogStatusDuplicate` condition) and the RPC-layer construction guard (Task 2.2.4c) actually prevent the write, closing the pre-mortem P2 finding that `opts`/`SetDuplicateOfID` were previously gated only on `duplicate_of_id != ""` with no check on the target status.
- Files: `server/services/backlog_service_test.go`

##### Task 2.2.5f: Add `TestTransitionBacklogItemStatus_DuplicateOfIdLookupInfraError_ReturnsInternal_NotFailedPrecondition` (~4 min)
- Mirrors Task 3.1.3c's MCP-side assertion shape, but exercises the RPC handler's Task 2.2.4a lookup directly: simulate a non-not-found error from the `duplicate_of_id` lookup (e.g. an injected/faked storage error, following whatever fault-injection seam this suite already uses elsewhere) and assert `TransitionBacklogItemStatus` returns `connect.CodeInternal`, not `connect.CodeFailedPrecondition` — proving a genuine infra failure on the second lookup is not misclassified as "your duplicate_of_id is bad." Closes the asymmetric test-coverage gap flagged by pre-mortem P2 #4 (Task 3.1.3c has this test for the MCP path; Story 2.2.5 previously had no equivalent for the structurally identical RPC-handler lookup).
- Files: `server/services/backlog_service_test.go`

##### Task 2.2.5g: Add `TestTransitionBacklogItemStatus_DoneToDuplicate_RejectsWithInvalidArgument` (~3 min)
- Create an item at `done`. Call the RPC handler with `TargetStatus: "duplicate"` and a valid `DuplicateOfId`. Assert `connect.CodeInvalidArgument` (per `server/services/backlog_service.go:617-620`'s `CanTransition` gate — this is a structurally-invalid edge, not a guard-rule rejection) and that no row mutation occurs. Closes the validation-mapping gap where AC2's `done→duplicate` rejection was previously proven only at the pure-function level (`session/backlog_test.go`), never through the actual RPC handler.
- Files: `server/services/backlog_service_test.go`

---

## Phase 3: `mark_duplicate` MCP Tool

### Epic 3.1: Agent-facing self-service duplicate marking
**Goal**: agents can mark an item duplicate without falling back to archive + free-text note (AC8).

#### Story 3.1.1: Implement the `mark_duplicate` handler
**As a** triage or work agent session, **I want** a `mark_duplicate(item_id, duplicate_of_id, note?)` tool, **so that** I can record a duplicate finding without an RPC round trip through the web UI.

**Acceptance Criteria**:
- AC8: `mark_duplicate` performs `CanTransitionBacklog` + `TransitionGuard` + the transition end-to-end, with not-found vs. infra-error disambiguation for BOTH ids, and a best-effort note append. It also enforces the same session-item authorization check every other backlog-mutating MCP tool in this file enforces.
  - *Given* a caller session UUID that is valid but is NOT linked to `item_id` via any `ItemSession` (or is linked to a completely unrelated item), *When* `mark_duplicate({item_id: "67de6c7b-...", duplicate_of_id: "10128af0-e1eb-47bc-9016-3af8fde83b4d"})` is called, *Then* it returns `errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", "")` and no transition occurs.
  - *Given* item `67de6c7b-...` at status `idea` and canonical item `10128af0-e1eb-47bc-9016-3af8fde83b4d` at status `idea`, and the caller session IS linked to `67de6c7b-...` (any role, triage or work), *When* `mark_duplicate({item_id: "67de6c7b-...", duplicate_of_id: "10128af0-e1eb-47bc-9016-3af8fde83b4d", note: "same install-service.sh .zshrc bug"})` is called, *Then* the tool returns a success text result, item `67de6c7b-...`'s status becomes `duplicate`, its `duplicate_of_id` becomes `10128af0-e1eb-47bc-9016-3af8fde83b4d`, and its `notes` field has the note appended.
  - *Given* `item_id` does not exist, *When* `mark_duplicate` is called, *Then* it returns `errResult(ErrItemNotFound, ...)` referencing `item_id` specifically (not a generic internal error).
  - *Given* `item_id` exists but `duplicate_of_id` does not exist, *When* `mark_duplicate` is called, *Then* it returns `errResult(ErrItemNotFound, ...)` referencing `duplicate_of_id` specifically — this is the highest-risk case per pitfalls research (the second lookup is new, easy to forget the `errors.Is(err, session.ErrNotFound)` check on).
  - *Given* `item_id == duplicate_of_id` (self-reference), *When* `mark_duplicate` is called, *Then* it returns `errResult(ErrInvalidArgument, ...)` (a guard/business-rule rejection, not a not-found or infra error).
  - *Given* a valid transition but the item's `notes` update fails (simulated storage error on the follow-up `UpdateBacklogItem` call), *When* `mark_duplicate` is called, *Then* the tool still returns success for the transition (note-append failure is logged as a warning and does not fail the overall call).

**Files**: `server/mcp/tools_backlog.go`

##### Task 3.1.1a: Implement argument parsing + validation (~4 min)
- New function `func (h *backlogHandlers) markDuplicate(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)` following the exact shape of `requestReview` (`server/mcp/tools_backlog.go:235-274`): `callerSessionUUID` → parse `item_id`/`duplicate_of_id`/`note` from `req.GetArguments()` → `validateUUID` on both ids.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1a-bis: Verify the caller session is linked to `item_id` (~3 min)
- **This closes a real authorization hole**: without this check, any session holding *any* valid `STAPLER_SESSION_UUID` — including one linked to a completely unrelated item, or not linked to anything — could mark an arbitrary `item_id` duplicate-of an arbitrary `duplicate_of_id`. Immediately after Task 3.1.1a's UUID validation and before any `GetBacklogItem` lookups, add the same link check `reportProgress` already performs (`server/mcp/tools_backlog.go:207-213`):
  ```go
  _, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
  if linkErr != nil {
      if errors.Is(linkErr, session.ErrNotFound) {
          return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", ""), nil
      }
      return errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), ""), nil
  }
  ```
- Deliberately **any** role passes (no `SessionRole` check) — unlike `submitReviewVerdict` (`review`-only) or `submitTriageResult` (`triage`-only) — because `mark_duplicate` must work for both `triage` and `work` roles per FR5.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1b: Look up the source item and resolve `DuplicateOfExists`/`DuplicateOfStatus` (~4 min)
- `item, err := h.storage.GetBacklogItem(ctx, itemID)` — on `errors.Is(err, session.ErrNotFound)`, return `errResult(ErrItemNotFound, fmt.Sprintf("backlog item %q not found", itemID), "")`; on any other error, `errResult(ErrInternalError, ...)`.
- `target, targetErr := h.storage.GetBacklogItem(ctx, duplicateOfID)` — **this is the pitfall to guard against**: on `errors.Is(targetErr, session.ErrNotFound)`, return `errResult(ErrItemNotFound, fmt.Sprintf("duplicate_of_id %q not found", duplicateOfID), "")` (explicitly naming which id, per AC8); on any other error, `errResult(ErrInternalError, fmt.Sprintf("look up duplicate_of_id: %v", targetErr), "")`.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1c: Run `CanTransitionBacklog` + `TransitionGuard` directly (~4 min)
- ```go
  from := session.BacklogStatus(item.Status)
  if !session.CanTransitionBacklog(from, session.BacklogStatusDuplicate) {
      return errResult(ErrInvalidArgument, fmt.Sprintf("cannot mark item %s duplicate from status %q", itemID, from), ""), nil
  }
  guardInput := session.BacklogItemTransitionInput{
      ID:                itemID,
      Status:            from,
      AcCriteriaJSON:    item.AcceptanceCriteria,
      PlanApproved:      item.PlanApproved,
      SkipPlanning:      item.SkipPlanning,
      PlanArtifactsPath: item.PlanArtifactsPath,
      DuplicateOfID:     duplicateOfID,
      DuplicateOfExists: true, // only reached if the target lookup above succeeded
      DuplicateOfStatus: session.BacklogStatus(target.Status),
  }
  if guardErr := session.TransitionGuard(guardInput, session.BacklogStatusDuplicate); guardErr != nil {
      return errResult(ErrInvalidArgument, guardErr.Error(), ""), nil
  }
  ```
  Note: self-reference (`item_id == duplicate_of_id`) and chained-duplicate-target both surface here as `ErrInvalidArgument`-flavored guard rejections — consistent with "guard-rejected transitions map to the same not-found-flavored user error as a literally-missing item" being handled by the guard, not by a separate branch (`ErrDuplicateOfSelf`/`ErrDuplicateOfInvalidTarget` both read clearly via `guardErr.Error()`).
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1d: Perform the transition (~2 min)
- ```go
  precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(from)}
  opts := &session.TransitionOptions{DuplicateOfID: duplicateOfID}
  updated, transErr := h.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusDuplicate, precondition, opts)
  if transErr != nil {
      if errors.Is(transErr, session.ErrPreconditionFailed) {
          return errResult(ErrInvalidArgument, "item status changed concurrently — re-fetch with get_backlog_item and retry", ""), nil
      }
      return errResult(ErrInternalError, fmt.Sprintf("transition to duplicate: %v", transErr), ""), nil
  }
  _ = updated
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1e: Best-effort note append, structurally after success, behind a test seam (~4 min)
- Add a new field to `backlogHandlers` (`server/mcp/tools_backlog.go:63-67`) so the note-append call can be faked in a test without a real storage-layer fault-injection mechanism (none exists today — `backlogHandlers.storage` is a concrete `*session.Storage`, no interface/mock seam):
  ```go
  type backlogHandlers struct {
      storage  *session.Storage
      store    session.InstanceStore
      eventBus *events.EventBus // optional; nil means notifications are disabled
      // noteAppendFn, if set, is used instead of storage.UpdateBacklogItem for the
      // best-effort note append in markDuplicate. Exists purely as a test seam —
      // production code never sets it, so it always falls back to the real call.
      noteAppendFn func(ctx context.Context, id string, update session.BacklogItemUpdate, precondition *session.BacklogItemPrecondition) (*session.BacklogItemData, error)
  }
  ```
- After the transition has succeeded (i.e. after Task 3.1.1d's success path, and only building the success result after this block runs):
  ```go
  if note != "" {
      newNotes := item.Notes
      if newNotes != "" {
          newNotes += "\n"
      }
      newNotes += note
      appendFn := h.noteAppendFn
      if appendFn == nil {
          appendFn = h.storage.UpdateBacklogItem
      }
      if _, noteErr := appendFn(ctx, itemID, session.BacklogItemUpdate{Notes: &newNotes}, nil); noteErr != nil {
          log.WarningLog.Printf("[mcp:mark_duplicate] note append failed for item %s: %v", itemID, noteErr)
          // Non-fatal: the transition already succeeded and is reported as such below.
      }
  }
  return mcpgo.NewToolResultText(fmt.Sprintf(
      "Item %s marked duplicate of %s.", itemID, duplicateOfID,
  )), nil
  ```
- Files: `server/mcp/tools_backlog.go`

#### Story 3.1.2: Register the tool
**As a** developer, **I want** `mark_duplicate` registered on the MCP server, **so that** agent sessions can discover and call it.

**Acceptance Criteria**:
- AC8 (registration): `mark_duplicate` appears in the MCP tool list with `item_id`, `duplicate_of_id` required and `note` optional.
  - *Given* the MCP server is running, *When* a client lists available tools, *Then* `mark_duplicate` is present with the documented parameter schema.

**Files**: `server/mcp/tools_backlog.go`

##### Task 3.1.2a: Add `s.AddTool(mcpgo.NewTool("mark_duplicate", ...), h.markDuplicate)` (~3 min)
- Register in `registerBacklogTools` (`server/mcp/tools_backlog.go:541-...`), following the `submit_review_verdict` registration shape (lines 592-618):
  ```go
  s.AddTool(
      mcpgo.NewTool("mark_duplicate",
          mcpgo.WithDescription("Mark a backlog item as a duplicate of another item, linking it to the canonical item. Use this INSTEAD OF archiving + a free-text note when triage discovers an item describes the same problem as an existing item. Transitions item_id to 'duplicate' status. Fails if duplicate_of_id is empty, self-referencing, references a nonexistent item, or references an item that is itself already marked duplicate (chains are not allowed — always point at the canonical item directly)."),
          mcpgo.WithString("item_id", mcpgo.Description("UUID of the backlog item to mark as a duplicate"), mcpgo.Required()),
          mcpgo.WithString("duplicate_of_id", mcpgo.Description("UUID of the canonical item this is a duplicate of"), mcpgo.Required()),
          mcpgo.WithString("note", mcpgo.Description("Optional note appended to the item's notes field explaining the duplicate finding")),
      ),
      h.markDuplicate,
  )
  ```
- Files: `server/mcp/tools_backlog.go`

#### Story 3.1.3: Tests for `mark_duplicate`
**As a** developer, **I want** the happy path and every error branch covered, **so that** the two-id disambiguation risk flagged in pitfalls research is actually guarded by a test, not just code review.

**Acceptance Criteria**:
- AC12 (partial): `tools_backlog_test.go` covers `mark_duplicate` happy path, `item_id` not-found, `duplicate_of_id` not-found, guard rejection (self-ref and nonexistent), and best-effort note-append failure not failing the overall call.
- AC8 (session-item authorization, Task 3.1.1a-bis): a caller session not linked to `item_id` is rejected with `ErrPermissionDenied` and no transition occurs.

**Files**: `server/mcp/tools_backlog_test.go`

##### Task 3.1.3a: `TestMarkDuplicate_HappyPath_TransitionsAndAppendsNote` (~5 min)
- Create two items, call `markDuplicate` with a note, assert the tool result is success, re-fetch and assert status/`duplicate_of_id`/notes.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 3.1.3b: `TestMarkDuplicate_ItemIdNotFound_ReturnsItemNotFound` (~2 min)
- Call with a well-formed but nonexistent `item_id`; assert `ErrItemNotFound` and the message references `item_id`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 3.1.3c: `TestMarkDuplicate_DuplicateOfIdNotFound_ReturnsItemNotFound_NotInternalError` (~3 min)
- Create a real `item_id`, use a well-formed but nonexistent `duplicate_of_id`; assert `ErrItemNotFound` (NOT `ErrInternalError`) and the message references `duplicate_of_id` specifically. This is the test that directly catches the pitfalls-research-flagged regression risk.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 3.1.3d: `TestMarkDuplicate_SelfReference_ReturnsInvalidArgument` and `..._ChainedDuplicateTarget_ReturnsInvalidArgument` (~4 min)
- Two tests: `item_id == duplicate_of_id`; and `duplicate_of_id` pointing at an item whose status is already `duplicate`. Both assert `ErrInvalidArgument`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 3.1.3e: `TestMarkDuplicate_NoteAppendFailure_DoesNotFailTransition` (~4 min)
- Uses the `noteAppendFn` seam introduced in Task 3.1.1e (no storage-handle closing/corrupting — that approach is unreliable: closing the connection breaks the test's own verification re-fetch too, and file-corruption tricks don't reliably fail an already-open fd on Linux). Construct the handler with a failing fake:
  ```go
  handler := &backlogHandlers{
      storage: storage,
      noteAppendFn: func(ctx context.Context, id string, update session.BacklogItemUpdate, precondition *session.BacklogItemPrecondition) (*session.BacklogItemData, error) {
          return nil, errors.New("simulated note-append failure")
      },
  }
  ```
  Call `markDuplicate` with a non-empty `note` and assert: (1) the tool result is still success text (transition succeeded, note-append failure is non-fatal); (2) a follow-up `storage.GetBacklogItem` (using the real, non-faked storage) shows `status: "duplicate"` and the correct `duplicate_of_id` — proving the transition itself was unaffected by the injected fake being wired in only for the note-append call.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 3.1.3f: `TestMarkDuplicate_SessionNotLinkedToItem_ReturnsPermissionDenied` (~3 min)
- Create an item (or use a caller session UUID that is either unlinked to any item, or linked to a *different*, unrelated item — cover at least one of the two). Call `markDuplicate` with that caller session and assert: `errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", "")` is returned, and a follow-up `GetBacklogItem` shows the item's status unchanged (no transition occurred). This directly tests Task 3.1.1a-bis's authorization check (added in the first repair pass) — closes the gap where Story 3.1.1's own acceptance criteria describe this exact scenario verbatim but no prior task in this story's list (3.1.3a–e) enumerated a test for it.
- Files: `server/mcp/tools_backlog_test.go`

---

## Phase 4: List Exclusion

### Epic 4.1: Exclude `duplicate` from default/active views
**Goal**: `duplicate` items don't clutter the active backlog by default (AC9).

#### Story 4.1.1: Extend `StatusNotIn`
**As a** backlog user, **I want** `duplicate` items excluded from the default view, **so that** resolved duplicates don't clutter active triage.

**Acceptance Criteria**:
- AC9: `ListBacklogItems` excludes `duplicate` items from default/active-item results (`ExcludeTerminal`/`StatusNotIn`), matching existing `archived`/`done` exclusion behavior.
  - *Given* three items with statuses `idea`, `done`, `duplicate`, *When* `ListBacklogItems(ctx, BacklogItemFilter{ExcludeTerminal: true})` is called, *Then* only the `idea` item is returned.
  - *Given* the same three items, *When* `ListBacklogItems(ctx, BacklogItemFilter{Statuses: []string{"duplicate"}})` is called (explicit status filter, bypassing `ExcludeTerminal`), *Then* the `duplicate` item IS returned — explicit filters always override the default exclusion, matching existing `archived`/`done` behavior.

**Files**: `session/ent_repository_backlog.go`, `server/services/backlog_service_test.go`

##### Task 4.1.1a: Add `duplicate` to the `StatusNotIn` call (~1 min)
- `session/ent_repository_backlog.go:140-143`: change
  ```go
  q = q.Where(backlogitem.StatusNotIn(
      string(BacklogStatusDone),
      string(BacklogStatusArchived),
  ))
  ```
  to
  ```go
  q = q.Where(backlogitem.StatusNotIn(
      string(BacklogStatusDone),
      string(BacklogStatusArchived),
      string(BacklogStatusDuplicate),
  ))
  ```
- Files: `session/ent_repository_backlog.go`

##### Task 4.1.1b: Test the exclusion (~3 min)
- New test(s) in `server/services/backlog_service_test.go` — confirmed by direct inspection (`grep -rln "func TestListBacklogItems"`) that this, not `session/ent_repository_backlog_test.go` (which does not exist in this repo), is the actual home of `TestListBacklogItems*`. Add `TestListBacklogItems_ExcludesDuplicateByDefault` (extending the existing `TestListBacklogItems_DefaultFilterHidesTerminalStatuses`-shaped coverage): assert a `duplicate`-status item is absent from `ExcludeTerminal: true` results and present when explicitly filtered by `Statuses: []string{"duplicate"}`.
- Files: `server/services/backlog_service_test.go`

---

## Phase 5: Frontend

### Epic 5.1: Theme Tokens (6 themes)
**Goal**: a `duplicate` status has a distinct, WCAG-AA-compliant color token in every theme (AC10, contrast portion).

#### Story 5.1.1: Add the token triplet to the theme contract
**As a** frontend developer, **I want** `vars.statusBadge.duplicateBg/Fg/Border` declared in the shared contract, **so that** `createTheme` type-checks force every theme to supply real values.

**Acceptance Criteria**:
- AC10 (contract portion): the new triplet exists in `theme-contract.css.ts` and is NOT an alias of `idleBg/Fg/Border` or `archived`'s tokens (`surfaceMuted`/`textDisabled`).
  - *Given* `web-app/src/styles/theme-contract.css.ts`, *When* inspecting the `statusBadge` block, *Then* it contains `duplicateBg: null, duplicateFg: null, duplicateBorder: null` as three new, independent keys.

**Files**: `web-app/src/styles/theme-contract.css.ts`

##### Task 5.1.1a: Add the triplet (~1 min)
- In the `statusBadge` block (`web-app/src/styles/theme-contract.css.ts:92-111`), add after `processingBorder: null,`:
  ```ts
  duplicateBg: null,
  duplicateFg: null,
  duplicateBorder: null,
  ```
- Files: `web-app/src/styles/theme-contract.css.ts`

#### Story 5.1.2: Populate real values in all 6 themes
**As a** frontend developer, **I want** WCAG-AA-compliant hex values in every theme, **so that** the `duplicate` chip is legible and the TypeScript build enforces completeness across all 6 themes.

**Acceptance Criteria**:
- AC10 (values portion): all 6 `createTheme(vars, {...})` blocks supply the new triplet with contrast ≥ 4.5:1 (normal-text threshold — the chip's 10px text does not qualify for the relaxed 3:1 large-text threshold).
  - *Given* `lightTheme`'s new pair `duplicateBg: "#fae8ff"` / `duplicateFg: "#86198f"`, *When* the WCAG relative-luminance contrast ratio is computed, *Then* it is 7.08:1 (≥ 4.5:1, PASS) — computed and verified during planning (see values below); re-verify with `npm run check-contrast` once the script is extended (Task 5.1.2b), or manually if not.
  - *Given* `cyberpunk77Theme`'s new pair `duplicateBg: "#1a0022"` / `duplicateFg: "#ff6ec7"`, *When* the ratio is computed, *Then* it is 7.79:1 (PASS).

**Files**: `web-app/src/styles/theme.css.ts`

##### Task 5.1.2a: Add the 6 theme blocks with pre-computed, WCAG-AA-passing values (~5 min)
- Add to each theme's `statusBadge` block, using the repo's existing inline-ratio-comment convention:
  ```ts
  // lightTheme (theme.css.ts:124-144)
  duplicateBg: "#fae8ff",     /* ratio 7.08:1 ✅ vs duplicateFg */
  duplicateFg: "#86198f",
  duplicateBorder: "#e9a5f1",

  // darkTheme (theme.css.ts:218-...)
  duplicateBg: "#3b0764",     /* ratio 8.52:1 ✅ vs duplicateFg */
  duplicateFg: "#f0abfc",
  duplicateBorder: "#a21caf",

  // matrixTheme (theme.css.ts:315-...)
  duplicateBg: "#001a0d",     /* ratio 14.23:1 ✅ vs duplicateFg */
  duplicateFg: "#5fffb0",
  duplicateBorder: "#00994d",

  // cyberpunk77Theme (theme.css.ts:420-...)
  duplicateBg: "#1a0022",     /* ratio 7.79:1 ✅ vs duplicateFg */
  duplicateFg: "#ff6ec7",
  duplicateBorder: "#c026d3",

  // wh40kTheme (theme.css.ts:525-...)
  duplicateBg: "#1a0e08",     /* ratio 8.32:1 ✅ vs duplicateFg */
  duplicateFg: "#e0a030",
  duplicateBorder: "#b8860b",

  // cleanTheme (theme.css.ts:630-...)
  duplicateBg: "#f3e8ff",     /* ratio 5.92:1 ✅ vs duplicateFg */
  duplicateFg: "#7e22ce",
  duplicateBorder: "#c084fc",
  ```
- Note: `matrixTheme`/`cyberpunk77Theme` both use a magenta/violet family here deliberately — distinct from `wh40k`'s amber/gold choice (matched to that theme's gothic red/gold palette) and distinct in all themes from `processing`'s indigo/blue-violet tokens.
- Files: `web-app/src/styles/theme.css.ts`

##### Task 5.1.2b: Extend or manually verify contrast coverage (~4 min)
- `web-app/scripts/check-theme-contrast.ts` currently checks 4 generic text/bg pairs across only 4 of 6 themes and does not check any `statusBadge.*` pair at all (confirmed gap per pitfalls research). Two acceptable outcomes, pick one:
  - (a) Extend the script's pair list to include `{bg: "duplicateBg", fg: "duplicateFg"}` for all 6 themes, then run `npm run check-contrast` and confirm PASS; or
  - (b) Skip extending the script and rely on the inline ratios already computed and commented in Task 5.1.2a (all 6 independently verified above via the standard WCAG relative-luminance formula — see this plan's Task 5.1.2a values).
- Given the small, well-defined scope (one new triplet, ratios already computed by hand and double-checked), (b) is sufficient to close AC10's contrast requirement without expanding this feature's blast radius into unrelated script coverage gaps for other themes' existing pairs. Document the choice made in the PR description.
- Files: `web-app/scripts/check-theme-contrast.ts` (only if choosing (a))

---

### Epic 5.2: Shared Label + Type Plumbing
**Goal**: `duplicate` is a known status string end-to-end in the frontend type system (supports AC10, AC11).

#### Story 5.2.1: Add the shared label
**Acceptance Criteria**:
- Supporting AC10: `getStatusLabel("duplicate")` returns `"Duplicate"` (not the raw string or a humanized fallback).
  - *Given* `web-app/src/lib/backlog/status.ts`, *When* `getStatusLabel("duplicate")` is called, *Then* it returns `"Duplicate"`.

**Files**: `web-app/src/lib/backlog/status.ts`

##### Task 5.2.1a: Add to `STATUS_LABELS` (~1 min)
- Add `duplicate: "Duplicate",` to the map (`web-app/src/lib/backlog/status.ts:5-13`).
- Files: `web-app/src/lib/backlog/status.ts`

#### Story 5.2.2: Extend `KnownBacklogStatus` and the `BacklogItem` interface
**Acceptance Criteria**:
- Supporting AC10/AC11: `"duplicate"` is a `KnownBacklogStatus` and `duplicateOfId` is a mapped field on `BacklogItem`.
  - *Given* a proto `BacklogItemProto` with `duplicateOfId: "10128af0-e1eb-47bc-9016-3af8fde83b4d"`, *When* `mapBacklogItem(proto)` runs, *Then* the returned `BacklogItem.duplicateOfId` equals `"10128af0-e1eb-47bc-9016-3af8fde83b4d"`.

**Files**: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 5.2.2a: Add `"duplicate"` to `KnownBacklogStatus` (~1 min)
- `web-app/src/lib/hooks/useBacklogService.ts:20`: add `| "duplicate"` to the union.
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 5.2.2b: Add `duplicateOfId?: string` to the `BacklogItem` interface (~1 min)
- Add after `planArtifactsPath?: string;` (`web-app/src/lib/hooks/useBacklogService.ts:78`).
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 5.2.2c: Map it in `mapBacklogItem` (~1 min)
- Add `duplicateOfId: p.duplicateOfId || undefined,` to the return object (`web-app/src/lib/hooks/useBacklogService.ts:239-259`), alongside `planArtifactsPath`.
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 5.2.2d: Add `mapBacklogItem_should_IncludeDuplicateOfId_When_ProtoHasIt` (~2 min)
- New unit test (no existing test file covers `mapBacklogItem` directly for this field): construct a fake proto object with `duplicateOfId: "10128af0-e1eb-47bc-9016-3af8fde83b4d"`, call `mapBacklogItem(proto)`, assert `result.duplicateOfId === "10128af0-e1eb-47bc-9016-3af8fde83b4d"`. Closes the validation-mapping gap — Story 5.2.2's own acceptance criterion states this exact scenario but no prior task in Epic 5.4 tested `mapBacklogItem` directly.
- Files: `web-app/src/lib/hooks/__tests__/useBacklogService.test.ts` (create if it doesn't already exist; confirm via `grep -rl "mapBacklogItem" web-app/src/lib/hooks` first)

---

### Epic 5.3: The 4 Independent Status-Aware Surfaces
**Goal**: `duplicate` renders correctly and distinctly everywhere status is shown (AC10, AC11).

#### Story 5.3.1: `BacklogItemBadge`
**Acceptance Criteria**:
- AC10 (surface 1 of 4): the compact badge renders a distinct `duplicate` chip, not the `archived` fallback.
  - *Given* `<BacklogItemBadge status="duplicate" .../>`, *When* rendered, *Then* the status chip has class `styles.statusDuplicate` (not `styles.statusArchived`) and text "DUPLICATE" (uppercased via existing `text-transform: uppercase`).

**Files**: `web-app/src/components/backlog/BacklogItemBadge.tsx`, `web-app/src/components/backlog/BacklogItemBadge.css.ts`

##### Task 5.3.1a: Add `statusDuplicate` CSS export (~2 min)
- Add to `BacklogItemBadge.css.ts` (after `statusRefining`, line 67-71):
  ```ts
  export const statusDuplicate = style({
    background: vars.statusBadge.duplicateBg,
    color: vars.statusBadge.duplicateFg,
    border: `1px solid ${vars.statusBadge.duplicateBorder}`,
  });
  ```
- Files: `web-app/src/components/backlog/BacklogItemBadge.css.ts`

##### Task 5.3.1b: Add to `STATUS_CLASS` map, retyped as `Record<KnownBacklogStatus, string>` (~2 min)
- Add `duplicate: styles.statusDuplicate,` to `STATUS_CLASS` (`web-app/src/components/backlog/BacklogItemBadge.tsx:14-22`).
- Retype `STATUS_CLASS`'s declaration from `Record<string, string>` to `Record<KnownBacklogStatus, string>` (import `KnownBacklogStatus` from `web-app/src/lib/hooks/useBacklogService.ts` if not already imported in this file). This makes a missing status entry a `tsc` compile error at this definition site, instead of a silent runtime fallback — the single highest-risk silent-failure point this feature's own Domain Glossary flags. Keep the call-site lookup function's `?? styles.statusArchived` fallback as-is: it's still needed for the wider `BacklogItemStatus = KnownBacklogStatus | (string & {})` type at the lookup call site (defensive against a genuinely-unknown status string sent by the server), but the map *literal* itself is now compile-checked.
- Files: `web-app/src/components/backlog/BacklogItemBadge.tsx`

#### Story 5.3.2: `BacklogItemDetail` — badge + "Duplicate of:" link resolution
**Acceptance Criteria**:
- AC10 (surface 2 of 4): the detail panel's own (separate) status map also gets a `duplicate` entry.
- AC11: shows a client-side-resolved "Duplicate of: " link to the canonical item when `duplicate_of_id` is set, with 3 states — loading, resolved, missing.
  - *Given* item `67de6c7b-...` with `status: "duplicate"`, `duplicateOfId: "10128af0-e1eb-47bc-9016-3af8fde83b4d"`, and the canonical-item fetch in flight, *When* `BacklogItemDetail` renders, *Then* it shows the text "Duplicate of: Loading…" with `aria-live="polite"` on the container (label visible immediately, no spinner-only state; an explicit word is used instead of a bare ellipsis because default punctuation-verbosity settings in major screen readers commonly suppress a standalone "…", which would otherwise announce only "Duplicate of:" with no perceptible loading signal).
  - *Given* the canonical-item fetch resolves to `{id: "10128af0-e1eb-47bc-9016-3af8fde83b4d", title: "install-service.sh sources .zshrc unconditionally"}`, *When* rendered, *Then* it shows "Duplicate of: install-service.sh sources .zshrc unconditionally" as a clickable element that, when clicked, updates the URL to `/backlog?item=10128af0-e1eb-47bc-9016-3af8fde83b4d` (same mechanism as `handleRowClick` in `page.tsx`).
  - *Given* the canonical-item fetch returns `null` (not found or errored — `getBacklogItem`'s documented contract), *When* rendered, *Then* it shows plain, non-interactive text "Duplicate of: (item not found)" — never a dead link, never a thrown error.

**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/components/backlog/BacklogItemDetail.css.ts`, `web-app/src/app/backlog/page.tsx`

##### Task 5.3.2a: Add `statusDuplicate` CSS export to `BacklogItemDetail.css.ts` (~2 min)
- Same shape as Task 5.3.1a, in `BacklogItemDetail.css.ts`'s equivalent status-class section.
- Files: `web-app/src/components/backlog/BacklogItemDetail.css.ts`

##### Task 5.3.2b: Add to `BacklogItemDetail`'s own `STATUS_CLASS` map, retyped as `Record<KnownBacklogStatus, string>` (~2 min)
- Add `duplicate: styles.statusDuplicate,` to `STATUS_CLASS` (`web-app/src/components/backlog/BacklogItemDetail.tsx:21-29`).
- Retype this file's own independent `STATUS_CLASS` declaration from `Record<string, string>` to `Record<KnownBacklogStatus, string>` (import `KnownBacklogStatus` from `web-app/src/lib/hooks/useBacklogService.ts` if not already imported here) — same rationale as Task 5.3.1b: a missing entry becomes a compile error at this definition site. Keep the runtime `?? styles.statusArchived` fallback at the call-site lookup only.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.3.2c: Add `onNavigateToItem` prop threaded from `page.tsx` (~2 min)
- Add `onNavigateToItem?: (itemId: string) => void` to `BacklogItemDetailProps` (`web-app/src/components/backlog/BacklogItemDetail.tsx:16-19`).
- In `page.tsx`, pass `onNavigateToItem={handleRowClick}` to `<BacklogItemDetail>` (`web-app/src/app/backlog/page.tsx:462-465`) — reuses the exact same `?item=` query-param-setting function already used for row clicks, no new navigation mechanism.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/app/backlog/page.tsx`

##### Task 5.3.2d: Implement the 3-state canonical-item resolution (~6 min)
- Add local state `const [duplicateOfItem, setDuplicateOfItem] = useState<BacklogItem | null | undefined>(undefined)` (`undefined` = not yet fetched, `null` = fetched-and-missing, object = resolved) near the existing `item`/`loading` state.
- In the existing `load` callback (or a new `useEffect` keyed on `item?.duplicateOfId`), when `item?.status === "duplicate" && item.duplicateOfId`, call `getBacklogItem(item.duplicateOfId)` and set the result.
- **Reset on item navigation**: the effect resolving `duplicateOfItem` must reset it back to `undefined` when `item?.id` changes (not just when `duplicateOfId` changes) — otherwise navigating directly between two different `duplicate`-status items (e.g. via the link row itself, or a table row click) can briefly show the *previous* item's stale resolved title before the new fetch lands, since `id` changing doesn't necessarily mean `duplicateOfId` changed too in a way the effect's dependency array would catch on its own.
- Render, immediately after the status badge block (`web-app/src/components/backlog/BacklogItemDetail.tsx:399-410`):
  ```tsx
  {item.status === "duplicate" && item.duplicateOfId && (
    <div className={styles.duplicateOfRow} aria-live="polite">
      {duplicateOfItem === undefined && <span>Duplicate of: Loading…</span>}
      {duplicateOfItem === null && <span>Duplicate of: (item not found)</span>}
      {duplicateOfItem && (
        <button
          type="button"
          className={styles.duplicateOfLink}
          onClick={() => onNavigateToItem?.(duplicateOfItem.id)}
          data-testid="duplicate-of-link"
        >
          Duplicate of: {duplicateOfItem.title}
        </button>
      )}
    </div>
  )}
  ```
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.3.2e: Add `duplicateOfRow`/`duplicateOfLink` CSS (~2 min)
- Add minimal vanilla-extract styles to `BacklogItemDetail.css.ts`: `duplicateOfRow` (flex row, small top margin), `duplicateOfLink` (button-reset styling, `color: vars.color.actionPrimary` or nearest existing link-color token, `cursor: pointer`, underline on hover/focus).
- Files: `web-app/src/components/backlog/BacklogItemDetail.css.ts`

#### Story 5.3.3: `page.tsx` — table row chip + filter chips
**Acceptance Criteria**:
- AC10 (surface 3 of 4): the backlog table's row status chip AND the filter chips both handle `duplicate` distinctly.
  - *Given* a row for item `67de6c7b-...` with `status: "duplicate"`, *When* the backlog table renders, *Then* the row's status chip has class `styles.statusDuplicate`.
  - *Given* the default (no filter interaction) `StatusFilterChips` render, *When* inspecting `displayStatuses`, *Then* `duplicate` is excluded from the default-shown chips (same "too noisy" treatment as `archived`), while still present in `ALL_STATUSES` for completeness/sorting purposes.

**Files**: `web-app/src/app/backlog/page.tsx`, `web-app/src/app/backlog/backlog.css.ts`

##### Task 5.3.3a: Add `statusDuplicate` CSS export to `backlog.css.ts` (~2 min)
- Same shape as Task 5.3.1a.
- Files: `web-app/src/app/backlog/backlog.css.ts`

##### Task 5.3.3b: Add `duplicate` to `ALL_STATUSES` and `STATUS_CSS`, retyping `STATUS_CSS` as `Record<KnownBacklogStatus, string>` (~3 min)
- Add `"duplicate"` to `ALL_STATUSES` (`web-app/src/app/backlog/page.tsx:29-37`) — after `"archived"`.
- Add `duplicate: styles.statusDuplicate,` to `STATUS_CSS` (`web-app/src/app/backlog/page.tsx:39-47`).
- Retype `STATUS_CSS`'s declaration from `Record<string, string>` to `Record<KnownBacklogStatus, string>` (import `KnownBacklogStatus` from `web-app/src/lib/hooks/useBacklogService.ts` if not already imported here) — same rationale as Tasks 5.3.1b/5.3.2b: a missing entry becomes a compile error at this definition site. Keep the runtime `?? styles.statusArchived` fallback at the call-site lookup only.
- Files: `web-app/src/app/backlog/page.tsx`

##### Task 5.3.3c: Exclude `duplicate` from default filter chips (~1 min)
- Change `displayStatuses` (`web-app/src/app/backlog/page.tsx:88`) from
  ```ts
  const displayStatuses = ALL_STATUSES.filter((s) => s !== "archived");
  ```
  to
  ```ts
  const displayStatuses = ALL_STATUSES.filter((s) => s !== "archived" && s !== "duplicate");
  ```
- Files: `web-app/src/app/backlog/page.tsx`

#### Story 5.3.4: `BacklogItemCard` — action spec
**Acceptance Criteria**:
- AC10 (surface 4 of 4): the card's action button shows a sensible label for `duplicate` items, not the raw status string.
  - *Given* `item.status === "duplicate"`, *When* `getActionSpec(item)` is called, *Then* it returns `{label: "Duplicate", action: "duplicate", isDone: true}` (not the `default` branch's raw-string fallback).

**Files**: `web-app/src/components/backlog/BacklogItemCard.tsx`

##### Task 5.3.4a: Add the `case "duplicate":` branch (~1 min)
- Add before `default:` (`web-app/src/components/backlog/BacklogItemCard.tsx:42-45`):
  ```ts
  case "duplicate":
    return { label: "Duplicate", action: "duplicate", isDone: true };
  ```
- Files: `web-app/src/components/backlog/BacklogItemCard.tsx`

---

### Epic 5.4: Frontend Tests
**Goal**: the load-bearing exhaustiveness test exists — the one thing that actually catches a missing `duplicate` entry in any of the 3 independent status→class maps (AC12, partial).

#### Story 5.4.1: Jest coverage across all 3 badge surfaces + list exclusion + link resolution
**Acceptance Criteria**:
- AC12 (frontend portion): Jest tests cover badge rendering across ALL statuses including `duplicate` in each of the 3 independently-mapped surfaces, list-exclusion default-filter behavior, and all 3 canonical-item-link resolution states.
- AC10 (action-button no-op click, `BacklogItemCard`): clicking the `duplicate` action button does not invoke `onAction`, matching `isDone: true`'s existing no-op semantics for other terminal statuses.
- AC11 (archived canonical resolution): a `duplicate` item whose canonical target is itself `archived` still resolves to the RESOLVED (clickable-link) state, not MISSING.
- AC11/UX §5.12 (keyboard activation): the resolved "Duplicate of:" link is activatable via Enter when focused, not just via click.

**Files**: `web-app/src/components/backlog/BacklogItemBadge.test.tsx`, `web-app/src/components/backlog/BacklogItemDetail.test.tsx`, `web-app/src/app/backlog/page.test.tsx`, `web-app/src/components/backlog/BacklogItemCard.test.tsx` (or existing equivalent test files — locate via `grep -rl "BacklogItemBadge\|BacklogItemDetail\|BacklogItemCard" web-app/src --include=*.test.tsx`)

##### Task 5.4.1a: `BacklogItemBadge` — all-statuses render test (~3 min)
- Parameterized test rendering `<BacklogItemBadge status={s} .../>` for every status in `["idea","refining","ready","in_progress","review","done","archived","duplicate"]`, asserting each gets a distinct CSS class (specifically: `duplicate`'s class is NOT equal to `archived`'s class — the exact regression pitfalls research flags). Note: Task 5.3.1b's `Record<KnownBacklogStatus, string>` retyping now also catches a *missing* entry at compile time (`tsc` fails the build); this runtime test remains valuable in addition because it still catches the case a compile-time check cannot — an entry that exists but points at the wrong class (e.g. accidentally reusing `styles.statusArchived`).
- Files: `web-app/src/components/backlog/BacklogItemBadge.test.tsx`

##### Task 5.4.1b: `BacklogItemDetail` — same all-statuses test for its independent map (~3 min)
- Same assertion shape as 5.4.1a, against `BacklogItemDetail`'s own `STATUS_CLASS`. Same note as 5.4.1a: Task 5.3.2b's retyping adds a compile-time missing-entry check; this test still catches a wrong-class entry, which the type system can't.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 5.4.1c: `page.tsx` table chip — same all-statuses test for its independent map (~3 min)
- Same assertion shape, against `page.tsx`'s `STATUS_CSS`. Same note as 5.4.1a: Task 5.3.3b's retyping adds a compile-time missing-entry check; this test still catches a wrong-class entry, which the type system can't.
- Files: `web-app/src/app/backlog/page.test.tsx`

##### Task 5.4.1d: Filter-chip default-visibility test (~2 min)
- Assert `StatusFilterChips`' default rendered chips exclude both `archived` and `duplicate`, and that `duplicate` still appears in `ALL_STATUSES` (e.g. via a sort-order or "show all" affordance test if one exists).
- Files: `web-app/src/app/backlog/page.test.tsx`

##### Task 5.4.1e: `BacklogItemDetail` — 3-state canonical-link resolution tests (~5 min)
- Three tests: (1) loading state shows "Duplicate of: Loading…" (assert on the substring "Loading", not the bare `…` character) with `aria-live="polite"` before `getBacklogItem` resolves; (2) resolved state shows the clickable link with the canonical title and calls `onNavigateToItem` with the canonical id on click; (3) missing state (`getBacklogItem` mocked to resolve `null`) shows "Duplicate of: (item not found)" with no interactive element.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 5.4.1f: `getActionSpec` unit test for `duplicate` (~1 min)
- Assert `getActionSpec({status: "duplicate", ...})` returns the label `"Duplicate"`, not the raw status string.
- Files: `web-app/src/components/backlog/BacklogItemCard.test.tsx` (or nearest existing test file for this component)

##### Task 5.4.1g: `BacklogItemCard_should_NotInvokeOnAction_When_DuplicateButtonClicked` (~2 min)
- No test file currently exists for `BacklogItemCard` (confirmed by inspection — `getActionSpec`'s `isDone: true` no-op click behavior for `done`/`archived` has no existing test to inherit from, so this is new coverage, not redundant). Render `<BacklogItemCard item={{status: "duplicate", ...}} onAction={mockOnAction} .../>`, fire a click on the action button, and assert `mockOnAction` is never called — matching the existing `if (!actionSpec.disabled && !actionSpec.isDone) onAction(...)` short-circuit at `BacklogItemCard.tsx:114-115`. Closes the gap flagged by both `validation.md` and `design/ux.md` §5 criterion 4 (label alone was tested by Task 5.4.1f; the no-op click behavior was not).
- Files: `web-app/src/components/backlog/BacklogItemCard.test.tsx`

##### Task 5.4.1h: `BacklogItemDetail_should_ResolveArchivedCanonicalItem_When_DuplicateOfIsArchived` (~3 min)
- Mock `getBacklogItem` to resolve `{id: ..., status: "archived", title: "..."}` for the canonical target. Assert the "Duplicate of:" row renders the RESOLVED (clickable-link) state with the canonical title, NOT the MISSING state — per `design/ux.md` §3.4's explicit "canonical archived" edge case ("archived items remain individually fetchable by id; only the list view excludes them by default"). Task 5.4.1e's 3-state test only covers loading/resolved/missing generically with a non-terminal-status canonical item; this test specifically proves an archived canonical still resolves correctly.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 5.4.1i: `BacklogItemDetail_should_ActivateLinkOnEnterKey_When_Focused` (~2 min)
- With the canonical item resolved (RESOLVED state), focus the `data-testid="duplicate-of-link"` button and fire a `keydown` event with `key: "Enter"`. Assert `onNavigateToItem` is called with the canonical item's id — partial automation of `design/ux.md` §5 criterion 12's keyboard-only pass (native `<button>` elements already activate on Enter via the browser's default behavior, but this test proves the wiring end-to-end rather than relying on that default going untested).
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

---

## Phase 6: Backend Test Suite Regression Check

### Epic 6.1: Full suite pass
**Goal**: nothing in Phases 1-4 breaks existing behavior (AC12, full scope).

#### Story 6.1.1: Run and confirm the full Go + Jest suites
**Acceptance Criteria**:
- AC12 (full): existing and new Go/Jest test suites pass — `session/backlog_test.go`, `backlog_service_test.go`, `tools_backlog_test.go`, frontend Jest tests, including all new tests for duplicate transitions, guard, `mark_duplicate`, list exclusion, and badge/link rendering.
  - *Given* all Phase 1-5 code changes are in place, *When* `make build && make test` runs, *Then* it exits 0 with no failing tests.
  - *Given* the same state, *When* `cd web-app && npx jest --no-coverage` runs, *Then* it exits 0 with no failing tests.

**Files**: none (verification-only story)

##### Task 6.1.1a: Run `make build && make test` (~3 min)
- Fix any compile errors from the signature change (Story 2.2.2) surfacing in files not enumerated above (e.g. other test helpers constructing `BacklogItemTransitionInput` literals that now need the new fields, though as additive struct fields with zero-value defaults, positional-literal construction — if any exists — is the only thing that would break; keyed-field literals, which this codebase uses exclusively per the patterns observed above, are unaffected).
- Files: as needed based on build output

##### Task 6.1.1b: Run `make lint` (~2 min)
- Confirm no new lint violations (unused imports, etc.) from the additions across all touched Go files.
- Files: as needed based on lint output

##### Task 6.1.1c: Run `cd web-app && npx jest --no-coverage` (~2 min)
- Confirm the full frontend suite passes, including the new Epic 5.4 tests.
- Files: as needed based on test output

---

## Phase 7: Documentation + Backfill

### Epic 7.1: Triage-Facing Guidance
**Goal**: agents are steered toward `mark_duplicate` instead of archive + free-text note (AC13, guidance portion).

#### Story 7.1.1: Update the triage role-guidance text and tool description
**Acceptance Criteria**:
- AC13 (guidance portion): triage-facing guidance (CLAUDE.md/rules or the MCP tool's own description) directs agents to `mark_duplicate` instead of archive+note.
  - *Given* a triage-role session calls `get_backlog_item`, *When* the role-specific guidance text is returned, *Then* it mentions `mark_duplicate` as the tool to use when the item duplicates existing work, instead of archiving + a free-text note.

**Files**: `server/mcp/tools_backlog.go`

##### Task 7.1.1a: Add a `mark_duplicate` mention to the triage-role guidance block (~2 min)
- In the `case "triage":` block (`server/mcp/tools_backlog.go:128-134`), add a line after the existing workflow steps:
  ```go
  sb.WriteString("If this item describes the same problem as an existing item, call mark_duplicate(item_id, duplicate_of_id, note) instead of suggesting it be archived — this preserves a queryable link to the canonical item.\n")
  ```
- Also add `mark_duplicate` to the `default:` tool-list block (`server/mcp/tools_backlog.go:149-154`):
  ```go
  sb.WriteString("- mark_duplicate — mark an item as a duplicate of another, linking it (role: triage/work)\n")
  ```
- Files: `server/mcp/tools_backlog.go`

(The tool's own `WithDescription(...)` text, already written in Task 3.1.2a, is the second half of this AC — both are already worded to say "use this instead of archiving + a free-text note.")

---

### Epic 7.2: Backfill the 3 Motivating Items
**Goal**: prove the shipped tool works against the real data that motivated this feature (AC13, backfill portion). This is an **operational/data task, not a code change** — no Go/TS files are touched.

#### Story 7.2.1: Mark the two duplicate items against the canonical item
**As the** operator, **I want** the three motivating example items resolved using the shipped `mark_duplicate` tool, **so that** the feature has real adoption proof, not just passing tests.

**Acceptance Criteria**:
- AC13 (backfill portion): the three motivating items (`10128af0-e1eb-47bc-9016-3af8fde83b4d`, `1dc7ff10-326c-4276-a70f-eb8869713593`, short-id `67de6c7b*`) are backfilled — one picked as canonical, the other two marked as its duplicates via the shipped tool.
  - *Given* all three items currently independently describe the `install-service.sh` `.zshrc`-sourcing bug, *When* `get_backlog_item` is called on each of the three (via a session with `STAPLER_SESSION_UUID` set, or the `stapler-squad` MCP tools available to this planning/implementation session), *Then* their `created_at` timestamps and content completeness are compared to pick the canonical one (default rule per requirements: earliest-created/most-complete).
  - *Given* the canonical item is chosen (say `10128af0-e1eb-47bc-9016-3af8fde83b4d`, pending the actual comparison), *When* `mark_duplicate({item_id: "1dc7ff10-326c-4276-a70f-eb8869713593", duplicate_of_id: "10128af0-e1eb-47bc-9016-3af8fde83b4d", note: "duplicate of the install-service.sh .zshrc-sourcing bug, discovered via a separate GitHub-issue-sync import"})` and the equivalent call for the `67de6c7b*` item are made, *Then* both non-canonical items show `status: "duplicate"` and the correct `duplicate_of_id` when re-fetched via `get_backlog_item`.
  - **How to verify**: call `get_backlog_item` on all three ids after the backfill; the two non-canonical ones show `status: "duplicate"` with `duplicate_of_id` pointing at the canonical id, and the canonical item's status is unchanged.
  - *Given* the operator/backfill session's `STAPLER_SESSION_UUID` is linked (via `ItemSession`) only to this backlog item (`4f03de7b-3fca-4f3a-84cb-8c6c5abede50`), not to either of the two non-canonical motivating items, *When* Task 7.2.1a-bis's `AttachSessionToItem` RPC call is made for each non-canonical item before Task 7.2.1b runs, *Then* `GetItemSessionBySessionAndItem(ctx, operatorSessionUUID, nonCanonicalItemID)` succeeds (no longer returns `ErrNotFound`) for both non-canonical items, and the subsequent `mark_duplicate` calls in Task 7.2.1b succeed instead of failing with `ErrPermissionDenied` — proving the backfill is actually executable end-to-end given Task 3.1.1a-bis's authorization check, not just described in prose.

**Files**: none (data operation against the live/dev backlog, performed via the `mark_duplicate` MCP tool once Phase 3 ships)

##### Task 7.2.1a: Compare the three items and pick canonical (~3 min)
- Use `get_backlog_item` (or `mcp__stapler-squad__get_backlog_item`) on all three ids; compare `created_at` and description completeness; default to earliest-created/most-complete per requirements' explicit tie-breaking rule.
- Files: none

##### Task 7.2.1a-bis: Link the operator session to each non-canonical item before calling `mark_duplicate` (~4 min)
- **Why this task exists**: Task 3.1.1a-bis (added in the first repair pass) makes `mark_duplicate` reject any caller session that is not linked to `item_id` via an `ItemSession` record (`GetItemSessionBySessionAndItem`). The session performing this backfill is linked only to the backlog item that tracks this feature itself (`4f03de7b-3fca-4f3a-84cb-8c6c5abede50`) — it has no `ItemSession` row for either non-canonical motivating item (`1dc7ff10-326c-4276-a70f-eb8869713593` or the `67de6c7b*` item). Without establishing that link first, both `mark_duplicate` calls in Task 7.2.1b fail with `ErrPermissionDenied` on first attempt.
- **Confirmed by direct inspection**: no MCP tool in `server/mcp/tools_backlog.go` creates or attaches an `ItemSession` to an arbitrary item (the only 5 registered tools as of this plan are `get_backlog_item`, `report_progress`, `request_review`, `submit_review_verdict`, `submit_triage_result`). The only two places an `ItemSession` gets created are `session/backlog_lifecycle.go` (internal re-triage bookkeeping, not operator-invokable) and the two RPC handlers in `server/services/backlog_service.go`: `SpawnSessionFromItem` (spawns a brand-new tmux session — unnecessary overhead just to establish a link) and `AttachSessionToItem` (links an *existing* session UUID to an item, exactly the primitive needed here). `AttachSessionToItem` (`server/services/backlog_service.go:965-1010`) requires the target item's status to be `idea`, `ready`, or `in_progress` (line 990-996) — both non-canonical motivating items are expected to be in one of these statuses pre-backfill, since they are undecided triage items, not yet `done`/`archived`/`duplicate`.
- **Concrete mechanism**: call the `AttachSessionToItem` RPC directly against the running server (this repo already documents calling RPCs directly for verification/operational steps via `curl`/`grpcurl` against the ConnectRPC endpoint, e.g. `docs/tasks/llm-omnibar.md:285`'s `curl -X POST http://localhost:8543/session.v1.SessionService/ParseIntent ...` pattern). For each of the two non-canonical items:
  ```bash
  curl -s -X POST http://localhost:8543/session.v1.SessionService/AttachSessionToItem \
    -H "Content-Type: application/json" \
    -d '{"item_id": "1dc7ff10-326c-4276-a70f-eb8869713593", "session_uuid": "<the operator session'"'"'s own STAPLER_SESSION_UUID>"}'
  ```
  repeated with the `67de6c7b*` item's full id. This creates an `ItemSession` row linking the operator's own session UUID to each target item — the same `CreateItemSession` call `SpawnSessionFromItem` uses internally, without the overhead of spawning a new tmux session/worktree.
- **Verify the link before proceeding**: call `get_backlog_item` on each non-canonical item (or inspect the `AttachSessionToItem` response's `ItemSession`) to confirm the attach succeeded, then proceed to Task 7.2.1b's `mark_duplicate` calls.
- Files: none (operational RPC call against the live/dev server, not a code change)

##### Task 7.2.1b: Call `mark_duplicate` for the two non-canonical items (~3 min)
- Two `mark_duplicate` calls, each with a `note` explaining the duplicate finding (per the concrete example above). Requires Task 7.2.1a-bis to have already linked the operator session to both non-canonical items — otherwise both calls fail with `ErrPermissionDenied` per Task 3.1.1a-bis's authorization check.
- Files: none

##### Task 7.2.1c: Verify via `get_backlog_item` (~2 min)
- Re-fetch all three ids; confirm the two non-canonical items show `status: "duplicate"` with the correct `duplicate_of_id`, and the canonical item is untouched.
- Files: none

---

## Cross-Cutting Notes

- **Total scope discipline**: this plan implements exactly the 13 numbered acceptance criteria. `UpdateBacklogItem`'s identical TOCTOU gap (Risk Control section) and reverse-lookup (Non-Goals) are explicitly out of scope and are not touched by any task above. (Cutting Task 1.2.1d's `CreateBacklogItem` create-time symmetry work, per the adversarial review, makes this claim *more* true than the prior draft, not less — that task was scope creep with no AC backing it.)
- **Chain-prevention rule** (Non-Goals: "forbid marking an item duplicate-of a target that is itself already duplicate status") is in scope and implemented in Task 1.1.1e / tested in Task 1.1.2d, folded into `ErrDuplicateOfInvalidTarget` per ADR-002 to keep the sentinel count at exactly three per AC6's literal wording.
- **Reopen data-hygiene fix (Task 2.2.3c)**: clearing `duplicate_of_id` on `duplicate → idea` is a defensive data-hygiene addition made *beyond* the literal wording of AC2 and AC7 — AC2 only asserts `validTransitions` boolean correctness for the reopen edge (it never mentions `duplicate_of_id`), and AC7's own Given-When-Then examples only exercise the write when transitioning INTO `duplicate`, not clearing on reopen. It is not "required by" either AC in the strict sense. It is still worth doing and low-risk: without it, a reopened item would silently retain a stale, non-empty `DuplicateOfID` alongside `Status: idea` — an illegal-in-spirit combination the original draft of this plan left reachable. Called out explicitly here so the claim of "exactly the 13 ACs, no more" (see Total scope discipline above) stays honest about the one place this plan does a little more than the letter of the ACs strictly demands.
- **Session-item authorization (Task 3.1.1a-bis)**: closes a blocker-level gap identified by adversarial review — `mark_duplicate` now enforces the same "caller session must be linked to `item_id`" check every other backlog-mutating MCP tool in this file already enforces, matching AC8/FR5's requirement that it follow "the existing handler pattern." Not new scope; a correctness requirement AC8 already implied.
- **VCS**: plain git in this worktree. Standard PR workflow (branch → commits → PR → review → merge) applies to every phase above; no jj-specific commands appear anywhere in this plan.
