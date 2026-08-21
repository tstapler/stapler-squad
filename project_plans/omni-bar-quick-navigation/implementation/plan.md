# Implementation Plan: Omni Bar Quick Navigation

**Feature**: Keyboard-first omnibar with fast session creation, action registry, and TUI removal
**Date**: 2026-04-21
**Status**: Ready for implementation
**Type**: Frontend feature addition + Go cleanup
**ADRs**:
- ADR-001: Static discriminated union for action registry
- ADR-002: `Cmd+Shift+K` shortcut for creation mode (stays in `Cmd+K` family; `Cmd+N` is browser "New Window")
- ADR-003: Arrow-key radio group for session type (not Tab cycling — WCAG)

---

## Critical Design Decisions (Resolved by ADRs)

| Decision | Resolution |
|---|---|
| Action registry pattern | Static discriminated union; new types cause compile errors |
| "Open in creation mode" shortcut | `Cmd+Shift+K` (stays in `Cmd+K` family; `Cmd+N` = browser New Window) |
| Session type selection UX | ARIA radio group; ↑↓ arrow keys within group; Tab moves focus in/out |
| Mode state management | `useReducer` replacing 6+ scattered `setMode()` calls |
| Form state consolidation | Group 14 `useState` calls into 3 logical objects before adding more |
| TUI removal | Phased: audit → port navigation handlers → delete → go mod tidy |

---

## Dependency Visualization

```
Phase 1: Architecture Foundation (must ship first — all other phases depend on it)
  Story 1.1: Mode state machine (useReducer)
  Story 1.2: Action registry types

Phase 2: Keyboard Gaps (independent of Phase 3–4; can start after Phase 1)
  Story 2.1: scrollIntoView          ← no dependencies; quick win
  Story 2.2: new/ prefix detector    ← depends on 1.1 (mode transitions)
  Story 2.3: Cmd+Shift+K shortcut    ← depends on 1.1 (mode transitions)
  Story 2.4: Mode badge UI           ← depends on 1.1 (reads mode state)

Phase 3: Inline Creation Panel (depends on Phase 1)
  Story 3.1: OmnibarCreationPanel extraction
  Story 3.2: Session type radio group
  Story 3.3: Branch pre-fill + program smart default

Phase 4: Action Registry Integration (depends on Phases 1 + 3)
  Story 4.1: Register 5 initial actions + dispatch
  Story 4.2: Clone action
  Story 4.3: Session list cleanup (remove fork/duplicate)

Phase 5: TUI Removal (independent; can run in parallel with Phases 2–4)
  Story 5.1: TUI audit
  Story 5.2: Delete BubbleTea code
  Story 5.3: Cleanup go.mod + CI + docs
```

---

## Phase 1: Architecture Foundation

### Epic 1.1: Mode State Machine

**Goal**: Replace 6+ scattered `setMode()` calls in `Omnibar.tsx` with a single `useReducer` that owns all mode transitions and their associated data.

#### Story 1.1.1: Define mode state types and reducer

**As a** developer, **I want** all mode transitions in one place, **so that** adding new modes doesn't require hunting for every `setMode()` call.

**Acceptance Criteria**:
- `useModeReducer` hook exported from `web-app/src/lib/omnibar/modes/useModeReducer.ts`
- States: `discovery`, `creation`, `creation_with_repo`
- All 6 current `setMode()` calls in `Omnibar.tsx` replaced with `dispatch()`
- TypeScript: exhaustive switch in reducer (no implicit `any`)
- Existing tests still pass after refactor

**Files**:
- `web-app/src/lib/omnibar/modes/useModeReducer.ts` (NEW)
- `web-app/src/components/sessions/Omnibar.tsx` (MODIFY — mode state)

##### Task 1.1.1a: Create useModeReducer hook (~3 min)
```typescript
// web-app/src/lib/omnibar/modes/useModeReducer.ts

export type OmnibarModeState =
  | { type: "discovery" }
  | { type: "creation"; detection: DetectionResult }
  | { type: "creation_with_repo"; path: string; detection?: DetectionResult };

export type ModeAction =
  | { kind: "detect"; detection: DetectionResult }
  | { kind: "select_repo"; path: string }
  | { kind: "open_creation_direct" }  // Cmd+Shift+K triggered
  | { kind: "new_prefix_typed" }       // user typed "new/"
  | { kind: "reset_to_discovery" };

export function modeReducer(state: OmnibarModeState, action: ModeAction): OmnibarModeState {
  switch (action.kind) {
    case "detect":
      const isCreationType = [
        InputType.LocalPath, InputType.PathWithBranch,
        InputType.GitHubPR, InputType.GitHubBranch,
        InputType.GitHubRepo, InputType.GitHubShorthand,
      ].includes(action.detection.type);
      return isCreationType ? { type: "creation", detection: action.detection } : { type: "discovery" };
    case "select_repo":
      return { type: "creation_with_repo", path: action.path };
    case "open_creation_direct":
      return { type: "creation" };
    case "new_prefix_typed":
      return { type: "creation_with_repo", path: "" };
    case "reset_to_discovery":
      return { type: "discovery" };
  }
}

export function useModeReducer() {
  return useReducer(modeReducer, { type: "discovery" });
}
```
Files: `web-app/src/lib/omnibar/modes/useModeReducer.ts`

##### Task 1.1.1b: Replace useState mode in Omnibar.tsx (~4 min)
- Remove `const [mode, setMode] = useState<OmnibarMode>("discovery")`
- Add `const [modeState, dispatchMode] = useModeReducer()`
- Replace all `setMode("discovery")` → `dispatchMode({ kind: "reset_to_discovery" })`
- Replace all `setMode("creation")` in detection effect → `dispatchMode({ kind: "detect", detection: result })`
- Replace `setMode("discovery")` in empty-input branch → `dispatchMode({ kind: "reset_to_discovery" })`
- Replace `setMode("discovery")` in Escape handler → `dispatchMode({ kind: "reset_to_discovery" })`
- Replace `handleRepoSelect → setInput + setMode("creation")` → `dispatchMode({ kind: "select_repo", path })`
- Derive `isDiscoveryMode` from `modeState.type === "discovery"`
- On reset: clear all form fields (sessionName, branch, etc.) inside the reducer dispatch handler with a `useEffect` on `modeState.type`

Files: `web-app/src/components/sessions/Omnibar.tsx`

---

#### Story 1.1.2: Consolidate form state

**As a** developer, **I want** related form fields grouped, **so that** adding the inline creation panel doesn't push useState count above 25.

**Acceptance Criteria**:
- Form fields (sessionName, branch, program, category, autoYes, useTitleAsBranch, sessionType, existingWorktree, workingDir) grouped into one `formState` object
- UI state (showAdvanced, dropdownIndex, dropdownDismissed, resultHighlightIndex) grouped into one `uiState` object
- Total `useState` calls in Omnibar.tsx reduced from 18 to ≤ 6

**Files**:
- `web-app/src/components/sessions/Omnibar.tsx` (MODIFY)

##### Task 1.1.2a: Group form state (~3 min)
```typescript
interface OmnibarFormState {
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
  sessionName: "", branch: "", program: "claude", category: "",
  autoYes: false, useTitleAsBranch: true, sessionType: "new_worktree",
  existingWorktree: "", workingDir: "",
};

const [formState, setFormState] = useState<OmnibarFormState>(INITIAL_FORM_STATE);
// Helper: setFormField("sessionName", value) updates one field
const setFormField = <K extends keyof OmnibarFormState>(key: K, value: OmnibarFormState[K]) =>
  setFormState(prev => ({ ...prev, [key]: value }));
```
Files: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 1.1.2b: Group UI state (~2 min)
```typescript
interface OmnibarUIState {
  showAdvanced: boolean;
  dropdownIndex: number;
  dropdownDismissed: boolean;
  resultHighlightIndex: number;
}
const [uiState, setUIState] = useState<OmnibarUIState>({
  showAdvanced: false, dropdownIndex: -1, dropdownDismissed: false, resultHighlightIndex: -1,
});
const setUIField = <K extends keyof OmnibarUIState>(key: K, value: OmnibarUIState[K]) =>
  setUIState(prev => ({ ...prev, [key]: value }));
```
Files: `web-app/src/components/sessions/Omnibar.tsx`

---

### Epic 1.2: Action Registry Foundation

**Goal**: Define the typed action system. No UI changes yet — this is the architectural scaffold that Phases 3 and 4 build on.

#### Story 1.2.1: Action type definitions

**As a** developer, **I want** a compile-enforced action type, **so that** new actions cannot be silently omitted from the dispatcher.

**Acceptance Criteria**:
- `OmnibarAction` discriminated union exported from `lib/omnibar/actions/types.ts`
- `dispatchOmnibarAction()` function with exhaustive switch
- TypeScript strict mode: adding a new variant without a case = compile error
- No runtime behavior yet (Phase 4 wires actual handlers)

**Files**:
- `web-app/src/lib/omnibar/actions/types.ts` (NEW)
- `web-app/src/lib/omnibar/actions/dispatch.ts` (NEW)

##### Task 1.2.1a: Create action types (~2 min)
```typescript
// web-app/src/lib/omnibar/actions/types.ts
import { SessionType } from "@/gen/session/v1/types_pb";

export type OmnibarAction =
  | { type: "navigate_session"; sessionId: string; label: string }
  | { type: "create_session"; path: string; sessionType: string; branch?: string; program?: string }
  | { type: "clone_session"; sourceSessionId: string; label: string }
  | { type: "pause_session"; sessionId: string; label: string }
  | { type: "resume_session"; sessionId: string; label: string }
  | { type: "delete_session"; sessionId: string; label: string };

export type OmnibarActionType = OmnibarAction["type"];
```

##### Task 1.2.1b: Create dispatcher with exhaustive switch (~2 min)
```typescript
// web-app/src/lib/omnibar/actions/dispatch.ts
export interface ActionDeps {
  navigate: (sessionId: string) => void;
  createSession: (data: OmnibarSessionData) => Promise<void>;
  pauseSession: (id: string) => Promise<void>;
  resumeSession: (id: string) => Promise<void>;
  deleteSession: (id: string) => Promise<void>;
  close: () => void;
}

export function dispatchOmnibarAction(action: OmnibarAction, deps: ActionDeps): void {
  switch (action.type) {
    case "navigate_session":
      deps.navigate(action.sessionId);
      deps.close();
      return;
    case "create_session":
      void deps.createSession({ path: action.path, sessionType: action.sessionType, branch: action.branch, program: action.program ?? "claude" } as OmnibarSessionData);
      deps.close();
      return;
    case "clone_session":
      void deps.createSession({ /* pre-filled from source session */ } as OmnibarSessionData);
      deps.close();
      return;
    case "pause_session":
      void deps.pauseSession(action.sessionId);
      deps.close();
      return;
    case "resume_session":
      void deps.resumeSession(action.sessionId);
      deps.close();
      return;
    case "delete_session":
      void deps.deleteSession(action.sessionId);
      deps.close();
      return;
    // TypeScript exhaustiveness: if new action type added without case → compile error ✅
  }
}
```
Files: `web-app/src/lib/omnibar/actions/dispatch.ts`

---

## Phase 2: Keyboard Navigation Gaps

### Epic 2.1: ScrollIntoView for Result List

**Goal**: Ensure that arrow-key navigation always keeps the highlighted item visible in the result list viewport.

#### Story 2.1.1: Add scrollIntoView to OmnibarResultList

**As a** user, **I want** the highlighted result to scroll into view, **so that** I can navigate a long result list without the mouse.

**Acceptance Criteria**:
- When `highlightedIndex` changes, the corresponding `role="option"` element calls `scrollIntoView({ behavior: 'instant', block: 'nearest' })`
- No scroll occurs when `highlightedIndex < 0` (no highlight)
- Works when the list has overflow (more than ~5 visible items)

**Files**:
- `web-app/src/components/sessions/OmnibarResultList.tsx` (MODIFY)

##### Task 2.1.1a: Add useEffect for scrollIntoView (~2 min)
```typescript
// Inside OmnibarResultList, add:
const highlightedRef = useRef<HTMLLIElement | null>(null);

useEffect(() => {
  if (highlightedRef.current) {
    highlightedRef.current.scrollIntoView({ behavior: 'instant', block: 'nearest' });
  }
}, [highlightedIndex]);

// On each list item, add: ref={isHighlighted ? highlightedRef : undefined}
```
Files: `web-app/src/components/sessions/OmnibarResultList.tsx`

---

### Epic 2.2: `new/` Prefix Detector

**Goal**: Typing `new/` in the omnibar input forces creation mode, giving users an in-band keyboard alternative to `Cmd+Shift+K`.

#### Story 2.2.1: NewSessionDetector in detector registry

**As a** user, **I want** typing `new/` to immediately switch to session creation mode, **so that** I can start creating without using a shortcut key I might forget.

**Acceptance Criteria**:
- `new/` prefix (case-insensitive) detected before `SessionSearchDetector`
- Detection result: `InputType.NewSession` with `parsedValue` = text after `new/`
- `new/stapler` → creation mode with "stapler" as the search query for repo selection
- `new/` alone → creation mode with empty query
- Detector does NOT match paths starting with `new/` that also match `LocalPathDetector` (priority ordering handles this)

**Files**:
- `web-app/src/lib/omnibar/types.ts` (MODIFY — add `InputType.NewSession`)
- `web-app/src/lib/omnibar/detector.ts` (MODIFY — add `NewSessionDetector` at priority 150)
- `web-app/src/lib/omnibar/detector.test.ts` (MODIFY — add tests)
- `web-app/src/components/sessions/Omnibar.tsx` (MODIFY — handle `InputType.NewSession` in detection effect)

##### Task 2.2.1a: Add InputType.NewSession (~1 min)
```typescript
// web-app/src/lib/omnibar/types.ts — add to InputType enum:
NewSession = "new_session",

// Add to INPUT_TYPE_INFO:
[InputType.NewSession]: {
  label: "New Session",
  icon: "✨",
  description: "Create a new session",
},
```

##### Task 2.2.1b: Add NewSessionDetector (~2 min)
```typescript
// web-app/src/lib/omnibar/detector.ts — add after LocalPathDetector (priority 100):
class NewSessionDetector implements Detector {
  name = "NewSession";
  priority = 150;

  detect(input: string): DetectionResult | null {
    const lower = input.toLowerCase();
    if (!lower.startsWith("new/")) return null;
    const query = input.slice(4); // text after "new/"
    return {
      type: InputType.NewSession,
      confidence: 0.95,
      parsedValue: query,
      suggestedName: query || "",
    };
  }
}
// Register in createDefaultRegistry()
```

##### Task 2.2.1c: Handle NewSession in Omnibar detection effect (~2 min)
```typescript
// In Omnibar.tsx detection useEffect, add to the "isCreationType" check:
// InputType.NewSession → dispatch({ kind: "new_prefix_typed" })
// AND display a filtered repo list using parsedValue as search query
if (result.type === InputType.NewSession) {
  dispatchMode({ kind: "new_prefix_typed" });
  // If parsedValue is non-empty, filter repo entries by parsedValue
}
```
Files: All 4 listed above.

---

### Epic 2.3: Cmd+Shift+K Shortcut for Direct Creation Mode

**Goal**: `Cmd+Shift+K` opens the omnibar in creation mode directly, bypassing the discovery list. Stays in the `Cmd+K` shortcut family the app already uses.

#### Story 2.3.1: Cmd+Shift+K global listener in OmnibarContext

**As a** user, **I want** to press `Cmd+Shift+K` to immediately open the session creation form, **so that** I can create a session without first searching — using the same shortcut family I already know.

**Acceptance Criteria**:
- `Cmd+Shift+K` (metaKey + shiftKey + K) from anywhere in the app opens omnibar in creation mode
- If omnibar is already open in discovery mode, switches to creation mode (does not close)
- `e.preventDefault()` called; existing `Cmd+K` handler unaffected
- Mode badge tooltip shows: `Cmd+K` for Jump mode, `Cmd+Shift+K` for Create mode

**Files**:
- `web-app/src/components/sessions/OmnibarContext.tsx` (MODIFY — add Cmd+Shift+K listener)

##### Task 2.3.1a: Add Cmd+Shift+K to global keydown listener (~3 min)
```typescript
// In OmnibarContext.tsx, in the existing global keydown useEffect:
// Add BEFORE the existing Cmd+K check:
if (e.metaKey && e.shiftKey && e.key === "K") {
  e.preventDefault();
  e.stopPropagation();
  openInCreationMode(); // sets isOpen=true, initialMode="creation"
  return;
}

// Add openInCreationMode to context value and OmnibarProvider state:
const [initialMode, setInitialMode] = useState<"discovery" | "creation">("discovery");
const openInCreationMode = () => {
  setInitialMode("creation");
  setIsOpen(true);
};

// Omnibar.tsx: accept optional initialMode prop; on open useEffect,
// if initialMode === "creation", dispatch({ kind: "open_creation_direct" })
```
Files: `web-app/src/components/sessions/OmnibarContext.tsx`, `web-app/src/components/sessions/Omnibar.tsx`

---

### Epic 2.4: Mode Badge UI

**Goal**: A visible "Jump | Create" indicator shows the current mode and is clickable to toggle.

#### Story 2.4.1: Mode badge component

**As a** user, **I want** to see which mode the omnibar is in at a glance, **so that** I don't accidentally create a session when I meant to navigate.

**Acceptance Criteria**:
- Badge appears inside the modal, left of or below the input
- "Jump" label when in discovery mode; "Create" label when in creation mode
- Clicking badge toggles mode (discovery ↔ creation)
- Badge shows `Cmd+Shift+K` shortcut hint in a `title` attribute tooltip
- Badge styled with vanilla-extract; uses theme tokens (`vars.color.primary` for active mode)

**Files**:
- `web-app/src/components/sessions/OmnibarModeBadge.tsx` (NEW)
- `web-app/src/components/sessions/OmnibarModeBadge.css.ts` (NEW)
- `web-app/src/components/sessions/Omnibar.tsx` (MODIFY — render badge)

##### Task 2.4.1a: Create OmnibarModeBadge component (~3 min)
```tsx
// OmnibarModeBadge.tsx
interface OmnibarModeBadgeProps {
  mode: "discovery" | "creation";
  onToggle: () => void;
}
export function OmnibarModeBadge({ mode, onToggle }: OmnibarModeBadgeProps) {
  return (
    <div className={badgeContainer} role="group" aria-label="Omnibar mode">
      <button
        className={`${badgeButton} ${mode === "discovery" ? badgeActive : ""}`}
        onClick={mode !== "discovery" ? onToggle : undefined}
        aria-pressed={mode === "discovery"}
        title="Jump to existing session (Cmd+K)"
      >Jump</button>
      <button
        className={`${badgeButton} ${mode === "creation" ? badgeActive : ""}`}
        onClick={mode !== "creation" ? onToggle : undefined}
        aria-pressed={mode === "creation"}
        title="Create new session (Cmd+Shift+K)"
      >Create</button>
    </div>
  );
}
```

##### Task 2.4.1b: Style the mode badge (~2 min)
```typescript
// OmnibarModeBadge.css.ts — vanilla-extract
export const badgeContainer = style({
  display: 'flex', borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  overflow: 'hidden', flexShrink: 0,
});
export const badgeButton = style({
  padding: `${vars.space[1]} ${vars.space[2]}`,
  fontSize: vars.fontSize.xs, fontWeight: 500,
  background: 'transparent', color: vars.color.textMuted,
  cursor: 'pointer', border: 'none',
  selectors: { '&:hover': { background: vars.color.hoverBackground } },
});
export const badgeActive = style({
  background: vars.color.primary, color: vars.color.textInverse, cursor: 'default',
});
```
Files: both new files + Omnibar.tsx.

---

## Phase 3: Inline Creation Panel

### Epic 3.1: Extract OmnibarCreationPanel

**Goal**: Move the 200+ line creation form out of `Omnibar.tsx` into a self-contained component that receives form state and callbacks as props. Omnibar.tsx renders it in creation mode.

#### Story 3.1.1: Extract creation form to OmnibarCreationPanel

**As a** developer, **I want** the creation form isolated, **so that** I can test it independently and swap it with different panel types in Phase 4.

**Acceptance Criteria**:
- `OmnibarCreationPanel.tsx` receives `formState` + `setFormField` + `onSubmit` + `onCancel` as props
- Full form (name, type, branch, program, category, auto-yes, advanced) migrated
- `Omnibar.tsx` renders `<OmnibarCreationPanel>` only when `modeState.type !== "discovery"`
- All existing form behavior preserved (validation, Cmd+Enter submit, worktree suggestions)
- Creation panel tests can mount the component without the full Omnibar

**Files**:
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (NEW)
- `web-app/src/components/sessions/OmnibarCreationPanel.css.ts` (NEW — copy/move relevant styles)
- `web-app/src/components/sessions/Omnibar.tsx` (MODIFY — remove inline form JSX, render panel)

##### Task 3.1.1a: Create OmnibarCreationPanel shell (~3 min)
- Create new file with props interface matching all form fields currently in Omnibar.tsx
- Copy the form JSX body (lines ~714–905 in Omnibar.tsx) into the new component
- Export as `OmnibarCreationPanel`

##### Task 3.1.1b: Wire panel into Omnibar.tsx (~3 min)
```tsx
// Omnibar.tsx render section:
{modeState.type !== "discovery" && (
  <OmnibarCreationPanel
    formState={formState}
    setFormField={setFormField}
    onSubmit={handleSubmit}
    onCancel={() => dispatchMode({ kind: "reset_to_discovery" })}
    detection={modeState.type === "creation" ? modeState.detection : undefined}
    worktreeSuggestions={worktreeSuggestions}
    isSubmitting={isSubmitting}
    error={error}
    showAdvanced={uiState.showAdvanced}
    onToggleAdvanced={() => setUIField("showAdvanced", !uiState.showAdvanced)}
  />
)}
```

---

### Epic 3.2: Compact Fast-Path Creation Panel

**Goal**: The `OmnibarCreationPanel` shows a **compact view** by default — only the 3 most important fields (session type, branch, program). Full form hidden behind "Advanced" disclosure. This is the "≤2 keystrokes after repo selection" UX.

#### Story 3.2.1: Compact panel with ARIA radio group for session type

**As a** user, **I want** to create a session by pressing Enter immediately after selecting a repo, **so that** I don't have to fill out a long form.

**Acceptance Criteria**:
- Compact view shows: session type radio group + branch field (if new_worktree) + submit button
- Session type radio group: ↑↓ arrow keys cycle options; Tab moves focus in/out
- Smart defaults: session type = "new_worktree", branch = session name, program = "claude"
- "Advanced" disclosure toggle shows full form (name, category, auto-yes, program, working dir)
- Pressing Enter when focus is on any compact panel field submits the form
- Panel has smooth height animation: `max-height` transition when switching compact ↔ advanced

**Files**:
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (MODIFY)
- `web-app/src/components/sessions/OmnibarCreationPanel.css.ts` (MODIFY)

##### Task 3.2.1a: Session type radio group (~4 min)
```tsx
// In OmnibarCreationPanel.tsx:
const SESSION_TYPES = [
  { value: "new_worktree", label: "New Worktree", shortcut: "W" },
  { value: "directory", label: "Directory", shortcut: "D" },
  { value: "existing_worktree", label: "Use Worktree", shortcut: "U" },
] as const;

function SessionTypeRadioGroup({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const currentIndex = SESSION_TYPES.findIndex(t => t.value === value);

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      const next = (currentIndex + 1) % SESSION_TYPES.length;
      onChange(SESSION_TYPES[next].value);
    }
    if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      const prev = (currentIndex - 1 + SESSION_TYPES.length) % SESSION_TYPES.length;
      onChange(SESSION_TYPES[prev].value);
    }
  }

  return (
    <div role="radiogroup" aria-label="Session type" onKeyDown={handleKeyDown}>
      {SESSION_TYPES.map((type, i) => (
        <button
          key={type.value}
          role="radio"
          aria-checked={value === type.value}
          tabIndex={value === type.value ? 0 : -1}
          onClick={() => onChange(type.value)}
          className={`${radioBtn} ${value === type.value ? radioBtnActive : ""}`}
        >
          {type.label}
        </button>
      ))}
    </div>
  );
}
```
Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`, `OmnibarCreationPanel.css.ts`

##### Task 3.2.1b: Compact vs. advanced layout (~3 min)
- Default: compact view (session type + branch)
- `showAdvanced` toggle reveals: session name, category, program, auto-yes, working dir
- Use `max-height: 0` → `max-height: 400px` transition for smooth reveal
- `overflow: hidden` on the advanced section container

---

### Epic 3.3: Smart Defaults for Creation Panel

**Goal**: When a repo is pre-selected from the result list, the creation panel pre-fills sensible defaults so the user can press Enter immediately.

#### Story 3.3.1: Pre-fill branch from session name and program from history

**As a** user, **I want** smart defaults when I create a session from a known repo, **so that** I can press Enter without touching any form fields.

**Acceptance Criteria**:
- When `modeState.type === "creation_with_repo"`, the panel shows the pre-selected path in a read-only display above the form
- Branch is pre-filled with a slugified version of the session name (or left blank if name is empty)
- Program pre-filled with "claude" (default; no history-based inference in MVP)
- Session name left empty (user optionally types it; branch uses "auto" if name is empty)
- If Enter is pressed without touching the form, a valid session is created with auto-generated name

**Files**:
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (MODIFY)
- `web-app/src/components/sessions/Omnibar.tsx` (MODIFY — pass path to panel)

##### Task 3.3.1a: Path display + auto-branch logic (~3 min)
```tsx
// In OmnibarCreationPanel:
// If path prop is provided (creation_with_repo mode):
// Show: <div className={pathDisplay}>{truncatePath(path)}</div>
// Auto-branch: if useTitleAsBranch && sessionName, show sessionName slug
// If sessionName empty, branch field is optional (server generates name)

function slugify(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}

// useEffect: when sessionName changes and useTitleAsBranch, update branch to slug
useEffect(() => {
  if (formState.useTitleAsBranch) {
    setFormField("branch", slugify(formState.sessionName));
  }
}, [formState.sessionName, formState.useTitleAsBranch]);
```

---

## Phase 4: Action Registry Integration

### Epic 4.1: Wire Action Registry into OmnibarContext

**Goal**: The 5 initial actions (navigate, create, pause, resume, delete) are dispatched through `dispatchOmnibarAction()` rather than hardcoded callbacks.

#### Story 4.1.1: Wire dispatch into OmnibarContext

**As a** developer, **I want** session mutations going through the action dispatcher, **so that** the architecture enforces registration of new actions.

**Acceptance Criteria**:
- `OmnibarContext.tsx` creates `ActionDeps` object from session service and router
- `handleSessionSelect` in Omnibar calls `dispatchOmnibarAction({ type: "navigate_session", ... }, deps)` not `onNavigateToSession(id)`
- `handleSubmit` in Omnibar calls `dispatchOmnibarAction({ type: "create_session", ... }, deps)` not `onCreateSession(data)` directly
- Existing behavior unchanged (sessions still created, navigation still works)

**Files**:
- `web-app/src/components/sessions/OmnibarContext.tsx` (MODIFY)
- `web-app/src/components/sessions/Omnibar.tsx` (MODIFY — replace direct callbacks)

##### Task 4.1.1a: Build ActionDeps in OmnibarContext (~3 min)
```typescript
// OmnibarContext.tsx
const sessionService = useSessionService();
const router = useRouter();

const actionDeps: ActionDeps = useMemo(() => ({
  navigate: (sessionId: string) => router.push(`?session=${sessionId}`),
  createSession: (data) => sessionService.createSession(mapToRequest(data)),
  pauseSession: (id) => sessionService.pauseSession(id),
  resumeSession: (id) => sessionService.resumeSession(id),
  deleteSession: (id) => sessionService.deleteSession(id),
  close: () => setIsOpen(false),
}), [sessionService, router]);

// Pass actionDeps down to Omnibar as a prop
```

---

### Epic 4.2: Clone Session Action

**Goal**: "Clone session" is a registered omnibar action that creates a new session on the same repo as an existing session, with the same program. Replaces the confusing fork/duplicate/clone options in the session list.

#### Story 4.2.1: Clone action implementation

**As a** user, **I want** to clone a session from the omnibar, **so that** I can spin up a parallel session on the same repo in one action.

**Acceptance Criteria**:
- Searching for an existing session in discovery mode shows a "Clone" secondary action on each result
- Selecting clone → opens creation panel pre-filled with source session's path + program
- "Clone" triggers `dispatchOmnibarAction({ type: "clone_session", sourceSessionId })`
- Backend: calls existing `createSession` with same path + program as source (NOT `ForkSession` RPC — clone is a fresh session, not a state fork)
- `OmnibarSessionResult.tsx` shows clone button on hover/keyboard focus

**Files**:
- `web-app/src/components/sessions/OmnibarSessionResult.tsx` (MODIFY — add clone button)
- `web-app/src/lib/omnibar/actions/dispatch.ts` (MODIFY — implement clone_session case)
- `web-app/src/components/sessions/Omnibar.tsx` (MODIFY — handle clone action)

##### Task 4.2.1a: Add clone button to OmnibarSessionResult (~3 min)
```tsx
// OmnibarSessionResult.tsx — add to row:
<button
  className={cloneButton}
  onClick={(e) => { e.stopPropagation(); onClone(session); }}
  aria-label={`Clone session ${session.title}`}
  tabIndex={isHighlighted ? 0 : -1}
  title="Clone this session"
>⊕</button>
```

##### Task 4.2.1b: Implement clone_session dispatch (~2 min)
```typescript
// dispatch.ts — clone_session case:
case "clone_session":
  // Look up source session from the sessions store
  // (deps needs a getSession fn or the full session object)
  void deps.createSession({
    path: sourceSession.path,
    program: sourceSession.program,
    sessionType: "new_worktree",
    title: `${sourceSession.title} (clone)`,
  } as OmnibarSessionData);
  deps.close();
  return;
```

---

### Epic 4.3: Session List Cleanup

**Goal**: Remove the redundant fork/duplicate/clone options from session cards. Consolidate to 4 actions: Open, Pause/Resume, Clone, Delete.

#### Story 4.3.1: Consolidate session card actions

**As a** user, **I want** exactly 4 session card actions, **so that** I'm not confused by overlapping fork/clone/duplicate options.

**Acceptance Criteria**:
- Session card actions: Open, Pause/Resume, Clone, Delete (4 total)
- "Fork from Checkpoint" removed from session card context menu (still accessible if needed via future advanced menu)
- "Duplicate" button removed
- "Clone" button calls `onClone(session)` which opens omnibar in clone mode
- Backend `ForkSession` RPC **not** removed — only UI removed
- `session_service_fork_test.go` still passes (backend untouched)

**Files**:
- `web-app/src/components/sessions/SessionCard.tsx` (MODIFY — remove fork/duplicate, add clone)
- Wherever `handleForkFromCheckpoint` / `handleDuplicateSession` are called in `web-app/src/app/page.tsx` or similar (MODIFY — remove)

##### Task 4.3.1a: Remove fork/duplicate from SessionCard (~3 min)
- Remove `onForkFromCheckpoint` prop
- Remove `onDuplicate` prop
- Remove fork/duplicate action buttons from the card JSX
- Add `onClone` prop that opens the omnibar with clone action pre-filled

##### Task 4.3.1b: Remove handler wiring from page.tsx (~2 min)
- Remove `handleForkFromCheckpoint` function
- Remove `handleDuplicateSession` function
- Add `handleCloneSession` → `openOmnibarWithClone(sessionId)`

---

## Phase 5: TUI Removal

### Epic 5.1: TUI Audit

**Goal**: Identify every file that must change before TUI code can be safely deleted.

#### Story 5.1.1: Systematic tea import audit

**As a** developer, **I want** a complete list of TUI-only files, **so that** the deletion phase has no surprises.

**Acceptance Criteria**:
- Every `.go` file with a `bubbletea` or `tea.` import listed
- Each file categorized: TUI-only (delete) vs. mixed (must extract non-TUI parts first)
- CI/CD pipeline targets that reference TUI identified
- Documentation (CLAUDE.md, README) TUI references listed
- No code changes in this story — audit only

**Files**:
- No files modified — research output only (can be noted in a comment or summary)

##### Task 5.1.1a: Run tea import audit (~3 min)
```bash
# Run in project root:
grep -rn "bubbletea\|charmbracelet\|tea\." --include="*.go" . | grep -v "_test.go" | sort
grep -rn "bubbletea\|charmbracelet\|tea\." --include="*.go" . | grep "_test.go" | sort
grep -rn "TUI\|tui\|BubbleTea\|bubbletea" Makefile .github/workflows/ docs/ CLAUDE.md
```
Output: classified list of files to delete vs. files to surgically modify.

---

### Epic 5.2: Delete BubbleTea Code

**Goal**: Remove all TUI-specific Go code identified in the audit.

#### Story 5.2.1: Delete cmd/ TUI handler files

**As a** developer, **I want** the BubbleTea dependency removed, **so that** the codebase only has the web server path.

**Acceptance Criteria**:
- All files in `cmd/commands/` that only serve TUI handlers deleted
- `testutil/teatest_helpers.go` and `testutil/teatest_test.go` deleted
- `terminal/signals.go` and `terminal/size.go` reviewed — if they import tea, tea dependency removed or file deleted
- `cmd/migration.go` reviewed — tea reference removed if present
- `go build .` succeeds after deletions
- `go test ./...` still passes (no compilation errors from removed files)

**Files**: All files identified in Epic 5.1 audit (exact list produced at audit time)

##### Task 5.2.1a: Delete TUI-only Go files (~4 min)
- Delete each file identified as TUI-only in audit
- For mixed files: remove the tea import and the tea-dependent functions, keep the rest

##### Task 5.2.1b: Verify build after deletion (~2 min)
```bash
go build .
go test ./...
# Expect: no compilation errors; no test failures from removed code
```

---

### Epic 5.3: Cleanup and Documentation

**Goal**: Remove BubbleTea from go.mod, clean up CI, update CLAUDE.md.

#### Story 5.3.1: go mod tidy + CI + docs cleanup

**As a** developer, **I want** no remnants of the TUI in the codebase, **so that** future contributors don't get confused by dead references.

**Acceptance Criteria**:
- `go mod tidy` run; `go.mod` and `go.sum` updated; `charmbracelet/bubbletea` no longer listed
- Makefile TUI-related targets removed (if any found in audit)
- CLAUDE.md TUI documentation sections removed; web server sections verified accurate
- README (if any TUI references) updated to reference web UI only

**Files**:
- `go.mod`, `go.sum` (MODIFY via `go mod tidy`)
- `Makefile` (MODIFY — remove TUI targets if any)
- `CLAUDE.md` (MODIFY — remove TUI docs)

##### Task 5.3.1a: go mod tidy (~1 min)
```bash
go mod tidy
grep "bubbletea" go.mod  # should return nothing
```

##### Task 5.3.1b: Update CLAUDE.md (~3 min)
- Remove any TUI-specific sections in "Development Commands"
- Remove keyboard shortcuts that were TUI-specific (j/k navigation, etc.)
- Verify the web server section is accurate

---

## Quality Gates

### Phase 1 gates (before starting Phase 2 or 3)
- `go test ./...` passes
- `npx jest --no-coverage` (all frontend tests) passes
- `make lint` passes
- Omnibar opens, session search works, result navigation works (manual smoke test)

### Phase 2 gates
- `new/` prefix triggers creation mode (manual test: type `new/`, observe mode badge switches to "Create")
- `Cmd+Shift+K` opens omnibar in creation mode (manual test)
- Arrow keys navigate result list and highlighted item scrolls into view

### Phase 3 gates
- Selecting a repo from results pre-fills creation panel path
- Arrow keys cycle session type in radio group; Tab moves focus in/out of group
- `Cmd+Enter` creates a session from the compact panel

### Phase 4 gates
- TypeScript compiles cleanly (no `any` in action dispatch)
- Session card shows exactly 4 actions
- Clone action opens omnibar pre-filled with source session path

### Phase 5 gates
- `go build .` succeeds
- `grep "bubbletea" go.mod` returns nothing
- `go test ./...` passes (no TUI tests fail — they're deleted)

---

## File Reference

| File | Phase | Action |
|---|---|---|
| `web-app/src/lib/omnibar/modes/useModeReducer.ts` | 1 | CREATE |
| `web-app/src/lib/omnibar/actions/types.ts` | 1 | CREATE |
| `web-app/src/lib/omnibar/actions/dispatch.ts` | 1 | CREATE |
| `web-app/src/lib/omnibar/types.ts` | 2 | MODIFY (add NewSession) |
| `web-app/src/lib/omnibar/detector.ts` | 2 | MODIFY (add NewSessionDetector) |
| `web-app/src/lib/omnibar/detector.test.ts` | 2 | MODIFY (add tests) |
| `web-app/src/components/sessions/OmnibarResultList.tsx` | 2 | MODIFY (scrollIntoView) |
| `web-app/src/components/sessions/OmnibarModeBadge.tsx` | 2 | CREATE |
| `web-app/src/components/sessions/OmnibarModeBadge.css.ts` | 2 | CREATE |
| `web-app/src/components/sessions/OmnibarCreationPanel.tsx` | 3 | CREATE |
| `web-app/src/components/sessions/OmnibarCreationPanel.css.ts` | 3 | CREATE |
| `web-app/src/components/sessions/OmnibarSessionResult.tsx` | 4 | MODIFY (clone button) |
| `web-app/src/components/sessions/SessionCard.tsx` | 4 | MODIFY (remove fork/dup) |
| `web-app/src/components/sessions/Omnibar.tsx` | 1,2,3,4 | MAJOR MODIFY |
| `web-app/src/components/sessions/OmnibarContext.tsx` | 2,4 | MODIFY |
| `web-app/src/app/page.tsx` (or equivalent) | 4 | MODIFY (remove handlers) |
| `cmd/commands/*.go` (TUI files) | 5 | DELETE |
| `testutil/teatest_*.go` | 5 | DELETE |
| `go.mod`, `go.sum` | 5 | MODIFY via go mod tidy |
| `CLAUDE.md` | 5 | MODIFY (remove TUI docs) |
