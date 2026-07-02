# Architecture: Switching Session Programs Does Not Save Correctly

## Root Cause Hypothesis

The bug has one primary root cause and one secondary UX gap:

**Primary (backend guard blocks "clear to default"):** In `server/services/session_service.go` line 1430, the program update guard is:

```go
if req.Msg.Program != nil && *req.Msg.Program != "" && instance.Program != *req.Msg.Program {
```

The `*req.Msg.Program != ""` check silently drops any request that sends `program = ""`. The "System default" option in the UI has `value: ""` (in `web-app/src/lib/constants/programs.ts` line 8). So when a user selects "System default" from the dropdown and saves, the frontend correctly sends `program: ""` in the RPC call, the backend receives it, but the `!= ""` guard short-circuits and nothing changes. The API returns HTTP 200 with the unchanged session. The user sees no error and no visible feedback that the save was ignored.

**Secondary (UX gap — no feedback on "no-op" save):** `makeStringFieldEditor` in `SessionDetailView.tsx` only calls `updateFn` when `editValue !== originalValue`. If the session already has `program = ""` and the user picks the same "System default", the frontend skips the RPC call entirely (correct behavior). But when the guard silently drops the save on the backend side, the UI also provides no toast or error — making it impossible for the user to understand that anything went wrong.

---

## Data Flow Diagram

```
User selects "System default" (value="") from program dropdown
  |
  v
SessionDetailView.tsx: makeStringFieldEditor
  handleSave: editValue("") !== originalValue("claude") => true
  calls actions.update({ program: "" })
  |
  v
useSessionService.ts (line 295): updateSession({ program: "" })
  -> ConnectRPC -> UpdateSession RPC
  |
  v
session_service.go UpdateSession handler (line 1430):
  req.Msg.Program != nil  => true  (optional string, was set)
  *req.Msg.Program != ""  => FALSE (empty string)
  SHORT CIRCUIT — no update applied
  returns HTTP 200 with old session data
  |
  v
Frontend receives stale session (program still = "claude")
  UI state stays at "System default" (local state not reverted)
  => Apparent mismatch: dropdown shows "System default", server has "claude"
```

---

## Affected Components

| Layer | File | Issue |
|---|---|---|
| Backend handler | `server/services/session_service.go:1430` | `!= ""` guard blocks clearing to empty |
| Frontend UI | `web-app/src/components/sessions/SessionDetailView.tsx:341` | No error feedback when update returns stale data |
| Proto | `proto/session/v1/session.proto:570` | `optional string program = 5` — correct design, not broken |
| ORM/storage | `session/ent_repository.go` | `SetProgram(data.Program)` — correct, persists unconditionally |
| Frontend hook | `web-app/src/lib/hooks/useSessionService.ts:295` | Passes `program` through correctly, not broken |

---

## Proposed Fix Components

### Fix 1 — Backend (required, single-line)

Remove `*req.Msg.Program != ""` from the guard in `session_service.go`. Empty string is a valid semantic value meaning "use server/config default." The proto `optional` wrapper already distinguishes "not sent" (`nil`) from "explicitly set to empty" (`""`), so the nil check alone is sufficient to gate the update:

```go
// Before
if req.Msg.Program != nil && *req.Msg.Program != "" && instance.Program != *req.Msg.Program {

// After
if req.Msg.Program != nil && instance.Program != *req.Msg.Program {
```

The ORM layer already handles this correctly — `SetProgram("")` will persist an empty string, and the session launch code resolves empty program to the config default.

### Fix 2 — Frontend (optional, UX improvement)

In `makeStringFieldEditor`, after `updateFn(editValue)` resolves, compare the returned session's `program` value against what was sent. If they differ (indicating a silent backend drop), surface an error toast. This is a defensive guard and becomes unnecessary once Fix 1 is applied, but improves debuggability.

Alternatively, sync the local `programValue` state from the returned session after save — if the server returns a different value than what was submitted, the UI will visibly revert, making the failure apparent.

---

## Component Boundary

- **Fix 1 is backend-only.** No proto changes, no frontend changes required for the core save to work.
- **Fix 2 is frontend-only.** Improves UX resilience but is not required once Fix 1 is in place.
- No ORM schema changes needed.
- No proto changes needed (`optional string program = 5` is already correct).

## Verification

After Fix 1, the test path is: session with `program = "claude"` → select "System default" → save → verify `instance.Program == ""` in DB → verify next session launch uses config default program.
