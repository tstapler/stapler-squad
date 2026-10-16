# Validation Plan: autonomous-stall-diagnosis

**Date**: 2026-10-13
**Complexity**: 2 (unspecified in the request, defaulted)
**Inputs read in full**: `requirements.md`, `implementation/plan.md` (35 tasks, 14 stories), `decisions/ADR-001`, `ADR-002`, `research/{architecture,pitfalls}.md`, `implementation/{adversarial,architecture}-review.md`, `.claude/skills/deterministic-fast-tests/SKILL.md`.
**Base**: `6a395b58f`. Test files and helper names below were checked against that tree (helper line numbers in "Fakes" are VERIFIED by grep; every *new* test name is a design, not an existing test).

## Happy Path Scenario

Given a backlog `SessionRoleWork` `ItemSession` for an `in_progress` item whose autonomous driver is running, an open nudge budget and both `diagnose_nudge_enabled` and `autonomous_stall_nudge_enabled` on, when the driver exits with `orchestrator_error` while the pane sits at a safe idle prompt, then exactly one `[AutonomousDriver] stopped` line and one `autonomous_stuck` row context carry a plain-language lead sentence ("The autonomous run hit an internal error and stopped. The session was waiting at its prompt. It was prompted to continue automatically (nudge 1 of 2). You do not need to do anything.") followed by `stop_reason=orchestrator_error state=idle_prompt action=nudged` and a redacted tail, one fixed-text nudge is written under the terminal lease with `diagnose_nudge_count == 1`, a driver is registered again with `AutonomousMode == true` throughout, and the session is neither ended nor respawned (test `STL-1`).

## Conventions (deterministic-fast-tests applied)

- Every new test calls `t.Parallel()`, uses no `time.Sleep`, no `t.Setenv`, no real tmux, no real network. `make lint-custom` (`notimesleeptest`) enforces the sleep rule.
- Time is injected: `WithStartupTimeout(tiny)`, `turnLeaseWait`, a fake `clock` for readoption follow-ups and `stallRecoveryGuard.now`, `nextEligibleAt` set explicitly on rows. Cancellation tests cancel from inside a fake's callback, never from a timer.
- Concurrency tests use a start barrier (`sync.WaitGroup` + closed channel) and assert on counts, then run under `-race`.
- Scale stays small: cap 3, reserve 1, 8 goroutines, 10 readoption candidates. Nothing here is a benchmark.
- Storage tests use the in-memory ent storage (`createTestStorage` / `createTestStorageWithRepo`), not a file path.
- One test file per plan task so waves never share a file (plan Dependency Visualization).
- Subtests are named by the table row (`t.Run(row.name, ...)`) so a failure names the state, reason or gate.

### Fakes (existing = VERIFIED at base; new = created by the task named)

| Fake | Status | Used by |
|------|--------|---------|
| `fakeHeadlessPool`, `fakeSendKeysProcessManager{failOn}`, `fakePanePreviewer`, `pumpIdleSignals` (`session/autonomous_driver_test.go:16,241,306,336`) | existing | DRV-* |
| `session.AssertLeaseFree(t, inst)` (used in `server/mcp/tools_write_lease_test.go:56`) | existing | NDG-* |
| `captureLogs(t)` (`server/services/autonomous_orchestration_service_test.go:445`), `createTestStorage` (`server/services/session_service_test.go:43`), `createTestStorageWithRepo` (`backlog_service_triage_test.go:2750`) | existing | SVC-*, OBS-*, STL-* |
| `fakeStopSource` (status, last activity, liveness, pane exit, tail; can panic or record call order) | new, 2.1.1c | DRV-*, SNAP-* |
| `fakeStallNudger` (scripted error per call, blocks on a channel until ctx done, counts calls, records ctx) | new, 3.2.3d | STL-*, STP-* |
| `fakeDriverStarter` (`startDriverIfAbsent` stand-in: returns `ErrDriverPresent` / `ErrNoDriverPool` / start error / registers) | new, 3.2.3a | STL-*, RSP-*, RDP-* |
| `fakePaneProbe`, `fakeItemSessionLookup`, `fakeClock` | new, 3.1.3d | RDP-* |
| `fakeRespawner` (channel-signalled, as in the existing respawn tests) | existing pattern | STL-*, RSP-*, SVC-* |
| `blockingStorage` (blocks in `GetItemSessionBySessionUUID` until released) | new, 3.2.5c | STP-2 |
| `fakeNudgeController` (scripted `GetCurrentStatus`, per-call hook, counts `SendKeys`) | new, 3.2.2a | NDG-* |
| `lockProbeSeams` (instanceFinder / storageGetter / bus / pool / `driverStarter` fakes that call `a.mu.TryLock()` and fail the test when it returns false, i.e. the registry mutex is held) | new, 3.1.3d | REG-12 |
| `requireReturns(t, 5*time.Second, fn)` watchdog (fails the test, never hangs; a passing run never waits on it) | new, 3.1.3d | REG-12..REG-15, STP-* |

### Seams the tests require (plan deltas; none is a new task, each is a field or parameter inside a task already planned)

| # | Seam | Why a test needs it | Add to |
|---|------|---------------------|--------|
| S1 | `AutonomousOrchestrationService` reads `diagnose_nudge_enabled`, `autonomous_stall_nudge_enabled` and `autonomous_driver_readoption` through three injectable `func() bool` fields defaulting to `config.EffectiveDiagnoseNudgeEnabled(config.LoadConfig())` / `EffectiveAutonomousStallNudgeEnabled` / `EffectiveAutonomousDriverReadoption` | `config.LoadConfig()` resolves the config dir from the environment; the only other way to flip it is `t.Setenv`, which forbids `t.Parallel()`. No existing test sets `diagnose_nudge_enabled` (`git grep DiagnoseNudgeEnabled -- '*_test.go'` is empty), so there is no precedent to copy | 3.2.3a, 3.1.3b |
| S2 | `captureTimeout` field on `AutonomousDriver` (zero -> 2 s) | Task 2.1.1a hard-codes `context.WithTimeout(Background, 2s)`; testing "a hung tail never blocks completion" would otherwise cost 2 real seconds | 2.1.1a |
| S3 | `maxRateLimitWait` injectable (plan already says "add one field, zero -> const") | `waitForRateLimitClear` computes `time.Now().Add(maxRateLimitWait)` (`autonomous_driver.go:485`) | 2.1.1c (as planned) |
| S4 | Inner `nudgeStalledWith(ctx, inst, claim, settleWindow, poll, submit)` behind `NudgeStalledAutonomousSession` | The public function uses the 3 s settle window; a cancel-mid-settle test must use an hour-long window and a 1 ms poll so a regression to `time.Sleep` hangs the test instead of passing by luck | 3.2.2a |
| S5 | The Step 7 / 7.5 / 7.6 goroutine in `server/dependencies.go` extracted to a named function that takes a `signalSettled func()` and returns on every path | `RDP-14` asserts that every exit signals (b); an inline goroutine cannot be driven | 3.1.3c |
| S6 | `ClaimDiagnoseNudgeSlot` result readable without a prior read (`ClaimResult`) | already planned; listed so the concurrency test asserts on the returned struct, not on a re-read | 3.2.1a (as planned) |
| S7 | `AutonomousDriver.Finished() bool` (non-blocking read of `done`) | `startDriverIfAbsent`'s liveness check; `Wait(ctx)` with an already-cancelled ctx and a closed `done` picks at random in its `select`, so it cannot be used (pre-mortem #5 suggested it; corrected) | 2.1.1a |
| S8 | `requireReturns` watchdog and `lockProbeSeams` (Fakes table) | a self-deadlock must fail the test, not hang `go test` until `-timeout=20m` | 3.1.3d |
| S9 | `recordsFor(buf, sessionTitle)` log filter and a unique `"stall-" + t.Name()` session title | `captureLogs` serializes capturers (`slogDefaultMu`) but parallel emitters can still write into the swapped default (BUG-087), so "exactly one stopped line" and canary-absence must be asserted on the filtered set | 3.1.4c |

## Requirement -> Test Mapping

Types: **Unit** (pure function or fakes only), **Integration** (real in-memory `Storage` and/or several real components), **Concurrency** (barrier + `-race`), **Guard** (build, lint or pin gate with a pass/fail command), **Doc** (AC1 document verification), **Manual** (human-run, not in CI).

Requirement keys: AC1-AC4 and R1-R6 from `requirements.md`; **C1** no new proto/ent/web-app surface; **C2** existing safeguards not weakened (archival, rework cap, remediation backoff, nudge cap); **C3** an external stop is not a stall; plan-derived **P1** re-supervise a working session; **P2** boot re-adoption under gates; **P3** stale-remediator deferral; **P4** secret-safe tail; **P5** one pinned, guarded, cancellable write surface.

### A. Pure domain (`session/domain/autonomous_stop_test.go`; Tasks 1.1.1a-c)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| DOM-1 | AC2, R2 | `TestStopReason_Class_CoversEveryValue` | Unit | Table over all 10 values plus `""`. `Class()`: `done`->Done, `cancelled_external`/`cancelled_shutdown`->Cancelled, rest->Stall. `""` is a stall. Invariant loop: no value has `IsDone() && IsStall()`. `IsValid` false for `"bogus"` |
| DOM-2 | AC2, R2 | `TestStopReason_AllValuesListed_ExhaustiveSwitch` | Unit | The exported value slice equals the set of constants (reflection-free: slice length vs a hand-listed expected set), so adding a constant without a table row fails here and in DOM-1 |
| DOM-3 | R3, AC2 | `TestClassifyObserved_Table` | Unit | Rows: `safe_idle`+alive -> `idle_prompt`; `needs_input` -> `needs_approval`; `error` -> `error`; `active` -> `working`; `idle` -> `idle`; unknown bucket -> `unknown`; `PaneDead` exit 0 -> `exited`; `PaneDead` exit 1 with bucket `active` -> `crashed` (liveness beats bucket); `TmuxAlive=false` -> `exited`; no controller -> `exited`; precedence not-alive > no-controller > bucket |
| DOM-4 | R5, AC3 | `TestDecideStallAction_RuleTable` | Unit | One named case per ADR-001 row 1-8: `done`/`cancelled_*` x every state -> `none`; archived -> `none`; retryPending -> `none`; `max_turns`+`idle_prompt`+enabled+allowed -> `respawn`; `panic`+`idle_prompt` and `panic`+`working` with headroom and flags on -> `respawn` (never `nudge`/`resupervise`); `working`+headroom+`ItemInWorkStatus` -> `resupervise`; `working`, no headroom -> `respawn` (never `none`); `working`+headroom+`!ItemInWorkStatus` -> `respawn`; `idle_prompt`+enabled+allowed+`ItemInWorkStatus` -> `nudge`; `idle_prompt`+enabled+allowed+`!ItemInWorkStatus` -> `respawn`; `needs_approval`, `error`, `exited`, `crashed`, `idle`, `unknown` -> `respawn` |
| DOM-5 | R5, R6, AC4, C2 | `TestDecideStallAction_ExhaustiveInvariants` | Unit | Loop over every reason x every state x 2^6 booleans (5,632 combinations at base: 11 x 8 x 64; count derived from the slices, not hard-coded). Asserts: `nudge` iff `IsStall` and reason not in {`max_turns`, `panic`} and not archived and not retryPending and `ItemInWorkStatus` and state == `idle_prompt` and enabled and allowed; `resupervise` only when reason not in {`max_turns`, `panic`} and `ItemInWorkStatus` and state == `working` and allowed; no state other than `idle_prompt` yields `nudge`; non-Stall reason yields only `none`; a stall that is not archived or retry-pending never yields `none` |
| DOM-6 | R5, C2 | `TestDecideStallAction_NudgeNeverWhenNudgeDisabled` | Unit | `NudgeEnabled=false`, `NudgeAllowed=true`, `idle_prompt`, `ItemInWorkStatus=true` -> `respawn`; proves either flag off restores today's respawn (Risk Control "Rollback"; the AND itself is asserted in STL-7 and STL-11) |

### B. Tail bound and redaction (Tasks 1.1.2a-c)

| ID | Req | Test File | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|-----------|------|-----------------------|
| TAIL-1 | R4, P4 | `session/autonomous_stop_snapshot_test.go` | `TestBoundedStopTail_RuneSafeAtByteCap` | Unit | 10,000-byte input ending `"...é"` with `é` straddling byte 4096: result `utf8.ValidString`, `len <= 4096`, ends with the last line's text |
| TAIL-2 | R4, P4 | same | `TestBoundedStopTail_LineCapAndANSI` | Unit | 200 lines -> last 40 only; CSI/OSC escape sequences absent; trailing blank lines trimmed |
| TAIL-3 | R4, P4 | same | `TestBoundedStopTail_EdgeInputs` | Unit | Empty, ANSI-only, invalid UTF-8 bytes, exactly-at-cap (4096) and cap+1: no panic, no replacement of kept runes, valid UTF-8 |
| TAIL-4 | P4, R4 | `server/services/autonomous_stop_detail_test.go` | `TestSanitizeStopTail_CanaryFamilies` | Unit | One canary per family: `sk-` key, `ghp_`+36 alnum, `AKIA` id, `xox*-` token, JWT, `Authorization: Bearer` 20+ chars, `postgres://u:pw@host/db`, `password=hunter2`, PEM private-key block. Each: `!strings.Contains(out, canary)`; PEM case: output equals `[tail withheld: secret pattern detected]` |
| TAIL-5 | P4 | same | `TestSanitizeStopTail_OneLineAndCap` | Unit | Newlines become ` ⏎ `; output <= 1024 bytes and rune-safe; exactly-at-cap and cap+1 inputs |
| TAIL-6 | P4 | same | `TestSanitizeStopTail_NonSecretTextPreserved` | Unit | A plain Claude idle-prompt tail passes through unchanged apart from whitespace collapse (guards over-redaction that would make the row useless) |
| TAIL-7 | P4, G5 | `server/services/autonomous_stop_detail_test.go` | `TestSanitizeStopTail_FailClosedResiduals` | Unit | Engineering G5. (a) `AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` and `export client_secret_key=abcd1234efgh`: first assert `redactErrorDetail` alone leaves them unredacted (documents why the rule exists), then `sanitizeStopTail` returns the withheld marker; (b) a headerless base64 key-body line (`MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7VJTUt9Us8cKj`) mid-tail: withheld; (c) accepted residual pinned as a named subtest: `the password is hunter2` passes through |
| TAIL-8 | P4 | same | `TestSanitizeStopTail_FailClosedRulesNoFalsePositiveOnCommonTails` | Unit | A 60-character URL line, a 50-character file path, an idle-prompt tail, and `git status` output are not withheld; an `API_KEY=` config-dump line is (the intended cost of failing closed) |

### C. Driver exits and snapshot (Tasks 2.1.1a-c; `session/autonomous_driver_test.go`, `session/autonomous_stop_snapshot_test.go`)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| DRV-1 | AC2, R2 | `TestAutonomousDriver_StopReasonPerExit` | Unit | Table, one row per exit site, fakes `fakeHeadlessPool`/`fakeSendKeysProcessManager{failOn}`/`fakeStopSource`: startup deadline only -> `startup_timeout`, `Turns==0`; `Stop()` during startup -> `cancelled_external`; parent ctx cancel during startup -> `cancelled_shutdown`; pool error on turn 2 with `maxTurns:5` -> `orchestrator_error`, `Turns==2`, `Stuck==true` (today: `max turns reached`, `Turns==5`); LLM error caused by cancel -> `cancelled_*`, not `orchestrator_error`; lease held elsewhere with tiny `turnLeaseWait` -> `lease_timeout`; `failOn` submit -> `submit_failed`; rate limit beyond injected cap -> `rate_limit_exhausted`; suppressed-turn ctx check cancel -> `cancelled_*`; loop exhausted -> `max_turns`, `Turns==maxTurns`; DONE -> `done`; panic -> `panic`. Each asserts `Turns` is the 1-based in-progress turn, `maxTurns` only for loop exhaustion |
| DRV-2 | AC2 | `TestAutonomousDriver_FiresCompletionExactlyOnce` | Unit | Every DRV-1 row: callback counter == 1; `Stop()` after completion is a no-op; a panic inside the callback does not re-enter `finish` |
| DRV-3 | AC2, C3 | `TestAutonomousDriver_PanicRecovery` (updated existing, `:820`) | Unit | Pool panics on first call: callback receives `StopReason: panic` once; `driver.Wait` returns true |
| DRV-4 | AC3, R4 | `TestAutonomousDriver_SnapshotCapturedBeforeCallback` | Unit | `fakeStopSource` records an ordered event log; capture events precede the callback event; snapshot fields (`Bucket`, `LastActivityAt`, `PaneTail`, `TmuxAlive`) equal the fake's |
| DRV-5 | AC3, R4 | `TestAutonomousDriver_SnapshotFailureNeverBlocksCompletion` | Unit | `Tail` panics, and separately blocks until ctx done with `captureTimeout` tiny (S2): callback still fires, `Snapshot.CaptureErr != ""` |
| DRV-6 | AC3 | `TestAutonomousDriver_SnapshotSkippedForCancelled` | Unit | `cancelled_external` and `cancelled_shutdown`: `CaptureErr == "skipped"`, `fakeStopSource` call count 0 (shutdown must not wait on tmux) |
| DRV-7 | R2 | `TestNormalizeStopReason_LegacyLiterals` | Unit | `{Done:true}` -> `done`; `{Done:false}` -> stall (unspecified); `{Done:false, Stuck:false}` -> stall; explicit `StopReason` wins over legacy fields |
| DRV-8 | R3 | `TestInstanceStopSource_BucketsViaIsSafeSteerStatus` | Unit | Table of detector statuses -> `StatusBucket`; `StatusReady` (dead code per `instinct_detection_status_ready_dead_code`) is not `safe_idle` |
| DRV-9 | C3 | `TestAutonomousDriver_StopSetsRequestedBeforeCancel` | Concurrency | 50 iterations of `Stop()` racing the run loop under `-race`: every outcome is `cancelled_external`, never `cancelled_shutdown` |
| DRV-10 | AC3 | `TestAutonomousDriver_Finished` | Unit | Added by pre-mortem #5 (S7). `Finished()` is false before `Start`, false while running, true after every exit incl. panic; reads `done` without blocking |

### D. Instance setters (Tasks 3.1.2a; `session/instance_actor_setters_test.go`)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| INS-1 | AC2, C1 | `TestSetAutonomousStopped_OutcomeByReason` | Unit | Every `StopReason`: snapshot `AutonomousMode==false`, turns 0, outcome in `{"", "done", "stuck"}` only: `done`->`done`, `cancelled_external`->`""`, every stall (incl. `""`) -> `stuck`, `AutonomousStopReason` equals input |
| INS-2 | AC2 | `TestSetAutonomousStoppedQuiet_OutcomeAlwaysEmpty` | Unit | Every `StopReason`: outcome `""` (boot-cleared candidates show no `SessionCard.tsx:888` badge) |
| INS-3 | AC2, P1 | `TestSetAutonomousStopObserved_KeepsModeAndOutcome` | Unit | Mode true stays true, outcome untouched, `AutonomousTurn==0`, reason recorded; then `SetAutonomousMode(true, ...)` clears the reason |
| INS-4 | AC2 | `TestInstanceSnapshot_CarriesAutonomousStopReason` | Unit | `Snapshot().Autonomous.AutonomousStopReason` reflects each setter; reading it never takes `i.mu` (reads via `Snapshot()` per `instance-lock-free-reads.md`) |
| INS-5 | C2 | `TestAutonomousSetters_NoRaceWithSnapshotReaders` | Concurrency | 8 writers cycling setters, 8 readers on `Snapshot()`, `-race` clean |
| INS-6 | AC2, G7 | `TestAutonomousSetters_ReturnErrorWhenActorDone` | Unit | Engineering G7. `NewLiveInstance(inst)` then `li.cancel()`: `SetAutonomousStopped`, `SetAutonomousStopObserved` and `SetAutonomousStoppedQuiet` each return a non-nil error wrapping `context.Canceled` and the snapshot is unchanged; with a nil `liveInstance` (direct path) they return nil |

### E. Service: wire outcome, characterization, class guards (Tasks 3.1.1a, 3.1.2b, 3.1.3b/d)

| ID | Req | Test File | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|-----------|------|-----------------------|
| SVC-1 | C1, AC2 | `autonomous_apply_stop_test.go` | `TestApplyInstanceStop_WireOutcomeSetClosed` | Unit | Every `StopReason` x action {Respawn, None, Nudge, Resupervise}: published `autonomous_outcome` in `{"", "done", "stuck"}`; `NewSessionUpdatedEvent` fields `autonomous_mode`, `autonomous_outcome` present; plus a "write failed" row (actor ctx cancelled): one Warn, no event published, callback continues (engineering G7) |
| SVC-2 | R4, C2 | `autonomous_orchestration_service_characterization_test.go` | `TestOnAutonomousDriverComplete_Characterization_*` (4 funcs: `RemediationNotDue`, `JustParked`, `UpdateItemSessionEndedFails_NotifiesBookkeepingFailed`, `NilRespawner`) | Integration | Written and green **before** the extraction; unchanged after. Pins the Respawn body's ordering `UpdateItemSessionEnded` -> `RemediationDue` -> goroutine and the rework/backoff gates |
| SVC-3 | C2 | existing 21 `TestAutonomousOrchestrationService_*` / `TestOnAutonomousDriverComplete_*` | (existing, count by `grep -cE '^func Test(AutonomousOrchestrationService_\|OnAutonomousDriverComplete_)'`) | Integration | `go test ./server/services -run 'Autonomous\|OnAutonomousDriverComplete'` passes unchanged before and after Story 3.1.1; 21 at base |
| SVC-4 | C3, AC2 | `autonomous_orchestration_service_class_test.go` | `TestOnAutonomousDriverComplete_CancelledClassGuards` | Integration | 20 rows: roles {work, triage, review, unrecognized, unlinked} x reasons {`cancelled_external`, `cancelled_shutdown`} x readoption flag {ON, OFF}. Each asserts: 0 `autonomous_stuck` rows written, `ItemSession.EndedAt == nil`, respawner 0, nudger 0, driver starter 0, no notification, no `UpdateItemSessionEnded`; triage item still `idea` (no `idea -> ready`); review item still `review`; instance outcome `""` for external |
| SVC-5 | C3 | same | `TestOnAutonomousDriverComplete_CancelledExternal_PreexistingOpenRowStaysOpen` | Integration | Open `autonomous_stuck` row + work role + `cancelled_external`: row still open (`resolveAutonomousStuck` not reached), `remediation_attempts` unchanged |
| SVC-6 | C3 | same | `TestOnAutonomousDriverComplete_ReviewRole_CancelledAfterItemLeftReview_ResolvesRow` | Integration | Item no longer in `review`: row resolved exactly as the existing `status != review` case; `UpdateItemSessionEnded` still not called |
| SVC-7 | AC2, P2 | same | `TestCancelledShutdown_FlagOn_KeepsModeAndRoundTrips` | Integration | Flag ON: snapshot `AutonomousMode==true`, reason `cancelled_shutdown`, no row, no notification, no respawn/nudger; `ToInstanceData`->`FromInstanceData` keeps `AutonomousMode==true` |
| SVC-8 | AC2, P2 | same | `TestCancelledShutdown_FlagOff_ClearsModeOutcomeStuckOnly` | Integration | Flag OFF: mode false, outcome `"stuck"`, reason `cancelled_shutdown` via `SetAutonomousStopped`; still no row, notification, respawn or nudge (matches Risk Control "flag-OFF semantics") |
| SVC-9 | C3 | same | `TestOnAutonomousDriverComplete_UnlinkedCancelled_NoFinalNotification` | Integration | The final notification switch is three-way: `Cancelled` publishes neither "complete" nor "stuck"; update event and one Info log still fire |
| SVC-10 | AC2 | same | `TestOnAutonomousDriverComplete_StallOutcomeUnlinked_TodayHandling` | Integration | Unlinked stall: mode cleared, outcome `stuck`, generic notification (today's), not nudged, not re-supervised |
| SVC-11 | AC2, C2 | `autonomous_orchestration_service_characterization_test.go` | `TestOnAutonomousDriverComplete_Characterization_EventPayloadEqualsPostWriteSnapshot` | Integration | Pre-mortem #7. Written green **before** the extraction: the `SessionUpdatedEvent` (`autonomous_mode`, `autonomous_outcome`) carries values equal to the instance snapshot after the write, for done and stall outcomes; stays green after 3.1.2b moves the publish into `applyInstanceStop` |
| SVC-12 | C2 | same | `TestOnAutonomousDriverComplete_Characterization_MarkStuckOncePerRolePerStall` | Integration | Pre-mortem #7. Work, triage, review and unrecognized roles each: `MarkStuck` called exactly once for a stall, zero for done and cancelled; green before and after `MarkStuck` moves into `handleWorkStall` for the work role. After Task 3.2.3b a work **Nudge** plan calls it exactly twice (plan-time `action=nudging`, then the final-action refresh); every other plan and role stays at once (UX G1) |

### F. Registry and completion identity (Task 3.1.3a; `autonomous_driver_registry_test.go`)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| REG-1 | P5, C3 | `TestClaimCompletion_Superseded_NoSideEffects` | Unit | D1 (`RunID` 1) completes while D2 (`RunID` 2) is registered: result `superseded`; registry still holds D2; instance mode/outcome unchanged; one Warn naming the dropped `StopReason` (review M2) |
| REG-2 | P5 | `TestClaimCompletion_DuplicateSameRunID` | Unit | Second claim for the same `RunID` returns `duplicate`; the callback returns without acting |
| REG-3 | P5 | `TestClaimCompletion_RunIDZeroMatchesAnything` | Unit | Legacy literal with `RunID 0` proceeds against any registered driver |
| REG-4 | C3, P5 | `TestStopAndDeregisterDriver_CancelsRecoveryAfterClaim` | Unit | After `claimCompletion` returns (before any item lookup), `stopAndDeregisterDriver(title)` cancels the recovery ctx and deletes the entry |
| REG-5 | P5 | `TestClaimCompletion_NewerRunCancelsOlderRecovery` | Unit | Completion from a newer run cancels the older run's recovery and takes the entry |
| REG-6 | P5 | `TestStartDriverIfAbsent_ErrorTable` | Unit | Driver present -> `ErrDriverPresent`, registry unchanged; nil pool -> `ErrNoDriverPool`; `Start` error -> that error, nothing registered |
| REG-7 | P5 | `TestStartDriverIfAbsent_ConcurrentCallersOneWins` | Concurrency | 8 goroutines through a barrier: exactly one driver started, seven `ErrDriverPresent`, `-race` clean |
| REG-8 | P5 | `TestRegisterDriver_DisplacedDriverStoppedOnce` | Unit | Registering a different driver calls the previous driver's `Stop()` exactly once, outside `a.mu` (no deadlock under a `Stop` that takes the registry lock), and logs a Warn |
| REG-9 | P5 | `TestVoidStarters_LogErrorKeepSignatures` | Unit | `StartAutonomousDriverWithTimeout` / `ForInstance` still compile against the interface assertion; a nil pool logs and registers nothing |
| REG-10 | P5, C2 | `TestSetLifecycleContext_AtomicNoRace` | Concurrency | Pre-mortem P1 #1 (was "guarded under mutex"): concurrent `SetLifecycleContext` and `driverCtx()` under `-race`; `driverCtx()` returns without taking `a.mu` (asserted by calling it from a seam while `a.mu` is held by the test) |
| REG-11 | P5 | `TestRecoveryCtx_DerivedFromDriverCtx_ShutdownCancels` | Unit | Cancelling the lifecycle ctx cancels an in-flight recovery ctx (not `Background()`); the respawn goroutine's ctx also comes from `driverCtx()` |
| REG-12 | P5 | `TestRegistry_NoSeamCalledWithMutexHeld` | Unit | `lockProbeSeams`: through `claimCompletion`, `startDriverIfAbsent`, `restartAfterRecovery`, `registerDriver`, `stopAndDeregisterDriver`, every seam call observes `a.mu.TryLock() == true`; the probe fails the test otherwise (the `a.mu` rule) |
| REG-13 | P5 | `TestClaimCompletion_DoesNotReenterLock` | Unit | Under `requireReturns`: a callback that runs `claimCompletion`, then `canRestart`, then `stopAndDeregisterDriver` completes; `canRestartLocked` is reachable only from `restartAfterRecovery` |
| REG-14 | P5, C3 | `TestStopDuringStartDriver_NoDeadlockNewDriverTornDown` | Unit | Under `requireReturns`: `Stop` while `startDriverIfAbsent` is blocked in a fake `NewAutonomousDriver` (sentinel inserted): `Stop` returns, the build then returns `ErrDriverPresent`, the built driver's `Stop()` was called once and it is not registered |
| REG-15 | P5, C3 | `TestDeleteSession_ConcurrentWithCallbackAndStart_NoDeadlock` | Concurrency | Integration with a real `SessionService.DeleteSession`, a completion callback and a `startDriverIfAbsent` through a barrier under `-race`, inside `requireReturns` |
| REG-16 | P5 | `TestStartDriverIfAbsent_FinishedDriverIsReplaced` | Unit | Registered driver with `Finished()==true` (fake): a new driver is started and registered, one Warn; a live registered driver still returns `ErrDriverPresent`; a `starting` sentinel returns `ErrDriverPresent` |

### G. Boot re-adoption (Tasks 3.1.3c/d; `autonomous_readoption_test.go`)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| RDP-1 | P2, AC4 | `TestReadoptGates_Table` | Integration | **28 rows** (27 failing-gate rows, 1 all-pass control), each with the other 11 gates passing. Asserts class, driver start count, mode after, outcome after, and the log line `result=... gate=...`: `status_active` reject x6 (`Hibernated`, `Stopped`, `Crashed`, `PermanentlyFailed`, `Failed`, `Paused`); `status_active` unknown x2 (`Creating`, `Restoring`); `not_archived` reject; `not_paused` reject; `mode_persisted` reject (no-op); `controller_present` reject, and unknown on the fallback pass; `no_driver` skip (registered) and skip (`ErrDriverPresent` from the start); `not_retry_pending` reject; `item_session_open` reject (`ErrNotFound`), reject (`EndedAt != nil`), unknown (non-`ErrNotFound` error); `item_status_matches_role` reject x3 (work != `in_progress`, review != `review`, triage != `idea`) and unknown (storage error); `pane_alive` reject; `pane_owned` reject (mismatch) and unknown (probe timeout); `no_step75_driver` reject (`InitialPrompt != ""`, `InitialPromptSentAt` zero); all-pass -> exactly one driver started with 5 min warm-up |
| RDP-2 | P2 | `TestReadopt_RunTwice_SecondPassSkipsWithoutClear` | Integration | After pass 1 adopts X, pass 2 (test only; production is `sync.Once`) starts none and **X.AutonomousMode stays true**; log `skip gate=no_driver`; registry unchanged |
| RDP-3 | P2 | `TestReadopt_ErrDriverPresentFromStart_IsSkip` | Integration | `startDriverIfAbsent` returns `ErrDriverPresent`: mode untouched, registry unchanged, logged `skip gate=no_driver` |
| RDP-4 | P2 | `TestReadopt_TransientProbeError_LeavesModeAndReevaluates` | Integration | `fakePaneProbe` returns a timeout for `pane_owned`: mode untouched, no driver, Warn `unknown`; follow-up pass with the probe now passing adopts it; a probe that still errors after the last follow-up leaves the mode untouched and logs one Warn |
| RDP-5 | P2 | `TestReadopt_TransientStorageError_LeavesModeUntouched` | Integration | `fakeItemSessionLookup` returns a non-`ErrNotFound` error: same assertions as RDP-4 |
| RDP-6 | P2, AC2 | `TestReadopt_RejectClearsQuietly_OutcomeEmpty` | Integration | Reject-class candidate: `SetAutonomousStoppedQuiet(cancelled_shutdown)`, snapshot outcome `""`, one Info line naming instance, gate and class `reject`, no driver started, `NewSessionUpdatedEvent` published |
| RDP-7 | P2 | `TestReadopt_StartFailure_ClearsLikeReject` | Integration | `ErrNoDriverPool` and a `Start` error: mode cleared quietly, Warn `start_failed` |
| RDP-8 | P2 | `TestReadopt_PerBootCap_StableOrderDeferredNotCleared` | Integration | 10 passing candidates with shuffled input order: first 8 by `CreatedAt` ascending (ties by instance UUID) start; 2 logged `deferred` with mode untouched; follow-up passes 60 s apart (fake clock, at most 3) start them as slots allow; candidates still over cap after the last pass are cleared quietly with one Warn naming the count |
| RDP-9 | P2 | `TestReadopt_FlagOff_DryRun` | Integration | Flag OFF: gates evaluated and logged (`would_adopt` or failing gate/class), no driver started, reject and `would_adopt` candidates cleared quietly (outcome `""`), skip and unknown untouched |
| RDP-10 | P2 | `TestReadopt_InvariantNoModeWithoutDriver` | Integration | Mixed candidate set (adopted, rejected, skipped, unknown, deferred), flag ON and OFF: after the final pass no instance has `AutonomousMode==true` with no registered driver, except those logged `unknown` |
| RDP-11 | P2 | `TestReadopt_PersistedHibernatedRoundTrip_Rejected` | Integration | Instance saved with `AutonomousMode==true`, `Status==Hibernated`, loaded through `FromInstanceData`, pass runs: reject by `status_active`, mode cleared, outcome `""` |
| RDP-12 | P2 | `TestReadopt_TriggerOnceAfterBothSignals` | Unit | Order (a)(b) and (b)(a) each run the pass exactly once; (a) alone does not run it |
| RDP-13 | P2 | `TestReadopt_FallbackTimer` | Unit | (a) without (b): pass runs once when the fake clock passes 90 s, `controller_present` failures classified `unknown`; when (b) arrives first the fallback never runs a second pass |
| RDP-14 | P2 | `TestStep76Goroutine_SignalsSettledOnEveryExit` | Unit | Via seam S5: normal return, early error return and panic-recovered return each call `signalSettled` exactly once, after the settle step |
| RDP-15 | P2 | `TestReadopt_RefusesWhenLifecycleCtxNil` | Unit | Pass with `lifecycleCtx == nil` starts no driver (it would not stop on shutdown) |
| RDP-16 | P2 | `TestReadopt_AdoptionResetsTurnBudget_Logged_LeavesStopReason` | Unit | Adopted driver gets `AutonomousMaxTurnsOrDefault`; log line states it; `AutonomousStopReason` and the re-supervise budget are untouched; `stallRecoveryGuard` is stamped for the candidate's item (30 min, fake clock) so the stale detector defers (pre-mortem #8); each candidate log line carries `last_activity_age` |
| RDP-17 | P2, C2 | `TestPaneProbe_UsesLifecycleStatusNotRawStatus` | Unit | Liveness read goes through `GetLifecycleStatus()`; an instance whose raw `Status` is `Restoring` is `unknown`, not `reject` (review minor M1) |
| RDP-18 | P2, C1 | `TestFeatureFlag_AutonomousDriverReadoption_RegisteredAndLiveSettable` | Unit | Registered in `feature_flag_service.go`; default equals the constant `autonomousDriverReadoptionDefault` (the one owner decision; the test reads the constant, so it passes for either value); setting it live changes the next `maybeReadopt` decision |

### H. Observability and secret safety (Tasks 3.1.4a-c; `autonomous_stop_detail_test.go`, `autonomous_orchestration_service_stop_test.go`)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| OBS-1 | AC2, R2, R4, P4 | `TestOnAutonomousDriverComplete_StopDetailRedacted` | Integration | `{orchestrator_error, Turns 2, Snapshot{Bucket safe_idle, LastActivityAt 12:03:10, PaneTail "...password=hunter2..."}}`: `MarkStuck` context starts with the plain summary (`The autonomous run hit an internal error and stopped. The session was waiting at its prompt.`), then ` | stop_reason=orchestrator_error state=idle_prompt action=… turns=2 last_activity=`; contains `[REDACTED]`, not `hunter2` |
| OBS-2 | AC2 | `TestOnAutonomousDriverComplete_LogsExactlyOneStoppedLine` | Integration | `captureLogs` with a unique `session=` title and the `recordsFor(buf, title)` filter (engineering G4; BUG-087): exactly one `[AutonomousDriver] stopped` record per stop with keys `session`, `item`, `role`, `run_id`, `stop_reason`, `state`, `turns`, `max_turns`, `last_activity_age`, `action`, `tail_bytes`, `resupervise`, `nudge_outcome`, `resupervise_cap_hit` (and `prior_stop_reason` for Resupervise); for a work stall the single line is emitted at `handleWorkStall` exit with the **final** action (a Nudge that fell back to Respawn logs `respawn`); `nudge_outcome=disabled` when the plan would have been Nudge with both flags on (pre-mortem #10); no tail text in any attribute; level Info for done/cancelled, Warn otherwise |
| OBS-3 | AC2 | `TestStopDetail_StateDistinguishesIdleErrorExited` | Unit | Three snapshots (`safe_idle`, `error`, pane dead exit 0) produce `state=idle_prompt`, `state=error`, `state=exited`; a fourth (exit 1) produces `state=crashed`. Literal AC2 "distinguishes idle/awaiting-input, error, completed-or-exited" |
| OBS-4 | AC2 | `TestStopReasonPhrase_EveryReasonHasRow` | Unit | Completeness over the full value list (DOM-2 slice) for `reasonSentence`, all `ObservedState` values for `stateSentence`, and every nudge-failure class for `nudgeFailurePhrase`; no sentence contains a tail |
| OBS-5 | AC2 | `TestNotification_StartupTimeoutNeverSaysStoppedAfterZeroTurns` | Integration | `startup_timeout`, `Turns 0`: body lacks `stopped after 0 turns`; non-turn reasons read `during turn N` |
| OBS-6 | P4 | `TestCanary_AbsentFromRowNotificationAndLogs` | Integration | Canary (`ghp_` + 36 alnum) in the tail: absent from the stuck row context, the notification body and the filtered captured records (and, being absent, from the unfiltered buffer too); present nowhere durable |
| OBS-7 | AC2, R2 | `TestStuckRowContext_ReturnedByListStuckBacklogItems` | Integration | Real `Storage`, `MarkStuck` with a `StopDetail`, call the `ListStuckBacklogItems` handler: `StuckBacklogItem.context` (proto field 9) equals the detail. This is the operator read path ADR-002 names; the plan has no test for it (added here) |
| OBS-8 | AC2 | `TestStopDetail_SingleLineWithMarker` | Unit | Newlines in the tail render as ` ⏎ `; the whole detail has no `\n` (`StuckItemDetail.tsx:203` collapses whitespace) |
| OBS-9 | AC2, R2 | `TestStopDetail_ResuperviseAndCapKeys` | Unit | Every detail carries `resupervise=n/3`; the cap-reached row carries `resupervise_cap_reached runs=3 last_stop_reason=startup_timeout` |
| OBS-10 | AC2, G4, G5 | `TestStopCopy_GoldenStringsAndNoCodes` | Unit | UX G4/G5. Every row of the plan's Operator-facing copy tables (reason x state x action, plus the nudge-failure phrases and titles) equals its constant byte for byte; for every `StopReason`, `ObservedState` and `nudge_outcome` class, no composed summary contains the code string (`orchestrator_error`, `idle_prompt`, `lease_busy`, `slot_unavailable`, ...); `none: delivery uncertain` is the only non-unlinked row whose "You act?" is Yes |

### I. Atomic claim (Task 3.2.1a; `session/diagnose_nudge_test.go`)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| CLM-1 | R5, C2 | `TestClaimDiagnoseNudgeSlot_ConcurrentCap` | Concurrency | Open row `count=0`, cap 3, reserve 1, 8 goroutines through a barrier: exactly 2 return `Claimed==true`, row `count==2` |
| CLM-2 | R5, C2 | `TestClaimDiagnoseNudgeSlot_CooldownInSameUpdate` | Integration | `diagnose_next_eligible_at` in the future, count 0 -> `Claimed==false`; past or nil -> `Claimed==true` |
| CLM-3 | R5, C2 | `TestClaimDiagnoseNudgeSlot_NoOpenRowFailsClosed` | Integration | No open `(item, autonomous_stuck)` row -> `Claimed==false`, nil error |
| CLM-4 | R5 | `TestClaimDiagnoseNudgeSlot_JustCappedOnLastSlot` | Integration | Claiming the last claimable slot sets `JustCapped` |
| CLM-5 | C2 | `TestRecordDiagnoseNudgeAttempt_MCPPathUnchanged` | Integration | Existing `session/diagnose_nudge_test.go` tests pass unchanged (MCP path keeps its own UPDATE) |
| CLM-6 | R5, C2 | `TestDiagnoseNudgeAllowedWithReserve_Table` | Unit | count 2/cap 3/reserve 1 -> false; count 1 -> true; reserve 0 equals `DiagnoseNudgeAllowed`; no row -> allowed (documented fail-open, claim is authoritative) |
| CLM-7 | C2 | `TestNudgeCount_NotResetByMarkStuckReopenOrResetRemediation` | Integration | Characterizes the plan's "lifetime counter" fact: reopen via `MarkStuck` and `ResetStuckRemediation` leave `diagnose_nudge_count` unchanged |

### J. Nudge writer (Tasks 3.2.2a/b; `session/autonomous_stall_nudge_test.go`, `session/nudge_gate_test.go`)

File header states: service tests with a fake nudger cannot prove promptness or no-write; these do.

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| NDG-1 | P5, R5 | `TestNudgeStalledAutonomousSession_NotIdle_ClaimUntouched` | Unit | `StatusExecuting`: `errors.Is(err, ErrNudgeTargetNotIdle)`, claim called 0 times, `session.AssertLeaseFree` passes, 0 `SendKeys` |
| NDG-2 | P5 | `TestNudgeStalledAutonomousSession_LeaseBusy_ClaimUntouched` | Unit | Lease held by another writer: `ErrLeaseBusy`, claim 0 |
| NDG-3 | P5, R5 | `TestNudgeStalledAutonomousSession_ClaimLost_ReleasesLeaseNoWrite` | Unit | Claim returns `false`: `ErrNudgeSlotUnavailable`, lease released, 0 `SendKeys` |
| NDG-4 | P5, C2 | `TestNudgeStalledAutonomousSession_RefusesNonActiveArchivedPaused` | Unit | Three rows: status not `Active`, archived, paused: error before the gate, 0 claim, 0 write |
| NDG-5 | P5, C3 | `TestNudgeStalledAutonomousSession_CancelMidSettle_NoWriteNoLeaseNoClaim` | Unit | Via S4: settle window 1 h, poll 1 ms, `fakeNudgeController` reports safe idle each poll and calls `cancel()` on poll 3; returns `context.Canceled`; `SendKeys` 0, lease never taken, claim 0. A watchdog (`select` with `t.Context()` deadline) fails the test if it does not return; no wall-clock assertion |
| NDG-6 | P5, C3 | `TestNudgeStalledAutonomousSession_CancelAfterGateBeforeLease` | Unit | Cancel from the gate-pass hook: same zero-write, zero-claim result |
| NDG-7 | P5, R5 | `TestNudgeStalledAutonomousSession_SubmitNotConfirmed_UncertainNoRefund` | Unit | `fakeSendKeysProcessManager{failOn: Enter}`: `errors.Is(err, ErrNudgeSubmitUncertain)`, lease released, claim result persisted (row count stays incremented), no release call exists |
| NDG-8 | P5 | `TestNudgeStalledAutonomousSession_FailureAfterTextSent_Uncertain` | Unit | Failure after the text keystrokes: wraps `ErrNudgeSubmitUncertain` |
| NDG-9 | AC3, P5 | `TestAutonomousStallNudgeText_ExactBytes` | Unit | The constant equals the literal `The autonomous driver noticed this session stopped. If the task is finished, call request_review now. If it is not, continue working on it and call report_progress when you reach a milestone. Do not ask for confirmation.`; contains no `%`; the bytes the fake `SendKeys` receives equal the constant byte for byte; item title and pane tail never appear in them |
| NDG-10 | P5 | `TestCheckNudgeEligibleWithSettleWindowCtx_HonoursCancel` | Unit | Cancelled ctx returns within one poll; a live ctx with the window elapsed behaves as `CheckNudgeEligibleWithSettleWindow` did |
| NDG-11 | C2 | `TestCheckNudgeEligible_MCPPathUnchanged` | Unit | `CheckNudgeEligible` delegates with `context.Background()`; existing `nudge_gate_test.go` cases pass unchanged |
| NDG-12 | C2 | `server/mcp` `TestWriteNudge_CancelledRequestCtx_ClassifiedCleanly` | Unit | Review minor M3: a cancelled request ctx now yields `context.Canceled` from the gate; `writeNudge` maps it to a defined error, not a panic or a "not idle" mislabel |
| NDG-13 | P5 | `make test-delivery-guards` (sinkguard tag) | Guard | See "Pin guard protocol" below |

### K. Service nudge branch (Tasks 3.2.3a-d; `autonomous_stall_recovery_nudge_test.go`)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| STL-1 | AC3, R5, R4 | `TestHandleWorkStall_Nudge_HappyPath` | Integration | Item "Fix flaky retry test" `in_progress`, work `ItemSession`, open `autonomous_stuck` row `count=0`, both nudge flags injected ON (S1), pane `"Claude Code idle prompt showing ? for shortcuts"`, exit `orchestrator_error`: `fakeStallNudger` called once; `diagnose_nudge_count==1`; `EndedAt==nil`; `remediation_attempts==0`; `UpdateItemSessionEnded` and `RemediationDue` never called; respawner 0; a driver registered again; `AutonomousMode==true` at every recorded observation (nudger entry, restarter entry, final); one INFO "nudged" notification, no FAILURE |
| STL-2 | AC3, AC4 | `TestFallbackAfterNudge_FailureTable` | Unit | One subtest per row of the plan's failure table, top to bottom: success; recovery ctx cancelled (stop/delete/toggle-off/shutdown); instance not `Active`/archived/paused; `ErrNudgeTargetNotIdle`; `ErrLeaseBusy`; ownership mismatch or dead pane; `ErrNudgeSlotUnavailable`; `ErrNudgeSubmitUncertain`; successful nudge but restart error (successor registered / `ErrNoDriverPool` / `Start` failure); `duplicate` completion. Each asserts action, `AutonomousMode` after, notification, `diagnose_nudge_count` never decremented |
| STL-3 | AC3 | `TestFallbackAfterNudge_CtxCancelBeatsSubmitUncertain` | Unit | `ErrNudgeSubmitUncertain` returned while the recovery ctx is cancelled: ctx-cancel row wins (no Resupervise, no Respawn, no restart) |
| STL-4 | AC3 | `TestFallbackAfterNudge_SubmitUncertainLeavesPane` | Unit | Live ctx + `ErrNudgeSubmitUncertain`: `SetAutonomousStopped(<stall reason>)`, stuck row stays, notification "nudge delivery uncertain", driver restarter 0, respawner 0, `KillTmuxPaneOnly` 0, slot stays consumed |
| STL-5 | AC3, C2 | `TestHandleWorkStall_RespawnAfterNudgeFailure_RunsOriginalBody` | Integration | Each Respawn-mapped row: `UpdateItemSessionEnded` called, `RemediationDue` consulted, respawn dispatched, mode cleared via `SetAutonomousStopped`, body carries the operator phrase for the class (`... a prompt could not be sent (the session was busy again) ...`) and not the class code |
| STL-6 | AC3, P1 | `TestHandleWorkStall_RoleScope_NoNudgeNoRestart` | Integration | review, triage, unrecognized, unlinked at `idle_prompt` with flag ON and headroom: nudger 0, restarter 0, today's handling per arm |
| STL-7 | AC3, C2 | `TestHandleWorkStall_Fallbacks` | Integration | 9 rows: `diagnose_nudge_enabled` OFF (stall flag ON) -> today's respawn; **`diagnose_nudge_enabled` ON and `autonomous_stall_nudge_enabled` OFF (the live config's state) -> today's respawn, nudger 0, log `nudge_outcome=disabled`** (pre-mortem P1 #2); stall flag ON and diagnose OFF -> respawn (kill switch wins); `count==2` (cap 3, reserve 1) -> respawn; `needs_approval` -> nudger 0; `MarkStuck (false, nil)` -> nudger 0 (downgrade to `None`, Warn names item status); `IsRetryPending` -> neither nudge nor respawn; row reopened by `MarkStuck` at cap -> post-`MarkStuck` re-read downgrades to Respawn before the settle wait, nudger 0; item `review` -> see STL-18 |
| STL-8 | C2, AC3 | `TestHandleWorkStall_Archived_NoNudgeNoRespawnNoRow` | Integration | `inst.IsArchived()`: action `none`, no stuck row (M4), respawner 0, nudger 0, driver starter 0 (`noarchivedrevival` behaviour) |
| STL-9 | AC3, C2 | `TestHandleWorkStall_Concurrency_EightCompletionsCapHolds` | Concurrency | 8 completions for one item (distinct titles) through a barrier, cap 3: nudger write count <= 2, `diagnose_nudge_count <= 2`; `-race` clean |
| STL-10 | AC2 | `TestHandleWorkStall_NotificationContract` | Integration | Nudge success: INFO "nudged" once, no FAILURE; Resupervise: `Suppress`; Respawn after failure: the generic notification body carries the operator phrase for the failure class and no class code (UX G4); golden title/body for nudged, respawned, respawned-after-failed-nudge, `restart_failed` and uncertain |
| STL-11 | AC3 | `TestPlanStall_InputAssembly` | Unit | Both flag readers (S1) read at call instant (flip between two calls); `NudgeEnabled` is their AND; `NudgeAllowed` error -> false (fail closed); `Archived`/`RetryPending` from the snapshot; `ItemInWorkStatus` from the item status; `ResuperviseAllowed` from the budget; `State == ClassifyObserved(outcome.Snapshot)` |
| STL-12 | AC2, C2 | `TestApplyInstanceStop_PerformedOncePerPlanDecision` | Unit | Rewritten by pre-mortem #7 (was "once per return path via a deferred write"). Table over every return path of `handleWorkStall`: exactly one write and one event at plan time; `Nudge`/`Resupervise` use `SetAutonomousStopObserved`, `Respawn`/`None` use `SetAutonomousStopped`; a Nudge that falls back to Respawn adds exactly one second write and event; the event payload equals the post-write snapshot; a fallback write is skipped when a driver is registered, so a successor's turn counters are never zeroed |
| STL-13 | C2 | `TestHandleWorkStall_MaxTurns_NeverNudgesOrResupervises` | Integration | `max_turns` with `idle_prompt` and with `working`, flag ON, headroom: Respawn, nudger 0, restarter 0, rework cap and `RemediationDue` still consulted |
| STL-14 | AC3 | `TestHandleWorkStall_MarkStuckAfterPlan_ResuperviseWritesNoRow` | Integration | `resupervise` plan: no `autonomous_stuck` row created (planStall runs before `MarkStuck`) |
| STL-15 | AC2, C2 | `TestOnAutonomousDriverComplete_PanicInRecovery_ClearsModeNoReplay` | Integration | Pre-mortem #5. A fake nudger that panics: the callback's wrapper recovers, `AutonomousMode` is cleared (no ghost), one FAILURE notification, `UpdateItemSessionEnded` not replayed (called at most as many times as before the panic), `release` ran, guard re-marked short |
| STL-16 | C3, C2 | `TestHandleWorkStall_RecoveryCtxCancelled_BookkeepingStillPersists` | Integration | Pre-mortem #6. Recovery ctx cancelled (from inside the fake nudger) then fallback Respawn: `MarkStuck`, `UpdateItemSessionEnded` and `RemediationDue` run, and the fake storage records `ctx.Err() == nil` on each; the nudger and `canRestart` saw the cancelled ctx |
| STL-17 | AC3, C2 | `TestHandleWorkStall_MarkStuckError_FailsOpenToRespawn` | Integration | Pre-mortem #4. Storage fake returns `(false, err)` from `MarkStuck` for a work stall that planned Nudge: Warn logged, original Respawn body runs (ItemSession ended, respawn dispatched), never `None`, nudger 0; and `(false, nil)` -> `None` with no respawn (contrast row) |
| STL-18 | AC3, C2 | `TestHandleWorkStall_ItemNotInProgress_NoNudgeNoResupervise` | Integration | Pre-mortem #3. Item `review`, state `idle_prompt` and again `working`, flags on, headroom: nudger 0, restarter 0, no instance write of `Observed`, today's Respawn body (its `AutoRespawnAutonomousWork` re-check no-ops), log `action=respawn` |
| STL-19 | C1, R6 | `TestFeatureFlag_AutonomousStallNudge_RegisteredDefaultOffLiveSettable` | Unit | Pre-mortem P1 #2. Registered in `feature_flag_service.go`; default asserted as the **literal** `false` (flipping the shipped default requires editing this test); live set changes the next `planStall`; `diagnose_nudge_enabled`'s description names the in-process nudge |
| STL-20 | AC2, UX G1 | `TestHandleWorkStall_RowActionAnnotated` | Integration | Real `Storage`: Nudge success -> row `action=nudging` at plan time then `action=nudged nudge=1/2`, row still open, `diagnose_nudge_count` 1 (annotated, not resolved); Nudge -> fallback Respawn -> `action=respawned` plus the failure phrase; delivery uncertain -> `action=none`; Respawn/None plans -> one write with the final action; a storage fake failing the refresh: Warn, recovery outcome unchanged |
| STL-21 | AC2, UX G4 | `TestHandleWorkStall_NoClassCodeInOperatorText` | Integration | Each nudger error class driven through `fallbackAfterNudge`: the published notification title/body and the row context summary contain none of `not_idle`, `lease_busy`, `ownership`, `slot_unavailable`, `uncertain`, `restart_failed`, `ctx_cancelled`; the log record's `nudge_outcome` equals the class |

### L. Re-supervise (Tasks 3.2.4a/b; `autonomous_stall_recovery_resupervise_test.go`)

| ID | Req | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|------|-----------------------|
| RSP-1 | P1, AC3 | `TestResupervise_StartupTimeoutWorking_ReregistersDriver` | Integration | Work session, `startup_timeout`, state `working`: driver registered again with 5 min warm-up; `AutonomousMode==true` and the approval gate (`SetAutonomousChecker`) still returns true; no stuck row; `EndedAt==nil`; nudger 0, respawner 0; no notification; log has `action=resupervise`, `resupervise=1/3`, `prior_stop_reason=startup_timeout` |
| RSP-2 | P1, C2 | `TestResupervise_BudgetExhausted_FourthIsRespawn` | Integration | 4 identical stops for one `ItemSession`: 4th is Respawn (ended, respawner called, stuck row context contains `resupervise_cap_reached runs=3 last_stop_reason=startup_timeout`) |
| RSP-3 | P1 | `TestResupervise_StartFails_NoIncrementRespawnBody` | Integration | `ErrNoDriverPool` and `Start` error: budget not incremented, Respawn body runs, mode cleared |
| RSP-4 | P1, C3 | `TestResupervise_SuccessorRegistered_NoSecondDriver` | Integration | Toggle off/on registered D2 during the window: `ErrDriverPresent`, no second driver, D2 stays |
| RSP-5 | P1 | `TestResupervise_AfterSuccessfulNudge_ConsumesResuperviseNotNudgeSlots` | Integration | Nudge then `startup_timeout`/`working`: re-supervised; `diagnose_nudge_count` unchanged by the second stop, budget `1/3` |
| RSP-6 | P1 | `TestResupervise_ReasonByStateTable` | Unit | Reason x `working`: non-`max_turns`, non-`panic` stalls re-supervise; `max_turns` and `panic` Respawn; budget exhausted Respawn (log `resupervise_cap_hit=true`); item not `in_progress` Respawn; unlinked never re-supervised |
| RSP-7 | P1 | `TestResuperviseBudget_KeyedAndDeleted` | Unit | Keyed by `ItemSession.ID`; entry deleted on `done` and wherever `UpdateItemSessionEnded` ends the work session; a new session starts at 0; `Increment`/`Allowed` race-free under `-race` |
| RSP-8 | P1, AC2 | `TestResupervise_ChainLoggedOnEveryStop` | Integration | Three consecutive stops log `resupervise=1/3`, `2/3`, `3/3` |

### M. Stop-during-recovery and stale deferral (Tasks 3.2.5a-c)

| ID | Req | Test File | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|-----------|------|-----------------------|
| STP-1 | C3, P5 | `autonomous_stall_recovery_stop_test.go` | `TestStopDuringNudge_CancelsNudgerNoDriverModeNotRestored` | Integration | `fakeStallNudger` blocks on a channel until its ctx is done; test calls `StopDriverForSession(title)`: nudger ctx cancelled and returns, no driver registered afterwards, restarter 0, `AutonomousMode` not set back to true. Variant via `stopAndDeregisterDriver` directly |
| STP-2 | C3 | same | `TestStopAfterClaimBeforeItemLookup_NudgerNeverWrites` | Integration | `blockingStorage` blocks in `GetItemSessionBySessionUUID`; stop arrives; release: nudger never invoked, or invoked with an already-cancelled ctx and writes nothing |
| STP-3 | C3, C2 | same | `TestCanRestart_FailsWhenPausedArchivedOrNonActive` | Unit | Table mutating the instance while the nudger is blocked: paused, archived, `Hibernated`, `Stopped`, controller removed, successor registered, mode cleared by toggle-off: `canRestart` errors, no driver started |
| STP-4 | C3 | same | `TestToggleOffThenOn_DuringWindow_SuccessorStays` | Integration | D2 registered mid-recovery: recovery aborts, no extra driver, D2 stays registered |
| STP-5 | C3 | same | `TestCancelledRecovery_ModeRule` | Integration | Cancelled ctx with snapshot mode still true and no driver: `SetAutonomousStopped(cancelled_external)`; during shutdown follows the readoption flag exactly as Task 3.1.3b |
| DEF-1 | P3 | `stall_recovery_guard_test.go` | `TestStallRecoveryGuard_MarkDeferredAndExpires` | Unit | Fake `now`: `Mark(item, 30m)`, deferred; advance 30 m + 1 s: not deferred; independent per item; entries lapse by time only; a second `Mark(item, 2m)` replaces (shortens) the window (pre-mortem #12: stamp is short on every exit with no registered driver) |
| DEF-2 | P3 | `session/backlog_lifecycle_stale_deferral_test.go` | `TestRemediateStaleWorkWithBackoffGate_DeferredBeforeRemediationDue` | Integration | Real `Storage`, item `in_progress`, work session `LastProgressAt` 3 h old, guard stamped: `RemediationDue` not called (open `stale_work` row `remediation_attempts` unchanged), remediator not dispatched; window expired (fake clock) -> proceeds as today |
| DEF-3 | P3 | `backlog_service_stall_deferral_test.go` | `TestRemediateStaleWorkSession_Deferred_NoEndNoKillNoRespawn` | Integration | Guard stamped, direct call: returns nil; `UpdateItemSessionEnded`, `KillTmuxPaneOnly`, `AutoRespawnAutonomousWork` all 0 |
| DEF-4 | P3 | same | `TestRemediateStaleWorkSession_Deferred_ActiveNilStillRespawned` | Integration | Stale item with no active session while stamped: still respawned (safety net not deferred; adversarial M1) |
| DEF-5 | P3 | `backlog_service_stall_deferral_test.go` | `TestWiredStaleWorkRemediator_SatisfiesStaleWorkDeferrer` | Unit | The value passed to `SetStaleWorkRemediator` at the wiring site satisfies `session.StaleWorkDeferrer` (type assertion `ok == true`); a wrapper would make the guard silently inert (architecture review "Specific checks"); the wiring site logs Error when it does not (pre-mortem #12) |
| DEF-6 | P3, C2 | existing stale-detector tests | (existing, unchanged) | Integration | Pass unchanged when no guard is stamped and when the remediator does not implement `StaleWorkDeferrer` |

### O. Web-app Why value (Task 3.1.4d; CSS only, no TSX change)

| ID | Req | Test File | Test Name | Type | Scenario -> assertion |
|----|-----|-----------|-----------|------|-----------------------|
| UI-1 | AC2, UX G6 | `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx` | `renders a plain-language lead sentence followed by a long unbroken token` | Unit | Context `<sentence> \| stop_reason=… tail=<300 non-space chars>`: both visible in `data-testid="stuck-item-why"`; the data path is pinned (jsdom does not lay out, so the wrap itself is UI-2) |
| UI-2 | AC2, UX G6 | `tests/e2e/backlog-stuck-items.spec.ts` | `Why value with a long unbroken token does not overflow at 360px` | E2E | `seedStuckItem({context: <300 unbroken chars>})`, viewport 360 x 740, open the card: `document.documentElement.scrollWidth <= window.innerWidth`; `data-testid` locators, no `waitForTimeout`, file already annotated `// @feature backlog:list-stuck` |

### N. Documentation (AC1, R1) and gates

| ID | Req | Check | Type | Pass condition |
|----|-----|-------|------|----------------|
| DOC-1 | AC1, R1 | `docs/explanation/autonomous-stall-causes.md` exists, is Diataxis *explanation*, and `CLAUDE.md` Reference Documents Index has one row for it | Doc | `test -f` and `git grep -n 'autonomous-stall-causes' CLAUDE.md` returns one table row |
| DOC-2 | AC1, R1 | Every driver exit site has a row | Doc | `git grep -c 'fireCompletion(' -- session/autonomous_driver.go` at the branch HEAD: the doc has a row per call site (3 at base, plus the definition match not a row) **and** a row for the panic path (no call) **and** D1-D11 |
| DOC-3 | AC1, R1 | Every non-driver path has a row | Doc | Rows exist for S1-S4, O1-O7, O9 and the 5 external `Stop()` callers (StopSession, DeleteSession, toggle-off, `submit_review_verdict`, shutdown `lifecycleCtx` cancel), each with a verdict in `{real stall, mislabelled, not a stall}` |
| DOC-4 | AC1 | Every citation resolves | Doc | Citation protocol below: exits 0 and prints one `OK` per citation |
| DOC-5 | AC1 | Evidence labels and census | Doc | Each cause labelled VERIFIED or INFERRED; the doc states local logs held only `test-*` session evidence; census command and output pasted; production frequency stated as unknown, no ranking by count |
| DOC-6 | AC1, R2 | Taxonomy matches code | Doc | After Task 4.1.1a: every `StopReason` constant name appears in the doc mapped to its driver site by symbol name; the decision table rows equal ADR-001 rows 1-8 and `DOM-4` case names; known-gaps section lists the 6 gaps named in Task 1.2.1b |
| DOC-7 | AC1 | Doc drift guard (recommended, optional) | Unit | `session/domain` test reads `docs/explanation/autonomous-stall-causes.md` and asserts every `StopReason` string appears in it. Cheap, deterministic; not in the plan (see "Plan deltas") |
| GATE-1 | C1 | No wire/schema change | Guard | `make registry-diff` empty; `git diff --stat origin/main -- proto session/ent/schema` empty; `git diff --stat origin/main -- web-app` lists only `StuckItemDetail.css.ts` and `StuckItemDetail.test.tsx` (Task 3.1.4d); `make proto-breaking` clean |
| GATE-2 | C2 | Custom lints incl. `noarchivedrevival`, `silenttransition`, `notimesleeptest` | Guard | `make lint-custom` exits 0 |
| GATE-3 | C2, INS-5 | Actor-field writes | Guard | `make actor-field-guard` exits 0 (no raw `AutonomousStopReason` write) |
| GATE-4 | R6 | Complexity/duplication gate | Guard | `make ready-complexity-gate` exits 0 (`run()` and `onAutonomousDriverComplete` not flagged; if `run()` is, extract per Task 2.1.1b) |
| GATE-5 | R6 | Unit + race | Guard | `go test ./session/domain/... ./session/... ./server/services/... ./server/mcp/... -race -timeout=20m` exits 0 |
| GATE-6 | P2 | Feature-flag registry | Guard | Feature-flag service tests cover `autonomous_driver_readoption`, `autonomous_stall_nudge_enabled` (STL-19) and the updated `diagnose_nudge_enabled` description |
| GATE-7 | R6 | Full pipeline | Guard | `make ci` (build, test, test-race, lint, test-delivery-guards, actor-field-guard, registry-generate) exits 0; `make ready` before push |

## Plan's own acceptance cases -> test

| Plan acceptance case (Story) | Test IDs |
|------------------------------|----------|
| Stop during nudge (3.2.5): blocked nudger cancelled, no driver, restarter 0, mode not restored | STP-1; pre-lookup variant STP-2; pause/archive/non-Active STP-3; successor STP-4 |
| Per-role cancelled guards, every role incl. unlinked (3.1.3) | SVC-4 (20 rows), SVC-5, SVC-6, SVC-9 |
| Boot re-adoption 12-gate table (3.1.3) incl. skip-without-clear and transient-probe-error | RDP-1 (28 rows), RDP-2, RDP-3, RDP-4, RDP-5 |
| Persisted round trip / `Hibernated` (3.1.3) | RDP-11, SVC-7 |
| Trigger ordering and fallback timer (3.1.3) | RDP-12, RDP-13, RDP-14, RDP-15 |
| Per-boot cap, stable order, deferral (3.1.3) | RDP-8 |
| Flag OFF dry-run and shutdown branch (3.1.3, Risk Control) | RDP-9, SVC-8 |
| Stale-remediator deferral before `RemediationDue` (3.2.5) | DEF-2, DEF-3, DEF-4, DEF-5 |
| Re-supervise cap, 4th is Respawn (3.2.4) | RSP-2 (with RSP-3, RSP-4, RSP-5) |
| Claim cap race: 8 goroutines -> exactly 2 (3.2.1) | CLM-1; service-level STL-9 |
| Canary-secret redaction (1.1.2, 3.1.4) | TAIL-4, TAIL-5, OBS-1, OBS-6 |
| Nudge text byte-equality (3.2.2) | NDG-9 |
| Cancel mid-settle returns promptly with zero writes (3.2.2) | NDG-5, NDG-6, NDG-10 |
| Submit uncertainty, no refund (3.2.2, 3.2.3) | NDG-7, NDG-8, STL-3, STL-4 |
| Pin test handling via `make test-delivery-guards` (3.2.2b, 2.1.1b) | NDG-13 and the protocol below |
| Driver exits each get a typed reason, panic fires once (2.1.1) | DRV-1, DRV-2, DRV-3, DRV-5 |
| `ClassifyObserved` / `DecideStallAction` per state (1.1.1) | DOM-3, DOM-4, DOM-5 |
| Notification contract (3.2.3) | STL-10, OBS-5 |
| Registry never self-deadlocks or calls a seam under `a.mu` (3.1.3, pre-mortem P1 #1) | REG-10, REG-12, REG-13, REG-14, REG-15 |
| Dead registered driver does not block a restart; panic in the callback leaves no ghost (3.1.3, pre-mortem #5) | REG-16, DRV-10, STL-15 |
| In-process nudge dark by default, live config's `diagnose_nudge_enabled=true` alone does not nudge (3.2.3, pre-mortem P1 #2) | STL-7, STL-11, STL-19, DOM-6 |
| Nudge / Resupervise never fire for an item already out of `in_progress`; `panic` never re-supervised (3.2.3, pre-mortem #3, #13) | DOM-4, DOM-5, STL-18, RSP-6 |
| `MarkStuck` error fails open to Respawn (3.2.3, pre-mortem #4) | STL-17, STL-7 |
| Recovery ctx never reaches bookkeeping; write and event once per plan (3.1.2, 3.2.3, pre-mortem #6, #7) | STL-16, STL-12, SVC-11, SVC-12 |
| AC2 operator read path returns the detail (3.1.4c) | OBS-7 |

## Pin guard protocol (`make test-delivery-guards`)

The `sinkguard`-tagged tests are not part of `go test ./...` (plan fact table; `Makefile:718`), and they need generated `gen/` and `session/ent/*` (this worktree lacks them: architecture review header), so run `make build` first.

1. **Positive**: after Task 3.2.2b and again after Task 3.2.3a, `make test-delivery-guards` exits 0 with these rows present: `session:NudgeStalledAutonomousSession` (`kindAcquirer`), `server/services:defaultStallNudger.NudgeStalledSession` (`kindChain`), `"NudgeStalledAutonomousSession"` in `realCallerConfig().surface`, and the closed-acquirer test (T-RO-45) updated.
2. **Negative (once, recorded in the PR)**: delete the `kindChain` row, rerun, expect a failure naming `defaultStallNudger.NudgeStalledSession`; restore. Delete the `kindAcquirer` row, expect the closed-acquirer test to fail; restore. This is what "fails without them" means; it is not a committed test.
3. **Order**: the `kindChain` row lands in the same commit as the adapter (Task 3.2.3a); a surface entry without a caller is fine, an unpinned caller fails the guard.
4. **If `run()` moves its lease/submit block** (Task 2.1.1b): rename the pin row `session:AutonomousDriver.run` to `session:AutonomousDriver.injectTurn` in the same commit; rerun the guard before pushing.
5. **Closure rule**: `defaultStallNudger.NudgeStalledSession` and the `claim` callback contain no closure that calls a watched primitive (the scanner keys units by function name); the guard failing on `...funcN` is the symptom.

## AC1 verification protocol (documentation deliverable)

AC1 has no Go test: the deliverable is a document, so it is verified by existence, completeness counts and citation resolution, with the root-cause *quality* left to a human reviewer.

**Document format requirement** (so the checks are mechanical): each stop-path table row has `id` (D1.., S1.., O1.., X1..), `path:line` rendered as `[path:line](https://github.com/tstapler/stapler-squad/blob/<sha>/<path>#L<n>)`, a `symbol` column naming the function or statement, and a `verdict`.

**Citation resolution** (run in Task 4.1.1a and again before ship; paste output in the PR):

```bash
doc=docs/explanation/autonomous-stall-causes.md
grep -oE 'blob/[0-9a-f]{40}/[^#) ]+#L[0-9]+' "$doc" | sort -u | while IFS= read -r c; do
  sha=${c#blob/}; sha=${sha%%/*}; rest=${c#blob/$sha/}; path=${rest%%#*}; line=${rest##*#L}
  [ "$(git cat-file -t "$sha" 2>/dev/null)" = commit ] || { echo "BAD sha (not a commit; blob sha?): $c"; continue; }
  git show "$sha:$path" 2>/dev/null | sed -n "${line}p" | grep -q . && echo "OK $c" || echo "MISSING line: $c"
done
```

Then, for each row, `git show <sha>:<path> | sed -n '<line>p'` must contain the row's `symbol` (checked by eye or by extending the loop once the table is parsed). The `git cat-file -t` check is deliberate: the `contents` API blob SHA yields `blob`, which the loop reports as BAD (CLAUDE.md "Do not use the `.sha` from the `contents` endpoint").

**Checklist** (reviewer ticks each; maps to DOC-1..DOC-6):

- [ ] Doc exists; one `CLAUDE.md` index row (DOC-1).
- [ ] Row per `fireCompletion(` call site + panic path + D1-D11 (DOC-2); row per S1-S4, O1-O7, O9, external caller (DOC-3).
- [ ] Every row has a permalink at a commit SHA, a symbol and a verdict; citation script prints only `OK` (DOC-4).
- [ ] VERIFIED vs INFERRED label per cause; "local logs contain only `test-*` session evidence" stated; census command and its counts (21 / 8 / 11) pasted; no ranking by frequency (DOC-5).
- [ ] Root-cause statement ranked by confidence names the five causes of Task 1.2.1b (mislabel hides cause; recovery kills a resumable or working pane; external stop treated as stall; 60 s startup idle wait vs a long first turn (INFERRED); panic leaves `AutonomousMode` true).
- [ ] Known gaps section lists: stale-work detector kills with no nudge or snapshot (AC3 scope), lifetime nudge cap, pause and user stop never stop the driver, `buildTurnCallback` raw writes, `SessionDriver.attemptBacklogNudge` coexistence with a cited answer, open stuck row after a successful nudge (DOC-6).
- [ ] Post-change taxonomy and decision table match `DecideStallAction` and ADR-001 (DOC-6; DOC-7 if adopted).

## UX Acceptance Tests

No `design/ux.md` exists. The plan adds no component, RPC or marker; its one web-app change is a CSS-only overflow rule on `StuckItemDetail`'s `.value` (Task 3.1.4d, UI-1/UI-2). The user-visible surfaces are existing: the Unfinished tab's stuck-row context (`StuckItemDetail.tsx:102`, now led by a plain-language sentence) and `SessionCard.tsx:877,888`.

| UX Criterion | Test | Tool | Steps |
|--------------|------|------|-------|
| Operator can read why a session stopped (AC2) | `OBS-7` (RPC returns the detail) plus **MAN-1** | Go test + manual | MAN-1: on a manual instance (`PORT=62871`, per `CLAUDE.md` manual-testing recipe), stop a work session's pane output to an idle prompt, let the driver exit, open the Unfinished tab, read the plain-language lead sentence (what stopped, what the session was doing, what was done, whether you must act) and, after ` | `, `stop_reason=... state=idle_prompt action=...` in the item's detail |
| No false "stuck" badge after cancel or boot clear | `INS-1`, `INS-2`, `SVC-4`, `RDP-6` assert outcome `""` | Go test | `SessionCard.tsx:888` only fires on `"stuck"`, so outcome `""` is the proof; no Jest change needed |
| Nudged session continues and is not killed (AC3) | `STL-1` plus **MAN-2** | Go test + manual | MAN-2: flags `diagnose_nudge_enabled` and `autonomous_stall_nudge_enabled` on in the manual instance (the latter ships off), drive a session to an idle prompt, confirm one nudge line appears in the pane, the tmux session survives, and the log shows `action=nudge` |
| Boot re-adoption candidates are legible | `RDP-1`, `RDP-17` plus **MAN-3** | Go test + manual | MAN-3: first boot after deploy, read every `readopt candidate` line once (wall-clock blocker 3) |

## Test Stack

- **Unit**: `go test` with `testify` (`require`/`assert`) as used in `session` and `server/services`; table-driven; `t.Parallel()`.
- **Integration**: in-memory ent `Storage`, fake nudger/starter/probe/respawner/clock; real `ClaudeController` only through `pumpIdleSignals`.
- **Concurrency**: barrier plus `go test -race` (`make test-race`).
- **Guards**: `make lint-custom`, `make actor-field-guard`, `make test-delivery-guards`, `make ready-complexity-gate`, `make registry-diff`.
- **E2E / UX**: one Playwright case (UI-2, 360 px overflow) in the existing `backlog-stuck-items.spec.ts`; three manual checks above.
- **Migration**: N/A (plan "Migration Plan: None"); no `migration_should_be_reversible` test.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|-------|------------------|--------|
| Go (new/changed files) | `go test ./session/domain/... ./session/... ./server/services/... -coverprofile=cover.out && go tool cover -func=cover.out` | >= 80% line on `session/domain/autonomous_stop.go`, `session/autonomous_stop_snapshot.go`, `session/autonomous_stall_nudge.go`, `server/services/autonomous_driver_registry.go`, `autonomous_readoption.go`, `autonomous_stall_recovery.go`, `autonomous_stall_nudge_effect.go`, `autonomous_stall_resupervise.go`, `autonomous_stop_detail.go`, `stall_recovery_guard.go`; 100% of `StopReason` x `ObservedState` decision branches via DOM-5 |
| Race | `make test-race` | clean |
| Web-app | `cd web-app && pnpm exec jest --testPathPatterns StuckItemDetail --no-coverage`; `pnpm run lint:duplicates` | UI-1 passes; jscpd under its threshold (the new test adds no `jest.mock` block) |

- Every public/exported new function has a happy-path and an error-path test (DOM-*, TAIL-*, NDG-*, CLM-*).
- Every external integration (tmux write, DB claim, GitHub-free) has a unit with a fake and at least one integration against real `Storage` (CLM-*, STL-1, DEF-2).
- Every timing-dependent claim is proven with an injected clock or a cancel-from-callback, never a wall-clock sleep.

## Requirement -> Test Traceability Matrix

| Requirement | Automated tests | Manual / doc | Notes |
|-------------|-----------------|--------------|-------|
| **AC1** root cause identified, each stop path cited | DOC-7 (optional drift guard) | DOC-1..DOC-6, citation script, checklist | **Documentation deliverable; root-cause correctness is a human judgement and production frequency is unverifiable (no production logs).** Citation resolution is scripted, content quality is manual |
| **AC2** reason recorded and observable, distinguishes idle/error/exited | DOM-1, DOM-3, DRV-1, DRV-2, DRV-3, INS-1..INS-4, INS-6, SVC-1, OBS-1..OBS-10, STL-20, STL-21, UI-1, UI-2 | MAN-1 | Log (OBS-2), instance field (INS-4), stuck-row context via RPC (OBS-7) |
| **AC3** state inspected and captured before restart; nudge over respawn when waiting | DOM-4, DOM-5, DRV-4..DRV-6, DRV-10, TAIL-1..TAIL-3, CLM-1..CLM-7, NDG-1..NDG-12, STL-1..STL-19, RSP-1..RSP-8, STP-1..STP-5, DEF-1..DEF-5 | MAN-2; owner sign-off on "AC3 scope" | **Scope is driver-stop paths only** (plan Deferred "AC3 scope"). Stale-detector and health-checker restarts have no snapshot or nudge by design; DEF-4 and DEF-6 pin that they are unchanged. Owner sign-off is not testable |
| **AC4** tests cover classification and nudge-vs-respawn per state | DOM-3, DOM-4, DOM-5, STL-2, STL-7, STL-13, RSP-6 | none | Meta-requirement: satisfied by the existence and exhaustiveness of these tests; GATE-5 runs them |
| **R1** diagnosis document with cited paths | none | DOC-1..DOC-6 | Same as AC1 |
| **R2** typed reason; logged, persisted on row, surfaced on instance | DOM-1, DOM-2, DRV-1, DRV-7, INS-1, OBS-1, OBS-2, OBS-4, OBS-7, OBS-9 | none | Three sinks each have a test: log OBS-2, row OBS-1/OBS-7, instance INS-4 |
| **R3** observed state independent of why the driver stopped | DOM-3, DRV-8, OBS-3 | none | |
| **R4** last activity and output tail captured and recorded before respawn | DRV-4, DRV-5, DRV-6, TAIL-1..TAIL-6, OBS-1, OBS-6, SVC-2 | none | Ordering before `UpdateItemSessionEnded` pinned by SVC-2 and STL-5 |
| **R5** pure decision; waiting session nudged; bounded by nudge cap | DOM-4, DOM-5, DOM-6, CLM-1..CLM-4, CLM-6, NDG-1..NDG-4, NDG-9, STL-1, STL-7, STL-9 | none | |
| **R6** table-driven tests for classification and decision for every state | DOM-3, DOM-5, STL-2 | none | Same as AC4 |
| **C1** no new proto/ent/web-app surface | SVC-1, INS-1 (outcome set closed) | GATE-1 (command, not a Go test) | Guard-only for the "no diff" half. The CSS-only change in Task 3.1.4d adds no component, RPC, field or marker; GATE-1 names the two allowed web-app files |
| **C2** archival, rework cap, remediation backoff, nudge cap not weakened | DOM-5, DOM-6, STL-5, STL-7, STL-8, STL-13, STL-15..STL-18, CLM-1..CLM-3, CLM-5, CLM-7, SVC-2, SVC-3, SVC-11, SVC-12, NDG-4, NDG-11, DEF-6 | GATE-2 (`noarchivedrevival`), GATE-3 | |
| **C3** external stop is not a stall and triggers no respawn | DRV-1 (`cancelled_external`), DRV-9, SVC-4..SVC-9, REG-4, STP-1..STP-5, STL-3 | none | |
| **P1** re-supervise a working session, bounded | DOM-4, RSP-1..RSP-8, STL-6, STL-13, STL-14, STL-18 | none | |
| **P2** boot re-adoption under gates | SVC-7, SVC-8, RDP-1..RDP-18 | MAN-3 | |
| **P3** stale-remediator deferral | DEF-1..DEF-6 | none | |
| **P4** secret-safe tail | TAIL-1..TAIL-8, OBS-1, OBS-6 | none | Canary per family, 9 families, plus the env-style and headerless-key-body residuals fail closed (TAIL-7) with the free-prose residual pinned as accepted |
| **P5** one pinned, guarded, cancellable write surface | NDG-1..NDG-12, REG-1..REG-16, STP-1..STP-3 | NDG-13 (guard, run manually and in CI) | |

**Coverage**: 13/13 requirements from `requirements.md` (AC1-AC4, R1-R6, C1-C3) map to at least one concrete check; 5/5 plan-derived requirements (P1-P5) likewise. 16 of the 18 have at least one non-optional automated Go test; AC1 and R1 are verified by the document protocol only (scripted citation check plus reviewer checklist); see call-outs.

### Requirements with manual-only or partly-manual verification (call-outs)

1. **AC1 / R1 (diagnosis document)**: no Go test can assert that the root-cause *statement* is right. Mechanical parts (existence, row counts, SHA-pinned citations resolving, taxonomy listed) are scripted (DOC-2..DOC-4, optional DOC-7); causal ranking and verdicts are review-only. Production frequency is **unverifiable**: local logs hold only `test-*` sessions (plan fact row 47), so causes are VERIFIED-by-code, not by incident.
2. **AC3 scope sign-off**: the owner has not accepted that AC3 covers driver-stop paths only. That is a decision, not a test. Until signed, AC3 must be reported as "met for driver-stop paths".
3. **C1 "no new surface"**: proven by a diff/registry command (GATE-1), not a Go test.
4. **First-boot re-adoption (P2) against real data**: unit and integration tests prove the gates; whether the persisted `AutonomousMode == true` population is what the plan assumes is INFERRED-unknown (architecture review C5) and is checked only by reading the first boot's `readopt candidate` lines (MAN-3).
5. **Real tmux nudge delivery**: no automated test drives a real Claude pane; `StatusIdle`-detection against real Claude output and the 3 s settle are exercised only by MAN-2. The automated tests use a fake controller by design (deterministic-fast-tests: virtualize the slow dependency).

## Plan deltas this validation requires

None adds a task; each is a line inside an existing task. **Status (2026-10-13): all of 1-5 below are now written into `plan.md` as ACs of the named tasks**, and the pre-mortem added the rows noted after the list.

1. **S1-S6** seams above (injectable flag readers, `captureTimeout`, `nudgeStalledWith`, extracted Step 7.6 goroutine, rate-limit cap field): plan Tasks 3.2.3a (S1, three readers), 2.1.1a (S2), 2.1.1c (S3), 3.2.2a (S4), 3.1.3c (S5), 3.2.1a (S6).
2. **Task 1.2.1b**: `symbol` column and SHA-pinned permalinks on every stop-path row (so DOC-4 is mechanical); "stuck row stays open after a successful nudge" cosmetic note in known gaps. Written into the plan.
3. **Task 3.1.4c**: OBS-7 (`ListStuckBacklogItems` returns the detail). Written into the plan.
4. **Task 3.2.2a**: NDG-12 (MCP handler with a cancelled request ctx), file `server/mcp/tools_diagnose_test.go`. Written into the plan.
5. **Task 3.2.3d**: STL-8 (archived), STL-12 (rewritten: once per plan decision), STL-14 (resupervise writes no row). Written into the plan.
6. **Task 4.1.1a**: optionally add DOC-7.
7. **Pre-mortem additions (new rows)**: DRV-10, SVC-11, SVC-12, REG-12..REG-16, STL-15..STL-19; changed rows DOM-4, DOM-5 (2^6 booleans), DOM-6, REG-10, RDP-16, OBS-2, STL-1, STL-7, STL-11, STL-12, RSP-6, DEF-1, DEF-5; new seams S7, S8; new fakes `lockProbeSeams`, `requireReturns`.
8. **Triad repair (iteration 5)**: new rows TAIL-7, TAIL-8, INS-6, OBS-10, STL-20, STL-21, UI-1, UI-2; changed rows SVC-1, SVC-12, OBS-1, OBS-2, OBS-4, OBS-6, STL-5, STL-10, GATE-1; new seam S9; BUG-079 baseline run is a Task 2.1.1c step, not a row; Task ids 3.1.3e, 3.1.3f, 3.1.4d added to the plan (38 tasks).

## Summary counts

Counts are derived from the ID and Type columns above (command below), not hand-tallied. A row is one test function, one table-driven test with several cases, one existing suite, or one gate command.

| Kind | Rows | IDs |
|------|------|-----|
| Unit | 80 | includes DOC-7 (optional drift guard) and UI-1 (Jest) |
| Integration | 60 | real in-memory `Storage` and/or several real components |
| Concurrency (`-race`, barrier) | 7 | DRV-9, INS-5, REG-7, REG-10, REG-15, CLM-1, STL-9 |
| Guard (command gate) | 8 | GATE-1..GATE-7, NDG-13 |
| Doc (AC1 protocol) | 6 | DOC-1..DOC-6 |
| E2E (Playwright) | 1 | UI-2 |
| **Total check rows** | **162** | 146 Go test rows (79 + 60 + 7, of which 3 are existing suites run unchanged: SVC-3, CLM-5, DEF-6) + 1 Jest (UI-1) + 1 Playwright (UI-2) + 8 guards + 6 doc checks |

- Rows per area: DOM 6, TAIL 8, DRV 10, INS 6, SVC 12, REG 16, RDP 18, OBS 10, CLM 7, NDG 13, STL 21, RSP 8, STP 5, DEF 6, DOC 7, GATE 7, UI 2 (sum 162).
- Table cases inside rows that the plan named explicitly: readoption gates 28 (RDP-1), cancelled-class guards 20 (SVC-4), failure-mapping rows 10 (STL-2), fallback rows 9 (STL-7), secret canary families 9 (TAIL-4) plus 3 fail-closed residual cases (TAIL-7), exhaustive decision combinations 5,632 at base (11 x 8 x 64) (DOM-5, derived from the enum slices at runtime).
- UX acceptance tests: 2 automated (UI-1 Jest, UI-2 Playwright, CSS overflow only); 3 manual checks (MAN-1, MAN-2, MAN-3).
- Migration test: N/A.
- Requirement coverage: 13/13 source requirements, 5/5 plan-derived; 2 are verified by document protocol only (AC1, R1); 1 has a command-only half (C1).

```bash
f=project_plans/autonomous-stall-diagnosis/implementation/validation.md
grep -E '^\| (DOM|TAIL|DRV|INS|SVC|REG|RDP|OBS|CLM|NDG|STL|RSP|STP|DEF|DOC|GATE|UI)-[0-9]+ ' "$f" | wc -l          # 162
grep -E '^\| (DOM|TAIL|DRV|INS|SVC|REG|RDP|OBS|CLM|NDG|STL|RSP|STP|DEF|DOC|GATE|UI)-[0-9]+ ' "$f" | grep -c '| Unit |'   # 80
```
