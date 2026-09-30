# Validation Plan: github-call-optimization

**Date**: 2026-09-09

## Happy Path Scenario

Given a background poller call (`OriginPRStatusPoller`) has driven `DefaultRateLimiter`'s
published `RateLimiterSnapshot` below the reserved background headroom on the same process
(Baseline: "Interactive action failed live" — `requirements.md`), when Tyler performs an
interactive GitHub action (`GetPRInfo`, `MergePR`, etc.) with `github:priority-admission-control`
flipped on, then the interactive call still succeeds — proving `AdmitOrigin`'s reserved-headroom
gate protects interactive traffic from same-process background exhaustion (Success Metric #1).
This is the compound, cross-origin scenario every Phase 3 unit test in the plan stops short of —
closed explicitly below (REQ-2, Integration row 3).

## Requirement → Test Mapping

Requirements are the 5 `## Scope → In Scope` items in `requirements.md`, each mapped to its
corresponding plan.md Phase. Phase 6 (frontend error copy) is UX-derived from Phases 3/4's new
rate-limit-reason surfacing rather than its own numbered scope item; it is covered under the UX
Acceptance Tests section below plus a REQ-2/REQ-3-adjacent row here for its one non-UI unit.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: Observability (Phase 1, Epic 1.1 — origin/ctx plumbing) | `github/call_origin_test.go` | `TestGitHubCallOriginFrom_should_DefaultToPRStatusPoller_When_ContextUntagged` | Unit | Happy path — untagged ctx defaults to background tier per ADR-002 |
| REQ-1: Observability (Phase 1, Epic 1.1) | `github/call_origin_test.go` | `TestWithGitHubCallOrigin_should_RoundTripExactly_When_TaggedWithEachOfFiveConstants` | Unit | Happy path — table test over all 5 `CallOrigin` constants |
| REQ-1: Observability (Phase 1, Epic 1.1) | `session/pr_tracking_test.go` | `TestRefreshPRInfo_should_CallGetPRInfoCtxWithCallersContext_When_ContextTaggedInteractive` | Unit | Error-adjacent — asserts the ctx a caller tags actually reaches `github.GetPRInfoCtx` unmodified (regression for the plumbing refactor breaking silently and losing the tag) |
| REQ-1: Observability (Phase 1, Epic 1.2 — HTTP transport telemetry) | `github/telemetry_transport_test.go` | `TestGithubTelemetryTransport_should_RecordCallsTotalAndCacheHitMetric_When_ResponseIs304` | Unit | Happy path — 304 increments `github.cache.result_total{result="hit"}` |
| REQ-1: Observability (Phase 1, Epic 1.2) | `github/telemetry_transport_test.go` | `TestGithubTelemetryTransport_should_RecordSpanWithAdmissionSkipTrue_When_RateLimiterAlreadyLimited` | Unit (error path) | `IsLimited()==true` fail-fast still produces a span with `github.admission_skip=true` instead of vanishing silently (plan Task 1.2.1a's explicit AC) |
| REQ-1: Observability (Phase 1, Epic 1.3 — gh-CLI instrumentation) | `github/gh_exec_test.go` | `TestRunGHCLICommand_should_EmitSpanWithOriginAndCallSite_When_ExecSucceeds` | Unit | Happy path — span attributes populated, `exec()`'s return value passed through unchanged |
| REQ-1: Observability (Phase 1, Epic 1.3) | `github/gh_exec_test.go` | `TestRunGHCLICommand_should_PassThroughErrorUnwrapped_When_ExecFails` | Unit | Error path — `runGHCLICommand` must not wrap/mask `exec()`'s error (plan's explicit "no error wrapping" AC) |
| REQ-1: Observability (Phase 1, Epic 1.3) | `github/client_test.go` | `TestGetPRInfoCtx_should_EmitGhPrViewSpanThroughRunGHCLICommand_When_CalledWithTaggedContext` | Integration | External call (real `gh` subprocess invocation path, mocked at the `safeexec.CommandContext` boundary) — confirms the wrapping doesn't change `GetPRInfoCtx`'s existing behavior, only adds telemetry |
| REQ-2: Priority-aware admission control (Phase 3) | `github/rate_limit_test.go` | `TestAdmitOrigin_should_AdmitInteractive_When_RemainingBelowBackgroundHeadroom` | Unit | Happy path — `OriginInteractive` always admitted while `!IsLimited()`, regardless of headroom |
| REQ-2: Priority-aware admission control (Phase 3) | `github/rate_limit_test.go` | `TestAdmitOrigin_should_RejectBackgroundOrigin_When_RemainingBelowBackgroundHeadroomPercent` | Unit | Error path — background origin rejected with `"background headroom reserved"` once `Remaining < Limit*backgroundHeadroomPercent/100` |
| REQ-2: Priority-aware admission control (Phase 3) | `github/http_client_test.go` | `TestRateLimitTransport_should_AdmitInteractiveCall_When_BackgroundOriginExhaustedSameRateLimiter` | **Integration** | **Closes the adversarial review's named gap** — see "Compound Two-Origin Test" section below for full design |
| REQ-2: Priority-aware admission control (Phase 3) | `github/rate_limit_test.go` | `TestAdmitOrigin_should_AdmitBackgroundOrigin_When_SnapshotNeverPublished` | Unit (error/edge path) | Adversarial review's zero-value-snapshot gap — cold-start `Snapshot()=={0,0}` must not panic or wrongly reject; asserts fail-open is deliberate, not accidental |
| REQ-2: Priority-aware admission control (Phase 3) | `github/rate_limit_test.go` | `TestRateLimiterSnapshot_Concurrent` | Integration (concurrency, `-race` mandatory gate) | 50 goroutines `Update()` concurrently with 50 goroutines `Snapshot()` for 1s — `go test -race` must show no data race (plan Task 3.1.1d, architecture-review's explicit merge-gate call) |
| REQ-2: Priority-aware admission control (Phase 3) | `session/pr_status_poller_test.go` | `TestFetchAndUpdatePRStatus_should_SkipCleanlyViaHandleFetchError_When_AdmitOriginRejects` | Integration | Confirms `AdmitOrigin`'s rejection error text lands in `handleFetchError`'s existing rate-limit branch (architecture-review's stringly-typed-classification concern — see Notes below) |
| REQ-3: GraphQL batching + `GetPRInfoCtx` migration (Phase 4) | `github/client_graphql_test.go` | `TestGetPRInfoGraphQL_should_PopulateFieldsMatchingCLI_When_PRHasApprovalsAndPassingChecks` | Unit | Happy path — single representative case, GraphQL decode alone |
| REQ-3: GraphQL batching + `GetPRInfoCtx` migration (Phase 4) | `github/client_graphql_test.go` | `TestGetPRInfoGraphQL_should_ReturnError_When_GraphQLResponseContainsErrorsArray` | Unit | Error path — GitHub GraphQL's `{"errors": [...]}` envelope (distinct from HTTP-level errors) must surface as a Go error, not a zero-value `*PRInfo` |
| REQ-3: GraphQL batching + `GetPRInfoCtx` migration (Phase 4) | `github/client_graphql_test.go` | `TestGetPRInfoGraphQL_should_MatchCLIFieldByField_When_PRInStateVariant` | **Integration** (table-driven, fixture-based) | **Widened parity matrix** — see "Widened Parity Test" section below; replaces the plan's under-specified "2-3 representative PRs" |
| REQ-3: GraphQL batching + `GetPRInfoCtx` migration (Phase 4) | `github/client_test.go` | `TestGetPRInfoCtx_should_DispatchToGraphQL_When_FlagOn` / `TestGetPRInfoCtx_should_UseUnchangedCLIPath_When_FlagOff` | Unit | Flag-gated dispatch, both directions (plan Task 4.2.1c) |
| REQ-4: Webhook-driven poller invalidation (Phase 5) | `github/etag_cache_test.go` | `TestETagCacheInvalidate_should_ForceFullFetch_When_EntryPreviouslyCached` | Unit | Happy path — invalidated entry sends no `If-None-Match` on next fetch |
| REQ-4: Webhook-driven poller invalidation (Phase 5) | `session/pr_status_poller_test.go` | `TestInvalidateAndRefresh_should_ReturnMatchedFalse_When_NoTrackedInstanceMatches` | Unit | Error/edge path — documented no-op, not an error, on a cache-miss with no matching instance |
| REQ-4: Webhook-driven poller invalidation (Phase 5) | `session/pr_status_poller_test.go` | `TestPRStatusPoller_should_TagOriginWebhookReconcileDistinctFromTickerOrigin_When_InvalidateAndRefreshDispatchesFetch` | **Integration** | **New — closes the ctx-threading repair pass's specific correctness property.** See "ctx-Threading / OriginWebhookReconcile Test" section below |
| REQ-4: Webhook-driven poller invalidation (Phase 5) | `server/services/github_webhook_pr_fix_test.go` | `TestHandlePRFixEvent_should_CallInvalidateForEventOncePerPRNumber_When_CheckRunEventReceived` | Integration | Spy `GitHubPollerInvalidator`, asserts per-PR-number call shape matches `TriggerPRFixForEvent`'s loop (plan Task 5.3.1f) |
| REQ-5: Conditional-requests-by-default infrastructure (Phase 2) | `tools/lint/norawghrequest/analyzer_test.go` | `TestNorawghrequest_should_FlagRawHTTPNewRequestWithContext_When_URLTargetsGitHubHost` | Unit | Happy path (golden-file) — violation case is flagged |
| REQ-5: Conditional-requests-by-default infrastructure (Phase 2) | `tools/lint/norawghrequest/analyzer_test.go` | `TestNorawghrequest_should_NotFlag_When_CallIsNolintSuppressedOrIsApprovedConstructor` | Unit | Error/negative path — suppressed + self-exempted constructor cases both pass clean |
| REQ-5: Conditional-requests-by-default infrastructure (Phase 2) | `github/http_client_test.go` | `TestNewConditionalRequest_should_SetIfNoneMatch_When_CacheHasEntry_And_OmitHeader_When_CacheEmpty` | Integration | Exercises the constructor against a real `*ETagCache` instance (in-process, not mocked) across both branches |
| REQ-2/3-adjacent: Frontend rate-limit reason classification (Phase 6, backend half) | `server/services/github_service_test.go` | `TestGetPRInfo_should_ReturnReasonExhaustedError_When_RateLimiterPrimaryLimitHit` | Unit | Happy/error path — RPC-layer classification the frontend's `getGitHubRateLimitMessage` depends on |

## Compound Two-Origin Test (Phase 3 / Success Metric #1)

The architecture and adversarial reviews both flag the same gap: every existing test drives
`AdmitOrigin` in isolation against a fabricated `Snapshot()`, or drives a single origin through
`rateLimitTransport` alone. Neither proves the actual claim requirements.md makes — that a
background-origin call *causing* exhaustion doesn't block a *subsequent* interactive call on the
same `RateLimiter`. This closes it:

**`TestRateLimitTransport_should_AdmitInteractiveCall_When_BackgroundOriginExhaustedSameRateLimiter`**
(`github/http_client_test.go`, Integration)

1. Stand up an `httptest.Server` that returns `200` with response headers
   `X-RateLimit-Remaining: 400`, `X-RateLimit-Limit: 5000` (8%, below the default 10%
   `backgroundHeadroomPercent`) and `X-RateLimit-Resource: core` on every request, and a counter
   tracking how many requests actually reached the handler.
2. Build one shared `*RateLimiter` + `rateLimitTransport{next: ...}` chain pointed at that server
   (not `httptest.NewServer`'s default transport substitution trick — the real transport chain
   under test), with `github:priority-admission-control` flipped **on** via the test's own
   `config.Config` instance.
3. **Step A (background call causes exhaustion)**: dispatch one request tagged
   `github.WithGitHubCallOrigin(ctx, github.OriginPRStatusPoller)` through the transport. Assert it
   succeeds (200) and that `RateLimiter.Snapshot().Remaining == 400` afterward — i.e., `Update()`
   actually published the low-remaining state from this real response, not a fabricated one.
4. **Step B (a second background call gets rejected)**: dispatch a second
   `OriginPRStatusPoller`-tagged request through the *same* transport. Assert it returns an error
   (rejected before dispatch) and that the server's request counter did **not** increment for this
   call — proving `AdmitOrigin` actually short-circuits the RoundTrip, not just returns a boolean
   nobody acts on.
5. **Step C (interactive call still succeeds)**: dispatch a third request tagged
   `github.WithGitHubCallOrigin(ctx, github.OriginInteractive)` through the same shared transport.
   Assert it succeeds (200) and the server's request counter *did* increment for this call.
6. Run under `go test -race` — this is a merge-gate test per `research/build-vs-buy.md` §6 and the
   architecture review's Task 3.1.1d note, and it exercises the exact `Update()`/`Snapshot()`
   cross-field-write concern the architecture review raises for Epic 3.1.

This is the one test in the suite that proves Success Metric #1's actual claim end-to-end through
`rateLimitTransport`, not through `AdmitOrigin` called directly.

## Widened Parity Test (Phase 4)

`TestGetPRInfoGraphQL_should_MatchCLIFieldByField_When_PRInStateVariant`
(`github/client_graphql_test.go`, table-driven over `github/testdata/pr_info_parity/`)

Per the adversarial review's finding that "2-3 representative PRs" under-covers
`DerivePRPriority`'s (`github/priority.go:21-60`) actual branches, the fixture table covers one
named case per distinct branch value, not just "draft/approved/CI-failing":

| Case name | Field(s) exercised | Value |
|---|---|---|
| `draft_in_progress_checks` | `IsDraft`, `CheckStatus` | `true`, `"in_progress"` |
| `approved_passing_checks` | `ApprovedCount`, `CheckConclusion` | `>0`, `"success"` |
| `changes_requested` | `ChangesRequestedCount` | `>0` |
| `checks_failure` | `CheckConclusion` | `"failure"` |
| `checks_action_required` | `CheckConclusion` | `"action_required"` |
| `checks_empty_conclusion` | `CheckConclusion` | `""` |
| `checks_pending` | `CheckConclusion` | `"pending"` |
| `terminal_state_merged` | `State` | `"merged"` |
| `terminal_state_closed` | `State` | `"closed"` |
| `mergeable_conflicting` | `Mergeable` | GraphQL `CONFLICTING` → CLI-equivalent lowercase string |
| `mergeable_unknown` | `Mergeable` | GraphQL `UNKNOWN` → CLI-equivalent lowercase string |

Each case gets a fixture pair (recorded gh-CLI JSON + recorded/hand-constructed GraphQL response,
per the adversarial review's explicit allowance for synthetic fixtures where a live PR in that
exact state is inconvenient to find — `mergeable_conflicting`/`mergeable_unknown`/`checks_pending`
are the ones most likely to need hand construction). The test asserts field-by-field equality
between the two decoded `*PRInfo` structs for every case, failing loudly (not silently) on any
mismatch — same assertion style as the plan's original Task 4.1.2b, just over 11 cases instead of
2-3.

## ctx-Threading / `OriginWebhookReconcile` Test (Phase 5)

The repair pass's specific new correctness property: `fetchAndUpdatePRStatus`/`fetchAndStore` now
take `ctx` as a real parameter (Tasks 5.1.2a-prime/5.2.1a-prime) specifically so
`InvalidateAndRefresh`'s webhook-triggered dispatch can tag `OriginWebhookReconcile`, distinct from
the ticker fan-out's own `OriginPRStatusPoller`/`OriginWorktreePRPoller` tag. This is new behavior
the repair introduced, not covered by a generic "invalidate works" test:

**`TestPRStatusPoller_should_TagOriginWebhookReconcileDistinctFromTickerOrigin_When_InvalidateAndRefreshDispatchesFetch`**
(`session/pr_status_poller_test.go`, Integration)

1. Construct a `*PRStatusPoller` with a test `*Instance` tracking `(owner, repo, prNumber)`, wired
   to a fake GitHub HTTP transport (or a `github.GetPRInfoConditional`-shaped test seam) that
   records `github.GitHubCallOriginFrom(req.Context())` for every call it receives, keyed by call
   sequence.
2. Run the ticker's normal fan-out path once (`checkAllSessions` → `fetchAndUpdatePRStatus(ctx,
   inst)` built internally by the ticker loop per Task 5.1.2a-prime) and assert the recorded origin
   for that call is `github.OriginPRStatusPoller`.
3. Call `InvalidateAndRefresh(ctx, owner, repo, prNumber)` directly and assert the recorded origin
   for *that* dispatched call is `github.OriginWebhookReconcile` — a different value from step 2,
   proving the two call paths are distinguishable in telemetry, which is the acceptance criterion
   Phase 5's cross-cutting "done" bar and the plan's own stated rationale for the signature change
   require ("Phase 5's own cross-cutting acceptance criterion requires webhook-triggered refreshes
   to be observable as distinct from ticker refreshes in telemetry").
4. Negative check: assert the two recorded origins are `!=` each other, not just individually
   correct — a test that only checked each in isolation could pass even if both paths accidentally
   defaulted to the same tag.

## UX Acceptance Tests

Per `design/ux.md`, this project's only user-facing surface is Phase 6, Epic 6.1 — friendlier
GitHub rate-limit copy in two *existing* error slots (`VcsPanel.tsx`, `VcsWidgetComments.tsx`), no
new component or screen. `design/ux.md` §2 documents 3 error states (transient, exhausted, generic
fallback) and §5 lists 9 numbered acceptance criteria; each maps to a concrete test/check below.
Per this repo's own `tests/e2e/vcs-widget.spec.ts` precedent (which explicitly deprioritizes the
live-GitHub-API-polling half of the VCS widget for e2e determinism, covering only the DB-backed
durable-snapshot path with Playwright), a rate-limited-response UI state is likewise better
exercised as a Jest/RTL component test (mocking the classified error) than a live Playwright flow
that would require an actually-exhausted real GitHub token — consistent with this repo's
`e2e-test-conventions` and `ui-playwright` guidance to prefer deterministic component-level tests
over flaky live-API e2e for a state that can't be reliably reproduced against a real backend.

| UX Criterion (design/ux.md §5, item #) | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| #1 No dead ends — Retry reachable in ≤1 click in every state, both surfaces | `web-app/src/components/shared/vcs-widget/VcsWidgetComments.test.tsx` | `VcsWidgetComments_should_RenderKeyboardReachableRetryButton_When_LoadFails` | Jest/RTL | Mock `fetchComments` to reject with a classified error; assert a `<button>` with accessible name "Retry" renders (today's zero-retry dead end this criterion is not yet met against) |
| #1 (regression, existing surface) | `web-app/src/components/sessions/VcsPanel.test.tsx` | `VcsPanel_should_KeepRetryButtonVisible_When_ErrorStateIsTransientOrExhausted` | Jest/RTL | Assert Retry button remains present/functional across all 3 states, not suppressed by the new copy |
| #2 Copy correctness — State A (transient) | `web-app/src/lib/vcs/githubRateLimit.test.ts` | `getGitHubRateLimitMessage_should_ReturnTransientCopyWithRelativeTimeNoISOTimestamp_When_ReasonIsTransient` | Jest | Assert output matches the exact pattern `"GitHub is temporarily rate-limited — this should clear up in about <relative time>, retrying automatically."` and contains no raw ISO-8601 string |
| #3 Copy correctness — State B (exhausted) | `web-app/src/lib/vcs/githubRateLimit.test.ts` | `getGitHubRateLimitMessage_should_ReturnExhaustedCopyWithRelativeTimeNoISOTimestamp_When_ReasonIsExhausted` | Jest | Assert output matches `"GitHub rate limit reached — try again in ~<relative time>."`, no raw ISO-8601 |
| #4 Fallback preserved — State C | `web-app/src/components/sessions/VcsPanel.test.tsx` + `VcsWidgetComments.test.tsx` | `VcsPanel_should_RenderRawErrorMessage_When_ErrorHasNoReasonMarker` / `VcsWidgetComments_should_RenderFailedToLoadCommentsCopy_When_ErrorHasNoReasonMarker` | Jest/RTL | Feed an error with no `reason` field; assert unchanged legacy copy renders in both components (`getGitHubRateLimitMessage` must not misclassify) |
| #5 Screen-reader announcement | `VcsPanel.test.tsx` + `VcsWidgetComments.test.tsx` | `*_should_HaveStatusAriaLivePolite_When_ErrorStateRenders` (one per component, run across all 3 states) | Jest/RTL | Query the error container and assert `role="status"` + `aria-live="polite"` attribute pair present in State A, B, and C |
| #6 Color is not the only signal | manual | Icon+text pairing visual check, both surfaces, all 3 states | Manual | Confirm `⚠️` (VcsPanel) / plain-text label (VcsWidgetComments) still accompanies the message — no state relies on a color swatch alone |
| #7 No new components | manual (code review) | Structural review at PR time | Manual | Confirm `githubRateLimit.ts` is the only new file per Task 6.1.1a's Files list; both components keep existing DOM structure |
| #8 Keyboard navigable | `VcsWidgetComments.test.tsx` | `VcsWidgetComments_should_RenderRealButtonElement_When_RetryActionAdded` | Jest/RTL (+ manual) | RTL: assert the Retry control's tag is `BUTTON` (not `DIV` with an onClick) and is enabled/focusable; manual: real-browser Tab-to-focus + Enter/Space activation check, since jsdom doesn't fully model browser focus/tab order |
| #9 Contrast ≥4.5:1 | manual / existing CI | Axe Core check (already runs in this repo's UX-analysis CI per `CLAUDE.md`'s "UX analysis CI... blocks on WCAG AA violations" for PRs touching `web-app/src/`) | Automated (existing gate) + manual spot-check | No new colors introduced (per plan) — rely on the existing repo-wide Axe Core CI gate rather than a new contrast test; manual spot-check only if Axe flags something unexpected |

## Test Stack

- **Unit (Go)**: `go test` + stdlib `testing`, table-driven where the plan specifies (e.g. Phase 4's
  parity matrix, Phase 3's `AdmitOrigin` cases). `httptest.Server`/`httptest.NewRecorder` for HTTP
  boundary tests; `go test -race` mandatory for Epic 3.1/3.2's concurrency-touching tests per the
  architecture and build-vs-buy reviews' explicit call.
- **Integration (Go)**: same `go test` binary, no separate harness — "integration" here means
  driving a real chain of production types together (`rateLimitTransport` → `RateLimiter` →
  `httptest.Server`; `PRStatusPoller` → `ETagCache` → fake transport) rather than a single function
  in isolation, consistent with how `github/client_pr_by_number_test.go` and
  `session/pr_status_poller_test.go` already test this codebase.
- **Frontend unit**: Jest + React Testing Library, matching `VcsPanel.test.tsx`/
  `VcsWidgetComments.test.tsx`'s existing `Component_should_Behavior_When_Condition` `describe`/`it`
  convention.
- **E2E / UX**: Playwright (`tests/e2e/`, `ui-playwright` skill as the implementation model) is the
  stack's standard tool, but per this project's own UX design (no new screen, live-rate-limit state
  hard to reproduce deterministically against a real GitHub token) and the precedent in
  `tests/e2e/vcs-widget.spec.ts`, the 3 error-state UX criteria that need code-level verification are
  covered by Jest/RTL component tests instead of a new Playwright spec; criteria that are inherently
  visual/manual (#6, #7, #9) stay manual or rely on the existing Axe Core CI gate. No new
  `tests/e2e/*.spec.ts` file is introduced by this plan.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with `go test -race ./github/...` as an additional mandatory (not just coverage-counted) gate for Epic 3.1/3.2 |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` (run from `web-app/`) | ≥80% line |

- All public service methods touched by this plan (`RateLimiter.AdmitOrigin`/`Snapshot`,
  `ETagCache.Invalidate`, `PRStatusPoller.InvalidateAndRefresh`, `WorktreePRPoller.InvalidateCache`,
  `GetPRInfoGraphQL`, `getGitHubRateLimitMessage`): happy path + error path covered per the mapping
  table above.
- All external integrations (native HTTP to GitHub, `gh` CLI subprocess, GraphQL POST, webhook
  delivery → poller invalidation): unit mocked (`httptest.Server`, fake `exec` function, fixture
  responses) **and** at least one integration-style test per the mapping table — satisfied
  explicitly by the three named tests above (compound two-origin test, widened parity matrix,
  `OriginWebhookReconcile` ctx test) plus the webhook-handler spy test (Task 5.3.1f).
- UX acceptance criteria: all 9 of `design/ux.md` §5's numbered criteria have a corresponding test
  or manual step in the UX Acceptance Tests table above — 0 criteria left unmapped.

## Notes for Implementers

- The architecture review's stringly-typed-error-classification concern
  (`PRStatusPoller.handleFetchError`'s `strings.Contains(msg, "rate limit")` check vs.
  `AdmitOrigin`'s literal rejection string not containing that substring) is exercised directly by
  `TestFetchAndUpdatePRStatus_should_SkipCleanlyViaHandleFetchError_When_AdmitOriginRejects` above.
  If that test fails as written against the plan's example reason string
  (`"background headroom reserved"`), it is evidence for the architecture review's recommended fix
  — either reword the rejection string to contain "rate limit," or introduce
  `github.ErrAdmissionRejected` and check `errors.Is` first in `handleFetchError` — not a reason to
  weaken the test's assertion.
- Both feature flags (`github:priority-admission-control`, `github:graphql-pr-info`) default off at
  merge time (user decision, 2026-09-08). Every Phase 3/4 test above must exercise **both** flag
  states explicitly (already reflected in the mapping table's flag-dispatch rows) — a test suite
  that only ever sets the flag on would never catch a regression in the default (off) path that
  every existing user actually runs.

## Summary

Recounted directly from the mapping table above (not estimated):

- **Test counts by type (Go)**: **19 unit** (11 happy-path/dispatch, 8 explicitly error/edge-path)
  + **8 integration** — of which 3 are the newly-designed gap-closing tests this task specifically
  required (compound two-origin admission test, widened GraphQL parity matrix,
  `OriginWebhookReconcile` ctx-threading test), and 1 (`TestRateLimiterSnapshot_Concurrent`) is a
  mandatory `-race`-gated concurrency test. **27 Go tests total.**
- **Test counts by type (frontend)**: **8 Jest/RTL tests** (UX Acceptance Tests table) — no new
  Playwright spec (see rationale in that section).
- **Manual/existing-CI checks**: **3** (color-not-only-signal, no-new-components code-review check,
  contrast — the last backed by this repo's existing Axe Core CI gate, not net-new).
- **Grand total**: 35 automated tests + 3 manual/existing-gate checks.
- **Requirements coverage**: 5/5 Scope items (REQ-1 through REQ-5) mapped to at least one
  happy-path unit, one error-path unit, and one integration test each — **100%** (verified by
  scanning the mapping table's "Requirement" column for all 5 REQ labels, each appearing with all
  three Type values present).
- **UX acceptance tests**: 9/9 of `design/ux.md` §5's numbered criteria mapped (6 automated
  Jest/RTL rows, 1 automated via the existing Axe CI gate, 2 manual) — **0 criteria unmapped**.
- **Migration test**: N/A — no schema changes in this plan (confirmed: plan.md contains no
  Migration Plan section; all new state is in-memory `RateLimiter`/`ETagCache` fields, feature
  flags, and OTel metric registrations, none of which touch the ent schema or require an
  up/down migration).
