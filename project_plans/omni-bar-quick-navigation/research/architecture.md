# Research: Architecture — Omni Bar Quick Navigation

**Date**: 2026-04-21
**Dimension**: Architecture

---

## Part 1: Codebase Survey

### 1. Current Omnibar Architecture — Detector Pattern

**Location:** `web-app/src/lib/omnibar/detector.ts`

The omnibar uses a **priority-based detector registry** for input classification:

- `Detector` interface (line 8-12): `name`, `priority` (lower = higher precedence), `detect(input): DetectionResult | null`
- `DetectorRegistry` class (lines 283-318): manages detectors, auto-sorts by priority; exposes `detect(input)` and `detectAll(input)`
- Singleton access via `getDefaultRegistry()` and `detect(input)` utilities (lines 334-348)

**Current detectors (7 total):**

| Detector | Priority |
|---|---|
| `GitHubPRDetector` | 10 |
| `GitHubBranchDetector` | 20 |
| `GitHubRepoDetector` | 30 |
| `GitHubShorthandDetector` | 40 |
| `PathWithBranchDetector` | 50 |
| `LocalPathDetector` | 100 |
| `SessionSearchDetector` | 200 (catch-all) |

All 7 are registered statically in `createDefaultRegistry()` (lines 321-331).

**Critical distinction:** The detector pattern classifies *input type* — it does NOT dispatch actions or expose capabilities. Extending it to an action registry is additive, not a replacement.

---

### 2. Redux Store Structure

**Slices** (`web-app/src/lib/store/`):
- `bulkSelectionReducer`
- `reviewQueueReducer`
- `sessionsReducer` — entity adapter; actions: `setSessions`, `upsertSession`, `removeSession`, `setLoading`, `setError`, `setConnectionState`, `updateSessionStatus`
- `connectApi` — RTK Query API reducer

**No `OmnibarSlice` exists.** Omnibar state is entirely local (`useState` in `Omnibar.tsx`). This is the right architecture — omnibar is ephemeral UI state, not domain state.

---

### 3. Session Actions and Mutations

**API hook:** `useSessionService()` (`web-app/src/lib/hooks/useSessionService.ts:37-62`):
- `createSession(request)` → `upsertSession` dispatch
- `updateSession(id, updates)` → `upsertSession` dispatch
- `deleteSession(id)` → `removeSession` dispatch
- `pauseSession(id)` → `updateSession` with `PAUSED` status
- `resumeSession(id, updates?)` → `updateSession` with `RUNNING` status
- `renameSession(id, newTitle)` → `upsertSession` dispatch
- `forkSession(sessionId, checkpointId, newTitle)` → `upsertSession` dispatch

**Pattern:** Method-driven (verb-based), not capability-driven. `pauseSession()` not `sessionCapabilities.pause()`. The action registry should wrap these methods as registered `OmnibarAction` handlers.

---

### 4. How `Omnibar.tsx` Consumes Results

**Architecture:**
- `OmnibarResultList` receives: `sessionResults`, `repoEntries`, `highlightedIndex`, and callbacks (`onSessionSelect`, `onRepoSelect`, `onCreateNew`)
- Results are **stateless presentation** — no behavior of their own
- Session navigation: `onNavigateToSession(sessionId)` → `router.push(?session=id)`
- Repo selection: `onRepoSelect(path)` → fills input, transitions to creation mode
- Creation: `onCreateNew()` → transitions to creation mode

---

### 5. Component Tree and Props

```
OmnibarProvider (owns isOpen, handleCreateSession, handleNavigateToSession)
  └─ Omnibar (owns input, mode, form state)
      ├─ OmnibarResultList (discovery mode: session results + repo results)
      ├─ PathCompletionDropdown (creation mode: file system completion)
      └─ [inline form fields] (creation mode: session name, type, branch, etc.)
```

**Handler wiring** (`OmnibarContext.tsx:78-101`):
- `handleCreateSession()` → `useSessionService().createSession()` → maps `OmnibarSessionData` to protobuf `CreateSessionRequest`
- `handleNavigateToSession()` → `router.push(?session=id)` then `close()`

---

## Part 2: Architecture Design Decisions

### Decision 1: Action Registry Pattern

**Three options evaluated:**

**Option A — Static Registration (Barrel File + Discriminated Union)**
- All actions in one file; TypeScript discriminated union enforces exhaustive handling
- Zero runtime overhead
- Actions visible in one place (IDE intellisense)
- Con: requires editing barrel file to add new actions

**Option B — Dynamic Registration (`registry.register()` module-level)**
- Actions register near their implementation
- Plugin-like; easy to add without central changes
- Con: order-dependent, harder to discover

**Option C — React Context-Based (components register on mount)**
- Aligns with existing `OmnibarContext` usage
- Con: actions unavailable until providers mount; complex prop drilling

**RECOMMENDATION: Option A (Static Discriminated Union)**

Rationale:
1. Codebase already uses strongly-typed Redux slices and discriminated union patterns (`sessionType: "new_worktree" | "directory" | "existing_worktree"`)
2. Omnibar actions are well-scoped — they live in the omnibar module, not scattered globally
3. Parallel to the existing detector registry pattern (also statically registered)
4. < 20 actions expected; no need for dynamic plugin architecture
5. TypeScript compile error on unhanded action type is the "free" architectural guard

```typescript
// lib/omnibar/actions/types.ts
export type OmnibarAction =
  | { type: "navigate_session"; sessionId: string }
  | { type: "create_session"; path: string; sessionType: SessionType }
  | { type: "clone_session"; sourceSessionId: string }
  | { type: "pause_session"; sessionId: string }
  | { type: "resume_session"; sessionId: string }
  | { type: "delete_session"; sessionId: string };

// Exhaustive dispatch — new action types cause compile errors if not handled
function dispatchAction(action: OmnibarAction, services: SessionServices): void {
  switch (action.type) {
    case "navigate_session": return services.navigate(action.sessionId);
    case "create_session": return services.create(action.path, action.sessionType);
    case "clone_session": return services.clone(action.sourceSessionId);
    case "pause_session": return services.pause(action.sessionId);
    case "resume_session": return services.resume(action.sessionId);
    case "delete_session": return services.delete(action.sessionId);
    // Adding new action type without a case here → TypeScript error ✅
  }
}
```

---

### Decision 2: Inline Creation Panel Architecture

**Three options evaluated:**

**Option A — Inline in Omnibar.tsx (status quo)**
- Simple but Omnibar.tsx already 963 lines; form is 200+ lines of JSX

**Option B — Separate `OmnibarCreationPanel.tsx`**
- Cleaner separation of concerns
- Testable in isolation
- Con: prop drilling (detection, form state, handlers)

**Option C — Action-Driven Panel Composition (recommended)**
```typescript
interface CreationPanelAction extends OmnibarAction {
  type: "creation_panel";
  panel: React.ComponentType<CreationPanelProps>;
  validation: (state: FormState) => boolean;
}
```
The Omnibar looks up the active panel from the action registry and renders it.

**RECOMMENDATION: Option C (Action-Driven)**

Rationale:
1. Clone panel, fork panel, and creation panel all have *different* form fields — action-driven composition avoids a giant `if/else` in Omnibar.tsx
2. Each panel declares its own validation and submit logic
3. Aligns with the action registry being built for Decision 1
4. Panels can be tested independently (no full Omnibar mount needed)
5. Keyboard shortcuts can trigger panels: `Cmd+N` → creation panel, `Tab` on repo result → creation panel pre-filled

---

### Decision 3: Mode State Machine

**Current state:** `mode: "discovery" | "creation"` via `useState` with transitions scattered across 6+ locations in `Omnibar.tsx`.

**Three options evaluated:**

**Option A — useState (status quo)**
- Transitions at lines 249, 258, 281, 353, 442, 675 — hard to trace bugs

**Option B — useReducer with Explicit State Machine**
```typescript
type OmnibarModeState =
  | { type: "discovery" }
  | { type: "creation"; detection: DetectionResult; sessionName: string }
  | { type: "creation_with_repo"; path: string };

type ModeAction =
  | { kind: "detect"; detection: DetectionResult }
  | { kind: "open_creation"; detection: DetectionResult }
  | { kind: "select_repo"; path: string }
  | { kind: "reset_to_discovery" };
```

**Option C — External state machine (xstate)**
- New dependency; not aligned with codebase's minimal-dep philosophy

**RECOMMENDATION: Option B (useReducer)**

Rationale:
1. Mode has *associated data* (not just a string): detection result, path context — `useState<string>` loses this
2. All transitions in one place → debug with logging: `dispatch({ kind: "select_repo", path })`
3. Invalid transitions can be guarded (e.g., can't go `discovery → discovery` with a new path)
4. Prepares for future modes: `"clone_session"`, `"fork_session"`
5. Redux/reducer pattern already familiar in codebase

---

## Recommended Architecture

### File Organization

```
web-app/src/lib/omnibar/
├── actions/
│   ├── types.ts              NEW — OmnibarAction discriminated union
│   ├── registry.ts           NEW — ActionRegistry + singleton getter
│   └── handlers.ts           NEW — dispatch logic for each action type
├── modes/
│   └── useModeReducer.ts     NEW — useReducer hook for mode FSM
├── detector.ts               UNCHANGED
├── index.ts                  UNCHANGED
└── types.ts                  UNCHANGED

web-app/src/components/sessions/
├── Omnibar.tsx               REFACTOR — use action registry + mode reducer
├── OmnibarCreationPanel.tsx  NEW — extracted from Omnibar.tsx
├── OmnibarCreationPanel.css.ts  NEW — vanilla-extract styles
├── OmnibarResultList.tsx     UNCHANGED
└── ...
```

### Mode State Machine (Full)

```
          ┌─────────────────────────────────────────────────┐
          │                    DISCOVERY                    │
          │  - fuzzy session search (Fuse.js)               │
          │  - recent repos list                            │
          │  - action badge on each result                  │
          └──────────┬─────────────────────────┬───────────┘
                     │ detect(LocalPath)        │ Tab on repo result
                     │ detect(GitHubXxx)        │ Cmd+N shortcut
                     │ select_repo              │ new/ prefix
                     ▼                          ▼
          ┌─────────────────────────────────────────────────┐
          │                    CREATION                     │
          │  - inline creation panel (compact form)         │
          │  - session type Tab-cycling                     │
          │  - branch pre-fill                              │
          └──────────┬──────────────────────────────────────┘
                     │ Escape
                     ▼
                  DISCOVERY
```

### Integration Points

1. **`OmnibarContext.tsx`**: Wire `dispatchAction(action, services)` with the session service methods. Remove direct `handleCreateSession` prop — actions carry their own handler.

2. **`Omnibar.tsx`**: Replace `const [mode, setMode] = useState` with `const [modeState, dispatchMode] = useModeReducer()`. Replace hardcoded `handleSessionSelect`/`handleRepoSelect` with `dispatchAction`.

3. **`OmnibarResultList.tsx`**: Each result item renders an action badge (Session | Repo | Action) from the registry. Unchanged for now; badge rendering added in a follow-up pass.

4. **`OmnibarCreationPanel.tsx`**: Receives `modeState.detection` + `modeState.path` as props. Owns all form field state internally. Calls `dispatchAction({ type: "create_session", ... })` on submit.
