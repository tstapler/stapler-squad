# omnibar-session-name-ux — Implementation Plan

Status: Ready for implementation
Phase: 5 — Implementation
Created: 2026-05-05
Spec: `project_plans/omnibar-session-name-ux/`
ADR: `project_plans/omnibar-session-name-ux/decisions/ADR-001-omnibar-inline-shorthand-language.md`

---

## Epic Overview

Users who type a name or description into the omnibar must currently re-type that
text into the session name field. There is no way to provide an initial Claude
prompt at creation time, and switching session types requires reaching for the
mouse. This epic removes all three friction points through a layered set of
keyboard-first features.

### Success Metrics

1. Session name field is pre-populated from omnibar input in every creation mode
   with no manual re-entry required.
2. User can optionally attach a first prompt inline (via `>` separator) or via
   an expandable textarea — no extra navigation needed.
3. Tab key cycles session types without triggering browser autocomplete or path
   completion dropdowns.
4. `/oneoff`, `/worktree`, `/dir`, `/existing` prefix commands switch session
   type and strip themselves from the resolved name.

### Scope Boundary

- In scope: TypeScript/React changes, one proto field thread-through, one ADR.
- Out of scope: new session types, persistent prompt templates, new
  DetectorRegistry entries for the slash commands themselves.

### Architecture Decision Record

ADR-001 (Omnibar Inline Shorthand Language) is filed in the project plans store,
not in `docs/adr/` — it governs the planning-layer decision, not a persistent
repo ADR. If the team decides to promote it to the repo ADR canon, assign the
next number (012).

---

## Dependency Map

```
Story 1A: toSessionSlug()
    └─> Story 1B: SessionSearchDetector fix (suggestedName)
            └─> Story 2A: > separator parsing (depends on slug + detection type)
            └─> Story 2B: /command prefix parsing (depends on slug)
Story 1C: firstPrompt field in OmnibarFormState
    └─> Story 1D: firstPrompt textarea in OmnibarCreationPanel
            └─> Story 1E: firstPrompt thread-through (OmnibarContext + useSessionService)
                    └─> Story 2A: > separator populates firstPrompt
Story 3A: Tab cycling (independent, no blockers)
```

Stories 1A-1E are fully independent of Story 3A. Story 2 requires both Story 1
chains to be complete. Run Stories 1 and 3 in parallel.

---

## Story 1 — Core Auto-fill and First Prompt

**Goal:** Typing in the omnibar populates the session name automatically. A
collapsible textarea lets users attach an optional initial Claude prompt.

### Task 1.1 — `toSessionSlug` pure function

**Files:** `web-app/src/lib/omnibar/slugify.ts` (new file)

Implement a zero-dependency slug generator as a named export. The function must:

- Strip leading/trailing whitespace.
- Normalize Unicode to NFC, then replace non-ASCII with their closest ASCII
  equivalent where possible (e.g. `é` → `e`), then drop anything remaining
  outside `[a-zA-Z0-9]`.
- Convert spaces and underscores to hyphens.
- Collapse multiple consecutive hyphens to one.
- Lowercase the result.
- Trim leading and trailing hyphens.
- Return `""` for degenerate inputs (empty string, emoji-only, CJK-only).
  **Never** return a hardcoded fallback like `"session"` — the caller decides
  the fallback.

```ts
export function toSessionSlug(input: string): string { ... }
```

Test file: `web-app/src/lib/omnibar/slugify.test.ts`

Required test cases (add as `it` blocks under `describe("toSessionSlug")`):

| Input | Expected output |
|---|---|
| `"my feature branch"` | `"my-feature-branch"` |
| `"Fix: Auth / Login"` | `"fix-auth-login"` |
| `"  leading spaces  "` | `"leading-spaces"` |
| `"café au lait"` | `"cafe-au-lait"` |
| `"emoji 🎉 party"` | `"emoji-party"` |
| `"中文"` | `""` |
| `""` | `""` |
| `"---"` | `""` |
| `"a".repeat(120)` | string with length <= 60 (truncate at 60 chars) |

Add a max-length truncation of 60 characters (trim at word boundary if
possible; hard-truncate otherwise) and strip trailing hyphens after truncation.

Effort estimate: 2-3h

### Task 1.2 — Wire `toSessionSlug` into `SessionSearchDetector`

**Files:** `web-app/src/lib/omnibar/detector.ts`

In `SessionSearchDetector.detect()`, change:

```ts
// Before
suggestedName: "",
```

to:

```ts
// After — import { toSessionSlug } from "./slugify";
suggestedName: toSessionSlug(trimmed),
```

The `lastSuggestedNameRef` guard in `Omnibar.tsx` (lines 361-366) already
handles the "don't overwrite manually edited names" invariant. This change
makes session name auto-fill work for free-text input for the first time.

Verify: open the omnibar, type `"auth service refactor"`, switch to creation
mode — session name field should read `"auth-service-refactor"`.

Effort estimate: 30m (plus verifying existing guard logic still applies)

### Task 1.3 — Add `firstPrompt` to `OmnibarFormState`

**Files:** `web-app/src/components/sessions/Omnibar.tsx`

Extend `OmnibarFormState` (line 36-50):

```ts
export interface OmnibarFormState {
  // ... existing fields ...
  firstPrompt: string;
}
```

Extend `INITIAL_FORM_STATE` (line 52-66):

```ts
const INITIAL_FORM_STATE: OmnibarFormState = {
  // ... existing fields ...
  firstPrompt: "",
};
```

No other logic changes in this task — that comes in Task 1.4 and 1.5.

Effort estimate: 30m

### Task 1.4 — `firstPrompt` textarea in `OmnibarCreationPanel`

**Files:** `web-app/src/components/sessions/OmnibarCreationPanel.tsx`,
`web-app/src/components/sessions/OmnibarCreationPanel.css.ts` (new or extend)

Add a collapsible "First prompt (optional)" section below the session name
field. Use the existing `collapsible`, `collapsibleHeader`, `collapsibleTitle`,
`collapsibleIcon`, `expanded`, `collapsibleContent` style tokens already
imported from `Omnibar.css` — do NOT add a new CSS module.

UI behavior:
- Collapsed by default. Disclosure triangle on click.
- When `formState.firstPrompt` is non-empty (e.g. populated by the `>`
  separator in Task 2.1), auto-expand on render.
- Textarea: `rows={3}`, `placeholder="Claude will receive this as its first
  message"`, `maxLength={4000}`.
- onChange wires to `onFormChange({ firstPrompt: e.target.value })`.

The `OmnibarCreationPanel` already receives `formState` and `onFormChange` as
props. No new props are needed.

If any local CSS needs to be added for the textarea specifically (e.g. full
width, resize vertical only), add it to a `OmnibarCreationPanel.css.ts` file
using vanilla-extract `style()`. Import from `web-app/src/styles/theme.css.ts`
vars — never hardcode values.

Effort estimate: 2h

### Task 1.5 — Thread `firstPrompt` through the submission pipeline

**Files:** `web-app/src/components/sessions/Omnibar.tsx`,
`web-app/src/lib/contexts/OmnibarContext.tsx`,
`web-app/src/lib/hooks/useSessionService.ts`

**Step A — Fix the critical data loss bug in `handleSubmit` (Omnibar.tsx line 641):**

Current code (BUGGY — silently drops firstPrompt):
```ts
const finalPrompt = imagePaths.length > 0 ? imagePaths.join(" ") : undefined;
```

Correct code:
```ts
const firstPromptText = formState.firstPrompt?.trim() || "";
const finalPrompt =
  [firstPromptText, ...imagePaths].filter(Boolean).join("\n") || undefined;
```

This fix must land before `firstPrompt` is wired anywhere else. Without it,
firstPrompt data is silently dropped on submit regardless of what the UI shows.

**Step B — Add `initialPrompt` to `OmnibarSessionData` (Omnibar.tsx line 76-97):**

```ts
export interface OmnibarSessionData {
  // ... existing fields ...
  initialPrompt?: string;
}
```

**Step C — Pass `firstPrompt` through `handleSubmit`:**

In both the `new_project` and else branch of `handleSubmit`, add:
```ts
prompt: finalPrompt,
```
(This field already exists in the data object; the fix in Step A is what makes
it correct. No structural change to sessionData is needed here.)

**Step D — Thread `initialPrompt` in `OmnibarContext.tsx`:**

The `handleCreateSession` callback currently passes `prompt: data.prompt`.
Confirm this maps correctly to `initial_prompt` field 15 in the proto after
Step E. If `data.prompt` already maps to `prompt` in `createSession`, no
structural change is needed — only the fix in Step A matters.

Check `proto/session/v1/session.proto` field 15 (`initial_prompt`). Confirm
`useSessionService.ts` `createSession` passes `prompt: request.prompt` to the
RPC. If `initial_prompt` maps to `prompt` in the proto-generated types, no new
field is needed. If there is a name mismatch, add `initialPrompt` to the
`CreateSessionRequest` partial and thread it explicitly.

**Step E — Confirm `useSessionService.ts` passes the field:**

Current line 169: `prompt: request.prompt`. Verify this sends to
`initial_prompt = 15` in the proto. If the generated field name differs from
`prompt`, update both the context and the service hook.

Effort estimate: 2-3h (the Step A bug fix is the highest-risk change; test
carefully with image attachment + firstPrompt simultaneously)

---

## Story 2 — Separator Shorthand and Slash Commands

**Prerequisite:** Story 1 complete (Tasks 1.1-1.5 merged and green).

**Goal:** `my session > do something` splits on `>`. `/oneoff my task` switches
session type and slugifies `my task` as the name.

### Task 2.1 — `parseInputWithSeparator` pure function

**Files:** `web-app/src/lib/omnibar/parseInput.ts` (new file)

```ts
export interface ParsedInput {
  name: string;       // raw text before separator (trimmed)
  prompt: string;     // raw text after separator (trimmed), or ""
}

export function parseInputWithSeparator(input: string): ParsedInput {
  // Split on first '>' only
  const idx = input.indexOf(">");
  if (idx === -1) return { name: input.trim(), prompt: "" };
  return {
    name: input.slice(0, idx).trim(),
    prompt: input.slice(idx + 1).trim(),
  };
}
```

Gating rule: this function is invoked only when
`detection?.type === InputType.SessionSearch`. For all other detection types
(LocalPath, GitHub URLs, etc.) the `>` character is treated as literal input.

Apply in `Omnibar.tsx` at the points where `canSubmit` is computed and where
`handleSubmit` builds the session data:

In `canSubmit` (around line 610): derive `parsedName` from
`parseInputWithSeparator(input)` when `detection?.type === InputType.SessionSearch`,
then check `parsedName.name` for the session name validity test rather than
raw `sessionName`.

In `handleSubmit`: when `detection?.type === InputType.SessionSearch`, use
`parsedName.name` as `title` and prepend `parsedName.prompt` to `finalPrompt`.

In the detection debounce effect: when detection is `SessionSearch` and
`parseInputWithSeparator` finds a prompt part, also auto-expand the firstPrompt
textarea by setting `formState.firstPrompt` to the prompt part (so the user
sees the split visually).

Test file: `web-app/src/lib/omnibar/parseInput.test.ts`

Required test cases:

| Input | name | prompt |
|---|---|---|
| `"auth service"` | `"auth service"` | `""` |
| `"auth service > fix login bug"` | `"auth service"` | `"fix login bug"` |
| `" > only prompt"` | `""` | `"only prompt"` |
| `"name > "` | `"name"` | `""` |
| `"a > b > c"` | `"a"` | `"b > c"` |
| `""` | `""` | `""` |

Effort estimate: 2h

### Task 2.2 — `parseSlashCommand` pure function

**Files:** `web-app/src/lib/omnibar/parseSlashCommand.ts` (new file)

```ts
export interface SlashCommandResult {
  matched: boolean;
  sessionType: OmnibarFormState["sessionType"] | null;
  remainder: string;  // input with prefix stripped, trimmed
}

// KNOWN_SLASH_COMMANDS is the sole extension point.
// To add a new slash command: add one entry here. Nothing else changes.
const KNOWN_SLASH_COMMANDS: Record<string, OmnibarFormState["sessionType"]> = {
  "/oneoff":    "one_off",
  "/one-off":   "one_off",
  "/worktree":  "new_worktree",
  "/dir":       "directory",
  "/directory": "directory",
  "/existing":  "existing_worktree",
};

export function parseSlashCommand(input: string): SlashCommandResult {
  const trimmed = input.trimStart();
  if (!trimmed.startsWith("/")) return { matched: false, sessionType: null, remainder: trimmed };

  for (const [prefix, sessionType] of Object.entries(KNOWN_SLASH_COMMANDS)) {
    if (trimmed.toLowerCase().startsWith(prefix + " ") || trimmed.toLowerCase() === prefix) {
      const remainder = trimmed.slice(prefix.length).trimStart();
      return { matched: true, sessionType, remainder };
    }
  }

  // Starts with "/" but not a known command — do not consume
  return { matched: false, sessionType: null, remainder: trimmed };
}
```

Wire into `Omnibar.tsx` detection debounce (the `useEffect` at line 377):
Pre-process the raw `input` string through `parseSlashCommand` before calling
`detect(remainder)`. If `matched`, update `formState.sessionType` via
`setFormState` and pass `remainder` to `detect()`. The session type change must
be visible in the radio group immediately.

Important: this pre-processing happens at the top of the debounce callback,
before `detect()` is called. This is why slash commands are not a new Detector
— the Detector receives the cleaned remainder, never the raw `/oneoff` prefix.

This also resolves the HIGH bug: `/oneoff` currently matches `LocalPathDetector`
(which fires on leading `/`). Because pre-processing runs first, `detector.detect()`
never sees the `/oneoff` prefix.

Test file: `web-app/src/lib/omnibar/parseSlashCommand.test.ts`

Required test cases:

| Input | matched | sessionType | remainder |
|---|---|---|---|
| `"/oneoff auth service"` | true | `"one_off"` | `"auth service"` |
| `"/ONEOFF auth"` | true | `"one_off"` | `"auth"` |
| `"/worktree"` | true | `"new_worktree"` | `""` |
| `"/dir my project"` | true | `"directory"` | `"my project"` |
| `"/existing"` | true | `"existing_worktree"` | `""` |
| `"/unknown command"` | false | null | `"/unknown command"` |
| `"normal input"` | false | null | `"normal input"` |
| `"/oneofftogether"` | false | null | `"/oneofftogether"` |

Effort estimate: 2h

### Task 2.3 — Input preview label for separator

**Files:** `web-app/src/components/sessions/Omnibar.tsx` (display only)

When `detection?.type === InputType.SessionSearch` and the `>` separator is
present, display a visual preview below the input showing how the input was
split. This is informational only — no new state is introduced. Derive from
the existing `input` string at render time.

Format:
```
Name: "auth-service"  |  Prompt: "fix login bug"
```

Use an existing CSS class (e.g. `detectionInfo`) for the container. The name
portion should show the slugified form (apply `toSessionSlug` inline).

Gate: only render when `detection?.type === InputType.SessionSearch` AND
`input.includes(">")`.

Effort estimate: 1h

---

## Story 3 — Tab Cycling and Polish

**Goal:** Tab key cycles session types. Empty slug shows validation hint.
Both stories in this epic are independent of Story 2.

### Task 3.1 — Tab session-type cycling

**Files:** `web-app/src/components/sessions/Omnibar.tsx`

Add a keydown handler on the main omnibar input element. When `key === "Tab"`
and `!isDropdownVisible` (the boolean is already computed in Omnibar.tsx):

1. `e.preventDefault()` — suppress browser tab navigation.
2. Read current `formState.sessionType`.
3. Find its index in `SESSION_TYPES` (imported from `OmnibarCreationPanel.tsx`
   or duplicated locally — prefer a single source of truth via export).
4. Advance to `(index + 1) % SESSION_TYPES.length`.
5. `setFormState(prev => ({ ...prev, sessionType: next }))`.

Gate enforcement: if `isDropdownVisible` is true, do NOT preventDefault and
allow the Tab to interact with the dropdown normally. This resolves the HIGH
bug where Tab cycles session type while the path completion dropdown is open.

Do NOT implement Ctrl+Tab — Chrome intercepts this key combination at the
browser level and it cannot be reliably prevented.

Where to attach: the existing `handleKeyDown` function on the main `<input>`
element (search for `onKeyDown` usage in Omnibar.tsx). Add the Tab case before
the existing cases.

Test: add a Jest/RTL test that fires a Tab keydown event and asserts the
session type cycles. Assert no-op when dropdown is visible.

Effort estimate: 2h

### Task 3.2 — Empty slug validation hint

**Files:** `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

When `formState.sessionName === ""` and `formState.sessionType !== "one_off"`
and the user has typed something in the omnibar (i.e. the parent has non-empty
`input`), display an inline hint below the session name input:

```
Session name could not be generated from this input — please enter a name.
```

Use the existing `errorClass` style token (imported from `Omnibar.css`). This
is a hint, not a hard error — it should not block the submit button (submit is
already blocked by `!sessionName.trim()`).

This resolves the MEDIUM bug: when input is emoji-only or CJK-only,
`toSessionSlug` returns `""` and the user sees a blank name field with no
explanation. The hint makes the silent disable visible.

The hint requires a new prop: `hasTypedInput: boolean` (true when
`input.trim().length > 0`). Add this prop to `OmnibarCreationPanel`'s
interface.

Effort estimate: 1h

---

## Known Issues

### CRITICAL: `handleSubmit` silently drops `firstPrompt` (data loss)

**Location:** `web-app/src/components/sessions/Omnibar.tsx`, line 641

**Description:** The current implementation builds `finalPrompt` exclusively
from `imagePaths`. Any value in `formState.firstPrompt` is never included. If
Task 1.3-1.4 are shipped without the Task 1.5 fix, users who enter a first
prompt will see no error but their prompt will not reach Claude.

**Fix:** Task 1.5 Step A. Must be the first change landed in Task 1.5.

**Mitigation:** Implement Task 1.5 before or simultaneously with Task 1.4 so
the UI and the wiring ship together. Never ship the textarea (Task 1.4) without
the fix (Task 1.5).

---

### HIGH: `/oneoff` prefix matched as `LocalPathDetector` input

**Location:** `web-app/src/lib/omnibar/detector.ts`, `LocalPathDetector.detect()`

**Description:** `LocalPathDetector` fires on any input starting with `/` (line
222). If a user types `/oneoff`, it is currently classified as a local path,
causing incorrect UX (path completion attempts, wrong mode). Task 2.2
pre-processes the input before `detect()` is called, which prevents the
detector from ever seeing the `/oneoff` prefix.

**Fix:** Task 2.2. Pre-processing in the debounce effect resolves this
completely. No change to `LocalPathDetector` is required.

**Risk if unaddressed:** `/oneoff` triggers path completion dropdown and shows
an invalid-path indicator instead of switching session type.

---

### HIGH: Tab cycles session type while path-completion dropdown is open

**Location:** `web-app/src/components/sessions/Omnibar.tsx` (Tab handler to be
added in Task 3.1)

**Description:** The existing `isDropdownVisible` boolean tracks whether the
path-completion dropdown is open. Without gating the Tab handler on this
boolean, pressing Tab while the dropdown is open would simultaneously dismiss
the dropdown AND advance the session type — a confusing two-action side effect.

**Fix:** Task 3.1 explicitly gates `e.preventDefault()` and type cycling on
`!isDropdownVisible`. When dropdown is visible, Tab falls through to default
browser behavior (dropdown interaction).

---

### MEDIUM: Degenerate slug silently disables submit with no user feedback

**Location:** `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

**Description:** When `toSessionSlug` returns `""` (e.g. user typed `"中文"`
or `"🎉🎉🎉"`), the session name field is empty, `canSubmit` returns false, and
the submit button is disabled — but there is no message explaining why. The
user's only recourse is to clear the field and type something else.

**Fix:** Task 3.2 adds an inline validation hint that surfaces when the slug
degenerates. The hint is visible only when the user has typed something and the
name field is empty, making the causality clear.

---

## Integration Checkpoints

After Task 1.2 is merged: manually verify auto-fill for free-text input in
the omnibar. Type `"auth service refactor"` → switch to creation mode →
session name must show `"auth-service-refactor"`.

After Task 1.5 is merged: create a session with both `firstPrompt` text and an
attached image. Verify the submitted prompt contains both, separated by `\n`.
Verify with an empty `firstPrompt` that no extra `\n` appears.

After Task 2.1 is merged: type `"my feature > fix the auth bug"` in the omnibar.
Verify: session name shows `"my-feature"`, firstPrompt textarea expands with
`"fix the auth bug"`.

After Task 2.2 is merged: type `/oneoff build the thing` in the omnibar. Verify:
session type radio jumps to "One-off", session name shows `"build-the-thing"`,
no path completion dropdown appears.

After Task 3.1 is merged: press Tab in the creation panel input. Verify session
type advances through the list. Open path-completion dropdown, press Tab, verify
no session type change occurs.

---

## Implementation Order (Critical Path)

Run in this order, Stories 1 and 3 in parallel:

1. Task 1.1 — `toSessionSlug` (unblocks 1.2, 2.1, 2.2)
2. Task 1.3 — `firstPrompt` field in `OmnibarFormState` (unblocks 1.4, 1.5)
3. Task 1.2 — Wire slug into `SessionSearchDetector`
4. Task 1.4 + 1.5 simultaneously — textarea UI + submission fix (ship together)
5. Task 3.1 — Tab cycling (independent, can run in parallel with 1.x)
6. Task 2.1 — `parseInputWithSeparator` (requires 1.1 + 1.5 complete)
7. Task 2.2 — `parseSlashCommand` (requires 1.1 + 1.2 complete)
8. Task 2.3 — Preview label (requires 2.1)
9. Task 3.2 — Empty slug hint (requires 1.1)

---

## Registry Checklist

Slash commands alias existing session creation modes — they do NOT constitute a
new session creation mode. The 7-touchpoint session-creation-registry checklist
does NOT apply to Tasks 2.2 or 3.1.

The `firstPrompt` field added in Task 1.3 threads through `OmnibarFormState`
→ `OmnibarSessionData` → `handleCreateSession` → `createSession` RPC. Confirm
`initial_prompt` (field 15) exists in `proto/session/v1/session.proto` before
starting Task 1.5. If `initial_prompt` is already wired in `useSessionService`,
no proto change is needed.

The OmnibarAction union (`types.ts`) does NOT need a new action type. The
`create_session` action already handles all new parameters via existing fields.
The dispatch test suite does not need new describe blocks unless `dispatch.ts`
logic changes.

Run after each story:
```bash
cd web-app && npx jest --no-coverage --testPathPatterns="slugify|parseInput|parseSlashCommand|dispatch|detector"
make quick-check
```
