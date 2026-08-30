# Validation Plan: backlog-self-resolve

**Date**: 2026-08-02
**Scope**: `report_duplicate` MCP tool + `request_review` CAS generalization (FR1–FR10,
`project_plans/backlog-self-resolve/requirements.md`). Backend/MCP-only — no `design/ux.md`
exists (`research/ux.md` confirmed this feature has no user-facing surface), so the UX
Acceptance Tests step is skipped entirely, not just left blank.

**Note on a prior draft of this file**: an earlier version of this document (same filename,
same date) was written against an intermediate draft of `plan.md` and is superseded by this
version. That draft's gap list (G1–G8) predates `adversarial-review.md`'s iteration-2
re-verification; four of its gaps (length-cap test, `request_review` fail-closed test,
`report_duplicate` conservative-message-on-error test, cross-repo test) are now already closed
as tracked Phase 4 tasks (4.2.2e, 4.1.4c, 4.2.5b, 4.2.2f respectively) and are not repeated here.
This version re-derives the gap list by cross-checking the **current** `plan.md` (post
iteration-2 fixes) directly, task by task, against every FR's stated acceptance-criteria GWTs.

---

## Happy Path Scenario

Given a backlog item at `in_progress` status whose linked work-role session discovers that its
assigned work duplicates an already-shipped PR, when that session calls
`report_duplicate(item_id, duplicate_ref, reason)` with a `duplicate_ref` that GitHub
verification confirms exists, then the item transitions from `in_progress` to `review` (never
directly to `done`/`archived`), a `BacklogStatusEvent` audit row is recorded with
`TriggeredBy: "agent"` and a human-legible `Note`, the item session's `VerificationNotes` are
updated with the `duplicate_ref`/`reason`, and the caller receives a success message that
accurately states whether a live reviewer will see the evidence now or only on the next review
pass.

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| FR1: Generalize `request_review`'s CAS precondition to `in_progress`\|`pr_pending`, pinned to the validated observed status | `server/mcp/tools_backlog_test.go` | `TestRequestReview_TransitionsPRPendingItemToReview` (Task 4.1.2a) | Unit | `pr_pending`-sourced `request_review` succeeds; precondition built from `validStatus`, not a hardcoded constant |
| FR1 (cont.) | `server/mcp/tools_backlog_test.go` | `TestRequestReview_RejectsWhenSourceStatusNotAllowed` (Task 4.1.3a) | Unit | Table-driven over `{done, idea, review, archived}` — refused before any mutation |
| FR2: Refuse `request_review` on `pr_pending` source when an active review-role session exists; `in_progress` path unchanged | `server/mcp/tools_backlog_test.go` | `TestRequestReview_RejectsWhenActiveReviewSessionExists_AndSourceIsPRPending` (Task 4.1.4a) | Unit | `pr_pending` + active review session → refused, zero mutation |
| FR2 (cont.) | `server/mcp/tools_backlog_test.go` | `TestRequestReview_AllowsActiveReviewSession_WhenSourceIsInProgress` (Task 4.1.4b) | Unit | Same active-session setup but `in_progress` source → succeeds (guard is `pr_pending`-scoped only) |
| FR2 (cont., fail-closed on storage error) | `server/mcp/tools_backlog_test.go` | `TestRequestReview_FailsClosed_WhenListItemSessionsErrors` (Task 4.1.4c) | Unit | `ListItemSessions` error on the `pr_pending` path → `ErrInternalError`, not a silent pass-through |
| FR3: New `report_duplicate` tool; GitHub verification before any mutation; routes to `review` only | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_TransitionsInProgressItemToReview_WithVerifiedPR` (Task 4.2.1b) | Unit | PR ref, `in_progress` source, verified success path |
| FR3 (cont.) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_TransitionsPRPendingItemToReview_WithVerifiedIssue` (Task 4.2.1c) | Unit | Issue ref, `pr_pending` source |
| FR3 (cont.) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_TransitionsInProgressItemToReview_WithVerifiedCommit` (Task 4.2.1d) | Unit | Commit-SHA ref |
| FR3 (cont., pre-network parse/type validation — **gap: no Phase 4 task covers this**) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenDuplicateRefNotParseable` (**NEW**) | Unit | `duplicate_ref="not a url at all"` → `github.ParseGitHubRef` fails → `ErrInvalidArgument`, no network call (Story 3.2.1's first GWT / Task 3.2.1a — no corresponding test task exists anywhere in Epic 4.2) |
| FR3 (cont., **gap**) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenDuplicateRefIsUnsupportedRefType` (**NEW**) | Unit | `duplicate_ref` parses successfully but as `RefTypeBranch` (not PR/issue/commit) → `ErrInvalidArgument`, no network call (Story 3.2.1's second GWT — same gap) |
| FR4: Two-channel verification errors — definitive-invalid (`ErrInvalidArgument`, no retry) vs. transient (`ErrInternalError`, "retry" wording) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenGitHubRefNotFound` (Task 4.2.3a) | Unit | 404 → `ErrInvalidArgument` |
| FR4 (cont.) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_ReturnsRetryableError_WhenGitHubVerificationTimesOut` (Task 4.2.3b) | Unit | Network timeout → `ErrInternalError` with "retry" |
| FR4 (cont.) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_ReturnsRetryableError_WhenGitHubRateLimited` (Task 4.2.3c) | Unit | 429 → `ErrInternalError` with "retry" |
| FR4 (cont.) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenGitHubAccessDenied` (Task 4.2.3d) | Unit | Bare 403 (no rate-limit signal) → `ErrInvalidArgument` per ADR-002/UQ-2 |
| FR4 (cont., no-credentials case — **gap: promised in Task 3.2.2a's own text ("Add `TestReportDuplicate_ReturnsNonRetryableError_WhenNotAuthenticated` to Story 4.2.3") but Story 4.2.3's actual task list (4.2.3a–d) never includes it**) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_ReturnsNonRetryableError_WhenNotAuthenticated` (**NEW**, formalizes the promised-but-untracked test) | Unit | `errors.Is(verifyErr, githubpkg.ErrNotAuthenticated)` → `ErrInternalError` with explicit non-retryable/escalate-to-operator wording (Task 3.2.2b) |
| FR5: Accurate "when will this be seen" messaging | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_MessageSaysNextReviewPass_WhenReviewSessionActive` (Task 4.2.5a) | Unit | Active review session present → "next review pass" wording, never "Reviewer notified" |
| FR5 (cont.) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_MessageSaysNextReviewPass_WhenListItemSessionsErrors` (Task 4.2.5b) | Unit | `ListItemSessions` errors during message branching → conservative wording, transition still succeeds |
| FR5 (cont., positive branch — **gap: no task explicitly asserts the affirmative "Reviewer notified" message text**) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_MessageSaysReviewerNotified_WhenNoActiveReviewSession` (**NEW**) | Unit | No active review session → success text says "Reviewer notified" (Story 3.3.3's first GWT; Tasks 4.2.1b–d assert transition/audit fields but not this message text) |
| FR6: Refuse `SkipReviewGate`, non-`work` role, unlinked session, disallowed source status — zero mutation on every refusal path | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenSkipReviewGateEnabled` (Task 4.2.2a) | Unit | `SkipReviewGate: true` → refused before any GitHub call |
| FR6 (cont.) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenSessionRoleNotWork` (Task 4.2.2b) | Unit | Review-role caller → `ErrPermissionDenied` |
| FR6 (cont.) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenSessionNotLinked` (Task 4.2.2c) | Unit | Unlinked session → `ErrPermissionDenied` |
| FR6 (cont., 4th refusal condition layered on top per Pattern Decisions/ADR-005, not literally in FR6's 3-item list) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenSourceStatusNotAllowed` (Task 4.2.2d) | Unit | Table-driven over `{done, idea, review, archived}` |
| FR6 (cont., length caps, adversarial-review-added) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_RejectsWhenDuplicateRefOrReasonTooLong` (Task 4.2.2e) | Unit | Table over 501-char `duplicate_ref` / 1001-char `reason` → `ErrInvalidArgument`, no GitHub call |
| FR7: Every transition audited via `BacklogStatusEvent`/`recordStatusEvent` with `TriggeredBy="agent"`; `duplicate_ref`/`reason` persisted into `VerificationNotes` | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_TransitionsInProgressItemToReview_WithVerifiedPR` (Task 4.2.1b — assertions on `TriggeredBy=="agent"`, `BacklogStatusEvent.Note`, `VerificationNotes`) | Unit | Success test doubles as the audit-trail assertion per its own AC text |
| FR7 (cont., append-not-overwrite behavior — **gap: Task 3.3.2a's "append rather than overwrite prior VerificationNotes" fix has no test anywhere in Phase 4**) | `server/mcp/tools_backlog_test.go` | `TestReportDuplicate_PreservesExistingVerificationNotes_WhenAppendingNewEntry` (**NEW**) | Unit | Seed a work-role `ItemSession` with non-empty `VerificationNotes` (e.g. left over from an earlier `request_review` call before a rework cycle), call `reportDuplicate`, assert the persisted notes contain both the prior content and the new `duplicate_ref=... reason=...` entry separated by the `\n\n---\n\n` delimiter (Task 3.3.2a) |
| FR7 (cont., `TriggeredByAgent` constant) | `session/backlog.go` | n/a — compile-only (Task 1.1.1a), plain untyped string const, no exhaustiveness switch | Build check | `go build ./...` succeeds after adding `TriggeredByAgent = "agent"` |
| FR7 (cont., `request_review`-side audit attribution — **gap: Task 2.1.1c's switch from `TriggeredBySystem` to `TriggeredByAgent` had zero test coverage**; closed by extending an existing task rather than adding a new one) | `server/mcp/tools_backlog_test.go` | `TestRequestReview_TransitionsPRPendingItemToReview` (Task 4.1.2a, extended) | Unit | Asserts the resulting `BacklogStatusEvent.TriggeredBy == "agent"`, not just `Status == "review"` |
| FR8: No schema changes; `go build ./...` succeeds with zero `ent generate` runs | n/a (verification story, no production code) | Story 4.4.1 / Task 4.4.1a — `go build ./... && make test && make lint` | Build/CI gate | `git status session/ent/` shows zero diff — confirms no `BacklogStatus` enum value, ent schema field, or migration introduced |
| FR9: Existing `request_review` suite passes unmodified for the `in_progress`-sourced path | `server/mcp/tools_backlog_test.go` | `TestRequestReview_TransitionsItemToReview`, `_TransitionsDirectlyToDone_When_SkipReviewGateEnabled`, `_PersistsVerificationNotesOnWorkSession`, `_RejectsVerificationNotesOver4000Chars`, `_RejectsWhenSessionNotLinked` (Story 4.1.1 / Task 4.1.1a) | Unit (regression) | `go test ./server/mcp/... -run TestRequestReview -v` — zero edits to these 5 functions |
| FR10 (part 1): `report_duplicate`'s tool description gives explicit retry guidance for `INTERNAL_ERROR` — **gap: Story 3.4.1/Task 3.4.1a state this as an AC but no Phase 4 task verifies the registered description text**, despite an established in-repo precedent for exactly this kind of test | `server/mcp/tools_backlog_test.go` | `TestRegisterBacklogTools_ReportDuplicate_DescribesRetryGuidance` (**NEW**) | Unit (source-scan) | Mirrors `TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement` (`tools_backlog_test.go:1060` — `os.ReadFile("tools_backlog.go")` + `assert.Contains`); asserts the `report_duplicate` registration block contains "retry" in the `INTERNAL_ERROR` context plus the non-retryable no-credentials escalation line (Task 3.4.1a) |
| FR10 (part 2): stuck `pr_pending` items eventually surface via the existing stuck-item notification path | `session/backlog_lifecycle_test.go` (pre-existing, not part of this feature's diff) | N/A — no new test needed | N/A | Per Observability Plan's corrected citation (adversarial-review.md iteration-2, item 1, verified directly against `session/backlog_lifecycle.go:3850`): `ReconcilePRPending` already covers any `pr_pending` item with a real PR reference regardless of *why* it's stuck, and is exercised by its own pre-existing test suite. This feature adds no new detector/`StuckReason` and needs no new test for this half of FR10. |

**Coverage**: 10/10 FRs have at least one test mapped (existing-planned or gap-fill) or an
explicit non-test verification mechanism (build gate for FR8, regression run for FR9,
pre-existing detector suite for FR10 part 2).

---

## Gaps Found and Closed

Cross-checking every Phase 4 task in the **current** `plan.md` (post iteration-2 fixes) against
every FR's acceptance-criteria GWTs — not assuming the plan's own "Phase 4 is thorough" framing —
surfaced five requirement-level GWTs with no corresponding tracked Phase 4 task:

1. **FR3** — Story 3.2.1's two pre-network parse/ref-type-validation refusal GWTs (malformed URL;
   unsupported ref type, e.g. a branch URL) have no test anywhere in Epic 4.2. Added
   `TestReportDuplicate_RejectsWhenDuplicateRefNotParseable` and
   `TestReportDuplicate_RejectsWhenDuplicateRefIsUnsupportedRefType`.
2. **FR4** — Task 3.2.2b's own text says "Add `TestReportDuplicate_ReturnsNonRetryableError_WhenNotAuthenticated`
   to Story 4.2.3," but Story 4.2.3's actual task list (4.2.3a–d) never includes it. Added it as a
   tracked test.
3. **FR5** — the affirmative "Reviewer notified" (no active session) message text from Story
   3.3.3's first GWT is never explicitly asserted; Tasks 4.2.1b–d check transition/audit-field
   outcomes but not this message string. Added
   `TestReportDuplicate_MessageSaysReviewerNotified_WhenNoActiveReviewSession`.
4. **FR7** — Task 3.3.2a's "append, don't overwrite prior `VerificationNotes`" fix (added
   specifically so a `report_duplicate` call doesn't discard notes left by an earlier
   `request_review` call on the same session) has no test anywhere in Phase 4. Added
   `TestReportDuplicate_PreservesExistingVerificationNotes_WhenAppendingNewEntry`.
5. **FR10** (part 1) — Story 3.4.1's "description contains 'retry'" acceptance criterion has no
   verifying test, despite an established in-repo precedent for exactly this pattern
   (`TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement`).
   Added `TestRegisterBacklogTools_ReportDuplicate_DescribesRetryGuidance`.

**Superseded gaps** (present in an earlier draft of this document, now already closed as tracked
Phase 4 tasks in the current plan.md — not repeated as open gaps here):
- Length-cap test → now Task 4.2.2e.
- `request_review` fail-closed-on-storage-error test → now Task 4.1.4c.
- `report_duplicate` conservative-message-on-storage-error test → now Task 4.2.5b.
- Cross-repo `duplicate_ref` test → now Task 4.2.2f, with an explicit "cross-repo is allowed"
  decision recorded alongside it.
- "Never calls GitHub verification on a refusal path" — no longer needs a separate new test
  function; Epic 3.1's Goal text (added per adversarial review) now requires every one of Tasks
  4.2.2a–e to construct `verifyGitHubRef` as a `t.Fatal`-on-call stub, folding the safeguard into
  the existing refusal tests rather than adding a ninth one.

**Minor, non-blocking recommendation** (not counted as a gap): Tasks 4.2.3a–d (FR4's GitHub-error
tests) assert the returned error text/code but their task descriptions don't explicitly call out
a `GetBacklogItem`-unchanged assertion, unlike Stories 3.1.1/3.1.2's refusal GWTs which do. This
is structurally guaranteed regardless (the transition call is unreached when verification returns
an error — the code never gets there), so it's a one-line assertion worth adding to those 4 tasks
at implementation time for symmetry with the FR6 refusal tests, not a missing-coverage gap.

---

## Test Stack

- **Unit**: Go stdlib `testing` + `github.com/stretchr/testify` (`require`/`assert` — confirmed
  in `server/mcp/tools_backlog_test.go`'s import block) + this repo's existing temp-dir,
  ent/sqlite-backed `session.Storage` test double (`newTestBacklogStorage(t)`,
  `tools_backlog_test.go:21`) for `server/mcp` handler tests — not a mocked repository. The only
  mocked seam is the injectable `verifyGitHubRef`/`verifyPRMatchesBranch` func-value fields on
  `backlogHandlers`, standing in for the GitHub network boundary specifically (the same seam
  `report_pr_created`'s existing tests already use).
- **Integration**: The `github` package tests (Tasks 1.2.3b/1.2.3c: `github/commits_test.go`,
  `github/repos_pr_test.go`) are integration-shaped even though they run as plain `go test` — they
  exercise the real HTTP request/status-code-classification path against an `httptest.Server`
  (via the `resetGhBaseURL` helper pattern borrowed from
  `server/services/backlog_github_rpc_test.go:19-22`), not a stubbed function value. This is the
  layer that actually proves `GetPR`/`GetCommit`/the retrofitted `GetIssue` correctly classify
  200/404/401/403(±Retry-After)/429 into the right sentinel or plain-transient error, rather than
  the MCP handler's re-labeling of it.
- **E2E / UX**: N/A — no user-facing surface. `research/ux.md` confirmed backend/MCP-tool scope
  only; no `design/ux.md` was produced or needed.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line for `server/mcp/tools_backlog.go` and `github/{repos,commits}.go` — the files this feature touches. (Repo-standard `make test-coverage` runs the same underlying command as an HTML report; the Makefile enforces no hard numeric gate, so this is reviewed, not CI-blocking, per existing convention.) |

- Every public/handler function this feature adds or modifies (`reportDuplicate`,
  `requestReview`'s generalized path, `validateSelfResolveSource`, `verifyGitHubRefExists`,
  `GetPR`, `GetCommit`, retrofitted `GetIssue`) has both a happy path and every documented error
  path covered — see the mapping table above.
- Every external integration (GitHub HTTP calls) is covered at two layers: unit-mocked at the
  MCP-handler layer (func-value seam) **and** integration-shaped against a real `httptest.Server`
  at the `github`-package layer for each new/changed function (`GetPR`, `GetCommit`; `GetIssue`'s
  sentinel retrofit should get an equivalent table-case addition to its existing test at
  implementation time if one doesn't already exist).

## Migration Test

**N/A.** This feature makes no schema or ent-migration changes (FR8, ADR-001 — duplicate outcomes
are represented via the existing `review` `BacklogStatus` plus the free-text `VerificationNotes`/
`BacklogStatusEvent.Note` fields, not a new enum value or column). Story 4.4.1/Task 4.4.1a is the
explicit build-level check standing in for a migration test: `go build ./...` must succeed with
zero `ent generate` invocations, and `git status session/ent/` must show zero diff.

---

## Summary

- **Total planned test functions in the current plan.md's Phase 4** (recounted directly from
  task names, not estimated): **31** — Epic 4.1 (`request_review`): 11 total (5 pre-existing
  regression tests reverified by Task 4.1.1a, plus 6 new: `TestRequestReview_TransitionsPRPendingItemToReview`,
  `_RejectsWhenSourceStatusNotAllowed`, `_RejectsWhenActiveReviewSessionExists_AndSourceIsPRPending`,
  `_AllowsActiveReviewSession_WhenSourceIsInProgress`, `_FailsClosed_WhenListItemSessionsErrors`,
  `_ReportsDistinctMessage_WhenCASPreconditionFails`). Epic 4.2 (`report_duplicate`): 18 new named
  test functions across Stories 4.2.1–4.2.6 (3 + 6 + 4 + 2 + 2 + 1). Epic 4.3: 2 `github`-package
  table tests (Tasks 1.2.3b `GetCommit`, 1.2.3c `GetPR`), each covering ~5–6 HTTP-status cases.
- **Gap-fill tests added by this validation pass**: 5 (`TestReportDuplicate_RejectsWhenDuplicateRefNotParseable`,
  `_RejectsWhenDuplicateRefIsUnsupportedRefType`, `_ReturnsNonRetryableError_WhenNotAuthenticated`,
  `_MessageSaysReviewerNotified_WhenNoActiveReviewSession`,
  `_PreservesExistingVerificationNotes_WhenAppendingNewEntry`) plus 1 tool-registration test
  (`TestRegisterBacklogTools_ReportDuplicate_DescribesRetryGuidance`) — 6 total.
- **Requirements coverage**: 10/10 FRs mapped to a test (existing-planned or gap-fill) or an
  explicit non-test verification mechanism (build gate for FR8, regression run for FR9,
  pre-existing detector suite for FR10 part 2).
- **Migration test**: N/A — no schema/migration changes (FR8, ADR-001); verified via
  `go build ./...` succeeding without `ent generate` (Task 4.4.1a), not a migration test.
