# Validation Plan: session-multi-pr-tracking

**Date**: 2026-11-03 (updated by the Phase 4 patch pass and Triad iteration 2)

Derived from `implementation/plan.md` (Stories 1.1 to 11.1), `requirements.md`, `design/ux.md` and ADR-001..004. Nothing here was run; the plan is not yet implemented. Test names use `TestSubject_ShouldX_WhenY` (Go) and `subject should X when Y` (Jest/Playwright). Type names follow the plan's Domain Glossary (`PRRef`, `PullRequestState`, `SessionPR`, `PRView`, `ActivePR`, `ItemPRVerdict`, `PRCleanupGate`, `PRSetReader`, `ScanState`, `ArtifactRecord`, `ArtifactIndex`).

## Happy Path Scenario

Given a stored single-PR session `refactor-auth` (legacy `github_pr_number=11`, `github_pr_status_terminal=true`) linked to a `pr_pending` backlog item, when the service boots, the agent merges PR #11, opens follow-up PR #12 on the same branch and the operator opens the session detail view, then `PullRequestList` shows #11 `merged` and #12 `open` (active, "+1" on the card), the poller fetches only #12, Merge with no explicit target hits #12, the legacy `github_pr_number` column reads 12, the item stays `pr_pending` until #12 merges and then goes `done`, and the worktree is removed only after both PRs are terminal. *(Anchor for every test below; error paths, data-loss paths and UX paths are variations on this flow. Headline tests: `TestPoller_MergedAThenOpenB_PollsAndActsOnB` and `TestPRActions_DefaultTargetsActivePR`, plan T1.3.4.)*

## Naming and convention corrections (applied to plan.md in the Phase 4 patch pass; listed here so a reviewer can check both files agree)

| Plan text | Problem | Use instead |
|---|---|---|
| `session/pr_cleanup_gate_test.go` (T5.4.1, formerly) | Test filename contains `gate` (BUG-087 trips) | `session/pr_cleanup_blocker_test.go`; test funcs `TestCleanupBlocker_*`. Renamed in plan.md T5.4.1 |
| `server/services/backlog_service_cleanup_gate_test.go` (T5.4.1, formerly) | Same | `server/services/backlog_service_cleanup_blocker_test.go`. Renamed in plan.md T5.4.1 |
| Plan phrase "cleanup gate" in test names | Name-based matching of "gate" in filenames only; keep function names free of it too for grep safety | `Blocker` / `Hold` |
| Walls clocks in poller / lifecycle tests | Repo rule: deterministic fast tests | Every time-dependent test below uses an injected clock (existing seams in `pr_status_poller_test.go`); none uses `time.Sleep` or `waitForTimeout` |

Mobile assertions run in `*-mobile.spec.ts` files that the `mobile-chromium` (gating, Pixel 7, 390x844, touch) and `mobile-webkit` (informational, `PW_WEBKIT=1`) projects pick up through `MOBILE_SPECS` (plan T6.3.3) and that desktop `chromium` ignores; a "390px" assertion in a desktop-project spec is only a resized window and is not used. All e2e specs start with `// @feature ...`, use `data-testid`/ARIA locators only, helpers in `tests/e2e/pages/`, and use `baseUrl()`/`slotTmp()` from `tests/e2e/helpers/base-url.ts`. Jest tests mock `@/lib/nav` with `jest.requireActual("@/lib/nav/impl.memory")` spread.

## Requirement Index

Every item of `requirements.md` that carries an obligation gets an ID so coverage is countable.

| ID | Requirement (requirements.md section) |
|---|---|
| SM1 | Success Metric 1: a session with N PRs shows all N, each with its own state, in detail view and session-list data |
| SM2 | Success Metric 2: PR actions target an explicit PR; default is the active PR, never stale; test with A merged, B open |
| SM3 | Success Metric 3: backlog item with 2+ PRs reaches done by the rule and is not stuck in `pr_pending` on an old PR |
| SM4 | Success Metric 4: existing single-PR sessions load unchanged via migration, no data loss |
| SM5 | Success Metric 5: re-importing a finished item's GitHub issue says so and lets the user close the issue with one click, never silently (owner request 2026-11-03) |
| IS1 | In scope: audit of every read and write of the single-PR fields |
| IS2 | In scope: session-to-PR relation with per-PR state, role, timestamps, plus migration |
| IS3 | In scope: define active PR; make actions and polling PR-aware |
| IS4 | In scope: backlog multi-PR handling (completion, `pr_pending`, session-item linking) |
| IS5 | In scope: proto/RPC changes, web display (card and detail), mobile layout |
| IS6a | In scope Artifacts (1): tab reliability (store empty scans, re-enqueue on link, subagent transcripts, newest-wins caps) |
| IS6b | In scope Artifacts (2): richer structured capture (PRs, branches, commits, issues, plan docs, CI links, commands paired to results) |
| IS6c | In scope Artifacts (3): cross-session artifact search returning sessions (BM25 plus exact PR lookup) |
| IS6d | In scope Artifacts (4): browsable artifacts view across sessions |
| IS7 | In scope: tests (unit, integration, e2e), registry updates, docs |
| IS8 | In scope: issue close on re-import (offer on `ImportGitHubIssue`, explicit close RPC reusing forward-sync, github.com with token, behind `reader:backlog`) |
| IS9 | In scope: mobile e2e coverage under the real `mobile-chromium` / `mobile-webkit` projects (T6.3.3) |
| IS10 | In scope: create-PR early-return relaxation (T4.2.4) and terminal-scan detection tightening (T2.2.1) |
| D1 | Decision: active PR = pin, else open on current branch, else newest non-terminal, else newest; every action accepts explicit PR ref |
| D2 | Decision: done when >=1 merged and none open (closed-unmerged allowed); cleanup and retention sweeper require ALL PRs terminal |
| D3 | Decision: scanner `PRURLs` stays separate; `SessionPR` holds agent-claimed and API-created PRs only; scanner does not seed |
| D4 | Decision: Artifacts scope is all four (A1 to A4); mapped by IS6a to IS6d |
| AW1 | Accepted: backfill-seeded unverified-terminal PRs count non-terminal for gate, sweeper and done verdict until verified (5 counted failures over >=30 min) |
| AW2 | Accepted: item-direct PRs are never superseded; released on terminal or detach |
| AW3 | Accepted: force-reset and backward-to-ready stay ungated by open PRs, with a visible warning |
| AW4 | Accepted: reopened PR noticed only via `pull_request.reopened` webhook; merged/closed rows stay terminal |
| AM | Success metrics for Artifacts (E7-E10): zero scanned-but-empty sessions stuck in pending (against the T7.1.1 baseline), search returns the owning session for a PR, branch, SHA and plan-doc string, browse lists every scanned session with the unscanned count |
| AU | Adoption signal: pin uses, artifact search queries (kind, hit or miss) and issue-close offers (offered, closed, declined) are counted |
| N1 | NFR: list RPC latency does not regress; PR list bounded, no N+1 |
| N2 | NFR: polling cost grows with open PRs only (merged/closed not re-polled) |
| O1 | Observability: log lines on attach, active-PR change, migration backfill |
| O2 | Observability: histogram of PRs per session; poll count by PR state |
| RC | Risk control: live-settable rollout flag; off keeps legacy behavior; legacy fields dual-written; rollback needs no restore |
| C1 | Constraint: ent/SQLite schema change, generated ent output not committed |
| C2 | Constraint: Instance fields read via `Snapshot()` |
| C3 | Constraint: new RPC needs proto + proto-gen + registry; new page needs `route-meta.ts` row |
| C4 | Constraint: GitHub requests via `NewConditionalRequest`; ETag behavior, no extra rate-limit per PR |
| C5 | Constraint: automated lifecycle code must not revive archived sessions |
| C6 | Constraint: rollout flags live-settable, never env vars |
| C7 | Constraint: mobile and desktop UX |
| C8 | Constraint: legacy single-PR fields stay readable (older clients, stored JSON) |
| OQ1 | Open question: rollout flag name |
| OQ2 | Open question: role chips at launch |
| OQ3 | Open question: next free `Session` proto field number |
| OQ4 | Open question: many-to-many session-to-PR vs one owning session |
| OQ5 | Open question: Jules and up-next-tabs write through the relation |

Count after Triad iteration 1: 46 requirement IDs (AM and AU added to the 44 below). Previous: 44 requirement IDs (5 success metrics, 13 scope items with the four Artifacts items counted separately, 4 decisions, 4 accepted-as-written, 2 NFR, 2 observability, 1 risk control, 8 constraints, 5 open questions); the 40 of the previous pass plus SM5, IS8, IS9, IS10.

## Requirement → Test Mapping

Test files are the plan's files (with the naming corrections above). Go tests are in `session/` unless a path is given. `U-n` = unit, `I-n` = integration (real in-memory/temp SQLite via `NewEntRepository`, real `Storage`, fake GitHub client, injected clock), tests marked **DL** are the data-loss negatives (fail closed).

### E1: Domain, schema, flag, backfill (SM4, IS2, C1, OQ1, OQ4)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| IS2, OQ4 | `pr_set_test.go` | U-01 `TestNewPRRef_ShouldCanonicalizeHostOwnerRepo_WhenMixedCase` | Unit | Happy: `""`/`GitHub.com` host equal `github.com`; `Acme/App` equals `acme/app` |
| IS2 | `pr_set_test.go` | U-02 `TestPRRef_ShouldBeZeroAndRejected_WhenBuiltWithoutConstructor` | Unit | Error: `PRRef{}` `IsZero()`; every repository entry returns `ErrZeroPRRef` |
| IS2 | `pr_set_test.go` | U-03 `TestPRState_ShouldFailParse_WhenUnrecognizedString` | Unit | Error path; `unknown` is neither open nor terminal |
| D1, SM2 | `pr_set_test.go` | U-04 `TestActivePR_ShouldPickPinThenBranchThenNewestNonTerminal` | Unit | Happy: #11 merged, #12 open feat-b, #13 open feat-c, branch feat-c gives #13; pinned #11 gives #11; branch main gives #13 |
| D1 | `pr_set_test.go` | U-05 `TestActivePR_ShouldRankUnknownBelowOpen_WhenBothNonTerminal` | Unit | `#12 open` older beats `#14 unknown` newer; only merged + unknown gives unknown |
| D1, AW1 | `pr_set_test.go` | U-06 `TestActivePR_ShouldUseEffectiveTerminal_WhenSeededUnknownTerminalRow` | Unit | Exhausted seeded row does not outrank newer verified merged; not-exhausted seeded row counts non-terminal |
| D1 | `pr_set_test.go` | U-07 `TestActivePR_ShouldReturnNil_WhenNoPRs` | Unit | Error/edge: empty slice, nil |
| D1 | `pr_set_test.go` | U-08 `TestActivePR_ShouldReturnNewestOpen_WhenStackedAndBranchMatchesNeither` | Unit | Stacked case |
| D2 | `pr_set_test.go` | U-09 `TestItemPRVerdict_ShouldBeDone_WhenMergedAndClosedUnmerged` | Unit | #11 merged + #12 closed gives `Done`; only closed gives `AllClosedUnmerged`; merged + open gives `Open` |
| D2, AW1 | `pr_set_test.go` | U-10 `TestItemPRVerdict_ShouldHold_WhenUnknownOrUnverifiedTerminal` | Unit | Error path: merged + unknown(non-terminal) = `Open`; merged + unknown terminal (exhausted or not) = `Open`; never `Done` |
| D2 | `pr_set_test.go` | U-11 `TestItemPRVerdict_ShouldCountOnce_WhenSameStateViaSessionAndItemLink` | Unit | Dedup by state id, "1 of 1 merged" |
| D2, SM3 | `pr_set_test.go` | U-12 `TestItemPRVerdict_ShouldBeNoPRs_WhenEmptySet` | Unit | Empty set is `NoPRs`, not `Done` |
| D2 | `pr_set_test.go` | U-13 `TestPRsAllTerminal_ShouldBeFalse_WhenEmptyRowsAndLegacyNumberSet` | Unit | Empty + legacy 21 is false; empty + 0 true only via explicit `NoPRs` path |
| IS3 | `pr_set_test.go` | U-14 `TestAssignRole_ShouldFollowTable_WhenAttachSourceAndOrderVary` | Unit | primary / stacked / followup table (no `rework` or `external`; `unknown` only as a parse fallback); stacked only when base equals another head; a backfill import into a session that already has a primary is `followup` |
| IS2 | `ent_repository_session_pr_test.go` | I-01 `TestUpsertSessionPR_ShouldBeIdempotent_WhenCalledTwice` | Integration | One state row, one link, `attached_at` unchanged |
| IS2, OQ4 | `ent_repository_session_pr_test.go` | I-02 `TestListBySessionUUIDs_ShouldReturnTwoLinksOneState_WhenTwoSessionsAttachSamePR` | Integration | Many-to-many; update state once changes both views; query count `<= 2*ceil(n/500)` |
| IS2 | `ent_repository_session_pr_test.go` | I-03 `TestUpsertSessionPR_ShouldKeepOneStateRow_WhenSessionItemAndJulesLinkSamePR` | Integration | 1 state, 3 links |
| N1 | `ent_repository_session_pr_test.go` | I-04 `TestListBySessionUUIDs_ShouldBoundQueries_WhenTwoHundredSessionsAndOver500UUIDs` | Integration | Query-count hook: 200 sessions <= 2; 1,001 uuids <= 6 |
| IS2, C1 | `ent_repository_session_pr_test.go` | I-05 `TestUpsertSessionPR_ShouldRejectUppercaseColumns_WhenRawEntCreate` | Integration | Error path: ent validator rejects `Acme` |
| IS2 | `pr_ref_construction_test.go` | U-15 `TestRepositoryEntryPoints_ShouldResolveOneState_WhenCaseVariesAcrossAcmeApp` | Unit | `Acme/App`, `acme/app`, `ACME/APP#12` give one state (EnsureState, Upsert, tombstone, Delete) |
| IS2 | `ent_repository_session_pr_test.go` | I-06 `TestTombstone_ShouldBlockDiscoveryAndBackfill_WhenOwnerDetached` | Integration | No link recreated from `branch_discovery`/`backfill`; `ClearTombstone` then `UpsertSessionPR(source=manual)` clears it and creates the link (repository level; the RPC-level counterpart is I-110, wave 2) |
| IS2 | `ent_repository_session_pr_test.go` | I-07 `TestUpdatePullRequestState_ShouldBeMonotonic_WhenOlderOrOverlappingObservation` | Integration | merged stays merged; closed sticky vs tick `open`; webhook-reopen path exception |
| IS2 | `ent_repository_session_pr_test.go` | U-16 `TestSessionPRValidator_ShouldRejectBothOrNeitherOwner_WhenLinkCreated` | Unit | Exactly one of `session_uuid`/`backlog_item_id` |
| IS2 | `ent_repository_session_pr_test.go` | U-17 `TestSessionPRIndex_ShouldRejectSecondPrimary_WhenSameSession` | Unit | One-primary partial unique index |
| IS2 | `ent/schema/migration_test.go` | I-08 `migration_should_be_reversible` | Integration (Migration) | `Schema.Create` adds three tables additively with indexes; legacy columns untouched; flag off leaves tables populated and harmless; "down" = revert binary then re-deploy imports (see I-32). No column drops asserted |
| IS2 | `session_pr_gc_test.go` | I-09 `TestSessionPRLinkGC_ShouldDeleteOnlyOrphansPastGrace` | Integration | Deletes links/tombstones whose owner row is gone and older than 24h; state rows via one conditional DELETE |
| IS2 | `session_pr_gc_test.go` | U-18 `TestSessionPRLinkGC_ShouldKeepAttachRacingGC_WhenEnsureStateFindsRow` | Unit | Error/race: attach concurrent with GC does not lose state (conditional DELETE) |
| IS2, C1 | (CI) | I-10 `check_no_generated_ent_committed` (script step in `make ready`: `git ls-files session/ent` allows only `schema/` and `generate.go`) | Integration | Generated ent output never committed |
| OQ1, RC, C6 | `server/services/feature_flag_service_test.go` | U-19 `TestUpdateFeatureFlag_ShouldSetReaderScope_WhenPollerEnabled` | Unit | `config.SessionMultiPR("reader:poller")` true, `reader:ui` false |
| OQ1, RC | `server/services/feature_flag_service_test.go` | U-20 `TestSessionMultiPRScopes_ShouldResolveInOrder_WhenEnabledOutOfOrderOrPollerDisabled` | Unit | Triad iteration 2 (verified flag semantics, plan Story 1.4): an out-of-order `SET_ENABLED` is stored but `config.SessionMultiPR(scope)` resolves false until its prerequisites are effective; enabling poller, backlog, actions, ui in order resolves each true; disabling `reader:poller` makes every later scope resolve false at once and re-enabling restores them; the `refresher_off` scope is outside the order (U-129) |
| C6 | `config/session_multi_pr_flag_test.go` | U-21 `TestSessionMultiPRFlag_ShouldIgnoreEnvVars_WhenEnvSet` | Unit | Env var never flips the flag; only the RPC does |
| RC, OQ1 | `server/services/config_surface_proof_test.go` | I-11 `TestConfigSurfaceProof_SessionMultiPR_ChangesReader` | Integration | Entry `flag:session_multi_pr` present; reader behavior changes after `UpdateFeatureFlag`; fails if registry entry removed; also `artifact_scan_v2` |
| SM4 | `session_pr_backfill_test.go` | I-12 `TestBackfill_ShouldSeedUnverifiedTerminal_WhenLegacyFlagTrue` | Integration | `refactor-auth` (#11, `Acme/App`, host `""`, terminal true): one state `acme/app#11 unknown terminal=true`, link `primary/backfill`; second run creates nothing |
| SM4 | `session_pr_backfill_test.go` | I-13 `TestBackfill_ShouldSeedUnknownNonTerminal_WhenLegacyFlagFalse` | Integration | Never seeds `open`/`merged` |
| SM4, AW1 | `session_pr_backfill_test.go` | I-14 `TestBackfill_ShouldMergeFailClosed_WhenSessionAndItemShareAPR` | Integration | Terminal session + `pr_pending` item gives non-terminal in either order; + `done` item gives unverified terminal; top-up changes no state column |
| SM4 | `session_pr_backfill_test.go` | I-15 `TestBackfill_ShouldSkipAndCount_WhenNoOwnerRepo` | Integration | Error path: no link, `skipped=1`, empty host alone is not a skip |
| SM4 | `session_pr_backfill_test.go` | I-16 `TestBackfill_ShouldNotCollide_WhenTwoEmptyUUIDSessionsShareAPR` | Integration | `skipped_no_uuid` or assigned uuid; no unique collision |
| SM4 | `session_pr_backfill_test.go` | I-17 `TestBackfill_ShouldNotResurrectOrDuplicate_WhenTombstonedOrRepresentedOrRerun` | Integration | Tombstoned not recreated; represented left alone; no second primary; idempotent without marker |
| SM4 | `session_pr_backfill_test.go` | I-18 `TestBackfill_ShouldCreateItemDirectLink_WhenItemPRNumberUnrepresented` | Integration | `done` item gives unverified terminal; `pr_pending`/`archived` give `unknown` non-terminal |
| SM4 | `session_pr_backfill_test.go` | I-19 `TestBackfill_ShouldRollBackCandidateAndRetry_WhenTransientWriteFailure` | Integration | Error path: a busy/commit error rolls back that candidate (nothing persists) and is retried in-process; the full retry-then-continue-booting case is I-101; once the fault clears the import lands before `LoadInstances` |
| SM4 | `session_pr_backfill_test.go` | I-20 `TestBackfill_ShouldCountSkipNotAbort_WhenPoisonedRow` | Integration | Deterministic failure (validator, 50-link cap, bad item URL) counted in `skipped_*`, pass continues, no boot loop; an unrecognized error type is retried like a transient one and boot continues |
| SM4, N1 | `session_pr_backfill_test.go` | I-21 `TestBackfill_ShouldBoundReadsAndWrites_WhenFiveThousandItems` | Integration | 5,000 represented items: no write, reads <= `c*ceil(5000/500)`; 2,500 unrepresented: 2,000 imported, `deferred=500` |
| SM4 | `session_pr_backfill_test.go` | I-22 `TestBackfill_ShouldNotRecreateSessionDerivedPR_WhenItemPRNumberMirrorsCurrentRound` | Integration | No item-direct link for #30 reachable via current-round session link; detach does not re-import |
| SM4 | `session_pr_backfill_test.go` | I-23 `TestNonServiceProcess_ShouldNotRunMaintenance_WhenSecondNewEntRepository` | Integration | `ssq-hooks`-style `NewEntRepository` under SQLite lock: no counter change, no import, no error; the service path runs it exactly once (the `buildMCPDeps` case is I-100) |
| SM4, AW1 | `session_pr_backfill_test.go` | I-24 `TestBackfill_ShouldNotTrustStaleTerminalFlag_WhenScanReplacedPRNumber` | Integration | Stale `true` from merged A rides onto open B: row `unknown terminal=true`, blocks until exhausted; one poll of B open makes it `open terminal=false` |
| SM4, O1 | `session_pr_backfill_test.go` | U-22 `TestBackfill_ShouldLogCountsAndFirstTenTitles_WhenSkipsExist` | Unit | `[Migration] session_pr backfill: created=N imported_unrepresented=R skipped=M skipped_no_uuid=K` |
| SM4, RC | `session_pr_backfill_test.go` | I-100 `TestBuildMCPDeps_ShouldNotRunPRMaintenance_WhenStdioFallbackBuildsCoreDeps` | Integration | B1: the plain `BuildCoreDeps()` of `buildMCPDeps` on a DB with `poll_fail_count=3` rows: counters and links untouched, no candidate query (query-count hook), no error under a held SQLite lock; the service `core-deps` phase and `BuildDependencies` (`BuildOptions.RunPRMaintenance` true) run it exactly once, after `Storage` and before poller, sweeper and cleanup ticker (recording fake) |
| SM4, RC | `session_pr_backfill_test.go` | I-101 `TestRunPRStartupMaintenance_ShouldRetryThenContinueBooting_WhenBusyFourTimes` | Integration | P1 #1: 4 injected busy errors and an injected sleeper: 3 retries, `ssq_pr_maintenance_failed_total` +1, returns nil; the gate and sweeper still fail closed for the unimported number; a full save leaves the session's legacy columns untouched; the 5-minute ticker imports it once the fault clears and stops; a DB that cannot be opened still aborts |
| SM4, AW1, **DL** | `session_pr_backfill_test.go` | I-102 `TestVerifyExhausted_ShouldBeFalse_UntilMaintenanceSucceededInThisProcess` | Integration | **DL-cleanup-7**: unverified-terminal rows left with `poll_fail_count=5` and a 40-minute span by an earlier process read as NOT exhausted (gate and sweeper keep holding) until the first maintenance pass succeeds |
| SM4, N1 | `session_pr_rehearsal_test.go` | I-103 `TestRehearsal_ShouldRecordCountsAndBudgets_WhenProductionDBCopyProvided` | Integration (opt-in evidence, `SSQ_REHEARSAL_DB`) | T1.5.4: maintenance duration inside the start-limit window, backfill counts, synthesized URLs for empty-URL rows, `ListSessions` p99 within +5 ms, pool wait recorded; a recording cleaner and deleter prove nothing was removed; the maintenance retry ticker is started (failing fake first pass) and stopped by context cancel with `goleak.VerifyNone` (Triad iteration 2); what it cannot show is Known Risk K13 (manual first-deploy rehearsal); skipped when the variable is unset; a full boot on the copy is forbidden |
| SM4, RC | `pr_state_writer_guard_test.go` | U-128 `TestOnScanComplete_ShouldNeverCreateSessionPR_WhenFlagOff` | Unit | B2: every scope OFF, blob `PRURLs` `.../pull/99` on a session with no PR: no `SessionPR`, no legacy number change (twin of U-34) |
| RC, C6 | `server/services/feature_flag_service_test.go` | U-129 `TestRefresherOffScope_ShouldRunByDefaultAndStopWhenSet_IndependentOfGlobalAndReaderScopes` | Unit | P1 #3 (Triad iteration 2: scope renamed `refresher_off`, read as an explicit override only): `config.SessionMultiPRRefresher()` true by default and when the global flag is turned on; `SET_ENABLED` on `refresher_off` returns false with no prerequisite; `CLEAR_SCOPE` restores true; global on or off and the four reader scopes do not change it |
| IS3 | `ent_repository_session_pr_test.go` | U-130 `TestUpdatePullRequestState_ShouldWriteOnlyPollErrorEnum_WhenNonCountedFailure` | Unit | C8: an `error_note` observation writes `poll_error` only (streak, `first_fail_at`, `last_polled_at` untouched); any success clears it |
| IS3 | `pr_set_test.go` | U-131 `TestIsLiveWork_ShouldBeLiveOnlyInsideWindow_WhenUnendedAndActive` | Unit | P1 #4: injected-clock table: active inside the window live, idle past it not, ended not, zero time not; nil `LiveWorkProbe` means no hold |

### E2: Instance writes, snapshot, mirror, persistence (SM1, SM4, IS1, C2, C8)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| IS3, C2 | `instance_pr_attach_test.go` | U-23 `TestAttachPR_ShouldKeepBoth_WhenTwoGoroutinesAttach` | Unit | `-race`: #12 and #13 both in `Snapshot().GitHub.PRs`, attach order |
| C2 | `instance_pr_attach_test.go` | U-24 `TestAttachPR_ShouldNotMutatePublishedSnapshot_WhenAdding` | Unit | Earlier snapshot still length 1 (copy on write) |
| IS3, C8, RC | `instance_pr_attach_test.go` | U-25 `TestAttachPR_ShouldProjectActivePRToLegacyFields_WhenAnyScopeState` | Unit | Links #11 merged, #12 open: legacy number 12, log `session active pr changed from=11 to=12`; flipping scopes republishes nothing |
| IS2 | `instance_pr_attach_test.go` | U-26 `TestAttachPR_ShouldDeferPersist_WhenSessionRowNotYetCreated` | Unit | `prPersistPending`, first full save writes state+link+mirror; failed save retried; empty uuid case |
| C8 | `instance_pr_attach_test.go` | U-27 `TestAttachPR_ShouldShowNewestNonTerminal_WhenNoBranchMatch` | Unit | Legacy overwrite semantics preserved; #11 link untouched |
| C2 | `instance_snapshot_test.go` | U-28 `TestBuildSnapshot_ShouldCopyPRsDeeply_WhenPublished` | Unit | Deep copy; `LegacyPRUnrepresented`/`HasPRLinks` copied, never re-derived |
| IS1 | `legacy_pr_readers_test.go` | U-29 `TestLegacyPRReaders_ShouldMatchAllowList_WhenGrepping27GoFiles` | Unit | Ratchet over `GitHubPRNumber|GitHubPRURL`; each entry tagged snapshot-backed or `InstanceData`-backed; new reader fails |
| IS1 | `legacy_item_pr_readers_test.go` | U-30 `TestLegacyItemPRReaders_ShouldMatchAllowList_WhenGreppingPrNumberPrURL` | Unit | Backlog-side ratchet with dispositions |
| IS1, C2 | Makefile `pr-field-guard` fixture | U-31 `pr_field_guard_should_fail_on_new_direct_write_and_pass_on_repo` | Unit | Red-then-green: new `inst.GitHubPRURL =` fails; `inst.RetryMaxAttempts =` / `inst.Status =` in `session_driver.go` do not trip |
| IS1 | `session_driver_test.go` | U-32 `TestScanAndLinkPRURL_ShouldAttachAndClearTerminal_WhenNumberChanges` | Unit | `https://github.com/acme/app/pull/12` with #11 held: both rows, #12 `push_scan`; terminal flag cleared (also flag off) and persisted via `UpdateGitHubPRStatusTerminal(title,false)` |
| IS1 | `session_driver_test.go` | U-33 `TestScanAndLinkPRURL_ShouldNotAttach_WhenRepoMismatchOrBareMention` | Unit | Error path: `other/lib` URL and non-push mention create no link, debug line names mismatch |
| D3 | `server/dependencies_test.go` | U-34 `TestOnScanComplete_ShouldNotSeedSessionPR_WhenFlagOn` | Unit | Blob `PRURLs` with `acme/app/pull/99`: no link, no legacy number change (the seed block is deleted outright; the flag-off twin is U-128) |
| IS1 | `server/services/session_service_create_test.go`, `pr_creation_service_test.go` | I-25 `TestCreatePaths_ShouldAttachViaAttachPR_WhenCreateFromURLOrResolutionOrOneShot` | Integration | Writers 2, 6, 7, 8 of the checklist each call `AttachPR` with the right `AttachSource`; no overwrite flag on |
| SM4, C8 | `ent_repository_session_pr_test.go` | I-26 `TestPersistPRMirror_ShouldClearLegacyNumber_WhenLastLinkDetached` | Integration | `github_pr_number` becomes 0 (old `>0` guard cannot) |
| C8 | `ent_repository_session_pr_test.go` | I-27 `TestLegacyReader_ShouldSeeActivePR_WhenOldSchemaLoadsFlagOnData` | Integration | Old-schema reader using only `InstanceData` JSON tags sees active PR in `github_pr_number`/`github_pr_url` |
| N1 | `storage_test.go` | I-28 `TestLoadInstances_ShouldBulkHydrate_WhenTwoHundredSessions` | Integration | Link+state queries <= `2*ceil(n/500)`; hydrated before first snapshot publish |
| IS2 | `ent_repository_session_pr_test.go` | I-29 `TestAttachSessionPR_ShouldBeAllOrNothing_WhenFailureBeforeMirrorUpdate` | Integration | Injected failure: no link, no state change, no mirror change |
| SM2, SM4 | `ent_repository_session_pr_test.go` | I-30 `TestFullSave_ShouldClearTerminalFlag_WhenOpenPRAttachedToTerminalSession` | Integration | Create and update paths (~412/~758) write `false`; `ListInstanceDataWithWorktree()` returns false |
| SM4 | `storage_test.go` | I-31 `TestLoadSaveLoad_ShouldKeepUnrepresentedLegacyNumber_WhenSkippedClass` | Integration | `LegacyPRUnrepresented` survives load, full save, load; columns unchanged |
| SM4, RC | `session_pr_backfill_test.go` + `session_pr_mirror_reconciler_test.go` | I-32 `TestRollbackRedeploy_ShouldImportBeforeProjection_WhenOldBinaryWroteLegacyOnly` | Integration | Real sequence `NewEntRepository` -> `RunPRStartupMaintenance` -> `LoadInstances` -> full save: #12 imported as `followup` (primary exists), not overwritten; tombstoned legacy value overwritten by projection |
| IS2 | `pr_state_writer_guard_test.go` | I-33 `TestFullSave_ShouldNotOverwritePolledState_WhenAnyFullSavePath` | Integration | `unknown terminal=true` then `UpdatePullRequestState` writes `open`; create, update, `prPersistPending`, archived and suspended stale snapshots: still `open` (invariant I1) |
| IS2 | `pr_state_writer_guard_test.go` | U-35 `TestStateWriterAllowList_ShouldFail_WhenNewCallerOfUpdatePullRequestState` | Unit | Caller allow-list; `ResetVerifyCounters` restricted by WHERE to `unknown AND terminal` |
| SM4 | `session_pr_mirror_reconciler_test.go` | I-34 `TestMirrorReconciler_ShouldCorrectViaActor_WhenLegacyNumberDriftsFromActivePR` | Integration | Legacy 11 vs `ActivePR` 12: counter +1, fixed through actor setter, no scope needed |
| SM4 | `session_pr_mirror_reconciler_test.go` | U-36 `TestMirrorReconciler_ShouldSkipAndCount_WhenLegacyPRUnrepresented` | Unit | Error path: untombstoned unrepresented value left alone |
| SM4, RC | `ent_repository_session_pr_test.go` | I-105 `TestHookProcess_ShouldNotChangeLegacyColumns_WhenLinklessSessionSavedViaSsqHooksPath` | Integration | Pre-mortem #13 (the claim was INFERRED): drives `cmd/ssq-hooks`' real `NewEntRepository` + save path against a session with a legacy number and no link: `github_pr_number`, `github_pr_url` and the terminal flag unchanged |

### E3: Poller and tracking core (SM2, IS3, N2, C4, O1, O2)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM2, N2 | `pr_multi_poller_test.go` | I-35 `TestPoller_MergedAThenOpenB_PollsAndActsOnB` (headline, T1.3.4) | Integration | Session with #11 merged terminal and #12 open: `GetPRInfoConditional` once for 12, never 11; update lands on #12 state |
| SM2 | `pr_multi_actions_test.go` | I-36 `TestPRActions_DefaultTargetsActivePR` (headline, T1.3.4) | Integration | Merge/close/comment with no `pr` hit #12 on the fake GitHub client |
| N2, O2 | `pr_multi_poller_test.go` | U-37 `TestPoller_ShouldNotRepollMerged_WhenThreeTicks` | Unit | #11 0 calls, #12 3 calls (or 304s), `ssq_pr_poll_total{state="open"}` +3 |
| IS3 | `pr_multi_poller_test.go` | I-37 `TestPoller_ShouldResumeAndClearTerminal_WhenDiscoveryFindsPRBOnBranch` | Integration | Only #11 merged; discovery attaches #12, terminal flag false, next tick polls #12 |
| IS3, N2 | `pr_multi_poller_test.go` | U-38 `TestPoller_ShouldFetchOnceAndRefreshBothSessions_WhenTwoSessionsShareOpenPR` | Unit | One GitHub call, one state update |
| IS3 | `pr_multi_poller_test.go` | U-39 `TestPoller_ShouldPollItemOnlyState_WhenNoSessionLink` | Unit | Jules/backfill item-direct #40 fetched and updated |
| IS3 | `pr_multi_poller_test.go` | U-40 `TestPoller_ShouldKeyBackoffByUUIDOrTitle_WhenLegacyEmptyUUID` | Unit | No collision between sessions |
| N2 | `pr_multi_poller_test.go` | U-41 `TestPoller_ShouldBoundColdStart_WhenOneHundredTwentyUnknownStates` | Unit | At most 20 per tick, non-terminal before unverified terminal, newest `attached_at` first |
| IS3 | `pr_multi_poller_test.go` | U-42 `TestPoller_ShouldMarkUnknownOnce_WhenFiveCountedFailuresOverThirtyMinutes` | Unit | Error path: `unknown`, `poll_error` set, one notification; non-counted failures (5xx, timeouts, rate limit, auth, inaccessible) never count |
| IS3 | `pr_multi_poller_test.go` | U-43 `TestClassifyPollError_ShouldClassifyAllReasons` | Unit | Table: counted, rate_limit, auth, repo_inaccessible, transient; K5 never exhausts |
| IS3 | `pr_multi_poller_test.go` | U-44 `TestPoller_ShouldDoubleBackoffToSixHourCap_WhenStateFailsEveryPoll` | Unit | Injected clock; success resets `poll_fail_count` |
| IS3 | `pr_multi_poller_test.go` | U-45 `TestPoller_ShouldRefreshSupersededRoundPRs_WhenOwnerSessionOldRound` | Unit | Both earlier-round and current-round states fetched |
| RC | `pr_multi_legacy_test.go` | I-38 `TestRefresher_ShouldResolveRows_WhenAllScopesOff` | Integration | Merged non-active PR, item-only PR, suspended session's PR all become terminal with no scope on; session snapshots and `GitHubPRStatusTerminal` unchanged |
| RC, C4 | `pr_multi_legacy_test.go` | U-46 `TestFlagOffDoubleFetch_ShouldBeAtMostTwoRequestsPerPRPerInterval` | Unit | Legacy loop plus refresher <= 2 conditional GETs, second is 304; no extra 200 |
| C4 | `pr_multi_poller_test.go` | U-47 `TestPoller_ShouldUseHostAwareToken_WhenItemOnlyStateOnGHE` | Unit | `newGHRequestForHostWithToken` for `ghe.acme.io`; no token = counted failure, no fallback to github.com |
| C4 | `pr_multi_poller_test.go` | U-48 `TestDiscovery_ShouldMakeZeroListCalls_WhenOpenPROnCurrentBranch` | Unit | Rate limit: at most one list call per tick when no open PR |
| C4 | `tools/lint` via `make lint-custom` | I-39 `norawghrequest_should_pass_on_new_github_calls` | Integration | New poll/attach code builds requests only via approved constructors |
| IS3 | `pr_multi_legacy_test.go` | U-49 `TestFlagOffTwoLinks_ShouldNotOverwriteLegacyMerged_WhenStalePRViewOpen` | Unit | Characterization: legacy "merged" observation survives `buildSnapshot` |
| IS3 | `pr_multi_legacy_test.go` | U-50 `TestLegacyPoller_ShouldSkipTerminalSession_WhenFlagOff` (T1.1.1) | Unit | Zero GitHub calls for `GitHubPRNumber=11` + terminal; also `SetGitHubPR` stale terminal, link-once, create short-circuit pinned |
| IS3 | `pr_status_poller_test.go` | U-51 `TestPRStatusPoller_ShouldPassUnchanged_WhenRefactorOnly` | Unit | Existing poller tests unmodified after T3.1.1/T3.1.2 |
| IS3 | `pr_multi_poller_test.go` | U-52 `TestSlowDiscovery_ShouldBackOffTenXNoPRBackoff_WhenAllTerminalAndLiveBranch` | Unit | Skipped for suspended/archived; log line on resume |
| IS3, O1 | `pr_multi_poller_test.go` | I-40 `TestDiscovery_ShouldAttachFollowupWithoutOverwriting_WhenNewOpenPROnBranch` | Integration | #11 kept, #12 `branch_discovery`/`followup`; tombstoned not recreated; cap 50; other repo ignored |
| IS3 | `pr_multi_poller_test.go` | U-53 `TestInvalidateAndRefresh_ShouldRefreshRow12_WhenWebhookForPR12` | Unit | Matches rows not the single field; 10s timeout; error path returns after dropped/failed |
| O1, O2 | `pr_multi_poller_test.go` | U-54 `TestAttach_ShouldObserveHistogramAndLog_WhenThirdPRAttached` | Unit | `ssq_session_pr_count` observes 3; logs `session pr attached` (session, PRRef, role, source) |
| O1, O2 | `pr_multi_poller_test.go` | U-55 `TestMetrics_ShouldExposeAllPlannedSeries_WhenRegistered` | Unit | `ssq_pr_poll_total{state}` incl. unknown, `ssq_pr_poll_failures_total{reason}`, `ssq_session_pr_mirror_drift_total`, `ssq_pr_discovery_list_calls_total`, blocked-items gauge, artifact metrics |
| AW4 | `pr_multi_poller_test.go` | U-56 `TestPoller_ShouldKeepClosedTerminal_WhenNoReopenWebhook` | Unit | Closed row sticky and unpolled; only `pull_request.reopened` path (I-41) flips it |
| AW4 | `server/webhook_test.go` | I-41 `TestWebhookReopened_ShouldReopenTerminalRow_WhenPullRequestReopenedEvent` | Integration | Documented accepted-risk path works; no other path does |
| RC | `pr_multi_legacy_test.go` | I-104 `TestRefresher_ShouldMakeNoCallsAndWriteNothing_WhenRefresherOffScopeSet` | Integration | P1 #3: `refresher_off` set, a tick makes zero GitHub calls and rows are unchanged; back on, the next tick polls; B4 moved clause: the refresher is constructed after `RunPRStartupMaintenance` (recording fake over the service bootstrap) |
| IS1 | `server/prwatch_sessions_test.go` | U-57 `TestPrwatch_ShouldEmitOneEntryPerOpenPR_WhenReaderPollerOn` | Unit | A merged PR cannot hide an open one; audit result recorded for `worktree_pr_poller.go` |

### E4: API surface and server readers (SM1, SM2, IS5, C3, DL)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| IS5, C8, OQ3 | `make proto-breaking` + `server/adapters/instance_adapter_test.go` | I-42 `TestProtoAdditive_ShouldPassBreakingCheck_WhenPullRequests100Added` | Integration | `Session.pull_requests = 100`, scalars 25/26/33-35 still populated; field 100 confirmed free |
| SM1, IS5 | `server/adapters/instance_adapter_test.go` | U-58 `TestInstanceAdapter_ShouldMapTwoPRsWithActiveFlag_WhenMergedAndOpen` | Unit | `pull_requests` has 2, `is_active` on #12, `github_pr_number=12`, `poll_error` enum mapped (RATE_LIMITED keeps `rate_limit_reset_at`), `outcome_unverified` for `unknown`+terminal |
| SM1 | `server/adapters/instance_adapter_test.go` | U-59 `TestInstanceAdapter_ShouldMapItemPullRequests_WhenItemHasTwoPRs` | Unit | Backlog item `pull_requests` from `PRSetReader.itemPRs` |
| SM2 | `server/services/session_service_pr_refs_test.go` | I-43 `TestMergePR_ShouldTargetActivePR_WhenNoPRSelector` | Integration | #11 merged, #12 open: GitHub receives merge for #12 |
| SM2 | `server/services/session_service_pr_refs_test.go` | U-60 `TestResolveTargetPR_ShouldFailPrecondition_WhenPRNotAttached` | Unit | Error: `MergePR{acme/app#99}` returns FailedPrecondition, GitHub not called |
| SM2 | `server/services/session_service_pr_refs_test.go` | U-61 `TestResolveTargetPR_ShouldFailPrecondition_WhenUnsetSelectorAndNoPRs` | Unit | Error: "no pull request", never PR number 0 |
| SM2 | `server/services/session_service_pr_refs_test.go` | U-62 `TestResolveTargetPR_ShouldFallBackToLegacyPR_WhenReaderActionsOff` | Unit | Flag off keeps single legacy PR |
| SM2 | `server/services/github_service_test.go` | U-63 `TestCommentsDiffAskToFix_ShouldUseResolvedTarget_WhenSelectorGiven` | Unit | Same helper; `GeneratePRContextPrompt` names its PR; `pr_annotations.go` takes `PRRef` |
| IS3 | `server/services/pr_creation_service_test.go` | I-44 `TestCreatePullRequest_ShouldCreateAndAttach_WhenPriorPRMerged` | Integration | #11 merged, branch feat-b no PR: creates and attaches; open PR on feat-b returned as `ExistingPrUrl` |
| IS3 | `server/review_queue_manager_test.go` | U-64 `TestAutoCreate_ShouldShortCircuitOnlyOnOpenPROnCurrentBranch` | Unit | `hasOpenPROnCurrentBranch` |
| IS3, D1 | `server/services/session_service_pr_refs_test.go` | I-45 `TestPinSessionPullRequest_ShouldMakePinnedActive_UntilUnpinned` | Integration | #11 pinned returns #11; unset returns derived; automation never pins |
| IS3 | `server/services/session_service_pr_refs_test.go` | I-46 `TestDetachSessionPullRequest_ShouldDeleteOneLinkAndTombstoneInOneTxn` | Integration | Only `u1`'s link removed; item-direct link untouched; discovery does not recreate; audit log |
| IS3 | `server/services/session_service_pr_refs_test.go` | U-65 `TestAttachSessionPullRequest_ShouldVerifyViaConditionalRequestAndClearTombstone_WhenURLGiven` | Unit | `source=manual`; nonexistent PR rejected |
| IS3 | `server/services/session_service_pr_refs_test.go` | I-110 `TestAttachAndDetachRPC_ShouldClearAndWriteTombstone_WhenUserAttachesOrDetaches` | Integration | B3: the RPC-level counterparts of the wave-0 repository cases (I-06, I-22): user `AttachSessionPullRequest` clears the tombstone and creates the link; `DetachSessionPullRequest(session, #30)` writes a session tombstone and a later backfill does not recreate #30 |
| IS3 | `server/services/session_service_pr_refs_test.go` | U-133 `TestRefreshSessionPullRequests_ShouldRefreshOpenAndUnverifiedRowsOnly_WhenCalled` | Unit | N7 (S2 header Refresh): awaits `InvalidateAndRefresh` for open, `unknown` and unverified-terminal rows (see I-122), bounded 10 s; verified terminal rows are not refetched; classified not-terminal-write |
| SM2 | `server/services/github_service_test.go` | U-134 `TestMergePR_ShouldPassSelectedMethod_WhenMethodSet` | Unit | N7 (S3 method select): `MERGE|SQUASH|REBASE` reaches the GitHub call, unset means merge (squash and rebase only when explicitly requested) |
| IS4, AW2 | `server/services/session_service_pr_refs_test.go` | I-47 `TestDetachItemPullRequest_ShouldReleaseItemAndWriteItemTombstone` | Integration | Verdict and cleanup see the release; session link remains |
| IS5 | `server/services/session_service_pr_refs_test.go` | U-66 `TestListSessions_ShouldFilterByPR_WhenPRSetAndNotZero` | Unit | `u1` (#12) returned for filter #12; unset means no filter |
| C3 | `server/services/terminal_write_rpcs_test.go` | U-67 `TestSinkguard_ShouldClassifyNewRPCs_WhenPRPinDetachAttachRescanSearch` | Unit | `Pin/Detach/Attach/RefreshSessionPullRequest(s)`, `DetachItemPullRequest`, `CloseFinishedItemIssue` classified (not terminal where applicable); `RescanSessionArtifacts`, `SearchSessionArtifacts`, `FindSessionsByPullRequest`, `ListSessionArtifacts` in `notTerminalWriteRPCs` |
| C3, IS7 | `docs/registry` check (`make registry-diff`) | I-48 `registry_should_have_hand_set_tested_testIds_for_every_new_RPC_and_component` | Integration | `make registry-generate` clean diff; `tested`/`testIds` set |
| SM1, C2 | `server/services/session_retention_sweeper_test.go` | I-49 `TestRetentionSweeper_ShouldNotDelete_WhenOneLinkedPROpen` | Integration | Archived `InstanceData`, no live instance, #11 merged + #12 open: not eligible; both terminal: eligible |
| D2, **DL** | `server/services/session_retention_sweeper_test.go` | I-50 `TestRetentionSweeper_ShouldFailClosed_WhenLegacyNumberAndNoLinkRows` | Integration | **DL-sweeper-1**: `GitHubPRNumber=9`, flag false, no rows: NOT eligible; flag true: eligible only via `LegacyOnlyPRSafe`, never via the empty row set |
| D2, AW1, **DL** | `server/services/session_retention_sweeper_test.go` | I-51 `TestRetentionSweeper_ShouldFailClosed_WhenUnknownOrUnverifiedTerminal` | Integration | **DL-sweeper-2**: `unknown terminal=false` blocks; seeded `unknown terminal=true` blocks until `VerifyExhausted()`, then releases |
| D2, AW2, **DL** | `server/services/session_retention_sweeper_test.go` | I-52 `TestRetentionSweeper_ShouldFailClosed_WhenItemDirectPROpen` | Integration | **DL-sweeper-3**: session's own links terminal, item has open item-direct #22: NOT eligible; #22 merged: eligible |
| D2, **DL** | `server/services/session_retention_sweeper_test.go` | I-53 `TestRetentionSweeper_ShouldFailClosed_WhenLinkOrItemLookupErrors` | Integration | **DL-sweeper-4**: injected error from `ListSessionPRViewsByUUIDs` or `ListItemIDsBySessionUUIDs` or item-link query: NOT eligible, `DeleteSession` never called, error logged |
| D2, **DL** | `server/services/session_retention_sweeper_test.go` | I-54 `TestRetentionSweeper_ShouldFailClosed_WhenRepresentationMismatch` | Integration | **DL-sweeper-5**: `GitHubPRNumber>0` not represented by links (rollback/skipped): NOT safe (step 3 exception); item with skip-class ref and no link NOT safe |
| RC, D2, **DL** | `server/services/session_retention_sweeper_test.go` | I-55 `TestRetentionSweeper_ShouldFailClosed_WhenAllScopesOff` | Integration | **DL-sweeper-6**: fail-closed behavior is not flag-gated |
| D2 | `server/services/session_retention_sweeper_test.go` | U-68 `TestRetentionSweeper_ShouldBeEligible_WhenNoLinksAndNoLegacyNumber` | Unit | Happy path for decision step 1 |
| D2, N1 | `server/services/session_retention_sweeper_test.go` | U-69 `TestRetentionSweeper_ShouldIssueBoundedQueries_WhenManySessions` | Unit | One `map[uuid][]PRView` per cycle; no per-session query |
| IS3 | `server/services/approval_service_test.go` | U-70 `TestApprovalGate_ShouldReadActivePRConclusion_WhenMergedAndOpenWithFailure` | Unit | CI-failure approval reads #12's conclusion; `debug_snapshot` dumps all rows (string includes `host/owner/repo#n`) |
| C5 | `tools/lint/noarchivedrevival` via `make lint-custom` | I-56 `TestNoArchivedRevival_ShouldPassAndNotRevive_WhenNewPollerOrReconcilerCode` | Integration | New poller/reconciler/gate code never calls `Start(false)`/`RecoverFromStopped`; archived session with open PR is not revived by attach, backfill or the last-PR-terminal hook |

### E5: Backlog multi-PR handling (SM3, IS4, D2, AW1-AW3, DL)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM3 | `backlog_lifecycle_pr_characterization_test.go` | I-57 `TestReconcilePRPending_ShouldMoveToDone_WhenSingleMergedPRFlagOff` | Integration | Legacy `pr_number=21` merged goes `done` exactly as today (T5.1.1 pins `reconcileBouncingItems`, `reconcilePRPendingWithoutPRItems`, `FindPRPendingItems`) |
| IS4 | `backlog_item_prs_test.go` | U-71 `TestItemPRs_ShouldSynthesizeUnknownPR_WhenLegacyNumberUnrepresented` | Unit | One snapshot #21 `unknown` non-terminal; never read as all-terminal |
| IS4 | `backlog_item_prs_test.go` | U-72 `TestItemPRs_ShouldReturnOnePR_WhenSameStateViaItemAndSessionLink` | Unit | Dedup by state id |
| IS4 | `backlog_item_prs_test.go` | U-73 `TestItemPRs_ShouldDropPR_WhenOnlySessionLinkDetachedOrSessionDeleted` | Unit | Derived, never stored |
| IS4 | `backlog_item_prs_test.go` | U-74 `TestItemPRs_ShouldUseLatestWorkRoundOnly_WhenReopened` | Unit | Round 1 #30, round 2 #31: set contains #31; #30 stays linked, polled |
| IS4 | `backlog_item_prs_test.go` | U-75 `TestItemPRs_ShouldNotSupersede_WhenLaterReviewSessionOrEndedAt` | Unit | Only a later WORK session supersedes; `ended_at` and nullable `started_at` never read; tie-break by id |
| IS4 | `backlog_item_prs_test.go` | U-76 `TestItemPRs_ShouldNotResurrectEarlierRound_WhenLatestRoundLinkDetached` | Unit | Error path: session tombstone, no link: earlier round's merged #30 does not re-enter |
| IS4 | `backlog_item_prs_test.go` | U-77 `TestItemPRs_ShouldKeepLivePR_WhenNewerWorkSessionHasNoLinkYet` | Unit | `NoPRs`/spurious `PRPendingNoPR` window closed; leaves once new session links |
| IS4, AW2 | `backlog_item_prs_test.go` | U-78 `TestItemPRs_ShouldKeepItemDirectPR_WhenRound2Merges` | Unit | Item-direct #30 open + round-2 #31 merged: item stays `pr_pending`, stuck report names #30 |
| IS4 | `backlog_item_prs_test.go` | U-79 `TestItemPRs_ShouldKeepSharedPR_WhenOneOfTwoRoundsDetaches` | Unit | Session tombstone does not hide a PR another live contributor holds |
| IS4 | `backlog_item_prs_test.go` | U-80 `TestItemPRs_ShouldIgnoreUnrelatedSession_WhenManualAttachToOtherSession` | Unit | Item unchanged |
| IS4 | `backlog_item_prs_test.go` | U-81 `TestItemPRs_ShouldStillReturnItemDirect_WhenSessionLinkBroken` | Unit | Broken session link does not hide item-direct #21 |
| IS4 | `backlog_item_prs_test.go` + `server/services/backlog_service_triage_test.go` | I-58 `TestRoundMembership_ShouldHoldThenSwitch_WhenEachReopenPathCreatesWorkSession` | Integration | Reopen/rework respawn (`triage.go:1261`), stale respawn, bounce, `AttachSessionToItem` (`sync.go:111`): earlier PR stays until new session links, no "abandon" call |
| SM3, D2 | `backlog_lifecycle_pr_test.go` | I-59 `TestReconcilePRPending_ShouldStayPending_WhenOldMergedAndNewOpen` | Integration | `reader:backlog` on: #21 merged + #22 open stays `pr_pending`, #22 checked |
| SM3, D2 | `backlog_lifecycle_pr_test.go` | I-60 `TestReconcilePRPending_ShouldGoDone_WhenMergedPlusClosedUnmerged` | Integration | Done; only-closed does not go done and falls into `closeIfSupersededByMain` |
| SM3 | `backlog_lifecycle_pr_test.go` | U-82 `TestStuckDetector_ShouldRaisePRPendingNoPR_WhenEmptySetAndNumberZero` | Unit | Unchanged semantics, empty set |
| SM3 | `backlog_lifecycle_pr_test.go` | I-61 `TestItemMirror_ShouldFollowSet_WhenSetChanges` | Integration | `item.pr_number` = 22 for #21 merged/#22 open; `FindPRPendingItems` still returns the item; atomic CAS (no `pr_pending` with `pr_number=0`); empty set never zeroes |
| SM3 | `backlog_lifecycle_pr_test.go` | I-62 `TestReconcilePRPending_ShouldAwaitRefreshAndNotClose_WhenClosedRowMergedOnGitHub` | Integration | Row `closed` but merged on GitHub: refresh makes it `merged`, item `done` that pass, no `closeIfSupersededByMain`; refresh timeout closes/archives nothing |
| SM3 | `server/mcp/tools_stuck_report_test.go` | U-83 `TestStuckReport_ShouldNameBlockingPRAndBothDetaches_WhenUnknownHoldsItem` | Unit | "k of N merged, waiting on #23" and owner actions |
| SM3, AW2 | `server/mcp/tools_backlog_pr_test.go` | I-63 `TestReportPRCreated_ShouldAttachFollowup_WhenNumberDiffersFlagOn` | Integration | #22 `followup`, item stays `pr_pending`, no `override_reason`, head-branch verification applies |
| SM3 | `server/mcp/tools_backlog_pr_test.go` | I-64 `TestReportPRCreated_ShouldReassignOrRefuse_WhenReplacesPRNumber` | Integration | Correction path unchanged; #21 merged refused (`ErrPRReassignmentNotAllowed`) |
| SM3 | `server/mcp/tools_backlog_pr_test.go` | U-84 `TestReportPRCreated_ShouldReject_WhenCallerSessionNotLinkedToItem` | Unit | Error path: caller identity from MCP credential, not request field (T5.3.0, written first) |
| SM3, OQ5 | `jules_session_poller_test.go` | I-65 `TestJulesApplyCompleted_ShouldCreateTwoItemDirectLinks_WhenTwoOutputs` | Integration | #31 and #32 `attach_source=jules`; `jules multiple pr outputs` warning gone; tombstoned ref skipped on re-poll |
| SM3, RC | `server/mcp/tools_backlog_pr_test.go` | I-66 `TestReportPRCreated_ShouldWriteItemDirectLink_WhenAllScopesOff` | Integration | Link + state for #22 exist, refresher polls it, cleanup and sweeper step 4 hold; legacy reassign rules unchanged |
| IS4 | `server/services/backlog_github_progress_sync_test.go` | U-85 `TestProgressSync_ShouldLinkAllPRs_WhenTwoPRs` | Unit | Comment lists both, marks merged/open/unknown |
| SM3, AW1, **DL** | `pr_cleanup_blocker_test.go` | I-67 `TestCleanupBlocker_ShouldBlockEachPath_WhenOneOpenPR` | Integration | **DL-cleanup-1** (one subtest per path 1 to 6): archive RPC, `TransitionBacklogItemStatus`, `cleanupTerminalItemSync` from PR-merge-done, one `reconcileTerminalItemWorktrees` tick, reopen with prior round open PR, tombstone-orphan sweep: worktree retained, reason names holder |
| AW2, **DL** | `pr_cleanup_blocker_test.go` | I-68 `TestCleanupBlocker_ShouldBlock_WhenOpenPRIsItemDirectOnly` | Integration | **DL-cleanup-2**: archive RPC retains worktree, reason names "item" |
| AW1, **DL** | `pr_cleanup_blocker_test.go` | I-69 `TestCleanupBlocker_ShouldNotSettle_WhenBlocked` | Integration | **DL-cleanup-3**: `Blocked` returned, `terminalSettled.settle` not called, re-evaluated on second tick; after #22 merges next tick returns `Done` and settles |
| AW1, **DL** | `pr_cleanup_blocker_test.go` | I-70 `TestCleanupBlocker_ShouldFailClosed_WhenBadRefUnknownLegacyOrLookupError` | Integration | **DL-cleanup-4**: `pr_number=5` with empty `pr_url` and no link; only-`unknown` PR; legacy number + no link + terminal flag not true; link/state lookup error: worktree retained, stuck report lists it |
| AW1, **DL** | `pr_cleanup_blocker_test.go` | I-71 `TestCleanupBlocker_ShouldTreatUnverifiedTerminalAsTerminal_OnlyWhenVerifyExhausted` | Integration | **DL-cleanup-5**: seeded unverified terminal holds the worktree until 5 counted failures over >=30 min; service restart resets counters |
| AW2, **DL** | `pr_cleanup_blocker_test.go` | I-72 `TestCleanupBlocker_ShouldHoldSupersededRoundWorktree_UntilDetachOrTerminal` | Integration | **DL-cleanup-6**: round 1 `s1` open #30, round 2 merged #31, item `done`: verdict `Done` but cleanup `Blocked` naming `s1`; detach or terminal releases next attempt |
| D2 | `pr_cleanup_blocker_test.go` | I-73 `TestCleanupRerun_ShouldRemoveWorktreeWithoutWaiting_WhenLastPRTerminal` | Integration | `PRTerminalListener` invokes `CleanupTerminalItem` at once; with hook unavailable the next 60s tick removes it |
| D2 | `pr_cleanup_blocker_test.go` | I-74 `TestCleanup_ShouldNotReopenItemOrDelete_WhenPRAttachedAfterDone` | Integration | Item stays `done`, nothing deleted while #60 open; cleaned after it merges |
| AW3, **DL** | `pr_cleanup_blocker_test.go` + `server/mcp/tools_lifecycle_test.go` | I-75 `TestForceReset_ShouldWarnAndNameOpenPRs_WhenUngatedRemoval` | Integration | **DL-warning**: force-reset and backward-to-ready remove a clean checkout but response text and one structured log line name open/`unknown` PRs; MCP `pause`/`destroy` result lists them; rows 10 to 14 stay ungated by design |
| D2, **DL** | `pr_cleanup_blocker_test.go` | U-86 `TestCleanupBlocker_ShouldAllow_WhenAllPRsTerminalAcrossSessionsAndItem` | Unit | Happy path allowed |
| D2, AW1, **DL** | `backlog_lifecycle_pr_test.go` | I-106 `TestReconcilePRPending_ShouldHoldPending_WhenMergedAndWorkSessionLive` | Integration | **DL-cleanup-8** (P1 #4): merged #21 and a work session active 2 minutes ago stay `pr_pending` (`merged, session still active`), no cleanup or archive, pane and worktree intact; idle past the window or ended: the next pass moves to `done`; a probe error holds; a second PR attached by the session makes the verdict `Open`; the first-step characterization pins today's path |
| D2, **DL** | `pr_cleanup_blocker_test.go` | I-107 `TestCleanupBlocker_ShouldBlock_WhenWorkSessionLiveOrProbeErrors` | Integration | **DL-cleanup-9** (P1 #4 gate half): all PRs terminal but the current-round work session live: `Blocked` names it on paths 1-6; released once idle or ended; probe error blocks |
| D2 | `pr_cleanup_blocker_test.go` | I-108 `TestCleanupListener_ShouldStartAtMostTwentyCleanupsPerTick_WhenThreeHundredItemsResolve` | Integration | P2 #6: FIFO cap `maxListenerCleanupsPerTick`; the rest left to the 60 s tick; blocked gauge carries `reason="unverified_drain"` for not-yet-verified seeds |
| SM5, IS8 | `backlog_item_prs_test.go` | U-136 `TestItemFinished_ShouldBeTrueOnlyForDoneWithNoPRsOrDoneVerdict` | Unit | Table: `done`+no PRs true; `done`+#21 merged/#22 closed true; `done`+#21 merged/#22 open false; `done`+only #22 closed false; `done`+merged+unverified-terminal false; `pr_pending` all merged false; `archived` false; `duplicate` false |
| SM5, IS8 | `server/services/backlog_github_rpc_test.go` | I-111 `TestImportGitHubIssue_ShouldOfferClose_WhenExistingItemFinishedAndIssueOpen` | Integration | `already_existed=true` and `issue_close_offer{issue_url, issue_number, pr_urls}` set for a finished item with an open github.com issue, token present, `reader:backlog` on; ZERO GitHub write calls (offer, never a close) |
| SM5, IS8 | `server/services/backlog_github_rpc_test.go` | I-112 `TestImportGitHubIssue_ShouldNotOffer_WhenNotFinishedOrIssueClosedOrFlagOff` | Integration | Response equals today's for unfinished item, closed issue, flag off; GHE host sets `blocked_reason=unsupported_host`, missing token `no_token`; the MCP `import_github_issue` is unchanged (no `ImportGitHubIssue` call) |
| SM5, IS8 | `server/services/backlog_github_forward_sync_test.go` | I-113 `TestCloseFinishedItemIssue_ShouldCloseAndComment_WhenFinishedAndOpen` | Integration | `CLOSED`; the comment equals `forwardSyncCloseCommentWithPR(host, prURL)`; watermark persisted; audit log line |
| SM5, IS8, **DL** | `server/services/backlog_github_forward_sync_test.go` | I-114 `TestCloseFinishedItemIssue_ShouldRefuse_WhenNotFinishedOrFlagOffOrClientClaimsFinished` | Integration | **DL-issue-close-1** (never silent, never forced): `NOT_FINISHED` for an item that stopped being finished; `FailedPrecondition` with the scope off; the request carries only `item_id`, so a client cannot assert "finished"; zero GitHub writes in every refusal |
| SM5, IS8 | `server/services/backlog_github_forward_sync_test.go` | I-115 `TestCloseFinishedItemIssue_ShouldReportOutcome_WhenAlreadyClosedGHEOrNoTokenOrGitHubError` | Integration | `ALREADY_CLOSED` (no write), `UNSUPPORTED_HOST`, `NO_TOKEN`, `FAILED` (watermark untouched) |
| SM5, IS8 | `server/services/backlog_github_forward_sync_test.go` | I-116 `TestForwardSyncClose_ShouldBehaveUnchanged_WhenCloseCoreExtracted` | Integration | The existing forward-sync tests pass unmodified after `closeSourcelessIssue` is extracted; `make ready-complexity-gate` (dupl) clean |

### DeleteSession data-loss site

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| D2, IS2, **DL** | `session_pr_gc_test.go` | I-76 `TestDeleteSession_ShouldRollBackEverything_WhenLinkOrTombstoneDeleteFails` | Integration | **DL-delete-1**: injected failure deleting links/tombstones: session row, worktree data and links all remain (all-or-nothing); error returned |
| D2, **DL** | `session_pr_gc_test.go` | I-77 `TestDeleteSession_ShouldKeepItemDirectLinksAndSharedState_WhenSessionDeleted` | Integration | **DL-delete-2**: only this session's links and its session-keyed tombstones removed; item-direct links, item tombstones and a `PullRequestState` another owner still links survive (state never deleted outside the GC's conditional DELETE) |
| D2, **DL** | `server/services/session_retention_sweeper_test.go` | I-78 `TestSweeper_ShouldNotCallDeleteSession_WhenAnyHoldingConditionOrAnyError` | Integration | **DL-delete-3**: spy on `DeleteSession`; across the I-49..I-55 matrix zero calls on every holding or error case; one call only on the allowed case |
| AW3, **DL** | `server/services/session_service_lifecycle_test.go` | I-79 `TestDeleteSessionRPC_ShouldWarnWithOpenPRs_WhenExplicitUserDelete` | Integration | **DL-delete-4**: explicit RPC (inventory rows 10 to 12) succeeds by design but the response and UI confirm text list open/`unknown` PRs; no silent open-PR delete |
| D2, **DL** | `session_pr_gc_test.go` | I-80 `TestSessionPRLinkGC_ShouldNeverDeleteLinks_WhenOwnerRowExistsOrYoungerThanGrace` | Integration | **DL-delete-5**: owner present or <24h: nothing deleted; GC with a DB error deletes nothing |

### E6: Web UI (SM1, IS5, OQ2, C7) , Jest/RTL unless noted

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM1, C7 | `web-app/src/lib/pr/pullRequests.test.ts` | U-87 `pullRequests helpers should sort active first then open by recency then terminal and compute earlierCount` | Unit | Mirrors server `is_active`, does not recompute the rule |
| SM1, C7 | `web-app/src/lib/pr/pullRequests.test.ts` | U-88 `pullRequests helpers should tolerate empty list and missing optional fields` | Unit | Error/edge path |
| IS5, C7 | `web-app/src/components/shared/PullRequestChip.test.tsx` | U-89 `PullRequestChip should render icon and text for merged state when compact` | Unit | WCAG 1.4.1; `title` fallback |
| IS5, C7 | `web-app/src/lib/unfinished/chipContrast.test.ts` | U-90 `CHIP_TOKEN_PAIRS should meet 4.5:1 text and 3:1 glyph in light and dark for new states` | Unit | open, draft, merged, closed, checking, stale |
| IS5 | `web-app/src/components/shared/PullRequestList.test.tsx` | U-91 `PullRequestList should show open and two latest merged then earlier-PRs button when five PRs` | Unit | `aria-expanded="false"` button "2 earlier PRs" |
| IS5 | `web-app/src/components/shared/PullRequestList.test.tsx` | U-92 `PullRequestList should set aria-current and unique link names when active PR present` | Unit | Names contain `#12`, `acme/app`, "opens in new tab" |
| OQ2 | `web-app/src/components/shared/PullRequestList.test.tsx` | U-93 `PullRequestList should show role chip only for followup stacked when row has role` | Unit | No chip for primary/unknown; never on card |
| IS5 | `web-app/src/components/shared/PullRequestList.test.tsx` | U-94 `PullRequestList should show per-row error chip and keep other rows actionable when one row fails` | Unit | Error path |
| SM2 | `web-app/src/components/shared/PullRequestList.test.tsx` | U-95 `PullRequestList row actions should pass the row PullRequestRef when merge close ask-to-fix pin or not-mine clicked` | Unit | Explicit PR reference always sent |
| SM1 | `web-app/src/components/sessions/SessionCard.test.tsx` | U-96 `SessionCard should show one badge and +2 button when three PRs` | Unit | Click navigates to `#pull-requests` via `replaceHash` |
| SM1 | `web-app/src/components/sessions/SessionDetailView.test.tsx` | U-97 `SessionDetailView should list three rows with state text and default-target hint when three PRs` | Unit | |
| SM4, C8 | `web-app/src/components/sessions/SessionDetailView.test.tsx` | U-98 `SessionDetailView should show one row no chip no disclosure when migrated single-PR session` | Unit | Legacy session unchanged |
| RC | `web-app/src/components/sessions/SessionCard.test.tsx` | U-99 `SessionCard should show legacy single badge when reader:ui off with three pull_requests` | Unit | Flag-off fallback |
| IS5 | `web-app/src/components/shared/VcsWidget.test.tsx` | U-100 `VcsWidget should default switcher to active PR and use native select at mobile width when two open PRs` | Unit | |
| SM3 | `web-app/src/components/backlog/detail/PullRequestSection.test.tsx` | U-101 `PullRequestSection should show "2 of 3 PRs merged" and rows when pr_pending` | Unit | Renders for every status incl. `done`/`archived`; each `unknown` or open item-direct row offers detach |
| IS5 | `web-app/src/components/unfinished/prs/PRCard.test.tsx` | U-102 `PRCard should show session chip for one match and "2 sessions" control for two` | Unit | No match: no chip, no gap |
| C8 | `web-app/src/lib/vcs/adapters.test.ts` | U-103 `vcs adapters should map github list with active index and keep legacy single when flag off` | Unit | `bun run typecheck` repo-wide as AC |
| IS1 | ESLint `bun run lint:eslint` | I-81 `eslint_should_forbid_githubPrNumber_githubPrUrl_outside_allowlist` | Integration | TS ratchet counterpart of U-29 (8 non-test files allow-listed) |
| IS1, C3 | `web-app/src/app/route-meta.test.ts` | I-82 `route_meta_should_have_row_for_artifacts_page_with_matching_title` | Integration | New page row; no feature guard |
| IS9, C7 | `tests/e2e/baseline/mobile-specs-registered.test.ts` | U-135 `mobile_specs_should_register_the_five_new_specs_and_allow_list_the_legacy_ones` | Unit (bun) | T6.3.3 (scoped after checking `playwright.config.ts:55`): each of the five new specs is in `MOBILE_SPECS` and in the `testIgnore` of `chromium` and `chromium-dom`; any other `*-mobile.spec.ts` must be in `MOBILE_SPECS` or the test's `LEGACY_MOBILE_NAMED` allow-list (`backlog-result-panels-mobile.spec.ts`, `toast-stack-mobile.spec.ts`; `files-mobile.spec.ts` matches the `files-mobile*` glob); `files-large.spec.ts` stays on desktop chromium; a new unregistered mobile spec fails |
| SM5, IS8, C7 | `web-app/src/components/backlog/CloseIssuePrompt.test.tsx` | U-137 `CloseIssuePrompt should call close once on confirm and nothing on Esc or Not now and show a distinct message per outcome` | Unit | Focus trapped, focus on Not now first, Retry on `FAILED`, no button for `UNSUPPORTED_HOST`/`NO_TOKEN`, focus returns to the trigger |
| SM5, IS8 | `web-app/src/app/backlog/__tests__/BacklogPage.githubImportClose.test.tsx` | U-138 `BacklogPage import should list close offers with one Close button each and never close in bulk` | Unit | Single-URL form and bulk picker: offers shown, the existing duplicates message unchanged, no RPC until a Close click |
| IS7 | `bun run lint:duplicates` (jscpd) | I-83 `jscpd_should_stay_under_0_14_percent_with_shared_chip_and_list` | Integration | One `PullRequestChip`/`PullRequestList` implementation across surfaces |

### E7 to E10: Artifacts (IS6a-IS6d, D3, D4)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| IS6a | `session/artifacts/scan_test.go` | U-104 `TestScan_ShouldPersistNoneFound_WhenTranscriptHasNoMatches` | Unit | `inst.Artifacts != nil`, `ScanState=none_found`; the tab no longer says "Extraction pending" |
| IS6a | `session/history_linker_test.go` | I-84 `TestHistoryLinker_ShouldEnqueueFileAndScan_WhenSetHistoryInfoAfterFileExisted` | Integration | Late link triggers scan without restart |
| IS6a | `session/artifacts/store_test.go` | I-85 `TestScan_ShouldRecordSubagentPRCreate_WhenAgentJsonlMapsToParent` | Integration | `agent-abc.jsonl` result recorded on `refactor-auth`; read-only scan needs no archival guard |
| IS6a | `session/artifacts/store_test.go` | U-105 `TestMerge_ShouldKeepLastFifty_WhenSixtyURLs` | Unit | Newest-wins; same for `maxCommands` |
| IS6a | `server/services/session_service_artifacts_test.go` | I-86 `TestRescanSessionArtifacts_ShouldResetPositionAndScan_WhenAnyState` | Integration | Per-session reset, global scan version not bumped; classified not terminal write |
| IS6a | `server/services/session_service_artifacts_test.go` | U-106 `TestRescanSessionArtifacts_ShouldFail_WhenSessionMissing` | Unit | Error path |
| IS6a | `server/adapters/instance_adapter_test.go` | U-107 `TestAdapter_ShouldMapScanStateAndTranscriptSupported_WhenNonClaudeProgram` | Unit | `not_linked` reason for programs that never get a transcript; bundle (`session_bundle_service.go:412`) handles nil |
| IS6a | `.../` read-only diagnosis | I-87 `diagnosis_should_record_counts_before_T7_2_scope_final` | Integration (manual, evidence) | T7.1.1: count of sessions with empty `session_artifacts` but non-empty `history_file_path`, `[ArtifactExtractor]` warning counts, pasted in PR description |
| IS6b | `session/artifacts/jsonl_test.go` | U-108 `TestExtractor_ShouldEmitAuthoredPRRecord_WhenToolUsePairedWithResult` | Unit | `gh pr create --base feat-a --head feat-b` + result `.../pull/12`: `{acme/app#12, created, feat-b, feat-a, firstSeenAt}` |
| IS6b | `session/artifacts/jsonl_test.go` | U-109 `TestExtractor_ShouldMarkMentioned_WhenGhPrViewOnly` | Unit | Error/negative: `gh pr view 99` is `mentioned` |
| IS6b | `session/artifacts/extractor_test.go` | U-110 `TestExtractor_ShouldKeepHost_WhenGHEPullURL` | Unit | `ghe.acme.io` host |
| IS6b | `session/artifacts/extractor_test.go` | U-111 `TestExtractor_ShouldEmitCommitBranchIssuePlanCI_WhenSamples` | Unit | `[feat-b 1a2b3c4] fix: token refresh`, `git checkout -b`, `Fixes #45`, Write to `project_plans/x/plan.md`, `/actions/runs/123` |
| IS6b | `session/artifacts/redact_test.go` | U-112 `TestRedact_ShouldReplaceSecretValues_WhenTokenInURLOrFlag` | Unit | `[redacted]` in blob and bundle export |
| IS6b | `session/artifacts/store_test.go` | I-88 `TestBlobV1_ShouldLoadAndKeepPRURLs_WhenStoredBeforeV2` | Integration | v1 renders as before |
| IS6b | `session/artifacts/store_test.go` | I-89 `TestBlobV2_ShouldBeReadableByV1Decoder_WhenWrittenByThisVersion` | Integration | Pre-change struct fixture decodes v2, v1 lists populated (older-binary rollback) |
| IS6b | `session/artifacts/records_test.go` | U-113 `TestArtifactRecord_ShouldDropStrayField_AndSkipUnknownKind` | Unit | `{"kind":"commit","host":"x"}` drops `host`; `{"kind":"unknown"}` skipped with counted warning; `CommitRecord` has no `Host` (compile-time) |
| IS6b, RC | `session/artifacts/store_test.go` | U-114 `TestScanV2Rescan_ShouldHonorKillSwitch_WhenArtifactScanV2OffOnOffMidRun` | Unit | Off: no enqueue on version bump; on: paced queue starts; off mid-run: no further files |
| IS6b | `session/artifacts/scan_test.go` | U-115 `TestScan_ShouldPairCommandWithResult_WhenCommandRecordCaptured` | Unit | `CommandRecord` linked to result |
| IS6b | `session/artifacts/store_test.go` | I-109 `TestScan_ShouldCapBlobAt128KB_WhenLargestRealTranscript` | Integration | P2 #9: blob <= 128 KB, per-kind caps with the oldest dropped, command results <= 512 B, `ssq_artifact_blob_bytes` observed; fixture of the same shape when the real file is absent |
| IS6b | `web-app/src/components/sessions/ArtifactsTab.test.tsx` | U-116 `ArtifactsTab should group commit under PR created before it and plan doc under later PR and rest under Unassigned` | Unit | 10:30 commit under #11, 11:15 plan under #12 |
| IS6b | `web-app/src/components/sessions/ArtifactsTab.test.tsx` | U-117 `ArtifactsTab should fall back to flat v1 list when records missing` | Unit | Older proto |
| IS6c | `session/artifact_index_test.go` | U-118 `TestArtifactIndex_ShouldRetrievePRCommitBranch_WhenQueryHasOwnerRepoHashOrShaPrefix` | Unit | `acme/app#42`, `1a2b3c4`, `feat-b` |
| IS6c, D3 | `session/artifact_index_test.go` | I-90 `TestSessionsForPR_ShouldUnionBothStoresWithSource_WhenSessionPRAndArtifactRecord` | Integration | `u1` `source=session_pr`, `u2` `source=artifact`; index never writes `SessionPR` |
| IS6c | `session/artifact_index_test.go` | I-91 `TestArtifactIndex_ShouldIncludeNewScan_WhenOnScanCompleteFires` | Integration | No restart |
| IS6c | `session/artifact_index_test.go` | U-132 `TestArtifactIndex_ShouldBuildInBackground_WhenStartupIndexing` | Unit | P3 #14: the listener is up before the index finishes building; `ssq_artifact_index_docs` exported |
| IS6c | `session/artifact_index_test.go` | I-92 `TestSearchClaudeHistory_ShouldBeUnchanged_WhenHundredArtifactDocsIndexed` | Integration | Hits, order, scores identical (separate Engine/DocumentStore) |
| IS6c | `server/services/session_service_artifacts_test.go` | I-93 `TestSearchSessionArtifacts_ShouldReturnSessionWithMatchType_WhenShaQuery` | Integration | `u1.matches[0].type=commit`; hidden/archived visibility as `SearchClaudeHistoryRequest`; short query (<2 chars) rejected |
| IS6c | `server/services/session_service_artifacts_test.go` | U-119 `TestSearchRPCs_ShouldReportUnscannedCount_WhenSomeSessionsNotLinkedOrScanning` | Unit | `unscanned_session_count=3` |
| IS6c, **DL** | `server/services/session_service_artifacts_test.go` | U-120 `TestFindSessionsByPullRequest_ShouldFilterOrphanLinks_WhenSessionDeleted` | Unit | No stale `source=session_pr` hit |
| IS6c | `server/services/session_service_artifacts_test.go` | U-151 `TestFindSessionsByPullRequest_ShouldRankTrackedAboveSeen_ThenRecency_WithOneRowPerSession` | Unit | Triad iteration 2 (design/ux.md S9): `source=session_pr` rows before `source=artifact` rows, then `last_activity` descending with title ascending as the tie-break; a session matched by both sources appears once, as Tracked |
| SM3 | `session/pr_live_work_probe_test.go` | U-152 `TestLiveWorkProbe_ShouldMapSnapshotToLiveEndedIdle_AndHeldByLiveWorkShouldHoldOnErrorAndNilMeansToday` | Unit | T5.2.1 (Triad iteration 3): real probe over fake `Snapshot()` data: Active and recent `LastMeaningfulChangeAt` live; Paused, Stopped, Hibernated or `ArchivedAt != nil` ended; idle past the window not live; `TurnEnded` ignored; `heldByLiveWork` returns (held, false) inside the max hold, (held=false, expired=true) past it, held on probe error, and (false, false) for a nil probe |
| IS6c, N1 | `server/services/session_service_artifacts_test.go` | U-121 `TestSearchSessionArtifacts_ShouldObserveLatencyMetric_WhenCalled` | Unit | `ssq_artifact_search_seconds` observed |
| IS6c | `web-app/src/components/omnibar/detector.test.ts` | U-122 `detector should offer Find sessions for this PR when GitHub PR URL pasted` | Unit | GHE host-aware; unknown host falls back to generic URL |
| IS6c | `web-app/src/components/omnibar/dispatch.test.ts` | U-123 `dispatch should navigate to /artifacts?pr=acme/app%2312 when FindSessionsForPR chosen` | Unit | Both registries in sync (`OmnibarAction` union and `DetectorRegistry`) |
| IS6d | `server/services/session_service_artifacts_test.go` | I-94 `TestListSessionArtifacts_ShouldFilterByTypeRepoPRAndPaginate_WhenTwelveRecords` | Integration | `types[]`, `repo`, `state`, `pr`, `page_token`; bounded page; `unscanned_session_count` |
| IS6d | `web-app/src/app/artifacts/page.test.tsx` | U-124 `ArtifactsPage should list only acme/app PR records with chip and session link when type=pr&repo=acme/app` | Unit | URL filters restore on reload |
| IS6d | `web-app/src/app/artifacts/page.test.tsx` | U-125 `ArtifactsPage should show SearchSessionArtifacts result when query submitted` | Unit | Clear returns to browse |
| D3 | `server/dependencies_test.go` | U-126 `TestArtifactScanner_ShouldNeverCreateSessionPR_WhenScanOrExactLookupRuns` | Unit | Separation guard across scan, index, lookup |

### Cross-cutting (RC, C1 to C6, IS7, OQ1 to OQ5)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| C2 | `.claude/rules` ratchet via `make actor-field-guard` and `make lint-custom` | I-95 `actor_field_guard_and_lint_custom_should_pass_after_epic` | Integration | `instance-lock-free-reads`: no raw `i.Path`/PR field reads in new code |
| IS7 | `make ready` | I-96 `make_ready_should_pass_on_merged_main` | Integration | proto-breaking, complexity gate (dupl, gocognit, funlen, gocyclo on modified functions), jscpd, `TestConfigSurfaceProof`, sinkguard `UIRpcSurface`, `actor-field-guard`, `pr-field-guard`, lint, race tests |
| IS7 | `docs` review | I-97 `docs_should_exist_and_runbook_lists_flag_scope_metric_rollback_per_stage` | Integration (manual) | `docs/how-to/roll-out-session-multi-pr.md`, `docs/reference/session-pull-requests.md`, `CLAUDE.md` index row; runbook states cold-poll drain time and supported rollback cycle |
| RC | `session/pr_multi_legacy_test.go` | I-98 `TestRollbackByFlagOff_ShouldRestoreLegacyReadersWithoutDataRestore_WhenScopesCleared` | Integration | All scopes off: readers use legacy columns that were dual-written throughout; no restore step |
| OQ4 | `ent_repository_session_pr_test.go` | I-99 `TestSameStateTwoLinks_ShouldNotDivergeOrDoublePoll_WhenReworkAndOriginalSessions` | Integration | Resolves OQ4: many-to-many via one state row |
| OQ5 | `jules_session_poller_test.go` + `github_user_nudge_test.go` | U-127 `TestUpNext_ShouldOnlyReadRelation_WhenUserPRListed` | Unit | Up-next writes no link; Jules writes item-direct (I-65) |
| SM3 | `session/pr_set_test.go` | U-139 `TestLiveWorkHoldExpired_ShouldBeTrueAtMaxHold_AndFalseForZeroMergedAt` | Unit | T1.2.3: 59 minutes false, 60 and 61 true, zero `latestMergedAt` never expired |
| SM3 | `session/backlog_lifecycle_pr_test.go` | I-117 `TestReconcilePRPending_ShouldProceedToDoneWithWarning_WhenLiveSessionPastMaxHold` | Integration | Story 5.2 AC: merge 61 min ago and session active 1 min ago, injected clock: item `done`, warning and one notification, `CleanupTerminalItem` reports the worktree `Blocked` AND the live session is neither archived nor pane-killed (recording stopper; RULE L, I-130); at 59 min the item stays `pr_pending`; reason metric `max_hold` |
| D2, AW1, **DL** | `server/services/backlog_service_cleanup_blocker_test.go` | I-130 `TestCleanupTerminalItem_ShouldSkipArchiveAndPaneKill_WhenCurrentRoundWorkSessionLive` | Integration | **DL-cleanup-10** (Triad iteration 2, RULE L): a `done` item reached by manual archive, by PR merge after the max hold, or by the shipped-without-PR bounce path with a live current-round work session: `ArchiveSessionByUUID` and `KillTmuxPaneOnly` are NOT called for it, its review-role and earlier-round sessions ARE archived, the worktree is `Blocked`; idle or ended: archived and killed on the next call; probe error: skipped; `archiveItemWorkSessions` called directly by the rework respawn path is unchanged |
| D2, AW1, **DL** | `session/backlog_lifecycle_archive_test.go` | I-131 `TestReconcileTerminalItemSessions_ShouldSkipLiveSession_AndArchiveOnceIdleOrEnded` | Integration | **DL-cleanup-11**: the 60 s sweep does not archive or kill a live session of a `done` item and leaves that item UNSETTLED (see I-135); it archives the session on the first sweep after the idle window or the end; max hold reached and session live: no pane kill in either sweep; warning shown on the item (UX-S5-5) |
| D2, AW1, **DL** | `session/backlog_lifecycle_archive_test.go` | I-135 `TestReconcileTerminalItemSessions_ShouldNotSettleItem_WhenLiveSessionSkipped_AndArchiveOnNextPassNotAfterTTL` | Integration | **DL-cleanup-13** (Triad iteration 3, RULE L vs the sweep): REAL `settledTracker` and injected `sweepNow`; pass 1 with a live current-round session: archiver and pane-kill recorders not called for it and `isSettled(settleKey)` is FALSE (the skip sets `clean=false`; without it `clean` stays true and the item is settled for `terminalSweepSettleTTL` = 10 minutes); advance the clock 60 s, session now idle past `liveWorkIdleWindow` (or ended): pass 2 archives it and kills its pane, with the clock still far inside the 10-minute TTL (a skipped session is not archived 10 minutes late); pass 3 with everything archived settles the item; a skipped live session next to an archive failure still leaves the item unsettled; counts unchanged (`terminalSessionCounts`) |
| D2, AW1, **DL** | `session/backlog_lifecycle_test.go` | I-132 `TestReconcileBouncingItems_ShouldHoldDoneTransition_WhenMergedAndWorkSessionLive` | Integration | **DL-cleanup-12** (Triad iteration 2, second merge-to-done path): `in_progress`/`review` item with a merged, branch-verified PR and a live current-round session: no `transitionBouncingItemToDone`, no PASS verdict recorded, no cleanup, not flagged bouncing; idle or ended: `done` as today; merge older than `liveWorkMaxHold`: `done` with the warning and one notification; probe error: held; the commit-shipped-without-PR branch is unchanged |
| SM5, IS8 | `server/services/backlog_github_forward_sync_test.go` | I-133 `TestSourcelessClose_ShouldStayGithubComOnly_AndSourceLinkedStillClose_AfterCoreExtraction` | Integration | T5.5.2 policy split (verified: `ParseGitHubRefWithHosts(url, nil)` is github.com only): a sourceless GHE `external_url` with a token for that host resolves a nil action and makes zero calls before and after the extraction; a sourceless github.com issue still closes through the core; a source-linked item whose source lists a GHE host still closes; the `3ae9e6e78` routing test passes unchanged |
| AM, SM4 | T1.5.4 PR description + `requirements.md` Baseline | I-134 `baseline (PR side and issue close) recorded from the backup copy before the first reader:ui flip` | Integration (manual, evidence) | Triad iteration 2, product j: sessions with more than one distinct PR URL in artifacts, items `done` with a GitHub issue and how many of the 50 most recent are still open (read-only `gh issue view`); numbers written into requirements.md Baseline; run on a `sqlite3 .backup` copy, never the live database |
| SM2 | `server/mcp/tools_pr_rpc_unreachable_test.go` | U-140 `TestMCPTools_ShouldNotRouteToPRMutatingRPCs` | Unit | T4.2.3/T4.2.7: no MCP tool reaches Pin, Refresh, Detach, Attach or DetachItem PullRequest; plus a handler test with the MCP credential expecting `PermissionDenied` if T5.3.0 finds the Connect middleware accepts it |
| SM2 | `server/services/session_service_pr_refs_test.go` | I-122 `TestRefreshSessionPullRequests_ShouldRepollUnverifiedTerminalRow_AndSkipVerifiedMerged` | Integration | Story 4.2 AC: seeded unverified `#9` and open `#12` are re-polled; verified merged `#11` is not; `poll_fail_count` not reset by the user refresh |
| SM2 | `server/services/session_service_pr_refs_test.go` | I-123 `TestAttachSessionPullRequest_ShouldRelinkAndSurviveDiscovery_WhenCalledAfterDetach` | Integration | Detach then attach: link exists once, tombstone cleared, next discovery keeps it |
| N1, IS5 | `server/adapters/instance_adapter_test.go` | U-141 `TestListSessionsPayload_ShouldStayUnderCeiling_WhenTwoHundredSessionsHaveThreePRs` | Unit | K11 measured AC (T4.1.2): `proto.Size` added bytes <= 1.5 KiB per session and <= 40 KiB total; prints both; list responses carry no `pr_holds` |
| IS5 | `server/adapters/instance_adapter_test.go` | U-142 `TestSessionDetail_ShouldFillPrHolds_ForOpenUnverifiedSupersededAndSessionActive` | Unit | T4.1.2: each hold kind from the blocker and live-work reasons; MAX_HOLD after the maximum hold |
| C1, IS7 | `session/session_pr_backfill_test.go` | I-124 `TestPRMaintenanceTicker_ShouldStopOnServerStop_AndNotStartWhenDisabled` | Integration | T1.5.1 / BUG-084 class: `goleak.VerifyNone` with `IgnoreCurrent` after stop; `RunPRMaintenance=false` starts no ticker |
| C4 | `session/pr_status_poller_test.go` | I-125 `TestRepoMetadataProbe_ShouldUseConditionalRequestWithHostToken_WhenCountedCandidate404` | Integration | T3.2.4: probe hits the fake server's host-aware URL with the host token; `make lint-custom` (norawghrequest) is green |
| N2 | `session/pr_status_poller_test.go` | I-126 `TestPollerMutex_ShouldBeFreeDuringInFlightPoll` | Integration | T3.1.1 / BUG-023 interplay: a blocked fake GitHub client, `p.mu` acquirable meanwhile |
| SM3, D2 | `session/backlog_storage_pr_test.go` | I-127 `TestPushAndCreatePR_ShouldWriteFieldsAndLinkAtomically_AndKeepReviewOnFailure` | Integration | T5.3.1/T5.3.3 BUG-058 narrowed: forced failure leaves neither `pr_number` nor link, item stays `review`; `pr_pending` never has an empty link set |
| SM5, IS8 | `server/services/backlog_github_forward_sync_test.go` | I-128 `TestExecuteCloseAction_ShouldReturnTypedOutcome_AndLeaveHandleForwardSyncCloseUnchanged` | Integration | T5.5.2: Closed, AlreadyClosed, update error; the existing `handleForwardSyncCloseWith` tests including the 3ae9e6e78 routing test pass unchanged; `SOURCE_MANAGED` for a source-linked item |
| SM5, IS8 | `server/services/backlog_github_rpc_test.go` | I-129 `TestImportGitHubIssue_ShouldReportSourceManaged_AndNeverOfferClose_WhenItemHasSource` | Integration | T5.5.1: sourceless finished item gets the offer; source-linked gets `blocked_reason=source_managed` |
| AM | `session/artifacts/scan_test.go` + T7.1.1 evidence | I-121 `T7.1.1 baseline recorded in the PR; after T7.2.1 the stuck-pending query returns 0` | Integration (evidence) | Baseline numbers (a), (b), (c) of requirements.md; the same query re-run; a completed empty scan stores `none_found` and never "Extraction pending" |
| AM, IS6c | `server/services/session_service_artifacts_test.go` | I-118 `TestSearchSessionArtifacts_ShouldReturnOwningSession_ForPRBranchShaAndPlanDocStrings` | Integration | Four seeded strings return their sessions; a miss returns none and counts `result=miss`; a 1-char query is `InvalidArgument`, a 2-char query runs |
| AM, IS6d | `server/services/session_service_artifacts_test.go` | U-143 `TestListSessionArtifacts_ShouldListAllScannedSessions_AndCountUnscanned` | Unit | `unscanned_session_count` equals sessions with no blob |
| AU | `server/services/session_service_pr_refs_test.go` | I-119 `TestPinSessionPullRequest_ShouldCountPinUnpinAndUndo` | Integration | `ssq_pr_pin_total{action}` increments per action |
| AU | `server/services/session_service_artifacts_test.go` | I-120 `TestSearchSessionArtifacts_ShouldCountQueriesByKindAndResult` | Integration | `ssq_artifact_search_queries_total{kind,result}` (plus `ssq_artifact_browse_total`) |
| AU | `session/backlog_item_prs_test.go` | U-146 `TestIssueCloseOfferMetric_ShouldCountOfferedBlockedClosedFailed` | Unit | `ssq_issue_close_offer_total{outcome}` increments at T5.5.1 (offered, blocked) and T5.5.2 (closed, failed) |
| O2 | `docs/observability/grafana/dashboards/session-pr-tracking.json` | U-144 `TestDashboard_ShouldParseAndReferenceOnlyExistingMetrics` | Unit | T3.3.3: JSON parses; every `ssq_*` name in a panel or alert exists in Go source; two warning alert rules present |
| SM5 | `web-app/src/components/backlog/CloseIssuePrompt.test.tsx` | U-145 `CloseIssuePrompt should show per-row busy and result and announce each result once in bulk` | Unit | UX-S11-5: row-level `aria-busy`, Open on GitHub after close, Dismiss all disabled while busy, `SOURCE_MANAGED` copy with no button |
| IS5 | `web-app/src/components/shared/PullRequestList.test.tsx` | U-147 `PullRequestList should render held lines and distinct unverified copy and refresh a seeded unverified row` | Unit | UX-S2-7, UX-S2-8: held line per `pr_holds` kind with release verb; "Checking... (attempt 2 of 5)" vs "Couldn't verify after 5 attempts"; Refresh calls `RefreshSessionPullRequests` |
| IS5 | `web-app/src/components/shared/PullRequestList.test.tsx` | U-148 `PullRequestList should write undo toast on detach and re-attach on Undo and offer Track a PR by URL` | Unit | UX-S3-8: dialog copy says it won't be added back; Undo calls `AttachSessionPullRequest`; URL dialog inline errors |
| IS5 | `web-app/src/components/backlog/detail/PullRequestSection.test.tsx` | U-149 `PullRequestSection should show waiting-for-session-idle line with server idle window and the max-hold warning` | Unit | UX-S5-5 |
| IS6c | `web-app/src/app/artifacts/__tests__/ArtifactsSearch.test.tsx` | U-150 `search box should run at 2 chars, reject 1 char inline, and show the SHA hint for 2 to 6 hex chars` | Unit | UX-S9-4; same rule in the omnibar action |

## UX Acceptance Tests

One row per criterion in `design/ux.md` (69 total). Playwright specs, desktop (project `chromium`, 1280px): `tests/e2e/multi-pr-session.spec.ts` (`// @feature session:detail, session:pull-requests`), `tests/e2e/artifacts-tab.spec.ts` (`// @feature session:artifacts`, new), `tests/e2e/artifacts-view.spec.ts` (`// @feature session:artifacts`), `tests/e2e/omnibar-find-pr.spec.ts` (`// @feature omnibar:detect`, new), `tests/e2e/backlog-import-close-issue.spec.ts` (`// @feature backlog:import-github`, new). Mobile siblings (projects `mobile-chromium` gating and `mobile-webkit` informational, Pixel 7 / iPhone 14 with touch and `isMobile`, registered by plan T6.3.3 and ignored by desktop `chromium`): `multi-pr-session-mobile.spec.ts`, `artifacts-tab-mobile.spec.ts`, `artifacts-view-mobile.spec.ts`, `omnibar-find-pr-mobile.spec.ts`, `backlog-import-close-issue-mobile.spec.ts`, each with its own `// @feature` header. Conventions for all ten: no `waitForTimeout`, `data-testid`/ARIA locators only, page helpers in `tests/e2e/pages/` (`SessionDetailPage.ts`, `ArtifactsPage.ts`, `OmnibarPage.ts` extended, `BacklogItemPage.ts` extended, `BacklogImportPage.ts` new). The 390px criteria are asserted ONLY in the mobile specs (a resized desktop window has no touch, no mobile UA and no `isMobile`); the 1280px criteria stay in the desktop specs. Axe via the existing UX-analysis helper (`axeAaViolations` in `tests/e2e/helpers/a11y.ts`; real runs listed in A-8 and A-3). "Jest" = RTL in `web-app/src`, "Manual" = scripted check recorded in PR.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UX-S1-1 3 PRs show one badge and one +2 | `multi-pr-session.spec.ts` | `card should show one pr-chip and +2 when session has three PRs` | Playwright | Seed 3 PRs; list view; count `pr-chip` = 1, `pr-more-button` text "+2" on card and row |
| UX-S1-2 +2 reaches detail list, focus on heading | `multi-pr-session.spec.ts` | `plus-two should open detail and focus Pull requests heading when clicked` | Playwright | Click `pr-more-button`; URL hash `#pull-requests`; heading `toBeFocused`; Back returns to list |
| UX-S1-3 glyph + text, contrast 4.5:1 | `chipContrast.test.ts` + `PullRequestChip.test.tsx` | `chip states should expose glyph and text and meet contrast` | Jest | U-89, U-90 |
| UX-S1-4 390px no h-scroll, targets >=44px | `multi-pr-session-mobile.spec.ts` | `card should have no horizontal scroll and 44px tap targets, and tapping +N must not also open the card or GitHub` | Playwright (mobile-chromium) | `scrollWidth <= clientWidth`; bounding boxes of badge and +N >= 44; both activated with `tap()`; after `tap()` on `+N` the URL is the detail hash only, no popup/new page event and the card default target is not navigated; after `tap()` on the badge only a GitHub page opens |
| UX-S1-5 badge aria-label | `PullRequestChip.test.tsx` | `badge should read View GitHub Pull Request #12 acme/app status open opens in new tab` | Jest | `getByRole('link', {name})` |
| UX-S2-1 3 rows, state text, hint | `multi-pr-session.spec.ts` | `detail should list three rows with state text and default-target hint` | Playwright | `pr-row-11/12/10`; hint line present |
| UX-S2-2 disclosure for 5 PRs | `multi-pr-session.spec.ts` | `detail should collapse earlier merged PRs behind a 2 earlier PRs button` | Playwright | Open+2 merged shown; button `aria-expanded=false`; click reveals, focus stays, label "Hide earlier PRs" |
| UX-S2-3 failing row isolated | `PullRequestList.test.tsx` + `multi-pr-session.spec.ts` | `one failing row should show Can't refresh chip and Retry while others stay actionable` | Jest + Playwright | Mock `poll_error=auth` on one row; Retry works; others' Merge enabled |
| UX-S2-4 legacy single-PR | `multi-pr-session.spec.ts` | `detail should show one row without role chip disclosure or hint when legacy single PR` | Playwright | Seed legacy session |
| UX-S2-5 390px, long titles wrap, nothing hover-only | `multi-pr-session-mobile.spec.ts` | `detail list should wrap long titles with no horizontal scroll and no hover-only content` | Playwright (mobile-chromium) | 200-char title; no h-scroll; tooltip content is in the row text, asserted without hovering |
| UX-S2-6 keyboard order | `multi-pr-session.spec.ts` | `list should be operable by keyboard in DOM order` | Playwright | Tab: link, overflow, disclosure; Enter/Space operates; focus ring visible |
| UX-S3-1 default merge targets #12 and says so | `multi-pr-session.spec.ts` | `merge dialog should name #12 when active PR is open and #11 merged` | Playwright | Dialog text "Merge #12 acme/app"; network request carries `pr=#12` |
| UX-S3-2 explicit row target | `multi-pr-session.spec.ts` | `row merge should target that row regardless of active` | Playwright | Open overflow on non-active open row; request carries that row |
| UX-S3-3 focus trap, Esc, focus return | `PullRequestList.test.tsx` + `multi-pr-session.spec.ts` + `multi-pr-session-mobile.spec.ts` | `dialogs should trap focus close on Esc and return focus` and A-16 | Jest + Playwright (chromium and mobile-chromium) | Merge, Close, Not-mine dialogs at 1280px; the same four sheets (plus Track a PR by URL) under touch; after a detach/merge that removes or reorders the invoking row, focus is on the next row, else the previous row, else the heading (never body) |
| UX-S3-4 failures keep dialog open with Retry/Cancel | `PullRequestList.test.tsx` | `dialog should stay open with inline alert Retry and Cancel when merge fails` | Jest | Not mergeable, auth, rate limit, network, FailedPrecondition, already-merged switches to Close-only |
| UX-S3-5 Close/Detach copy | `PullRequestList.test.tsx` | `close and detach dialogs should state GitHub effect` | Jest | "PR stays on GitHub" copy present |
| UX-S3-6 Pin undo 8s keyboard | `PullRequestList.test.tsx` + `multi-pr-session.spec.ts` | `pin toast should offer keyboard-reachable Undo for 8s pausing on focus` and A-11 | Jest + Playwright | Jest: fake timers, Undo restores derived active PR; A-11: the same in a real browser with `page.clock`, plus the untimed row-menu Unpin path |
| UX-S2-7 held lines with why and release | `PullRequestList.test.tsx` | U-147 | Jest | Each hold kind has copy and a release verb; none when nothing holds |
| UX-S2-8 unverified refresh and distinct copy | `PullRequestList.test.tsx` + `multi-pr-session.spec.ts` | U-147 and `refresh should re-poll a seeded unverified row and show checking then could-not-verify copy` | Jest + Playwright | Seed an unverified row; click Refresh; request carries the row; two distinct strings |
| UX-S3-8 detach copy, Undo, Track by URL, toast does not overlap | `PullRequestList.test.tsx` + `multi-pr-session-mobile.spec.ts` | U-148 and `undo toast should not overlap the bottom sheet buttons or the bottom nav at 390px` | Jest + Playwright (mobile-chromium) | Bounding boxes of toast vs sheet buttons and nav do not intersect; Undo re-attaches |
| UX-S3-7 overflow items >=44px at 390px | `multi-pr-session-mobile.spec.ts` | `overflow menu items should be 44px tap targets` | Playwright (mobile-chromium) | Bounding boxes; menu opened and items chosen with `tap()` |
| UX-S4-1 switcher radiogroup / select | `VcsWidget.test.tsx` + `multi-pr-session.spec.ts` (segmented control at 1280px) + `multi-pr-session-mobile.spec.ts` (native select) | `switcher should default to active PR; segmented on desktop, native select on mobile` | Jest + Playwright | U-100; under `mobile-chromium` the native `<select>` is present and the segmented control is absent |
| UX-S4-2 target stated in text | `VcsWidget.test.tsx` | `widget should state Actions apply to #N next to buttons always` | Jest | Also after fallback "#14 is now merged; showing #12" |
| UX-S4-3 keyboard + aria-live | `multi-pr-session.spec.ts` | `switcher should be keyboard operable and announce selection` | Playwright | Arrow keys; `aria-live=polite` text changes |
| UX-S4-4 no switcher for one open PR | `VcsWidget.test.tsx` | `widget should render no switcher when one open PR` | Jest | Zero open PRs: no switcher, Merge/Close disabled with reason and Create PR |
| UX-S5-1 "2 of 3 PRs merged" + 3 rows | `PullRequestSection.test.tsx` | U-101 | Jest | |
| UX-S5-2 blocker sentence | `PullRequestSection.test.tsx` | `section should name the blocking PR in a sentence` | Jest | "waiting on #23"; StuckItem shows k of N |
| UX-S5-3 progressbar semantics | `PullRequestSection.test.tsx` | `progress should expose role progressbar with text, not color only` | Jest | `aria-valuenow`, text |
| UX-S5-5 merged, session still active hold copy | `PullRequestSection.test.tsx` | U-149 | Jest | Idle window comes from the response; max-hold warning; no "stuck" wording |
| UX-S5-4 closed-unmerged only | `PullRequestSection.test.tsx` | `section should explain cannot complete and offer Attach another PR when only closed` | Jest | "0 merged, 3 closed..." with exit link |
| UX-S6-1 Up Next chip link name | `PRCard.test.tsx` | `chip link name should be Open session refactor-auth` | Jest | Two sessions: "2 sessions" menu, Esc closes |
| UX-S6-2 no match = no chip, no gap | `PRCard.test.tsx` | `card should render no chip or gap when no tracked session` | Jest | Snapshot of layout |
| UX-S7-1 distinct scan states | `artifacts-tab.spec.ts` | `tab should show distinct text for not_linked scanning none_found has_data and failed` | Playwright | `artifacts-scan-state`; "Extraction pending" absent for completed empty scan |
| UX-S7-2 Rescan reachable, disabled during scan, announced | `artifacts-tab.spec.ts` | `rescan should be one click, disabled with spinner during scan and announced on completion` | Playwright | `artifacts-rescan` |
| UX-S7-3 not_linked explains, no useless Rescan | `ArtifactsTab.test.tsx` | `not_linked should explain and hide Rescan` | Jest | Non-Claude program-aware reason |
| UX-S7-4 scanning >60s escape | `ArtifactsTab.test.tsx` | `scanning past 60s should offer Rescan and Cancel waiting` | Jest | Fake timers (no real wait) |
| UX-S7-5 grouping by PR | `ArtifactsTab.test.tsx` | U-116 | Jest | |
| UX-S7-6 390px groups collapse, 44px | `artifacts-tab-mobile.spec.ts` | `tab should collapse groups with 44px tap targets and no h-scroll` | Playwright (mobile-chromium) | Group disclosure buttons activated with `tap()` |
| UX-S7-7 mentioned PRs collapsed + labeled | `ArtifactsTab.test.tsx` | `mentioned PRs should be collapsed and labeled not created by this session` | Jest | |
| UX-S7-8 secrets show `[redacted]` | `ArtifactsTab.test.tsx` + `artifacts-tab.spec.ts` | `tab should show redacted marker never the token value` | Jest + Playwright | Seed blob with token URL |
| UX-S8-1 filtered browse | `artifacts-view.spec.ts` | `page should list only acme/app PR records with session link and chip` | Playwright | `?type=pr&repo=acme/app` |
| UX-S8-2 search and clear | `artifacts-view.spec.ts` | `search should show matched type per row and Clear returns to browse in one action` | Playwright | `artifacts-search`, `1a2b3c4` |
| UX-S8-3 distinct state copy | `artifacts-view.spec.ts` | `page should show distinct copy and exit action for loading empty-browse empty-filter empty-search search-error browse-error load-more-error` | Playwright | Route-mock failures; assert each exit action; query <2 chars inline validation; the 15 s loading timeout (A-14) |
| UX-S8-4 responsive layout | `artifacts-view.spec.ts` (1280px) + `artifacts-view-mobile.spec.ts` | `page should be tabular on desktop and single column with filters behind a button on mobile` | Playwright | Desktop: `<th scope>` table; mobile: cards, Filters button opens the sheet by `tap()`, 44px targets, no h-scroll |
| UX-S8-5 URL state | `artifacts-view.spec.ts` | `reload and Back should restore filters and query` | Playwright | |
| UX-S8-6 single status region | `artifacts-view.spec.ts` | `result count and loading should be announced via one role=status region` | Playwright | Count of `role=status` = 1 |
| UX-S8-7 Axe | `artifacts-view.spec.ts` | `page should have no WCAG AA violations at both widths` | Playwright (Axe) | |
| UX-S8-8 reachable <=2 clicks from nav | `artifacts-view.spec.ts` (desktop nav) + `artifacts-view-mobile.spec.ts` (mobile menu) | `artifacts page should be reachable from main navigation in two clicks / two taps` | Playwright | Desktop: nav link; mobile: open the mobile menu and tap the entry, route is `/artifacts` |
| UX-S9-1 omnibar action | `omnibar-find-pr.spec.ts` | `omnibar should offer Find sessions for acme/app#12 and select via keyboard` | Playwright | Paste URL; arrows + Enter; navigates `/artifacts?pr=acme/app%2312`; one match opens session directly with toast |
| UX-S9-2 error keeps omnibar open | `omnibar-find-pr.spec.ts` | `omnibar should show inline error and keep other actions when lookup fails` | Playwright | RPC mocked to fail; Retry; Esc closes |
| UX-S9-3 single match keeps View all and labels | `omnibar-find-pr.spec.ts` | `single match should navigate with a View all action and Tracked or Seen in transcript label` | Playwright | Toast action opens `/artifacts?pr=`; detail header link present; several-match ordering is A-15 |
| UX-S9-4 query length rule and SHA hint | `ArtifactsSearch.test.tsx` | U-150 | Jest | 1 char: inline validation, no request; 2 to 6 hex: runs plus hint |
| UX-S10-1 flag toggle without reload | `multi-pr-session.spec.ts` | `toggling reader:ui should change UI on next render without reload` | Playwright | Via feature-flag RPC |
| UX-S10-2 no console errors flag off | `multi-pr-session.spec.ts` | `flag off should produce no console errors` | Playwright | Console listener |
| UX-S10-3 live announcements | `PullRequestList.test.tsx` | `status region should announce PR #12 is now merged debounced 2s and never initial render` | Jest | Fake timers |
| UX-S10-4 token/contrast registry | `chipContrast.test.ts` | U-90 | Jest | Light and dark |
| UX-S10-5 test hooks present | `multi-pr-session.spec.ts` | `ids pr-chip pr-more-button pr-list pr-row-N pr-switcher artifacts-scan-state artifacts-rescan artifacts-search should exist` | Playwright | Also asserts no CSS-class locators in new specs (convention check) |
| UX-S11-1 prompt names issue and PRs; nothing closes until confirmed | `backlog-import-close-issue.spec.ts` | `reimporting a finished item should show the close prompt and send no close request until Close issue is activated` | Playwright | Route-mock `ImportGitHubIssue` (finished, issue open) then count `CloseFinishedItemIssue` requests: zero before the click, one after; the consequence line text is asserted verbatim (A-12) |
| UX-S11-2 390px bottom sheet, trap, dismiss | `backlog-import-close-issue-mobile.spec.ts` | `close prompt should be a bottom sheet with 44px stacked tap targets, trap focus and dismiss without any request` | Playwright (mobile-chromium) | `tap()` targets, no h-scroll; Esc and Not now send no request; focus returns to the trigger |
| UX-S11-3 outcomes have copy and an exit | `CloseIssuePrompt.test.tsx` + `backlog-import-close-issue.spec.ts` | `prompt should show distinct copy and an exit for closed, already closed, no longer finished, GHE, no token and failed` | Jest + Playwright | U-137; route-mock each outcome |
| UX-S11-4 no prompt when not applicable | `BacklogPage.githubImportClose.test.tsx` + `backlog-import-close-issue.spec.ts` | `no prompt for an unfinished item, a closed issue or the flag off, and the duplicates message is unchanged` | Jest + Playwright | U-138 |
| UX-S11-5 bulk per-row busy, result, announcement, Open on GitHub | `CloseIssuePrompt.test.tsx` + `backlog-import-close-issue.spec.ts` | U-145 and `bulk prompt should close one row at a time with its own busy state and result` | Jest + Playwright | Route-mock two offers; one slow close; the other row stays operable; one status announcement per result |
| UX-X-1 keyboard everywhere | `multi-pr-session.spec.ts`, `artifacts-view.spec.ts`, `backlog-import-close-issue.spec.ts` | `every control S1..S9 and S11 should be tab reachable in visual order with visible focus ring` | Playwright | Focus ring contrast >=3:1 (computed style check) |
| UX-X-2 unique accessible names, not color-only | `PullRequestList.test.tsx` | `links buttons and status regions should have unique names containing #N owner/repo` | Jest | |
| UX-X-3 touch targets | `multi-pr-session-mobile.spec.ts` + `multi-pr-session.spec.ts` | `targets should be >=44px on a touch device and >=24px at 1280px` | Playwright | Mobile file: tap targets >=44px; desktop file: >=24px |
| UX-X-4 no h-scroll, 200% zoom reflow | `multi-pr-session-mobile.spec.ts` | `surfaces should not scroll horizontally on a phone or when the layout viewport is halved to 640px and 320px` | Playwright (mobile-chromium) | A-17: viewport-width emulation of browser zoom with `assertReflow` (CSS `zoom` is not used: it is not what a browser zoom does). Text-only zoom: manual, see Manual-only |
| UX-X-5 every error has an exit | `multi-pr-session.spec.ts` | `every mocked error state should expose at least one exit action` | Playwright | Walk the exit-path matrix rows S1..S9 |
| UX-X-6 reduced motion | `PullRequestChip.test.tsx` | `spinners and skeletons should stop animating under prefers-reduced-motion` | Jest | `emulateMedia` equivalent |
| UX-X-7 task budgets | `multi-pr-session.spec.ts`, `artifacts-tab.spec.ts`, `omnibar-find-pr.spec.ts` | `task budgets: card to other PR detail <=2 clicks; merge non-active <=4 clicks; pasted URL to session <=3 steps; manual rescan <=2 clicks` | Playwright | Count actions per budget |

### Additional automated UX checks (Triad iteration 1, UX 7; not counted as UX criteria; all in T6.3.2 unless noted)

| # | Check | File | Method |
|---|---|---|---|
| A-1 | Live-region debounce for real: two state changes within 2 s give one announcement, none on initial render (UX-S10-3) | `multi-pr-session.spec.ts` | `page.clock.install()`, advance 1.9 s then 2.1 s, count `role=status` text changes |
| A-2 | Reduced motion (UX-X-6) in a real browser | `multi-pr-session.spec.ts` | `emulateMedia({reducedMotion:'reduce'})`; every spinner/skeleton has `getAnimations()` empty or `animation-name: none` |
| A-3 | Dark-mode contrast (Axe `color-contrast`, `colorScheme:'dark'`) of all chip states AND of the surfaces added since: the S4 switcher and VCS widget, the S5 backlog `PullRequestSection` with its progress bar and held lines, the S6 Up Next chip and its "2 sessions" menu, the S7 Artifacts tab, the S8 `/artifacts` page, the S11 prompt, every held line and the warning-style maximum-hold line (UX-S10-4). The warning style is added to `CHIP_TOKEN_PAIRS` so `chipContrast.test.ts` (U-90) covers it in light and dark (Jest) | `multi-pr-session.spec.ts` (also drives the S4, S5 and S6 pages; its `// @feature` header adds `backlog:item-detail, upnext:pr-card`), `artifacts-tab.spec.ts`, `artifacts-view.spec.ts`, `backlog-import-close-issue.spec.ts` | `axeAaViolations(page, scope)` from `tests/e2e/helpers/a11y.ts` with the contrast rule, one scope per surface, in dark mode |
| A-4 | Focus lands on the Pull requests heading after `+N` (UX-S1-2) on a touch device too | `multi-pr-session-mobile.spec.ts` | `tap()` `pr-more-button`, `toBeFocused` on the heading, Back restores the list |
| A-5 | Segmented control with 4 open PRs, native select from the 5th open PR | `multi-pr-session.spec.ts` (+ `VcsWidget.test.tsx`) | Seed 4 then 5 open PRs, assert radiogroup then `<select>` (UX-S4-1 boundary) |
| A-6 | Undo toast vs the 390px bottom sheet and bottom navigation | `multi-pr-session-mobile.spec.ts` | Bounding boxes do not intersect while both are visible (UX-S3-8) |
| A-7 | Held-line text present for each hold kind in the browser | `multi-pr-session.spec.ts` | Route-mock `pr_holds`; assert copy and release link |
| A-8 | REAL light-mode Axe runs (Triad iteration 2: the earlier A-3 claimed light-mode Axe through UX-S1-3, which is Jest-only): zero WCAG AA violations on the session-detail PR list, the open merge, close and Not mine / Stop tracking dialogs, the Track a PR by URL dialog, the S4 switcher and VCS widget (segmented control and native select), the S5 `PullRequestSection` (progress bar and every held line), the S6 Up Next chip and its open "2 sessions" menu, the Artifacts tab (each of its five states) and the S11 prompt (single and bulk) | `multi-pr-session.spec.ts` (S4, S5, S6 included), `artifacts-tab.spec.ts`, `backlog-import-close-issue.spec.ts` | `axeAaViolations(page, scope)` (wcag2a, wcag2aa, wcag21a, wcag21aa) scoped to each surface; UX-S8-7 already covers `/artifacts` |
| A-9 | Held-state lines wrap at 390px (UX-S2-5 extension): each of the six hold kinds including the longest (maximum hold) and the idle-window line, with a 200-character PR title, shows its full text, no truncation, no horizontal scroll | `multi-pr-session-mobile.spec.ts` | Route-mock `pr_holds` for each kind; `assertReflow(page)` plus `scrollWidth <= clientWidth`; text equals the copy in design/ux.md |
| A-10 | First appearance of a held line is announced once through the status region: not on initial render, once when it appears, not again across polls or re-renders, again after it clears and returns | `multi-pr-session.spec.ts` | `page.clock.install()`, mock `pr_holds` appearing, poll twice, count `role=status` text changes; clear and re-add |
| A-11 | Undo toast (pin and detach) in a real browser: keyboard reach, timing and the untimed path (design/ux.md "Undo toast") | `multi-pr-session.spec.ts` | `page.clock.install()`; pin by keyboard; assert focus is ALREADY on the toast Undo (a keyboard-initiated action moves it there; a pointer-initiated pin does not); fast-forward 9 s and assert the toast is still present while focus is inside; move focus out, fast-forward 8 s, assert it is gone; assert row-menu Unpin still works; repeat for detach with Track a PR by URL; Undo restores the previous derived active PR; Esc or Undo returns focus to the UX-S3-3 destination |
| A-12 | S11 irreversibility: the consequence line is asserted verbatim ("Closing posts a comment that links the merged PR. To undo, reopen the issue on GitHub; this app can't."); plus the bulk prompt on a phone: per-row busy, per-row result, Dismiss all disabled while busy, 44px targets | `backlog-import-close-issue.spec.ts` (line), `backlog-import-close-issue-mobile.spec.ts` (bulk) | `getByTestId('close-issue-prompt')` text equals; mobile: two offers, one slow close, `tap()`, `assertReflow` |
| A-13 | S8 interaction rows: Esc in the search box clears it; mobile filter sheet returns focus to the Filters button on Done and Esc; Load more keeps focus on the button and announces the new count; slow search (> 1 s) dims the previous results and shows the inline spinner; the "N sessions aren't scanned yet" notice appears when `unscanned_session_count > 0` | `artifacts-view.spec.ts` (Esc, load more, slow search, notice), `artifacts-view-mobile.spec.ts` (filter sheet) | `page.clock` for the 1 s slow-search threshold; route-mocks; `toBeFocused` |
| A-14 | Loading-to-error timeouts: S2 skeleton becomes the section error after 10 s; S8 skeleton becomes the browse error after 15 s; each error shows its exit action | `multi-pr-session.spec.ts` (10 s), `artifacts-view.spec.ts` (15 s) | `page.clock.install()`; route that never responds; `fastForward(9_900)` still skeleton, `fastForward(200)` error with Retry |
| A-15 | Pasted-PR lookup ranking (design/ux.md S9): Tracked rows above Seen-in-transcript rows, then session recency, one row per session even when matched by both sources | `omnibar-find-pr.spec.ts` (+ a Go ordering test in T9.2.1) | Mock `FindSessionsByPullRequest` with 4 matches in shuffled order and one session matched by both; assert row order and count |
| A-16 | Touch bottom-sheet variants on `mobile-chromium`: Merge, Close, Not mine and Track a PR by URL sheets trap focus, close on Esc and return focus (UX-S3-3); the Track a PR by URL field and the S8 search box stay inside `visualViewport` when the soft keyboard opens; S4 fallback message ("#14 is now merged; showing #12") and a native-select change are announced in the live region; focus destination after the invoking row is removed or reordered (UX-S3-3): detach of a middle row focuses the next row, of the last row the previous row, of the only row the "Pull requests" heading, never `document.body` (also a Jest row in `PullRequestList.test.tsx`) | `multi-pr-session-mobile.spec.ts`, `artifacts-view-mobile.spec.ts` | `stubVisualViewport` (`tests/e2e/helpers/a11y.ts`) shrinks the viewport; bounding box of the focused input and its button inside `visualViewport`; `tabOrderNames`; `role=status` text. A real on-screen keyboard stays a manual real-device check |
| A-17 | Reflow at 200% (UX-X-4): shrink the CSS layout viewport to 640 px wide (1280 at 200%) and 320 px (WCAG 1.4.10) on every new surface and assert no horizontal scroll, no control outside the viewport | `multi-pr-session-mobile.spec.ts` (and the three other mobile specs) | `page.setViewportSize` + `assertReflow(page)`. Text-only zoom is manual (see below) |
| A-18 | Real-browser WCAG 1.4.1 non-color check (Triad iteration 3): with emulated achromatopsia (`CDPSession` `Emulation.setEmulatedVisionDeficiency`, Chromium only) every S1/S2 chip state, the S5 progress bar and the S7 chips still expose their state as text and a glyph that differs per state: assert the visible label text per state is distinct, each glyph carries a distinct `data-glyph`/shape id, the S5 progress shows "k of N" text beside the `role=progressbar` value, and attach a screenshot of each to the Allure report for the manual copy review | `multi-pr-session.spec.ts`, `artifacts-tab.spec.ts` | Per-state locator text and attribute sets have no duplicates; no assertion reads a computed color; Jest `chipContrast.test.ts` keeps the contrast pairs |
| A-19 | Mobile rows for the surfaces that had none (Triad iteration 3), all under `mobile-chromium`: (S5) backlog `PullRequestSection` at 390px has no horizontal scroll, the progress bar and held/idle lines wrap, the "Stop tracking" and "Track a PR by URL" controls are `tap()`-able at >= 44px; (S6) the Up Next chip and the "N sessions" menu: chip `tap()` is >= 44px and opens the session, the menu opens by `tap()`, every menu link is >= 44px, Esc/outside tap closes and focus returns; (S9) the single-match toast "Opened the session tracking #12 (Tracked). 1 match. View all" does not intersect the bottom navigation or the detail header controls, and its View all is >= 44px and `tap()`-able | `multi-pr-session-mobile.spec.ts` (S5, S6), `omnibar-find-pr-mobile.spec.ts` (S9) | Bounding boxes; `tap()`; `assertReflow(page)`; boxes of toast vs bottom nav do not intersect |

### Mobile-only checks (plan T6.3.3 and the five `*-mobile.spec.ts` files; not counted as UX criteria)

| # | Check | File | Why a resized desktop window cannot give it |
|---|---|---|---|
| M-1 | Precondition: `matchMedia('(pointer: coarse)').matches`, `navigator.maxTouchPoints > 0` and a mobile user agent, asserted by the first test of every mobile spec | all five `*-mobile.spec.ts` | Fails loudly if the spec ever runs under desktop `chromium` |
| M-2 | Real touch tap targets: every control is activated with `locator.tap()` and measures >= 44px | `multi-pr-session-mobile`, `artifacts-tab-mobile`, `artifacts-view-mobile`, `backlog-import-close-issue-mobile` | `click()` on a resized window has no touch events and no hit-target slop |
| M-3 | Mobile menu route: open the mobile menu, tap the Artifacts entry, land on `/artifacts` in <= 2 taps | `artifacts-view-mobile` | The mobile menu only renders under the mobile breakpoint with `isMobile` |
| M-4 | No hover-only affordance: with `(hover: none)` every tooltip's text is also in the row and every action is reachable by tap | `multi-pr-session-mobile`, `artifacts-view-mobile` | Desktop pointer can hover; touch cannot |
| M-5 | Omnibar on a touch device: paste a PR URL, tap "Find sessions for acme/app#12", error keeps the omnibar open with the other actions tappable | `omnibar-find-pr-mobile` | The omnibar mobile layout and touch dispatch |
| M-6 | Issue-close prompt renders as a bottom sheet with stacked >= 44px buttons | `backlog-import-close-issue-mobile` | The sheet variant only renders at the mobile breakpoint |
| M-7 | The five specs are picked up by `mobile-chromium` (gating) and `mobile-webkit` (informational) and ignored by `chromium` and `chromium-dom` | U-135 | Guards the registration itself |

Manual-only items (cannot be asserted deterministically): screen-reader announcement WORDING on a real AT (UX-S10-3, UX-X-2), text-only zoom at 200% (UX-X-4; the layout-viewport emulation A-17 is automated, a browser minimum-font-size or text-size change is not), real on-screen soft-keyboard behavior and real-device 44px touch feel (UX-X-3, A-16), and mobile app (`https://onyx.staplerhome.internal:8444`) rendering of legacy scalars (C8, IS5). Owner of all of them: Tyler. Each is a PR checklist line on the T6.3.2 PR with the automated assertion above as the gating check; a copy-review checkpoint in the same PR covers the "Not mine" versus "Stop tracking" split, the held-state and maximum-hold lines (read for plain wording: user-facing strings say "local files", never "worktree"; the technical term stays in docs) and the S11 consequence line (plan Owner confirmations pending #13). The `reader:ui` flip is gated on this checklist.

## Test Stack
- **Unit (Go)**: `testing` + `testify` (`require`/`assert`), table-driven, `t.Parallel()` where state-free, injected clock and fake `prGitHubClient`, no `time.Sleep`; run with `gotestsum` via `make test`; `-race` mandatory for E2 concurrency tests (`make test-race`).
- **Integration (Go)**: real `NewEntRepository` on a temp SQLite under a short non-git path (memory: long temp path breaks socket/git tests), real `Storage`, query-count hook on the ent driver, spies for `DeleteSession`, `terminalSettled.settle` and `WorktreeCleaner`; `httptest` fake GitHub; no network (`BuildDependencies()` not used, narrow `ServerDependencies` instead).
- **Unit (web)**: Jest + React Testing Library via `cd web-app && bunx jest --no-coverage`; fake timers; `@/lib/nav` mocked with the `impl.memory` spread; Bun only.
- **E2E / UX**: Playwright + Allure under `tests/e2e/` (isolated server per worker, `E2E_WORKERS`), desktop specs in `chromium` at 1280px and `*-mobile.spec.ts` specs in `mobile-chromium` (gating, Pixel 7, 390x844, touch) and `mobile-webkit` (informational, `PW_WEBKIT=1`; nightly), Axe Core, specs carry `// @feature` headers, `data-testid`/ARIA locators only, no `waitForTimeout`, helpers in `tests/e2e/pages/`.
- **Static gates as tests**: `make pr-field-guard`, `make actor-field-guard`, `make lint-custom` (norawghrequest, noarchivedrevival, entfullscan, nowrongdbentity), `make proto-breaking`, `make ready-complexity-gate`, `bun run lint:duplicates`, ESLint ratchet, `route-meta.test.ts`.
- **Flaky-risk guards**: no wall-clock thresholds (new-RPC CI gate memory), filenames avoid `gate`, jittered backoff tests pin `Math.random`/seeded jitter, `scrollIntoView` before tray clicks.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./server/... ./config/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | >=80% line overall; 100% branch on the decision lists of `baseSafeToDelete`, `PRCleanupGate.Blocked`, `ActivePR`, `ItemPRVerdict`, `classifyPollError`, `UpdatePullRequestState` (measured with `go tool cover -html` on those functions) |
| TypeScript/Jest | `cd web-app && bunx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | >=80% line; new `PullRequestChip`, `PullRequestList`, `lib/pr/pullRequests.ts` >=90% |
| Playwright | `cd tests/e2e && bun run test` (then `make e2e-report`); mobile only: `npx playwright test --project=mobile-chromium` | Every UX criterion row above green: desktop rows in `chromium` at 1280px, 390px rows in `mobile-chromium`; Axe zero AA violations in both |

- All public service methods: happy path + error paths covered.
- All external integrations (GitHub client, SQLite, history files, feature-flag RPC): unit-mocked plus at least one integration test (I-35, I-49..I-55, I-67..I-72, I-84..I-94).
- UX acceptance criteria: each of the 69 criteria in `design/ux.md` has a row above.
- Data-loss sites: each has a fail-closed negative (below).

## Data-loss sites: fail-closed negatives

| Site | Fail-closed negatives (all must show NO delete/removal) | Test IDs |
|---|---|---|
| Retention sweeper (`baseSafeToDelete`, plan decision list steps 1 to 4) | Legacy number + no links + flag false; unknown/unverified terminal; open item-direct PR for a session whose own links are terminal; link, item-id or item-link lookup error; legacy number not represented by links; every scope off | I-50, I-51, I-52, I-53, I-54, I-55, I-78 |
| Cleanup gate (`cleanupItemWorktreesExcept` choke point; paths 1 to 6) | Open PR on each path; item-direct only; blocked not settled; bad item ref/unknown/legacy/lookup error; unverified terminal until exhausted; superseded round holds its worktree | I-67, I-68, I-69, I-70, I-71, I-72 |
| `DeleteSession` | Link/tombstone delete failure rolls back all; item-direct links and shared state survive; sweeper never calls it on any holding or error case; explicit user delete warns with open PRs; GC deletes nothing on error or when owner present | I-76, I-77, I-78, I-79, I-80 |
| Live work (P1 #4, RULE L) | A merged first PR does not complete or clean up an item whose work session is still active, on both merge-to-done paths; a live session is neither archived nor pane-killed by terminal-item cleanup or the 60 s sweep, including after the max hold, and the sweep retries it every pass (never settled while skipped); a probe error holds; stale counters are not trusted before maintenance succeeds | I-102, I-106, I-107, I-130, I-131, I-132, I-135 |
| Issue close on re-import (SM5) | Never silent, never forced by the client: the offer closes nothing; the close RPC re-checks `ItemFinished` and the issue state server-side; zero GitHub writes on every refusal | I-111, I-112, I-114 |
| Explicitly ungated paths (AW3) | Warning text and log line present (visible, not silent) | I-75, I-79 |

## Requirements coverage

Computed against the 44 IDs in the Requirement Index. A requirement counts as covered when at least one test row above names it in the first column.

| Requirement | Covered by (first-listed tests) |
|---|---|
| SM1 | U-58, U-59, U-96, U-97, I-02, UX-S1-1, UX-S2-1 |
| SM2 | I-35, I-36, I-43, U-60, U-61, U-95, U-134, UX-S3-1/2 |
| SM3 | I-57, I-59..I-66, U-82, U-85, U-101, I-106, U-152 |
| SM4 | I-12..I-24, I-26, I-31, I-32, I-100..I-103, I-105, U-98 |
| IS1 | U-29, U-30, U-31, U-32, U-33, U-57, I-25, I-81 |
| IS2 | U-01..U-03, I-01..I-10, U-15..U-18, I-29, I-33 |
| IS3 | U-23..U-27, U-37..U-45, I-37, I-40, I-44..I-46, U-130, U-131, I-110, U-133 |
| IS4 | U-71..U-81, I-47, I-58, U-85 |
| IS5 | I-42, U-58, U-87..U-103, I-82 |
| IS6a | U-104..U-107, I-84..I-87 |
| IS6b | U-108..U-117, I-88, I-89, U-114, I-109 |
| IS6c | U-118..U-123, I-90..I-93, U-119..U-121, U-151 |
| IS6d | I-94, U-124, U-125, I-82, UX-S8-1..8 |
| IS7 | I-10, I-48, I-83, I-96, I-97, `make ready`, U-135 |
| SM5 | U-136, I-111..I-115, I-133, U-137, U-138, UX-S11-1..4 |
| IS8 | U-136, I-111..I-116, I-133, U-137, U-138, UX-S11-1..4, M-6 |
| IS9 | U-135, M-1..M-7, UX-S1-4, UX-S2-5, UX-S3-7, UX-S7-6, UX-S8-4, UX-S8-8, UX-X-3, UX-X-4 |
| IS10 | I-44, U-64, U-32, U-33 |
| D1 | U-04..U-08, I-45 |
| D2 | U-09..U-13, I-49..I-55, I-67..I-74, I-76..I-78, U-86, I-106..I-108, I-130..I-132, I-135 |
| D3 | U-34, I-90, U-126 |
| D4 | via IS6a, IS6b, IS6c, IS6d |
| AW1 | U-06, U-10, I-14, I-24, I-51, I-69, I-71, I-102, I-106, I-130..I-132, I-135 |
| AW2 | U-78, I-47, I-52, I-63, I-68, I-72 |
| AW3 | I-75, I-79 |
| AW4 | U-56, I-41 |
| N1 | I-04, I-21, I-28, U-69 |
| N2 | I-35, U-37, U-38, U-41 |
| O1 | U-22, U-25, U-54, I-40 |
| O2 | U-37, U-54, U-55 |
| RC | U-19, U-20, I-11, I-32, I-38, I-55, I-66, I-98, U-99, U-129, I-104 |
| C1 | I-05, I-10 |
| C2 | U-24, U-28, U-31, I-95 |
| C3 | U-67, I-48, I-82 |
| C4 | U-46..U-48, I-39 |
| C5 | I-56 |
| C6 | U-19, U-21, I-11 |
| C7 | U-87..U-103, U-135, UX-S1-4, UX-S2-5, UX-S7-6, UX-S8-4, UX-X-3, UX-X-4 (all 390px rows in the mobile specs), M-1..M-7 |
| C8 | U-25, U-27, I-26, I-27, U-98, U-103, I-42 |
| OQ1 | U-19, U-20, I-11 |
| OQ2 | U-93 |
| OQ3 | I-42 |
| OQ4 | I-02, I-99 |
| OQ5 | I-65, U-127 |
| AM | U-139, I-93, I-117, I-118, I-121, I-134 |
| AU | I-119, I-120, U-146 |

Covered: 46/46 (AM: I-93, U-139..U-141, I-117, I-118; AU: I-119, I-120; D4 and IS7 are covered through their component items and gates; none is uncovered).

UX coverage: 69/69 criteria have a row (S1 5, S2 8, S3 8, S4 4, S5 5, S6 2, S7 8, S8 8, S9 4, S10 5, S11 5, cross-cutting 7).

## Counts

Derived by a script over the test tables (rows whose third column starts with `U-n` or `I-n`) and `grep -cE '^\| UX-'`:

| Type | Count |
|---|---|
| Unit (U-01..U-152) | 152 |
| Integration (I-01..I-135, incl. 1 migration test I-08, 7 manual-evidence/CI-gate items I-10, I-87, I-96, I-97, I-103, I-121, I-134) | 135 |
| UX acceptance (one per criterion) | 69 |
| Mobile-only checks M-1..M-7 (not UX criteria) | 7 |
| Additional automated UX checks A-1..A-19 (not UX criteria) | 19 |
| Migration test `migration_should_be_reversible` | yes (I-08; rollback/re-deploy cycle I-32) |
| Data-loss fail-closed negatives (tagged **DL**, recounted by command over the U/I rows) | 28 rows (was 20 before the Phase 4 pass: +I-102, I-106, I-107, I-114; +I-130, I-131, I-132 in Triad iteration 2; +I-135 in Triad iteration 3), 5 sites |

## Risks to test validity (not coverage gaps)

1. DeleteSession by an explicit user or MCP `destroy` is ungated by design (plan inventory rows 10 to 14); the fail-closed guarantee tested is at the storage level and at the sweeper boundary, plus a visible warning. If the owner wants a hard refusal, add a negative test then (plan T5.4.5 mentions escalation).
2. I-87 (T7.1.1 diagnosis) is evidence, not an automated test; it gates the final T7.2 scope.
3. Plan rows for `worktree_pr_poller.go`, `entfullscan`/`nowrongdbentity` are INFERRED; U-57 and I-39/I-56 force them to be opened during implementation.
4. Real-AT screen-reader and mobile-app checks stay manual (listed above).
