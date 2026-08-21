# Requirements: terminal-copy-select

**Date**: 2026-05-30
**Type**: bug fix + feature addition

## Problem Statement

Clicking and selecting text in the terminal causes a React re-render (triggered by
`setCopyButtonPos` state update inside `terminal.onSelectionChange`), which disrupts
the selection in progress and prevents right-click from working on the selected text.
All users experience this on every session where they try to copy terminal output.

The current floating "Copy" button approach creates a re-render loop:
- `xterm.onSelectionChange` → `setCopyButtonPos` → React re-render → selection lost or stutters
- Right-click after selection fails because the selection state is cleared by the re-render

Beyond fixing the bug, the goal is a best-in-class experience that surpasses native
terminal emulators (iTerm2, Terminal.app, Windows Terminal) on both desktop and mobile.

**Stack context:**
- Terminal renderer: xterm.js v6 (`@xterm/xterm`) with WebGL addon, hosted in React (Next.js)
- Mobile gesture handling: `useTerminalGestures` hook (5-state machine: IDLE/PENDING/SCROLLING/SELECTING/TAPPING)
- Existing floating Copy button: `XtermTerminal.tsx` lines 104–503 — this is the broken path

## Users / Consumers

Desktop users (mouse/trackpad, macOS/Linux/Windows) and mobile users (iOS/Android touch)
who use the terminal to view and copy AI agent output, command output, and log streams.
All users across all sessions.

## Success Metrics

1. **Bug fixed**: Clicking and dragging to select text does NOT cause a visible re-render
   or reset the selection. Right-click on selected text shows the browser context menu with
   "Copy" option intact.
2. **No regression**: Scrolling, keyboard input, ANSI rendering, session management, and
   the resize/fit system are unaffected.
3. **Performance ceiling**: Selection state changes add <1ms overhead per event to terminal
   rendering (no synchronous DOM layout during selection).
4. **Desktop excellence**: Selection persists across mousedown→mousemove→mouseup. Double-click
   selects a word. Triple-click (or shift+click) selects a line. Cmd/Ctrl+C copies selection.
   Right-click on selection shows context menu (Copy, Select All).
5. **Mobile excellence**: Long-press (400ms) initiates selection with haptic feedback. Drag
   handles appear at selection boundaries for precise adjustment. Double-tap selects a word.
   Floating "Copy" button appears above selection without triggering a re-render.
6. **Clipboard integration**: Copied text is available in the system clipboard. Works in
   iOS Safari (requires synchronous clipboard write in user gesture handler — already partially
   implemented in the current code via `onPointerDown`).

## Constraints

- **Must not break**: scrolling, keyboard input, ANSI rendering, resize/FitAddon, session
  management, the `TerminalStreamManager` scrollback system, WebGL renderer.
- **Performance**: Selection overlay/UI changes must not cause synchronous React re-renders
  during active mouse drag. Use refs, CSS, or canvas overlays instead of React state for
  transient selection UI.
- **No terminal library swap**: xterm.js stays. Fix must work with xterm.js v6 APIs.
- **iOS Safari clipboard constraint**: Clipboard writes must happen synchronously inside a
  user gesture handler (existing `onPointerDown` pattern is correct — preserve it).
- **Xterm.js mouse tracking mode**: When a PTY sets mouse tracking mode (vim, tmux),
  xterm.js intercepts mouse events and selection via the normal mechanism may be suppressed.
  The fix must handle both tracking-mode-on and tracking-mode-off cases.

## Scope

### In Scope

- Fix the re-render-on-selection bug (root cause: `setCopyButtonPos` useState in
  `onSelectionChange` callback in `XtermTerminal.tsx`)
- Right-click context menu (Copy, Select All) on desktop without re-render
- Floating Copy button (mobile + desktop) that appears without triggering React re-render
  (use CSS/DOM directly or a portal with stable identity)
- Double-click word selection
- Triple-click line selection
- Cmd/Ctrl+C keyboard shortcut copies selection to clipboard
- Mobile: long-press selection with drag handles, double-tap word selection
- "Select All" keyboard shortcut (Cmd/Ctrl+A) selects all terminal buffer content
- Visual selection highlight that matches terminal theme (already provided by xterm.js)
- Regression tests covering the re-render bug and the keyboard shortcut paths

### Out of Scope

- Changing the terminal library (xterm.js stays)
- Persistent selection toolbar/UI chrome that is always visible
- Custom ANSI-aware copy (copy with ANSI escape codes stripped — xterm already does this)
- Search-and-replace within terminal buffer
- Remote clipboard sync (for SSH sessions)

## Open Questions

1. **Re-render root cause depth**: Does `setCopyButtonPos` in `onSelectionChange` always
   cause the full `XtermTerminal` component to re-render, or does React batch it? Needs
   profiling confirmation. Alternative fix: move Copy button to a sibling component using
   a shared ref (no state in XtermTerminal).

2. **Right-click context menu**: Should we use a custom DOM context menu (full control,
   consistent cross-browser) or rely on the browser's native contextmenu event + system
   clipboard? Custom menu requires a portal; native is simpler but less styleable.

3. **xterm.js selection API**: Does xterm.js v6 expose a public `onRightClick` event or
   do we need to listen to `contextmenu` on the `.xterm-screen` element? Research needed.

4. **Mobile drag handles**: xterm.js does not natively render drag handles for touch
   selection. Should we implement custom CSS-positioned handles (complex but polished) or
   rely on the long-press → drag approach already partially implemented?

5. **Cmd/Ctrl+C conflict**: In terminal mode, Ctrl+C sends SIGINT. The copy shortcut must
   only activate when there IS a selection (otherwise it should pass Ctrl+C through to the PTY).
   Research how other web terminals (Hyper, Tabby) solve this.
