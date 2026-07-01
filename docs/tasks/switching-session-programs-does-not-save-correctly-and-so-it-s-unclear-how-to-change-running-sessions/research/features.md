# Feature Research: Switching Session Programs

## How Program Is Set at Session Creation

`program` is an optional string field on `CreateSessionRequest` (proto field 5, default `"claude"`). At creation time:

- The omnibar form state carries a `program` field.
- `OmnibarCreationPanel` renders a `<select>` under "Advanced Options" populated by `useAvailablePrograms()`, which merges the static `PROGRAMS` constant list with any extras returned by `/api/server-info`.
- `OmnibarContext` passes `program: data.program` through to `useSessionService.createSession`.
- `useSessionService.createSession` sends `program: request.program` in the ConnectRPC call body.
- The Go handler stores it on `session.Instance.Program`.

The program value is visible on the `Session` proto (field 7) and returned in all list/watch responses.

## The UpdateSession Update Path (Backend)

`UpdateSessionRequest` has an `optional string program = 5` field. The backend handler (`session_service.go` lines 1429–1441) handles it:

```go
if req.Msg.Program != nil && *req.Msg.Program != "" && instance.Program != *req.Msg.Program {
    instance.Program = *req.Msg.Program
    updatedFields = append(updatedFields, "program")
    if instance.Status == session.Active {
        if err := instance.Restart(true); err != nil { ... }
    }
}
```

The backend:
1. Updates the stored `Program` field on the instance.
2. If the session is Active, restarts the process with the new program immediately.
3. Saves the updated instance and publishes a `sessionUpdated` event.

This is correct in isolation. The restart-on-change behavior is the intended design.

## Frontend: Existing Program Edit UI

`SessionDetailView.tsx` already has an inline program editor on the "Info" tab (lines 944–977):
- Displays the current `session.program` with a human-readable label via `getProgramDisplay()`.
- An edit button (`✏️`) opens an in-place `<select>` populated by `useAvailablePrograms()`.
- On save, calls `actions.update({ program: v })` which goes through `useSessionActions.update` → `useSessionService.updateSession` → `UpdateSession` RPC.

The `useSessionService.updateSession` method (lines 290–316) passes `program: updates.program` in the RPC body.

## The Bug: Why Program Changes Don't Save

The `updateSession` call in `useSessionService` sends:
```ts
program: updates.program,
```

The proto field `program` in `UpdateSessionRequest` is `optional string` — it uses a pointer in Go. The `@bufbuild/protobuf` generated code serializes an `undefined` value as absent (not set), but an **empty string** as an explicit empty string. The backend guard is:
```go
if req.Msg.Program != nil && *req.Msg.Program != "" && ...
```

The likely failure mode: if `programValue` in the edit state is `""` (cleared or uninitialized), the `<select>` renders with no matching option and the save sends an empty string or the field is serialized incorrectly, which the backend skips. Additionally, the `isKnownProgram` check and custom-option fallback suggest the UI was patched reactively but the core `programValue` initialization (`useState(session.program || "")`) may not re-sync when `session.program` changes from a watch event — so the edit form could show a stale value that gets saved back incorrectly.

A secondary discoverability issue: the program edit UI is buried under the "Info" tab in the detail view, with no affordance from the session list, session card, or overflow menu. Users have no clear path to find it.

## Existing UI Patterns That Can Be Reused

1. **Inline string field editor pattern** (`makeStringFieldEditor` in `SessionDetailView.tsx` lines 332–355): generic factory for edit/save/cancel of a single string field. Already used for program, workingDir, category. Reusable as-is.

2. **`useAvailablePrograms` hook**: fetches the program list from `/api/server-info` and merges with the static `PROGRAMS` constant. Already imported in `SessionDetailView` and `OmnibarCreationPanel`.

3. **Overflow menu dialogs** (`SessionActionsOverflow.tsx`): pattern for adding a new menu item with an inline dialog (confirm, text input, steer). The checkpoint and steer dialogs are the closest analogs.

4. **Tag editor dialog** (`TagEditor.tsx`): a standalone dialog triggered from the overflow menu. A similar "Change Program" dialog could follow this pattern for the primary access point.

## What Needs Fixing

1. **Save correctness**: Verify that `updateSession` sends the `program` field as a non-nil, non-empty optional proto field when the user selects a value. Check whether the `programValue` state re-syncs with `session.program` when the session updates via WatchSessions.

2. **Discoverability**: The program editor should be accessible from the overflow menu (not just the Info tab). A "Change Program" menu item in `SessionActionsOverflow` — using the existing dialog pattern — would close the discoverability gap.

3. **Restart feedback**: When the backend restarts the session after a program change (Active sessions), the UI should reflect this. The restart is currently silent from the user's perspective.
