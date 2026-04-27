# Session Creation Mode Registry (ADR-implicit)

Every session creation mode is registered in **7 touchpoints** across the stack. When adding a new mode (or modifying an existing one), all 7 must be updated. Missing any one breaks the full creation flow silently.

## The 7 Touchpoints

### 1. Proto enum — `proto/session/v1/types.proto`
Add a new `SESSION_TYPE_*` value to the `SessionType` enum.

```protobuf
enum SessionType {
  SESSION_TYPE_UNSPECIFIED = 0;
  SESSION_TYPE_DIRECTORY = 1;
  SESSION_TYPE_NEW_WORKTREE = 2;
  SESSION_TYPE_EXISTING_WORKTREE = 3;
  // Add here if the mode has a distinct type
}
```

> **One-off exception**: one-off reuses `SESSION_TYPE_DIRECTORY` and uses a separate `bool one_off` flag instead of a new enum value. Use this pattern when the backend session type is shared but behavior is driven by additional request parameters.

---

### 2. Proto request message — `proto/session/v1/session.proto`
If the new mode needs parameters not covered by the existing `CreateSessionRequest` fields, add them here (new `bool`, `string`, or nested message). Always use the next available field number.

```protobuf
message CreateSessionRequest {
  // ... existing fields ...
  bool one_off = 14;  // example: one-off uses a flag, not a new enum value
}
```

Run `make generate-proto` after any proto change. This regenerates:
- `session/gen/session/v1/*.go` (Go bindings)
- `web-app/src/gen/session/v1/*_pb.ts` (TypeScript bindings)

---

### 3. Go backend handler — `server/services/session_service.go`

The `CreateSession` function has two registration points:

**3a. Path validation guard** — if the new mode doesn't require a path, add it to the guard condition:
```go
if !req.Msg.OneOff && req.Msg.Path == "" {
    return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("path is required"))
}
```

**3b. Session type resolution block** — the `switch req.Msg.SessionType` maps proto enum values to Go `session.SessionType` constants. Add a case if you added a new enum value:
```go
switch req.Msg.SessionType {
case sessionv1.SessionType_SESSION_TYPE_DIRECTORY:
    sessionType = session.SessionTypeDirectory
case sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE:
    sessionType = session.SessionTypeNewWorktree
// add here
}
```

**3c. Mode-specific logic block** — add a block after path resolution for any mode-specific behavior (directory creation, URL fetching, etc.):
```go
if req.Msg.OneOff {
    baseDir, _ := cfg.OneOffBaseDirOrDefault()
    name, _ := namegen.GenerateUnique(baseDir, 10)
    resolvedPath = filepath.Join(baseDir, name)
    sessionType = session.SessionTypeDirectory
}
```

---

### 4. Go session type constants — `session/instance.go`

If the new mode represents a structurally different session lifecycle (different worktree behavior, different start path resolution), add a new `SessionType` constant. If the new mode reuses an existing lifecycle, skip this.

```go
const (
    SessionTypeDirectory        SessionType = "directory"
    SessionTypeNewWorktree      SessionType = "new_worktree"
    SessionTypeExistingWorktree SessionType = "existing_worktree"
    // Add here only if lifecycle differs
)
```

Also update `SessionType.IsValid()` if a new constant is added.

---

### 5. Frontend type union — `web-app/src/components/sessions/Omnibar.tsx`

Add the new mode's string identifier to the `sessionType` union in `OmnibarFormState`:

```ts
type OmnibarFormState = {
  sessionType: "directory" | "new_worktree" | "existing_worktree" | "one_off";
  // ...
}
```

Also update `canSubmit` (what fields are required for this mode) and `handleSubmit` (what fields are passed to `OmnibarSessionData` on submit). Conditionally suppress the path input/dropdown/detection badge for modes that don't use a path.

---

### 6. Frontend radio group — `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

Add an entry to `SESSION_TYPES` and any mode-specific hint text or hidden/shown fields:

```ts
const SESSION_TYPES = [
  { value: "new_worktree", label: "New Worktree" },
  { value: "directory", label: "Directory" },
  { value: "existing_worktree", label: "Use Worktree" },
  { value: "one_off", label: "One-off" },
] as const;
```

Add a hint string for the new mode in the conditional hint block and conditionally render or hide the working directory field.

---

### 7. Frontend context + hook — `OmnibarContext.tsx` + `useSessionService.ts`

**`OmnibarContext.tsx`** — add the new mode to `sessionTypeMap` (maps frontend string → proto `SessionType` enum):
```ts
const sessionTypeMap: Record<string, SessionType> = {
  directory: SessionType.DIRECTORY,
  new_worktree: SessionType.NEW_WORKTREE,
  existing_worktree: SessionType.EXISTING_WORKTREE,
  one_off: SessionType.DIRECTORY, // reuses DIRECTORY; server handles the distinction
};
```

Pass any new request fields through the `createSession` call:
```ts
oneOff: data.oneOff ?? false,
```

**`useSessionService.ts`** — thread new fields to the ConnectRPC call body:
```ts
oneOff: request.oneOff ?? false,
```

---

## Checklist for a New Session Creation Mode

Copy this checklist into the PR description when adding a new mode:

- [ ] `proto/session/v1/types.proto` — new enum value (or reuse existing + flag)
- [ ] `proto/session/v1/session.proto` — new request field(s) if needed
- [ ] `make generate-proto` — regenerated bindings
- [ ] `server/services/session_service.go` — path guard, switch case, mode logic
- [ ] `session/instance.go` — new `SessionType` constant (if lifecycle differs)
- [ ] `web-app/src/components/sessions/Omnibar.tsx` — type union, canSubmit, handleSubmit
- [ ] `web-app/src/components/sessions/OmnibarCreationPanel.tsx` — SESSION_TYPES, hint, field visibility
- [ ] `web-app/src/lib/contexts/OmnibarContext.tsx` — sessionTypeMap, createSession passthrough
- [ ] `web-app/src/lib/hooks/useSessionService.ts` — RPC call body
- [ ] Unit tests: backend logic (Go), frontend form behavior (Jest/RTL)
- [ ] Integration test: `CreateSession` RPC with new mode creates expected session state

## One-Off Session Reference Implementation

The one-off session feature (2026-04-24) is the canonical example of the "flag on existing type" pattern. See:
- `session/namegen/` — name generation package
- `config/config.go` `OneOffBaseDirOrDefault()` — config with lazy default
- `server/services/session_service.go` lines ~510–615 — full handler flow
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` — UI integration
