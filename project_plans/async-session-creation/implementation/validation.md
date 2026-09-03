# Validation Plan: async-session-creation

**Date**: 2026-08-26

## Happy Path Scenario
Given a user has a `stapler-squad` session list open (Baseline: omnibar visible, no in-flight creations), when the user pastes a `github_url` pointing at a slow GHE host and clicks Create, then `CreateSession` returns within ~500ms with a `SESSION_STATUS_CREATING` instance, the omnibar closes immediately, the card shows live `creation_progress` text as the Background Resolution Pipeline advances through phases, and the card silently transitions to `Active` on success with no dialog ever having blocked. *(This is the anchor: every other test below is either a slice of this pipeline, a failure/race variant of its terminal write, or a UX surface this flow touches.)*

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ Epic 1.1 Story 1.1.2: `Failed` FSM transitions | `session/instance_state_test.go` | `TestTransitionTo_should_AllowCreatingToFailed_When_PipelineFails` | Unit | Happy path |
| REQ Epic 1.1 Story 1.1.2: illegal transitions rejected | `session/instance_state_test.go` | `TestTransitionTo_should_RejectStoppedToFailed_When_StoppedIsTerminal` | Unit | Error path |
| REQ Epic 1.1 Story 1.1.2: `Failed→Creating` (retry path) | `session/instance_state_test.go` | `TestTransitionTo_should_AllowFailedToCreating_When_RetryRequested` | Unit | Happy path |
| REQ Epic 1.1 Story 1.1.3: `adapters.StatusToProto` maps `Failed` | `server/services/adapters_test.go` (or located file) | `TestStatusToProto_should_ReturnFailed_When_StatusIsFailed` | Unit | Happy path |
| REQ Epic 1.1 Story 1.1.3: exhaustiveness guard on unmapped status | `server/services/adapters_test.go` | `TestStatusToProto_should_ReturnExplicitError_When_StatusIsUnrecognized` | Unit | Error path |
| REQ Epic 1.1 Story 1.1.3: `WatchSessions` `StatusFilter=[FAILED]` | `server/services/session_service_test.go` | `TestWatchSessions_should_DeliverEvent_When_StatusFilterMatchesFailed` | Integration | Happy path (event bus + storage) |
| REQ Epic 1.1 Story 1.1.4: `CreationProgressUpdatedAt` bumped by `SetCreationProgress` | `session/instance_state_test.go` | `TestSetCreationProgress_should_UpdateTimestamp_When_Called` | Unit | Happy path |
| REQ Epic 1.1 Story 1.1.4: timestamp survives restart via `UpdateInstance` (not `SaveInstances`) | `server/services/session_service_test.go` | `TestCreateSession_should_PersistCreationProgressUpdatedAt_When_PhaseTransitionsBeforeStarted` | Integration | Happy path — proves the real pipeline call, not a hand-set fixture |
| REQ Epic 1.2 Story 1.2.1: `bumpCreationEpoch` increments | `session/instance_epoch_test.go` | `TestBumpCreationEpoch_should_Increment_When_CalledTwice` | Unit | Happy path |
| REQ Epic 1.2 Story 1.2.2: `TryForceStatusIfEpoch` matching epoch wins | `session/instance_epoch_test.go` | `TestTryForceStatusIfEpoch_should_ApplyWrite_When_EpochMatches` | Unit | Happy path |
| REQ Epic 1.2 Story 1.2.2: stale epoch no-ops | `session/instance_epoch_test.go` | `TestTryForceStatusIfEpoch_should_ReturnFalse_When_EpochIsStale` | Unit | Error path |
| REQ Epic 1.2 Story 1.2.2: exactly one winner under race | `session/instance_epoch_test.go` | `TestTryForceStatusIfEpoch_should_ProduceExactlyOneWinner_When_CalledConcurrently` | Integration (`-race -count=50`) | Race/concurrency |
| REQ Epic 1.2 Story 1.2.3: `TryStartRetry` on `Failed` succeeds | `session/instance_epoch_test.go` | `TestTryStartRetry_should_BumpEpochAndResetToCreating_When_StatusIsFailed` | Unit | Happy path |
| REQ Epic 1.2 Story 1.2.3: `TryStartRetry` no-ops on non-`Failed` | `session/instance_epoch_test.go` | `TestTryStartRetry_should_ReturnNotStarted_When_StatusIsNotFailed` | Unit | Error path |
| REQ Epic 1.2 Story 1.2.3: double-click retry race | `session/instance_epoch_test.go` | `TestTryStartRetry_should_StartExactlyOnce_When_CalledConcurrently` | Integration (`-race -count=50`) | Race/concurrency |
| REQ Epic 1.3 Story 1.3.1: `StartLinkedBackgroundSpan` safe with OTel disabled | `telemetry/telemetry_test.go` | `TestStartLinkedBackgroundSpan_should_ReturnNoopSpan_When_OtelDisabled` | Unit | Error path (degraded dependency) |
| REQ Epic 1.3 Story 1.3.1: linked new-root span when OTel enabled | `telemetry/telemetry_test.go` | `TestStartLinkedBackgroundSpan_should_LinkToParentTrace_When_OtelEnabled` | Unit | Happy path |
| REQ Epic 1.3 Story 1.3.2: outcome/duration metrics recorded once | `server/services/session_creation_metrics_test.go` | `TestSessionCreationMetrics_should_RecordOutcomeAndDuration_When_TerminalWriteSucceeds` | Integration (in-memory meter reader) | Happy path |
| REQ Epic 2.1 Story 2.1.1: fast-fail validation stays synchronous, no instance created | `server/services/session_service_test.go` | `TestCreateSession_should_ReturnInvalidArgument_When_TitleIsEmpty` | Unit | Error path |
| REQ Epic 2.1 Story 2.1.1: duplicate title still synchronous `AlreadyExists` | `server/services/session_service_test.go` | `TestCreateSession_should_ReturnAlreadyExists_When_TitleDuplicates` | Integration (storage) | Error path |
| REQ Epic 2.1 Story 2.1.1: instance created+published before resolution, RPC fast for slow GitHub URL | `server/services/session_service_test.go` | `TestCreateSession_should_ReturnWithinSLO_When_GithubURLResolutionIsSlow` | Integration | Happy path — anchor scenario's backend half |
| REQ Epic 2.1 Story 2.1.1c: title-uniqueness race preserved under new ordering | `server/services/session_service_test.go` | `TestCreateSession_should_RejectSecondDuplicate_When_TwoRapidCallsShareTitle` | Integration | Race/concurrency |
| REQ Epic 2.2 Story 2.2.1: RPC-context cancellation doesn't kill pipeline | `server/services/session_service_test.go` | `TestBackgroundResolutionPipeline_should_ContinueRunning_When_RPCContextIsCanceled` | Integration | Happy path (the pitfalls.md §2 regression) |
| REQ Epic 2.2 Story 2.2.1: pipeline timeout produces terminal Failed | `server/services/session_service_test.go` | `TestBackgroundResolutionPipeline_should_WriteFailed_When_ResolutionExceedsTimeout` | Integration | Error path |
| REQ Epic 2.2 Story 2.2.2: phase transitions publish progressively | `server/services/session_service_test.go` | `TestBackgroundResolutionPipeline_should_PublishProgressPerPhase_When_GithubURLSession` | Integration | Happy path |
| REQ Epic 2.2 Story 2.2.2: directory session pipeline is near-instant, no regression | `server/services/session_service_test.go` | `TestBackgroundResolutionPipeline_should_CompleteWithoutNetworkIO_When_PlainDirectorySession` | Unit | Happy path |
| REQ Epic 2.2 Story 2.2.2d: panic in pipeline recovered, terminal Failed write | `server/services/session_service_test.go` | `TestBackgroundResolutionPipeline_should_WriteFailedAndNotCrashProcess_When_PhaseFuncPanics` | Integration | Error path |
| REQ Epic 2.2 Story 2.2.3: successful terminal write increments success metric | `server/services/session_service_test.go` | `TestBackgroundResolutionPipeline_should_TransitionToActive_When_ResolutionSucceeds` | Integration | Happy path |
| REQ Epic 2.2 Story 2.2.3: stale-epoch pipeline write is a no-op (cancel already won) | `server/services/session_service_test.go` | `TestBackgroundResolutionPipeline_should_SkipTerminalWrite_When_EpochAlreadyBumped` | Integration | Error path |
| REQ Epic 2.2 Story 2.2.3c: exactly one terminal event under cancel/success race | `server/services/session_service_test.go` | `TestCreateSession_should_PublishExactlyOneTerminalEvent_When_CancelRacesSuccess` | Integration (`-race -count=50`) | Race/concurrency |
| REQ Epic 3.1 Story 3.1.1: `cleanupPartialCreation` no-ops with nothing to clean | `server/services/session_service_test.go` | `TestCleanupPartialCreation_should_ReturnNil_When_NoWorktreeOrTmuxExists` | Unit | Happy path |
| REQ Epic 3.1 Story 3.1.1: `cleanupPartialCreation` removes partial worktree | `server/services/session_service_test.go` | `TestCleanupPartialCreation_should_RemoveWorktree_When_CreatedBeforeTmuxStartup` | Integration (filesystem) | Happy path |
| REQ Epic 3.1 Story 3.1.1c: idempotent double-call | `server/services/session_service_test.go` | `TestCleanupPartialCreation_should_BeIdempotent_When_CalledTwice` | Integration | Error path (degenerate input) |
| REQ Epic 3.2 Story 3.2.1: cancel a live `Creating` instance | `server/services/session_service_test.go` | `TestCancelSessionCreation_should_RemoveInstanceAndCleanUp_When_StatusIsCreating` | Integration | Happy path |
| REQ Epic 3.2 Story 3.2.1: cancel with nil `CancelFunc` (post-restart) | `server/services/session_service_test.go` | `TestCancelSessionCreation_should_SucceedWithoutPanic_When_CancelFuncIsNil` | Integration | Error path (degraded dependency) |
| REQ Epic 3.2 Story 3.2.1: cancel on non-`Creating` instance rejected | `server/services/session_service_test.go` | `TestCancelSessionCreation_should_ReturnFailedPrecondition_When_StatusIsActive` | Unit | Error path |
| REQ Epic 3.2 Story 3.2.1c/d: cancel-vs-success race resolved deterministically | `server/services/session_service_test.go` | `TestCancelSessionCreation_should_ResolveDeterministically_When_RacingPipelineSuccess` | Integration (`-race -count=50`) | Race/concurrency |
| REQ Epic 3.3 Story 3.3.1: retry re-runs in place, no duplicate row/event | `server/services/session_service_test.go` | `TestRetrySessionCreation_should_TransitionFailedToCreating_When_SameInstanceRetried` | Integration | Happy path |
| REQ Epic 3.3 Story 3.3.1: retry rejected on non-`Failed` instance | `server/services/session_service_test.go` | `TestRetrySessionCreation_should_ReturnFailedPrecondition_When_StatusIsNotFailed` | Unit | Error path |
| REQ Epic 3.3 Story 3.3.1d: double-click retry spawns exactly one pipeline | `server/services/session_service_test.go` | `TestRetrySessionCreation_should_SpawnExactlyOnePipeline_When_CalledConcurrently` | Integration (`-race -count=50`) | Race/concurrency |
| REQ Epic 4.1 Story 4.1.1: `ThresholdMinutesOrDefault` default | `config/types_test.go` | `TestCreationStaleConfig_ThresholdMinutesOrDefault_should_Return10_When_Unset` | Unit | Happy path |
| REQ Epic 4.1 Story 4.1.1: configured override respected | `config/types_test.go` | `TestCreationStaleConfig_ThresholdMinutesOrDefault_should_ReturnConfiguredValue_When_Set` | Unit | Happy path (alt input) |
| REQ Epic 4.1 Story 4.1.2: sweeper flips instance past threshold | `server/services/stale_creation_sweeper_test.go` | `TestStaleCreationSweeper_should_FlipToFailedStale_When_LastProgressExceedsThreshold` | Integration | Happy path |
| REQ Epic 4.1 Story 4.1.2: sweeper leaves fresh instance alone | `server/services/stale_creation_sweeper_test.go` | `TestStaleCreationSweeper_should_LeaveInstanceUntouched_When_BelowThreshold` | Integration | Error path (negative case) |
| REQ Epic 4.1 Task 4.1.2d: restart-orphan case via real pipeline persistence path | `server/services/stale_creation_sweeper_test.go` | `TestStaleCreationSweeper_should_FlipReloadedInstance_When_OrphanedAcrossRestart` | Integration | Happy path (drives real `UpdateInstance` call, not a hand-set fixture, per Task 4.1.2d's explicit anti-pattern warning) |
| REQ Epic 4.1 Task 4.1.2e: stale-flip vs. late-success race | `server/services/stale_creation_sweeper_test.go` | `TestStaleCreationSweeper_should_ResolveDeterministically_When_RacingLatePipelineSuccess` | Integration (`-race -count=50`) | Race/concurrency |
| REQ Epic 4.1 Task 4.1.2f: zero-progress-updates-ever fallback to `CreatedAt` | `server/services/stale_creation_sweeper_test.go` | `TestStaleCreationSweeper_should_UseCreatedAtBaseline_When_NoProgressUpdateEverRecorded` | Integration | Error path (degenerate input) |
| REQ Epic 6.1: all 7 session-creation modes reach `Active` via new pipeline | `server/services/session_service_test.go` | `TestCreateSession_should_ReachActiveViaPipeline_When_ModeIs<Directory\|OneOff\|Restart\|Fork\|Alias\|Autonomous\|Remote>` (table-driven, one subtest per mode) | Integration | Happy path (registry re-verification) |
| REQ Epic 6.2: no goroutine leak across repeated create/fail/retry/cancel | `server/services/session_service_test.go` | `TestCreateSession_should_LeaveNoGoroutines_When_HammeredWithFailRetryCancelCycles` | Integration (`-race`, `goleak.VerifyNone`) | Race/concurrency + leak |
| Migration: ent schema fields reversible | `session/ent/schema/migration_test.go` (new) | `migration_should_be_reversible` | Migration | See Step 5 below |

**Coverage note**: 34 requirement-level items above span every Epic (1.1–6.2); Epic 6.3 (registry/doc regeneration) is a Make-target/process check, not a test artifact, and is intentionally omitted from this table — it's verified by running `make registry-generate` and diffing, not by a test assertion.

## UX Acceptance Tests

Implementation model: `ui-playwright` skill conventions (this repo's `tests/e2e/`, Playwright + Allure, `@feature` header, `data-testid`/ARIA locators only, no `waitForTimeout`).

| UX Criterion (design/ux.md) | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Dialog closes within 500ms of Create, even for slow GitHub-URL session (Surface 1) | `tests/e2e/session-creation-async.spec.ts` | `omnibar closes within SLO for a slow GitHub URL session` | Playwright | Stub/mock a slow background resolution via test-mode hook → paste GHE URL → click Create → assert dialog `data-testid="omnibar"` is detached from DOM within 500ms via `expect(locator).toBeHidden()` (no `waitForTimeout`) |
| Create button disables instantly, no double-card on double-click (Surface 1) | `tests/e2e/session-creation-async.spec.ts` | `double-clicking Create produces exactly one session card` | Playwright | Fill omnibar → rapid double-click `getByRole('button', {name: 'Create'})` → assert exactly one `data-testid="session-card"` with the given title appears in the list |
| Fast-fail validation keeps dialog open, no card created (Surface 1, regression) | `tests/e2e/session-creation-async.spec.ts` | `duplicate title keeps the omnibar open with inline error` | Playwright | Create a session, then attempt a second with the same title → assert omnibar `data-testid="omnibar"` remains visible, inline error text shown, session list count unchanged |
| Card appears within ~1s with live progress text updating in place (Surface 2) | `tests/e2e/session-creation-async.spec.ts` | `session card shows progress text updating without remount` | Playwright | Create GitHub-URL session → assert `data-testid="session-progress-text"` becomes visible within 1s → capture its DOM node handle → assert the same node (not a new one) updates text as phases advance |
| Cancel button present/clickable throughout Creating (Surface 2/3) | `tests/e2e/session-creation-async.spec.ts` | `cancel button is clickable immediately after session creation starts` | Playwright | Create slow session → immediately click `getByRole('button', {name: 'Cancel session creation'})` → assert card is removed from the list |
| Cancel-loses-race shows Running, not a Cancelled flash (Surface 3) | `tests/e2e/session-creation-async.spec.ts` | `cancel racing pipeline success shows Running not a cancelled flash` | Playwright | Use test-mode hook to let pipeline win the race → click Cancel → assert card transitions directly to `[data-testid="status-pill"]` text `Running`/`Active`, never rendering a `Cancelled` intermediate state |
| Retry transitions same card Failed→Creating in place, never a second card (Surface 3) | `tests/e2e/session-creation-async.spec.ts` | `retry transitions the same card in place with no duplicate` | Playwright | Force a resolution failure → assert Failed card → click `getByRole('button', {name: 'Retry creating session'})` → assert the same `data-testid` node ID shows `Creating`, and session list count is unchanged |
| Double-click Retry guarded client-side (Surface 3) | `tests/e2e/session-creation-async.spec.ts` | `double-clicking Retry only fires one retry` | Playwright | Force Failed → rapid double-click Retry → assert button becomes disabled after first click, exactly one `RetrySessionCreation` network request fires (`page.on('request')` assertion) |
| Failed message is one of three specific strings, not generic (Surface 3) | `tests/e2e/session-creation-async.spec.ts` | `failed card shows reason-specific message for GitHubResolutionError/StartupError/Stale` | Playwright | Table-driven: inject each `FailureReason` via test-mode hook → assert `data-testid="failure-message"` text matches the exact expected copy per reason |
| Cancel/Retry are real buttons with distinct `aria-label`s, in Tab order (Surface 3, a11y) | `tests/e2e/accessibility.spec.ts` | `Cancel and Retry controls are keyboard-reachable buttons with distinct aria-labels` | Playwright + axe-core | Create Failed and Creating cards → run axe scan → assert `role=button` with `aria-label="Cancel session creation"` / `"Retry creating session"` present and reachable via `page.keyboard.press('Tab')` |
| Failed status pill contrast ≥4.5:1 in light and dark (Surface 3, a11y) | `tests/e2e/accessibility.spec.ts` | `Failed status pill meets WCAG AA contrast in both themes` | Playwright + axe-core | Render Failed card in light then dark theme → run axe `color-contrast` rule → assert zero violations |
| Exactly one `role="status"` live region across Creating→Failed, `aria-live` flips polite→assertive (Surface 3, a11y) | `tests/e2e/accessibility.spec.ts` | `Failed transition reuses the single existing live region` | Playwright | Create session → count `[role="status"]` nodes on the card (assert 1) → force failure → re-count (still 1) → assert the same node's `aria-live` attribute changed from `polite` to `assertive` |
| Reduced-motion Failed state is static, no animation (Surface 3, a11y) | `tests/e2e/accessibility.spec.ts` | `Failed icon has no animation under prefers-reduced-motion` | Playwright | `page.emulateMedia({reducedMotion: 'reduce'})` → force Failed → assert the warning glyph has no active CSS animation (computed style check) |
| No focus theft on background card transitioning to Failed (Surface 3, a11y) | `tests/e2e/accessibility.spec.ts` | `focus stays on active element when a background card fails` | Playwright | Focus the omnibar input for session B → force session A (different card) to Failed → assert `document.activeElement` is still the omnibar input |
| Toast fires once with reason-specific copy, regardless of current page (Surface 4) | `tests/e2e/session-creation-async.spec.ts` | `failure toast fires exactly once with reason-specific copy` | Playwright | Navigate away from the session list → force a failure → assert exactly one `data-testid="toast"` appears with the expected copy variant |
| Toast dismissal doesn't remove the card's Failed state (Surface 4) | `tests/e2e/session-creation-async.spec.ts` | `dismissing the toast leaves the Failed card intact` | Playwright | Force failure → dismiss toast via its close button → assert the session card's Failed state and message are still present |
| Stale transition arrives as an ordinary stream event, same code path as other Failed reasons (Surface 5) | `tests/e2e/session-creation-async.spec.ts` | `stale session appears as Failed with distinct stale copy after reopening the app` | Playwright | Use test-mode hook to fast-forward a `Creating` session past the stale threshold server-side → reload the page → assert the card renders `Failed` with the stale-specific message, sourced from persisted state (no client timer involved) |

## Test Stack
- **Unit**: Go `testing` package + table-driven tests, this repo's existing convention (no external assertion library observed in `server/services/*_test.go` beyond stdlib `testing`; frontend unit tests use Jest + `@testing-library/react`).
- **Integration**: Go `testing` with in-process fixtures (real `session.Instance`, `Storage`, `events.EventBus`), `-race` for concurrency-sensitive tests, `go.uber.org/goleak` for goroutine-leak assertions, per Epic 6.2 and the repo's documented `goleak` convention.
- **E2E / UX**: Playwright against the isolated e2e server (`tests/e2e/global-setup.ts`), Axe Core for WCAG AA checks, following `.claude/rules/e2e-test-conventions.md` (feature header, no `waitForTimeout`, `data-testid`/ARIA locators only, new page helpers under `tests/e2e/pages/`).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% of `TryForceStatusIfEpoch`/`TryStartRetry` branches (fencing correctness is the project's core safety property, per ADR-002) |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods touched by this project (`CreateSession`, `CancelSessionCreation`, `RetrySessionCreation`, `TryForceStatusIfEpoch`, `TryStartRetry`, `cleanupPartialCreation`): happy path + error paths covered.
- All external integrations (GitHub clone/resolution, tmux/worktree startup, ent storage): unit mocked (fake clone/worktree) + at least one integration test exercising the real pipeline call sites (per Task 4.1.2d's explicit "don't fixture-shortcut this" instruction).
- Every UX acceptance criterion in `design/ux.md` Surfaces 1–5 has a corresponding Playwright test above; the Cross-Cutting Accessibility Verification section's 6 items are covered by `tests/e2e/accessibility.spec.ts` entries.
- `make ci`/`make ready` must pick up the new `-race -count=50` hammer and fencing tests — confirmed as its own task (Epic 6.2's Task 6.2.1b), not assumed.

## Migration Test Design (Step 5)

Per `CLAUDE.md`, `session/ent/schema` changes are hand-written and generated output (`session/ent/*.go`) is never committed — `make ent-gen`/`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` regenerates it from the schema on every build. This means there is no traditional "migration file" to run up/down against a live database in the Rails/Alembic sense; the "migration" here is: (a) the new hand-written ent schema fields (`creationEpoch uint64`, `FailureReason string`, `CreationProgressUpdatedAt time.Time`) generate correctly, and (b) an existing persisted `session.json`/ent row from before this deploy loads correctly with those fields at their zero values (no backfill needed, per the plan's Migration Plan section) and round-trips a write back out.

`migration_should_be_reversible` is therefore designed as a **schema-generation + load/round-trip reversibility test**, not a SQL up/down test:

1. **Up**: with the new schema fields present in `session/ent/schema`, run the documented generate command, then `go build ./session/ent/...` to confirm the generated code compiles with the three new fields exposed as expected Go types.
2. **Verify schema state**: construct a fresh ent client against an in-memory/temp SQLite instance (this repo's existing ent test pattern — confirm via `session/ent`'s test setup), create a row, and assert `creationEpoch`, `FailureReason`, and `CreationProgressUpdatedAt` are all queryable and round-trip their zero values correctly for a row that never sets them (simulating a pre-deploy row).
3. **Down (rollback)**: `git revert` the schema-field commit (this project's stated rollback procedure — no flag, no DB migration to undo per the Migration Plan), regenerate ent code from the reverted schema, and confirm `go build ./...` succeeds — i.e. removing the fields doesn't leave any dangling reference to `CreationEpoch()`/`FailureReason()`/`CreationProgressUpdatedAt()` accessors outside code also reverted in the same commit.
4. **Assert rollback correctness**: after the down step, re-run the ent test suite (`go test ./session/ent/...`) and confirm it passes with the fields absent — proving the "up" schema change and the code that depends on it are correctly co-scoped in one revertible commit, matching this project's explicit no-feature-flag Risk Control section.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Migration Plan: new ent schema fields generate, load with zero-value defaults for pre-deploy rows, and are cleanly revertible | `session/ent/schema/migration_test.go` (new) | `migration_should_be_reversible` | Migration | Up: generate + build + round-trip zero-value read for a pre-deploy-shaped row. Down: revert schema commit, regenerate, confirm build and existing ent tests pass with fields absent. |

## Summary

- **Unit tests**: 20
- **Integration tests**: 27 (including 7 `-race -count=50` concurrency tests and the Epic 6.1 7-mode table-driven test counted as 1 row/7 subtests)
- **Migration test**: 1 (`migration_should_be_reversible`)
- **UX/Playwright acceptance tests**: 17
- **Total test cases designed**: 65
- **Requirements coverage**: all 6 plan phases (1–6) and all requirements.md Scope/Success-Metrics items have at least one mapped test; Epic 6.3 (registry regen) is process-only and explicitly excluded from the count, noted in the table.
