# Cockpit Tiling Layout — Requirements

## Problem Statement

The stapler-squad web UI currently has a fixed 3-column layout (session list | terminal/detail | optional context panel). Users cannot resize these columns or split the detail area into multiple panes. Power users working with multiple sessions simultaneously have no way to view them side-by-side, and there are no resize handles to adjust the layout for different work styles.

## Goal

Implement a tmux/i3-style free-split tiling layout engine inside the cockpit detail area, enabling users to view multiple sessions (each showing any tab: Terminal, Diff, Logs, VCS, Info) side-by-side. Columns must be resizable with drag handles. Layout persists across page reloads.

---

## User Stories

### US-1: Resize the session list column
**As a** user on desktop  
**I want to** drag the boundary between the session list and the detail area  
**So that** I can give more or less space to the session list

**Acceptance criteria:**
- A visible drag handle (≥6px hit target, visual indicator) sits on the border between the session list column and detail area
- Dragging adjusts the session list column width in real time
- Minimum width: 160px (list column), 320px (detail area)
- Maximum width: 50% of viewport
- Width is stored in `localStorage` under key `cockpit.listColumnWidth`
- On page reload, the stored width is applied before first paint (no layout shift)

### US-2: Split the detail area vertically (side-by-side sessions)
**As a** power user  
**I want to** split the detail pane into two or more panes side-by-side  
**So that** I can monitor multiple sessions simultaneously

**Acceptance criteria:**
- Keyboard shortcut `Ctrl+\` splits the focused pane vertically (left | right)
- Keyboard shortcut `Ctrl+-` splits the focused pane horizontally (top | bottom)
- Each pane independently shows a session + tab (Terminal, Diff, Logs, VCS, Info)
- A pane can be closed with `Ctrl+W` (or a close ✕ button per pane)
- The focused pane has a visible highlight border using the theme primary color
- `Ctrl+→ / ← / ↑ / ↓` moves focus between panes
- Arrow-key resize: `Ctrl+Alt+→ / ← / ↑ / ↓` nudges the active split boundary by 20px

### US-3: Assign a session to a pane
**As a** user with multiple panes  
**I want to** click a session in the list and have it open in the focused pane  
**So that** I can fill each pane with a different session

**Acceptance criteria:**
- Clicking a session in the left panel opens it in the currently focused pane (not always column 2)
- An omnibar command `split <session-title>` opens the named session in a new split
- Each pane shows the session title + active tab in its header bar
- Pane header height: 32px (compact)

### US-4: Drag-resize between panes
**As a** user with multiple panes  
**I want to** drag the handle between panes to resize them  
**So that** I can give more space to the pane I'm focused on

**Acceptance criteria:**
- Drag handles appear between adjacent panes (horizontal splits get a horizontal handle, vertical gets vertical)
- Handle is 6px wide/tall with a visual indicator (chevrons or dotted line)
- Dragging is smooth (rAF-based, no jank)
- Minimum pane size: 200px × 150px
- Sizes are saved per layout configuration in `localStorage` under `cockpit.paneLayout`

### US-5: Mobile touch support
**As a** mobile user  
**I want to** pinch/drag resize handles on touch  
**So that** I can adjust the layout on my phone

**Acceptance criteria:**
- Touch drag on any resize handle works identically to mouse drag
- Touch hit target is at least 20px (larger than the visual 6px to be usable with fingers)
- On viewport < 768px, vertical splits stack to a single pane (no side-by-side — too narrow); a tab row at the bottom allows switching between stacked panes
- Horizontal splits remain available on mobile (stacked sections with touch-drag resize)

### US-6: Layout persistence
**As a** user who customizes their layout  
**I want to** my layout to survive page reloads  
**So that** I don't have to re-configure after every visit

**Acceptance criteria:**
- Full pane tree (split directions, sizes, which session + tab each pane shows) is serialized to `localStorage` key `cockpit.paneLayout` on every layout change
- On load, the stored layout is restored; if a stored session ID no longer exists, that pane shows an empty state ("Session not found — click a session to load it")
- A "Reset layout" button in the header resets to the default single-pane view and clears localStorage

### US-7: Keyboard split/navigate mirrors tmux defaults
**As a** user familiar with tmux  
**I want to** use familiar keyboard shortcuts  
**So that** I don't have to learn a new key map

| Action | Shortcut |
|--------|----------|
| Split vertical | `Ctrl+\` |
| Split horizontal | `Ctrl+-` |
| Close pane | `Ctrl+W` |
| Focus right | `Ctrl+→` |
| Focus left | `Ctrl+←` |
| Focus up | `Ctrl+↑` |
| Focus down | `Ctrl+↓` |
| Resize right | `Ctrl+Alt+→` |
| Resize left | `Ctrl+Alt+←` |
| Resize up | `Ctrl+Alt+↑` |
| Resize down | `Ctrl+Alt+↓` |
| Zoom pane (fullscreen) | `Ctrl+Z` |

All shortcuts must be registered through the existing `ShortcutRegistry` and appear in the `?` keyboard overlay.

---

## Technical Constraints

- **Framework**: React 18, Next.js 15, TypeScript
- **CSS**: vanilla-extract only for new styles (no CSS modules, no inline styles except for dynamic values via CSS custom properties)
- **State**: React `useState`/`useReducer` for the pane tree; no new global state library
- **Persistence**: `localStorage` only (no server round-trip)
- **No new dependencies** beyond what's already in `package.json` unless strictly necessary
- **Theme tokens**: all colors/spacing/radii from `vars.*` in `theme.css.ts`
- **Tests**: Jest + RTL unit tests for the pane tree reducer; no Playwright e2e required for MVP
- **Feature registry**: add entries to `docs/registry/features/frontend/` after implementation

---

## Non-Goals (MVP)

- Cross-device layout sync (server persistence)
- Named/saved layout presets ("save layout as 'debug view'")
- Drag-and-drop reordering of panes (resize only)
- Terminal-level splits (xterm.js splits within a single terminal pane)
- Context panel (3rd column) splitting

---

## Success Metrics

1. User can open 2 sessions side-by-side with keyboard in < 5 seconds
2. Drag resize works smoothly on desktop and mobile (no visible jank)
3. Layout survives a hard page reload with no visible shift
4. All shortcuts appear in the `?` shortcut overlay
5. `make ci` passes (no regressions)
