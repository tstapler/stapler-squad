# Implementation Plan: ux-overhaul

**Feature**: Mobile friction fixes, design system foundation, and UX best-practices alignment for Stapler Squad
**Date**: 2026-04-26
**Status**: Ready for implementation
**ADRs**: ADR-001 (ViewportProvider bridge), ADR-002 (ResizeObserver xterm fit), ADR-003 (sticky flex layout), ADR-004 (mobile keyboard toggle localStorage), ADR-009 (vanilla-extract CSS), ADR-010 (Radix UI primitives), ADR-011 (createThemeContract tokens), ADR-012 (RTK Query protobuf boundary)

---

## Dependency Visualization

```
Milestone 1 — Mobile Friction (ship first, incremental PRs)
│
├── Epic 1.1: Toolbar Toggle ──────────────────────────────────┐
│   └── Story 1.1.1: toolbarExpanded state + toggle button     │
│       └── Story 1.1.2: localStorage persistence              │
│                                                               │
├── Epic 1.2: Keyboard-Aware Layout ──────────────────────────►│
│   ├── Story 1.2.1: globals.css safe-area + dvh              │
│   ├── Story 1.2.2: SessionDetail height → var(--viewport-height)
│   └── Story 1.2.3: 100vh audit → var(--viewport-height)      │
│                                                               │
├── Epic 1.3: Session Name Visibility ────────────────────────►│ (after 1.2.2)
│   └── Story 1.3.1: Sticky header with session name          │
│                                                               │
├── Epic 1.4: Session Actions in Session View ──────────────── ┘
│   ├── Story 1.4.1: Action sheet (⋯ button + Modal bottom sheet)
│   ├── Story 1.4.2: Delete + Pause/Resume in sheet
│   └── Story 1.4.3: Rename + Tag Edit via existing modals
│
├── Epic 1.5: Bottom Navbar Clearance ────────────────────────
│   └── Story 1.5.1: BottomNav safe-area + page padding-bottom
│
└── Epic 1.6: Touch Target Audit ─────────────────────────────
    └── Story 1.6.1: 44px min on mobile keyboard buttons + toolbar

Milestone 2 — Design System Foundation (3–6 month window)
│
├── Epic 2.1: Token Contract
├── Epic 2.2: Primitive Library (Button, Modal, Card, Input, Badge, ActionBar)
├── Epic 2.3: CSS Module Migration (top-10 files)
└── Epic 2.4: Responsive Header

Milestone 3 — UX Best Practices (after M2)
│
├── Epic 3.1: Navigation Patterns Review
├── Epic 3.2: Consistent Action Surface
└── Epic 3.3: Interaction Latency Instrumentation
```

---

## Milestone 1 — Mobile Friction

**Goal**: Ship incremental PRs that eliminate the most painful day-to-day mobile friction points. Each story is independently PR-able. No new npm packages.

---

### Epic 1.1: Terminal Toolbar Compact/Expanded Toggle

**Goal**: Give users a persistent one-tap toggle to maximize terminal space. State survives page reloads.

#### Story 1.1.1: Add `toolbarExpanded` state and toggle button to TerminalOutput
**As a** mobile user, **I want** to collapse the terminal toolbar with one tap, **so that** the terminal occupies the full screen height during active sessions.
**Acceptance Criteria**:
- A toggle button (chevron or hamburger icon) is always visible in the toolbar at ≥44×44px
- When `toolbarExpanded = false`: only the toggle button (and Reconnect button if disconnected) are shown
- When `toolbarExpanded = true`: all current toolbar items are shown (existing behavior)
- Default state is `true` (expanded) for new users
- State is stored in localStorage under key `'stapler-squad-toolbar-expanded'`
- Desktop layout is not affected (toolbar always visible at ≥1024px via CSS media query)
**Files**:
- `web-app/src/components/sessions/TerminalOutput.tsx`
- `web-app/src/components/sessions/TerminalOutput.module.css`

##### Task 1.1.1a: Add `toolbarExpanded` useState with localStorage init (~3 min)
- In `TerminalOutput.tsx`, add `const [toolbarExpanded, setToolbarExpanded] = useState(() => { const s = localStorage.getItem('stapler-squad-toolbar-expanded'); return s === null ? true : s === 'true'; })`
- Add `useEffect` that calls `localStorage.setItem('stapler-squad-toolbar-expanded', String(toolbarExpanded))` when `toolbarExpanded` changes
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.1.1b: Add toggle button to toolbar JSX and conditionally render toolbar items (~4 min)
- In the `styles.toolbar` div, wrap all existing toolbar buttons in `{toolbarExpanded && <div className={styles.toolbarActions}>...</div>}`
- Add a `<button className={styles.toolbarToggle} onClick={() => setToolbarExpanded(v => !v)} aria-label={toolbarExpanded ? 'Collapse toolbar' : 'Expand toolbar'}>` always rendered first in the toolbar
- The button renders a chevron-up icon when expanded and chevron-down when collapsed (use existing icon approach or inline SVG)
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.1.1c: Style the toggle button and compact toolbar mode (~3 min)
- In `TerminalOutput.module.css`, add `.toolbarToggle` with `min-width: 44px; min-height: 44px; display: flex; align-items: center; justify-content: center`
- Add `.toolbarCompact` variant for the toolbar container: when compact, `padding: 4px 8px` (reduced from normal)
- Apply `@media (min-width: 1024px) { .toolbarToggle { display: none; } .toolbarActions { display: flex !important; } }` so desktop always shows full toolbar
- Files: `web-app/src/components/sessions/TerminalOutput.module.css`

---

#### Story 1.1.2: Replace independent mobile detection with `useViewport()` in TerminalOutput
**As a** developer, **I want** TerminalOutput to use the shared `useViewport()` hook, **so that** mobile detection is consistent and not duplicated across components.
**Acceptance Criteria**:
- `TerminalOutput.tsx` no longer does its own `window.matchMedia('(max-width: 768px)')` — it calls `useViewport()`
- The `isMobile` value used to gate mobile keyboard display comes from `useViewport().isMobile`
- No behavior change visible to users
**Files**:
- `web-app/src/components/sessions/TerminalOutput.tsx`
- `web-app/src/components/providers/ViewportProvider.tsx` (read only — no change needed)

##### Task 1.1.2a: Replace local matchMedia with useViewport() (~3 min)
- Import `useViewport` from `@/components/providers/ViewportProvider`
- Replace the `useEffect` + `matchMedia` breakpoint detection with `const { isMobile } = useViewport()`
- Remove the local `isMobile` state and its resize listener
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

---

### Epic 1.2: Keyboard-Aware Layout

**Goal**: When the iOS/Android virtual keyboard opens, the terminal and input remain fully visible. Content does not hide behind the keyboard.

#### Story 1.2.1: Audit and migrate hardcoded `100vh` to `var(--viewport-height)` in globals and page-level CSS
**As a** mobile user, **I want** the app layout to correctly shrink when the keyboard opens, **so that** no interactive elements are hidden behind the keyboard.
**Acceptance Criteria**:
- Zero occurrences of bare `100vh` remain in `globals.css`, `page.module.css`, and layout CSS files
- All height-full declarations use `var(--viewport-height)` or `100dvh` (via the CSS variable)
- `globals.css` root definition `--viewport-height: 100dvh` is the SSR fallback (already present)
- ViewportProvider JS override `--viewport-height: ${vv.height}px` runs client-side (already present)
**Files**:
- `web-app/src/app/globals.css`
- `web-app/src/app/page.module.css`
- Any other `.module.css` / `.css.ts` files identified in audit (see task)

##### Task 1.2.1a: Audit all CSS files for `100vh` occurrences (~2 min)
- Search for `100vh` across `web-app/src/` — expected to find 13 files per requirements
- Document the list of files in a comment at top of this story (do not edit — this task is analysis only)
- Files: (read-only audit)

##### Task 1.2.1b: Migrate `globals.css` 100vh occurrences (~3 min)
- Replace any `height: 100vh` or `min-height: 100vh` in `globals.css` with `height: var(--viewport-height)` or `min-height: var(--viewport-height)`
- Verify the root `:root { --viewport-height: 100dvh; }` definition is present (add if missing)
- Files: `web-app/src/app/globals.css`

##### Task 1.2.1c: Migrate `page.module.css` 100vh occurrences (~3 min)
- Replace `100vh` with `var(--viewport-height)` in `web-app/src/app/page.module.css`
- The `.modal` height rules specifically mentioned in requirements as having iOS issues
- Files: `web-app/src/app/page.module.css`

##### Task 1.2.1d: Migrate remaining layout CSS files (~4 min)
- For each remaining file identified in Task 1.2.1a, replace `100vh` with `var(--viewport-height)`
- Apply `overflow: hidden` on any container that was previously `height: 100vh` to prevent dvh sub-pixel gap (pitfall mitigation)
- Files: (each identified `.module.css` file, up to ~13 files)

---

#### Story 1.2.2: Fix SessionDetail container height to consume `var(--viewport-height)`
**As a** mobile user, **I want** the session view to resize correctly when the keyboard appears, **so that** the terminal scrollback and input row remain accessible.
**Acceptance Criteria**:
- `SessionDetail` container uses `height: calc(var(--viewport-height) - var(--header-height))` (or equivalent) — not a hardcoded `100vh`
- When the iOS keyboard opens, the terminal container shrinks to fit the visible area
- When fullscreen mode is active, `height: var(--viewport-height)` applies
- Desktop layout unchanged
**Files**:
- `web-app/src/components/sessions/SessionDetail.tsx`
- `web-app/src/components/sessions/SessionDetail.module.css` (or `.css.ts`)

##### Task 1.2.2a: Update SessionDetail container height CSS (~3 min)
- In `SessionDetail`'s CSS, change the main `.container` height from any hardcoded value to `height: calc(var(--viewport-height) - var(--header-height))`
- For fullscreen state: `height: var(--viewport-height)` (check existing fullscreen CSS class)
- Files: `web-app/src/components/sessions/SessionDetail.module.css`

##### Task 1.2.2b: Update TerminalOutput inner terminal div to use `height: 100%` (not `100vh`) (~2 min)
- In `TerminalOutput.module.css`, ensure `.terminal` (the xterm wrapper div) uses `height: 100%` not `height: 100vh`
- This lets the parent (`SessionDetail.content`) define the height, and xterm fills it (ADR-003 sticky flex pattern)
- Files: `web-app/src/components/sessions/TerminalOutput.module.css`

---

#### Story 1.2.3: Apply safe-area insets to layout shells
**As a** user on a notched iPhone or Android device with gesture navigation, **I want** the app to never clip content behind the notch or home indicator, **so that** all controls are fully tappable.
**Acceptance Criteria**:
- `globals.css` `--safe-area-*` custom properties are defined (already present — verify)
- `BottomNav` applies `padding-bottom: max(var(--safe-area-bottom), 8px)`
- The main page wrapper applies `padding-top: var(--safe-area-top)` for notch clearance
- Terminal container applies `padding-left: var(--safe-area-left); padding-right: var(--safe-area-right)` for landscape notch
- All `env()` safe-area values use the `max()` pattern for Android 3-button nav fallback
**Files**:
- `web-app/src/app/globals.css`
- `web-app/src/components/layout/BottomNav.tsx` (or `BottomNav.css.ts`)
- `web-app/src/app/layout.tsx`

##### Task 1.2.3a: Verify globals.css safe-area variable definitions (~2 min)
- Confirm `--safe-area-top/bottom/left/right: env(safe-area-inset-*, 0px)` are all defined in `:root`
- Add any missing definitions
- Files: `web-app/src/app/globals.css`

##### Task 1.2.3b: Apply safe-area to BottomNav (~3 min)
- In `BottomNav.css.ts` (or `.module.css`), add `paddingBottom: 'max(env(safe-area-inset-bottom, 0px), 8px)'`
- Ensure `BottomNav` has `position: fixed; bottom: 0` so the padding correctly extends to screen edge
- Files: `web-app/src/components/layout/BottomNav.tsx`, `web-app/src/components/layout/BottomNav.css.ts`

##### Task 1.2.3c: Apply safe-area padding to terminal container for landscape notch (~3 min)
- In `TerminalOutput.module.css`, add to the `.terminalContainer`: `padding-left: var(--safe-area-left); padding-right: var(--safe-area-right)`
- This prevents xterm canvas from extending under the notch in landscape orientation
- Files: `web-app/src/components/sessions/TerminalOutput.module.css`

---

### Epic 1.3: Session Name Visibility

**Goal**: The session name is always visible in the session view header, even with the keyboard open and during mobile use.

#### Story 1.3.1: Persistent session name in SessionDetail header
**As a** mobile user, **I want** to always see which session I am in, **so that** I never lose context when the keyboard is open or when navigating between sessions.
**Acceptance Criteria**:
- The session title is displayed in the `SessionDetail` header as a sticky element
- Title is visible at all times including when iOS keyboard is open (keyboard-aware layout from Epic 1.2 is a prerequisite)
- Session status badge (Running/Paused/etc.) is visible next to the title
- On mobile, the title truncates with ellipsis if too long; full title shown on long-press (title attr) or tap-to-expand
- The header does not shift or resize when the keyboard appears
**Files**:
- `web-app/src/components/sessions/SessionDetail.tsx`
- `web-app/src/components/sessions/SessionDetail.module.css`

##### Task 1.3.1a: Ensure SessionDetail header is position:sticky and not affected by keyboard-height changes (~3 min)
- The `.header` in `SessionDetail` should be `position: sticky; top: 0; z-index: 10`
- Add `flex-shrink: 0` to prevent the header from being squeezed when the container height changes
- Files: `web-app/src/components/sessions/SessionDetail.module.css`

##### Task 1.3.1b: Add session status badge to header if not already present (~3 min)
- In `SessionDetail.tsx` header JSX, ensure `<SessionStatusBadge status={session.status} />` (or equivalent) is rendered next to the `<h2>{session.title}</h2>`
- If no `SessionStatusBadge` component exists, render a simple `<span className={styles.statusBadge}>` with status text and color via CSS
- Files: `web-app/src/components/sessions/SessionDetail.tsx`, `web-app/src/components/sessions/SessionDetail.module.css`

##### Task 1.3.1c: Add title truncation with tooltip on mobile (~2 min)
- Apply `overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: calc(100vw - 120px)` to the `<h2>` (120px reserved for action buttons)
- Add `title={session.title}` attribute for browser tooltip on long-press
- Files: `web-app/src/components/sessions/SessionDetail.module.css`

---

### Epic 1.4: Session Actions in Session View

**Goal**: Users can delete, pause/resume, rename, and update tags from within the session view, without returning to the session list.

#### Story 1.4.1: Session actions bottom sheet triggered by ⋯ button in SessionDetail header
**As a** mobile user, **I want** to access session actions from within the session view, **so that** I can manage sessions without navigating back to the list.
**Acceptance Criteria**:
- A ⋯ (more actions) button appears in `SessionDetail` header, ≥44×44px touch target
- Tapping ⋯ opens a bottom sheet (existing Modal with `globals.css` mobile bottom-sheet behavior)
- The sheet lists: Pause (or Resume), Rename, Edit Tags, Switch Workspace (existing button), Delete
- The sheet title displays the session name
- Tapping outside the sheet or pressing Escape dismisses it
- The sheet appears above the bottom navbar (correct z-index)
- Destructive actions (Delete) are visually separated and shown in red
- For external sessions (`instanceType === EXTERNAL`): Pause/Resume is hidden or disabled
**Files**:
- `web-app/src/components/sessions/SessionDetail.tsx`
- `web-app/src/components/sessions/SessionDetail.module.css`
- `web-app/src/components/ui/Modal.tsx` (or equivalent — read-only, use as-is)

##### Task 1.4.1a: Add action sheet state and ⋯ button to SessionDetail header (~4 min)
- Add `const [actionSheetOpen, setActionSheetOpen] = useState(false)` to `SessionDetail`
- In the header JSX, add `<button className={styles.moreActionsBtn} onClick={() => setActionSheetOpen(true)} aria-label="Session actions">⋯</button>`
- Add `.moreActionsBtn { min-width: 44px; min-height: 44px; ... }` to CSS
- Files: `web-app/src/components/sessions/SessionDetail.tsx`, `web-app/src/components/sessions/SessionDetail.module.css`

##### Task 1.4.1b: Render the action sheet Modal with action list (~5 min)
- Below the main JSX, render:
  ```tsx
  {actionSheetOpen && (
    <Modal title={session.title} onClose={() => setActionSheetOpen(false)}>
      <div className={styles.actionSheet}>
        <button onClick={handlePauseResume}>...</button>
        <button onClick={() => { setActionSheetOpen(false); setShowRenameModal(true); }}>Rename</button>
        <button onClick={() => { setActionSheetOpen(false); setShowTagEditor(true); }}>Edit Tags</button>
        <button onClick={() => { setActionSheetOpen(false); setWorkspaceSwitchOpen(true); }}>Switch Workspace</button>
        <hr className={styles.actionDivider} />
        <button className={styles.destructive} onClick={handleDelete}>Delete</button>
      </div>
    </Modal>
  )}
  ```
- The `globals.css` mobile bottom-sheet override handles positioning automatically
- Files: `web-app/src/components/sessions/SessionDetail.tsx`

##### Task 1.4.1c: Style action sheet list items (~3 min)
- Add to `SessionDetail.module.css`:
  - `.actionSheet`: `display: flex; flex-direction: column; gap: 4px`
  - `.actionSheet button`: `min-height: 52px; padding: 0 16px; text-align: left; font-size: 16px; border-radius: 8px; background: transparent; border: none; cursor: pointer`
  - `.actionSheet button:hover, .actionSheet button:active`: hover/active states
  - `.destructive`: `color: var(--error)` (uses existing CSS var)
  - `.actionDivider`: `border: none; border-top: 1px solid var(--border-color); margin: 8px 0`
- Files: `web-app/src/components/sessions/SessionDetail.module.css`

---

#### Story 1.4.2: Wire Delete and Pause/Resume actions in SessionDetail
**As a** mobile user, **I want** to pause or delete a session from the session view, **so that** I can manage session state without leaving.
**Acceptance Criteria**:
- Pause: calls `pauseSession(session.id)` and closes the sheet; button label changes to "Resume" when `session.status === PAUSED`
- Resume: opens `ResumeSessionModal` (which allows updating title+tags before resuming)
- Delete: shows a confirmation dialog ("Session is running — stop and delete?" for running sessions); calls `deleteSession(session.id)` on confirm; navigates back to session list
- For external sessions: Pause/Resume buttons are not rendered
**Files**:
- `web-app/src/components/sessions/SessionDetail.tsx`
- `web-app/src/components/sessions/ResumeSessionModal.tsx` (read-only, used as-is)

##### Task 1.4.2a: Import useSessionService and add action handlers to SessionDetail (~4 min)
- Add `const { deleteSession, pauseSession, resumeSession } = useSessionService()` to `SessionDetail`
- Add `const [showResumeModal, setShowResumeModal] = useState(false)`
- Add `const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)`
- Implement `handlePauseResume`: if `session.status === PAUSED` then `setShowResumeModal(true)` else `pauseSession(session.id); setActionSheetOpen(false)`
- Implement `handleDelete`: if session is running, `setShowDeleteConfirm(true)` else call `deleteSession(session.id)` directly then navigate to list
- Files: `web-app/src/components/sessions/SessionDetail.tsx`

##### Task 1.4.2b: Render ResumeSessionModal and delete confirmation dialog (~4 min)
- Conditionally render `<ResumeSessionModal session={session} onResume={...} onCancel={...} />` when `showResumeModal`
- Render a small confirmation Modal for delete: "This session is currently running. Stop and delete it?" with Confirm/Cancel buttons
- On confirm delete: call `deleteSession(session.id)` then navigate to `/` or session list
- Files: `web-app/src/components/sessions/SessionDetail.tsx`

---

#### Story 1.4.3: Wire Rename and Edit Tags actions via existing modal components
**As a** mobile user, **I want** to rename my session or update tags from within the session view, **so that** I can organize sessions in context.
**Acceptance Criteria**:
- Tapping "Rename" in the action sheet closes the sheet and opens a rename modal
- The rename modal is a centered Modal (not bottom-anchored), to avoid keyboard-pushes-sheet pitfall
- Rename input has `font-size: 16px` to prevent iOS zoom
- Tapping "Edit Tags" closes the sheet and opens the existing `TagEditor` component
- On save: calls `renameSession(session.id, newTitle)` or `updateSession(session.id, {tags})` respectively
- Empty title is rejected with inline validation error
**Files**:
- `web-app/src/components/sessions/SessionDetail.tsx`
- `web-app/src/components/sessions/TagEditor.tsx` (read-only, used as-is)

##### Task 1.4.3a: Add rename modal state and inline RenameModal component (~5 min)
- Add `const [showRenameModal, setShowRenameModal] = useState(false)` and `const [renameValue, setRenameValue] = useState(session.title)`
- Import or inline a small `RenameModal` — a Modal with a single `<input>` and Save/Cancel buttons
- The input: `<input value={renameValue} onChange={e => setRenameValue(e.target.value)} style={{ fontSize: '16px' }} />`
- On save: guard `if (!renameValue.trim()) { show error }` then call `renameSession(session.id, renameValue.trim())`
- Files: `web-app/src/components/sessions/SessionDetail.tsx`

##### Task 1.4.3b: Integrate TagEditor into SessionDetail (~3 min)
- Add `const [showTagEditor, setShowTagEditor] = useState(false)` to SessionDetail
- Conditionally render `<TagEditor tags={session.tags} onSave={(tags) => { updateSession(session.id, {tags}); setShowTagEditor(false); }} onCancel={() => setShowTagEditor(false)} sessionTitle={session.title} />` when `showTagEditor`
- Import `useSessionService` `updateSession` if not already imported
- Files: `web-app/src/components/sessions/SessionDetail.tsx`

---

### Epic 1.5: Bottom Navbar Content Clearance

**Goal**: The bottom navigation bar does not obscure page content on any screen. All pages have correct `padding-bottom` to clear the nav.

#### Story 1.5.1: Fix bottom navbar safe-area padding and page-level padding-bottom
**As a** mobile user, **I want** all content to be visible above the bottom nav bar, **so that** I never lose the last list item or action button behind the nav.
**Acceptance Criteria**:
- `BottomNav` height is a CSS custom property `--bottom-nav-height` (e.g. `56px`) defined in `:root`
- Each page that scrolls (session list, logs, etc.) has `padding-bottom: calc(var(--bottom-nav-height) + var(--safe-area-bottom))`
- `BottomNav` itself has `padding-bottom: max(env(safe-area-inset-bottom, 0px), 8px)` so nav items clear the home indicator
- Terminal view (`SessionDetail`) is not affected — it already fills full height with `overflow: hidden`
**Files**:
- `web-app/src/app/globals.css`
- `web-app/src/components/layout/BottomNav.css.ts` (or `.module.css`)
- `web-app/src/app/page.module.css` (session list page)
- `web-app/src/app/logs/page.module.css` (if it exists)

##### Task 1.5.1a: Define `--bottom-nav-height` CSS custom property (~2 min)
- Add `--bottom-nav-height: 56px` to `:root` in `globals.css`
- Files: `web-app/src/app/globals.css`

##### Task 1.5.1b: Apply padding-bottom to BottomNav for safe-area (~3 min)
- In `BottomNav.css.ts` (preferred) or `.module.css`:
  - `paddingBottom: 'max(env(safe-area-inset-bottom, 0px), 8px)'`
  - The BottomNav total height thus becomes `56px + safe-area-inset-bottom` on iOS
- Files: `web-app/src/components/layout/BottomNav.css.ts`

##### Task 1.5.1c: Add page-level padding-bottom to scrolling pages (~3 min)
- In `page.module.css` (session list) and any other scrollable page CSS:
  - Add `.pageContent { padding-bottom: calc(var(--bottom-nav-height) + max(env(safe-area-inset-bottom, 0px), 0px)); }`
- Verify on session list and logs pages; check for overflow: scroll containers that need the padding
- Files: `web-app/src/app/page.module.css`, `web-app/src/app/logs/page.module.css`

---

### Epic 1.6: Touch Target Audit and Fixes

**Goal**: All interactive elements in the terminal and session view meet the 44px minimum touch target required for comfortable one-handed mobile use.

#### Story 1.6.1: Apply `--min-touch-target` to mobile keyboard buttons and toolbar controls
**As a** mobile user, **I want** all buttons to be large enough to tap accurately, **so that** I don't accidentally trigger the wrong action while using the terminal one-handed.
**Acceptance Criteria**:
- All `.mobileKey` buttons in `TerminalOutput` have `min-height: 44px; min-width: 44px`
- The toolbar toggle button has `min-height: 44px; min-width: 44px`
- The ⋯ actions button in `SessionDetail` header has `min-height: 44px; min-width: 44px`
- The `--min-touch-target: 44px` CSS variable defined in `globals.css` is referenced by the above rules
**Files**:
- `web-app/src/components/sessions/TerminalOutput.module.css`
- `web-app/src/components/sessions/SessionDetail.module.css`

##### Task 1.6.1a: Apply min touch targets to mobile keyboard buttons (~3 min)
- In `TerminalOutput.module.css`, update `.mobileKey`:
  ```css
  min-height: var(--min-touch-target, 44px);
  min-width: var(--min-touch-target, 44px);
  display: flex;
  align-items: center;
  justify-content: center;
  ```
- Files: `web-app/src/components/sessions/TerminalOutput.module.css`

##### Task 1.6.1b: Verify all SessionDetail header buttons meet 44px (~2 min)
- Inspect `styles.moreActionsBtn`, the fullscreen toggle, and close/back buttons in `SessionDetail.module.css`
- Add `min-height: var(--min-touch-target, 44px); min-width: var(--min-touch-target, 44px)` to any that are missing it
- Files: `web-app/src/components/sessions/SessionDetail.module.css`

---

#### Story 1.6.2: Add xterm.js visualViewport resize loop guard
**As a** developer, **I want** the terminal resize handler to be loop-safe, **so that** keyboard open/close on iOS does not cause flickering or layout instability.
**Acceptance Criteria**:
- `TerminalOutput.tsx` has an `isFittingRef = useRef(false)` guard around the `fit()` call
- The `visualViewport` resize handler checks `isFittingRef.current` before calling `fit()` and sets it to `true` before, clears in `requestAnimationFrame` after
- Debounce on mobile is increased to 400ms (vs 300ms on desktop)
**Files**:
- `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.6.2a: Add isFittingRef guard to visualViewport resize handler (~3 min)
- Add `const isFittingRef = useRef(false)` to TerminalOutput
- Wrap the `fit()` call in the viewport resize handler:
  ```ts
  if (isFittingRef.current) return;
  isFittingRef.current = true;
  setTimeout(() => {
    xtermRef.current?.fit();
    requestAnimationFrame(() => { isFittingRef.current = false; });
  }, isMobile ? 400 : 300);
  ```
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

---

## Milestone 2 — Design System Foundation

**Goal**: Establish a shared design token contract and primitive component library. Migrate the most-used CSS Module files to vanilla-extract. This is a 3–6 month window; epics are higher-level.

---

### Epic 2.1: Vanilla-Extract Token Contract

**Goal**: Establish a full `createThemeContract` token system in `web-app/src/styles/theme.css.ts` covering all color, spacing, typography, and radii tokens currently scattered across `globals.css` and inline CSS.

#### Story 2.1.1: Define full `createThemeContract` in theme.css.ts
**As a** developer, **I want** all design tokens in a typed vanilla-extract contract, **so that** I get compile-time safety and can never reference an undefined token.
**Acceptance Criteria**:
- `theme.css.ts` exports `vars` with full token groups: `color`, `space`, `fontSize`, `fontWeight`, `radii`, `shadow`, `breakpoint`, `zIndex`
- All tokens currently in `globals.css` `:root` are represented (not duplicated — globals.css light/dark values set the contract implementation)
- The token contract is used by all new `.css.ts` files
**Files**:
- `web-app/src/styles/theme.css.ts`
- `web-app/src/app/globals.css`

---

### Epic 2.2: Primitive Component Library

**Goal**: Build Button, Modal/BottomSheet, Card, Input, ActionBar, and Badge as shared primitives using vanilla-extract + Radix UI (ADR-010).

#### Story 2.2.1: Button primitive with variants and sizes
#### Story 2.2.2: Modal primitive (Radix Dialog + globals.css bottom-sheet behavior for mobile)
#### Story 2.2.3: Input primitive with 16px font-size enforcement (iOS anti-zoom)
#### Story 2.2.4: Card primitive
#### Story 2.2.5: Badge primitive for session status display
#### Story 2.2.6: ActionBar primitive — refactor `ActionBar.tsx` to use vanilla-extract `.css.ts`

*(Each story follows the same epic/story/task breakdown as Milestone 1; detailed task decomposition to happen as each story is scheduled for implementation.)*

---

### Epic 2.3: CSS Module Migration (Top-10 Most-Used Files)

**Goal**: Migrate the 10 most-used `.module.css` files to `.css.ts` to eliminate the dual-system coexistence.

**Candidate files** (to be confirmed against usage frequency):
1. `SessionCard.module.css`
2. `SessionList.module.css`
3. `SessionDetail.module.css`
4. `TerminalOutput.module.css`
5. `ActionBar.module.css`
6. `Header.module.css`
7. `BottomNav.module.css`
8. `Modal.module.css`
9. `page.module.css`
10. `FilterBar.module.css`

Each file gets its own story. Migration must be non-breaking — CSS class names in TSX stay the same, only the source of truth changes from `.module.css` to `.css.ts`.

---

### Epic 2.4: Responsive Header

**Goal**: The app header works correctly at all widths including the 800–1100px intermediate breakpoint (per `responsive-nav-actionbars` absorbed plan).

#### Story 2.4.1: Header breakpoint at 800px collapses secondary navigation
#### Story 2.4.2: Header breakpoint at 1100px restores full nav

*(Detailed task decomposition at implementation time.)*

---

## Milestone 3 — UX Best Practices Alignment

**Goal**: Systematic review and alignment to established mobile UX patterns. Depends on Milestone 2 design system being in place.

---

### Epic 3.1: Navigation Patterns Review

**Goal**: Review all navigation patterns against the established patterns in Linear, Vercel, and GitHub Mobile. Ensure session list ↔ session view ↔ action sheet navigation is consistent and intuitive.

#### Story 3.1.1: Navigation audit document and gap list
#### Story 3.1.2: Implement any navigation gaps identified (per audit)

---

### Epic 3.2: Consistent Action Surface

**Goal**: The same set of session actions is available from both the session list (SessionCard) and session view (SessionDetail action sheet). No action requires the user to know which surface to use.

#### Story 3.2.1: Audit action parity between SessionCard and SessionDetail action sheet
#### Story 3.2.2: Add any missing actions to SessionDetail action sheet (Clone, Create Checkpoint)
#### Story 3.2.3: Refactor shared action logic into a `useSessionActions(sessionId)` hook

---

### Epic 3.3: Interaction Latency Instrumentation

**Goal**: Click-to-render and RPC durations are observable via OpenTelemetry for all primary session actions.

#### Story 3.3.1: Instrument RPC calls in `useSessionService` with OTel spans
**As a** developer, **I want** to see how long each session action RPC takes, **so that** I can identify and fix latency regressions.
**Acceptance Criteria**:
- `deleteSession`, `pauseSession`, `resumeSession`, `renameSession`, `updateSession` emit OTel spans with duration
- Span attributes include `session.id`, `session.status`, `rpc.method`
- Spans visible in existing OTel pipeline (already configured per CLAUDE.md)
**Files**:
- `web-app/src/hooks/useSessionService.ts`

#### Story 3.3.2: Instrument click-to-render latency for session list load
#### Story 3.3.3: Add Web Vitals (LCP, CLS, FID) reporting to OTel for mobile

---

## Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| Bottom sheet + iOS keyboard: text inputs hidden | High | **Resolved in Milestone 1**: bottom sheet is action-only (no text inputs); Rename and Tag Edit open as separate centered modals (pitfalls ADR guidance) |
| xterm.js visualViewport resize loop | Medium | Story 1.6.2: `isFittingRef` guard + 400ms mobile debounce |
| Radix Portal z-index conflict with xterm Canvas | Medium | Never apply `transform`/`will-change` to xterm container; Modal uses Radix Portal which already handles z-index; verify `SessionDetail.css.ts` has no transform animations |
| dvh sub-pixel rounding gap on iOS | Low | Use `var(--viewport-height)` (integer px from visualViewport) not bare `100dvh`; add `overflow: hidden` |
| Android 3-button nav: `env(safe-area-inset-bottom)` = 0 | Low | Always use `max(env(safe-area-inset-bottom, 0px), 8px)` pattern |
| iOS input zoom on font-size < 16px | Low | All text inputs in modals must have `font-size: 16px`; enforced in Story 1.4.3a |
| CSS Module migration breaks styling | Medium | Migrate one file at a time in Milestone 2; visual regression test each before merging |

---

## PR Sequencing (Milestone 1)

Each story maps to one PR. Recommended order:

1. **PR 1** — Epic 1.2, Story 1.2.1: `100vh` audit + globals.css/page.module.css migration (no behavior change, safe first PR)
2. **PR 2** — Epic 1.2, Story 1.2.2 + 1.2.3: SessionDetail height fix + safe-area insets (keyboard-aware layout)
3. **PR 3** — Epic 1.5: Bottom navbar clearance (quick win, no dependencies)
4. **PR 4** — Epic 1.1: Toolbar toggle (isolated to TerminalOutput)
5. **PR 5** — Epic 1.3: Session name visibility (depends on PR 2 header stickiness)
6. **PR 6** — Epic 1.4, Stories 1.4.1 + 1.4.2: Action sheet + Delete/Pause/Resume
7. **PR 7** — Epic 1.4, Story 1.4.3: Rename + Tag Edit wiring
8. **PR 8** — Epic 1.6: Touch targets + resize loop guard

Each PR adds one semver label (`patch` for fixes, `minor` for the action sheet feature).
