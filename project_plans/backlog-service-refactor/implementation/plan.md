# Implementation Plan: backlog-service-refactor

**Feature**: Decompose the backlog subsystem into focused, independently testable units
**Date**: 2026-07-09
**Status**: Ready for implementation
**ADRs**: ADR-010 (session/domain sub-package), ADR-011 (Storage interface domain DTOs)

---

## Step 0.5 — Creative Pass: Approaches Considered

Three high-level sequencing strategies were evaluated before selecting one:

| Approach | Strength | Weakness | Decision |
|---|---|---|---|
| **A: Big-Bang Sequenced PR** — all 6 items in one PR | No integration drift between PRs | Massive review surface; single CI break reverts everything | Rejected |
| **B: Dependency-Ordered Incremental PRs** — each item ships as its own green PR | Each PR is reviewable, CI validates preservation at each step | More PR overhead; rebasing needed if base drifts | **CHOSEN** |
| **C: Domain-First Inversion** — P5/P6 (domain layer) before P1/P2 (service layer) | Cleanest DDD sequence | P5 carries highest import-cycle risk; blocks everything if it fails | Rejected |

Approach B is chosen: guard-rail lint first, then P2 (smallest win), then P1+P3 in parallel, then P6, then P4, then P5.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `BacklogItem` | A unit of planned work tracked through a lifecycle from Idea → Done | Root aggregate |
| `BacklogStatus` | The lifecycle state of a `BacklogItem` (8 values: idea, refining, ready, in_progress, review, pr_pending, done, archived) | Lives in `session/backlog.go`; candidate for `session/domain` in P5 |
| `AcCriterion` | A single acceptance criterion with an index, text, and `AcStatus` | Lives in `session/backlog.go` |
| `AcCriteriaJSON` | Named string type for the JSON-serialized `[]AcCriterion` form stored in DB | Prevents accidentally passing description strings where AC JSON is expected |
| `AcStatus` | Status of one AC criterion: pending, in_progress, done, fail | Lives in `session/backlog.go` |
| `ReviewOutcome` | The verdict of a review gate run: PASS, FAIL, PARTIAL | Lives in `session/backlog.go` |
| `CriterionVerdict` | The per-criterion verdict from a review gate run | Lives in `session/backlog.go` |
| `AggregateOutcome` | Rollup of all `CriterionVerdict`s into a single `ReviewOutcome` | Computed in `session/backlog.go` |
| `ItemSession` | A DB record joining a `BacklogItem` to a `session.Instance`; tracks role (work/triage/review), start/end time, and verdict | `*ent.ItemSession` currently leaks through Storage; P6 introduces `ItemSessionSummary` |
| `ItemSessionSummary` | Domain DTO replacing `*ent.ItemSession` in Storage returns and `BacklogItemData.ItemSessions` | Introduced in P6 |
| `ReviewVerdictData` | Domain DTO replacing `*ent.ReviewVerdict` in Storage returns | May already exist; confirm in P6 |
| `SourceSyncEventData` | Domain DTO replacing `*ent.SourceSyncEvent` in `ListSourceSyncEvents` | Introduced in P6 |
| `BacklogStatusEventData` | Domain DTO replacing `*ent.BacklogStatusEvent` in `BacklogItemData.StatusEvents` | Introduced in P6 |
| `BacklogItemData` | Domain DTO for a single backlog item with all relations; returned by `GetBacklogItem` | 22 fields; `ItemSessions` and `StatusEvents` are the ent-leaking fields |
| `BacklogItemSummary` | Narrower DTO for list-view queries; includes scalar item fields plus `ItemSessions []ItemSessionSummary` carrying minimal session data (role, status, triage summary, overall outcome) for board view rendering | Introduced in P4; `ItemSessions` must carry enough data to reconstruct `triageStatus`, `gateVerdict`, and `linkedSessions` in the board view hook |
| `ReviewGateRunner` | A new value type that encapsulates `spawnReviewGate` logic: storage + headless pool + shutdown context + autoReopener callback + sessionCreator callback | Introduced in P3; `BacklogLifecycleListener` holds one as a field; `autoReopener` triggers auto-reopen on FAIL/PARTIAL outcomes; `sessionCreator` handles the legacy tmux review path |
| `MergeAcCriteria` | Pure function: merges incoming `[]AcCriterion` into existing by index, never removes unmentioned criteria | Extracted in P2 from inline logic in `submitTriageResult` |
| `ErrNotFound` | Domain sentinel error (`session.ErrNotFound`); the only error check allowed above the `Storage` layer | Already defined in `session/repository.go`; P6 ensures consistent wrapping |
| `session/domain` | Proposed sub-package for pure leaf types (no ent, headless, or git imports); created in P5 | Type aliases in `session/` preserve backward compat for 65+ importers |
| `depguard no_ent_in_services` | Lint rule preventing `server/services` from importing `session/ent` | Added in Phase 1 (guard rail) |
| `forbidigo ent.IsNotFound` | Lint rule banning `ent.IsNotFound` calls in `server/services` | Added in Phase 1 (guard rail) |
| `goda reach` | Tool to pre-flight check import graph before creating `session/domain` | Run before Phase 7 |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| P1: backlog_service.go split | File split within `package services` — all in same Go package | `session_service_shells.go` precedent | Package split (new sub-package under `server/services/`) | ConnectRPC handler registration wires the struct, not files; no other service uses a sub-package; test helpers are in `package services` and would need duplication |
| P2: mergeAcCriteria location | Move to `session/backlog.go` as exported `MergeAcCriteria` | Types `AcCriterion`/`AcCriteriaJSON` already live there | Keep in `server/mcp/tools_backlog.go` | Placing it in `session/` makes it reusable from any AC-editing surface without pulling in `server/mcp`; MCP handler becomes a thin parse→call→serialize wrapper |
| P3: ReviewGateRunner shape | Standalone struct with 5 constructor args (`storage`, `pool`, `shutdownCtx`, `autoReopener`, `sessionCreator`) | Architecture research; `interface-pollution-checklist.md` | Option struct; embedded fields on BacklogLifecycleListener | 5-field direct constructor is clear for a fixed dependency set; `BacklogLifecycleListener` holds `*ReviewGateRunner` as a field; `autoReopener` and `sessionCreator` are typed func args so `BacklogLifecycleListener` methods can be wired in without circular field references; `pushAndCreatePR` passed as a callback arg to `Run` |
| P4: BacklogItemSummary design | Separate Go struct + `ListBacklogItemSummaries` method using three-phase query: ent `.Select().Scan()` for scalar fields + separate `WHERE backlog_item_id IN (<page IDs>)` query on `item_sessions` (selecting `id, session_role, ended_at, triage_result`) + third query on `review_verdicts` for `overall_outcome`; result includes `ItemSessions []ItemSessionSummary` | UX research (board view risk; `mapBacklogItem` audit), build-vs-buy | Omit ItemSessions entirely; two booleans only; widening Select() to `*ent.BacklogItem` | `ItemSessions []ItemSessionSummary` carries `Role`, `EndedAt`, `TriageResultSummary` (parsed from `triage_result` JSON), `OverallOutcome` (from `review_verdicts`) — the fields `mapBacklogItem` in `useBacklogService.ts` actually reads to derive `triageStatus`, `gateVerdict`, and `linkedSessions`; two booleans cannot reconstruct these derivations; `item_sessions` has no `status`, `triage_result_summary`, or `overall_outcome` columns |
| P4: ent projection | `ent BacklogItemQuery.Select(fields...).Scan(ctx, &out)` — ent built-in | stack.md confirmed `BacklogItemSelect` exists | Raw SQL scan; mapping layer over full hydration | ent's built-in Select/Scan is the documented approach; no new library needed |
| P5: session/domain approach | Create `session/domain` with pure types + type aliases in `session/` for backward compat | build-vs-buy.md; architecture Option B reasoning | Keep types in `session/` root (architecture Option A) | Requirements explicitly mandate the sub-package; alias bridge means 65+ importers need zero changes; P5 is optional until a second use case makes the aliases clearly suboptimal |
| P5: sequencing guard | Run `goda reach` pre-flight before cutting `session/domain` | pitfalls.md (HIGH import-cycle risk) | Skip pre-flight, rely on `go build` | `goda reach` reveals cycles before you commit the import statement; `go build` only fails after you've already written the broken import |
| P6: lint guard timing | Add `forbidigo` + `depguard` rules BEFORE implementing cleanups | build-vs-buy.md Top Recommendations #1 | Add lint rules after cleanup | Guard rails must land first so regressions in other PRs become CI failures immediately |
| P6: error wrapping | `ent_repository_backlog.go` wraps ALL ent errors → `session.ErrNotFound`; callers use `errors.Is(err, session.ErrNotFound)` | pitfalls.md; architecture research §5 | Dual-check (`ent.IsNotFound || errors.Is`) at every call site | Dual-check is inconsistent (7 of 14 sites already drop the fallback); consistent wrapping at the boundary is the only maintainable strategy |
| Global sequencing | Lint guard → P2 → P1+P3 (parallel PRs) → P6 → P4 → P5 | Task sequencing notes | Domain-first (P5/P6 before P1) | P5 is highest-risk; P1 is highest-churn (ship early to reduce conflict surface) but P6 must land before P4 |

---

## Observability Plan

This is a pure structural refactoring — no new metrics, no new log lines, and no behavior changes. Existing observability is preserved:

- All `log.With(ctx).Info(...)` call sites move with their containing functions and are not renamed.
- The file split (P1) keeps all functions on `*BacklogService`, so any existing tracing or metrics middleware that wraps the ConnectRPC handlers continues to work without change.
- `ReviewGateRunner.Run` (P3) should carry through the `context.Context` from the caller — do not store a derived context on the runner struct itself.
- No new Prometheus metrics are warranted; structural complexity metrics (cognitive complexity, file line count) are measured via `make analyze` and checked before/after each phase.

---

## Risk Control

| Risk | Severity | Mitigation |
|---|---|---|
| Import cycle when creating `session/domain` | HIGH | Run `goda reach` pre-flight; create package with types only (no session import); use alias bridge for backward compat; `go build ./...` immediately after each file change |
| `ent.IsNotFound` leakage persisting after P6 | HIGH | `forbidigo` lint rule blocks new occurrences; CI must be green before P4 ships |
| Board view silently broken by P4 (missing session data) | MEDIUM | `BacklogItemSummary` MUST include `ItemSessions []ItemSessionSummary` with `Role`, `EndedAt`, `TriageResultSummary` (parsed from `triage_result` JSON), and `OverallOutcome` (from `review_verdicts` JOIN); Story 5.1.3 maps these to the EXISTING proto `repeated ItemSession item_sessions` field (partially populated) — no frontend change needed since `mapBacklogItem` in `useBacklogService.ts` already reads `item_sessions`; add a targeted test asserting list response includes correct session data for an item with a live triage session |
| P1 mid-PR build break (partial function move) | MEDIUM | Move one semantic group per commit; run `make build` after each group; keep all commits green before pushing |
| White-box test breakage (unexported symbols) | MEDIUM | All test files stay in `package services`; unexported symbols that tests call (`itemSessionToProto`, `itemSourceBackend`) stay in `package services` files; never export them as a workaround |
| `ReviewGateRunner` re-entrancy (semaphore ownership) | LOW | `reviewSem` stays on `BacklogLifecycleListener`; `ReviewGateRunner.Run` is stateless per call except for the 5 constructor fields |

---

## Unresolved Questions

1. **RESOLVED**: `ent BacklogItemSelect.Scan()` does not support scanning ItemSessions edge fields in a single query. The three-phase approach is used: (1) scalar field scan via `Select().Scan()`, (2) separate `WHERE backlog_item_id IN (<page IDs>)` query on `item_sessions` selecting `id, backlog_item_id, session_role, ended_at, triage_result` — note: `item_sessions` has NO `status`, `triage_result_summary`, or `overall_outcome` columns; `TriageResultSummary` is populated by parsing `triage_result` JSON in Go; running status is derived from `ended_at IS NULL`; (3) `WHERE item_session_id IN (<session IDs>)` query on `review_verdicts` selecting `item_session_id, overall_outcome` to populate `ItemSessionSummary.OverallOutcome`. Results are grouped and assigned to `BacklogItemSummary.ItemSessions []ItemSessionSummary`. `HasLinkedSession`/`TriageRunning` booleans were rejected — they cannot reconstruct the derived fields `mapBacklogItem` actually reads.

2. **Does `*ent.ReviewVerdict` already have a domain DTO (`ReviewVerdictData`)?** Architecture research says it might. Confirm in `session/repository.go` and `session/ent_repository_backlog.go` before designing P6. **Resolve at P6 start.**

3. **Which of the 65+ `session` importers import it for domain types only (no Storage/Instance)?** Research identified `pkg/events` and `server/adapters`. A full audit with `goda` or grep may surface 1–2 more. These are the only ones that need to update imports after P5. **Resolve at P5 start.**

---

## Dependency Visualization

```
Phase 1 (Lint Guard)
    |
    +-- no deps -- ship independently as PR-0
    |
Phase 2 (P2: MergeAcCriteria)
    |
    +-- no deps -- ship independently as PR-1
    |
    +----------------------------+
    |                            |
Phase 3 (P1: File Split)   Phase 3b (P3: ReviewGateRunner)
    |                            |
    +----------------------------+
                 |
          Phase 4 (P6: Storage Interface Cleanup)
                 |
          Phase 5 (P4: BacklogItemSummary)
                 |
          Phase 6 (P5: session/domain)
```

P3 (ReviewGateRunner) and P1 (file split) are independent and can be submitted as parallel PRs in Phase 3.
P5 (session/domain) MAY be shipped before P4 as a pure type move — but P6 must land first because the alias bridge in session/ needs `ItemSessionSummary` to be clean.

---

## Phase 1: Infrastructure Guards

### Epic 1.1: Lint Guard for ent Leakage

**Goal**: Add `forbidigo` and `depguard` rules to `.golangci.yml` that enforce the ent boundary before a single refactor line is written. This turns future regressions into immediate CI failures.

**PR label**: `chore` | **Branch**: `refactor/lint-guard-ent-boundary`

#### Story 1.1.1: Add `forbidigo` rule banning `ent.IsNotFound` in `server/services`

**Acceptance criteria**:
1. `.golangci.yml` contains a `forbidigo` entry matching `ent\.IsNotFound` with message directing devs to `errors.Is(err, session.ErrNotFound)`
2. The exclusion list covers `session/ent_repository_backlog.go` and `session/storage_backlog.go` (the two files allowed to use `ent.IsNotFound` for wrapping)
3. An `issues.exclude-rules` entry is added to `.golangci.yml` scoped to `server/services/backlog_service.*\.go` for the `forbidigo` linter, temporarily suppressing the 14 pre-existing `ent.IsNotFound` violations so CI stays green; the pattern `backlog_service.*\.go` covers the base file AND all files created in the Phase 3 split (`backlog_service_query.go`, `backlog_service_lifecycle.go`, `backlog_service_triage.go`) so Phase 3 PRs do not fail lint CI; this exclusion is annotated with a comment `# removed-in: refactor/storage-interface-cleanup (P6)` and is removed in the P6 PR
4. `make lint` with the new rule + exclusion reports 0 new violations (pre-existing ones are suppressed by the exclusion entry); the PR description documents that running without the exclusion would show ~14 hits in `backlog_service.go`
5. `make ci` green end-to-end

**Given-When-Then**:
- Given: `backlog_service.go` line 889 contains `if ent.IsNotFound(err) {`
- When: `make lint` runs with the new rule and the temporary `exclude-rules` entry in place
- Then: golangci-lint exits 0; the violation is suppressed by the exclusion entry; removing the exclusion entry causes golangci-lint to report `server/services/backlog_service.go:889: use errors.Is(err, session.ErrNotFound) in server/services`

**Tasks**:
1. Open `.golangci.yml`; locate `linters-settings.forbidigo.forbid` list
2. Add entry: `pattern: 'ent\.IsNotFound'`, `msg: "use errors.Is(err, session.ErrNotFound) in server/services — wrap ent errors at the storage layer"`
3. Add `issues.exclude-rules` entry: linter `forbidigo`, path `server/services/backlog_service.*\.go`, text `ent\.IsNotFound` — annotate with `# removed-in: refactor/storage-interface-cleanup (P6)`; using the glob `backlog_service.*\.go` (not just `backlog_service\.go`) ensures Phase 3 split files are also covered and do not fail lint CI
4. Add exclusion for storage files (`session/ent_repository_backlog.go`, `session/storage_backlog.go`) so the wrapping sites don't trip the rule
5. Run `make lint`; confirm 0 new violations; document pre-existing count (~14) in PR description
6. Run `make ci` to confirm overall pipeline stays green

---

#### Story 1.1.2: Add `depguard` skeleton rule for `no_ent_in_services`

**Acceptance criteria**:
1. `.golangci.yml` contains a `depguard` rule `no_ent_in_services` with `deny: [pkg: "github.com/tstapler/stapler-squad/session/ent"]` scoped to `**/server/services/**/*.go`
2. Rule uses a `files:` exclusion pattern covering ALL `server/services/*.go` files that currently import `session/ent` (not only `backlog_service*.go`) — confirmed by `grep -rl "session/ent" server/services/` before writing the exclusion; known files as of planning: `backlog_service*.go`, `analytics_escape_service.go`, `session_service.go`, `workflow_service.go`, `error_registry.go`; annotated with `# removed-in: <per-file cleanup PR>`; note: depguard uses `files:` not `allowed-modules:` for per-file exceptions
3. The rule itself is annotated with a comment: `# activated-after: all server/services exclusions removed`

**Triad fix (2026-07-09)**: The original exclusion covered only `backlog_service*.go`. Triad engineering review confirmed 4 other files in `server/services/` also import `session/ent` — those would immediately fail CI if not excluded. Run `grep -rl "session/ent" server/services/` as the first task and enumerate ALL files, not just the backlog ones.

**Given-When-Then**:
- Given: `server/services/backlog_service.go` imports `session/ent` on line 21, AND `session_service.go`, `analytics_escape_service.go`, `workflow_service.go`, `error_registry.go` also import `session/ent`
- When: `make lint` runs after the rule is added with exclusions for all current importers
- Then: `make lint` exits 0; adding a new `session/ent` import to any non-excluded file causes golangci-lint to flag it

**Tasks**:
1. Run `grep -rl "session/ent" server/services/` — record the full list of files; this is the exclusion list
2. Open `.golangci.yml`; locate `depguard.rules`
3. Add `no_ent_in_services` rule (deny + files pattern) with `files:` exclusion for ALL files from step 1
4. Annotate each exclusion entry with `# removed-in: <cleanup PR>` noting which refactor phase will remove it
5. `make ci` green

---

## Phase 2: Quick Wins

### Epic 2.1: Extract `MergeAcCriteria`

**Goal**: Extract the inline AC merge logic from `server/mcp/tools_backlog.go::submitTriageResult` into a named, tested pure function in `session/backlog.go`.

**PR label**: `refactor` | **Branch**: `refactor/extract-merge-ac-criteria`

#### Story 2.1.1: Define `MergeAcCriteria` in `session/backlog.go`

**Acceptance criteria**:
1. `session/backlog.go` contains `func MergeAcCriteria(existing []AcCriterion, incoming []AcCriterion) (AcCriteriaJSON, error)`
2. The function never removes criteria not mentioned in `incoming`; it updates matching indices and appends new ones
3. The function returns an error if `incoming` contains duplicate indices
4. The result is a valid `AcCriteriaJSON` (round-trips through `ParseAcCriteria` without loss)
5. `submitTriageResult` in `server/mcp/tools_backlog.go` calls `session.MergeAcCriteria` and its inline ~50-line merge block is removed

**Given-When-Then**:
- Given: `existing = [{Index:0, Text:"Write unit tests", Status:"pending"}]` and `incoming = [{Index:0, Text:"Write unit tests", Status:"done"}, {Index:1, Text:"Update docs", Status:"pending"}]`
- When: `MergeAcCriteria(existing, incoming)` is called
- Then: the result is `AcCriteriaJSON` that parses to `[{Index:0, Text:"Write unit tests", Status:"done"}, {Index:1, Text:"Update docs", Status:"pending"}]` — one updated, one added

**Tasks**:
1. Read `server/mcp/tools_backlog.go` lines ~533–584 to understand the exact merge semantics
2. Write `MergeAcCriteria` in `session/backlog.go` (pure function, no imports beyond `encoding/json`)
3. Replace the inline block in `submitTriageResult` with `session.MergeAcCriteria(existingCriteria, incomingCriteria)`
4. `make build` green

---

#### Story 2.1.2: Table-driven tests for `MergeAcCriteria`

**Acceptance criteria**:
1. `session/backlog_test.go` (new file if it doesn't exist) contains `TestMergeAcCriteria` with at least 5 cases:
   - `empty_existing_non_empty_incoming` — existing nil, incoming has 2 items → result has 2 items
   - `update_existing_criterion` — index 0 status pending → done
   - `preserve_unmentioned_criteria` — existing has indices 0,1,2; incoming only mentions index 1 → indices 0 and 2 are preserved unchanged
   - `append_new_criterion_with_gap_in_indices` — existing has index 0; incoming has index 5 → result has indices 0 and 5 (gap preserved, no renumbering)
   - `duplicate_index_in_incoming_returns_error` — incoming has two entries with index 0 → error returned
2. `go test ./session/... -run TestMergeAcCriteria` passes
3. Fuzz test `FuzzMergeAcCriteria` added with 1 seed corpus entry; runs cleanly for 5 seconds: `go test ./session/... -run=^$ -fuzz=FuzzMergeAcCriteria -fuzztime=5s`

**Given-When-Then**:
- Given: `existing = []` and `incoming = [{Index:3, Text:"AC A"}, {Index:7, Text:"AC B"}]`
- When: `MergeAcCriteria(existing, incoming)` is called
- Then: result parses to `[{Index:3, Text:"AC A"}, {Index:7, Text:"AC B"}]` in stable (sorted-by-index) order

**Tasks**:
1. Create or open `session/backlog_test.go`
2. Write the 5 table cases; use `testify/require` for assertions
3. Add the fuzz function with seed
4. `make test` green

---

## Phase 3: Service File Split and ReviewGateRunner

These two stories may be worked in parallel by different agents/branches; they do not share files.

### Epic 3.1: Split `backlog_service.go`

**Goal**: Reduce `backlog_service.go` from 2,570 lines to ~550 lines by extracting query, lifecycle, and triage groups into companion files. All stay in `package services`.

**PR label**: `refactor` | **Branch**: `refactor/split-backlog-service`

#### Story 3.1.1: Create `backlog_service_query.go` and move query handlers

**Acceptance criteria**:
1. `server/services/backlog_service_query.go` exists with `package services` and a file-top comment naming the RPC group
2. The following methods are defined in `backlog_service_query.go` (not in `backlog_service.go`): `GetBacklogItem`, `ListBacklogItems`, `ListItemSources`, `SuggestNextItem`, `GetSyncHistory`, `SearchGitHubRepos`, `ListGitHubIssues`, `GetBacklogItemDiff`, `GetBacklogItemCost`, `GetSessionBacklogIndex`
3. Private helpers used exclusively by query handlers (`backlogItemToProto`, `buildCostLookup`, `itemSessionToProto`, `approvalRuleToProto`, `statusEventToProto`) move with them
4. `make build` green after the move
5. `make test` green
6. `backlog_service.go` line count drops to approximately 1,850 (query group removed)

**Given-When-Then**:
- Given: `backlog_service.go` contains `func (s *BacklogService) GetBacklogItem(...)` at approximately line 667
- When: `GetBacklogItem` is moved to `backlog_service_query.go`
- Then: `grep -c "GetBacklogItem" server/services/backlog_service.go` returns 0; `grep -c "GetBacklogItem" server/services/backlog_service_query.go` returns 1; `make build` passes

**Tasks**:
1. Create `server/services/backlog_service_query.go` with `package services` header and doc comment
2. Move the 10 query method bodies (lines ~667–2530) and their private helpers
3. Verify no symbol is defined twice by running `make build`
4. Run `make quick-check`

---

#### Story 3.1.2: Create `backlog_service_lifecycle.go` and move lifecycle handlers

**Acceptance criteria**:
1. `server/services/backlog_service_lifecycle.go` exists with `package services`
2. The following methods are defined there: `CreateBacklogItem`, `UpdateBacklogItem`, `ArchiveBacklogItem`, `DeleteBacklogItem`, `TransitionBacklogItemStatus`, `ApprovePlan`, `CreateItemSource`, `UpdateItemSource`, `DeleteItemSource`, `OverrideVerdict`
3. `backlog_service.go` retains: struct definition, `NewBacklogService`, all `Set*` setter methods, shared helpers (`slugify`, `triageShortTitle`, `acCriteriaToJSON`, `resolveRepoPathInput`, `encryptAndMergeToken`, `buildProtoHeaders`), and all type/interface declarations
4. `make build` and `make test` green
5. `backlog_service.go` line count drops to ~550

**Given-When-Then**:
- Given: `CreateBacklogItem` at approximately line 598 of `backlog_service.go`
- When: moved to `backlog_service_lifecycle.go`
- Then: `go build ./server/services` passes in < 5 seconds; `go test ./server/services` passes

**Tasks**:
1. Create `server/services/backlog_service_lifecycle.go`
2. Move the 10 lifecycle method bodies
3. Move private helpers scoped to lifecycle operations
4. Run `make build` after each group of 3–4 method moves
5. Final `make quick-check`

---

#### Story 3.1.3: Create `backlog_service_triage.go` and move triage/sync handlers

**Acceptance criteria**:
1. `server/services/backlog_service_triage.go` exists with `package services`
2. The following methods are defined there: `SpawnSessionFromItem`, `AutoReopenAfterFailedReview`, `AutoReopenForPRFix`, `AttachSessionToItem`, `TriggerTriage`, `CancelTriage`, `TriggerReReview`, `TriggerSync`, `ImportGitHubIssue`
3. Private helpers scoped to triage operations (`headlessTriageUUIDPrefix`, `maxAutoReworkIterations`, `defaultTriageCleanupTimeout`, `defaultTriggerSyncTimeout`, `maxTriageSessionAge` constants) move into this file
4. `make ci` green end-to-end
5. Combined line count across all four files: ≤ 2,600 (no duplication)
6. Each file ≤ 900 lines

**Given-When-Then**:
- Given: `TriggerTriage` at approximately line 1662 of `backlog_service.go`
- When: moved to `backlog_service_triage.go`
- Then: `wc -l server/services/backlog_service.go` outputs ≤ 560; `make ci` passes

**Tasks**:
1. Create `server/services/backlog_service_triage.go`
2. Move constants block first (easier to verify), then method bodies
3. Run `make build` after constants + first 2 methods; confirm green
4. Move remaining methods; final `make ci`

---

### Epic 3.2: Extract `ReviewGateRunner`

**Goal**: Extract `spawnReviewGate` from `session/backlog_lifecycle.go` into a standalone `ReviewGateRunner` type in `session/review_gate.go`, making it independently constructable and testable.

**PR label**: `refactor` | **Branch**: `refactor/extract-review-gate-runner`

#### Story 3.2.1: Define `ReviewGateRunner` in `session/review_gate.go`

**Acceptance criteria**:
1. `session/review_gate.go` exists with `package session`
2. `ReviewGateRunner` is a struct with exactly 5 fields:
   ```go
   type ReviewGateRunner struct {
       storage        Storage
       pool           *headless.Pool
       shutdownCtx    context.Context
       autoReopener   func(ctx context.Context, itemID uuid.UUID) error
       sessionCreator func(ctx context.Context, item *ent.BacklogItem, is *ent.ItemSession) error
   }
   ```
3. `NewReviewGateRunner(storage Storage, pool *headless.Pool, shutdownCtx context.Context, autoReopener func(ctx context.Context, itemID uuid.UUID) error, sessionCreator func(ctx context.Context, item *ent.BacklogItem, is *ent.ItemSession) error) *ReviewGateRunner` constructor exists
4. `(r *ReviewGateRunner) Run(ctx context.Context, item *ent.BacklogItem, is *ent.ItemSession, onPass func(ctx context.Context, item *ent.BacklogItem, is *ent.ItemSession) error)` method contains the body of the current `spawnReviewGate`, using `r.autoReopener` on FAIL/PARTIAL outcomes (lines ~498–505 of the original) and `r.sessionCreator` for the legacy tmux review path (lines ~517–540 of the original)
5. `BacklogLifecycleListener` holds a `runner *ReviewGateRunner` field, initialized in its constructor with `l.getAutoReopener()` and `l.sessionCreator` wired as the two function args
6. `BacklogLifecycleListener.spawnReviewGate` becomes a one-liner: `l.runner.Run(ctx, item, is, l.pushAndCreatePR)`
7. `session/backlog_lifecycle.go` cognitive complexity for `spawnReviewGate` is reduced (function extracted into a testable type); confirm with `gocognit ./session/backlog_lifecycle.go`
8. `make build` and `make test` green; existing `TestBacklogIntegration_IT002` and `IT006` pass

**Given-When-Then**:
- Given: a `ReviewGateRunner` is constructed with a real `Storage`, a `fakeHeadlessPool` configured to return `ReviewOutcome = FAIL` for all criteria, a cancel-ready `shutdownCtx`, a `noopAutoReopener` that records its invocations, and a nil `sessionCreator` (legacy path not exercised in this test)
- When: `runner.Run(ctx, item, is, noopOnPass)` is called with a backlog item that has `SkipReviewGate: false` and one acceptance criterion
- Then: `autoReopener` is called exactly once with the item's ID; `onPass` is NOT called; ItemSession is updated with a non-nil `VerdictAt` and a FAIL outcome

**Tasks**:
1. Read `session/backlog_lifecycle.go` lines 379–end to map all `l.` field accesses in `spawnReviewGate` — confirm `l.getAutoReopener()` and `l.sessionCreator` are used on FAIL/PARTIAL and legacy paths respectively
2. Create `session/review_gate.go` with struct definition (5 fields), constructor, and empty `Run` stub
3. Copy `spawnReviewGate` body into `Run`; replace all `l.` accesses with `r.` field references or callback args (`r.autoReopener`, `r.sessionCreator`, `r.pool`, `r.storage`, `r.shutdownCtx`)
4. Add `runner` field to `BacklogLifecycleListener`; initialize in constructor passing `l.getAutoReopener()` and `l.sessionCreator`
5. Slim `spawnReviewGate` to one-liner delegation
6. `make build`; fix any missing field references
7. `make test -run TestBacklogIntegration`

---

#### Story 3.2.2: Unit test for `ReviewGateRunner`

**Acceptance criteria**:
1. `session/review_gate_test.go` exists with at least 2 test cases:
   - `TestReviewGateRunner_SkipReviewGate`: item has `SkipReviewGate: true`; `Run` marks session ended without calling pool; `onPass` is NOT called
   - `TestReviewGateRunner_HeadlessPassPath`: item has `SkipReviewGate: false` with a `fakePool` returning PASS; `onPass` IS called; ItemSession verdict is persisted
2. Tests use fake storage (`createTestStorage` pattern from `session_service_test.go`) — no mock library
3. `go test ./session/... -run TestReviewGateRunner` passes

**Given-When-Then**:
- Given: a `ReviewGateRunner` with a `fakeHeadlessPool` configured to return `ReviewOutcome = PASS` for all criteria
- When: `Run` is called with a backlog item that has 2 acceptance criteria, `SkipReviewGate: false`
- Then: `onPass` callback is invoked exactly once; the ItemSession in storage has a non-nil `VerdictAt` timestamp; no error is returned

**Tasks**:
1. Create `session/review_gate_test.go`
2. Define `fakeHeadlessPool` that returns configurable `ReviewOutcome`
3. Write the 2 test cases using `testify/require`
4. `go test ./session/... -run TestReviewGateRunner -v`

---

## Phase 4: Storage Interface Cleanup

### Epic 4.1: Introduce Domain DTOs for `ItemSession` and Related Types

**Goal**: Replace `*ent.ItemSession`, `*ent.ReviewVerdict`, `*ent.SourceSyncEvent`, and `*ent.BacklogStatusEvent` return types in `session.Storage` with domain structs, breaking the ent-leakage chain.

**PR label**: `refactor` | **Branch**: `refactor/storage-interface-cleanup`
**Depends on**: Phase 3 PRs merged (P1 file split must be done so that the ent imports in backlog_service.go are isolated before being removed)

#### Story 4.1.1: Define domain DTOs in `session/repository.go`

**Acceptance criteria**:
1. `session/repository.go` contains:
   - `type ItemSessionSummary struct { ID string; BacklogItemID string; SessionUUID string; Role string; AcSnapshot AcCriteriaJSON; LastCommitSha string; StartedAt time.Time; EndedAt *time.Time; TriageResultSummary string; OverallOutcome string }`
   - Schema note: `item_sessions` has NO `status` column — running status is derived from `EndedAt == nil` at the caller/conversion layer; `Status string` is therefore NOT a field on `ItemSessionSummary`
   - Schema note: `TriageResultSummary` is NOT a direct DB column; the actual column is `triage_result` (full JSON blob); `itemSessionToSummary` (Story 4.1.2) must unmarshal `triage_result` JSON and extract the summary field to populate `TriageResultSummary`
   - Schema note: `OverallOutcome` does NOT live in `item_sessions`; it lives in the `review_verdicts` table (linked via `item_session_id`); the conversion code must either JOIN `review_verdicts` or issue a second query keyed on `item_session_id` to populate this field
   - `type BacklogStatusEventData struct { ID string; Status BacklogStatus; TriggeredBy string; CreatedAt time.Time }`
   - `type SourceSyncEventData struct { ID string; ItemID string; EventType string; Summary string; CreatedAt time.Time }`
2. If `ReviewVerdictData` already exists, it is unchanged; if not, it is added with fields that mirror `*ent.ReviewVerdict`
3. No ent imports are added to `session/repository.go` (it already imports `session/ent` for `*ent.Shell` — this is acceptable for now; the new structs must NOT reference ent types)
4. `make build` green

**Given-When-Then**:
- Given: `BacklogItemData` currently has field `ItemSessions []*ent.ItemSession`
- When: field type is changed to `ItemSessions []ItemSessionSummary`
- Then: `grep "ent.ItemSession" session/repository.go` returns 0 hits

**Tasks**:
1. Open `session/repository.go`; locate `BacklogItemData` struct
2. Add `ItemSessionSummary`, `BacklogStatusEventData`, `SourceSyncEventData` type definitions; `ItemSessionSummary` must NOT have a `Status` field (no DB column); include `EndedAt *time.Time` (exists in `item_sessions`) and `TriageResultSummary string` (populated by parsing `triage_result` JSON in `itemSessionToSummary` in Story 4.1.2) and `OverallOutcome string` (populated via a `review_verdicts` query/JOIN in Story 4.1.2)
3. Change `BacklogItemData.ItemSessions` from `[]*ent.ItemSession` to `[]ItemSessionSummary`
4. Change `BacklogItemData.StatusEvents` from `[]*ent.BacklogStatusEvent` to `[]BacklogStatusEventData`
5. `make build` to surface all call-site breakage; record the list

---

#### Story 4.1.2: Add conversion functions in `session/ent_repository_backlog.go`

**Acceptance criteria**:
1. `session/ent_repository_backlog.go` contains `func itemSessionToSummary(is *ent.ItemSession) ItemSessionSummary` (following the pattern of existing `backlogItemToData`)
2. `func backlogStatusEventToData(e *ent.BacklogStatusEvent) BacklogStatusEventData` is added
3. `func sourceSyncEventToData(e *ent.SourceSyncEvent) SourceSyncEventData` is added
4. `backlogItemToData` is updated to use `itemSessionToSummary` for the `ItemSessions` field
5. All methods that currently return `*ent.ItemSession` (e.g., `GetItemSession`, `CreateItemSession`, `ListItemSessions`, `GetItemSessionBySessionUUID`, `GetItemSessionBySessionAndItem`, `CreateItemSessionWithVerdict`) are updated to return `ItemSessionSummary` or `(ItemSessionSummary, error)` as appropriate
6. `make build` green

**Given-When-Then**:
- Given: `GetItemSession(ctx, id)` previously returned `(*ent.ItemSession, error)`
- When: changed to return `(ItemSessionSummary, error)` using `itemSessionToSummary`
- Then: `grep "GetItemSession" session/storage.go | grep "ent.ItemSession"` returns 0 hits

**Tasks**:
1. Read `session/ent_repository_backlog.go` to find all ItemSession-returning methods
2. Write `itemSessionToSummary`, `backlogStatusEventToData`, `sourceSyncEventToData`
3. Update each method's return type and call the conversion function at the return site
4. Update `session/storage_backlog.go` delegation methods to match new signatures
5. `make build`; iterate on call-site breakage

---

#### Story 4.1.3: Update all callers of changed methods in `server/services` and `session/`

**Acceptance criteria**:
1. `server/services/backlog_service.go` (and the split files from P1) contain zero references to `ent.ItemSession`, `ent.BacklogStatusEvent`, `ent.ReviewVerdict`, `ent.SourceSyncEvent`
2. `session/backlog_lifecycle.go` contains zero direct ent type references for ItemSession fields (uses `ItemSessionSummary` fields instead)
3. The `session/ent` import is removed from `server/services/backlog_service.go`
4. `make ci` green end-to-end
5. All 5 call sites in `server/mcp/tools_backlog.go` are updated to use `ItemSessionSummary` fields: `.ID.String()` becomes `.ID` (already a `string` in `ItemSessionSummary`) and `.SessionRole` becomes `.Role`; `make build` passes for the `server/mcp` package

**Given-When-Then**:
- Given: `backlog_service.go` line 21 imports `"github.com/tstapler/stapler-squad/session/ent"`
- When: all 14 `ent.IsNotFound` calls are replaced with `errors.Is(err, session.ErrNotFound)` AND all `*ent.ItemSession` usages are replaced with `session.ItemSessionSummary`
- Then: removing line 21 causes `make build` to still pass (no remaining ent references in the file)

- Given: `submit_triage_result` in `server/mcp/tools_backlog.go` calls `GetItemSessionBySessionAndItem` and previously read `.ID.String()` and `.SessionRole` on the `*ent.ItemSession` result
- When: `GetItemSessionBySessionAndItem` is changed to return `ItemSessionSummary` (Story 4.1.2)
- Then: the handler reads `.ID` (not `.ID.String()`) and `.Role` (not `.SessionRole`) from the result; `make build ./server/mcp/...` passes

**Tasks**:
1. Run `grep -n "ent\." server/services/backlog_service*.go` to enumerate remaining ent usages
2. Run `grep -n "GetItemSessionBySessionAndItem\|\.ID\.String()\|\.SessionRole" server/mcp/tools_backlog.go` to locate the 5 call sites in `tools_backlog.go`
3. For each `ent.IsNotFound(err)` in `server/services/`: replace with `errors.Is(err, session.ErrNotFound)`; confirm ent_repository_backlog already wraps that error path
4. For each `*ent.ItemSession` field access in `server/services/`: map to the corresponding `ItemSessionSummary` field
5. For `triageShortTitle(sessions []*ent.ItemSession, ...)`: change signature to `triageShortTitle(sessions []session.ItemSessionSummary, ...)`
6. Update all 5 call sites in `server/mcp/tools_backlog.go`: `.ID.String()` → `.ID`, `.SessionRole` → `.Role`
7. Remove `session/ent` import from `backlog_service.go`; `make build`
8. Enable the `no_ent_in_services` depguard rule (remove `files:` exclusion added in Phase 1)
9. `make ci`

---

#### Story 4.1.4: Add ItemSession CRUD methods to `Repository` interface

**Acceptance criteria**:
1. `session/repository.go`'s `Repository` interface includes:
   - `CreateItemSession(ctx context.Context, backlogItemID, sessionUUID, role string) (ItemSessionSummary, error)`
   - `GetItemSession(ctx context.Context, id string) (ItemSessionSummary, error)`
   - `GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (ItemSessionSummary, error)`
   - `GetItemSessionBySessionAndItem(ctx context.Context, sessionUUID, backlogItemID string) (ItemSessionSummary, error)`
   - `ListItemSessions(ctx context.Context, backlogItemID string) ([]ItemSessionSummary, error)`
   - `CreateItemSessionWithVerdict(ctx context.Context, params ItemSessionWithVerdictParams) (ItemSessionSummary, error)`
2. `*EntRepository` satisfies the updated `Repository` interface (confirmed by `make build` — Go will report if any method is missing)
3. `*Storage` delegates to `Repository` for all added methods (removes the `type assertion er, ok := s.repo.(*EntRepository)` workarounds)
4. `make test` green (existing integration tests exercise these paths)

**Given-When-Then**:
- Given: `session.Storage.ListItemSessions(ctx, itemID)` previously worked only via type assertion to `*EntRepository`
- When: `ListItemSessions` is added to the `Repository` interface and `Storage` delegates to `s.repo.ListItemSessions(...)`
- Then: a fake `Repository` implementation in tests can stub `ListItemSessions` without a real SQLite DB

**Tasks**:
1. Review current `Session/storage_backlog.go` for type-assertion workarounds; list all affected methods
2. Add method signatures to `Repository` interface in `session/repository.go`
3. Verify `*EntRepository` methods already have the right signatures (they do from Story 4.1.2)
4. Update `*Storage` delegation methods to remove type assertions
5. `make build`; `make test`

---

## Phase 5: BacklogItemSummary for List Views

### Epic 5.1: Add `BacklogItemSummary` DTO and List Projection

**Goal**: Introduce a narrower DTO for list-view queries that avoids over-hydrating full backlog items, while preserving all data the board view and list view actually need.

**PR label**: `feat` (additive internal type) | **Branch**: `refactor/backlog-item-summary`
**Depends on**: Phase 4 (P6 must be complete so `ItemSessionSummary` exists cleanly)

#### Story 5.1.1: Define `BacklogItemSummary` struct

**Acceptance criteria**:
1. `session/repository.go` contains:
   ```go
   type BacklogItemSummary struct {
       ID                  string         `json:"id"`
       ExternalID          string         `json:"external_id"`
       Title               string         `json:"title"`
       Status              BacklogStatus  `json:"status"`
       Priority            int            `json:"priority"`
       RepoPath            string         `json:"repo_path"`
       AcceptanceCriteria  AcCriteriaJSON `json:"acceptance_criteria"`
       Notes               string         `json:"notes"`
       PrURL               string         `json:"pr_url"`
       PrNumber            int            `json:"pr_number"`
       CreatedAt           time.Time      `json:"created_at"`
       UpdatedAt           time.Time      `json:"updated_at"`
       ArchivedAt          *time.Time     `json:"archived_at"`
       // Minimal session data for board view rendering — no ent types
       // Role, EndedAt, TriageResultSummary, OverallOutcome carry the fields
       // mapBacklogItem in useBacklogService.ts reads to derive triageStatus,
       // gateVerdict, and linkedSessions.
       ItemSessions        []ItemSessionSummary
   }
   ```
2. No ent types appear in `BacklogItemSummary`; `ItemSessionSummary` is already defined in `session/repository.go` (Story 4.1.1)
3. `BacklogItemSummary` can live in `session/repository.go` or (after P5/session/domain) in `session/domain`
4. `make build` green
5. All fields in `BacklogItemSummary` have explicit `json:"..."` struct tags matching ent's column name convention; `PrURL` must be tagged `json:"pr_url"`, `ExternalID` as `json:"external_id"`, `RepoPath` as `json:"repo_path"`, `AcceptanceCriteria` as `json:"acceptance_criteria"`, `ArchivedAt` as `json:"archived_at"` — ent's `ScanSlice` uses `strings.ToLower(fieldName)` (no underscores) when no tag is present, causing a hard `"sql/scan: missing struct field for column: pr_url"` error at runtime (not a silent zero value)

**Given-When-Then**:
- Given: a backlog item with one ItemSession in role `triage`, status `running`, and `triage_result_summary = "Tests passing"`
- When: the list query maps this item to `BacklogItemSummary`
- Then: `len(summary.ItemSessions) == 1`, `summary.ItemSessions[0].Role == "triage"`, `summary.ItemSessions[0].Status == "running"`, `summary.ItemSessions[0].TriageResultSummary == "Tests passing"`

**Tasks**:
1. Add `BacklogItemSummary` to `session/repository.go` (confirm `ItemSessionSummary` is already defined from Story 4.1.1 — `BacklogItemSummary.ItemSessions` references it without adding any ent import); **include explicit `json:"..."` struct tags on every field** matching the DB column name (e.g., `json:"pr_url"` not `json:"prurl"`) — ent's `ScanSlice` derives column names from these tags; missing or wrong tags cause a hard scan error
2. `make build`

---

#### Story 5.1.2: Add `ListBacklogItemSummaries` to `EntRepository` and `Repository` interface

**Acceptance criteria**:
1. `session/ent_repository_backlog.go` contains `ListBacklogItemSummaries(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemSummary, error)` that uses ent's `Select(fields...).Scan(ctx, &out)` projection
2. The scalar query selects: `backlogitem.FieldID`, `backlogitem.FieldTitle`, `backlogitem.FieldStatus`, `backlogitem.FieldPriority`, `backlogitem.FieldRepoPath`, `backlogitem.FieldAcceptanceCriteria`, `backlogitem.FieldNotes`, `backlogitem.FieldPrURL`, `backlogitem.FieldPrNumber`, `backlogitem.FieldExternalID`, `backlogitem.FieldCreatedAt`, `backlogitem.FieldUpdatedAt`, `backlogitem.FieldArchivedAt`
3. **Board view safety**: After the scalar scan, a second minimal query fetches session rows for each item in the current page — `WHERE backlog_item_id IN (<IDs from the page returned by query 1>)` on the `item_sessions` table, selecting `id`, `backlog_item_id`, `session_role`, `ended_at`, `triage_result`; a third query (or LEFT JOIN on the second) fetches `item_session_id, overall_outcome` from `review_verdicts WHERE item_session_id IN (<session IDs from the second query>)`; the conversion code (a) parses `triage_result` JSON to populate `ItemSessionSummary.TriageResultSummary`, (b) matches verdict rows by `item_session_id` to populate `ItemSessionSummary.OverallOutcome`, (c) derives running status from `ended_at IS NULL`; all results are grouped by `backlog_item_id` and appended to `BacklogItemSummary.ItemSessions`; the IN-list for the second query MUST be built from the page IDs only (not all items in the table) — document this constraint with a code comment; **do NOT select `status`, `triage_result_summary`, or `overall_outcome` directly from `item_sessions` — these columns do not exist in that table**
4. `ListBacklogItemSummaries` is added to the `Repository` interface
5. `Storage` delegates to `repo.ListBacklogItemSummaries`
6. `make build` green
7. Integration test: `TestListBacklogItemSummaries` in `session/backlog_integration_test.go` with at least 2 items (item A has 1 live triage session with `triage_result_summary = "All clear"`, item B has no sessions); asserts item A's `ItemSessions` has one entry with correct `Role`, `Status`, and `TriageResultSummary`; item B's `ItemSessions` is empty; add a 50-item fixture with a 10-item page to verify the second query does not cross page boundaries

**Given-When-Then**:
- Given: 2 backlog items in DB; item A has 1 ItemSession (role=triage, status=running, ended_at=nil, triage_result_summary="All clear"); item B has no ItemSessions
- When: `ListBacklogItemSummaries(ctx, filter)` is called
- Then: item A's `BacklogItemSummary.ItemSessions` contains one entry with `Role="triage"`, `Status="running"`, `TriageResultSummary="All clear"`; item B's `ItemSessions` is empty (`len == 0`)

**Tasks**:
1. Write `ListBacklogItemSummaries` with the three-phase approach: (a) `ent BacklogItemQuery.Select(scalar fields).Scan(ctx, &rawOut)` for the page; (b) build `ids` from `rawOut`; (c) query `item_sessions WHERE backlog_item_id IN (ids)` selecting `id, backlog_item_id, session_role, ended_at, triage_result` — **not** `status`, `triage_result_summary`, or `overall_outcome` (these columns do not exist in `item_sessions`); parse `triage_result` JSON in Go to populate `TriageResultSummary`; (d) query `review_verdicts WHERE item_session_id IN (<session IDs from step c>)` selecting `item_session_id, overall_outcome`; join verdict rows into the corresponding `ItemSessionSummary.OverallOutcome`; group all results into `map[string][]ItemSessionSummary`; assign to each `BacklogItemSummary.ItemSessions`
2. Add an explicit code comment on the IN-list construction: `// IN-list is page-scoped — do not pass all item IDs from the table`
3. Add `ListBacklogItemSummaries` to the `Repository` interface in `session/repository.go`; add delegation in `Storage`
4. Write `TestListBacklogItemSummaries` integration test including the 50-item / 10-item-page cross-page verification
5. `make test -run TestListBacklogItemSummaries`

---

#### Story 5.1.3: Update `BacklogService.ListBacklogItems` to use `BacklogItemSummary`

> **Note**: The board view already reads `item_sessions` from the `BacklogItem` proto via `mapBacklogItem` in `useBacklogService.ts` — no frontend change is needed. This story populates the EXISTING `repeated ItemSession item_sessions` proto field (partially, with only the fields the board view needs) on list responses. No new proto message, no `make proto-gen`, no TypeScript changes.
>
> **User-visible improvement (intentional)**: The current `ListBacklogItems` path returns `BacklogItemData` which over-hydrates each item but may not populate `item_sessions` on list responses (or returns stale counts). After this story, the board view's "View Session" button and triage spinner will correctly reflect live session state for all items on the list. This is a deliberate improvement, not a regression — document it explicitly in the PR description.
>
> **Performance note (triad 2026-07-09)**: The three-phase query adds 2 extra DB roundtrips per `ListBacklogItems` call (beyond the scalar SELECT). This is acceptable because: (a) the IN-list for both extra queries is page-scoped (not all items in the table), (b) `item_sessions.backlog_item_id` and `review_verdicts.item_session_id` are indexed foreign keys, (c) list calls are paginated (default 20 items/page). Verify with `make test-coverage` and observe no new allocations on the path. If this proves measurable in prod, the mitigation is to make the ItemSessions hydration opt-in via a request flag — but do not add the flag speculatively.

**Acceptance criteria**:
1. `BacklogService.ListBacklogItems` (in `backlog_service_query.go` after P1) calls `s.storage.ListBacklogItemSummaries` instead of `s.storage.ListBacklogItems`
2. `backlogItemSummaryToProto` (new function in `backlog_service_query.go`) maps `BacklogItemSummary` to `*sessionv1.BacklogItem` proto, populating the EXISTING `repeated ItemSession item_sessions` field from `BacklogItemSummary.ItemSessions`; each entry carries only `role`, `status`, `triage_result_summary`, and `overall_outcome` (other `ItemSession` proto fields left as zero values); no new proto messages or fields are added; `make proto-gen` is NOT run
3. The board view's "View Session" button is NOT broken: items with at least one entry in `item_sessions` still enable the button
4. The board view's triage spinner is NOT broken: items with an `item_sessions` entry where `status == "running"` and `role == "triage"` still show the spinner
5. `make ci` green

**Given-When-Then**:
- Given: a backlog item with 1 active triage session (role=triage, status=running) exists in DB
- When: `ListBacklogItems` RPC is called (via the board view's data fetch)
- Then: the proto response's `BacklogItem` for that item has `item_sessions` with one entry where `role == "triage"` and `status == "running"`; `mapBacklogItem` already reads `item_sessions` and derives `triageStatus = "running"` from this; board view renders the triage spinner with no TypeScript changes required

**Tasks**:
1. Write `backlogItemSummaryToProto` in `backlog_service_query.go`, mapping `BacklogItemSummary.ItemSessions` to the existing `item_sessions` proto field; populate only `role`, `status`, `triage_result_summary`, `overall_outcome` on each `ItemSession` proto entry (leave all other fields as zero values)
2. Update `ListBacklogItems` to call `ListBacklogItemSummaries` and use `backlogItemSummaryToProto`
3. Remove old `backlogItemToProto` call in the list path (keep for single-item `GetBacklogItem`)
4. `make ci`
5. Manual smoke: start a triage session for a backlog item; load board view; confirm spinner

---

## Phase 6: `session/domain` Sub-Package

### Epic 6.1: Create `session/domain` and Move Pure Leaf Types

**Goal**: Extract pure domain types (no ent/headless/git imports) from `session/` into a `session/domain` sub-package, enabling consumers to import just the domain types without pulling in infrastructure.

**PR label**: `refactor` | **Branch**: `refactor/session-domain-package`
**Depends on**: Phase 4 (so `ItemSessionSummary` is clean before deciding what moves)

**Pre-flight (required before any code is written)**:
```bash
# Confirm current session/ imports
go list -f '{{join .Imports "\n"}}' github.com/tstapler/stapler-squad/session

# Map what server/adapters and pkg/events import from session/
goda reach "github.com/tstapler/stapler-squad/server/adapters" "github.com/tstapler/stapler-squad/session"
goda reach "github.com/tstapler/stapler-squad/pkg/events" "github.com/tstapler/stapler-squad/session"

# After creating session/domain (later), confirm no cycle:
goda reach "github.com/tstapler/stapler-squad/session/domain" "github.com/tstapler/stapler-squad/session"
# MUST output nothing
```

#### Story 6.1.1: Create `session/domain` package with pure types

**Acceptance criteria**:
1. `session/domain/` directory exists with `package domain`
2. `session/domain/backlog.go` contains: `BacklogStatus`, `AcStatus`, `AcCriteriaJSON`, `AcCriterion`, `ReviewOutcome`, `CriterionVerdict`, `AggregateOutcome`, `ParseAcCriteria`, `SerializeAcCriteria`, all related constants (session role constants, AC status constants, `DefaultBacklogPriority`, etc.)
3. `session/domain/backlog.go` has ZERO imports from `github.com/tstapler/stapler-squad/session` (no cycle!)
4. `session/domain/` has no imports from `session/ent`, `session/headless`, `session/git`
5. `go build ./session/domain/...` passes immediately after creation
6. `goda reach github.com/tstapler/stapler-squad/session/domain github.com/tstapler/stapler-squad/session` produces no output

**Given-When-Then**:
- Given: `session/domain/` is created with `BacklogStatus` defined as `type BacklogStatus string`
- When: `go build ./session/domain/...` is run
- Then: exits 0 with no errors

**Tasks**:
1. Create `session/domain/` directory
2. Copy (not remove yet) types from `session/backlog.go` into `session/domain/backlog.go` — change `package session` to `package domain`
3. Remove any imports that reference `session/` (there should be none — `backlog.go` imports only `encoding/json` and `errors`)
4. `go build ./session/domain/...`
5. Run `goda reach` to confirm no cycle

---

#### Story 6.1.2: Add type aliases in `session/` for backward compatibility

**Acceptance criteria**:
1. `session/backlog.go` replaces type definitions with type aliases: `type BacklogStatus = domain.BacklogStatus`, `type AcCriterion = domain.AcCriterion`, etc. (using `=` syntax for transparent aliases)
2. All constants that were in `session/backlog.go` are replaced with `const BacklogStatusIdea = domain.BacklogStatusIdea` (or the `session/` file re-exports via `domain.BacklogStatusIdea` directly)
3. All 65+ existing callers that `import "github.com/tstapler/stapler-squad/session"` and use `session.BacklogStatus`, `session.AcCriterion`, etc. continue to compile WITHOUT any import changes
4. `make build` green
5. No changes needed in `server/services/`, `server/mcp/`, `session/backlog_lifecycle.go`, or any other file outside `session/backlog.go` at this step

**Given-When-Then**:
- Given: `server/services/backlog_service_triage.go` uses `session.BacklogStatus`
- When: Story 6.1.2 is applied (alias added to `session/backlog.go`)
- Then: `server/services/backlog_service_triage.go` compiles unchanged — `session.BacklogStatus` still resolves via the alias

**Tasks**:
1. Open `session/backlog.go`
2. Add `import "github.com/tstapler/stapler-squad/session/domain"` at top
3. Replace each `type X string/int` definition with `type X = domain.X`
4. Replace `const` blocks with forwarding constants (either re-declare using `= domain.XConst` or keep them as-is if they duplicate from domain)
5. `make build`; fix any alias breakage (watch for function signatures — aliases work transparently)

---

#### Story 6.1.3: Update direct importers of `session/domain` types

**Acceptance criteria**:
1. `pkg/events` imports `session/domain` (not `session`) for the types it uses; its `session` import is dropped if no other `session.*` type is used
2. `server/adapters` imports `session/domain` for domain types; `session` import kept only for `Storage`, `Instance`, etc.
3. `make build` and `make ci` green
4. No other file is changed in this story (the alias bridge handles the rest)

**Given-When-Then**:
- Given: `pkg/events/events.go` imports `session` and uses `session.BacklogStatus`
- When: import changed to `domain "github.com/tstapler/stapler-squad/session/domain"` and all `session.BacklogStatus` → `domain.BacklogStatus`
- Then: `go build ./pkg/events/...` passes; `session` import in `pkg/events` is either gone or retained only for non-domain types

**Tasks**:
1. `grep -rn "session\." pkg/events/ server/adapters/ | grep -E "BacklogStatus|AcCriterion|AcCriteriaJSON|ReviewOutcome"` to find the symbol usages
2. Update each file's import to add `session/domain`
3. Replace `session.TypeX` with `domain.TypeX` for moved types
4. `make build`; `make ci`

---

## Acceptance Criteria Roll-Up (Success Metrics from Requirements)

| Metric | Target | Measured How |
|---|---|---|
| `backlog_service.go` splits into ≥3 files, each ≤900 lines | 4 files (struct, query, lifecycle, triage); each ≤900 lines | `wc -l server/services/backlog_service*.go` |
| Peak cognitive complexity per file < 40 (down from 87) | Verified after P1 and P3 | `make analyze` or `gocognit ./server/services/backlog_service*.go ./session/backlog_lifecycle.go` |
| `mergeAcCriteria` has ≥3 table-driven unit tests | 5 cases written in Phase 2 | `go test ./session/... -run TestMergeAcCriteria -v` |
| `ReviewGateRunner` in `session/review_gate.go` | Struct + tests exist | File existence + `go test ./session/... -run TestReviewGateRunner` |
| `BacklogItemSummary` struct exists with `ItemSessions []ItemSessionSummary`; no list query returns full `BacklogItemData` | True after Phase 5 | Grep: `grep -n "ListBacklogItems" server/services/backlog_service_query.go` calls `ListBacklogItemSummaries`; `BacklogItemSummary` has no `HasLinkedSession`/`TriageRunning` fields |
| `session/domain` sub-package exists with ≥4 moved types | `AcCriterion`, `BacklogStatus`, `AcCriteriaJSON`, `ReviewOutcome` confirmed moved | `go doc github.com/tstapler/stapler-squad/session/domain` |
| Packages importing `session` only for domain types now import `session/domain` | `pkg/events`, `server/adapters` updated | Grep check |
| No ent types visible in `backlog_service.go` | `grep "ent\." server/services/backlog_service.go` returns 0 hits | Grep + `no_ent_in_services` depguard rule in CI |
| All existing tests pass (`make ci` green) | Required at every PR | CI pipeline |
