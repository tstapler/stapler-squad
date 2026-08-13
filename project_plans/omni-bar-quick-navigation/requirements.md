# Requirements: Omni Bar Quick Navigation

**Date**: 2026-04-21
**Domain**: stapler-squad (personal tool)
**Type**: feature addition + cleanup

---

## Problem Statement

The omnibar (`Cmd+K`) has a working fuzzy search layer (session discovery + repo recent-pick) but the **creation flow is still too slow**. When a developer realizes mid-session they need to work on the same repo in a different context (new worktree, same directory, parallel branch), the journey from "I know what I want" to "session is created" is 5–6 steps. The user must navigate multiple form fields, redundant action options (fork/duplicate/clone) confuse the session list, and there is no keyboard-driven way to adjust creation options (session type, worktree vs directory) without reaching for the mouse.

Additionally, the architecture has no enforcement mechanism ensuring that new app features register omnibar actions — meaning the command palette will drift out of sync as the product grows.

The TUI is dead code that should be removed.

---

## Users / Consumers

- **Tyler** (solo practitioner): Heavy multi-session developer; context-switches between parallel worktrees frequently; strong keyboard-first preference (Vim, IntelliJ, VS Code command palette user).

---

## Success Metrics

1. **Speed**: Creating a new session on a previously-used repo takes ≤2 keystrokes after the repo is identified (e.g., select repo result → `Tab` to pick session type → `Enter` to confirm).
2. **Navigation**: Arrow-key and Tab navigation fully replaces mouse in the discovery → creation flow.
3. **Mode clarity**: The user can tell at a glance whether they're in "jump to session" mode vs "create session" mode, and can toggle between them without typing.
4. **Architecture**: Any new action added to the app has a defined registration point in the omnibar action registry; the pattern prevents silent omission.
5. **Cleanup**: TUI code is removed; fork/duplicate/clone redundancy is resolved to a single "clone" action surfaced through the omnibar.
6. **Performance**: All result updates remain <100ms; omnibar open-to-first-result is <100ms.

---

## Scope

### In Scope

#### 1. Keyboard Navigation Completeness
- Arrow keys (↑↓) navigate the result list in discovery mode — **already partially implemented**; verify completeness and fix any gaps.
- `Tab` from discovery mode result toggles into creation mode with that result's repo pre-selected.
- `Tab` within creation mode cycles through session type options (new worktree → directory → existing worktree).
- Keyboard shortcuts shown in the footer update dynamically based on current mode and selection state.
- `Escape` behavior: first press clears result highlight; second press closes the omnibar.

#### 2. Fast Session Creation Addon (Inline Creation Panel)
- When a repo is selected from the result list (via Enter or Tab), the creation form appears **inline within the omnibar modal** as a compact addon panel — not a full separate form.
- The addon panel shows only the essential fields with smart defaults:
  - Session type selector (new worktree / directory / existing worktree) — keyboard-navigable with Tab or arrow keys
  - Branch name (pre-filled from title, editable) — shown only for new worktree
  - Program (pre-filled from last used for this repo, or "claude" default)
- `Cmd+Enter` (or just `Enter` when no result is highlighted) creates the session.
- Advanced options (category, auto-yes, prompt, etc.) collapse behind a disclosure — accessible but not shown by default.

#### 3. Mode Modifier Shortcuts
- `Cmd+N` from anywhere in the omnibar (open or closed) opens omnibar in creation mode directly, bypassing discovery.
- `new/` prefix in the search input forces creation mode (type "new/stapler" → creates a new session against the stapler repo path).
- A visible mode badge or toggle button shows the current mode (Jump | Create) and is clickable to switch.

#### 4. Omnibar Action Registry (Architecture)
- Define a typed `OmnibarAction` registration interface that new features implement.
- Actions can be: navigation (jump-to), mutation (create/delete/update), or command (arbitrary callback).
- The existing result list renders registered actions alongside session/repo results.
- A lint rule or architectural guard (TypeScript discriminated union) ensures no silent omission when adding new action types.
- Initial actions to register: "New Session", "Open Session", "Delete Session", "Pause Session", "Resume Session".

#### 5. Session List Cleanup
- Remove or consolidate the "fork", "duplicate", "clone" context-menu options from the session list into a single **"Clone session"** action.
- Surface "Clone session" as an omnibar action (registered via the action registry).
- The session card contextual actions should be: Open, Pause/Resume, Clone, Delete — four actions, no redundancy.

#### 6. TUI Removal
- Delete all TUI-specific Go code (BubbleTea model, TUI-specific views, TUI routing).
- Remove TUI-related CLI flags and entry points.
- Ensure the web server startup path is unaffected.
- Update `CLAUDE.md` to remove TUI-specific documentation.

---

### Out of Scope

- Global OS-level hotkey (system-wide `Cmd+K` outside the browser tab).
- AI/LLM-powered action suggestions.
- Mobile/responsive layout changes (covered by `mobile-ux-improvements` project).
- Multi-window or multi-tab synchronization.
- Undo/redo for session actions.
- Session action history/audit log.

---

## Constraints

**Tech stack (already decided):**
- Frontend: React + TypeScript, vanilla-extract CSS, Fuse.js (already installed), ConnectRPC
- Backend: Go, ConnectRPC/Protobuf
- Existing `Omnibar.tsx` with discovery/creation mode state, `OmnibarResultList`, `useSessionSearch`, `usePathHistory` all exist and work.

**Builds on top of:**
- `omni-bar-session-search` (complete) — fuzzy search, two-phase discovery/creation mode, result list components
- `omni-bar-path-completion` (complete) — path completion, directory cache, worktree suggestions

**Performance constraint:** All result updates <100ms; omnibar open-to-first-result <100ms.

**Keyboard shortcut constraint:** `Cmd+K` is the primary trigger (already implemented). `Cmd+N` for direct creation mode must not conflict with any existing browser or app shortcuts.

**Backward compatibility:** Existing mouse-driven session creation flow (the full form) must remain accessible — the inline addon is a fast path, not a replacement.

---

## Open Questions

1. **Mode badge position**: Should the "Jump | Create" mode toggle live inside the input row (left of the text field) or below it (as a tab strip)?
2. **`new/` prefix conflict**: Does the `new/` prefix collide with any existing path completion patterns (e.g., a directory literally named `new`)?
3. **Action registry enforcement**: TypeScript compile-time guard (discriminated union + exhaustive switch) or runtime assertion? Compile-time preferred but may require refactor.
4. **TUI removal scope**: Are there any features in the TUI that are NOT replicated in the web UI that need to be ported first? (Needs audit before deletion.)
5. **Clone vs fork/duplicate naming**: What's the right verb? "Clone session" (spawns a new session on the same repo) seems clearest but needs confirmation.
