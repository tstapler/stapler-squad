# Validation: Switching Session Programs Does Not Save Correctly

> **Superseded 2026-07-01.** Replaces the 2026-06-30 version, which mapped tests to a fix that has
> since shipped (commit `914138ec`). This revision maps tests to the **residual gaps** (see
> `plan.md`), since the core save-and-persist behavior is already correct on `main` but untested.

## Acceptance Criteria → Test Mapping

### AC1: Program change persists correctly for every session state (regression coverage for the shipped fix — currently untested)

| Test | Type | Covers |
|------|------|--------|
| `TestUpdateSession_ProgramUpdate_Active_RestartsAndPersists` | Go unit | Active session: `SaveInstances` called before `Restart`; DB has new program even if `Restart` errors |
| `TestUpdateSession_ProgramUpdate_Stopped_SavesNoRestart` | Go unit | Non-Active session: program persisted, `Restart` never invoked |
| `TestUpdateSession_ProgramUpdate_EmptyString_ResolvesToConfigDefault` | Go unit | `program: ""` resolves to `config.LoadConfig().DefaultProgram` before persistence, not dropped and not stored as `""` |
| `TestUpdateSession_ProgramUpdate_SameValue_NoOp` | Go unit | No restart/save-of-new-value churn when requested program equals current |
| `TestUpdateSession_ProgramUpdate_RestartError_DBStillConsistent` | Go unit | Restart failure returns `CodeInternal` but DB already has the new program (pre-save ordering) |

### AC2: Program change is discoverable and usable from the overflow menu (regression coverage for shipped UI)

| Test | Type | Covers |
|------|------|--------|
| `SessionActionsOverflow_should_showChangeProgramItem_When_menuOpen` | Jest/RTL | Menu item renders |
| `SessionActionsOverflow_should_prefillCurrentProgram_When_pickerOpens` | Jest/RTL | Dialog pre-selects `session.program` |
| `SessionActionsOverflow_should_callOnChangeProgram_When_saved` | Jest/RTL | Save wires to `onChangeProgram(sessionId, value)` |
| `SessionActionsOverflow_should_sendEmptyString_When_systemDefaultSelected` | Jest/RTL | "System default" option saves `""`, not omitted |
| `SessionActionsOverflow_should_showRestartHint_When_sessionActive` | Jest/RTL | Restart warning text only renders for `SessionStatus.ACTIVE` (also regression-guards Task 11's literal→enum fix) |

### AC3: Two independent program-switch code paths stay consistent (new — closes pitfalls gap)

| Test | Type | Covers |
|------|------|--------|
| `TestUpdateSessionProgram_EmptyString_ResolvesToConfigDefault` | Go unit | Capacity-monitor path gets the same empty→default guard as the RPC path (currently missing) |
| `TestProgramSwitch_ConcurrentManualAndAutoFallback_NoDoubleRestart` | Go unit (race) | Both paths firing near-simultaneously on the same instance serialize instead of double-restarting/double-porting history; run under `go test -race` per `make ci`'s `test-race` target |

### AC4: Claude conversation UUID does not leak across a program-family round trip (new — closes pitfalls gap)

| Test | Type | Covers |
|------|------|--------|
| `TestUpdateSession_ProgramSwitch_ClearsClaudeUUID_LeavingFamily` | Go unit | Switching claude → aider clears `claudeSession.ConversationUUID` / `HistoryFilePath` |
| `TestUpdateSession_ProgramSwitch_PreservesUUID_WithinClaudeAntigravityFamily` | Go unit | Switching claude ↔ antigravity still ports history via `PortSessionHistory`, UUID not blanked |
| `TestUpdateSession_ProgramSwitch_RoundTrip_NoStaleResume` | Go unit (regression) | claude → aider → claude does not pass a stale `--resume <uuid>` on the second switch |

### AC5: Active-session program change requires explicit confirmation (new — UX gap)

| Test | Type | Covers |
|------|------|--------|
| `SessionActionsOverflow_should_showConfirmDialog_When_savingProgramOnActiveSession` | Jest/RTL | Two-step confirm renders instead of immediate save, matching `Restart`/`Delete` pattern |
| `SessionActionsOverflow_should_notCallOnChangeProgram_When_confirmCancelled` | Jest/RTL | Cancel path is a no-op |
| `SessionActionsOverflow_should_callOnChangeProgram_When_confirmAccepted` | Jest/RTL | Confirm path proceeds |

### AC6: Non-Active sessions surface a "pending on resume" state after a program change (new — UX gap)

| Test | Type | Covers |
|------|------|--------|
| `SessionCard_should_showPendingProgramBadge_When_stoppedSessionProgramChangedSinceLastLaunch` | Jest/RTL | Badge renders when `session.program` differs from the program embedded in `session.launchCommand` |

### AC7: Overflow-menu program picker doesn't clobber concurrent server-side changes (new — closes pitfalls gap)

| Test | Type | Covers |
|------|------|--------|
| `SessionActionsOverflow_should_resyncPickerValue_When_sessionProgramChangesWhileDialogOpen` | Jest/RTL | Mirrors `SessionDetailView`'s existing re-sync `useEffect`; open dialog reflects an external `WatchSessions` push instead of saving stale local state |

### AC8: End-to-end flow works through the real UI

| Test | Type | Covers |
|------|------|--------|
| `tests/e2e/session-program-change.spec.ts` | Playwright e2e | Overflow menu → change program → save → session list/detail view reflect new program; asserts via `data-testid`/ARIA locators per `.claude/rules/e2e-test-conventions.md`, no `waitForTimeout` |

---

## Edge Cases and Error Scenarios

### Edge Case 1: `program` not in the available-programs list
- Program string is stale (PATH changed, or a custom/typo'd value) after session creation.
- **Expected:** backend stores the string as-is (no content validation); session fails to start with a
  surfaced error on next launch — existing behavior, no change required.
- **Test:** `TestUpdateSession_ProgramNotInAvailableList_SavesAnyway`

### Edge Case 2: `instance.started == false` (fallback load path)
- Poller unavailable; instance loaded via `loadInstancesWithWiring()` without `started = true`.
- **Expected:** `Restart()` returns `ErrCannotRestart`; `UpdateSession` propagates `CodeInternal`, but
  since the pre-save already ran, the program metadata is durable and a manual restart afterward picks
  up the new program.
- **Test:** `TestUpdateSession_Active_StartedFalse_RestartErrorsButProgramPersists`

### Edge Case 3: Concurrent manual change + capacity-monitor auto-fallback (see AC3)
- Both paths read the pre-change `Program`, both decide "changed," both call `SetProgram` /
  `PortSessionHistory` / `SaveInstances` / `Restart` independently.
- **Expected (post-Task-4 fix):** second writer blocks or is rejected with a clear "program change
  already in progress" error rather than double-restarting the tmux session.
- **Test:** `TestProgramSwitch_ConcurrentManualAndAutoFallback_NoDoubleRestart` (see AC3); run with
  `go test -race`.

### Edge Case 4: Restart on a paused worktree session
- Restarting a paused worktree session recreates the worktree as a side effect unrelated to the program
  field, and can independently affect Claude conversation UUID state.
- **Expected:** documented as existing, unrelated behavior — not in scope for this backlog item; note
  in the PR description so it isn't conflated with AC4's targeted UUID-clearing fix.
- **Test:** none required; documentation-only callout.

### Edge Case 5: Substring-match false positive in history-porting heuristic
- `strings.Contains(program, "claude")` / `"agy"` also matches custom program strings like
  `claude-experimental` or `aider --model agy-local`.
- **Expected:** flagged as a known limitation; out of scope for this backlog item unless it causes a
  concrete user-facing bug report. If addressed, needs an exact-match or prefix-based classifier instead
  of substring containment.
- **Test:** not required for this item; `TestPortSessionHistory_SubstringFalsePositive_KnownLimitation`
  could be added as a documented-limitation regression test if the team decides to harden the heuristic
  later.

---

## Test Infrastructure Notes

- Go tests go in `server/services/session_service_test.go` alongside existing `TestUpdateSession_*`
  tests (tag/title/status coverage already lives there per `research/architecture.md`).
- `TestProgramSwitch_ConcurrentManualAndAutoFallback_NoDoubleRestart` must run under `go test -race`
  (part of `make ci`'s `test-race` target) since it's specifically testing a suspected data race.
- Jest tests for the overflow-menu picker go in a new
  `web-app/src/components/sessions/__tests__/SessionActionsOverflow.test.tsx` (file currently has zero
  program-related coverage per `research/pitfalls.md`).
- Playwright e2e spec: `tests/e2e/session-program-change.spec.ts`, requires the test server running
  (`STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server`),
  must start with `// @feature session:update`, use only `data-testid`/ARIA locators, and avoid
  `waitForTimeout`.
- After adding tests: flip `docs/registry/features/backend/session/update.json` `tested` to `true` with
  real `testIds`; add a new `docs/registry/features/frontend/session-change-program.json` entry (none
  currently exists); run `make registry-generate` and confirm `docs/registry/coverage-gaps.json` shrinks
  rather than grows.
