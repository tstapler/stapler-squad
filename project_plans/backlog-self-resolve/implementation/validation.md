# Validation Plan: backlog-self-resolve

**Date**: 2026-08-02
**Scope**: `report_duplicate` MCP tool + `request_review` CAS generalization (FR1–FR10,
`project_plans/backlog-self-resolve/requirements.md`). Backend/MCP-only — no `design/ux.md`
exists (confirmed: no user-facing UI surface), so the UX Acceptance Tests section is skipped.
No schema/migration changes (FR8, ADR-001), so the migration-test step is skipped — see
Coverage Targets for how FR8 is actually verified instead.

This document does not redesign the test suite from scratch — `implementation/plan.md`'s
Phase 4 (Epics 4.1–4.3) already specifies ~20 named test functions with GWTs per AC. This
document maps those planned tests to FR1–FR10, confirms every FR has coverage, and lists the
genuine gaps found by cross-checking plan.md against `architecture-review.md` and
`adversarial-review.md` (both dated 2026-08-02) that are **not yet** reflected as Phase 4 tasks.

---

## Happy Path Scenario

Given backlog item `da58b867-bf4e-4720-8fe4-9cfcfa5b6eed` at status `in_progress` with a linked
work-role `ItemSession`, when that session calls `report_duplicate` with a GitHub-verified PR
reference (`https://github.com/tstapler/stapler-squad/pull/272`) and a reason, then the
reference is verified against GitHub *before* any mutation, the item transitions to `review`
status with `TriggeredBy="agent"`, `duplicate_ref`/`reason` are persisted into the item
session's verification notes, and the success message correctly states whether an active
reviewer will see the evidence now or only on the next review pass.

---

## Requirement → Test Mapping

| FR | Requirement (short) | Unit — happy | Unit — error | Integration | Notes / gaps |
|----|---|---|---|---|---|
| FR1 | Generalize `request_review` CAS to `{in_progress, pr_pending}` | `TestRequestReview_TransitionsItemToReview` (existing, in_progress) + **new** `TestRequestReview_TransitionsPRPendingItemToReview` (Task 4.1.2a) | **new** `TestRequestReview_RejectsWhenSourceStatusNotAllowed` (Task 4.1.3a, table over `done`/`idea`/`review`/`archived`) | Inherent — all `tools_backlog_test.go` tests run against a real ent-backed sqlite `session.Storage` (`newTestBacklogStorage`, not a mock), so the CAS precondition is exercised against the real repository layer, not stubbed | Regression floor: `TestRequestReview_*` (5 existing tests, Task 4.1.1a) must pass unmodified |
| FR2 | Refuse `request_review` re-route when active review session + source=`pr_pending` | **new** `TestRequestReview_AllowsActiveReviewSession_WhenSourceIsInProgress` (Task 4.1.4b — confirms guard is scoped, not "always refuse") | **new** `TestRequestReview_RejectsWhenActiveReviewSessionExists_AndSourceIsPRPending` (Task 4.1.4a) | Same real-storage inherent coverage as FR1 | **GAP** — see G4 below: the `ListItemSessions`-errors-fails-open branch (adversarial-review.md concern) has no test and, per that review, arguably wrong behavior |
| FR3 | New `report_duplicate` tool, GitHub-verify before mutation, routes to `review` only | **new** `TestReportDuplicate_TransitionsInProgressItemToReview_WithVerifiedPR` / `_WithVerifiedIssue` (4.2.1c) / `_WithVerifiedCommit` (4.2.1d) | **new** `TestReportDuplicate_RejectsWhenGitHubRefNotFound` (4.2.3a) | `github` package httptest.Server tests (Tasks 1.2.3b/1.2.3c) exercise the real HTTP round trip (status codes, JSON shape) that `GetPR`/`GetCommit`/`GetIssue` depend on — the closest thing this codebase has to an "external call" integration test (no live network call; the repo has no `-tags integration` GitHub-touching tests) | **GAP** — see G8 below: verification-failure tests (4.2.3a–d) assert the returned error but not that `GetBacklogItem` afterward shows the item unchanged, unlike the FR6 refusal tests which do assert this explicitly |
| FR4 | Two-channel errors: `ErrInvalidArgument` (no retry) vs `ErrInternalError` ("retry" wording) | *(happy path is FR3's)* | **new** `TestReportDuplicate_RejectsWhenGitHubRefNotFound` (404→`ErrInvalidArgument`, 4.2.3a), `_RejectsWhenGitHubAccessDenied` (bare 403→`ErrInvalidArgument`, 4.2.3d), `_ReturnsRetryableError_WhenGitHubVerificationTimesOut` (4.2.3b), `_ReturnsRetryableError_WhenGitHubRateLimited` (4.2.3c) | Same `github` package httptest tests classify 401/403(no Retry-After)/403(w/ Retry-After)/404/429 via `errors.Is` against the new sentinels — this is where the classification logic itself is proven, not just the handler's re-labeling of it | None — well covered |
| FR5 | Accurate "next review pass" vs "reviewer notified" messaging | **new** `TestReportDuplicate_MessageSaysNextReviewPass_WhenReviewSessionActive` (4.2.5a) covers the active-session branch | *(no dedicated "no active session ⇒ 'Reviewer notified'" assertion — folded implicitly into 4.2.1b's success check, but its GWT as written only asserts transition succeeded, not message text)* | Inherent (real storage) | **GAP** — see G5 below: `ListItemSessions`-errors branch defaults to the *optimistic* message per current plan code sketch (Task 3.3.3a), which is the literal thing FR5 forbids — no test, and adversarial-review.md flags this as a likely-wrong default |
| FR6 | Refuse (zero mutation): `SkipReviewGate`, role≠work, session not linked [, disallowed source status] | *(happy path is FR3's — refusal absent)* | **new** `TestReportDuplicate_RejectsWhenSkipReviewGateEnabled` (4.2.2a), `_RejectsWhenSessionRoleNotWork` (4.2.2b), `_RejectsWhenSessionNotLinked` (4.2.2c), `_RejectsWhenSourceStatusNotAllowed` (4.2.2d, table over 4 statuses) — each asserts `GetBacklogItem` unchanged afterward | Same real-storage zero-mutation verification | **GAPs** — see G2 (length-cap test, adversarial-flagged, absent from Phase 4) and G3 (no test proves the GitHub call is never made on a refusal — adversarial-flagged) |
| FR7 | Audit trail: `TriggeredBy="agent"`, `duplicate_ref`/`reason` in `VerificationNotes` | Folded into Task 4.2.1b's AC ("`VerificationNotes` contains the ref, `BacklogStatusEvent.Note` is populated, `TriggeredBy == "agent"`") — no separate test needed, same assertion block | *(N/A — audit trail has no distinct error path beyond FR6's zero-mutation guarantee, which already proves no event is written on refusal)* | Real `BacklogStatusEvent`/ent-backed read-back, not a mock | **GAP** — see G7 below: Task 3.3.2a's "append, don't overwrite prior `VerificationNotes`" behavior (added specifically to avoid discarding evidence from an earlier `request_review` call) has no test anywhere in Phase 4 |
| FR8 | No schema changes; `go build ./...` succeeds without `ent generate` | N/A — not a unit test | N/A | N/A | Verified by **Task 5.0a** (Final Verification): `go build ./... && make test && make lint`, plus `git status session/ent/` showing zero diff. This is a build/CI gate, not a named test function — matches the instruction that FR8 is proven this way, not via a unit test |
| FR9 | Existing `request_review` suite passes unmodified | N/A — regression check, not a new test | N/A | N/A | Verified by **Task 4.1.1a**: `go test ./server/mcp/... -run TestRequestReview -v` after Phase 2 lands, confirming the 5 pre-existing tests (`TestRequestReview_TransitionsItemToReview`, `_TransitionsDirectlyToDone_When_SkipReviewGateEnabled`, `_PersistsVerificationNotesOnWorkSession`, `_RejectsVerificationNotesOver4000Chars`, `_RejectsWhenSessionNotLinked`) pass with zero edits |
| FR10 | (a) tool description has explicit "retry" guidance for `INTERNAL_ERROR`; (b) stuck `pr_pending` items eventually surface | *(a: no test)* *(b: N/A, existing code)* | *(a: no test)* | (b) `ReconcilePRPending` (`session/backlog_lifecycle.go:3850`) already has existing test coverage in `session/backlog_lifecycle_test.go` and `session/backlog_lifecycle_stuck_test.go` (confirmed via grep — independent of this feature's diff) — architecture-review.md verified this detector correctly covers FR10(b)'s scenario (a `pr_pending` item *with* a real PR reference), superseding adversarial-review.md's original BLOCKED verdict, which had cited the wrong detector (`pr_pending_no_pr`) | **GAP** — see G1 below: FR10(a) (the tool-description "retry" wording, Story 3.4.1's own AC) has **no corresponding Phase 4 test task at all** — contrast with the existing precedent `TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement` (`tools_backlog_test.go:1060`) for the sibling tool |

**Coverage**: 10/10 FRs have at least one planned test or an explicit non-test verification
mechanism (build gate for FR8, regression run for FR9, pre-existing detector test suite for
FR10b). Every FR has a documented path to verification; the gaps below are refinements/additions
to already-adequate coverage, not missing coverage entirely.

---

## Gaps Found (not yet in plan.md's Phase 4 task list)

These come from cross-referencing plan.md's Phase 4 against `architecture-review.md` (Concerns)
and `adversarial-review.md` (Concerns — its one Blocker, FR10, was independently resolved by
architecture-review.md and is not repeated here). None of these are structural blockers; all are
addable as new test tasks within Epic 4.1/4.2's existing shape.

| ID | Gap | FR | Proposed test | Source |
|----|---|---|---|---|
| G1 | FR10(a)'s tool-description "retry" wording has zero test coverage | FR10 | `TestRegisterBacklogTools_ReportDuplicate_DescribesRetryGuidance` — mirror the existing `TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement` pattern; assert the registered tool's description contains "retry" in the `INTERNAL_ERROR` context | Cross-check: Story 3.4.1 has an AC, Epic 4.1/4.2 has no matching task |
| G2 | No test for `duplicate_ref`/`reason` exceeding their 500/1000-char caps | FR6 | `TestReportDuplicate_RejectsWhenDuplicateRefOrReasonTooLong` (table over both fields), mirroring the existing sibling `TestRequestReview_RejectsVerificationNotesOver4000Chars` | adversarial-review.md Concerns |
| G3 | No test proves the refusal-check order (SkipReviewGate → role/link → whitelist → GitHub verify); the existing 4.2.2a–d tests only check the returned error, not that `h.verifyGitHubRef` was never invoked | FR6 | `TestReportDuplicate_NeverCallsGitHubVerification_WhenRefused` — set `h.verifyGitHubRef` to a stub that calls `t.Fatal` if invoked, run it against each of the 4 refusal fixtures | adversarial-review.md Concerns |
| G4 | `TestRequestReview_RejectsWhenActiveReviewSessionExists_AndSourceIsPRPending` proves the guard fires when `ListItemSessions` succeeds; no test covers the `ListItemSessions`-errors path, which the current plan code (Task 2.2.1b) silently falls through to "no active reviewer" | FR2 | `TestRequestReview_FailsClosed_WhenListItemSessionsErrors_OnPRPendingPath` — requires a corresponding plan/code change (fail closed with `ErrInternalError`, not fall through) per adversarial-review.md's explicit recommendation; write the test alongside that fix, not as a test of current (silently-wrong) behavior |
| G5 | Same swallowed-error shape on the messaging side: Task 3.3.3a's `itemSessions, _ := h.storage.ListItemSessions(...)` defaults to the optimistic "Reviewer notified" message on error, which is the literal thing FR5 forbids | FR5 | `TestReportDuplicate_MessageDefaultsToNextReviewPass_WhenListItemSessionsErrors` — again pairs with a plan/code fix (default to the conservative message on error), not a test of current behavior | adversarial-review.md Concerns |
| G6 | Cross-repo `duplicate_ref` (pointing at a different `owner/repo` than the item's own) is implicitly allowed by the current design but never decided or tested | FR3 | Needs an explicit decision first (allow vs. same-repo-only); once decided, `TestReportDuplicate_AllowsCrossRepoDuplicateRef` or `_RejectsCrossRepoDuplicateRef` accordingly | adversarial-review.md Concerns |
| G7 | Task 3.3.2a's "append, don't overwrite prior `VerificationNotes`" behavior (added to avoid discarding an earlier `request_review` call's notes) has no test | FR7 | `TestReportDuplicate_PreservesExistingVerificationNotes_WhenAppendingNewEntry` — seed a work-role `ItemSession` with non-empty `VerificationNotes` (e.g. from a prior `request_review`), call `reportDuplicate`, assert the persisted notes contain both the old content and the new `duplicate_ref=...` entry | Cross-check: Story 3.3.2 has an AC for the happy "notes contain the ref" case (folded into 4.2.1b) but not for the append-preserves-prior-content case |
| G8 | Verification-failure tests (4.2.3a–d) assert the returned error only; none assert `GetBacklogItem` shows the item unchanged, unlike the FR6 refusal tests which do | FR3 | Extend Tasks 4.2.3a–d's existing assertions to add a `GetBacklogItem` unchanged-status check — not a new test function, a one-line addition to each of the 4 existing tasks | Cross-check against FR3's literal "verified... before any state mutation" text |

**Not carried forward as a test gap** (process/design items only, no test implication):
- ADR-003/ADR-004 still `Status: Proposed` rather than `Accepted` — a sign-off gap, not a test gap (adversarial-review.md Concerns).
- The CAS-trap "3 independently-reviewed call sites" concern (adversarial-review.md) recommends a single `validateSelfResolveSource` helper to structurally close the risk — a refactor recommendation, not a missing test; the existing per-site tests (4.1.3a, 4.2.2d) already catch a regression at any one site today.
- The read-then-append race on `VerificationNotes` under true concurrent callers (adversarial-review.md) is flagged as an accepted low-probability tradeoff given the single-threaded-per-session tool-call model — no test recommended by that review either.
- Task 4.2.6a's rewritten construction (already corrected in the plan.md text read for this validation pass, per its own "Corrected per adversarial review" annotation) still carries a stale name, `TestReportDuplicate_LoserGetsDistinctMessage_WhenRacingReportPRCreated`, that contradicts its own now-corrected description. Rename to `TestReportDuplicate_RefusedAfterAlreadyTransitionedToReview` per architecture-review.md's nitpick — a rename, not a new test.
- Task 4.1.5a's stated "fallback: unit-test the branch logic in isolation" is not buildable (nothing extracts `errors.Is(transErr, session.ErrPreconditionFailed)` into a separately-callable function per the current plan). Commit to the primary "seed at in_progress, force a stale precondition via a direct `TransitionBacklogItemStatus` call with a mismatched `ExpectedStatus`" construction; drop the fallback sentence from the task description.

---

## Test Naming Convention

`server/mcp/tools_backlog_test.go` actually has two coexisting conventions (checked directly,
not assumed):
1. **`TestX_VerbPhrase`** — the majority convention, e.g. `TestRequestReview_TransitionsItemToReview`,
   `TestReportProgress_RejectsWhenNoSessionUUID`, `TestGetBacklogItem_ReturnsNotFoundError`.
2. **`TestX_should_VerbPhrase_When_Condition`** (lowercase `should`/`When`) — used only by the
   `report_pr_created` sibling tests, e.g. `TestReportPRCreated_should_TransitionToPRPending_When_ValidPR`.

Every new test named in plan.md's Phase 4 (`TestRequestReview_*`, `TestReportDuplicate_*`) uses
convention **#1** — correct, since it matches both the file's dominant style and the existing
`request_review` tests these new ones sit alongside. This validation.md's proposed gap-filling
tests (G1–G8) follow the same convention for consistency.

---

## Test Stack

- **Framework**: Go stdlib `testing` + `testify` (`assert`/`require`) — no other test framework
  is used in this codebase.
- **Backend/MCP layer**: `go test ./server/mcp/...` — handler tests run against a real,
  temp-file-backed ent/sqlite `session.Storage` (`newTestBacklogStorage`, `tools_backlog_test.go:21`),
  not a mocked repository; the only mocked seam is the injectable `verifyGitHubRef`/
  `verifyPRMatchesBranch` func-value fields on `backlogHandlers`, which stand in for the network
  boundary specifically (the same seam `report_pr_created`'s existing tests already use).
- **GitHub package layer**: `go test ./github/...` — new `github/commits_test.go` and
  `github/repos_pr_test.go` (or `repos_test.go`) use `httptest.Server` + the `resetGhBaseURL`
  helper pattern (borrowed from `server/services/backlog_github_rpc_test.go:19-22`, the first
  precedent for this pattern to live inside the `github` package itself) to exercise the real
  HTTP status-code → sentinel-error classification without a live network call.
- **Full local run**: `make build && make test` (generates protos, then runs `go test -short ./...`);
  `make quick-check` adds coverage + race + lint for a fast pre-push gate; `make ci` is the
  definitive pre-push check (`build`, `test`, `test-race`, `vet`, `lint`, `lint-css-tokens`,
  `test-integration`, `fmt-check`, `registry-generate`, `actor-field-guard`).
- **This feature has no `-tags integration`-gated tests** — the repo's `test-integration` target
  (`go test -race -tags integration ./...`) exists for real-tmux-dependent session tests
  elsewhere in the codebase; nothing in this feature's diff needs that tag. The closest analog
  to "integration with an external call" is the `httptest.Server`-backed `github` package tests
  described above.

## Coverage Targets

- `make test-coverage` (`TMUX_BIN=... go test -short -cover ./... -coverprofile=coverage.out`,
  then `go tool cover -html=coverage.out -o coverage.html`) is this repo's standard coverage
  command — no numeric threshold is enforced by the Makefile itself (no `go tool cover -func`
  percentage gate found in `test-coverage`, `coverage-func`, or `coverage-gaps` targets); coverage
  is reviewed via the generated HTML/func report, not a hard CI gate.
- Practical target for this feature: every new exported function (`GetPR`, `GetCommit`,
  `verifyGitHubRefExists`, `reportDuplicate`, the modified `requestReview`) should show 100% or
  near-100% statement coverage in `coverage-func`'s output given the ~22 planned tests (Phase 4)
  plus the 8 gap-fill tests (G1–G8) proposed above — every branch enumerated in plan.md's GWTs
  (whitelist pass/reject, all 4 refusal reasons, 4-way GitHub error classification, idempotency
  hit/miss, active-session message branch) has a corresponding test once the gaps are filled.
- FR8's "no `ent generate` run" and FR9's "existing suite passes unmodified" are verified as
  build/regression gates (Tasks 5.0a and 4.1.1a respectively), not coverage-percentage targets.

---

## Summary

- **Total planned test functions (plan.md Phase 4, as written)**: 22 — 5 in Epic 4.1
  (`request_review`), 15 in Epic 4.2 (`report_duplicate`), 2 in Epic 4.3 (`github` package,
  each table-driven over ~5–6 HTTP-status cases).
- **Proposed additional tests to close gaps (G1–G8)**: 8 — see table above; 6 are net-new test
  functions (G1, G2, G3, G4, G6, G7), 1 is a modification to an existing planned test (G8, extend
  4.2.3a–d's assertions), 1 pairs a test with a required behavior fix rather than testing current
  behavior as-is (G5).
- **Requirements coverage**: 10/10 FRs have a mapped test or an explicit non-test verification
  mechanism (build gate / regression run / pre-existing detector suite) — see the Requirement →
  Test Mapping table. Two FRs (FR8, FR9) are intentionally verified via build/CI gates rather than
  named unit tests, per the plan's own design.
- **Migration test**: N/A — no schema/migration changes (FR8, ADR-001); verified via `go build ./...`
  succeeding without `ent generate` (Task 5.0a), not a migration test.
