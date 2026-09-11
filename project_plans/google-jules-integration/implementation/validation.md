# Validation Plan: google-jules-integration

**Date**: 2026-09-01

## Happy Path Scenario
Given a user with a resolvable `JulesAPIKey`, `JulesConfig.Enabled = true`, and
the item's repo already present in `EgressAcknowledgedRepos` (baseline per
requirements.md's Success Metrics and plan.md's Risk Control), when the user
opens a `ready` backlog item, clicks **Dispatch to Jules**, and submits an
already-pushed `GitHubBranchRef` and a prompt, then a `JulesSession` is
created via `JulesDispatchService.DispatchToJules`, the item transitions
`ready → in_progress`, `JulesSessionPoller` observes `COMPLETED` with a
`JulesPullRequestOutput` on a later tick and calls
`SetBacklogItemPRAndTransition`, and the PR converges through the existing
`ReconcilePRPending`/`WorktreePRPoller` review path — the same session-list
and PR-review UI a local-agent session uses, satisfying requirements.md's
Success Metrics without the user ever visiting jules.google.com.

Every other scenario below is a variation on this path: a guard rejecting the
dispatch before the billed call, the poller applying a different
`JulesSessionState`, or a UX surface reporting one of the outcomes in
ux.md §6's error table.

---

## Requirement → Test Mapping

Requirements are keyed by plan.md Story ID (the plan's own acceptance
criteria are already Given/When/Then; the "Scenario" column cites the
specific criterion each test proves). Domain Glossary names from plan.md are
used verbatim in test signatures.

### Epic 1.1 — Typed Jules API gateway (`jules/`)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 1.1.1: identifier newtypes reject malformed input | `jules/types_test.go` | `TestParseJulesSourceName_should_ReturnJulesSourceName_When_PrefixValid` | Unit | Happy path — parses `"sources/github-tstapler-stapler-squad"` |
| Story 1.1.1: identifier newtypes reject malformed input | `jules/types_test.go` | `TestParseJulesSourceName_should_RejectMissingPrefix_When_SourcesPrefixAbsent` | Unit | Error path — `"github-tstapler-stapler-squad"` (no `sources/`) returns zero value + error naming the prefix |
| Story 1.1.1: `JulesAPIKey` cannot be printed in full | `jules/types_test.go` | `TestJulesAPIKey_String_should_RedactValue_When_Formatted` | Unit | `%v %s` on a parsed key yields `"jules-api-key(redacted) jules-api-key(redacted)"`, no key substring |
| Story 1.1.1: unrecognized wire state is distinguishable | `jules/types_test.go` | `TestJulesSessionState_UnmarshalJSON_should_ParseAsUnknown_When_WireValueUnrecognized` | Unit | `"AWAITING_HUMAN_TEA_BREAK"` decodes to `JulesStateUnknown`, `IsKnown()==false`, `Raw()` preserved |
| Story 1.1.2: `JulesClient` authenticates and shapes requests | `jules/client_test.go` | `TestJulesClient_GetSession_should_SendGoogApiKeyHeader_When_Called` | Integration | `httptest.Server` observes one `GET /v1alpha/sessions/abc` with `x-goog-api-key` set, `Authorization` empty |
| Story 1.1.2: `CreateSession` body shape | `jules/client_test.go` | `TestJulesClient_CreateSession_should_SendFireAndForgetBody_When_Called` | Integration | Request body decodes to the exact `{"prompt",...,"automationMode":"AUTO_CREATE_PR"}` shape |
| Story 1.1.2: `GetSession` surfaces the PR | `jules/client_test.go` | `TestJulesClient_GetSession_should_ParsePullRequestOutput_When_SessionCompleted` | Integration | `Outputs[0].PullRequest.URL` populated from a `COMPLETED` fixture response |
| Story 1.1.2: package isolation (requirements.md Constraints) | `jules/client_test.go` | `TestJulesPackage_should_NotImportSessionOrServer_When_DepsListed` | Unit (fitness fn) | `go list -deps ./jules` contains no `session/`/`server/` import path |
| Story 1.1.3: HTTP failures classify to sentinels | `jules/errors_test.go` | `TestClassifyJulesResponse_should_MapStatusToSentinel_When_TableDriven` | Unit | 401/403→`ErrJulesNotConfigured`, 404→`ErrJulesSessionNotFound`, 429→`ErrJulesRateLimited`, 5xx→`ErrJulesTransient` |
| Story 1.1.3: a 403 never leaks key material in the error | `jules/errors_test.go` | `TestClassifyJulesResponse_should_ExcludeKeyMaterial_When_BodyEchoesKey` | Unit | Error string contains neither the key nor `x-goog-api-key` |
| Story 1.1.3: rate limiter arms and disarms | `jules/client_test.go` | `TestJulesClient_GetSession_should_ArmLimiterAndExposeRetryAfter_When_ServerReturns429` | Integration | Real HTTP round trip against `httptest.Server`; `IsLimited()==true`, `RetryAfter()==120s`, disarms after injected-clock advance |
| Story 1.1.4: golden fixtures decode via real DTOs | `jules/golden_test.go` | `TestGoldenFixtures_should_DecodeIntoJulesSession_When_SessionCompleted` | Unit (fixture-based) | `session_completed.json` decodes with `State==JulesStateCompleted` and non-empty PR URL |
| Story 1.1.4: schema drift is loud, not silently absorbed | `jules/golden_test.go` | `TestGoldenFixtures_should_RejectUnknownFields_When_SchemaDrifts` | Unit | `DisallowUnknownFields()` fails naming the unexpected field |

### Epic 1.2 — Credential storage

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 1.2.1: key round-trips through the keychain under the right service name | `jules/keychain_test.go` | `TestKeyringTokenSource_APIKey_should_RoundTripThroughKeyring_When_SetThenRead` | Unit (seam vars) | `SetJulesAPIKey`→`APIKey` round-trip targets `"stapler-squad-jules"`, not the GitHub/SSH service names |
| Story 1.2.1: absent key is "feature off", not a retry-worthy error | `jules/keychain_test.go` | `TestKeyringTokenSource_APIKey_should_ReturnErrJulesNotConfigured_When_KeyringEmpty` | Unit | Empty keyring → `errors.Is(err, ErrJulesNotConfigured)` |
| Story 1.2.1: a hung Secret Service does not hang the server, the first time | `jules/keychain_test.go` | `TestKeyringTokenSource_APIKey_should_ReturnWithinFiveSecondsOnFirstCall_When_SecretServiceHangs` | Integration (external-system boundary, stubbed D-Bus) | Stub blocks 30s; the first `APIKey` call returns within ~5s via the timeout-raced synchronous read (same guard as `session/sshremote/keystore.go:171`), then opens the circuit |
| Story 1.2.1 (pre-mortem P1 #4): a wedged keychain bounds concurrent probes to one, not one per caller | `jules/keychain_test.go` | `TestKeyringTokenSource_APIKey_should_BoundConcurrentHungKeychainProbesToOne_When_FiftyCallsRaceDuringOutage` | Integration (fake keychain, controllable hang) | Keyring stub blocks on a test-controlled channel; the first call times out and opens the circuit, then 50 more calls (simulating 50 poll ticks/HTTP requests during the outage) each return within 1ms as `ErrJulesKeychainPaused`; assertion is on an injected `onProbeStart` hook delivering exactly one signal over a bounded-wait channel (`select` with a short timeout) — **not** `runtime.NumGoroutine()`, which the pre-mortem itself flags as flake-prone under `-race`/parallel tests. Test then unblocks the stub's channel so the one background probe goroutine can exit cleanly before the test ends |
| Story 1.2.1: a resolved key is served from cache, not re-resolved every call | `jules/keychain_test.go` | `TestKeyringTokenSource_APIKey_should_ServeFromCacheWithoutReResolving_When_CalledWithinTTL` | Unit (seam vars, injected clock) | A succeeding keyring stub with `cacheTTL` test-shortened; 10 calls within the TTL window → underlying keyring `Get` seam invoked exactly once, remaining 9 calls return the cached value |
| Story 1.2.1: the paused-feature degraded mode surfaces `ErrJulesKeychainPaused` and logs once | `jules/keychain_test.go` | `TestKeyringTokenSource_APIKey_should_ReturnErrJulesKeychainPausedAndLogOnce_When_CircuitOpen` | Unit (captured `slog.Handler`) | With the circuit open, every call returns an error satisfying both `errors.Is(err, ErrJulesKeychainPaused)` and `errors.Is(err, ErrJulesNotConfigured)` (existing "feature off" handling applies unchanged); the captured log shows exactly one `jules keychain paused` record at `Warn` across all calls in the window, not one per call |
| Story 1.2.1: the circuit reopens for exactly one probe after cooldown and closes on success | `jules/keychain_test.go` | `TestKeyringTokenSource_APIKey_should_ReopenForOneProbeAndCloseOnSuccess_When_CooldownElapses` | Integration (fake keychain, injected clock) | Circuit opened after a timeout with `cooldown = 1s` test-injected; clock advanced past cooldown and the stub now succeeds → exactly one probe attempted, it succeeds, cache populated, circuit closes, and a subsequent call within the new TTL hits the cache with zero further keyring calls |
| Story 1.2.2: provider `"jules"` resolves through `CredentialChain` | `server/services/credentials_test.go` | `TestCredentialChain_Resolve_should_ReturnKeychainCredential_When_ProviderIsJules` | Unit | Chain returns `Credential{Source:"jules_keychain", Token:"AIzaSyD-EXAMPLE"}` |
| Story 1.2.2: env override still wins | `server/services/credentials_test.go` | `TestCredentialChain_Resolve_should_PreferEnvVarOverKeychain_When_JulesAPIKeyEnvSet` | Unit | `JULES_API_KEY=env-key` wins over a keyring value |
| Story 1.2.2: no `jules/` source path can log the key | `jules/secrets_guard_test.go` | `TestJulesPackage_should_NotLogSecrets_When_SourceScanned` | Integration (whole-package static scan) | `reveal()` appears exactly once, only inside `newRequest`, never in a `slog`/`fmt.Print`/`log.` call |

### Epic 1.3 — Source registry

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 1.3.1: cache hit avoids a second API call | `jules/source_registry_test.go` | `TestJulesSourceRegistry_Resolve_should_ServeFromCacheWithoutSecondCall_When_CalledWithinTTL` | Unit | Second `Resolve` within TTL → same result, fake client recorded exactly one `ListSources` call |
| Story 1.3.1: a miss names the repo and points at jules.google.com | `jules/source_registry_test.go` | `TestJulesSourceRegistry_Resolve_should_ReturnErrJulesSourceNotRegistered_When_RepoAbsentFromSources` | Unit | Error names `tstapler/stapler-squad`, satisfies `errors.Is(err, ErrJulesSourceNotRegistered)` |
| Story 1.3.1: the cache expires | `jules/source_registry_test.go` | `TestJulesSourceRegistry_Resolve_should_RefetchSources_When_TTLExpired` | Unit (fake client, injected clock) | Clock advanced past `TTL=10m` triggers a second `ListSources` call |

### Epic 2.1 — Session role and storage primitives

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 2.1.1: `jules_work` is excluded from the tmux predicate | `session/backlog_test.go` | `TestIsTmuxBackedSessionRole_should_ReturnFalse_When_RoleIsJulesWork` | Unit | `IsTmuxBackedSessionRole(SessionRoleJulesWork)==false`; `work`/`review` still `true` |
| Story 2.1.1: exhaustiveness locks in every declared role | `session/backlog_test.go` | `TestIsTmuxBackedSessionRole_should_CoverEveryDeclaredRole_When_RolesEnumerated` | Unit | Table asserts an exact truth value per role; fails if a new role is added without a row |
| Story 2.1.1: terminal-item sweep skips Jules rows without error | `session/backlog_lifecycle_archive_test.go` | `TestReconcileTerminalItemSessions_should_SkipTmuxKill_When_ItemSessionRoleIsJulesWork` | Integration (storage-backed) | `done` item with an ended `jules_work` row: no tmux-kill attempt logged, item stays `done` |
| Story 2.1.2: open-session listing returns only unfinished Jules rows | `session/storage_backlog_test.go` | `TestListOpenJulesItemSessions_should_ReturnOnlyUnfinishedJulesRows_When_MixedRolesPresent` | Integration | 3-row fixture (ended `jules_work`, open `jules_work`, open `work`) → exactly the one open `jules_work` row returned |
| Story 2.1.2: daily count respects the 24h window | `session/storage_backlog_test.go` | `TestCountJulesItemSessionsSince_should_RespectWindow_When_RowsSpanBoundary` | Integration | Rows at 2h and 30h ago → `CountJulesItemSessionsSince(now-24h)==2` (excludes the 30h row) |
| Story 2.1.2 (pre-mortem P2 #5): the daily count excludes attempts that never billed | `session/storage_backlog_test.go` | `TestCountJulesItemSessionsSince_should_ExcludePendingReservationsAndDispatchFailedRows_When_MixedOutcomesInWindow` | Integration | Three rows in the trailing 24h — one reached a real session (`session_uuid` starts `jules-sessions/`), one still reserved (`session_uuid` starts with `julesPendingUUIDPrefix`), one ended `end_reason="dispatch_failed"` — → `CountJulesItemSessionsSince(now-24h)==1`, proving a bad key or Jules outage doesn't burn the daily cap on failed attempts |
| Story 2.1.2: progress touch updates only `last_progress_at` | `session/storage_backlog_test.go` | `TestTouchItemSessionProgress_should_UpdateOnlyLastProgressAt_When_Called` | Integration | `last_commit_sha`, `base_commit_sha`, `commit_count_since_spawn` stay at zero value |
| Story 2.1.3 (pre-mortem P1 #1): `hasActiveSession` recognizes an open Jules session | `session/backlog_lifecycle_test.go` | `TestHasActiveSession_should_ReturnTrue_When_OpenJulesSessionPresent` | Unit | `[]ItemSessionSummary{{Role: SessionRoleJulesWork, EndedAt: nil}}` → `true`; the same row with `EndedAt` set → `false` |
| Story 2.1.3: `hasActiveSession` preserves existing `work`/`review` gating unchanged (regression) | `session/backlog_lifecycle_test.go` | `TestHasActiveSession_should_PreserveWorkAndReviewGating_When_JulesRoleAdded` | Unit (table) | Table over `work`/`review`/`triage`/`jules_work` × `EndedAt` nil/set asserts the exact pre-existing truth value for `work`/`review`/`triage` is untouched by folding in the new `jules_work` branch |
| Story 2.1.3: the 8b spawn guard rejects a new local session over an open Jules session (happy path) | `server/services/backlog_service_triage_test.go` | `TestSpawnSessionAfterGates_should_ReturnAlreadyExistsNamingJulesSession_When_OpenJulesSessionPresent` | Integration | Item has one open `jules_work` `ItemSession` and no `work` session → `connect.CodeAlreadyExists` naming a Jules session already running; `SessionCreator.CreateSession` (local tmux path) never called |
| Story 2.1.3: `AutoRespawnAutonomousWork` does not respawn over an open Jules session | `server/services/backlog_service_triage_test.go` | `TestAutoRespawnAutonomousWork_should_SkipRespawn_When_OpenJulesSessionPresent` | Integration | Item in `in_progress` with one open `jules_work` session, no `work` session → returns without spawning, logs the same "respawn blocked, active session found" shape as the existing work-session case, no new `ItemSession` row created |
| Story 2.1.3: `AutoRespawnReview` does not respawn a review session over an open Jules session | `server/services/backlog_service_triage_test.go` | `TestAutoRespawnReview_should_SkipRespawn_When_OpenJulesSessionPresent` | Integration | Mirrors the existing `findActiveWorkSession`/`findActiveReviewSession` block; no review session spawned |
| Story 2.1.3: steering a Jules session fails loudly instead of attempting a tmux write (error/edge path — the `AutoReopenForPRFix` steering exception) | `server/services/backlog_service_triage_test.go` | `TestSteerActiveSessionForPRFix_should_LogErrorAndSkipTmuxWrite_When_ActiveSessionIsJulesWork` | Unit | Active-session lookup constructed directly in the test with `Role == SessionRoleJulesWork` (not via the poller's real end-before-PR invariant) → returns an error / logs at `Error` naming the invariant violation, makes no tmux write |
| Story 2.1.3: `countLiveBacklogWorkSessions` is deliberately untouched by the Jules gating change (documents the explicit out-of-scope carve-out) | `server/services/backlog_service_triage_test.go` | `TestCountLiveBacklogWorkSessions_should_ExcludeJulesWorkRows_When_MixedRolesPresent` | Unit | An open `jules_work` row does not increment the count feeding `MaxConcurrentBacklogWorkItems` — a separate WIP pool from `MaxConcurrentJulesSessions` |

### Epic 2.2 — Dispatch service

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 2.2.1: successful dispatch reserves → creates → confirms → transitions | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_ReserveCreateConfirmAndTransition_When_ClientSucceeds` | Unit (fake `julesSessionCreator`, fake `julesTransitionGuard`) | Ordered assertions over the full 6-step guard sequence (Task 2.2.1b): reservation row, one `CreateSession` call, `session_uuid` becomes `jules-sessions/xyz`, `transitionWithGuard` called with `hasUnresolvedBlockers` sourced from the fake `julesTransitionGuard.hasUnresolvedBlockers` returning `false` (not a hardcoded literal at the call site), `ready→in_progress` |
| Story 2.2.1 (persisted-state guard, Task 2.2.1b step 2) — a sequential second dispatch is refused by durable state alone, after the mutex has already been released | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_ReturnErrJulesDispatchInFlight_When_PersistedOpenSessionFoundAfterFirstCallReturned` | Unit | First `DispatchToJules` call returns (mutex now free, `jules_work` row persisted); a second, non-racing call for the same item → `errors.Is(err, ErrJulesDispatchInFlight)` sourced from `ListOpenJulesItemSessions`, zero `CreateSession` calls on the second call — distinct code path from the mutex guard below per Task 2.2.1c's reviewer-visibility requirement |
| Story 2.2.1 (in-process mutex guard, Task 2.2.1a `TryLock`) — concurrent double-clicks collapse to one create before any persisted row exists | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_CollapseConcurrentDoubleClicksToOneCreateViaMutex_When_TwoGoroutinesRaceBeforePersistedRowExists` | Integration (in-memory storage + goroutines) | Two simultaneous calls, `CreateSession` blocking 50ms → exactly one success, the other returns `ErrJulesDispatchInFlight` from `TryLock` (asserted as originating from the mutex, not `ListOpenJulesItemSessions`, since no row exists yet for either goroutine to find), one `CreateSession` call |
| Story 2.2.1 (blocker gate, Task 2.2.1b step 5) — an item with an unresolved blocker is rejected before any reservation or billed call | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_RejectWithErrUnresolvedBlockers_When_ItemHasUnresolvedBlocker` | Unit (fake `julesTransitionGuard.hasUnresolvedBlockers` returns `true`) | `errors.Is(err, session.ErrUnresolvedBlockers)`, zero `ItemSession` reservation rows created, zero `CreateSession` calls — proves the real `hasUnresolvedBlockers` gate is wired in, not the pre-redesign hardcoded `false` |
| Story 2.2.1: create failure leaves no orphan claim | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_EndReservationWithDispatchFailedReason_When_CreateSessionFails` | Unit | Reservation ends with `end_reason="dispatch_failed"`, item stays `ready`, progress note appended |
| Story 2.2.2: concurrency ceiling blocks a dispatch | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_RejectWithConcurrencyCapMessage_When_OpenSessionsAtCeiling` | Unit | `FailedPrecondition`-mapped error naming `"2 Jules sessions are already running (limit 2)"`, no `CreateSession` call |
| Story 2.2.2: daily cap blocks a dispatch even with nothing running | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_RejectWithDailyCapMessage_When_TwentyFourHourCountAtLimit` | Integration (storage-backed count query) | 15 rows in the trailing 24h, zero open → error names the daily limit, no `CreateSession` call |
| Story 2.2.2: config values are clamped, never trusted raw | `config/config_test.go` | `TestMaxConcurrentJulesSessionsOrDefault_should_ClampToHardCeilingOrDefault_When_ConfigOutOfRange` | Unit | `500→10` (ceiling), `0→2` (default) |
| Story 2.2.3: an unacknowledged repo is refused, naming the repo | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_RejectNamingRepo_When_EgressNotAcknowledged` | Unit | Error names `tstapler/stapler-squad`, no `CreateSession` call |
| Story 2.2.3: an already-acknowledged repo proceeds without re-confirmation | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_ProceedWithoutReconfirmation_When_RepoAlreadyAcknowledged` | Integration (fake client + storage) | Repo pre-populated into `JulesConfig.EgressAcknowledgedRepos` via a prior `ConfirmEgressConsent` call (Story 2.4.2), never via the request itself; dispatch proceeds normally — supersedes the pre-redesign row this replaced, which incorrectly had `DispatchToJules` itself writing to `EgressAcknowledgedRepos` (see pre-mortem P1 #3) |
| Story 2.2.3: `Enabled:false` refuses regardless of key/ack | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_ReturnErrJulesNotConfigured_When_EnabledFalse` | Unit | `errors.Is(err, ErrJulesNotConfigured)` even with valid key and acknowledged repo |
| Story 2.2.3: `DispatchToJules` cannot create a new `EgressAcknowledgedRepos` entry under any call pattern | `server/services/jules_dispatch_service_test.go` | `TestJulesDispatchService_DispatchToJules_should_LeaveEgressAcknowledgedReposUnchanged_When_CalledTwentyTimesForUnacknowledgedRepo` | Unit + signature check | 20 repeated calls for an unacknowledged item → persisted config read back after each call shows no change; a compile-level check asserts `checkEgressConsent`'s signature carries no boolean/`EgressAcknowledged`-shaped parameter |

### Epic 2.3 — `JulesSessionPoller`

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 2.3.1: one tick polls each open session exactly once | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_PollEachOpenSessionExactlyOnce_When_ThreeSessionsOpen` | Unit (fake `julesStatusClient`) | 3 open sessions → 3 `GetSession` calls, one per `JulesSessionName` |
| Story 2.3.1: a rate-limited client skips the whole tick | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_SkipTickEntirely_When_ClientIsRateLimited` | Unit | Zero `GetSession` calls, one `jules poll tick` line recording `skipped_rate_limited` |
| Story 2.3.1: one failing session does not abort the others | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_ApplyRemainingSessions_When_OneSessionGetSessionFails` | Unit | 3 sessions, 2nd fails with `ErrJulesTransient` → all 3 attempted, 1st/3rd applied, one `Warn` log |
| Story 2.3.1: `Start` is cancellable and idempotent | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_Start_should_ReturnWithinOneTickInterval_When_ContextCancelled` | Integration (real ticker/goroutine lifecycle) | Cancel returns the goroutine within one tick interval; second `Start` is a no-op |
| Story 2.3.2: non-terminal states touch progress only | `session/jules_session_poller_test.go` | `TestApplyJulesState_should_TouchProgressOnly_When_StateIsNonTerminal` | Unit | `IN_PROGRESS` → `TouchItemSessionProgress` called, no transition/PR call |
| Story 2.3.2: a state change is one note; unchanged is zero | `session/jules_session_poller_test.go` | `TestApplyJulesState_should_AppendExactlyOneNote_When_StateChangesThenRepeats` | Unit | `QUEUED→PLANNING` writes one note; a repeated `PLANNING` poll writes none |
| Story 2.3.2: `COMPLETED` with a PR hands off to `ReconcilePRPending` | `session/jules_session_poller_test.go` | `TestApplyJulesState_should_RecordPRAndHandOffToReconcilePRPending_When_CompletedWithPullRequestOutput` | Integration (storage-backed) | `SetBacklogItemPRAndTransition` called once with the parsed PR number; session ends `"jules_completed"` |
| Story 2.3.2: `COMPLETED` with no PR is surfaced, not silent success | `session/jules_session_poller_test.go` | `TestApplyJulesState_should_SurfaceMissingPR_When_CompletedWithEmptyOutputs` | Unit | Item stays out of `pr_pending`; session ends `"jules_completed_no_pr"`; note points at the Jules web URL |
| Story 2.3.2: an unknown state is loud | `session/jules_session_poller_test.go` | `TestApplyJulesState_should_LogUnknownStateAtError_When_StateIsUnrecognized` | Unit | `Error`-level `jules unknown session state` with `raw_state`; progress touched; no transition |
| Story 2.3.2: exhaustiveness is enforced | `session/jules_session_poller_test.go` | `TestApplyJulesState_should_HandleEveryDeclaredState_When_StatesEnumerated` | Unit | Every exported `JulesSessionState` constant has a non-default effect asserted |
| Story 2.3.3: `FAILED` ends session and returns item to `ready` with Jules' message | `session/jules_session_poller_test.go` | `TestApplyJulesState_should_ReturnItemToReadyWithJulesMessage_When_StateIsFailed` | Unit | Session ends `"jules_failed"`, `in_progress→ready`, note has Jules' text + session URL |
| Story 2.3.3: a vanished session does not retry forever | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_EndSessionAsSessionMissing_When_GetSessionReturnsNotFound` | Unit | `ErrJulesSessionNotFound` → ends `"jules_session_missing"`, item → `ready` |
| Story 2.3.3: a session exceeding `MaxSessionAge` is failed, not polled forever | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_TimeOutSession_When_StartedAtExceedsMaxSessionAge` | Unit (injected clock) | 25h-old session with `MaxSessionAge=24h` → ends `"jules_timed_out"`, no further `GetSession` call next tick |
| Story 2.3.3: an abandoned reservation is cleaned up | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_FailAbandonedReservation_When_PendingOlderThanTenMinutes` | Integration (storage + injected clock) | `jules-pending-` row 15m old → ends `"dispatch_incomplete"`, item → `ready`, actionable note |
| Story 2.3.4: a 401/403 mid-poll is distinguished from staleness/failure, not silently swallowed | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_SetAuthReconnectRequiredAndSkipSession_When_GetSessionReturnsErrJulesNotConfigured` | Unit | `GetSession` error satisfies `errors.Is(err, jules.ErrJulesNotConfigured)` → session not ended, item not transitioned, `TouchItemSessionProgress` not called for it, `AuthReconnectRequired()==true` |
| Story 2.3.4: the reauth condition is surfaced once per occurrence, not once per tick | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_AppendReauthNoteExactlyOnce_When_ThreeConsecutiveTicksReturnErrJulesNotConfigured` | Unit | Three ticks all returning `ErrJulesNotConfigured` for the same open session → exactly one `AppendProgressNote(itemID, -1, "Jules session needs reauthentication — update your API key in Settings.", "blocked")` |
| Story 2.3.4: recovery is automatic on the next successful poll, no manual retry | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_ClearAuthReconnectRequiredAndAppendRecoveryNote_When_SubsequentTickSucceeds` | Unit | `AuthReconnectRequired()==true` going in; a tick whose `GetSession` succeeds for any open session → `AuthReconnectRequired()==false` after, one recovery `AppendProgressNote(itemID, -1, "Jules reconnected — resuming normal polling.", "in_progress")` per item with an outstanding blocked note, ordinary `applyJulesState` handling resumes next tick |
| Story 2.3.4: the flag is process-level, not per-session | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_SetAuthReconnectRequiredOnceNotPerSession_When_TwoSessionsOnDifferentItemsBothFail` | Unit | Two open sessions on different items both return `ErrJulesNotConfigured` in the same tick → `AuthReconnectRequired()` is `true` exactly once (not toggled twice), each item still gets its own dedup'd note |
| Story 2.3.4: every other error path is unaffected | `session/jules_session_poller_test.go` | `TestJulesSessionPoller_tick_should_LeaveAuthReconnectRequiredUntouched_When_TransientOrSessionNotFoundErrorsOccur` | Unit | `ErrJulesTransient` and `ErrJulesSessionNotFound` cases apply their existing Story 2.3.1/2.3.3 handling unchanged; `AuthReconnectRequired()` is not touched either way |

### Epic 2.4 — Proto, RPC, and server wiring

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 2.4.1: the key is never returned to the client | `server/services/jules_config_service_test.go` | `TestJulesConfigService_GetJulesConfig_should_NeverReturnKeyMaterial_When_KeyStored` | Unit | Response has `has_api_key==true`, no field holds the key or a prefix of it |
| Story 2.4.1: updating the key writes the keychain, not `config.json` | `server/services/jules_config_service_test.go` | `TestJulesConfigService_UpdateJulesConfig_should_WriteKeychainNotConfigJSON_When_APIKeyProvided` | Integration (keychain + config persistence) | `KeyringTokenSource` holds the new key; marshalled `config.json` bytes contain no occurrence of it |
| Story 2.4.1: connection test names the concrete prerequisite | `server/services/jules_config_service_test.go` | `TestJulesConfigService_TestJulesConnection_should_NameUnconnectedRepo_When_SourceNotInListSources` | Unit | `ok:false`, message names `tstapler/stapler-squad` + jules.google.com |
| Story 2.4.1: the reconnect-required flag is read live from the poller, not persisted config | `server/services/jules_config_service_test.go` | `TestJulesConfigService_GetJulesConfig_should_ReflectPollerAuthReconnectRequiredLive_When_PollerFlagToggles` | Integration (real `*session.JulesSessionPoller` dependency, not a fake) | With the poller's `AuthReconnectRequired()==true`, `GetJulesConfig` response has `auth_reconnect_required==true`; after the flag clears on the poller's own next successful tick (Story 2.3.4), a subsequent `GetJulesConfig` call returns `false` — no separate poll or cache on the RPC side |
| Story 2.4.1: a nil poller (feature off / not started) never reports reconnect-required | `server/services/jules_config_service_test.go` | `TestJulesConfigService_GetJulesConfig_should_ReturnAuthReconnectRequiredFalse_When_PollerDependencyIsNil` | Unit | `deps.JulesSessionPoller == nil` → `auth_reconnect_required` is always `false`, no nil-pointer panic |
| Story 2.4.2: confirming appends and persists the repo (happy path) | `server/services/jules_config_service_test.go` | `TestJulesConfigService_ConfirmEgressConsent_should_AppendAndPersistRepo_When_RepoNotAlreadyAcknowledged` | Integration (config persistence) | Repo appended to `EgressAcknowledgedRepos`, change persisted to `config.json`, response echoes the updated list |
| Story 2.4.2: confirming is idempotent | `server/services/jules_config_service_test.go` | `TestJulesConfigService_ConfirmEgressConsent_should_AvoidDuplicateEntry_When_RepoAlreadyAcknowledged` | Unit | Repo already present → no duplicate entry added, call still succeeds |
| Story 2.4.2: `DispatchToJules` cannot reach this write path (error path — proves the RPC alone can't create a new entry) | `server/services/jules_config_service_test.go` | `TestJulesDispatchService_should_NeverCallConfirmEgressConsentMutation_When_SourceScanned` | Integration (whole-file static scan) | Greps `server/services/jules_dispatch_service.go` for any call to the config-mutation function backing `ConfirmEgressConsent` → zero occurrences outside `jules_config_service.go` itself |
| Story 2.4.2: a malformed repo path is rejected, not silently accepted into the allowlist | `server/services/jules_config_service_test.go` | `TestJulesConfigService_ConfirmEgressConsent_should_ReturnInvalidArgument_When_RepoPathEmpty` | Unit | `repo_path:""` → `connect.CodeInvalidArgument`, `EgressAcknowledgedRepos` unchanged |
| Story 2.4.3: a valid request dispatches and returns the `ItemSession` | `server/services/jules_dispatch_service_test.go` | `TestBacklogService_DispatchToJules_should_ReturnItemSessionWithJulesWorkRole_When_RequestValid` | Unit | Response `ItemSession.role=="jules_work"`, `session_uuid` starts `"jules-sessions/"`, item `in_progress` |
| Story 2.4.3: a missing branch is `InvalidArgument`, not a generic error | `server/services/jules_dispatch_service_test.go` | `TestBacklogService_DispatchToJules_should_ReturnInvalidArgument_When_BranchEmpty` | Unit | `connect.CodeInvalidArgument`, message states the pushed-branch requirement |
| Story 2.4.3: guard rejections map to `FailedPrecondition`, not `Internal` | `server/services/jules_dispatch_service_test.go` | `TestBacklogService_DispatchToJules_should_ReturnFailedPrecondition_When_ConcurrencyCeilingReached` | Integration (RPC handler + storage) | `connect.CodeFailedPrecondition` returned so the UI can show the reason inline |
| Story 2.4.3: unconfigured Jules is unavailable, not broken | `server/services/jules_dispatch_service_test.go` | `TestBacklogService_DispatchToJules_should_ReturnFailedPreconditionPointingAtSettings_When_JulesDisabled` | Unit | `connect.CodeFailedPrecondition`, message points at Settings → Jules |
| Story 2.4.3: an unacknowledged repo is rejected by the RPC the same way the service rejects it | `server/services/jules_dispatch_service_test.go` | `TestBacklogService_DispatchToJules_should_ReturnFailedPreconditionDirectingToDispatchDialog_When_RepoNotAcknowledged` | Unit | `connect.CodeFailedPrecondition`, message directs the user to confirm cloud egress in the dispatch dialog; there is no request field the caller could set instead to bypass this |
| Story 2.4.4: the poller starts only when enabled | `server/server_test.go` | `TestServer_should_LogJulesSessionPollerStarted_When_JulesEnabledAndKeyResolvable` | Integration (server startup) | `"JulesSessionPoller started"` logged beside `"WorktreePRPoller started"` |
| Story 2.4.4: the poller stays nil and silent when disabled | `server/server_test.go` | `TestServer_should_LeaveJulesSessionPollerNil_When_JulesConfigDisabled` | Integration | No `jules poll tick` line ever logged; `deps.JulesSessionPoller` is `nil` |
| Story 2.4.4: construction failures degrade the feature, not the server | `server/dependencies_test.go` | `TestServerDependencies_should_DegradeFeatureNotServer_When_KeychainUnreadableAtStartup` | Integration | Server starts normally, logs `jules disabled` once at `Info`, every other subsystem unaffected |

### Epic 3.1–3.3 — Frontend components

Jest/RTL naming follows this repo's existing `it("should ...")` convention
(not Go `TestX_should_Y_When_Z` — see `web-app/src/**/*.test.tsx`); ARIA
roles/`data-testid` locators only, per the `e2e-test-conventions` skill.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 3.1.1: key input is write-only and masked | `web-app/src/components/settings/JulesSettings.test.tsx` | `it("renders the key field empty with the stored-key placeholder and type=password, never the real key")` | Unit | DOM contains no key characters after a stored key loads |
| Story 3.1.1: connection test surfaces the actionable prerequisite | `web-app/src/components/settings/JulesSettings.test.tsx` | `it("shows the not-connected message naming the repo when Test connection targets an unregistered source")` | Unit (RPC mocked) | `role="status"` message names `tstapler/stapler-squad` + jules.google.com |
| Story 3.1.1: acknowledged repos are listed and revocable | `web-app/src/components/settings/JulesSettings.test.tsx` | `it("calls UpdateJulesConfig without the repo when Revoke is clicked")` | Integration (mocked ConnectRPC round trip) | Clicking Revoke removes the repo via `UpdateJulesConfig`; row disappears |
| Story 3.2.1: branch field is pre-filled from the item's most recent tracked branch, never opened blank when one is known | `web-app/src/components/backlog/JulesDispatchDialog.test.tsx` | `it("pre-fills the Branch field with the item's most recent non-empty worktree_branch and keeps it editable")` | Unit | Dialog opens with `initialBranch="backlog/fix-flaky-poller-test"` (the newest non-empty `worktree_branch` across the item's sessions) → Branch field's initial value equals it, not blank, and remains focusable/editable before submit |
| Story 3.2.1: confirmation names the concrete repo and gates Dispatch | `web-app/src/components/backlog/JulesDispatchDialog.test.tsx` | `it("disables Dispatch until the named-repo egress checkbox is checked for an unacknowledged repo")` | Unit | Checkbox unchecked → `Dispatch` disabled even with branch+prompt filled |
| Story 3.2.1: an already-acknowledged repo does not re-prompt | `web-app/src/components/backlog/JulesDispatchDialog.test.tsx` | `it("omits the egress confirmation block when the repo is already acknowledged")` | Unit | No confirmation block rendered; `Dispatch` enables on branch+prompt alone |
| Story 3.2.1: branch requirement is explained on focus | `web-app/src/components/backlog/JulesDispatchDialog.test.tsx` | `it("shows the pushed-branch helper text on focus and keeps Dispatch disabled for an empty branch")` | Unit | Helper text reads the exact copy from the acceptance criterion |
| Story 3.2.2: hidden when the feature is off | `web-app/src/components/backlog/detail/ActionsSection.jules.test.tsx` | `it("renders no dispatch-to-jules element when GetJulesConfig reports enabled:false")` | Unit | `data-testid="dispatch-to-jules"` absent |
| Story 3.2.2: disabled with a reason when enabled without a key | `web-app/src/components/backlog/detail/ActionsSection.jules.test.tsx` | `it("disables the button with the add-a-key description when enabled but has_api_key is false")` | Unit | Accessible description reads the exact copy |
| Story 3.2.2: disabled while a Jules session is already open | `web-app/src/components/backlog/detail/ActionsSection.jules.test.tsx` | `it("disables the button with the already-running description when an open jules_work session exists")` | Unit | Description reads `"A Jules session is already running for this item."` |
| Story 3.2.2: disabled when the item has no branch to dispatch | `web-app/src/components/backlog/detail/ActionsSection.jules.test.tsx` | `it("disables the button with the no-branch description when enabled, keyed, no open session, but zero ItemSession rows carry a worktree_branch")` | Unit | Description reads `"This item has no branch yet — spawn a local session (or push a branch) before dispatching to Jules."` |
| Story 3.3.1: never color-alone | `web-app/src/components/backlog/JulesStatusBadge.test.tsx` | `it("renders a distinct icon, the visible text, and a matching aria-label for the running phase")` | Unit | Assertions made without reference to any color value |
| Story 3.3.1: nothing renders before a real state is known | `web-app/src/components/backlog/JulesStatusBadge.test.tsx` | `it("returns null when phase is undefined")` | Unit | No neutral/optimistic placeholder chip |
| Story 3.3.1: a stale poll is labeled, not disguised as failure | `web-app/src/components/backlog/JulesStatusBadge.test.tsx` | `it("keeps the running label and adds retrying text when pollHealthy is false, without switching to failed")` | Unit | Badge text stays `Jules: Running` + `Last updated 8m ago, retrying…` |
| Story 3.3.2: a Jules row renders the badge instead of a branch chip | `web-app/src/components/backlog/detail/SessionsSection.jules.test.tsx` | `it("renders JulesStatusBadge with no branch badge and no SessionMonitor for a jules_work row")` | Unit | Role-based assertions on the row's children |
| Story 3.3.2: terminal Jules rows read as ended, not stuck | `web-app/src/components/backlog/detail/SessionsSection.jules.test.tsx` | `it("shows the failed badge without the generic orphan-ended treatment for an ended jules_work row")` | Unit | Distinguishes from the leaked-local-session treatment |
| Story 3.3.2: PR provenance marker | `web-app/src/components/backlog/detail/PullRequestSection.test.tsx` | `it("renders the unmodified GitHubBadge plus an adjacent Jules marker when the PR-producing session role is jules_work")` | Integration (cross-component) | `GitHubBadge` unchanged; marker has a distinct `aria-label` |

### Epic 4.1 — Observability, registry, E2E

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 4.1.1: every documented log line exists with no secret material | `server/services/jules_usage_counter_test.go` | `TestJulesFullCycle_should_LogNoSecretMaterial_When_DispatchPollCompleteCycleRunsAgainstFakes` | Integration (dispatch+poll cycle, captured `slog.Handler`) | Captured records include the four named `msg` values; none contain the test key or `x-goog-api-key` |
| Story 4.1.1: counters increment exactly once per event | `server/services/jules_usage_counter_test.go` | `TestJulesUsageCounter_Snapshot_should_IncrementDispatchedAndAPIErrorExactlyOnce_When_OneDispatchAndOnePollFailure` | Unit | `jules.session.dispatched==1`, `jules.api.error==1` |
| Story 4.1.2: registry generation is idempotent | CI check (no test file — `make registry-generate`) | N/A — `make registry-generate` run twice produces no diff | Tooling/CI | `docs/registry/features/backend/backlog/dispatch-to-jules.json` has `"markerFound": true` |
| Story 4.1.3: gated-off state end to end | `tests/e2e/jules-dispatch.spec.ts` | `test("dispatch-to-jules is not attached when Jules is disabled")` | E2E | Isolated test server, Jules disabled → `[data-testid="dispatch-to-jules"]` not attached |
| Story 4.1.3: egress confirmation blocks dispatch until checked | `tests/e2e/jules-dispatch.spec.ts` | `test("Dispatch stays disabled until the egress checkbox is checked, then enables")` | E2E | Fill branch+prompt, box unchecked → `[disabled]`; check it → enabled |

---

## Test Case Summary

| Type | Count |
|---|---|
| Go Unit | 59 |
| Go Integration | 31 |
| Frontend Unit/Integration (Jest/RTL) | 19 |
| E2E (Playwright, incl. UX table) | 16 |
| Manual (screen-reader pass, §7.6; colorblind judgment half of §7.4) | 1 dedicated + 1 mixed-mode |
| Tooling/CI (registry idempotency) | 1 |
| **Total distinct test cases** | **127** |

Counts are of distinct tests: the UX table's §5.2 row is a cross-reference to
an already-counted `session/jules_session_poller_test.go` test, not a new
case, so it is excluded here.

**Requirements coverage**: 29/29 (100%) of plan.md's stories — every Story
1.1.1 through 4.1.3, including the newly added/renumbered Story 2.1.3,
Story 2.4.2, Story 2.4.3, Story 2.4.4, and the newly added Story 2.3.4
(auth-reconnect-required detection and automatic recovery) — have at least
one corresponding row in the Requirement → Test Mapping tables above.

---

## UX Acceptance Tests

One row per criterion in `design/ux.md` §7 (16 numbered) plus §5.1/§5.2's own
per-surface criteria not already covered by the numbered list. Tool is
Playwright (`ui-playwright` skill / `tests/e2e/` conventions) for anything
automatable through the DOM/accessibility tree; genuinely human-only checks
(live screen-reader output, visual grayscale/colorblind simulation) are
marked **Manual** — Playwright cannot drive VoiceOver/NVDA or a colorblind
filter, so these stay a documented manual pass gating ship, per
`design/ux.md`'s own framing ("human-verifiable... not by reading source").

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| §7.1 Settings round trip in ≤4 actions | `tests/e2e/jules-settings.spec.ts` | `test("configures Jules — key, enable, test connection — in 4 actions")` | Playwright | Open Settings→Jules → paste key + Save → toggle Enable → Test connection; assert success `role="status"` text and count the 4 actions |
| §7.2 Dispatch happy path in ≤3 clicks | `tests/e2e/jules-dispatch.spec.ts` | `test("dispatches to Jules in 3 clicks when repo already acknowledged")` | Playwright | Dispatch to Jules → fill/confirm prefilled branch+prompt → Dispatch; assert new `queued` row appears |
| §7.3 First-use egress confirmation cannot be skipped | `tests/e2e/jules-dispatch.spec.ts` | `test("Dispatch stays disabled until the egress checkbox is checked, then enables")` | Playwright | Unacknowledged repo: attempt submit with box unchecked, confirm zero network calls; check box, confirm it enables |
| §7.4 No color-only signal across every badge state | `tests/e2e/jules-status-badge.spec.ts` | `test("every JulesStatusBadge phase is distinguishable under a grayscale filter")` | Playwright + Axe Core (`ui-web-design-guidelines`) for the automatable half; **Manual** grayscale/colorblind-simulator pass for the visual judgment | Render each of the six phase variants; assert icon+text present per phase (automated), then visually confirm distinctness with color removed (manual) |
| §7.5 Staleness never reads as failure | `tests/e2e/jules-status-badge.spec.ts` | `test("a throttled poll keeps the Running label and adds retrying text, never the Failed variant")` | Playwright (network throttling via CDP) | Block the Jules API mid-session, assert badge text/role stay on `Running` + secondary "retrying…" text throughout the outage |
| §7.6 Failure is announced, not just displayed | Manual checklist (`docs/how-to/dispatch-work-to-google-jules.md` ship checklist) | `Manual: VoiceOver/NVDA pass — Failed interrupts, Queued→Running does not` | **Manual** (screen reader) | With VoiceOver/NVDA running, drive a session to `Failed`; confirm an assertive interruption; separately drive `Queued→Running`; confirm no interruption |
| §7.7 Every error state has a visible exit path | `tests/e2e/jules-dispatch.spec.ts` | `test("every ux.md §6 error state exposes an in-view actionable next step")` | Playwright, parametrized over ux.md §6's table rows | For each stubbed rejection reason, assert a clickable/actionable element is present in the same view (no reload/source-reading required) |
| §7.8 Focus discipline in the dispatch dialog | `web-app/src/components/backlog/JulesDispatchDialog.test.tsx` | `it("traps Tab at the dialog boundary and returns focus to the opening button on close")` | Jest + RTL (mirrors `BacklogItemDetail.focusReturn.test.tsx`) | Tab past the last control wraps to the first; Esc/Cancel returns focus to `Dispatch to Jules` |
| §7.9 Keyboard-only completion | `tests/e2e/jules-dispatch.spec.ts` | `test("completes the full dispatch flow with keyboard only")` | Playwright (`page.keyboard.*`, no `page.mouse.*`) | Open dialog → check confirmation → fill branch/prompt → submit, using only keyboard input |
| §7.10 Screen-reader labels distinct from `title` | `tests/e2e/jules-status-badge.spec.ts` | `test("every icon-bearing element has a non-empty accessible name distinct from its title attribute")` | Playwright accessibility-tree snapshot (`page.accessibility.snapshot()`) | Assert `aria-label`/accessible name on badge, escape-hatch link, Revoke button, PR marker |
| §7.11 Color contrast ≥4.5:1, both themes | `tests/e2e/jules-status-badge.spec.ts` | `test("every JulesStatusBadge phase meets 4.5:1 contrast in light and dark themes")` | Playwright + Axe Core (color-contrast rule), run against the rendered page in both `data-theme` states | Render each phase variant under both themes; assert Axe reports no contrast violation |
| §7.12 No optimistic flash before real state is known | `tests/e2e/jules-status-badge.spec.ts` | `test("no badge renders before the first Jules state arrives")` | Playwright (throttle initial data fetch) | Delay the item-sessions fetch; assert no chip (neutral or otherwise) is present until real data lands |
| §7.13 Settings changes reflected live, no stale cache | `tests/e2e/jules-dispatch.spec.ts` | `test("revoking egress consent immediately re-shows the confirmation in a freshly opened dialog")` | Playwright | Revoke in Settings → open dispatch dialog for an item in that repo → confirmation checkbox re-appears |
| §7.14 API key never in DOM or client-visible response bodies | `tests/e2e/jules-settings.spec.ts` | `test("no key substring appears in the DOM or the GetJulesConfig network response after saving")` | Playwright (`page.content()` + `read_network_requests`-equivalent response inspection) | Save a key; inspect rendered DOM and the `GetJulesConfig` response body for any substring of the key |
| §7.15 Branch prefill and no-branch gating | `tests/e2e/jules-dispatch.spec.ts` | `test("prefills the Branch field from the item's tracked branch, and disables Dispatch to Jules with a no-branch reason when none exists")` | Playwright | For an item with a prior local session: open the dispatch dialog, assert the Branch field's value equals the tracked branch (never blank); for a fresh item with no prior local session: assert `Dispatch to Jules` is disabled with the no-branch description and the dialog cannot be opened |
| §7.16 Reconnect-required clears itself | `tests/e2e/jules-status-badge.spec.ts` | `test("shows Reconnect required after an auth failure, then reverts to the normal phase automatically once a working key is saved")` | Playwright | With an open session's key artificially invalidated (stubbed 401/403 on the next poll), assert the badge reads `Jules: Reconnect required`; save a working key in Settings and wait one poll interval; assert the badge returns to its normal phase with no "Retry" action clicked |
| §5.1 (Surface D) marker appears only for `jules_work`-sourced PRs | `web-app/src/components/backlog/detail/PullRequestSection.test.tsx` | `it("omits the Jules marker for a PR whose most recent session role is not jules_work")` | Jest + RTL | Non-Jules session role → `GitHubBadge` renders alone, no marker |
| §5.2 (Surface E) exactly one note per state *change* | `session/jules_session_poller_test.go` | `TestApplyJulesState_should_AppendExactlyOneNote_When_StateChangesThenRepeats` (see Epic 2.3 table above) | Unit | Cross-referenced here as the backend half of the UX guarantee that `ProgressHistorySection.tsx` gets zero Jules-specific branches |

---

## Test Stack

- **Unit (Go)**: stdlib `testing` + table-driven tests (`golang-testing` skill conventions), no `testify` dependency introduced beyond what's already in `go.mod`. Fakes are hand-written, narrow, locally-declared interfaces (`julesSessionCreator`, `julesStatusClient`, `julesPollerStorage`) per plan.md's Dependency Inversion choice — no mocking framework needed.
- **Integration (Go)**: same `testing` package, exercising a real boundary — `httptest.Server` for HTTP, the repo's existing in-memory/sqlite-backed `session.Storage` test harness for DB-touching cases, and injected clocks (`now func() time.Time`) instead of real sleeps.
- **Frontend (Jest/RTL)**: `web-app`'s existing `jest` + `@testing-library/react` setup; ARIA roles and `data-testid` only, per `e2e-test-conventions`.
- **E2E / UX**: Playwright against `tests/e2e/global-setup.ts`'s isolated test-mode server (own port, own `STAPLER_SQUAD_TEST_DIR`), driving Jules config through its own settings RPC rather than editing files — per the repo's established E2E conventions and the `ui-playwright` skill.
- **Manual**: screen-reader (VoiceOver/NVDA) interruption behavior and grayscale/colorblind-simulator visual distinctness — the two UX criteria genuinely outside Playwright's reach — run once as a pre-ship checklist item in `docs/how-to/dispatch-work-to-google-jules.md`, not automated.

## Migration Test

**N/A.** plan.md's Migration Plan section states explicitly: "No ent schema
migration and no data backfill" — `SessionRoleJulesWork` is a new *value* of
an existing string column (`ItemSession.session_role`), not a new column or
table, and `session_uuid` already tolerates an arbitrary string per its
existing "loose FK, not an ent edge" documentation
(`session/ent/schema/item_session.go:23-24`). Per Step 5's instruction, this
is treated as N/A and `migration_should_be_reversible` is not designed. The
one forward-compat check the plan does call for — re-reading every
`IsTmuxBackedSessionRole` caller during Story 2.1.1 — is covered above by
`TestIsTmuxBackedSessionRole_should_CoverEveryDeclaredRole_When_RolesEnumerated`
and `TestReconcileTerminalItemSessions_should_SkipTmuxKill_When_ItemSessionRoleIsJulesWork`,
not a schema migration test.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./jules/... ./session/... ./server/... ./config/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with `jules/` and the Jules-specific files in `session/`/`server/services/` as the actual gate (repo-wide 80% is already a `make ci` target; this is scoped to the new code) |
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="Jules" --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line on the five new/touched Jules components |

- All public service methods (`JulesDispatchService.DispatchToJules`, `JulesSessionPoller.tick`/`Start`, `JulesClient`'s three endpoints, `JulesConfigService`'s four RPCs — `GetJulesConfig`/`UpdateJulesConfig`/`TestJulesConnection`/`ConfirmEgressConsent`): happy path + every named error path covered above.
- All external integrations (Jules REST API, OS keychain): unit-mocked (fake `julesSessionCreator`/`julesStatusClient`, seam-var keyring) **and** at least one integration test each — `httptest.Server`-backed HTTP tests for the API, the TTL-cache/circuit-breaker tests for the keychain.
- Every one of `design/ux.md` §7's 14 numbered acceptance criteria, plus §5.1/§5.2's condensed-surface criteria, has a corresponding automated test or an explicit manual checklist entry above — none are left unassigned.
- `go list -deps ./jules` isolation check (Story 1.1.2) and the `reveal()` source-scan (Story 1.2.2) double as architecture-fitness tests enforcing requirements.md's Constraints section in CI, not just at review time.
