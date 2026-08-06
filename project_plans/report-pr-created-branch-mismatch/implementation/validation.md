# Validation Plan: report-pr-created-branch-mismatch

**Date**: 2026-08-04

## Happy Path Scenario

Given backlog item `3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d` is `review` status with
tracked branch `backlog/stapler-squad-ci-status-diff-viewer`, and GitHub PR
`https://github.com/tstapler/stapler-squad/pull/326` really exists with head branch
`feature/ci-status-diff-viewer` and state `merged`, when the linked `role=work` session
calls `report_pr_created` with `pr_number=326`, a `summary`, and a non-empty
`override_reason` explaining the fallback branch, then `SetBacklogItemPRAndTransition`
persists and the item transitions `review` → `pr_pending` with `PrNumber == 326` — where
before this fix the identical call permanently hard-rejected with `PR #326 does not
match this item's branch ... on GitHub — refusing to record it.`

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1 (happy path): same tool links a fallback-branch PR, GitHub-verified | `github/client_pr_by_number_test.go` | `TestGetPRByNumber_should_ReturnPRInfo_When_PRExists` | Unit | 200 response, `head.ref="feature/ci-status-diff-viewer"`, `merged=true` → `State == github.PRStateMerged` (root-cause data source for AC1) |
| AC1 (error path): PR lookup for override candidate returns "does not exist" | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_RejectCall_When_PRNumberDoesNotExist` | Unit (wiring confirmation; decision logic itself pinned by `TestDecideOverridePolicy`'s not-exists case) | `verifyPRMatchesBranch` → `NewPRVerification(false, false, "", "", "")` (5-arg signature, third plan-repair pass — trailing `""` for `author`); request includes `override_reason` → still `ErrInvalidArgument` "does not exist"; item untouched — existence can never be overridden. **(Third plan-repair pass)** also asserts `resolveCallerGitHubLogin` is never called (call-counting stub) — `decideOverridePolicy`'s guard requires `Exists`, which is `false` here, so identity resolution is never reached |
| Live-GitHub re-verification of the REST response-shape assumptions (fourth plan-repair pass — resolves pre-mortem.md P1 #1/#3) | `github/client_pr_by_number_live_test.go` | `TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326` | Manual / live-network (excluded from CI by `live_github` build tag — not run by `go test ./...`, `make test`, `make test-race`, or `make test-integration`/`make ci`) | Calls the real, unauthenticated `github.GetPRByNumber(ctx, "tstapler", "stapler-squad", 326)` against real `api.github.com` (PR #326 is real, already-merged, publicly visible). Asserts `err == nil`, `HeadRef == "feature/ci-status-diff-viewer"`, `State == github.PRStateMerged`, `BaseRef` non-empty, and `Author` non-empty (logged via `t.Logf` for manual cross-check against `gh pr view 326 --json author`). This is the only test in the whole plan that exercises `base.repo.full_name`/`user.login`/`head.ref`/`base.ref` parsing against real GitHub data rather than an `httptest.Server` fixture or a stubbed seam — closes the residual "the mocks pass but the shape assumption could still be wrong" gap the pre-mortem flagged. **Required before shipping, not just written**: per plan.md's Definition of Done and Task 6.7 checklist, it must be run locally with `go test -tags live_github -run TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326 -v ./github/...` and its `-v` output (including the logged `Author` value) pasted into the PR description or session notes — a passing write of this test's code is not sufficient, the run itself is the required artifact |
| AC1 (integration): full handler persists fallback-branch PR end to end | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_TransitionToPRPending_When_FallbackBranchWithOverrideReason` | Integration (real storage/persistence via test fixture, not just the decision) | Confirmed repro shape: item `review`, tracked branch `backlog/stapler-squad-ci-status-diff-viewer`, verification `Matched=false`/`ActualHeadBranch="feature/ci-status-diff-viewer"`/`State=merged`, `override_reason` set → item `pr_pending`, `PrNumber=326`, `log.Warn` audit line emitted |
| AC2 (happy path): manual-override path exists and is gated by an explicit reason | `server/mcp/tools_backlog_test.go` | `TestDecideOverridePolicy` (mismatch + non-empty `overrideReason` + author-matches + `State ∈ {open, merged}` cases) | Unit, table-driven | Pure-function pin of the override-accept branch, no I/O — the authoritative place AC2's "gated" behavior is proven. **(Third plan-repair pass)** `decideOverridePolicy` is now a 3-arg function (`v PRVerification, overrideReason, callerLogin string`), and this table also carries two new author-mismatch rows (Task 4.0) that pin the author check firing *before* the state check — see the AC3 open/merged row below for the full-handler wiring test this table-driven pin backs |
| AC2 (error path): override path rejected when reason omitted | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_RejectCall_When_FallbackBranchMissingOverrideReason` | Unit (wiring confirmation; decision logic pinned by `TestDecideOverridePolicy`'s mismatch/empty-reason case) | Same fixture as the fallback-accept test but `override_reason` omitted → `ErrInvalidArgument`; item remains `review`, `PrNumber == 0` |
| AC2 (integration): override use is audited via structured log | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_TransitionToPRPending_When_FallbackBranchWithOverrideReason` (same test as AC1 integration row — same call under test, audit assertion folded in per Task 4.1) | Integration | Asserts `log.Warn` fired with `session`, `item`, `pr_number`, `actual_head_branch`, `tracked_branch`, `override_reason` fields — an operator can grep this after the fact even with no technical human gate |
| AC3: rejection message documents the workaround | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_DocumentOverrideWorkaround_When_BranchMismatchRejected` | Unit (full-handler; exact message text is formatted in `reportPRCreated`, not in the pure decision function, so this can't be pinned at the pure-function level) | Same fixture/request as the missing-override-reason test; asserts the returned error contains `"override_reason"`, the actual head branch string, and the tracked branch string — the caller is told concretely what to retry with |
| AC3 (negative control, closed-state case) — a real PR that's not open/merged must still be rejected even with a reason | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_RejectCall_When_UnrelatedClosedPRWithOverrideReason` (**renamed, third plan-repair pass**, from `..._UnrelatedPRWithOverrideReason` — the old name implied broader "no relationship whatsoever" coverage than this test ever had, since it only ever exercised the closed-state rejection; see the Task 4.4a row below for the open/merged case the requirement actually needs proven) | Unit (wiring confirmation; decision logic pinned by `TestDecideOverridePolicy`'s mismatch+reason+author-matches+closed-state case) | `verifyPRMatchesBranch` → `NewPRVerification(true, false, "totally-unrelated-branch", github.PRStateClosed, "tstapler")` — a real PR whose author **deliberately matches** the caller (isolates this test to the state gate alone; an author mismatch would also reject a closed PR, but for the wrong reason); `resolveCallerGitHubLogin` → `"tstapler", nil`; request has `override_reason` → still `ErrInvalidArgument`; item untouched — proves the state gate, not just the reason, is required. **Only covers the closed-state case** — does not by itself prove requirements.md's "no relationship whatsoever" constraint for an open/merged unrelated PR (a PR the pre-repair design would have accepted); that gap is closed by the next row |
| AC3 (negative control, open/merged case) — the test that actually proves requirements.md's "a PR that has no relationship whatsoever to the item's work must still be rejected" constraint | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_RejectCall_When_UnrelatedPRAuthorMismatch` (**new, third plan-repair pass, Task 4.4a** — closes the cross-artifact consistency gap a review found between requirements.md's constraint and this plan's own prior test coverage) | Unit (wiring confirmation; decision logic — mismatch + reason + author-mismatch, checked *before* the state gate — pinned by `TestDecideOverridePolicy`'s two new author-mismatch rows in Task 4.0) | `verifyPRMatchesBranch` → `NewPRVerification(true, false, "totally-unrelated-branch", github.PRStateOpen, "a-different-github-user")` — a real, **open**, correct-repo PR (everything the pre-repair design required for acceptance) but authored by someone else; `resolveCallerGitHubLogin` → `"tstapler", nil`; request has `override_reason` → still `ErrInvalidArgument`, message names both `verification.Author` (`"a-different-github-user"`) and `callerLogin` (`"tstapler"`); item untouched. Unlike the closed-state sibling above, this fixture is **open** — the pre-repair design (existence + correct repo + open/merged state + non-empty reason, no authorship check) would have accepted it. This is the row that proves the open/merged unrelated-PR case is rejected via author-mismatch, not via PR state — the case that actually matters for requirements.md's constraint |
| Constraint: existing branch-mismatch rejection (no relationship, no override) must not regress | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_RejectCall_When_BranchMismatch` (pre-existing test, updated in Task 2.3 for the new `PRVerification` return type only — assertions unchanged) | Unit | No `override_reason` in the request; mismatch → `ErrInvalidArgument`, item untouched — same outcome as before this fix, confirming the fast/strict path is preserved in spirit |
| Constraint: existing idempotency behavior preserved | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_NoOp_When_AlreadyPRPendingSamePR` (pre-existing test, updated in Task 2.3 for the new `PRVerification` return type only) | Unit | Re-reporting the same `pr_number` on an already-`pr_pending` item short-circuits before `verifyPRMatchesBranch` is even invoked — success no-op, unchanged by this fix |
| Constraint: existing transient-GitHub-error handling preserved | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails` (pre-existing test, updated in Task 2.3 for the new `PRVerification` return type) | Unit | `verifyPRMatchesBranch` returns `(PRVerification{}, err)` for a transient GitHub error → `ErrInternalError`, retryable, unchanged from before this fix |
| Root cause: number-keyed GitHub lookup (vs. branch-keyed) | `github/client_pr_by_number_test.go` | `TestGetPRByNumber_should_ReturnPRInfo_When_PRExists` | Unit | Same as AC1 happy-path row — the root-cause data-source fix |
| Root cause error path: nonexistent PR number | `github/client_pr_by_number_test.go` | `TestGetPRByNumber_should_ReturnErrNoPR_When_PRDoesNotExist` | Unit | 404 → `errors.Is(err, github.ErrNoPR)` — typed sentinel, not a string-sniffed `gh` CLI stderr, closing the anti-pattern `VerifyPRMatchesBranch`'s own doc comment warns against |
| Root cause defensive check: response owner/repo must match request | `github/client_pr_by_number_test.go` | `TestGetPRByNumber_should_ReturnError_When_RepoFullNameMismatch` | Unit | 200 with well-formed body but `base.repo.full_name` set to a different `owner/repo` than requested → non-nil error, confirming the defensive check actually rejects rather than trusting the body blindly |
| Root cause transient-error shape (feeds `ErrInternalError`, not a hard reject) | `github/client_pr_by_number_test.go` | `TestGetPRByNumber_should_ReturnError_When_Forbidden`, `TestGetPRByNumber_should_ReturnError_When_ServerError` | Unit | 403 / 500 → non-nil, non-`ErrNoPR` error, mirroring `GetPRForBranch`'s existing status-code branches — pins the shape `reportPRCreated` depends on to surface a retryable error |
| `PRVerification` invariant (`Matched ⇒ Exists`) enforced at construction | `server/mcp/tools_github_test.go` | `TestNewPRVerification_should_ForceMatchedFalse_When_MatchedTrueButNotExists` | Unit | **Gap plan.md doesn't name a test for** — plan.md (Task 2.1) specifies the *behavior* (`matched && !exists` is a construction error, logged and forced to `matched=false`, never panics) but the task list has no explicit test name pinning it. Added here to close the gap; table-driven alongside a legal-construction control case (`matched=true, exists=true` passes through unchanged). **(Third plan-repair pass)** `NewPRVerification` is now `NewPRVerification(exists, matched bool, actualHeadBranch, state, author string) PRVerification` — a 5th `author` parameter, carried through unvalidated (it is not part of the `Matched ⇒ Exists` invariant this constructor enforces, and no additional invariant is added for it); this test's construction-error and legal-construction cases should pass any fixed placeholder `author` value through both without asserting on it |
| Rewritten `VerifyPRMatchesBranch` — happy/fast path (`Matched=true`) | `server/mcp/tools_github_test.go` | `TestVerifyPRMatchesBranch_should_ReturnMatchedTrue_When_HeadBranchEqualsExpected` | Integration (`httptest.Server` + `GhBaseURL` override, exercises `GetPRByNumber` through the real function, not a mock) | **Gap plan.md doesn't name a test for** — Task 2.1 rewrites this function's body and doc comment but Story 2's task list has no dedicated test for the rewritten function itself (only its *callers*' seam literals are updated in Task 2.3). Added here as the integration test for the `VerifyPRMatchesBranch → GetPRByNumber` REST round trip requirements.md's own constraint ("no new external dependency… confirm what's available") implies must still work end to end, not just at the unit-mocked seam. **(Third plan-repair pass)** the success construction is now `NewPRVerification(true, info.HeadRef == expectedBranch, info.HeadRef, info.State, info.Author)` — this test's `httptest.Server` fixture should include a `user.login` value in its response body and this test should assert the returned `PRVerification.Author` equals it, not just `Matched`/`ActualHeadBranch`/`State` |
| Rewritten `VerifyPRMatchesBranch` — error path (GitHub lookup fails) | `server/mcp/tools_github_test.go` | `TestVerifyPRMatchesBranch_should_ReturnError_When_GetPRByNumberFails` | Integration (`httptest.Server` returning 500) | Same gap as above — confirms `(PRVerification{}, err)` propagates unchanged from `GetPRByNumber`'s error, the exact contract `reportPRCreated`'s transient-error handling (unchanged by this fix) depends on |
| Story 6 / reconciliation-automation guard — happy/matched path | `session/backlog_lifecycle_pr_branch_guard_test.go` | `TestVerifyPRHeadBranchMatchesTracked_should_ReturnTrue_When_HeadBranchMatches` | Unit | Finder stub returns matching `HeadRef` → `true, nil` |
| Story 6 / reconciliation-automation guard — error/fail-closed path | `session/backlog_lifecycle_pr_branch_guard_test.go` | `TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_FinderErrors`, `TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_TrackedBranchEmpty` | Unit | Finder error, or empty tracked branch (short-circuits before any GitHub call, finder-not-called assertion) → both fail closed (`false`, non-nil error), never read as "verified match" |
| Story 6 / reconciliation-automation guard — integration: mutation actually skipped on mismatch | `session/backlog_lifecycle_pr_branch_guard_test.go` | `TestCloseIfSupersededByMain_should_NotClosePR_When_HeadBranchMismatchDetected`, `TestReconcilePRPending_should_NotTransitionToDone_When_HeadBranchMismatchDetected`, `TestReconcileBouncingItems_should_NotTransitionToDone_When_HeadBranchMismatchDetected` | Integration (existing `backlog_lifecycle_test.go` fixture patterns — mock storage, `prPendingChecker`, `fakePRFixSpawner`) | Stubbed `prByNumberFinder` returns a mismatched `HeadRef`; asserts `checker.ClosePR` is never called / item never transitions to `done` — the direct proof of the adversarial review's Blocker fix: an override-linked item is provably never auto-closed or auto-completed on `item.PrNumber` alone |
| Story 6.3a / fix-spawn disclaimer — unverified association surfaced to spawned session | `session/backlog_lifecycle_pr_branch_guard_test.go` | `TestReconcilePRPending_should_IncludeUnverifiedDisclaimer_When_ClosedPRHeadBranchMismatchDetected`, `TestReconcilePRPending_should_IncludeUnverifiedDisclaimer_When_CIFailingPRHeadBranchMismatchDetected` | Integration (`fakePRFixSpawner.lastFixContext` recording field) | Mismatched `prByNumberFinder` stub at both `fixCtx`-building call sites → `lastFixContext` contains the `"NOTE: this PR's association with this backlog item could not be verified"` disclaimer prefix |
| Story 6.3a / fix-spawn disclaimer — control case, verified association unchanged | `session/backlog_lifecycle_pr_branch_guard_test.go` | `TestReconcilePRPending_should_OmitUnverifiedDisclaimer_When_CIFailingPRHeadBranchMatchesTracked` | Integration | Matching `prByNumberFinder` stub → `lastFixContext` does not contain `"NOTE:"` and is byte-for-byte identical to the pre-Task-6.3a fixture assertion — pins that the normal, verified CI-fix path is unchanged |
| Story 5 blind-spot note (documentation only, no behavior change) | N/A | N/A | N/A | Task 5.1 adds a one-line comment in `session/backlog_lifecycle.go` above `reconcileOrphanedAgentPRs`'s existing branch-keyed lookup; no functional change, so no test is expected or needed — noted here so the mapping table isn't silently missing this requirement-adjacent task |

### Gaps found in plan.md's own test list

Two tests are added above that plan.md's task list does not explicitly name, both flagged
inline in the table:

1. **`NewPRVerification`'s invariant-enforcement behavior** (Task 2.1 describes the
   behavior — force `matched=false` + loud log on `matched && !exists` — but Story 2's
   task list names no test for the smart constructor itself, only for its call sites).
2. **`VerifyPRMatchesBranch`'s rewritten body**, exercised end-to-end against a real
   `httptest.Server` (Task 2.1 rewrites the function; Task 2.3 only updates the *seam
   literals* that stand in for it in `tools_backlog_test.go`, so nothing in Story 2/4
   actually calls the real rewritten function through an HTTP stub). Without this,
   `GetPRByNumber` (Story 1, tested directly) and `decideOverridePolicy`/`reportPRCreated`
   (Story 3/4, tested against a stubbed seam) are each covered, but the glue that wires
   them together inside `VerifyPRMatchesBranch` itself has no test exercising the real
   REST round trip. Both gaps are closed by new tests in `server/mcp/tools_github_test.go`
   above; no existing plan.md task name is duplicated or contradicted.

**Re-checked against the current (post-third/fourth-plan-repair-pass) plan.md**: both gaps
are still accurate. `grep -n "TestNewPRVerification\|TestVerifyPRMatchesBranch\|tools_github_test.go"
project_plans/report-pr-created-branch-mismatch/implementation/plan.md` returns no hits —
Task 2.1 still only describes `NewPRVerification`'s invariant behavior and
`VerifyPRMatchesBranch`'s rewritten body in prose, and Task 2.3 still only updates the
seam-literal callers in `tools_backlog_test.go`, not a dedicated test for either function
itself. The third plan-repair pass's `author` parameter and fourth pass's Task 1.3 did not
touch this: neither gap is about the `author`/`callerLogin` additions, and neither closes it
incidentally.

**Task 1.3 (fourth plan-repair pass) is not a third instance of this same kind of gap** —
it's the opposite case. Where the two gaps above are tests validation.md added because
plan.md's task list never named them, Task 1.3's live-GitHub test
(`TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326`) is explicitly authored and
named by plan.md itself (Story 1, ~line 207-266), in direct response to a pre-mortem P1
finding — validation.md's job for it is only to map it into the Requirement → Test Mapping
table (done above), not to originate it.

No requirement was found for which plan.md's tests provide zero coverage — every AC and
constraint has at least a wiring-confirmation or table-driven pin in the existing task
list; the two gaps above are additions for completeness (the smart constructor's own
invariant, and the rewritten integration function's own round trip), not corrections.

## UX Acceptance Tests

N/A — no user-facing surface. `project_plans/report-pr-created-branch-mismatch/design/ux.md`
does not exist; this is a pure MCP-tool/backend bug fix (confirmed: the only caller-visible
surface is the `report_pr_created` MCP tool's schema/description and its error/success
message text, covered above as AC2/AC3 unit and integration tests, not a UI).

## Migration Plan

N/A — no schema changes. No ent schema migration, no persisted data format change
(`PRVerification`, `GetPRByNumber`, and the `reportPRCreated` branching are all
pure/stateless; see plan.md's Risk Control section: "Reverting the commit fully restores
the prior … behavior with zero cleanup").

## Test Stack

- **Unit**: Go `testing` package, table-driven where applicable (matches this repo's
  existing `server/mcp/tools_backlog_test.go` and `github/*_test.go` conventions)
- **Integration**: Go `testing` + `httptest.Server` for GitHub API stubs (matches the
  `GhBaseURL` override pattern already used in `server/services/backlog_github_rpc_test.go`'s
  `resetGhBaseURL` helper — `github.GhBaseURL = ts.URL + "/"`, deferred reset), in-memory/mock
  storage seams already established in `backlogHandlers` (`resolveSessionBranch`/
  `verifyPRMatchesBranch` function-field overrides) and in `BacklogLifecycleListener`
  (`SetOrphanedPRFinder`-style `SetPRByNumberFinder` seam, Task 6.1)
- **E2E / UX**: N/A — no user-facing surface
- **Manual / live-network** (fourth plan-repair pass, Task 1.3): `github/client_pr_by_number_live_test.go`,
  gated behind a dedicated `//go:build live_github` tag (deliberately not this repo's
  existing `integration` or `harness` tags — see plan.md Task 1.3 for why). Calls real
  `api.github.com` against the real, already-merged, publicly-visible PR #326. Not part of
  any automated `go test` invocation by design; run once manually before shipping (Task
  6.7's checklist), with its `-v` output pasted into the PR description/session notes.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

`github/client_pr_by_number_live_test.go` (Task 1.3) is intentionally **excluded** from
this coverage run and from every other automated `go test`/`make` target in this repo by
its `live_github` build tag — this matches plan.md's own framing of it as a deliberately
manual, out-of-CI check, not an omission from this coverage measurement. Its separate,
required-before-shipping invocation is
`go test -tags live_github -run TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326 -v ./github/...`
(see the Test Stack section above and the Requirement → Test Mapping table's live-GitHub
row) — it is not expected to, and does not need to, contribute to the ≥80% line target.

- All public service methods: happy path + error paths covered
- All external integrations: unit mocked (`backlogHandlers` seams, `SetPRByNumberFinder`)
  + at least one integration test (`httptest.Server`-backed `GetPRByNumber`/
  `VerifyPRMatchesBranch` tests in `github/client_pr_by_number_test.go` and
  `server/mcp/tools_github_test.go`)
- Verification command for this project specifically (per plan.md Tasks 4.6 / 6.7):
  `go build ./... && go test ./server/mcp/... ./github/... ./session/...`
