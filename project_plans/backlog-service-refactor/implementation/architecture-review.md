# Architecture Review: backlog-service-refactor
**Date**: 2026-07-09
**Reviewer**: Architecture review subagent
**Verdict**: BLOCKED

---

## Constitution Violations

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository. No constitution constraints to apply. Findings are grounded in the project's existing ADRs, CLAUDE.md rules, and the referenced interface-pollution checklist.

---

## Blockers

- [x] **Story 3.2.1** — `ReviewGateRunner` struct under-specified: 3 constructor fields cannot hold `spawnReviewGate`'s full dependency set.

  `spawnReviewGate` in `session/backlog_lifecycle.go` uses **5** distinct dependencies from `BacklogLifecycleListener`:
  - `l.storage` — included ✓
  - `l.getHeadlessPool()` (the `*headless.Pool`) — included ✓
  - `l.shutdownCtx` — included ✓
  - `l.getAutoReopener()` (`AutoReopenSpawner`) — **missing** from proposed struct
  - `l.sessionCreator` (`ReviewGateSpawner`) — **missing** from proposed struct

  The auto-reopen path (lines 498–505) calls `reopener.AutoReopenAfterFailedReview` on FAIL/PARTIAL outcomes; the legacy path (lines 517–540) calls `l.sessionCreator.SpawnReviewSession`. Neither is reachable from the proposed 3-field struct. The plan's claim that `BacklogLifecycleListener.spawnReviewGate` "becomes a one-liner: `l.runner.Run(ctx, item, is, l.pushAndCreatePR)`" cannot compile as written — `Run` would panic or silently skip the auto-reopen and legacy paths.

  **Remediation**: Either (a) add `autoReopener AutoReopenSpawner` and `sessionCreator ReviewGateSpawner` as constructor fields (making it a 5-field struct), or (b) add a second callback parameter `onFail func(ctx context.Context, itemID string) error` alongside `onPass`, and drop the legacy `sessionCreator` path before P3 (requires a separate prep story). Update Story 3.2.1 acceptance criteria and constructor signature before implementation begins.

- [x] **Story 4.1.3** — `server/mcp/tools_backlog.go` is not in the caller-update sweep, but it will break when `GetItemSessionBySessionAndItem` changes return type.

  Story 4.1.2 changes `GetItemSessionBySessionAndItem` (and all other ItemSession methods) to return `ItemSessionSummary` where `ID` is a `string`, not `uuid.UUID`. The file `server/mcp/tools_backlog.go` calls `GetItemSessionBySessionAndItem` **5 times** and accesses `.ID.String()` (lines 380, 386, 597) and `.SessionRole` (lines 136, 351, 460). After Story 4.1.2 ships, `.ID.String()` becomes `.ID` (already a string), and story 4.1.3's acceptance criteria limit scope to `server/services` and `session/`. CI will break on the `server/mcp` package.

  Confirmed via grep: `grep -n "\.ID\.String()\|\.SessionRole" server/mcp/tools_backlog.go` hits exist.

  **Remediation**: Add `server/mcp/tools_backlog.go` to the caller-update checklist in Story 4.1.3 AC #1 and task list. The change is mechanical (`.ID.String()` → `.ID`) but must be committed in the same PR as the type change.

---

## Concerns

- [ ] **Story 3.2.2** — `TestReviewGateRunner_SkipReviewGate` tests a path that `ReviewGateRunner.Run` can never execute.

  `SkipReviewGate: true` is checked by the **caller** (`onSessionExited`, lines 249 and 311) before `spawnReviewGate` is called — `spawnReviewGate` itself never receives an item with `SkipReviewGate: true`. If `ReviewGateRunner.Run` inherits that body verbatim, the same invariant holds: `Run` is never invoked in this state. The test exercises a dead branch and gives false confidence.

  **Recommendation**: Replace with `TestReviewGateRunner_NoMechanismConfigured` (pool=nil, sessionCreator=nil) or a test at the `BacklogLifecycleListener.onSessionExited` level that asserts the short-circuit before delegation. Keep the headless PASS path test (which is valid).

- [ ] **Story 1.1.1** — Lint guard relies on undocumented CI behavior: "violations are pre-existing, so CI stays green."

  The plan's AC #4 depends on `make lint` treating pre-existing violations as non-blocking. If CI runs `golangci-lint` with `--max-issues-per-linter=0` or any fail-on-warning config, adding a `forbidigo` rule against 14 existing violations will immediately break the pipeline. The plan notes "CI must be green because violations are pre-existing (tracked, not blocking this PR itself)" but does not verify the CI lint configuration.

  **Recommendation**: Before implementing Story 1.1.1, read `.golangci.yml` and `.github/workflows/` to confirm how lint failures are surfaced. If necessary, add the new rule with an `issues.exclude-rules` block covering the 14 specific file:line pairs, and remove them one-by-one in the P6 PR. Document the CI lint gate behavior explicitly in the PR description.

- [ ] **Story 4.1.4** — AC #3 ("removes the type assertion workarounds") over-scopes relative to the 6 ItemSession methods added.

  There are **25** `s.repo.(*EntRepository)` type assertions in `session/storage.go`. Story 4.1.4 adds only 6 ItemSession CRUD method signatures to `Repository`. The remaining 19+ assertions (covering backlog CRUD, worktree data, review verdicts, sync events, and shell management) are untouched. The acceptance criteria as written implies all type assertions are removed, but the tasks only enumerate ItemSession methods. An implementer following AC #3 literally would attempt to add 19 more methods to the interface, massively expanding scope.

  **Recommendation**: Revise AC #3 to: "The 6 ItemSession methods no longer require type assertion in `Storage`; remaining non-ItemSession type assertions are tracked in a follow-on issue." Or split the story into a full interface-completion epic.

- [ ] **ADR numbering conflict** — Plan references ADR-010 (session/domain sub-package) and ADR-011 (Storage interface domain DTOs), which are already claimed in this repo.

  `docs/adr/010-frontend-modularity.md` and `docs/adr/011-prefer-lock-free-concurrency.md` both exist. The plan's ADR references use numbers that conflict with real, committed decisions. If these are ADRs to be written during implementation, they need new numbers (currently the last used appears to be around 022–023 based on the `ADR-022` and `ADR-023` files).

  **Recommendation**: Audit the highest existing ADR number, assign sequential numbers (e.g., ADR-024, ADR-025), write the ADR files before P5/P6 implementation begins, and update the plan header.

- [ ] **P2 function signature mismatch between requirements and plan**

  Requirements specify `mergeAcCriteria(existing []AcCriterion, incoming []rawAC) ([]AcCriterion, error)` (raw-typed input, slice output). Story 2.1.1 specifies `MergeAcCriteria(existing []AcCriterion, incoming []AcCriterion) (AcCriteriaJSON, error)` (domain-typed input, named-type output). The plan's version is objectively better (prevents double-serialization and uses the named type as a parse-at-boundary guarantee), but the discrepancy means the requirements document is stale and a reviewer doing spot-checks against requirements will flag it.

  **Recommendation**: Update `requirements.md` to reflect the plan's signature, or add a "Deviation from requirements" note to Story 2.1.1.

---

## Nitpicks

- Story 3.1.3 AC #7 says "cognitive complexity drops to < 40 (from 35 — already close)". If the current value is 35, the requirement is already met before any change. Likely a confusion between `backlog_lifecycle.go`'s file-level complexity (which is already lower) and the problematic `submitTriageResult` function (87). Verify with `gocognit ./session/backlog_lifecycle.go` and calibrate the target.

- The plan uses dual numbering (Phase 1–6 for sequencing, P1–P6 for problem areas) where Phase 3 = P1+P3, Phase 4 = P6, Phase 5 = P4, Phase 6 = P5. This inversion will cause confusion for implementers. Consider renaming the sequencing phases as "PR-0, PR-1, PR-2a, PR-2b, PR-3, PR-4, PR-5" to eliminate the ambiguity.

- Story 6.1.3's grep to audit importers uses `"session\."` prefix: `grep -rn "session\." pkg/events/ ...`. If a file imports the package with an alias other than `session` (e.g., `import sess "..."`) this grep will miss it. Safer: `grep -rn "BacklogStatus\|AcCriterion\|AcCriteriaJSON\|ReviewOutcome" pkg/events/ server/adapters/`.

- `BacklogItemSummary.AcceptanceCriteria` is `AcCriteriaJSON` (an opaque serialized string). Board views that need to render per-criterion status indicators will need to parse this on the read path. Add a struct comment noting this expected use pattern so future callers don't invent a second encoding.

- Unresolved Question #1 (can `ent BacklogItemSelect.Scan()` be used with ItemSessions?) is answered by the plan's own Story 5.1.2 task #1 ("two-phase approach: scan scalar fields, then lookup session flags"). The question can be closed as "resolved: use two queries."

---

## Re-review (2026-07-09)

**Re-reviewer**: Architecture re-review subagent
**Scope**: Prior blockers only (Story 3.2.1 and Story 4.1.3)

### Blocker 1 — Story 3.2.1 ReviewGateRunner: RESOLVED

The updated plan specifies a **5-field** struct in AC #2:
```go
type ReviewGateRunner struct {
    storage        Storage
    pool           *headless.Pool
    shutdownCtx    context.Context
    autoReopener   func(ctx context.Context, itemID uuid.UUID) error
    sessionCreator func(ctx context.Context, item *ent.BacklogItem, is *ent.ItemSession) error
}
```
The constructor in AC #3 names all 5 arguments. AC #4 explicitly states `Run` uses `r.autoReopener` on FAIL/PARTIAL outcomes (lines ~498–505) and `r.sessionCreator` for the legacy path (lines ~517–540). The G-W-T tests the FAIL path: `autoReopener` called once, `onPass` not called. All prior requirements are satisfied. The Pattern Decisions table (plan line 59) independently confirms the 5-field constructor.

### Blocker 2 — Story 4.1.3 tools_backlog.go caller sweep: RESOLVED (with new concern)

`server/mcp/tools_backlog.go` now appears in:
- Task 2 of Story 4.1.3 (explicit grep step against `tools_backlog.go`)
- AC #5 covering all 5 call sites
- A second G-W-T block whose "Then" uses `.SessionID` and `.Role`

The original blocker (file absent from the sweep) is resolved.

**New concern surfaced during re-review**: Story 4.1.1 AC #1 defines `ItemSessionSummary` with fields `ID string` and `SessionRole string`, but Story 4.1.3 AC #5 and its G-W-T tell callers to use `.SessionID` and `.Role`. These field names do not match the struct definition — an implementer following Story 4.1.3 literally will get a compile error. This inconsistency must be resolved before implementation: either rename the struct fields to `SessionID` and `Role`, or update Story 4.1.3 AC #5 and G-W-T to reference `.ID` and `.SessionRole` respectively. This is a new concern, not a re-opened blocker.

### Overall verdict

Both prior blockers are resolved. The plan may proceed to implementation with one new pre-implementation fix required: reconcile `ItemSessionSummary` field names between Story 4.1.1 (`ID`, `SessionRole`) and Story 4.1.3 (`.SessionID`, `.Role`).
