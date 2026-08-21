# Architecture Research: UX Overhaul

## 1. Session Actions API — What's Already Available in Session View

### Session Object in SessionDetail
`SessionDetail` receives a `session: Session` prop (protobuf type from `@/gen/session/v1/types_pb`). The Session object includes:
- `id`, `title`, `status`, `sessionType`, `instanceType`
- `path`, `workingDir`, `branch`, `program`, `category`, `tags`
- `gitWorktree`, `diffStats`, `githubPrUrl`, etc.
- All fields needed for delete/pause/rename/retag actions are present.

**No additional data fetching is needed for session actions.** The session object is already fully hydrated when SessionDetail mounts.

### ConnectRPC Calls Available (via `useSessionService`)
All needed RPCs already exist in `useSessionService`:
| Action | Method | RPC |
|---|---|---|
| Delete | `deleteSession(id, force?)` | `deleteSession` |
| Pause | `pauseSession(id)` | `updateSession` (status=PAUSED) |
| Resume | `resumeSession(id, updates?)` | `updateSession` (status=RUNNING) |
| Rename | `renameSession(id, newTitle)` | `renameSession` |
| Update Tags | `updateSession(id, {tags})` | `updateSession` |
| Switch Workspace | Already in SessionDetail via `WorkspaceSwitchModal` | dedicated RPC |

**Resume requires `ResumeSessionModal`** — the modal that allows updating title+tags before resuming. This component already exists in `web-app/src/components/sessions/ResumeSessionModal.tsx`.

### What Needs to Be Added to SessionDetail
1. `useSessionService()` hook call (currently only `updateSession` is imported for the Info tab edits)
2. Action sheet state: `const [actionSheetOpen, setActionSheetOpen] = useState(false)`
3. Action handlers: `handleDelete`, `handlePause`, `handleResume`, `handleRename`, `handleEditTags`
4. For Resume: render `<ResumeSessionModal>` conditionally
5. For Rename: either inline title editing in the `<h2>` header or a small modal
6. For Tags: reuse the existing `<TagEditor>` component

## 2. Where Should Action Menu State Live?

### Option A: Local state in SessionDetail (Recommended for Milestone 1)
- `actionSheetOpen: boolean` in `SessionDetail` component state
- Sheet opens/closes within the component; no global state needed
- **Pros**: Simple, co-located with the session data, no Redux boilerplate
- **Cons**: State resets if SessionDetail unmounts (acceptable for a modal)

### Option B: Redux slice
- Add `sessionActionsOpen: string | null` (sessionId) to `sessionsSlice`
- Allows opening the action sheet from outside SessionDetail (e.g. from a notification)
- **Overkill for Milestone 1** — the primary entry point is always within SessionDetail

### Option C: Context
- `SessionActionsContext` with `open(sessionId)` / `close()` methods
- Useful if the action sheet needs to be triggered from deep children (e.g. from the terminal toolbar)
- **Worth considering if** the terminal toolbar gets a "session actions" quick button

**Decision**: Local state (`useState`) in `SessionDetail` for Milestone 1. The action handlers call `useSessionService` methods directly. No Redux changes needed.

## 3. ViewportProvider Current Implementation

From `web-app/src/components/providers/ViewportProvider.tsx`:

```
ViewportProvider
├── CSS var bridge (Effect 1)
│   ├── Listens: visualViewport.resize + visualViewport.scroll
│   ├── Writes: --keyboard-height, --viewport-height (to documentElement)
│   └── Wrapped in requestAnimationFrame (correct)
└── Breakpoint state (Effect 2)
    ├── Listens: window.resize
    ├── Breakpoints: mobile (<600px), foldable (600-899px), innerScreen (≥900px)
    └── Exposes: isMobile, isFoldable, isInnerScreen via useViewport()
```

**Current gaps**:
1. The `--keyboard-height` and `--viewport-height` CSS vars are set on `:root` but are **not being consumed** by the main layout. The terminal container in `SessionDetail` uses hardcoded CSS classes, not these variables.
2. The breakpoints (600px / 900px) match the Pixel 9 Pro Fold fold/inner screen sizes per `globals.css` `--breakpoint-fold: 600px` and `--breakpoint-inner: 900px` — these are correct and should not change.
3. `useViewport()` is available but appears unused in `SessionDetail` and `TerminalOutput` — they each do their own `window.matchMedia('(max-width: 768px)')` detection independently.

**Architectural improvement for Milestone 1**: Make `TerminalOutput` and `SessionDetail` consume `useViewport()` instead of their own breakpoint detection. This consolidates mobile logic.

## 4. Terminal Toolbar State — Where It Lives Now

From `TerminalOutput.tsx`:

```
TerminalOutput component state
├── isKeyboardVisible: boolean   ← localStorage persisted ('stapler-squad-mobile-keyboard-visible')
├── mouseMode: 'none' | 'any'    ← defaults to 'any' desktop, 'none' mobile
├── ctrlActive: boolean          ← sticky Ctrl modifier
├── altActive: boolean           ← sticky Alt modifier
├── streamingMode: string        ← not persisted
├── debugMode: boolean           ← localStorage persisted
└── (no expanded/compact toolbar toggle yet — the whole toolbar is always visible)
```

**The toolbar has no expanded/compact mode yet.** All buttons are always shown in a single `styles.actions` div. The requirements call for:
- **Compact mode**: Terminal maximized, toolbar minimized (one-tap to expand)
- **Expanded mode**: Toolbar fully visible with all controls

**Proposed structure for Milestone 1**:
```
TerminalOutput state
└── toolbarExpanded: boolean  ← localStorage persisted ('stapler-squad-toolbar-expanded')
    ├── false (compact): show only [toggle button] + [Reconnect if needed]
    └── true (expanded): show all current toolbar items
```

The toggle button should be a fixed-position icon (hamburger or expand chevron) that is always visible regardless of mode. The `--min-touch-target: 44px` must be applied.

## 5. Layout Architecture for Keyboard-Aware Full-Height Terminal

### Current Structure (SessionDetail)
```
.container (flex column)
├── .header (h2 + ActionBar buttons)
├── .tabs (tab row)
└── .content (flex: 1, contains terminal/diff/vcs/etc.)
    └── <TerminalOutput> (position: absolute fill within pool div)
        ├── .toolbar (flex row)
        └── .terminal (flex: 1, contains xterm Canvas)
```

### Problem
When the iOS keyboard opens:
1. `window.innerHeight` stays at full height (viewport doesn't shrink)
2. `visualViewport.height` shrinks to the area above the keyboard
3. The terminal content overflows behind the keyboard
4. `--keyboard-height` is updated by `ViewportProvider` but nothing *uses* it

### Required Fix (per ADR-001 and ADR-003)
The `SessionDetail` container needs:
```css
height: calc(var(--viewport-height) - var(--header-height));
/* or for fullscreen: */
height: var(--viewport-height);
```
This causes the container to shrink when the keyboard appears (because `--viewport-height` is set to `vv.height` by `ViewportProvider`).

Additionally, the terminal container within `TerminalOutput` needs:
```css
height: 100%;
/* NOT: height: 100vh or 100dvh */
```

This is the "sticky flex" pattern (ADR-003) — let the parent define the height, let children fill it.

## 6. Integration Points

### Redux Store Integration
- `useSessionService()` manages its own Redux dispatch — the action handlers in `SessionDetail` can call `deleteSession`, `pauseSession`, etc. and the Redux store updates automatically via the `useSessionService` hook's dispatch calls.
- Session state updates are propagated via the `watchSessions` stream, so after a pause/delete, the `session` prop passed to `SessionDetail` will update automatically.

### ResumeSessionModal Integration
`ResumeSessionModal` is a standalone component with its own props:
```tsx
<ResumeSessionModal
  session={session}
  onResume={(title, tags) => resumeSession(session.id, {title, tags})}
  onCancel={() => setShowResumeModal(false)}
/>
```
This can be added directly to `SessionDetail` without architectural changes.

### TagEditor Integration  
`TagEditor` component:
```tsx
<TagEditor
  tags={session.tags}
  onSave={(newTags) => updateSession(session.id, {tags: newTags})}
  onCancel={() => setShowTagEditor(false)}
  sessionTitle={session.title}
/>
```

## Summary

- **All needed APIs already exist** — `useSessionService` exports `deleteSession`, `pauseSession`, `resumeSession`, `renameSession`, `updateSession`; the `session` prop in `SessionDetail` has all required fields; no new RPCs needed for Milestone 1.
- **Action state belongs in local `SessionDetail` state** — `useState(false)` for `actionSheetOpen`, `showResumeModal`, `showTagEditor`; no Redux changes required.
- **Toolbar expanded/compact toggle** should be a new `toolbarExpanded` boolean in `TerminalOutput`, persisted to `localStorage`, with the toggle button always visible at 44px+ touch target.
- **Keyboard-aware layout fix**: `SessionDetail.container` needs `height: var(--viewport-height)` (minus header height), and the `TerminalOutput` inner container must use `height: 100%` — the `ViewportProvider` already writes the correct CSS variable, it just needs to be consumed.
