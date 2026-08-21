# Research: Features — Web UI Session Creation

## Summary

Session creation flows through the Omnibar component. The creation panel is `OmnibarCreationPanel`. Adding a "One-off" session type requires adding it to `SESSION_TYPES`, adding form state, and conditionally hiding the path field (which lives in the Omnibar's main input, not the panel).

---

## 1. New Session Page

**File**: `web-app/src/app/sessions/new/page.tsx`

This is a redirect-only stub — it redirects to `/?new=true` or `/?duplicate=<id>`. No form here. The actual session creation UI is in the Omnibar component.

---

## 2. Omnibar — Main Session Creation Entry Point

**File**: `web-app/src/components/sessions/Omnibar.tsx`

The Omnibar is a command-palette–style modal. It has two states:
- **Discovery mode**: search existing sessions by typing a session name.
- **Creation mode**: triggered when the input is detected as a path (local path or GitHub URL).

### Detection Flow

The user types a path into the main `<input>` field. The `detect()` function classifies the input:
- `InputType.LocalPath` → creation mode activates.
- `InputType.GitHubURL` → creation mode activates.
- `InputType.SessionSearch` → discovery mode (no creation panel).

**Key implication for one-off**: There's no creation mode today that doesn't require a path input. One-off sessions need a new creation mode that activates without a path — e.g., via a dedicated button or by detecting a special trigger like typing "oneoff" or clicking a button.

### Form State (`OmnibarFormState`, line 35–57)

```typescript
export interface OmnibarFormState {
  sessionName: string;
  branch: string;
  program: string;
  category: string;
  autoYes: boolean;
  useTitleAsBranch: boolean;
  sessionType: "directory" | "new_worktree" | "existing_worktree";
  existingWorktree: string;
  workingDir: string;
}

const INITIAL_FORM_STATE: OmnibarFormState = {
  sessionName: "",
  branch: "",
  program: "claude",
  category: "",
  autoYes: false,
  useTitleAsBranch: true,
  sessionType: "new_worktree",
  existingWorktree: "",
  workingDir: "",
};
```

To support one-off:
- Add `"one_off"` to the `sessionType` union type.
- Or handle one-off as a distinct Omnibar mode (simpler, cleaner separation).

### Submit Logic (`handleSubmit`, line 569–635)

Builds `OmnibarSessionData` from `formState` and `detection`, then calls `onCreateSession(sessionData)`. The `path` field comes from `detection?.localPath`. For one-off, path should be omitted or sent as empty — the backend generates it.

### `canSubmit` (line 551–566)

Currently requires:
1. `input.trim()` — path input is non-empty.
2. `sessionName.trim()` — session name is non-empty.
3. `detection` is valid (not Unknown or SessionSearch).

For one-off mode, validation should only require `sessionName.trim()`.

---

## 3. OmnibarCreationPanel — Session Type Radio Group

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

### Current Session Types (line 17–21)

```typescript
const SESSION_TYPES = [
  { value: "new_worktree", label: "New Worktree" },
  { value: "directory", label: "Directory" },
  { value: "existing_worktree", label: "Use Worktree" },
] as const;
```

These render as an ARIA radio group with arrow-key navigation.

### Conditional Field Rendering Pattern

The panel already hides/shows fields based on `sessionType`:
- Branch fields shown when `sessionType === "new_worktree"` (lines 153–185).
- Existing worktree path shown when `sessionType === "existing_worktree"` (lines 187–223).
- Working directory shown always (lines 225–239).

For one-off:
- Add `{ value: "one_off", label: "One-off" }` to `SESSION_TYPES`.
- When `sessionType === "one_off"`, hide the path input (the main Omnibar input) and hide the Working Directory field.
- Show a hint: "A fresh directory will be created automatically."

### Path Field Location

The path is **not** in `OmnibarCreationPanel` — it's the main `<input>` in `Omnibar.tsx`. When one-off mode is active:
- Either disable/hide the main input (and show a placeholder like "Type a session name").
- Or enter a distinct Omnibar mode that skips the path input entirely.

---

## 4. Recommended UI Implementation Approach

**Option A: Add "One-off" as a 4th session type in the radio group** (simplest)

- Add a button in the main Omnibar (e.g., "Quick Start" / "One-off") that sets mode to `creation_one_off`.
- When the mode is `creation_one_off`:
  - The main input field shows placeholder "Session title…" (not a path).
  - `sessionType` is forced to `"one_off"`.
  - `canSubmit` requires only `sessionName.trim()`.
  - The path input is hidden or shows a read-only generated preview.
  - On submit: `path` is sent empty or omitted; `session_type = directory`; `one_off = true`.

**Option B: Dedicated "Quick Start" button outside the Omnibar**

- A floating button or card in the main sessions view.
- Clicking it opens a minimal modal with only a title field.
- Calls `CreateSession` with `one_off=true`.

**Recommendation**: Option A (Omnibar mode) is consistent with existing UX and requires fewer new components.

---

## 5. Frontend → Backend RPC Call

**File**: Likely in `web-app/src/app/page.tsx` or a hook wrapping `createSession` from the generated ConnectRPC client.

The `onCreateSession` prop in Omnibar ultimately calls the `CreateSession` RPC with a `CreateSessionRequest`. For one-off sessions, the request should include:
- `title`: user-provided
- `path`: empty or omitted (backend validates and generates)
- `one_off: true` (new proto field, or use a dedicated boolean field)
- `session_type: SESSION_TYPE_DIRECTORY` (backend creates directory session)

The frontend needs to be updated wherever `CreateSessionRequest` is built to pass the new `one_off` field.

---

## 6. Key Files for Implementation

| Component | File |
|---|---|
| Omnibar form state & submit | `web-app/src/components/sessions/Omnibar.tsx` |
| Session type radio + panel fields | `web-app/src/components/sessions/OmnibarCreationPanel.tsx` |
| Panel CSS | `web-app/src/components/sessions/OmnibarCreationPanel.css.ts` |
| Omnibar CSS | `web-app/src/components/sessions/Omnibar.css.ts` |
| New session redirect page | `web-app/src/app/sessions/new/page.tsx` |
| Proto: `CreateSessionRequest` | `proto/session/v1/session.proto:276–317` |
| Generated proto TS types | `web-app/src/gen/session/v1/session_pb.ts` |
