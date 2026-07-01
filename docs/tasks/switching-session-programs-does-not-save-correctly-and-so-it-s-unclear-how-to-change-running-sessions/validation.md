# Validation: Switching Session Programs Does Not Save Correctly

## Acceptance Criteria → Test Mapping

### AC1: Program change saves correctly for all session states

| Test | Type | Covers |
|------|------|--------|
| `TestUpdateSession_ProgramUpdate_Active_Restarts` | Go unit | Active session: program updated in DB + `Restart()` called |
| `TestUpdateSession_ProgramUpdate_Stopped_SavesNorestart` | Go unit | Stopped session: program updated in DB, no restart |
| `TestUpdateSession_ProgramUpdate_Paused_SavesNoRestart` | Go unit | Paused session: program saved, no restart, used on resume |
| `TestUpdateSession_ProgramUpdate_SameValue_NoOp` | Go unit | No restart when program unchanged |
| `TestUpdateSession_ProgramUpdate_RestartError_RollsBack` | Go unit | Restart failure: verify DB not left in inconsistent state |

### AC2: "System default" (empty string) clears the program

| Test | Type | Covers |
|------|------|--------|
| `TestUpdateSession_ProgramUpdate_EmptyString_ClearsToDefault` | Go unit | Empty string → persists resolved config default, not blocked by guard |
| `TestUpdateSession_ProgramUpdate_EmptyString_WasBlockedByGuard` | Go unit (regression) | Guard regression: verify `*req.Msg.Program != ""` is gone |

### AC3: React state syncs from WatchSessions after external update

| Test | Type | Covers |
|------|------|--------|
| `program edit shows updated value after session stream update` | Jest/RTL | `useEffect` re-syncs `programValue` when `session.program` changes and `!isEditingProgram` |
| `program edit form does not re-sync while editing is active` | Jest/RTL | Re-sync guard: `isEditingProgram == true` blocks overwrite |

### AC4: Program change is discoverable from overflow menu

| Test | Type | Covers |
|------|------|--------|
| `Change Program appears in session overflow menu` | Jest/RTL | Menu item renders |
| `Change Program opens inline select dialog` | Jest/RTL | Dialog opens with current program pre-selected |
| `Change Program dialog calls updateSession on save` | Jest/RTL | Save wires to hook |
| `session-program-change.spec.ts` | Playwright e2e | Full flow: overflow → select → save → session list shows new program |

### AC5: Active session restart requires confirmation

| Test | Type | Covers |
|------|------|--------|
| `Change Program on Active session shows confirmation dialog` | Jest/RTL | Confirmation renders with restart warning |
| `Change Program confirmation cancel does not call updateSession` | Jest/RTL | Cancel path |
| `Change Program confirmation confirm calls updateSession` | Jest/RTL | Confirm path |

### AC6: Non-Active sessions show pending indicator

| Test | Type | Covers |
|------|------|--------|
| `Stopped session shows pending-program indicator after change` | Jest/RTL | Pending state renders |

### AC7: Claude UUID cleared on program switch away from claude

| Test | Type | Covers |
|------|------|--------|
| `TestUpdateSession_ProgramSwitch_ClearsClaudeUUID` | Go unit | UUID cleared when switching from `claude` → non-claude |
| `TestUpdateSession_ProgramSwitch_PreservesUUID_SameFamily` | Go unit | UUID preserved when switching within claude family |

---

## Edge Cases and Error Scenarios

### Edge Case 1: Program not in available programs list
- User's shell PATH changed after session was created; program no longer resolvable.
- **Expected:** Backend stores the string as-is; session fails to start with a clear error (existing behavior — no change needed).
- **Test:** `TestUpdateSession_ProgramNotInAvailableList_SavesAnyway` — verify no validation on program string content.

### Edge Case 2: Empty program value and `NotEmpty()` constraint
- User selects "System default" (value `""`); backend must not persist `""` to DB.
- **Expected:** Backend resolves `""` to `config.DefaultProgram` before persisting.
- **Test:** `TestUpdateSession_EmptyProgram_PersistsResolvedDefault` — verify DB stores resolved name, not empty string.

### Edge Case 3: Restart fails mid-update (RC4 ordering)
- `Restart()` succeeds but `SaveInstances` fails (disk full, DB locked).
- **Expected after fix:** `SaveInstances` runs before `Restart()`; if save fails, restart is skipped and error is returned.
- **Test:** `TestUpdateSession_SaveFails_DoesNotRestart` — mock storage to fail; verify `Restart()` never called.

### Edge Case 4: `instance.started == false` (fallback load path)
- Poller unavailable; instance loaded from DB without `started = true`.
- **Expected:** `Restart()` returns `ErrCannotRestart`; user gets clear error, not HTTP 200.
- **Test:** `TestUpdateSession_Active_StartedFalse_ReturnsError` — mock poller unavailable path.

### Edge Case 5: Concurrent program change from two tabs
- Two UI tabs open; both change program to different values simultaneously.
- **Expected:** Last write wins (existing mutex behavior); no crash.
- **Test:** Not required (existing session lock covers this; document as known behavior).

### Edge Case 6: Switch back to claude after UUID was not cleared (regression guard)
- Pre-fix: UUID not cleared on switch away from claude; switch back attempts `--resume <stale-uuid>`.
- **Expected post-fix:** UUID cleared on switch away, so switch-back starts fresh.
- **Test:** `TestUpdateSession_SwitchBackToClaude_NoStaleResume` — verify UUID is absent after round-trip.

---

## Test Infrastructure Notes

- Go tests for `UpdateSession` go in `server/services/session_service_test.go` alongside existing `TestUpdateSession_*` tests.
- Jest tests for program edit UI go alongside `SessionDetailView` tests if they exist, or in a new `SessionDetailView.program.test.tsx`.
- Playwright e2e test requires the test server running with `STAPLER_SQUAD_INSTANCE=e2e-local`; follows existing e2e conventions (data-testid locators, no waitForTimeout).
- Register the new e2e spec in `backend-features.json` and `frontend-features.json` per the feature registry rules.
