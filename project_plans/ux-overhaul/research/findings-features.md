# Features Research: UX Overhaul

## 1. Session Actions Audit — What Exists on Sessions List vs. Session View

### Actions Available on SessionList (via SessionCard)
From `SessionList.tsx` props interface and `SessionCard` usage:
- **Delete** (`onDeleteSession`) — triggers `deleteSession(id)` via `useSessionService`
- **Pause** (`onPauseSession`) — triggers `pauseSession(id)`
- **Resume** (`onResumeSession`) — triggers `ResumeSessionModal` (modal with optional title/tag updates)
- **Clone** (`onCloneSession`) — creates a clone of a session
- **New Workspace** (`onNewWorkspaceSession`) — creates new worktree from existing session
- **Rename** (`onRenameSession`) — inline rename via `SessionCard`
- **Restart** (`onRestartSession`) — restart a session
- **Update Tags** (`onUpdateTags`) — opens `TagEditor` modal
- **Create Checkpoint** (`onCreateCheckpoint`) — named checkpoint
- **List Checkpoints** (`onListCheckpoints`) — view/fork checkpoints
- **Bulk Actions** (multi-select): Pause All, Resume All, Stop All, Delete All, Add Tag to All

### Actions Available on SessionDetail (Session View)
From `SessionDetail.tsx`:
- **Switch Workspace** (⎇ Switch button) — opens `WorkspaceSwitchModal` for branch/worktree switching
- **Edit Working Directory** (inline, Info tab)
- **Edit Program** (inline select, Info tab)
- **Fullscreen toggle** (terminal/diff/vcs tabs)
- **Review queue navigation** (prev/next)
- **Dismiss from queue**

### MISSING from SessionDetail (the critical gap):
- Delete session
- Pause session
- Resume session (no ResumeSessionModal accessible)
- Rename session (title is display-only in the header)
- Update Tags
- Clone session
- Create Checkpoint (only visible from SessionCard)
- Restart session

**The session name/title in `SessionDetail` is rendered as a plain `<h2>` — not editable.**

## 2. ActionBar Usage Audit

`ActionBar` is used in 3 places currently:
1. **`SessionDetail.tsx`** — header actions bar (fullscreen, nav, close buttons)
2. **`SessionList.tsx`** — collapsible filter controls bar (status, category, tag, group, sort selectors)
3. **`app/logs/page.tsx`** — log viewer toolbar
4. **`HistoryFilterBar.tsx`** — history search/filter controls

`ActionBar.tsx` supports: `gap`, `justify`, `scroll`, `compact` props. It renders as a `<div>` with flex layout. The `compact` prop reduces gap and enables horizontal scroll at 768px — this is the right primitive for any toolbar that must work on mobile.

**Usage gap**: `TerminalOutput.tsx` has its own `styles.toolbar` / `styles.actions` structure that does NOT use `ActionBar`. The terminal toolbar is a manual flex layout. Migrating it to `ActionBar` would normalize behavior.

## 3. Industry Patterns — Bottom Sheet / Action Menus

### Linear (project management)
- Session-level actions appear in a **3-dot (⋯) overflow menu** in the issue detail header, not a bottom sheet.
- On mobile, the overflow menu slides up as a bottom sheet with large touch targets.
- Actions: Change Status, Assign, Add Label, Move to Cycle, Delete, Archive, Duplicate.
- Key pattern: **destructive actions (Delete) are at the bottom of the sheet, visually separated with a red color**.

### Vercel Dashboard
- Project/deployment actions use an **action menu dropdown** (not bottom sheet) on desktop.
- On mobile, the same actions appear as a bottom sheet that slides up from below the nav bar.
- Actions per deployment: Visit, Inspect, Redeploy, Cancel, Delete.
- Key pattern: **Redeploy / Cancel are primary; Delete is destructive, visually demoted and separated**.

### GitHub Mobile
- Issue/PR actions use a **context sheet** that slides up with sections: primary actions, secondary actions, destructive.
- Long-press on item cards triggers the sheet.
- Actions: Edit, Comment, Close/Reopen, Lock, Delete.
- Key pattern: **The sheet uses grouped sections with labels** ("Actions", "Danger Zone").

### Common Patterns Across All Three
1. **One "⋯" or "..." button in the header/toolbar** triggers the action menu/sheet.
2. **Sheet anatomy**: Handle/drag indicator at top → title → action list → cancel.
3. **Destructive actions are red, at the bottom, and require confirmation**.
4. **Sheet appears above the bottom nav** — z-index layering is critical.
5. **Swipe down to dismiss** or tap outside.

## 4. Unstated User Needs (Beyond Explicit Requirements)

### Contextual Actions (from primary workflow)
The primary workflow is: receive push notification → tap → land in session view → decide what to do. This means:
- **Quick status assessment**: Is the session still running? Does it need attention? → Session name + status badge must be *instantly* visible.
- **One-tap Pause**: For sessions waiting for user input that the user isn't ready for yet. This is the highest-frequency action from the session view.
- **One-tap Delete**: For completed sessions the user wants to clean up immediately after review.
- **Edit title inline in header**: The session name may have been auto-generated and needs cleanup after work is done. Users won't navigate to a separate "info" tab just to rename.

### Tag/Category in Session View
- Tags are useful for organizing sessions into projects, but editing them is only available from the list. Users who open a session via notification and realize it needs a tag have no way to add it without leaving.

### Terminal + Action Coexistence
- The session actions should be reachable **without leaving the terminal view**. A bottom sheet that slides up over the terminal (not replacing it) is the right pattern.
- The sheet must dismiss when the user taps outside it, returning them to the terminal immediately.

### Edge Cases
- **Pausing an external session** (mux socket): The pause/resume behavior is undefined for external sessions — the action menu should hide or grey out Pause/Resume for `InstanceType.EXTERNAL`.
- **Deleting a running session**: The current `deleteSession(id)` does not confirm if the session is running. A confirmation dialog is needed ("Session is running. Stop and delete?").
- **Rename with empty title**: The `renameSession` call should be guarded against empty string input.
- **Tag editor on small screen**: The `TagEditor` modal has an input + tag list that may overflow on 375px screens.

## Summary

- **Critical gap**: SessionDetail has no Delete, Pause/Resume, Rename, or Update Tags — all of these exist in `useSessionService` and are already wired to SessionList; they need to be plumbed into a new session action sheet in SessionDetail.
- **ActionBar is underused**: The terminal toolbar (`TerminalOutput`) has its own flex layout that should be refactored to use ActionBar for responsive scroll behavior.
- **Industry pattern**: A single "⋯" button in SessionDetail's header, opening a bottom sheet with grouped actions (primary / destructive), is the established mobile pattern — matching Linear, Vercel, and GitHub Mobile.
- **User need**: Inline title editing in the session header is a high-frequency need that will be missed if only available via the Info tab.
